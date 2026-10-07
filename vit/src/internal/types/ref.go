package types

import (
	"fmt"
	"strings"
)

// -- Ref --------------------------------------------------------------------
// ---------------------------------------------------------------------------

// Ref uniquely identifies a version of an asset in a vit repository.
// It can point to a branch (mutable head), a commit (immutable snapshot),
// or a tag (named release marker, optionally floating if only tagFamily
// is defined and not tagName).
//
// A commit ref cn be epxressed through a "snapshot" path, meant for users to
// find commit more easily than having to know a commit id.
//
// The Ref also always include the root path of its vit repository
// Vit shall support cross repository linking, so the repo path might differs
// from one another.
//
// Ref can either link to a commit, a branch or a tag.
// The type of the ref is defined in RefType,
// Depending on RefType, only relevant fields of the Ref will be filled
//
// Ref can be represented as strings such as:
// The string form follows the pattern: "/repo/asset/path@type=value"
//
// a ref string is structured like this
//
//		 <vit repository absolute path>/<asset path><object path>
//	eg:                      /path/repo/assets/mod  @branch=fix
//	                         /path/repo/assets/mod  @tag=release
//	                         /path/repo/assets/mod  @tag=release@name=12
//	                         /path/repo/assets/mod  @commit=ad12b8
//	                         /path/repo/assets/mod  @snapshot=2026-10-05T11-18-15-user1-e11a
//
// a ref can be stringified using either:
//   - Absolute path: <repo path>/<asset path>@<object path>
//   - Relative path:             <asset path>@<object path>
//   - Object path:                           @<object path>
//
// Ref is a value type — all fields are comparable, so two Refs with
// identical fields are equal via ==.
type Ref struct {
	RepoUID   string  `json:"repo_uid"`
	RepoPath  string  `json:"-"`
	AssetPath string  `json:"asset_path"`
	RefType   RefType `json:"ref_type"`

	// Branch Data
	BranchName string `json:"branch_name,omitempty"`

	// Tag Data
	TagFamily string `json:"tag_family,omitempty"`
	TagName   string `json:"tag_name,omitempty"`
	Site      string `json:"site,omitempty"`

	// Commit Data
	CommitID string `json:"commit_id,omitempty"`
	// HR form of a commit which is was snapshot (@snapshot=<ts>-<user>),
	// format is : @snapshot=<ts>-<user>-<four first charac of commit id>
	SnapshotKey string `json:"-"`
}

type RefType string

const (
	RefTypeBranch RefType = "branch"
	RefTypeCommit RefType = "commit"
	RefTypeTag    RefType = "tag"
	RefTypeEmpty  RefType = "empty"
)

type RepoRef struct {
	UID  string
	Path string
	Site string
}

func NewRefBranch(repoRef *RepoRef, assetPath, branchName string) *Ref {
	return &Ref{
		RepoUID:    repoRef.UID,
		RepoPath:   repoRef.Path,
		AssetPath:  assetPath,
		RefType:    RefTypeBranch,
		BranchName: branchName,
	}
}

func NewRefTag(repoRef *RepoRef, assetPath, tagFamily, tagName, site string) *Ref {
	// site is optionnal.
	return &Ref{
		RepoUID:   repoRef.UID,
		RepoPath:  repoRef.Path,
		AssetPath: assetPath,
		RefType:   RefTypeTag,
		TagFamily: tagFamily,
		TagName:   tagName,
		Site:      site,
	}
}

// NewRefTagFloating does not point directly to a commit (here only a group of commit)
func NewRefTagFloating(repoRef *RepoRef, assetPath, tagFamily, site string) *Ref {
	// site is optionnal.
	return &Ref{
		RepoUID:   repoRef.UID,
		RepoPath:  repoRef.Path,
		AssetPath: assetPath,
		RefType:   RefTypeTag,
		TagFamily: tagFamily,
		Site:      site,
	}
}

func NewRefCommit(repoRef *RepoRef, assetPath, commitID string) *Ref {
	return &Ref{
		RepoUID:   repoRef.UID,
		RepoPath:  repoRef.Path,
		AssetPath: assetPath,
		RefType:   RefTypeCommit,
		CommitID:  commitID,
	}
}

