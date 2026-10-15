package db

import (
	"context"
	"net/url"
	"os"

	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
)

type ReverseEntry struct {
	Type    types.AssetDependencyType `json:"type"`
	Peer    *types.Ref                `json:"peer"` // the consumer (source) or origin (product)
	Name    string                    `json:"name"`
	RefSpec *types.Ref                `json:"ref_spec"`
	Commit  string                    `json:"commit,omitempty"` // pinned target commit (source edges; product pins none yet)
}

// AddReverseEdge records a reverse edge at the resolved target's object, and — for
// a source edge whose consumer pinned a MUTABLE spec (a branch / floating tag) — a
// SECOND copy at that spec's own object (see reverseAlsoAtSpec). The second copy is
// the "live consumers of this tag/branch" index that `source push` reads (it keys
// off the spec, not the resolved commit). Both copies share the same filename
// (keyed by type+peer), so a re-pin overwrites the spec copy in place.
func AddReverseEdge(
	repoPath, targetAssetUID, targetObjectPath string,
	e ReverseEntry,
) error {
	write := func(objPath string) error {
		dir := pathReverseDir(repoPath, targetAssetUID, objPath)
		file := fsutil.PathOS(dir, reverseEdgeFileName(e.Type, e.Peer))
		if err := fsutil.NewJSONHandlerFromPath(file, &e).Write(); err != nil {
			return newErrDBInternal(err, "could not write reverse-dep entry")
		}
		return nil
	}

	if err := write(targetObjectPath); err != nil {
		return err
	}
	if alt := reverseAlsoAtSpec(e, targetObjectPath); alt != "" {
		return write(alt)
	}
	return nil
}

// RemoveReverseEdge deletes a reverse edge at ONE object (targetObjectPath).
//
// It deliberately does NOT touch the mutable-spec second copy (reverseAlsoAtSpec).
// Its only caller is `source pull`, which RE-PINS a consumer: it removes the stale
// resolved-commit copy (the OldTarget commit object, never the spec object) and
// then re-adds at the new commit, overwriting the spec copy in place — so the spec
// copy must survive. Removing it here would also risk dropping the consumer from
// the source-push index if the best-effort re-add then failed. A future true
// source-REMOVE (none exists yet) must clear BOTH copies — use reverseAlsoAtSpec to
// find the second, exactly as AddReverseEdge does.
func RemoveReverseEdge(
	repoPath, targetAssetUID, targetObjectPath string,
	t types.AssetDependencyType,
	peer *types.Ref,
) error {
	dir := pathReverseDir(repoPath, targetAssetUID, targetObjectPath)
	file := fsutil.PathOS(dir, reverseEdgeFileName(t, peer))
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return newErrDBInternal(err, "could not remove reverse-dep entry")
	}
	return nil
}

// reverseAlsoAtSpec returns the SECOND object path a reverse edge is mirrored at —
// the mutable spec's own object — or "" when the edge is single-written. A source
// edge is mirrored there when its consumer is a branch pinning a mutable spec (a
// branch / floating tag) whose object differs from the resolved target's. This is
// the single source of truth for the dual-write predicate, shared by Add (and, when
// it lands, a true remove).
func reverseAlsoAtSpec(e ReverseEntry, targetObjectPath string) string {
	if e.Type == types.AssetDependencySource &&
		e.Peer != nil && e.Peer.RefType == types.RefTypeBranch &&
		e.RefSpec != nil && e.RefSpec.IsMutable() &&
		e.RefSpec.ObjectPath() != targetObjectPath {
		return e.RefSpec.ObjectPath()
	}
	return ""
}

// ListReverseEdges returns a target object's reverse edges, filtered by type (pass
// "" for all). A missing directory means none (not an error). Entry Refs' RepoPath
// (json:"-") is rehydrated here from each Ref's RepoUID.
func ListReverseEdges(
	opctx *opcontext.OperationContext,
	repoPath, targetAssetUID, targetObjectPath string,
	filter types.AssetDependencyType,
) ([]ReverseEntry, error) {
	var ret []ReverseEntry
	if err := readReverseDir(
		opctx, pathReverseDir(repoPath, targetAssetUID, targetObjectPath), filter, &ret,
	); err != nil {
		return nil, err
	}
	return ret, nil
}

