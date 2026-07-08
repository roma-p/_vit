package blob

import (
	"context"
	"os"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

// TOOD:
// --> 1. copy dir
// --> 2. init tree content from dir
// --> 3. init tree content from dir

// InitAsset copies the source file into the blob store for a new asset.
// Creates both the immutable commit blob and the mutable branch blob.
// Returns the commit blob path, branch blob path, and metadata.
func InitAsset(
	ctx context.Context,
	repoPath, assetPath, src string,
	user string,
	timestamp time.Time,
	progress types.ProgressWriter,
) (branchContent, commitContentNodeUID string, err error) {
	commitPath := commitBlobPath(repoPath, assetPath, timestamp)
	branchPath := branchBlobPath(repoPath, assetPath, types.DefaultBranchName)

	// 1. Copy source → commit blob
	_, _, err = copyToBlob(ctx, src, commitPath, progress)
	if err != nil {
		return "", "", err
	}

	// 2. Write manifest next to commit blob
	ref := types.NewRefBranch(repoPath, assetPath, types.DefaultBranchName)

	// TODO: manifest with all the files (inlcuding those who are not there)
	// and a small "sparesfile" bool to knwo at a glance if there is everything or not.
	if err := writeManifest(commitPath, ref, user, timestamp, nil); err != nil {
		return "", "", err
	}

	// 3. Full copy commit blob → branch blob (artist's mutable working copy)
	if err := copyCommitToBranch(ctx, commitPath, branchPath, progress); err != nil {
		return "", "", err
	}

	rootUID, err := initAssetContentBlob(repoPath, assetPath, commitPath)
	if err != nil {
		return "", "", err
	}

	// The branch's Content entry stores a repo-relative POSIX path (portable
	// across machines and OSes), NOT the absolute on-disk branch path.
	branchContent, err = fsutil.RelInternal(
		fsutil.PathOS(repoPath, ".vit", "blob"),
		branchPath,
	)
	if err != nil {
		return "", "", err
	}

	return branchContent, rootUID, nil
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
