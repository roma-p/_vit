package vit

import (
	"context"
	"fmt"
	"os"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
)

// TOOD: full path in asset! no, internal rel to repoPath

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

	c.logger.Info("AddAsset", "assetPath", assetPath, "src", src)

	var pathbranch string
	_ = pathbranch

	err := opcontext.WithOperationContext(ctx, assetPath, func(
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
				return operateAdd(ctx, opctx, c, dbClient, op)
			},
		)
	})

	return &types.StringResult{String: "prout"}, err
}

func operateAdd(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	client *Client,
	dbClient *db.Client,
	opAdd *operationAdd,
) error {
	// TODO: client shan't be arg of blob update.
	// so i guess client get a blob client?
	if err := opAdd.BlobUpdate(ctx, opctx, client); err != nil {
		return err
	}

	if err := opAdd.DBUpdate(ctx, opctx, dbClient); err != nil {
		return err
	}

	return nil
}

type operationAdd struct {
	RepoPath  string
	AssetPath string
	Src       string

	// filled at Validate
	ClosestPathFound string

	// filled at BlobUpdate
	BlobPathBranch string
	ContentNodeUID string
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
	dbClient *db.Client, // TODO(operation): add this to operation
) error {
	closestFound, err := dbClient.IsAssetPathAvailable(ctx, opctx, o.RepoPath, o.AssetPath)
	if err != nil {
		return err
	}

	o.ClosestPathFound = closestFound
	return nil
}

func (o *operationAdd) BlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	c *Client,
) error {
	var err error
	o.BlobPathBranch, o.ContentNodeUID, err = blob.InitAsset(
		ctx,
		o.RepoPath,
		o.AssetPath,
		o.Src,
		opctx.User,
		opctx.TimeStamp,
		c.progress,
	)
	return err
}

func (o *operationAdd) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client, // TODO(operation): add this to operation
) error {
	jsonAsset, err := dbClient.AddAssetToTree(
		ctx, opctx,
		o.RepoPath, o.AssetPath,
		o.BlobPathBranch, o.ContentNodeUID,
	)
	if err != nil {
		return err
	}

	if err := opctx.JSONPool.WriteHandler(jsonAsset); err != nil {
		return err
	}

	return nil
}

func (o *operationAdd) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	c *Client,
) error {
	return nil
}

// addExtToAssetPath modify assetPath based on src.
// by adding the ext of src if assetPath does not have it already.
func addExtToAssetPath(assetPath, src string) string {
	extSrc := fsutil.ExtOS(src)
	extAssetPath := fsutil.ExtInternal(assetPath)
	if extSrc != extAssetPath {
		assetPath = assetPath + extSrc
	}
	return assetPath
}

func newSourceNotFoundErr(assetPath, src string) error {
	return types.NewVitError(
		types.ErrFileNotFound,
		[]string{fmt.Sprintf("can't create asset '%s': source not found: '%s'", assetPath, src)},
		[]any{"assetPath", assetPath, "src", src},
	)
}
