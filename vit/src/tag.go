package vit

import (
	"context"
	"log/slog"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

func (c *Client) Tag(ctx context.Context, path, tagfamily, message string) (*types.StringResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("Tag", "path", path, "message", message)

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	var tagWorkspacePath string

	err = opcontext.WithOperationContext(ctx, path, func(
		opctx *opcontext.OperationContext,
		path string, // path is internal here.
	) error {
		ref, err := workspace.ResolveRefFromPath(ctx, opctx.JSONPool, opctx.RepoPath(), path)
		if err != nil {
			return err
		}

		// for tag: if no site, defaulting to current.
		if ref.RefType == types.RefTypeTag && ref.Site == "" {
			ref.Site = opctx.Site()
		}

		dbClient, err := db.NewClient(ctx, opctx, c.logger)
		if err != nil {
			return err
		}
		defer dbClient.Close()

		op := &operationTag{
			InputRef:  ref,
			TagFamily: tagfamily,
			Message:   message,
		}

		var tt transaction.TransactionOperation
		if ref.RefType == types.RefTypeBranch {
			tt = transaction.TransactionBranch
		} else {
			tt = transaction.TransactionTag
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
				Command:   types.CommandTag,
				User:      opctx.User,
				RepoPath:  opctx.RepoPath(),
				Timestamp: opctx.TimeStamp,
			},
			[]*types.Ref{ref}, tt,
		); err != nil {
			return err
		}

		tagWorkspacePath = op.TagWorkspacePath

		return nil
	})
	if err != nil {
		return nil, err
	}

	// TODO: presenting path
	return &types.StringResult{String: tagWorkspacePath}, nil
}

type operationTag struct {
	// input given at init
	InputRef  *types.Ref // object to tag.
	TagFamily string
	Message   string
	// TODO: add the possibility of TagName

	// filled at validate
	doCommit     bool             // if the ref is a branch with non commit changes, will commit branch
	oc           *operationCommit // the commit operation used to tag the branch refed used.
	targetCommit string           // a tag always point to a commit in the end. Resolved commit UID.
	targetSite   string

	// filled at DBUpdate
	contentNode *types.ContentNode // the content node used to present the tag.
	outputRef   *types.Ref         // ref to the tag object that will eventually be created.

	// filled at Finish
	TagWorkspacePath string
}

func (o *operationTag) Validate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	if o.TagFamily == "" {
		return types.NewVitError(
			types.ErrInvalidInut,
			[]string{"a tag family name is required"},
			[]any{"ref", o.InputRef.RelativePath()},
		)
	}

	o.targetSite = opctx.RepoRefFor(o.InputRef.RepoUID).Site

	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.InputRef, false)
	if err != nil {
		return err
	}
	switch o.InputRef.RefType {
	case types.RefTypeCommit:
		// tag an existing commit directly
		if _, err := db.GetCommit(asset.Data, o.InputRef.CommitID); err != nil {
			return err
		}
		o.targetCommit = o.InputRef.CommitID

	case types.RefTypeBranch:
		oc := &operationCommit{
			Ref:     o.InputRef,
			Message: o.Message,
		}
		err := oc.Validate(ctx, opctx, dbClient)
		if types.IsErrCode(err, types.ErrNoChangeToCommit) {
			// if nothing to commit on branch, rather than failing,
			// we tag the latest commit on branch (if not already tag for this site/family)
			branchCommit, err := db.GetBranchCommit(asset.Data, o.InputRef.BranchName)
			if err != nil {
				return err
			}
			o.targetCommit = branchCommit.Parent
		} else if err != nil {
			return err
		} else {
			// if there is change to commit on branch, we commit first, then we tag.
			// in this case the targetCommit is only resolved ar DbUpdate
			o.oc = oc
			o.doCommit = true
			o.targetCommit = ""
			return nil
		}

	case types.RefTypeTag:
		// if tag from tag: resolve underlying commit.
		tag, _, err := db.GetTag(asset.Data, o.InputRef)
		if err != nil {
			return err
		}
		o.targetCommit = tag.Commit

	default:
		panic("unreachable")
	}

	return db.CheckCommitNotAlreadyOnTagFamily(
		asset.Data, o.targetCommit, o.TagFamily, o.targetSite,
	)
}

func (o *operationTag) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	// TODO: if "heavy" commit: do a copy here!
	if o.doCommit {
		return o.oc.PlanBlobUpdate(ctx, opctx)
	} else {
		return &types.BlobUpdatePlan{}, nil
	}
}

func (o *operationTag) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.InputRef, true)
	if err != nil {
		return err
	}

	if o.doCommit {
		if err := o.oc.DBUpdate(ctx, opctx, dbClient); err != nil {
			return err
		}
		o.targetCommit = o.oc.NewCommitID
	}

	tagName, err := db.TagObject(db.TagObjectRequest{
		Asset:        asset.Data,
		Author:       opctx.User,
		Message:      o.Message,
		TimeStamp:    opctx.TimeStamp,
		TagFamily:    o.TagFamily,
		TargetCommit: o.targetCommit,
		SiteUID:      o.targetSite,
	})
	if err != nil {
		return err
	}

	commit, err := db.GetCommit(asset.Data, o.targetCommit)
	if err != nil {
		return err
	}

	o.contentNode, err = blob.LoadContentTree(
		o.InputRef.RepoPath,
		o.InputRef.AssetPath,
		commit.Content,
	)
	if err != nil {
		return err
	}

	o.outputRef = types.NewRefTag(
		o.InputRef.RepoRef(),
		o.InputRef.AssetPath,
		o.TagFamily,
		tagName,
		o.targetSite,
	)

	return nil
}

func (o *operationTag) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	if o.doCommit {
		o.oc.Finish(ctx, opctx, logger, p)
	}

	var err error

	o.TagWorkspacePath, err = workspace.PresentTag(
		ctx,
		opctx.JSONPool,
		o.InputRef.RepoPath,
		o.outputRef,
		p,
		o.contentNode,
	)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "workspace present failed", "err", err)
	}
}
