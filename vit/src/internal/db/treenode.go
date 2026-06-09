package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
)

// TreeNode represent either a directory or an asset in the virtual vit tree.
// Each one of them stored in a separate JSON file.
//
// If it represents an asset, it contains an uuid pointing to the AssetNode
// where all the asset data lives.
//
// TreeNode only store the name of the dir/asset, not the all path. And
// a ref to the parent UID. It does not refernce children.
// This is done so moving a TreeNode can be done by modyfing a single JSON.
type TreeNode struct {
	ID       string       `json:"id"`        // uuid
	Name     string       `json:"name"`      // name of the curr leaf, not full path
	ParentID string       `json:"parent_id"` // uuid of parent.
	Type     TreeNodeType `json:"type"`      // tree node describes both sub dir and assets

	// only filled if Type == TreeNodeTypeAsset
	AssetID string            `json:"asset_id,omitempty"`
	Attr    map[string]string `json:"attr,omitempty"`
}

type TreeNodeType string

const (
	TreeNodeTypeDir   TreeNodeType = "dir"
	TreeNodeTypeAsset TreeNodeType = "asset"
)

type JSONTreeNode = fsutil.JSONHandler[TreeNode]

func CreateNewJSONTreeNode(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	name, parentID string,
	treeNodeType TreeNodeType,
) (*JSONTreeNode, error) {
	uid := fsutil.GenerateUID(16)
	return fsutil.ResolveHandler(
		ctx,
		opctx.JSONPool,
		TreeNodeJSONPath(opctx.RepoPath, uid),
		true,
		&TreeNode{
			ID:       uid,
			Name:     name,
			ParentID: parentID,
			Type:     treeNodeType,
		},
	)
}

func (c *Client) ResolveJSONTreeNode(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, treePath string,
	forWrite bool,
) (*JSONTreeNode, bool, error) {
	treeIndex, err := c.GetTreeIndex(ctx, opctx, repoPath)
	if err != nil {
		return nil, false, err
	}

	id, ok := treeIndex.PathToID[treePath]
	if !ok {
		return nil, false, nil
	}

	path := TreeNodeJSONPath(repoPath, id)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, false, newTreeNodeNotFoundError(repoPath, treePath, path)
	}
	ret, err := fsutil.ResolveHandler[TreeNode](ctx, opctx.JSONPool, path, forWrite, nil)
	return ret, true, err
}

func TreeNodeJSONPath(repoPath, uid string) string {
	return filepath.Join(
		repoPath,
		".vit",
		"tree",
		uid[:2],
		uid[2:]+".json",
	)
}

func newTreeNodeNotFoundError(repoPath, treePath, expectedPath string) error {
	fullPath := filepath.Join(repoPath, treePath)
	return types.NewStandardError(
		types.ErrDBTreeNodeNotFound,
		[]string{fmt.Sprintf("tree node not found for: %s at %s", fullPath, expectedPath)},
		[]any{"repoPath", repoPath, "treeNode", treePath, "treeNodePath", expectedPath},
	)
}
