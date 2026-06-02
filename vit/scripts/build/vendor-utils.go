
package main

import (
	"os"
	"path/filepath"

	"vit-scripts/build/platform"
)

func vendorBinPath(binaryName string, p platform.Platform) string {
	return filepath.Join(vendorDir(p), vendorBinName(binaryName, p.OS))
}

func vendorBinName(binaryName string, os string) string {
	if os == "windows" {
		return binaryName + ".exe"
	}
	return binaryName
}

func vendorDir(p platform.Platform) string {
	return filepath.Join("vendor-bins", p.String())

}

func vendorBinPathPerPlatform(binaryName string, p *platform.Platform) (string, bool) {
	path := vendorBinPath(binaryName, *p)
	_, err := os.Stat(path)
	exists := err == nil
	return path, exists
}

func vendorBinBuildPathPerPlatform(binaryName string, p platform.Platform) string {
	return filepath.Join(
		buildPathPortable,
		"vendor-bins",
		p.String(),
		vendorBinName(binaryName, p.OS),
	)
}
