package db

import (
	"context"
	"log/slog"
	"os"

	"vit/internal/opcontext"
	"vit/internal/types"
)

type Client struct {
	TreeIndexPool map[string]*TreeIndex
	TreeCachePool map[string]*TreeCache
	Logger        *slog.Logger
	TreeStale     map[string]bool
}

func NewClient(
	cxt context.Context,
	opctx *opcontext.OperationContext,
	logger *slog.Logger,
) (*Client, error) {
	ret := Client{
		TreeIndexPool: make(map[string]*TreeIndex),
		TreeCachePool: make(map[string]*TreeCache),
		TreeStale:     make(map[string]bool),
		Logger:        logger,
	}

	_, err := ret.GetTreeIndex(cxt, opctx, opctx.RepoPath())
	if err != nil {
		return nil, err
	}

	return &ret, nil
}

func (c *Client) Close() {
	for p, s := range c.TreeStale {
		if !s {
			continue
		}
		if err := os.WriteFile(pathTreeStale(p), []byte{}, 0o644); err != nil {
			c.Logger.Error("Error invalidating tree caches", "repo", p)
		}
	}
}

func (c *Client) GetTreeIndex(
	cxt context.Context,
	opctx *opcontext.OperationContext,
	repoPath string,
) (*TreeIndex, error) {
	treeIndex, ok := c.TreeIndexPool[repoPath]
	if ok {
		return treeIndex, nil
	}

	treeIndex, err := getTreeIndex(cxt, opctx, repoPath)
	if err != nil {
		return nil, err
	}

	c.TreeIndexPool[repoPath] = treeIndex
	return treeIndex, nil
}

func (c *Client) GetTreeCache(
	cxt context.Context,
	opctx *opcontext.OperationContext,
	repoPath string,
) (*TreeCache, error) {
	treeCache, ok := c.TreeCachePool[repoPath]
	if ok {
		return treeCache, nil
	}

	treeCache, err := getTreeCache(cxt, opctx, repoPath)
	if err != nil {
		return nil, err
	}

	c.TreeCachePool[repoPath] = treeCache
	return treeCache, nil
}

func (c *Client) MarkTreeStale(repoPath string) {
	c.TreeStale[repoPath] = true
}

// -- FILE LOCAL -------------------------------------------------------------

func newErrDBInternal(nestedErr error, message string) error {
	return types.NewInternalVitError(types.ErrDBInternal, nestedErr, []string{message}, nil)
}
