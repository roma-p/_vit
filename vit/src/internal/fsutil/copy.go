package fsutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"vit/internal/types"
)

// CopyFile copies a file as atomicaly as you can on NFS,
// with exclusive locking and progress reporting.
// Read the file twice: Pass 1: hash + size. Pass 2: copy with progress via temp file + rename.
func CopyFile(ctx context.Context, src, dst string, progress *CopyProgress) (hash string, size int64, err error) {
	stop, err := AcquireExclusiveLockWithKeepalive(ctx, dst)
	if err != nil {
		return "", 0, err
	}
	defer stop()

	// Pass 1: hash + size
	hash, size, err = HashFile(src)
	if err != nil {
		return "", 0, err
	}

	progress.init("Copying")
	defer func() { progress.close(err == nil) }()

	// Pass 2: copy
	if err = copyFileAtomic(src, dst, size, progress); err != nil {
		return "", 0, err
	}

	return hash, size, nil
}

// CopyDir does the same as CopyFile but with a directory.
func CopyDir(ctx context.Context, src, dst string, progress *CopyProgress) (hash string, size int64, err error) {
	stop, err := AcquireExclusiveLockWithKeepalive(ctx, dst)
	if err != nil {
		return "", 0, err
	}
	defer stop()

	// Pass 1: hash + size
	hash, size, err = HashDir(src)
	if err != nil {
		return "", 0, err
	}

	progress.init("Copying")
	defer func() { progress.close(err == nil) }()

	// Pass 2: copy to temp dir, then atomic rename
	dstParent := filepath.Dir(dst)
	tmpDir, err := os.MkdirTemp(dstParent, ".copy-tmp-*")
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp dir: %w", err)
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	if err = copyDirContents(src, tmpDir, progress); err != nil {
		cleanup()
		return "", 0, err
	}

	if err = SyncDir(tmpDir); err != nil {
		cleanup()
		return "", 0, err
	}

	// Preserve source permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		cleanup()
		return "", 0, err
	}
	if err = os.Chmod(tmpDir, srcInfo.Mode()); err != nil {
		cleanup()
		return "", 0, err
	}

	if err = os.Rename(tmpDir, dst); err != nil {
		cleanup()
		return "", 0, err
	}

	_ = SyncDir(dstParent)
	return hash, size, nil
}

// CopyProgress wraps a ProgressWriter for copy operations.
// Use this to bind progress communication!
type CopyProgress struct {
	writer      types.ProgressWriter
	currentFile string
	currentSize int
	bytesRead   int
}

func NewCopyProgress(writer types.ProgressWriter) *CopyProgress {
	return &CopyProgress{writer: writer}
}

func (p *CopyProgress) init(operation string) {
	p.writer.InitProgress(types.ProgressManifest{Operation: operation})
}

func (p *CopyProgress) updateFile(name string, size int) {
	p.currentFile = name
	p.currentSize = size
	p.bytesRead = 0
}

func (p *CopyProgress) updateBytes(n int) {
	p.bytesRead += n
	p.writer.UpdateProgress(types.ProgressItem{
		Name: p.currentFile,
		Size: p.bytesRead,
	})
}

func (p *CopyProgress) close(success bool) {
	p.writer.CloseProgress(types.ProgressFinish{Status: success})
}

func copyFileAtomic(src, dst string, size int64, progress *CopyProgress) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	progress.updateFile(filepath.Base(src), int(size))

	return WriteFileAtomic(dst, srcInfo.Mode(), func(f *os.File) error {
		reader := &progressReader{reader: srcFile, progress: progress}
		_, err := io.Copy(f, reader)
		return err
	})
}

func copyDirContents(src, dst string, progress *CopyProgress) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirContents(srcPath, dstPath, progress); err != nil {
				return err
			}
		} else {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			progress.updateFile(entry.Name(), int(info.Size()))
			if err := copyFileDirect(srcPath, dstPath, progress); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFileDirect(src, dst string, progress *CopyProgress) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	reader := &progressReader{reader: srcFile, progress: progress}

	if _, err := io.Copy(dstFile, reader); err != nil {
		return err
	}

	return dstFile.Sync()
}

type progressReader struct {
	reader   io.Reader
	progress *CopyProgress
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.progress.updateBytes(n)
	}
	return n, err
}
