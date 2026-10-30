package vit_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	must(env.Snapshot(ctx, env.branchRef("asset/rig"), "rig v2"))
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

// reverseStaleMarkers lists the repo's .reverse_stale marker files.
func (r *vitEnv) reverseStaleMarkers() []string {
	var out []string
	_ = filepath.WalkDir(r.depGraphDir(), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".reverse_stale") {
			out = append(out, path)
		}
		return nil
	})
	return out
}

// A source add marks the provider's reverse graph stale; the next reverse read
// rebuilds it AND clears the marker, and later reads still see the consumer.
func TestSourcePush_clearsReverseStaleMarker(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("asset/rig"), env.fileSrc("rig.ma", "rig v1")))
	must(env.Tag(ctx, env.branchRef("asset/rig"), "approved", "rig approved"))
	must(env.Add(ctx, env.abs("seq/shot0010/anim"), env.fileSrc("anim.ma", "a")))
	must(env.SourceAdd(ctx, env.branchRef("seq/shot0010/anim"), env.abs("asset/rig")+"@tag=approved", "rig"))

	if len(env.reverseStaleMarkers()) == 0 {
		t.Fatal("source add should leave a reverse-stale marker on the provider")
	}
	must(env.SourcePush(ctx, env.abs("asset/rig")+"@tag=approved", nil))
	if m := env.reverseStaleMarkers(); len(m) != 0 {
		t.Fatalf("reverse read should clear the marker, still have %v", m)
	}

	res := must(env.SourcePush(ctx, env.abs("asset/rig")+"@tag=approved", nil))
	if len(res.Consumers) != 1 {
		t.Fatalf("after clearing, the cached reverse graph should still list 1 consumer, got %d", len(res.Consumers))
	}
}

// Sourcing an origin marks only the origin's SOURCE closure stale, not the
// products it produces into (their consumers did not change).
func TestSourceAdd_marksOnlySourceClosureStale(t *testing.T) {
	env := newVitEnv(t)
	ws := env.originWithProduct(t, "shot/lgt", "v1")
	env.editFile(filepath.Join(ws, "render", "out.exr"), "frame1")
	env.editFile(filepath.Join(ws, "src"), "v2")
	must(env.Snapshot(ctx, env.branchRef("shot/lgt"), "lgt + render")) // the pinned commit produced render
	must(env.Add(ctx, env.abs("shot/comp"), env.fileSrc("comp.nk", "c1")))
	for _, m := range env.reverseStaleMarkers() {
		if err := os.Remove(m); err != nil {
			t.Fatal(err)
		}
	}

	must(env.SourceAdd(ctx, env.branchRef("shot/comp"), env.branchRef("shot/lgt"), "lgt"))

	if m := env.reverseStaleMarkers(); len(m) != 1 {
		t.Fatalf("expected only shot/lgt marked stale, got %d markers: %v", len(m), m)
	}
}
