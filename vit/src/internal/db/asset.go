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
		Content:      r.RootcontentNodeUID,
		Author:       r.Author,
		Site:         r.Site,
		Message:      r.Message,
		Parent:       branchCommit.Parent,
		Timestamp:    r.TimeStamp,
		Dependencies: *deepCopyDependencies(&branchCommit.Dependencies),
		IsSnapshot:   r.IsSnapshot,
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
	IsSnapshot         bool
	// TODO dependancies
}

func GetCommit(a *types.Asset, commitID string) (*types.AssetCommit, error) {
	if a.Commits == nil {
		return nil, newErrObjectNotFound(types.NewRefCommit(a.RepoRef(), a.AssetPath, commitID))
	}

	commitObj, ok := a.Commits[commitID]
	if !ok {
		return nil, newErrObjectNotFound(types.NewRefCommit(a.RepoRef(), a.AssetPath, commitID))
	}

	return commitObj, nil
}

func GetBranchCommit(a *types.Asset, branchName string) (*types.AssetCommit, error) {
	if a.Branches == nil {
		return nil, newErrObjectNotFound(types.NewRefBranch(a.RepoRef(), a.AssetPath, branchName))
	}
	commitObj, ok := a.Branches[branchName]
	if !ok {
		return nil, newErrObjectNotFound(types.NewRefBranch(a.RepoRef(), a.AssetPath, branchName))
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
				types.NewRefTag(a.RepoRef(), a.AssetPath, tagFamily, version, siteUID),
			)
		}
	}
	return nil
}

func GetDependencyByName(
	srcRef *types.Ref,
	c *types.AssetCommit,
	dependencyName string,
) (*types.AssetDependency, error) {
	ret, ok := c.Dependencies[dependencyName]
	if !ok {
		return nil, newErrDependencyNotFound(srcRef, dependencyName)
	} else {
		return ret, nil
	}
}

func CheckDependencyNameAvailable(
	ref *types.Ref,
	c *types.AssetCommit,
	dependencyName string,
) error {
	if _, ok := c.Dependencies[dependencyName]; ok {
		return newErrDependencyAlreadyExists(ref, dependencyName)
	}
	return nil
}

func ResolveCommitForDependency(a *types.Asset, ref *types.Ref) (*types.Ref, string, error) {
	var commitID string
	var outRef *types.Ref

	switch ref.RefType {
	// if ref is commit: just check commit exists and returns its input.
	case types.RefTypeCommit:
		if ref.CommitID == "" && ref.SnapshotKey != "" {
			id, err := resolveSnapshotKey(a, ref.SnapshotKey)
			if err != nil {
				return nil, "", err
			}
			commitID = id
		} else {
			commitID = ref.CommitID
		}
		outRef = types.NewRefCommit(ref.RepoRef(), ref.AssetPath, commitID)

	// if ref is tag: outRef is the non floating tag ref, commitID is the underlying commit.
	case types.RefTypeTag:
		tag, tagName, err := GetTag(a, ref)
		if err != nil {
			return nil, "", err
		}
		commitID = tag.Commit
		outRef = types.NewRefTag(ref.RepoRef(), ref.AssetPath, ref.TagFamily, tagName, ref.Site)

	// if ref is branch: outRef is the last commit on branch, commitId is its commit id.
	case types.RefTypeBranch:
		branchCommit, err := GetBranchCommit(a, ref.BranchName)
		if err != nil {
			return nil, "", err
		}

		commitID = branchCommit.Parent
		outRef = types.NewRefCommit(ref.RepoRef(), ref.AssetPath, commitID)
	}

	if _, err := GetCommit(a, commitID); err != nil {
		// TODO, maybre wrap this in a better error...
		return nil, "", err
	}

	return outRef, commitID, nil
}

func AddSource(r AddSourceRequest) error {
	branchCommit, err := GetBranchCommit(r.Asset, r.BranchName)
	if err != nil {
		return err
	}

	if branchCommit.Dependencies == nil {
		branchCommit.Dependencies = make(types.AssetDependencies)
	}
	branchCommit.Dependencies[r.SourceName] = &types.AssetDependency{
		Type:        types.AssetDependencySource,
		RefSpec:     r.RefSpec,
		RefResolved: r.RefResolved,
		Commit:      r.CommitID,
	}
	return nil
}

type AddSourceRequest = struct {
	Asset       *types.Asset
	BranchName  string
	SourceName  string
	RefSpec     *types.Ref
	RefResolved *types.Ref
	CommitID    string
}

func AddProduct(r AddProductRequest) error {
	branchCommit, err := GetBranchCommit(r.Asset, r.BranchName)
	if err != nil {
		return err
	}

	if branchCommit.Dependencies == nil {
		branchCommit.Dependencies = make(types.AssetDependencies)
	}
	branchCommit.Dependencies[r.ProductName] = &types.AssetDependency{
		Type:    types.AssetDependencyProduct,
		RefSpec: r.RefSpec,
	}
	return nil
}

type AddProductRequest = struct {
	Asset       *types.Asset
	BranchName  string
	ProductName string
	RefSpec     *types.Ref
}

// -- USED ACROSS vit.internal.DB --------------------------------------------

