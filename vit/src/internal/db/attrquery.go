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
	repoPath, under string,
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
	return evaluateQuery(req, under, shards, tree), nil
}

// -- FILE LOCAL -------------------------------------------------------------

func evaluateQuery(req types.QueryRequest, under string, shards map[string]*attrShard, tree *TreeIndex) *types.QueryResult {
	uids := matchingUIDs(req, under, shards, tree)
	if req.Set {
		out := &types.QueryResult{Sets: make(map[string][]string, len(req.Fetch))}
		for _, k := range req.Fetch {
			out.Sets[k] = shards[k].valuesOf(uids)
		}
		return out
	}
	rows := fetchRows(req, shards, tree, uids)
	if req.Distinct {
		rows = distinctRows(rows, req.Fetch)
	}
	return &types.QueryResult{Rows: rows}
}

func matchingUIDs(req types.QueryRequest, under string, shards map[string]*attrShard, tree *TreeIndex) []string {
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
		return !ok || !isUnder(e.Data, under) || slices.ContainsFunc(without, func(s []string) bool { return contains(s, uid) })
	})
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(tree.IDToPath[a].Data, tree.IDToPath[b].Data) })
	return out
}

func fetchRows(req types.QueryRequest, shards map[string]*attrShard, tree *TreeIndex, uids []string) []types.QueryRow {
	values := make(map[string]map[string]string, len(req.Fetch))
	for _, k := range req.Fetch {
		values[k] = shards[k].valueByUID()
	}
	rows := make([]types.QueryRow, 0, len(uids))
	for _, uid := range uids {
		row := types.QueryRow{}
		if req.Asset {
			row.Asset = tree.IDToPath[uid].Data
		}
		if len(req.Fetch) > 0 {
			row.Attrs = make(map[string]string, len(req.Fetch))
			for _, k := range req.Fetch {
				row.Attrs[k] = values[k][uid]
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func distinctRows(rows []types.QueryRow, keys []string) []types.QueryRow {
	tuple := func(r types.QueryRow) []string {
		t := make([]string, len(keys))
		for i, k := range keys {
			t[i] = r.Attrs[k]
		}
		return t
	}
	slices.SortStableFunc(rows, func(a, b types.QueryRow) int { return slices.Compare(tuple(a), tuple(b)) })
	return slices.CompactFunc(rows, func(a, b types.QueryRow) bool { return slices.Equal(tuple(a), tuple(b)) })
}

func (s *attrShard) allUIDs() []string {
	var out []string
	for _, uids := range s.Values {
		out = append(out, uids...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (s *attrShard) valueByUID() map[string]string {
	out := map[string]string{}
	for v, uids := range s.Values {
		for _, uid := range uids {
			out[uid] = v
		}
	}
	return out
}

func (s *attrShard) valuesOf(uids []string) []string {
	in := make(map[string]bool, len(uids))
	for _, uid := range uids {
		in[uid] = true
	}
	var out []string
	for v, us := range s.Values {
		if slices.ContainsFunc(us, func(uid string) bool { return in[uid] }) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

func contains(sorted []string, uid string) bool {
	_, found := slices.BinarySearch(sorted, uid)
	return found
}

func isUnder(path, under string) bool {
	under = strings.TrimSuffix(under, "/")
	return under == "" || under == "." || path == under || strings.HasPrefix(path, under+"/")
}
