package main

import (
	"fmt"
	"os"
	"path/filepath"

	"vit-scripts/build/archive"
	"vit-scripts/build/platform"
)

func FetchVendor() {
	fmt.Println("Fetching vendored binaries...")

	if err := createVendorBinDirs(); err != nil {
		fatal("x  Failed to create directories: %v", err)
	}

	s := true

	fmt.Println()
	fmt.Println("==> Downloading fzf binaries...")
	fmt.Printf("    Using fzf version: %s", fzfVersion)
	fmt.Println()
	fmt.Println()
	if err := downloadAllFZF(); err != nil {
		fmt.Fprintf(os.Stderr, "%s", err.Error())
		s = false
	}

	if !s {
		fatal(" x Failed to download every vendor binary. Only building 'vit-minimal' would be possible.")
	}

	fmt.Println()
	fmt.Println("==> Vendored binaries downloaded!")
	fmt.Println()
}

func createVendorBinDirs() error {
	for _, p := range platform.All {
		dir := vendorDir(p)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		gitkeep := filepath.Join(dir, ".gitkeep")
		if err := os.WriteFile(gitkeep, []byte{}, 0644); err != nil {
			return err
		}
	}
	return nil
}

// FZF is downloaded directly from official repo (which provide binaries!)
func downloadAllFZF() error {
	tmpDir := filepath.Join(os.TempDir(), "vit-vendor-download")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	s := true

	for _, p := range platform.All {
		if err := downloadFZF(p, tmpDir); err != nil {
			s = false
			fmt.Fprintf(os.Stderr, "  x %s/fzf: %s\n", p.String(), err.Error())
		}
		fmt.Printf("  o %s/fzf\n", p.String())
	}

	if s {
		return nil
	}
	return fmt.Errorf("failed downloading every fzf release")
}

func downloadFZF(p platform.Platform, tmpDir string) error {
	baseName := fmt.Sprintf("fzf-%s-%s_%s", fzfVersion, p.OS, p.Arch)
	var archiveType string
	if p.OS == "windows" {
		archiveType = "zip"
	} else {
		archiveType = "tar.gz"
	}

	archiveName := baseName + "." + archiveType
	url := fmt.Sprintf("%s/releases/download/v%s/%s", RepoURLFzf, fzfVersion, archiveName)

	tmpFile := filepath.Join(tmpDir, archiveName)
	if err := downloadFile(url, tmpFile); err != nil {
		return err
	}

	binName := vendorBinName("fzf", p.OS)
	switch archiveType {
	case "zip":
		return archive.ExtractZip(tmpFile, vendorDir(p), binName)
	case "tar.gz":
		return archive.ExtractTarGz(tmpFile, vendorDir(p), binName)
	default:
		panic("unreachable")
	}
}
