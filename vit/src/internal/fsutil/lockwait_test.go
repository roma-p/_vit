package fsutil

import (
	"context"
	"path/filepath"
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
