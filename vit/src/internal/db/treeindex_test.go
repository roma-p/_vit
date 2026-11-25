package db

import (
	"context"
	"maps"
	"os"
	"strings"
	"testing"

	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
)

func TestTreeIndexRoundTripsAsSortedRows(t *testing.T) {
	idx := NewTreeIndex()
	idx.AddPath("s20/sh010", "u2", types.TreeNodeTypeAsset)
	idx.AddPath("s20", "u1", types.TreeNodeTypeDir)
	asset := idx.IDToPath["u2"]
	asset.AssetUID = "a1"
	idx.IDToPath["u2"] = asset

	repo := t.TempDir()
	if err := writeTreeIndex(repo, idx); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(pathTreeIndex(repo))
	if err != nil {
		t.Fatal(err)
	}
	if want := `[["u1","dir","s20",""],["u2","asset","s20/sh010","a1"]]` + "\n"; string(data) != want {
		t.Fatalf("encoded = %s", data)
	}
	got, err := readTreeIndex(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got.IDToPath, idx.IDToPath) || got.PathToID["s20/sh010"] != (TreeIndexEntry{Data: "u2", Type: types.TreeNodeTypeAsset, AssetUID: "a1"}) {
		t.Fatalf("decoded = %+v", got)
	}
}

func TestTreeIndexInTheOldFormatIsRebuilt(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u1abcdef", types.TreeNodeTypeAsset, nil)
	for path, content := range map[string]string{pathTreeIndex(repo): `{"id_to_path":{},"path_to_id":{}}`, pathAssetList(repo): `{}`} {
		if err := os.MkdirAll(fsutil.DirOS(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opctx := &opcontext.OperationContext{JSONPool: fsutil.NewJSONHandlerPool()}
	t.Cleanup(opctx.Close)

	idx, err := getTreeIndex(context.Background(), opctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if idx.IDToPath["u1abcdef"].Data != "u1abcdef" {
		t.Fatalf("index = %+v", idx.IDToPath)
	}
	if data, _ := os.ReadFile(pathTreeIndex(repo)); !strings.HasPrefix(string(data), "[[") {
		t.Fatalf("rewritten as %s", data)
	}
}

func TestTreeScanSurvivesLocksComingAndGoing(t *testing.T) {
	repo := t.TempDir()
	for _, id := range []string{"u1abcdef", "u2abcdef", "u3abcdef"} {
		writeNode(t, repo, id, types.TreeNodeTypeAsset, nil)
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		lock := fsutil.PathOS(repo, ".vit", "tree", "u1", ".abcdef.json.lock")
		for {
			select {
			case <-stop:
				return
			default:
				_ = os.Mkdir(lock, 0o755)
				_ = os.WriteFile(fsutil.PathOS(lock, "owner"), nil, 0o644)
				_ = os.RemoveAll(lock)
			}
		}
	}()
	defer func() { close(stop); <-done }()

	for range 2000 {
		paths, err := scanAllTreeNodeFiles(repo)
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != 3 {
			t.Fatalf("paths = %v", paths)
		}
	}
}
