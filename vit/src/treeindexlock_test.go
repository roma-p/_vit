package vit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"vit/internal/db"
	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/transaction"
	"vit/internal/types"
)

func TestAddSeesAnAssetAddedAfterTheTreeIndexWasLoaded(t *testing.T) {
	ctx := context.Background()
	client, repoPath, _ := newPreparedRepo(t)
	src := filepath.Join(t.TempDir(), "leg.txt")
	if err := os.WriteFile(src, []byte("LEG"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := opcontext.WithOperationContext(ctx, filepath.Join(repoPath, "props", "chair", "leg"), func(opctx *opcontext.OperationContext, assetPath string) error {
		assetPath = fsutil.PathInternal(assetPath)
		dbClient, err := db.NewClient(ctx, opctx, client.logger)
		if err != nil {
			return err
		}
		defer dbClient.Close()

		if _, err := client.Add(ctx, filepath.Join(repoPath, "props", "chair"), src); err != nil {
			t.Fatal(err)
		}

		return transaction.RunTreeChanged(ctx, transaction.Request{
			Opctx:     opctx,
			DBClient:  dbClient,
			Operation: newOperationAdd(opctx.Repo(), assetPath, src, nil),
			Logger:    client.logger,
			Command:   types.CommandAdd,
			User:      opctx.User,
			RepoPath:  opctx.RepoPath(),
			Timestamp: opctx.TimeStamp,
		}, []string{assetPath})
	})
	if !types.IsErrCode(err, types.ErrDBTreeNode) {
		t.Fatalf("want %s, got %v", types.ErrDBTreeNode, err)
	}
}
