package types

import (
	"time"
)

type (
	TagFamily = string
	SiteUID   = string
	TagName   = string

	TagsCollection = map[TagFamily]map[SiteUID]map[TagName]*AssetTag
)

type Asset struct {
	AssetUID string                  `json:"asset-uid"`
	Commits  map[string]*AssetCommit `json:"commits"`
	Branches map[string]*AssetCommit `json:"branches"`
	Tags     TagsCollection          `json:"tags,omitempty"`

	// set at load time, not persisted, buffered data for ease
	RepoUID   string `json:"-"`
	RepoPath  string `json:"-"`
	Site      string `json:"-"`
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
	Site         string            `json:"site,omitempty"`
	Message      string            `json:"message"`
	Parent       string            `json:"parent"`
	Dependencies AssetDependencies `json:"dependencies,omitempty"`

	// IsSnapshot marks a user snapshot:
	//	- the `add` baseline (first snapshot)
	//  - every `vit snapshot`.
	IsSnapshot bool `json:"is_snapshot,omitempty"`
}

const snapshotIDLen = 4

func (c *AssetCommit) SnapshotKey(commitID string) string {
	return SnapshotKeyFrom(FormatTimestampLocal(c.Timestamp), c.Author, commitID)
}

func SnapshotKeyFrom(dateStr, author, commitID string) string {
	id := commitID
	if len(id) > snapshotIDLen {
		id = id[:snapshotIDLen]
	}
	return dateStr + "-" + author + "-" + id
}

type AssetDependencies = map[string]*AssetDependency

type AssetDependency struct {
	Type        AssetDependencyType `json:"type"`
	RefSpec     *Ref                `json:"ref_spec"`
	RefResolved *Ref                `json:"ref_resolved"`
	Commit      string              `json:"commit"`
}

type AssetDependencyType string

const (
	AssetDependencySource  AssetDependencyType = "source"
	AssetDependencyProduct AssetDependencyType = "product"
)

type AssetTag struct {
	Message   string    `json:"message,omitempty"`
	Author    string    `json:"author"`
	Commit    string    `json:"commit"`
	Timestamp time.Time `json:"timestamp"`
}

func (a *Asset) SetTag(family, site, name string, tag *AssetTag) {
	if a.Tags == nil {
		a.Tags = make(TagsCollection)
	}
	if a.Tags[family] == nil {
		a.Tags[family] = make(map[SiteUID]map[TagName]*AssetTag)
	}
	if a.Tags[family][site] == nil {
		a.Tags[family][site] = make(map[TagName]*AssetTag)
	}
	a.Tags[family][site][name] = tag
}

func (a *Asset) TagVersions(family, site string) (map[TagName]*AssetTag, bool) {
	fam, ok := a.Tags[family]
	if !ok {
		return nil, false
	}
	leaf, ok := fam[site]
	return leaf, ok
}

func (a *Asset) RepoRef() *RepoRef {
	return &RepoRef{
		UID:  a.RepoUID,
		Path: a.RepoPath,
		Site: a.Site,
	}
}
