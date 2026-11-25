package vit

import (
	"context"
	"maps"
	"slices"

	"vit/internal/db"
	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
)

func (c *Client) Query(ctx context.Context, path string, req types.QueryRequest) (*types.QueryResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("Query", "path", path, "request", req)

	if err := checkQueryRequest(req); err != nil {
		return nil, err
	}
	var out *types.QueryResult
	err := opcontext.WithOperationContext(ctx, path, func(opctx *opcontext.OperationContext, p string) error {
		dbClient, err := db.NewClient(ctx, opctx, c.logger)
		if err != nil {
			return err
		}
		defer dbClient.Close()

		out, err = dbClient.Query(ctx, opctx, opctx.RepoPath(), fsutil.PathInternal(p), req)
		if err != nil {
			return err
		}
		out.SetFetchOrder(req.Fetch)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// -- FILE LOCAL -------------------------------------------------------------

func checkQueryRequest(req types.QueryRequest) error {
	for _, k := range slices.Concat(req.With, req.Without, req.Fetch, slices.Collect(maps.Keys(req.Eq))) {
		if err := types.CheckAttrKey(k); err != nil {
			return err
		}
	}
	switch {
	case len(req.Fetch) == 0 && !req.Asset:
		return newQueryErr("nothing to return: fetch some keys or ask for the asset")
	case req.Distinct && req.Set:
		return newQueryErr("distinct and set are exclusive")
	case (req.Distinct || req.Set) && req.Asset:
		return newQueryErr("distinct and set return values, not assets: drop the asset")
	}
	return nil
}

func newQueryErr(message string) error {
	return types.NewVitError(types.ErrQuery, []string{message}, nil)
}
