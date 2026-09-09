package vit

import (
	"context"
	"os"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
	"vit/internal/workspace"
)

func (c *Client) Status(ctx context.Context, path string) (*types.StatusResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}

	path = fsutil.PathInternal(path)

	var changeRecord *types.ChangeRecord

	err := opcontext.WithOperationContext(ctx, path, func(opctx *opcontext.OperationContext, path string) error {
		ref, err := workspace.ResolveRefFromPath(ctx, opctx, path)
		if err != nil {
			return err
		}
		if err := ref.CheckTypeOfRef(types.RefTypeBranch); err != nil {
			return err
		}

		dbClient, err := db.NewClient(ctx, opctx, c.logger)
		if err != nil {
			return err
		}
		defer dbClient.Close()

		r, err := resolveChangeRecord(ctx, resolveChangeRecordRequest{
			Opctx:     opctx,
			BranchRef: ref,
			DBClient:  dbClient,
		})
		if err != nil {
			return err
		}

		changeRecord = r.ChangeRecord
		return nil
	})
	if err != nil {
		return nil, err
	}

	ret := (*types.StatusResult)(changeRecord)
	return ret, nil
}

type resolveChangeRecordRequest struct {
	Opctx     *opcontext.OperationContext
	BranchRef *types.Ref
	DBClient  *db.Client
}

type resolveChangeRecordResult struct {
	ChangeRecord          *types.ChangeRecord
	PrevCommitContentNode *types.ContentNode
	WorkspaceDirStat      *types.DirStat
}

func resolveChangeRecord(
	ctx context.Context,
	request resolveChangeRecordRequest,
) (*resolveChangeRecordResult, error) {
	workspacePath := fsutil.PathOS(
		request.BranchRef.RepoPath,
		workspace.RefToWorkspacePath(request.BranchRef),
	)
	if _, err := os.Stat(workspacePath); err != nil {
		return nil, workspace.NewErrUntrackedFile(request.BranchRef.RelativePath())
	}

	jsonAsset, err := request.DBClient.GetJSONAssetFromRef(
		ctx,
		request.Opctx,
		request.BranchRef,
		false,
	)
	if err != nil {
		return nil, err
	}

	commitBranch, err := db.GetBranchCommit(
		jsonAsset.Data,
		request.BranchRef.BranchName,
	)
	if err != nil {
		return nil, err
	}

	// excluding sources from status evaluation.
	// TODO: exclude results as some point!
	sourcePaths := make([]string, 0, len(commitBranch.Dependencies))
	for name := range commitBranch.Dependencies {
		sourcePaths = append(sourcePaths, name)
	}
	workspaceDirStat, err := fsutil.NewDirStatFromDir(ctx, workspacePath, sourcePaths...)
	if err != nil {
		return nil, workspace.NewErrUntrackedFile(request.BranchRef.RelativePath())
	}

	baseline := commitBranch.Timestamp

	parentCommit, err := db.GetCommit(jsonAsset.Data, commitBranch.Parent)
	if err != nil {
		return nil, err
	}

	contentRoot, err := blob.LoadContentTree(
		request.BranchRef.RepoPath,
		request.BranchRef.AssetPath,
		parentCommit.Content,
	)
	if err != nil {
		return nil, err
	}
	lastCommitStat := contentRoot.ToDirStat("") // TODO: do we need root path?

	changes := workspaceDirStat.Diff(lastCommitStat, baseline)

	return &resolveChangeRecordResult{
		ChangeRecord:          changes,
		PrevCommitContentNode: contentRoot,
		WorkspaceDirStat:      workspaceDirStat,
	}, nil
}
