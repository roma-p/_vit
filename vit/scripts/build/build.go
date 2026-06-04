package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func Build() {
	platform, err := getCurrentPlatform()
	if err != nil {
		fatal("Build Failed: %v", err)
	}

	_, found := vendorBinPathPerPlatform("fzf", platform)
	if found {
		fmt.Println("o Vendor-bins found - building both versions")
		fmt.Println()
		buildPortable()
		fmt.Println()
		buildMinimal()
		fmt.Println()
		fmt.Println("Both versions built successfully!")
		fmt.Println()
	} else {
		fmt.Println("Vendor-bins not found - building only minimal version")
		fmt.Println()
		fmt.Println("Note: To build portable version, download vendor bin using: 'make fetch-vendor'")
		fmt.Println()
		buildMinimal()
		fmt.Println()
	}
}

func buildMinimal() {
	fmt.Println("Building VIT Minimal (relying on system dependencies)...")
	binaryPath := filepath.Join(buildPathMinimal, "vit")
	goBuild(binaryPath)
	fmt.Println(" o Built: bin/vit-minimal")
	generatePythonAPI(binaryPath)
}

func buildPortable() {
	fmt.Println("Building VIT (with vendored dependancies)...")
	platform, err := getCurrentPlatform()
	if err != nil {
		fatal("build failed: %v", err)
	}

	fzfPath, found := vendorBinPathPerPlatform("fzf", platform)

	if !found {
		fmt.Println()
		fmt.Println("x Error: Vendored binaries not found!")
		fmt.Println()
		fmt.Println("To fetch vendor binaries: 'make fetch-vendor'")
		fmt.Println()
		return
	}

	if err := CopyFile(fzfPath, vendorBinBuildPathPerPlatform("fzf", *platform)); err != nil {
		fatal("error copying bin: %v", err)
	}

	binaryPath := filepath.Join(buildPathPortable, "vit")
	goBuild(binaryPath)

	fmt.Println(" o Built: bin/vit")
	fmt.Println("   (uses vendor-bins")

	generatePythonAPI(binaryPath)
}

func generatePythonAPI(binaryPath string) {
	fmt.Println(" o Generated: vit.py")
	outputPath := filepath.Join(filepath.Dir(binaryPath), "vit.py")

	cmd := exec.Command(binaryPath, "dev", "genpy", outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fatal("Failed to generate Python API: %v", err)
	}
}

func goBuild(outputPath string) {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fatal("Build failed: %v", err)
	}
	cmd := exec.Command("go", "build", "-C", "src", "-o", "../"+outputPath, "./cmd/vit")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("Build failed: %v", err)
	}
}
