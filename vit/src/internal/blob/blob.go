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

// InitAsset copies the source file into the blob store for a new asset.
// Creates both the immutable commit blob and the mutable branch blob.
// Returns the commit blob path, branch blob path, and metadata.
func InitAsset(
	ctx context.Context,
	progress types.ProgressWriter,
	request InitAssetRequest,
) (result *InitAssetResult, err error) {
	commitPath := commitBlobPath(request.RepoPath, request.AssetPath, request.Timestamp)
	branchPath := branchBlobPath(request.RepoPath, request.AssetPath, types.DefaultBranchName)

	// 1. Copy source → commit blob
	_, _, err = copyToBlob(ctx, request.Src, commitPath, progress)
	if err != nil {
		return nil, err
	}

	// 2. Write manifest next to commit blob
	ref := types.NewRefBranch(request.RepoPath, request.AssetPath, types.DefaultBranchName)

	// TODO: manifest with all the files (inlcuding those who are not there)
	// and a small "sparesfile" bool to knwo at a glance if there is everything or not.
	if err := writeManifest(commitPath, ref, request.User, request.Timestamp, nil); err != nil {
		return nil, err
	}

	// 3. Full copy commit blob → branch blob (artist's mutable working copy)
	if err := copyCommitToBranch(ctx, commitPath, branchPath, progress); err != nil {
		return nil, err
	}

	rootContentNode, err := initAssetContentBlob(request.RepoPath, request.AssetPath, commitPath)
	if err != nil {
		return nil, err
	}

	// The branch's Content entry stores a repo-relative POSIX path (portable
	// across machines and OSes), NOT the absolute on-disk branch path.
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

func copyToBlob(
	ctx context.Context,
	src, dst string,
	progress types.ProgressWriter,
) (hash string, size int64, err error) {
	if err := os.MkdirAll(fsutil.DirOS(dst), 0o755); err != nil {
		return "", 0, err
	}

	cp := fsutil.NewCopyProgress(progress)

	srcInfo, err := os.Stat(src)
	if err != nil {
		return "", 0, err
	}

	if srcInfo.IsDir() {
		return fsutil.CopyDir(ctx, src, dst, cp)
	} else {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return "", 0, err
		}
		dstPath := fsutil.PathOS(dst, fsutil.BaseOS(src))
		return fsutil.CopyFile(ctx, src, dstPath, cp)
	}
}

func copyCommitToBranch(ctx context.Context, commitPath, branchPath string, progress types.ProgressWriter) error {
	cp := fsutil.NewCopyProgress(progress)
	_, _, err := fsutil.CopyDir(ctx, commitPath, branchPath, cp)
	return err
}
