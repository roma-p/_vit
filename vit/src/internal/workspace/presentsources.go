package workspace

import (
	"context"
	"os"

	"vit/internal/blob"
	"vit/internal/fsutil"
	"vit/internal/types"
)

// PresentSources materializes an object's dependency closure into its workspace reccursively
// Sources are made readonly. The consumer's workspace path is derived from root.
func PresentSources(
	ctx context.Context,
	g *types.DepGraph,
	root *types.Ref,
	p Presenter,
) error {
	repoPath := root.RepoPath
	consumerWorkspacePath := fsutil.PathOS(repoPath, RefToWorkspacePath(root))

	if err := presentSourcesRecc(ctx, repoPath, consumerWorkspacePath, g, root, p); err != nil {
		return err
	}
	for i := range g.Edges {
		e := g.Edges[i]
		if e.Type != types.AssetDependencySource || !sameObject(e.Consumer, root) {
			continue
		}
		if err := p.Seal(ctx, fsutil.PathOS(consumerWorkspacePath, e.Name)); err != nil {
			return err
		}
	}
	return nil
}

func sameObject(a, b *types.Ref) bool {
	return a.RepoUID == b.RepoUID && a.AssetPath == b.AssetPath && a.ObjectPath() == b.ObjectPath()
}

// -- FILE LOCAL -------------------------------------------------------------

func presentSourcesRecc(
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
		node, ok := lookupNode(g, target)
		if !ok {
			continue
		}

		targetRepo := target.RepoPath
		if targetRepo == "" {
			targetRepo = repoPath // local fallback
		}
		contentNode, err := blob.LoadContentTree(targetRepo, target.AssetPath, node.PayloadFile)
		if err != nil {
			return err
		}

		sourceWorkspacePath := fsutil.PathOS(currentWorkspacePath, e.Name)
		if _, err := os.Lstat(sourceWorkspacePath); err == nil {
			_ = makeTreeWritable(sourceWorkspacePath)
			_ = os.RemoveAll(sourceWorkspacePath)
		}

		if err := presentFromContentNode(
			ctx, contentNode, targetRepo, sourceWorkspacePath, true, p,
		); err != nil {
			return err
		}

		if err := presentSourcesRecc(
			ctx, repoPath, sourceWorkspacePath, g, target, p,
		); err != nil {
			return err
		}
	}
	return nil
}

func makeTreeWritable(dir string) error {
	if err := fsutil.MakeDirectoryWritable(dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := makeTreeWritable(fsutil.PathOS(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func lookupNode(g *types.DepGraph, ref *types.Ref) (types.DepGraphNode, bool) {
	byAsset, ok := g.Nodes[ref.RepoUID]
	if !ok {
		return types.DepGraphNode{}, false
	}
	byObject, ok := byAsset[ref.AssetPath]
	if !ok {
		return types.DepGraphNode{}, false
	}
	n, ok := byObject[ref.ObjectPath()]
	return n, ok
}
