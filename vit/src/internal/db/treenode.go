package db

import (
	"context"
	"fmt"
	"os"
	"strings"

	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
)

type JSONTreeNode = fsutil.JSONHandler[types.TreeNode]

func (c *Client) GetTreeNode(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, treePath string,
) (*types.TreeNode, error) {
	node, err := c.getTreeNode(ctx, opctx, repoPath, treePath, false)
	if err != nil {
		return nil, err
	}
	return node.Data, nil
}

func (c *Client) GetTreeNodeForWrite(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, treePath string,
) (*JSONTreeNode, error) {
	return c.getTreeNode(ctx, opctx, repoPath, treePath, true)
}

func (c *Client) ResolveAssetPath(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, path string,
) (string, error) {
	idx, err := c.GetTreeIndex(ctx, opctx, repoPath)
	if err != nil {
		return "", err
	}
	p := fsutil.PathInternal(path)
	if e, ok := idx.PathToID[p]; ok && e.Type == types.TreeNodeTypeAsset {
		return p, nil
	}
	return "", newErrNotAnAssetPath(idx, repoPath, p)
}

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

	if _, err := os.Stat(path); fsutil.IsNotExist(err) {
		return nil, false, newTreeNodeNotFoundError(repoPath, treeNodePath, path)
	}
	ret, err := fsutil.ResolveHandler[types.TreeNode](ctx, opctx.JSONPool, path, forWrite, nil)
	return ret, true, err
}

// -- FILE LOCAL -------------------------------------------------------------

func (c *Client) getTreeNode(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, treePath string,
	forWrite bool,
) (*JSONTreeNode, error) {
	node, found, err := c.resolveJSONTreeNode(ctx, opctx, repoPath, treePath, forWrite)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, newErrNoTreeNode(repoPath, treePath)
	}
	return node, nil
}

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

func newErrNoTreeNode(repoPath, treePath string) error {
	fullPath := fsutil.PathOS(repoPath, treePath)
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{fmt.Sprintf("no asset or directory at: %s", fullPath)},
		[]any{"error", "NoTreeNode", "repoPath", repoPath, "treeNode", treePath},
	)
}

func newErrNotAnAssetPath(idx *TreeIndex, repoPath, p string) error {
	fullPath := fsutil.PathOS(repoPath, p)
	msg := fmt.Sprintf("no asset at: %s", fullPath)
	if e, ok := idx.PathToID[p]; ok && e.Type == types.TreeNodeTypeDir {
		msg = fmt.Sprintf("%s is a directory, not an asset", fullPath)
	} else {
		cur, _, _ := strings.Cut(p, "@")
		for ; cur != "" && cur != "."; cur = fsutil.DirInternal(cur) {
			if e, ok := idx.PathToID[cur]; ok && e.Type == types.TreeNodeTypeAsset {
				msg = fmt.Sprintf("%s is not an asset path, give the asset itself: %s", fullPath, fsutil.PathOS(repoPath, cur))
				break
			}
		}
	}
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{msg},
		[]any{"error", "NotAnAssetPath", "repoPath", repoPath, "path", p},
	)
}
