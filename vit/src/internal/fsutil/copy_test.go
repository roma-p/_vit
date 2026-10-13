package fsutil

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

// testProgressWriter records all progress events for assertions.
type testProgressWriter struct {
	mu        sync.Mutex
	inited    bool
	operation string
	updates   []types.ProgressItem
	closed    bool
	success   bool
}

func (w *testProgressWriter) InitProgress(m types.ProgressManifest) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.inited = true
	w.operation = m.Operation
}

func (w *testProgressWriter) UpdateProgress(item types.ProgressItem) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.updates = append(w.updates, item)
}

func (w *testProgressWriter) CloseProgress(f types.ProgressFinish) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	w.success = f.Status
}

// dirStat walks src and returns its DirStat (fails the test on error).
func dirStat(t *testing.T, src string) *types.DirStat {
	t.Helper()
	ds, err := NewDirStatFromDir(context.Background(), src)
	testutils.AssertNoError(t, err)
	return ds
}

// copyFilesOne is a single-request CopyFiles, convenient for tests.
func copyFilesOne(ctx context.Context, dst string, src *types.DirStat, p *CopyProgress) error {
	return CopyFiles(ctx, p, []types.CopyFilesRequest{{DstRoot: dst, Src: []*types.DirStat{src}}})
}

func TestCopyFiles_Basic(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755)
	testutils.CreateFile(t, filepath.Join(srcDir, "a.txt"), "file a")
	testutils.CreateFile(t, filepath.Join(srcDir, "sub", "b.txt"), "file b")

	pw := &testProgressWriter{}
	err := copyFilesOne(context.Background(), dstDir, dirStat(t, srcDir), NewCopyProgress(pw))
	testutils.AssertNoError(t, err)

	content, err := os.ReadFile(filepath.Join(dstDir, "a.txt"))
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, string(content), "file a")

	content, err = os.ReadFile(filepath.Join(dstDir, "sub", "b.txt"))
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, string(content), "file b")

	// Progress was used
	testutils.AssertTrue(t, pw.inited)
	testutils.AssertEqual(t, pw.operation, "Copying")
	testutils.AssertTrue(t, len(pw.updates) > 0)
	testutils.AssertTrue(t, pw.closed)
	testutils.AssertTrue(t, pw.success)

	// Lock dir cleaned up
	testutils.AssertNotExists(t, filepath.Join(tmpDir, ".dst.lock"))
}

func TestCopyFiles_PreservesFilePermissions(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	os.MkdirAll(srcDir, 0o755)

	script := filepath.Join(srcDir, "script.sh")
	testutils.CreateFile(t, script, "#!/bin/bash")
	os.Chmod(script, 0o755)

	err := copyFilesOne(context.Background(), dstDir, dirStat(t, srcDir), NewCopyProgress(&testProgressWriter{}))
	testutils.AssertNoError(t, err)

	info, err := os.Stat(filepath.Join(dstDir, "script.sh"))
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, info.Mode().Perm(), os.FileMode(0o755))
}

func TestCopyFiles_SrcFileMissing(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	// A DirStat that references a file which doesn't exist on disk.
	ds := types.NewDirStat(filepath.Join(tmpDir, "src"))
	ds.Data["ghost.txt"] = types.FileStat{Size: 3}

	err := copyFilesOne(context.Background(), filepath.Join(tmpDir, "dst"), ds, NewCopyProgress(&testProgressWriter{}))
	testutils.AssertError(t, err)
}

func TestCopyFiles_LargeFile(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	os.MkdirAll(srcDir, 0o755)

	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(srcDir, "large.bin"), data, 0o644)

	pw := &testProgressWriter{}
	err := copyFilesOne(context.Background(), dstDir, dirStat(t, srcDir), NewCopyProgress(pw))
	testutils.AssertNoError(t, err)

	copied, err := os.ReadFile(filepath.Join(dstDir, "large.bin"))
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, len(copied), len(data))

	// Multiple progress updates, last one cumulative to file size.
	testutils.AssertTrue(t, len(pw.updates) > 1)
	lastUpdate := pw.updates[len(pw.updates)-1]
	testutils.AssertEqual(t, lastUpdate.Done, 1024*1024)
}

func TestCopyFiles_ProgressTracksEachFile(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	os.MkdirAll(srcDir, 0o755)
	testutils.CreateFile(t, filepath.Join(srcDir, "alpha.txt"), "aaa")
	testutils.CreateFile(t, filepath.Join(srcDir, "beta.txt"), "bbb")

	pw := &testProgressWriter{}
	err := copyFilesOne(context.Background(), dstDir, dirStat(t, srcDir), NewCopyProgress(pw))
	testutils.AssertNoError(t, err)

	names := make(map[string]bool)
	for _, u := range pw.updates {
		names[u.Name] = true
	}
	testutils.AssertTrue(t, names["alpha.txt"])
	testutils.AssertTrue(t, names["beta.txt"])
}

func TestCopyFiles_Cancelled(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	os.MkdirAll(srcDir, 0o755)
	testutils.CreateFile(t, filepath.Join(srcDir, "a.txt"), "data")

	ds := dirStat(t, srcDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	err := copyFilesOne(ctx, dstDir, ds, NewCopyProgress(&testProgressWriter{}))
	testutils.AssertError(t, err)
}

func TestCopyFiles_MultiRequestSingleProgress(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	srcDir := filepath.Join(tmpDir, "src")
	os.MkdirAll(srcDir, 0o755)
	testutils.CreateFile(t, filepath.Join(srcDir, "a.txt"), "aaa")

	ds := dirStat(t, srcDir)
	dstA := filepath.Join(tmpDir, "dstA")
	dstB := filepath.Join(tmpDir, "dstB")

	pw := &testProgressWriter{}
	err := CopyFiles(context.Background(), NewCopyProgress(pw), []types.CopyFilesRequest{
		{DstRoot: dstA, Src: []*types.DirStat{ds}},
		{DstRoot: dstB, Src: []*types.DirStat{ds}},
	})
	testutils.AssertNoError(t, err)

	// Both destinations produced.
	testutils.AssertExists(t, filepath.Join(dstA, "a.txt"))
	testutils.AssertExists(t, filepath.Join(dstB, "a.txt"))

	// Exactly one progress session for the whole batch.
	testutils.AssertTrue(t, pw.inited)
	testutils.AssertTrue(t, pw.closed)
	testutils.AssertTrue(t, pw.success)
}
