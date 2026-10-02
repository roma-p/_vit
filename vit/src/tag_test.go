package vit_test

import (
	"path/filepath"
	"testing"

	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

func TestTag_branchNoDiffTagsHead(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("props/table"), env.fileSrc("a.txt", "AAA"))).String

	tagPath := must(env.Tag(ctx, branchPath, "lookdev", "first")).String

	testutils.AssertEqual(t, filepath.Base(tagPath), "lookdev-001")
	testutils.AssertEqual(t, env.read(filepath.Join(tagPath, "a.txt")), "AAA")
}

func TestTag_autoVersionsPerFamily(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("props/table"), env.fileSrc("a.txt", "AAA"))).String

	t1 := must(env.Tag(ctx, branchPath, "lookdev", "v1")).String
	testutils.AssertEqual(t, filepath.Base(t1), "lookdev-001")

	env.editFile(filepath.Join(branchPath, "a.txt"), "WWW")
	t2 := must(env.Tag(ctx, branchPath, "lookdev", "v2")).String
	testutils.AssertEqual(t, filepath.Base(t2), "lookdev-002")
}

func TestTag_dedupSameCommitSameFamily(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("props/table"), env.fileSrc("a.txt", "AAA"))).String

	must(env.Tag(ctx, branchPath, "lookdev", "v1")) // head -> lookdev-001

	_, err := env.Tag(ctx, branchPath, "lookdev", "again")
	assertErrCode(t, err, types.ErrAssetObjectAlreadyExists)
}

func TestTag_presentsFullContent(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(
		env.Add(
			ctx, env.abs("props/table"),
			env.dirSrc(
				map[string]string{
					"a.txt":     "AAA",
					"sub/b.txt": "BBB",
				},
			),
		),
	).String

	env.editFile(filepath.Join(branchPath, "a.txt"), "WWW")
	tagPath := must(env.Tag(ctx, branchPath, "final", "v1")).String

	testutils.AssertEqual(t, env.read(filepath.Join(tagPath, "a.txt")), "WWW")
	testutils.AssertEqual(t, env.read(filepath.Join(tagPath, "sub", "b.txt")), "BBB")
}

func TestTag_commitSourceByWorkspacePath(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("props/table"), env.fileSrc("a.txt", "AAA"))).String
	env.editFile(filepath.Join(branchPath, "a.txt"), "WWW")
	commitPath := must(env.Snapshot(ctx, branchPath, "edit")).String

	tagPath := must(env.Tag(ctx, commitPath, "review", "from commit")).String
	testutils.AssertEqual(t, filepath.Base(tagPath), "review-001")
	testutils.AssertEqual(t, env.read(filepath.Join(tagPath, "a.txt")), "WWW")
}

func TestTag_dirtyBranchCommitsAndCleans(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("props/table"), env.fileSrc("a.txt", "AAA"))).String

	env.editFile(filepath.Join(branchPath, "a.txt"), "WWW")
	must(env.Tag(ctx, branchPath, "lookdev", "v1")) // dirty → commit + tag

	assertClean(t, must(env.Status(ctx, branchPath)))
}

func TestTag_tagFromTag(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("props/table"), env.fileSrc("a.txt", "AAA"))).String

	srcTag := must(env.Tag(ctx, branchPath, "lookdev", "v1")).String

	newTag := must(env.Tag(ctx, srcTag, "approved", "promote")).String
	testutils.AssertEqual(t, filepath.Base(newTag), "approved-001")
	testutils.AssertEqual(t, env.read(filepath.Join(newTag, "a.txt")), "AAA")
}

func TestTag_emptyFamily(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("props/table"), env.fileSrc("a.txt", "AAA"))).String

	_, err := env.Tag(ctx, branchPath, "", "no family")
	assertErrCode(t, err, types.ErrInvalidInut)
}
