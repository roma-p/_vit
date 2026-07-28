package vit_test

import (
	"path/filepath"
	"testing"

	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

func TestCommit_roundTripCleansStatus(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("props/table"), env.fileSrc("a.txt", "AAA"))).String

	env.editFile(filepath.Join(branchPath, "a.txt"), "WWW")
	testutils.AssertTrue(t, must(env.Status(ctx, branchPath)).HasChange()) // dirty

	must(env.Commit(ctx, branchPath, "edit a"))

	assertClean(t, must(env.Status(ctx, branchPath))) // clean again
}

func TestCommit_noChange(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("props/table"), env.fileSrc("a.txt", "AAA"))).String

	_, err := env.Commit(ctx, branchPath, "nothing")
	assertErrCode(t, err, types.ErrNoChangeToCommit)
}

func TestCommit_presentsCommittedContent(t *testing.T) {
	env := newVitEnv(t)
	branchPath := must(env.Add(ctx, env.abs("props/table"), env.dirSrc(map[string]string{
		"a.txt":     "AAA",
		"sub/b.txt": "BBB",
	}))).String

	env.editFile(filepath.Join(branchPath, "a.txt"), "WWW")
	commitPath := must(env.Commit(ctx, branchPath, "edit a")).String

	testutils.AssertEqual(t, env.read(filepath.Join(commitPath, "a.txt")), "WWW")        // edited → captured
	testutils.AssertEqual(t, env.read(filepath.Join(commitPath, "sub", "b.txt")), "BBB") // unchanged → inherited & present
}
