package db

import (
	"context"
	"maps"
	"slices"
	"strings"

	"vit/internal/opcontext"
	"vit/internal/types"
)

func (c *Client) GetAttrIndex(repoPath string) *AttrIndex {
	i, ok := c.AttrIndexPool[repoPath]
	if !ok {
		i = NewAttrIndex(repoPath)
		c.AttrIndexPool[repoPath] = i
	}
	return i
}

func (c *Client) Query(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath string,
	req types.QueryRequest,
) (*types.QueryResult, error) {
	keys := slices.Concat(req.With, req.Without, req.Fetch, slices.Collect(maps.Keys(req.Eq)))
	shards, err := c.GetAttrIndex(repoPath).get(ctx, keys)
	if err != nil {
		return nil, err
	}
	tree, err := c.GetTreeIndex(ctx, opctx, repoPath)
	if err != nil {
		return nil, err
	}
	return evaluateQuery(req, shards, tree), nil
}

// -- FILE LOCAL -------------------------------------------------------------

func evaluateQuery(req types.QueryRequest, shards map[string]*attrShard, tree *TreeIndex) *types.QueryResult {
	return &types.QueryResult{Rows: fetchRows(req, tree, matchingUIDs(req, shards, tree))}
}

func matchingUIDs(req types.QueryRequest, shards map[string]*attrShard, tree *TreeIndex) []string {
	var sets [][]string
	for k, v := range req.Eq {
		sets = append(sets, shards[k].Values[v])
	}
	for _, k := range slices.Concat(req.With, req.Fetch) {
		sets = append(sets, shards[k].allUIDs())
	}
	var out []string
	if len(sets) == 0 {
		for uid, e := range tree.IDToPath {
			if e.Type == types.TreeNodeTypeAsset {
				out = append(out, uid)
			}
		}
	} else {
		slices.SortFunc(sets, func(a, b []string) int { return len(a) - len(b) })
		out = slices.Clone(sets[0])
		for _, s := range sets[1:] {
			out = slices.DeleteFunc(out, func(uid string) bool { return !contains(s, uid) })
		}
	}
	var without [][]string
	for _, k := range req.Without {
		without = append(without, shards[k].allUIDs())
	}
	out = slices.DeleteFunc(out, func(uid string) bool {
		e, ok := tree.IDToPath[uid]
		return !ok || !isUnder(e.Data, req.Under) || slices.ContainsFunc(without, func(s []string) bool { return contains(s, uid) })
	})
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(tree.IDToPath[a].Data, tree.IDToPath[b].Data) })
	return out
}

func fetchRows(req types.QueryRequest, tree *TreeIndex, uids []string) []types.QueryRow {
	rows := make([]types.QueryRow, 0, len(uids))
	for _, uid := range uids {
		row := types.QueryRow{}
		if req.Asset {
			row.Asset = tree.IDToPath[uid].Data
		}
		rows = append(rows, row)
	}
	return rows
}

func (s *attrShard) allUIDs() []string {
	var out []string
	for _, uids := range s.Values {
		out = append(out, uids...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func contains(sorted []string, uid string) bool {
	_, found := slices.BinarySearch(sorted, uid)
	return found
}

func isUnder(path, under string) bool {
	under = strings.TrimSuffix(under, "/")
	return under == "" || under == "." || path == under || strings.HasPrefix(path, under+"/")
}
