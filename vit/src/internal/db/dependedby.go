package db

import (
	"context"
	"net/url"
	"os"

	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
)

// DependentEntry is the reverse dependency index ("who depends on me").
// It is a directory-as-set: one file per (consumer object) writer inside the
// target asset's depended_by dir. Distinct filenames per writer mean concurrent
// adds never collide, so writing a reverse edge needs NO lock on the target
// (same idiom as .vit/transaction/).
type DependentEntry struct {
	Consumer     *types.Ref `json:"consumer"`
	Name         string     `json:"name"`
	RefSpec      *types.Ref `json:"ref_spec"`
	TargetCommit string     `json:"target_commit"`
}

// AddDependent records that consumer depends (under dependency name) on the
// target asset's OBJECT, write on disk
func AddDependent(
	repoPath, targetAssetUID, targetObjectPath, name string,
	consumer *types.Ref,
	refSpec *types.Ref,
	targetCommit string,
) error {
	dir := pathDependedByDir(repoPath, targetAssetUID, targetObjectPath)
	file := fsutil.PathOS(dir, dependedByFileName(consumer))
	entry := DependentEntry{
		Consumer:     consumer,
		Name:         name,
		RefSpec:      refSpec,
		TargetCommit: targetCommit,
	}
	if err := fsutil.NewJSONHandlerFromPath(file, &entry).Write(); err != nil {
		return newErrDBInternal(err, "could not write depended_by entry")
	}
	return nil
}

// RemoveDependent deletes the reverse-index entry recording that consumer depended
// on the target asset's OBJECT — the inverse of AddDependent.
func RemoveDependent(
	repoPath, targetAssetUID, targetObjectPath string,
	consumer *types.Ref,
) error {
	dir := pathDependedByDir(repoPath, targetAssetUID, targetObjectPath)
	file := fsutil.PathOS(dir, dependedByFileName(consumer))
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return newErrDBInternal(err, "could not remove depended_by entry")
	}
	return nil
}

// ListDependents returns every consumer of a specific target OBJECT. A missing
// directory means nobody depends on that object (not an error). The entry Refs'
// RepoPath (json:"-") is rehydrated here — at this deserialization boundary — from
// each Ref's RepoUID, so callers get complete Refs (mirrors the asset-load path).
func ListDependents(
	opctx *opcontext.OperationContext,
	repoPath, targetAssetUID, targetObjectPath string,
) ([]DependentEntry, error) {
	dir := pathDependedByDir(repoPath, targetAssetUID, targetObjectPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, newErrDBInternal(err, "could not read depended_by dir")
	}
	fillRepoPath := func(ref *types.Ref) {
		if ref != nil {
			ref.RepoPath = opctx.RepoRefFor(ref.RepoUID).Path
		}
	}
	ret := make([]DependentEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		h := fsutil.NewJSONHandlerFromPath(fsutil.PathOS(dir, e.Name()), &DependentEntry{})
		if err := h.Read(); err != nil {
			return nil, newErrDBInternal(err, "could not read depended_by entry")
		}
		fillRepoPath(h.Data.Consumer)
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
		for assetPath, objs := range byAsset {
			y := &types.Ref{RepoUID: repoUID, RepoPath: repoPath, AssetPath: assetPath}
			uid, err := dbClient.assetUIDFromIndex(ctx, opctx, idx, y)
			if err != nil {
				return err
			}
			for objPath := range objs {
				if err := touchMarker(pathReverseStale(repoPath, uid, objPath)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// GetDependents returns the reverse closure of ref's OBJECT — every object that
// transitively depends on it.
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
	resolved, _, err := ResolveCommitForDependency(asset.Data, ref)
	if err != nil {
		return nil, err
	}
	resolved.RepoPath = ref.RepoPath
	objPath := resolved.ObjectPath()
	cachePath := pathDepGraph(ref.RepoPath, uid)
	markerPath := pathReverseStale(ref.RepoPath, uid, objPath)

	// 1. try to get from cache.
	cache, err := readGraphCache(cachePath)
	if err != nil {
		return nil, err
	}
	if cache != nil && !markerExists(markerPath) {
		if g, ok := cache.Reverse[objPath]; ok {
			return g, nil
		}
	}

	// 2. in other case, refresh cache using by recc comparing DependentEntry
	// with stale markers.
	g := types.NewDepGraph()
	if _, rootNode, derr := dependenciesForRef(asset.Data, resolved); derr == nil {
		g.AddNode(resolved, rootNode)
	} else {
		return nil, derr
	}

	entries, err := ListDependents(opctx, ref.RepoPath, uid, objPath)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		cons := e.Consumer

		cAsset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, cons, false)
		if err != nil {
			return nil, err
		}
		cResolved, _, err := ResolveCommitForDependency(cAsset.Data, cons)
		if err != nil {
			return nil, err
		}
		cResolved.RepoPath = cons.RepoPath
		if _, cNode, derr := dependenciesForRef(cAsset.Data, cResolved); derr == nil {
			g.AddNode(cResolved, cNode)
		} else {
			return nil, derr
		}

		g.Edges = append(g.Edges, types.DependencyGraphEdge{
			Consumer: cResolved,
			Provider: resolved,
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
	_ = os.Remove(markerPath) // best-effort clear
	return g, nil
}

// RecordDependentsForCommit records, for each source dep of a freshly-created
// commit object, a reverse edge on the target keyed at the target's object, with
// the consumer = THIS COMMIT
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
		if err := AddDependent(
			target.RepoPath,
			targetUID,
			target.ObjectPath(),
			name,
			commitRef,
			dep.RefSpec,
			dep.Commit,
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

func pathReverseStale(repoPath, assetUID, objectPath string) string {
	return fsutil.PathOS(
		repoPath,
		".vit", "cache", "depgraph",
		assetUID[:2], assetUID[2:]+".reverse_stale",
		url.QueryEscape(objectPath),
	)
}

func pathDependedByDir(repoPath, targetAssetUID, targetObjectPath string) string {
	return fsutil.PathOS(
		repoPath,
		".vit", "cache", "depgraph",
		targetAssetUID[:2], targetAssetUID[2:]+".depended_by",
		url.QueryEscape(targetObjectPath),
	)
}

func dependedByFileName(consumer *types.Ref) string {
	return consumer.RepoUID + "_" +
		url.QueryEscape(consumer.AssetPath) +
		url.QueryEscape(consumer.ObjectPath()) + ".json"
}
