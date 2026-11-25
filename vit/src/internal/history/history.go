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
	"slices"
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
	if fsutil.IsNotExist(err) {
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

// Cursor is a cross site position on a history.
// <site> : cursor position on the history on that site.
// (segment id, line number on that segment)
type Cursor map[string]Position

type Position struct {
	Segment string `json:"segment"`
	Line    int    `json:"line"`
}

// Located is an entry with its site and position: At is the cursor that would read it next.
type Located struct {
	Entry types.HistoryEntry
	Site  string
	At    Position
}

func ReadSince(repoPath string, c Cursor) ([]types.HistoryEntry, Cursor, error) {
	located, next, err := ReadSinceAt(repoPath, c)
	if err != nil {
		return nil, nil, err
	}
	out := make([]types.HistoryEntry, 0, len(located))
	for _, l := range located {
		out = append(out, l.Entry)
	}
	return out, next, nil
}

func ReadSinceAt(repoPath string, c Cursor) ([]Located, Cursor, error) {
	var out []Located
	next, err := walkStreams(repoPath, c, func(site string, segs []string, siteDir string) (Position, error) {
		pos := c[site]
		for _, seg := range segs {
			if seg < pos.Segment {
				continue
			}
			lines, err := completeLines(fsutil.PathOS(siteDir, seg))
			if err != nil {
				return Position{}, err
			}
			start := 0
			if seg == pos.Segment {
				start = min(pos.Line, len(lines))
			}
			for i := start; i < len(lines); i++ {
				if e, ok := parseLine(lines[i]); ok {
					out = append(out, Located{Entry: e, Site: site, At: Position{Segment: seg, Line: i}})
				}
			}
			pos = Position{Segment: seg, Line: len(lines)}
		}
		return pos, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return out, next, nil
}

func Head(repoPath string) (Cursor, error) {
	return walkStreams(repoPath, nil, func(_ string, segs []string, siteDir string) (Position, error) {
		last := segs[len(segs)-1]
		lines, err := completeLines(fsutil.PathOS(siteDir, last))
		return Position{Segment: last, Line: len(lines)}, err
	})
}

// -- FILE LOCAL -------------------------------------------------------------

func walkStreams(
	repoPath string,
	c Cursor,
	visit func(site string, segs []string, siteDir string) (Position, error),
) (Cursor, error) {
	base := fsutil.PathOS(repoPath, ".vit", dirName)
	sites, err := os.ReadDir(base)
	if err != nil && !fsutil.IsNotExist(err) {
		return nil, err
	}
	next := Cursor{}
	for site, pos := range c {
		next[site] = pos
	}
	for _, s := range sites {
		if !s.IsDir() {
			continue
		}
		siteDir := fsutil.PathOS(base, s.Name())
		segs, err := listSegments(siteDir)
		if err != nil {
			return nil, err
		}
		if len(segs) == 0 {
			continue
		}
		pos, err := visit(s.Name(), segs, siteDir)
		if err != nil {
			return nil, err
		}
		next[s.Name()] = pos
	}
	return next, nil
}

func completeLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	all := strings.Split(string(data), "\n")
	var out []string
	for _, line := range all[:len(all)-1] {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

func parseLines(lines []string) []types.HistoryEntry {
	var out []types.HistoryEntry
	for _, line := range lines {
		if e, ok := parseLine(line); ok {
			out = append(out, e)
		}
	}
	return out
}

func parseLine(line string) (types.HistoryEntry, bool) {
	var e types.HistoryEntry
	return e, json.Unmarshal([]byte(line), &e) == nil
}

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

func DropAborted(es []types.HistoryEntry) []types.HistoryEntry {
	aborted := map[string]bool{}
	for _, e := range es {
		if e.Aborted != "" {
			aborted[e.Aborted] = true
		}
	}
	return slices.DeleteFunc(es, func(e types.HistoryEntry) bool {
		return e.Aborted != "" || aborted[e.UID]
	})
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
	if fsutil.IsNotExist(err) {
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
	if fsutil.IsNotExist(err) {
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
	return parseLines(strings.Split(string(data), "\n")), nil
}
