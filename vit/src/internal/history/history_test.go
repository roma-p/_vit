package history

import (
	"context"
	"maps"
	"os"
	"slices"
	"testing"

	"vit/internal/fsutil"
	"vit/internal/types"
)

func TestReadSinceReturnsOnlyNewEntries(t *testing.T) {
	repo := t.TempDir()
	record(t, repo, "", "a", "b")

	got, c := readSince(t, repo, nil)
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("first read = %v", got)
	}
	if got, _ := readSince(t, repo, c); len(got) != 0 {
		t.Fatalf("nothing new, got %v", got)
	}
	record(t, repo, "", "c")
	if got, _ := readSince(t, repo, c); !slices.Equal(got, []string{"c"}) {
		t.Fatalf("after c = %v", got)
	}
}

func TestReadSinceFollowsEverySite(t *testing.T) {
	repo := t.TempDir()
	record(t, repo, "paris", "p1")
	_, c := readSince(t, repo, nil)
	record(t, repo, "paris", "p2")
	record(t, repo, "tokyo", "t1")

	got, _ := readSince(t, repo, c)
	slices.Sort(got)
	if !slices.Equal(got, []string{"p2", "t1"}) {
		t.Fatalf("got %v", got)
	}
}

func TestReadSinceCrossesSegments(t *testing.T) {
	repo := t.TempDir()
	record(t, repo, "", "a")
	_, c := readSince(t, repo, nil)
	dir := streamDir(repo, "")
	if err := os.WriteFile(fsutil.PathOS(dir, nextSegmentName("seg-000001.jsonl")), []byte(`{"uid":"b"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record(t, repo, "", "c")

	got, c := readSince(t, repo, c)
	if !slices.Equal(got, []string{"b", "c"}) {
		t.Fatalf("got %v", got)
	}
	record(t, repo, "", "d")
	if got, _ := readSince(t, repo, c); !slices.Equal(got, []string{"d"}) {
		t.Fatalf("an older segment was reread: %v", got)
	}
}

func TestReadSinceSkipsATornLine(t *testing.T) {
	repo := t.TempDir()
	record(t, repo, "", "a")
	seg := fsutil.PathOS(streamDir(repo, ""), "seg-000001.jsonl")
	f, err := os.OpenFile(seg, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"uid":"b"`)
	_ = f.Close()

	got, c := readSince(t, repo, nil)
	if !slices.Equal(got, []string{"a"}) || c["local"].Line != 1 {
		t.Fatalf("got %v, cursor %v", got, c)
	}
}

func TestHeadIsTheEndOfEveryStream(t *testing.T) {
	repo := t.TempDir()
	if c, err := Head(repo); err != nil || len(c) != 0 {
		t.Fatalf("empty repo: %v %v", c, err)
	}
	record(t, repo, "paris", "p1", "p2")
	record(t, repo, "", "a")

	head, err := Head(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := readSince(t, repo, head); len(got) != 0 {
		t.Fatalf("entries past the head: %v", got)
	}
	_, end := readSince(t, repo, nil)
	if !maps.Equal(head, end) {
		t.Fatalf("head %v, end of read %v", head, end)
	}
}

// -- FILE LOCAL -------------------------------------------------------------

func record(t *testing.T, repo, site string, uids ...string) {
	t.Helper()
	for _, uid := range uids {
		if err := Record(context.Background(), repo, site, types.HistoryEntry{UID: uid}); err != nil {
			t.Fatal(err)
		}
	}
}

func readSince(t *testing.T, repo string, c Cursor) ([]string, Cursor) {
	t.Helper()
	es, next, err := ReadSince(repo, c)
	if err != nil {
		t.Fatal(err)
	}
	var uids []string
	for _, e := range es {
		uids = append(uids, e.UID)
	}
	return uids, next
}

func TestDropAbortedHidesTheEntryAndItsAbort(t *testing.T) {
	es := []types.HistoryEntry{{UID: "a"}, {UID: "b"}, {UID: "c", Aborted: "a"}}

	got := DropAborted(es)
	if len(got) != 1 || got[0].UID != "b" {
		t.Fatalf("got %+v", got)
	}
}
