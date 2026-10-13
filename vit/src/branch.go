package vit

import (
	"context"
	"log/slog"
	"time"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

// Branch creates a new branch <name> rooted at the object referenced by path (a
// clean branch, a commit, a tag). Branching from a DIRTY branch is not allowed.
// new branch files are presented with inherited sources.
// TODO: needs to handle product also.
func (c *Client) Branch(ctx context.Context, path, name string) (*types.StringResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("Branch", "path", path, "name", name)

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	var branchWorkspacePath string
	err = opcontext.WithOperationContext(ctx, path, func(
		opctx *opcontext.OperationContext,
		path string, // internal here
	) error {
		ref, err := workspace.ResolveRefFromPath(ctx, opctx, path)
		if err != nil {
			return err
		}

		dbClient, err := db.NewClient(ctx, opctx, c.logger)
		if err != nil {
			return err
		}
		defer dbClient.Close()

		op := &operationBranch{InputRef: ref, BranchName: name}

		if err := transaction.RunEdit(
			ctx,
			transaction.Request{
				Opctx:     opctx,
				DBClient:  dbClient,
				Presenter: presenter,
				Operation: op,
				Logger:    c.logger,
				Progress:  c.progress,
				Command:   types.CommandBranch,
				User:      opctx.User,
				RepoPath:  opctx.RepoPath(),
				Timestamp: opctx.TimeStamp,
			},
			[]*types.Ref{ref}, transaction.TransactionBranch,
		); err != nil {
			return err
		}

		branchWorkspacePath = op.BranchWorkspacePath
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &types.StringResult{String: branchWorkspacePath}, nil
}

type operationBranch struct {
	// input
	InputRef   *types.Ref
	BranchName string

	// filled at Validate
	targetCommit string             // the commit the new branch roots at
	contentNode  *types.ContentNode // its content

	// filled at DBUpdate
	outputRef *types.Ref

	// filled at Finish
	BranchWorkspacePath string
}

func (o *operationBranch) Validate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	if o.BranchName == "" {
		return types.NewVitError(
			types.ErrInvalidInut,
			[]string{"a branch name is required"},
			[]any{"ref", o.InputRef.RelativePath()},
		)
	}

	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.InputRef, false)
	if err != nil {
		return err
	}

	// Fail early (before any blob copy) if the name is taken.
	if _, ok := asset.Data.Branches[o.BranchName]; ok {
		return types.NewVitError(
			types.ErrAssetObjectAlreadyExists,
			[]string{"a branch named '" + o.BranchName + "' already exists on this asset"},
			[]any{"asset", o.InputRef.AssetPath, "branch", o.BranchName},
		)
	}

	switch o.InputRef.RefType {
	case types.RefTypeBranch:
		// check branch is clean.
		r, cErr := resolveChangeRecord(ctx, resolveChangeRecordRequest{
			Opctx:     opctx,
			BranchRef: o.InputRef,
			DBClient:  dbClient,
		})
		if cErr != nil {
			return cErr
		}
		if r.ChangeRecord.HasChange() {
			return types.NewVitError(
				types.ErrBranchDirty,
				[]string{"branch '" + o.InputRef.BranchName + "' has uncommitted changes — snapshot it first, then branch"},
				[]any{"branch", o.InputRef.RelativePath()},
			)
		}
		bc, bErr := db.GetBranchCommit(asset.Data, o.InputRef.BranchName)
		if bErr != nil {
			return bErr
		}
		o.targetCommit = bc.Parent

	case types.RefTypeCommit, types.RefTypeTag:
		_, o.targetCommit, err = db.ResolveCommitForDependency(asset.Data, o.InputRef)
		if err != nil {
			return err
		}

	default:
		panic("unreachable: ref type is branch, commit or tag")
	}

	commit, err := db.GetCommit(asset.Data, o.targetCommit)
	if err != nil {
		return err
	}
	o.contentNode, err = blob.LoadContentTree(o.InputRef.RepoPath, o.InputRef.AssetPath, commit.Content)
	return err
}

func (o *operationBranch) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	return blob.PlanBranchFromContentNode(
		o.InputRef.RepoPath, o.InputRef.AssetPath, o.BranchName, o.contentNode,
	)
}

func (o *operationBranch) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.InputRef, true)
	if err != nil {
		return err
	}
	commit, err := db.GetCommit(asset.Data, o.targetCommit)
	if err != nil {
		return err
	}
	if err := db.AddBranch(
		asset.Data, o.targetCommit, o.BranchName, opctx.User, commit.Content, time.Now(),
	); err != nil {
		return err
	}
	o.outputRef = types.NewRefBranch(o.InputRef.RepoRef(), o.InputRef.AssetPath, o.BranchName)
	return nil
}

func (o *operationBranch) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	blobPathBranch, err := blob.BranchContentPath(o.InputRef.RepoPath, o.InputRef.AssetPath, o.BranchName)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "branch: blob path failed", "err", err)
		return
	}

	o.BranchWorkspacePath, err = workspace.PresentBranch(
		ctx, opctx.JSONPool, o.InputRef.RepoPath, blobPathBranch, p, o.outputRef,
	)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "branch: workspace present failed", "err", err)
		return
	}

	// Present the branch's inherited sources (deep-copied from the root commit).
	g, err := db.GetDependencies(ctx, opctx, dbClient, o.outputRef)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "branch: dep graph read failed", "err", err)
		return
	}
	if len(g.Edges) > 0 {
		if err := workspace.PresentSources(ctx, g, o.outputRef, p); err != nil {
			logger.Log(ctx, slog.LevelError, "branch: present sources failed", "err", err)
		}
	}
}
