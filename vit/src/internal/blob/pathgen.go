package blob

import (
	"fmt"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

// commitBlobPath returns the path for an immutable commit blob.
// Format: .vit/blob/<asset>/commits/<name>-<timestamp><ext>
func commitBlobPath(repoPath, assetPath string, timestamp time.Time) string {
	base := fsutil.BaseInternal(assetPath)
	filename := fmt.Sprintf("%s-%s", base, types.FormatTimestampLocal(timestamp))
	return fsutil.PathOS(repoPath, ".vit", "blob", assetPath, "commits", filename)
}

// branchBlobPath returns the path for a mutable branch blob (staging area).
// Format: .vit/blob/<asset>/branches/<name>-branch=<branch><ext>
func branchBlobPath(repoPath, assetPath, branchName string) string {
	base := fsutil.BaseInternal(assetPath)
	filename := fmt.Sprintf("%s-branch=%s", base, branchName)
	return fsutil.PathOS(repoPath, ".vit", "blob", assetPath, "branches", filename)
}

// manifestPath returns the manifest sidecar path for a given blob path.
func manifestPath(blobPath string) string {
	return blobPath + "_manifest.json"
}
