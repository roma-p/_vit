package db

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	"vit/internal/types"
)

func TestQueryFilters(t *testing.T) {
	cases := []struct {
		name string
		req  types.QueryRequest
		want []string
	}{
		{"eq", types.QueryRequest{Eq: map[string]string{"seq": "20"}}, []string{"s20/sh010/lgt", "s20/sh010/mod", "s20/sh020/mod"}},
		{"two eq", types.QueryRequest{Eq: map[string]string{"seq": "20", "task": "mod"}}, []string{"s20/sh010/mod", "s20/sh020/mod"}},
		{"unknown value", types.QueryRequest{Eq: map[string]string{"seq": "99"}}, nil},
		{"with", types.QueryRequest{With: []string{"shot"}}, []string{"s20/sh010/lgt", "s20/sh010/mod", "s20/sh020/mod", "s30/sh010/mod"}},
		{"with without", types.QueryRequest{With: []string{"task"}, Without: []string{"shot"}}, []string{"s30/env"}},
		{"without only", types.QueryRequest{Without: []string{"task"}}, []string{"lib/tree"}},
		{"under", types.QueryRequest{Under: "s20/sh010"}, []string{"s20/sh010/lgt", "s20/sh010/mod"}},
		{"under slash", types.QueryRequest{With: []string{"task"}, Under: "s30/"}, []string{"s30/env", "s30/sh010/mod"}},
		{"under is not a prefix", types.QueryRequest{With: []string{"task"}, Under: "s2"}, nil},
		{"under an asset", types.QueryRequest{Under: "s30/env"}, []string{"s30/env"}},
	}
	for _, c := range cases {
		c.req.Asset = true
		var got []string
		for _, r := range evaluateQuery(c.req, queryShards(), queryTree()).Rows {
			got = append(got, r.Asset)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestQueryFetch(t *testing.T) {
	req := types.QueryRequest{Eq: map[string]string{"shot": "010"}, Fetch: []string{"seq", "task"}}

	got := evaluateQuery(req, queryShards(), queryTree()).Rows
	want := []types.QueryRow{
		{Attrs: map[string]string{"seq": "20", "task": "lgt"}},
		{Attrs: map[string]string{"seq": "20", "task": "mod"}},
		{Attrs: map[string]string{"seq": "30", "task": "mod"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}

	req.Asset = true
	if got := evaluateQuery(req, queryShards(), queryTree()).Rows; got[2].Asset != "s30/sh010/mod" || got[2].Attrs["task"] != "mod" {
		t.Fatalf("with asset: %+v", got)
	}
}

func TestQueryDistinct(t *testing.T) {
	req := types.QueryRequest{With: []string{"task"}, Fetch: []string{"task", "seq"}, Distinct: true}

	got := evaluateQuery(req, queryShards(), queryTree()).Rows
	want := []types.QueryRow{
		{Attrs: map[string]string{"task": "lgt", "seq": "20"}},
		{Attrs: map[string]string{"task": "mod", "seq": "20"}},
		{Attrs: map[string]string{"task": "mod", "seq": "30"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}

	req.Fetch = []string{"task"}
	got = evaluateQuery(req, queryShards(), queryTree()).Rows
	want = []types.QueryRow{{Attrs: map[string]string{"task": "lay"}}, {Attrs: map[string]string{"task": "lgt"}}, {Attrs: map[string]string{"task": "mod"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("one key: got %+v", got)
	}
}

func TestQuerySet(t *testing.T) {
	req := types.QueryRequest{Eq: map[string]string{"seq": "20"}, Fetch: []string{"shot", "task"}, Set: true}

	got := evaluateQuery(req, queryShards(), queryTree())
	want := map[string][]string{"shot": {"010", "020"}, "task": {"lgt", "mod"}}
	if got.Rows != nil || !maps.EqualFunc(got.Sets, want, slices.Equal) {
		t.Fatalf("got %+v", got)
	}
}

// -- FILE LOCAL -------------------------------------------------------------

func queryTree() *TreeIndex {
	tree := NewTreeIndex()
	tree.AddPath("s20", "d1", types.TreeNodeTypeDir)
	for path, uid := range map[string]string{
		"s20/sh010/mod": "u4", "s20/sh010/lgt": "u3", "s20/sh020/mod": "u2",
		"s30/sh010/mod": "u1", "s30/env": "u5", "lib/tree": "u6",
	} {
		tree.AddPath(path, uid, types.TreeNodeTypeAsset)
	}
	return tree
}

func queryShards() map[string]*attrShard {
	shard := func(values map[string][]string) *attrShard { return &attrShard{Values: values} }
	return map[string]*attrShard{
		"seq":  shard(map[string][]string{"20": {"ghost", "u2", "u3", "u4"}, "30": {"u1"}}),
		"shot": shard(map[string][]string{"010": {"u1", "u3", "u4"}, "020": {"u2"}}),
		"task": shard(map[string][]string{"lay": {"u5"}, "lgt": {"u3"}, "mod": {"u1", "u2", "u4"}}),
	}
}
