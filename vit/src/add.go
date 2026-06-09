package vit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"vit/internal/opcontext"
	"vit/internal/types"
)

func (c *Client) Add(ctx context.Context, assetPath, src string, isDir bool) (*types.StringResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}

	c.logger.Info("AddAsset", "assetPath", assetPath, "src", src, "isDir", isDir)

	var pathbranch string
	_ = pathbranch

	err := opcontext.WithOperationContext(ctx, assetPath, func(
		opctx *opcontext.OperationContext,
		assetPath string,
	) error {
		srcInfo, err := os.Stat(src)
		if err != nil {
			return newSourceNotFoundErr(assetPath, src)
		}
		if !srcInfo.IsDir() {
			assetPath = addExtToAssetPath(assetPath, src)
			ref := types.NewRefBranch(opctx.RepoPath, assetPath, types.DefaultBranchName)
			_ = ref
		}

		return nil
	})

	return nil, err
}

// addExtToAssetPath modify assetPath based on src.
// by adding the ext of src if assetPath does not have it already.
func addExtToAssetPath(assetPath, src string) string {
	extSrc := filepath.Ext(src)
	extAssetPath := filepath.Ext(assetPath)
	if extSrc != extAssetPath {
		assetPath = assetPath + extSrc
	}
	return assetPath
}

func newSourceNotFoundErr(assetPath, src string) error {
	return types.NewVitError(
		types.ErrFileNotFound,
		[]string{fmt.Sprintf("can't create asset '%s': source not found: '%s'", assetPath, src)},
		[]any{"assetPath", assetPath, "src", src},
	)
}
