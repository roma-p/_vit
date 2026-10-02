package workspace

import (
	"context"
	"fmt"
	"os"

	"vit/internal/fsutil"
	"vit/internal/types"
)

func ResolvePresenter(
	ctx context.Context,
	logger interface {
		Debug(msg string, args ...any)
		Info(msg string, args ...any)
	},
) (Presenter, error) {
	p, err := newHardlinkPresenter(logger)
	if err != nil {
		return nil, newErrWorkspaceInternal(
			err,
			"no usable workspace presenter (hardlinks may be unsupported here)",
		)
	}
	return p, nil
}

func PresentBranch(
	ctx context.Context,
	pool *fsutil.JSONHandlerPool,
	repoPath, blobPath string,
	p Presenter,
	ref *types.Ref,
) (string, error) {
	ref.AssertTypeOfRef(types.RefTypeBranch)
	wsPath := fsutil.PathOS(repoPath, RefToWorkspacePath(ref))
	blobAbs := fsutil.PathOS(repoPath, ".vit", "blob", blobPath)
	if err := presentFromBlobDir(ctx, wsPath, blobAbs, false, p); err != nil {
		return "", newErrWorkspaceInternal(err, fmt.Sprintf("failed to present branch to workspace: %s", ref.RelativePath()))
	}
	if err := writePointer(ctx, pool, repoPath, wsPath, ref); err != nil {
		return "", err
	}
	return wsPath, nil
}

func PresentCommit(
	ctx context.Context,
	pool *fsutil.JSONHandlerPool,
	ref *types.Ref,
	srcBranchName, user, commitDate string,
	p Presenter,
	rootContentNode *types.ContentNode,
	sourceGraph *types.DepGraph,
) (string, error) {
	ref.AssertTypeOfRef(types.RefTypeCommit)
	ret := fsutil.PathOS(
		ref.RepoPath,
		genWorkspacePathForCommit(ref.AssetPath, commitDate, srcBranchName, user))
	if err := presentFromContentNode(
		ctx,
		rootContentNode,
		ref.RepoPath,
		ret,
		true,
		p,
	); err != nil {
		return "", newErrWorkspaceInternal(
			err,
			fmt.Sprintf("failed to present commit to workspace: %s", ref.AssetPath),
		)
	}

	if sourceGraph != nil {
		if err := presentSourcesRecc(ctx, ref.RepoPath, ret, sourceGraph, ref, p); err != nil {
			return "", newErrWorkspaceInternal(
				err,
				fmt.Sprintf("failed to present commit sources: %s", ref.AssetPath),
			)
		}
	}
	if err := p.Seal(ctx, ret); err != nil {
		return "", newErrWorkspaceInternal(
			err,
			fmt.Sprintf("failed to seal commit presentation: %s", ref.AssetPath),
		)
	}
	if err := writePointer(ctx, pool, ref.RepoPath, ret, ref); err != nil {
		return "", err
	}
	return ret, nil
}

func PresentTag(
	ctx context.Context,
	pool *fsutil.JSONHandlerPool,
	repoPath string,
	ref *types.Ref,
	p Presenter,
	rootContentNode *types.ContentNode,
	sourceGraph *types.DepGraph,
) (string, error) {
	ref.AssertTypeOfRef(types.RefTypeTag)
	ret := fsutil.PathOS(repoPath, RefToWorkspacePath(ref))
	if err := presentFromContentNode(
		ctx,
		rootContentNode,
		repoPath,
		ret,
		true,
		p,
	); err != nil {
		return "", newErrWorkspaceInternal(
			err,
			fmt.Sprintf("failed to present tag to workspace: %s", ref.RelativePath()),
		)
	}

	if sourceGraph != nil {
		if err := presentSourcesRecc(ctx, repoPath, ret, sourceGraph, ref, p); err != nil {
			return "", newErrWorkspaceInternal(
				err,
				fmt.Sprintf("failed to present tag sources: %s", ref.RelativePath()),
			)
		}
	}
	if err := p.Seal(ctx, ret); err != nil {
		return "", newErrWorkspaceInternal(
			err,
			fmt.Sprintf("failed to seal tag presentation: %s", ref.RelativePath()),
		)
	}
	if err := writePointer(ctx, pool, repoPath, ret, ref); err != nil {
		return "", err
	}
	return ret, nil
}

// -- FILE LOCAL -------------------------------------------------------------

func presentFromContentNode(
	ctx context.Context,
	node *types.ContentNode,
	repoPath, wsPath string,
	readOnly bool,
	p Presenter,
) error {
	if err := p.Present(ctx, "", wsPath, true, readOnly); err != nil {
		return err
	}

	for _, f := range node.Files {
		if err := p.Present(
			ctx,
			fsutil.PathOS(repoPath, ".vit", "blob", f.BlobTarget),
			fsutil.PathOS(wsPath, f.Name),
			false, readOnly,
		); err != nil {
			return err
		}
	}

	for _, c := range node.Children {
		if err := presentFromContentNode(
			ctx,
			c,
			repoPath,
			fsutil.PathOS(wsPath, c.Name),
			readOnly,
			p,
		); err != nil {
			return err
		}
	}

	return nil
}

func presentFromBlobDir(
	ctx context.Context,
	wsPath, blobPath string,
	readOnly bool,
	p Presenter,
) error {
	if err := p.Present(ctx, "", wsPath, true, readOnly); err != nil {
		return err
	}

	entries, err := os.ReadDir(blobPath)
	if err != nil {
		return err
	}

	for _, e := range entries {
		src := fsutil.PathOS(blobPath, e.Name())
		dst := fsutil.PathOS(wsPath, e.Name())
		if e.IsDir() {
			if err := presentFromBlobDir(ctx, dst, src, readOnly, p); err != nil {
				return err
			}
		} else {
			if err := p.Present(ctx, src, dst, false, readOnly); err != nil {
				return err
			}
		}
	}

	return nil
}

func newErrWorkspaceInternal(nested error, msg string) error {
	return types.NewInternalVitError(
		types.ErrWorkspaceInternal,
		nested,
		[]string{msg},
		nil,
	)
}
