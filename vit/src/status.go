package vit

import (
	"context"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
	"vit/internal/workspace"
)

// TODO: pointer nodes and "ref from path" (either from ref path or workspace path)

// DO better in the result
func (c *Client) Status(ctx context.Context, path string) (*types.StatusResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}

	path = fsutil.PathInternal(path)

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	_ = presenter
	if err != nil {
		return nil, err
	}

	var changeRecord *types.ChangeRecord

	err = opcontext.WithOperationContext(ctx, path, func(opctx *opcontext.OperationContext, path string) error {
		ref, err := types.NewRefFromPath(opctx.RepoPath, path)
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
		
		changeRecord, err = resolveBranchChangeRecord(ctx, opctx, ref, dbClient, false)

		return err
	})

	if err != nil {
		return nil, err
	}

	ret := (*types.StatusResult)(changeRecord)
	return ret, nil
}

func resolveBranchChangeRecord(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	branchRef *types.Ref,
	dbClient *db.Client,
	editAsset bool, // TODO: remove this: will always be false, as it will sit in Validate during commit op.
) (*types.ChangeRecord, error) {

	workspacePath := fsutil.PathOS(branchRef.RepoPath, workspace.RefToWorkspacePath(branchRef))

	workspaceFileStat, err := fsutil.NewDirStatFromDir(ctx, workspacePath)
	if err != nil {
		return nil, err
	}

	jsonAsset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, branchRef, editAsset)
	if err != nil {
		return nil, err
	}

	commitBranch, err := db.GetBranchCommit(
		jsonAsset.Data,
		branchRef.BranchName,
	)
	if err != nil {
		return nil, err
	}

	baseline := commitBranch.Timestamp

	parentCommit, err := db.GetCommit(jsonAsset.Data, commitBranch.Parent)
	if err != nil {
		return nil, err
	}
	

	contentRoot, err := blob.LoadContentTree(
		branchRef.RepoPath,
		branchRef.AssetPath,
		parentCommit.Content,
	)
	if err != nil {
		return nil, err
	}

	lastCommitStat := contentRoot.ToDirStat("") // TODO: do we need root path?

	changes := workspaceFileStat.Diff(lastCommitStat, baseline)

	return changes, nil
}
