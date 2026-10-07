package workspace

import (
	"vit/internal/fsutil"
	"vit/internal/types"
)

func RefToWorkspacePath(ref *types.Ref) string {
	// date only needed for commit.

	switch ref.RefType {
	case types.RefTypeCommit:
		panic("use genWorkspacePathForCommit for commit")
	case types.RefTypeTag:
		return genWorkspacePathForTag(ref.AssetPath, ref.TagFamily, ref.TagName)
	case types.RefTypeBranch:
		return genWorkspacePathForBranch(ref.AssetPath, ref.BranchName)
	default:
		panic("unreachable")
	}
}

func genWorkspacePathForBranch(assetPath, branch string) string {
	return fsutil.PathInternal(
		assetPath,
		"branches",
		"branch-"+branch,
	)
}

func genWorkspacePathForCommit(assetPath, date, srcBranchName, user, commitID string) string {
	return fsutil.PathInternal(
		assetPath,
		"snapshots",
		"snapshot-"+srcBranchName+"-"+types.SnapshotKeyFrom(date, user, commitID),
	)
}

func genWorkspacePathForTag(assetPath, tagfamily, tagname string) string {
	return fsutil.PathInternal(
		assetPath,
		"tags",
		tagfamily,
		tagfamily+"-"+tagname,
	)
}
