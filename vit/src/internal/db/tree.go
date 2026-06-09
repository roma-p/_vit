package db

import (
	"context"
	"fmt"
	"path/filepath"

	"vit/internal/opcontext"
	"vit/internal/types"
)

func (c *Client) GetJSONAssetFromRef(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	ref *types.Ref,
	forWrite bool,
) (*JSONAsset, error) {
	jsonTreeNode, found, err := c.ResolveJSONTreeNode(ctx, opctx, ref.RepoPath, ref.AssetPath, false)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, newTreeNodeAssetNotFound(ref.RepoPath, ref.AssetPath)
	}

	treeNode := jsonTreeNode.Data
	if treeNode.Type == TreeNodeTypeDir {
		return nil, newTreeNodeIsDirError(ref.RepoPath, ref.AssetPath)
	}

	assetNode, found, err := ResolveJSONAsset(
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
		return nil, newTreeNodeAssetNotFound(ref.RepoPath, ref.AssetPath)
	}
	return assetNode, nil
}

func newTreeNodeIsDirError(repoPath, treePath string) error {
	fullPath := filepath.Join(repoPath, treePath)
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{fmt.Sprintf("path %s point to a directory, not an asset.", fullPath)},
		[]any{"repoPath", repoPath, "treeNode", treePath},
	)
}

func newTreeNodeAssetNotFound(repoPath, treePath string) error {
	fullPath := filepath.Join(repoPath, treePath)
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{fmt.Sprintf("asset not found at: %s", fullPath)},
		[]any{"repoPath", repoPath, "treeNode", treePath},
	)
}
