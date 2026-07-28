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
	closestPathFound string

	// filled at PlanBlobUpdate
	blobPathBranch    string
	contentNode       *types.ContentNode
	baselineTimeStamp time.Time

	// filled at DBUpdate
	firstCommitID string

	// filled at Finish
	WorkspaceBranchPath string
}

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
	o.closestPathFound, err = dbClient.IsAssetPathAvailable(ctx, opctx, o.RepoPath, o.AssetPath)
	return err
}

func (o *operationAdd) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	plan, err := blob.PlanInitAsset(
		ctx,
		blob.PlanInitAssetRequest{
			RepoPath:  o.RepoPath,
			AssetPath: o.AssetPath,
			Src:       o.Src,
			User:      opctx.User,
			Timestamp: opctx.TimeStamp,
		},
	)
	if err != nil {
		return nil, err
	}

	o.contentNode = plan.ContentRoots[0].Root
	o.blobPathBranch, err = blob.BranchContentPath(o.RepoPath, o.AssetPath, types.DefaultBranchName)
	if err != nil {
		return nil, err
	}

	return plan, nil
}

func (o *operationAdd) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	o.baselineTimeStamp = time.Now()

	asset, err := dbClient.AddAssetToTree(
		ctx,
		opctx,
		db.AddAssetToTreeRequest{
			RepoPath:          o.RepoPath,
			TreePath:          o.AssetPath,
			BlobBranch:        o.blobPathBranch,
			ContentRootHash:   o.contentNode.ContentHash,
			BaselineTimeStamp: o.baselineTimeStamp,
		},
	)
	if err != nil {
		return err
	}

	branch, err := db.GetBranchCommit(asset.Data, types.DefaultBranchName)
	if err != nil {
		return err
	}
	o.firstCommitID = branch.Parent
	return nil
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
		opctx.JSONPool,
		opctx.RepoPath,
		o.blobPathBranch,
		p,
		types.NewRefBranch(opctx.RepoPath, o.AssetPath, types.DefaultBranchName),
	)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "workspace present failed", "err", err)
	}

	if _, err := workspace.PresentCommit(
		ctx,
		opctx.JSONPool,
		types.NewRefCommit(opctx.RepoPath, o.AssetPath, o.firstCommitID),
		types.DefaultBranchName,
		opctx.User,
		types.FormatTimestampLocal(opctx.TimeStamp),
		p,
		o.contentNode,
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
