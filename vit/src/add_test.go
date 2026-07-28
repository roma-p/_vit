package vit_test

import (
	"path/filepath"
	"testing"

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

func TestAdd_emptySrc(t *testing.T) {
	env := newVitEnv(t)
	_, err := env.Add(ctx, env.abs("chars/hero"), "")
	if err == nil {
		t.Fatal("expected an error for empty src, got nil")
	}
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
