package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agent-dance/luban/commands"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	toolinteraction "github.com/agent-dance/luban/internal/tools/interaction"
	toollsp "github.com/agent-dance/luban/internal/tools/lsp"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

func TestSetupRegistryOwnsLSPManagerLifecycle(t *testing.T) {
	t.Setenv("ENABLE_LSP_TOOL", "true")
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)

	registered, ok := deps.Registry.Get("LSP").(*toollsp.LSPTool)
	if !ok {
		t.Fatalf("registered LSP tool = %T, want *lsp.LSPTool", deps.Registry.Get("LSP"))
	}
	if deps.lspManager == nil {
		t.Fatal("RegistryDeps did not retain the LSP composition owner")
	}
	if registered.Manager != deps.lspManager {
		t.Fatal("RegistryDeps and registered LSP tool do not share the same manager")
	}
	if err := deps.ShutdownLSP(context.Background()); err != nil {
		t.Fatalf("ShutdownLSP returned error: %v", err)
	}
}

func TestRegistryDepsShutdownLSPIsNilSafe(t *testing.T) {
	var deps *RegistryDeps
	if err := deps.ShutdownLSP(context.Background()); err != nil {
		t.Fatalf("nil RegistryDeps ShutdownLSP returned error: %v", err)
	}
	if err := (&RegistryDeps{}).ShutdownLSP(context.Background()); err != nil {
		t.Fatalf("RegistryDeps without LSP owner returned error: %v", err)
	}
}

type registrySetupReadProvider struct {
	name  string
	model string
}

func (p *registrySetupReadProvider) Name() string    { return p.name }
func (p *registrySetupReadProvider) ModelID() string { return p.model }
func (p *registrySetupReadProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	ch := make(chan types.StreamEvent)
	close(ch)
	return ch, nil
}

type registrySetupCountingProvider struct {
	registrySetupReadProvider
	count int
	calls *int
}

func (p *registrySetupCountingProvider) CountTokens(context.Context, string) (int, error) {
	if p.calls != nil {
		(*p.calls)++
	}
	return p.count, nil
}

func hasRegisteredTool(deps *RegistryDeps, name string) bool {
	if deps == nil || deps.Registry == nil {
		return false
	}
	return deps.Registry.Get(name) != nil
}

func fileToolsUseSkillManager(deps *RegistryDeps) bool {
	if deps == nil || deps.FileReadTool == nil || deps.FileEditTool == nil || deps.SkillManager == nil {
		return false
	}
	readAdapter, readOK := deps.FileReadTool.SkillManager.(*fileSkillActivator)
	editAdapter, editOK := deps.FileEditTool.SkillManager.(*fileSkillActivator)
	return readOK && editOK && readAdapter == editAdapter && readAdapter.manager == deps.SkillManager
}

func TestSetupRegistryRegistersSendUserMessageOnce(t *testing.T) {
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	canonical := deps.Registry.Get("SendUserMessage")
	if canonical == nil {
		t.Fatal("SendUserMessage is not registered")
	}
	if _, ok := canonical.(*toolinteraction.SendUserMessageTool); !ok {
		t.Fatalf("registered SendUserMessage = %T, want *interaction.SendUserMessageTool", canonical)
	}
	count := 0
	for _, tool := range deps.Registry.All() {
		if tool.Name() == "SendUserMessage" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("canonical SendUserMessage registrations = %d, want 1", count)
	}
}

func TestSetupRegistryWebSearchRequiresNativeProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		visible  bool
	}{
		{name: "anthropic", provider: "anthropic", model: "claude-sonnet-4-6", visible: true},
		{name: "foundry", provider: "foundry", model: "claude-sonnet-4-6", visible: true},
		{name: "vertex claude 4", provider: "vertex", model: "claude-sonnet-4-6", visible: true},
		{name: "vertex older model", provider: "vertex", model: "claude-3-haiku", visible: false},
		{name: "openai", provider: "openai", model: "gpt-5", visible: false},
		{name: "gemini", provider: "gemini", model: "gemini-2.5-pro", visible: false},
		{name: "deepseek", provider: "deepseek", model: "deepseek-v4-flash", visible: false},
		{name: "bedrock", provider: "bedrock", model: "claude-sonnet-4-6", visible: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref := provider.NewProviderRef(&registrySetupReadProvider{name: test.provider, model: test.model})
			deps := SetupRegistry(ref, t.TempDir(), nil, sandbox.NoopBackend{}, nil)
			t.Cleanup(deps.StopWebFetchCache)
			webSearch := deps.Registry.Get("WebSearch")
			if webSearch == nil {
				t.Fatal("WebSearch is not registered")
			}
			if got := deps.Registry.IsToolEnabled(webSearch); got != test.visible {
				t.Fatalf("WebSearch visibility = %v, want %v for provider=%q model=%q", got, test.visible, test.provider, test.model)
			}
			if alias := deps.Registry.Get("Search"); alias != nil {
				t.Fatalf("removed Search alias is still registered as %T", alias)
			}
		})
	}
}

