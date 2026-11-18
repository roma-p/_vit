package clicore

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vit"
	"vit/internal/types"
)

type rustTestInner struct {
	Name string `json:"name"`
}

type rustTestNode struct {
	Next *rustTestNode `json:"next"`
}

type rustTestResult struct {
	Plain    string `json:"plain,omitempty"`
	NoTag    int
	Hidden   string `json:"-"`
	hidden   string
	Size     int64                    `json:"size"`
	When     time.Time                `json:"when"`
	Pointer  *rustTestInner           `json:"pointer"`
	Items    []rustTestInner          `json:"items"`
	ByName   map[string]rustTestInner `json:"by_name"`
	Anything any                      `json:"anything"`
	Type     string                   `json:"type"`
	Node     rustTestNode             `json:"node"`
	Renamed  string                   `json:"RepoUID"`
}

func rustTestRun(context.Context, *vit.Client, *CmdParser) (types.Result, error) {
	return nil, nil
}

func rustTestTree() *CmdTree {
	flags := NewArgParser("run", []string{"branchPath"}, []string{"src"})
	_ = flags.AddFlag("all-of-it", false, "a bool")
	_ = flags.AddArgString("label", "", "a string")
	_ = flags.AddArgCommaString("names", "a list")
	variadic := NewArgParser("many", []string{"path"}, []string{})
	_ = variadic.AddVariadic("names", "the names")
	return &CmdTree{
		SubCommands: map[string]*CmdTree{
			"run":    {Description: "Run it", Run: rustTestRun, ArgParser: flags, ResultType: rustTestResult{}},
			"hidden": {Run: rustTestRun, ArgParser: NewArgParser("hidden", nil, nil), NoAPI: true},
			"group": {SubCommands: map[string]*CmdTree{
				"many": {Description: "Many", Run: rustTestRun, ArgParser: variadic},
			}},
		},
	}
}

// The generated file is compared with testdata/rust_api.golden.rs:
// UPDATE_GOLDEN=1 go test ./internal/cli/clicore accepts a change.
func TestGenRustAPI_golden(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sub", "lib.rs")
	if err := rustTestTree().GenRustAPI(out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "rust_api.golden.rs")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("generated file differs from %s (UPDATE_GOLDEN=1 to accept)", golden)
	}
}

func TestGenRustAPI_rejectsUnsupportedTypes(t *testing.T) {
	type badKey struct {
		M map[int]string `json:"m"`
	}
	type badChan struct {
		C chan int `json:"c"`
	}
	for _, result := range []any{badKey{}, badChan{}} {
		tree := &CmdTree{SubCommands: map[string]*CmdTree{
			"run": {Run: rustTestRun, ArgParser: NewArgParser("run", nil, nil), ResultType: result},
		}}
		if err := tree.GenRustAPI(filepath.Join(t.TempDir(), "lib.rs")); err == nil {
			t.Errorf("%T: expected an error", result)
		}
	}
}

func TestRustIdent(t *testing.T) {
	for in, want := range map[string]string{
		"branchPath": "branch_path",
		"RepoUID":    "repo_uid",
		"repo_uid":   "repo_uid",
		"all-of-it":  "all_of_it",
		"Consumer":   "consumer",
		"type":       "r#type",
		"ref":        "r#ref",
		"self":       "self_",
	} {
		if got := rustIdent(in); got != want {
			t.Errorf("rustIdent(%q) = %q, want %q", in, got, want)
		}
	}
}
