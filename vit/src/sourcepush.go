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

	op := &operationSourcePush{restrict: restrict}

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

			return transaction.Run(
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
			)
		},
	)
	if err != nil {
		return nil, err
	}

	res := &types.SourcePushResult{Producer: op.Producer}
	for _, s := range op.subOps {
		if op.failed[s.RefConsumer.RelativePath()] {
			continue
		}
		res.Consumers = append(res.Consumers, types.SourcePullResult{
			Consumer: s.RefConsumer,
			Updates:  s.Updates,
		})
	}
	if len(op.errs) > 0 {
		return res, newErrSourcePushPartial(op.Producer, res, op.errs)
	}
	return res, nil
}

type operationSourcePush struct {
	Producer *types.Ref
	restrict map[string]bool
	subOps   []*operationSourcePull
	failed   map[string]bool // consumer RelativePath -> failed
	errs     []pushConsumerErr
}

type pushConsumerErr struct {
	consumer *types.Ref
	err      error
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

func (o *operationSourcePush) Locks() transaction.Locks {
	refs := make([]*types.Ref, 0, len(o.subOps))
	for _, s := range o.subOps {
		refs = append(refs, s.RefConsumer)
	}
	return transaction.Locks{Edited: refs, EditOp: transaction.TransactionBranch}
}

// Prepare finds the consumers before the lock: a consumer added in between is
// missed, every found one is rechecked at Validate.
func (o *operationSourcePush) Prepare(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	var err error
	o.subOps, err = discoverPushConsumers(ctx, opctx, dbClient, o.Producer, o.restrict)
	if err != nil {
		return err
	}
	for _, s := range o.subOps {
		if err := s.Prepare(ctx, opctx, dbClient); err != nil {
			return err
		}
	}
	return nil
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
	o.failed = make(map[string]bool)
	for _, s := range o.subOps {
		if err := s.DBUpdate(ctx, opctx, dbClient); err != nil {
			o.failed[s.RefConsumer.RelativePath()] = true
			o.errs = append(o.errs, pushConsumerErr{consumer: s.RefConsumer, err: err})
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
		if o.failed[s.RefConsumer.RelativePath()] {
			continue // don't present if dbupdate failed for that consumer.
		}
		s.Finish(ctx, opctx, dbClient, logger, p)
	}
}

// -- FILE LOCAL -------------------------------------------------------------

func newErrSourcePushPartial(
	producer *types.Ref,
	res *types.SourcePushResult,
	errs []pushConsumerErr,
) error {
	msgs := []string{fmt.Sprintf("source push partially failed: %s", producer.RelativePath())}

	updated := make([]string, 0, len(res.Consumers))
	for _, c := range res.Consumers {
		if len(c.Updates) > 0 {
			updated = append(updated, c.Consumer.RelativePath())
		}
	}
	if len(updated) > 0 {
		msgs = append(msgs, "updated: "+strings.Join(updated, ", "))
	}

	failed := make([]string, 0, len(errs))
	for _, e := range errs {
		msgs = append(msgs, fmt.Sprintf("FAILED: %s — %s", e.consumer.RelativePath(), e.err.Error()))
		failed = append(failed, e.consumer.RelativePath())
	}

	return types.NewVitError(
		types.ErrSourcePushPartial,
		msgs,
		[]any{"producer", producer, "updated", updated, "failed", failed},
	)
}

func discoverPushConsumers(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	producer *types.Ref,
	restrict map[string]bool,
) ([]*operationSourcePull, error) {
	g, err := db.GetDependents(ctx, opctx, dbClient, producer)
	if err != nil {
		return nil, err
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
	return subOps, nil
}
