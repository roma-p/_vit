package fsutil

import (
	"slices"
	"strings"
	"sync"
	"time"
)

// LockWait sums the time this process waited to acquire the locks of one kind.
type LockWait struct {
	Kind     string        `json:"kind"`
	Count    int           `json:"count"`
	Attempts int           `json:"attempts"`
	Total    time.Duration `json:"total_ns"`
	Max      time.Duration `json:"max_ns"`
}

var lockWaits = struct {
	sync.Mutex
	byKind map[string]*LockWait
}{byKind: map[string]*LockWait{}}

func LockWaits() []LockWait {
	lockWaits.Lock()
	defer lockWaits.Unlock()
	out := make([]LockWait, 0, len(lockWaits.byKind))
	for _, w := range lockWaits.byKind {
		out = append(out, *w)
	}
	slices.SortFunc(out, func(a, b LockWait) int { return strings.Compare(a.Kind, b.Kind) })
	return out
}

func recordLockWait(lockPath string, start time.Time, attempts int) {
	kind := BaseOS(lockPath)
	switch {
	case strings.HasSuffix(kind, ".json"):
		kind = "*.json"
	case !strings.HasPrefix(kind, ".") && !strings.HasSuffix(kind, ".lock"):
		kind = "other"
	}
	wait := time.Since(start)
	lockWaits.Lock()
	defer lockWaits.Unlock()
	w := lockWaits.byKind[kind]
	if w == nil {
		w = &LockWait{Kind: kind}
		lockWaits.byKind[kind] = w
	}
	w.Count++
	w.Attempts += attempts
	w.Total += wait
	w.Max = max(w.Max, wait)
}
