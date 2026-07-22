package vit

import (
	"context"
	"fmt"
	"log/slog"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

func (c *Client) Commit(ctx context.Context, path, message string) (*types.StringResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("Commit", "path", path, "message", message) // maybe user?

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	var pathCommit string

	err = opcontext.WithOperationContext(ctx, path, func(
		opctx *opcontext.OperationContext,
		path string, // path is internal here.
	) error {
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

		op := &operationCommit{
			Ref:     ref,
			Message: message,
		}

		if err := transaction.RunEdit(
			ctx,
			transaction.Request{
				Opctx:     opctx,
				DBClient:  dbClient,
				Presenter: presenter,
				Operation: op,
				Logger:    c.logger,
				Progress:  c.progress,
				Command:   types.CommandCommit,
				User:      opctx.User,
				RepoPath:  opctx.RepoPath,
				Timestamp: opctx.TimeStamp,
			},
			[]*types.Ref{ref},
			transaction.TransactionBranch,
		); err != nil {
			return err
		}

		pathCommit = op.CommitPath
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &types.StringResult{String: pathCommit}, nil
}

type operationCommit struct {
	Ref     *types.Ref
	Message string

	ChangeRecord     *types.ChangeRecord
	ContentNode      *types.ContentNode
	WorkspaceDirStat *types.DirStat
	CommitPath       string
}

var _ transaction.Operation = (*operationCommit)(nil) // enforcing operation interface.

func (o *operationCommit) Validate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	r, err := resolveChangeRecord(ctx, resolveChangeRecordRequest{
		Opctx:     opctx,
		BranchRef: o.Ref,
		DBClient:  dbClient,
	})
	if err != nil {
		return err
	}

	if !r.ChangeRecord.HasChange() {
		return newNothingToCommitErr(o.Ref)
	}

	o.ChangeRecord = r.ChangeRecord
	o.ContentNode = r.PrevCommitContentNode
	o.WorkspaceDirStat = r.WorkspaceDirStat
	return nil
}

func (o *operationCommit) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	plan, err := blob.PlanCommitBranch(
		ctx,
		blob.PlanCommitBranchRequest{
			RepoPath:         o.Ref.RepoPath,
			AssetPath:        o.Ref.AssetPath,
			BranchName:       o.Ref.BranchName,
			User:             opctx.User,
			Message:          o.Message,
			Timestamp:        opctx.TimeStamp,
			ContentNode:      o.ContentNode,
			WorkspaceDirStat: o.WorkspaceDirStat,
			ChangeRecord:     o.ChangeRecord,
		},
	)
	if err != nil {
		return nil, err
	}
	// TODO : things missing here.
	return plan, nil
}

func (o *operationCommit) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.Ref, true)
	if err != nil {
		return err
	}

	return db.CommitBranch(
		db.CommitBranchRequest{
			Asset:              asset.Data,
			BranchName:         o.Ref.BranchName,
			RootcontentNodeUID: o.ContentNode.ContentHash,
			Author:             opctx.User,
			Message:            o.Message,
			TimeStamp:          opctx.TimeStamp,
		},
	)
}

func (o *operationCommit) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	var err error
	o.CommitPath, err = workspace.PresentCommit(
		ctx,
		o.Ref.RepoPath,
		o.Ref.AssetPath,
		types.FormatTimestampLocal(opctx.TimeStamp),
		p,
		o.ContentNode,
	)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "workspace present failed", "err", err)
	}
}

func newNothingToCommitErr(ref *types.Ref) error {
	return types.NewVitError(
		types.ErrNoChangeToCommit,
		[]string{fmt.Sprintf("no change to commit at: %s", ref.RelativePath())},
		[]any{"ref", ref},
	)
}
