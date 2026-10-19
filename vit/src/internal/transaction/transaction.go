package transaction

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"time"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/fsutil"
	"vit/internal/history"
	"vit/internal/opcontext"
	"vit/internal/types"
	"vit/internal/workspace"
)

// RunTreeChanged drives an operation that mutates the tree (add/move/delete).
// Conflicts with ANY existing transaction on overlapping paths.
func RunTreeChanged(
	ctx context.Context,
	r Request,
	assetPaths []string,
) error {
	tids := make([]TransactionID, len(assetPaths))
	for i, p := range assetPaths {
		tids[i] = newTransactionID(TransactionTreeChanged, types.NewRefEmpty(r.RepoPath, p))
	}
	return run(ctx, r, tids)
}

// RunEdit drives an operation that edits existing assets (commit/branch).
// Conflicts with TreeChanged on overlapping paths, or same-ref operations.
func RunEdit(
	ctx context.Context,
	r Request,
	refs []*types.Ref,
	op TransactionOperation,
) error {
	tids := make([]TransactionID, len(refs))
	for i, ref := range refs {
		tids[i] = newTransactionID(op, ref)
	}
	return run(ctx, r, tids)
}

// RunTreeChangedAndEdit combines RunTreeChanged and RunEdit.
func RunTreeChangedAndEdit(
	ctx context.Context,
	r Request,
	createRefs []*types.Ref,
	editRefs []*types.Ref,
	editOp TransactionOperation,
) error {
	tids := make([]TransactionID, 0, len(createRefs)+len(editRefs))
	for _, ref := range createRefs {
		tids = append(
			tids,
			newTransactionID(
				TransactionTreeChanged,
				types.NewRefEmpty(ref.RepoPath, ref.AssetPath),
			),
		)
	}
	for _, ref := range editRefs {
		tids = append(tids, newTransactionID(editOp, ref))
	}
	return run(ctx, r, tids)
}

type Request struct {
	Opctx     *opcontext.OperationContext
	DBClient  *db.Client
	Presenter workspace.Presenter
	Operation Operation
	Logger    *slog.Logger
	Progress  types.ProgressWriter

	Command   types.VitCommand
	User      string
	RepoPath  string
	Timestamp time.Time

	Ref     *types.Ref
	Message string
}

// -- FILE LOCAL -------------------------------------------------------------

func run(
	ctx context.Context,
	r Request,
	tids []TransactionID,
) error {
	err := withTransaction(ctx, tids, r.Command, r.User, r.Timestamp,
		func() error {
			// 0. We now hold the scan-lock(s). Any DB read taken before it
			//    may be stale, so evict the read cache:
			//     Validate must re-parse the source of truth under the
			//    lock, not reuse a pre-lock parse.
			r.Opctx.JSONPool.DropReadCache()

			// 1. check for on going transaction conflict, then run operation validate.
			if err := scanForConflicts(tids); err != nil {
				return err
			}
			if err := r.Operation.Validate(ctx, r.Opctx, r.DBClient); err != nil {
				return err
			}
			return nil
		},
		func() error {
			// 2. Plan the blob object to copy (plan only, do nothing), then execute them
			//    (copy blobs, write content nodes + manifests).
			plan, err := r.Operation.PlanBlobUpdate(ctx, r.Opctx)
			if err != nil {
				return err
			}
			// skip the blob-update phase (and its progress bar) when there is
			// nothing to copy/write
			if !plan.IsEmpty() {
				if err := blob.UpdateBlob(ctx, r.Progress, plan); err != nil {
					return err
				}
			}

			// 3. run the function responsible for updating the data, and write all of them on disk.
			//    Write lock on DB only hold here!
			if err := r.Operation.DBUpdate(ctx, r.Opctx, r.DBClient); err != nil {
				return err
			}
			if err := r.Opctx.JSONPool.FlushForWrite(); err != nil {
				return err
			}

			// 4. Best effort finish: presentation, cache update.
			//    all of what is done here should be re-derivable using a vit gc (todo)
			r.Operation.Finish(ctx, r.Opctx, r.DBClient, r.Logger, r.Presenter)

			return nil
		},
	)
	if err != nil {
		return err
	}

	// 5. Record the operation in the activity log — after a successful flush,
	//    outside the repo transaction lock, best-effort
	recordHistory(ctx, r, tids)
	return nil
}

// historyDescriber is optionally implemented by operations that want to add a
// command-specific summary to their activity-log entry.
type historyDescriber interface {
	HistorySummary() string
}

func recordHistory(ctx context.Context, r Request, tids []TransactionID) {
	subject := r.Ref
	if subject == nil && len(tids) > 0 {
		subject = tids[0].Ref
	}
	e := types.HistoryEntry{
		Command:   r.Command,
		Message:   r.Message,
		User:      r.User,
		Timestamp: r.Timestamp,
	}
	if subject != nil {
		e.Ref = subject
		e.AssetPath = subject.AssetPath
	}
	if d, ok := r.Operation.(historyDescriber); ok {
		e.Details = d.HistorySummary()
	}
	if err := history.Record(ctx, r.RepoPath, r.Opctx.Site(), e); err != nil {
		r.Logger.Warn("history: failed to record entry", "command", r.Command, "err", err)
	}
}

