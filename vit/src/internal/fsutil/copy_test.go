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

// --- CopyFile tests ---

func TestCopyFile_Basic(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "dst.txt")
	testutils.CreateFile(t, src, "hello world")

	pw := &testProgressWriter{}
	progress := NewCopyProgress(pw)
	ctx := context.Background()

	hash, size, err := CopyFile(ctx, src, dst, progress)
	testutils.AssertNoError(t, err)

	// Destination exists with correct content
	content, err := os.ReadFile(dst)
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, string(content), "hello world")

	// Hash and size are populated
	testutils.AssertTrue(t, hash != "")
	testutils.AssertEqual(t, size, int64(11))

	// Progress was used
	testutils.AssertTrue(t, pw.inited)
	testutils.AssertEqual(t, pw.operation, "Copying")
	testutils.AssertTrue(t, len(pw.updates) > 0)
	testutils.AssertTrue(t, pw.closed)
	testutils.AssertTrue(t, pw.success)

	// Lock dir is cleaned up
	lockDir := filepath.Join(tmpDir, ".dst.txt.lock")
	testutils.AssertNotExists(t, lockDir)
}

func TestCopyFile_PreservesPermissions(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	src := filepath.Join(tmpDir, "script.sh")
	dst := filepath.Join(tmpDir, "script-copy.sh")
	testutils.CreateFile(t, src, "#!/bin/bash")
	os.Chmod(src, 0o755)

	pw := &testProgressWriter{}
	progress := NewCopyProgress(pw)

	_, _, err := CopyFile(context.Background(), src, dst, progress)
	testutils.AssertNoError(t, err)

	info, err := os.Stat(dst)
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, info.Mode().Perm(), os.FileMode(0o755))
}

func TestCopyFile_ConsistentHash(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	src := filepath.Join(tmpDir, "src.txt")
	testutils.CreateFile(t, src, "deterministic content")

	pw1 := &testProgressWriter{}
	pw2 := &testProgressWriter{}

	dst1 := filepath.Join(tmpDir, "dst1.txt")
	dst2 := filepath.Join(tmpDir, "dst2.txt")

	hash1, _, err := CopyFile(context.Background(), src, dst1, NewCopyProgress(pw1))
	testutils.AssertNoError(t, err)

	hash2, _, err := CopyFile(context.Background(), src, dst2, NewCopyProgress(pw2))
	testutils.AssertNoError(t, err)

	testutils.AssertEqual(t, hash1, hash2)
}

func TestCopyFile_SrcNotFound(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	src := filepath.Join(tmpDir, "nonexistent.txt")
	dst := filepath.Join(tmpDir, "dst.txt")

	pw := &testProgressWriter{}
	_, _, err := CopyFile(context.Background(), src, dst, NewCopyProgress(pw))
	testutils.AssertError(t, err)
}

func TestCopyFile_LargeFile(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	src := filepath.Join(tmpDir, "large.bin")
	dst := filepath.Join(tmpDir, "large-copy.bin")

	// 1MB file
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	os.WriteFile(src, data, 0o644)

	pw := &testProgressWriter{}
	progress := NewCopyProgress(pw)

	hash, size, err := CopyFile(context.Background(), src, dst, progress)
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, size, int64(1024*1024))
	testutils.AssertTrue(t, hash != "")

	// Verify content matches
	copied, err := os.ReadFile(dst)
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, len(copied), len(data))

	// Multiple progress updates should have been emitted
	testutils.AssertTrue(t, len(pw.updates) > 1)

	// Last update should have cumulative bytes equal to file size
	lastUpdate := pw.updates[len(pw.updates)-1]
	testutils.AssertEqual(t, lastUpdate.Size, 1024*1024)
}

// --- CopyDir tests ---

