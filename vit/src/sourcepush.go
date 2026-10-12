package vit

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"vit/internal/db"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

// SourcePush resolve an object from path, resolve every mutable object
// where that object "family" is used as source, and update it to its later version.
//
//	-> for tag: from a floating tag ref, set all sources to the latest version.
//	-> for branch: update to the latest commit on branch
//
// Under the hood, this is a bulk source pull operation.
func (c *Client) SourcePush(
	ctx context.Context,
	path string,
	consumers []*types.Ref,
) (*types.SourcePushResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("SourcePush", "path", path)

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	restrict := map[string]bool{}
	for _, r := range consumers {
		restrict[r.RelativePath()] = true
	}

	op := &operationSourcePush{}

	err = opcontext.WithOperationContext(
		ctx,
		path,
		func(opctx *opcontext.OperationContext, pathProducer string) error {
			producer, err := workspace.ResolveRefFromPath(ctx, opctx, pathProducer)
			if err != nil {
				return err
			}

			if !producer.IsMutable() {
				return types.NewVitError(
					types.ErrInvalidInut,
					[]string{"source push target must be a floating tag family or a branch"},
					[]any{"ref", producer.RelativePath()},
				)
			}
			op.Producer = producer

			dbClient, err := db.NewClient(ctx, opctx, c.logger)
			if err != nil {
				return err
			}
			defer dbClient.Close()

			// find every object consummers before holding the transction lock.
			// so could look like a TOCTU race but:
			// - 1. we need it to know the list of ref we need the transaction to lock.
			// - 2. every consumers is recheck at Validate. so, worst case scenario is
			//      between discoverPushConsumers() and transaction lock, we missed one.
			//      not too bad.
			refs, subOps, err := discoverPushConsumers(ctx, opctx, dbClient, producer, restrict)
			if err != nil {
				return err
			}
			op.subOps = subOps

			if len(refs) == 0 {
				return nil
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
					Command:   types.CommandSourcePush,
					User:      opctx.User,
					RepoPath:  opctx.RepoPath(),
					Timestamp: opctx.TimeStamp,
					Ref:       producer, // history subject = the producer, not a consumer
				},
				refs, transaction.TransactionBranch,
			)
		},
	)
	if err != nil {
		return nil, err
	}

	res := &types.SourcePushResult{Producer: op.Producer}
	for _, s := range op.subOps {
		res.Consumers = append(res.Consumers, types.SourcePullResult{
			Consumer: s.RefConsumer,
			Updates:  s.Updates,
		})
	}
	return res, nil
}

type operationSourcePush struct {
	Producer *types.Ref
	subOps   []*operationSourcePull
}

// HistorySummary: "N update(s) → M consumer(s): <consumer refs>".
func (o *operationSourcePush) HistorySummary() string {
	if len(o.subOps) == 0 {
		return "no live consumers"
	}
	names := make([]string, 0, len(o.subOps))
	updates := 0
	for _, s := range o.subOps {
		names = append(names, s.RefConsumer.RelativePath())
		updates += len(s.Updates)
	}
	return fmt.Sprintf("%d update(s) → %d consumer(s): %s",
		updates, len(o.subOps), strings.Join(names, ", "))
}

func (o *operationSourcePush) Validate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	for _, s := range o.subOps {
		if err := s.Validate(ctx, opctx, dbClient); err != nil {
			return err
		}
	}
	return nil
}

func (o *operationSourcePush) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	return &types.BlobUpdatePlan{}, nil
}

func (o *operationSourcePush) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	for _, s := range o.subOps {
		if err := s.DBUpdate(ctx, opctx, dbClient); err != nil {
			return err
		}
	}
	return nil
}

func (o *operationSourcePush) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	for _, s := range o.subOps {
		s.Finish(ctx, opctx, dbClient, logger, p)
	}
}

// -- FILE LOCAL -------------------------------------------------------------

func discoverPushConsumers(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	producer *types.Ref,
	restrict map[string]bool,
) ([]*types.Ref, []*operationSourcePull, error) {
	g, err := db.GetDependents(ctx, opctx, dbClient, producer)
	if err != nil {
		return nil, nil, err
	}
	byConsumer := map[string]*operationSourcePull{}
	var refs []*types.Ref
	for i := range g.Edges {
		e := g.Edges[i]
		if e.Provider.RelativePath() != producer.RelativePath() {
			continue // direct consumers only
		}
		if e.Consumer.RefType != types.RefTypeBranch {
			continue // only mutable objects
		}
		if len(restrict) > 0 && !restrict[e.Consumer.RelativePath()] {
			continue
		}
		key := e.Consumer.RelativePath()
		sub, ok := byConsumer[key]
		if !ok {
			sub = &operationSourcePull{RefConsumer: e.Consumer}
			byConsumer[key] = sub
			refs = append(refs, e.Consumer)
		}
		sub.SourceNames = append(sub.SourceNames, e.Name)
	}
	var subOps []*operationSourcePull
	for _, r := range refs {
		subOps = append(subOps, byConsumer[r.RelativePath()])
	}
	return refs, subOps, nil
}
