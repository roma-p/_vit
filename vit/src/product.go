package vit

import (
	"context"
	"fmt"
	"log/slog"

	"vit/internal/db"
	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

func (c *Client) ProductAdd(
	ctx context.Context,
	pathOrigin, pathTarget, productName string,
	create bool,
) (*types.StringResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}

	productName = fsutil.PathInternal(productName)
	if err := validateDependencyName(productName); err != nil {
		return nil, err
	}

	c.logger.Info(
		"ProductAdd",
		"origin", pathOrigin,
		"target", pathTarget,
		"name", productName,
		"create", create,
	)

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	err = opcontext.WithOperationContext(
		ctx,
		pathOrigin,
		func(opctx *opcontext.OperationContext, pathOrigin string) error {
			refOrigin, err := workspace.ResolveRefFromPath(ctx, opctx, pathOrigin)
			if err != nil {
				return err
			}
			if err := refOrigin.CheckTypeOfRef(types.RefTypeBranch); err != nil {
				return err
			}

			refTarget, err := workspace.ResolveCreateTarget(ctx, opctx, pathTarget)
			if err != nil {
				return err
			}
			if err := refTarget.CheckTypeOfRef(types.RefTypeBranch); err != nil {
				return err
			}

			dbClient, err := db.NewClient(ctx, opctx, c.logger)
			if err != nil {
				return err
			}
			defer dbClient.Close()

			op := &operationProductAdd{
				RefOrigin:   refOrigin,
				RefTarget:   refTarget,
				ProductName: productName,
			}

			targetExists := false
			if create {
				idx, ierr := dbClient.GetTreeIndex(ctx, opctx, refTarget.RepoPath)
				if ierr != nil {
					return ierr
				}
				if entry, ok := idx.PathToID[refTarget.AssetPath]; ok &&
					entry.Type == db.TreeNodeTypeAsset && entry.AssetUID != "" {
					targetExists = true
				}
			}
			req := transaction.Request{
				Opctx:     opctx,
				DBClient:  dbClient,
				Presenter: presenter,
				Operation: op,
				Logger:    c.logger,
				Progress:  c.progress,
				Command:   types.CommandProductAdd,
				User:      opctx.User,
				RepoPath:  refOrigin.RepoPath,
				Timestamp: opctx.TimeStamp,
				Ref:       refOrigin, // history subject = the origin getting the product
			}

			if create && !targetExists {
				op.addOp = newOperationAdd(opctx.RepoRefFor(refTarget.RepoUID), refTarget.AssetPath, "")
				if err := transaction.RunTreeChangedAndEdit(
					ctx, req,
					[]*types.Ref{refTarget},
					[]*types.Ref{refOrigin},
					transaction.TransactionBranch,
				); err != nil {
					return err
				}
			} else if err := transaction.RunEdit(
				ctx, req,
				[]*types.Ref{refOrigin}, transaction.TransactionBranch,
			); err != nil {
				return err
			}

			registerCrossRepo(ctx, c.logger, refOrigin, refTarget)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return &types.StringResult{}, nil
}

type operationProductAdd struct {
	// input given at init
	RefOrigin   *types.Ref
	RefTarget   *types.Ref
	ProductName string

	addOp *operationAdd // if product is to be created, nested  add op.
}

func (o *operationProductAdd) Validate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	assetJSON, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.RefOrigin, false)
	if err != nil {
		return err
	}
	branchCommit, err := db.GetBranchCommit(assetJSON.Data, o.RefOrigin.BranchName)
	if err != nil {
		return err
	}
	if err := db.CheckDependencyNameAvailable(o.RefOrigin, branchCommit, o.ProductName); err != nil {
		return err
	}

	if o.addOp != nil {
		return o.addOp.Validate(ctx, opctx, dbClient)
	}
	if _, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.RefTarget, false); err != nil {
		return newErrProductTargetNotFound(o.RefTarget, err)
	}
	return nil
}

func (o *operationProductAdd) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	if o.addOp != nil {
		return o.addOp.PlanBlobUpdate(ctx, opctx)
	} else {
		return &types.BlobUpdatePlan{}, nil
	}
}

func (o *operationProductAdd) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	// resolve target asset FIRST!
	// so it will be written first on the DB.
	// this way, if a failure happen between the non atomic writing of
	// creating the product asset and the update of the origin asset, we are left
	// with an new asset (the product), and the origin not referencing it as product.
	// this is normal.
	// doing ht other way could lead to the origin asset referencing a product taht
	// don't exist, which is a bug.
	if o.addOp != nil {
		if err := o.addOp.DBUpdate(ctx, opctx, dbClient); err != nil {
			return err
		}
	}

	assetJSON, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.RefOrigin, true)
	if err != nil {
		return err
	}
	return db.AddProduct(db.AddProductRequest{
		Asset:       assetJSON.Data,
		BranchName:  o.RefOrigin.BranchName,
		ProductName: o.ProductName,
		RefSpec:     o.RefTarget,
	})
}

func (o *operationProductAdd) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	if o.addOp != nil {
		o.addOp.Finish(ctx, opctx, dbClient, logger, p)
	}

	if _, err := db.UpdateDepGraphForProduct(
		ctx,
		opctx,
		dbClient,
		db.UpdateDepGraphForProductRequest{
			Origin: o.RefOrigin,
			Target: o.RefTarget,
			Spec:   o.RefTarget,
			Name:   o.ProductName,
		},
	); err != nil {
		logger.Log(ctx, slog.LevelError, "product add: dep graph update failed", "err", err)
	}

	if err := workspace.PresentProduct(
		ctx,
		o.RefOrigin,
		o.ProductName,
		o.RefTarget,
		p,
	); err != nil {
		logger.Log(ctx, slog.LevelError, "product add: present product failed", "err", err)
	}
}

// HistorySummary: "<name> → <target>".
func (o *operationProductAdd) HistorySummary() string {
	return fmt.Sprintf("%s → %s", o.ProductName, o.RefTarget.RelativePath())
}

func newErrProductTargetNotFound(ref *types.Ref, cause error) error {
	return types.NewVitError(
		types.ErrProductTargetNotFound,
		[]string{fmt.Sprintf(
			"product target not found: %s (pass --create to create it)",
			ref.RelativePath(),
		)},
		[]any{"target", ref, "cause", cause},
	)
}