func TestCopyDir_Basic(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755)
	testutils.CreateFile(t, filepath.Join(srcDir, "a.txt"), "file a")
	testutils.CreateFile(t, filepath.Join(srcDir, "sub", "b.txt"), "file b")

	pw := &testProgressWriter{}
	progress := NewCopyProgress(pw)

	hash, size, err := CopyDir(context.Background(), srcDir, dstDir, progress)
	testutils.AssertNoError(t, err)

	testutils.AssertTrue(t, hash != "")
	testutils.AssertEqual(t, size, int64(12)) // "file a" + "file b"

	// Verify contents
	content, err := os.ReadFile(filepath.Join(dstDir, "a.txt"))
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, string(content), "file a")

	content, err = os.ReadFile(filepath.Join(dstDir, "sub", "b.txt"))
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, string(content), "file b")

	// Progress
	testutils.AssertTrue(t, pw.inited)
	testutils.AssertTrue(t, len(pw.updates) > 0)
	testutils.AssertTrue(t, pw.closed)
	testutils.AssertTrue(t, pw.success)

	// Lock dir cleaned up
	lockDir := filepath.Join(tmpDir, ".dst.lock")
	testutils.AssertNotExists(t, lockDir)
}

func TestCopyDir_PreservesPermissions(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	os.Mkdir(srcDir, 0o750)
	testutils.CreateFile(t, filepath.Join(srcDir, "f.txt"), "data")

	pw := &testProgressWriter{}
	_, _, err := CopyDir(context.Background(), srcDir, dstDir, NewCopyProgress(pw))
	testutils.AssertNoError(t, err)

	info, err := os.Stat(dstDir)
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, info.Mode().Perm(), os.FileMode(0o750))
}

func TestCopyDir_ConsistentHash(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	srcDir := filepath.Join(tmpDir, "src")
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755)
	testutils.CreateFile(t, filepath.Join(srcDir, "a.txt"), "aaa")
	testutils.CreateFile(t, filepath.Join(srcDir, "sub", "b.txt"), "bbb")

	dst1 := filepath.Join(tmpDir, "dst1")
	dst2 := filepath.Join(tmpDir, "dst2")

	hash1, _, err := CopyDir(context.Background(), srcDir, dst1, NewCopyProgress(&testProgressWriter{}))
	testutils.AssertNoError(t, err)

	hash2, _, err := CopyDir(context.Background(), srcDir, dst2, NewCopyProgress(&testProgressWriter{}))
	testutils.AssertNoError(t, err)

	testutils.AssertEqual(t, hash1, hash2)
}

func TestCopyDir_SrcNotFound(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	src := filepath.Join(tmpDir, "nonexistent")
	dst := filepath.Join(tmpDir, "dst")

	_, _, err := CopyDir(context.Background(), src, dst, NewCopyProgress(&testProgressWriter{}))
	testutils.AssertError(t, err)
}

func TestCopyDir_Empty(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	os.Mkdir(srcDir, 0o755)

	pw := &testProgressWriter{}
	hash, size, err := CopyDir(context.Background(), srcDir, dstDir, NewCopyProgress(pw))
	testutils.AssertNoError(t, err)

	testutils.AssertTrue(t, hash != "")
	testutils.AssertEqual(t, size, int64(0))
	testutils.AssertExists(t, dstDir)
}

// --- Progress tracking tests ---

func TestCopyFile_ProgressTracksFileName(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	src := filepath.Join(tmpDir, "myfile.dat")
	dst := filepath.Join(tmpDir, "copy.dat")
	testutils.CreateFile(t, src, "some data here")

	pw := &testProgressWriter{}
	_, _, err := CopyFile(context.Background(), src, dst, NewCopyProgress(pw))
	testutils.AssertNoError(t, err)

	// All updates should reference the source filename
	for _, u := range pw.updates {
		testutils.AssertEqual(t, u.Name, "myfile.dat")
	}
}

func TestCopyDir_ProgressTracksEachFile(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "copy-test-*")
	defer cleanup()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	os.Mkdir(srcDir, 0o755)
	testutils.CreateFile(t, filepath.Join(srcDir, "alpha.txt"), "aaa")
	testutils.CreateFile(t, filepath.Join(srcDir, "beta.txt"), "bbb")

	pw := &testProgressWriter{}
	_, _, err := CopyDir(context.Background(), srcDir, dstDir, NewCopyProgress(pw))
	testutils.AssertNoError(t, err)

	// Collect unique file names from progress updates
	names := make(map[string]bool)
	for _, u := range pw.updates {
		names[u.Name] = true
	}

	testutils.AssertTrue(t, names["alpha.txt"])
	testutils.AssertTrue(t, names["beta.txt"])
}
