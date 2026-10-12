package cli

import (
	"context"
	"errors"
	"strings"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
	"vit/internal/vendor"
)

// fuzzyFind runs vit's two-stage picker and returns the selected path or ref.
//
//		stage 1: all tree paths. Enter → take it; @ -> select asset object at stage 2.
//		stage 2: select asset object. Enter -> accept it;
//	          backspace on empty -> go back to stage 1
func fuzzyFind(ctx context.Context, client *vit.Client) (string, error) {
	fzfPath, err := vendor.FzfCmd()
	if err != nil {
		return "", err
	}

	c1, err := client.FuzzyCandidates(ctx, ".", "", "")
	if err != nil {
		return "", err
	}

	query := ""
	for {
		r1, err := clicore.FuzzyFind(ctx, clicore.FuzzyRequest{
			FzfPath:    fzfPath,
			Candidates: c1.StringList,
			Prompt:     "path> ",
			Header:     "enter: pick · @: list its branches/tags/snapshots · esc: quit",
			Query:      query,
			ExpectKeys: []string{"@"},
		})
		if err != nil {
			return "", err // Esc/Ctrl-C at the path list → quit.
		}
		query = r1.Query
		sel := strings.TrimSuffix(r1.Selection, "/")
		isDir := sel != r1.Selection
		if r1.Key != "@" {
			return sel, nil // picked a tree path (asset or dir)
		}

		// go stage 2!!
		var objects []string
		if !isDir {
			c2, err := client.FuzzyCandidates(ctx, ".", "@", sel)
			if err != nil {
				return "", err
			}
			objects = c2.StringList
		}
		header := "enter: pick an object · backspace the @ (or esc): back to paths"
		if len(objects) == 0 {
			if isDir {
				header = sel + "/ is a directory — no objects · backspace (or esc) to go back"
			} else {
				header = sel + " has no branches/tags/snapshots · backspace (or esc) to go back"
			}
		}
		r2, err := clicore.FuzzyFind(ctx, clicore.FuzzyRequest{
			FzfPath:    fzfPath,
			Candidates: objects,
			Prompt:     sel + "@",
			Header:     header,
			Binds:      []string{"backward-eof:abort"},
		})
		if err != nil {
			var cancelled *types.CancelledError
			if errors.As(err, &cancelled) {
				continue // back to the (cached) path list, query restored
			}
			return "", err
		}
		if r2.Selection == "" {
			continue // empty state: Enter on nothing → back to the path list
		}
		return r2.Selection, nil
	}
}
