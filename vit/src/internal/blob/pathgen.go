package blob

import (
	"fmt"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

func commitBlobPath(repoPath, assetPath, branch, user string, timestamp time.Time) string {
	base := fsutil.BaseInternal(assetPath)
	filename := fmt.Sprintf("%s-%s-%s-%s-%s",
		base, branch, types.FormatTimestampLocal(timestamp), user, fsutil.GenerateUID(2))
	return fsutil.PathOS(repoPath, ".vit", "blob", assetPath, "commits", filename)
}

func branchBlobPath(repoPath, assetPath, branchName string) string {
	base := fsutil.BaseInternal(assetPath)
	filename := fmt.Sprintf("%s-branch=%s", base, branchName)
	return fsutil.PathOS(repoPath, ".vit", "blob", assetPath, "branches", filename)
}

func manifestPath(blobPath string) string {
	return blobPath + "_manifest.json"
}
