package blob

import (
	"fmt"
	"strings"
	"time"

	"vit/internal/fsutil"
)

// commitBlobPath returns the path for an immutable commit blob.
// Format: .vit/blob/<asset>/commits/<name>-<timestamp><ext>
func commitBlobPath(repoPath, assetPath string, timestamp time.Time) string {
	name, ext := splitNameExt(assetPath)
	filename := fmt.Sprintf("%s-%s%s", name, fsutil.FormatTimestamp(timestamp), ext)
	return fsutil.PathOS(repoPath, ".vit", "blob", assetPath, "commits", filename)
}

// branchBlobPath returns the path for a mutable branch blob (staging area).
// Format: .vit/blob/<asset>/branches/<name>-branch=<branch><ext>
func branchBlobPath(repoPath, assetPath, branchName string) string {
	name, ext := splitNameExt(assetPath)
	filename := fmt.Sprintf("%s-branch=%s%s", name, branchName, ext)
	return fsutil.PathOS(repoPath, ".vit", "blob", assetPath, "branches", filename)
}

// manifestPath returns the manifest sidecar path for a given blob path.
func manifestPath(blobPath string) string {
	return blobPath + "_manifest.json"
}

func splitNameExt(assetPath string) (name, ext string) {
	base := fsutil.BaseInternal(assetPath)
	ext = fsutil.ExtInternal(base)
	name = strings.TrimSuffix(base, ext)
	return name, ext
}
