package vit

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"vit/internal/db"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

func (c *Client) AttrSet(ctx context.Context, path string, values map[string]string) (*types.AttrResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("AttrSet", "path", path, "values", values)

	if len(values) == 0 {
		return nil, newAttrErr("no attribute to set", nil)
	}
	for k := range values {
		if err := types.CheckAttrKey(k); err != nil {
			return nil, err
		}
	}
	return c.runAttr(ctx, path, types.CommandAttrSet, values, nil)
}

func (c *Client) AttrRm(ctx context.Context, path string, keys []string) (*types.AttrResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("AttrRm", "path", path, "keys", keys)

	if len(keys) == 0 {
		return nil, newAttrErr("no attribute to remove", nil)
	}
	for _, k := range keys {
		if err := types.CheckAttrKey(k); err != nil {
			return nil, err
		}
	}
	return c.runAttr(ctx, path, types.CommandAttrRm, nil, keys)
}

func (c *Client) AttrLs(ctx context.Context, path string) (*types.AttrResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, types.NewCancelledError(err)
	}
	c.logger.Info("AttrLs", "path", path)

	var out *types.AttrResult
	err := opcontext.WithOperationContext(ctx, path, func(opctx *opcontext.OperationContext, p string) error {
		dbClient, err := db.NewClient(ctx, opctx, c.logger)
		if err != nil {
			return err
		}
		defer dbClient.Close()

		treePath, err := dbClient.ResolveAssetPath(ctx, opctx, opctx.RepoPath(), p)
		if err != nil {
			return err
		}

		node, err := dbClient.GetTreeNode(ctx, opctx, opctx.RepoPath(), treePath)
		if err != nil {
			return err
		}
		out = newAttrResult(treePath, node)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

type operationAttr struct {
	// input given at init
	RepoRef  *types.RepoRef
	TreePath string
	Set      map[string]string
	Rm       []string

	// filled at Validate, then DBUpdate
	node *types.TreeNode
}

func (o *operationAttr) Locks() transaction.Locks {
	return transaction.Locks{
		Edited: []*types.Ref{types.NewRefEmpty(o.RepoRef.Path, o.TreePath)},
		EditOp: transaction.TransactionAttr,
	}
}

func (o *operationAttr) Validate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	node, err := dbClient.GetTreeNode(ctx, opctx, o.RepoRef.Path, o.TreePath)
	if err != nil {
		return err
	}
	live := node.LiveAttrs()
	for _, k := range o.Rm {
		if _, ok := live[k]; !ok {
			return newAttrErr(fmt.Sprintf("no attribute %q on %s", k, o.TreePath), []any{"key", k, "path", o.TreePath})
		}
	}
	o.node = node
	return nil
}

func (o *operationAttr) PlanBlobUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
) (*types.BlobUpdatePlan, error) {
	return &types.BlobUpdatePlan{}, nil
}

func (o *operationAttr) DBUpdate(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
) error {
	h, err := dbClient.GetTreeNodeForWrite(ctx, opctx, o.RepoRef.Path, o.TreePath)
	if err != nil {
		return err
	}
	for _, k := range sortedKeys(o.Set) {
		h.Data.SetAttr(k, o.Set[k], opctx.User, opctx.TimeStamp)
	}
	for _, k := range o.Rm {
		if err := h.Data.RemoveAttr(k, opctx.User, opctx.TimeStamp); err != nil {
			return err
		}
	}
	o.node = h.Data
	return nil
}

func (o *operationAttr) Finish(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	dbClient *db.Client,
	logger *slog.Logger,
	p workspace.Presenter,
) {
}

func (o *operationAttr) HistorySummary() string {
	parts := make([]string, 0, len(o.Set)+len(o.Rm))
	for _, k := range sortedKeys(o.Set) {
		parts = append(parts, k+"="+o.Set[k])
	}
	for _, k := range o.Rm {
		parts = append(parts, "-"+k)
	}
	return strings.Join(parts, " ")
}

func (o *operationAttr) HistoryAttrs() (string, []types.AttrChange) {
	changes := make([]types.AttrChange, 0, len(o.Set)+len(o.Rm))
	for _, k := range sortedKeys(o.Set) {
		changes = append(changes, types.AttrChange{Key: k, Value: o.Set[k]})
	}
	for _, k := range o.Rm {
		changes = append(changes, types.AttrChange{Key: k, Removed: true})
	}
	return o.node.ID, changes
}

// -- FILE LOCAL -------------------------------------------------------------

func (c *Client) runAttr(
	ctx context.Context,
	path string,
	command types.VitCommand,
	set map[string]string,
	rm []string,
) (*types.AttrResult, error) {
	presenter, err := workspace.ResolvePresenter(ctx, c.logger)
	if err != nil {
		return nil, err
	}

	var out *types.AttrResult
	err = opcontext.WithOperationContext(ctx, path, func(opctx *opcontext.OperationContext, p string) error {
		dbClient, err := db.NewClient(ctx, opctx, c.logger)
		if err != nil {
			return err
		}
		defer dbClient.Close()

		treePath, err := dbClient.ResolveAssetPath(ctx, opctx, opctx.RepoPath(), p)
		if err != nil {
			return err
		}

		op := &operationAttr{RepoRef: opctx.Repo(), TreePath: treePath, Set: set, Rm: rm}
		if err := transaction.Run(
			ctx,
			transaction.Request{
				Opctx:     opctx,
				DBClient:  dbClient,
				Presenter: presenter,
				Operation: op,
				Logger:    c.logger,
				Progress:  c.progress,
				Command:   command,
				User:      opctx.User,
				RepoPath:  opctx.RepoPath(),
				Timestamp: opctx.TimeStamp,
				Ref:       types.NewRefEmpty(opctx.RepoPath(), treePath),
			},
		); err != nil {
			return err
		}
		out = newAttrResult(treePath, op.node)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func newAttrResult(treePath string, node *types.TreeNode) *types.AttrResult {
	live := node.LiveAttrs()
	out := &types.AttrResult{Path: treePath, Attrs: make([]types.AttrEntry, 0, len(live))}
	for _, k := range sortedKeys(live) {
		a := live[k]
		out.Attrs = append(out.Attrs, types.AttrEntry{Key: k, Value: a.Value, User: a.User, TS: a.TS})
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func newAttrErr(message string, extra []any) error {
	return types.NewVitError(types.ErrAttr, []string{message}, extra)
}
