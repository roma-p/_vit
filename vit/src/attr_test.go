package vit

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"vit/internal/types"
)

func TestAttrSetAndLs(t *testing.T) {
	env := newLocksEnv(t)
	env.add("seq20/sh010")

	env.must(env.client.AttrSet(ctx(), env.abs("seq20/sh010"), map[string]string{"task": "mod", "dcc": "maya"}))

	if got := attrPairs(t, env, env.abs("seq20/sh010")); !slices.Equal(got, []string{"dcc=maya", "task=mod"}) {
		t.Fatalf("attrs = %v", got)
	}
}

func TestAttrRefusesAPathThatIsNotAnAsset(t *testing.T) {
	env := newLocksEnv(t)
	env.add("seq20/sh010")
	asset := env.abs("seq20/sh010")

	cases := []struct {
		name, path, hint string
	}{
		{"directory", env.abs("seq20"), "is a directory"},
		{"repo root", env.repoPath, "no asset at"},
		{"missing", env.abs("seq30"), "no asset at"},
		{"ref", env.branch("seq20/sh010"), "give the asset itself: " + asset},
		{"workspace", env.workspaces["seq20/sh010"], "give the asset itself: " + asset},
		{"file in workspace", filepath.Join(env.workspaces["seq20/sh010"], "f.txt"), "give the asset itself: " + asset},
	}
	for _, c := range cases {
		for op, err := range map[string]error{
			"set": second(env.client.AttrSet(ctx(), c.path, map[string]string{"task": "mod"})),
			"ls":  second(env.client.AttrLs(ctx(), c.path)),
		} {
			if !types.IsErrCode(err, types.ErrDBTreeNode) || !strings.Contains(err.Error(), c.hint) {
				t.Errorf("%s %s: want %s with %q, got %v", op, c.name, types.ErrDBTreeNode, c.hint, err)
			}
		}
	}
}

// -- FILE LOCAL -------------------------------------------------------------

func attrPairs(t *testing.T, env *locksEnv, path string) []string {
	t.Helper()
	res, err := env.client.AttrLs(ctx(), path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, a := range res.Attrs {
		out = append(out, a.Key+"="+a.Value)
	}
	return out
}

func second[T any](_ T, err error) error { return err }
