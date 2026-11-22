package types

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
	AssetID string            `json:"asset_id,omitempty"`
	Attr    map[string]string `json:"attr,omitempty"`
}

type TreeNodeType string

const (
	TreeNodeTypeDir   TreeNodeType = "dir"
	TreeNodeTypeAsset TreeNodeType = "asset"
)
