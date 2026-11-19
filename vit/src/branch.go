package vit

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

// Branch creates one new branch per name, all rooted at the object referenced by
// path (a clean branch, a commit, a tag). Branching from a DIRTY branch is not
// allowed. New branch files are presented with inherited dependencies. Returns one
// workspace path per created branch, in the order of names.
func (c *Client) Branch(
	ctx context.Context,
	path string,
	names []string,
) (*types.StringListResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("Branch", "path", path, "names", names)

	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	var branchWorkspacePaths []string
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

		products, err := productEdgesForRef(ctx, opctx, dbClient, ref, true)
		if err != nil {
			return err
		}

		op := &operationBranch{InputRef: ref, BranchNames: names, productTargets: products}

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
				Command:   types.CommandBranch,
				User:      opctx.User,
				RepoPath:  opctx.RepoPath(),
				Timestamp: opctx.TimeStamp,
				Ref:       ref,
			},
			refs, transaction.TransactionBranch,
		); err != nil {
			return err
		}

		branchWorkspacePaths = op.BranchWorkspacePaths
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &types.StringListResult{StringList: branchWorkspacePaths}, nil
}

type operationBranch struct {
	// input
	InputRef       *types.Ref
	BranchNames    []string
	productTargets []originProduct
	empty          bool

	// filled at Prepare
	prepared        *resolveChangeRecordResult
	preparedContent *types.ContentNode

	// filled at Validate
	targetCommit    string             // the commit every new branch roots at
	contentNode     *types.ContentNode // its content
	productBranches []*operationBranch

	// filled at PlanBlobUpdate (empty branches only): the empty content hash.
	emptyContentHash string

	// filled at DBUpdate, one per BranchNames entry
	outputRefs []*types.Ref

	// filled at Finish, one per successfully presented branch
	BranchWorkspacePaths []string
}

func (o *operationBranch) Prepare(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	if o.empty {
		return nil
	}
	if o.InputRef.RefType == types.RefTypeBranch {
		r, err := resolveChangeRecord(ctx, resolveChangeRecordRequest{
			Opctx:     opctx,
			BranchRef: o.InputRef,
			DBClient:  dbClient,
		})
		if err != nil {
			return err
		}
		o.prepared = r
		o.preparedContent = r.PrevCommitContentNode
		return nil
	}
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.InputRef, false)
	if err != nil {
		return err
	}
	_, target, err := db.ResolveCommitForDependency(asset.Data, o.InputRef)
	if err != nil {
		return err
	}
	commit, err := db.GetCommit(asset.Data, target)
	if err != nil {
		return err
	}
	o.preparedContent, err = blob.LoadContentTree(o.InputRef.RepoPath, o.InputRef.AssetPath, commit.Content)
	return err
}

func (o *operationBranch) Validate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.InputRef, false)
	if err != nil {
		return err
	}

	o.BranchNames, err = checkNewBranchNames(asset.Data.Branches, o.InputRef.AssetPath, o.BranchNames)
	if err != nil {
		return err
	}
	if len(o.BranchNames) == 0 {
		return types.NewVitError(
			types.ErrInvalidInut,
			[]string{"at least one branch name is required"},
			[]any{"ref", o.InputRef.RelativePath()},
		)
	}

	switch o.InputRef.RefType {
	case types.RefTypeBranch:
		r, cErr := resolveChangeRecord(ctx, resolveChangeRecordRequest{
			Opctx:     opctx,
			BranchRef: o.InputRef,
			DBClient:  dbClient,
			Prepared:  o.prepared,
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
	if !o.empty {
		o.contentNode, err = o.loadContent(commit.Content)
		if err != nil {
			return err
		}
	}

	// prepare creating, on every product, the branches it does not have yet.
	for _, pt := range o.productTargets {
		tAsset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, pt.targetRef, false)
		if err != nil {
			return err
		}
		var missing []string
		for _, name := range o.BranchNames {
			if _, exists := tAsset.Data.Branches[name]; !exists {
				missing = append(missing, name)
			}
		}
		if len(missing) == 0 {
			continue
		}
		tBase, err := db.GetBranchCommit(tAsset.Data, pt.targetRef.BranchName)
		if err != nil {
			return err
		}
		sub := &operationBranch{
			InputRef: types.NewRefCommit(
				pt.targetRef.RepoRef(), pt.targetRef.AssetPath, tBase.Parent,
			),
			BranchNames: missing,
			empty:       true,
		}
		if err := sub.Validate(ctx, opctx, dbClient); err != nil {
			return err
		}
		o.productBranches = append(o.productBranches, sub)
		// TODO: what happen if the branch is dirty? should be allow "product add"
		// meaning if we commit origin, product would be committing with artefacts
		// that were not producted by origin.
	}
	return nil
}

func (o *operationBranch) loadContent(hash string) (*types.ContentNode, error) {
	if o.preparedContent != nil && o.preparedContent.ContentHash == hash {
		return o.preparedContent, nil
	}
	return blob.LoadContentTree(o.InputRef.RepoPath, o.InputRef.AssetPath, hash)
}

func (o *operationBranch) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	plan := &types.BlobUpdatePlan{}

	for _, sub := range o.productBranches {
		p, err := sub.PlanBlobUpdate(ctx, opctx)
		if err != nil {
			return nil, err
		}
		appendPlan(plan, p)
	}

	for _, name := range o.BranchNames {
		if o.empty {
			self, node, err := blob.PlanEmptyBranch(o.InputRef.RepoPath, o.InputRef.AssetPath, name)
			if err != nil {
				return nil, err
			}
			o.contentNode = node
			o.emptyContentHash = node.ContentHash
			appendPlan(plan, self)
			continue
		}

		self, err := blob.PlanBranchFromContentNode(
			o.InputRef.RepoPath, o.InputRef.AssetPath, name, o.contentNode,
		)
		if err != nil {
			return nil, err
		}
		appendPlan(plan, self)
	}
	return plan, nil
}

