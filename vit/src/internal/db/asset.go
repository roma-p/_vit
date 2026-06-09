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

type JSONAsset = fsutil.JSONHandler[types.Asset]

func CreateNewJSONAsset(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	assetPath string,
) (*JSONAsset, error) {
	uid := fsutil.GenerateUID(16)
	return fsutil.ResolveHandler(
		ctx,
		opctx.JSONPool,
		pathJSONAsset(opctx.RepoPath, uid),
		true,
		&types.Asset{
			AssetUID:  uid,
			AssetPath: assetPath, // comodity, not exported written in the json, and not source of truth
		},
	)
}

func ResolveJSONAsset(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, assetPath, uid string, // asset path just used for formatting
	forWrite bool,
) (*JSONAsset, bool, error) {
	jsonPath := pathJSONAsset(repoPath, uid)
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return nil, false, newAssetObjectIndexNotFound(repoPath, assetPath, uid)
	}
	ret, err := fsutil.ResolveHandler[types.Asset](ctx, opctx.JSONPool, jsonPath, forWrite, nil)
	if err != nil {
		return nil, false, err
	}
	ret.Data.AssetPath = assetPath // commodity, not persisted in json, not source of truth
	return ret, true, nil
}

func pathJSONAsset(repoPath, assetUID string) string {
	return filepath.Join(repoPath, ".vit", "assets", assetUID[:2], assetUID[2:]+".json")
}

func newAssetObjectIndexNotFound(repoPath, assetPath, uid string) error {
	fullPath := filepath.Join(repoPath, assetPath)
	return types.NewInternalVitError(
		types.ErrDBInternal,
		nil,
		[]string{fmt.Sprintf("asset object index not found for %s at: %s", fullPath, uid)},
		[]any{"repoPath", repoPath, "assetPath", assetPath, "assetUID", uid},
	)
}
