package vit

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"vit/internal/db"
	"vit/internal/opcontext"
	testutils "vit/internal/testhelpers"
	"vit/internal/types"
	"vit/internal/workspace"
)

func TestCommitValidateReusesThePreparedChangeRecord(t *testing.T) {
	client, branchPath := preparedRepo(t)
	op := prepareCommit(t, client, branchPath)

	if err := os.WriteFile(filepath.Join(branchPath, "b.txt"), []byte("BBB"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := validateCommit(t, client, branchPath, op); err != nil {
		t.Fatal(err)
	}
	if op.changeRecord != op.prepared.ChangeRecord {
		t.Fatal("change record recomputed under the lock")
	}
}

func TestCommitValidateRecomputesWhenTheBranchHeadMoved(t *testing.T) {
	client, branchPath := preparedRepo(t)
	op := prepareCommit(t, client, branchPath)

	if _, err := client.Snapshot(context.Background(), branchPath, "other"); err != nil {
		t.Fatal(err)
	}

	err := validateCommit(t, client, branchPath, op)
	if !types.IsErrCode(err, types.ErrNoChangeToCommit) {
		t.Fatalf("want %s, got %v", types.ErrNoChangeToCommit, err)
	}
}

// -- FILE LOCAL -------------------------------------------------------------

func preparedRepo(t *testing.T) (*Client, string) {
	t.Helper()
	ctx := context.Background()
	dir, cleanup := testutils.TempDir(t, "vit-repo-*")
	t.Cleanup(cleanup)
	repoPath := filepath.Join(dir, "repo")
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("AAA"), 0o644); err != nil {
		t.Fatal(err)
	}

	client, err := NewClient(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.InitRepo(ctx, repoPath); err != nil {
		t.Fatal(err)
	}
	added, err := client.Add(ctx, filepath.Join(repoPath, "props", "table"), src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(added.String, "a.txt"), []byte("WWW"), 0o644); err != nil {
		t.Fatal(err)
	}
	return client, added.String
}

func prepareCommit(t *testing.T, client *Client, branchPath string) *operationCommit {
	t.Helper()
	var op *operationCommit
	withCommitContext(t, client, branchPath, func(ctx context.Context, opctx *opcontext.OperationContext, dbClient *db.Client, ref *types.Ref) error {
		op = &operationCommit{Ref: ref, Message: "edit"}
		return op.Prepare(ctx, opctx, dbClient)
	})
	return op
}

func validateCommit(t *testing.T, client *Client, branchPath string, op *operationCommit) error {
	t.Helper()
	var err error
	withCommitContext(t, client, branchPath, func(ctx context.Context, opctx *opcontext.OperationContext, dbClient *db.Client, _ *types.Ref) error {
		err = op.Validate(ctx, opctx, dbClient)
		return nil
	})
	return err
}

func withCommitContext(
	t *testing.T,
	client *Client,
	branchPath string,
	fn func(context.Context, *opcontext.OperationContext, *db.Client, *types.Ref) error,
) {
	t.Helper()
	ctx := context.Background()
	err := opcontext.WithOperationContext(ctx, branchPath, func(opctx *opcontext.OperationContext, path string) error {
		ref, err := workspace.ResolveRefFromPath(ctx, opctx, path)
		if err != nil {
			return err
		}
		dbClient, err := db.NewClient(ctx, opctx, client.logger)
		if err != nil {
			return err
		}
		defer dbClient.Close()
		return fn(ctx, opctx, dbClient, ref)
	})
	if err != nil {
		t.Fatal(err)
	}
}
