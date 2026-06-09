package db

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"vit/internal/opcontext"
	"vit/internal/types"
)

func (c *Client) GetJSONAssetFromRef(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	ref *types.Ref,
	forWrite bool,
) (*JSONAsset, error) {
	jsonTreeNode, found, err := c.resolveJSONTreeNode(ctx, opctx, ref.RepoPath, ref.AssetPath, false)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, newErrTreeNodeAssetNotFound(ref.RepoPath, ref.AssetPath)
	}

	treeNode := jsonTreeNode.Data
	if treeNode.Type == TreeNodeTypeDir {
		return nil, newErrTreeNodeIsDir(ref.RepoPath, ref.AssetPath)
	}

	assetNode, found, err := resolveJSONAsset(
		ctx, opctx,
		ref.RepoPath,
		ref.AssetPath,
		treeNode.AssetID,
		forWrite,
	)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, newErrTreeNodeAssetNotFound(ref.RepoPath, ref.AssetPath)
	}
	return assetNode, nil
}

func (c *Client) AddAssetToTree(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath string,
	treePath string,
) (*JSONAsset, error) {
	closestPath, err := c.isAssetPathAvailable(ctx, opctx, repoPath, treePath)
	if err != nil {
		return nil, err
	}

	// TODO(db): look for orphan node tree (and then authorize rewrite for them)
	treeNode, err := c.createPackageSubDir(ctx, opctx, repoPath, treePath, closestPath)
}

func (c *Client) isAssetPathAvailable(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath string,
	treePath string,
) (string, error) {
	treeIndex, err := c.GetTreeIndex(ctx, opctx, repoPath)
	if err != nil {
		return "", err
	}

	_, ok := treeIndex.PathToID[treePath]
	if ok {
		return "", newErrTreePathNotAvailable(repoPath, treePath)
	}

	closer := ""
	for p, e := range treeIndex.PathToID {
		if strings.HasPrefix(treePath, p+"/") { // TODO(windows): how to handle windows path here?
			if e.Type == TreeNodeTypeAsset {
				return "", newErrTreePathNotAvailableWithinAsset(repoPath, treePath, p)
			}
			if len(p) > len(closer) {
				closer = p
			}
		}
	}

	return closer, nil
}

func (c *Client) createPackageSubDir(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, treePath, fromPath string,
) (*JSONTreeNode, error) {
	treeIndex, err := c.GetTreeIndex(ctx, opctx, repoPath)
	if err != nil {
		return nil, err
	}

	subPath := strings.TrimPrefix(treePath, fromPath)
	subPath = strings.TrimPrefix(subPath, "/") // TODO(windows)

	subDirs := strings.Split(subPath, "/") // TODO(windows)

	currDir := fromPath
	var parentID string

	if fromPath == "" {
		parentID = ""
	} else {
		tmp, ok := treeIndex.PathToID[fromPath]
		if !ok {
			return nil, nil // TODO error
		}
		parentID = tmp.Data
	}

	var jsonHandlerBuffer []*JSONTreeNode

	for _, d := range subDirs {
		currDir = filepath.Join(currDir, d) // TODO(windows)
		jsonTreeNode, err := c.createNewJSONTreeNode(
			ctx, opctx, repoPath,
			currDir, parentID, TreeNodeTypeDir,
		)
		if err != nil {
			return nil, err
		}

		jsonHandlerBuffer = append(jsonHandlerBuffer, jsonTreeNode)

		parentID = jsonTreeNode.Data.ID
	}

	for _, h := range jsonHandlerBuffer {
		if err := opctx.JSONPool.WriteHandler(h); err != nil {
			return nil, err
		}
	}

	if len(jsonHandlerBuffer) == 0 {
		return nil, nil
	} else {
		return jsonHandlerBuffer[len(jsonHandlerBuffer)-1], nil
	}
}

func newErrTreeNodeIsDir(repoPath, treePath string) error {
	fullPath := filepath.Join(repoPath, treePath)
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{fmt.Sprintf("path %s point to a directory, not an asset.", fullPath)},
		[]any{"error", "pathIsDir", "repoPath", repoPath, "treeNode", treePath},
	)
}

func newErrTreeNodeAssetNotFound(repoPath, treePath string) error {
	fullPath := filepath.Join(repoPath, treePath)
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{fmt.Sprintf("asset not found at: %s", fullPath)},
		[]any{"error", "AssetNotFound", "repoPath", repoPath, "treeNode", treePath},
	)
}

// TODO(error): maybe unforce that first item of extra has to be "error", and second a string.
func newErrTreePathNotAvailable(repoPath, treePath string) error {
	fullPath := filepath.Join(repoPath, treePath)
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{fmt.Sprintf("tree path not available: %s", fullPath)},
		[]any{"error", "PathNotAvailable", "repoPath", repoPath, "treeNode", treePath},
	)
}

func newErrTreePathNotAvailableWithinAsset(repoPath, treePath, existingAssetPath string) error {
	fullPath1 := filepath.Join(repoPath, treePath)
	fullPath2 := filepath.Join(repoPath, existingAssetPath)
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{
			fmt.Sprintf("tree path not available: %s", fullPath1),
			fmt.Sprintf("an asset already exists at: %s", fullPath2),
			"Asset can't be nested within another asset",
		},
		[]any{
			"error", "PathNotAvailableWithinAsset",
			"repoPath", repoPath,
			"treeNode", treePath,
			"existingAsset", existingAssetPath,
		},
	)
}