// MarkDependentsStale drops a per-OBJECT reverse-stale marker on every object whose
// reverse changed when a source edge into targetB was added: targetB and every
// object in its forward closure.
func MarkDependentsStale(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	targetB *types.Ref,
) error {
	gB, err := GetDependencies(ctx, opctx, dbClient, targetB)
	if err != nil {
		return err
	}
	idx, err := dbClient.GetTreeIndex(ctx, opctx, targetB.RepoPath)
	if err != nil {
		return err
	}
	for repoUID, byAsset := range gB.Nodes {
		repoPath := opctx.RepoRefFor(repoUID).Path
		for assetPath := range byAsset {
			y := &types.Ref{RepoUID: repoUID, RepoPath: repoPath, AssetPath: assetPath}
			uid, err := dbClient.assetUIDFromIndex(ctx, opctx, idx, y)
			if err != nil {
				return err
			}
			if err := touchMarker(pathReverseStale(repoPath, uid)); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetDependents returns the reverse SOURCE closure of ref's OBJECT — every object
// that transitively CONSUMES it. It reads only source-typed reverse edges (never
// producers), so impact ops like source push never pick up producers.
func GetDependents(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	ref *types.Ref,
) (*types.DepGraph, error) {
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, ref, false)
	if err != nil {
		return nil, err
	}
	uid := asset.Data.AssetUID
	objPath := ref.ObjectPath()
	cachePath := pathDepGraph(ref.RepoPath, uid)
	markerPath := pathReverseStale(ref.RepoPath, uid)

	// 1. cache: fresh hit if present and the asset isn't marked stale.
	cache, err := readGraphCache(cachePath)
	if err != nil {
		return nil, err
	}
	stale := cache != nil && markerExists(markerPath)
	if cache != nil && !stale {
		if g, ok := cache.Reverse[objPath]; ok {
			return g, nil
		}
	}

	// 2. rebuild from the reverse index (direct consumers) + recursion into each
	g := types.NewDepGraph()
	if _, rootNode, derr := dependenciesForRef(asset.Data, ref); derr == nil {
		g.AddNode(ref, rootNode)
	} else {
		return nil, derr
	}

	entries, err := ListReverseEdges(opctx, ref.RepoPath, uid, objPath, types.AssetDependencySource)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		cons := e.Peer

		cAsset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, cons, false)
		if err != nil {
			return nil, err
		}
		if _, cNode, derr := dependenciesForRef(cAsset.Data, cons); derr == nil {
			g.AddNode(cons, cNode)
		} else {
			return nil, derr
		}

		g.Edges = append(g.Edges, types.DependencyGraphEdge{
			Consumer: cons,
			Provider: ref,
			Name:     e.Name,
			Type:     types.AssetDependencySource,
			RefSpec:  e.RefSpec,
		})

		childG, err := GetDependents(ctx, opctx, dbClient, cons)
		if err != nil {
			return nil, err
		}
		g.Merge(childG)
	}

	if cache == nil {
		cache = newGraphCache()
	}
	if cache.Reverse == nil {
		cache.Reverse = make(map[string]*types.DepGraph)
	}
	cache.Reverse[objPath] = g
	if err := fsutil.NewJSONHandlerFromPath(cachePath, cache).Write(); err != nil {
		return nil, newErrDBInternal(err, "could not write reverse graph to dep cache")
	}
	return g, nil
}

func ListAllReverseEdges(
	opctx *opcontext.OperationContext,
	repoPath, assetUID string,
) ([]ReverseEntry, error) {
	base := pathReverseBase(repoPath, assetUID)
	objDirs, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, newErrDBInternal(err, "could not read reverse-dep base dir")
	}
	var ret []ReverseEntry
	for _, od := range objDirs {
		if !od.IsDir() {
			continue
		}
		if err := readReverseDir(opctx, fsutil.PathOS(base, od.Name()), "", &ret); err != nil {
			return nil, err
		}
	}
	return ret, nil
}

func GetUsedBy(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	ref *types.Ref,
) ([]*types.Ref, error) {
	visited := map[string]bool{}
	var out []*types.Ref
	if err := walkUsedBy(ctx, opctx, dbClient, ref, visited, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func GetProducedBy(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	ref *types.Ref,
) ([]*types.Ref, error) {
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, ref, false)
	if err != nil {
		return nil, err
	}
	all, err := ListAllReverseEdges(opctx, ref.RepoPath, asset.Data.AssetUID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []*types.Ref
	for i := range all {
		e := all[i]
		if e.Type != types.AssetDependencyProduct || e.Peer == nil {
			continue
		}
		k := usedByKey(e.Peer)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e.Peer)
	}
	return out, nil
}

func usedByKey(ref *types.Ref) string {
	return ref.RepoUID + "\x00" + ref.AssetPath + "\x00" + ref.ObjectPath()
}

func walkUsedBy(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	ref *types.Ref,
	visited map[string]bool,
	out *[]*types.Ref,
) error {
	if visited[usedByKey(ref)] {
		return nil
	}
	visited[usedByKey(ref)] = true

	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, ref, false)
	if err != nil {
		return err
	}
	_, commitID, err := ResolveCommitForDependency(asset.Data, ref)
	if err != nil {
		return err
	}

	all, err := ListAllReverseEdges(opctx, ref.RepoPath, asset.Data.AssetUID)
	if err != nil {
		return err
	}
	for i := range all {
		e := all[i]
		if e.Type != types.AssetDependencySource || e.Commit != commitID || e.Peer == nil {
			continue
		}
		if visited[usedByKey(e.Peer)] {
			continue
		}
		*out = append(*out, e.Peer)
		if err := walkUsedBy(ctx, opctx, dbClient, e.Peer, visited, out); err != nil {
			return err
		}
	}

	deps, _, err := dependenciesForRef(asset.Data, ref)
	if err != nil {
		return err
	}
	for _, dep := range deps {
		if dep.Type != types.AssetDependencyProduct || dep.RefResolved == nil {
			continue
		}
		prod := *dep.RefResolved // copy; rehydrate RepoPath without mutating the loaded asset
		if prod.RepoPath == "" {
			prod.RepoPath = opctx.RepoRefFor(prod.RepoUID).Path
		}
		if err := walkUsedBy(ctx, opctx, dbClient, &prod, visited, out); err != nil {
			return err
		}
	}
	return nil
}

