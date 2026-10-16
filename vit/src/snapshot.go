package vit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

func (c *Client) Snapshot(ctx context.Context, path, message string) (*types.StringResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("Snapshot", "path", path, "message", message) // maybe user?

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	var pathCommit string

	err = opcontext.WithOperationContext(ctx, path, func(
		opctx *opcontext.OperationContext,
		path string, // path is internal here.
	) error {
		ref, err := workspace.ResolveRefFromPath(ctx, opctx, path)
		if err != nil {
			return err
		}
		if err := ref.CheckTypeOfRef(types.RefTypeBranch); err != nil {
			return err
		}

		dbClient, err := db.NewClient(ctx, opctx, c.logger)
		if err != nil {
			return err
		}
		defer dbClient.Close()

		op := &operationCommit{
			Ref:        ref,
			Message:    message,
			IsSnapshot: true, // explicit `vit snapshot` → a user snapshot
		}

		products, err := productEdgesForRef(ctx, opctx, dbClient, ref, false)
		if err != nil {
			return err
		}
		refs := []*types.Ref{ref}
		for _, p := range products {
			op.productCommits = append(op.productCommits, &operationCommit{
				Ref:         p.targetRef,
				Message:     message,
				IsSnapshot:  true,
				ProductName: p.name,
			})
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
				Command:   types.CommandCommit,
				User:      opctx.User,
				RepoPath:  opctx.RepoPath(),
				Timestamp: opctx.TimeStamp,
				Message:   message,
				Ref:       ref, // history subject = the origin
			},
			refs, transaction.TransactionBranch,
		); err != nil {
			return err
		}

		pathCommit = op.CommitPath
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &types.StringResult{String: pathCommit}, nil
}

type operationCommit struct {
	// input given at init
	Ref            *types.Ref
	Message        string
	IsSnapshot     bool // true via Client.Snapshot; false when reused by Tag
	productCommits []*operationCommit
	ProductName    string

	// filled at validate
	changeRecord     *types.ChangeRecord
	contentNode      *types.ContentNode
	workspaceDirStat *types.DirStat

	// filled at DBUpdate — the created commit's ID
	// (reused by operationTag, so exposed)
	NewCommitID string

	// filled at Finish
	CommitPath string
}

func (o *operationCommit) Validate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	r, err := resolveChangeRecord(ctx, resolveChangeRecordRequest{
		Opctx:     opctx,
		BranchRef: o.Ref,
		DBClient:  dbClient,
	})
	if err != nil {
		return err
	}

	if !r.ChangeRecord.HasChange() {
		return newNothingToCommitErr(o.Ref)
	}

	o.changeRecord = r.ChangeRecord
	o.contentNode = r.PrevCommitContentNode
	o.workspaceDirStat = r.WorkspaceDirStat

	kept := o.productCommits[:0]
	for _, pc := range o.productCommits {
		if err := pc.Validate(ctx, opctx, dbClient); err != nil {
			if isNoChangeToCommit(err) {
				continue
			}
			return err
		}
		kept = append(kept, pc)
	}
	o.productCommits = kept
	return nil
}

func (o *operationCommit) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	plan := &types.BlobUpdatePlan{}

	for _, pc := range o.productCommits {
		p, err := pc.PlanBlobUpdate(ctx, opctx)
		if err != nil {
			return nil, err
		}
		appendPlan(plan, p)
	}

	self, err := blob.PlanCommitBranch(
		ctx,
		blob.PlanCommitBranchRequest{
			RepoRef:          o.Ref.RepoRef(),
			AssetPath:        o.Ref.AssetPath,
			BranchName:       o.Ref.BranchName,
			User:             opctx.User,
			Message:          o.Message,
			Timestamp:        opctx.TimeStamp,
			ContentNode:      o.contentNode,
			WorkspaceDirStat: o.workspaceDirStat,
			ChangeRecord:     o.changeRecord,
		},
	)
	if err != nil {
		return nil, err
	}
	appendPlan(plan, self)
	return plan, nil
}

func (o *operationCommit) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	// product resolved first so flush first!
	var produced map[string]*types.Ref
	if len(o.productCommits) > 0 {
		produced = make(map[string]*types.Ref, len(o.productCommits))
		for _, pc := range o.productCommits {
			if err := pc.DBUpdate(ctx, opctx, dbClient); err != nil {
				return err
			}
			produced[pc.ProductName] = types.NewRefCommit(
				pc.Ref.RepoRef(), pc.Ref.AssetPath, pc.NewCommitID,
			)
		}
	}

	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.Ref, true)
	if err != nil {
		return err
	}

	o.NewCommitID, err = db.CommitBranch(
		db.CommitBranchRequest{
			Asset:              asset.Data,
			BranchName:         o.Ref.BranchName,
			RootcontentNodeUID: o.contentNode.ContentHash,
			Author:             opctx.User,
			Site:               opctx.RepoRefFor(o.Ref.RepoUID).Site,
			Message:            o.Message,
			TimeStamp:          opctx.TimeStamp,
			IsSnapshot:         o.IsSnapshot,
			ProducedProducts:   produced, // pins product deps' RefResolved
		},
	)
	return err
}

