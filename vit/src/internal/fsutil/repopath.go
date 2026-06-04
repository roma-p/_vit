package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CheckPathIsVitRepo checks if the given path contains a .vit directory.
func CheckPathIsVitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".vit"))
	if err != nil {
		return false
	}
	return info.IsDir()
}

// FindVitRepoFromPath walks up the directory tree from path to find a vit repository.
// Returns (repoPath, relativePath, error).
// If ignoreNonExisting is true, it won't check if path exists before walking up.
func FindVitRepoFromPath(path string, ignoreNonExisting bool) (string, string, error) {
	if !ignoreNonExisting {
		if _, err := os.Stat(path); err != nil {
			return "", "", fmt.Errorf("no vit repo path found from: %s", path)
		}
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("failed to get absolute path: %s: %w", path, err)
	}

	currpath := absPath
	for {
		if currpath == string(filepath.Separator) || currpath == "." {
			break
		}
		if CheckPathIsVitRepo(currpath) {
			var relativePath string
			if absPath == currpath {
				relativePath = ""
			} else {
				relativePath = strings.TrimPrefix(absPath, currpath+string(filepath.Separator))
			}
			return currpath, relativePath, nil
		}
		currpath = filepath.Dir(currpath)
	}

	return "", "", fmt.Errorf("no vit repo path found from: %s", path)
}
