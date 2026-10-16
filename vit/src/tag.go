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
		ref, err := workspace.ResolveRefFromPath(ctx, opctx, path)
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

		products, tt, err := discoverTagProducts(ctx, opctx, dbClient, ref)
		if err != nil {
			return err
		}

		op := &operationTag{
			InputRef:       ref,
			TagFamily:      tagfamily,
			Message:        message,
			productTargets: products,
		}

		refs := []*types.Ref{ref}
		for _, p := range products {
			refs = append(refs, p.targetRef)
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
				Message:   message,
				Ref:       ref,
			},
			refs, tt,
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
	// TODO: add the possibility of TagName
	InputRef       *types.Ref // object to tag.
	TagFamily      string
	Message        string
	productTargets []originProduct

	// filled at validate
	doCommit     bool             // if the ref is a branch with non commit changes, will commit branch
	oc           *operationCommit // the commit operation used to tag the branch refed used.
	targetCommit string           // a tag always point to a commit in the end. Resolved commit UID.
	targetSite   string

	// filled at DBUpdate
	contentNode *types.ContentNode // the content node used to present the tag.
	outputRef   *types.Ref         // ref to the tag object that will eventually be created.
	productTags []taggedProduct    // the product tags mirrored from this one (for Finish).

	// filled at Finish
	TagWorkspacePath string
}

type taggedProduct struct {
	name        string
	outputRef   *types.Ref
	refSpec     *types.Ref
	contentNode *types.ContentNode
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
		if _, err := db.GetCommit(asset.Data, o.InputRef.CommitID); err != nil {
			return err
		}
		o.targetCommit = o.InputRef.CommitID

	case types.RefTypeBranch:
		oc := &operationCommit{Ref: o.InputRef, Message: o.Message}
		for _, p := range o.productTargets {
			oc.productCommits = append(oc.productCommits, &operationCommit{
				Ref:         p.targetRef,
				Message:     o.Message,
				ProductName: p.name,
			})
		}
		err := oc.Validate(ctx, opctx, dbClient)
		if types.IsErrCode(err, types.ErrNoChangeToCommit) {
			branchCommit, err := db.GetBranchCommit(asset.Data, o.InputRef.BranchName)
			if err != nil {
				return err
			}
			o.targetCommit = branchCommit.Parent
		} else if err != nil {
			return err
		} else {
			o.oc = oc
			o.doCommit = true
			o.targetCommit = ""
			return nil
		}

	case types.RefTypeTag:
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
	}
	return &types.BlobUpdatePlan{}, nil
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

	// 1. Tag the origin commit.
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
	o.contentNode, err = blob.LoadContentTree(o.InputRef.RepoPath, o.InputRef.AssetPath, commit.Content)
	if err != nil {
		return err
	}
	o.outputRef = types.NewRefTag(
		o.InputRef.RepoRef(), o.InputRef.AssetPath, o.TagFamily, tagName, o.targetSite,
	)

	// 2. CORE MIRROR: tag the product commits this origin commit PRODUCED — those
	//    with RefResolved set.
	return o.tagProducts(ctx, opctx, dbClient, commit)
}

func (o *operationTag) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	if o.doCommit {
		o.oc.Finish(ctx, opctx, dbClient, logger, p)
	}

	o.TagWorkspacePath = presentTag(ctx, opctx, dbClient, logger, p, o.outputRef, o.contentNode)
	for _, pt := range o.productTags {
		presentTag(ctx, opctx, dbClient, logger, p, pt.outputRef, pt.contentNode)
		if err := db.RecordProducerForProduct(
			ctx, opctx, dbClient, pt.outputRef, o.outputRef, pt.name, pt.refSpec,
		); err != nil {
			logger.Log(ctx, slog.LevelError, "tag: record producer failed", "err", err)
		}
	}
}

func (o *operationTag) tagProducts(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	originCommit *types.AssetCommit,
) error {
	for name, dep := range originCommit.Dependencies {
		if dep.Type != types.AssetDependencyProduct || dep.RefResolved == nil {
			continue
		}
		prodRef := dep.RefResolved
		prodSite := opctx.RepoRefFor(prodRef.RepoUID).Site

		prodAsset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, prodRef, true)
		if err != nil {
			return err
		}
		// already on this family for this site → skip (no double-tag).
		if err := db.CheckCommitNotAlreadyOnTagFamily(
			prodAsset.Data, prodRef.CommitID, o.TagFamily, prodSite,
		); err != nil {
			continue
		}

		tagName, err := db.TagObject(db.TagObjectRequest{
			Asset:        prodAsset.Data,
			Author:       opctx.User,
			Message:      o.Message,
			TimeStamp:    opctx.TimeStamp,
			TagFamily:    o.TagFamily,
			TargetCommit: prodRef.CommitID,
			SiteUID:      prodSite,
		})
		if err != nil {
			return err
		}
		prodCommit, err := db.GetCommit(prodAsset.Data, prodRef.CommitID)
		if err != nil {
			return err
		}
		content, err := blob.LoadContentTree(prodRef.RepoPath, prodRef.AssetPath, prodCommit.Content)
		if err != nil {
			return err
		}
		o.productTags = append(o.productTags, taggedProduct{
			name: name,
			outputRef: types.NewRefTag(
				prodRef.RepoRef(), prodRef.AssetPath, o.TagFamily, tagName, prodSite,
			),
			refSpec:     dep.RefSpec,
			contentNode: content,
		})
	}
	return nil
}

func presentTag(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
	tagRef *types.Ref,
	contentNode *types.ContentNode,
) string {
	if err := db.UpdateDepGraphForTag(ctx, opctx, dbClient, tagRef); err != nil {
		logger.Log(ctx, slog.LevelError, "tag: dep graph update failed", "err", err)
	}
	g, err := db.GetDependencies(ctx, opctx, dbClient, tagRef)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "tag: dep graph read failed", "err", err)
	}
	path, err := workspace.PresentTag(ctx, opctx.JSONPool, tagRef.RepoPath, tagRef, p, contentNode, g)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "workspace present failed", "err", err)
	}
	return path
}

func discoverTagProducts(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	ref *types.Ref,
) ([]originProduct, transaction.TransactionOperation, error) {
	tt := transaction.TransactionTag
	if ref.RefType == types.RefTypeBranch {
		tt = transaction.TransactionBranch
	}
	products, err := productEdgesForRef(ctx, opctx, dbClient, ref, false)
	return products, tt, err
}
