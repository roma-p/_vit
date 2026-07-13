package types

import (
	"time"
)

type Asset struct {
	AssetUID string                          `json:"asset-uid"`
	Commits  map[string]*AssetCommit         `json:"commits"`
	Branches map[string]*AssetCommit         `json:"branches"`
	Tags     map[string]map[string]*AssetTag `json:"tags,omitempty"`

	// set at load time, not persisted, buffered data for ease of conveniance
	RepoPath  string `json:"-"`
	AssetPath string `json:"-"`
}

type AssetCommit struct {
	// if branch: link to blob dir. if commit: content hash of the root content node.
	Content string `json:"content"`

	// timestamp of commit: used to reference the commit in a timeline,
	//  registered at very start of the vit op.
	//  (using OperationContext.Timestamp)
	// when used as branch commit, the timestamp is registered after blob copy is done.
	//  it is then used as a baseline timestamp to detect changes on files.
	Timestamp time.Time `json:"timestamp"`

	Author       string            `json:"author"`
	Message      string            `json:"message,omitempty"`
	Parent       string            `json:"parent,omitempty"`
	Dependencies AssetDependencies `json:"dependencies,omitempty"`
}

type AssetDependencies = map[string]*AssetDependency

type AssetDependency struct {
	Type   *AssetDependencyType `json:"type"`
	Ref    *Ref                 `json:"ref"`
	Commit string               `json:"commit"`
}

type AssetDependencyType string

const (
	AssetDependencySource AssetDependencyType = "source"
	AssetDependencyResult AssetDependencyType = "result"
)

type AssetTag struct {
	Message   string    `json:"message,omitempty"`
	Author    string    `json:"author"`
	Commit    string    `json:"commit"`
	Timestamp time.Time `json:"timestamp"`
}
