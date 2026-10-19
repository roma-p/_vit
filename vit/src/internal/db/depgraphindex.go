package db

import (
	"context"
	"maps"
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
// the branch head's, and records the commit's producer edges (produced-by on the
// products it captured). Source reverse edges are the live-branch index, owned by
// `source add`, not recorded per-commit.
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
	return recordProducerEdges(ctx, opctx, dbClient, commitRef)
}

// UpdateDepGraphForTag materializes the new tag's forward closure and records
// the producer edges that make the tagged products' produced-by resolve to this
// tag (the tag carries its own product deps).
func UpdateDepGraphForTag(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	tagRef *types.Ref,
) error {
	if _, err := CacheDependencies(ctx, opctx, dbClient, tagRef); err != nil {
		return err
	}
	return recordProducerEdges(ctx, opctx, dbClient, tagRef)
}

// UpdateDepGraphForDependency materializes the DEPENDENT object's forward closure
// and records the reverse edge on the target (+ marks the target's dependents
// stale). It returns the dependent's closure so the caller can present it. This
// is the single incremental edge-creation path for BOTH directions:
//   - `source add`  => Type=source,  Dependent = the consumer
//   - `product add` => Type=product, Dependent = the origin
func UpdateDepGraphForDependency(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	r UpdateDepGraphForDependencyRequest,
) (*types.DepGraph, error) {
	g, err := CacheDependencies(ctx, opctx, dbClient, r.Dependent)
	if err != nil {
		return nil, err
	}
	targetAsset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, r.Target, false)
	if err != nil {
		return g, err
	}
	if err := AddReverseEdge(
		r.Target.RepoPath, targetAsset.Data.AssetUID, r.Target.ObjectPath(),
		ReverseEntry{
			Type:    r.Type,
			Peer:    r.Dependent,
			Name:    r.Name,
			RefSpec: r.Spec,
			Commit:  r.Commit,
		},
	); err != nil {
		return g, err
	}
	if err := MarkDependentsStale(ctx, opctx, dbClient, r.Target); err != nil {
		return g, err
	}
	return g, nil
}

type UpdateDepGraphForDependencyRequest struct {
	Dependent *types.Ref // the consumer (source) or the origin (product)
	Target    *types.Ref
	Spec      *types.Ref
	Name      string
	Type      types.AssetDependencyType
	Commit    string // dependent's underlying commit id (source); "" for product
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

func buildDependencies(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	root *types.Ref,
) (*types.DepGraph, error) {
	g := types.NewDepGraph()

	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, root, false)
	if err != nil {
		return nil, err
	}
	deps, node, err := dependenciesForRef(asset.Data, root)
	if err != nil {
		return nil, err
	}
	g.AddNode(root, node)

	for name, dep := range deps {
		switch dep.Type {
		case types.AssetDependencySource:
			child := dep.RefResolved
			if err := g.Connect(root, child, dep.RefSpec, name, dep.Type); err != nil {
				return nil, err
			}
			childG, err := GetDependencies(ctx, opctx, dbClient, child)
			if err != nil {
				return nil, err
			}
			g.Merge(childG)
		case types.AssetDependencyProduct:
			provider := productProvider(root, dep)
			if provider != nil {
				if err := addProductTargetNode(ctx, opctx, dbClient, g, provider); err != nil {
					return nil, err
				}
			}
			if err := g.Connect(root, provider, dep.RefSpec, name, dep.Type); err != nil {
				return nil, err
			}
		}
	}
	return g, nil
}

func productProvider(root *types.Ref, dep *types.AssetDependency) *types.Ref {
	// if refResolved is nil: no product was produced!!
	if dep.RefResolved != nil {
		return dep.RefResolved
	}
	if root.RefType == types.RefTypeBranch {
		return dep.RefSpec
	}
	return nil
}

func addProductTargetNode(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	g *types.DepGraph,
	target *types.Ref,
) error {
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, target, false)
	if err != nil {
		return err
	}
	_, node, err := dependenciesForRef(asset.Data, target)
	if err != nil {
		return err
	}
	g.AddNode(target, node)
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
		deps := c.Dependencies
		if len(tag.Dependencies) > 0 {
			merged := make(types.AssetDependencies, len(deps)+len(tag.Dependencies))
			maps.Copy(merged, deps)
			maps.Copy(merged, tag.Dependencies) // product overrides win
			deps = merged
		}
		return deps, types.DepGraphNode{
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
