package vit

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"vit/internal/db"
	"vit/internal/opcontext"
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

func TestAttrSetKeepsThePreviousValue(t *testing.T) {
	env := newLocksEnv(t)
	env.add("seq20/sh010")

	env.must(env.client.AttrSet(ctx(), env.abs("seq20/sh010"), map[string]string{"task": "mdo"}))
	res, err := env.client.AttrSet(ctx(), env.abs("seq20/sh010"), map[string]string{"task": "mod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Attrs) != 1 || res.Attrs[0].Value != "mod" || res.Attrs[0].User == "" || res.Attrs[0].TS.IsZero() {
		t.Fatalf("result = %+v", res.Attrs)
	}

	a := readTreeNode(t, env, "seq20/sh010").Attr["task"]
	if a.Value != "mod" || len(a.History) != 1 || a.History[0].Value != "mdo" {
		t.Fatalf("task = %+v", a)
	}
}

func TestAttrRm(t *testing.T) {
	env := newLocksEnv(t)
	env.add("seq20/sh010")
	path := env.abs("seq20/sh010")
	env.must(env.client.AttrSet(ctx(), path, map[string]string{"task": "mod", "dcc": "maya"}))

	env.must(env.client.AttrRm(ctx(), path, []string{"task"}))

	if got := attrPairs(t, env, path); !slices.Equal(got, []string{"dcc=maya"}) {
		t.Fatalf("attrs = %v", got)
	}
	if a := readTreeNode(t, env, "seq20/sh010").Attr["task"]; !a.Removed || len(a.History) != 1 {
		t.Fatalf("task = %+v, want a tombstone", a)
	}
	for _, key := range []string{"task", "variant"} {
		if _, err := env.client.AttrRm(ctx(), path, []string{key}); !types.IsErrCode(err, types.ErrAttr) {
			t.Fatalf("rm %s: want %s, got %v", key, types.ErrAttr, err)
		}
	}
}

func TestAttrRejectsBadInput(t *testing.T) {
	env := newLocksEnv(t)
	env.add("seq20/sh010")
	path := env.abs("seq20/sh010")

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"invalid key", second(env.client.AttrSet(ctx(), path, map[string]string{"ta sk": "mod"})), types.ErrAttr},
		{"nothing to set", second(env.client.AttrSet(ctx(), path, nil)), types.ErrAttr},
		{"nothing to remove", second(env.client.AttrRm(ctx(), path, nil)), types.ErrAttr},
	}
	for _, c := range cases {
		if !types.IsErrCode(c.err, c.want) {
			t.Errorf("%s: want %s, got %v", c.name, c.want, c.err)
		}
	}
	if got := attrPairs(t, env, path); len(got) != 0 {
		t.Fatalf("a rejected call wrote attrs: %v", got)
	}
}

func TestAttrSetDoesNotMarkTheTreeStale(t *testing.T) {
	env := newLocksEnv(t)
	env.add("seq20/sh010")
	env.must(env.client.AttrLs(ctx(), env.abs("seq20/sh010")))
	stale := filepath.Join(env.repoPath, ".vit", "cache", ".tree_stale")
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("tree stale before the set: %v", err)
	}

	env.must(env.client.AttrSet(ctx(), env.abs("seq20/sh010"), map[string]string{"task": "mod"}))

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("attr set marked the tree stale: %v", err)
	}
}

func TestAttrSetIsRecordedInTheLog(t *testing.T) {
	env := newLocksEnv(t)
	env.add("seq20/sh010")
	env.must(env.client.AttrSet(ctx(), env.abs("seq20/sh010"), map[string]string{"task": "mod"}))
	env.must(env.client.AttrRm(ctx(), env.abs("seq20/sh010"), []string{"task"}))

	res, err := env.client.Log(ctx(), env.repoPath)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range res.Entries {
		if e.AssetPath == "seq20/sh010" && (e.Command == types.CommandAttrSet || e.Command == types.CommandAttrRm) {
			got = append(got, string(e.Command)+" "+e.Details)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"attr-rm -task", "attr-set task=mod"}) {
		t.Fatalf("log = %v", got)
	}
}

func TestAttrHistoryCarriesTheChanges(t *testing.T) {
	env := newLocksEnv(t)
	env.add("seq20/sh010")
	path := env.abs("seq20/sh010")
	env.must(env.client.AttrSet(ctx(), path, map[string]string{"task": "mod", "dcc": "maya"}))
	env.must(env.client.AttrRm(ctx(), path, []string{"task"}))
	nodeID := readTreeNode(t, env, "seq20/sh010").ID

	res, err := env.client.Log(ctx(), env.repoPath)
	if err != nil {
		t.Fatal(err)
	}
	want := map[types.VitCommand][]types.AttrChange{
		types.CommandAttrSet: {{Key: "dcc", Value: "maya"}, {Key: "task", Value: "mod"}},
		types.CommandAttrRm:  {{Key: "task", Removed: true}},
	}
	for _, e := range res.Entries {
		changes, ok := want[e.Command]
		if !ok {
			if e.NodeID != "" || e.Attrs != nil {
				t.Errorf("%s carries attr changes: %s %+v", e.Command, e.NodeID, e.Attrs)
			}
			continue
		}
		if e.NodeID != nodeID || !slices.Equal(e.Attrs, changes) {
			t.Errorf("%s: node %q attrs %+v, want %q %+v", e.Command, e.NodeID, e.Attrs, nodeID, changes)
		}
		delete(want, e.Command)
	}
	if len(want) != 0 {
		t.Fatalf("missing entries: %v", want)
	}
}

func TestAttrSetInvalidatesTheAttrIndexWhenTheLogFails(t *testing.T) {
	env := newLocksEnv(t)
	env.add("seq20/sh010")
	build := filepath.Join(env.repoPath, ".vit", "cache", "attrindex", "_build.json")
	if err := os.MkdirAll(filepath.Dir(build), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(build, []byte(`{"cursor":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(env.repoPath, ".vit", "history")
	if err := os.RemoveAll(logDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logDir, nil, 0o444); err != nil {
		t.Fatal(err)
	}

	env.must(env.client.AttrSet(ctx(), env.abs("seq20/sh010"), map[string]string{"task": "mod"}))

	if _, err := os.Stat(build); !os.IsNotExist(err) {
		t.Fatalf("attr index still built after a failed log: %v", err)
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

func readTreeNode(t *testing.T, env *locksEnv, treePath string) *types.TreeNode {
	t.Helper()
	var node *types.TreeNode
	withCommitContext(t, env.client, env.branch(treePath), func(ctx context.Context, opctx *opcontext.OperationContext, dbClient *db.Client, _ *types.Ref) error {
		var err error
		node, err = dbClient.GetTreeNode(ctx, opctx, opctx.RepoPath(), treePath)
		return err
	})
	return node
}

func second[T any](_ T, err error) error { return err }
