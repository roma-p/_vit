package db

import (
	"context"

	"vit/internal/opcontext"
	"vit/internal/types"
)

// GetLineage returns the provenance of ref's object, computed on demand (never
// cached): for the object and every object it reaches through sources, who
// PRODUCED it, then transitively what those producers consumed and who produced
// that. Edges have the same shape as forward ones (producer —product→ object,
// consumer —source→ provider).
//
// Producers expand only their sources and their own producers, never their other
// product outputs: those are siblings, not ancestry. The graph is read from the
// produced-by index at query time, so it is always current.
func GetLineage(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *Client,
	ref *types.Ref,
) (*types.DepGraph, error) {
	fwd, err := GetDependencies(ctx, opctx, dbClient, ref)
	if err != nil {
		return nil, err
	}
	w := &lineageWalker{ctx: ctx, opctx: opctx, dbClient: dbClient,
		g: types.NewDepGraph(), done: map[string]bool{}}
	// the source closure is already in the forward graph: only its producers are
	// lineage. Marked done so an ancestry walk reaching it does not re-expand it.
	closure := fwd.SourceClosure(ref)
	for _, n := range closure {
		w.done[usedByKey(n)] = true
	}
	for _, n := range closure {
		if err := w.addProducers(w.withRepoPath(n)); err != nil {
			return nil, err
		}
	}
	return w.g, nil
}

// -- FILE LOCAL -------------------------------------------------------------

type lineageWalker struct {
	ctx      context.Context
	opctx    *opcontext.OperationContext
	dbClient *Client
	g        *types.DepGraph
	done     map[string]bool // objects already expanded (also guards cycles)
}

// addProducers links obj to each of its producers and walks them as ancestry.
func (w *lineageWalker) addProducers(obj *types.Ref) error {
	entries, err := producerEntries(w.ctx, w.opctx, w.dbClient, obj)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	if err := w.addNode(obj); err != nil {
		return err
	}
	for _, e := range entries {
		if e.Peer == nil {
			continue
		}
		w.g.Edges = append(w.g.Edges, types.DependencyGraphEdge{
			Consumer: e.Peer,
			Provider: obj,
			RefSpec:  e.RefSpec,
			Name:     e.Name,
			Type:     types.AssetDependencyProduct,
		})
		if err := w.walkAncestor(e.Peer); err != nil {
			return err
		}
	}
	return nil
}

// walkAncestor expands a producer (or one of its sources): its sources, then its
// own producers.
func (w *lineageWalker) walkAncestor(obj *types.Ref) error {
	if w.done[usedByKey(obj)] {
		return nil
	}
	w.done[usedByKey(obj)] = true

	if err := w.addNode(obj); err != nil {
		return err
	}
	asset, err := w.dbClient.GetJSONAssetFromRef(w.ctx, w.opctx, obj, false)
	if err != nil {
		return err
	}
	deps, _, err := dependenciesForRef(asset.Data, obj)
	if err != nil {
		return err
	}
	for name, dep := range deps {
		if dep.Type != types.AssetDependencySource || dep.RefResolved == nil {
			continue
		}
		child := w.withRepoPath(dep.RefResolved)
		w.g.Edges = append(w.g.Edges, types.DependencyGraphEdge{
			Consumer: obj,
			Provider: child,
			RefSpec:  dep.RefSpec,
			Name:     name,
			Type:     types.AssetDependencySource,
		})
		if err := w.walkAncestor(child); err != nil {
			return err
		}
	}
	return w.addProducers(obj)
}

func (w *lineageWalker) addNode(obj *types.Ref) error {
	asset, err := w.dbClient.GetJSONAssetFromRef(w.ctx, w.opctx, obj, false)
	if err != nil {
		return err
	}
	_, node, err := dependenciesForRef(asset.Data, obj)
	if err != nil {
		return err
	}
	w.g.AddNode(obj, node)
	return nil
}

// withRepoPath returns ref with its RepoPath (json:"-") rehydrated from its UID.
func (w *lineageWalker) withRepoPath(ref *types.Ref) *types.Ref {
	if ref.RepoPath != "" {
		return ref
	}
	r := *ref
	r.RepoPath = w.opctx.RepoRefFor(r.RepoUID).Path
	return &r
}
