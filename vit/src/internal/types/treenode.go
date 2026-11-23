package types

import (
	"fmt"
	"time"
)

// TreeNode represent either a directory or an asset in the virtual vit tree.
// Each one of them stored in a separate JSON file.
//
// If it represents an asset, it contains an uuid pointing to the AssetNode
// where all the asset data lives.
//
// TreeNode only store the name of the dir/asset, not the all path. And
// a ref to the parent UID. It does not refernce children.
// This is done so moving a TreeNode can be done by modyfing a single JSON.
type TreeNode struct {
	ID       string       `json:"id"`        // uuid
	Name     string       `json:"name"`      // name of the curr leaf, not full path
	ParentID string       `json:"parent_id"` // uuid of parent.
	Type     TreeNodeType `json:"type"`      // tree node describes both sub dir and assets

	// only filled if Type == TreeNodeTypeAsset
	AssetID string `json:"asset_id,omitempty"`

	Attr map[string]AttrKey `json:"attr,omitempty"`
}

type TreeNodeType string

const (
	TreeNodeTypeDir   TreeNodeType = "dir"
	TreeNodeTypeAsset TreeNodeType = "asset"
)

type AttrKey struct {
	AttrValue
	History []AttrValue `json:"history,omitempty"`
}

type AttrValue struct {
	Value   string    `json:"value"`
	User    string    `json:"user"`
	TS      time.Time `json:"ts"`
	Removed bool      `json:"removed,omitempty"`
}

func (n *TreeNode) SetAttr(key, value, user string, ts time.Time) {
	n.putAttr(key, AttrValue{Value: value, User: user, TS: ts})
}

func (n *TreeNode) RemoveAttr(key, user string, ts time.Time) error {
	if _, ok := n.LiveAttrs()[key]; !ok {
		return NewVitError(
			ErrAttr,
			[]string{fmt.Sprintf("no attribute %q on %s", key, n.Name)},
			[]any{"key", key, "node", n.ID},
		)
	}
	n.putAttr(key, AttrValue{User: user, TS: ts, Removed: true})
	return nil
}

func (n *TreeNode) LiveAttrs() map[string]AttrValue {
	ret := make(map[string]AttrValue, len(n.Attr))
	for k, a := range n.Attr {
		if !a.Removed {
			ret[k] = a.AttrValue
		}
	}
	return ret
}

func CheckAttrKey(key string) error {
	if key == "" || key[0] == '_' || key[0] == '.' || key[0] == '-' {
		return newInvalidAttrKeyErr(key)
	}
	for _, r := range key {
		if !isAttrKeyRune(r) {
			return newInvalidAttrKeyErr(key)
		}
	}
	return nil
}

// -- FILE LOCAL -------------------------------------------------------------

func (n *TreeNode) putAttr(key string, v AttrValue) {
	if n.Attr == nil {
		n.Attr = make(map[string]AttrKey)
	}
	a, ok := n.Attr[key]
	if ok {
		a.History = append(a.History, a.AttrValue)
	}
	a.AttrValue = v
	n.Attr[key] = a
}

func isAttrKeyRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '-'
}

func newInvalidAttrKeyErr(key string) error {
	return NewVitError(
		ErrAttr,
		[]string{fmt.Sprintf("invalid attribute key %q: use lowercase letters, digits, '_', '.', '-', starting with a letter or a digit", key)},
		[]any{"key", key},
	)
}
