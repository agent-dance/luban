package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

const defaultWorktreeHookTimeout = 10 * time.Second

type WorktreeHook struct {
	Name    string
	Command string
	Args    []string
	Timeout time.Duration
	Shell   bool
	WorkDir string
}

type WorktreeHookResult struct {
	Path   string
	Branch string
	Output string
}

// WorktreeHookBridge retains the original Lookup/Run surface so configured
// runtimes remain source-compatible. Bridges that return create output also
// implement WorktreeHookResultBridge.
type WorktreeHookBridge interface {
	Lookup(name string) (WorktreeHook, bool)
	Run(ctx context.Context, name string, payload map[string]any) error
}

type WorktreeHookResultBridge interface {
	RunWithResult(ctx context.Context, name string, payload map[string]any) (WorktreeHookResult, error)
}

type InMemoryWorktreeHookBridge struct {
	mu    sync.RWMutex
	hooks map[string]WorktreeHook
}

func NewInMemoryWorktreeHookBridge() *InMemoryWorktreeHookBridge {
	return &InMemoryWorktreeHookBridge{hooks: make(map[string]WorktreeHook)}
}

// NewInMemoryHookBridge is kept for callers of the original constructor.
func NewInMemoryHookBridge() WorktreeHookBridge { return NewInMemoryWorktreeHookBridge() }

func (b *InMemoryWorktreeHookBridge) Register(hook WorktreeHook) {
	if b == nil || strings.TrimSpace(hook.Name) == "" {
		return
	}
	hook.Args = append([]string(nil), hook.Args...)
	b.mu.Lock()
	b.hooks[hook.Name] = hook
	b.mu.Unlock()
}

func (b *InMemoryWorktreeHookBridge) Lookup(name string) (WorktreeHook, bool) {
	if b == nil {
		return WorktreeHook{}, false
	}
	b.mu.RLock()
	hook, ok := b.hooks[name]
	b.mu.RUnlock()
	hook.Args = append([]string(nil), hook.Args...)
	return hook, ok
}

func (b *InMemoryWorktreeHookBridge) Run(ctx context.Context, name string, payload map[string]any) error {
	_, err := b.RunWithResult(ctx, name, payload)
	return err
}

func (b *InMemoryWorktreeHookBridge) RunWithResult(ctx context.Context, name string, payload map[string]any) (WorktreeHookResult, error) {
	hook, ok := b.Lookup(name)
	if !ok {
		return WorktreeHookResult{}, errHookNotRegistered
	}
	if strings.TrimSpace(hook.Command) == "" {
		return WorktreeHookResult{}, i18n.NewError(i18n.KeyToolWorktreeHookCommandEmpty, name)
	}
	timeout := hook.Timeout
	if timeout <= 0 {
		timeout = defaultWorktreeHookTimeout
	}
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	body, err := json.Marshal(payload)
	if err != nil {
		return WorktreeHookResult{}, i18n.WrapError(i18n.KeyToolWorktreeHookEncodePayload, err, name)
	}
	var cmd *exec.Cmd
	if hook.Shell {
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(hookCtx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", hook.Command)
		} else {
			cmd = exec.CommandContext(hookCtx, "bash", "-c", hook.Command)
		}
	} else {
		cmd = exec.CommandContext(hookCtx, hook.Command, hook.Args...)
	}
	if strings.TrimSpace(hook.WorkDir) != "" {
		cmd.Dir = hook.WorkDir
	}
	cmd.Stdin = bytes.NewReader(append(body, '\n'))
	stdout := &cappedBuffer{cap: 1 << 20}
	stderr := &cappedBuffer{cap: 1 << 20}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(hookCtx.Err(), context.DeadlineExceeded) {
			return WorktreeHookResult{}, i18n.NewError(i18n.KeyToolWorktreeHookTimedOut, name, timeout)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		return WorktreeHookResult{}, i18n.NewError(i18n.KeyToolWorktreeHookFailed, name, detail)
	}
	if stdout.dropped || stderr.dropped {
		return WorktreeHookResult{}, i18n.NewError(i18n.KeyToolWorktreeHookOutputLarge, name)
	}
	return parseWorktreeHookOutput(name, stdout.String())
}

