package db

import (
	"context"
	"log/slog"

	"vit/internal/opcontext"
)

type Client struct {
	TreeIndexPool map[string]*TreeIndex
	TreeCachePool map[string]*TreeCache
	Logger        *slog.Logger
}

func NewClient(
	cxt context.Context,
	opctx *opcontext.OperationContext,
	logger *slog.Logger,
) (*Client, error) {
	ret := Client{
		TreeIndexPool: make(map[string]*TreeIndex),
		TreeCachePool: make(map[string]*TreeCache),
		Logger:        logger,
	}

	_, err := ret.GetTreeIndex(cxt, opctx, opctx.RepoPath)
	if err != nil {
		return nil, err
	}

	return &ret, nil
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
