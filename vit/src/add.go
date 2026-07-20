package vit

import (
	"context"
	"errors"
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
	c.logger.Info("AddAsset", "assetPath", assetPath, "src", src)

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
		defer dbClient.Close()

		op := newOperationAdd(opctx.RepoPath, assetPath, src)

		if err := transaction.RunTreeChanged(
			ctx,
			transaction.Request{
				Opctx:     opctx,
				DBClient:  dbClient,
				Presenter: presenter,
				Operation: op,
				Logger:    c.logger,
				Progress:  c.progress,
				Command:   types.CommandAdd,
				User:      opctx.User,
				RepoPath:  opctx.RepoPath,
				Timestamp: opctx.TimeStamp,
			},
			[]string{assetPath},
		); err != nil {
			return err
		}

		pathbranch = op.WorkspaceBranchPath
		return nil
	})

	// TODO WTF?
	if err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		err = types.NewCancelledError(err)
	}

	return &types.StringResult{String: pathbranch}, err
}

type operationAdd struct {
	// filled at init.
	RepoPath  string
	AssetPath string
	Src       string

	// filled at Validate
	ClosestPathFound string

	// filled at PlanBlobUpdate
	BlobPathBranch    string
	ContentNode       *types.ContentNode
	BaselineTimeStamp time.Time

	// filled at Finish
	WorkspaceBranchPath string
}

var _ transaction.Operation = (*operationAdd)(nil) // enforcing operation interface.

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

func (o *operationAdd) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	progress types.ProgressWriter,
) (types.BlobUpdateRequest, error) {
	plan, err := blob.InitAsset(
		ctx,
		blob.InitAssetRequest{
			RepoPath:  o.RepoPath,
			AssetPath: o.AssetPath,
			Src:       o.Src,
			User:      opctx.User,
			Timestamp: opctx.TimeStamp,
		},
	)
	if err != nil {
		return types.BlobUpdateRequest{}, err
	}

	o.ContentNode = plan.ContentRoots[0].Root
	o.BlobPathBranch, err = blob.BranchContentPath(o.RepoPath, o.AssetPath, types.DefaultBranchName)
	if err != nil {
		return types.BlobUpdateRequest{}, err
	}

	return plan, nil
}

func (o *operationAdd) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	o.BaselineTimeStamp = time.Now()

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
	logger *slog.Logger,
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
		logger.Log(ctx, slog.LevelError, "workspace present failed", "err", err)
	}

	if err := workspace.PresentCommit(
		ctx,
		opctx.RepoPath,
		o.AssetPath,
		types.FormatTimestampLocal(opctx.TimeStamp),
		p,
		o.ContentNode,
	); err != nil {
		logger.Log(ctx, slog.LevelError, "workspace present failed", "err", err)
	}
}

func newSourceNotFoundErr(assetPath, src string) error {
	return types.NewVitError(
		types.ErrFileNotFound,
		[]string{fmt.Sprintf("can't create asset '%s': source not found: '%s'", assetPath, src)},
		[]any{"assetPath", assetPath, "src", src},
	)
}
