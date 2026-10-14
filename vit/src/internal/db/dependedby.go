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
	if e.Type == types.AssetDependencySource &&
		e.Peer.RefType == types.RefTypeBranch && e.RefSpec.IsMutable() &&
		e.RefSpec.ObjectPath() != targetObjectPath {
		if err := write(e.RefSpec.ObjectPath()); err != nil {
			return err
		}
	}
	return nil
}

// RemoveReverseEdge deletes a typed reverse edge — the inverse of AddReverseEdge.
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

// ListReverseEdges returns a target object's reverse edges, filtered by type (pass
// "" for all). A missing directory means none (not an error). Entry Refs' RepoPath
// (json:"-") is rehydrated here from each Ref's RepoUID.
func ListReverseEdges(
	opctx *opcontext.OperationContext,
	repoPath, targetAssetUID, targetObjectPath string,
	filter types.AssetDependencyType,
) ([]ReverseEntry, error) {
	dir := pathReverseDir(repoPath, targetAssetUID, targetObjectPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, newErrDBInternal(err, "could not read reverse-dep dir")
	}
	fillRepoPath := func(ref *types.Ref) {
		if ref != nil {
			ref.RepoPath = opctx.RepoRefFor(ref.RepoUID).Path
		}
	}
	ret := make([]ReverseEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		h := fsutil.NewJSONHandlerFromPath(fsutil.PathOS(dir, e.Name()), &ReverseEntry{})
		if err := h.Read(); err != nil {
			return nil, newErrDBInternal(err, "could not read reverse-dep entry")
		}
		if filter != "" && h.Data.Type != filter {
			continue
		}
		fillRepoPath(h.Data.Peer)
		fillRepoPath(h.Data.RefSpec)
		ret = append(ret, *h.Data)
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

func pathReverseDir(repoPath, targetAssetUID, targetObjectPath string) string {
	return fsutil.PathOS(
		repoPath,
		".vit", "cache", "depgraph",
		targetAssetUID[:2], targetAssetUID[2:]+".rdeps",
		url.QueryEscape(targetObjectPath),
	)
}

func reverseEdgeFileName(t types.AssetDependencyType, peer *types.Ref) string {
	return string(t) + "_" + peer.RepoUID + "_" +
		url.QueryEscape(peer.AssetPath) +
		url.QueryEscape(peer.ObjectPath()) + ".json"
}
