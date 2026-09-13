package vit

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"vit/internal/db"
	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

func (c *Client) SourceAdd(
	ctx context.Context,
	pathObjectWithNewSource, pathObjectSource, sourceName string,
) (*types.StringResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}

	sourceName = fsutil.PathInternal(sourceName)
	if sourceName == "" || sourceName == "." ||
		strings.HasPrefix(sourceName, "/") ||
		strings.HasPrefix(sourceName, "..") {
		return nil, types.NewVitError(
			types.ErrInvalidInut,
			[]string{fmt.Sprintf("invalid source name, must be a relative path: %q",
				sourceName)},
			nil,
		)
	}

	c.logger.Info(
		"SourceAdd",
		"pathObjectSource", pathObjectSource,
		"pathObjectWithNewSource", pathObjectWithNewSource,
		"sourceName", sourceName,
	)

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	err = opcontext.WithOperationContext(
		ctx,
		pathObjectWithNewSource,
		func(opctx *opcontext.OperationContext, pathConsumer string) error {
			// consumer (object with new source) has to be a branch
			refConsumer, err := workspace.ResolveRefFromPath(ctx, opctx, pathConsumer)
			if err != nil {
				return err
			}
			if err := refConsumer.CheckTypeOfRef(types.RefTypeBranch); err != nil {
				return err
			}

			srcPath := pathObjectSource
			if rp := opctx.RepoPath(); strings.HasPrefix(srcPath, rp) {
				if rel, rerr := fsutil.RelInternal(rp, srcPath); rerr == nil {
					srcPath = rel
				}
			}
			refSource, err := workspace.ResolveRefFromPath(ctx, opctx, srcPath)
			if err != nil {
				return err
			}

			dbClient, err := db.NewClient(ctx, opctx, c.logger)
			if err != nil {
				return err
			}
			defer dbClient.Close()

			op := &operationSourceAdd{
				RefWithNewDep:    refConsumer,
				RefDepSpec:       refSource,
				NewDepedencyName: sourceName,
			}

			return transaction.RunEdit(
				ctx,
				transaction.Request{
					Opctx:     opctx,
					DBClient:  dbClient,
					Presenter: presenter,
					Operation: op,
					Logger:    c.logger,
					Progress:  c.progress,
					Command:   types.CommandSourceAdd,
					User:      opctx.User,
					RepoPath:  opctx.RepoPath(),
					Timestamp: opctx.TimeStamp,
				},
				[]*types.Ref{refConsumer}, transaction.TransactionBranch,
			)
		},
	)
	if err != nil {
		return nil, err
	}

	return &types.StringResult{}, nil
}

type operationSourceAdd struct {
	// input given at init
	RefWithNewDep    *types.Ref
	RefDepSpec       *types.Ref
	NewDepedencyName string

	// filled at validate
	RefDepResolved *types.Ref
	DepCommitID    string
}

func (o *operationSourceAdd) Validate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	// TODO : release all write at end of validate in trasaction
	// TODO: resolve all graph and check name conflict.

	// 1. checking ref with new dep.
	assetJSON, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.RefWithNewDep, false)
	if err != nil {
		return err
	}
	asset := assetJSON.Data

	c, err := db.GetBranchCommit(asset, o.RefWithNewDep.BranchName)
	if err != nil {
		return err
	}

	if err := db.CheckDependencyNameAvailable(o.RefWithNewDep, c, o.NewDepedencyName); err != nil {
		return err
	}
	// TODO: resolve graph and "collapse it" to check for naming clash.

	// 2. checking target ref
	assetJSON, err = dbClient.GetJSONAssetFromRef(ctx, opctx, o.RefDepSpec, false)
	if err != nil {
		return err
	}
	asset = assetJSON.Data

	o.RefDepResolved, o.DepCommitID, err = db.ResolveCommitForDependency(asset, o.RefDepSpec)
	if err != nil {
		return err
	}

	// reject a cyclic dependency now, before the transaction writes anything.
	return db.CheckSourceAddNoCycle(
		ctx, opctx, dbClient,
		o.RefWithNewDep, o.RefDepResolved,
	)
}

func (o *operationSourceAdd) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	return &types.BlobUpdatePlan{}, nil
}

func (o *operationSourceAdd) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	assetJSON, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.RefWithNewDep, true)
	if err != nil {
		return err
	}

	return db.AddSource(db.AddSourceRequest{
		Asset:       assetJSON.Data,
		BranchName:  o.RefWithNewDep.BranchName,
		SourceName:  o.NewDepedencyName,
		RefSpec:     o.RefDepSpec,
		RefResolved: o.RefDepResolved,
		CommitID:    o.DepCommitID,
	})
}

func (o *operationSourceAdd) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	g, err := db.UpdateDepGraphForSource(
		ctx, opctx, dbClient,
		o.RefWithNewDep, o.RefDepResolved, o.RefDepSpec, o.NewDepedencyName, o.DepCommitID,
	)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "source add: dep graph update failed", "err", err)
		return
	}

	// present the closure into the consumer workspace (recursive hardlinks under
	// the dependency name path).
	consumerWs := fsutil.PathOS(
		o.RefWithNewDep.RepoPath,
		workspace.RefToWorkspacePath(o.RefWithNewDep),
	)
	if err := workspace.PresentSources(ctx, o.RefWithNewDep.RepoPath, consumerWs, g, o.RefWithNewDep, p); err != nil {
		logger.Log(ctx, slog.LevelError, "source add: present sources failed", "err", err)
	}
}
