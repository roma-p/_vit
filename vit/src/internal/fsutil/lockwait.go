package fsutil

import (
	"slices"
	"strings"
	"sync"
	"time"
)

// LockWait sums the time this process waited to acquire the locks of one kind, and held them.
type LockWait struct {
	Kind     string        `json:"kind"`
	Count    int           `json:"count"`
	Attempts int           `json:"attempts"`
	Total    time.Duration `json:"total_ns"`
	Max      time.Duration `json:"max_ns"`
	Held     time.Duration `json:"held_ns"`
	HeldMax  time.Duration `json:"held_max_ns"`
}

type heldLock struct {
	kind  string
	since time.Time
}

var lockWaits = struct {
	sync.Mutex
	byKind map[string]*LockWait
	held   map[string]heldLock
}{byKind: map[string]*LockWait{}, held: map[string]heldLock{}}

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

func recordLockWait(lockPath, lockDir string, start time.Time, attempts int) {
	kind := lockKind(lockPath)
	now := time.Now()
	lockWaits.Lock()
	defer lockWaits.Unlock()
	w := lockWaitOf(kind)
	w.Count++
	w.Attempts += attempts
	w.Total += now.Sub(start)
	w.Max = max(w.Max, now.Sub(start))
	if lockDir != "" {
		lockWaits.held[lockDir] = heldLock{kind: kind, since: now}
	}
}

func recordLockRelease(lockDir string) {
	lockWaits.Lock()
	defer lockWaits.Unlock()
	h, ok := lockWaits.held[lockDir]
	if !ok {
		return
	}
	delete(lockWaits.held, lockDir)
	w := lockWaitOf(h.kind)
	w.Held += time.Since(h.since)
	w.HeldMax = max(w.HeldMax, time.Since(h.since))
}

func lockWaitOf(kind string) *LockWait {
	w := lockWaits.byKind[kind]
	if w == nil {
		w = &LockWait{Kind: kind}
		lockWaits.byKind[kind] = w
	}
	return w
}

func lockKind(lockPath string) string {
	kind := BaseOS(lockPath)
	switch {
	case strings.HasSuffix(kind, ".json"):
		return "*.json"
	case !strings.HasPrefix(kind, ".") && !strings.HasSuffix(kind, ".lock"):
		return "other"
	}
	return kind
}
