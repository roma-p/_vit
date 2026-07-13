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

// Check : here just check that hardlink are supported (might not be the case on windows).
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

// PresentFinish seals a directory read-only once all its contents are linked in.
// Must be called post-order: once a dir is read-only, nothing more can be created
// inside it. A mutable (branch) presentation leaves the dir writable.
func (p *HardlinkPresenter) PresentFinish(
	ctx context.Context,
	workspacePath string,
	readOnly bool,
) error {
	if !readOnly {
		return nil
	}
	return fsutil.MakeDirectoryReadOnly(workspacePath)
}

// Unpresent removes a single presented file from the workspace. The parent dir
// is made writable first, since an immutable presentation seals it read-only.
// Tree teardown removes the emptied directories separately.
func (p *HardlinkPresenter) Unpresent(workspacePath string) error {
	parent := fsutil.DirOS(workspacePath)
	if err := fsutil.MakeDirectoryWritable(parent); err != nil {
		return err
	}
	return os.Remove(workspacePath)
}
