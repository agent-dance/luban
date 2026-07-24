package tools

import (
	"sync"
	"time"
)

// worktree_caches.go — WT-07 cache invalidation registry.
//
// On ExitWorktree the session's cwd switches back to OriginalDir, but
// any caches built while inside the worktree may still point at paths
// inside the now-deleted directory:
//
//   * memory-file cache (system-prompt CLAUDE.md lookups)
//   * plans directory cache
//   * hooks config snapshot
//   * system-prompt section cache
//
// To avoid coupling the worktree tool directly to each subsystem, every
// cache registers an invalidator function with this registry. ExitWorktree
// invokes them all on cleanup.

var (
	worktreeCacheMu           sync.Mutex
	worktreeCacheInvalidators []func()
)

func init() {
	// The prompt memory loader's mtime cache is process-wide and keyed by path.
	// Clearing it here is the concrete Go equivalent of TS
	// clearMemoryFileCaches(); plan/runtime/settings roots are retargeted by the
	// WorktreeRuntime switcher before these invalidators run.
	RegisterWorktreeCacheInvalidator(func() {
		defaultMemoryFileMtimeCache.mu.Lock()
		defaultMemoryFileMtimeCache.entries = make(map[string]time.Time)
		defaultMemoryFileMtimeCache.mu.Unlock()
	})
}

// RegisterWorktreeCacheInvalidator adds a callback that will be invoked
// every time ExitWorktree completes. Safe to call from any subsystem.
func RegisterWorktreeCacheInvalidator(fn func()) {
	if fn == nil {
		return
	}
	worktreeCacheMu.Lock()
	worktreeCacheInvalidators = append(worktreeCacheInvalidators, fn)
	worktreeCacheMu.Unlock()
}

// InvalidateWorktreeCaches runs every registered invalidator in
// registration order. Errors / panics in one invalidator do not stop the
// others (they are recovered).
func InvalidateWorktreeCaches() {
	worktreeCacheMu.Lock()
	snapshot := make([]func(), len(worktreeCacheInvalidators))
	copy(snapshot, worktreeCacheInvalidators)
	worktreeCacheMu.Unlock()
	for _, fn := range snapshot {
		runInvalidator(fn)
	}
}

func runInvalidator(fn func()) {
	defer func() {
		_ = recover()
	}()
	fn()
}
