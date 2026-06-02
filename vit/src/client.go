package vit

import (
	"context"
	"log/slog"

	"vit/internal/types"
)

type Client struct {
	logger   *slog.Logger
	progress types.ProgressWriter
}

func NewClient(ctx context.Context, logger *slog.Logger, progress types.ProgressWriter) (*Client, error) {
	if progress == nil {
		progress = &types.ProgressWriterEmpty{}
	}

	return &Client{
		logger:   logger,
		progress: progress,
	}, nil
}

func (c *Client) Dispose() {
}