func NewRefFromPath(repoPath, refPath string) (*Ref, error) {
	assetPath, tmp, found := strings.Cut(refPath, "@")
	if !found {
		return nil, newErrInvalidRef(refPath)
	}

	refTypeStr, tmp, found := strings.Cut(tmp, "=")
	if !found {
		return nil, newErrInvalidRef(refPath)
	}
	snapshot := refTypeStr == "snapshot"
	refType := RefType(refTypeStr)
	if snapshot {
		refType = RefTypeCommit
	}

	switch refType {
	case RefTypeBranch, RefTypeTag, RefTypeCommit:
	default:
		return nil, newErrInvalidRef(refPath)
	}

	ret := Ref{
		RepoPath:  repoPath,
		AssetPath: assetPath,
		RefType:   refType,
	}

	switch refType {
	case RefTypeBranch:
		ret.BranchName = tmp
	case RefTypeCommit:
		if snapshot {
			ret.SnapshotKey = tmp
		} else {
			ret.CommitID = tmp
		}
	case RefTypeTag:
		// TODO(site): does NOT parse @site= yet
		tagfamily, tmp2, found := strings.Cut(tmp, "@")
		if !found {
			ret.TagFamily = tmp
		} else if tagnameid, tagname, found := strings.Cut(tmp2, "="); found && tagnameid == "name" {
			ret.TagFamily = tagfamily
			ret.TagName = tagname
		} else {
			ret.TagFamily = tmp
		}
	}
	return &ret, nil
}

func NewRefEmpty(repoPath, assetPath string) *Ref {
	return &Ref{
		RepoPath:  repoPath,
		AssetPath: assetPath,
		RefType:   RefTypeEmpty,
	}
}

func (r *Ref) IsMutable() bool {
	return r.RefType == RefTypeBranch || (r.RefType == RefTypeTag && r.TagName == "")
}

func (r *Ref) ObjectPath() string {
	var refPath string

	switch r.RefType {
	case RefTypeBranch:
		refPath = fmt.Sprintf("@branch=%s", r.BranchName)
	case RefTypeCommit:
		refPath = fmt.Sprintf("@commit=%s", r.CommitID)
	case RefTypeTag:
		if r.TagName == "" {
			// Floating tag (no specific name)
			refPath = fmt.Sprintf("@tag=%s", r.TagFamily)
		} else {
			// Named tag
			refPath = fmt.Sprintf("@tag=%s@name=%s", r.TagFamily, r.TagName)
		}
	}
	return refPath
}

func (r *Ref) SnapshotPath() string {
	if r.SnapshotKey != "" {
		return r.AssetPath + "@snapshot=" + r.SnapshotKey
	}
	return r.RelativePath()
}

func (r *Ref) RepoRef() *RepoRef {
	return &RepoRef{UID: r.RepoUID, Path: r.RepoPath, Site: r.Site}
}

func (r *Ref) RelativePath() string {
	return fmt.Sprintf("%s%s", r.AssetPath, r.ObjectPath())
}

func (r *Ref) AbsolutePath() string {
	return fmt.Sprintf("%s/%s", r.RepoPath, r.RelativePath())
}

func (r *Ref) AssertTypeOfRef(t RefType) {
	if r.RefType != t {
		panic(fmt.Sprintf(
			"Unexpected type for Ref: %s, expected %s, got %s",
			r.RelativePath(),
			t,
			r.RefType,
		))
	}
}

func (r *Ref) CheckTypeOfRef(t RefType) error {
	if r.RefType != t {
		return NewVitError(
			ErrInvalidInut,
			[]string{fmt.Sprintf(
				"Unexpected type for Ref: %s, expected %s, got %s",
				r.RelativePath(),
				t,
				r.RefType,
			)},
			[]any{"Ref", r.RelativePath(), "ExpectedType", t, "ReceviedType", r.RefType},
		)
	} else {
		return nil
	}
}

// -- FILE LOCAL -------------------------------------------------------------

func newErrInvalidRef(path string) error {
	return NewVitError(
		ErrInvalidRef,
		[]string{fmt.Sprintf("not a valid ref path %s", path)},
		[]any{"refPath", path},
	)
}
