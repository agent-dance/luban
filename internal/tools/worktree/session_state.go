package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	storepaths "github.com/agent-dance/luban/internal/store/paths"
)

// WorktreeRuntime owns the session-local working directory. The context-aware
// switcher lets the registry update every cwd-aware tool in one operation.
// It deliberately never calls os.Chdir: process cwd is shared by all sessions.
type WorktreeRuntime struct {
	mu              sync.RWMutex
	cwd             string
	contextSwitcher func(context.Context, string) error
}

func NewWorktreeRuntime(cwd string) *WorktreeRuntime {
	return &WorktreeRuntime{cwd: cleanWorktreePath(cwd)}
}

func (r *WorktreeRuntime) CurrentCWD() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cwd
}

// SwitchCWDContext lets production wiring authorize a worktree retarget
// against the exact loop-owned tool execution.
func (r *WorktreeRuntime) SwitchCWDContext(ctx context.Context, cwd string) error {
	if r == nil {
		return i18n.NewError(i18n.KeyToolSourceSinkWorktreeRuntimeMissing)
	}
	cwd = cleanWorktreePath(cwd)
	if cwd == "" {
		return i18n.NewError(i18n.KeyToolSourceSinkWorktreeCWDEmpty)
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkWorktreeCWDUnavailable, err, cwd)
	}
	if !info.IsDir() {
		return i18n.NewError(i18n.KeyToolSourceSinkWorktreeCWDNotDir, cwd)
	}

	r.mu.RLock()
	contextSwitcher := r.contextSwitcher
	r.mu.RUnlock()
	if contextSwitcher != nil {
		if err := contextSwitcher(ctx, cwd); err != nil {
			return err
		}
	}
	r.mu.Lock()
	r.cwd = cwd
	r.mu.Unlock()
	return nil
}

// SetContextSwitcher installs the authoritative production transition.
func (r *WorktreeRuntime) SetContextSwitcher(switcher func(context.Context, string) error) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.contextSwitcher = switcher
	r.mu.Unlock()
}

func cleanWorktreePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = filepath.Clean(resolved)
	}
	return path
}

func worktreeStateFilePath(root, sessionID string) string {
	root = cleanWorktreePath(root)
	if root == "" {
		return ""
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	return filepath.Join(storepaths.RuntimeSessionDir(root, sessionID), "worktree-state.json")
}
