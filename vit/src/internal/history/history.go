// Package history records an append-only activity log of every mutating operation
// on a repo
//
// Current layout: one shared append stream per site,
//
//	.vit/history/<siteKey>/seg-NNNNNN.jsonl
package history

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

const (
	maxEntriesPerSegment = 1000
	dirName              = "history"
	lockName             = ".lock"
	segmentPrefix        = "seg-"
	segmentSuffix        = ".jsonl"
	segmentFormat        = "seg-%06d.jsonl"
)

func Record(ctx context.Context, repoPath, siteUID string, e types.HistoryEntry) error {
	if e.UID == "" {
		e.UID = fsutil.GenerateUID(8)
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	dir := streamDir(repoPath, siteUID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	lockPath := fsutil.PathOS(repoPath, ".vit", dirName, lockName)
	return fsutil.WithExclusiveLock(ctx, lockPath, func() error {
		return appendEntry(dir, e)
	})
}

func appendEntry(dir string, e types.HistoryEntry) error {
	seg, count, err := findActiveSegment(dir)
	if err != nil {
		return err
	}
	if seg == "" || count >= maxEntriesPerSegment {
		seg = fsutil.PathOS(dir, nextSegmentName(seg))
	}

	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return fsutil.AppendLine(seg, line)
}

func ReadAll(repoPath, siteUID string) ([]types.HistoryEntry, error) {
	all, err := readSiteDir(fsutil.PathOS(repoPath, ".vit", dirName, siteKey(siteUID)))
	if err != nil {
		return nil, err
	}
	sortDesc(all)
	return all, nil
}

func ReadAllSites(repoPath string) ([]types.HistoryEntry, error) {
	base := fsutil.PathOS(repoPath, ".vit", dirName)
	sites, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []types.HistoryEntry
	for _, s := range sites {
		if !s.IsDir() {
			continue
		}
		es, err := readSiteDir(fsutil.PathOS(base, s.Name()))
		if err != nil {
			return nil, err
		}
		all = append(all, es...)
	}
	sortDesc(all)
	return all, nil
}

// -- FILE LOCAL -------------------------------------------------------------

func streamDir(repoPath, siteUID string) string {
	return fsutil.PathOS(repoPath, ".vit", dirName, siteKey(siteUID))
}

func siteKey(siteUID string) string {
	if siteUID == "" {
		return "local"
	}
	return url.QueryEscape(siteUID)
}

func readSiteDir(siteDir string) ([]types.HistoryEntry, error) {
	segs, err := listSegments(siteDir)
	if err != nil {
		return nil, err
	}
	var all []types.HistoryEntry
	for _, s := range segs {
		es, err := readSegment(fsutil.PathOS(siteDir, s))
		if err != nil {
			return nil, err
		}
		all = append(all, es...)
	}
	return all, nil
}

func sortDesc(es []types.HistoryEntry) {
	sort.Slice(es, func(i, j int) bool { return es[i].Timestamp.After(es[j].Timestamp) })
}

func findActiveSegment(dir string) (string, int, error) {
	segs, err := listSegments(dir)
	if err != nil || len(segs) == 0 {
		return "", 0, err
	}
	active := fsutil.PathOS(dir, segs[len(segs)-1])
	count, err := countLines(active)
	if err != nil {
		return "", 0, err
	}
	return active, count, nil
}

func listSegments(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var segs []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, segmentPrefix) && strings.HasSuffix(n, segmentSuffix) {
			segs = append(segs, n)
		}
	}
	sort.Strings(segs)
	return segs, nil
}

func nextSegmentName(currentPath string) string {
	n := 0
	if currentPath != "" {
		base := fsutil.BaseOS(currentPath)
		base = strings.TrimPrefix(base, segmentPrefix)
		base = strings.TrimSuffix(base, segmentSuffix)
		fmt.Sscanf(base, "%d", &n)
	}
	return fmt.Sprintf(segmentFormat, n+1)
}

func countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count, nil
}

func readSegment(path string) ([]types.HistoryEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []types.HistoryEntry
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e types.HistoryEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}
