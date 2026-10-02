package vit_test

import (
	"os"
	"path/filepath"
	"testing"

	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

func TestSourceAdd_sourceIsTag(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("assets/mod"), env.fileSrc("model.ma", "geo")))
	lgt := must(env.Add(ctx, env.abs("assets/lgt"), env.fileSrc("lgt.ma", "scene"))).String
	must(env.Tag(ctx, env.branchRef("assets/mod"), "approved", "v1"))

	must(env.SourceAdd(ctx, env.branchRef("assets/lgt"), env.abs("assets/mod")+"@tag=approved", "model"))

	testutils.AssertEqual(t, env.read(filepath.Join(lgt, "model", "model.ma")), "geo")
}

func TestSourceAdd_presentsAtNamePath(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("assets/mod"), env.dirSrc(map[string]string{
		"model.ma":     "geo",
		"tex/skin.tex": "diffuse",
	})))
	lgt := must(env.Add(ctx, env.abs("assets/lgt"), env.fileSrc("lgt.ma", "scene"))).String

	must(env.SourceAdd(ctx, env.branchRef("assets/lgt"), env.branchRef("assets/mod"), ".src/model"))

	testutils.AssertEqual(t, env.read(filepath.Join(lgt, ".src", "model", "model.ma")), "geo")
	testutils.AssertEqual(t, env.read(filepath.Join(lgt, ".src", "model", "tex", "skin.tex")), "diffuse")
	if _, err := os.Stat(filepath.Join(lgt, "sources")); err == nil {
		t.Fatal("did not expect a reserved sources/ dir")
	}
	testutils.AssertEqual(t, env.read(filepath.Join(lgt, "lgt.ma")), "scene")
}

func TestSourceAdd_transitiveClosureNested(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("assets/rig"), env.fileSrc("rig.ma", "RIG")))
	animBranch := must(env.Add(ctx, env.abs("assets/anim"), env.fileSrc("anim.ma", "ANIM"))).String
	lgt := must(env.Add(ctx, env.abs("assets/lgt"), env.fileSrc("lgt.ma", "LGT"))).String

	must(env.SourceAdd(ctx, env.branchRef("assets/anim"), env.branchRef("assets/rig"), "rig"))
	env.editFile(filepath.Join(animBranch, "anim.ma"), "ANIM-v2")
	must(env.Snapshot(ctx, env.branchRef("assets/anim"), "publish anim with rig"))
	must(env.SourceAdd(ctx, env.branchRef("assets/lgt"), env.branchRef("assets/anim"), "anim"))

	testutils.AssertEqual(t, env.read(filepath.Join(lgt, "anim", "anim.ma")), "ANIM-v2")
	testutils.AssertEqual(t, env.read(filepath.Join(lgt, "anim", "rig", "rig.ma")), "RIG")
	if _, err := os.Stat(filepath.Join(lgt, "anim", "rig")); err != nil {
		t.Fatalf("expected recursively presented rig under anim: %v", err)
	}
}

func TestSourceAdd_sourceIsReadOnly(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("assets/mod"), env.fileSrc("model.ma", "geo")))
	lgt := must(env.Add(ctx, env.abs("assets/lgt"), env.fileSrc("lgt.ma", "scene"))).String

	must(env.SourceAdd(ctx, env.branchRef("assets/lgt"), env.branchRef("assets/mod"), "model"))

	fi, err := os.Stat(filepath.Join(lgt, "model", "model.ma"))
	if err != nil {
		t.Fatalf("stat sourced file: %v", err)
	}
	if fi.Mode().Perm()&0o222 != 0 {
		t.Fatalf("expected sourced file read-only, got mode %v", fi.Mode())
	}
}

func TestSourceAdd_cycleRejected(t *testing.T) {
	env := newVitEnv(t)
	aBranch := must(env.Add(ctx, env.abs("assets/a"), env.fileSrc("a.ma", "A"))).String
	must(env.Add(ctx, env.abs("assets/b"), env.fileSrc("b.ma", "B")))

	must(env.SourceAdd(ctx, env.branchRef("assets/a"), env.branchRef("assets/b"), "b"))
	env.editFile(filepath.Join(aBranch, "a.ma"), "A-v2")
	must(env.Snapshot(ctx, env.branchRef("assets/a"), "publish a->b"))

	_, err := env.SourceAdd(ctx, env.branchRef("assets/b"), env.branchRef("assets/a"), "a")
	assertErrCode(t, err, types.ErrCircularDependency)
}

func TestSourceAdd_duplicateNameRejected(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("assets/mod"), env.fileSrc("model.ma", "geo")))
	must(env.Add(ctx, env.abs("assets/cam"), env.fileSrc("cam.ma", "cam")))
	must(env.Add(ctx, env.abs("assets/lgt"), env.fileSrc("lgt.ma", "scene")))

	must(env.SourceAdd(ctx, env.branchRef("assets/lgt"), env.branchRef("assets/mod"), "input"))

	_, err := env.SourceAdd(ctx, env.branchRef("assets/lgt"), env.branchRef("assets/cam"), "input")
	assertErrCode(t, err, types.ErrDependencyAlreadyExists)
}

func TestSourceAdd_missingTargetRejected(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("assets/lgt"), env.fileSrc("lgt.ma", "scene")))

	_, err := env.SourceAdd(ctx, env.branchRef("assets/lgt"), env.branchRef("assets/ghost"), "x")
	if err == nil {
		t.Fatal("expected error sourcing a non-existent asset, got nil")
	}
}
