package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"vit-scripts/build/platform"
)

// DownloadFile downloads a file from a URL to a destination path
func DownloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return os.Chmod(dest, 0755)
}

// GetCurrentPlatform returns the current platform
func GetCurrentPlatform() (*platform.Platform, error) {
	currentOS := runtime.GOOS
	currentArch := runtime.GOARCH

	for _, p := range platform.All {
		if p.OS == currentOS && p.Arch == currentArch {
			return &p, nil
		}
	}

	return nil, fmt.Errorf("unsupported platform: %s-%s", currentOS, currentArch)
}

// CopyFile copies a file from source to destination
func CopyFile(sourcePath, destinationPath string) error {
	src, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	destinationDir := filepath.Dir(destinationPath)
	if err := os.MkdirAll(destinationDir, 0755); err != nil {
		return err
	}

	dst, err := os.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

// CopyDir recursively copies a directory
func CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return CopyFile(path, dstPath)
	})
}