func TestSetupRegistrySendUserMessageEnabledRequiresOptIn(t *testing.T) {
	t.Setenv("LUBAN_CODE_SEND_USER_MESSAGE", "")
	disabled := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	tool := disabled.Registry.Get("SendUserMessage")
	if disabled.Registry.IsToolEnabled(tool) {
		t.Fatal("SendUserMessage must be registered but hidden before explicit Brief opt-in")
	}

	t.Setenv("LUBAN_CODE_SEND_USER_MESSAGE", "true")
	enabled := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if !enabled.Registry.IsToolEnabled(enabled.Registry.Get("SendUserMessage")) {
		t.Fatal("LUBAN_CODE_SEND_USER_MESSAGE=true must enable SendUserMessage")
	}
}

func TestSetupRegistry_GatesTriggerToolsWhenDisabled(t *testing.T) {
	t.Setenv("AGENT_TRIGGERS", "")
	t.Setenv("AGENT_TRIGGERS_REMOTE", "")
	t.Setenv("LUBAN_CODE_DISABLE_CRON", "")

	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if hasRegisteredTool(deps, "CronCreate") || hasRegisteredTool(deps, "CronDelete") || hasRegisteredTool(deps, "CronList") {
		t.Fatalf("expected cron tools to be absent when AGENT_TRIGGERS is disabled")
	}
	if hasRegisteredTool(deps, "RemoteTrigger") {
		t.Fatalf("expected RemoteTrigger to be absent when AGENT_TRIGGERS_REMOTE is disabled")
	}
}

func TestSetupRegistry_GatesTriggerToolsWhenEnabled(t *testing.T) {
	t.Setenv("AGENT_TRIGGERS", "1")
	t.Setenv("AGENT_TRIGGERS_REMOTE", "true")
	t.Setenv("LUBAN_CODE_DISABLE_CRON", "")

	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if deps.Schedule != nil {
		t.Cleanup(func() { stopScheduleForTest(t, deps) })
	}
	if !hasRegisteredTool(deps, "CronCreate") || !hasRegisteredTool(deps, "CronDelete") || !hasRegisteredTool(deps, "CronList") {
		t.Fatalf("expected cron tools to be present when AGENT_TRIGGERS is enabled")
	}
	if deps.scheduleStarted {
		t.Fatal("schedule delivery started before Agent runtime composition completed")
	}
	if err := deps.StartSchedule(context.Background()); err != nil {
		t.Fatalf("StartSchedule: %v", err)
	}
	if !deps.scheduleStarted {
		t.Fatal("StartSchedule did not activate delivery")
	}
	if !hasRegisteredTool(deps, "RemoteTrigger") {
		t.Fatalf("expected RemoteTrigger to be present when AGENT_TRIGGERS_REMOTE is enabled")
	}
}

func TestSetupRegistry_GatesTriggerToolsWhenCronKillSwitchEnabled(t *testing.T) {
	t.Setenv("AGENT_TRIGGERS", "1")
	t.Setenv("AGENT_TRIGGERS_REMOTE", "true")
	t.Setenv("LUBAN_CODE_DISABLE_CRON", "true")

	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if hasRegisteredTool(deps, "CronCreate") || hasRegisteredTool(deps, "CronDelete") || hasRegisteredTool(deps, "CronList") {
		t.Fatalf("expected cron tools to be absent when LUBAN_CODE_DISABLE_CRON is true")
	}
	if !hasRegisteredTool(deps, "RemoteTrigger") {
		t.Fatalf("cron kill switch must not disable RemoteTrigger")
	}
}

