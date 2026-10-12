package vendor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// FzfCmd returns a usable fzf binary
func FzfCmd() (string, error) {
	if p, err := exec.LookPath("fzf"); err == nil {
		return p, nil
	}
	if v := vendoredFzfPath(); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v, nil
		}
	}
	return "", fmt.Errorf("fzf not found: install fzf or use a vendored vit build")
}

func vendoredFzfPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	bin := "fzf"
	if runtime.GOOS == "windows" {
		bin = "fzf.exe"
	}
	return filepath.Join(filepath.Dir(exe), "vendor-bins", runtime.GOOS+"-"+runtime.GOARCH, bin)
}
