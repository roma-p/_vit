package vit

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"log/slog"
	"vit/internal/db"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
	"vit/internal/workspace"
)

type operationAttr struct {
	// input given at init
	RepoRef  *types.RepoRef
	TreePath string
	Set      map[string]string
	Rm       []string

	// filled at DBUpdate
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

// -- FILE LOCAL -------------------------------------------------------------

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