func TestSetupRegistry_RegistersPowerShellOnlyOnWindows(t *testing.T) {
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if deps.PowerShellTool == nil {
		t.Fatalf("expected PowerShellTool dependency to be initialized")
	}
	registered := hasRegisteredTool(deps, "PowerShell")
	if registered != (runtime.GOOS == "windows") {
		t.Fatalf("PowerShell registered=%v on %s", registered, runtime.GOOS)
	}
}

func TestSetupRegistryExposesLifecycleHookConsumers(t *testing.T) {
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if deps.TaskCreateTool == nil {
		t.Fatal("TaskCreate lifecycle hook consumer was not exposed for runtime wiring")
	}
	if registered := deps.Registry.Get("TaskCreate"); registered != deps.TaskCreateTool {
		t.Fatalf("TaskCreate dependency does not match registered tool: registered=%T dependency=%T", registered, deps.TaskCreateTool)
	}
	if deps.BackgroundTasks == nil {
		t.Fatal("background notification lifecycle consumer was not exposed")
	}
}

func TestSetupRegistry_AskUserQuestionSharesPlanState(t *testing.T) {
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	registered, ok := deps.Registry.Get("AskUserQuestion").(*toolinteraction.AskUserQuestionTool)
	if !ok {
		t.Fatalf("registered AskUserQuestion = %T", deps.Registry.Get("AskUserQuestion"))
	}
	if registered != deps.AskUserQuestionTool {
		t.Fatalf("AskUserQuestion registry pointer = %p, injected pointer = %p", registered, deps.AskUserQuestionTool)
	}
}

