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

func createNewJSONAsset(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, assetPath string,
) (*JSONAsset, error) {
	uid := fsutil.GenerateUID(16)
	return fsutil.ResolveHandler(
		ctx,
		opctx.JSONPool,
		pathJSONAsset(opctx.RepoPath, uid),
		true,
		&types.Asset{
			AssetUID: uid,
			// comodity, not exported written in the json, and not source of truth
			AssetPath: assetPath,
			RepoPath:  repoPath,
		},
	)
}

func resolveJSONAsset(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, assetPath, uid string, // asset path just used for formatting
	forWrite bool,
) (*JSONAsset, bool, error) {
	jsonPath := pathJSONAsset(repoPath, uid)
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return nil, false, newErrObjectIndexNotFound(repoPath, assetPath, uid)
	}
	ret, err := fsutil.ResolveHandler[types.Asset](ctx, opctx.JSONPool, jsonPath, forWrite, nil)
	if err != nil {
		return nil, false, err
	}
	// commodity, not persisted in json, not source of truth
	ret.Data.AssetPath = assetPath
	ret.Data.RepoPath = repoPath
	return ret, true, nil
}

func initAsset(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	asset *types.Asset,
	branchName string,
	blobFirstCommit, blobBranch, blobHash string,
	blobSize int,
) error {
	commitFirst := types.AssetCommit{
		PayloadSize: int64(blobSize),
		PayloadFile: blobFirstCommit,
		PayloadHash: blobHash,
		Author:      opctx.User,
		Message:     "Asset creation",
		Timestamp:   opctx.TimeStamp,
		Parent:      "",
	}

	commitID, err := AddCommit(asset, &commitFirst)
	if err != nil {
		return err
	}

	err = AddBranch(asset, commitID, branchName, opctx.User, blobBranch)
	if err != nil {
		return err
	}

	return nil
}

// AddCommit adds a commit to the asset and returns the generated commit ID.
func AddCommit(a *types.Asset, commit *types.AssetCommit) (string, error) {
	commitID := fsutil.GenerateUID(8)
	if a.Commits == nil {
		a.Commits = make(map[string]*types.AssetCommit)
	}
	a.Commits[commitID] = commit
	return commitID, nil
}

// GetCommit retrieves a commit by ID.
func GetCommit(a *types.Asset, commitID string) (*types.AssetCommit, error) {
	if a.Commits == nil {
		return nil, newErrObjectNotFound(types.NewRefCommit(a.RepoPath, a.AssetPath, commitID))
	}

	commitObj, ok := a.Commits[commitID]
	if !ok {
		return nil, newErrObjectNotFound(types.NewRefCommit(a.RepoPath, a.AssetPath, commitID))
	}

	return commitObj, nil
}

// AddBranch creates a new branch pointing to the given commit.
func AddBranch(a *types.Asset, commitID, branchName, author, payloadfile string) error {
	if a.Branches == nil {
		a.Branches = make(map[string]*types.AssetCommit)
	}

	if _, ok := a.Branches[branchName]; ok {
		return newErrObjectAlreadyExists(types.NewRefBranch(a.RepoPath, a.AssetPath, branchName))
	}

	commitFrom, err := GetCommit(a, commitID)
	if err != nil {
		return err
	}

	commitBranch := types.AssetCommit{
		PayloadSize:  commitFrom.PayloadSize,
		PayloadFile:  payloadfile,
		PayloadHash:  commitFrom.PayloadHash,
		Author:       author,
		Message:      "",
		Parent:       commitID,
		Dependencies: *deepCopyDependencies(&commitFrom.Dependencies),
	}

	a.Branches[branchName] = &commitBranch
	return nil
}

func deepCopyDependencies(src *types.AssetDependencies) *types.AssetDependencies {
	var ret types.AssetDependencies
	if len(*src) > 0 {
		ret = make(types.AssetDependencies, len(*src))
		for k, v := range *src {
			cp := *v
			if v.Ref != nil {
				refCp := *v.Ref
				cp.Ref = &refCp
			}
			ret[k] = &cp
		}
	}
	return &ret
}

func pathJSONAsset(repoPath, assetUID string) string {
	return filepath.Join(repoPath, ".vit", "assets", assetUID[:2], assetUID[2:]+".json")
}

func newErrObjectIndexNotFound(repoPath, assetPath, uid string) error {
	fullPath := filepath.Join(repoPath, assetPath)
	return types.NewInternalVitError(
		types.ErrDBInternal,
		nil,
		[]string{fmt.Sprintf("asset object index not found for %s at: %s", fullPath, uid)},
		[]any{"repoPath", repoPath, "assetPath", assetPath, "assetUID", uid},
	)
}

func newErrObjectNotFound(ref *types.Ref) error {
	return types.NewVitError(
		types.ErrAssetObjectNotFound,
		[]string{fmt.Sprintf("object not found on asset %s", ref.AbsolutePath())},
		[]any{"ref", ref},
	)
}

func newErrObjectAlreadyExists(ref *types.Ref) error {
	return types.NewVitError(
		types.ErrAssetObjectAlreadyExists,
		[]string{fmt.Sprintf("object already exists on asset %s", ref.AbsolutePath())},
		[]any{"ref", ref},
	)
}
