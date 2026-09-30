package vit

import (
	"context"
	"fmt"
	"log/slog"

	"vit/internal/db"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

// SourcePull re-resolves a branch's sources to the current version of each
// source's RefSpec — branch -> new head commit, floating tag -> newer named tag;
// an @commit= source is left untouched.
// the dep graph is then rebuilt.
func (c *Client) SourcePull(
	ctx context.Context,
	path string,
	sourceNames []string,
) (*types.SourceUpdateResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("SourcePull", "path", path, "sourceNames", sourceNames)

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	op := &operationSourcePull{SourceNames: sourceNames}

	err = opcontext.WithOperationContext(
		ctx,
		path,
		func(opctx *opcontext.OperationContext, pathConsumer string) error {
			refConsumer, err := workspace.ResolveRefFromPath(ctx, opctx, pathConsumer)
			if err != nil {
				return err
			}
			if err := refConsumer.CheckTypeOfRef(types.RefTypeBranch); err != nil {
				return err
			}
			op.RefConsumer = refConsumer

			dbClient, err := db.NewClient(ctx, opctx, c.logger)
			if err != nil {
				return err
			}
			defer dbClient.Close()

			return transaction.RunEdit(
				ctx,
				transaction.Request{
					Opctx:     opctx,
					DBClient:  dbClient,
					Presenter: presenter,
					Operation: op,
					Logger:    c.logger,
					Progress:  c.progress,
					Command:   types.CommandSourcePull,
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

	return &types.SourceUpdateResult{
		Consumer: op.RefConsumer,
		Updates:  op.Updates,
	}, nil
}

type operationSourcePull struct {
	// init
	RefConsumer *types.Ref
	SourceNames []string // nil/empty == all sources

	// filled at Validate
	Updates []types.SourceUpdate
}

func (o *operationSourcePull) Validate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	assetJSON, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.RefConsumer, false)
	if err != nil {
		return err
	}

	commit, err := db.GetBranchCommit(assetJSON.Data, o.RefConsumer.BranchName)
	if err != nil {
		return err
	}

	var names []string
	if len(o.SourceNames) > 0 {
		names = o.SourceNames
	} else {
		for name, dep := range commit.Dependencies {
			if dep.Type == types.AssetDependencySource {
				names = append(names, name)
			}
		}
	}

	for _, name := range names {
		dep, ok := commit.Dependencies[name]
		if !ok || dep.Type != types.AssetDependencySource {
			return types.NewVitError(
				types.ErrInvalidInut,
				[]string{fmt.Sprintf("no source named %q on %s", name, o.RefConsumer.RelativePath())},
				nil,
			)
		}

		srcJSON, err := dbClient.GetJSONAssetFromRef(ctx, opctx, dep.RefSpec, false)
		if err != nil {
			return err
		}

		newTarget, newCommit, err := db.ResolveCommitForDependency(srcJSON.Data, dep.RefSpec)
		if err != nil {
			return err
		}

		if newCommit == dep.Commit {
			continue
		}

		if err := db.CheckSourceAddNoCycle(ctx, opctx, dbClient, o.RefConsumer, newTarget); err != nil {
			return err
		}

		o.Updates = append(o.Updates, types.SourceUpdate{
			Name:      name,
			RefSpec:   dep.RefSpec,
			OldTarget: dep.RefResolved,
			OldCommit: dep.Commit,
			NewTarget: newTarget,
			NewCommit: newCommit,
			SrcUID:    srcJSON.Data.AssetUID,
		})
	}
	return nil
}

func (o *operationSourcePull) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	return &types.BlobUpdatePlan{}, nil
}

func (o *operationSourcePull) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	if len(o.Updates) == 0 {
		return nil
	}

	assetJSON, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.RefConsumer, true)
	if err != nil {
		return err
	}

	for _, u := range o.Updates {
		if err := db.AddSource(db.AddSourceRequest{
			Asset:       assetJSON.Data,
			BranchName:  o.RefConsumer.BranchName,
			SourceName:  u.Name,
			RefSpec:     u.RefSpec,
			RefResolved: u.NewTarget,
			CommitID:    u.NewCommit,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (o *operationSourcePull) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	if len(o.Updates) == 0 {
		return
	}
	var g *types.DepGraph
	for _, u := range o.Updates {
		if err := db.RemoveDependent(
			u.OldTarget.RepoPath,
			u.SrcUID,
			u.OldTarget.ObjectPath(),
			o.RefConsumer,
		); err != nil {
			logger.Log(ctx, slog.LevelError, "source pull: reverse prune failed", "err", err)
		}

		if err := db.MarkDependentsStale(ctx, opctx, dbClient, u.OldTarget); err != nil {
			logger.Log(ctx, slog.LevelError, "source pull: mark-stale (old pin) failed", "err", err)
		}

		ng, err := db.UpdateDepGraphForSource(
			ctx, opctx, dbClient,
			db.UpdateDepGraphForSourceRequest{
				Consumer: o.RefConsumer,
				Target:   u.NewTarget,
				Spec:     u.RefSpec,
				Name:     u.Name,
				Commit:   u.NewCommit,
			},
		)
		if err != nil {
			logger.Log(ctx, slog.LevelError, "source pull: dep graph update failed", "err", err)
			continue
		}
		g = ng
	}
	if g == nil {
		return
	}
	if err := workspace.PresentSources(ctx, g, o.RefConsumer, p); err != nil {
		logger.Log(ctx, slog.LevelError, "source pull: present sources failed", "err", err)
	}
}
