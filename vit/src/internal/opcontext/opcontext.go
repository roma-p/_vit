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
	User        string
	TimeStamp   time.Time
	JSONPool    *fsutil.JSONHandlerPool
	CurrRepoUID string
	RefConfigs  map[string]config.RepoConfig // repo uid -> config of that repo.
}

func NewOperationContext(ctx context.Context, fullAssetPath string) (*OperationContext, string, error) {
	opctx := &OperationContext{
		RefConfigs: make(map[string]config.RepoConfig),
	}

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

	opctx.User = usr.Username
	opctx.TimeStamp = time.Now()
	opctx.JSONPool = fsutil.NewJSONHandlerPool()

	opctx.CurrRepoUID, err = opctx.AddRepoConfig(repoPath)
	if err != nil {
		return nil, "", err
	}

	return opctx, assetPath, nil
}

// Close releases all cached JSON handlers and any locks they hold.
func (o *OperationContext) Close() {
	o.JSONPool.ReleaseAll()
}

func (o *OperationContext) RepoPath() string {
	return o.RefConfigs[o.CurrRepoUID].RepoPath
}

func (o *OperationContext) Site() string {
	return o.RefConfigs[o.CurrRepoUID].CurrentSite
}

func (o *OperationContext) Repo() *types.RepoRef {
	return o.RepoRefFor(o.CurrRepoUID)
}

func (o *OperationContext) RepoRefFor(uid string) *types.RepoRef {
	c := o.RefConfigs[uid]
	return &types.RepoRef{UID: uid, Path: c.RepoPath, Site: c.CurrentSite}
}

func (o *OperationContext) AddRepoConfig(repoPath string) (string, error) {
	rConfig, err := config.LoadRepoConfig(repoPath)
	if err != nil {
		return "", err
	}
	o.RefConfigs[rConfig.RepoUID] = *rConfig
	return rConfig.RepoUID, nil
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
