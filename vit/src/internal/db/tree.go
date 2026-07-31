package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"vit/internal/fsutil"
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
		ref.RepoRef(),
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

type AddAssetToTreeRequest struct {
	RepoRef           *types.RepoRef
	TreePath          string
	BlobBranch        string
	ContentRootHash   string
	BaselineTimeStamp time.Time
}

func (c *Client) AddAssetToTree(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	request AddAssetToTreeRequest,
) (*JSONAsset, error) {
	treePath := fsutil.PathInternal(request.TreePath)
	closestPath, err := c.IsAssetPathAvailable(ctx, opctx, request.RepoRef.Path, request.TreePath)
	if err != nil {
		return nil, err
	}

	// TODO(db): look for orphan node tree (and then authorize rewrite for them)
	asset, err := createNewJSONAsset(ctx, opctx, request.RepoRef, request.TreePath)
	if err != nil {
		return nil, err
	}

	if err := initAsset(
		opctx,
		asset.Data,
		request.BlobBranch,
		request.ContentRootHash,
		request.BaselineTimeStamp,
	); err != nil {
		return nil, err
	}

	if _, err := c.createAssetTreeNode(
		ctx, opctx,
		request.RepoRef,
		treePath,
		closestPath,
		asset.Data.AssetUID); err != nil {
		return nil, err
	}

	return asset, nil
}

func (c *Client) IsAssetPathAvailable(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath string,
	treePath string,
) (string, error) {
	treePath = fsutil.PathInternal(treePath)
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
		if strings.HasPrefix(treePath, p+"/") {
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

// -- FILE LOCAL -------------------------------------------------------------

func (c *Client) createAssetTreeNode(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoRef *types.RepoRef,
	treePath, fromPath, assetID string,
) (*JSONTreeNode, error) {
	treeIndex, err := c.GetTreeIndex(ctx, opctx, repoRef.Path)
	if err != nil {
		return nil, err
	}

	subPath := strings.TrimPrefix(treePath, fromPath)
	subPath = strings.TrimPrefix(subPath, "/")

	subDirs := strings.Split(subPath, "/")

	if len(subDirs) == 0 {
		return nil, nil
	}

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

	for _, d := range subDirs[:len(subDirs)-1] {
		currDir = fsutil.PathInternal(currDir, d)
		jsonTreeNode, err := c.createNewJSONTreeNode(
			ctx, opctx,
			repoRef.Path,
			currDir,
			parentID,
			TreeNodeTypeDir,
		)
		if err != nil {
			return nil, err
		}

		parentID = jsonTreeNode.Data.ID
	}

	ret, err := c.createNewJSONTreeNode(
		ctx, opctx,
		repoRef.Path,
		treePath,
		parentID,
		TreeNodeTypeAsset,
	)
	if err != nil {
		return nil, err
	}
	ret.Data.AssetID = assetID

	c.MarkTreeStale(repoRef.Path)

	return ret, nil
}

func newErrTreeNodeIsDir(repoPath, treePath string) error {
	fullPath := fsutil.PathOS(repoPath, treePath)
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{fmt.Sprintf("path %s point to a directory, not an asset.", fullPath)},
		[]any{"error", "pathIsDir", "repoPath", repoPath, "treeNode", treePath},
	)
}

func newErrTreeNodeAssetNotFound(repoPath, treePath string) error {
	fullPath := fsutil.PathOS(repoPath, treePath)
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{fmt.Sprintf("asset not found at: %s", fullPath)},
		[]any{"error", "AssetNotFound", "repoPath", repoPath, "treeNode", treePath},
	)
}

// TODO(error): maybe unforce that first item of extra has to be "error", and second a string.
func newErrTreePathNotAvailable(repoPath, treePath string) error {
	fullPath := fsutil.PathOS(repoPath, treePath)
	return types.NewVitError(
		types.ErrDBTreeNode,
		[]string{fmt.Sprintf("tree path not available: %s", fullPath)},
		[]any{"error", "PathNotAvailable", "repoPath", repoPath, "treeNode", treePath},
	)
}

func newErrTreePathNotAvailableWithinAsset(repoPath, treePath, existingAssetPath string) error {
	fullPath1 := fsutil.PathOS(repoPath, treePath)
	fullPath2 := fsutil.PathOS(repoPath, existingAssetPath)
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
