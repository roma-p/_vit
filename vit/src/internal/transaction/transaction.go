package transaction

import (
	"context"
	"log/slog"
	"os"
	"time"

	"vit/internal/blob"
	"vit/internal/db"
	"vit/internal/fsutil"
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
	return run(ctx, r, func() error { return validateTreeChanged(r.RepoPath, assetPaths) }, tids)
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
	return run(ctx, r, func() error { return validateEdit(r.RepoPath, refs) }, tids)
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
}

// -- FILE LOCAL -------------------------------------------------------------

func run(
	ctx context.Context,
	r Request,
	extraValidate func() error,
	tids []TransactionID,
) error {
	return withTransaction(ctx, r.RepoPath, tids, r.Command, r.User, r.Timestamp,
		func() error {
			// 1. domain validate (conflict scan) first, then operation validate.
			if err := extraValidate(); err != nil {
				return err
			}
			if err := r.Operation.Validate(ctx, r.Opctx, r.DBClient); err != nil {
				return err
			}
			return nil
		},
		func() error {
			// 3. Plan the object-tier writes (writes nothing), then execute them
			//    (copy blobs, write content nodes + manifests).
			plan, err := r.Operation.PlanBlobUpdate(ctx, r.Opctx)
			if err != nil {
				return err
			}
			if err := blob.UpdateBlob(ctx, r.Progress, plan); err != nil {
				return err
			}

			// 4. run the function responsible for updating the data, and write all of them on disk.
			//    Write lock on DB only hold here!
			if err := r.Operation.DBUpdate(ctx, r.Opctx, r.DBClient); err != nil {
				return err
			}
			if err := r.Opctx.JSONPool.FlushForWrite(); err != nil {
				return err
			}

			// 5. Best effort finish.
			r.Operation.Finish(ctx, r.Opctx, r.DBClient, r.Logger, r.Presenter)

			return nil
		},
	)
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
	repoPath string,
	tids []TransactionID,
	command types.VitCommand,
	user string,
	timestamp time.Time,
	validate func() error,
	operation func() error,
) error {
	lockPath := fsutil.PathOS(repoPath, ".vit", "transaction.lock")
	transactionDir := fsutil.PathOS(repoPath, ".vit", "transaction")

	var transactionFiles []string

	// 1. Lock all transaction for a short time
	//    We don't want a new transaction beeing created while we scan all of them
	//    to detect conflict.
	//    ! This imply that for that short time, the entire repo is read only!
	//      This is the only place where it happens
	err := fsutil.WithExclusiveLock(ctx, lockPath, func() error {
		cleanOrphanedTransactions(transactionDir)

		if err := validate(); err != nil {
			return err
		}

		for _, tid := range tids {
			f := fsutil.PathOS(transactionDir, tid.Encode())
			transactionFiles = append(transactionFiles, f)
			err := fsutil.NewJSONHandlerFromPath(f, &transactionPayload{
				AssetPath: tid.Ref.AssetPath,
				Operation: tid.Operation,
				Command:   command,
				User:      user,
				Modified:  timestamp,
			}).Write()
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 2. Keep transaction files alive while operating.
	done := make(chan struct{})
	go touchFilesLoop(ctx, transactionFiles, done)

	tx := &activeTransaction{files: transactionFiles, stop: func() { close(done) }}
	defer tx.finish()

	// 3. Do the actual work.
	return operation()
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
