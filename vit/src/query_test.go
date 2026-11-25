package vit

import (
	"reflect"
	"slices"
	"testing"

	"vit/internal/types"
)

func TestQueryFetchesTheAttributes(t *testing.T) {
	env := newQueryEnv(t)

	res, err := env.client.Query(ctx(), env.repoPath, types.QueryRequest{Eq: map[string]string{"seq": "20"}, Fetch: []string{"task"}, Asset: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []types.QueryRow{
		{Asset: "s20/sh010/lgt", Attrs: map[string]string{"task": "lgt"}},
		{Asset: "s20/sh010/mod", Attrs: map[string]string{"task": "mod"}},
		{Asset: "s20/sh020/mod", Attrs: map[string]string{"task": "mod"}},
	}
	if !reflect.DeepEqual(res.Rows, want) {
		t.Fatalf("rows = %+v", res.Rows)
	}
}

func TestQuerySeesTheChangesAfterTheFirstQuery(t *testing.T) {
	env := newQueryEnv(t)
	queryAssets(t, env, env.repoPath, types.QueryRequest{With: []string{"task"}})

	env.must(env.client.AttrSet(ctx(), env.abs("s20/sh010/lgt"), map[string]string{"task": "mod"}))
	env.must(env.client.AttrRm(ctx(), env.abs("s30/sh010/mod"), []string{"task"}))
	env.add("s40/sh010/mod")
	env.must(env.client.AttrSet(ctx(), env.abs("s40/sh010/mod"), map[string]string{"task": "mod"}))

	got := queryAssets(t, env, env.repoPath, types.QueryRequest{Eq: map[string]string{"task": "mod"}})
	if !slices.Equal(got, []string{"s20/sh010/lgt", "s20/sh010/mod", "s20/sh020/mod", "s40/sh010/mod"}) {
		t.Fatalf("assets = %v", got)
	}
}

func TestQueryIsScopedByThePath(t *testing.T) {
	env := newQueryEnv(t)
	req := types.QueryRequest{With: []string{"task"}}

	cases := map[string][]string{
		"s20/sh010": {"s20/sh010/lgt", "s20/sh010/mod"},
		"s30":       {"s30/sh010/mod"},
		"s2":        nil,
	}
	for path, want := range cases {
		if got := queryAssets(t, env, env.abs(path), req); !slices.Equal(got, want) {
			t.Errorf("%s: assets = %v, want %v", path, got, want)
		}
	}
}

func TestQueryRejectsBadRequests(t *testing.T) {
	env := newQueryEnv(t)
	fetch := []string{"task"}

	cases := map[string]struct {
		req  types.QueryRequest
		code string
	}{
		"nothing to return": {types.QueryRequest{With: fetch}, types.ErrQuery},
		"distinct and set":  {types.QueryRequest{Fetch: fetch, Distinct: true, Set: true}, types.ErrQuery},
		"distinct asset":    {types.QueryRequest{Fetch: fetch, Distinct: true, Asset: true}, types.ErrQuery},
		"set asset":         {types.QueryRequest{Fetch: fetch, Set: true, Asset: true}, types.ErrQuery},
		"set without fetch": {types.QueryRequest{With: fetch, Set: true}, types.ErrQuery},
		"bad key":           {types.QueryRequest{Fetch: []string{"Task"}}, types.ErrAttr},
	}
	for name, c := range cases {
		if _, err := env.client.Query(ctx(), env.repoPath, c.req); !types.IsErrCode(err, c.code) {
			t.Errorf("%s: want %s, got %v", name, c.code, err)
		}
	}
}

// -- FILE LOCAL -------------------------------------------------------------

func newQueryEnv(t *testing.T) *locksEnv {
	t.Helper()
	env := newLocksEnv(t)
	for _, path := range []string{"s20/sh010/mod", "s20/sh010/lgt", "s20/sh020/mod", "s30/sh010/mod"} {
		env.add(path)
		seq, task := path[1:3], path[len(path)-3:]
		env.must(env.client.AttrSet(ctx(), env.abs(path), map[string]string{"seq": seq, "task": task}))
	}
	return env
}

func queryAssets(t *testing.T, env *locksEnv, path string, req types.QueryRequest) []string {
	t.Helper()
	req.Asset = true
	res, err := env.client.Query(ctx(), path, req)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range res.Rows {
		out = append(out, r.Asset)
	}
	return out
}
