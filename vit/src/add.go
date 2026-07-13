package vit

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

func (c *Client) Add(ctx context.Context, assetPath, src string) (*types.StringResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}

	assetPath = fsutil.PathOS(assetPath)

	if src != "" {
		src = fsutil.PathOS(src)
		_, err := os.Stat(src)
		if err != nil {
			return nil, newSourceNotFoundErr(assetPath, src)
		}
	}

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	c.logger.Info("AddAsset", "assetPath", assetPath, "src", src)

	var pathbranch string

	err = opcontext.WithOperationContext(ctx, assetPath, func(
		opctx *opcontext.OperationContext,
		assetPath string, // assetPath is internal here.
	) error {
		assetPath = fsutil.PathInternal(assetPath)

		dbClient, err := db.NewClient(ctx, opctx, c.logger)
		if err != nil {
			return err
		}

		op := newOperationAdd(opctx.RepoPath, assetPath, src)

		return transaction.WithTransactionTreeChanged(
			ctx,
			opctx.RepoPath,
			[]string{assetPath},
			types.CommandAdd,
			opctx.User,
			opctx.TimeStamp,
			func() error {
				return op.Validate(ctx, opctx, dbClient)
			},
			func() error {
				ret := operateAdd(ctx, opctx, c, dbClient, op, presenter)
				pathbranch = op.WorkspaceBranchPath
				return ret
			},
		)
	})

	return &types.StringResult{String: pathbranch}, err
}

func operateAdd(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	client *Client,
	dbClient *db.Client,
	opAdd *operationAdd,
	presenter workspace.Presenter,
) error {
	if err := opAdd.BlobUpdate(ctx, opctx, client.progress); err != nil {
		return err
	}

	if err := opAdd.DBUpdate(ctx, opctx, dbClient); err != nil {
		return err
	}

	if err := opctx.JSONPool.FlushForWrite(); err != nil {
		return err
	}

	opAdd.Finish(ctx, opctx, client, presenter)

	return nil
}

type operationAdd struct {
	RepoPath  string
	AssetPath string
	Src       string

	// filled at Validate
	ClosestPathFound string

	// filled at BlobUpdate
	BlobPathBranch    string
	ContentNode       *types.ContentNode
	BaselineTimeStamp time.Time

	// filled at Finish
	WorkspaceBranchPath string
}

var _ Operation = (*operationAdd)(nil)

func newOperationAdd(repoPath, assetPath, src string) *operationAdd {
	return &operationAdd{
		RepoPath:  repoPath,
		AssetPath: assetPath,
		Src:       src,
	}
}

func (o *operationAdd) Validate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	var err error
	o.ClosestPathFound, err = dbClient.IsAssetPathAvailable(ctx, opctx, o.RepoPath, o.AssetPath)
	return err
}

func (o *operationAdd) BlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	progress types.ProgressWriter,
) error {
	result, err := blob.InitAsset(
		ctx,
		progress,
		blob.InitAssetRequest{
			RepoPath:  o.RepoPath,
			AssetPath: o.AssetPath,
			Src:       o.Src,
			User:      opctx.User,
			Timestamp: opctx.TimeStamp,
		},
	)
	if err != nil {
		return err
	}

	o.BlobPathBranch = result.BranchContentBlobPath
	o.ContentNode = result.CommitRootContentNode
	o.BaselineTimeStamp = result.BaselineTimeStamp

	return nil
}

func (o *operationAdd) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	_, err := dbClient.AddAssetToTree(
		ctx,
		opctx,
		db.AddAssetToTreeRequest{
			RepoPath:          o.RepoPath,
			TreePath:          o.AssetPath,
			BlobBranch:        o.BlobPathBranch,
			ContentRootHash:   o.ContentNode.ContentHash,
			BaselineTimeStamp: o.BaselineTimeStamp,
		},
	)
	return err
}

func (o *operationAdd) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	c *Client,
	p workspace.Presenter,
) {
	var err error
	o.WorkspaceBranchPath, err = workspace.PresentBranch(
		ctx,
		opctx.RepoPath,
		o.BlobPathBranch,
		p,
		types.NewRefBranch(opctx.RepoPath, o.AssetPath, types.DefaultBranchName),
	)
	if err != nil {
		c.logger.Log(ctx, slog.LevelError, "workspace present failed", "err", err)
	}

	if err := workspace.PresentCommit(
		ctx,
		opctx.RepoPath,
		o.AssetPath,
		types.FormatTimestampLocal(opctx.TimeStamp),
		p,
		o.ContentNode,
	); err != nil {
		c.logger.Log(ctx, slog.LevelError, "workspace present failed", "err", err)
	}
}

func newSourceNotFoundErr(assetPath, src string) error {
	return types.NewVitError(
		types.ErrFileNotFound,
		[]string{fmt.Sprintf("can't create asset '%s': source not found: '%s'", assetPath, src)},
		[]any{"assetPath", assetPath, "src", src},
	)
}
