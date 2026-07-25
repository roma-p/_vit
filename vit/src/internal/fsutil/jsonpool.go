package fsutil

import (
	"context"
	"fmt"
)

// JSONHandlerPool caches JSON file handlers and manages their locks.
// Handlers are reused across calls within the same operation.
// Use one JSONHandlerPool per Operation!
// Use ReleaseAll (typically via defer) to release all locks when done.
type JSONHandlerPool struct {
	data  map[string]*poolEntry
	order []string
}

type poolEntry struct {
	jsonhandler AnyJSONHandler
	forWrite    bool
	lockDir     string
}

// NewJSONHandlerPool init a new pool
func NewJSONHandlerPool() *JSONHandlerPool {
	return &JSONHandlerPool{
		data: make(map[string]*poolEntry),
	}
}

// ResolveHandler returns a typed JSON handler for the given path, caching and
// reusing entries across calls in the same pool.
//
// Behavior:
//
//   - If the entry is cached at a sufficient lock level (forWrite >= requested),
//     the cached handler is returned.
//
//   - If cached as read but write is requested, the entry is escalated: the
//     lock is acquired, the file is re-read from disk (to prevent stale data)
//
//   - If not cached: acquires lock if forWrite,
//     -- if no initData: reads the file to populate data
//     -- if initData: use them to populate .Data (even if the file exists)
//     -- if file not exist and initData is null: return an error
func ResolveHandler[T any](
	ctx context.Context,
	pool *JSONHandlerPool,
	path string,
	forWrite bool,
	initData *T,
) (*JSONHandler[T], error) {
	if existing, ok := pool.data[path]; ok {
		jsonhandler, ok := existing.jsonhandler.(*JSONHandler[T])
		if !ok {
			return nil, fmt.Errorf("handler for %s has wrong type", path)
		}

		if forWrite && !existing.forWrite {
			lockDir, err := AcquireExclusiveLock(ctx, path)
			if err != nil {
				return nil, err
			}
			fresh, err := jsonRead[T](path)
			if err != nil {
				ReleaseExclusiveLock(lockDir)
				return nil, fmt.Errorf("re-read on escalation failed for %s: %w", path, err)
			}
			jsonhandler.Data = fresh
			existing.forWrite = true
			existing.lockDir = lockDir
		}

		return jsonhandler, nil
	}

	return addHandler(ctx, pool, path, forWrite, initData)
}

// FlushForWrite write all the json handler hold with Write privilege
// in the order they were gathered  first (escalation don't change order)
// ideally, the only way to write db is using this.
func (p *JSONHandlerPool) FlushForWrite() error {
	for _, path := range p.order {
		entry, ok := p.data[path]
		if !ok || !entry.forWrite {
			continue
		}
		if err := entry.jsonhandler.Write(); err != nil {
			return fmt.Errorf("flush write failed for %s: %w", path, err)
		}
	}
	return nil
}

// Release releases a single entry and its lock. No-op if not cached.
func (p *JSONHandlerPool) Release(path string) {
	entry, ok := p.data[path]
	if !ok {
		return
	}
	if entry.lockDir != "" {
		ReleaseExclusiveLock(entry.lockDir)
	}
	delete(p.data, path)
}

// ReleaseAll releases every cached entry. Intended for defer at the
// top of an operation. Safe to call multiple times.
func (p *JSONHandlerPool) ReleaseAll() {
	for _, entry := range p.data {
		if entry.lockDir != "" {
			ReleaseExclusiveLock(entry.lockDir)
		}
	}
	p.data = make(map[string]*poolEntry)
	p.order = nil
}

// ListJSONPath list path to all json handler in the pool (either read or write)
func (p *JSONHandlerPool) ListJSONPath() []string {
	ret := []string{}
	for i := range p.data {
		ret = append(ret, i)
	}
	return ret
}

// -- FILE LOCAL -------------------------------------------------------------

func addHandler[T any](
	ctx context.Context,
	pool *JSONHandlerPool,
	path string,
	forWrite bool,
	initData *T,
) (*JSONHandler[T], error) {
	entry := poolEntry{forWrite: forWrite}

	if forWrite {
		lockDir, err := AcquireExclusiveLock(ctx, path)
		if err != nil {
			return nil, err
		}
		entry.lockDir = lockDir
	}

	handler, err := NewJSONHandlerFromFile[T](path)
	if err != nil {
		if initData == nil {
			if entry.lockDir != "" {
				ReleaseExclusiveLock(entry.lockDir)
			}
			return nil, err
		}
		handler = NewJSONHandlerFromPath(path, initData)
	}

	entry.jsonhandler = handler
	pool.data[path] = &entry
	pool.order = append(pool.order, path)
	return handler, nil
}
