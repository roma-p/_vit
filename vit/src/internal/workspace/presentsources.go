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
	return aliasSources(ctx, repoPath, fsutil.PathOS(repoPath, rel), g, root, p)
}

func sameObject(a, b *types.Ref) bool {
	return a.RepoUID == b.RepoUID && a.AssetPath == b.AssetPath && a.ObjectPath() == b.ObjectPath()
}

// -- FILE LOCAL -------------------------------------------------------------

func aliasSources(
	ctx context.Context,
	repoPath, currentWorkspacePath string,
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

		rel, ok, err := ResolveWorkspacePathFromRef(repoPath, target)
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
		if err := p.Alias(ctx, fsutil.PathOS(repoPath, rel), sourceWorkspacePath); err != nil {
			return err
		}
	}
	return nil
}
