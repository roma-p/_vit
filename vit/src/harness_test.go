package vit_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	vit "vit"
	testutils "vit/internal/testhelpers"
	"vit/internal/types"
)

var ctx = context.Background()

// vitEnv is an isolated vit environment for a test: an embedded *vit.Client
// and temp repo dir. It also provides helpers to avoid boiler plate tests.
type vitEnv struct {
	*vit.Client
	t        *testing.T
	repoPath string
}

func newVitEnv(t *testing.T) *vitEnv {
	t.Helper()

	dir, cleanup := testutils.TempDir(t, "vit-repo-*")
	t.Cleanup(cleanup)
	repoPath := filepath.Join(dir, "repo")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := must(vit.NewClient(ctx, logger, nil))
	must(client.InitRepo(ctx, repoPath))
	return &vitEnv{Client: client, t: t, repoPath: repoPath}
}

func (r *vitEnv) abs(assetPath string) string { return filepath.Join(r.repoPath, assetPath) }

func (r *vitEnv) branchRef(assetPath string) string { return r.abs(assetPath) + "@branch=base" }

func (r *vitEnv) fileSrc(name, content string) string {
	r.t.Helper()
	dir, cleanup := testutils.TempDir(r.t, "vit-src-*")
	r.t.Cleanup(cleanup)
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		r.t.Fatalf("write source %s: %v", p, err)
	}
	return p
}

func (r *vitEnv) dirSrc(files map[string]string) string {
	r.t.Helper()
	dir, cleanup := testutils.TempDir(r.t, "vit-srcdir-*")
	r.t.Cleanup(cleanup)
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			r.t.Fatalf("mkdir source %s: %v", p, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			r.t.Fatalf("write source %s: %v", p, err)
		}
	}
	return dir
}

func (r *vitEnv) read(path string) string {
	r.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		r.t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func (r *vitEnv) editFile(path, content string) {
	r.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.t.Fatalf("edit %s: %v", path, err)
	}
}

func (r *vitEnv) removeFile(path string) {
	r.t.Helper()
	if err := os.Remove(path); err != nil {
		r.t.Fatalf("remove %s: %v", path, err)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func assertClean(t *testing.T, res *types.StatusResult) {
	t.Helper()
	if len(res.Files) != 0 {
		t.Fatalf("expected clean status, got %d change(s): %+v", len(res.Files), res.Files)
	}
}

func assertErrCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %q, got nil", code)
	}
	if !types.IsErrCode(err, code) {
		t.Fatalf("expected error code %q, got: %v", code, err)
	}
}
