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

// InitAsset creates a BlobUpdateRequest plan for a new asset.
// - plan the first commit (a copy of the src if any)
// - plan the default branch wokring area
func InitAsset(
	ctx context.Context,
	request InitAssetRequest,
) (types.BlobUpdateRequest, error) {
	commitPath := commitBlobPath(request.RepoPath, request.AssetPath, request.Timestamp)
	branchPath := branchBlobPath(request.RepoPath, request.AssetPath, types.DefaultBranchName)
	blobRootDir := fsutil.PathOS(request.RepoPath, ".vit", "blob")

	srcDirStat, err := sourceToDirStat(ctx, request.Src)
	if err != nil {
		return types.BlobUpdateRequest{}, err
	}

	// Copy source → commit blob (immutable), then commit → branch blob.
	copyRequests := []types.CopyFilesRequest{
		{DstRoot: commitPath, Src: srcDirStat},
		{DstRoot: branchPath, Src: srcDirStat.WithRoot(commitPath)},
	}

	// Build the content tree in memory
	rootContentNode, err := buildContentTreeFromDirStat(srcDirStat, commitPath, blobRootDir)
	if err != nil {
		return types.BlobUpdateRequest{}, err
	}

	ref := types.NewRefBranch(request.RepoPath, request.AssetPath, types.DefaultBranchName)

	return types.BlobUpdateRequest{
		CopyRequests: copyRequests,
		ContentRoots: []types.ContentRootWrite{
			{ContentRootPath: genContentPath(request.RepoPath, request.AssetPath), Root: rootContentNode},
		},
		Manifests: []types.ManifestWrite{
			{
				CommitBlobPath: commitPath,
				Manifest: types.Manifest{
					Timestamp: request.Timestamp,
					Ref:       ref,
					User:      request.User,
					// TODO: missing: all files.
				},
			},
		},
	}, nil
}

func BranchContentPath(repoPath, assetPath, branchName string) (string, error) {
	return fsutil.RelInternal(
		fsutil.PathOS(repoPath, ".vit", "blob"),
		branchBlobPath(repoPath, assetPath, branchName),
	)
}

// UpdateBlob executes a plan: it materializes the whole object tier in order —
// blob copies, then content-node trees, then manifests.
// Everything it writes is content-addressed or write-once and therefore orphan-safe.
func UpdateBlob(ctx context.Context, progress types.ProgressWriter, plan types.BlobUpdateRequest) error {
	if err := fsutil.CopyFiles(ctx, fsutil.NewCopyProgress(progress), plan.CopyRequests); err != nil {
		return err
	}

	for _, cr := range plan.ContentRoots {
		if err := writeContentNodesRecc(cr.ContentRootPath, cr.Root); err != nil {
			return err
		}
	}

	for _, m := range plan.Manifests {
		if err := writeManifest(m.CommitBlobPath, m.Manifest); err != nil {
			return err
		}
	}

	return nil
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
