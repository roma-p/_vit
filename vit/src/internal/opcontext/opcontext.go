package opcontext

import (
	"context"
	"os/user"
	"time"

	"vit/internal/config"
	"vit/internal/fsutil"
	"vit/internal/types"
)

// OperationContext bundles per-operation state:
// - info about the operation
// - JSONPool holding all json data used for that operation.
// - Repo config
type OperationContext struct {
	RepoPath   string
	RepoConfig *config.RepoConfig
	User       string
	TimeStamp  time.Time
	JSONPool   *fsutil.JSONHandlerPool
}

func NewOperationContext(ctx context.Context, fullAssetPath string) (*OperationContext, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", types.NewCancelledError(err)
	}

	fullAssetPath = fsutil.PathInternal(fullAssetPath)
	repoPath, assetPath, err := fsutil.FindVitRepoFromPath(fullAssetPath, true)
	if err != nil {
		return nil, "", err
	}
	usr, err := user.Current()
	if err != nil {
		return nil, "", err
	}

	repoConfig, err := config.LoadRepoConfig(repoPath)
	if err != nil {
		return nil, "", err
	}

	return &OperationContext{
		RepoPath:   repoPath,
		RepoConfig: repoConfig,
		User:       usr.Username,
		TimeStamp:  time.Now(),
		JSONPool:   fsutil.NewJSONHandlerPool(),
	}, assetPath, nil
}

// Close releases all cached JSON handlers and any locks they hold.
func (o *OperationContext) Close() {
	o.JSONPool.ReleaseAll()
}

// WithOperationContext create a context manager around operation context.
// calling this ensure json pool will be released.
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
