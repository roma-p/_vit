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
	pathObjectConsumer, pathObjectProvider, sourceName string,
) (*types.StringResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}

	sourceName = fsutil.PathInternal(sourceName)
	if err := validateDependencyName(sourceName); err != nil {
		return nil, err
	}

	c.logger.Info(
		"SourceAdd",
		"provider", pathObjectProvider,
		"consumer", pathObjectConsumer,
		"sourceName", sourceName,
	)

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	err = opcontext.WithOperationContext(
		ctx,
		pathObjectConsumer,
		func(opctx *opcontext.OperationContext, pathObjectConsumer string) error {
			refConsumer, err := workspace.ResolveRefFromPath(ctx, opctx, pathObjectConsumer)
			if err != nil {
				return err
			}
			if err := refConsumer.CheckTypeOfRef(types.RefTypeBranch); err != nil {
				return err
			}

			refSource, err := workspace.ResolveRefFromPath(ctx, opctx, pathObjectProvider)
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

			if err := transaction.RunEdit(
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
			); err != nil {
				return err
			}

			registerCrossRepo(ctx, c.logger, refConsumer, refSource)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	// TODO change eturn tye.
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
	// TODO: resolve graph and "collapse it" to check for cross asset source name clash.
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

// HistorySummary: "<name> ← <requested spec> (→ <resolved product>)".
func (o *operationSourceAdd) HistorySummary() string {
	return fmt.Sprintf("%s ← %s (→ %s)",
		o.NewDepedencyName,
		o.RefDepSpec.RelativePath(),
		o.RefDepResolved.RelativePath(),
	)
}

func (o *operationSourceAdd) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	g, err := db.UpdateDepGraphForSource(
		ctx,
		opctx,
		dbClient,
		db.UpdateDepGraphForSourceRequest{
			Consumer: o.RefWithNewDep,
			Target:   o.RefDepResolved,
			Spec:     o.RefDepSpec,
			Name:     o.NewDepedencyName,
			Commit:   o.DepCommitID,
		},
	)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "source add: dep graph update failed", "err", err)
		return
	}

	if err := workspace.PresentSources(ctx, g, o.RefWithNewDep, p); err != nil {
		logger.Log(ctx, slog.LevelError, "source add: present sources failed", "err", err)
	}
}

func validateDependencyName(name string) error {
	if name == "" || name == "." ||
		strings.HasPrefix(name, "/") ||
		strings.HasPrefix(name, "..") {
		return types.NewVitError(
			types.ErrInvalidInut,
			[]string{fmt.Sprintf("invalid dependency name, must be a relative path: %q",
				name)},
			nil,
		)
	}
	return nil
}