func TestPrepareSessionContextReportsInvalidPlanStateWithoutPublishingWorkspace(t *testing.T) {
	current := t.TempDir()
	target := t.TempDir()
	statePath := filepath.Join(target, ".luban-code", "plan-mode.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	deps := SetupRegistry(provider.NewProviderRef(nil), current, []string{current}, sandbox.NoopBackend{}, nil)
	if _, err := deps.PrepareSessionContext(target); err == nil {
		t.Fatal("session preparation ignored invalid persisted plan state")
	}
	if got := deps.RuntimeScope.ProjectRoot(); got != current {
		t.Fatalf("failed preparation published project root %q, want %q", got, current)
	}
}

func TestSetupRegistry_FileMutationToolsShareReadState(t *testing.T) {
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if deps.BashTool == nil || deps.FileReadTool == nil || deps.FileEditTool == nil || deps.FileWriteTool == nil || deps.NotebookEditTool == nil {
		t.Fatalf("missing file tools: bash=%p read=%p edit=%p write=%p notebook=%p", deps.BashTool, deps.FileReadTool, deps.FileEditTool, deps.FileWriteTool, deps.NotebookEditTool)
	}
	shared := deps.FileReadTool.ReadState
	if shared == nil || deps.BashTool.FileMutations == nil || deps.FileEditTool.ReadState != shared || deps.FileWriteTool.ReadState != shared || deps.NotebookEditTool.ReadState != shared {
		t.Fatalf("file mutation dependencies are incomplete: coordinator=%T read=%p edit=%p write=%p notebook=%p", deps.BashTool.FileMutations, shared, deps.FileEditTool.ReadState, deps.FileWriteTool.ReadState, deps.NotebookEditTool.ReadState)
	}
	if deps.FileReadTool.Runtime == nil || deps.FileEditTool.Runtime == nil || deps.FileWriteTool.Runtime == nil || deps.NotebookEditTool.Runtime == nil {
		t.Fatalf("file tools must share the live runtime scope: read=%T edit=%T write=%T notebook=%T", deps.FileReadTool.Runtime, deps.FileEditTool.Runtime, deps.FileWriteTool.Runtime, deps.NotebookEditTool.Runtime)
	}
	registered := map[string]any{
		"Read": deps.Registry.Get("Read"), "Edit": deps.Registry.Get("Edit"),
		"Write": deps.Registry.Get("Write"), "NotebookEdit": deps.Registry.Get("NotebookEdit"),
	}
	if _, ok := registered["Read"].(*toolfile.FileReadTool); !ok {
		t.Fatalf("registered Read = %T, want file.FileReadTool", registered["Read"])
	}
	if _, ok := registered["Edit"].(*toolfile.FileEditTool); !ok {
		t.Fatalf("registered Edit = %T, want file.FileEditTool", registered["Edit"])
	}
	if _, ok := registered["Write"].(*toolfile.FileWriteTool); !ok {
		t.Fatalf("registered Write = %T, want file.FileWriteTool", registered["Write"])
	}
	if _, ok := registered["NotebookEdit"].(*toolfile.NotebookEditTool); !ok {
		t.Fatalf("registered NotebookEdit = %T, want file.NotebookEditTool", registered["NotebookEdit"])
	}
	if deps.SkillManager == nil || deps.SkillTool == nil || deps.SkillTool.Manager != deps.SkillManager ||
		deps.AgentTool.SkillManager != deps.SkillManager {
		t.Fatalf("registry consumers do not share one required SkillManager: manager=%p skill=%p agent=%p",
			deps.SkillManager, deps.SkillTool.Manager, deps.AgentTool.SkillManager)
	}
	if !fileToolsUseSkillManager(deps) {
		t.Fatalf("registered Edit tool does not share the bootstrap skill manager: edit=%T shared=%T", deps.FileEditTool.SkillManager, deps.SkillManager)
	}
}

func TestSetupRegistry_ReadUsesProviderPreciseTokenCounter(t *testing.T) {
	t.Setenv("LUBAN_CODE_FILE_READ_MAX_OUTPUT_TOKENS", "1")
	root := t.TempDir()
	path := filepath.Join(root, "counted.txt")
	if err := os.WriteFile(path, []byte("alpha beta gamma delta"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	counting := &registrySetupCountingProvider{
		registrySetupReadProvider: registrySetupReadProvider{name: "anthropic", model: "claude-opus-4-6"},
		count:                     1,
		calls:                     &calls,
	}
	deps := SetupRegistry(provider.NewProviderRef(counting), root, []string{root}, sandbox.NoopBackend{}, nil)
	if deps.Schedule != nil {
		t.Cleanup(func() { stopScheduleForTest(t, deps) })
	}
	result, err := deps.FileReadTool.Execute(context.Background(), map[string]any{"file_path": path})
	if err != nil || result.IsError {
		t.Fatalf("provider token counter result=%+v err=%v", result, err)
	}
	if calls != 1 {
		t.Fatalf("provider CountTokens calls = %d, want 1", calls)
	}
}

func TestSetupRegistry_TaskToolsRemainVisibleAcrossInteractionModes(t *testing.T) {
	t.Setenv("ENABLE_TOOL_SEARCH", "0")

	interactive := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil, true)
	interactiveNames := toolNamesByDefinition(interactive.Registry.VisibleDefinitions(nil))
	if !interactiveNames["TaskCreate"] || !interactiveNames["TaskGet"] || !interactiveNames["TaskList"] || !interactiveNames["TaskUpdate"] {
		t.Fatalf("interactive task visibility mismatch: %v", interactiveNames)
	}

	nonInteractive := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil, false)
	nonInteractiveNames := toolNamesByDefinition(nonInteractive.Registry.VisibleDefinitions(nil))
	if !nonInteractiveNames["TaskCreate"] || !nonInteractiveNames["TaskGet"] || !nonInteractiveNames["TaskList"] || !nonInteractiveNames["TaskUpdate"] {
		t.Fatalf("non-interactive task visibility mismatch: %v", nonInteractiveNames)
	}
}

func TestUpdateSessionContextRefreshesSearchAndPermissionScope(t *testing.T) {
	initial := t.TempDir()
	next := t.TempDir()
	deps := SetupRegistry(provider.NewProviderRef(nil), initial, []string{initial}, sandbox.NoopBackend{}, nil)
	deps.UpdateSessionContext(next, []string{next})

	ctx := deps.RuntimeScope.ToolRuntimeContext()
	if ctx.ProjectRoot != next || len(ctx.AllowedDirs) != 1 || ctx.AllowedDirs[0] != next {
		t.Fatalf("runtime scope not refreshed: %+v", ctx)
	}
	allowed := deps.BashTool.CurrentAllowedDirs()
	if filepath.Clean(deps.BashTool.CurrentCWD()) != filepath.Clean(next) || len(allowed) != 1 || filepath.Clean(allowed[0]) != filepath.Clean(next) {
		t.Fatalf("bash scope not refreshed: cwd=%q allowed=%v", deps.BashTool.CurrentCWD(), allowed)
	}
	if deps.Registry.Get("Glob") == nil || deps.Registry.Get("Grep") == nil {
		t.Fatalf("search tools were not registered")
	}
}

func TestSetupRegistryWorktreeRuntimeUsesContextSwitcher(t *testing.T) {
	initial := t.TempDir()
	next := t.TempDir()
	deps := SetupRegistry(provider.NewProviderRef(nil), initial, []string{initial}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() { stopScheduleForTest(t, deps) })

	if err := deps.WorktreeRuntime.SwitchCWDContext(context.Background(), next); err != nil {
		t.Fatalf("switch worktree runtime: %v", err)
	}
	resolvedNext, err := filepath.EvalSymlinks(next)
	if err != nil {
		t.Fatalf("resolve expected worktree cwd: %v", err)
	}
	if got := deps.WorktreeRuntime.CurrentCWD(); filepath.Clean(got) != filepath.Clean(resolvedNext) {
		t.Fatalf("worktree cwd = %q, want %q", got, resolvedNext)
	}
	if got := deps.RuntimeScope.ToolRuntimeContext().ProjectRoot; filepath.Clean(got) != filepath.Clean(resolvedNext) {
		t.Fatalf("registry runtime root = %q, want %q", got, resolvedNext)
	}
}

func TestSessionSwitcherRejectsMalformedTargetMCPSettingsBeforeResume(t *testing.T) {
	fixture := newSessionSwitcherTestFixture(t)
	targetCWD := t.TempDir()
	fixture.engine.sessions.messages["target-session"] = []types.Message{{ID: "target-message", Role: types.RoleAssistant}}
	if err := os.WriteFile(filepath.Join(targetCWD, ".mcp.json"), []byte(`{"mcpServers":`), 0o600); err != nil {
		t.Fatalf("write malformed MCP settings: %v", err)
	}

	err := fixture.switcher.switchTo(context.Background(), commands.SessionListEntry{
		ID: "target-session", ProjectDir: filepath.Join(t.TempDir(), "target-project"), CWD: targetCWD,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "mcp") {
		t.Fatalf("switch error = %v, want traceable MCP settings error", err)
	}
	assertSessionSwitcherPreviousState(t, fixture)
	if len(fixture.engine.resumeIDs) != 0 || fixture.engine.commits != 0 {
		t.Fatalf("malformed MCP settings reached resume: resumes=%v commits=%d", fixture.engine.resumeIDs, fixture.engine.commits)
	}
}

func TestGlobAllowedDirsAreIsolatedPerRegistry(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(firstRoot, "first.txt"), []byte("first"), 0o644); err != nil {
		t.Fatalf("write first fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secondRoot, "second.txt"), []byte("second"), 0o644); err != nil {
		t.Fatalf("write second fixture: %v", err)
	}

	first := SetupRegistry(provider.NewProviderRef(nil), firstRoot, []string{firstRoot}, sandbox.NoopBackend{}, nil)
	second := SetupRegistry(provider.NewProviderRef(nil), secondRoot, []string{secondRoot}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		stopScheduleForTest(t, first)
		stopScheduleForTest(t, second)
	})

	firstResult := first.Registry.ExecuteTool(context.Background(), "Glob", map[string]any{"pattern": "*.txt"})
	if firstResult.IsError || !strings.Contains(firstResult.Content, "first.txt") || strings.Contains(firstResult.Content, "second.txt") {
		t.Fatalf("first registry Glob leaked process-global search state: error=%v content=%q", firstResult.IsError, firstResult.Content)
	}
	secondResult := second.Registry.ExecuteTool(context.Background(), "Glob", map[string]any{"pattern": "*.txt"})
	if secondResult.IsError || !strings.Contains(secondResult.Content, "second.txt") || strings.Contains(secondResult.Content, "first.txt") {
		t.Fatalf("second registry Glob leaked process-global search state: error=%v content=%q", secondResult.IsError, secondResult.Content)
	}
}

func TestGlobUpdateSessionContextUsesScopedCwdWithoutChdir(t *testing.T) {
	initial := t.TempDir()
	next := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(next, "next.txt"), []byte("next"), 0o644); err != nil {
		t.Fatalf("write next fixture: %v", err)
	}

	deps := SetupRegistry(provider.NewProviderRef(nil), initial, []string{initial}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		stopScheduleForTest(t, deps)
	})
	deps.UpdateSessionContext(next, []string{next})

	result := deps.Registry.ExecuteTool(context.Background(), "Glob", map[string]any{"pattern": "*.txt"})
	if result.IsError || !strings.Contains(result.Content, "next.txt") {
		t.Fatalf("updated scoped cwd was not used: error=%v content=%q", result.IsError, result.Content)
	}
	denied := deps.Registry.ExecuteTool(context.Background(), "Glob", map[string]any{
		"pattern": filepath.Join(outside, "*.txt"),
	})
	if !denied.IsError || !strings.Contains(strings.ToLower(denied.Content), "outside the allowed working directories") {
		t.Fatalf("absolute pattern outside updated allowedDirs must be denied: error=%v content=%q", denied.IsError, denied.Content)
	}
}

