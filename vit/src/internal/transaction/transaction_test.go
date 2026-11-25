package transaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
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

func TestPrepareRunsBeforeTheScanLock(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(repo, "asset.json")
	writeDoc(t, doc, "before")

	lockDir := filepath.Join(repo, ".vit", ".transaction.lock.lock")
	op := &preparedDocOperation{docOperation: &docOperation{path: doc, value: "after"}, lockDir: lockDir}
	op.duringValidate = func() {
		if !op.prepared {
			t.Fatal("Validate ran before Prepare")
		}
	}
	if err := runDoc(t, repo, op); err != nil {
		t.Fatal(err)
	}
	if op.lockedDuringPrepare {
		t.Fatal("Prepare ran under the scan-lock")
	}
	if op.locksBeforePrepare {
		t.Fatal("Locks read before Prepare")
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

func TestAttrEntryIsRecordedBeforeTheWrite(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(repo, "asset.json")
	writeDoc(t, doc, "before")
	var atWrite []types.HistoryEntry
	var tidsAtWrite []string
	var payloadAtWrite []byte
	op := &attrDocOperation{&docOperation{path: doc, value: "after", duringWrite: func() {
		atWrite, _ = history.ReadAll(repo, "")
		tidsAtWrite = tidFiles(t, repo)
		if len(tidsAtWrite) > 0 {
			payloadAtWrite, _ = os.ReadFile(tidsAtWrite[0])
		}
	}}}

	if err := runDoc(t, repo, op); err != nil {
		t.Fatal(err)
	}
	if len(atWrite) != 1 || atWrite[0].NodeID != "uid" || atWrite[0].Tx == "" {
		t.Fatalf("history at write: %+v", atWrite)
	}
	if !slices.ContainsFunc(tidsAtWrite, func(f string) bool { return types.TransactionFileToken(filepath.Base(f)) == atWrite[0].Tx }) {
		t.Fatalf("entry token %q matches no live transaction file: %v", atWrite[0].Tx, tidsAtWrite)
	}
	var payload transactionPayload
	if err := json.Unmarshal(payloadAtWrite, &payload); err != nil || payload.AssetPath != "asset" || payload.User != "tester" {
		t.Fatalf("transaction file = %s (%v)", payloadAtWrite, err)
	}
	if entries, _ := history.ReadAll(repo, ""); len(entries) != 1 {
		t.Fatalf("history after: %+v", entries)
	}
}

func TestAttrEntryIsAbortedWhenTheWriteFails(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(repo, "asset.json")
	writeDoc(t, doc, "before")
	op := &attrDocOperation{&docOperation{path: doc, value: "after", duringDBUpdate: func() {
		lockDir := filepath.Join(repo, ".asset.json.lock")
		if err := os.Remove(lockDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(lockDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}}}

	if err := runDoc(t, repo, op); !isVitError(err, types.ErrTransactionLost) {
		t.Fatalf("want %s, got %v", types.ErrTransactionLost, err)
	}
	entries, err := history.ReadAll(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	var change, abort *types.HistoryEntry
	for i := range entries {
		if entries[i].Aborted != "" {
			abort = &entries[i]
		} else {
			change = &entries[i]
		}
	}
	if len(entries) != 2 || change == nil || abort == nil || abort.Aborted != change.UID {
		t.Fatalf("history: %+v", entries)
	}
}

func TestTransactionLogsItsPhases(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(repo, "asset.json")
	writeDoc(t, doc, "before")
	var logs bytes.Buffer
	opctx := &opcontext.OperationContext{User: "tester", TimeStamp: time.Now(), JSONPool: fsutil.NewJSONHandlerPool()}
	t.Cleanup(opctx.Close)

	err := Run(context.Background(), Request{
		Opctx:     opctx,
		Operation: &attrDocOperation{&docOperation{path: doc, value: "after"}},
		Logger:    slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Command:   types.CommandAttrSet,
		User:      opctx.User,
		RepoPath:  repo,
		Timestamp: opctx.TimeStamp,
	})
	if err != nil {
		t.Fatal(err)
	}
	var line map[string]any
	for _, l := range strings.Split(logs.String(), "\n") {
		if strings.Contains(l, `"transaction phases"`) {
			_ = json.Unmarshal([]byte(l), &line)
		}
	}
	for _, phase := range []string{"scan_lock", "validate", "stake", "blob", "db_update", "history_entry", "flush", "finish"} {
		if _, ok := line[phase+"_ms"].(float64); !ok {
			t.Errorf("no %s_ms in %v", phase, line)
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

func (o *docOperation) Locks() Locks {
	return Locks{Edited: []*types.Ref{types.NewRefEmpty(filepath.Dir(o.path), "asset")}, EditOp: TransactionTag}
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

type attrDocOperation struct {
	*docOperation
}

func (o *attrDocOperation) HistoryAttrs() (string, []types.AttrChange) {
	return "uid", []types.AttrChange{{Key: "task", Value: o.value}}
}

type preparedDocOperation struct {
	*docOperation
	lockDir             string
	prepared            bool
	lockedDuringPrepare bool
	locksBeforePrepare  bool
}

func (o *preparedDocOperation) Locks() Locks {
	o.locksBeforePrepare = o.locksBeforePrepare || !o.prepared
	return o.docOperation.Locks()
}

func (o *preparedDocOperation) Prepare(context.Context, *opcontext.OperationContext, *db.Client) error {
	_, err := os.Stat(o.lockDir)
	o.lockedDuringPrepare = err == nil
	o.prepared = true
	return nil
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
	return Run(context.Background(), Request{
		Opctx:     opctx,
		Operation: op,
		Logger:    slog.New(slog.DiscardHandler),
		Command:   types.CommandTag,
		User:      opctx.User,
		RepoPath:  repo,
		Timestamp: opctx.TimeStamp,
	})
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

func TestTransactionReapsOrphanedTransactionFiles(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(repo, "asset.json")
	writeDoc(t, doc, "before")
	orphan := filepath.Join(repo, ".vit", "transaction", "other---Tag---tx=dead")
	if err := os.MkdirAll(filepath.Dir(orphan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * fsutil.LockAcquireTimeout)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}

	if err := runDoc(t, repo, &docOperation{path: doc, value: "after"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan still there: %v", err)
	}
}
