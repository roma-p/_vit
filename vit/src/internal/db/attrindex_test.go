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

func TestAttrIndexRebuildRemovesStaleShards(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod"})
	stale := pathAttrShard(repo, "old")
	if err := fsutil.NewJSONHandlerFromPath(stale, &attrShard{}).Write(); err != nil {
		t.Fatal(err)
	}

	mustShards(t, repo, "task")

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale shard kept: %v", err)
	}
	if _, err := os.Stat(pathAttrIndexBuild(repo)); err != nil {
		t.Fatalf("build marker: %v", err)
	}
}

func TestAttrIndexRefreshSeesChangesAfterTheBuild(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod", "seq": "20"})
	writeNode(t, repo, "u2", types.TreeNodeTypeAsset, map[string]string{"task": "mod"})
	mustShards(t, repo, "task", "seq")

	writeNode(t, repo, "u3", types.TreeNodeTypeAsset, map[string]string{"task": "mod"})
	recordAttr(t, repo, "u3", "task")
	writeNode(t, repo, "u2", types.TreeNodeTypeAsset, map[string]string{"task": "anim"})
	recordAttr(t, repo, "u2", "task")
	writeNode(t, repo, "d1", types.TreeNodeTypeDir, map[string]string{"task": "mod"})
	recordAttr(t, repo, "d1", "task")
	n := writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod", "seq": "20", "variant": "a"})
	_ = n.RemoveAttr("seq", "bob", time.Now())
	writeNodeData(t, repo, n)
	recordAttr(t, repo, "u1", "seq", "variant")

	got := mustShards(t, repo, "task", "seq", "variant")

	if want := map[string][]string{"mod": {"u1", "u3"}, "anim": {"u2"}}; !maps.EqualFunc(got["task"].Values, want, slices.Equal) {
		t.Fatalf("task = %v", got["task"].Values)
	}
	if len(got["seq"].Values) != 0 {
		t.Fatalf("seq = %v", got["seq"].Values)
	}
	if want := map[string][]string{"a": {"u1"}}; !maps.EqualFunc(got["variant"].Values, want, slices.Equal) {
		t.Fatalf("variant = %v", got["variant"].Values)
	}
	head, _ := history.Head(repo)
	disk, err := readAttrShard(repo, "task")
	if err != nil || !maps.Equal(disk.Cursor, head) || !maps.EqualFunc(disk.Values, got["task"].Values, slices.Equal) {
		t.Fatalf("not stored at the head %v: %+v %v", head, disk, err)
	}
}

func TestAttrIndexRefreshOnlyAdvancesTheAskedShards(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod", "seq": "20"})
	built := mustShards(t, repo, "seq")["seq"].Cursor
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod", "seq": "30"})
	recordAttr(t, repo, "u1", "seq")

	mustShards(t, repo, "task")

	disk, err := readAttrShard(repo, "seq")
	if err != nil || !maps.Equal(disk.Cursor, built) || disk.Values["20"] == nil {
		t.Fatalf("seq advanced: %+v %v", disk, err)
	}
	if got := mustShards(t, repo, "seq")["seq"].Values; got["30"] == nil || got["20"] != nil {
		t.Fatalf("seq = %v", got)
	}
}

func TestAttrIndexRefreshInMemoryForTheSameIndex(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod"})
	idx := NewAttrIndex(repo)
	if _, err := idx.get(context.Background(), []string{"task"}); err != nil {
		t.Fatal(err)
	}
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "anim"})
	recordAttr(t, repo, "u1", "task")

	got, err := idx.get(context.Background(), []string{"task"})
	if err != nil || got["task"].Values["anim"] == nil {
		t.Fatalf("task = %v %v", got["task"].Values, err)
	}
}

