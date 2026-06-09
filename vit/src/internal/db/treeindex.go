package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"vit/internal/fsutil"
	"vit/internal/opcontext"
)

type TreeIndex struct {
	IDToPath map[string]TreeIndexEntry `json:"id_to_path"`
	PathToID map[string]TreeIndexEntry `json:"path_to_id"`
}

func (t *TreeIndex) AddPath(path, uid string, treeNodeType TreeNodeType) {
	t.IDToPath[uid] = TreeIndexEntry{Data: path, Type: treeNodeType}
	t.PathToID[path] = TreeIndexEntry{Data: uid, Type: treeNodeType}
}

type TreeIndexEntry struct {
	Data string       `json:"data"`
	Type TreeNodeType `json:"type"`
}

func NewTreeIndex() *TreeIndex {
	return &TreeIndex{
		IDToPath: make(map[string]TreeIndexEntry),
		PathToID: make(map[string]TreeIndexEntry),
	}
}

type TreeCache struct {
	AssetPaths []string `json:"asset_paths"` // does not include subdir
}

func (t *TreeCache) AddPath(path string) {
	t.AssetPaths = append(t.AssetPaths, path)
}

func getTreeIndex(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath string,
) (*TreeIndex, error) {
	doRebuild, err := istreeCacheToRebuild(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	if doRebuild {
		treeIndex, _, err := buildTreeIndexAndCache(ctx, opctx, repoPath)
		if err != nil {
			return nil, err
		} else {
			return treeIndex, nil
		}
	} else {
		handler, err := fsutil.NewJSONHandlerFromFile[TreeIndex](pathTreeIndex(repoPath))
		if err != nil {
			return nil, fmt.Errorf("failed to read tree index: %w", err)
		}
		return handler.Data, nil
	}
}

func getTreeCache(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath string,
) (*TreeCache, error) {
	doRebuild, err := istreeCacheToRebuild(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	if doRebuild {
		_, treeCache, err := buildTreeIndexAndCache(ctx, opctx, repoPath)
		if err != nil {
			return nil, err
		} else {
			return treeCache, nil
		}
	} else {
		handler, err := fsutil.NewJSONHandlerFromFile[TreeCache](pathAssetList(repoPath))
		if err != nil {
			return nil, fmt.Errorf("failed to read tree index: %w", err)
		}
		return handler.Data, nil
	}
}

// MarkTreeStale marks the tree cache as stale.
// Call this after any write operation that modifies .vit/tree/.
// TODO(clean): probably not exported.
func MarkTreeStale(repoPath string) error {
	path := pathTreeStale(repoPath)
	return os.WriteFile(path, []byte{}, 0o644)
}

func istreeCacheToRebuild(
	ctx context.Context,
	repoPath string,
) (bool, error) {
	// 1. check no rebuild in progress: if so, wait for rebuild to finish
	if err := fsutil.WaitForLockRelease(ctx, pathTreeRebuild(repoPath)); err != nil {
		return false, err
	}

	// 2. otherwise: check for cache staleness OR missing tree index / cache
	return istreeCacheStale(repoPath)
}

func istreeCacheStale(repoPath string) (bool, error) {
	stalePath := pathTreeStale(repoPath)
	indexPath := pathTreeIndex(repoPath)
	assetListPath := pathAssetList(repoPath)

	_, staleErr := os.Stat(stalePath)
	_, indexErr := os.Stat(indexPath)
	_, assetListErr := os.Stat(assetListPath)

	if staleErr == nil {
		return true, nil
	}

	if os.IsNotExist(indexErr) || os.IsNotExist(assetListErr) {
		return true, nil
	}

	return false, nil
}

// buildTreeIndexAndCache rebuild TreeIndex and treeCache and write them on disk
// as well as returning an in-memory version of them.
// For now the rebuild mechanism is very naive: it scan the entire database.
// TODO(cache): incremental rebuild of cache?
func buildTreeIndexAndCache(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath string,
) (*TreeIndex, *TreeCache, error) {
	stopFunc, err := fsutil.AcquireExclusiveLockWithKeepalive(ctx, pathTreeRebuild(repoPath))
	if err != nil {
		return nil, nil, err
	}
	defer stopFunc()

	// Double-check after acquiring lock: another process may have rebuilt while we waited.
	doRebuild, err := istreeCacheStale(repoPath)
	if err != nil {
		return nil, nil, err
	}
	if !doRebuild {
		idx, err := fsutil.NewJSONHandlerFromFile[TreeIndex](pathTreeIndex(repoPath))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read tree index after rebuild: %w", err)
		}
		cache, err := fsutil.NewJSONHandlerFromFile[TreeCache](pathAssetList(repoPath))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read tree cache after rebuild: %w", err)
		}
		return idx.Data, cache.Data, nil
	}

	treeIndex := NewTreeIndex()
	treeCache := &TreeCache{}

	// 0. listing all existing json files in pool
	preexistingSet := make(map[string]bool)
	for _, p := range opctx.JSONPool.ListJSONPath() {
		preexistingSet[p] = true
	}

	// 1. scanning all tree node files.
	treeNodePaths, err := scanAllTreeNodeFiles(repoPath)
	if err != nil {
		return nil, nil, err
	}

	// 2. Reading all of them and storing them in a map.
	idToTreeNode := make(IDToTreeNode)
	for _, path := range treeNodePaths {
		treenode, err := fsutil.ResolveHandler[TreeNode](ctx, opctx.JSONPool, path, false, nil)
		if err != nil {
			return nil, nil, err
		}
		idToTreeNode[treenode.Data.ID] = *treenode.Data
		if !preexistingSet[path] {
			opctx.JSONPool.Release(path)
		}
	}

	// 3. Building TreeIndex from idToTreeNode (path resolution)
	for id, treenode := range idToTreeNode {
		if _, ok := treeIndex.IDToPath[id]; ok {
			continue
		}
		path, err := reccBuildIDToPath(&idToTreeNode, treeIndex, treenode.Name, treenode.ParentID)
		if err != nil {
			// TODO(error): return err or log?
			continue
		}

		treeIndex.IDToPath[id] = TreeIndexEntry{Data: path, Type: treenode.Type}
		treeIndex.PathToID[path] = TreeIndexEntry{Data: id, Type: treenode.Type}
	}

	// 4. Building treeCache (asset list) from resolved paths
	for id, treenode := range idToTreeNode {
		if treenode.Type == TreeNodeTypeAsset {
			if entry, ok := treeIndex.IDToPath[id]; ok {
				treeCache.AssetPaths = append(treeCache.AssetPaths, entry.Data)
			}
		}
	}

	// 5. Write to disk.
	indexPath := pathTreeIndex(repoPath)
	if err := fsutil.NewJSONHandlerFromPath(indexPath, treeIndex).Write(); err != nil {
		return nil, nil, fmt.Errorf("failed to write tree index: %w", err)
	}

	cachePath := pathAssetList(repoPath)
	if err := fsutil.NewJSONHandlerFromPath(cachePath, treeCache).Write(); err != nil {
		return nil, nil, fmt.Errorf("failed to write tree cache: %w", err)
	}

	os.Remove(pathTreeStale(repoPath))
	return treeIndex, treeCache, nil
}

