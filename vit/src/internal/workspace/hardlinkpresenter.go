package workspace

import (
	"context"
	"os"

	"vit/internal/fsutil"
)

// HardlinkPresenter implements Presenter using hardlinks.
// Creates hardlinks from workspace to blobs, providing zero-copy presentation.
// As we can't hardlinks directories, directories are created by the HardlinkPresenter
// and make them readonly.
//
// Requirements:
// - Workspace and .vit/blob/ must be on the same filesystem
// - On Windows: requires Developer Mode or Administrator privileges
type HardlinkPresenter struct {
	logger interface {
		Debug(msg string, args ...any)
		Info(msg string, args ...any)
	}
}

func newHardlinkPresenter(
	logger interface {
		Debug(msg string, args ...any)
		Info(msg string, args ...any)
	},
) (*HardlinkPresenter, error) {
	ret := HardlinkPresenter{
		logger: logger,
	}

	if err := ret.Check(); err != nil {
		return nil, err
	}

	return &ret, nil
}

func (p *HardlinkPresenter) Check() error {
	return fsutil.CheckHardlinkSupport()
}

func (p *HardlinkPresenter) Present(
	ctx context.Context,
	blobPath, workspacePath string,
	isDir, readOnly bool,
) error {
	if isDir {
		if err := os.MkdirAll(workspacePath, 0o755); err != nil {
			return err
		}
	} else {
		if err := os.Link(blobPath, workspacePath); err != nil {
			return err
		}
		if readOnly {
			if err := fsutil.MakeFileReadOnly(workspacePath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *HardlinkPresenter) Seal(ctx context.Context, root string) error {
	return sealDirsPostOrder(root)
}

func sealDirsPostOrder(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := sealDirsPostOrder(fsutil.PathOS(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return fsutil.MakeDirectoryReadOnly(dir)
}

func (p *HardlinkPresenter) Unpresent(workspacePath string) error {
	parent := fsutil.DirOS(workspacePath)
	if err := fsutil.MakeDirectoryWritable(parent); err != nil {
		return err
	}
	return os.Remove(workspacePath)
}
