package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/skills"
)

func TestSkillRuntimeSharesOneRegistryStoreAndExplicitInvoker(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	writeRootTask23Skill(t, root, "task23-initial")
	writeRootTask23Skill(t, target, "task23-target")
	p := &registrySetupReadProvider{name: "test", model: "test-model"}
	ref := provider.NewProviderRef(p)
	deps := SetupRegistry(ref, root, []string{root}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})

	if deps.SkillManager == nil || deps.SkillTool == nil || deps.SkillOverrideStore == nil || deps.SkillSessionOverrides == nil {
		t.Fatalf("skill runtime dependencies incomplete: manager=%p tool=%p store=%p session=%p",
			deps.SkillManager, deps.SkillTool, deps.SkillOverrideStore, deps.SkillSessionOverrides)
	}
	if deps.SkillTool.Manager != deps.SkillManager || deps.AgentTool.SkillManager != deps.SkillManager ||
		deps.TeamManager.SkillManager != deps.SkillManager || deps.FileReadTool.SkillManager != deps.SkillManager ||
		deps.FileEditTool.SkillManager != deps.SkillManager {
		t.Fatal("registry consumers do not share the authoritative Skill Manager")
	}
	if registered := deps.Registry.Get("Skill"); registered != deps.SkillTool {
		t.Fatalf("registered Skill tool = %T %p, want %p", registered, registered, deps.SkillTool)
	}
	count := 0
	for _, tool := range deps.Registry.All() {
		if tool.Name() == "Skill" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("registered Skill tool count = %d, want 1", count)
	}
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(root)
	eng, err := engine.New(engine.Config{
		Provider: p, ProviderRef: ref, Registry: deps.Registry,
		Sessions:    engine.NewRepositorySessionManager(repo, func() string { return projectDir }),
		ProjectRoot: root, CWD: root, SkillManager: deps.SkillManager,
		SkillSessionOverrides: deps.SkillSessionOverrides,
	})
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })

	deps.BindSessionIdentity("ui-session")
	invoker := configureSkillRuntime(deps, eng)
	if invoker == nil || deps.SkillTool.LanguageResolver == nil || deps.SkillTool.SessionIDResolver == nil || deps.SkillTool.LoadedLedgerResolver == nil {
		t.Fatal("dynamic SkillTool runtime was not configured")
	}
	if got := deps.SkillTool.SessionIDResolver(context.Background()); got != "ui-session" {
		t.Fatalf("UI session resolver = %q, want ui-session", got)
	}
	modelCtx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{SessionID: "model-session"})
	if got := deps.SkillTool.SessionIDResolver(modelCtx); got != "model-session" {
		t.Fatalf("model execution session resolver = %q, want model-session", got)
	}

	previousLanguage := i18n.DetectOrLoadLanguage()
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatalf("set runtime language: %v", err)
	}
	t.Cleanup(func() { _ = i18n.SaveLanguage(previousLanguage) })
	// File-tool descriptions and schema copy intentionally follow the active
	// runtime language, so pin the fixed-schema comparison after switching it.
	definitionsBefore := deps.Registry.Definitions()
	if got := deps.SkillTool.LanguageResolver(context.Background()); got != i18n.LangZH {
		t.Fatalf("SkillTool language = %v, want zh", got)
	}
	localized, localizedErr := invoker.InvokeSkill(context.Background(), commands.SkillInvocationRequest{
		SessionID: "ui-session", Selector: "task23-missing", Origin: skills.InvocationOriginUser,
	})
	if localizedErr != nil || !localized.IsError ||
		!strings.Contains(localized.Content, i18n.Format(i18n.LangZH, i18n.KeySkillToolNotFound, "task23-missing")) {
		t.Fatalf("runtime-language rejection mismatch: result=%+v err=%v", localized, localizedErr)
	}

	snapshot, err := deps.SkillManager.Snapshot("ui-session")
	initialRow, found := rootTask23Skill(snapshot, "task23-initial")
	if err != nil || !found {
		t.Fatalf("initial skill snapshot: skills=%+v err=%v", snapshot.Skills, err)
	}
	result, err := invoker.InvokeSkill(context.Background(), commands.SkillInvocationRequest{
		SessionID: "ui-session", Selector: string(initialRow.ID),
		ExpectedRevision: initialRow.Revision,
		// The composition adapter must not let a surface downgrade or forge its
		// authority. This field is deliberately malicious.
		Origin: skills.InvocationOriginModel,
	})
	if err != nil || result.IsError || result.Metadata["invocationOrigin"] != string(skills.InvocationOriginUser) {
		t.Fatalf("explicit Skill invoker did not force user origin: result=%+v err=%v", result, err)
	}

	tuiConfig := TUIREPLConfig{SkillManager: deps.SkillManager, SkillInvoker: invoker}
	screenReaderConfig := TUIREPLConfig{SkillManager: deps.SkillManager, SkillInvoker: invoker}
	if tuiConfig.SkillManager != screenReaderConfig.SkillManager ||
		reflect.ValueOf(tuiConfig.SkillInvoker).Pointer() != reflect.ValueOf(screenReaderConfig.SkillInvoker).Pointer() {
		t.Fatal("TUI and screen-reader configs do not share Manager and SkillInvoker")
	}

	toggle, err := deps.SkillManager.ToggleProjectVisibility("ui-session", initialRow.ID, snapshot.Revision)
	if err != nil || toggle.Outcome != skills.ProjectVisibilityToggleCommitted {
		t.Fatalf("toggle project visibility through shared store: result=%+v err=%v", toggle, err)
	}
	if err := deps.UpdateSessionContext(target, []string{target}); err != nil {
		t.Fatalf("retarget skill workspace: %v", err)
	}
	if deps.SkillTool.Manager != deps.SkillManager || deps.AgentTool.SkillManager != deps.SkillManager {
		t.Fatal("workspace retarget replaced the shared Manager pointer")
	}
	targetSnapshot, err := deps.SkillManager.Snapshot("ui-session")
	_, hasInitial := rootTask23Skill(targetSnapshot, "task23-initial")
	_, hasTarget := rootTask23Skill(targetSnapshot, "task23-target")
	if err != nil || hasInitial || !hasTarget {
		t.Fatalf("retargeted snapshot leaked old project: skills=%+v err=%v", targetSnapshot.Skills, err)
	}
	if definitionsAfter := deps.Registry.Definitions(); !reflect.DeepEqual(definitionsAfter, definitionsBefore) {
		t.Fatal("catalog mutation changed fixed tool schemas")
	}
}

