package vit

import (
	"context"
	"sort"
	"strings"
	"time"

	"vit/internal/db"
	"vit/internal/opcontext"
	"vit/internal/types"
)

// FuzzyCandidates produces candidates for cli fuzzy finder
//   - query has no '@'       → all TREE PATHS (dirs + assets); Enter picks one.
//   - query contains an '@'  → the HIGHLIGHTED item's asset objects: tags / branches / snapshot.
func (c *Client) FuzzyCandidates(ctx context.Context, path, query, selected string) (*types.StringListResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}

	var out []string
	err := opcontext.WithOperationContext(ctx, path, func(opctx *opcontext.OperationContext, _ string) error {
		dbClient, err := db.NewClient(ctx, opctx, c.logger)
		if err != nil {
			return err
		}
		defer dbClient.Close()

		if strings.Contains(query, "@") {
			assetPath, _, _ := strings.Cut(selected, "@")
			assetPath = strings.TrimSpace(assetPath)
			if assetPath == "" {
				return nil
			}
			repo := opctx.Repo()
			ref := &types.Ref{RepoUID: repo.UID, RepoPath: repo.Path, Site: repo.Site, AssetPath: assetPath}
			asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, ref, false)
			if err != nil {
				return nil // unknown asset → no candidates
			}
			out = assetRefCandidates(assetPath, asset.Data)
			return nil
		}

		idx, err := dbClient.GetTreeIndex(ctx, opctx, opctx.RepoPath())
		if err != nil {
			return err
		}
		for p, entry := range idx.PathToID {
			if p == "" || p == "." {
				continue
			}
			if entry.Type == db.TreeNodeTypeDir {
				p += "/"
			}
			out = append(out, p)
		}
		sort.Strings(out)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &types.StringListResult{StringList: out}, nil
}

func assetRefCandidates(assetPath string, a *types.Asset) []string {
	var out []string

	branches := make([]string, 0, len(a.Branches))
	for b := range a.Branches {
		branches = append(branches, b)
	}
	sort.Strings(branches)
	for _, b := range branches {
		out = append(out, assetPath+"@branch="+b)
	}

	fams := make([]string, 0, len(a.Tags))
	for f := range a.Tags {
		fams = append(fams, string(f))
	}
	sort.Strings(fams)
	for _, f := range fams {
		out = append(out, assetPath+"@tag="+f) // floating family
		names := map[string]bool{}
		for _, byName := range a.Tags[types.TagFamily(f)] {
			for n := range byName {
				names[string(n)] = true
			}
		}
		sorted := make([]string, 0, len(names))
		for n := range names {
			sorted = append(sorted, n)
		}
		sort.Strings(sorted)
		for _, n := range sorted {
			out = append(out, assetPath+"@tag="+f+"@name="+n)
		}
	}

	type snap struct {
		key string
		ts  time.Time
	}
	snaps := make([]snap, 0)
	for id, c := range a.Commits {
		if c.IsSnapshot {
			snaps = append(snaps, snap{c.SnapshotKey(id), c.Timestamp})
		}
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].ts.After(snaps[j].ts) })
	for _, s := range snaps {
		out = append(out, assetPath+"@snapshot="+s.key)
	}

	return out
}
