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
			AssetUID: uid,
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
	return ret, true, err
}

func pathJSONAsset(repoPath, assetUID string) string {
	return filepath.Join(
		repoPath,
		".vit",
		"assets",
		assetUID[:2],
		assetUID[2:]+".json",
	)
}

// TODO(error): internal + standard shall be the same struct with an extra flag. internal shall have extra
func newAssetObjectIndexNotFound(repoPath, assetPath, uid string) error {
	fullPath := filepath.Join(repoPath, assetPath)
	return types.NewStandardError( // TODO(error) make this internal !!
		types.ErrDBAssetObjectIndexNotFound,
		[]string{fmt.Sprintf("asset object index not found for %s at: %s", fullPath, uid)},
		// TODO: func should accept assetPath for logging...
		[]any{"repoPath", repoPath, "assetPath", assetPath, "assetUID", uid},
	)
}
