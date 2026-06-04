package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SyncDir syncs a directory to ensure the directory entry
// is persisted to disk (important for NFS).
func SyncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// RemoveFileAtomic removes a file by first renaming it to a temp name,
// then deleting. This avoids a window where the path is half-deleted.
func RemoveFileAtomic(filename string) error {
	dir := filepath.Dir(filename)
	base := filepath.Base(filename)
	tmpFile := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%d", base, time.Now().UnixNano()))

	if err := os.Rename(filename, tmpFile); err != nil {
		return fmt.Errorf("failed to move file to temp location: %w", err)
	}
	if err := os.Remove(tmpFile); err != nil {
		return fmt.Errorf("failed to remove temp file: %w", err)
	}
	return nil
}
