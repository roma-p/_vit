package blob

import (
	"encoding/json"
	"os"

	"vit/internal/fsutil"
	"vit/internal/types"
)

func writeManifest(commitBlobPath string, m types.Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	path := manifestPath(commitBlobPath)
	if err := os.MkdirAll(fsutil.DirOS(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
