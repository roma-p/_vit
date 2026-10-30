package vit_test

import (
	"path/filepath"
	"testing"

	"vit/internal/types"
)

// lineageHas reports whether an ls'd object's lineage has an edge of type t from
// the consumer asset to the provider asset.
func lineageHas(res *types.LsResult, t types.AssetDependencyType, consumer, provider string) bool {
	if res.Object == nil || res.Object.Lineage == nil {
		return false
	}
	for _, e := range res.Object.Lineage.Edges {
		if e.Type == t && e.Consumer.AssetPath == consumer && e.Provider.AssetPath == provider {
			return true
		}
	}
	return false
}

func lineageHasNode(res *types.LsResult, assetPath string) bool {
	if res.Object == nil || res.Object.Lineage == nil {
		return false
	}
	for _, byAsset := range res.Object.Lineage.Nodes {
		if _, ok := byAsset[assetPath]; ok {
			return true
		}
	}
	return false
}

// produce makes an origin produce into its product: write a file through the
// product link, change the origin, snapshot (captures the product) and tag.
func (r *vitEnv) produce(origin, product, file, content string) {
	ws := r.baseWs(origin)
	r.editFile(filepath.Join(ws, product, file), content)
	r.editFile(filepath.Join(ws, "src"), content)
	must(r.Snapshot(ctx, r.branchRef(origin), origin+" produced "+content))
	must(r.Tag(ctx, r.branchRef(origin), "approved", origin+" approved"))
}

// mod —product→ geo; anim sources geo, —product→ cache (and a sibling lowres);
// prerender sources cache. prerender's lineage walks back through both producers.
func TestLineage_transitiveThroughProducers(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("asset/mod"), env.fileSrc("src", "m1")))
	must(env.ProductAdd(ctx, env.branchRef("asset/mod"), env.abs("asset/geo"), "geo", true))
	env.produce("asset/mod", "geo", "geo.abc", "g1")

	must(env.Add(ctx, env.abs("shot/anim"), env.fileSrc("src", "a1")))
	must(env.SourceAdd(ctx, env.branchRef("shot/anim"), env.abs("asset/geo")+"@tag=approved", "geo"))
	must(env.ProductAdd(ctx, env.branchRef("shot/anim"), env.abs("shot/cache"), "cache", true))
	must(env.ProductAdd(ctx, env.branchRef("shot/anim"), env.abs("shot/lowres"), "lowres", true))
	ws := env.baseWs("shot/anim")
	env.editFile(filepath.Join(ws, "lowres", "low.abc"), "l1")
	env.produce("shot/anim", "cache", "cache.abc", "c1")

	must(env.Add(ctx, env.abs("shot/prerender"), env.fileSrc("src", "p1")))
	must(env.SourceAdd(ctx, env.branchRef("shot/prerender"), env.abs("shot/cache")+"@tag=approved", "cache"))

	res := must(env.Ls(ctx, env.branchRef("shot/prerender")))
	for _, c := range []struct {
		t                  types.AssetDependencyType
		consumer, provider string
	}{
		{types.AssetDependencyProduct, "shot/anim", "shot/cache"},
		{types.AssetDependencySource, "shot/anim", "asset/geo"},
		{types.AssetDependencyProduct, "asset/mod", "asset/geo"},
	} {
		if !lineageHas(res, c.t, c.consumer, c.provider) {
			t.Fatalf("prerender lineage: missing %s —%s→ %s in %+v", c.consumer, c.t, c.provider, res.Object.Lineage)
		}
	}
	if lineageHasNode(res, "shot/lowres") {
		t.Fatal("prerender lineage: sibling product shot/lowres must not appear")
	}
	if res.Object.Dependencies != nil {
		for _, e := range res.Object.Dependencies.Edges {
			if e.Consumer.AssetPath == "shot/anim" {
				t.Fatalf("forward graph must stay lineage-free, got %+v", e)
			}
		}
	}
}

// A product's own ls shows its producer's lineage; an object with no producer
// anywhere upstream has none.
func TestLineage_productAndNone(t *testing.T) {
	env := newVitEnv(t)
	must(env.Add(ctx, env.abs("shot/anim"), env.fileSrc("src", "a1")))
	must(env.ProductAdd(ctx, env.branchRef("shot/anim"), env.abs("shot/cache"), "cache", true))
	env.produce("shot/anim", "cache", "cache.abc", "c1")

	if !lineageHas(must(env.Ls(ctx, env.abs("shot/cache")+"@tag=approved")),
		types.AssetDependencyProduct, "shot/anim", "shot/cache") {
		t.Fatal("cache@tag=approved lineage: expected produced by shot/anim")
	}
	if l := must(env.Ls(ctx, env.branchRef("shot/anim"))).Object.Lineage; l != nil {
		t.Fatalf("anim has no producer upstream, expected no lineage, got %+v", l)
	}
}

// The producer lives in ANOTHER repo: lineage crosses back into it.
func TestLineage_crossRepo(t *testing.T) {
	a := newVitEnv(t)
	b := a.sibling("repoB")
	must(a.Add(ctx, a.abs("shot/anim"), a.fileSrc("src", "a1")))
	must(a.ProductAdd(ctx, a.branchRef("shot/anim"), b.abs("shot/cache"), "cache", true))
	a.produce("shot/anim", "cache", "cache.abc", "c1")

	must(b.Add(ctx, b.abs("shot/prerender"), b.fileSrc("src", "p1")))
	must(b.SourceAdd(ctx, b.branchRef("shot/prerender"), b.abs("shot/cache")+"@tag=approved", "cache"))

	res := must(b.Ls(ctx, b.branchRef("shot/prerender")))
	if !lineageHas(res, types.AssetDependencyProduct, "shot/anim", "shot/cache") {
		t.Fatalf("repo B prerender lineage: expected repo A shot/anim producing cache, got %+v", res.Object.Lineage)
	}
	for _, e := range res.Object.Lineage.Edges {
		if e.Consumer.AssetPath == "shot/anim" && e.Consumer.RepoUID != a.repoUID(t) {
			t.Fatalf("producer should be in repo A %q, got %q", a.repoUID(t), e.Consumer.RepoUID)
		}
	}
}
