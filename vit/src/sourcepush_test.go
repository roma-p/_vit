package vit_test

import (
	"path/filepath"
	"testing"

	testutils "vit/internal/testhelpers"
)

func TestSourcePush_updatesAllLiveConsumers(t *testing.T) {
	env := newVitEnv(t)

	rigB := must(env.Add(ctx, env.abs("asset/rig"), env.fileSrc("rig.ma", "rig v1"))).String
	must(env.Tag(ctx, env.branchRef("asset/rig"), "approved", "rig v1 approved"))

	shots := []string{"shot0010", "shot0020", "shot0030"}
	animB := map[string]string{}
	for _, s := range shots {
		ap := "seq/" + s + "/anim"
		animB[s] = must(env.Add(ctx, env.abs(ap), env.fileSrc("anim.ma", s))).String
		must(env.SourceAdd(ctx, env.branchRef(ap), env.abs("asset/rig")+"@tag=approved", "rig"))
		testutils.AssertEqual(t, env.read(filepath.Join(animB[s], "rig", "rig.ma")), "rig v1")
	}

	env.editFile(filepath.Join(rigB, "rig.ma"), "rig v2")
	must(env.Commit(ctx, env.branchRef("asset/rig"), "rig v2"))
	must(env.Tag(ctx, env.branchRef("asset/rig"), "approved", "rig v2 approved"))

	testutils.AssertEqual(t, env.read(filepath.Join(animB["shot0010"], "rig", "rig.ma")), "rig v1")
	res := must(env.SourcePush(ctx, env.abs("asset/rig")+"@tag=approved", nil))

	if len(res.Consumers) != 3 {
		t.Fatalf("expected 3 consumers pushed, got %d", len(res.Consumers))
	}
	for _, s := range shots {
		testutils.AssertEqual(t, env.read(filepath.Join(animB[s], "rig", "rig.ma")), "rig v2")
	}
}

func TestSourcePush_noopWhenUpToDate(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("asset/rig"), env.fileSrc("rig.ma", "rig v1")))
	must(env.Tag(ctx, env.branchRef("asset/rig"), "approved", "rig approved"))
	must(env.Add(ctx, env.abs("seq/shot0010/anim"), env.fileSrc("anim.ma", "a")))
	must(env.SourceAdd(ctx, env.branchRef("seq/shot0010/anim"), env.abs("asset/rig")+"@tag=approved", "rig"))

	res := must(env.SourcePush(ctx, env.abs("asset/rig")+"@tag=approved", nil))

	for _, c := range res.Consumers {
		if len(c.Updates) != 0 {
			t.Fatalf("expected no updates, got %v", c.Updates)
		}
	}
}