func TestAttrIndexRefreshShardsAtDifferentCursors(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod", "seq": "20"})
	mustShards(t, repo, "task", "seq")
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod", "seq": "30"})
	recordAttr(t, repo, "u1", "seq")
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "anim", "seq": "30"})
	recordAttr(t, repo, "u1", "task")
	mustShards(t, repo, "task")

	got := mustShards(t, repo, "task", "seq")

	if got["task"].Values["anim"] == nil || got["seq"].Values["30"] == nil || got["seq"].Values["20"] != nil {
		t.Fatalf("task = %v, seq = %v", got["task"].Values, got["seq"].Values)
	}
	if !maps.Equal(got["task"].Cursor, got["seq"].Cursor) {
		t.Fatalf("cursors differ: %v %v", got["task"].Cursor, got["seq"].Cursor)
	}
}

func TestAttrIndexRefreshIsIdempotent(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u1", types.TreeNodeTypeAsset, map[string]string{"task": "mod"})
	recordAttr(t, repo, "u1", "task")
	recordAttr(t, repo, "u1", "task")
	idx := &AttrIndex{repoPath: repo}
	s := &attrShard{Values: map[string][]string{"anim": {"u1"}}}

	for range 2 {
		s.Cursor = nil
		if err := idx.refreshShardsFromHistory(map[string]*attrShard{"task": s}); err != nil {
			t.Fatal(err)
		}
	}
	if want := map[string][]string{"mod": {"u1"}}; !maps.EqualFunc(s.Values, want, slices.Equal) {
		t.Fatalf("task = %v", s.Values)
	}
}

func TestOldestCursor(t *testing.T) {
	pos := func(seg string, line int) history.Position { return history.Position{Segment: seg, Line: line} }
	shards := map[string]*attrShard{
		"a": {Cursor: history.Cursor{"paris": pos("seg-000002.jsonl", 1), "tokyo": pos("seg-000001.jsonl", 9)}},
		"b": {Cursor: history.Cursor{"paris": pos("seg-000001.jsonl", 7)}},
		"c": {Cursor: history.Cursor{"paris": pos("seg-000001.jsonl", 3)}},
	}
	want := history.Cursor{"paris": pos("seg-000001.jsonl", 3), "tokyo": {}}
	if got := oldestCursor(shards); !maps.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAttrShardIsStoredAsSortedRows(t *testing.T) {
	repo := t.TempDir()
	s := &attrShard{Cursor: history.Cursor{"paris": {Segment: "seg-000001.jsonl", Line: 2}}, Values: map[string][]string{"mod": {"u1", "u3"}, "rig": {"u4"}, "anim": {"u2"}, "lay": {"u5"}, "cfx": {"u6"}}}
	if err := writeAttrShard(repo, "task", s); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(pathAttrShard(repo, "task"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"cursor":{"paris":{"segment":"seg-000001.jsonl","line":2}},"values":[["anim","u2"],["cfx","u6"],["lay","u5"],["mod","u1","u3"],["rig","u4"]]}` + "\n"; string(data) != want {
		t.Fatalf("stored = %s", data)
	}
	got, err := readAttrShard(repo, "task")
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got.Cursor, s.Cursor) || !maps.EqualFunc(got.Values, s.Values, slices.Equal) {
		t.Fatalf("read back = %+v", got)
	}
}

func TestAttrIndexInTheOldFormatIsRebuilt(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u1abcdef", types.TreeNodeTypeAsset, map[string]string{"task": "mod"})
	for path, content := range map[string]string{
		pathAttrIndexBuild(repo):    `{"cursor":{}}`,
		pathAttrShard(repo, "task"): `{"cursor":{},"values":{"anim":["u1abcdef"]}}`,
	} {
		if err := os.MkdirAll(fsutil.DirOS(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if got := mustShards(t, repo, "task")["task"].Values; !maps.EqualFunc(got, map[string][]string{"mod": {"u1abcdef"}}, slices.Equal) {
		t.Fatalf("values = %v", got)
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
	path := treeNodeJSONPath(repo, n.ID)
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

func recordAttr(t *testing.T, repo, uid string, keys ...string) {
	t.Helper()
	if err := history.Record(context.Background(), repo, "", attrEntry(uid, keys...)); err != nil {
		t.Fatal(err)
	}
}

func attrEntry(uid string, keys ...string) types.HistoryEntry {
	e := types.HistoryEntry{NodeID: uid}
	for _, k := range keys {
		e.Attrs = append(e.Attrs, types.AttrChange{Key: k})
	}
	return e
}
