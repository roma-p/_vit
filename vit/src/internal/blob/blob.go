package blob

import (
	"context"
	"os"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

type BlobMetadata struct {
	Size int64
	Hash string
}

// InitAsset copies the source file into the blob store for a new asset.
// Creates both the immutable commit blob and the mutable branch blob.
// Returns the commit blob path, branch blob path, and metadata.
func InitAsset(
	ctx context.Context,
	repoPath, assetPath, src string,
	user string,
	timestamp time.Time,
	progress types.ProgressWriter,
) (commitPath, branchPath string, meta *BlobMetadata, err error) {
	commitPath = commitBlobPath(repoPath, assetPath, timestamp)
	branchPath = branchBlobPath(repoPath, assetPath, types.DefaultBranchName)

	// 1. Copy source → commit blob
	hash, size, err := copyToBlob(ctx, src, commitPath, progress)
	if err != nil {
		return "", "", nil, err
	}

	meta = &BlobMetadata{Size: size, Hash: hash}

	// 2. Write manifest next to commit blob
	ref := types.NewRefBranch(repoPath, assetPath, types.DefaultBranchName)
	if err := writeManifest(commitPath, ref, user, timestamp, nil); err != nil {
		return "", "", nil, err
	}

	// 3. Full copy commit blob → branch blob (artist's mutable working copy)
	srcInfo, err := os.Stat(src)
	if err != nil {
		return "", "", nil, err
	}
	if err := copyCommitToBranch(ctx, commitPath, branchPath, srcInfo.IsDir(), progress); err != nil {
		return "", "", nil, err
	}

	return commitPath, branchPath, meta, nil
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
	}
	return fsutil.CopyFile(ctx, src, dst, cp)
}

func copyCommitToBranch(ctx context.Context, commitPath, branchPath string, isDir bool, progress types.ProgressWriter) error {
	if err := os.MkdirAll(fsutil.DirOS(branchPath), 0o755); err != nil {
		return err
	}

	cp := fsutil.NewCopyProgress(progress)
	if isDir {
		_, _, err := fsutil.CopyDir(ctx, commitPath, branchPath, cp)
		return err
	}
	_, _, err := fsutil.CopyFile(ctx, commitPath, branchPath, cp)
	return err
}
