package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/registry"
)

type sessionSwitcher struct {
	mu                   sync.Mutex
	repo                 *session.Repository
	deps                 *RegistryDeps
	eng                  engine.Engine
	sessionID            *string
	sessionProjectDir    *string
	cwd                  *string
	hookRunnerRef        **hooks.Runner
	systemPromptOverride string
	extraAllowedDirs     []string
}

func allowedDirsForSession(cwd string, extra []string) []string {
	// No explicit allow-list means the filesystem is unrestricted. Supplying
	// --allowed-dir opts into a scoped workspace-plus-extra allow-list.
	if len(extra) == 0 {
		return nil
	}
	return append([]string{cwd}, extra...)
}

type workspacePrompt struct {
	system       string
	systemBlocks prompt.SystemPrompt
	userContext  prompt.UserContext
	toolSnapshot registry.VisibleToolSnapshot
	promptConfig prompt.Config
	generated    bool
	catalogErr   error
}

func buildWorkspacePrompt(override string, reg *registry.Registry, cwd string) workspacePrompt {
	instructions := prompt.DiscoverInstructions(cwd)
	userContext := prompt.UserContextBuilder{}.
		FromConfig(prompt.Config{CustomInstructions: instructions}).
		Build()

	promptConfig := prompt.Config{CWD: cwd}
	var blocks prompt.SystemPrompt
	var toolSnapshot registry.VisibleToolSnapshot
	var catalogErr error
	generated := false
	if reg != nil && reg.ModelToolProfile() == registry.ModelToolProfileAgenticV2 {
		toolSnapshot, catalogErr = reg.SnapshotVisibleTools(nil)
	}
	if text := strings.TrimSpace(override); text != "" {
		blocks = prompt.SystemPrompt{{Text: text, Source: "override", Name: "override"}}
	} else if toolSnapshot.Valid() {
		if catalogErr == nil {
			blocks = prompt.BuildSystemPromptBlocksForDefinitions(toolSnapshot.Definitions(), promptConfig)
			generated = true
		}
	} else {
		blocks = prompt.BuildSystemPromptBlocks(reg.All(), promptConfig)
	}
	return workspacePrompt{
		system:       blocks.JoinedText(),
		systemBlocks: blocks,
		userContext:  userContext,
		toolSnapshot: toolSnapshot,
		promptConfig: promptConfig,
		generated:    generated,
		catalogErr:   catalogErr,
	}
}

