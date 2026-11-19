package fsutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNowAtLeavesNoProbe(t *testing.T) {
	dir := t.TempDir()
	now := NowAt(dir)
	if d := time.Since(now); d < -5*time.Second || d > 5*time.Second {
		t.Fatalf("NowAt is %s away from the local clock", d)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("probe left behind: %v", entries)
	}
}

func TestNowAtFallsBackToLocalTime(t *testing.T) {
	now := NowAt(filepath.Join(t.TempDir(), "missing"))
	if d := time.Since(now); d < 0 || d > 5*time.Second {
		t.Fatalf("NowAt is %s away from the local clock", d)
	}
}

func TestTouchNowRefreshesFilesAndDirs(t *testing.T) {
	dir := t.TempDir()
	file, sub := filepath.Join(dir, "f"), filepath.Join(dir, "d")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	for _, p := range []string{file, sub} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
		if err := TouchNow(p); err != nil {
			t.Fatal(err)
		}
		stale, err := IsStaleAt(p, NowAt(dir), time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if stale {
			t.Fatalf("%s still stale after TouchNow", p)
		}
	}
}

func TestTouchNowDoesNotRecreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gone")
	if err := TouchNow(path); !os.IsNotExist(err) {
		t.Fatalf("want not-exist, got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("TouchNow created %s", path)
	}
}

func TestIsStaleAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	mtime := now.Add(-2 * time.Minute)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	for d, want := range map[time.Duration]bool{time.Minute: true, 5 * time.Minute: false} {
		got, err := IsStaleAt(path, now, d)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("IsStaleAt(%s) = %v, want %v", d, got, want)
		}
	}
	if _, err := IsStaleAt(path+"-missing", now, time.Minute); !os.IsNotExist(err) {
		t.Fatalf("want not-exist, got %v", err)
	}
}
