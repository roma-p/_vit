package fsutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/mod/sumdb/dirhash"
)

// GenerateUID generates a random UID of the specified byte length.
// Panics on failure.
func GenerateUID(byteLength int) string {
	bytes := make([]byte, byteLength)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("FATAL: system entropy unavailable: %v - cannot generate secure UUIDs", err))
	}
	return hex.EncodeToString(bytes)
}

// HashFile returns the SHA256 hash and size of a file.
func HashFile(path string) (hash string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("failed to open file for hashing: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("failed to stat file for hashing: %w", err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", 0, fmt.Errorf("failed to read file for hashing: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), info.Size(), nil
}

// HashDir returns a hash of a directory's contents and its total size.
func HashDir(path string) (hash string, size int64, err error) {
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", 0, fmt.Errorf("failed to resolve symlink %s: %w", path, err)
	}

	hash, err = dirhash.HashDir(realPath, "", dirhash.DefaultHash)
	if err != nil {
		return "", 0, fmt.Errorf("failed to hash directory: %w", err)
	}

	size, err = dirSize(realPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to calculate directory size: %w", err)
	}

	return hash, size, nil
}

func dirSize(path string) (int64, error) {
	var total int64
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		p := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			sub, err := dirSize(p)
			if err != nil {
				return 0, err
			}
			total += sub
		} else {
			info, err := entry.Info()
			if err != nil {
				return 0, err
			}
			total += info.Size()
		}
	}
	return total, nil
}