func commitPreparedRuntimeResume(ctx context.Context, prepared engine.PreparedRuntimeContextResume) error {
	if contextual, ok := prepared.(engine.ContextualPreparedRuntimeContextResume); ok {
		return contextual.CommitContext(ctx)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return prepared.Commit()
}

func loadSessionHooks(cwd string) (*hooks.Runner, error) {
	type hookSource struct {
		settings string
		dir      string
	}
	sources := []hookSource{
		{
			settings: filepath.Join(cwd, brand.ConfigDirName, "settings.json"),
			dir:      filepath.Join(cwd, brand.ConfigDirName, "hooks"),
		},
	}
	merged := hooks.NewRunner(nil)
	for _, source := range sources {
		settingsRunner, err := hooks.LoadFromSettings(source.settings)
		if err != nil {
			return nil, rootRuntimeWrap(i18n.KeyRootSessionHookSettingsLoad, err, source.settings)
		}
		dirRunner, err := hooks.LoadFromDir(source.dir)
		if err != nil {
			return nil, rootRuntimeWrap(i18n.KeyRootSessionHookDirectoryLoad, err, source.dir)
		}
		merged = merged.Merge(settingsRunner).Merge(dirRunner)
	}
	return merged, nil
}

func (s *sessionSwitcher) switchTo(ctx context.Context, entry commands.SessionListEntry) error {
	if s == nil || s.repo == nil || s.deps == nil || s.eng == nil || s.sessionID == nil || s.sessionProjectDir == nil || s.cwd == nil {
		return rootRuntimeError(i18n.KeyRootSessionSwitcherIncomplete)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(entry.ID) == "" {
		return rootRuntimeError(i18n.KeyRootSessionTargetIDRequired)
	}
	s.deps.BindSessionIdentity(*s.sessionID)

	targetProjectDir := strings.TrimSpace(entry.ProjectDir)
	if targetProjectDir == "" {
		targetProjectDir = s.repo.ProjectDirForCWD(entry.CWD)
	}
	targetCWD := strings.TrimSpace(entry.CWD)
	if targetCWD == "" {
		if meta, _, err := s.repo.GetMeta(entry.ID, targetProjectDir); err == nil && strings.TrimSpace(meta.CWD) != "" {
			targetCWD = strings.TrimSpace(meta.CWD)
		}
	}
	if targetCWD == "" {
		targetCWD = *s.cwd
	}

	info, err := os.Stat(targetCWD)
	if err != nil {
		return rootRuntimeWrap(i18n.KeyRootSessionCWDUnavailable, err)
	}
	if !info.IsDir() {
		return rootRuntimeError(i18n.KeyRootSessionCWDNotDirectory, targetCWD)
	}

	targetHooks, err := loadSessionHooks(targetCWD)
	if err != nil {
		return err
	}
	preparedRegistry, err := s.deps.PrepareSessionContext(targetCWD)
	if err != nil {
		return err
	}
	if preparedRegistry == nil || filepath.Clean(preparedRegistry.cwd) != filepath.Clean(targetCWD) {
		return rootRuntimeError(i18n.KeyRootSessionPreparedMismatch, targetCWD)
	}
	targetAllowedDirs := allowedDirsForSession(targetCWD, s.extraAllowedDirs)
	targetPrompt := buildWorkspacePrompt(s.systemPromptOverride, s.deps.Registry, targetCWD)
	if targetPrompt.catalogErr != nil {
		return i18n.WrapInternalError(i18n.KeyRootVisibleToolCatalogInvalid, targetPrompt.catalogErr)
	}
	targetRuntime := engine.RuntimeContext{
		SystemPrompt:        targetPrompt.system,
		SystemPromptBlocks:  targetPrompt.systemBlocks,
		UserContext:         targetPrompt.userContext,
		HookRunner:          targetHooks,
		ProjectRoot:         targetCWD,
		CWD:                 targetCWD,
		VisibleTools:        targetPrompt.toolSnapshot,
		ToolPromptConfig:    targetPrompt.promptConfig,
		GeneratedToolPrompt: targetPrompt.generated,
	}
	preparer, ok := s.eng.(engine.RuntimeContextResumePreparer)
	if !ok {
		return rootRuntimeError(i18n.KeyRootSessionResumeUnsupported)
	}
	prepared, err := preparer.PrepareRuntimeContextResume(ctx, entry.ID, targetProjectDir, targetRuntime)
	if err != nil {
		return err
	}
	defer prepared.Abort()
	if err := ctx.Err(); err != nil {
		return err
	}

	// Revalidate after preparation and keep a handle open through commit. The
	// process-global CWD is intentionally not changed; workspace consumers use
	// the explicit runtime snapshot published below.
	targetDir, err := os.Open(targetCWD)
	if err != nil {
		return rootRuntimeWrap(i18n.KeyRootSessionTargetDirectoryOpen, err)
	}
	defer targetDir.Close()
	if err := s.deps.commitPreparedSessionRuntimeWithAfter(
		entry.ID, targetCWD, targetAllowedDirs, targetPrompt.system, targetHooks, preparedRegistry,
		func() error {
			if err := commitPreparedRuntimeResume(ctx, prepared); err != nil {
				return rootRuntimeWrap(i18n.KeyRootSessionConversationCommit, err)
			}
			// CommitProjectSources still holds the Manager writer here. Advance
			// engine defaults before releasing it so a concurrent direct/SDK Query
			// can observe only A/A or B/B, never old engine runtime plus new skills.
			if configurable, ok := s.eng.(engine.RuntimeConfigurable); ok {
				configurable.UpdateRuntimeContext(targetRuntime)
			}
			return nil
		},
		func() {
			*s.cwd = targetCWD
			*s.sessionProjectDir = targetProjectDir
			*s.sessionID = entry.ID
			if s.hookRunnerRef != nil {
				*s.hookRunnerRef = targetHooks
			}
		},
	); err != nil {
		return err
	}
	if saver, ok := s.eng.Sessions().(engine.SessionContextSaver); ok {
		_ = saver.SaveSessionContext(entry.ID, targetCWD, detectGitBranch(targetCWD))
	}
	return nil
}

func detectGitBranch(cwd string) string {
	trimmed := strings.TrimSpace(cwd)
	if trimmed == "" {
		return ""
	}
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = trimmed
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
