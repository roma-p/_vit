package vit_test

import (
	"path/filepath"
	"strings"
	"testing"

	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

func TestBranch_fromCleanBranch(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))

	wip := must(env.Branch(ctx, env.branchRef("chars/hero"), []string{"wip"})).StringList[0]
	testutils.AssertExists(t, wip)
	testutils.AssertEqual(t, env.read(filepath.Join(wip, "hero.txt")), "v1")
	assertClean(t, must(env.Status(ctx, env.abs("chars/hero")+"@branch=wip")))
}

func TestBranch_fromDirtyBranch_fails(t *testing.T) {
	env := newVitEnv(t)
	base := must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1"))).String
	env.editFile(filepath.Join(base, "hero.txt"), "dirty")

	_, err := env.Branch(ctx, env.branchRef("chars/hero"), []string{"wip"})
	assertErrCode(t, err, types.ErrBranchDirty)
}

func TestBranch_afterSnapshotSucceeds(t *testing.T) {
	env := newVitEnv(t)
	base := must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1"))).String
	env.editFile(filepath.Join(base, "hero.txt"), "v2")
	must(env.Snapshot(ctx, env.branchRef("chars/hero"), "v2"))

	wip := must(env.Branch(ctx, env.branchRef("chars/hero"), []string{"wip"})).StringList[0]
	testutils.AssertEqual(t, env.read(filepath.Join(wip, "hero.txt")), "v2")
}

func TestBranch_fromTag(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))
	tagWs := must(env.Tag(ctx, env.branchRef("chars/hero"), "approved", "ok")).String

	fromTag := must(env.Branch(ctx, tagWs, []string{"fromtag"})).StringList[0]
	testutils.AssertEqual(t, env.read(filepath.Join(fromTag, "hero.txt")), "v1")
}

func TestBranch_independentFromSource(t *testing.T) {
	env := newVitEnv(t)
	base := must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1"))).String

	wip := must(env.Branch(ctx, env.branchRef("chars/hero"), []string{"wip"})).StringList[0]
	env.editFile(filepath.Join(wip, "hero.txt"), "wip-edit")

	// editing the new branch's working dir must not touch the source branch.
	testutils.AssertEqual(t, env.read(filepath.Join(base, "hero.txt")), "v1")
}

func TestBranch_duplicateNameFails(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))
	must(env.Branch(ctx, env.branchRef("chars/hero"), []string{"wip"}))

	_, err := env.Branch(ctx, env.branchRef("chars/hero"), []string{"wip"})
	assertErrCode(t, err, types.ErrAssetObjectAlreadyExists)
}

func TestBranch_multipleNames(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))

	ws := must(env.Branch(ctx, env.branchRef("chars/hero"), []string{"wip", "lookdev"})).StringList
	if len(ws) != 2 {
		t.Fatalf("expected one workspace per branch, got %v", ws)
	}
	for i, name := range []string{"wip", "lookdev"} {
		testutils.AssertEqual(t, env.read(filepath.Join(ws[i], "hero.txt")), "v1")
		assertClean(t, must(env.Status(ctx, env.abs("chars/hero")+"@branch="+name)))
	}

	// each branch is independent from the others.
	env.editFile(filepath.Join(ws[0], "hero.txt"), "wip-edit")
	testutils.AssertEqual(t, env.read(filepath.Join(ws[1], "hero.txt")), "v1")
}

func TestBranch_duplicateNameDeduped(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))

	ws := must(env.Branch(ctx, env.branchRef("chars/hero"), []string{"wip", "wip"})).StringList
	if len(ws) != 1 {
		t.Fatalf("expected the duplicate to be created once, got %v", ws)
	}
	testutils.AssertEqual(t, env.read(filepath.Join(ws[0], "hero.txt")), "v1")
}

func TestBranch_reportsAllExistingNames(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))
	must(env.Branch(ctx, env.branchRef("chars/hero"), []string{"wip", "lookdev"}))

	_, err := env.Branch(ctx, env.branchRef("chars/hero"), []string{"lookdev", "fresh", "wip"})
	assertErrCode(t, err, types.ErrAssetObjectAlreadyExists)
	for _, name := range []string{"wip", "lookdev"} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error should list existing branch %q: %v", name, err)
		}
	}
	if hasBranch(must(env.Ls(ctx, env.abs("chars/hero"))), "fresh") {
		t.Fatal("fresh was created although the request failed")
	}
}

func TestBranch_noNameFails(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))

	_, err := env.Branch(ctx, env.branchRef("chars/hero"), nil)
	assertErrCode(t, err, types.ErrInvalidInut)
}

func TestBranch_oneExistingNameFailsAll(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("chars/hero"), env.fileSrc("hero.txt", "v1")))
	must(env.Branch(ctx, env.branchRef("chars/hero"), []string{"wip"}))

	_, err := env.Branch(ctx, env.branchRef("chars/hero"), []string{"lookdev", "wip"})
	assertErrCode(t, err, types.ErrAssetObjectAlreadyExists)
	if hasBranch(must(env.Ls(ctx, env.abs("chars/hero"))), "lookdev") {
		t.Fatal("lookdev was created although the request failed")
	}
}
