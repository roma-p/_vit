package clicore

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"vit/internal/types"
)

type FuzzyRequest struct {
	FzfPath    string
	Candidates []string
	Prompt     string
	Header     string
	Query      string   // initial filter text (restores a previous session's typing)
	ExpectKeys []string // keys reported back (besides Enter), e.g. "@"
	Binds      []string // raw fzf key binds, e.g. "backward-eof:abort"
}

type FuzzyResult struct {
	Key       string
	Selection string
	Query     string
}

// FuzzyFind runs one picker and returns the selection + terminating key. A user
// cancel (fzf exit 130) returns a CancelledError.
func FuzzyFind(ctx context.Context, r FuzzyRequest) (FuzzyResult, error) {
	args := []string{"--height=40%", "--reverse", "--border", "--print-query"}
	if r.Prompt != "" {
		args = append(args, "--prompt="+r.Prompt)
	}
	if r.Header != "" {
		args = append(args, "--header="+r.Header)
	}
	if r.Query != "" {
		args = append(args, "--query="+r.Query)
	}
	if len(r.ExpectKeys) > 0 {
		args = append(args, "--expect="+strings.Join(r.ExpectKeys, ","))
	}
	for _, b := range r.Binds {
		args = append(args, "--bind="+b)
	}

	cmd := exec.CommandContext(ctx, r.FzfPath, args...)
	cmd.Stdin = strings.NewReader(strings.Join(r.Candidates, "\n"))
	cmd.Stderr = os.Stderr
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 130 {
			return FuzzyResult{}, types.NewCancelledError(fmt.Errorf("fuzzy selection cancelled"))
		}
		return FuzzyResult{}, fmt.Errorf("fzf failed: %w", err)
	}

	// output layout (--print-query, optionally --expect):
	//   line 0: the typed query (empty line if none)
	//   line 1: the terminating key (only with --expect; empty for Enter)
	//   line N: the selected item
	raw := strings.TrimRight(out.String(), "\n")
	var lines []string
	if raw != "" {
		lines = strings.Split(raw, "\n")
	}
	var res FuzzyResult
	i := 0
	if i < len(lines) { // --print-query is always set
		res.Query = lines[i]
		i++
	}
	if len(r.ExpectKeys) > 0 && i < len(lines) {
		res.Key = strings.TrimSpace(lines[i])
		i++
	}
	if i < len(lines) {
		res.Selection = strings.TrimSpace(lines[len(lines)-1])
	}
	return res, nil
}