func createNewJSONAsset(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoRef *types.RepoRef,
	assetPath string, // TODO: change repo Path with refREPO
) (*JSONAsset, error) {
	uid := fsutil.GenerateUID(16)
	return fsutil.ResolveHandler(
		ctx,
		opctx.JSONPool,
		pathJSONAsset(repoRef.Path, uid),
		true,
		&types.Asset{
			AssetUID: uid,

			// comodity, not exported written in the json, and not source of truth
			// just to avoid having to carry multiple data object.
			AssetPath: assetPath,
			RepoPath:  repoRef.Path,
			RepoUID:   repoRef.UID,
			Site:      repoRef.Site,
		},
	)
}

func resolveJSONAsset(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	repoRef *types.RepoRef,
	assetPath, uid string, // asset path just used for formatting
	forWrite bool,
) (*JSONAsset, bool, error) {
	jsonPath := pathJSONAsset(repoRef.Path, uid)
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return nil, false, newErrObjectIndexNotFound(repoRef.Path, assetPath, uid)
	}
	ret, err := fsutil.ResolveHandler[types.Asset](ctx, opctx.JSONPool, jsonPath, forWrite, nil)
	if err != nil {
		return nil, false, err
	}

	// commodity, not persisted in json, not source of truth
	// just to avoid having to carry multiple data object.
	ret.Data.AssetPath = assetPath
	ret.Data.RepoPath = repoRef.Path
	ret.Data.RepoUID = repoRef.UID
	ret.Data.Site = repoRef.Site

	// same for every ref in dependencies.
	fillDependencyRepoPaths(opctx, ret.Data)

	return ret, true, nil
}

func fillDependencyRepoPaths(opctx *opcontext.OperationContext, asset *types.Asset) {
	// repopath is not filled in the json, so we need to feed it from site uid / opctx
	fill := func(ref *types.Ref) {
		if ref != nil {
			ref.RepoPath = opctx.RepoRefFor(ref.RepoUID).Path
		}
	}
	for _, commits := range []map[string]*types.AssetCommit{asset.Commits, asset.Branches} {
		for _, c := range commits {
			for _, dep := range c.Dependencies {
				fill(dep.RefSpec)
				fill(dep.RefResolved)
			}
		}
	}
}

func initAsset(
	opctx *opcontext.OperationContext,
	asset *types.Asset,
	blobBranch, contentRootHash string,
	baselineTimeStamp time.Time,
) error {
	commitFirst := types.AssetCommit{
		Site:       opctx.RepoRefFor(asset.RepoUID).Site,
		Author:     opctx.User,
		Message:    "Asset creation",
		Timestamp:  opctx.TimeStamp,
		Parent:     "",
		Content:    contentRootHash,
		IsSnapshot: true, // the add baseline is the asset's first snapshot
	}

	commitID, err := addCommit(asset, &commitFirst)
	if err != nil {
		return err
	}

	err = AddBranch(
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

func resolveSnapshotKey(a *types.Asset, key string) (string, error) {
	var matches []string
	for id, c := range a.Commits {
		if c.IsSnapshot && c.SnapshotKey(id) == key {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", types.NewVitError(
			types.ErrInvalidRef,
			[]string{"no snapshot matches: " + key},
			[]any{"snapshot", key},
		)
	default:
		return "", types.NewVitError(
			types.ErrInvalidRef,
			[]string{"ambiguous snapshot (more than one at that timestamp): " + key},
			[]any{"snapshot", key, "count", len(matches)},
		)
	}
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

// AddBranch creates a new branch entry on an existing asset, rooted at commitID
// Errors if branchName already exists.
func AddBranch(
	a *types.Asset,
	commitID, branchName, author, payloadfile string,
	baselineTimeStamp time.Time,
) error {
	if a.Branches == nil {
		a.Branches = make(map[string]*types.AssetCommit)
	}

	if _, ok := a.Branches[branchName]; ok {
		return newErrObjectAlreadyExists(types.NewRefBranch(a.RepoRef(), a.AssetPath, branchName))
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
			if v.RefSpec != nil {
				refCp := *v.RefSpec
				cp.RefSpec = &refCp
			}
			if v.RefResolved != nil {
				refCp := *v.RefResolved
				cp.RefResolved = &refCp
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
	return newErrObjectNotFound(types.NewRefTagFloating(a.RepoRef(), a.AssetPath, tagFamily, site))
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

func newErrDependencyNotFound(ref *types.Ref, dependencyName string) error {
	return types.NewVitError(
		types.ErrDependencyNotFound,
		[]string{fmt.Sprintf(
			"no dependency named '%s' found at %s",
			dependencyName,
			ref.AbsolutePath(),
		)},
		[]any{"ref", ref, "name", dependencyName},
	)
}

func newErrDependencyAlreadyExists(ref *types.Ref, dependencyName string) error {
	return types.NewVitError(
		types.ErrDependencyAlreadyExists,
		[]string{fmt.Sprintf(
			"a dependency named '%s' already exists at %s",
			dependencyName,
			ref.AbsolutePath(),
		)},
		[]any{"ref", ref, "name", dependencyName},
	)
}
