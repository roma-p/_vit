package db

import (
	"context"
	"fmt"
	"os"

	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
)

type JSONTreeNode = fsutil.JSONHandler[types.TreeNode]

func (c *Client) createNewJSONTreeNode(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, treePath, parentID string,
	treeNodeType types.TreeNodeType,
) (*JSONTreeNode, error) {
	name := fsutil.BaseInternal(treePath)

	treeIndex, err := c.GetTreeIndex(ctx, opctx, repoPath)
	if err != nil {
		return nil, err
	}
	uid := fsutil.GenerateUID(16)
	ret, err := fsutil.ResolveHandler(
		ctx,
		opctx.JSONPool,
		treeNodeJSONPath(repoPath, uid),
		true,
		&types.TreeNode{
			ID:       uid,
			Name:     name,
			ParentID: parentID,
			Type:     treeNodeType,
		},
	)
	if err != nil {
		return nil, err
	}

	treeIndex.AddPath(treePath, uid, treeNodeType)

	return ret, nil
}

func (c *Client) resolveJSONTreeNode(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, treeNodePath string,
	forWrite bool,
) (*JSONTreeNode, bool, error) {
	treeIndex, err := c.GetTreeIndex(ctx, opctx, repoPath)
	if err != nil {
		return nil, false, err
	}

	entry, ok := treeIndex.PathToID[treeNodePath]
	if !ok {
		return nil, false, nil
	}

	path := treeNodeJSONPath(repoPath, entry.Data)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, false, newTreeNodeNotFoundError(repoPath, treeNodePath, path)
	}
	ret, err := fsutil.ResolveHandler[types.TreeNode](ctx, opctx.JSONPool, path, forWrite, nil)
	return ret, true, err
}

// -- FILE LOCAL -------------------------------------------------------------

func treeNodeJSONPath(repoPath, uid string) string {
	return fsutil.PathOS(repoPath, ".vit", "tree", uid[:2], uid[2:]+".json")
}

func newTreeNodeNotFoundError(repoPath, treePath, expectedPath string) error {
	fullPath := fsutil.PathOS(repoPath, treePath)
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{fmt.Sprintf("tree node not found for: %s at %s", fullPath, expectedPath)},
		[]any{"repoPath", repoPath, "treeNode", treePath, "treeNodePath", expectedPath},
	)
}
