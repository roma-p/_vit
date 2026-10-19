package vit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	testutils "vit/internal/testhelpers"
)

// sibling spins up a SECOND repo (its own temp dir) sharing this env's client,
// for cross-repo tests. The vit.Client is repo-agnostic: it resolves repos from
// the absolute paths it is handed, so one client drives both repos.
func (r *vitEnv) sibling(name string) *vitEnv {
	r.t.Helper()
	dir, cleanup := testutils.TempDir(r.t, "vit-repo2-*")
	r.t.Cleanup(cleanup)
	repoPath := filepath.Join(dir, name)
	must(r.Client.InitRepo(ctx, repoPath))
	return &vitEnv{Client: r.Client, t: r.t, repoPath: repoPath}
}

func (r *vitEnv) config(t *testing.T) struct {
	RepoUID string            `json:"repo_uid"`
	Repos   map[string]string `json:"repos"`
} {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.repoPath, ".vit", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		RepoUID string            `json:"repo_uid"`
		Repos   map[string]string `json:"repos"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return cfg
}

// knownRepos reads a repo's cross-repo registry (config.Repos: repoUID -> path).
func (r *vitEnv) knownRepos(t *testing.T) map[string]string { return r.config(t).Repos }

// repoUID reads a repo's own UID.
func (r *vitEnv) repoUID(t *testing.T) string { return r.config(t).RepoUID }

func TestProductAdd_crossRepoCreate(t *testing.T) {
	a := newVitEnv(t)
	b := a.sibling("repoB")
	anim := must(a.Add(ctx, a.abs("shot/anim"), a.fileSrc("anim.ma", "A"))).String

	// Explicit @branch=base qualifier on a target in ANOTHER repo.
	must(a.ProductAdd(ctx, a.branchRef("shot/anim"), b.abs("render/beauty")+"@branch=base", "render", true))

	// The target asset is created in repo B, at a CLEAN path (no @branch= leak,
	// no doubled repo segment).
	if _, err := os.Stat(b.targetBranchWs("render/beauty")); err != nil {
		t.Fatalf("expected product asset created in repo B at render/beauty: %v", err)
	}
	if _, err := os.Stat(b.abs("render/beauty@branch=base")); err == nil {
		t.Fatal("qualifier leaked into the asset path: render/beauty@branch=base exists")
	}

	// Rendering through the symlink under A's origin lands in B's branch ws.
	link := filepath.Join(anim, "render")
	a.editFile(filepath.Join(link, "out.exr"), "frame1")
	testutils.AssertEqual(t, b.read(filepath.Join(b.targetBranchWs("render/beauty"), "out.exr")), "frame1")

	// Both repos learn each other in their cross-repo registries.
	if _, ok := a.knownRepos(t)[b.repoUID(t)]; !ok {
		t.Fatalf("repo A did not register repo B: %+v", a.knownRepos(t))
	}
	if _, ok := b.knownRepos(t)[a.repoUID(t)]; !ok {
		t.Fatalf("repo B did not register repo A: %+v", b.knownRepos(t))
	}
}

// After a cross-repo product add, a FRESH command (new opctx) with a COLD cache
// must still resolve the foreign target via the registry, not fail on an empty
// RepoPath.
func TestProductAdd_crossRepoReadBack(t *testing.T) {
	a := newVitEnv(t)
	b := a.sibling("repoB")
	must(a.Add(ctx, a.abs("shot/anim"), a.fileSrc("anim.ma", "A")))
	must(a.ProductAdd(ctx, a.branchRef("shot/anim"), b.abs("render/beauty"), "render", true))

	// Force a forward-graph rebuild in a fresh opctx: drop A's dep-graph cache,
	// then ls the origin. The rebuild walks the product edge and must read the
	// target asset out of repo B (resolving B's path from the registry).
	if err := os.RemoveAll(a.depGraphDir()); err != nil {
		t.Fatalf("drop cache: %v", err)
	}
	if _, err := a.Ls(ctx, a.branchRef("shot/anim")); err != nil {
		t.Fatalf("ls after cold cache failed to resolve cross-repo product: %v", err)
	}
}

func TestSourceAdd_crossRepo(t *testing.T) {
	a := newVitEnv(t)
	b := a.sibling("repoB")
	must(b.Add(ctx, b.abs("lib/rig"), b.fileSrc("rig.ma", "RIG")))
	lgt := must(a.Add(ctx, a.abs("shot/lgt"), a.fileSrc("lgt.ma", "LGT"))).String

	// Consumer in A, provider in B.
	must(a.SourceAdd(ctx, a.branchRef("shot/lgt"), b.abs("lib/rig")+"@branch=base", "rig"))

	// The provider is presented into A's workspace from repo B.
	testutils.AssertEqual(t, a.read(filepath.Join(lgt, "rig", "rig.ma")), "RIG")

	// Both repos learn each other.
	if _, ok := a.knownRepos(t)[b.repoUID(t)]; !ok {
		t.Fatalf("repo A did not register repo B: %+v", a.knownRepos(t))
	}
	if _, ok := b.knownRepos(t)[a.repoUID(t)]; !ok {
		t.Fatalf("repo B did not register repo A: %+v", b.knownRepos(t))
	}
}
