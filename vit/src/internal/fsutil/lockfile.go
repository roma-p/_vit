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
	LockAcquireTimeout = 60 * time.Second
	initialBackoff     = 50 * time.Millisecond
	maxBackoff         = 1 * time.Second
)

// AcquireExclusiveLock acquires an exclusive lock using os.Mkdir (atomic on NFS).
// Returns the lock directory path for later release via ReleaseExclusiveLock.
func AcquireExclusiveLock(ctx context.Context, lockPath string) (string, error) {
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

// ReleaseExclusiveLock releases a lock acquired by AcquireExclusiveLock.
func ReleaseExclusiveLock(lockDir string) {
	_ = os.Remove(lockDir)
}

// WithExclusiveLock is a wrapper on AcquireExclusiveLock / ReleaseExclusiveLock
// Acquire the lock, and if sucesss return a a function that would release the lock upon call.
func WithExclusiveLock(ctx context.Context, lockPath string, operation func() error) error {
	lockDir, err := AcquireExclusiveLock(ctx, lockPath)
	if err != nil {
		return err
	}
	defer ReleaseExclusiveLock(lockDir)
	return operation()
}

func cleanOrphanedLock(lockDir string) {
	info, err := os.Stat(lockDir)
	if err != nil {
		return
	}
	if time.Since(info.ModTime()) > LockAcquireTimeout {
		_ = os.Remove(lockDir)
	}
}

func lockDirPath(lockPath string) string {
	dir := filepath.Dir(lockPath)
	base := filepath.Base(lockPath)
	return filepath.Join(dir, "."+base+".lock")
}

func newErrLockAcquireTimeout(ctx context.Context, lockPath string, attempts int) error {
	return types.NewStandardError(
		types.ErrLockAcquireTimeout,
		[]string{
			fmt.Sprintf("timeout acquiring lock on %s after %d attempts: %s",
				lockPath, attempts, ctx.Err()),
			"Another instance of vit may be working on the same resource",
		},
		[]any{"path", lockPath, "attempts", attempts},
	)
}
