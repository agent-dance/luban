// Package tools — file_read_memory_mtime_cache.go implements an
// in-process cache of (path → mtime) pairs for CLAUDE.md / AGENTS.md and
// other memory files. Mirrors TS memoryFileMtimes WeakMap which avoids
// re-stat'ing the same memory file repeatedly during a session.
//
// The cache is conservative: it remembers an mtime keyed by absolute
// path, and exposes a Stat-replacement helper that returns the cached
// value when present. Callers should consult Stat() — it falls through
// to os.Stat on miss and self-populates.
package tools

import (
	"os"
	"sync"
	"time"
)

// memoryFileMtimeCache is a process-wide cache of memory-file mtimes.
type memoryFileMtimeCache struct {
	mu      sync.RWMutex
	entries map[string]time.Time
}

var defaultMemoryFileMtimeCache = &memoryFileMtimeCache{
	entries: make(map[string]time.Time),
}

// MemoryFileMtimeCacheStat returns the mtime for path, hitting the
// in-process cache when available. Falls through to os.Stat on miss
// (which also seeds the cache). Returns (zero, false) on error.
func MemoryFileMtimeCacheStat(path string) (time.Time, bool) {
	if path == "" {
		return time.Time{}, false
	}
	if !isClaudeMemoryWritePath(path) {
		// Don't cache arbitrary files — cache footprint must stay tiny.
		fi, err := os.Stat(path)
		if err != nil {
			return time.Time{}, false
		}
		return fi.ModTime(), true
	}
	defaultMemoryFileMtimeCache.mu.RLock()
	t, ok := defaultMemoryFileMtimeCache.entries[path]
	defaultMemoryFileMtimeCache.mu.RUnlock()
	if ok {
		return t, true
	}
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	defaultMemoryFileMtimeCache.mu.Lock()
	defaultMemoryFileMtimeCache.entries[path] = fi.ModTime()
	defaultMemoryFileMtimeCache.mu.Unlock()
	return fi.ModTime(), true
}

// InvalidateMemoryFileMtime clears the cached entry for path. Callers
// that mutate a memory file (e.g. Write) should invoke this so a
// subsequent read picks up the new mtime.
func InvalidateMemoryFileMtime(path string) {
	if path == "" {
		return
	}
	defaultMemoryFileMtimeCache.mu.Lock()
	delete(defaultMemoryFileMtimeCache.entries, path)
	defaultMemoryFileMtimeCache.mu.Unlock()
}

// ResetMemoryFileMtimeCacheForTest clears all cached entries. Test-only.
func ResetMemoryFileMtimeCacheForTest() {
	defaultMemoryFileMtimeCache.mu.Lock()
	defaultMemoryFileMtimeCache.entries = make(map[string]time.Time)
	defaultMemoryFileMtimeCache.mu.Unlock()
}
