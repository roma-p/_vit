package vit

import (
	"context"
	"sort"
	"strings"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
	"vit/internal/workspace"
)

// Ls lists a tree path or an object. Read-only (no transaction), rich result:
//   - a directory      → its children (dirs + assets)
//   - an asset         → its branches + tag families (with the latest version)
//   - an object (a ref)→ author/size/date...
//
// Accepts a workspace path, a tree path, or a ref string (`…@branch=base`).
func (c *Client) Ls(ctx context.Context, path string) (*types.LsResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("Ls", "path", path)

	var out *types.LsResult
	err := opcontext.WithOperationContext(ctx, path, func(opctx *opcontext.OperationContext, p string) error {
		dbClient, err := db.NewClient(ctx, opctx, c.logger)
		if err != nil {
			return err
		}
		defer dbClient.Close()

		if strings.Contains(p, "@") {
			out, err = lsObject(ctx, opctx, dbClient, p)
			return err
		}

		idx, err := dbClient.GetTreeIndex(ctx, opctx, opctx.RepoPath())
		if err != nil {
			return err
		}

		pInt := fsutil.PathInternal(p)
		if pInt == "" || pInt == "." {
			out = lsDir(idx, ".")
			return nil
		}
		if entry, ok := idx.PathToID[pInt]; ok {
			switch entry.Type {
			case db.TreeNodeTypeAsset:
				out, err = lsAsset(ctx, opctx, dbClient, pInt)
				return err
			case db.TreeNodeTypeDir:
				out = lsDir(idx, pInt)
				return nil
			}
		}
		out, err = lsObject(ctx, opctx, dbClient, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// -- FILE LOCAL -------------------------------------------------------------

func lsDir(idx *db.TreeIndex, dirPath string) *types.LsResult {
	var children []types.LsChild
	for p, entry := range idx.PathToID {
		if p == "" || p == "." {
			continue
		}
		if fsutil.DirInternal(p) != dirPath {
			continue
		}
		children = append(children, types.LsChild{
			Name: fsutil.BaseInternal(p),
			Type: string(entry.Type),
		})
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	return &types.LsResult{Kind: "dir", Path: dirPath, Children: children}
}

func lsAsset(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	assetPath string,
) (*types.LsResult, error) {
	repo := opctx.Repo()
	ref := &types.Ref{RepoUID: repo.UID, RepoPath: repo.Path, Site: repo.Site, AssetPath: assetPath}
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, ref, false)
	if err != nil {
		return nil, err
	}

	branches := make([]string, 0, len(asset.Data.Branches))
	for b := range asset.Data.Branches {
		branches = append(branches, b)
	}
	sort.Strings(branches)

	fams := make([]types.LsTagFamily, 0, len(asset.Data.Tags))
	for family, bySite := range asset.Data.Tags {
		latest := ""
		for _, byName := range bySite {
			for name := range byName {
				if string(name) > latest {
					latest = string(name)
				}
			}
		}
		fams = append(fams, types.LsTagFamily{Family: string(family), Latest: latest})
	}
	sort.Slice(fams, func(i, j int) bool { return fams[i].Family < fams[j].Family })

	return &types.LsResult{
		Kind:        "asset",
		Path:        assetPath,
		Branches:    branches,
		TagFamilies: fams,
	}, nil
}

func lsObject(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	pathOrRef string,
) (*types.LsResult, error) {
	ref, err := workspace.ResolveRefFromPath(ctx, opctx, pathOrRef)
	if err != nil {
		return nil, err
	}
	asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, ref, false)
	if err != nil {
		return nil, err
	}
	_, commitID, err := db.ResolveCommitForDependency(asset.Data, ref)
	if err != nil {
		return nil, err
	}
	commit, err := db.GetCommit(asset.Data, commitID)
	if err != nil {
		return nil, err
	}

	if ref.RefType == types.RefTypeCommit {
		if ref.CommitID == "" {
			ref.CommitID = commitID
		}
		if commit.IsSnapshot {
			ref.SnapshotKey = commit.SnapshotKey(commitID)
		}
	}

	size, err := contentSize(ref.RepoPath, ref.AssetPath, commit.Content)
	if err != nil {
		return nil, err
	}

	g, _ := db.GetDependencies(ctx, opctx, dbClient, ref)

	wsPath, _, _ := workspace.ResolveWorkspacePathFromRef(ref.RepoPath, ref)

	return &types.LsResult{
		Kind: "object",
		Path: ref.SnapshotPath(),
		Object: &types.LsObject{
			Ref:           ref,
			WorkspacePath: wsPath,
			Author:        commit.Author,
			Timestamp:     commit.Timestamp,
			Size:          size,
			Sources:       g,
		},
	}, nil
}

func contentSize(repoPath, assetPath, contentHash string) (int64, error) {
	node, err := blob.LoadContentTree(repoPath, assetPath, contentHash)
	if err != nil {
		return 0, err
	}
	return sumContentSize(node), nil
}

func sumContentSize(n *types.ContentNode) int64 {
	var s int64
	for _, f := range n.Files {
		s += f.Size
	}
	for _, c := range n.Children {
		s += sumContentSize(c)
	}
	return s
}
