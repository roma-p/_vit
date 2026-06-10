package blob

import (
	"encoding/json"
	"os"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

type manifest struct {
	Timestamp time.Time         `json:"timestamp"`
	Ref       *types.Ref        `json:"ref"`
	User      string            `json:"user"`
	Sources   map[string]string `json:"sources,omitempty"`
}

func writeManifest(
	blobPath string,
	ref *types.Ref,
	user string,
	timestamp time.Time,
	sources map[string]string,
) error {
	m := &manifest{
		Timestamp: timestamp,
		Ref:       ref,
		User:      user,
		Sources:   sources,
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	path := manifestPath(blobPath)
	if err := os.MkdirAll(fsutil.DirOS(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
