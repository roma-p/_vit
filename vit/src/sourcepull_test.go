package vit_test

import (
	"path/filepath"
	"testing"

	testutils "vit/internal/testhelpers"
)

func TestSourcePull_updatesPinAfterHeadMoves(t *testing.T) {
	env := newVitEnv(t)
	modelB := must(env.Add(ctx, env.abs("assets/model"), env.fileSrc("m.ma", "M1"))).String
	lgt := must(env.Add(ctx, env.abs("assets/lgt"), env.fileSrc("l.ma", "L"))).String

	must(env.SourceAdd(ctx, env.branchRef("assets/lgt"), env.branchRef("assets/model"), "model"))
	testutils.AssertEqual(t, env.read(filepath.Join(lgt, "model", "m.ma")), "M1")

	env.editFile(filepath.Join(modelB, "m.ma"), "M2")
	must(env.Commit(ctx, env.branchRef("assets/model"), "model v2"))

	testutils.AssertEqual(t, env.read(filepath.Join(lgt, "model", "m.ma")), "M1")

	res := must(env.SourcePull(ctx, env.branchRef("assets/lgt"), nil))

	if len(res.Updates) != 1 || res.Updates[0].Name != "model" {
		t.Fatalf("expected one update on \"model\", got %v", res.Updates)
	}

	testutils.AssertEqual(t, env.read(filepath.Join(lgt, "model", "m.ma")), "M2")
}

func TestSourcePull_noopWhenUpToDate(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("assets/model"), env.fileSrc("m.ma", "M1")))
	lgt := must(env.Add(ctx, env.abs("assets/lgt"), env.fileSrc("l.ma", "L"))).String
	must(env.SourceAdd(ctx, env.branchRef("assets/lgt"), env.branchRef("assets/model"), "model"))

	res := must(env.SourcePull(ctx, env.branchRef("assets/lgt"), nil))

	testutils.AssertEqual(t, env.read(filepath.Join(lgt, "model", "m.ma")), "M1")
	if len(res.Updates) != 0 {
		t.Fatalf("expected no updates when up to date, got %v", res.Updates)
	}
}