func TestSkillWorkspacePreparationFailsClosedBeforeRuntimePublish(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	writeRootTask23Skill(t, target, "task23-target")
	deps := SetupRegistry(provider.NewProviderRef(nil), root, []string{root}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})

	missing := filepath.Join(t.TempDir(), "missing-workspace")
	if err := deps.UpdateSessionContext(missing, []string{missing}); err == nil {
		t.Fatal("missing target workspace unexpectedly prepared")
	}
	if got := deps.RuntimeScope.ProjectRoot(); got != root {
		t.Fatalf("missing target partially published runtime root %q, want %q", got, root)
	}

	deps.SkillManager.SetOverrideStore(nil)
	if err := deps.UpdateSessionContext(target, []string{target}); err == nil {
		t.Fatal("target workspace without override store unexpectedly prepared")
	}
	if got := deps.RuntimeScope.ProjectRoot(); got != root {
		t.Fatalf("missing skill store partially published runtime root %q, want %q", got, root)
	}
	snapshot, err := deps.SkillManager.Snapshot("session-a")
	if err != nil {
		t.Fatalf("catalog failed after rejected workspace switch: %v", err)
	}
	if _, found := rootTask23Skill(snapshot, "task23-target"); found {
		t.Fatalf("rejected workspace switch exposed target project skills: %+v", snapshot.Skills)
	}
}

func TestSkillWorkspacePreparationRejectsMalformedTargetOverrides(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	writeRootTask23Skill(t, root, "task23-initial")
	writeRootTask23Skill(t, target, "task23-target")
	targetSettings := filepath.Join(target, ".luban-code", "settings.json")
	if err := os.WriteFile(targetSettings, []byte("{\"skillOverrides\":[]}"), 0o600); err != nil {
		t.Fatalf("write malformed target overrides: %v", err)
	}

	deps := SetupRegistry(provider.NewProviderRef(nil), root, []string{root}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})
	before, err := deps.SkillManager.Snapshot("session-a")
	if err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}
	if _, found := rootTask23Skill(before, "task23-initial"); !found {
		t.Fatalf("initial project skill missing: %+v", before.Skills)
	}

	if err := deps.UpdateSessionContext(target, []string{target}); err == nil {
		t.Fatal("malformed target override document unexpectedly prepared")
	}
	if got := deps.RuntimeScope.ProjectRoot(); got != root {
		t.Fatalf("malformed target partially published runtime root %q, want %q", got, root)
	}
	after, err := deps.SkillManager.Snapshot("session-a")
	if err != nil {
		t.Fatalf("old catalog unavailable after rejected target: %v", err)
	}
	_, hasInitial := rootTask23Skill(after, "task23-initial")
	_, hasTarget := rootTask23Skill(after, "task23-target")
	if !hasInitial || hasTarget || !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected target changed authoritative catalog: before=%+v after=%+v", before.Skills, after.Skills)
	}
}

