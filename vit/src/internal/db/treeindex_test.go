package db

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"
	"sync"
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

func TestTreeIndexKeepsAnInvalidationPostedDuringItsRebuild(t *testing.T) {
	repo := t.TempDir()
	const n = 200
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for i := range n {
			id := fmt.Sprintf("u%03dabcdef", i)
			data, _ := json.Marshal(types.TreeNode{ID: id, Name: id, Type: types.TreeNodeTypeAsset})
			path := treeNodeJSONPath(repo, id)
			if err := os.MkdirAll(fsutil.DirOS(path), 0o755); err != nil {
				t.Error(err)
				return
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Error(err)
				return
			}
			if err := markTreeStale(repo); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	readIndex := func() *TreeIndex {
		opctx := &opcontext.OperationContext{JSONPool: fsutil.NewJSONHandlerPool()}
		defer opctx.Close()
		idx, err := getTreeIndex(context.Background(), opctx, repo)
		if err != nil {
			t.Error(err)
		}
		return idx
	}
	var readers sync.WaitGroup
	for range 3 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-writerDone:
					return
				default:
					readIndex()
				}
			}
		}()
	}
	readers.Wait()

	if idx := readIndex(); idx == nil || len(idx.IDToPath) != n {
		t.Fatalf("index has %d of %d nodes", len(idx.IDToPath), n)
	}
}

func TestTreeIndexRebuildsOnTheLegacyStaleFile(t *testing.T) {
	repo := t.TempDir()
	writeNode(t, repo, "u1abcdef", types.TreeNodeTypeAsset, nil)
	opctx := &opcontext.OperationContext{JSONPool: fsutil.NewJSONHandlerPool()}
	t.Cleanup(opctx.Close)
	if _, err := getTreeIndex(context.Background(), opctx, repo); err != nil {
		t.Fatal(err)
	}
	writeNode(t, repo, "u2abcdef", types.TreeNodeTypeAsset, nil)
	if err := os.WriteFile(pathLegacyTreeStale(repo), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := getTreeIndex(context.Background(), opctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.IDToPath["u2abcdef"]; !ok {
		t.Fatalf("index = %v", idx.IDToPath)
	}
	if _, err := os.Stat(pathLegacyTreeStale(repo)); !os.IsNotExist(err) {
		t.Fatalf("legacy stale file left: %v", err)
	}
}
