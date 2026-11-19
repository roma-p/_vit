package transaction

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vit/internal/db"
	"vit/internal/fsutil"
	"vit/internal/history"
	"vit/internal/opcontext"
	"vit/internal/types"
	"vit/internal/workspace"
)

func TestTransactionLostAfterReclaim(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(repo, "asset.json")
	writeDoc(t, doc, "before")

	op := &docOperation{path: doc, value: "after", duringDBUpdate: func() {
		for _, f := range tidFiles(t, repo) {
			if err := os.Remove(f); err != nil {
				t.Fatal(err)
			}
		}
	}}
	err := runDoc(t, repo, op)

	var verr *types.VitError
	if !errors.As(err, &verr) || verr.Name != types.ErrTransactionLost {
		t.Fatalf("want %s, got %v", types.ErrTransactionLost, err)
	}
	if got := readDoc(t, doc); got != "before" {
		t.Fatalf("doc written after the transaction was lost: %q", got)
	}
}

func TestTransactionLostCancelsTheOperation(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(repo, "asset.json")
	writeDoc(t, doc, "before")

	op := &docOperation{path: doc, value: "after", waitForCancel: true, duringDBUpdate: func() {
		for _, f := range tidFiles(t, repo) {
			if err := os.Remove(f); err != nil {
				t.Fatal(err)
			}
		}
	}}
	err := runDoc(t, repo, op)

	var verr *types.VitError
	if !errors.As(err, &verr) || verr.Name != types.ErrTransactionLost {
		t.Fatalf("want %s, got %v", types.ErrTransactionLost, err)
	}
	if got := readDoc(t, doc); got != "before" {
		t.Fatalf("doc written after the transaction was lost: %q", got)
	}
}

