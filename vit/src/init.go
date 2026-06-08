package vit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"vit/internal/fsutil"
	"vit/internal/types"
)

func (c *Client) InitRepo(ctx context.Context, path string) (*types.StringResult, error) {
	c.logger.Info("InitRepo", "path", path)

	// Check if directory already exists
	if _, err := os.Stat(path); err == nil {
		return nil, newRepoInitFailed(
			path,
			[]string{"Directory already exists"},
			[]any{"error", "DirAlreadyExists"},
		)
	} else if !os.IsNotExist(err) {
		return nil, newRepoInitFailed(
			path,
			[]string{"can't access directory"},
			[]any{"error", "AccessError"},
		)
	}

	// Check if parent directory exists
	parentDir := filepath.Dir(path)
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		return nil, newRepoInitFailed(
			path,
			[]string{"parent directory does not exist"},
			[]any{"error", "ParentDirDoesNotExists"},
		)
	}

	// Check if trying to create repo within an existing repo
	// Use ignoreNonExisting=true since path doesn't exist yet
	if existingRepo, _, err := fsutil.FindVitRepoFromPath(path, true); err == nil {
		return nil, newRepoInitFailed(
			path,
			[]string{
				fmt.Sprintf("a repo already exists at: %s", existingRepo),
				"cannot create a vit repo within another vit repo",
			},
			[]any{"error", "NestedRepoError", "RepoPath", existingRepo},
		)
	}

	dirs := []string{
		path,
		filepath.Join(path, ".vit"),
		filepath.Join(path, ".vit", "tree"),
		filepath.Join(path, ".vit", "cache"),
		filepath.Join(path, ".vit", "assets"),
		filepath.Join(path, ".vit", "blob"),
		filepath.Join(path, ".vit", "workspace"),
		filepath.Join(path, ".vit", "attrs"),
		filepath.Join(path, ".vit", "transaction"),
	}

	for _, dir := range dirs {
		if err := os.Mkdir(dir, 0o755); err != nil {
			return nil, newRepoInitFailed(
				path,
				[]string{fmt.Sprintf("failed to create directory: %s", dir)},
				[]any{"error", "DirCreationError", "Dir", dir},
			)
		}
	}

	return &types.StringResult{String: path}, nil
}

func newRepoInitFailed(path string, message []string, extra []any) error {
	_extra := []any{"path", path}
	_extra = append(_extra, extra...)
	return types.NewTopLevelErrorWithStandardError(
		string(types.CommandInit),
		types.ErrRepoInitFailed,
		message,
		_extra,
	)
}
