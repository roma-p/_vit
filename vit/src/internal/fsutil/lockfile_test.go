package fsutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	testutils "vit/internal/testhelpers"
)

// --- WithExclusiveLock tests

func TestWithExclusiveLock_Success(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	ctx := context.Background()
	executed := false

	err := WithExclusiveLock(ctx, testPath, func() error {
		executed = true
		return nil
	})

	testutils.AssertNoError(t, err)
	testutils.AssertTrue(t, executed)

	lockDir := filepath.Join(tmpDir, ".resource.lock")
	testutils.AssertNotExists(t, lockDir)
}

func TestWithExclusiveLock_OperationError(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	ctx := context.Background()
	expectedErr := fmt.Errorf("operation failed")

	err := WithExclusiveLock(ctx, testPath, func() error {
		return expectedErr
	})

	testutils.AssertEqual(t, err, expectedErr)

	lockDir := filepath.Join(tmpDir, ".resource.lock")
	testutils.AssertNotExists(t, lockDir)
}

func TestWithExclusiveLock_Concurrent(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	ctx := context.Background()
	var counter int
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Launch 5 concurrent operations
	for i := range 5 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			err := WithExclusiveLock(ctx, testPath, func() error {
				mu.Lock()
				current := counter
				mu.Unlock()

				time.Sleep(10 * time.Millisecond) // Simulate work

				mu.Lock()
				counter = current + 1
				mu.Unlock()

				return nil
			})
			if err != nil {
				t.Errorf("goroutine %d failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// All 5 operations should have completed
	testutils.AssertEqual(t, counter, 5)
}

func TestWithExclusiveLock_Timeout(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	// Create blocking lock directory
	lockDir := filepath.Join(tmpDir, ".resource.lock")
	testutils.AssertNoError(t, os.Mkdir(lockDir, 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := WithExclusiveLock(ctx, testPath, func() error {
		return nil
	})
	elapsed := time.Since(start)

	testutils.AssertError(t, err)

	if elapsed < 150*time.Millisecond {
		t.Errorf("should have retried with backoff, elapsed: %v", elapsed)
	}
}

func TestWithExclusiveLock_OrphanedLockCleanup(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	// Create old orphaned lock directory
	lockDir := filepath.Join(tmpDir, ".resource.lock")
	testutils.AssertNoError(t, os.Mkdir(lockDir, 0o755))

	// Set modification time to old (orphaned)
	oldTime := time.Now().Add(-61 * time.Second)
	os.Chtimes(lockDir, oldTime, oldTime)

	ctx := context.Background()
	executed := false

	start := time.Now()
	err := WithExclusiveLock(ctx, testPath, func() error {
		executed = true
		return nil
	})
	elapsed := time.Since(start)

	testutils.AssertNoError(t, err)
	testutils.AssertTrue(t, executed)

	if elapsed > 500*time.Millisecond {
		t.Errorf("lock acquisition took too long (%v), orphaned lock should be cleaned immediately", elapsed)
	}
}

func TestWithExclusiveLock_NestedPath(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()

	// Create the parent directories so mkdir can work
	nestedDir := filepath.Join(tmpDir, "deep", "nested")
	testutils.AssertNoError(t, os.MkdirAll(nestedDir, 0o755))

	testPath := filepath.Join(nestedDir, "resource.txt")

	ctx := context.Background()
	executed := false

	err := WithExclusiveLock(ctx, testPath, func() error {
		executed = true
		return nil
	})

	testutils.AssertNoError(t, err)
	testutils.AssertTrue(t, executed)

	// Verify lock dir was created in correct location and cleaned up
	lockDir := filepath.Join(nestedDir, ".resource.txt.lock")
	testutils.AssertNotExists(t, lockDir)
}

// --- AcquireExclusiveLock / ReleaseExclusiveLock tests ---

func TestAcquireRelease_Basic(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	ctx := context.Background()

	lockDir, err := AcquireExclusiveLock(ctx, testPath)
	testutils.AssertNoError(t, err)

	expectedLockDir := filepath.Join(tmpDir, ".resource.lock")
	testutils.AssertEqual(t, lockDir, expectedLockDir)
	testutils.AssertExists(t, lockDir)

	ReleaseExclusiveLock(lockDir)

	testutils.AssertNotExists(t, lockDir)
}

func TestAcquireRelease_BlocksSecondAcquire(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	ctx := context.Background()

	lockDir, err := AcquireExclusiveLock(ctx, testPath)
	testutils.AssertNoError(t, err)

	ctx2, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err = AcquireExclusiveLock(ctx2, testPath)
	testutils.AssertError(t, err)

	ReleaseExclusiveLock(lockDir)

	lockDir2, err := AcquireExclusiveLock(ctx, testPath)
	testutils.AssertNoError(t, err)
	ReleaseExclusiveLock(lockDir2)
}

func TestAcquireRelease_ConcurrentContention(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	ctx := context.Background()
	var counter int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := range 5 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			lockDir, err := AcquireExclusiveLock(ctx, testPath)
			if err != nil {
				t.Errorf("goroutine %d failed to acquire: %v", id, err)
				return
			}

			mu.Lock()
			current := counter
			mu.Unlock()

			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			counter = current + 1
			mu.Unlock()

			ReleaseExclusiveLock(lockDir)
		}(i)
	}

	wg.Wait()
	testutils.AssertEqual(t, counter, 5)
}

func TestAcquireRelease_OrphanedCleanup(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	lockDir := filepath.Join(tmpDir, ".resource.lock")
	testutils.AssertNoError(t, os.Mkdir(lockDir, 0o755))

	oldTime := time.Now().Add(-61 * time.Second)
	os.Chtimes(lockDir, oldTime, oldTime)

	ctx := context.Background()

	start := time.Now()
	acquired, err := AcquireExclusiveLock(ctx, testPath)
	elapsed := time.Since(start)

	testutils.AssertNoError(t, err)
	ReleaseExclusiveLock(acquired)

	if elapsed > 500*time.Millisecond {
		t.Errorf("should have cleaned orphaned lock quickly, took %v", elapsed)
	}
}

// --- AcquireExclusiveLockWithKeepalive tests ---

func TestKeepalive_Basic(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	ctx := context.Background()

	stop, err := AcquireExclusiveLockWithKeepalive(ctx, testPath)
	testutils.AssertNoError(t, err)

	lockDir := filepath.Join(tmpDir, ".resource.lock")
	testutils.AssertExists(t, lockDir)

	stop()

	testutils.AssertNotExists(t, lockDir)
}

func TestKeepalive_PreventsOrphanCleanup(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	ctx := context.Background()

	stop, err := AcquireExclusiveLockWithKeepalive(ctx, testPath)
	testutils.AssertNoError(t, err)
	defer stop()

	lockDir := filepath.Join(tmpDir, ".resource.lock")

	// Backdate the lock dir to look orphaned
	oldTime := time.Now().Add(-61 * time.Second)
	os.Chtimes(lockDir, oldTime, oldTime)

	// Wait for keepalive to touch it
	time.Sleep(3 * time.Second)

	// Mtime should have been refreshed by keepalive
	info, err := os.Stat(lockDir)
	testutils.AssertNoError(t, err)

	if time.Since(info.ModTime()) > 5*time.Second {
		t.Errorf("keepalive should have refreshed mtime, but it's %v old", time.Since(info.ModTime()))
	}

	// Another process trying to acquire should NOT be able to steal it
	ctx2, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err = AcquireExclusiveLock(ctx2, testPath)
	testutils.AssertError(t, err)
}

func TestKeepalive_StopsOnContextCancel(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "lockfile-test-*")
	defer cleanup()
	testPath := filepath.Join(tmpDir, "resource")

	ctx, cancel := context.WithCancel(context.Background())

	stop, err := AcquireExclusiveLockWithKeepalive(ctx, testPath)
	testutils.AssertNoError(t, err)

	// Cancel context — keepalive goroutine should exit
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Lock dir still exists (stop() not called yet), but keepalive is dead
	lockDir := filepath.Join(tmpDir, ".resource.lock")
	testutils.AssertExists(t, lockDir)

	// Clean up
	stop()
	testutils.AssertNotExists(t, lockDir)
}
