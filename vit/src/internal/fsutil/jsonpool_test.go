package fsutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFlushForWriteRefusesAStolenLock(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json")
	for _, p := range []string{a, b} {
		if err := NewJSONHandlerFromPath(p, &testData{Name: "before"}).Write(); err != nil {
			t.Fatal(err)
		}
	}

	pool := NewJSONHandlerPool()
	for _, p := range []string{a, b} {
		h, err := ResolveHandler[testData](context.Background(), pool, p, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		h.Data.Name = "after"
	}
	stealLock(t, b)

	if err := pool.FlushForWrite(); !errors.Is(err, ErrLockLost) {
		t.Fatalf("want ErrLockLost, got %v", err)
	}
	for _, p := range []string{a, b} {
		h, err := NewJSONHandlerFromFile[testData](p)
		if err != nil {
			t.Fatal(err)
		}
		if h.Data.Name != "before" {
			t.Fatalf("%s written: %q", p, h.Data.Name)
		}
	}

	pool.ReleaseAll()
	if _, err := os.Stat(lockDirPath(b)); err != nil {
		t.Fatalf("the stolen lock was released: %v", err)
	}
	if _, err := os.Stat(lockDirPath(a)); !os.IsNotExist(err) {
		t.Fatalf("our own lock was not released: %v", err)
	}
}

func stealLock(t *testing.T, path string) {
	t.Helper()
	lockDir := lockDirPath(path)
	if err := os.Remove(lockDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
}
