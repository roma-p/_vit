package vit_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

func (r *vitEnv) targetBranchWs(assetPath string) string {
	return filepath.Join(r.abs(assetPath), "branches", "branch-base")
}

func (r *vitEnv) depGraphDir() string {
	return filepath.Join(r.repoPath, ".vit", "cache", "depgraph")
}

// forwardHasProductEdge reports whether any forward dep-graph cache file carries a
// product-typed edge (the cache files are the <uid>.json leaves, not the
// depended_by/produced_by index dirs).
func (r *vitEnv) forwardHasProductEdge(t *testing.T) bool {
	t.Helper()
	type fwdCache struct {
		Forward map[string]struct {
			Edges []struct {
				Type string `json:"type"`
			} `json:"edges"`
		} `json:"forward"`
	}
	found := false
	_ = filepath.WalkDir(r.depGraphDir(), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		if strings.Contains(path, ".rdeps") || strings.Contains(path, ".reverse_stale") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		var c fwdCache
		if json.Unmarshal(b, &c) != nil {
			return nil
		}
		for _, g := range c.Forward {
			for _, e := range g.Edges {
				if e.Type == string(types.AssetDependencyProduct) {
					found = true
				}
			}
		}
		return nil
	})
	return found
}

type producedByEntry struct {
	Type string     `json:"type"`
	Peer *types.Ref `json:"peer"`
	Name string     `json:"name"`
}

// readProducedBy collects every product-typed reverse-index entry across the
// repo's unified .rdeps cache.
func (r *vitEnv) readProducedBy(t *testing.T) []producedByEntry {
	t.Helper()
	var out []producedByEntry
	_ = filepath.WalkDir(r.depGraphDir(), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.Contains(path, ".rdeps") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		var e producedByEntry
		if json.Unmarshal(b, &e) == nil && e.Type == string(types.AssetDependencyProduct) {
			out = append(out, e)
		}
		return nil
	})
	return out
}

func TestProductAdd_createDeliversWritableSymlink(t *testing.T) {
	env := newVitEnv(t)
	anim := must(env.Add(ctx, env.abs("shot/anim"), env.fileSrc("anim.ma", "A"))).String

	must(env.ProductAdd(ctx, env.branchRef("shot/anim"), env.abs("render/beauty"), "render", true))

	// The delivery is a symlink under the origin workspace.
	link := filepath.Join(anim, "render")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat product link: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected a symlink at %s, got mode %v", link, fi.Mode())
	}

	// Rendering through the symlink lands in the target's branch workspace.
	env.editFile(filepath.Join(link, "out.exr"), "frame1")
	testutils.AssertEqual(t, env.read(filepath.Join(env.targetBranchWs("render/beauty"), "out.exr")), "frame1")
	testutils.AssertEqual(t, env.read(filepath.Join(link, "out.exr")), "frame1")
}

func TestProductAdd_attachExistingTarget(t *testing.T) {
	env := newVitEnv(t)
	anim := must(env.Add(ctx, env.abs("shot/anim"), env.fileSrc("anim.ma", "A"))).String
	must(env.Add(ctx, env.abs("render/beauty"), "")) // target created empty, out of band

	must(env.ProductAdd(ctx, env.branchRef("shot/anim"), env.abs("render/beauty"), "render", false))

	env.editFile(filepath.Join(anim, "render", "out.exr"), "frame1")
	testutils.AssertEqual(t, env.read(filepath.Join(env.targetBranchWs("render/beauty"), "out.exr")), "frame1")
}

// --create on a target that already exists must NOT fail re-creating it: it
// downgrades to a plain attach (create-if-missing semantics).
func TestProductAdd_createOnExistingTargetAttaches(t *testing.T) {
	env := newVitEnv(t)
	anim := must(env.Add(ctx, env.abs("shot/anim"), env.fileSrc("anim.ma", "A"))).String
	must(env.Add(ctx, env.abs("render/beauty"), "")) // target already exists

	// create=true even though the target is already there -> attach, no error.
	must(env.ProductAdd(ctx, env.branchRef("shot/anim"), env.abs("render/beauty"), "render", true))

	env.editFile(filepath.Join(anim, "render", "out.exr"), "frame1")
	testutils.AssertEqual(t, env.read(filepath.Join(env.targetBranchWs("render/beauty"), "out.exr")), "frame1")
}

func TestProductAdd_attachMissingTargetRejected(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("shot/anim"), env.fileSrc("anim.ma", "A")))

	_, err := env.ProductAdd(ctx, env.branchRef("shot/anim"), env.abs("render/ghost"), "render", false)
	assertErrCode(t, err, types.ErrProductTargetNotFound)
}

func TestProductAdd_recordsGraphEdges(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("shot/anim"), env.fileSrc("anim.ma", "A")))

	must(env.ProductAdd(ctx, env.branchRef("shot/anim"), env.abs("render/beauty"), "render", true))

	// Forward: the origin's cached closure carries a product-typed edge.
	if !env.forwardHasProductEdge(t) {
		t.Fatal("expected a product edge in a forward dep-graph cache, found none")
	}

	// Reverse: a produced_by entry on the target names the origin.
	prods := env.readProducedBy(t)
	if len(prods) == 0 {
		t.Fatal("expected a produced_by entry on the target, found none")
	}
	found := false
	for _, p := range prods {
		if p.Name == "render" && p.Peer != nil && strings.HasSuffix(p.Peer.AssetPath, "shot/anim") {
			found = true
		}
	}
	if !found {
		t.Fatalf("produced_by does not name origin shot/anim via 'render': %+v", prods)
	}
}

func TestProductAdd_lsManifestsProduct(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("shot/anim"), env.fileSrc("anim.ma", "A")))
	must(env.ProductAdd(ctx, env.branchRef("shot/anim"), env.abs("render/beauty"), "render", true))

	res := must(env.Ls(ctx, env.branchRef("shot/anim")))
	lines := strings.Join(res.ToStringSlice(), "\n")
	if !strings.Contains(lines, "products:") || !strings.Contains(lines, "render") {
		t.Fatalf("ls did not manifest the product:\n%s", lines)
	}
}

func TestProductAdd_duplicateNameRejected(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("shot/anim"), env.fileSrc("anim.ma", "A")))

	must(env.ProductAdd(ctx, env.branchRef("shot/anim"), env.abs("render/beauty"), "render", true))

	_, err := env.ProductAdd(ctx, env.branchRef("shot/anim"), env.abs("render/depth"), "render", true)
	assertErrCode(t, err, types.ErrDependencyAlreadyExists)
}
