package fsutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"vit/internal/types"
)

const (
	LockAcquireTimeout = 60 * time.Second
	KeepaliveInterval  = 2 * time.Second
	initialBackoff     = 50 * time.Millisecond
	maxBackoff         = 1 * time.Second
)

// AcquireExclusiveLock acquires an exclusive lock using os.Mkdir (atomic on NFS).
// Returns the lock directory path for later release via ReleaseExclusiveLock.
func AcquireExclusiveLock(ctx context.Context, lockPath string) (string, error) {
	lockPath = PathOS(lockPath)
	lockDir := lockDirPath(lockPath)

	ctx, cancel := context.WithTimeout(ctx, LockAcquireTimeout)
	defer cancel()

	cleanOrphanedLock(lockDir)

	backoff := initialBackoff
	attempts := 0

	for {
		attempts++

		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			return lockDir, nil
		}

		if !os.IsExist(err) {
			if mkErr := os.MkdirAll(DirOS(lockDir), 0o755); mkErr != nil {
				return "", fmt.Errorf("failed to create parent directory for lock: %w", mkErr)
			}
			if err2 := os.Mkdir(lockDir, 0o755); err2 == nil {
				return lockDir, nil
			}
		}

		cleanOrphanedLock(lockDir)

		select {
		case <-ctx.Done():
			return "", newErrLockAcquireTimeout(ctx, lockPath, attempts)
		case <-time.After(backoff):
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

// AcquireExclusiveLockWithKeepalive is like AcquireExclusiveLock but spawns a
// goroutine that periodically touches the lock directory to prevent orphan cleanup.
// Use this for long-running operations (file copies, transactions).
// The returned stop function stops the keepalive and releases the lock.
func AcquireExclusiveLockWithKeepalive(ctx context.Context, lockPath string) (stop func(), err error) {
	lockDir, err := AcquireExclusiveLock(ctx, lockPath)
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(KeepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				_ = TouchNow(lockDir)
			}
		}
	}()

	return func() {
		close(done)
		ReleaseExclusiveLock(lockDir)
	}, nil
}

// ReleaseExclusiveLock releases a lock acquired by AcquireExclusiveLock.
func ReleaseExclusiveLock(lockDir string) {
	_ = os.Remove(lockDir)
}

// WithExclusiveLock is a wrapper on AcquireExclusiveLock / ReleaseExclusiveLock.
// Acquires the lock, runs the operation, and releases the lock.
func WithExclusiveLock(ctx context.Context, lockPath string, operation func() error) error {
	lockDir, err := AcquireExclusiveLock(ctx, lockPath)
	if err != nil {
		return err
	}
	defer ReleaseExclusiveLock(lockDir)
	return operation()
}

// WaitForLockRelease blocks until no active lock is held at lockPath.
// Returns immediately if the lock doesn't exist.
// Cleans orphaned locks (stale mtime) and returns immediately after cleanup.
func WaitForLockRelease(ctx context.Context, lockPath string) error {
	lockPath = PathOS(lockPath)
	lockDir := lockDirPath(lockPath)

	ctx, cancel := context.WithTimeout(ctx, LockAcquireTimeout)
	defer cancel()

	backoff := initialBackoff
	for {
		_, err := os.Stat(lockDir)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to stat lock: %w", err)
		}
		stale, err := IsStaleAt(lockDir, NowAt(DirOS(lockDir)), LockAcquireTimeout)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to stat lock: %w", err)
		}

		// Orphaned lock — clean it and we're done.
		if stale {
			_ = os.Remove(lockDir)
			return nil
		}

		select {
		case <-ctx.Done():
			return newErrLockAcquireTimeout(ctx, lockPath, 0)
		case <-time.After(backoff):
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

// -- FILE LOCAL -------------------------------------------------------------

func cleanOrphanedLock(lockDir string) {
	if _, err := os.Stat(lockDir); err != nil {
		return
	}
	if stale, _ := IsStaleAt(lockDir, NowAt(DirOS(lockDir)), LockAcquireTimeout); stale {
		_ = os.Remove(lockDir)
	}
}

var (
	ErrLockLost        = errors.New("lock lost")
	ErrConcurrentWrite = errors.New("lock lost during the write")
)

func acquireOwnedLock(ctx context.Context, path string) (string, os.FileInfo, error) {
	lockDir, err := AcquireExclusiveLock(ctx, path)
	if err != nil {
		return "", nil, err
	}
	lockID, err := os.Stat(lockDir)
	if err != nil {
		ReleaseExclusiveLock(lockDir)
		return "", nil, err
	}
	return lockDir, lockID, nil
}

func holdsLock(lockDir string, id os.FileInfo) bool {
	info, err := os.Stat(lockDir)
	return err == nil && os.SameFile(info, id)
}

func releaseOwnedLock(lockDir string, id os.FileInfo) {
	if holdsLock(lockDir, id) {
		ReleaseExclusiveLock(lockDir)
	}
}

func lockDirPath(lockPath string) string {
	dir := DirOS(lockPath)
	base := BaseOS(lockPath)
	return PathOS(dir, "."+base+".lock")
}

func newErrLockAcquireTimeout(ctx context.Context, lockPath string, attempts int) error {
	return types.NewVitError(
		types.ErrLockAcquireTimeout,
		[]string{
			fmt.Sprintf("timeout acquiring lock on %s after %d attempts: %s",
				lockPath, attempts, ctx.Err()),
			"Another instance of vit may be working on the same resource",
		},
		[]any{"path", lockPath, "attempts", attempts},
	)
}
