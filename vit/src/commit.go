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
				RepoPath:  opctx.RepoPath(),
				Timestamp: opctx.TimeStamp,
			},
			[]*types.Ref{ref}, transaction.TransactionBranch,
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
	// input given at init
	Ref     *types.Ref
	Message string

	// filled at validate
	changeRecord     *types.ChangeRecord
	contentNode      *types.ContentNode
	workspaceDirStat *types.DirStat

	// filled at DBUpdate — the created commit's ID
	// (reused by operationTag, so exposed)
	NewCommitID string

	// filled at Finish
	CommitPath string
}

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

	o.changeRecord = r.ChangeRecord
	o.contentNode = r.PrevCommitContentNode
	o.workspaceDirStat = r.WorkspaceDirStat
	return nil
}

func (o *operationCommit) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	plan, err := blob.PlanCommitBranch(
		ctx,
		blob.PlanCommitBranchRequest{
			RepoRef:          o.Ref.RepoRef(),
			AssetPath:        o.Ref.AssetPath,
			BranchName:       o.Ref.BranchName,
			User:             opctx.User,
			Message:          o.Message,
			Timestamp:        opctx.TimeStamp,
			ContentNode:      o.contentNode,
			WorkspaceDirStat: o.workspaceDirStat,
			ChangeRecord:     o.changeRecord,
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

	o.NewCommitID, err = db.CommitBranch(
		db.CommitBranchRequest{
			Asset:              asset.Data,
			BranchName:         o.Ref.BranchName,
			RootcontentNodeUID: o.contentNode.ContentHash,
			Author:             opctx.User,
			Site:               opctx.RepoRefFor(o.Ref.RepoUID).Site,
			Message:            o.Message,
			TimeStamp:          opctx.TimeStamp,
		},
	)
	return err
}

func (o *operationCommit) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	var err error
	o.CommitPath, err = workspace.PresentCommit(
		ctx,
		opctx.JSONPool,
		types.NewRefCommit(o.Ref.RepoRef(), o.Ref.AssetPath, o.NewCommitID),
		o.Ref.BranchName,
		opctx.User,
		types.FormatTimestampLocal(opctx.TimeStamp),
		p,
		o.contentNode,
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
