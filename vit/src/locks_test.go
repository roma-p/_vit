package vit

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"vit/internal/db"
	"vit/internal/opcontext"
	testutils "vit/internal/testhelpers"
	"vit/internal/transaction"
	"vit/internal/types"
)

func TestOperationLocks(t *testing.T) {
	env := newLocksEnv(t)
	env.add("shot/anim")
	env.must(env.client.ProductAdd(ctx(), env.branch("shot/anim"), env.abs("render/beauty"), "render", true))
	env.edit("shot/anim")
	env.must(env.client.Snapshot(ctx(), env.branch("shot/anim"), "with render"))
	env.add("asset/rig")
	env.must(env.client.Tag(ctx(), env.branch("asset/rig"), "approved", "v1"))
	for _, consumer := range []string{"seq/a/anim", "seq/b/anim"} {
		env.add(consumer)
		env.must(env.client.SourceAdd(ctx(), env.branch(consumer), env.abs("asset/rig")+"@tag=approved", "rig"))
	}

	cases := []struct {
		name string
		path string
		op   func(opctx *opcontext.OperationContext, dbClient *db.Client, ref *types.Ref) transaction.Operation
		want []string
	}{
		{
			name: "add",
			path: env.branch("shot/anim"),
			op: func(opctx *opcontext.OperationContext, _ *db.Client, _ *types.Ref) transaction.Operation {
				return newOperationAdd(opctx.Repo(), "props/new", "", nil)
			},
			want: []string{"TreeChanged props/new"},
		},
		{
			name: "snapshot with a product",
			path: env.branch("shot/anim"),
			op: func(_ *opcontext.OperationContext, _ *db.Client, ref *types.Ref) transaction.Operation {
				return &operationCommit{Ref: ref, Message: "m", IsSnapshot: true}
			},
			want: []string{"Branch render/beauty@branch=base", "Branch shot/anim@branch=base"},
		},
		{
			name: "tag a branch with a product",
			path: env.branch("shot/anim"),
			op: func(_ *opcontext.OperationContext, _ *db.Client, ref *types.Ref) transaction.Operation {
				return &operationTag{InputRef: ref, TagFamily: "approved", Message: "m"}
			},
			want: []string{"Branch render/beauty@branch=base", "Branch shot/anim@branch=base"},
		},
		{
			name: "tag a commit with a product",
			path: env.branch("shot/anim"),
			op: func(_ *opcontext.OperationContext, _ *db.Client, ref *types.Ref) transaction.Operation {
				return &operationTag{InputRef: env.headCommit(ref), TagFamily: "approved", Message: "m"}
			},
			want: []string{"Tag render/beauty@branch=base", "Tag shot/anim@commit=" + env.headCommitID("shot/anim")},
		},
		{
			name: "branch with a product",
			path: env.branch("shot/anim"),
			op: func(_ *opcontext.OperationContext, _ *db.Client, ref *types.Ref) transaction.Operation {
				return &operationBranch{InputRef: ref, BranchNames: []string{"wip"}}
			},
			want: []string{"Branch render/beauty@branch=base", "Branch shot/anim@branch=base"},
		},
		{
			name: "source push restricted to one consumer",
			path: env.abs("asset/rig") + "@tag=approved",
			op: func(_ *opcontext.OperationContext, _ *db.Client, ref *types.Ref) transaction.Operation {
				return &operationSourcePush{Producer: ref, restrict: map[string]bool{"seq/b/anim@branch=base": true}}
			},
			want: []string{"Branch seq/b/anim@branch=base"},
		},
		{
			name: "product add creating the target",
			path: env.branch("shot/anim"),
			op: func(_ *opcontext.OperationContext, _ *db.Client, ref *types.Ref) transaction.Operation {
				return &operationProductAdd{RefOrigin: ref, RefTarget: types.NewRefBranch(ref.RepoRef(), "render/depth", types.DefaultBranchName), ProductName: "depth", Create: true}
			},
			want: []string{"Branch shot/anim@branch=base", "TreeChanged render/depth"},
		},
		{
			name: "product add creating the target branch",
			path: env.branch("shot/anim"),
			op: func(_ *opcontext.OperationContext, _ *db.Client, ref *types.Ref) transaction.Operation {
				return &operationProductAdd{RefOrigin: ref, RefTarget: types.NewRefBranch(ref.RepoRef(), "render/beauty", "wip"), ProductName: "wip", Create: true}
			},
			want: []string{"Branch render/beauty@branch=wip", "Branch shot/anim@branch=base"},
		},
		{
			name: "product add attaching an existing target",
			path: env.branch("shot/anim"),
			op: func(_ *opcontext.OperationContext, _ *db.Client, ref *types.Ref) transaction.Operation {
				return &operationProductAdd{RefOrigin: ref, RefTarget: types.NewRefBranch(ref.RepoRef(), "render/beauty", types.DefaultBranchName), ProductName: "again"}
			},
			want: []string{"Branch shot/anim@branch=base"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			withCommitContext(t, env.client, c.path, func(ctx context.Context, opctx *opcontext.OperationContext, dbClient *db.Client, ref *types.Ref) error {
				op := c.op(opctx, dbClient, ref)
				if p, ok := op.(interface {
					Prepare(context.Context, *opcontext.OperationContext, *db.Client) error
				}); ok {
					if err := p.Prepare(ctx, opctx, dbClient); err != nil {
						return err
					}
				}
				got = lockKeys(op.Locks())
				return nil
			})
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

// -- FILE LOCAL -------------------------------------------------------------

type locksEnv struct {
	t          *testing.T
	client     *Client
	repoPath   string
	workspaces map[string]string
}

func newLocksEnv(t *testing.T) *locksEnv {
	t.Helper()
	dir, cleanup := testutils.TempDir(t, "vit-repo-*")
	t.Cleanup(cleanup)
	client, err := NewClient(ctx(), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatal(err)
	}
	env := &locksEnv{t: t, client: client, repoPath: filepath.Join(dir, "repo"), workspaces: map[string]string{}}
	env.must(client.InitRepo(ctx(), env.repoPath))
	return env
}

func ctx() context.Context { return context.Background() }

func (e *locksEnv) abs(assetPath string) string { return filepath.Join(e.repoPath, assetPath) }

func (e *locksEnv) branch(assetPath string) string { return e.abs(assetPath) + "@branch=base" }

func (e *locksEnv) add(assetPath string) {
	e.t.Helper()
	src := filepath.Join(e.t.TempDir(), "f.txt")
	if err := os.WriteFile(src, []byte(assetPath), 0o644); err != nil {
		e.t.Fatal(err)
	}
	added, err := e.client.Add(ctx(), e.abs(assetPath), src)
	if err != nil {
		e.t.Fatal(err)
	}
	e.workspaces[assetPath] = added.String
}

func (e *locksEnv) edit(assetPath string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.workspaces[assetPath], "f.txt"), []byte("edited"), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *locksEnv) must(_ any, err error) {
	e.t.Helper()
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *locksEnv) headCommitID(assetPath string) string {
	e.t.Helper()
	var id string
	withCommitContext(e.t, e.client, e.branch(assetPath), func(_ context.Context, _ *opcontext.OperationContext, _ *db.Client, ref *types.Ref) error {
		id = e.headCommit(ref).CommitID
		return nil
	})
	return id
}

func (e *locksEnv) headCommit(ref *types.Ref) *types.Ref {
	e.t.Helper()
	var commit *types.Ref
	withCommitContext(e.t, e.client, e.branch(ref.AssetPath), func(ctx context.Context, opctx *opcontext.OperationContext, dbClient *db.Client, _ *types.Ref) error {
		asset, err := dbClient.GetJSONAssetFromRef(ctx, opctx, ref, false)
		if err != nil {
			return err
		}
		bc, err := db.GetBranchCommit(asset.Data, ref.BranchName)
		if err != nil {
			return err
		}
		commit = types.NewRefCommit(ref.RepoRef(), ref.AssetPath, bc.Parent)
		return nil
	})
	return commit
}

func lockKeys(l transaction.Locks) []string {
	var keys []string
	for _, r := range l.Created {
		keys = append(keys, "TreeChanged "+r.AssetPath)
	}
	for _, r := range l.Edited {
		keys = append(keys, string(l.EditOp)+" "+r.RelativePath())
	}
	slices.Sort(keys)
	return keys
}
