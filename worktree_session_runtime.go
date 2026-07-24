package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
)

// configureWorktreeSessionRuntime binds EnterWorktree/ExitWorktree to the
// active engine conversation and registry publication barrier. Worktree
// changes keep the durable SessionProjectDir untouched; only the workspace
// runtime and the single shared skill Manager's project authority are moved.
func configureWorktreeSessionRuntime(
	deps *RegistryDeps,
	eng *engine.CoreEngine,
	cwd *string,
	hookRunnerRef **hooks.Runner,
	systemPromptOverride string,
	extraAllowedDirs []string,
) {
	if deps == nil || deps.WorktreeRuntime == nil || eng == nil || cwd == nil {
		return
	}
	deps.WorktreeRuntime.SetContextSwitcher(func(ctx context.Context, targetCWD string) error {
		targetCWD = strings.TrimSpace(targetCWD)
		if targetCWD == "" {
			return rootRuntimeError(i18n.KeyRootWorkspaceRequired)
		}
		targetHooks, err := loadSessionHooks(targetCWD)
		if err != nil {
			return err
		}
		targetAllowed := allowedDirsForSession(targetCWD, extraAllowedDirs)
		targetSystem := buildSystemPromptForCWD(systemPromptOverride, deps.Registry, targetCWD)
		targetRuntime := engine.RuntimeContext{
			SystemPrompt: targetSystem, HookRunner: targetHooks,
			AllowedDirs: targetAllowed, ProjectRoot: targetCWD, CWD: targetCWD,
		}

		deps.runtimePublishMu.Lock()
		defer deps.runtimePublishMu.Unlock()
		// Re-run every fallible registry check inside the publication barrier.
		// No Manager/tool consumer can capture a half-old workspace snapshot.
		prepared, err := deps.PrepareSessionContext(targetCWD)
		if err != nil {
			return err
		}
		if prepared == nil || !sameRuntimePath(prepared.cwd, targetCWD) {
			return rootRuntimeError(i18n.KeyRootSessionPreparedMismatch, targetCWD)
		}
		sessionID := deps.currentSessionID()
		if err := deps.commitPreparedSessionRuntimeLocked(
			sessionID, targetCWD, targetAllowed, targetSystem, targetHooks, prepared,
			func() error {
				if err := eng.RebindWorkspaceRuntime(ctx, sessionID, targetRuntime); err != nil {
					return rootRuntimeErrorWithCause(i18n.KeyRootWorktreeRebindRejected, err)
				}
				return nil
			},
		); err != nil {
			return err
		}
		*cwd = targetCWD
		if hookRunnerRef != nil {
			*hookRunnerRef = targetHooks
		}
		return nil
	})
}

func sameRuntimePath(left, right string) bool {
	canonical := func(value string) string {
		value = strings.TrimSpace(value)
		if value == "" {
			return ""
		}
		if abs, err := filepath.Abs(value); err == nil {
			value = abs
		}
		value = filepath.Clean(value)
		if resolved, err := filepath.EvalSymlinks(value); err == nil {
			value = filepath.Clean(resolved)
		}
		return value
	}
	return canonical(left) != "" && canonical(left) == canonical(right)
}