func TestInitialSkillRuntimeRejectsMalformedOverridesBeforeEngine(t *testing.T) {
	root := t.TempDir()
	writeRootTask23Skill(t, root, "task23-initial")
	settings := filepath.Join(root, ".luban-code", "settings.json")
	if err := os.WriteFile(settings, []byte("{\"skillOverrides\":[]}"), 0o600); err != nil {
		t.Fatalf("write malformed initial overrides: %v", err)
	}
	deps := SetupRegistry(provider.NewProviderRef(nil), root, []string{root}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})
	if deps.skillInitErr == nil {
		t.Fatal("malformed initial overrides did not fail registry skill initialization")
	}
	if deps.SkillManager != nil || deps.SkillOverrideStore != nil || deps.SkillTool.Manager != nil ||
		deps.AgentTool.SkillManager != nil || deps.TeamManager.SkillManager != nil ||
		deps.FileReadTool.SkillManager != nil || deps.FileEditTool.SkillManager != nil {
		t.Fatalf("malformed initial overrides left a default authority: deps=%p store=%p tool=%p agent=%p team=%p read=%p edit=%p",
			deps.SkillManager, deps.SkillOverrideStore, deps.SkillTool.Manager, deps.AgentTool.SkillManager,
			deps.TeamManager.SkillManager, deps.FileReadTool.SkillManager, deps.FileEditTool.SkillManager)
	}
	if err := prepareInitialRegistryRuntime(deps, root, []string{root}); err == nil {
		t.Fatal("startup gate accepted malformed initial skill policy")
	}
	result, err := deps.SkillTool.Execute(context.Background(), map[string]any{"skill": "task23-initial"})
	if err != nil || !result.IsError ||
		!strings.Contains(result.Content, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeySkillToolUnavailable)) {
		t.Fatalf("failed registry Skill execution did not fail closed: result=%+v err=%v", result, err)
	}
}

func TestInitialSkillRuntimeRejectsMalformedUserOverridesBeforeEngine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	writeRootTask23Skill(t, root, "task23-initial")
	userDir := filepath.Join(home, ".luban-code")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("create user config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "settings.json"), []byte("{\"skillOverrides\":[]}"), 0o600); err != nil {
		t.Fatalf("write malformed user overrides: %v", err)
	}
	deps := SetupRegistry(provider.NewProviderRef(nil), root, []string{root}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})
	if deps.skillInitErr == nil || deps.SkillManager != nil || deps.SkillTool.Manager != nil {
		t.Fatalf("malformed user policy did not fail closed: initErr=%v manager=%p toolManager=%p",
			deps.skillInitErr, deps.SkillManager, deps.SkillTool.Manager)
	}
	if err := prepareInitialRegistryRuntime(deps, root, []string{root}); err == nil {
		t.Fatal("startup gate accepted malformed user skill policy")
	}
}

func TestSkillWorkspacePreparationBlocksCrossRootRunningAgent(t *testing.T) {
	initial := t.TempDir()
	target := t.TempDir()
	writeRootTask23Skill(t, initial, "task23-initial")
	writeRootTask23Skill(t, target, "task23-target")
	deps := SetupRegistry(provider.NewProviderRef(nil), initial, []string{initial}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})
	before, err := deps.SkillManager.Snapshot("session-a")
	if err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}

	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	task, err := deps.BackgroundTasks.StartAgentTask(context.Background(), "prompt", "running agent",
		func(ctx context.Context, _ io.Writer) (string, error) {
			select {
			case <-release:
				return "completed", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})
	if err != nil {
		t.Fatalf("start background agent: %v", err)
	}
	if _, err := deps.PrepareSessionContext(initial); err != nil {
		t.Fatalf("same-workspace running agent was incorrectly blocked: %v", err)
	}
	if err := deps.UpdateSessionContext(target, []string{target}); err == nil {
		t.Fatal("cross-workspace switch accepted a running model consumer")
	}
	if got := deps.RuntimeScope.ProjectRoot(); got != initial {
		t.Fatalf("blocked switch published runtime root %q, want %q", got, initial)
	}
	after, err := deps.SkillManager.Snapshot("session-a")
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("blocked switch changed Manager catalog: before=%+v after=%+v err=%v", before.Skills, after.Skills, err)
	}

	close(release)
	if finished, status := deps.BackgroundTasks.Wait(task.ID, 5*time.Second); status != "success" || finished.Status != "completed" {
		t.Fatalf("background agent did not finish naturally: status=%s snapshot=%+v", status, finished)
	}
	if err := deps.UpdateSessionContext(target, []string{target}); err != nil {
		t.Fatalf("terminal background agent still blocked workspace switch: %v", err)
	}
	targetSnapshot, err := deps.SkillManager.Snapshot("session-a")
	if err != nil {
		t.Fatalf("target snapshot: %v", err)
	}
	if _, found := rootTask23Skill(targetSnapshot, "task23-target"); !found {
		t.Fatalf("allowed terminal-task switch did not publish target catalog: %+v", targetSnapshot.Skills)
	}
}

func rootTask23Skill(snapshot skills.CatalogSnapshot, name string) (skills.EffectiveSkill, bool) {
	for _, row := range snapshot.Skills {
		if row.Name == name && row.Source == skills.SourceProject {
			return row, true
		}
	}
	return skills.EffectiveSkill{}, false
}

func writeRootTask23Skill(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, ".luban-code", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + name + "\n---\nbody"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}
}
