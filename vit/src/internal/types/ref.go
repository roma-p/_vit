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
//
// a ref can be stringified using either:
//   - Absolute path: <repo path>/<asset path>@<object path>
//   - Relative path:             <asset path>@<object path>
//   - Object path:                           @<object path>
//
// Ref is a value type — all fields are comparable, so two Refs with
// identical fields are equal via ==.
type Ref struct {
	RepoPath  string  `json:"repo_path"`
	AssetPath string  `json:"asset_path"`
	RefType   RefType `json:"ref_type"`

	// Branch Data
	BranchName string `json:"branch_name,omitempty"`

	// Tag Data
	TagFamily string `json:"tag_family,omitempty"`
	TagName   string `json:"tag_name,omitempty"`

	// Commit Data
	CommitID string `json:"commit_id,omitempty"`
}

type RefType string

const (
	RefTypeBranch RefType = "branch"
	RefTypeCommit RefType = "commit"
	RefTypeTag    RefType = "tag"
)

func NewRefBranch(repoPath, assetPath, branchName string) *Ref {
	return &Ref{
		RepoPath:   repoPath,
		AssetPath:  assetPath,
		RefType:    RefTypeBranch,
		BranchName: branchName,
	}
}

func NewRefTag(repoPath, assetPath, tagFamily, tagName string) *Ref {
	return &Ref{
		RepoPath:  repoPath,
		AssetPath: assetPath,
		RefType:   RefTypeTag,
		TagFamily: tagFamily,
		TagName:   tagName,
	}
}

// NewRefTagFloating does not point directly to a commit (here only a group of commit)
func NewRefTagFloating(repoPath, assetPath, tagFamily string) *Ref {
	return &Ref{
		RepoPath:  repoPath,
		AssetPath: assetPath,
		RefType:   RefTypeTag,
		TagFamily: tagFamily,
	}
}

func NewRefCommit(repoPath, assetPath, commitID string) *Ref {
	return &Ref{
		RepoPath:  repoPath,
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

	refType := RefType(refTypeStr)
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
		ret.CommitID = tmp
	case RefTypeTag:
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

func (r *Ref) RelativePath() string {
	return fmt.Sprintf("%s%s", r.AssetPath, r.ObjectPath())
}

func (r *Ref) AbsolutePath() string {
	return fmt.Sprintf("%s/%s", r.RepoPath, r.RelativePath())
}

func newErrInvalidRef(path string) error {
	return NewVitError(
		ErrInvalidRef,
		[]string{fmt.Sprintf("not a valid ref path %s", path)},
		[]any{"refPath", path},
	)
}
