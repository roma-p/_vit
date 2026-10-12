package vit

import (
	"context"

	"vit/internal/history"
	"vit/internal/opcontext"
	"vit/internal/types"
)

// Log history at repo wide or assert level.
func (c *Client) Log(ctx context.Context, path string) (*types.HistoryLogResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("Log", "path", path)

	var out *types.HistoryLogResult
	err := opcontext.WithOperationContext(ctx, path, func(opctx *opcontext.OperationContext, _ string) error {
		entries, err := history.ReadAll(opctx.RepoPath(), opctx.Site())
		if err != nil {
			return types.NewInternalVitError(
				types.ErrHistoryInternal, err,
				[]string{"could not read the activity log"}, nil,
			)
		}
		out = &types.HistoryLogResult{Entries: entries}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
