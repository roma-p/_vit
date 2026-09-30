package db

import (
	"context"
	"os"

	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
)

// GraphCache is the per-asset dep-graph cache file.
// derived from asset json files. contains complete closure for every object,
// not just the direct dependancies. (meaning:: A->B->C->D etc... not just A->B)
// cache is split in two half:
//
//   - forward: "on who, a vit object depends."
//
//     for this part of the cache, we have a complete reccusrive graph.
//
//     A -> B --> ( object)
//     C -> D /
//     E /
//
//   - reverse: who dpeends on me.
//
//     for this part of the cache, we don't  have a forward graph of the object
//     who depends on the object.
//
//     (object) -> A -> B -> C
//     but not
//     (object) -> A --> B -> C
//     ................. D /
//
//     reverse is build in code at dependedby.go
type GraphCache struct {
	Forward map[string]*types.DepGraph `json:"forward"`
	Reverse map[string]*types.DepGraph `json:"reverse,omitempty"`
}

func newGraphCache() *GraphCache {
	return &GraphCache{
		Forward: make(map[string]*types.DepGraph),
	}
}

// GetDependencies returns the dependency closure for ref's object. It is a pure READ:
// /!\ it will refreseh the cash if needed but won't write on disk.
// to update and write a dep graph cache, use: CacheDependencies
func GetDependencies(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	ref *types.Ref,
) (*types.DepGraph, error) {
	idx, err := dbClient.GetTreeIndex(ctx, opctx, ref.RepoPath)
	if err != nil {
		return nil, err
	}
	if entry, ok := idx.PathToID[ref.AssetPath]; ok &&
		entry.Type == TreeNodeTypeAsset && entry.AssetUID != "" {
		cache, err := readGraphCache(pathDepGraph(ref.RepoPath, entry.AssetUID))
		if err != nil {
			return nil, err
		}
		if cache != nil {
			if g, ok := cache.Forward[ref.ObjectPath()]; ok {
				return g, nil // HIT — index lookup + cache read, no JSON.
			}
		}
	}
	// MISS build from truth IN MEMORY, do not persist
	// (only the owning asset's Finish writes the cache, under its own lock).
	return buildDependencies(ctx, opctx, dbClient, ref)
}

// CacheDependencies refresh/build ref's closure from the source of truth and
// writes it into the asset's cache file
func CacheDependencies(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	ref *types.Ref,
) (*types.DepGraph, error) {
	g, err := buildDependencies(ctx, opctx, dbClient, ref)
	if err != nil {
		return nil, err
	}

	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, ref, false)
	if err != nil {
		return nil, err
	}
	cachePath := pathDepGraph(ref.RepoPath, asset.Data.AssetUID)

	cache, err := readGraphCache(cachePath)
	if err != nil {
		return nil, err
	}
	if cache == nil {
		cache = newGraphCache()
	}
	if cache.Forward == nil {
		cache.Forward = make(map[string]*types.DepGraph)
	}
	cache.Forward[ref.ObjectPath()] = g

	if err := fsutil.NewJSONHandlerFromPath(cachePath, cache).Write(); err != nil {
		return nil, newErrDBInternal(err, "could not write dep graph cache")
	}
	return g, nil
}

// -- OPERATION API : update the dep graph after a operation -----------------

// UpdateDepGraphForCommit materializes the new commit's forward closure, refreshes
// the branch head's, and records the commit's reverse edges.
func UpdateDepGraphForCommit(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	commitRef, branchRef *types.Ref,
) error {
	if _, err := CacheDependencies(ctx, opctx, dbClient, commitRef); err != nil {
		return err
	}
	if _, err := CacheDependencies(ctx, opctx, dbClient, branchRef); err != nil {
		return err
	}
	return RecordDependentsForCommit(ctx, opctx, dbClient, commitRef)
}

// UpdateDepGraphForTag materializes the new tag's forward closure.
func UpdateDepGraphForTag(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	tagRef *types.Ref,
) error {
	_, err := CacheDependencies(ctx, opctx, dbClient, tagRef)
	return err
}

// UpdateDepGraphForSource materializes the consumer's forward closure and records
// the reverse edge on the target (+ marks its dependents stale). It returns the
// consumer's closure so the caller can present it into the workspace.
func UpdateDepGraphForSource(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	r UpdateDepGraphForSourceRequest,
) (*types.DepGraph, error) {
	g, err := CacheDependencies(ctx, opctx, dbClient, r.Consumer)
	if err != nil {
		return nil, err
	}
	targetAsset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, r.Target, false)
	if err != nil {
		return g, err
	}
	if err := AddDependent(
		r.Target.RepoPath, targetAsset.Data.AssetUID, r.Target.ObjectPath(),
		r.Name, r.Consumer, r.Spec, r.Commit,
	); err != nil {
		return g, err
	}
	if err := MarkDependentsStale(ctx, opctx, dbClient, r.Target); err != nil {
		return g, err
	}
	return g, nil
}

