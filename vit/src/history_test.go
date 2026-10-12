package vit_test

import (
	"path/filepath"
	"testing"

	"vit/internal/history"
	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

func TestHistory_recordsMutatingOps(t *testing.T) {
	env := newVitEnv(t)

	branchPath := must(env.Add(ctx, env.abs("props/table"), env.fileSrc("a.txt", "AAA"))).String
	env.editFile(filepath.Join(branchPath, "a.txt"), "WWW")
	must(env.Snapshot(ctx, branchPath, "edit a"))
	must(env.Tag(ctx, branchPath, "approved", "looks good"))

	entries := must(history.ReadAllSites(env.repoPath))

	cmds := map[types.VitCommand]bool{}
	var snapMsg, snapAsset string
	for _, e := range entries {
		cmds[e.Command] = true
		testutils.AssertTrue(t, e.UID != "")
		testutils.AssertTrue(t, !e.Timestamp.IsZero())
		if e.Command == types.CommandCommit {
			snapMsg = e.Message
			snapAsset = e.AssetPath
		}
	}

	testutils.AssertTrue(t, cmds[types.CommandAdd])
	testutils.AssertTrue(t, cmds[types.CommandCommit])
	testutils.AssertTrue(t, cmds[types.CommandTag])
	testutils.AssertEqual(t, snapMsg, "edit a")
	testutils.AssertEqual(t, snapAsset, "props/table")
}
