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
	IDToPath map[string]string `json:"id_to_path"`
	PathToID map[string]string `json:"path_to_id"`
}

func NewTreeIndex() *TreeIndex {
	return &TreeIndex{
		IDToPath: make(map[string]string),
		PathToID: make(map[string]string),
	}
}

type TreeCache struct {
	AssetPaths []string `json:"asset_paths"` // does not include subdir
}

// GetTreeIndex rebuilding if stale or missing.
func GetTreeIndex(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath string,
) (*TreeIndex, error) {
	doRebuild, err := isTreeCacheToRebuild(ctx, repoPath)
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

// MarkTreeStale marks the tree cache as stale.
// Call this after any write operation that modifies .vit/tree/.
func MarkTreeStale(repoPath string) error {
	path := pathTreeStale(repoPath)
	return os.WriteFile(path, []byte{}, 0o644)
}

func isTreeCacheToRebuild(
	ctx context.Context,
	repoPath string,
) (bool, error) {
	// 1. check no rebuild in progress: if so, wait for rebuild to finish
	if err := fsutil.WaitForLockRelease(ctx, pathTreeRebuild(repoPath)); err != nil {
		return false, err
	}

	// 2. otherwise: check for cache staleness OR missing tree index / cache

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

// buildTreeIndexAndCache rebuild TreeIndex and TreeCache and write them on disk
// as well as returning an in-memory version of them.
// For now the rebuild mechanism is very naive: it scan the entire database.
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
		treeIndex.IDToPath[id] = path
		treeIndex.PathToID[path] = id
	}

	// 4. Building TreeCache (asset list) from resolved paths
	for id, treenode := range idToTreeNode {
		if treenode.Type == TreeNodeTypeAsset {
			if path, ok := treeIndex.IDToPath[id]; ok {
				treeCache.AssetPaths = append(treeCache.AssetPaths, path)
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

	// checking if by any chance, the parent node has not beeing already handled
	// to avoid too much repetitive recc.
	parentPath, ok := treeIndex.IDToPath[uid]
	if ok {
		return filepath.Join(parentPath, path), nil
	}

	treeNode, ok := (*idToTreeNode)[uid]
	if !ok {
		return "", fmt.Errorf("no parent path found for %s", path)
	}

	parentPath, err := reccBuildIDToPath(idToTreeNode, treeIndex, treeNode.Name, treeNode.ParentID)
	if err != nil {
		return "", err
	}
	treeIndex.IDToPath[uid] = parentPath
	treeIndex.PathToID[parentPath] = uid
	return filepath.Join(parentPath, path), nil
}

type IDToTreeNode = map[string]TreeNode

// scanAllTreeNodeFiles returns the paths of every JSON file under .vit/tree/.
// Files are stored as .vit/tree/<id[:2]>/<id[2:]>.json.
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
