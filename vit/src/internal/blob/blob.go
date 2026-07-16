package blob

import (
	"context"
	"os"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

type InitAssetRequest struct {
	RepoPath  string
	AssetPath string
	Src       string
	User      string
	Timestamp time.Time
}

type InitAssetResult struct {
	BranchContentBlobPath string
	CommitRootContentNode *types.ContentNode
	BaselineTimeStamp     time.Time
}

func InitAsset(
	ctx context.Context,
	progress types.ProgressWriter,
	request InitAssetRequest,
) (result *InitAssetResult, err error) {
	commitPath := commitBlobPath(request.RepoPath, request.AssetPath, request.Timestamp)
	branchPath := branchBlobPath(request.RepoPath, request.AssetPath, types.DefaultBranchName)

	srcDirStat, err := sourceToDirStat(ctx, request.Src)
	if err != nil {
		return nil, err
	}

	// 1. Copy source → commit blob (immutable), then commit → branch blob (the
	if err = fsutil.CopyFiles(ctx, fsutil.NewCopyProgress(progress), []fsutil.CopyFilesRequest{
		{DstRoot: commitPath, Src: srcDirStat},
		{DstRoot: branchPath, Src: srcDirStat.WithRoot(commitPath)},
	}); err != nil {
		return nil, err
	}

	// 2. Write manifest next to commit blob
	ref := types.NewRefBranch(request.RepoPath, request.AssetPath, types.DefaultBranchName)

	// TODO: manifest with all the files (inlcuding those who are not there)
	// and a small "sparesfile" bool to knwo at a glance if there is everything or not.
	if err := writeManifest(commitPath, ref, request.User, request.Timestamp, nil); err != nil {
		return nil, err
	}

	rootContentNode, err := initAssetContentBlob(request.RepoPath, request.AssetPath, commitPath)
	if err != nil {
		return nil, err
	}

	branchContent, err := fsutil.RelInternal(
		fsutil.PathOS(request.RepoPath, ".vit", "blob"),
		branchPath,
	)
	if err != nil {
		return nil, err
	}

	return &InitAssetResult{
		BranchContentBlobPath: branchContent,
		CommitRootContentNode: rootContentNode,
		BaselineTimeStamp:     time.Now(),
	}, nil
}

func sourceToDirStat(ctx context.Context, src string) (*types.DirStat, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	if srcInfo.IsDir() {
		return fsutil.NewDirStatFromDir(ctx, src)
	}
	ds := types.NewDirStat(fsutil.DirOS(src))
	ds.Data[fsutil.BaseOS(src)] = types.FileStat{Size: srcInfo.Size(), MTime: srcInfo.ModTime()}
	return ds, nil
}
