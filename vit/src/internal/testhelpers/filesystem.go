package testutils

import (
	"os"
	"path/filepath"
	"testing"
)

func TempDir(t *testing.T, pattern string) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", pattern)
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	cleanup := func() {
		// Make all directories writable before cleanup (in case they were made read-only)
		filepath.WalkDir(tempDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // Skip errors during walk
			}
			if d.IsDir() {
				os.Chmod(path, 0755) // Make writable, ignore errors
			}
			return nil
		})

		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to cleanup temp dir %s: %v", tempDir, err)
		}
	}
	return tempDir, cleanup
}

func CreateDirectories(t *testing.T, basePath string, dirs []string) {
	t.Helper()
	for _, dir := range dirs {
		fullPath := filepath.Join(basePath, dir)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			t.Errorf("Failed to create test directory %s: %v", basePath, err)
		}
	}
}

func CreateFile(t *testing.T, filePath string, content string) {
	t.Helper()
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file %s: %v", filePath, err)
	}
}
