package db

import (
	"context"

	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
)

type GraphCache struct {
	Graphs map[string]*types.DepGraph `json:"graphs"` // key = ref.ObjectPath(), one per ACTIVE ref
	// TODO: no ! use UID for depgraph to only store them once.
	SourcesSnapshot map[string]string `json:"sources_snapshot"` // direct-source UID -> JSON hash, for incremental rebuild
}

type GraphCachePool map[string]*types.DepGraph

func getDepGraph(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, assetUID string,
) (*types.DepGraph, error) {
	// 1. parse from disk
	// 2. if non: create a basic one an set stale=True
	// 3. if stale: list all non referfeced product in source.
	return nil, nil
}

func newGraphCache() *GraphCache {
	return &GraphCache{
		Graphs:          make(map[string]*types.DepGraph),
		SourcesSnapshot: make(map[string]string),
	}
}

func buildDepGraphFromScratch(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	asset *types.Asset,
) (*GraphCache, error) {
	// for
	return nil, nil
}

func pathDepGraph(repoPath, assetUID string) string {
	return fsutil.PathOS(
		repoPath,
		".vit", "cache", "depgraph",
		assetUID[:2], assetUID[2:]+".json",
	)
}