func TestSetupRegistryGrepAllowedDirsAndSessionCwd(t *testing.T) {
	initial := t.TempDir()
	next := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(initial, "initial.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatalf("write initial fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(next, "next.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatalf("write next fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}

	deps := SetupRegistry(provider.NewProviderRef(nil), initial, []string{initial}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() { stopScheduleForTest(t, deps) })
	result := deps.Registry.ExecuteTool(context.Background(), "Grep", map[string]any{"pattern": "needle"})
	if result.IsError || !strings.Contains(result.Content, "initial.txt") || strings.Contains(result.Content, "next.txt") {
		t.Fatalf("initial scoped Grep mismatch: error=%v content=%q", result.IsError, result.Content)
	}
	denied := deps.Registry.ExecuteTool(context.Background(), "Grep", map[string]any{"pattern": "needle", "path": outside})
	if !denied.IsError || !strings.Contains(strings.ToLower(denied.Content), "outside the allowed working directories") {
		t.Fatalf("outside Grep path must take production permission denial: error=%v content=%q", denied.IsError, denied.Content)
	}

	deps.UpdateSessionContext(next, []string{next})
	updated := deps.Registry.ExecuteTool(context.Background(), "Grep", map[string]any{"pattern": "needle"})
	if updated.IsError || !strings.Contains(updated.Content, "next.txt") || strings.Contains(updated.Content, "initial.txt") {
		t.Fatalf("updated scoped Grep mismatch: error=%v content=%q", updated.IsError, updated.Content)
	}
}

func TestSetupRegistryGrepScopeIsIsolatedPerRegistry(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(firstRoot, "first.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatalf("write first fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secondRoot, "second.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatalf("write second fixture: %v", err)
	}
	first := SetupRegistry(provider.NewProviderRef(nil), firstRoot, []string{firstRoot}, sandbox.NoopBackend{}, nil)
	second := SetupRegistry(provider.NewProviderRef(nil), secondRoot, []string{secondRoot}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		stopScheduleForTest(t, first)
		stopScheduleForTest(t, second)
	})
	firstResult := first.Registry.ExecuteTool(context.Background(), "Grep", map[string]any{"pattern": "needle"})
	secondResult := second.Registry.ExecuteTool(context.Background(), "Grep", map[string]any{"pattern": "needle"})
	if firstResult.IsError || !strings.Contains(firstResult.Content, "first.txt") || strings.Contains(firstResult.Content, "second.txt") {
		t.Fatalf("first registry Grep scope leaked: error=%v content=%q", firstResult.IsError, firstResult.Content)
	}
	if secondResult.IsError || !strings.Contains(secondResult.Content, "second.txt") || strings.Contains(secondResult.Content, "first.txt") {
		t.Fatalf("second registry Grep scope leaked: error=%v content=%q", secondResult.IsError, secondResult.Content)
	}
}

func toolNamesByDefinition(definitions []types.ToolDefinition) map[string]bool {
	out := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		out[definition.Name] = true
	}
	return out
}
