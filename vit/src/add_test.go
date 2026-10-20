package vit_test

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"vit/internal/db"
	"vit/internal/opcontext"
	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

// --- success cases ---------------------------------------------------------

func TestAdd_fromFile(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1"))).String
	testutils.AssertExists(t, branchPath)
	testutils.AssertEqual(t, env.read(filepath.Join(branchPath, "hero.txt")), "v1")
	assertClean(t, must(env.Status(ctx, env.branchRef("chars/hero"))))
}

func TestAdd_fromDirNested(t *testing.T) {
	env := newVitEnv(t)

	src := env.dirSrc(map[string]string{
		"a.txt":       "AAA",
		"sub/b.txt":   "BBB",
		"sub/c/d.txt": "DDD",
	})
	branchPath := must(env.Add(ctx, env.abs("props/table"), src)).String

	testutils.AssertEqual(t, env.read(filepath.Join(branchPath, "a.txt")), "AAA")
	testutils.AssertEqual(t, env.read(filepath.Join(branchPath, "sub", "b.txt")), "BBB")
	testutils.AssertEqual(t, env.read(filepath.Join(branchPath, "sub", "c", "d.txt")), "DDD")
	assertClean(t, must(env.Status(ctx, env.branchRef("props/table"))))
}

func TestAdd_withDeepPath(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("a/b/c/hero"), env.fileSrc("hero.txt", "deep"))).String
	testutils.AssertExists(t, branchPath)
	assertClean(t, must(env.Status(ctx, env.branchRef("a/b/c/hero"))))
}

func TestAdd_underExistingDirNode(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "h")))
	if _, err := env.Add(ctx, env.abs("chars/villain"), env.fileSrc("villain.txt", "v")); err != nil {
		t.Fatalf("add sibling under dir node: %v", err)
	}
}

// --- error cases -----------------------------------------------------------

func TestAdd_missingSrc(t *testing.T) {
	env := newVitEnv(t)
	_, err := env.Add(ctx, env.abs("chars/hero"), filepath.Join(env.repoPath, "does-not-exist"))
	assertErrCode(t, err, types.ErrFileNotFound)
}

func TestAdd_emptySrc_createsEmptyAsset(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("chars/hero"), "")).String
	testutils.AssertExists(t, branchPath)
	assertClean(t, must(env.Status(ctx, env.branchRef("chars/hero"))))
}

func TestAdd_pathAlreadyTaken(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))
	_, err := env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v2"))
	assertErrCode(t, err, types.ErrDBTreeNode) // "PathNotAvailable" (in Extra)
}

func TestAdd_withinExistingAsset(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))
	_, err := env.Add(ctx, env.abs("chars/hero/sub"), env.fileSrc("x.txt", "x"))
	assertErrCode(t, err, types.ErrDBTreeNode) // "PathNotAvailableWithinAsset" (in Extra)
}

// branchParents returns, for an asset, each branch's root commit (Parent). Not
// observable through the API; read from the asset JSON to assert an invariant.
func (r *vitEnv) branchParents(t *testing.T, assetPath string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := opcontext.WithOperationContext(ctx, r.abs(assetPath), func(
		opctx *opcontext.OperationContext, rel string,
	) error {
		dbClient, err := db.NewClient(ctx, opctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			return err
		}
		defer dbClient.Close()
		ref := types.NewRefBranch(opctx.Repo(), rel, types.DefaultBranchName)
		a, err := dbClient.GetJSONAssetFromRef(ctx, opctx, ref, false)
		if err != nil {
			return err
		}
		for name, b := range a.Data.Branches {
			out[name] = b.Parent
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read asset %s: %v", assetPath, err)
	}
	return out
}

// Extra init branches all root on the SAME root base commit, with its content.
func TestAdd_withBranches(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1"), "wip", "lookdev"))

	parents := env.branchParents(t, "chars/hero")
	root := parents[types.DefaultBranchName]
	for _, name := range []string{"wip", "lookdev"} {
		if p, ok := parents[name]; !ok || p != root {
			t.Fatalf("branch %q should root on base's root commit %q, got %q (present=%v)", name, root, p, ok)
		}
		ws := filepath.Join(env.abs("chars/hero"), "branches", "branch-"+name)
		testutils.AssertEqual(t, env.read(filepath.Join(ws, "hero.txt")), "v1")
		assertClean(t, must(env.Status(ctx, env.abs("chars/hero")+"@branch="+name)))
	}

	// branches are independent working dirs.
	env.editFile(filepath.Join(env.abs("chars/hero"), "branches", "branch-wip", "hero.txt"), "wip-edit")
	testutils.AssertEqual(t, env.read(filepath.Join(env.targetBranchWs("chars/hero"), "hero.txt")), "v1")
}

// base is created by the add itself: naming it as an extra branch is ignored.
func TestAdd_baseBranchIgnored(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1"), types.DefaultBranchName, "wip"))

	parents := env.branchParents(t, "chars/hero")
	if _, ok := parents["wip"]; !ok || len(parents) != 2 { // base + wip
		t.Fatalf("expected base + wip, got %v", parents)
	}
}

// An extra branch given twice is created once.
func TestAdd_duplicateBranchesDeduped(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1"), "wip", "wip"))

	parents := env.branchParents(t, "chars/hero")
	if len(parents) != 2 { // base + wip
		t.Fatalf("expected base + one wip, got %v", parents)
	}
}
