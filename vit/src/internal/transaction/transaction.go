package transaction

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

type activeTransaction struct {
	files []string
	stop  func()
}

func (t *activeTransaction) finish() {
	if t.stop != nil {
		t.stop()
	}
	for _, f := range t.files {
		_ = os.Remove(f)
	}
}

// WithTransactionTreeChanged wraps an operation that mutates the tree (add/move/delete).
// Conflicts with ANY existing transaction on overlapping paths.
func WithTransactionTreeChanged(
	ctx context.Context,
	repoPath string,
	assetPaths []string,
	command types.VitCommand,
	user string,
	timestamp time.Time,
	operation func() error,
) error {
	tids := make([]TransactionID, len(assetPaths))
	for i, p := range assetPaths {
		tids[i] = newTransactionID(TransactionTreeChanged, types.NewRefEmpty(repoPath, p))
	}
	return withTransaction(ctx, repoPath, tids, command, user, timestamp,
		func() error { return validateTreeChanged(repoPath, assetPaths) },
		operation,
	)
}

// WithTransactionEdit wraps an operation that edits existing assets (commit/branch).
// Conflicts with TreeChanged on overlapping paths, or same-ref operations.
func WithTransactionEdit(
	ctx context.Context,
	repoPath string,
	refs []*types.Ref,
	op TransactionOperation,
	command types.VitCommand,
	user string,
	timestamp time.Time,
	operation func() error,
) error {
	tids := make([]TransactionID, len(refs))
	for i, r := range refs {
		tids[i] = newTransactionID(op, r)
	}
	return withTransaction(ctx, repoPath, tids, command, user, timestamp,
		func() error { return validateEdit(repoPath, refs) },
		operation,
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
	lockPath := filepath.Join(repoPath, ".vit", "transaction.lock")
	transactionDir := filepath.Join(repoPath, ".vit", "transaction")

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
			f := filepath.Join(transactionDir, tid.Encode())
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

// transactionPayload is the JSON content written inside each transaction file.
// Only read when reporting conflicts to the user.
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
		path := filepath.Join(transactionDir, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > fsutil.LockAcquireTimeout {
			_ = os.Remove(path)
		}
	}
}
