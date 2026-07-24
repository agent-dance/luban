package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/commands"
	legacyMCP "github.com/agent-dance/luban/mcp"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/types"
)

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

func TestSetupRegistrySendUserMessageCanonicalAlias(t *testing.T) {
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	canonical := deps.Registry.Get("SendUserMessage")
	legacy := deps.Registry.Get("Brief")
	if canonical == nil || legacy == nil || canonical != legacy {
		t.Fatalf("canonical/alias = %T %T (same=%v)", canonical, legacy, canonical == legacy)
	}
	if _, ok := canonical.(*tools.SendUserMessageTool); !ok {
		t.Fatalf("registered SendUserMessage = %T, want *tools.SendUserMessageTool", canonical)
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

func TestSetupRegistryEnablesWebSearchFallbackForEveryProvider(t *testing.T) {
	for _, providerName := range []string{"anthropic", "openai", "gemini", "deepseek", "bedrock", "vertex"} {
		t.Run(providerName, func(t *testing.T) {
			ref := provider.NewProviderRef(&registrySetupReadProvider{name: providerName, model: "test-model"})
			deps := SetupRegistry(ref, t.TempDir(), nil, sandbox.NoopBackend{}, nil)
			t.Cleanup(deps.StopWebFetchCache)
			webSearch := deps.Registry.Get("WebSearch")
			if webSearch == nil || !deps.Registry.IsToolEnabled(webSearch) {
				t.Fatalf("WebSearch disabled for provider %q", providerName)
			}
			if alias := deps.Registry.Get("Search"); alias != webSearch {
				t.Fatalf("Search alias = %T, want registered WebSearch", alias)
			}
		})
	}
}

func TestSetupRegistrySendUserMessageEnabledRequiresOptIn(t *testing.T) {
	t.Setenv("CLAUDE_CODE_BRIEF", "")
	disabled := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	tool := disabled.Registry.Get("SendUserMessage")
	if disabled.Registry.IsToolEnabled(tool) {
		t.Fatal("SendUserMessage must be registered but hidden before explicit Brief opt-in")
	}

	t.Setenv("CLAUDE_CODE_BRIEF", "true")
	enabled := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if !enabled.Registry.IsToolEnabled(enabled.Registry.Get("SendUserMessage")) {
		t.Fatal("CLAUDE_CODE_BRIEF=true must enable SendUserMessage")
	}
}

func TestSetupRegistry_GatesTriggerToolsWhenDisabled(t *testing.T) {
	t.Setenv("AGENT_TRIGGERS", "")
	t.Setenv("AGENT_TRIGGERS_REMOTE", "")
	t.Setenv("CLAUDE_CODE_DISABLE_CRON", "")

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
	t.Setenv("CLAUDE_CODE_DISABLE_CRON", "")

	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if deps.CronStore != nil {
		t.Cleanup(deps.CronStore.Stop)
	}
	if !hasRegisteredTool(deps, "CronCreate") || !hasRegisteredTool(deps, "CronDelete") || !hasRegisteredTool(deps, "CronList") {
		t.Fatalf("expected cron tools to be present when AGENT_TRIGGERS is enabled")
	}
	if !hasRegisteredTool(deps, "RemoteTrigger") {
		t.Fatalf("expected RemoteTrigger to be present when AGENT_TRIGGERS_REMOTE is enabled")
	}
}

func TestSetupRegistry_GatesTriggerToolsWhenCronKillSwitchEnabled(t *testing.T) {
	t.Setenv("AGENT_TRIGGERS", "1")
	t.Setenv("AGENT_TRIGGERS_REMOTE", "true")
	t.Setenv("CLAUDE_CODE_DISABLE_CRON", "true")

	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if hasRegisteredTool(deps, "CronCreate") || hasRegisteredTool(deps, "CronDelete") || hasRegisteredTool(deps, "CronList") {
		t.Fatalf("expected cron tools to be absent when CLAUDE_CODE_DISABLE_CRON is true")
	}
	if !hasRegisteredTool(deps, "RemoteTrigger") {
		t.Fatalf("cron kill switch must not disable RemoteTrigger")
	}
}

func TestSetupRegistry_CanEnablePowerShellTool(t *testing.T) {
	t.Setenv("ENABLE_POWERSHELL_TOOL", "1")

	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if deps.PowerShellTool == nil {
		t.Fatalf("expected PowerShellTool dependency to be initialized")
	}
	if !hasRegisteredTool(deps, "PowerShell") {
		t.Fatalf("expected PowerShell tool to be registered when ENABLE_POWERSHELL_TOOL=1")
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
	registered, ok := deps.Registry.Get("AskUserQuestion").(*tools.AskUserQuestionTool)
	if !ok {
		t.Fatalf("registered AskUserQuestion = %T", deps.Registry.Get("AskUserQuestion"))
	}
	if registered.PlanState == nil || registered.PlanState != deps.PlanState {
		t.Fatalf("AskUserQuestion PlanState = %p, shared PlanState = %p", registered.PlanState, deps.PlanState)
	}
	if registered != deps.AskUserQuestionTool {
		t.Fatalf("AskUserQuestion registry pointer = %p, injected pointer = %p", registered, deps.AskUserQuestionTool)
	}
}

func TestSetupRegistry_BashSharesReadFileState(t *testing.T) {
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if deps.BashTool == nil || deps.FileReadTool == nil || deps.FileEditTool == nil || deps.FileWriteTool == nil {
		t.Fatalf("missing file tools: bash=%p read=%p edit=%p write=%p", deps.BashTool, deps.FileReadTool, deps.FileEditTool, deps.FileWriteTool)
	}
	shared := deps.FileReadTool.ReadState
	if shared == nil || deps.BashTool.ReadFileState != shared || deps.FileEditTool.ReadState != shared || deps.FileWriteTool.ReadState != shared {
		t.Fatalf("file tools do not share ReadFileState: bash=%p read=%p edit=%p write=%p", deps.BashTool.ReadFileState, shared, deps.FileEditTool.ReadState, deps.FileWriteTool.ReadState)
	}
	if !deps.BashTool.SedValidationEnabled {
		t.Fatal("registered Bash tool must enforce sed Read-before-edit validation")
	}
	if deps.FileEditTool.SkillManager == nil || deps.FileEditTool.SkillManager != deps.SkillManager {
		t.Fatalf("registered Edit tool does not share the bootstrap skill manager: edit=%T shared=%T", deps.FileEditTool.SkillManager, deps.SkillManager)
	}
}

func TestSetupRegistry_ReadTracksProviderRefInstalledAfterBootstrap(t *testing.T) {
	tools.SetActiveModelForCyberGating("")
	t.Cleanup(func() { tools.SetActiveModelForCyberGating("") })

	root := t.TempDir()
	ref := provider.NewProviderRef(nil)
	deps := SetupRegistry(ref, root, []string{root}, sandbox.NoopBackend{}, nil)
	if deps.CronStore != nil {
		t.Cleanup(deps.CronStore.Stop)
	}

	ref.Swap(&registrySetupReadProvider{name: "anthropic", model: "claude-sonnet-4-5"})
	if got := deps.RuntimeScope.ToolRuntimeContext().Model; got != "claude-sonnet-4-5" {
		t.Fatalf("runtime model after nil-to-live swap = %q", got)
	}
	nonExemptPath := filepath.Join(root, "non-exempt.txt")
	if err := os.WriteFile(nonExemptPath, []byte("ordinary text"), 0o600); err != nil {
		t.Fatal(err)
	}
	nonExempt, err := deps.FileReadTool.Execute(context.Background(), map[string]any{"file_path": nonExemptPath})
	if err != nil || nonExempt.IsError || strings.Contains(nonExempt.Content, tools.CyberRiskMitigationReminder) || len(nonExempt.NewMessages) != 1 ||
		nonExempt.NewMessages[0].InternalKind != types.InternalMessageKindFileReadSecurity || !nonExempt.NewMessages[0].IsMeta ||
		!strings.Contains(nonExempt.NewMessages[0].GetText(), strings.TrimSpace(tools.CyberRiskMitigationReminder)) {
		t.Fatalf("non-exempt swapped model was not used by Read: result=%+v err=%v", nonExempt, err)
	}

	ref.Swap(&registrySetupReadProvider{name: "anthropic", model: "claude-opus-4-6"})
	exemptPath := filepath.Join(root, "exempt.txt")
	if err := os.WriteFile(exemptPath, []byte("ordinary text"), 0o600); err != nil {
		t.Fatal(err)
	}
	exempt, err := deps.FileReadTool.Execute(context.Background(), map[string]any{"file_path": exemptPath})
	if err != nil || exempt.IsError || strings.Contains(exempt.Content, tools.CyberRiskMitigationReminder) || len(exempt.NewMessages) != 0 {
		t.Fatalf("exempt swapped model was not used by Read: result=%+v err=%v", exempt, err)
	}

	ref.Swap(&registrySetupReadProvider{name: "anthropic", model: "claude-3-haiku-20240307"})
	pdfPath := filepath.Join(root, "unsupported.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unsupported, err := deps.FileReadTool.Execute(context.Background(), map[string]any{"file_path": pdfPath})
	if err != nil || !unsupported.IsError || !strings.Contains(unsupported.Content, "Reading full PDFs is not supported with this model") {
		t.Fatalf("Haiku 3 PDF gate did not follow swapped model: result=%+v err=%v", unsupported, err)
	}
	if unsupported.Data != nil || len(unsupported.NewMessages) != 0 || strings.Contains(unsupported.TextContent(), tools.CyberRiskMitigationReminder) {
		t.Fatalf("unsupported PDF leaked rich data/reminder: %+v", unsupported)
	}
}

func TestSetupRegistry_ReadUsesProviderPreciseTokenCounter(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS", "1")
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
	if deps.CronStore != nil {
		t.Cleanup(deps.CronStore.Stop)
	}
	result, err := deps.FileReadTool.Execute(context.Background(), map[string]any{"file_path": path})
	if err != nil || result.IsError {
		t.Fatalf("provider token counter result=%+v err=%v", result, err)
	}
	if calls != 1 {
		t.Fatalf("provider CountTokens calls = %d, want 1", calls)
	}
}

func TestSetupRegistry_ReadDiscoversAndActivatesRealSkillManager(t *testing.T) {
	tools.ResetDynamicSkillTriggersForTest()
	t.Cleanup(tools.ResetDynamicSkillTriggersForTest)
	root := t.TempDir()
	skillDir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(filepath.Join(skillDir, "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "review", "SKILL.md"), []byte("---\nname: review\ndescription: Review nearby source files\n---\n# Review\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(root, "src")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targetDir, "trigger.go")
	if err := os.WriteFile(target, []byte("package trigger\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools.RegisterDynamicSkillBasenameTrigger(filepath.Base(target), "review")

	deps := SetupRegistry(provider.NewProviderRef(&registrySetupReadProvider{name: "anthropic", model: "claude-opus-4-6"}), root, []string{root}, sandbox.NoopBackend{}, nil)
	if deps.CronStore != nil {
		t.Cleanup(deps.CronStore.Stop)
	}
	result := deps.Registry.ExecuteTool(context.Background(), "Read", map[string]any{"file_path": target})
	if result.IsError {
		t.Fatalf("production registry Read failed: %s", result.TextContent())
	}
	if got := deps.SkillManager.Get("review"); got == nil {
		t.Fatal("nearby review skill was not loaded into the shared production manager")
	}
	activated := deps.SkillManager.ActivatedConditionalSkillNames()
	if len(activated) != 1 || activated[0] != "review" {
		t.Fatalf("named conditional activation = %v, want [review]", activated)
	}
	dirs := deps.FileReadTool.DynamicSkillDirTriggers()
	if len(dirs) != 1 || filepath.Clean(dirs[0]) != filepath.Clean(skillDir) {
		t.Fatalf("dynamic skill directories = %v, want %q", dirs, skillDir)
	}
}

func TestSetupRegistry_VisibilityUsesInteractiveTaskGate(t *testing.T) {
	t.Setenv("CLAUDE_CODE_ENABLE_TASKS", "")
	t.Setenv("ENABLE_TOOL_SEARCH", "0")

	interactive := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil, true)
	interactiveNames := toolNamesByDefinition(interactive.Registry.VisibleDefinitions(nil))
	if !interactiveNames["TaskCreate"] || interactiveNames["TodoWrite"] {
		t.Fatalf("interactive task visibility mismatch: %v", interactiveNames)
	}

	nonInteractive := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil, false)
	nonInteractiveNames := toolNamesByDefinition(nonInteractive.Registry.VisibleDefinitions(nil))
	if nonInteractiveNames["TaskCreate"] || !nonInteractiveNames["TodoWrite"] {
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
	if filepath.Clean(deps.BashTool.CWD) != filepath.Clean(next) || len(deps.BashTool.AllowedDirs) != 1 || filepath.Clean(deps.BashTool.AllowedDirs[0]) != filepath.Clean(next) {
		t.Fatalf("bash scope not refreshed: cwd=%q allowed=%v", deps.BashTool.CWD, deps.BashTool.AllowedDirs)
	}
	if deps.GlobTool == nil || deps.GrepTool == nil {
		t.Fatalf("search tools were not exposed for scoped runtime wiring: Glob=%p Grep=%p", deps.GlobTool, deps.GrepTool)
	}
}

func TestUpdateSessionContextReplacesWorkspaceMCPAndRetargetsFileHistory(t *testing.T) {
	initial := t.TempDir()
	target := t.TempDir()
	writeRegistryMCPSettings(t, initial, "old-project", "old-command")
	writeRegistryMCPSettings(t, target, "target-project", "target-command")

	deps := SetupRegistry(provider.NewProviderRef(nil), initial, []string{initial}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.MCPManager.Shutdown()
	})
	deps.MCPManager.AddConfig("user-server", legacyMCP.ServerConfig{Command: "user-command", Scope: string(svcmcp.ScopeUser)})
	deps.ServiceMCP.AddConfig("user-server", svcmcp.MCPServerConfig{Command: "user-command", Scope: svcmcp.ScopeUser})
	staleDynamic := tools.NewDynamicMCPTool(deps.ServiceMCP, "old-project", svcmcp.ToolDefinition{Name: "stale-tool"})
	deps.Registry.SyncMCPDynamicTools([]types.Tool{staleDynamic})
	if deps.Registry.Get(staleDynamic.Name()) == nil {
		t.Fatal("failed to install stale dynamic MCP fixture")
	}

	if err := deps.UpdateSessionContext(target, []string{target}); err != nil {
		t.Fatalf("update target session context: %v", err)
	}

	wantNames := []string{"target-project", "user-server"}
	if got := deps.MCPManager.ServerNames(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("legacy MCP names = %v, want %v", got, wantNames)
	}
	if got := deps.ServiceMCP.ServerNames(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("service MCP names = %v, want %v", got, wantNames)
	}
	if state, ok := deps.ServiceMCP.State("user-server"); !ok || state.Config.Scope != svcmcp.ScopeUser {
		t.Fatalf("user-scoped MCP config was not preserved: ok=%v state=%+v", ok, state)
	}
	if deps.Registry.Get(staleDynamic.Name()) != nil {
		t.Fatalf("stale dynamic MCP tool %q survived workspace replacement", staleDynamic.Name())
	}

	trackedPath := filepath.Join(target, "tracked.txt")
	entry := tools.FileHistoryEntry{Path: trackedPath, Before: "before", After: "after", Tool: "Edit", Ts: time.Now().UnixMilli()}
	if err := deps.FileEditTool.HistoryStore.TrackEdit(entry); err != nil {
		t.Fatalf("track target edit: %v", err)
	}
	targetHistory := tools.NewFileHistoryStore(filepath.Join(target, ".luban-code", "file-history"))
	entries, err := targetHistory.ListEdits(trackedPath)
	if err != nil {
		t.Fatalf("read target history: %v", err)
	}
	if len(entries) != 1 || entries[0].After != "after" {
		t.Fatalf("target history entries = %+v, want one retargeted edit", entries)
	}
	if deps.FileWriteTool.HistoryStore != deps.FileEditTool.HistoryStore || deps.NotebookEditTool.HistoryStore != deps.FileEditTool.HistoryStore {
		t.Fatal("File/Edit/Notebook no longer share one retargeted history store")
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

func TestUpdateSessionContextMalformedMCPLeavesPublishedRegistryUnchanged(t *testing.T) {
	initial := t.TempDir()
	target := t.TempDir()
	writeRegistryMCPSettings(t, initial, "old-project", "old-command")
	if err := os.WriteFile(filepath.Join(target, ".mcp.json"), []byte(`{"mcpServers":`), 0o600); err != nil {
		t.Fatalf("write malformed target MCP settings: %v", err)
	}
	deps := SetupRegistry(provider.NewProviderRef(nil), initial, []string{initial}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.MCPManager.Shutdown()
	})

	err := deps.UpdateSessionContext(target, []string{target})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "mcp") {
		t.Fatalf("update error = %v, want traceable MCP error", err)
	}
	if runtime := deps.RuntimeScope.ToolRuntimeContext(); runtime.ProjectRoot != initial {
		t.Fatalf("malformed target partially published runtime: %+v", runtime)
	}
	if got := deps.MCPManager.ServerNames(); !reflect.DeepEqual(got, []string{"old-project"}) {
		t.Fatalf("legacy MCP changed after rejected target: %v", got)
	}
	if got := deps.ServiceMCP.ServerNames(); !reflect.DeepEqual(got, []string{"old-project"}) {
		t.Fatalf("service MCP changed after rejected target: %v", got)
	}

	trackedPath := filepath.Join(initial, "unchanged.txt")
	if err := deps.FileWriteTool.HistoryStore.TrackEdit(tools.FileHistoryEntry{
		Path: trackedPath, Before: "before", After: "after", Tool: "Write", Ts: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("track edit after rejected target: %v", err)
	}
	entries, err := tools.NewFileHistoryStore(filepath.Join(initial, ".luban-code", "file-history")).ListEdits(trackedPath)
	if err != nil || len(entries) != 1 {
		t.Fatalf("history root changed after rejected target: entries=%+v err=%v", entries, err)
	}
}

func writeRegistryMCPSettings(t *testing.T, cwd, name, command string) {
	t.Helper()
	dir := filepath.Join(cwd, ".luban-code")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create MCP settings directory: %v", err)
	}
	payload := []byte(`{"mcpServers":{"` + name + `":{"type":"stdio","command":"` + command + `"}}}`)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), payload, 0o600); err != nil {
		t.Fatalf("write MCP settings: %v", err)
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
		first.CronStore.Stop()
		second.CronStore.Stop()
		tools.SetAllowedSearchDirs(nil)
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
		deps.CronStore.Stop()
		tools.SetAllowedSearchDirs(nil)
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
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
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
	t.Cleanup(func() { deps.CronStore.Stop() })
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
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
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
		first.CronStore.Stop()
		second.CronStore.Stop()
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
