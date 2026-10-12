package workspace

import (
	"context"
	"os"
	"path/filepath"

	"vit/internal/fsutil"
)

// HardlinkPresenter implements Presenter using hardlinks.
// Creates hardlinks from workspace to blobs, providing zero-copy presentation.
// As we can't hardlinks directories, directories are created by the HardlinkPresenter
// and make them readonly.
// Sources and products are simlinked to their workspace target
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

func (p *HardlinkPresenter) Alias(ctx context.Context, target, link string) error {
	if err := os.MkdirAll(fsutil.DirOS(link), 0o755); err != nil {
		return err
	}
	rel, err := filepath.Rel(fsutil.DirOS(link), target)
	if err != nil {
		return err
	}
	return os.Symlink(rel, link)
}

func (p *HardlinkPresenter) Seal(ctx context.Context, root string) error {
	fi, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	return sealDirsPostOrder(root)
}

func sealDirsPostOrder(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}
		if e.IsDir() {
			if err := sealDirsPostOrder(fsutil.PathOS(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return fsutil.MakeDirectoryReadOnly(dir)
}

// Unpresent removes whatever is presented at workspacePath — a file, an alias
// (symlink), or a whole hardlink subtree — making it writable first. A symlink is
// removed as a link, never followed.
func (p *HardlinkPresenter) Unpresent(workspacePath string) error {
	parent := fsutil.DirOS(workspacePath)
	if err := fsutil.MakeDirectoryWritable(parent); err != nil && !os.IsNotExist(err) {
		return err
	}
	fi, err := os.Lstat(workspacePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink == 0 && fi.IsDir() {
		if err := makeTreeWritable(workspacePath); err != nil {
			return err
		}
	}
	return os.RemoveAll(workspacePath)
}

func makeTreeWritable(dir string) error {
	if err := fsutil.MakeDirectoryWritable(dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := makeTreeWritable(fsutil.PathOS(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
