package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSetAttrPushesThePreviousValueToHistory(t *testing.T) {
	n := &TreeNode{}
	n.SetAttr("task", "mdo", "alice", at(1))
	n.SetAttr("task", "mod", "bob", at(2))

	a := n.Attr["task"]
	if a.Value != "mod" || a.User != "bob" || !a.TS.Equal(at(2)) {
		t.Fatalf("current = %+v", a.AttrValue)
	}
	if len(a.History) != 1 || a.History[0].Value != "mdo" || a.History[0].User != "alice" {
		t.Fatalf("history = %+v", a.History)
	}
}

func TestRemoveAttrLeavesATombstone(t *testing.T) {
	n := &TreeNode{}
	n.SetAttr("task", "mod", "alice", at(1))
	n.SetAttr("dcc", "maya", "alice", at(1))

	if err := n.RemoveAttr("task", "bob", at(2)); err != nil {
		t.Fatal(err)
	}

	a := n.Attr["task"]
	if !a.Removed || a.User != "bob" || len(a.History) != 1 || a.History[0].Value != "mod" {
		t.Fatalf("task = %+v", a)
	}
	live := n.LiveAttrs()
	if _, ok := live["task"]; ok || live["dcc"].Value != "maya" {
		t.Fatalf("live = %+v", live)
	}
}

func TestRemoveAttrRejectsAMissingKey(t *testing.T) {
	n := &TreeNode{}
	if err := n.RemoveAttr("task", "bob", at(1)); !IsErrCode(err, ErrAttr) {
		t.Fatalf("missing key: want %s, got %v", ErrAttr, err)
	}
	n.SetAttr("task", "mod", "alice", at(1))
	if err := n.RemoveAttr("task", "bob", at(2)); err != nil {
		t.Fatal(err)
	}
	if err := n.RemoveAttr("task", "bob", at(3)); !IsErrCode(err, ErrAttr) {
		t.Fatalf("removed key: want %s, got %v", ErrAttr, err)
	}
}

func TestSetAttrRevivesARemovedKey(t *testing.T) {
	n := &TreeNode{}
	n.SetAttr("task", "mod", "alice", at(1))
	if err := n.RemoveAttr("task", "alice", at(2)); err != nil {
		t.Fatal(err)
	}
	n.SetAttr("task", "rig", "alice", at(3))

	if n.LiveAttrs()["task"].Value != "rig" || len(n.Attr["task"].History) != 2 {
		t.Fatalf("task = %+v", n.Attr["task"])
	}
}

func TestCheckAttrKey(t *testing.T) {
	for _, key := range []string{"task", "frame_range", "dcc.version", "v-2", "Seq20"} {
		if err := CheckAttrKey(key); err != nil {
			t.Errorf("%q rejected: %v", key, err)
		}
	}
	for _, key := range []string{"", "a=b", "a/b", "a b", "tâche"} {
		if err := CheckAttrKey(key); !IsErrCode(err, ErrAttr) {
			t.Errorf("%q: want %s, got %v", key, ErrAttr, err)
		}
	}
}

func TestTreeNodeAttrJSON(t *testing.T) {
	n := &TreeNode{ID: "id", Type: TreeNodeTypeDir}
	n.SetAttr("seq", "20", "alice", at(1))

	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Attr map[string]map[string]any `json:"attr"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Attr["seq"]["value"] != "20" {
		t.Fatalf("value not flat in %s", b)
	}

	var back TreeNode
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Attr["seq"].Value != "20" || !back.Attr["seq"].TS.Equal(at(1)) {
		t.Fatalf("round trip = %+v", back.Attr)
	}

	empty, _ := json.Marshal(&TreeNode{ID: "id"})
	var fields map[string]any
	_ = json.Unmarshal(empty, &fields)
	if _, ok := fields["attr"]; ok {
		t.Fatalf("attr written on a node without attributes: %s", empty)
	}
}

// -- FILE LOCAL -------------------------------------------------------------

func at(sec int) time.Time {
	return time.Date(2026, 11, 22, 0, 0, sec, 0, time.UTC)
}
