package workspace

import (
	"context"
	"fmt"

	"vit/internal/fsutil"
	"vit/internal/types"
)

func PresentSources(
	ctx context.Context,
	g *types.DepGraph,
	root *types.Ref,
	p Presenter,
) error {
	repoPath := root.RepoPath
	rel, ok, err := ResolveWorkspacePathFromRef(repoPath, root)
	if err != nil {
		return err
	}
	if !ok {
		return newErrWorkspaceInternal(
			fmt.Errorf("consumer not presented: %s", root.RelativePath()),
			"present sources: consumer has no pointer",
		)
	}
	return aliasSources(ctx, fsutil.PathOS(repoPath, rel), g, root, p)
}

func PresentProduct(
	ctx context.Context,
	origin *types.Ref,
	name string,
	target *types.Ref,
	p Presenter,
) error {
	originRel, ok, err := ResolveWorkspacePathFromRef(origin.RepoPath, origin)
	if err != nil {
		return err
	}
	if !ok {
		return newErrWorkspaceInternal(
			fmt.Errorf("origin not presented: %s", origin.RelativePath()),
			"present product: origin has no pointer",
		)
	}
	productWorkspacePath := fsutil.PathOS(origin.RepoPath, originRel, name)

	targetRepo := refRepoPath(target, origin.RepoPath)
	targetRel, ok, err := ResolveWorkspacePathFromRef(targetRepo, target)
	if err != nil {
		return err
	}
	if !ok {
		return newErrWorkspaceInternal(
			fmt.Errorf("product target not presented: %s", target.RelativePath()),
			"present product: target branch has no presentation",
		)
	}

	if err := p.Unpresent(productWorkspacePath); err != nil {
		return err
	}
	return p.Alias(ctx, fsutil.PathOS(targetRepo, targetRel), productWorkspacePath)
}

func sameObject(a, b *types.Ref) bool {
	return a.RepoUID == b.RepoUID && a.AssetPath == b.AssetPath && a.ObjectPath() == b.ObjectPath()
}

func refRepoPath(ref *types.Ref, fallback string) string {
	if ref.RepoPath != "" {
		return ref.RepoPath
	}
	return fallback
}

// -- FILE LOCAL -------------------------------------------------------------

func aliasSources(
	ctx context.Context,
	currentWorkspacePath string,
	g *types.DepGraph,
	current *types.Ref,
	p Presenter,
) error {
	for i := range g.Edges {
		e := g.Edges[i]
		if e.Type != types.AssetDependencySource || !sameObject(e.Consumer, current) {
			continue
		}

		target := e.Provider
		sourceWorkspacePath := fsutil.PathOS(currentWorkspacePath, e.Name)

		if err := p.Unpresent(sourceWorkspacePath); err != nil {
			return err
		}

		targetRepo := refRepoPath(target, current.RepoPath)
		rel, ok, err := ResolveWorkspacePathFromRef(targetRepo, target)
		if err != nil {
			return err
		}
		if !ok {
			// TODO: later.
			return newErrWorkspaceInternal(
				fmt.Errorf("source provider not presented: %s", target.RelativePath()),
				"present sources: cannot alias a provider that has no presentation",
			)
		}
		if err := p.Alias(ctx, fsutil.PathOS(targetRepo, rel), sourceWorkspacePath); err != nil {
			return err
		}
	}
	return nil
}
