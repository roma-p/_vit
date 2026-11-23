package db

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"slices"
	"testing"
	"time"

	"vit/internal/fsutil"
	"vit/internal/history"
	"vit/internal/types"
)

func TestAttrIndexBuildsOneShardPerKey(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u2", types.TreeNodeTypeAsset, map[string]string{"task": "mod", "seq": "20"})
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod"})
	writeNode(t, repo, "u3", types.TreeNodeTypeAsset, map[string]string{"task": "anim"})
	writeNode(t, repo, "d1", types.TreeNodeTypeDir, map[string]string{"task": "mod"})
	removed := writeNode(t, repo, "u4", types.TreeNodeTypeAsset, map[string]string{"task": "mod"})
	_ = removed.RemoveAttr("task", "bob", time.Now())
	writeNodeData(t, repo, removed)

	shards := mustShards(t, repo, "task", "seq")

	if want := map[string][]string{"mod": {"u1", "u2"}, "anim": {"u3"}}; !maps.EqualFunc(shards["task"].Values, want, slices.Equal) {
		t.Fatalf("task = %v", shards["task"].Values)
	}
	if want := map[string][]string{"20": {"u2"}}; !maps.EqualFunc(shards["seq"].Values, want, slices.Equal) {
		t.Fatalf("seq = %v", shards["seq"].Values)
	}
}

func TestAttrIndexCursorIsTheHistoryHeadAtBuild(t *testing.T) {
	repo := t.TempDir()
	if err := history.Record(context.Background(), repo, "", types.HistoryEntry{UID: "e1"}); err != nil {
		t.Fatal(err)
	}
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod"})
	head, err := history.Head(repo)
	if err != nil {
		t.Fatal(err)
	}

	shards := mustShards(t, repo, "task", "never_set")

	for k, s := range shards {
		if !maps.Equal(s.Cursor, head) {
			t.Fatalf("%s cursor = %v, want %v", k, s.Cursor, head)
		}
	}
	if len(shards["never_set"].Values) != 0 {
		t.Fatalf("never_set = %v", shards["never_set"].Values)
	}
}

func TestAttrIndexLoadsAShardOnce(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod"})
	idx := NewAttrIndex(repo)
	first, err := idx.get(context.Background(), []string{"task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(pathAttrShard(repo, "task")); err != nil {
		t.Fatal(err)
	}

	again, err := idx.get(context.Background(), []string{"task", "task"})
	if err != nil {
		t.Fatal(err)
	}
	if again["task"] != first["task"] {
		t.Fatalf("task reloaded: %v", again["task"].Values)
	}
}

func TestAttrIndexIsNotRebuiltOnceBuilt(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod"})
	mustShards(t, repo, "task")
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "anim"})

	if got := mustShards(t, repo, "task")["task"].Values; got["mod"] == nil {
		t.Fatalf("rebuilt: %v", got)
	}
}

// -- FILE LOCAL -------------------------------------------------------------

func writeNode(t *testing.T, repo, id string, typ types.TreeNodeType, attrs map[string]string) *types.TreeNode {
	t.Helper()
	n := &types.TreeNode{ID: id, Name: id, Type: typ}
	for k, v := range attrs {
		n.SetAttr(k, v, "alice", time.Now())
	}
	writeNodeData(t, repo, n)
	return n
}

func writeNodeData(t *testing.T, repo string, n *types.TreeNode) {
	t.Helper()
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	path := fsutil.PathOS(repo, ".vit", "tree", n.ID+".json")
	if err := os.MkdirAll(fsutil.DirOS(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustShards(t *testing.T, repo string, keys ...string) map[string]*attrShard {
	t.Helper()
	shards, err := NewAttrIndex(repo).get(context.Background(), keys)
	if err != nil {
		t.Fatal(err)
	}
	return shards
}
