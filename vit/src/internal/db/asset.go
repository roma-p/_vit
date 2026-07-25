package db

import (
	"context"
	"fmt"
	"os"
	"time"

	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
)

type JSONAsset = fsutil.JSONHandler[types.Asset]

func TagObject(r TagObjectRequest) (string, error) {
	return addTag(
		r.Asset,
		&types.AssetTag{
			Message:   r.Message,
			Author:    r.Author,
			Commit:    r.TargetCommit,
			Timestamp: r.TimeStamp,
		},
		r.TagFamily, r.SiteUID,
	)
}

type TagObjectRequest struct {
	Asset        *types.Asset
	Author       string
	Message      string
	TimeStamp    time.Time
	TagFamily    string
	TargetCommit string
	SiteUID      string
}

// CommitBranch appends a new commit and advances the branch to it (parent + baseline
// timestamp). Returns the new commit's ID (needed e.g. by tag-on-branch, which tags
// the commit it just created).
func CommitBranch(r CommitBranchRequest) (string, error) {
	branchCommit, err := GetBranchCommit(r.Asset, r.BranchName)
	if err != nil {
		return "", err
	}

	ret := &types.AssetCommit{
		Content:   r.RootcontentNodeUID,
		Author:    r.Author,
		Site:      r.Site,
		Message:   r.Message,
		Parent:    branchCommit.Parent,
		Timestamp: r.TimeStamp,
	}

	newCommitID, err := addCommit(r.Asset, ret)
	if err != nil {
		return "", err
	}
	branchCommit.Parent = newCommitID
	branchCommit.Timestamp = r.TimeStamp
	return newCommitID, nil
}

type CommitBranchRequest struct {
	Asset              *types.Asset
	BranchName         string
	RootcontentNodeUID string
	Author             string
	Site               string
	Message            string
	TimeStamp          time.Time
	// TODO dependancies
}

func GetCommit(a *types.Asset, commitID string) (*types.AssetCommit, error) {
	if a.Commits == nil {
		return nil, newErrObjectNotFound(types.NewRefCommit(a.RepoPath, a.AssetPath, commitID))
	}

	commitObj, ok := a.Commits[commitID]
	if !ok {
		return nil, newErrObjectNotFound(types.NewRefCommit(a.RepoPath, a.AssetPath, commitID))
	}

	return commitObj, nil
}

func GetBranchCommit(a *types.Asset, branchName string) (*types.AssetCommit, error) {
	if a.Branches == nil {
		return nil, newErrObjectNotFound(types.NewRefBranch(a.RepoPath, a.AssetPath, branchName))
	}
	commitObj, ok := a.Branches[branchName]
	if !ok {
		return nil, newErrObjectNotFound(types.NewRefBranch(a.RepoPath, a.AssetPath, branchName))
	}
	return commitObj, nil
}

func GetTag(a *types.Asset, ref *types.Ref) (*types.AssetTag, string, error) {
	ref.AssertTypeOfRef(types.RefTypeTag)

	versions, err := getTagFamilyPerSite(a, ref.TagFamily, ref.Site)
	if err != nil {
		return nil, "", newErrObjectNotFound(ref)
	}

	tagName := ref.TagName
	if tagName == "" { // floating ref → the latest version
		tagName, _ = highestVersion(*versions)
		if tagName == "" {
			return nil, "", newErrObjectNotFound(ref)
		}
	}

	tag, ok := (*versions)[tagName]
	if !ok {
		return nil, "", newErrObjectNotFound(ref)
	}
	return tag, tagName, nil
}

// CheckCommitNotAlreadyOnTagFamily errors if commitID is already tagged within
// (tagFamily, siteUID) — a commit may hold at most one version per family/site.
// A not-yet-existing (family, site) stack means nothing is tagged → OK.
func CheckCommitNotAlreadyOnTagFamily(a *types.Asset, commitID, tagFamily, siteUID string) error {
	versions, err := getTagFamilyPerSite(a, tagFamily, siteUID)
	if err != nil {
		return nil // no such (family, site) yet → nothing tagged
	}
	for version, tag := range *versions {
		if tag.Commit == commitID {
			return newErrObjectAlreadyExists(
				types.NewRefTag(a.RepoPath, a.AssetPath, tagFamily, version, siteUID),
			)
		}
	}
	return nil
}

// -- USED ACROSS vit.internal.DB --------------------------------------------

func createNewJSONAsset(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, assetPath string,
) (*JSONAsset, error) {
	uid := fsutil.GenerateUID(16)
	return fsutil.ResolveHandler(
		ctx,
		opctx.JSONPool,
		pathJSONAsset(opctx.RepoPath, uid),
		true,
		&types.Asset{
			AssetUID: uid,

			// comodity, not exported written in the json, and not source of truth
			// just to avoid having to carry multiple data object.
			AssetPath: assetPath,
			RepoPath:  repoPath,
		},
	)
}

func resolveJSONAsset(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoPath, assetPath, uid string, // asset path just used for formatting
	forWrite bool,
) (*JSONAsset, bool, error) {
	jsonPath := pathJSONAsset(repoPath, uid)
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return nil, false, newErrObjectIndexNotFound(repoPath, assetPath, uid)
	}
	ret, err := fsutil.ResolveHandler[types.Asset](ctx, opctx.JSONPool, jsonPath, forWrite, nil)
	if err != nil {
		return nil, false, err
	}

	// commodity, not persisted in json, not source of truth
	// just to avoid having to carry multiple data object.
	ret.Data.AssetPath = assetPath
	ret.Data.RepoPath = repoPath

	return ret, true, nil
}

