package fsutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"vit/internal/types"
)

const (
	lockAcquireTimeout = 60 * time.Second
	keepaliveInterval  = 2 * time.Second
	initialBackoff     = 50 * time.Millisecond
	maxBackoff         = 1 * time.Second
)

// AcquireExclusiveLock acquires an exclusive lock using os.Mkdir (atomic on NFS).
// Returns the lock directory path for later release via ReleaseExclusiveLock.
func AcquireExclusiveLock(ctx context.Context, lockPath string) (string, error) {
	lockDir := lockDirPath(lockPath)

	ctx, cancel := context.WithTimeout(ctx, lockAcquireTimeout)
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
			if mkErr := os.MkdirAll(filepath.Dir(lockDir), 0o755); mkErr != nil {
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
		ticker := time.NewTicker(keepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				now := time.Now()
				_ = os.Chtimes(lockDir, now, now)
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

// WithExclusiveLockKeepalive is like WithExclusiveLock but keeps the lock alive
// for the duration of the operation. Use for long-running operations.
func WithExclusiveLockKeepalive(ctx context.Context, lockPath string, operation func() error) error {
	stop, err := AcquireExclusiveLockWithKeepalive(ctx, lockPath)
	if err != nil {
		return err
	}
	defer stop()
	return operation()
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
	lockDir := lockDirPath(lockPath)

	ctx, cancel := context.WithTimeout(ctx, lockAcquireTimeout)
	defer cancel()

	backoff := initialBackoff
	for {
		info, err := os.Stat(lockDir)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to stat lock: %w", err)
		}

		// Orphaned lock — clean it and we're done.
		if time.Since(info.ModTime()) > lockAcquireTimeout {
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

func cleanOrphanedLock(lockDir string) {
	info, err := os.Stat(lockDir)
	if err != nil {
		return
	}
	if time.Since(info.ModTime()) > lockAcquireTimeout {
		_ = os.Remove(lockDir)
	}
}

func lockDirPath(lockPath string) string {
	dir := filepath.Dir(lockPath)
	base := filepath.Base(lockPath)
	return filepath.Join(dir, "."+base+".lock")
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
