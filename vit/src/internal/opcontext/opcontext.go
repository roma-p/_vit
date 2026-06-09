package opcontext

import (
	"context"
	"os/user"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

// OperationContext bundles per-operation state:
// - info about the operation
// - JSONPool holding all json data used for that operation.
type OperationContext struct {
	RepoPath  string
	User      string
	TimeStamp time.Time
	JSONPool  *fsutil.JSONHandlerPool
}

func NewOperationContext(ctx context.Context, fullAssetPath string) (*OperationContext, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", types.NewCancelledError(err)
	}

	repoPath, assetPath, err := fsutil.FindVitRepoFromPath(fullAssetPath, true)
	if err != nil {
		return nil, "", err
	}
	usr, err := user.Current()
	if err != nil {
		return nil, "", err
	}

	return &OperationContext{
		RepoPath:  repoPath,
		User:      usr.Username,
		TimeStamp: time.Now(),
		JSONPool:  fsutil.NewJSONHandlerPool(),
	}, assetPath, nil
}

// Close releases all cached JSON handlers and any locks they hold.
// Intended to be called via defer at the top of an operation.
func (o *OperationContext) Close() {
	o.JSONPool.ReleaseAll()
}

// WithOperationContext creates an OperationContext, runs fn, and guarantees
// Close is called when fn returns. This is the preferred way to use an
// OperationContext — it makes it impossible to forget cleanup.
func WithOperationContext(
	ctx context.Context,
	path string,
	fn func(*OperationContext, string) error,
) error {
	opctx, resolvedPath, err := NewOperationContext(ctx, path)
	if err != nil {
		return err
	}
	defer opctx.Close()
	return fn(opctx, resolvedPath)
}
