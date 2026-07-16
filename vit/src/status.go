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

// TODO: need a checkRefType (err, no panic), and assertRefType
// TODO: pointer nodes and "ref from path" (either from ref path or workspace path)

// DO better in the result
func (c *Client) Status(ctx context.Context, path string) (*types.StringResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}

	path = fsutil.PathInternal(path)

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	_ = presenter
	if err != nil {
		return nil, err
	}

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

		return nil
	})

	return nil, err
}

func doStatus(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	branchRef *types.Ref,
	dbClient *db.Client,
	editAsset bool,
) error {
	workspacePath := fsutil.PathOS(branchRef.RepoPath, workspace.RefToWorkspacePath(branchRef))

	workspaceFileStat, err := fsutil.NewDirStatFromDir(ctx, workspacePath)
	if err != nil {
		return err
	}

	jsonAsset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, branchRef, editAsset)
	if err != nil {
		return err
	}

	commitBranch, err := db.GetBranchCommit(
		jsonAsset.Data,
		branchRef.BranchName,
	)
	if err != nil {
		return err
	}

	baseline := commitBranch.Timestamp

	parentCommit, err := db.GetCommit(jsonAsset.Data, commitBranch.Parent)
	if err != nil {
		return err
	}

	contentRoot, err := blob.LoadContentTree(
		branchRef.RepoPath,
		branchRef.AssetPath,
		parentCommit.Content,
	)
	if err != nil {
		return err
	}

	lastCommitStat := contentRoot.ToDirStat("")

	changes := workspaceFileStat.Diff(lastCommitStat, baseline)

	_ = changes

	// TODO: when SRC / LINK done update this! a diff can only consists on

	return nil
}
