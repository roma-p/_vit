package vit

import (
	"context"

	"vit/internal/db"
	"vit/internal/opcontext"
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

	c.logger.Info(
		"AddSource",
		"pathObjectSource", pathObjectSource,
		"pathObjectWithNewSource", pathObjectWithNewSource,
		"sourceName", sourceName,
	)

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}
	_ = presenter

	err = opcontext.WithOperationContext(
		ctx,
		pathObjectWithNewSource,
		func(opctx *opcontext.OperationContext,
			pathObjectWithNewSource string,
		) error {
			return nil
		},
	)

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
	// TODO : blob update: if blob plan empty: DO NOHTING! no progress bar

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

	return nil
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