func initAsset(
	opctx *opcontext.OperationContext,
	asset *types.Asset,
	blobBranch, contentRootHash string,
	baselineTimeStamp time.Time,
) error {
	commitFirst := types.AssetCommit{
		Site:      opctx.RepoConfig.CurrentSite,
		Author:    opctx.User,
		Message:   "Asset creation",
		Timestamp: opctx.TimeStamp,
		Parent:    "",
		Content:   contentRootHash,
	}

	commitID, err := addCommit(asset, &commitFirst)
	if err != nil {
		return err
	}

	err = addBranch(
		asset,
		commitID,
		types.DefaultBranchName,
		opctx.User,
		blobBranch,
		baselineTimeStamp,
	)
	if err != nil {
		return err
	}

	return nil
}

// -- FILE LOCAL -------------------------------------------------------------

func addCommit(a *types.Asset, commit *types.AssetCommit) (string, error) {
	commitID := fsutil.GenerateUID(8)
	if a.Commits == nil {
		a.Commits = make(map[string]*types.AssetCommit)
	}
	a.Commits[commitID] = commit
	return commitID, nil
}

func addBranch(
	a *types.Asset,
	commitID, branchName, author, payloadfile string,
	baselineTimeStamp time.Time,
) error {
	if a.Branches == nil {
		a.Branches = make(map[string]*types.AssetCommit)
	}

	if _, ok := a.Branches[branchName]; ok {
		return newErrObjectAlreadyExists(types.NewRefBranch(a.RepoPath, a.AssetPath, branchName))
	}

	commitFrom, err := GetCommit(a, commitID)
	if err != nil {
		return err
	}

	commitBranch := types.AssetCommit{
		Content:      payloadfile,
		Author:       author,
		Message:      "",
		Parent:       commitID,
		Timestamp:    baselineTimeStamp,
		Dependencies: *deepCopyDependencies(&commitFrom.Dependencies),
	}

	a.Branches[branchName] = &commitBranch
	return nil
}

func addTag(a *types.Asset, tag *types.AssetTag, tagFamily, siteUID string) (string, error) {
	version := 1
	if highest, _, err := findHighestVersionnedTag(a, tagFamily, siteUID); err == nil {
		version = highest + 1
	}
	tagName := fmt.Sprintf("%03d", version)

	a.SetTag(tagFamily, siteUID, tagName, tag)
	return tagName, nil
}

func findHighestVersionnedTag(a *types.Asset, tagFamily, siteUID string) (int, string, error) {
	r, err := getTagFamilyPerSite(a, tagFamily, siteUID)
	if err != nil {
		return 0, "", err
	}
	highestStr, highestInt := highestVersion(*r)
	return highestInt, highestStr, nil
}

func highestVersion(versions map[string]*types.AssetTag) (string, int) {
	highestInt := 0
	highestStr := ""
	for key := range versions {
		var version int
		if _, err := fmt.Sscanf(key, "%d", &version); err == nil && version > highestInt {
			highestInt = version
			highestStr = key
		}
	}
	return highestStr, highestInt
}

func getTagFamilyPerSite(
	a *types.Asset,
	tagFamily, siteUID string,
) (*map[string]*types.AssetTag, error) {
	leaf, ok := a.TagVersions(tagFamily, siteUID)
	if !ok {
		return nil, newErrFloatingTagObjectNotFound(a, tagFamily, siteUID)
	}
	return &leaf, nil
}

func pathJSONAsset(repoPath, assetUID string) string {
	return fsutil.PathOS(repoPath, ".vit", "assets", assetUID[:2], assetUID[2:]+".json")
}

func deepCopyDependencies(src *types.AssetDependencies) *types.AssetDependencies {
	var ret types.AssetDependencies
	if len(*src) > 0 {
		ret = make(types.AssetDependencies, len(*src))
		for k, v := range *src {
			cp := *v
			if v.Ref != nil {
				refCp := *v.Ref
				cp.Ref = &refCp
			}
			ret[k] = &cp
		}
	}
	return &ret
}

func newErrObjectIndexNotFound(repoPath, assetPath, uid string) error {
	fullPath := fsutil.PathOS(repoPath, assetPath)
	return types.NewInternalVitError(
		types.ErrDBInternal,
		nil,
		[]string{fmt.Sprintf("asset object index not found for %s at: %s", fullPath, uid)},
		[]any{"repoPath", repoPath, "assetPath", assetPath, "assetUID", uid},
	)
}

func newErrFloatingTagObjectNotFound(a *types.Asset, tagFamily, site string) error {
	return newErrObjectNotFound(types.NewRefTagFloating(a.RepoPath, a.AssetPath, tagFamily, site))
}

func newErrObjectNotFound(ref *types.Ref) error {
	return types.NewVitError(
		types.ErrAssetObjectNotFound,
		[]string{fmt.Sprintf("object not found on asset %s", ref.AbsolutePath())},
		[]any{"ref", ref},
	)
}

func newErrObjectAlreadyExists(ref *types.Ref) error {
	return types.NewVitError(
		types.ErrAssetObjectAlreadyExists,
		[]string{fmt.Sprintf("object already exists on asset %s", ref.AbsolutePath())},
		[]any{"ref", ref},
	)
}
