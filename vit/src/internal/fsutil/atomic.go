package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// syncDir syncs a directory to ensure the directory entry
// is persisted to disk (important for NFS).
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// WriteFileAtomic writes to filename via a temp file + sync + rename.
// Concurrent readers never should see partial data since rename is supposed atomic.
// (but that is not entrerly true on nfs)
// Creates parent directories if needed.
func WriteFileAtomic(filename string, mode os.FileMode, write func(f *os.File) error) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if err := write(tmpFile); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Chmod(tmpPath, mode); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}

	if err := os.Rename(tmpPath, filename); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// Flush directory entry so NFS clients see the new file.
	_ = syncDir(dir)

	return nil
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