func reccBuildIDToPath(
	idToTreeNode *IDToTreeNode,
	treeIndex *TreeIndex,
	path, uid string,
) (string, error) {
	// no more parent node, returning the path that is now complete.
	if uid == "" {
		return path, nil
	}

	// checking if by any chance, the parent node has already been handled
	// to avoid too much repetitive recursion.
	if entry, ok := treeIndex.IDToPath[uid]; ok {
		return filepath.Join(entry.Data, path), nil
	}

	treeNode, ok := (*idToTreeNode)[uid]
	if !ok {
		return "", fmt.Errorf("no parent path found for %s", path)
	}

	parentPath, err := reccBuildIDToPath(idToTreeNode, treeIndex, treeNode.Name, treeNode.ParentID)
	if err != nil {
		return "", err
	}

	treeIndex.IDToPath[uid] = TreeIndexEntry{Data: parentPath, Type: treeNode.Type}
	treeIndex.PathToID[parentPath] = TreeIndexEntry{Data: uid, Type: treeNode.Type}
	return filepath.Join(parentPath, path), nil
}

type IDToTreeNode = map[string]TreeNode

func scanAllTreeNodeFiles(repoPath string) ([]string, error) {
	treeDir := filepath.Join(repoPath, ".vit", "tree")

	if _, err := os.Stat(treeDir); os.IsNotExist(err) {
		return nil, nil
	}

	var paths []string

	err := filepath.WalkDir(treeDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan tree directory: %w", err)
	}

	return paths, nil
}

func pathTreeIndex(repoPath string) string {
	return filepath.Join(repoPath, ".vit", "cache", "tree_index.json")
}

func pathAssetList(repoPath string) string {
	return filepath.Join(repoPath, ".vit", "cache", "asset_list.json")
}

func pathTreeStale(repoPath string) string {
	return filepath.Join(repoPath, ".vit", "cache", ".tree_stale")
}

func pathTreeRebuild(repoPath string) string {
	return filepath.Join(repoPath, ".vit", "cache", ".tree_rebuild")
}
