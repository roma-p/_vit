package workspace

import (
	"context"
	"fmt"
	"os"

	"vit/internal/fsutil"
	"vit/internal/types"
)

// newErrWorkspaceInternal wraps a raw presentation failure (bare os / fsutil
// errors from the presenter or producers) as an internal VitError, so the API
// layer never returns a raw error. The recursive producers and the presenter
// keep returning raw errors; this is the single wrap point at the package's
// public boundary.
func newErrWorkspaceInternal(nested error, msg string) error {
	return types.NewInternalVitError(
		types.ErrWorkspaceInternal,
		nested,
		[]string{msg},
		nil,
	)
}

func ResolvePresenter(
	ctx context.Context,
	logger interface {
		Debug(msg string, args ...any)
		Info(msg string, args ...any)
	},
) (Presenter, error) {
	p, err := newHardlinkPresenter(logger)
	if err != nil {
		return nil, newErrWorkspaceInternal(err, "no usable workspace presenter (hardlinks may be unsupported here)")
	}
	return p, nil
}

func PresentBranch(
	ctx context.Context,
	repoPath, blobPath string,
	p Presenter,
	ref *types.Ref,
) (string, error) {
	ref.AssertTypeOfRef(types.RefTypeBranch)
	wsPath := fsutil.PathOS(repoPath, refToWorkspacePath(ref))
	// blobPath is the branch content ref, relative to .vit/blob (portable, as
	// stored in the asset JSON) — resolve it to the real on-disk blob dir.
	blobAbs := fsutil.PathOS(repoPath, ".vit", "blob", blobPath)
	if err := presentFromBlobDir(ctx, wsPath, blobAbs, false, p); err != nil {
		return "", newErrWorkspaceInternal(err, fmt.Sprintf("failed to present branch to workspace: %s", ref.RelativePath()))
	}
	return wsPath, nil
}

func PresentCommit(
	ctx context.Context,
	repoPath, assetPath string,
	commitDate string,
	p Presenter,
	rootContentNode *types.ContentNode,
) error {
	if err := presentFromContentNode(
		ctx,
		rootContentNode,
		repoPath,
		fsutil.PathOS(repoPath, genWorkspacePathForCommit(assetPath, commitDate)),
		true,
		p,
	); err != nil {
		return newErrWorkspaceInternal(err, fmt.Sprintf("failed to present commit to workspace: %s", assetPath))
	}
	return nil
}

func PresentTag(
	ctx context.Context,
	repoPath string,
	ref *types.Ref,
	p Presenter,
	rootContentNode *types.ContentNode,
) error {
	ref.AssertTypeOfRef(types.RefTypeTag)
	if err := presentFromContentNode(
		ctx,
		rootContentNode,
		repoPath,
		fsutil.PathOS(repoPath, refToWorkspacePath(ref)),
		true,
		p,
	); err != nil {
		return newErrWorkspaceInternal(err, fmt.Sprintf("failed to present tag to workspace: %s", ref.RelativePath()))
	}
	return nil
}

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

	return p.PresentFinish(ctx, wsPath, readOnly)
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

	return p.PresentFinish(ctx, wsPath, readOnly)
}
