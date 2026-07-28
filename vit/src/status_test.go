package vit_test

import (
	"path/filepath"
	"testing"

	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

// --- success cases ---------------------------------------------------------

func TestStatus_newlyAddedAsset(t *testing.T) {
	env := newVitEnv(t)
	src := env.dirSrc(map[string]string{
		"a.txt":       "AAA",
		"sub/b.txt":   "BBB",
		"sub/c/d.txt": "DDD",
	})
	branchPath := must(env.Add(ctx, env.abs("props/table"), src)).String

	sr1 := must(env.Status(ctx, branchPath))
	testutils.AssertFalse(t, sr1.HasChange())

	sr2 := must(env.Status(ctx, env.abs("props/table@branch=base")))
	testutils.AssertFalse(t, sr2.HasChange())
}

func TestStatus_uncommittedChange(t *testing.T) {
	env := newVitEnv(t)
	src := env.dirSrc(map[string]string{
		"a.txt":       "AAA",
		"sub/b.txt":   "BBB",
		"sub/c/d.txt": "DDD",
	})
	branchPath := must(env.Add(ctx, env.abs("props/table"), src)).String

	env.editFile(filepath.Join(branchPath, "a.txt"), "WWW")
	env.editFile(filepath.Join(branchPath, "sub", "c", "d.txt"), "WWW")
	env.editFile(filepath.Join(branchPath, "sub", "c", "NEW", "e.txt"), "WWW")
	env.removeFile(filepath.Join(branchPath, "sub", "b.txt"))

	sr1 := must(env.Status(ctx, branchPath))
	assertChanges(t, sr1, map[string]types.FileChangeType{
		"a.txt":           types.FileChangeEdited,
		"sub/c/d.txt":     types.FileChangeEdited,
		"sub/c/NEW/e.txt": types.FileChangeAdded,
		"sub/b.txt":       types.FileChangeRemoved,
	})
}

// --- error cases -----------------------------------------------------------

func TestStatus_missingAsset_refString(t *testing.T) {
	env := newVitEnv(t)
	_, err := env.Status(ctx, env.abs("ghost")+"@branch=base")
	assertErrCode(t, err, types.ErrPointerNotFound)
}

func TestStatus_missingAsset_workspacePath(t *testing.T) {
	env := newVitEnv(t)
	_, err := env.Status(ctx, env.abs("ghost"))
	assertErrCode(t, err, types.ErrPointerNotFound)
}

func assertChanges(t *testing.T, res *types.StatusResult, want map[string]types.FileChangeType) {
	t.Helper()
	got := make(map[string]types.FileChangeType, len(res.Files))
	for _, f := range res.Files {
		got[f.Path] = f.Type
	}
	if len(got) != len(want) {
		t.Fatalf("status: got %d change(s) %v, want %d %v", len(got), got, len(want), want)
	}
	for path, typ := range want {
		if got[path] != typ {
			t.Fatalf("status: change for %q = %q, want %q (full: %v)", path, got[path], typ, got)
		}
	}
}
