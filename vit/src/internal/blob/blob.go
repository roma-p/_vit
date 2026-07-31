package blob

import (
	"context"
	"os"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

func PlanCommitBranch(
	ctx context.Context,
	r PlanCommitBranchRequest,
) (*types.BlobUpdatePlan, error) {
	topCopy := filterFilesToCopyToBlob(r.WorkspaceDirStat, r.ChangeRecord)
	commitPath := commitBlobPath(r.RepoRef.Path, r.AssetPath, r.BranchName, r.User, r.Timestamp)
	blobRootDir := fsutil.PathOS(r.RepoRef.Path, ".vit", "blob")

	if err := applyChangeRecord(
		r.ContentNode,
		r.ChangeRecord,
		r.Timestamp,
		commitPath,
		blobRootDir,
	); err != nil {
		return nil, err
	}

	return &types.BlobUpdatePlan{
		CopyRequests: []types.CopyFilesRequest{
			{
				Src:     topCopy,
				DstRoot: commitPath,
			},
		},
		ContentRoots: []types.ContentRootWrite{
			{
				ContentRootPath: genContentPath(r.RepoRef.Path, r.AssetPath),
				Root:            r.ContentNode,
			},
		},
		Manifests: []types.ManifestWrite{
			{
				CommitBlobPath: commitPath,
				Manifest: types.Manifest{
					Timestamp: r.Timestamp,
					Ref:       types.NewRefBranch(r.RepoRef, r.AssetPath, r.BranchName),
					User:      r.User,
					Message:   r.Message,
				},
			},
		},
	}, nil
}

type PlanCommitBranchRequest struct {
	RepoRef          *types.RepoRef
	AssetPath        string
	BranchName       string
	User             string
	Message          string
	Timestamp        time.Time
	ContentNode      *types.ContentNode
	WorkspaceDirStat *types.DirStat
	ChangeRecord     *types.ChangeRecord
}

func filterFilesToCopyToBlob(d *types.DirStat, c *types.ChangeRecord) *types.DirStat {
	ret := types.NewDirStat(d.RootAbsPath)
	for _, f := range c.Files {
		if f.Type == types.FileChangeRemoved {
			continue
		}
		origF, ok := d.Data[f.Path]
		if !ok {
			panic("passed a change record and a dir stat that are not matching !")
		}
		ret.Data[f.Path] = origF
	}
	return ret
}

// PlanInitAsset builds the BlobUpdatePlan for a new asset.
// - plan the first commit (a copy of the src if any)
// - plan the default branch wokring area
func PlanInitAsset(
	ctx context.Context,
	request PlanInitAssetRequest,
) (*types.BlobUpdatePlan, error) {
	commitPath := commitBlobPath(
		request.RepoRef.Path,
		request.AssetPath,
		types.DefaultBranchName,
		request.User,
		request.Timestamp,
	)
	branchPath := branchBlobPath(request.RepoRef.Path, request.AssetPath, types.DefaultBranchName)
	blobRootDir := fsutil.PathOS(request.RepoRef.Path, ".vit", "blob")

	srcDirStat, err := sourceToDirStat(ctx, request.Src)
	if err != nil {
		return nil, err
	}

	// Copy source → commit blob (immutable), then commit → branch blob.
	copyRequests := []types.CopyFilesRequest{
		{DstRoot: commitPath, Src: srcDirStat},
		{DstRoot: branchPath, Src: srcDirStat.WithRoot(commitPath)},
	}

	// Build the content tree in memory
	rootContentNode, err := buildContentTreeFromDirStat(srcDirStat, commitPath, blobRootDir)
	if err != nil {
		return nil, err
	}

	ref := types.NewRefBranch(request.RepoRef, request.AssetPath, types.DefaultBranchName)

	return &types.BlobUpdatePlan{
		CopyRequests: copyRequests,
		ContentRoots: []types.ContentRootWrite{
			{
				ContentRootPath: genContentPath(request.RepoRef.Path, request.AssetPath),
				Root:            rootContentNode,
			},
		},
		Manifests: []types.ManifestWrite{
			{
				CommitBlobPath: commitPath,
				Manifest: types.Manifest{
					Timestamp: request.Timestamp,
					Ref:       ref,
					User:      request.User,
					Message:   "Asset created",
					// TODO: missing: all files.
				},
			},
		},
	}, nil
}

type PlanInitAssetRequest struct {
	RepoRef   *types.RepoRef
	AssetPath string
	Src       string
	User      string
	Timestamp time.Time
}

func BranchContentPath(repoPath, assetPath, branchName string) (string, error) {
	return fsutil.RelInternal(
		fsutil.PathOS(repoPath, ".vit", "blob"),
		branchBlobPath(repoPath, assetPath, branchName),
	)
}

// UpdateBlob executes a plan:
// blob copies, then write content-node trees to disk, then manifests, finally manifest
// Everything it writes is content-addressed or write-once so orphan-safe.
func UpdateBlob(ctx context.Context, progress types.ProgressWriter, plan *types.BlobUpdatePlan) error {
	if plan == nil {
		return nil
	}
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
