package fsutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestLockWaitsSumTheTimeSpentWaiting(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "asset.json")
	held, err := AcquireExclusiveLock(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	before := waitOf("*.json")
	go func() {
		time.Sleep(200 * time.Millisecond)
		ReleaseExclusiveLock(held)
	}()

	lockDir, err := AcquireExclusiveLock(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	ReleaseExclusiveLock(lockDir)

	w := waitOf("*.json")
	if w.Count != before.Count+1 || w.Attempts < before.Attempts+2 || w.Total-before.Total < 150*time.Millisecond || w.Max < 150*time.Millisecond {
		t.Fatalf("before %+v, after %+v", before, w)
	}
}

func TestLockWaitsSumTheTimeHeld(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), ".tree_rebuild")
	before := waitOf(".tree_rebuild")
	lockDir, err := AcquireExclusiveLock(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	ReleaseExclusiveLock(lockDir)
	ReleaseExclusiveLock(lockDir)

	w := waitOf(".tree_rebuild")
	if w.Held-before.Held < 90*time.Millisecond || w.Held-before.Held > time.Second || w.HeldMax < 90*time.Millisecond {
		t.Fatalf("before %+v, after %+v", before, w)
	}
}

func TestJitterStaysBetweenHalfAndFull(t *testing.T) {
	for range 1000 {
		if d := jitter(100 * time.Millisecond); d < 50*time.Millisecond || d > 100*time.Millisecond {
			t.Fatalf("jitter = %s", d)
		}
	}
}

func TestLockWaitKinds(t *testing.T) {
	for path, want := range map[string]string{
		"/r/.vit/transaction.lock": "transaction.lock", "/r/.vit/history/.lock": ".lock",
		"/r/.vit/tree/ab/cdef.json": "*.json", "/r/.vit/blob/x/anim-base-2026": "other",
	} {
		before := waitOf(want).Count
		recordLockWait(path, "", time.Now(), 1)
		if waitOf(want).Count != before+1 {
			t.Errorf("%s: not counted as %s", path, want)
		}
	}
}

func waitOf(kind string) LockWait {
	for _, w := range LockWaits() {
		if w.Kind == kind {
			return w
		}
	}
	return LockWait{Kind: kind}
}

func TestIsNotExistCoversNFSStaleHandles(t *testing.T) {
	stale := &os.PathError{Op: "stat", Path: "/prod/.vit/x.lock", Err: syscall.ESTALE}
	for name, c := range map[string]struct {
		err  error
		want bool
	}{
		"missing":       {&os.PathError{Op: "stat", Path: "/x", Err: syscall.ENOENT}, true},
		"stale":         {stale, true},
		"wrapped stale": {fmt.Errorf("read the history: %w", stale), true},
		"permission":    {&os.PathError{Op: "stat", Path: "/x", Err: syscall.EACCES}, false},
		"no error":      {nil, false},
	} {
		if got := IsNotExist(c.err); got != c.want {
			t.Errorf("%s: got %v", name, got)
		}
	}
}