type UpdateDepGraphForSourceRequest struct {
	Consumer *types.Ref
	Target   *types.Ref
	Spec     *types.Ref
	Name     string
	Commit   string // consumer underlying commit id
}

// CheckSourceAddNoCycle checks that adding a source won't create a cycle.
// /!\ it will refreseh the cash if needed but won't write on disk.
// to update and write a dep graph cache, use: CacheDependencies
func CheckSourceAddNoCycle(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	consumer, target *types.Ref,
) error {
	g, err := GetDependencies(ctx, opctx, dbClient, target)
	if err != nil {
		return err
	}
	return g.CheckAddSourceCycle(consumer, target)
}

// -- FILE LOCAL -------------------------------------------------------------

// buildDependencies assembles ONE object's closure by walking its direct (pinned)
// source edges down to the leaves. /!\ Reads from truth (the asset JSONs).
func buildDependencies(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	root *types.Ref,
) (*types.DepGraph, error) {
	g := types.NewDepGraph()
	visited := map[types.Ref]bool{}
	if err := walkDependencies(ctx, opctx, dbClient, g, visited, root); err != nil {
		return nil, err
	}
	return g, nil
}

// walkDependencies recursively adds ref and its transitive source edges into g.
func walkDependencies(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	g *types.DepGraph,
	visited map[types.Ref]bool,
	ref *types.Ref,
) error {
	if visited[*ref] {
		return nil // already in the closure — dedup by ref.
	}
	visited[*ref] = true

	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, ref, false)
	if err != nil {
		return err
	}
	deps, node, err := dependenciesForRef(asset.Data, ref)
	if err != nil {
		return err
	}
	g.AddNode(ref, node)

	for name, dep := range deps {
		if dep.Type != types.AssetDependencySource {
			continue
		}
		child := dep.RefResolved
		// RepoPath is not persisted (json:"-") — fill it from the ref's UID.
		child.RepoPath = opctx.RepoRefFor(child.RepoUID).Path

		if err := g.Connect(ref, child, dep.RefSpec, name, dep.Type); err != nil {
			return err // if cycle detected return err.
		}
		if err := walkDependencies(ctx, opctx, dbClient, g, visited, child); err != nil {
			return err
		}
	}
	return nil
}

// dependenciesForRef returns, for an object: its direct dependencies and its
// graph node
func dependenciesForRef(
	a *types.Asset,
	ref *types.Ref,
) (types.AssetDependencies, types.DepGraphNode, error) {
	switch ref.RefType {
	case types.RefTypeBranch:
		c, err := GetBranchCommit(a, ref.BranchName)
		if err != nil {
			return nil, types.DepGraphNode{}, err
		}
		return c.Dependencies, types.DepGraphNode{
			PayloadFile: c.Content,
			Author:      c.Author,
			Timestamp:   c.Timestamp,
		}, nil
	case types.RefTypeCommit:
		c, err := GetCommit(a, ref.CommitID)
		if err != nil {
			return nil, types.DepGraphNode{}, err
		}
		return c.Dependencies, types.DepGraphNode{
			PayloadFile: c.Content,
			Author:      c.Author,
			Timestamp:   c.Timestamp,
		}, nil
	case types.RefTypeTag:
		tag, _, err := GetTag(a, ref)
		if err != nil {
			return nil, types.DepGraphNode{}, err
		}
		c, err := GetCommit(a, tag.Commit)
		if err != nil {
			return nil, types.DepGraphNode{}, err
		}
		return c.Dependencies, types.DepGraphNode{
			PayloadFile: c.Content,
			Author:      tag.Author,
			Timestamp:   tag.Timestamp,
		}, nil
	}
	return nil, types.DepGraphNode{}, nil
}

func readGraphCache(path string) (*GraphCache, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, nil // not present yet = cache miss, not an error.
	}
	h := fsutil.NewJSONHandlerFromPath(path, &GraphCache{})
	if err := h.Read(); err != nil {
		return nil, newErrDBInternal(err, "could not read dep graph cache")
	}
	return h.Data, nil
}

func pathDepGraph(repoPath, assetUID string) string {
	return fsutil.PathOS(
		repoPath,
		".vit", "cache", "depgraph",
		assetUID[:2], assetUID[2:]+".json",
	)
}
