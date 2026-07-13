package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// /!\ WARNING: vibe coded! probably to rewrite at some point (especially the windows part)

// testHardlinkCreation tests if we can actually create a hardlink
// by doing a quick test with temporary files.
func testHardlinkCreation() (bool, error) {
	// Create a temporary file
	tmpDir := os.TempDir()
	srcPath := filepath.Join(tmpDir, ".vit-hardlink-test-src")
	dstPath := filepath.Join(tmpDir, ".vit-hardlink-test-dst")

	// Clean up in case previous test files exist
	os.Remove(srcPath)
	os.Remove(dstPath)
	defer os.Remove(srcPath)
	defer os.Remove(dstPath)

	// Create a source file
	if err := os.WriteFile(srcPath, []byte("test"), 0o644); err != nil {
		return false, fmt.Errorf("failed to create test file: %w", err)
	}

	// Try to create a hardlink
	err := os.Link(srcPath, dstPath)
	if err != nil {
		return false, err
	}

	// Verify it's actually a hardlink (same inode)
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return false, err
	}

	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		return false, err
	}

	return os.SameFile(srcInfo, dstInfo), nil
}
