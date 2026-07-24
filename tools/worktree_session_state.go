package tools

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

// WorktreeRuntime owns the session-local working directory. The optional
// switcher lets the registry update every cwd-aware tool in one operation.
// It deliberately never calls os.Chdir: process cwd is shared by all sessions.
type WorktreeRuntime struct {
	mu              sync.RWMutex
	cwd             string
	switcher        func(string) error
	contextSwitcher func(context.Context, string) error
}

func NewWorktreeRuntime(cwd string, switcher func(string) error) *WorktreeRuntime {
	return &WorktreeRuntime{cwd: cleanWorktreePath(cwd), switcher: switcher}
}

func (r *WorktreeRuntime) CurrentCWD() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cwd
}

func (r *WorktreeRuntime) SwitchCWD(cwd string) error {
	return r.SwitchCWDContext(context.Background(), cwd)
}

// SwitchCWDContext lets production wiring authorize a worktree retarget
// against the exact loop-owned tool execution. Legacy embedders continue to
// use SetSwitcher/SwitchCWD without carrying a context.
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
	switcher := r.switcher
	contextSwitcher := r.contextSwitcher
	r.mu.RUnlock()
	if contextSwitcher != nil {
		if err := contextSwitcher(ctx, cwd); err != nil {
			return err
		}
	} else if switcher != nil {
		if err := switcher(cwd); err != nil {
			return err
		}
	}
	r.mu.Lock()
	r.cwd = cwd
	r.mu.Unlock()
	return nil
}

func (r *WorktreeRuntime) SetSwitcher(switcher func(string) error) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.switcher = switcher
	r.mu.Unlock()
}

// SetContextSwitcher installs the authoritative production transition. When
// set it takes precedence over the compatibility switcher.
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
		sessionID = "default"
	}
	sum := sha256.Sum256([]byte(sessionID))
	return filepath.Join(root, brand.ConfigDirName, "worktree-sessions", fmt.Sprintf("%x.json", sum[:12]))
}