func (o *operationCommit) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	for _, pc := range o.productCommits {
		pc.Finish(ctx, opctx, dbClient, logger, p)
	}

	commitRef := types.NewRefCommit(o.Ref.RepoRef(), o.Ref.AssetPath, o.NewCommitID)

	if err := db.UpdateDepGraphForCommit(ctx, opctx, dbClient, commitRef, o.Ref); err != nil {
		logger.Log(ctx, slog.LevelError, "commit: dep graph update failed", "err", err)
	}

	for _, pc := range o.productCommits {
		produced := types.NewRefCommit(pc.Ref.RepoRef(), pc.Ref.AssetPath, pc.NewCommitID)
		if err := db.RecordProducerForProduct(
			ctx, opctx, dbClient, produced, commitRef, pc.ProductName, pc.Ref,
		); err != nil {
			logger.Log(ctx, slog.LevelError, "commit: record producer failed", "err", err)
		}
	}
	g, err := db.GetDependencies(ctx, opctx, dbClient, commitRef)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "commit: dep graph read failed", "err", err)
	}

	o.CommitPath, err = workspace.PresentCommit(
		ctx,
		opctx.JSONPool,
		commitRef,
		o.Ref.BranchName,
		opctx.User,
		types.FormatTimestampLocal(opctx.TimeStamp),
		p,
		o.contentNode,
		g,
	)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "workspace present failed", "err", err)
	}
}

func (o *operationCommit) HistorySummary() string {
	if len(o.productCommits) == 0 {
		return ""
	}
	names := make([]string, 0, len(o.productCommits))
	for _, pc := range o.productCommits {
		names = append(names, pc.ProductName)
	}
	return fmt.Sprintf("captured %d product(s): %s", len(o.productCommits), strings.Join(names, ", "))
}

func newNothingToCommitErr(ref *types.Ref) error {
	return types.NewVitError(
		types.ErrNoChangeToCommit,
		[]string{fmt.Sprintf("no change to snapshot at: %s", ref.RelativePath())},
		[]any{"ref", ref},
	)
}

// -- SNAPSHOT CASCADE (products) --------------------------------------------

type originProduct struct {
	name      string
	targetRef *types.Ref // the product target's branch ref — what we capture
}

// productEdgesForRef find products of given ref.
// if ref is branch : returns either branch proeucts or last commit on branch
func productEdgesForRef(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	ref *types.Ref,
	committed bool,
) ([]originProduct, error) {
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, ref, false)
	if err != nil {
		return nil, err
	}

	var deps types.AssetDependencies
	switch ref.RefType {
	case types.RefTypeBranch:
		bc, err := db.GetBranchCommit(asset.Data, ref.BranchName)
		if err != nil {
			return nil, err
		}
		if committed {
			c, err := db.GetCommit(asset.Data, bc.Parent)
			if err != nil {
				return nil, err
			}
			deps = c.Dependencies
		} else {
			deps = bc.Dependencies
		}
	case types.RefTypeCommit:
		c, err := db.GetCommit(asset.Data, ref.CommitID)
		if err != nil {
			return nil, err
		}
		deps = c.Dependencies
	case types.RefTypeTag:
		_, commitID, err := db.ResolveCommitForDependency(asset.Data, ref)
		if err != nil {
			return nil, err
		}
		c, err := db.GetCommit(asset.Data, commitID)
		if err != nil {
			return nil, err
		}
		deps = c.Dependencies
	}
	return collectProductEdges(deps), nil
}

func collectProductEdges(deps types.AssetDependencies) []originProduct {
	out := make([]originProduct, 0, len(deps))
	for name, dep := range deps {
		if dep.Type == types.AssetDependencyProduct {
			out = append(out, originProduct{name: name, targetRef: dep.RefSpec})
		}
	}
	return out
}

func appendPlan(dst, src *types.BlobUpdatePlan) {
	dst.CopyRequests = append(dst.CopyRequests, src.CopyRequests...)
	dst.ContentRoots = append(dst.ContentRoots, src.ContentRoots...)
	dst.Manifests = append(dst.Manifests, src.Manifests...)
}

func isNoChangeToCommit(err error) bool {
	var ve *types.VitError
	if errors.As(err, &ve) {
		return ve.Name == types.ErrNoChangeToCommit
	}
	return false
}