func (o *operationBranch) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	// product sub-branches first, then self.
	for _, sub := range o.productBranches {
		if err := sub.DBUpdate(ctx, opctx, dbClient); err != nil {
			return err
		}
	}

	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, o.InputRef, true)
	if err != nil {
		return err
	}
	commit, err := db.GetCommit(asset.Data, o.targetCommit)
	if err != nil {
		return err
	}

	content := commit.Content
	if o.empty {
		content = o.emptyContentHash
	}

	for _, name := range o.BranchNames {
		if err := db.AddBranch(
			asset.Data, o.targetCommit, name, opctx.User, content, time.Now(),
		); err != nil {
			return err
		}

		// rewire each product dep onto the product's branch of the same name.
		newBranch, err := db.GetBranchCommit(asset.Data, name)
		if err != nil {
			return err
		}
		for _, pt := range o.productTargets {
			dep, ok := newBranch.Dependencies[pt.name]
			if !ok || dep.Type != types.AssetDependencyProduct {
				continue
			}
			dep.RefSpec = types.NewRefBranch(pt.targetRef.RepoRef(), pt.targetRef.AssetPath, name)
			dep.RefResolved = nil
			dep.Commit = ""
		}

		o.outputRefs = append(o.outputRefs,
			types.NewRefBranch(o.InputRef.RepoRef(), o.InputRef.AssetPath, name))
	}
	return nil
}

func (o *operationBranch) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
) {
	for _, sub := range o.productBranches {
		sub.Finish(ctx, opctx, dbClient, logger, p)
	}

	// best-effort per branch: one failed presentation does not skip the others.
	for _, outputRef := range o.outputRefs {
		if ws, ok := o.finishBranch(ctx, opctx, dbClient, logger, p, outputRef); ok {
			o.BranchWorkspacePaths = append(o.BranchWorkspacePaths, ws)
		}
	}
}

func (o *operationBranch) finishBranch(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
	outputRef *types.Ref,
) (string, bool) {
	name := outputRef.BranchName

	blobPathBranch, err := blob.BranchContentPath(o.InputRef.RepoPath, o.InputRef.AssetPath, name)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "branch: blob path failed", "branch", name, "err", err)
		return "", false
	}

	ws, err := workspace.PresentBranch(
		ctx, opctx.JSONPool, o.InputRef.RepoPath, blobPathBranch, p, outputRef,
	)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "branch: workspace present failed", "branch", name, "err", err)
		return "", false
	}

	// Present the branch's inherited sources (deep-copied from the root commit).
	g, err := db.GetDependencies(ctx, opctx, dbClient, outputRef)
	if err != nil {
		logger.Log(ctx, slog.LevelError, "branch: dep graph read failed", "branch", name, "err", err)
		return ws, true
	}
	if len(g.Edges) > 0 {
		if err := workspace.PresentSources(ctx, g, outputRef, p); err != nil {
			logger.Log(ctx, slog.LevelError, "branch: present sources failed", "branch", name, "err", err)
		}
	}

	for _, pt := range o.productTargets {
		targetBranch := types.NewRefBranch(pt.targetRef.RepoRef(), pt.targetRef.AssetPath, name)
		if _, err := db.UpdateDepGraphForDependency(
			ctx, opctx, dbClient,
			db.UpdateDepGraphForDependencyRequest{
				Dependent: outputRef,
				Target:    targetBranch,
				Spec:      targetBranch,
				Name:      pt.name,
				Type:      types.AssetDependencyProduct,
			},
		); err != nil {
			logger.Log(ctx, slog.LevelError, "branch: product dep graph update failed", "branch", name, "err", err)
		}
		if err := workspace.PresentProduct(
			ctx, outputRef, pt.name, targetBranch, p,
		); err != nil {
			logger.Log(ctx, slog.LevelError, "branch: present product failed", "branch", name, "err", err)
		}
	}
	return ws, true
}

func checkNewBranchNames(
	existing map[string]*types.AssetCommit,
	assetPath string,
	names []string,
) ([]string, error) {
	seen := make(map[string]bool, len(names))
	var unique, taken []string
	for _, name := range names {
		if name == "" {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		unique = append(unique, name)
		if _, ok := existing[name]; ok {
			taken = append(taken, name)
		}
	}
	if len(taken) > 0 {
		return nil, types.NewVitError(
			types.ErrAssetObjectAlreadyExists,
			[]string{"branches already exist on this asset: " + strings.Join(taken, ", ")},
			[]any{"asset", assetPath, "branches", taken},
		)
	}
	return unique, nil
}
