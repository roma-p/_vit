package vit_test

import (
	"os"
	"path/filepath"
	"testing"

	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

// -- helpers ----------------------------------------------------------------

// branchWs is an asset's base-branch workspace dir (where its product symlinks
// and content live).
func (r *vitEnv) baseWs(assetPath string) string {
	return filepath.Join(r.abs(assetPath), "branches", "branch-base")
}

// productEdge returns the forward product edge named `name` on an ls'd object.
func productEdge(res *types.LsResult, name string) *types.DependencyGraphEdge {
	if res.Object == nil || res.Object.Dependencies == nil {
		return nil
	}
	for i := range res.Object.Dependencies.Edges {
		e := &res.Object.Dependencies.Edges[i]
		if e.Type == types.AssetDependencyProduct && e.Name == name {
			return e
		}
	}
	return nil
}

func hasTagFamily(res *types.LsResult, fam string) bool {
	for _, f := range res.TagFamilies {
		if f.Family == fam {
			return true
		}
	}
	return false
}

func tagLatest(res *types.LsResult, fam string) string {
	for _, f := range res.TagFamilies {
		if f.Family == fam {
			return f.Latest
		}
	}
	return ""
}

func hasBranch(res *types.LsResult, name string) bool {
	for _, b := range res.Branches {
		if b == name {
			return true
		}
	}
	return false
}

func usedByHas(res *types.LsResult, assetPath string) bool {
	if res.Object == nil {
		return false
	}
	for _, r := range res.Object.UsedBy {
		if r.AssetPath == assetPath {
			return true
		}
	}
	return false
}

func producedByHas(res *types.LsResult, assetPath string) bool {
	if res.Object == nil {
		return false
	}
	for _, r := range res.Object.ProducedBy {
		if r.AssetPath == assetPath {
			return true
		}
	}
	return false
}

// producerRef returns the produced-by entry for assetPath of the given ref type, or
// nil if absent.
func producerRef(res *types.LsResult, assetPath string, rt types.RefType) *types.Ref {
	if res.Object == nil {
		return nil
	}
	for _, r := range res.Object.ProducedBy {
		if r.AssetPath == assetPath && r.RefType == rt {
			return r
		}
	}
	return nil
}

// originWithProduct adds an origin asset and attaches a `render` product (created
// empty). Returns the origin's base-branch workspace dir.
func (r *vitEnv) originWithProduct(t *testing.T, origin, seed string) string {
	t.Helper()
	must(r.Add(ctx, r.abs(origin), r.fileSrc("src", seed)))
	must(r.ProductAdd(ctx, r.branchRef(origin), r.abs("render/beauty"), "render", true))
	return r.baseWs(origin)
}

// -- SNAPSHOT cascade -------------------------------------------------------

// A snapshot captures a CHANGED product and pins it on the origin commit
// (RefResolved → a product commit); an origin snapshot where the product did NOT
// change leaves it unproduced (RefResolved nil, still visible).
func TestSnapshotCascade_capturesChangedProductElseNone(t *testing.T) {
	env := newVitEnv(t)
	ws := env.originWithProduct(t, "shot/anim", "v1")

	// render + change origin → snapshot captures render.
	env.editFile(filepath.Join(ws, "render", "out.exr"), "frame1")
	env.editFile(filepath.Join(ws, "src"), "v2")
	snap1 := must(env.Snapshot(ctx, env.branchRef("shot/anim"), "v2+render")).String

	e1 := productEdge(must(env.Ls(ctx, snap1)), "render")
	if e1 == nil {
		t.Fatal("v2 snapshot: no product edge")
	}
	if e1.Provider == nil || e1.Provider.RefType != types.RefTypeCommit {
		t.Fatalf("v2 snapshot: expected render produced (provider=commit), got %+v", e1.Provider)
	}

	// change origin only → render unchanged → not produced.
	env.editFile(filepath.Join(ws, "src"), "v3")
	snap2 := must(env.Snapshot(ctx, env.branchRef("shot/anim"), "v3 no render")).String

	e2 := productEdge(must(env.Ls(ctx, snap2)), "render")
	if e2 == nil {
		t.Fatal("v3 snapshot: product edge should still be present (declared)")
	}
	if e2.Provider != nil {
		t.Fatalf("v3 snapshot: expected render NOT produced (provider=nil), got %+v", e2.Provider)
	}
}

// -- TAG cascade ------------------------------------------------------------

// Tagging an origin tags the product commit it produced; tagging an origin
// version that produced nothing does not move the product's tag.
func TestTagCascade_mirrorsProducedProductElseUntouched(t *testing.T) {
	env := newVitEnv(t)
	ws := env.originWithProduct(t, "shot/anim", "v1")
	env.editFile(filepath.Join(ws, "render", "out.exr"), "frame1")
	env.editFile(filepath.Join(ws, "src"), "v2")

	// tag the dirty origin branch → commits (captures render) + tags both.
	must(env.Tag(ctx, env.branchRef("shot/anim"), "approved", "approve v2"))

	renderLatest := tagLatest(must(env.Ls(ctx, env.abs("render/beauty"))), "approved")
	if renderLatest == "" {
		t.Fatal("render was not tagged 'approved'")
	}
	// render@tag=approved must resolve (no error) to a product object.
	if must(env.Ls(ctx, env.abs("render/beauty")+"@tag=approved")).Object == nil {
		t.Fatal("render@tag=approved did not resolve to an object")
	}

	// tag an origin version that produced NO render → render's approved unchanged,
	// while the origin's approved DOES advance (confirms the second tag happened).
	env.editFile(filepath.Join(ws, "src"), "v3")
	must(env.Snapshot(ctx, env.branchRef("shot/anim"), "v3 no render"))
	must(env.Tag(ctx, env.branchRef("shot/anim"), "approved", "approve v3"))

	if got := tagLatest(must(env.Ls(ctx, env.abs("render/beauty"))), "approved"); got != renderLatest {
		t.Fatalf("render 'approved' moved on a no-render tag: %q -> %q", renderLatest, got)
	}
	originLatest := tagLatest(must(env.Ls(ctx, env.abs("shot/anim"))), "approved")
	if originLatest == renderLatest {
		t.Fatalf("origin 'approved' did not advance (expected a new version past %q)", renderLatest)
	}
}

// Viewing a TAGGED origin shows its product as the matching-family TAG (what the
// cascade mirrored), not the raw produced commit.
func TestTagView_productResolvesToTag(t *testing.T) {
	env := newVitEnv(t)
	ws := env.originWithProduct(t, "shot/lgt", "v1")
	env.editFile(filepath.Join(ws, "render", "out.exr"), "frame1")
	env.editFile(filepath.Join(ws, "src"), "v2")
	must(env.Tag(ctx, env.branchRef("shot/lgt"), "approved", "approve")) // dirty-branch: capture + tag

	e := productEdge(must(env.Ls(ctx, env.abs("shot/lgt")+"@tag=approved")), "render")
	if e == nil || e.Provider == nil {
		t.Fatalf("no resolved product edge: %+v", e)
	}
	if e.Provider.RefType != types.RefTypeTag || e.Provider.TagFamily != "approved" {
		t.Fatalf("expected product resolved to render@tag=approved, got %+v", e.Provider)
	}
	if e.Provider.TagName == "" {
		t.Fatalf("expected a precise named product tag, got floating: %+v", e.Provider)
	}
}

// -- BRANCH mirror ----------------------------------------------------------

// Branching an origin re-points its product edge to the product's same-named
// branch (created from base), and rendering through the new branch is isolated.
func TestBranchMirror_repointsAndIsolates(t *testing.T) {
	env := newVitEnv(t)
	ws := env.originWithProduct(t, "shot/anim", "v1")
	env.editFile(filepath.Join(ws, "render", "out.exr"), "frame1")
	env.editFile(filepath.Join(ws, "src"), "v2")
	must(env.Snapshot(ctx, env.branchRef("shot/anim"), "v2+render")) // origin clean

	must(env.Branch(ctx, env.branchRef("shot/anim"), "wip"))

	// origin@wip product edge re-pointed to render/beauty@branch=wip.
	owip := must(env.Ls(ctx, env.abs("shot/anim")+"@branch=wip"))
	e := productEdge(owip, "render")
	if e == nil || e.RefSpec == nil ||
		e.RefSpec.AssetPath != "render/beauty" || e.RefSpec.BranchName != "wip" {
		t.Fatalf("origin@wip render edge not re-pointed to render/beauty@branch=wip: %+v", e)
	}
	// Provider is the LIVE wip branch, not an inherited base commit: nothing is
	// produced on wip yet (RefResolved cleared on branch).
	if e.Provider == nil ||
		e.Provider.RefType != types.RefTypeBranch || e.Provider.BranchName != "wip" {
		t.Fatalf("origin@wip render provider should be render@branch=wip (not a stale pin), got %+v", e.Provider)
	}

	// render asset gained a wip branch.
	if !hasBranch(must(env.Ls(ctx, env.abs("render/beauty"))), "wip") {
		t.Fatal("render has no 'wip' branch")
	}

	// the wip product branch starts EMPTY — no render produced on it yet.
	wipRenderWs := filepath.Join(env.abs("render/beauty"), "branches", "branch-wip")
	if entries, err := os.ReadDir(wipRenderWs); err != nil || len(entries) != 0 {
		t.Fatalf("render@wip should start empty, got entries=%v err=%v", entries, err)
	}

	// produced by: the NEW branch's producer is recorded too (origin@wip), not only base.
	rwip := must(env.Ls(ctx, env.abs("render/beauty")+"@branch=wip"))
	foundWipProducer := false
	for _, r := range rwip.Object.ProducedBy {
		if r.AssetPath == "shot/anim" && r.BranchName == "wip" {
			foundWipProducer = true
		}
	}
	if !foundWipProducer {
		t.Fatalf("render@wip: expected a wip-branch producer in produced-by, got %+v", rwip.Object.ProducedBy)
	}

	// write-through isolation: origin@wip/render → render@wip ws; base untouched.
	wipWs := filepath.Join(env.abs("shot/anim"), "branches", "branch-wip")
	env.editFile(filepath.Join(wipWs, "render", "out.exr"), "WIP")
	testutils.AssertEqual(t,
		env.read(filepath.Join(env.abs("render/beauty"), "branches", "branch-wip", "out.exr")), "WIP")
	testutils.AssertEqual(t,
		env.read(filepath.Join(env.abs("render/beauty"), "branches", "branch-base", "out.exr")), "frame1")
}

// -- USED BY (downstream dataflow) ------------------------------------------

// `ls` shows downstream consumers: a product's direct consumer, and a producer's
// consumer reached THROUGH the product (lgt → render → comp).
func TestUsedBy_directAndViaProduct(t *testing.T) {
	env := newVitEnv(t)
	ws := env.originWithProduct(t, "shot/lgt", "v1")
	env.editFile(filepath.Join(ws, "render", "out.exr"), "frame1")
	env.editFile(filepath.Join(ws, "src"), "v2")
	must(env.Snapshot(ctx, env.branchRef("shot/lgt"), "v2+render"))
	must(env.Tag(ctx, env.branchRef("shot/lgt"), "approved", "approve"))

	// comp consumes the approved render.
	must(env.Add(ctx, env.abs("shot/comp"), env.fileSrc("comp.nk", "c1")))
	must(env.SourceAdd(ctx, env.branchRef("shot/comp"),
		env.abs("render/beauty")+"@tag=approved", ".src/render"))

	// direct: render used by comp.
	if !usedByHas(must(env.Ls(ctx, env.abs("render/beauty")+"@tag=approved")), "shot/comp") {
		t.Fatal("render@tag=approved: expected used by shot/comp")
	}
	// via product: lgt used by comp (through the render it produced).
	if !usedByHas(must(env.Ls(ctx, env.abs("shot/lgt")+"@tag=approved")), "shot/comp") {
		t.Fatal("shot/lgt@tag=approved: expected used by shot/comp (via render)")
	}
}

// -- PRODUCED BY (upstream producer) ----------------------------------------

// `ls` on a product names its producer: render is produced by lgt (the inverse of
// lgt's forward product edge), visible from any view of the product.
func TestProducedBy_showsOrigin(t *testing.T) {
	env := newVitEnv(t)
	ws := env.originWithProduct(t, "shot/lgt", "v1")
	env.editFile(filepath.Join(ws, "render", "out.exr"), "frame1")
	env.editFile(filepath.Join(ws, "src"), "v2")
	must(env.Snapshot(ctx, env.branchRef("shot/lgt"), "v2+render"))
	must(env.Tag(ctx, env.branchRef("shot/lgt"), "approved", "approve"))

	// branch view: the LIVE producer targeting that branch (shot/lgt@branch=base).
	if producerRef(must(env.Ls(ctx, env.abs("render/beauty")+"@branch=base")),
		"shot/lgt", types.RefTypeBranch) == nil {
		t.Fatal("render@branch=base: expected produced by shot/lgt@branch=base")
	}
	// tag view of an immutable object: the PRECISE producer tag (named, not the
	// floating family) — shot/lgt@tag=approved@name=<M>.
	tp := producerRef(must(env.Ls(ctx, env.abs("render/beauty")+"@tag=approved")),
		"shot/lgt", types.RefTypeTag)
	if tp == nil || tp.TagFamily != "approved" || tp.TagName == "" {
		t.Fatalf("render@tag=approved: expected produced by precise shot/lgt@tag=approved@name=…, got %+v", tp)
	}
	// the producer (lgt) is itself produced by nobody.
	if pb := must(env.Ls(ctx, env.abs("shot/lgt")+"@branch=base")).Object.ProducedBy; len(pb) != 0 {
		t.Fatalf("shot/lgt: expected no producer, got %v", pb)
	}
}
