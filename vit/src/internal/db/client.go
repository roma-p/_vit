package db

import (
	"context"
	"log/slog"

	"vit/internal/opcontext"
)

type Client struct {
	TreeIndexPool map[string]*TreeIndex
	Logger        *slog.Logger
}

func NewClient(
	cxt context.Context,
	opctx *opcontext.OperationContext,
	logger *slog.Logger,
) (*Client, error) {
	ret := Client{TreeIndexPool: make(map[string]*TreeIndex), Logger: logger}

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

	treeIndex, err := GetTreeIndex(cxt, opctx, repoPath)
	if err != nil {
		return nil, err
	}

	c.TreeIndexPool[repoPath] = treeIndex
	return treeIndex, nil
}