// withTransaction run a vit operation() within a transaction lock.
// Transactions is a layer added to prevent high level operation concurrency on a vit repo.
// (basic concurrency is handled through lockfiles in fsutil)
// This layer is "buiseness logic" awared and prevent conflicting operation such as:
//   - committing on a branch while someone else is already tagging it.
//   - moving an asset someone is currently committing on
//
// Vit (for now) has no queue system: either there is no conflicting operation going on
// Or it fails and user has to wait and retry.
//
// It works by declaring all the object that will be modified (using tids TransactionIDs)
// Running a validate() function. In case of success, transaction files will be written
// and keepalive for the duration of operation(), serving as defacto lockfile that can be
// used by the validate() method of other transaction.
func withTransaction(
	ctx context.Context,
	tids []TransactionID,
	command types.VitCommand,
	user string,
	timestamp time.Time,
	validate func() error,
	operation func() error,
) error {
	repos := listRepoPathsFromTids(tids)

	// 1. Lock every involved repo's transaction scan-lock, for a short time, so no
	//    new transaction is created in any of them while we scan for conflicts and
	//    stake our tids. They are acquired in a canonical (sorted) order so two
	//    multi-repo transactions can never deadlock, and held ONLY for the window
	//    below — never during operation().
	//    ! For that short window, each involved repo is read-only. This is the only
	//      place that happens.
	held, err := acquireScanLocks(ctx, repos)
	if err != nil {
		return err
	}

	var transactionFiles []string
	err = func() error {
		for _, repo := range repos {
			cleanOrphanedTransactions(fsutil.PathOS(repo, ".vit", "transaction"))
		}

		if verr := validate(); verr != nil {
			return verr
		}

		for _, tid := range tids {
			f := fsutil.PathOS(tid.Ref.RepoPath, ".vit", "transaction", tid.Encode())
			if werr := fsutil.NewJSONHandlerFromPath(f, &transactionPayload{
				AssetPath: tid.Ref.AssetPath,
				Operation: tid.Operation,
				Command:   command,
				User:      user,
				Modified:  timestamp,
			}).Write(); werr != nil {
				return werr
			}
			transactionFiles = append(transactionFiles, f)
		}
		return nil
	}()

	// 2. Release every scan-lock: a repo is read-only only for the window above.
	for _, d := range held {
		fsutil.ReleaseExclusiveLock(d)
	}
	if err != nil {
		for _, f := range transactionFiles { // roll back tids we managed to write
			_ = os.Remove(f)
		}
		return err
	}

	// 3. Keep transaction files alive while operating.
	done := make(chan struct{})
	go touchFilesLoop(ctx, transactionFiles, done)

	tx := &activeTransaction{files: transactionFiles, stop: func() { close(done) }}
	defer tx.finish()

	// 4. Do the actual work.
	return operation()
}

func listRepoPathsFromTids(tids []TransactionID) []string {
	seen := map[string]bool{}
	var repos []string
	for i := range tids {
		p := tids[i].Ref.RepoPath
		if !seen[p] {
			seen[p] = true
			repos = append(repos, p)
		}
	}
	sort.Strings(repos)
	return repos
}

func acquireScanLocks(ctx context.Context, repos []string) ([]string, error) {
	var held []string
	for _, repo := range repos {
		lockPath := fsutil.PathOS(repo, ".vit", "transaction.lock")
		lockDir, err := fsutil.AcquireExclusiveLock(ctx, lockPath)
		if err != nil {
			for _, d := range held {
				fsutil.ReleaseExclusiveLock(d)
			}
			return nil, err
		}
		held = append(held, lockDir)
	}
	return held, nil
}

type transactionPayload struct {
	AssetPath string               `json:"asset_path"`
	Operation TransactionOperation `json:"operation"`
	Command   types.VitCommand     `json:"command"`
	User      string               `json:"user"`
	Modified  time.Time            `json:"modified"`
}

func touchFilesLoop(ctx context.Context, files []string, done chan struct{}) {
	ticker := time.NewTicker(fsutil.KeepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			now := time.Now()
			for _, f := range files {
				_ = os.Chtimes(f, now, now)
			}
		}
	}
}

func cleanOrphanedTransactions(transactionDir string) {
	entries, err := os.ReadDir(transactionDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := fsutil.PathOS(transactionDir, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > fsutil.LockAcquireTimeout {
			_ = os.Remove(path)
		}
	}
}

type activeTransaction struct {
	files []string
	stop  func()
}

func (t *activeTransaction) finish() {
	if t.stop != nil {
		t.stop()
	}
	for _, f := range t.files {
		_ = os.Remove(f) // TODO(fsutil) atomic remove
	}
}