// RecordDependentsForCommit records, for each source dep of a freshly-created
// commit object, a reverse (source) edge on the target keyed at the target's
// object, with the consumer = THIS COMMIT.
func RecordDependentsForCommit(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	commitRef *types.Ref,
) error {
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, commitRef, false)
	if err != nil {
		return err
	}
	commit, err := GetCommit(asset.Data, commitRef.CommitID)
	if err != nil {
		return err
	}
	idx, err := dbClient.GetTreeIndex(ctx, opctx, commitRef.RepoPath)
	if err != nil {
		return err
	}
	for name, dep := range commit.Dependencies {
		if dep.Type != types.AssetDependencySource {
			continue
		}
		target := dep.RefResolved
		targetUID, err := dbClient.assetUIDFromIndex(ctx, opctx, idx, target)
		if err != nil {
			return err
		}
		if err := AddReverseEdge(
			target.RepoPath, targetUID, target.ObjectPath(),
			ReverseEntry{
				Type:    types.AssetDependencySource,
				Peer:    commitRef,
				Name:    name,
				RefSpec: dep.RefSpec,
				Commit:  dep.Commit,
			},
		); err != nil {
			return err
		}
		if err := MarkDependentsStale(ctx, opctx, dbClient, target); err != nil {
			return err
		}
	}
	return nil
}

// -- FILE LOCAL -------------------------------------------------------------

func touchMarker(path string) error {
	if err := fsutil.WriteFileAtomic(path, 0o644, func(*os.File) error { return nil }); err != nil {
		return newErrDBInternal(err, "could not write reverse-stale marker")
	}
	return nil
}

func markerExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func pathReverseStale(repoPath, assetUID string) string {
	return fsutil.PathOS(
		repoPath,
		".vit", "cache", "depgraph",
		assetUID[:2], assetUID[2:]+".reverse_stale",
	)
}

// pathReverseBase is the per-asset reverse-index root (.rdeps); each object's edges
// live in a QueryEscape(objectPath) subdir under it.
func pathReverseBase(repoPath, assetUID string) string {
	return fsutil.PathOS(
		repoPath,
		".vit", "cache", "depgraph",
		assetUID[:2], assetUID[2:]+".rdeps",
	)
}

func pathReverseDir(repoPath, targetAssetUID, targetObjectPath string) string {
	return fsutil.PathOS(
		pathReverseBase(repoPath, targetAssetUID),
		url.QueryEscape(targetObjectPath),
	)
}

// readReverseDir appends every ReverseEntry in one object's reverse dir to out,
// keeping only those of type `filter` ("" = all). A missing dir is not an error.
// Each entry's Ref RepoPath (json:"-") is rehydrated from its RepoUID.
func readReverseDir(
	opctx *opcontext.OperationContext,
	dir string,
	filter types.AssetDependencyType,
	out *[]ReverseEntry,
) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return newErrDBInternal(err, "could not read reverse-dep dir")
	}
	fillRepoPath := func(ref *types.Ref) {
		if ref != nil {
			ref.RepoPath = opctx.RepoRefFor(ref.RepoUID).Path
		}
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		h := fsutil.NewJSONHandlerFromPath(fsutil.PathOS(dir, f.Name()), &ReverseEntry{})
		if err := h.Read(); err != nil {
			return newErrDBInternal(err, "could not read reverse-dep entry")
		}
		if filter != "" && h.Data.Type != filter {
			continue
		}
		fillRepoPath(h.Data.Peer)
		fillRepoPath(h.Data.RefSpec)
		*out = append(*out, *h.Data)
	}
	return nil
}

func reverseEdgeFileName(t types.AssetDependencyType, peer *types.Ref) string {
	return string(t) + "_" + peer.RepoUID + "_" +
		url.QueryEscape(peer.AssetPath) +
		url.QueryEscape(peer.ObjectPath()) + ".json"
}
