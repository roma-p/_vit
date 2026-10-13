package vit_test

import (
	"path/filepath"
	"testing"

	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

func TestBranch_fromCleanBranch(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))

	wip := must(env.Branch(ctx, env.branchRef("chars/hero"), "wip")).String
	testutils.AssertExists(t, wip)
	testutils.AssertEqual(t, env.read(filepath.Join(wip, "hero.txt")), "v1")
	assertClean(t, must(env.Status(ctx, env.abs("chars/hero")+"@branch=wip")))
}

func TestBranch_fromDirtyBranch_fails(t *testing.T) {
	env := newVitEnv(t)
	base := must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1"))).String
	env.editFile(filepath.Join(base, "hero.txt"), "dirty")

	_, err := env.Branch(ctx, env.branchRef("chars/hero"), "wip")
	assertErrCode(t, err, types.ErrBranchDirty)
}

func TestBranch_afterSnapshotSucceeds(t *testing.T) {
	env := newVitEnv(t)
	base := must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1"))).String
	env.editFile(filepath.Join(base, "hero.txt"), "v2")
	must(env.Snapshot(ctx, env.branchRef("chars/hero"), "v2"))

	wip := must(env.Branch(ctx, env.branchRef("chars/hero"), "wip")).String
	testutils.AssertEqual(t, env.read(filepath.Join(wip, "hero.txt")), "v2")
}

func TestBranch_fromTag(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))
	tagWs := must(env.Tag(ctx, env.branchRef("chars/hero"), "approved", "ok")).String

	fromTag := must(env.Branch(ctx, tagWs, "fromtag")).String
	testutils.AssertEqual(t, env.read(filepath.Join(fromTag, "hero.txt")), "v1")
}

func TestBranch_independentFromSource(t *testing.T) {
	env := newVitEnv(t)
	base := must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1"))).String

	wip := must(env.Branch(ctx, env.branchRef("chars/hero"), "wip")).String
	env.editFile(filepath.Join(wip, "hero.txt"), "wip-edit")

	// editing the new branch's working dir must not touch the source branch.
	testutils.AssertEqual(t, env.read(filepath.Join(base, "hero.txt")), "v1")
}

func TestBranch_duplicateNameFails(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))
	must(env.Branch(ctx, env.branchRef("chars/hero"), "wip"))

	_, err := env.Branch(ctx, env.branchRef("chars/hero"), "wip")
	assertErrCode(t, err, types.ErrAssetObjectAlreadyExists)
}