func TestTransactionLostWhenTheDocLockIsStolen(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(repo, "asset.json")
	writeDoc(t, doc, "before")

	op := &docOperation{path: doc, value: "after", duringDBUpdate: func() {
		lockDir := filepath.Join(repo, ".asset.json.lock")
		if err := os.Remove(lockDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(lockDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}}
	err := runDoc(t, repo, op)

	var verr *types.VitError
	if !errors.As(err, &verr) || verr.Name != types.ErrTransactionLost {
		t.Fatalf("want %s, got %v", types.ErrTransactionLost, err)
	}
	if got := readDoc(t, doc); got != "before" {
		t.Fatalf("doc written after its lock was stolen: %q", got)
	}
}

func TestScanLockKeptAliveDuringValidate(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(repo, "asset.json")
	writeDoc(t, doc, "before")

	lockDir := filepath.Join(repo, ".vit", ".transaction.lock.lock")
	op := &docOperation{path: doc, value: "after", duringValidate: func() {
		old := time.Now().Add(-time.Hour)
		if err := os.Chtimes(lockDir, old, old); err != nil {
			t.Fatal(err)
		}
		time.Sleep(fsutil.KeepaliveInterval + 500*time.Millisecond)
		info, err := os.Stat(lockDir)
		if err != nil {
			t.Fatal(err)
		}
		if age := time.Since(info.ModTime()); age > fsutil.LockAcquireTimeout {
			t.Fatalf("scan-lock not refreshed: %s old", age)
		}
	}}
	if err := runDoc(t, repo, op); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentWriteDetectedAfterTheWrite(t *testing.T) {
	cases := map[string]func(repo string){
		"tid": func(repo string) {
			for _, f := range tidFiles(t, repo) {
				if err := os.Remove(f); err != nil {
					t.Fatal(err)
				}
			}
		},
		"doc lock": func(repo string) {
			lockDir := filepath.Join(repo, ".asset.json.lock")
			if err := os.Remove(lockDir); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(lockDir, 0o755); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, steal := range cases {
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			doc := filepath.Join(repo, "asset.json")
			writeDoc(t, doc, "before")

			op := &docOperation{path: doc, value: "after", duringWrite: func() { steal(repo) }}
			err := runDoc(t, repo, op)

			var verr *types.VitError
			if !errors.As(err, &verr) || verr.Name != types.ErrConcurrentWrite {
				t.Fatalf("want %s, got %v", types.ErrConcurrentWrite, err)
			}
			if got := readDoc(t, doc); got != "after" {
				t.Fatalf("got %q", got)
			}
			if !op.finished {
				t.Fatal("Finish skipped")
			}
			entries, err := history.ReadAll(repo, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || !strings.Contains(entries[0].Details, "concurrent write") {
				t.Fatalf("history: %+v", entries)
			}
		})
	}
}

func TestTransactionLostKeepsTheOtherTid(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(repo, "asset.json")
	writeDoc(t, doc, "before")

	var other string
	op := &docOperation{path: doc, value: "after", duringDBUpdate: func() {
		files := tidFiles(t, repo)
		if len(files) != 1 {
			t.Fatalf("want one tid file, got %v", files)
		}
		if err := os.Remove(files[0]); err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(files[0])
		other = filepath.Join(filepath.Dir(files[0]), name[:strings.LastIndex(name, tokenSeparator)]+tokenSeparator+"other")
		writeDoc(t, other, "")
	}}
	if err := runDoc(t, repo, op); err == nil {
		t.Fatal("want an error")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("the other transaction's tid was removed: %v", err)
	}
}

func TestTransactionWritesWhileValid(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(repo, "asset.json")
	writeDoc(t, doc, "before")

	if err := runDoc(t, repo, &docOperation{path: doc, value: "after"}); err != nil {
		t.Fatal(err)
	}
	if got := readDoc(t, doc); got != "after" {
		t.Fatalf("got %q", got)
	}
	if files := tidFiles(t, repo); len(files) != 0 {
		t.Fatalf("tid files left behind: %v", files)
	}
}

func TestTransactionIDFromNameIgnoresToken(t *testing.T) {
	ref := types.NewRefEmpty("/repo", "shots/sh 010")
	tid := newTransactionID(TransactionTreeChanged, ref)

	for _, name := range []string{tid.Encode(), tid.fileName("9f2c41d07a3b5e18")} {
		got, err := newTransactionIDFromName("/repo", name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Operation != tid.Operation || got.Ref.AssetPath != ref.AssetPath {
			t.Fatalf("%s: got %+v", name, got)
		}
	}
}

// -- FILE LOCAL -------------------------------------------------------------

type docOperation struct {
	path           string
	value          string
	duringValidate func()
	duringDBUpdate func()
	duringWrite    func()
	waitForCancel  bool
	finished       bool
}

func (o *docOperation) Validate(context.Context, *opcontext.OperationContext, *db.Client) error {
	if o.duringValidate != nil {
		o.duringValidate()
	}
	return nil
}

func (o *docOperation) PlanBlobUpdate(
	context.Context, *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	return &types.BlobUpdatePlan{}, nil
}

func (o *docOperation) DBUpdate(
	ctx context.Context, opctx *opcontext.OperationContext, _ *db.Client,
) error {
	h, err := fsutil.ResolveHandler[hookDoc](ctx, opctx.JSONPool, o.path, true, nil)
	if err != nil {
		return err
	}
	h.Data.V = o.value
	h.Data.onWrite = o.duringWrite
	if o.duringDBUpdate != nil {
		o.duringDBUpdate()
	}
	if o.waitForCancel {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * fsutil.KeepaliveInterval):
			return errors.New("operation not cancelled")
		}
	}
	return nil
}

func (o *docOperation) Finish(
	context.Context, *opcontext.OperationContext, *db.Client, *slog.Logger, workspace.Presenter,
) {
	o.finished = true
}

type hookDoc struct {
	V       string `json:"v"`
	onWrite func()
}

func (d hookDoc) MarshalJSON() ([]byte, error) {
	if d.onWrite != nil {
		d.onWrite()
	}
	return json.Marshal(map[string]string{"v": d.V})
}

func runDoc(t *testing.T, repo string, op Operation) error {
	t.Helper()
	opctx := &opcontext.OperationContext{
		User:      "tester",
		TimeStamp: time.Now(),
		JSONPool:  fsutil.NewJSONHandlerPool(),
	}
	t.Cleanup(opctx.Close)
	return RunEdit(context.Background(), Request{
		Opctx:     opctx,
		Operation: op,
		Logger:    slog.New(slog.DiscardHandler),
		Command:   types.CommandTag,
		User:      opctx.User,
		RepoPath:  repo,
		Timestamp: opctx.TimeStamp,
	}, []*types.Ref{types.NewRefEmpty(repo, "asset")}, TransactionTag)
}

func tidFiles(t *testing.T, repo string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(repo, ".vit", "transaction", "*"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func writeDoc(t *testing.T, path, value string) {
	t.Helper()
	data := map[string]string{"v": value}
	if err := fsutil.NewJSONHandlerFromPath(path, &data).Write(); err != nil {
		t.Fatal(err)
	}
}

func readDoc(t *testing.T, path string) string {
	t.Helper()
	h, err := fsutil.NewJSONHandlerFromFile[map[string]string](path)
	if err != nil {
		t.Fatal(err)
	}
	return (*h.Data)["v"]
}