// LoadWorktreeHookBridge reads project settings in legacy-to-current order.
// Current settings override the legacy entry for the same event. Worktree
// hooks use the documented command-hook format and execute through the shell,
// matching the general hook runtime's command semantics.
func LoadWorktreeHookBridge(cwd string) (*InMemoryWorktreeHookBridge, error) {
	bridge := NewInMemoryWorktreeHookBridge()
	cwd = cleanWorktreePath(cwd)
	paths := []string{
		filepath.Join(cwd, brand.LegacyConfigDirName, "settings.json"),
		filepath.Join(cwd, brand.LegacyDeepSeekConfigDirName, "settings.json"),
		filepath.Join(cwd, brand.ConfigDirName, "settings.json"),
	}
	for _, settingsPath := range paths {
		data, err := os.ReadFile(settingsPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, i18n.WrapError(i18n.KeyToolWorktreeHookReadSettings, err, settingsPath)
		}
		var settings struct {
			Hooks map[string][]struct {
				Hooks []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
					Timeout int    `json:"timeout"`
				} `json:"hooks"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal(data, &settings); err != nil {
			return nil, i18n.WrapError(i18n.KeyToolWorktreeHookParseSettings, err, settingsPath)
		}
		for _, event := range []string{"WorktreeCreate", "WorktreeRemove"} {
			for _, matcher := range settings.Hooks[event] {
				for _, configured := range matcher.Hooks {
					kind := strings.ToLower(strings.TrimSpace(configured.Type))
					if kind != "" && kind != "command" {
						continue
					}
					if strings.TrimSpace(configured.Command) == "" {
						continue
					}
					timeout := time.Duration(configured.Timeout) * time.Second
					bridge.Register(WorktreeHook{
						Name: event, Command: configured.Command, Timeout: timeout,
						Shell: true, WorkDir: cwd,
					})
				}
			}
		}
	}
	return bridge, nil
}

func parseWorktreeHookOutput(name, output string) (WorktreeHookResult, error) {
	trimmed := strings.TrimSpace(output)
	if name != "WorktreeCreate" {
		return WorktreeHookResult{Output: trimmed}, nil
	}
	if trimmed == "" {
		return WorktreeHookResult{}, i18n.NewError(i18n.KeyToolWorktreeHookNoOutput)
	}
	var wire struct {
		Path           string `json:"path"`
		WorktreePath   string `json:"worktreePath"`
		Branch         string `json:"branch"`
		WorktreeBranch string `json:"worktreeBranch"`
		HookSpecific   *struct {
			WorktreePath string `json:"worktreePath"`
		} `json:"hookSpecificOutput"`
	}
	if json.Unmarshal([]byte(trimmed), &wire) == nil {
		path := firstNonEmptyWorktreeHookValue(wire.Path, wire.WorktreePath)
		if path == "" && wire.HookSpecific != nil {
			path = wire.HookSpecific.WorktreePath
		}
		if path == "" {
			return WorktreeHookResult{}, i18n.NewError(i18n.KeyToolWorktreeHookPathMissing)
		}
		return WorktreeHookResult{
			Path:   strings.TrimSpace(path),
			Branch: strings.TrimSpace(firstNonEmptyWorktreeHookValue(wire.Branch, wire.WorktreeBranch)),
			Output: trimmed,
		}, nil
	}
	// TS command hooks may emit a bare path on stdout.
	if strings.ContainsAny(trimmed, "\r\n") {
		return WorktreeHookResult{}, i18n.NewError(i18n.KeyToolWorktreeHookOutputFormat)
	}
	return WorktreeHookResult{Path: trimmed, Output: trimmed}, nil
}

func firstNonEmptyWorktreeHookValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var errHookNotRegistered = errors.New("worktree: hook not registered")

var (
	globalHookBridgeMu sync.RWMutex
	globalHookBridge   WorktreeHookBridge = NewInMemoryWorktreeHookBridge()
)

func DefaultWorktreeHookBridge() WorktreeHookBridge {
	globalHookBridgeMu.RLock()
	defer globalHookBridgeMu.RUnlock()
	return globalHookBridge
}

func SetWorktreeHookBridge(bridge WorktreeHookBridge) {
	if bridge == nil {
		bridge = NewInMemoryWorktreeHookBridge()
	}
	globalHookBridgeMu.Lock()
	globalHookBridge = bridge
	globalHookBridgeMu.Unlock()
}
