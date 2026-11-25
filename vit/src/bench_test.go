package vit

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

// VIT_BENCH_REPO=<dir> go test -run '^$' -bench AttrQuery -benchtime 10x .
// keeps the 50k-asset fixture in <dir>/repo (generated once), otherwise a temp dir.

func BenchmarkAttrQuery(b *testing.B) {
	client, repo := benchRepo(b)
	cases := []struct {
		name string
		path string
		req  types.QueryRequest
	}{
		{"shots of a seq", repo, types.QueryRequest{Eq: map[string]string{"seq": "seq20"}, Fetch: []string{"shot"}, Set: true}},
		{"tasks of a shot", repo, types.QueryRequest{Eq: map[string]string{"seq": "seq20", "shot": "sh010"}, Fetch: []string{"task"}, Set: true}},
		{"tasks of a shot by path", filepath.Join(repo, "seq20", "sh010"), types.QueryRequest{Fetch: []string{"task"}, Set: true}},
		{"all assets with a task", repo, types.QueryRequest{With: []string{"task"}, Asset: true}},
		{"distinct task status", repo, types.QueryRequest{Fetch: []string{"task", "status"}, Distinct: true}},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := client.Query(ctx(), c.path, c.req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// -- FILE LOCAL -------------------------------------------------------------

var (
	benchTasks    = []string{"mod", "rig", "surf", "lay", "anim", "cfx", "fx", "lgt", "cmp", "roto"}
	benchDCC      = []string{"maya", "houdini", "nuke"}
	benchStatuses = []string{"wip", "review", "approved", "omit"}
)

func benchRepo(b *testing.B) (*Client, string) {
	b.Helper()
	dir := os.Getenv("VIT_BENCH_REPO")
	if dir == "" {
		dir = b.TempDir()
	}
	repo := filepath.Join(dir, "repo")
	client, err := NewClient(ctx(), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		b.Fatal(err)
	}
	done := filepath.Join(repo, ".vit", "bench_fixture")
	if _, err := os.Stat(done); err == nil {
		return client, repo
	}
	if err := os.RemoveAll(repo); err != nil {
		b.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Fatal(err)
	}
	if _, err := client.InitRepo(ctx(), repo); err != nil {
		b.Fatal(err)
	}
	write := func(n types.TreeNode) {
		path := filepath.Join(repo, ".vit", "tree", n.ID[:2], n.ID[2:]+".json")
		data, err := json.Marshal(n)
		if err == nil {
			err = os.MkdirAll(filepath.Dir(path), 0o755)
		}
		if err == nil {
			err = os.WriteFile(path, data, 0o644)
		}
		if err != nil {
			b.Fatal(err)
		}
	}
	dirNode := func(name, parent string) string {
		id := fsutil.GenerateUID(16)
		write(types.TreeNode{ID: id, Name: name, ParentID: parent, Type: types.TreeNodeTypeDir})
		return id
	}
	now := time.Now()
	for s := range 50 {
		seq := fmt.Sprintf("seq%02d", 10+s)
		seqID := dirNode(seq, "")
		for h := range 20 {
			shot := fmt.Sprintf("sh%03d", 10*(h+1))
			shotID := dirNode(shot, seqID)
			for t, task := range benchTasks {
				for v := range 5 {
					n := types.TreeNode{ID: fsutil.GenerateUID(16), Name: fmt.Sprintf("%s_v%d", task, v+1), ParentID: shotID, Type: types.TreeNodeTypeAsset}
					for k, val := range map[string]string{
						"seq": seq, "shot": shot, "task": task, "variant": fmt.Sprintf("v%d", v+1),
						"dcc": benchDCC[t%3], "status": benchStatuses[(s+h+t+v)%4],
					} {
						n.SetAttr(k, val, "bench", now)
					}
					write(n)
				}
			}
		}
	}
	for _, f := range []string{filepath.Join(repo, ".vit", "cache", ".tree_stale"), done} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return client, repo
}
