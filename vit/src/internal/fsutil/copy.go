package fsutil

import (
	"context"
	"fmt"
	"io"
	"os"

	"vit/internal/types"
)

type CopyFilesRequest struct {
	DstRoot string
	Src     *types.DirStat
}

func CopyFiles(ctx context.Context, progress *CopyProgress, requests []CopyFilesRequest) (err error) {
	// TODO: is this really a panic case?
	if progress == nil {
		panic("BUG: CopyFiles received an empty progress!")
	}

	var totalItems int
	var totalBytes int64
	for _, req := range requests {
		for _, stat := range req.Src.Data {
			totalItems++
			totalBytes += stat.Size
		}
	}
	progress.init("Copying", totalItems, totalBytes)
	defer func() { progress.close(err == nil) }()

	for _, req := range requests {
		if err = copyOneRequest(ctx, req.DstRoot, req.Src, progress); err != nil {
			return err
		}
	}
	return nil
}

func copyOneRequest(ctx context.Context, dstRoot string, src *types.DirStat, progress *CopyProgress) error {
	dstRoot = PathOS(dstRoot)

	stop, err := AcquireExclusiveLockWithKeepalive(ctx, dstRoot)
	if err != nil {
		return err
	}
	defer stop()

	dstParent := DirOS(dstRoot)
	if err := os.MkdirAll(dstParent, 0o755); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp(dstParent, ".copy-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	srcRoot := PathOS(src.RootAbsPath)
	for rel, stat := range src.Data {
		if err := ctx.Err(); err != nil {
			cleanup()
			return err
		}
		srcPath := PathOS(srcRoot, rel)
		dstPath := PathOS(tmpDir, rel)
		if err := os.MkdirAll(DirOS(dstPath), 0o755); err != nil {
			cleanup()
			return err
		}
		progress.updateFile(BaseOS(srcPath), int(stat.Size))
		if err := copyFileDirect(ctx, srcPath, dstPath, progress); err != nil {
			cleanup()
			return err
		}
		if stat.Size == 0 {
			progress.finishFile() // zero-byte files never trigger updateBytes
		}
	}

	if err := syncDir(tmpDir); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpDir, dstRoot); err != nil {
		cleanup()
		return err
	}

	_ = syncDir(dstParent)
	return nil
}

type CopyProgress struct {
	writer      types.ProgressWriter
	currentFile string
	currentSize int
	bytesRead   int
}

func NewCopyProgress(writer types.ProgressWriter) *CopyProgress {
	return &CopyProgress{writer: writer}
}

func (p *CopyProgress) init(operation string, totalItems int, totalBytes int64) {
	p.writer.InitProgress(types.ProgressManifest{
		Operation:  operation,
		TotalItems: totalItems,
		TotalBytes: totalBytes,
	})
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
		Size: p.currentSize,
		Done: p.bytesRead,
	})
}

func (p *CopyProgress) finishFile() {
	p.writer.UpdateProgress(types.ProgressItem{
		Name: p.currentFile,
		Size: p.currentSize,
		Done: p.currentSize,
	})
}

func (p *CopyProgress) close(success bool) {
	p.writer.CloseProgress(types.ProgressFinish{Status: success})
}

func copyFileDirect(ctx context.Context, src, dst string, progress *CopyProgress) error {
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

	reader := &progressReader{ctx: ctx, reader: srcFile, progress: progress}

	if _, err := io.Copy(dstFile, reader); err != nil {
		return err
	}

	return dstFile.Sync()
}

type progressReader struct {
	ctx      context.Context
	reader   io.Reader
	progress *CopyProgress
}

func (pr *progressReader) Read(p []byte) (int, error) {
	if err := pr.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.progress.updateBytes(n)
	}
	return n, err
}
