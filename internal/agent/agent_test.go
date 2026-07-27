package agent

import (
	"context"
	"encoding/json"
	"fmt"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	shell "github.com/agent-dance/luban/internal/tools/shell"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/gitutil"
	"github.com/agent-dance/luban/internal/runtime/loop"
	storepaths "github.com/agent-dance/luban/internal/store/paths"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	toolsearch "github.com/agent-dance/luban/internal/tools/search"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func agentExecuteInput(prompt string, fields map[string]any) map[string]any {
	input := map[string]any{
		"prompt":      prompt,
		"description": prompt,
	}
	for key, value := range fields {
		input[key] = value
	}
	return input
}

func TestAgentToolDepthLimit(t *testing.T) {
	reg := registry.New()
	agentTool := &AgentTool{
		Depth:    DefaultMaxAgentDepth, // already at max
		MaxDepth: DefaultMaxAgentDepth,
		Registry: reg,
	}

	result, err := agentTool.Execute(context.Background(), agentExecuteInput("do something", nil))
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error when depth limit exceeded")
	}
	if result.Content == "" {
		t.Error("expected error message about depth limit")
	}
}

func TestAgentToolDepthDefaultIs3(t *testing.T) {
	if DefaultMaxAgentDepth != 3 {
		t.Errorf("expected default max depth 3, got %d", DefaultMaxAgentDepth)
	}
}

func TestAgentToolSchema(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "")
	tool := &AgentTool{}
	schema := tool.Schema()
	if strings.Join(schema.Required, ",") != "description,prompt" {
		t.Fatalf("expected Agent schema to require description and prompt, got %#v", schema.Required)
	}
	if _, ok := schema.Properties["cwd"]; ok {
		t.Fatalf("expected external Agent schema to omit cwd")
	}
	if _, ok := schema.Properties["model"]; ok {
		t.Fatalf("expected Go/Codex Agent schema to omit Claude model override")
	}
	if _, ok := schema.Properties["mode"]; ok {
		t.Fatalf("Agent schema must not expose permission mode")
	}
	if _, ok := schema.Properties["run_in_background"]; !ok {
		t.Fatalf("expected run_in_background in schema when background tasks are enabled")
	}
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "1")
	if _, ok := tool.Schema().Properties["run_in_background"]; ok {
		t.Fatalf("expected run_in_background omitted when background tasks are disabled")
	}
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "1")
	if _, ok := tool.Schema().Properties["run_in_background"]; ok {
		t.Fatalf("expected run_in_background omitted when fork subagent mode is enabled")
	}
	reg := registry.New()
	reg.Register(tool)
	if got := reg.Get("Agent"); got == nil || got.Name() != "Agent" {
		t.Fatalf("expected Agent lookup, got %#v", got)
	}
}

func TestAgentTypesDoNotCarryPermissionModeInputs(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"AgentInput":      reflect.TypeOf(agentcontract.Input{}),
		"agentProfile":    reflect.TypeOf(agentProfile{}),
		"AgentDefinition": reflect.TypeOf(AgentDefinition{}),
	} {
		if _, ok := typ.FieldByName("Mode"); ok {
			t.Fatalf("%s must not carry a Mode permission input", name)
		}
		if _, ok := typ.FieldByName("PermissionMode"); ok {
			t.Fatalf("%s must not carry a PermissionMode input", name)
		}
	}
}

func TestAgentToolGuidanceDistinguishesParallelFromBackground(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "")
	tool := &AgentTool{}

	description := tool.Description()
	for _, want := range []string{
		"Subagents always inherit the current session model",
		"Use foreground by default when you need the agent's result before proceeding",
		"Parallel Agent calls are not the same as background agents",
		"you MUST send a single assistant message with multiple Agent tool calls",
		"omit run_in_background",
		"Omit isolation for read-only research or comparison agents",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("expected Agent description to contain %q, got:\n%s", want, description)
		}
	}

	runInBackground, ok := tool.Schema().Properties["run_in_background"].(map[string]any)
	if !ok {
		t.Fatalf("expected run_in_background schema property")
	}
	schemaDescription, _ := runInBackground["description"].(string)
	if !strings.Contains(schemaDescription, "continue or end without its result") ||
		!strings.Contains(schemaDescription, "Omit for parallel research/comparison agents") {
		t.Fatalf("expected run_in_background schema to discourage result-dependent parallel use, got %q", schemaDescription)
	}
}

func TestAgentToolTeamMemberGuidanceHidesUnavailableParameters(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "")
	tool := &AgentTool{TeamMember: true}

	description := tool.Description()
	if !strings.Contains(description, "The run_in_background, name, and team_name parameters are not available in this context") {
		t.Fatalf("expected teammate prompt to hide unavailable parameters, got:\n%s", description)
	}
	if strings.Contains(description, "Background agents:") {
		t.Fatalf("expected teammate prompt to omit background guidance, got:\n%s", description)
	}

	schema := tool.Schema()
	for _, param := range []string{"run_in_background", "name", "team_name"} {
		if _, ok := schema.Properties[param]; ok {
			t.Fatalf("expected teammate schema to omit %q, got %#v", param, schema.Properties[param])
		}
	}
}

func TestAgentToolExecuteIgnoresClaudeModelOverrideAndInheritsParentModel(t *testing.T) {
	provider := &captureAgentProvider{responses: []string{"same model done"}}
	tool := &AgentTool{
		Provider: provider,
		Registry: registry.New(),
	}
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		Model:  "openai/gpt-5.4",
		System: "parent system",
	})

	result, err := tool.Execute(ctx, agentExecuteInput("inspect with inherited model", map[string]any{
		"model": "sonnet",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected subagent success, got %s", result.Content)
	}
	if len(provider.params) != 1 {
		t.Fatalf("expected one provider call, got %d", len(provider.params))
	}
	if provider.params[0].Model != "openai/gpt-5.4" {
		t.Fatalf("expected parent model openai/gpt-5.4, got %q", provider.params[0].Model)
	}
}

func TestAgentToolStillRejectsTrueIsolation(t *testing.T) {
	provider := &captureAgentProvider{responses: []string{"must not run"}}
	tool := &AgentTool{Provider: provider, Registry: registry.New()}

	result, err := tool.Execute(context.Background(), agentExecuteInput("invalid isolation", map[string]any{
		"isolation": true,
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "cannot unmarshal bool") {
		t.Fatalf("isolation=true result = %#v", result)
	}
	if len(provider.params) != 0 {
		t.Fatalf("invalid isolation reached provider %d time(s)", len(provider.params))
	}
}

func TestAgentToolBuiltInProfilesDoNotInjectDefaultFiftyTurnCap(t *testing.T) {
	const toolTurns = 51
	provider := &turnLimitAgentProvider{
		toolName:  "Echo",
		toolTurns: toolTurns,
		finalText: "finished after explicit tool loop",
	}
	reg := registry.New()
	reg.Register(fakeTool{name: "Echo"})
	tool := &AgentTool{
		Provider: provider,
		Registry: reg,
		Model:    "parent-model",
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("search thoroughly", map[string]any{
		"subagent_type": "Explore",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("built-in Explore should not hard fail after 50 turns, got %s", result.Content)
	}
	if result.Content != provider.finalText {
		t.Fatalf("one-shot Explore should return plain content, got %q", result.Content)
	}
	if provider.calls != toolTurns+1 {
		t.Fatalf("expected %d provider calls, got %d", toolTurns+1, provider.calls)
	}
}

func TestAgentToolExploreOmitsParentBaseSystemContext(t *testing.T) {
	provider := &captureAgentProvider{responses: []string{"explore done"}}
	tool := &AgentTool{
		Provider: provider,
		Registry: registry.New(),
		Model:    "parent-model",
		System:   "base identity\n\n# Git Context\nWorking tree:\n M noisy.go\n\n# User Instructions\nAlways write implementation plans before searching.",
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("inspect", map[string]any{
		"subagent_type": "Explore",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected Explore success, got %s", result.Content)
	}
	if len(provider.params) != 1 {
		t.Fatalf("expected one provider call, got %d", len(provider.params))
	}
	system := provider.params[0].System
	if !strings.Contains(system, "file search specialist") {
		t.Fatalf("expected Explore system prompt, got %q", system)
	}
	for _, omitted := range []string{"# Git Context", "# User Instructions", "Always write implementation plans"} {
		if strings.Contains(system, omitted) {
			t.Fatalf("Explore should omit parent dynamic context %q, got:\n%s", omitted, system)
		}
	}
}

func TestAgentToolOneShotBuiltInBypassesRetainedSession(t *testing.T) {
	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	provider := &captureAgentProvider{responses: []string{"one-shot done"}}
	tool := &AgentTool{
		Provider:   provider,
		Registry:   registry.New(),
		Model:      "parent-model",
		Background: background,
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("inspect", map[string]any{
		"subagent_type": "Explore",
		"name":          "named-explore",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected one-shot Explore success, got %s", result.Content)
	}
	if strings.TrimSpace(result.Content) != "one-shot done" {
		t.Fatalf("expected plain one-shot content, got %q", result.Content)
	}

	background.mu.Lock()
	taskCount := len(background.tasks)
	sessionCount := len(background.sessions)
	background.mu.Unlock()
	if taskCount != 0 || sessionCount != 0 {
		t.Fatalf("one-shot Explore should not register retained/background state, tasks=%d sessions=%d", taskCount, sessionCount)
	}
}

func TestAgentToolOneShotBuiltInSuppressesAccidentalWorktreeIsolation(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	provider := &captureAgentProvider{responses: []string{"one-shot no worktree"}}
	tool := &AgentTool{
		Provider: provider,
		Registry: registry.New(),
		Model:    "parent-model",
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("inspect read-only", map[string]any{
		"subagent_type": "Explore",
		"isolation":     "worktree",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected accidental worktree isolation to be suppressed for one-shot Explore, got %s", result.Content)
	}
	if strings.TrimSpace(result.Content) != "one-shot no worktree" {
		t.Fatalf("expected plain one-shot content, got %q", result.Content)
	}
}

func TestOneShotWorktreeIsolationSuppressionRequiresExplicitWorktreeRequest(t *testing.T) {
	profile := agentProfile{Name: "Explore"}
	if !shouldSuppressOneShotWorktreeIsolation(agentcontract.Input{Prompt: "inspect read-only", Isolation: "worktree"}, profile) {
		t.Fatal("expected one-shot worktree isolation to be suppressed without an explicit worktree request")
	}
	if shouldSuppressOneShotWorktreeIsolation(agentcontract.Input{Prompt: "use a worktree for this read-only check", Isolation: "worktree"}, profile) {
		t.Fatal("expected explicit worktree request to be honored")
	}
	if !shouldSuppressOneShotWorktreeIsolation(agentcontract.Input{Prompt: "omit isolation and no worktree", Isolation: "worktree"}, profile) {
		t.Fatal("expected negated worktree wording to suppress isolation")
	}
}

func TestBuiltInExplorePlanPromptUsesPlatformShellGuidance(t *testing.T) {
	explorePrompt := builtInExplorePrompt()
	planPrompt := builtInPlanPrompt()
	if runtime.GOOS == "windows" {
		for name, prompt := range map[string]string{"Explore": explorePrompt, "Plan": planPrompt} {
			if !strings.Contains(prompt, "Use PowerShell ONLY for read-only operations") {
				t.Fatalf("%s prompt should guide Windows agents toward PowerShell:\n%s", name, prompt)
			}
			if strings.Contains(prompt, "Use Bash ONLY") {
				t.Fatalf("%s prompt should not force Bash on Windows:\n%s", name, prompt)
			}
		}
		return
	}
	for name, prompt := range map[string]string{"Explore": explorePrompt, "Plan": planPrompt} {
		if !strings.Contains(prompt, "Use Bash ONLY for read-only operations") {
			t.Fatalf("%s prompt should keep Bash guidance on non-Windows platforms:\n%s", name, prompt)
		}
	}
}

func TestAgentToolTeammatePlanModeCannotCreateChildApprovalWorkflow(t *testing.T) {
	root := t.TempDir()
	provider := &captureAgentProvider{responses: []string{"done"}}
	tool := &AgentTool{
		Provider: provider,
		Registry: registry.New(),
		Model:    "parent-model",
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "plan"}})
	bundle, err := tool.buildSubAgentLoopWithOptions("agent-team-plan", agentcontract.Input{
		Prompt:      "inspect as teammate",
		Description: "inspect as teammate",
	}, agentLoopOptions{OverrideModel: "parent-model", TeamMember: true})
	if err != nil {
		t.Fatalf("buildSubAgentLoopWithOptions: %v", err)
	}
	if _, err := runAgentQueryLoop(context.Background(), bundle.Loop, bundle.Metadata, "agent-team-plan", "inspect as teammate", nil); err != nil {
		t.Fatalf("runAgentQueryLoop: %v", err)
	}
	if len(provider.params) != 1 {
		t.Fatalf("expected one provider call, got %d", len(provider.params))
	}
	if !strings.Contains(provider.params[0].System, "Permission mode: plan.") ||
		strings.Contains(provider.params[0].System, "lead approval") {
		t.Fatalf("teammate child must inherit plan without creating an approval workflow:\n%s", provider.params[0].System)
	}
	toolNames := agentProviderToolNames(provider)
	if toolNames["ExitPlanMode"] || toolNames["EnterPlanMode"] {
		t.Fatalf("teammate child must not receive permission transition tools, got %#v", toolNames)
	}
}

func TestAgentToolSubagentInheritsParentPromptCacheLineage(t *testing.T) {
	root := t.TempDir()
	childProvider := &captureAgentProvider{responses: []string{"done"}}
	tool := &AgentTool{
		Provider: childProvider,
		Registry: registry.New(),
		Model:    "parent-model",
	}
	tool.SetSessionRuntime(AgentSessionRuntime{
		ToolRuntime: types.ToolRuntimeContext{
			SessionID:      "visible-parent-session",
			ProjectRoot:    root,
			AllowedDirs:    []string{root},
			PermissionMode: "default",
		},
	})
	parentContext := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		SessionID:      "visible-parent-session",
		CacheLineageID: "root-session-cache-lineage",
	})

	bundle, err := tool.buildSubAgentLoopWithOptions("child-agent-id", agentcontract.Input{
		Prompt: "inspect",
	}, agentLoopOptions{Context: parentContext, Profile: &agentProfile{Name: "general-purpose"}})
	if err != nil {
		t.Fatalf("buildSubAgentLoopWithOptions: %v", err)
	}
	defer runAgentCleanup(bundle.Cleanup)
	if _, err := runAgentQueryLoop(context.Background(), bundle.Loop, bundle.Metadata, "child-agent-id", "inspect", nil); err != nil {
		t.Fatalf("runAgentQueryLoop: %v", err)
	}
	if len(childProvider.params) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(childProvider.params))
	}
	if got := childProvider.params[0].PromptCacheKey; got != "root-session-cache-lineage" {
		t.Fatalf("subagent cache lineage = %q, want inherited root lineage", got)
	}
	if bundle.Metadata.CacheLineageID != "root-session-cache-lineage" {
		t.Fatalf("persisted subagent cache lineage = %q", bundle.Metadata.CacheLineageID)
	}
}

func TestAgentToolExplicitTeamNameRequiresTeammateContract(t *testing.T) {
	t.Setenv("LUBAN_CODE_CONFIG_DIR", t.TempDir())
	tool := &AgentTool{
		Provider: &captureAgentProvider{responses: []string{"unused"}},
		Registry: registry.New(),
	}

	noName, err := tool.Execute(context.Background(), agentExecuteInput("spawn worker", map[string]any{
		"team_name": "missing-team",
	}))
	if err != nil {
		t.Fatalf("Execute without name: %v", err)
	}
	if !noName.IsError || noName.Content != toolRuntimeText(i18n.KeyToolAgentTeammateNameRequired) {
		t.Fatalf("expected explicit team_name without name to fail clearly, got %q", noName.Content)
	}

	missingTeam, err := tool.Execute(context.Background(), agentExecuteInput("spawn worker", map[string]any{
		"team_name": "missing-team",
		"name":      "worker",
	}))
	if err != nil {
		t.Fatalf("Execute missing team: %v", err)
	}
	if !missingTeam.IsError || missingTeam.Content != toolRuntimeFormat(i18n.KeyToolAgentTeamMissing, "missing-team") {
		t.Fatalf("expected missing explicit team to fail clearly, got %q", missingTeam.Content)
	}
}

func TestAgentToolMissingPrompt(t *testing.T) {
	tool := &AgentTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{})
	if !result.IsError {
		t.Error("expected error for missing prompt")
	}
}

func TestAgentToolEmptyPrompt(t *testing.T) {
	tool := &AgentTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"prompt":      "",
		"description": "empty prompt",
	})
	if !result.IsError {
		t.Error("expected error for empty prompt")
	}
}

func TestAgentToolMissingDescription(t *testing.T) {
	tool := &AgentTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"prompt": "do something",
	})
	if !result.IsError || result.Content != toolRuntimeText(i18n.KeyToolAgentDescriptionRequired) {
		t.Fatalf("expected error for missing description, got: %#v", result)
	}
}

func TestAgentToolDepthIncrementsOnClone(t *testing.T) {
	parent := &AgentTool{
		Depth:    0,
		MaxDepth: 3,
	}

	child := &AgentTool{
		Depth:    parent.Depth + 1,
		MaxDepth: parent.MaxDepth,
	}
	if child.Depth != 1 {
		t.Errorf("expected child depth 1, got %d", child.Depth)
	}

	grandchild := &AgentTool{
		Depth:    child.Depth + 1,
		MaxDepth: child.MaxDepth,
	}
	if grandchild.Depth != 2 {
		t.Errorf("expected grandchild depth 2, got %d", grandchild.Depth)
	}
}

func TestAgentToolDepthZeroUsesDefault(t *testing.T) {
	tool := &AgentTool{
		Depth:    DefaultMaxAgentDepth,
		MaxDepth: 0, // should default to DefaultMaxAgentDepth
	}
	result, _ := tool.Execute(context.Background(), agentExecuteInput("test", nil))
	if !result.IsError {
		t.Error("expected depth limit error when MaxDepth=0 defaults to DefaultMaxAgentDepth")
	}
}

// Verify Clone exists and works with the registry
func TestRegistryCloneForAgent(t *testing.T) {
	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{})
	reg.Register(toolsearch.NewGlob(nil))

	cloned := reg.Clone()
	if cloned.Count() != 2 {
		t.Errorf("expected 2 tools in clone, got %d", cloned.Count())
	}

	// Overwrite in clone should not affect original
	cloned.Register(toolsearch.NewGrep(nil))
	if cloned.Count() != 3 {
		t.Errorf("expected 3 tools in clone after add, got %d", cloned.Count())
	}
	if reg.Count() != 2 {
		t.Error("original registry should not be affected by clone modifications")
	}
}

func TestAgentToolSchemaExternalIsolationOnlyWorktree(t *testing.T) {
	schema := (&AgentTool{}).Schema()
	isolation, ok := schema.Properties["isolation"].(map[string]any)
	if !ok {
		t.Fatalf("expected isolation schema")
	}
	enum, ok := isolation["enum"].([]string)
	if !ok {
		t.Fatalf("expected isolation enum, got %#v", isolation["enum"])
	}
	if len(enum) != 1 || enum[0] != "worktree" {
		t.Fatalf("expected external isolation enum to expose only worktree, got %#v", enum)
	}
}

func TestAgentToolRejectsCWDWithWorktreeIsolation(t *testing.T) {
	tool := &AgentTool{}
	result, err := tool.Execute(context.Background(), agentExecuteInput("do something", map[string]any{
		"cwd":       t.TempDir(),
		"isolation": "worktree",
	}))
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !result.IsError || result.Content != toolRuntimeText(i18n.KeyToolAgentDeepCWDWorktreeConflict) {
		t.Fatalf("expected cwd/worktree validation error, got: %#v", result)
	}
}

func TestAgentProfileFiltersExploreWriteTools(t *testing.T) {
	profile, err := resolveAgentProfile("Explore", "")
	if err != nil {
		t.Fatalf("resolveAgentProfile: %v", err)
	}
	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{})
	reg.Register(&toolfile.FileWriteTool{})
	reg.Register(&toolfile.FileEditTool{})

	filtered := registryForAgentProfile(reg, profile)
	if filtered.Get("Read") == nil {
		t.Fatal("expected Explore profile to keep Read")
	}
	if filtered.Get("Write") != nil {
		t.Fatal("expected Explore profile to remove Write")
	}
	if filtered.Get("Edit") != nil {
		t.Fatal("expected Explore profile to remove Edit")
	}
}

func TestAgentProfileRegistryInheritsRuntimePolicy(t *testing.T) {
	root := t.TempDir()
	scope := staticAgentRuntimeProvider{runtime: types.ToolRuntimeContext{
		ProjectRoot: root,
		AllowedDirs: []string{root},
		DeniedTools: map[string]bool{"Bash": true},
	}}

	reg := registry.New()
	reg.SetRuntimeContextProvider(scope)
	reg.Register(fakeTool{name: "Bash"})
	reg.Register(fakeTool{name: "Read"})

	filtered := registryForAgentProfile(reg, agentProfile{Name: "general-purpose"})
	if filtered.IsToolEnabled(filtered.Get("Bash")) {
		t.Fatal("subagent registry must preserve the parent Bash deny policy")
	}
	if !filtered.IsToolEnabled(filtered.Get("Read")) {
		t.Fatal("subagent registry unexpectedly disabled an allowed tool")
	}
	if got := filtered.RuntimeContext().ProjectRoot; got != root {
		t.Fatalf("subagent runtime project root = %q, want %q", got, root)
	}
}

func TestAgentProfileRebindsToolSearchToFilteredRegistry(t *testing.T) {
	reg := registry.New()
	reg.Register(fakeTool{name: "Read"})
	reg.Register(fakeTool{name: "TaskCreate"})
	reg.Register(toolsearch.NewToolSearch(reg, nil))

	profile := agentProfile{
		Name:                  "restricted",
		AllowedTools:          lowerToolNameSet("Read", "ToolSearch"),
		AllowedToolsSpecified: true,
		DisallowedTools:       map[string]struct{}{},
	}
	filtered := registryForAgentProfile(reg, profile)
	search := filtered.Get("ToolSearch")
	if search == nil {
		t.Fatal("filtered registry omitted ToolSearch")
	}
	result, err := search.Execute(context.Background(), map[string]any{"query": "select:TaskCreate"})
	if err != nil {
		t.Fatalf("ToolSearch.Execute: %v", err)
	}
	for _, block := range result.ContentBlocks {
		if ref, ok := block.(types.ToolReferenceBlock); ok && ref.ToolName == "TaskCreate" {
			t.Fatal("ToolSearch leaked a tool excluded by the subagent profile")
		}
	}
}

func TestAgentProfileAppliesBaseToolFilters(t *testing.T) {
	t.Setenv("USER_TYPE", "")
	reg := registry.New()
	for _, name := range []string{
		"Agent",
		"TaskOutput",
		"EnterPlanMode",
		"ExitPlanMode",
		"AskUserQuestion",
		"TaskStop",
		"Read",
		"mcp__github__search",
	} {
		reg.Register(fakeTool{name: name})
	}

	filtered := registryForAgentProfile(reg, agentProfile{Name: "general-purpose"})
	for _, blocked := range []string{"Agent", "TaskOutput", "EnterPlanMode", "ExitPlanMode", "AskUserQuestion", "TaskStop"} {
		if filtered.Get(blocked) != nil {
			t.Fatalf("expected base filter to remove %s", blocked)
		}
	}
	for _, allowed := range []string{"Read", "mcp__github__search"} {
		if filtered.Get(allowed) == nil {
			t.Fatalf("expected base filter to keep %s", allowed)
		}
	}

}

func TestAgentToolDoesNotReaddBaseFilteredNestedAgent(t *testing.T) {
	t.Setenv("USER_TYPE", "")
	provider := &captureAgentProvider{responses: []string{"done"}}
	reg := registry.New()
	reg.Register(&AgentTool{})
	reg.Register(&toolfile.FileReadTool{})

	tool := &AgentTool{Provider: provider, Registry: reg}
	result, err := tool.Execute(context.Background(), agentExecuteInput("inspect", nil))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected successful agent run, got: %s", result.Content)
	}
	if len(provider.params) == 0 {
		t.Fatal("expected provider call")
	}
	toolNames := map[string]bool{}
	for _, def := range provider.params[0].Tools {
		toolNames[def.Name] = true
	}
	if toolNames["Agent"] {
		t.Fatalf("expected subagent tool definitions to omit nested Agent, got %#v", toolNames)
	}
	if !toolNames["Read"] {
		t.Fatalf("expected subagent tool definitions to keep Read, got %#v", toolNames)
	}
}

func TestAgentProfileAllowsSynchronousNestedAgentForTeammates(t *testing.T) {
	t.Setenv("USER_TYPE", "")
	reg := registry.New()
	reg.Register(fakeTool{name: "Agent"})
	reg.Register(fakeTool{name: "Read"})

	filtered := registryForAgentProfileWithOptions(reg, agentProfile{Name: "general-purpose"}, agentToolRegistryOptions{
		AllowAgent: true,
	})
	if filtered.Get("Agent") == nil {
		t.Fatal("teammate synchronous registry removed Agent despite teammate delegation contract")
	}

	asyncFiltered := registryForAgentProfileWithOptions(reg, agentProfile{Name: "general-purpose"}, agentToolRegistryOptions{
		AllowAgent: true,
		IsAsync:    true,
	})
	if asyncFiltered.Get("Agent") != nil {
		t.Fatal("background subagent registry unexpectedly retained nested Agent")
	}
}

func TestAgentProfileAsyncToolWhitelist(t *testing.T) {
	t.Setenv("USER_TYPE", "")
	reg := registry.New()
	for _, name := range []string{
		"Read",
		"Bash",
		"Write",
		"ToolSearch",
		"TaskCreate",
		"SendMessage",
		"Agent",
	} {
		reg.Register(fakeTool{name: name})
	}

	filtered := registryForAgentProfileWithOptions(reg, agentProfile{Name: "async"}, agentToolRegistryOptions{IsAsync: true})
	for _, allowed := range []string{"Read", "Bash", "Write", "ToolSearch"} {
		if filtered.Get(allowed) == nil {
			t.Fatalf("expected async agent to keep %s", allowed)
		}
	}
	for _, blocked := range []string{"TaskCreate", "SendMessage", "Agent"} {
		if filtered.Get(blocked) != nil {
			t.Fatalf("expected async agent to remove %s", blocked)
		}
	}
}

func TestAgentProfileToolSpecsParsePermissionRules(t *testing.T) {
	set := toolNameSetFromYAML([]any{
		"Bash(git status)",
		"Agent(worker, researcher)",
		"TaskOutput",
	})
	for _, expected := range []string{"bash", "agent", "taskoutput"} {
		if _, ok := set[expected]; !ok {
			t.Fatalf("expected parsed tool spec %q in %#v", expected, set)
		}
	}
	if wildcard := toolNameSetFromYAML("*"); wildcard != nil {
		t.Fatalf("expected wildcard tools spec to mean unrestricted tools, got %#v", wildcard)
	}
	rules := toolPermissionRulesFromYAML(`Agent(worker, researcher), Bash(echo a,b), Read`)
	if len(rules) != 3 {
		t.Fatalf("expected parser to preserve commas inside rule contents, got %#v", rules)
	}
	if rules[0].ToolName != "agent" || rules[0].RuleContent != "worker, researcher" {
		t.Fatalf("unexpected Agent(...) rule parse: %#v", rules[0])
	}
	if rules[1].ToolName != "bash" || rules[1].RuleContent != "echo a,b" {
		t.Fatalf("unexpected Bash(...) rule parse: %#v", rules[1])
	}
	spaceRules := toolPermissionRulesFromYAML(`Read Bash(git status) Write`)
	if len(spaceRules) != 3 {
		t.Fatalf("expected parser to split spaces outside rule contents, got %#v", spaceRules)
	}
	if spaceRules[1].ToolName != "bash" || spaceRules[1].RuleContent != "git status" {
		t.Fatalf("expected Bash(...) space-preserving rule content, got %#v", spaceRules[1])
	}
}

func TestBuiltInAgentProfilesAlignKeyDefaults(t *testing.T) {
	t.Setenv("USER_TYPE", "")
	assertPromptContains := func(name string, prompt string, fragments ...string) {
		t.Helper()
		for _, fragment := range fragments {
			if !strings.Contains(prompt, fragment) {
				t.Fatalf("%s prompt missing %q\nprompt:\n%s", name, fragment, prompt)
			}
		}
	}

	names := builtinAgentNames()
	expectedNames := []string{"general-purpose", "statusline-setup", "Explore", "Plan", "verification"}
	if strings.Join(names, ",") != strings.Join(expectedNames, ",") {
		t.Fatalf("unexpected built-in agent order: got %v want %v", names, expectedNames)
	}

	general, err := resolveAgentProfile("general-purpose", "")
	if err != nil {
		t.Fatalf("resolve general-purpose: %v", err)
	}
	if !strings.Contains(general.WhenToUse, "General-purpose agent") {
		t.Fatalf("expected general-purpose whenToUse text, got %q", general.WhenToUse)
	}
	assertPromptContains("general-purpose", general.SystemPrefix,
		"Complete the task fully",
		"NEVER proactively create documentation files (*.md) or README files",
	)

	statusline, err := resolveAgentProfile("statusline-setup", "")
	if err != nil {
		t.Fatalf("resolve statusline: %v", err)
	}
	if statusline.Model != "inherit" {
		t.Fatalf("expected statusline model inherit, got %#v", statusline)
	}
	if !statusline.AllowedToolsSpecified {
		t.Fatalf("expected statusline tools allowlist to be explicit")
	}
	if statusline.Color != "orange" {
		t.Fatalf("expected statusline color orange, got %#v", statusline)
	}
	assertPromptContains("statusline-setup", statusline.SystemPrefix,
		"statusLine command will receive the following JSON input via stdin",
		"context_window",
		"If ~/.luban-code/settings.json is a symlink, update the target file instead",
	)
	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{})
	reg.Register(&toolfile.FileEditTool{})
	reg.Register(&toolfile.FileWriteTool{})
	statuslineTools := registryForAgentProfile(reg, statusline)
	if statuslineTools.Get("Read") == nil || statuslineTools.Get("Edit") == nil {
		t.Fatalf("expected statusline to keep Read/Edit")
	}
	if statuslineTools.Get("Write") != nil {
		t.Fatalf("expected statusline to omit Write")
	}

	verification, err := resolveAgentProfile("verification", "")
	if err != nil {
		t.Fatalf("resolve verification: %v", err)
	}
	if !verification.Background {
		t.Fatalf("expected verification agent to force background")
	}
	if verification.Model != "inherit" || verification.Color != "red" {
		t.Fatalf("expected verification model/color defaults, got %#v", verification)
	}
	if _, blocked := verification.DisallowedTools["write"]; !blocked {
		t.Fatalf("expected verification agent to disallow Write")
	}
	assertPromptContains("verification", verification.SystemPrefix,
		"Your report must include at least one adversarial probe you ran",
		"**Command run:**",
		"VERDICT: PASS",
		"VERDICT: PARTIAL",
	)

	explore, err := resolveAgentProfile("Explore", "")
	if err != nil {
		t.Fatalf("resolve explore: %v", err)
	}
	if explore.Model != "inherit" {
		t.Fatalf("expected external Explore model inherit, got %#v", explore)
	}
	assertPromptContains("Explore", explore.SystemPrefix,
		"=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===",
		"Creating temporary files anywhere, including /tmp",
		"spawn multiple parallel tool calls",
	)

	plan, err := resolveAgentProfile("Plan", "")
	if err != nil {
		t.Fatalf("resolve plan: %v", err)
	}
	if plan.Model != "inherit" {
		t.Fatalf("expected Plan model inherit, got %#v", plan)
	}
	assertPromptContains("Plan", plan.SystemPrefix,
		"=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===",
		"### Critical Files for Implementation",
		"REMEMBER: You can ONLY explore and plan",
	)
}

func TestAgentToolNonInteractiveCanDisableBuiltInAgents(t *testing.T) {
	t.Setenv("LUBAN_AGENT_SDK_DISABLE_BUILTIN_AGENTS", "1")
	t.Setenv("LUBAN_CODE_CONFIG_DIR", t.TempDir())
	tool := &AgentTool{NonInteractive: true}

	description := tool.Description()
	if strings.Contains(description, "- general-purpose:") || strings.Contains(description, "- Explore:") {
		t.Fatalf("expected noninteractive disabled built-ins to be hidden, got:\n%s", description)
	}
	if _, err := tool.resolveProfileForInput(agentcontract.Input{SubagentType: "Explore"}); err == nil {
		t.Fatalf("expected disabled built-in agent to fail resolution")
	}
}

func TestAgentToolBuiltInDisableDoesNotBlockInteractiveOrCustomAgents(t *testing.T) {
	t.Setenv("LUBAN_AGENT_SDK_DISABLE_BUILTIN_AGENTS", "true")
	configDir := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("LUBAN_CODE_CONFIG_DIR", configDir)
	agentsDir := filepath.Join(cwd, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: sdk-custom
description: SDK custom agent
---
Loaded custom SDK agent.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "sdk-custom.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write custom agent: %v", err)
	}

	interactive := &AgentTool{}
	if _, err := interactive.resolveProfileForInput(agentcontract.Input{SubagentType: "Explore"}); err != nil {
		t.Fatalf("interactive tool should keep built-ins despite env toggle: %v", err)
	}
	nonInteractive := &AgentTool{NonInteractive: true}
	profile, err := nonInteractive.resolveProfileForInput(agentcontract.Input{SubagentType: "sdk-custom", CWD: cwd})
	if err != nil {
		t.Fatalf("noninteractive tool should still resolve custom agent: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "custom SDK agent") {
		t.Fatalf("unexpected custom profile: %#v", profile)
	}
}

func TestAgentToolUnknownSubagentTypeFails(t *testing.T) {
	tool := &AgentTool{
		Provider: &mockProvider{responses: []string{"unused"}},
		Registry: registry.New(),
	}
	result, err := tool.Execute(context.Background(), agentExecuteInput("do something", map[string]any{
		"subagent_type": "does-not-exist",
	}))
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, `"does-not-exist"`) {
		t.Fatalf("expected unknown subagent_type error, got: %#v", result)
	}
}

func TestAgentToolLoadsCustomAgentProfile(t *testing.T) {
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: reviewer
description: Reviews code changes
tools:
  - Read
disallowedTools:
  - Write
model: haiku
maxTurns: 7
effort: high
skills:
  - code-review
mcpServers:
  - filesystem
requiredMcpServers:
  - github
initialPrompt: Read this before the task.
background: true
memory: project
color: blue
---
You review code with focused findings.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	profile, err := resolveAgentProfile("reviewer", base)
	if err != nil {
		t.Fatalf("resolveAgentProfile: %v", err)
	}
	if profile.Name != "reviewer" || profile.Model != "haiku" || profile.MaxTurns != 7 {
		t.Fatalf("unexpected custom profile: %#v", profile)
	}
	if profile.ReasoningEffort != "high" {
		t.Fatalf("expected custom effort in profile: %#v", profile)
	}
	if profile.WhenToUse != "Reviews code changes" || !profile.AllowedToolsSpecified {
		t.Fatalf("expected custom description and explicit tools flag in profile: %#v", profile)
	}
	if len(profile.Skills) != 1 || profile.Skills[0] != "code-review" {
		t.Fatalf("expected custom skills in profile: %#v", profile.Skills)
	}
	if len(profile.MCPServers) != 1 || profile.MCPServers[0] != "filesystem" {
		t.Fatalf("expected custom mcp servers in profile: %#v", profile.MCPServers)
	}
	if len(profile.RequiredMCPServers) != 1 || profile.RequiredMCPServers[0] != "github" {
		t.Fatalf("expected custom required mcp servers in profile: %#v", profile.RequiredMCPServers)
	}
	if profile.InitialPrompt != "Read this before the task." || !profile.Background || profile.Memory != "project" || profile.Color != "blue" {
		t.Fatalf("expected custom defaults in profile: %#v", profile)
	}
	if !strings.Contains(profile.SystemPrefix, "focused findings") {
		t.Fatalf("expected custom prompt in system prefix, got: %q", profile.SystemPrefix)
	}

	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{})
	reg.Register(&toolfile.FileEditTool{})
	reg.Register(&toolfile.FileWriteTool{})
	filtered := registryForAgentProfile(reg, profile)
	if filtered.Get("Read") == nil {
		t.Fatal("expected custom tools list to keep Read")
	}
	if filtered.Get("Edit") == nil {
		t.Fatal("expected memory-enabled custom tools list to include Edit")
	}
	if filtered.Get("Write") != nil {
		t.Fatal("expected disallowed Write to override memory tool injection")
	}
}

func TestCustomAgentRejectsUnknownFrontmatterField(t *testing.T) {
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: invalid
description: Contains an unknown field
unknownField: value
---
Do the task.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "invalid.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write unsafe agent: %v", err)
	}

	_, err := resolveAgentProfile("invalid", base)
	if err == nil || !strings.Contains(err.Error(), "unknownField") {
		t.Fatalf("expected strict frontmatter error, got %v", err)
	}
}

func TestAgentToolMarkdownFrontmatterMatchesOriginalStrictFields(t *testing.T) {
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}

	invalidBody := `---
name: strict-invalid
description: Invalid strict fields
background: "TRUE"
memory: Project
color: ultraviolet
mcpServers: filesystem
---
Prompt.
`
	invalidPath := filepath.Join(agentsDir, "strict-invalid.md")
	if err := os.WriteFile(invalidPath, []byte(invalidBody), 0o644); err != nil {
		t.Fatalf("write invalid agent: %v", err)
	}
	invalid, ok, err := parseCustomAgentProfileFile(invalidPath, base)
	if err != nil || !ok {
		t.Fatalf("parse invalid strict agent ok=%v err=%v", ok, err)
	}
	if invalid.Background || invalid.Memory != "" || invalid.Color != "" || len(invalid.MCPServers) != 0 || len(invalid.MCPServerConfigs) != 0 {
		t.Fatalf("expected invalid strict fields to be ignored like TS loader, got %#v", invalid)
	}

	validBody := `---
name: strict-valid
description: Valid strict fields
background: "true"
memory: project
color: cyan
mcpServers:
  - filesystem
---
Prompt.
`
	validPath := filepath.Join(agentsDir, "strict-valid.md")
	if err := os.WriteFile(validPath, []byte(validBody), 0o644); err != nil {
		t.Fatalf("write valid agent: %v", err)
	}
	valid, ok, err := parseCustomAgentProfileFile(validPath, base)
	if err != nil || !ok {
		t.Fatalf("parse valid strict agent ok=%v err=%v", ok, err)
	}
	if !valid.Background || valid.Memory != "project" || valid.Color != "cyan" || len(valid.MCPServers) != 1 || valid.MCPServers[0] != "filesystem" {
		t.Fatalf("expected valid strict fields to load, got %#v", valid)
	}
}

func TestAgentToolMarkdownModelInheritNormalizes(t *testing.T) {
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: inherit-model
description: Inherit model
model: Inherit
---
Prompt.
`
	path := filepath.Join(agentsDir, "inherit-model.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write inherit model agent: %v", err)
	}
	profile, ok, err := parseCustomAgentProfileFile(path, base)
	if err != nil || !ok {
		t.Fatalf("parse inherit model agent ok=%v err=%v", ok, err)
	}
	if profile.Model != "inherit" {
		t.Fatalf("expected model inherit to normalize and inherit parent model, got %#v", profile)
	}
}

func TestAgentToolLoadsCustomAgentWithExplicitEmptyTools(t *testing.T) {
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: no-tools
description: "Line one\nLine two"
tools: []
---
Answer without tools.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "no-tools.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	profile, err := resolveAgentProfile("no-tools", base)
	if err != nil {
		t.Fatalf("resolveAgentProfile: %v", err)
	}
	if !profile.AllowedToolsSpecified || len(profile.AllowedTools) != 0 {
		t.Fatalf("expected explicit empty tools list, got %#v", profile)
	}
	if profile.WhenToUse != "Line one\nLine two" {
		t.Fatalf("expected escaped newline in whenToUse to be expanded, got %q", profile.WhenToUse)
	}
	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{})
	if filtered := registryForAgentProfile(reg, profile); filtered.Get("Read") != nil {
		t.Fatalf("explicit empty tools should deny Read")
	}
}

func TestAgentToolDescriptionListsAvailableAgents(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "")
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: reviewer
description: "Review code\nwith care"
tools: Read
---
Review code.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(base); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldCWD)
	})

	description := (&AgentTool{}).Description()
	for _, expected := range []string{
		"Available agent types",
		"general-purpose",
		"General-purpose agent",
		toolRuntimeFormat(i18n.KeyToolAgentProfileLine, "reviewer", "Review code\nwith care", "Read"),
	} {
		if !strings.Contains(description, expected) {
			t.Fatalf("expected Agent description to contain %q\n%s", expected, description)
		}
	}
}

func TestAgentToolDescriptionIncludesOriginalUsageGuidance(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "")
	description := (&AgentTool{}).Description()
	for _, expected := range []string{
		"When NOT to use the Agent tool:",
		"Always include a short description (3-5 words)",
		"not visible to the user",
		"do NOT sleep, poll, or proactively check",
		"SendMessage with the agent's ID or name",
		"full context preserved",
		"single assistant message with multiple Agent tool calls",
		`isolation="worktree"`,
		"Do not delegate understanding",
	} {
		if !strings.Contains(description, expected) {
			t.Fatalf("expected Agent description to contain %q\n%s", expected, description)
		}
	}
}

func TestAgentToolDescriptionIncludesForkGuidanceWhenEnabled(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "1")
	description := (&AgentTool{}).Description()
	for _, expected := range []string{
		"omit it to fork yourself",
		"A fork inherits your full conversation context",
		"When to fork:",
		"Do not read or tail a fork's output file",
		"never fabricate or predict fork results",
	} {
		if !strings.Contains(description, expected) {
			t.Fatalf("expected fork Agent description to contain %q\n%s", expected, description)
		}
	}
	if strings.Contains(description, "run_in_background=true") {
		t.Fatalf("expected fork Agent description to omit background guidance\n%s", description)
	}
}

func TestAgentToolDescriptionFiltersUnavailableMCPAgents(t *testing.T) {
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "")
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	agentBody := `---
name: needs-github
description: Uses GitHub MCP
requiredMcpServers:
  - github
---
Inspect GitHub data.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "needs-github.md"), []byte(agentBody), 0o644); err != nil {
		t.Fatalf("write required MCP agent: %v", err)
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(base); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldCWD)
	})

	descriptionWithoutMCP := (&AgentTool{Registry: registry.New()}).Description()
	if strings.Contains(descriptionWithoutMCP, "- needs-github:") {
		t.Fatalf("expected description to hide MCP-gated agent without available MCP tools\n%s", descriptionWithoutMCP)
	}

	reg := registry.New()
	reg.Register(fakeTool{name: "mcp__github__search"})
	descriptionWithMCP := (&AgentTool{Registry: reg}).Description()
	wantProfile := toolRuntimeFormat(
		i18n.KeyToolAgentProfileLine,
		"needs-github",
		"Uses GitHub MCP",
		toolRuntimeText(i18n.KeyToolAgentProfileAllTools),
	)
	if !strings.Contains(descriptionWithMCP, wantProfile) {
		t.Fatalf("expected description to include MCP-gated agent when MCP tools are available\n%s", descriptionWithMCP)
	}
}

func TestAgentToolLoadsNestedCustomAgentProfile(t *testing.T) {
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents", "review", "security")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir nested agents dir: %v", err)
	}
	body := `---
name: security-reviewer
description: Reviews security changes
---
Review nested security-sensitive changes.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "security-reviewer.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write nested agent file: %v", err)
	}

	profile, err := resolveAgentProfile("security-reviewer", base)
	if err != nil {
		t.Fatalf("resolve nested profile: %v", err)
	}
	if profile.Name != "security-reviewer" || !strings.Contains(profile.SystemPrefix, "nested security-sensitive") {
		t.Fatalf("unexpected nested profile: %#v", profile)
	}
}

func TestAgentToolLoadsCustomAgentThroughSymlinkedDirectory(t *testing.T) {
	base := t.TempDir()
	targetDir := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: linked-agent
description: Linked agent
---
Loaded through a symlinked directory.
`
	if err := os.WriteFile(filepath.Join(targetDir, "linked-agent.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write linked agent file: %v", err)
	}
	linkPath := filepath.Join(agentsDir, "linked")
	if err := os.Symlink(targetDir, linkPath); err != nil {
		t.Skipf("directory symlinks are not available: %v", err)
	}

	profile, err := resolveAgentProfile("linked-agent", base)
	if err != nil {
		t.Fatalf("resolve linked profile: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "symlinked directory") {
		t.Fatalf("expected symlinked agent profile, got %#v", profile)
	}
}

func TestAgentToolIgnoresInvalidOptionalCustomAgentFields(t *testing.T) {
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: forgiving
description: Invalid optional fields should not hide the agent
effort: xhigh
memory: invalid-scope
isolation: container
---
Still load this agent.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "forgiving.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write forgiving agent file: %v", err)
	}

	profile, err := resolveAgentProfile("forgiving", base)
	if err != nil {
		t.Fatalf("resolve forgiving profile: %v", err)
	}
	if profile.Name != "forgiving" || !strings.Contains(profile.SystemPrefix, "Still load") {
		t.Fatalf("unexpected forgiving profile: %#v", profile)
	}
	if profile.ReasoningEffort != "" || profile.Memory != "" || profile.Isolation != "" {
		t.Fatalf("invalid optional fields should be ignored, got %#v", profile)
	}
}

func TestAgentToolIgnoresInvalidMarkdownMCPAndHooksItems(t *testing.T) {
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: tolerant
description: Invalid trust fields should not hide markdown agents
mcpServers:
  - valid-server
  - broken:
      args:
        - missing-command
hooks: not-a-valid-hooks-shape
---
Still load with valid MCP entries.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "tolerant.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write tolerant agent file: %v", err)
	}

	profile, err := resolveAgentProfile("tolerant", base)
	if err != nil {
		t.Fatalf("resolve tolerant profile: %v", err)
	}
	if profile.Name != "tolerant" || !strings.Contains(profile.SystemPrefix, "Still load") {
		t.Fatalf("unexpected tolerant profile: %#v", profile)
	}
	if got := strings.Join(profile.MCPServers, ","); got != "valid-server" {
		t.Fatalf("expected valid MCP server retained and invalid item ignored, got %#v", profile.MCPServers)
	}
	if profile.HookRunner != nil {
		t.Fatalf("expected invalid markdown hooks to be ignored, got %#v", profile.HookRunner)
	}
}

func TestAgentToolParsesNumericCustomAgentEffort(t *testing.T) {
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: numeric-effort
description: Uses numeric effort
effort: 75
---
Use numeric effort.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "numeric-effort.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write numeric effort agent file: %v", err)
	}

	profile, err := resolveAgentProfile("numeric-effort", base)
	if err != nil {
		t.Fatalf("resolve numeric effort profile: %v", err)
	}
	if profile.ReasoningEffort != "75" {
		t.Fatalf("expected numeric effort to be preserved, got %#v", profile)
	}
}

func TestAgentReasoningEffortParsing(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "level", value: "HIGH", want: "high"},
		{name: "max level", value: "max", want: "max"},
		{name: "numeric string", value: "42", want: "42"},
		{name: "parse int prefix", value: "-7extra", want: "-7"},
		{name: "json zero", value: float64(0), want: "0"},
		{name: "invalid alias", value: "xhigh", want: ""},
		{name: "fractional number", value: float64(1.5), want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reasoningEffortFromValue(tt.value); got != tt.want {
				t.Fatalf("reasoningEffortFromValue(%#v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestAgentToolCustomGeneralPurposeOverridesDefault(t *testing.T) {
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: general-purpose
description: Project default agent
---
Project-specific default agent prompt.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "general-purpose.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	profile, err := resolveAgentProfile("", base)
	if err != nil {
		t.Fatalf("resolve default profile: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "Project-specific default") {
		t.Fatalf("expected custom general-purpose override, got: %q", profile.SystemPrefix)
	}
}

func TestAgentToolSimpleModeSkipsCustomAgentProfiles(t *testing.T) {
	t.Setenv("LUBAN_CODE_SIMPLE", "true")
	base := t.TempDir()
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: general-purpose
description: Project default agent
---
Project-specific default agent prompt.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "general-purpose.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	profile, err := resolveAgentProfile("", base)
	if err != nil {
		t.Fatalf("resolve default profile: %v", err)
	}
	if strings.Contains(profile.SystemPrefix, "Project-specific default") {
		t.Fatalf("expected simple mode to use built-in default, got: %q", profile.SystemPrefix)
	}
}

func TestAgentToolLoadsUserAgentsFromConfigDir(t *testing.T) {
	configDir := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("LUBAN_CODE_CONFIG_DIR", configDir)
	agentsDir := filepath.Join(configDir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir config agents dir: %v", err)
	}
	body := `---
name: config-agent
description: Config home agent
---
Loaded from LUBAN_CODE_CONFIG_DIR.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "config-agent.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write config agent file: %v", err)
	}

	profile, err := resolveAgentProfile("config-agent", cwd)
	if err != nil {
		t.Fatalf("resolve config profile: %v", err)
	}
	if profile.Name != "config-agent" || !strings.Contains(profile.SystemPrefix, "LUBAN_CODE_CONFIG_DIR") {
		t.Fatalf("unexpected config profile: %#v", profile)
	}
}

func TestAgentToolProjectAgentOverridesUserAgent(t *testing.T) {
	configDir := t.TempDir()
	base := t.TempDir()
	t.Setenv("LUBAN_CODE_CONFIG_DIR", configDir)
	userAgentsDir := filepath.Join(configDir, "agents")
	projectAgentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(userAgentsDir, 0o755); err != nil {
		t.Fatalf("mkdir user agents dir: %v", err)
	}
	if err := os.MkdirAll(projectAgentsDir, 0o755); err != nil {
		t.Fatalf("mkdir project agents dir: %v", err)
	}
	userBody := `---
name: shared-agent
description: User agent
---
Loaded from user settings.
`
	projectBody := `---
name: shared-agent
description: Project agent
---
Loaded from project settings.
`
	if err := os.WriteFile(filepath.Join(userAgentsDir, "shared-agent.md"), []byte(userBody), 0o644); err != nil {
		t.Fatalf("write user agent file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectAgentsDir, "shared-agent.md"), []byte(projectBody), 0o644); err != nil {
		t.Fatalf("write project agent file: %v", err)
	}

	profile, err := resolveAgentProfile("shared-agent", base)
	if err != nil {
		t.Fatalf("resolve shared profile: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "project settings") {
		t.Fatalf("expected project agent to override user agent, got %#v", profile)
	}
}

func TestAgentToolParentProjectAgentOverridesChildProjectAgent(t *testing.T) {
	base := t.TempDir()
	child := filepath.Join(base, "child")
	parentAgentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	childAgentsDir := filepath.Join(child, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(parentAgentsDir, 0o755); err != nil {
		t.Fatalf("mkdir parent agents dir: %v", err)
	}
	if err := os.MkdirAll(childAgentsDir, 0o755); err != nil {
		t.Fatalf("mkdir child agents dir: %v", err)
	}
	parentBody := `---
name: layered-agent
description: Parent project agent
---
Loaded from parent project settings.
`
	childBody := `---
name: layered-agent
description: Child project agent
---
Loaded from child project settings.
`
	if err := os.WriteFile(filepath.Join(parentAgentsDir, "layered-agent.md"), []byte(parentBody), 0o644); err != nil {
		t.Fatalf("write parent agent file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(childAgentsDir, "layered-agent.md"), []byte(childBody), 0o644); err != nil {
		t.Fatalf("write child agent file: %v", err)
	}

	profile, err := resolveAgentProfile("layered-agent", child)
	if err != nil {
		t.Fatalf("resolve layered profile: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "parent project settings") {
		t.Fatalf("expected parent project agent to override child project agent, got %#v", profile)
	}
}

func TestAgentToolManagedAgentOverridesProjectAgent(t *testing.T) {
	managedDir := t.TempDir()
	base := t.TempDir()
	t.Setenv("USER_TYPE", "ant")
	t.Setenv("LUBAN_CODE_MANAGED_SETTINGS_PATH", managedDir)
	projectAgentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	managedAgentsDir := filepath.Join(managedDir, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(projectAgentsDir, 0o755); err != nil {
		t.Fatalf("mkdir project agents dir: %v", err)
	}
	if err := os.MkdirAll(managedAgentsDir, 0o755); err != nil {
		t.Fatalf("mkdir managed agents dir: %v", err)
	}
	projectBody := `---
name: policy-agent
description: Project agent
---
Loaded from project settings.
`
	managedBody := `---
name: policy-agent
description: Managed agent
---
Loaded from managed settings.
`
	if err := os.WriteFile(filepath.Join(projectAgentsDir, "policy-agent.md"), []byte(projectBody), 0o644); err != nil {
		t.Fatalf("write project agent file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(managedAgentsDir, "policy-agent.md"), []byte(managedBody), 0o644); err != nil {
		t.Fatalf("write managed agent file: %v", err)
	}

	profile, err := resolveAgentProfile("policy-agent", base)
	if err != nil {
		t.Fatalf("resolve policy profile: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "managed settings") {
		t.Fatalf("expected managed agent to override project agent, got %#v", profile)
	}
}

func TestAgentToolInlineAgentOverridesProjectAgent(t *testing.T) {
	base := t.TempDir()
	projectAgentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(projectAgentsDir, 0o755); err != nil {
		t.Fatalf("mkdir project agents dir: %v", err)
	}
	projectBody := `---
name: flag-agent
description: Project agent
---
Loaded from project settings.
`
	if err := os.WriteFile(filepath.Join(projectAgentsDir, "flag-agent.md"), []byte(projectBody), 0o644); err != nil {
		t.Fatalf("write project agent file: %v", err)
	}
	tool := &AgentTool{
		InlineProfiles: map[string]agentProfile{
			"flag-agent": {
				Name:         "flag-agent",
				WhenToUse:    "Inline agent",
				SystemPrefix: "Loaded from inline flag.",
			},
		},
	}

	profile, err := tool.resolveProfileForInput(agentcontract.Input{SubagentType: "flag-agent", CWD: base})
	if err != nil {
		t.Fatalf("resolve inline profile: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "inline flag") {
		t.Fatalf("expected inline agent to override project agent, got %#v", profile)
	}
}

func TestAgentToolManagedAgentOverridesInlineAgent(t *testing.T) {
	managedDir := t.TempDir()
	base := t.TempDir()
	t.Setenv("USER_TYPE", "ant")
	t.Setenv("LUBAN_CODE_MANAGED_SETTINGS_PATH", managedDir)
	managedAgentsDir := filepath.Join(managedDir, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(managedAgentsDir, 0o755); err != nil {
		t.Fatalf("mkdir managed agents dir: %v", err)
	}
	managedBody := `---
name: policy-agent
description: Managed agent
---
Loaded from managed settings.
`
	if err := os.WriteFile(filepath.Join(managedAgentsDir, "policy-agent.md"), []byte(managedBody), 0o644); err != nil {
		t.Fatalf("write managed agent file: %v", err)
	}
	tool := &AgentTool{
		InlineProfiles: map[string]agentProfile{
			"policy-agent": {
				Name:         "policy-agent",
				WhenToUse:    "Inline agent",
				SystemPrefix: "Loaded from inline flag.",
			},
		},
	}

	profile, err := tool.resolveProfileForInput(agentcontract.Input{SubagentType: "policy-agent", CWD: base})
	if err != nil {
		t.Fatalf("resolve policy profile: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "managed settings") {
		t.Fatalf("expected managed agent to override inline agent, got %#v", profile)
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(base); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldCWD) //nolint:errcheck
	description := tool.Description()
	allTools := toolRuntimeText(i18n.KeyToolAgentProfileAllTools)
	managedLine := toolRuntimeFormat(i18n.KeyToolAgentProfileLine, "policy-agent", "Managed agent", allTools)
	inlineLine := toolRuntimeFormat(i18n.KeyToolAgentProfileLine, "policy-agent", "Inline agent", allTools)
	if !strings.Contains(description, managedLine) || strings.Contains(description, inlineLine) {
		t.Fatalf("expected description to show managed override, got:\n%s", description)
	}
}

func TestAgentToolWorktreeFallsBackToMainRepoAgents(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	configDir := t.TempDir()
	t.Setenv("LUBAN_CODE_CONFIG_DIR", configDir)
	t.Setenv("USER_TYPE", "")
	repo := t.TempDir()
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "config", "user.email", "test@example.com")
	runGitCommand(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runGitCommand(t, repo, "add", "README.md")
	runGitCommand(t, repo, "commit", "-m", "initial")

	mainAgentsDir := filepath.Join(repo, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(mainAgentsDir, 0o755); err != nil {
		t.Fatalf("mkdir main agents dir: %v", err)
	}
	body := `---
name: main-only-agent
description: Main repo agent
---
Loaded from main repository agents.
`
	if err := os.WriteFile(filepath.Join(mainAgentsDir, "main-only-agent.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write main agent: %v", err)
	}

	worktreeParent := t.TempDir()
	worktreePath := filepath.Join(worktreeParent, "linked")
	branch := "agent-fallback-test"
	runGitCommand(t, repo, "worktree", "add", worktreePath, "-b", branch, "HEAD")
	t.Cleanup(func() {
		_, _ = gitutil.Run(repo, "worktree", "remove", worktreePath, "--force")
		_, _ = gitutil.Run(repo, "branch", "-D", branch)
	})
	if _, err := os.Stat(filepath.Join(worktreePath, brand.ConfigDirName, "agents")); !os.IsNotExist(err) {
		t.Fatalf("expected worktree root to lack .luban-code/agents, stat err=%v", err)
	}

	profile, err := resolveAgentProfile("main-only-agent", worktreePath)
	if err != nil {
		t.Fatalf("resolve main repo fallback profile: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "main repository agents") {
		t.Fatalf("expected worktree to load main repo agent fallback, got %#v", profile)
	}
}

func TestAgentUserMemoryUsesConfigDir(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LUBAN_CODE_CONFIG_DIR", configDir)
	t.Setenv("LUBAN_CODE_REMOTE_MEMORY_DIR", "")
	memoryDir, err := agentMemoryDir("memory:agent", "user", t.TempDir())
	if err != nil {
		t.Fatalf("agentMemoryDir: %v", err)
	}
	expected := filepath.Join(configDir, "agent-memory", "memory-agent")
	if memoryDir != expected {
		t.Fatalf("expected user memory under LUBAN_CODE_CONFIG_DIR, got %q want %q", memoryDir, expected)
	}
}

func TestAgentMemoryUsesRemoteMemoryDir(t *testing.T) {
	remoteDir := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("LUBAN_CODE_REMOTE_MEMORY_DIR", remoteDir)
	t.Setenv("LUBAN_CODE_DISABLE_AUTO_MEMORY", "")
	t.Setenv("LUBAN_CODE_REMOTE", "")
	t.Setenv("LUBAN_CODE_SIMPLE", "")

	userDir, err := agentMemoryDir("memory:agent", "user", cwd)
	if err != nil {
		t.Fatalf("agentMemoryDir user: %v", err)
	}
	if expected := filepath.Join(remoteDir, "agent-memory", "memory-agent"); userDir != expected {
		t.Fatalf("expected remote user memory dir %q, got %q", expected, userDir)
	}

	localDir, err := agentMemoryDir("memory:agent", "local", cwd)
	if err != nil {
		t.Fatalf("agentMemoryDir local: %v", err)
	}
	expectedLocal := filepath.Join(remoteDir, "projects", sanitizeMemoryProjectPath(agentMemoryCanonicalProjectRoot(cwd)), "agent-memory-local", "memory-agent")
	if localDir != expectedLocal {
		t.Fatalf("expected remote local memory dir %q, got %q", expectedLocal, localDir)
	}
}

func TestAgentToolLoadsCustomAgentMCPConfigsAndHooks(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LUBAN_AGENT_TRUSTED_DIRS", base)
	agentsDir := filepath.Join(base, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	body := `---
name: mcp-hooked
description: Uses inline MCP and hooks
mcpServers:
  - filesystem
  - local-tools:
      command: node
      args:
        - server.js
      env:
        TOKEN: test-token
hooks:
  PreToolUse:
    - matcher: Read
      hooks:
        - type: command
          command: echo hooked
  Stop:
    - matcher: mcp-hooked
      hooks:
        - type: command
          command: echo stop
---
Use MCP and hooks.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "mcp-hooked.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	profile, err := resolveAgentProfile("mcp-hooked", base)
	if err != nil {
		t.Fatalf("resolveAgentProfile: %v", err)
	}
	if got := strings.Join(profile.MCPServers, ","); !strings.Contains(got, "filesystem") || !strings.Contains(got, "local-tools") {
		t.Fatalf("expected referenced and inline MCP servers, got %#v", profile.MCPServers)
	}
	cfg, ok := profile.MCPServerConfigs["local-tools"]
	if !ok {
		t.Fatalf("expected inline MCP config, got %#v", profile.MCPServerConfigs)
	}
	if cfg.Command != "node" || len(cfg.Args) != 1 || cfg.Args[0] != "server.js" || cfg.Env["TOKEN"] != "test-token" {
		t.Fatalf("unexpected inline MCP config: %#v", cfg)
	}
	if profile.HookRunner == nil || !profile.HookRunner.HasHooks(hooks.HookPreToolUse) {
		t.Fatalf("expected frontmatter PreToolUse hook")
	}
	if !profile.HookRunner.HasHooks(hooks.HookSubagentStop) || profile.HookRunner.HasHooks(hooks.HookStop) {
		t.Fatalf("expected agent frontmatter Stop hooks to map to SubagentStop")
	}
}

func TestAgentToolLoadsInlineJSONAgentProfile(t *testing.T) {
	raw := `{
  "json-reviewer": {
    "description": "Reviews JSON configured work",
    "prompt": "Review carefully.",
    "tools": ["Read", "Bash(git diff)"],
    "disallowedTools": ["Write"],
    "model": "haiku",
    "effort": "high",
	    "mcpServers": [
      "github",
      {"local": {"command": "node", "args": ["server.js"], "env": {"TOKEN": "x"}}}
    ],
    "hooks": {
      "Stop": [
        {"matcher": "json-reviewer", "hooks": [{"type": "command", "command": "echo stop"}]}
      ]
    },
    "maxTurns": 9,
    "skills": ["code-review"],
    "initialPrompt": "Read first.",
    "memory": "project",
    "background": true,
    "isolation": "worktree",
    "requiredMcpServers": ["git"]
  }
}`
	tool := &AgentTool{}
	if err := tool.SetInlineProfilesFromJSON(raw); err != nil {
		t.Fatalf("SetInlineProfilesFromJSON: %v", err)
	}
	profile, err := tool.resolveProfileForInput(agentcontract.Input{SubagentType: "json-reviewer"})
	if err != nil {
		t.Fatalf("resolve inline profile: %v", err)
	}
	if profile.Name != "json-reviewer" || profile.Model != "haiku" || profile.ReasoningEffort != "high" || profile.MaxTurns != 9 {
		t.Fatalf("unexpected inline profile basics: %#v", profile)
	}
	if !profile.Background || profile.Memory != "project" || profile.Isolation != "worktree" {
		t.Fatalf("unexpected inline profile defaults: %#v", profile)
	}
	for _, expected := range []string{"read", "bash", "edit"} {
		if _, ok := profile.AllowedTools[expected]; !ok {
			t.Fatalf("expected allowed tool %q in %#v", expected, profile.AllowedTools)
		}
	}
	if !hasAgentPermissionRule(profile.AllowedToolRules, "bash", "git diff") {
		t.Fatalf("expected inline permission rule for Bash(git diff), got %#v", profile.AllowedToolRules)
	}
	if _, ok := profile.DisallowedTools["write"]; !ok {
		t.Fatalf("expected Write in disallowed tools: %#v", profile.DisallowedTools)
	}
	if got := strings.Join(profile.MCPServers, ","); !strings.Contains(got, "github") || !strings.Contains(got, "local") {
		t.Fatalf("expected JSON MCP servers, got %#v", profile.MCPServers)
	}
	if cfg := profile.MCPServerConfigs["local"]; cfg.Command != "node" || cfg.Args[0] != "server.js" || cfg.Env["TOKEN"] != "x" {
		t.Fatalf("unexpected JSON MCP config: %#v", cfg)
	}
	if profile.HookRunner == nil || !profile.HookRunner.HasHooks(hooks.HookSubagentStop) {
		t.Fatalf("expected JSON Stop hook mapped to SubagentStop")
	}
}

func TestInlineAgentJSONRejectsUnknownField(t *testing.T) {
	_, err := parseAgentProfilesJSON([]byte(`{
  "invalid": {
	"description": "Contains an unknown field",
	"prompt": "Do the task.",
	"unknownField": true
  }
}`))
	if err == nil || !strings.Contains(err.Error(), "unknownField") {
		t.Fatalf("expected strict JSON error, got %v", err)
	}
}

func TestAgentToolInlineJSONMemoryScopeIsCaseSensitive(t *testing.T) {
	tool := &AgentTool{}
	err := tool.SetInlineProfilesFromJSON(`{
  "json-reviewer": {
    "description": "Reviews JSON configured work",
    "prompt": "Review carefully.",
    "memory": "Project"
  }
}`)
	want := toolRuntimeFormat(i18n.KeyToolAgentDeepJSONMemoryUnsupported, "json-reviewer", "Project")
	if err == nil || err.Error() != want {
		t.Fatalf("expected JSON memory scope to match TS enum strictly, got %v", err)
	}
}

func TestAgentToolInlineJSONMaxTurnsMustBePositive(t *testing.T) {
	tool := &AgentTool{}
	for _, test := range []struct {
		raw      string
		maxTurns int
	}{
		{raw: `{"json-reviewer":{"description":"Reviews JSON configured work","prompt":"Review carefully.","maxTurns":0}}`, maxTurns: 0},
		{raw: `{"json-reviewer":{"description":"Reviews JSON configured work","prompt":"Review carefully.","maxTurns":-1}}`, maxTurns: -1},
	} {
		err := tool.SetInlineProfilesFromJSON(test.raw)
		want := toolRuntimeFormat(i18n.KeyToolAgentDeepJSONMaxTurnsUnsupported, "json-reviewer", test.maxTurns)
		if err == nil || err.Error() != want {
			t.Fatalf("expected JSON maxTurns to match TS positive-int schema, got %v", err)
		}
	}
}

func TestAgentToolInlineJSONMCPServersMustBeArray(t *testing.T) {
	tool := &AgentTool{}
	for _, raw := range []string{
		`{"json-reviewer":{"description":"Reviews JSON configured work","prompt":"Review carefully.","mcpServers":"github"}}`,
		`{"json-reviewer":{"description":"Reviews JSON configured work","prompt":"Review carefully.","mcpServers":{"github":{"command":"node"}}}}`,
	} {
		err := tool.SetInlineProfilesFromJSON(raw)
		want := toolRuntimeFormat(
			i18n.KeyToolAgentDeepJSONMCPServersInvalid,
			"json-reviewer",
			toolRuntimeText(i18n.KeyToolAgentDeepJSONArrayExpected),
		)
		if err == nil || err.Error() != want {
			t.Fatalf("expected JSON mcpServers to match TS array schema, got %v", err)
		}
	}
}

func TestAgentToolInlineJSONModelValidationMatchesOriginal(t *testing.T) {
	tool := &AgentTool{}
	err := tool.SetInlineProfilesFromJSON(`{
  "json-reviewer": {
    "description": "Reviews JSON configured work",
    "prompt": "Review carefully.",
    "model": "   "
  }
}`)
	want := toolRuntimeFormat(i18n.KeyToolAgentDeepJSONModelEmpty, "json-reviewer")
	if err == nil || err.Error() != want {
		t.Fatalf("expected empty JSON model to fail, got %v", err)
	}

	if err := tool.SetInlineProfilesFromJSON(`{
  "json-reviewer": {
    "description": "Reviews JSON configured work",
    "prompt": "Review carefully.",
    "model": "Inherit"
  }
}`); err != nil {
		t.Fatalf("SetInlineProfilesFromJSON inherit model: %v", err)
	}
	profile, err := tool.resolveProfileForInput(agentcontract.Input{SubagentType: "json-reviewer"})
	if err != nil {
		t.Fatalf("resolve inline profile: %v", err)
	}
	if profile.Model != "inherit" {
		t.Fatalf("expected JSON model inherit to normalize and inherit parent model, got %#v", profile)
	}
}

func TestAgentPluginProfileLoadsEnabledCachedAgent(t *testing.T) {
	configDir := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("LUBAN_CODE_CONFIG_DIR", configDir)
	t.Setenv("LUBAN_CODE_PLUGIN_CACHE_DIR", "")
	t.Setenv("LUBAN_CODE_USE_COWORK_PLUGINS", "")
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"enabledPlugins":{"sample@market":true}}`), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	pluginRoot := filepath.Join(configDir, "plugins", "cache", "market", "sample", "1.0.0")
	if err := os.MkdirAll(filepath.Join(pluginRoot, ".luban-plugin"), 0o755); err != nil {
		t.Fatalf("mkdir plugin manifest dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginRoot, "agents"), 0o755); err != nil {
		t.Fatalf("mkdir plugin agents dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, ".luban-plugin", "plugin.json"), []byte(`{"name":"sample","description":"Sample plugin"}`), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
	agentBody := `---
name: review
description: Review plugin
tools:
  - Read
  - Bash(git diff)
disallowedTools:
  - Write
effort: max
background: true
memory: project
isolation: worktree
---
Use ${LUBAN_PLUGIN_ROOT} safely.
`
	if err := os.WriteFile(filepath.Join(pluginRoot, "agents", "review.md"), []byte(agentBody), 0o644); err != nil {
		t.Fatalf("write plugin agent: %v", err)
	}

	profile, err := resolveAgentProfile("sample:review", cwd)
	if err != nil {
		t.Fatalf("resolve plugin profile: %v", err)
	}
	if profile.Name != "sample:review" {
		t.Fatalf("unexpected plugin profile name: %q", profile.Name)
	}
	if profile.WhenToUse != "Review plugin" || !profile.AllowedToolsSpecified {
		t.Fatalf("expected plugin description and explicit tools flag, got %#v", profile)
	}
	if !strings.Contains(profile.SystemPrefix, filepath.ToSlash(pluginRoot)) || strings.Contains(profile.SystemPrefix, "${LUBAN_PLUGIN_ROOT}") {
		t.Fatalf("expected plugin root substitution, got %q", profile.SystemPrefix)
	}
	for _, expected := range []string{"read", "bash", "edit"} {
		if _, ok := profile.AllowedTools[expected]; !ok {
			t.Fatalf("expected allowed tool %q in %#v", expected, profile.AllowedTools)
		}
	}
	if !hasAgentPermissionRule(profile.AllowedToolRules, "bash", "git diff") {
		t.Fatalf("expected Bash(git diff) permission rule, got %#v", profile.AllowedToolRules)
	}
	if _, ok := profile.DisallowedTools["write"]; !ok {
		t.Fatalf("expected Write in disallowed tools: %#v", profile.DisallowedTools)
	}
	if profile.HookRunner != nil || len(profile.MCPServers) != 0 || len(profile.MCPServerConfigs) != 0 {
		t.Fatalf("plugin agent should not import trust-sensitive fields: %#v", profile)
	}
	if profile.ReasoningEffort != "max" || !profile.Background || profile.Memory != "project" || profile.Isolation != "worktree" {
		t.Fatalf("expected safe plugin fields to load: %#v", profile)
	}
}

func TestAgentPluginMarkdownFrontmatterMatchesOriginalStrictFields(t *testing.T) {
	pluginRoot := t.TempDir()
	cwd := t.TempDir()
	agentsDir := filepath.Join(pluginRoot, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin agents dir: %v", err)
	}
	body := `---
name: strict
description: Strict plugin agent
background: "TRUE"
memory: Project
color: ultraviolet
---
Plugin prompt.
`
	path := filepath.Join(agentsDir, "strict.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write plugin agent: %v", err)
	}

	profile, ok, err := parsePluginAgentProfileFile(path, pluginAgentRoot{
		Name:   "sample",
		Source: "sample@market",
		Path:   pluginRoot,
		CWD:    cwd,
	}, nil)
	if err != nil || !ok {
		t.Fatalf("parse strict plugin agent ok=%v err=%v", ok, err)
	}
	if profile.Background || profile.Memory != "" || profile.Color != "" {
		t.Fatalf("expected invalid strict plugin fields to be ignored like TS loader, got %#v", profile)
	}
}

func TestAgentPluginProfileSubstitutesDataAndUserConfig(t *testing.T) {
	configDir := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("LUBAN_CODE_CONFIG_DIR", configDir)
	t.Setenv("LUBAN_CODE_PLUGIN_CACHE_DIR", "")
	t.Setenv("LUBAN_CODE_USE_COWORK_PLUGINS", "")
	settings := `{
  "enabledPlugins": {"configurable@market": true},
  "pluginConfigs": {
    "configurable@market": {
      "options": {
        "endpoint": "https://api.example.test"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	pluginRoot := filepath.Join(configDir, "plugins", "cache", "market", "configurable", "1.0.0")
	if err := os.MkdirAll(filepath.Join(pluginRoot, ".luban-plugin"), 0o755); err != nil {
		t.Fatalf("mkdir plugin manifest dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginRoot, "agents"), 0o755); err != nil {
		t.Fatalf("mkdir plugin agents dir: %v", err)
	}
	manifest := `{
  "name": "configurable",
  "description": "Configurable plugin",
  "userConfig": {
    "endpoint": {"type": "string", "title": "Endpoint", "description": "Endpoint"},
    "token": {"type": "string", "title": "Token", "description": "Token", "sensitive": true}
  }
}`
	if err := os.WriteFile(filepath.Join(pluginRoot, ".luban-plugin", "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
	agentBody := `---
name: helper
description: Uses plugin config
---
Root=${LUBAN_PLUGIN_ROOT}
Data=${LUBAN_PLUGIN_DATA}
Endpoint=${user_config.endpoint}
Token=${user_config.token}
Missing=${user_config.missing}
`
	if err := os.WriteFile(filepath.Join(pluginRoot, "agents", "helper.md"), []byte(agentBody), 0o644); err != nil {
		t.Fatalf("write plugin agent: %v", err)
	}

	profile, err := resolveAgentProfile("configurable:helper", cwd)
	if err != nil {
		t.Fatalf("resolve plugin profile: %v", err)
	}
	expectedDataDir := filepath.Join(configDir, "plugins", "data", "configurable-market")
	if !strings.Contains(profile.SystemPrefix, "Root="+filepath.ToSlash(pluginRoot)) {
		t.Fatalf("expected normalized plugin root substitution, got %q", profile.SystemPrefix)
	}
	if !strings.Contains(profile.SystemPrefix, "Data="+filepath.ToSlash(expectedDataDir)) {
		t.Fatalf("expected plugin data substitution, got %q", profile.SystemPrefix)
	}
	if _, err := os.Stat(expectedDataDir); err != nil {
		t.Fatalf("expected plugin data directory to be created: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "Endpoint=https://api.example.test") {
		t.Fatalf("expected non-sensitive user config substitution, got %q", profile.SystemPrefix)
	}
	if !strings.Contains(profile.SystemPrefix, "Token=[sensitive option 'token' not available in skill content]") {
		t.Fatalf("expected sensitive user config placeholder, got %q", profile.SystemPrefix)
	}
	if !strings.Contains(profile.SystemPrefix, "Missing=${user_config.missing}") {
		t.Fatalf("expected unknown user config reference to stay literal, got %q", profile.SystemPrefix)
	}
}

func TestAgentPluginVersionedCachePathKeepsSemverDots(t *testing.T) {
	got := filepath.ToSlash(versionedPluginCachePath(filepath.Join("C:", "plugins"), "sample@market", "1.2.3-beta.4+build"))
	if !strings.HasSuffix(got, "/cache/market/sample/1.2.3-beta.4-build") {
		t.Fatalf("expected semver dots to be preserved in plugin cache path, got %q", got)
	}
}

func TestAgentPluginProfileLoadsInstalledPluginPath(t *testing.T) {
	configDir := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("LUBAN_CODE_CONFIG_DIR", configDir)
	t.Setenv("LUBAN_CODE_PLUGIN_CACHE_DIR", "")
	t.Setenv("LUBAN_CODE_USE_COWORK_PLUGINS", "")
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"enabledPlugins":{"installed@market":true}}`), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	pluginRoot := filepath.Join(configDir, "outside-cache", "installed-plugin")
	if err := os.MkdirAll(filepath.Join(pluginRoot, ".luban-plugin"), 0o755); err != nil {
		t.Fatalf("mkdir plugin manifest dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginRoot, "agents"), 0o755); err != nil {
		t.Fatalf("mkdir plugin agents dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "plugins"), 0o755); err != nil {
		t.Fatalf("mkdir plugins dir: %v", err)
	}
	installed := fmt.Sprintf(`{"version":2,"plugins":{"installed@market":[{"scope":"user","installPath":%q,"version":"9.9.9"}]}}`, pluginRoot)
	if err := os.WriteFile(filepath.Join(configDir, "plugins", "installed_plugins.json"), []byte(installed), 0o644); err != nil {
		t.Fatalf("write installed plugins: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, ".luban-plugin", "plugin.json"), []byte(`{"name":"installed","description":"Installed plugin"}`), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
	agentBody := `---
name: review
description: Installed path agent
---
Loaded from installed_plugins.json.
`
	if err := os.WriteFile(filepath.Join(pluginRoot, "agents", "review.md"), []byte(agentBody), 0o644); err != nil {
		t.Fatalf("write plugin agent: %v", err)
	}

	profile, err := resolveAgentProfile("installed:review", cwd)
	if err != nil {
		t.Fatalf("resolve installed plugin profile: %v", err)
	}
	if profile.Name != "installed:review" || !strings.Contains(profile.SystemPrefix, "installed_plugins.json") {
		t.Fatalf("unexpected installed plugin profile: %#v", profile)
	}
}

func TestAgentPluginManifestAgentsSupplementAgentsDirectoryScan(t *testing.T) {
	configDir := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("LUBAN_CODE_CONFIG_DIR", configDir)
	t.Setenv("LUBAN_CODE_PLUGIN_CACHE_DIR", "")
	t.Setenv("LUBAN_CODE_USE_COWORK_PLUGINS", "")
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"enabledPlugins":{"pack@local":true}}`), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	pluginRoot := filepath.Join(configDir, "plugins", "cache", "local", "pack", "2.0.0")
	if err := os.MkdirAll(filepath.Join(pluginRoot, ".luban-plugin"), 0o755); err != nil {
		t.Fatalf("mkdir plugin manifest dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginRoot, "agents"), 0o755); err != nil {
		t.Fatalf("mkdir plugin agents dir: %v", err)
	}
	manifest := `{"name":"pack","description":"Pack","agents":["./extra.md"]}`
	if err := os.WriteFile(filepath.Join(pluginRoot, ".luban-plugin", "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
	extra := `---
name: extra
description: Extra
---
Use manifest-listed agent.
`
	if err := os.WriteFile(filepath.Join(pluginRoot, "extra.md"), []byte(extra), 0o644); err != nil {
		t.Fatalf("write manifest agent: %v", err)
	}
	ignored := `---
name: ignored
description: Ignored
---
Load directory agent alongside manifest-listed agents.
`
	if err := os.WriteFile(filepath.Join(pluginRoot, "agents", "ignored.md"), []byte(ignored), 0o644); err != nil {
		t.Fatalf("write ignored agent: %v", err)
	}

	profile, err := resolveAgentProfile("pack:extra", cwd)
	if err != nil {
		t.Fatalf("resolve manifest plugin profile: %v", err)
	}
	if profile.Name != "pack:extra" || !strings.Contains(profile.SystemPrefix, "manifest-listed") {
		t.Fatalf("unexpected manifest plugin profile: %#v", profile)
	}
	profile, err = resolveAgentProfile("pack:ignored", cwd)
	if err != nil {
		t.Fatalf("resolve directory plugin profile: %v", err)
	}
	if profile.Name != "pack:ignored" || !strings.Contains(profile.SystemPrefix, "directory agent") {
		t.Fatalf("manifest-listed agents should supplement agents directory scan, got %#v", profile)
	}
}

func TestAgentToolBuildLoopUsesInlineJSONAgentProfile(t *testing.T) {
	provider := &captureAgentProvider{responses: []string{"inline done"}}
	tool := &AgentTool{
		Provider: provider,
		Registry: registry.New(),
		Model:    "parent-model",
	}
	if err := tool.SetInlineProfilesFromJSON(`{
  "inline-builder": {
    "description": "Build inline",
    "prompt": "Inline system prompt",
    "model": "haiku",
    "maxTurns": 7
  }
}`); err != nil {
		t.Fatalf("SetInlineProfilesFromJSON: %v", err)
	}
	bundle, err := tool.buildSubAgentLoop("agent-inline", agentcontract.Input{
		Prompt:       "work",
		SubagentType: "inline-builder",
		Model:        "opus",
	})
	if err != nil {
		t.Fatalf("buildSubAgentLoop: %v", err)
	}
	if bundle.Metadata.AgentType != "inline-builder" || bundle.Metadata.Model != "parent-model" {
		t.Fatalf("expected inline profile metadata, got %#v", bundle.Metadata)
	}
	summary, err := runAgentQueryLoop(context.Background(), bundle.Loop, bundle.Metadata, "agent-inline", "work", nil)
	if err != nil {
		t.Fatalf("runAgentQueryLoop: %v", err)
	}
	if summary.Output != "inline done" {
		t.Fatalf("unexpected output: %q", summary.Output)
	}
	if len(provider.params) != 1 {
		t.Fatalf("expected one provider call, got %d", len(provider.params))
	}
	if !strings.Contains(provider.params[0].System, "Inline system prompt") {
		t.Fatalf("expected inline system prompt, got %q", provider.params[0].System)
	}
	if provider.params[0].Model != "parent-model" {
		t.Fatalf("expected inherited parent model, got %q", provider.params[0].Model)
	}
}

func TestAgentToolInlineJSONExplicitEmptyToolsDeniesAllTools(t *testing.T) {
	tool := &AgentTool{Registry: registry.New()}
	tool.Registry.Register(&toolfile.FileReadTool{})
	if err := tool.SetInlineProfilesFromJSON(`{
  "inline-empty": {
    "description": "No tools",
    "prompt": "Inline system prompt",
    "tools": []
  }
}`); err != nil {
		t.Fatalf("SetInlineProfilesFromJSON: %v", err)
	}
	profile, err := tool.resolveProfileForInput(agentcontract.Input{SubagentType: "inline-empty"})
	if err != nil {
		t.Fatalf("resolve inline profile: %v", err)
	}
	if !profile.AllowedToolsSpecified || len(profile.AllowedTools) != 0 {
		t.Fatalf("expected inline JSON empty tools to be explicit, got %#v", profile)
	}
	if filtered := registryForAgentProfile(tool.Registry, profile); filtered.Get("Read") != nil {
		t.Fatalf("expected inline JSON empty tools to deny Read")
	}
}

func TestAgentToolInlineGeneralPurposeOverridesDefault(t *testing.T) {
	tool := &AgentTool{}
	if err := tool.SetInlineProfilesFromJSON(`{"general-purpose":{"description":"Default","prompt":"Inline default prompt."}}`); err != nil {
		t.Fatalf("SetInlineProfilesFromJSON: %v", err)
	}
	profile, err := tool.resolveProfileForInput(agentcontract.Input{})
	if err != nil {
		t.Fatalf("resolve inline default profile: %v", err)
	}
	if profile.SystemPrefix != "Inline default prompt." {
		t.Fatalf("expected inline default profile, got %#v", profile)
	}
}

func TestAgentProfileDefaultsForceBackgroundAndPreloadSkills(t *testing.T) {
	projectRoot, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "review", "Use this review skill.")
	manager := newTestSkillManager(skillsDir)
	tool := &AgentTool{SkillManager: manager}

	input, err := tool.applyAgentProfileDefaults("agent-defaults", agentcontract.Input{Prompt: "main task"}, agentProfile{
		InitialPrompt: "Read this first.",
		Background:    true,
		Skills:        []string{"review"},
	}, captureSkillAuthorityForTest(t, manager, "agent-defaults-parent", projectRoot))
	if err != nil {
		t.Fatal(err)
	}

	if !input.RunInBackground {
		t.Fatal("expected background frontmatter to force run_in_background")
	}
	for _, expected := range []string{"Read this first.", `<skill name="review">`, "Use this review skill.", "main task"} {
		if !strings.Contains(input.Prompt, expected) {
			t.Fatalf("expected decorated prompt to contain %q, got: %s", expected, input.Prompt)
		}
	}
}

func TestAgentMCPRequirementsValidateAvailableTools(t *testing.T) {
	reg := registry.New()
	reg.Register(fakeTool{name: "mcp__github__search"})
	if err := validateAgentMCPRequirements(reg, agentProfile{Name: "needs-github", RequiredMCPServers: []string{"git"}}); err != nil {
		t.Fatalf("expected github MCP requirement to pass: %v", err)
	}
	err := validateAgentMCPRequirements(reg, agentProfile{Name: "needs-slack", RequiredMCPServers: []string{"slack"}})
	if err == nil || !strings.Contains(err.Error(), "slack") {
		t.Fatalf("expected missing MCP requirement error, got %v", err)
	}
}

func TestAgentMemoryPromptLoadsEntrypoint(t *testing.T) {
	base := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LUBAN_CODE_DISABLE_AUTO_MEMORY", "")
	t.Setenv("LUBAN_CODE_REMOTE", "")
	t.Setenv("LUBAN_CODE_SIMPLE", "")
	memoryDir, err := agentMemoryDir("reviewer", "project", base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir memory dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(memoryDir, "MEMORY.md"), []byte("- [Style](style.md) - prefers concise findings\n"), 0o644); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	prompt := buildAgentSystemPrompt("", agentProfile{Name: "reviewer", Memory: "project"}, "", base)
	for _, expected := range []string{"Persistent Agent Memory", memoryDir, "prefers concise findings"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected memory prompt to contain %q, got: %s", expected, prompt)
		}
	}
	for path, want := range map[string]os.FileMode{
		memoryDir:                             0o700,
		filepath.Join(memoryDir, "MEMORY.md"): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("mode(%s) = %04o, want %04o", path, got, want)
		}
	}
	if _, err := os.Lstat(filepath.Join(base, brand.ConfigDirName)); !os.IsNotExist(err) {
		t.Fatalf("agent memory dirtied project: %v", err)
	}
}

func TestAgentMemoryPromptHonorsAutoMemoryDisable(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LUBAN_CODE_DISABLE_AUTO_MEMORY", "1")
	prompt := buildAgentSystemPrompt("", agentProfile{Name: "reviewer", Memory: "project"}, "", base)
	if strings.Contains(prompt, "Persistent Agent Memory") {
		t.Fatalf("expected auto memory disable to suppress memory prompt, got: %s", prompt)
	}
}

func TestAgentMemoryPromptIncludesExtraGuidelinesAndTruncatesEntrypoint(t *testing.T) {
	base := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LUBAN_CODE_DISABLE_AUTO_MEMORY", "")
	t.Setenv("LUBAN_COWORK_MEMORY_EXTRA_GUIDELINES", "Never save incident IDs in memory.")
	memoryDir, err := agentMemoryDir("reviewer", "project", base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir memory dir: %v", err)
	}
	var b strings.Builder
	for i := 0; i < agentMemoryMaxEntrypointLines+3; i++ {
		fmt.Fprintf(&b, "- entry %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(memoryDir, "MEMORY.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	prompt := buildAgentSystemPrompt("", agentProfile{Name: "reviewer", Memory: "project"}, "", base)
	if !strings.Contains(prompt, "Never save incident IDs in memory.") {
		t.Fatalf("expected extra memory guidelines, got: %s", prompt)
	}
	if !strings.Contains(prompt, "WARNING: MEMORY.md exceeded") {
		t.Fatalf("expected truncation warning, got: %s", prompt)
	}
	if strings.Contains(prompt, "- entry 202") {
		t.Fatalf("expected memory entrypoint to be line-truncated, got: %s", prompt)
	}
}

func TestAgentMemoryPromptInitializesFromSnapshot(t *testing.T) {
	base := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LUBAN_CODE_DISABLE_AUTO_MEMORY", "")
	t.Setenv("LUBAN_CODE_REMOTE", "")
	t.Setenv("LUBAN_CODE_SIMPLE", "")
	snapshotDir := agentMemorySnapshotDir("reviewer", base)
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		t.Fatalf("mkdir snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshotDir, "snapshot.json"), []byte(`{"updatedAt":"2026-04-25T00:00:00Z"}`), 0o644); err != nil {
		t.Fatalf("write snapshot meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshotDir, "MEMORY.md"), []byte("- [Rules](rules.md) - seeded from snapshot\n"), 0o644); err != nil {
		t.Fatalf("write snapshot memory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshotDir, "rules.md"), []byte("Prefer evidence-bound reports.\n"), 0o644); err != nil {
		t.Fatalf("write snapshot detail: %v", err)
	}

	prompt := buildAgentSystemPrompt("", agentProfile{Name: "reviewer", Memory: "project"}, "", base)
	memoryDir, err := agentMemoryDir("reviewer", "project", base)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"seeded from snapshot", memoryDir} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected memory prompt to contain %q, got: %s", expected, prompt)
		}
	}
	if _, err := os.Stat(filepath.Join(memoryDir, "rules.md")); err != nil {
		t.Fatalf("expected snapshot detail copied to memory dir: %v", err)
	}
	synced, err := os.ReadFile(filepath.Join(memoryDir, ".snapshot-synced.json"))
	if err != nil {
		t.Fatalf("expected synced metadata: %v", err)
	}
	if !strings.Contains(string(synced), "2026-04-25T00:00:00Z") {
		t.Fatalf("expected synced metadata timestamp, got: %s", synced)
	}
}

func TestRunAgentQueryLoopRunsSubagentLifecycleHooks(t *testing.T) {
	provider := &captureAgentProvider{responses: []string{"first", "second", "third", "fourth"}}
	runner := hooks.NewRunner([]hooks.Hook{
		{
			Type:    hooks.HookSubagentStart,
			Matcher: "reviewer",
			Command: "echo start-context",
			Timeout: 5,
		},
		{
			Type:    hooks.HookSubagentStop,
			Matcher: "reviewer",
			Command: `echo '{"system_reminder":"stop-continue","block":true}'`,
			Timeout: 5,
		},
	})
	subLoop := loop.New(provider, registry.New(), loop.Config{
		MaxTurns:   1,
		HookRunner: runner,
		SessionID:  "agent-lifecycle",
	})

	summary, err := runAgentQueryLoop(context.Background(), subLoop, agentcontract.SessionMetadata{AgentType: "reviewer"}, "agent-lifecycle", "main prompt", nil)
	if err != nil {
		t.Fatalf("runAgentQueryLoop: %v", err)
	}
	if len(provider.params) != 4 {
		t.Fatalf("expected stop hook continuation limit to allow four model calls, got %d", len(provider.params))
	}
	firstPrompt := provider.params[0].Messages[0].GetText()
	if !strings.Contains(firstPrompt, "start-context") || !strings.Contains(firstPrompt, "main prompt") {
		t.Fatalf("expected SubagentStart output in first prompt, got: %q", firstPrompt)
	}
	secondPrompt := provider.params[1].Messages[len(provider.params[1].Messages)-1].GetText()
	if !strings.Contains(secondPrompt, "stop-continue") {
		t.Fatalf("expected SubagentStop block output to continue subagent, got: %q", secondPrompt)
	}
	for _, expected := range []string{"first", "second", "third", "fourth"} {
		if !strings.Contains(summary.Output, expected) {
			t.Fatalf("expected summary to include %q, got: %q", expected, summary.Output)
		}
	}
}

func TestRunAgentQueryLoopReturnsPartialOutputOnExplicitMaxTurns(t *testing.T) {
	provider := &sequencedAgentProvider{
		responses: [][]types.StreamEvent{
			agentTextAndToolEvents("partial before turn cap", "Echo", "call_cap"),
		},
	}
	reg := registry.New()
	reg.Register(fakeTool{name: "Echo"})
	subLoop := loop.New(provider, reg, loop.Config{
		MaxTurns:  1,
		MaxTokens: 1024,
		SessionID: "agent-max-turns",
	})

	summary, err := runAgentQueryLoop(context.Background(), subLoop, agentcontract.SessionMetadata{AgentType: "custom"}, "agent-max-turns", "main prompt", nil)
	if err != nil {
		t.Fatalf("runAgentQueryLoop should preserve partial output instead of hard failing: %v", err)
	}
	if !strings.Contains(summary.Output, "partial before turn cap") {
		t.Fatalf("expected partial assistant text in summary, got %q", summary.Output)
	}
}

func TestRunAgentQueryLoopAccumulatesUsageAcrossTurns(t *testing.T) {
	firstUsage := types.Usage{
		InputTokens:              10,
		OutputTokens:             2,
		CacheCreationInputTokens: 3,
		CacheReadInputTokens:     4,
		ServerToolUse: types.ServerToolUsage{
			WebSearchRequests: 1,
		},
	}
	secondUsage := types.Usage{
		InputTokens:              20,
		OutputTokens:             3,
		CacheCreationInputTokens: 2,
		CacheReadInputTokens:     8,
		ServerToolUse: types.ServerToolUsage{
			WebSearchRequests: 2,
			WebFetchRequests:  1,
		},
	}
	provider := &sequencedAgentProvider{responses: [][]types.StreamEvent{
		agentEventsWithUsage(agentToolEvents("Echo", "call_usage"), firstUsage),
		agentEventsWithUsage(agentTextEvents("done"), secondUsage),
	}}
	reg := registry.New()
	reg.Register(fakeTool{name: "Echo"})
	subLoop := loop.New(provider, reg, loop.Config{MaxTurns: 2, SessionID: "agent-usage"})

	emitter := newAgentProgressEmitter("agent-usage", "general-purpose")
	var progressEvents []agentcontract.ProgressEvent
	emitter.AddObserver(func(event agentcontract.ProgressEvent) {
		progressEvents = append(progressEvents, event)
	})
	ctx := withAgentProgressEmitter(context.Background(), emitter)
	summary, err := runAgentQueryLoop(ctx, subLoop, agentcontract.SessionMetadata{AgentType: "general-purpose"}, "agent-usage", "work", nil)
	if err != nil {
		t.Fatalf("runAgentQueryLoop: %v", err)
	}
	if summary.Usage == nil {
		t.Fatal("summary usage is nil")
	}
	want := types.Usage{
		InputTokens:              30,
		OutputTokens:             5,
		CacheCreationInputTokens: 5,
		CacheReadInputTokens:     12,
		ServerToolUse: types.ServerToolUsage{
			WebSearchRequests: 3,
			WebFetchRequests:  1,
		},
	}
	if *summary.Usage != want {
		t.Fatalf("summary usage = %+v, want %+v", *summary.Usage, want)
	}
	if summary.TotalTokens != 35 {
		t.Fatalf("summary TotalTokens = %d, want 35", summary.TotalTokens)
	}
	var terminal agentcontract.ProgressEvent
	for _, event := range progressEvents {
		if event.Phase == agentcontract.ProgressCompleted {
			terminal = event
		}
	}
	if terminal.Usage == nil || *terminal.Usage != want {
		t.Fatalf("terminal cumulative usage = %+v, want %+v", terminal.Usage, want)
	}
	if terminal.LastRequestUsage == nil || *terminal.LastRequestUsage != secondUsage {
		t.Fatalf("terminal last request usage = %+v, want %+v", terminal.LastRequestUsage, secondUsage)
	}
}

func TestAgentToolCompletedResultPropagatesUsageToParentBlock(t *testing.T) {
	usage := types.Usage{
		InputTokens:              25,
		OutputTokens:             7,
		CacheCreationInputTokens: 5,
		CacheReadInputTokens:     12,
		ServerToolUse: types.ServerToolUsage{
			WebSearchRequests: 2,
		},
	}
	provider := &sequencedAgentProvider{responses: [][]types.StreamEvent{
		agentEventsWithUsage(agentTextEvents("usage result"), usage),
	}}
	tool := &AgentTool{Provider: provider, Registry: registry.New()}

	result, err := tool.Execute(context.Background(), agentExecuteInput("report usage", nil))
	if err != nil || result.IsError {
		t.Fatalf("Execute err=%v result=%+v", err, result)
	}
	if result.Usage == nil || *result.Usage != usage {
		t.Fatalf("ToolResult.Usage = %+v, want %+v", result.Usage, usage)
	}
	if result.Metadata["usage.provider"] != "sequenced" || result.Metadata["usage.model"] != "sequenced-model" {
		t.Fatalf("ToolResult usage identity = %+v", result.Metadata)
	}
	block := types.MapToolResult(tool, result, "toolu_agent_usage")
	if block.Usage == nil || *block.Usage != usage {
		t.Fatalf("mapped ToolResultBlock.Usage = %+v, want %+v", block.Usage, usage)
	}
	if block.Metadata["usage.provider"] != "sequenced" || block.Metadata["usage.model"] != "sequenced-model" {
		t.Fatalf("mapped ToolResultBlock usage identity = %+v", block.Metadata)
	}

	var payload agentCompletedToolResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode completed JSON: %v", err)
	}
	if payload.Usage.InputTokens != usage.InputTokens || payload.Usage.OutputTokens != usage.OutputTokens {
		t.Fatalf("completed JSON usage = %+v, want input=%d output=%d", payload.Usage, usage.InputTokens, usage.OutputTokens)
	}
}

func TestAgentToolUsagePropagationRequiresMeasuredUsage(t *testing.T) {
	partial := AgentResultFromAsyncLaunch("agent-async", "general-purpose", "async", "work", "", false)
	if result := agentToolResult(partial, "launched", false); result.Usage != nil {
		t.Fatalf("async launch invented usage: %+v", result.Usage)
	}
	if result := agentFailureToolResult(context.Background(), "agent-failed", "general-purpose", "", time.Now(), fmt.Errorf("failed before provider call")); result.Usage != nil {
		t.Fatalf("unmeasured failure invented usage: %+v", result.Usage)
	}

	measured := &types.Usage{InputTokens: 9, OutputTokens: 4, CacheReadInputTokens: 6}
	result := agentFailureToolResultWithUsage(context.Background(), "agent-partial", "general-purpose", "", time.Now(), fmt.Errorf("failed after provider call"), measured)
	if result.Usage == nil || *result.Usage != *measured {
		t.Fatalf("measured failure usage = %+v, want %+v", result.Usage, measured)
	}
	measured.InputTokens = 99
	if result.Usage.InputTokens != 9 {
		t.Fatalf("ToolResult usage aliases mutable summary usage: %+v", result.Usage)
	}
}

func TestAgentToolForegroundAgentDoesNotInstallFixedDeadline(t *testing.T) {
	provider := &turnLimitAgentProvider{
		toolName:  "Echo",
		finalText: "foreground done",
	}
	tool := &AgentTool{
		Provider: provider,
		Registry: registry.New(),
		Model:    "parent-model",
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("finish directly", nil))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected foreground agent success, got %s", result.Content)
	}
	if provider.sawDeadline {
		t.Fatal("foreground Agent should inherit the parent context without installing a fixed deadline")
	}
}

func TestAgentToolAutoBackgroundLaunchesAfterThreshold(t *testing.T) {
	oldDelay := agentAutoBackgroundDelay
	agentAutoBackgroundDelay = func() time.Duration { return 10 * time.Millisecond }
	t.Cleanup(func() { agentAutoBackgroundDelay = oldDelay })

	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	notifications := make(chan agentcontract.RuntimeNotification, 1)
	background.SetNotificationConsumers(RuntimeNotificationSinkFunc(func(_ context.Context, notification agentcontract.RuntimeNotification) error {
		notifications <- notification
		return nil
	}), nil)
	provider := &captureAgentProvider{
		responses: []string{"auto-background done"},
		delay:     100 * time.Millisecond,
	}
	tool := &AgentTool{
		Provider:   provider,
		Registry:   registry.New(),
		Background: background,
	}

	parentCtx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{SessionID: "parent-auto-background-session"})
	result, err := tool.Execute(parentCtx, map[string]any{
		"prompt":      "slow task",
		"description": "slow task",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected async launch result, got error: %s", result.Content)
	}
	var payload agentAsyncToolResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("parse async result: %v\n%s", err, result.Content)
	}
	if !payload.IsAsync || payload.Status != "async_launched" || payload.AgentID == "" || payload.OutputFile == "" {
		t.Fatalf("unexpected async payload: %#v", payload)
	}
	if payload.CanReadOutputFile {
		t.Fatalf("expected unreadable output flag without Read/Bash tools, got %#v", payload)
	}
	snap, status := background.Wait(payload.AgentID, 2*time.Second)
	if status != "success" || snap.Status != "completed" {
		t.Fatalf("expected background agent to complete, wait status=%s snap=%#v", status, snap)
	}
	if !strings.Contains(snap.Result, "auto-background done") {
		t.Fatalf("expected background result to persist output, got: %q", snap.Result)
	}
	select {
	case notification := <-notifications:
		if notification.TaskID != payload.AgentID || notification.Status != "completed" || notification.SessionID != "parent-auto-background-session" {
			t.Fatalf("auto-background notification = %+v", notification)
		}
	case <-time.After(time.Second):
		t.Fatal("auto-background completion did not notify after foreground timeout")
	}
}

type contextBlockingAgentProvider struct {
	started chan struct{}
	once    sync.Once
}

func (p *contextBlockingAgentProvider) Name() string    { return "context-blocking" }
func (p *contextBlockingAgentProvider) ModelID() string { return "context-blocking-model" }

func (p *contextBlockingAgentProvider) CreateStream(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	p.once.Do(func() { close(p.started) })
	ch := make(chan types.StreamEvent)
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

func TestAgentToolAutoBackgroundCancellationStopsRetainedSession(t *testing.T) {
	oldDelay := agentAutoBackgroundDelay
	agentAutoBackgroundDelay = func() time.Duration { return 10 * time.Second }
	t.Cleanup(func() { agentAutoBackgroundDelay = oldDelay })

	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	p := &contextBlockingAgentProvider{started: make(chan struct{})}
	tool := &AgentTool{Provider: p, Registry: registry.New(), Background: background}
	ctx, cancel := context.WithCancel(context.Background())
	type executeResult struct {
		result types.ToolResult
		err    error
	}
	finished := make(chan executeResult, 1)
	go func() {
		result, err := tool.Execute(ctx, agentExecuteInput("wait until cancelled", nil))
		finished <- executeResult{result: result, err: err}
	}()

	select {
	case <-p.started:
	case <-time.After(time.Second):
		t.Fatal("subagent provider did not start")
	}
	cancel()
	var execution executeResult
	select {
	case execution = <-finished:
	case <-time.After(time.Second):
		t.Fatal("Agent execution did not return after parent cancellation")
	}
	if execution.err != nil || !execution.result.IsError {
		t.Fatalf("cancelled Agent result=%+v err=%v", execution.result, execution.err)
	}

	background.mu.Lock()
	var taskID string
	var session *backgroundAgentSession
	for id, candidate := range background.sessions {
		taskID, session = id, candidate
		break
	}
	background.mu.Unlock()
	if session == nil {
		t.Fatal("cancelled auto-background agent did not register a retained session")
	}
	select {
	case <-session.done:
	case <-time.After(time.Second):
		t.Fatal("retained agent session survived parent cancellation")
	}
	snapshot, ok := background.Snapshot(taskID)
	if !ok || snapshot.Status != "killed" {
		t.Fatalf("cancelled task snapshot=%+v ok=%v, want killed", snapshot, ok)
	}
}

func TestAgentToolBackgroundDisableForcesSynchronousRun(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "1")
	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	provider := &captureAgentProvider{responses: []string{"sync despite background"}}
	tool := &AgentTool{
		Provider:   provider,
		Registry:   registry.New(),
		Background: background,
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"prompt":            "run now",
		"description":       "run now",
		"run_in_background": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected synchronous success, got: %s", result.Content)
	}
	var payload agentCompletedToolResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("expected completed JSON result, got %v\n%s", err, result.Content)
	}
	if payload.Status != "completed" || !strings.Contains(payload.Content[0].Text, "sync despite background") {
		t.Fatalf("expected completed foreground result, got %#v", payload)
	}
}

func TestAgentToolForkSubagentInheritsParentContext(t *testing.T) {
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "1")
	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	provider := &captureAgentProvider{responses: []string{"fork done"}}
	reg := registry.New()
	reg.Register(&AgentTool{})
	reg.Register(&toolfile.FileReadTool{})
	tool := &AgentTool{
		Provider:   provider,
		Registry:   reg,
		Background: background,
		System:     "fallback system",
		Model:      "parent-model",
	}
	assistantMessage := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "I will fork this."},
			types.ToolUseBlock{
				Type:  types.ContentTypeToolUse,
				ID:    "toolu_agent",
				Name:  "Agent",
				Input: map[string]any{"prompt": "inspect context"},
			},
		},
	}
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		Messages:         []types.Message{types.UserMessage("parent context")},
		AssistantMessage: assistantMessage,
		ToolUse: types.ToolUseBlock{
			Type:  types.ContentTypeToolUse,
			ID:    "toolu_agent",
			Name:  "Agent",
			Input: map[string]any{"prompt": "inspect context"},
		},
		System: "parent system",
		Model:  "parent-model",
	})
	result, err := tool.Execute(ctx, map[string]any{
		"prompt":      "inspect context",
		"description": "inspect context",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected async fork launch, got error: %s", result.Content)
	}
	var payload agentAsyncToolResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("parse fork async result: %v\n%s", err, result.Content)
	}
	if payload.Status != "async_launched" || payload.AgentID == "" {
		t.Fatalf("unexpected fork payload: %#v", payload)
	}
	snap, status := background.Wait(payload.AgentID, 2*time.Second)
	if status != "success" || snap.Status != "completed" {
		t.Fatalf("expected fork task complete, status=%s snap=%#v", status, snap)
	}
	if len(provider.params) != 1 {
		t.Fatalf("expected provider call, got %d", len(provider.params))
	}
	params := provider.params[0]
	if params.System != "parent system" {
		t.Fatalf("expected parent system prompt, got %q", params.System)
	}
	if params.Model != "parent-model" {
		t.Fatalf("expected inherited parent model, got %q", params.Model)
	}
	if len(params.Messages) != 3 {
		t.Fatalf("expected parent + assistant + fork directive messages, got %#v", params.Messages)
	}
	if params.Messages[0].GetText() != "parent context" {
		t.Fatalf("expected parent context first, got %#v", params.Messages[0])
	}
	if len(params.Messages[2].Content) != 2 {
		t.Fatalf("expected placeholder tool result plus directive, got %#v", params.Messages[2].Content)
	}
	resultBlock, ok := params.Messages[2].Content[0].(types.ToolResultBlock)
	if !ok || resultBlock.ToolUseID != "toolu_agent" || resultBlock.Content != forkPlaceholderToolResultText() {
		t.Fatalf("unexpected fork placeholder result: %#v", params.Messages[2].Content[0])
	}
	forkDirective := params.Messages[2].GetText()
	if !strings.Contains(forkDirective, "<"+forkBoilerplateTag+">") || !strings.Contains(forkDirective, forkDirectivePrefix+"inspect context") {
		t.Fatalf("expected fork boilerplate directive, got %#v", params.Messages[2])
	}
	if !strings.Contains(forkDirective, `IGNORE IT — that's for the parent`) ||
		!strings.Contains(forkDirective, "Output format (plain text labels, not markdown headers):") ||
		!strings.Contains(forkDirective, "Files changed: <list with commit hash — include only if you modified files>") {
		t.Fatalf("expected fork directive text to align with upstream prompt, got %q", forkDirective)
	}
	toolNames := map[string]bool{}
	for _, def := range params.Tools {
		toolNames[def.Name] = true
	}
	if !toolNames["Agent"] || !toolNames["Read"] {
		t.Fatalf("expected exact parent tool pool for fork, got %#v", toolNames)
	}
}

func TestAgentToolForkSubagentRejectsRecursiveFork(t *testing.T) {
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "1")
	tool := &AgentTool{
		Provider:   &captureAgentProvider{responses: []string{"unused"}},
		Registry:   registry.New(),
		Background: NewBackgroundTaskManager(t.TempDir()),
	}
	t.Cleanup(func() { _ = tool.Background.Shutdown(context.Background()) })
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		Messages: []types.Message{types.UserMessage("<" + forkBoilerplateTag + ">")},
		AssistantMessage: types.Message{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "toolu_agent", Name: "Agent", Input: map[string]any{"prompt": "again"}},
			},
		},
	})
	result, err := tool.Execute(ctx, agentExecuteInput("again", nil))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || result.Content != toolRuntimeText(i18n.KeyToolAgentDeepForkNestedUnavailable) {
		t.Fatalf("expected recursive fork rejection, got %#v", result)
	}
}

func TestForkWorktreeNoticeAlignsUpstreamText(t *testing.T) {
	notice := buildForkWorktreeNotice("/repo", "/repo/.luban-code/worktrees/agent")
	for _, expected := range []string{
		"You've inherited the conversation context above from a parent agent working in /repo.",
		"You are operating in an isolated git worktree at /repo/.luban-code/worktrees/agent — same repository",
		"translate them to your worktree root",
		"Your changes stay in this worktree and will not affect the parent's files.",
	} {
		if !strings.Contains(notice, expected) {
			t.Fatalf("expected worktree notice to contain %q, got %q", expected, notice)
		}
	}
}

type fakeTool struct {
	name string
}

func (f fakeTool) Name() string        { return f.name }
func (f fakeTool) Description() string { return "fake" }
func (f fakeTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}

func hasAgentPermissionRule(rules []agentPermissionRule, toolName, ruleContent string) bool {
	for _, rule := range rules {
		if rule.ToolName == toolName && rule.RuleContent == ruleContent {
			return true
		}
	}
	return false
}

type captureAgentProvider struct {
	responses []string
	params    []provider.Params
	delay     time.Duration
}

type turnLimitAgentProvider struct {
	toolName    string
	toolTurns   int
	finalText   string
	calls       int
	params      []provider.Params
	sawDeadline bool
}

func (p *turnLimitAgentProvider) Name() string    { return "turn-limit" }
func (p *turnLimitAgentProvider) ModelID() string { return "turn-limit-model" }

func (p *turnLimitAgentProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	if _, ok := ctx.Deadline(); ok {
		p.sawDeadline = true
	}
	call := p.calls
	p.calls++
	p.params = append(p.params, params)
	if call < p.toolTurns {
		return eventStream(agentToolEvents(p.toolName, fmt.Sprintf("call_%d", call))), nil
	}
	return eventStream(agentTextEvents(p.finalText)), nil
}

type sequencedAgentProvider struct {
	responses [][]types.StreamEvent
	calls     int
}

func (p *sequencedAgentProvider) Name() string    { return "sequenced" }
func (p *sequencedAgentProvider) ModelID() string { return "sequenced-model" }

func (p *sequencedAgentProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	call := p.calls
	p.calls++
	if call < len(p.responses) {
		return eventStream(p.responses[call]), nil
	}
	return eventStream(agentTextEvents("(no response)")), nil
}

func eventStream(events []types.StreamEvent) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, len(events))
	go func() {
		defer close(ch)
		for _, event := range events {
			ch <- event
		}
	}()
	return ch
}

func agentEventsWithUsage(events []types.StreamEvent, usage types.Usage) []types.StreamEvent {
	withUsage := make([]types.StreamEvent, 0, len(events)+1)
	inserted := false
	for _, event := range events {
		if !inserted && event.Type == types.EventMessageStop {
			usageCopy := usage
			withUsage = append(withUsage, types.StreamEvent{Type: types.EventMessageDelta, Usage: &usageCopy})
			inserted = true
		}
		withUsage = append(withUsage, event)
	}
	if !inserted {
		usageCopy := usage
		withUsage = append(withUsage, types.StreamEvent{Type: types.EventMessageDelta, Usage: &usageCopy})
	}
	return withUsage
}

func agentTextEvents(text string) []types.StreamEvent {
	return []types.StreamEvent{
		{
			Type:         types.EventContentBlockStart,
			Index:        0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
		},
		{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: text},
		},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
}

func agentToolEvents(toolName, toolID string) []types.StreamEvent {
	input, _ := json.Marshal(map[string]any{})
	return []types.StreamEvent{
		{
			Type:  types.EventContentBlockStart,
			Index: 0,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse,
				ID:   toolID,
				Name: toolName,
			},
		},
		{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(input)},
		},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
}

func agentTextAndToolEvents(text, toolName, toolID string) []types.StreamEvent {
	input, _ := json.Marshal(map[string]any{})
	return []types.StreamEvent{
		{
			Type:         types.EventContentBlockStart,
			Index:        0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
		},
		{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: text},
		},
		{Type: types.EventContentBlockStop, Index: 0},
		{
			Type:  types.EventContentBlockStart,
			Index: 1,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse,
				ID:   toolID,
				Name: toolName,
			},
		},
		{
			Type:  types.EventContentBlockDelta,
			Index: 1,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(input)},
		},
		{Type: types.EventContentBlockStop, Index: 1},
		{Type: types.EventMessageStop},
	}
}

func (p *captureAgentProvider) Name() string    { return "capture" }
func (p *captureAgentProvider) ModelID() string { return "capture-model" }

func (p *captureAgentProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.params = append(p.params, params)
	text := "(no response)"
	if len(p.params)-1 < len(p.responses) {
		text = p.responses[len(p.params)-1]
	}
	ch := make(chan types.StreamEvent, 16)
	go func() {
		defer close(ch)
		if p.delay > 0 {
			time.Sleep(p.delay)
		}
		ch <- types.StreamEvent{
			Type:         types.EventContentBlockStart,
			Index:        0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
		}
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: text},
		}
		ch <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
		ch <- types.StreamEvent{Type: types.EventMessageStop}
	}()
	return ch, nil
}
func (f fakeTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: "ok"}, nil
}

type denyToolPermissionHandler struct {
	tool string
}

func (h denyToolPermissionHandler) Check(_ context.Context, req permission.PermissionRequest) (permission.PermissionDecision, error) {
	if req.ToolName == h.tool {
		return permission.PermissionDeny, nil
	}
	return permission.PermissionAllow, nil
}

type captureToolPermissionHandler struct {
	requests []permission.PermissionRequest
}

func (h *captureToolPermissionHandler) Check(_ context.Context, req permission.PermissionRequest) (permission.PermissionDecision, error) {
	h.requests = append(h.requests, req)
	return permission.PermissionAllow, nil
}

type modeGatePermissionHandler struct {
	requests []permission.PermissionRequest
}

func (h *modeGatePermissionHandler) Check(_ context.Context, req permission.PermissionRequest) (permission.PermissionDecision, error) {
	h.requests = append(h.requests, req)
	if req.Mode == "acceptEdits" {
		return permission.PermissionAllow, nil
	}
	return permission.PermissionDeny, nil
}

type agentCWDProvider struct {
	turnIndex      int
	toolResultSeen string
}

func (p *agentCWDProvider) Name() string    { return "mock" }
func (p *agentCWDProvider) ModelID() string { return "mock-model" }

func (p *agentCWDProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	ch := make(chan types.StreamEvent, 16)
	go func() {
		defer close(ch)
		switch p.turnIndex {
		case 0:
			p.turnIndex++
			input, _ := json.Marshal(map[string]any{"file_path": "note.txt"})
			ch <- types.StreamEvent{
				Type:  types.EventContentBlockStart,
				Index: 0,
				ContentBlock: &types.ContentDelta{
					Type: types.ContentTypeToolUse,
					ID:   "read_1",
					Name: "Read",
				},
			}
			ch <- types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: 0,
				Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(input)},
			}
			ch <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
			ch <- types.StreamEvent{Type: types.EventMessageStop}
		default:
			p.turnIndex++
			p.toolResultSeen = extractToolResultText(params.Messages)
			ch <- types.StreamEvent{
				Type:         types.EventContentBlockStart,
				Index:        0,
				ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
			}
			ch <- types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: 0,
				Delta: &types.ContentDelta{Type: "text_delta", Text: "done"},
			}
			ch <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
			ch <- types.StreamEvent{Type: types.EventMessageStop}
		}
	}()
	return ch, nil
}

func extractToolResultText(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		for _, block := range messages[i].Content {
			if result, ok := block.(types.ToolResultBlock); ok {
				return result.Content
			}
		}
	}
	return ""
}

func TestAgentToolResolvesRelativePathsAgainstCWD(t *testing.T) {
	base := t.TempDir()
	subdir := filepath.Join(base, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "note.txt"), []byte("from subdir\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	provider := &agentCWDProvider{}
	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{})

	tool := &AgentTool{
		Provider: provider,
		Registry: reg,
	}
	summary, err := tool.runSubAgentWithOptions(context.Background(), "agent-test-cwd", agentcontract.Input{
		Prompt: "read note.txt",
		CWD:    subdir,
	}, nil, agentLoopOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Output != "done" {
		t.Fatalf("unexpected agent output: %q", summary.Output)
	}
	if !strings.Contains(provider.toolResultSeen, "from subdir") {
		t.Fatalf("expected tool result to include cwd-resolved file contents, got: %q", provider.toolResultSeen)
	}
	if summary.LatestToolUse != "Read" {
		t.Fatalf("latest tool use = %q, want Read", summary.LatestToolUse)
	}
}

type staticAgentRuntimeProvider struct {
	runtime types.ToolRuntimeContext
}

func (p staticAgentRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext { return p.runtime }

func TestAgentRuntimeContextOverlaysChildIdentityAndExecutionScope(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	parent := registry.New()
	parent.SetRuntimeContextProvider(staticAgentRuntimeProvider{runtime: types.ToolRuntimeContext{
		ProjectRoot:    parentRoot,
		AllowedDirs:    []string{parentRoot},
		Interactive:    true,
		AgentID:        "parent-agent",
		PermissionMode: "default",
		Provider:       "openai",
		Model:          "parent-model",
		DeniedTools:    map[string]bool{"Bash": true},
	}})

	snapshot := parent.RuntimeContext()
	snapshot.PermissionMode = "dontAsk"
	runtime := (agentRuntimeContextProvider{
		snapshot: snapshot,
		agentID:  "child-agent",
		cwd:      childRoot,
		model:    "child-model",
	}).ToolRuntimeContext()
	if runtime.AgentID != "child-agent" || runtime.Interactive || runtime.ProjectRoot != childRoot {
		t.Fatalf("child runtime identity/scope = %+v", runtime)
	}
	if runtime.PermissionMode != "dontAsk" || runtime.Model != "child-model" || runtime.Provider != "openai" {
		t.Fatalf("child runtime model/mode/provider = %+v", runtime)
	}
	if !runtime.DeniedTools["Bash"] {
		t.Fatalf("child runtime lost parent denied tools: %+v", runtime.DeniedTools)
	}
	foundChildRoot := false
	for _, allowed := range runtime.AllowedDirs {
		if filepath.Clean(allowed) == filepath.Clean(childRoot) {
			foundChildRoot = true
		}
	}
	if !foundChildRoot {
		t.Fatalf("child runtime allowed dirs = %v, missing cwd", runtime.AllowedDirs)
	}
}

func TestSnapshotAgentProviderKeepsProviderAndModelConsistent(t *testing.T) {
	first := &captureAgentProvider{responses: []string{"first"}}
	second := &captureAgentProvider{responses: []string{"second"}}
	ref := provider.NewProviderRef(first)
	snapshot := snapshotAgentProvider(ref)
	ref.Swap(second)
	if snapshot != first {
		t.Fatalf("provider snapshot = %T %p, want first provider %p", snapshot, snapshot, first)
	}
	if ref.Get() != second {
		t.Fatal("provider ref did not switch independently of child snapshot")
	}
}

func TestAgentCWDWrapperPreservesWriteToolLifecycle(t *testing.T) {
	cwd := t.TempDir()
	reg := registry.New()
	reg.Register(&toolfile.FileWriteTool{AllowedDirs: []string{cwd}})

	wrapRegistryForAgentCWD(reg, cwd)
	wrapped := reg.Get("Write")
	if _, ok := wrapped.(types.ToolMetadataProvider); !ok {
		t.Fatalf("wrapped Write lost ToolMetadata: %T", wrapped)
	}
	if _, ok := wrapped.(types.ToolPermissionChecker); !ok {
		t.Fatalf("wrapped Write lost CheckPermissions: %T", wrapped)
	}
	if _, ok := wrapped.(types.ToolResultMapper); !ok {
		t.Fatalf("wrapped Write lost result mapper: %T", wrapped)
	}
	normalizer, ok := wrapped.(interface {
		NormalizeToolInput(context.Context, map[string]any) (map[string]any, error)
	})
	if !ok {
		t.Fatalf("wrapped Write lost input normalization: %T", wrapped)
	}
	input, err := normalizer.NormalizeToolInput(context.Background(), map[string]any{
		"file_path": "nested/note.txt",
		"content":   "hello",
	})
	if err != nil {
		t.Fatalf("NormalizeToolInput: %v", err)
	}
	if got, want := input["file_path"], filepath.Join(cwd, "nested", "note.txt"); got != want {
		t.Fatalf("normalized file_path = %v, want %q", got, want)
	}
}

func TestAgentCWDClonedBashAllowsChildWorkingDirectory(t *testing.T) {
	parent := t.TempDir()
	child := t.TempDir()
	reg := registry.New()
	reg.Register(&shell.BashTool{CWD: parent, AllowedDirs: []string{parent}, Sandbox: &task07SandboxBackend{}})
	wrapRegistryForAgentCWD(reg, child)
	wrapper, ok := reg.Get("Bash").(*agentCWDBashToolWrapper)
	if !ok {
		t.Fatalf("wrapped Bash = %T", reg.Get("Bash"))
	}
	cloned := wrapper.BashTool
	if cloned.CWD != child {
		t.Fatalf("cloned Bash cwd = %q, want %q", cloned.CWD, child)
	}
	if !cloned.ForceSandbox {
		t.Fatal("isolated Bash did not force sandbox execution")
	}
	found := false
	for _, allowed := range cloned.AllowedDirs {
		if filepath.Clean(allowed) == filepath.Clean(child) {
			found = true
		}
	}
	if !found {
		t.Fatalf("cloned Bash allowed dirs = %v, missing child cwd", cloned.AllowedDirs)
	}
	permission, err := wrapper.CheckPermissions(context.Background(), map[string]any{"command": "date"}, types.ToolPermissionRequest{})
	if err != nil || permission.Behavior != types.PermissionBehaviorPassthrough {
		t.Fatalf("child Bash permission = %+v err=%v, want parent-handler passthrough", permission, err)
	}
}

func TestAgentToolPropagatesPermissionHandler(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "note.txt"), []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	provider := &agentCWDProvider{}
	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{AllowedDirs: []string{base}})
	tool := &AgentTool{
		Provider:          provider,
		Registry:          reg,
		PermissionHandler: denyToolPermissionHandler{tool: "Read"},
	}

	_, err := tool.runSubAgentWithOptions(context.Background(), "agent-permission", agentcontract.Input{
		Prompt: "read note",
		CWD:    base,
	}, nil, agentLoopOptions{})
	if err != nil {
		t.Fatalf("runSubAgent: %v", err)
	}
	if !strings.Contains(provider.toolResultSeen, toolRuntimeFormat(i18n.KeyRuntimePermissionDenied, "Read")) {
		t.Fatalf("expected inherited permission denial, got: %q", provider.toolResultSeen)
	}
}

func TestAgentToolBypassModePropagatesPermissionOverride(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "note.txt"), []byte("allowed\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	provider := &agentCWDProvider{}
	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{AllowedDirs: []string{base}})
	capture := &captureToolPermissionHandler{}
	tool := &AgentTool{
		Provider:          provider,
		Registry:          reg,
		PermissionHandler: capture,
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{SessionID: "parent-session", ProjectRoot: base, AllowedDirs: []string{base}, PermissionMode: "bypassPermissions"}})

	_, err := tool.runSubAgentWithOptions(context.Background(), "agent-permission-bypass", agentcontract.Input{
		Prompt: "read note",
		CWD:    base,
	}, nil, agentLoopOptions{})
	if err != nil {
		t.Fatalf("runSubAgent: %v", err)
	}
	if !strings.Contains(provider.toolResultSeen, "allowed") {
		t.Fatalf("expected bypass mode to allow the read, got: %q", provider.toolResultSeen)
	}
	if len(capture.requests) != 1 {
		t.Fatalf("expected one permission request, got %#v", capture.requests)
	}
	req := capture.requests[0]
	if req.Mode != "bypassPermissions" || req.AvoidPrompts {
		t.Fatalf("expected bypass permission override without prompt suppression, got %#v", req)
	}
}

func TestAgentBypassModeStillHonorsProfileDisallowedRules(t *testing.T) {
	profile := agentProfile{
		Name:                "guarded",
		DisallowedToolRules: toolPermissionRulesFromYAML([]any{"Read"}),
	}
	parent := &captureToolPermissionHandler{}
	handler := agentPermissionHandlerForSnapshot(types.ToolRuntimeContext{SessionID: "parent-session", PermissionMode: "bypassPermissions"}, parent, agentcontract.ApprovalAttached, profile, "parent-session")
	if handler == nil {
		t.Fatal("expected bypass mode to keep a permission handler")
	}
	decision, err := handler.Check(context.Background(), permission.PermissionRequest{
		ToolName: "Read",
		Input:    map[string]any{"file_path": "secret.txt"},
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if decision != permission.PermissionDeny {
		t.Fatalf("expected profile disallowed rule to deny in bypass mode, got %v", decision)
	}
	if len(parent.requests) != 0 {
		t.Fatalf("profile disallowed rule should deny before parent handler, got %#v", parent.requests)
	}
}

func TestAgentDefaultPermissionModePinsRuleBasedParentPolicy(t *testing.T) {
	parent := &captureToolPermissionHandler{}
	handler := agentPermissionHandlerForSnapshot(types.ToolRuntimeContext{SessionID: "parent-session", PermissionMode: "default"}, parent, agentcontract.ApprovalAttached, agentProfile{}, "parent-session")
	if handler == nil {
		t.Fatal("expected permission handler")
	}

	_, err := handler.Check(context.Background(), permission.PermissionRequest{
		ToolName: "Read",
		Input:    map[string]any{"file_path": "go.mod"},
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(parent.requests) != 1 {
		t.Fatalf("expected one parent request, got %#v", parent.requests)
	}
	if parent.requests[0].Mode != "default" {
		t.Fatalf("default child mode should be pinned explicitly, got %#v", parent.requests[0])
	}
}

func TestAgentToolInheritsRegistryPermissionSnapshotWhenSessionRuntimeIsUnset(t *testing.T) {
	root := t.TempDir()
	reg := registry.New()
	reg.SetRuntimeContextProvider(staticAgentRuntimeProvider{runtime: types.ToolRuntimeContext{
		SessionID:      "parent-session",
		ProjectRoot:    root,
		AllowedDirs:    []string{root},
		PermissionMode: "bypassPermissions",
		DeniedTools:    map[string]bool{"Write": true},
	}})
	parent := &captureToolPermissionHandler{}
	tool := &AgentTool{
		Provider:          &captureAgentProvider{},
		Registry:          reg,
		PermissionHandler: parent,
	}
	bundle, err := tool.buildSubAgentLoopWithOptions("agent-snapshot", agentcontract.Input{
		Description: "inherit snapshot",
		Prompt:      "inspect",
	}, agentLoopOptions{Profile: &agentProfile{Name: "general-purpose"}})
	if err != nil {
		t.Fatal(err)
	}
	defer runAgentCleanup(bundle.Cleanup)
	if bundle.PermissionHandler == nil {
		t.Fatal("expected inherited permission handler")
	}
	if _, err := bundle.PermissionHandler.Check(context.Background(), permission.PermissionRequest{ToolName: "Read"}); err != nil {
		t.Fatal(err)
	}
	if len(parent.requests) != 1 || parent.requests[0].Mode != "bypassPermissions" {
		t.Fatalf("child permission request = %#v", parent.requests)
	}
}

func TestBackgroundParentRouteKeepsInheritedPermissionMode(t *testing.T) {
	parent := &captureToolPermissionHandler{}
	handler := agentPermissionHandlerForSnapshot(types.ToolRuntimeContext{PermissionMode: "default"}, parent, agentcontract.ApprovalParentSession, agentProfile{}, "parent-session")
	if handler == nil {
		t.Fatal("expected inherited permission handler")
	}
	_, err := handler.Check(context.Background(), permission.PermissionRequest{
		SessionID: "agent-session", TurnID: "agent-turn", ActorID: "agent-42", ActorType: "executor", WorkUnitID: "work-42",
		ToolName: "Write", Input: map[string]any{"file_path": "out.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(parent.requests) != 1 {
		t.Fatalf("parent requests = %#v", parent.requests)
	}
	request := parent.requests[0]
	if request.SessionID != "parent-session" || request.ExecutionSessionID != "agent-session" || request.ActorID != "agent-42" || request.WorkUnitID != "work-42" || request.AvoidPrompts || request.Mode != "default" {
		t.Fatalf("parent-routed request lost inherited policy or identity: %#v", request)
	}
}

func TestAgentPermissionRulesCannotGrantBeyondParentPolicy(t *testing.T) {
	profile := agentProfile{
		Name:             "json-reviewer",
		AllowedTools:     toolNameSetFromYAML([]any{"Bash(git diff)", "Read"}),
		AllowedToolRules: toolPermissionRulesFromYAML([]any{"Bash(git diff)", "Read"}),
	}
	parent := &modeGatePermissionHandler{}
	handler := agentPermissionHandlerForSnapshot(types.ToolRuntimeContext{SessionID: "parent-session", PermissionMode: "default"}, parent, agentcontract.ApprovalAttached, profile, "parent-session")
	if handler == nil {
		t.Fatal("expected permission handler")
	}

	decision, err := handler.Check(context.Background(), permission.PermissionRequest{
		ToolName: "Bash",
		Input:    map[string]any{"command": "git diff"},
	})
	if err != nil {
		t.Fatalf("Check matching command: %v", err)
	}
	if decision != permission.PermissionDeny {
		t.Fatalf("profile allowed rule granted beyond parent policy: %v", decision)
	}
	if len(parent.requests) != 1 || parent.requests[0].Mode != "default" {
		t.Fatalf("profile allowed rule rewrote inherited policy, got %#v", parent.requests)
	}

	decision, err = handler.Check(context.Background(), permission.PermissionRequest{
		ToolName: "Bash",
		Input:    map[string]any{"command": "git status"},
	})
	if err != nil {
		t.Fatalf("Check nonmatching command: %v", err)
	}
	if decision != permission.PermissionDeny {
		t.Fatalf("expected nonmatching command to follow dontAsk parent denial, got %v", decision)
	}
	if len(parent.requests) != 2 || parent.requests[1].Mode != "default" || parent.requests[1].AvoidPrompts {
		t.Fatalf("expected nonmatching rule to keep inherited default policy, got %#v", parent.requests)
	}
}

func TestAgentPermissionRulesParseEscapedAndPrefixSpecs(t *testing.T) {
	rules := toolPermissionRulesFromYAML([]any{
		`Bash(python -c "print\(1\)")`,
		`Bash(git:*)`,
		`Read`,
	})
	if len(rules) != 3 {
		t.Fatalf("expected three rules, got %#v", rules)
	}
	if rules[0].ToolName != "bash" || rules[0].RuleContent != `python -c "print(1)"` || !rules[0].HasRuleContent {
		t.Fatalf("unexpected escaped rule parse: %#v", rules[0])
	}
	if !shellPermissionRuleMatches(rules[1].RuleContent, "git status --short") {
		t.Fatalf("expected prefix rule to match git subcommand")
	}
	if shellPermissionRuleMatches(rules[1].RuleContent, "gitlab status") {
		t.Fatalf("prefix rule should respect word boundaries")
	}
	if rules[2].ToolName != "read" || rules[2].HasRuleContent {
		t.Fatalf("unexpected tool-wide rule parse: %#v", rules[2])
	}
}

func TestAgentToolWildcardToolsMeansUnrestrictedTools(t *testing.T) {
	allowedTools, allowedRules, allowedSpecs, allowedSpecified := allowedToolProfileFieldsFromYAML([]any{"*", "Read"}, true)
	profile := agentProfile{
		Name:                  "wildcard",
		AllowedTools:          allowedTools,
		AllowedToolRules:      allowedRules,
		AllowedToolSpecs:      allowedSpecs,
		AllowedToolsSpecified: allowedSpecified,
	}
	if profile.AllowedTools != nil || profile.AllowedToolRules != nil {
		t.Fatalf("wildcard tools should mean unrestricted tools, got tools=%#v rules=%#v", profile.AllowedTools, profile.AllowedToolRules)
	}
	if profile.AllowedToolsSpecified {
		t.Fatalf("wildcard tools should not mark the profile as explicitly allowlisted")
	}
	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{})
	reg.Register(&toolfile.FileWriteTool{})
	filtered := registryForAgentProfile(reg, profile)
	if filtered.Get("Read") == nil || filtered.Get("Write") == nil {
		t.Fatalf("expected wildcard profile to keep registered tools")
	}
}

func TestAgentProfileExplicitEmptyToolsDeniesAllTools(t *testing.T) {
	allowedTools, allowedRules, allowedSpecs, allowedSpecified := allowedToolProfileFieldsFromYAML([]string{}, true)
	profile := agentProfile{
		Name:                  "no-tools",
		AllowedTools:          allowedTools,
		AllowedToolRules:      allowedRules,
		AllowedToolSpecs:      allowedSpecs,
		AllowedToolsSpecified: allowedSpecified,
	}
	if !profile.AllowedToolsSpecified {
		t.Fatalf("expected explicit empty tools list to be preserved")
	}
	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{})
	reg.Register(&toolfile.FileWriteTool{})
	reg.Register(fakeTool{name: "Bash"})
	filtered := registryForAgentProfile(reg, profile)
	if filtered.Get("Read") != nil || filtered.Get("Write") != nil || filtered.Get("Bash") != nil {
		t.Fatalf("expected explicit empty tools list to deny all registered tools")
	}
	if got := describeAgentProfileTools(profile); got != toolRuntimeText(i18n.KeyToolAgentProfileNoTools) {
		t.Fatalf("expected no-tools description, got %q", got)
	}
}

func TestAgentScopedAgentToolRestrictsNestedSubagentTypes(t *testing.T) {
	rules := toolPermissionRulesFromYAML([]any{"Agent(Explore)", "Agent(verification)", "Read"})
	allowed := allowedAgentTypesFromRules(rules)
	if strings.Join(allowed, ",") != "Explore,verification" {
		t.Fatalf("unexpected allowed agent types: %#v", allowed)
	}
	commaAllowed := allowedAgentTypesFromRules(toolPermissionRulesFromYAML([]any{"Agent(worker, researcher)", "Agent(Worker)"}))
	if strings.Join(commaAllowed, ",") != "worker,researcher" {
		t.Fatalf("expected comma-separated Agent(...) rule to expand and dedupe, got %#v", commaAllowed)
	}
	tool := &AgentTool{
		Provider:          &captureAgentProvider{responses: []string{"explore done"}},
		Registry:          registry.New(),
		AllowedAgentTypes: allowed,
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("plan work", map[string]any{
		"subagent_type": "Plan",
	}))
	if err != nil {
		t.Fatalf("Execute disallowed nested agent: %v", err)
	}
	want := toolRuntimeFormat(i18n.KeyToolAgentDeepSubagentTypeNotAllowed, "Plan", "Explore, verification")
	if !result.IsError || result.Content != want {
		t.Fatalf("expected scoped Agent restriction, got %#v", result)
	}

	result, err = tool.Execute(context.Background(), agentExecuteInput("explore work", map[string]any{
		"subagent_type": "Explore",
	}))
	if err != nil {
		t.Fatalf("Execute allowed nested agent: %v", err)
	}
	if result.IsError || !strings.Contains(result.Content, "explore done") {
		t.Fatalf("expected allowed nested agent to run, got %#v", result)
	}
}

func TestTeamMemberAgentCannotUseTeamRoutingParameters(t *testing.T) {
	tool := &AgentTool{
		Provider:   &mockProvider{responses: []string{"should not run"}},
		Registry:   registry.New(),
		TeamMember: true,
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("run subagent", map[string]any{
		"team_name": "alpha",
	}))
	if err != nil {
		t.Fatalf("unexpected team routing guard error: %v", err)
	}
	if !result.IsError || result.Content != toolRuntimeText(i18n.KeyToolAgentTeamNameUnavailable) {
		t.Fatalf("expected teammate routing guard, got %#v", result)
	}
}

func TestAgentToolWorktreeIsolationCreatesGitWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repo := t.TempDir()
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "config", "user.email", "test@example.com")
	runGitCommand(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "note.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("secret.env\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	runGitCommand(t, repo, "add", "note.txt", ".gitignore")
	runGitCommand(t, repo, "commit", "-m", "initial")
	if err := os.MkdirAll(filepath.Join(repo, brand.ConfigDirName), 0o755); err != nil {
		t.Fatalf("mkdir config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, brand.ConfigDirName, "settings.local.json"), []byte(`{"permissions":{"allow":["Read"]}}`), 0o644); err != nil {
		t.Fatalf("write settings.local.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".husky"), 0o755); err != nil {
		t.Fatalf("mkdir .husky: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".worktreeinclude"), []byte("secret.env\n"), 0o644); err != nil {
		t.Fatalf("write .worktreeinclude: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "secret.env"), []byte("TOKEN=secret\n"), 0o644); err != nil {
		t.Fatalf("write ignored secret.env: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	defer os.Chdir(oldWD) //nolint:errcheck

	tool := &AgentTool{
		Provider: &mockProvider{responses: []string{"done"}},
		Registry: registry.New(),
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{ProjectRoot: repo, AllowedDirs: []string{repo}, PermissionMode: permissionModeDefault}})
	bundle, err := tool.buildSubAgentLoop("agent-test-worktree", agentcontract.Input{
		Prompt:    "inspect",
		Isolation: "worktree",
	})
	if err != nil {
		t.Fatalf("buildSubAgentLoop: %v", err)
	}
	t.Cleanup(func() {
		_, _ = gitutil.Run(repo, "worktree", "remove", bundle.Metadata.WorktreePath, "--force")
		_, _ = gitutil.Run(repo, "branch", "-D", bundle.Metadata.WorktreeBranch)
	})
	if bundle.Metadata.Isolation != "worktree" {
		t.Fatalf("expected worktree isolation metadata, got %#v", bundle.Metadata)
	}
	if bundle.Metadata.WorktreePath == "" || bundle.Metadata.WorktreeBranch == "" {
		t.Fatalf("expected worktree metadata, got %#v", bundle.Metadata)
	}
	if bundle.Metadata.WorktreeHeadCommit == "" {
		t.Fatalf("expected worktree head commit metadata, got %#v", bundle.Metadata)
	}
	if _, err := os.Stat(bundle.Metadata.WorktreePath); err != nil {
		t.Fatalf("expected worktree path to exist: %v", err)
	}
	wantWorktreeRoot := filepath.Join(storepaths.RuntimeServiceDir(repo, "worktrees"), "agents")
	if !strings.HasPrefix(bundle.Metadata.WorktreePath, wantWorktreeRoot+string(filepath.Separator)) {
		t.Fatalf("expected agent worktree under %s, got %q", wantWorktreeRoot, bundle.Metadata.WorktreePath)
	}
	if !strings.HasPrefix(bundle.Metadata.WorktreeBranch, "luban-agent-") {
		t.Fatalf("expected PRC agent branch prefix, got %q", bundle.Metadata.WorktreeBranch)
	}
	localSettings, err := os.ReadFile(filepath.Join(bundle.Metadata.WorktreePath, brand.ConfigDirName, "settings.local.json"))
	if err != nil {
		t.Fatalf("expected settings.local.json to be copied into worktree: %v", err)
	}
	if !strings.Contains(string(localSettings), `"permissions"`) {
		t.Fatalf("unexpected copied settings.local.json: %s", string(localSettings))
	}
	hooksPath, err := gitutil.Run(bundle.Metadata.WorktreePath, "config", "core.hooksPath")
	if err != nil {
		t.Fatalf("expected worktree core.hooksPath config: %v", err)
	}
	if filepath.Clean(hooksPath) != filepath.Join(repo, ".husky") {
		t.Fatalf("expected worktree hooksPath to point at main repo .husky, got %q", hooksPath)
	}
	includedSecret, err := os.ReadFile(filepath.Join(bundle.Metadata.WorktreePath, "secret.env"))
	if err != nil {
		t.Fatalf("expected .worktreeinclude file to be copied into worktree: %v", err)
	}
	if string(includedSecret) != "TOKEN=secret\n" {
		t.Fatalf("unexpected copied .worktreeinclude file: %q", string(includedSecret))
	}
}

func TestAgentWorktreeCleanupRemovesCleanWorktreeAndBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repo := t.TempDir()
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "config", "user.email", "test@example.com")
	runGitCommand(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "note.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	runGitCommand(t, repo, "add", "note.txt")
	runGitCommand(t, repo, "commit", "-m", "initial")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	defer os.Chdir(oldWD) //nolint:errcheck

	worktree, err := createAgentWorktree("agent-clean-restore", repo)
	if err != nil {
		t.Fatalf("createAgentWorktree: %v", err)
	}
	metadata := agentcontract.SessionMetadata{
		CWD:                worktree.Path,
		Isolation:          "worktree",
		WorktreeRepoRoot:   worktree.RepoRoot,
		WorktreePath:       worktree.Path,
		WorktreeBranch:     worktree.Branch,
		WorktreeHeadCommit: worktree.HeadCommit,
	}
	t.Cleanup(func() {
		_, _ = gitutil.Run(repo, "worktree", "remove", metadata.WorktreePath, "--force")
		_, _ = gitutil.Run(repo, "branch", "-D", metadata.WorktreeBranch)
	})

	removed, err := cleanupAgentWorktreeIfClean(metadata)
	if err != nil {
		t.Fatalf("cleanupAgentWorktreeIfClean: %v", err)
	}
	if !removed {
		t.Fatal("expected clean worktree to be removed")
	}
	if _, err := os.Stat(metadata.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("expected clean worktree to be removed, stat err=%v", err)
	}
	if out, err := gitutil.Run(repo, "rev-parse", "--verify", metadata.WorktreeBranch); err == nil {
		t.Fatalf("expected worktree branch to be deleted, got %q", out)
	}

}

func TestAgentWorktreeCleanupKeepsChangedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repo := t.TempDir()
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "config", "user.email", "test@example.com")
	runGitCommand(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "note.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	runGitCommand(t, repo, "add", "note.txt")
	runGitCommand(t, repo, "commit", "-m", "initial")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	defer os.Chdir(oldWD) //nolint:errcheck

	worktree, err := createAgentWorktree("agent-dirty-keep", repo)
	if err != nil {
		t.Fatalf("createAgentWorktree: %v", err)
	}
	metadata := agentcontract.SessionMetadata{
		CWD:                worktree.Path,
		Isolation:          "worktree",
		WorktreeRepoRoot:   worktree.RepoRoot,
		WorktreePath:       worktree.Path,
		WorktreeBranch:     worktree.Branch,
		WorktreeHeadCommit: worktree.HeadCommit,
	}
	t.Cleanup(func() {
		_, _ = gitutil.Run(repo, "worktree", "remove", metadata.WorktreePath, "--force")
		_, _ = gitutil.Run(repo, "branch", "-D", metadata.WorktreeBranch)
	})
	if err := os.WriteFile(filepath.Join(worktree.Path, "note.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("write changed note: %v", err)
	}

	removed, err := cleanupAgentWorktreeIfClean(metadata)
	if err != nil {
		t.Fatalf("cleanupAgentWorktreeIfClean: %v", err)
	}
	if removed {
		t.Fatal("expected dirty worktree to be kept")
	}
	if _, err := os.Stat(metadata.WorktreePath); err != nil {
		t.Fatalf("expected dirty worktree path to remain: %v", err)
	}
}

func TestAgentToolWorktreeCleanupClearsCleanForegroundMetadata(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repo := t.TempDir()
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "config", "user.email", "test@example.com")
	runGitCommand(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "note.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	runGitCommand(t, repo, "add", "note.txt")
	runGitCommand(t, repo, "commit", "-m", "initial")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	defer os.Chdir(oldWD) //nolint:errcheck

	tool := &AgentTool{
		Provider: &captureAgentProvider{responses: []string{"done"}},
		Registry: registry.New(),
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{ProjectRoot: repo, AllowedDirs: []string{repo}, PermissionMode: permissionModeDefault}})
	summary, err := tool.runSubAgentWithOptions(context.Background(), "agent-clean-run", agentcontract.Input{
		Prompt:    "inspect",
		Isolation: "worktree",
	}, nil, agentLoopOptions{})
	if err != nil {
		t.Fatalf("runSubAgent: %v", err)
	}
	expectedPath := filepath.Join(storepaths.RuntimeServiceDir(repo, "worktrees"), "agents", "agent-clean-run")
	expectedBranch := "agent-agent-clean-run"
	t.Cleanup(func() {
		_, _ = gitutil.Run(repo, "worktree", "remove", expectedPath, "--force")
		_, _ = gitutil.Run(repo, "branch", "-D", expectedBranch)
	})
	if summary.WorktreePath != "" || summary.WorktreeBranch != "" || summary.CWD != "" {
		t.Fatalf("expected clean worktree metadata to be cleared, got %#v", summary)
	}
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Fatalf("expected clean foreground worktree to be removed, stat err=%v", err)
	}
	if out, err := gitutil.Run(repo, "rev-parse", "--verify", expectedBranch); err == nil {
		t.Fatalf("expected foreground worktree branch to be deleted, got %q", out)
	}
}

func TestAgentToolWorktreeCleanupClearsBackgroundSessionMetadata(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repo := t.TempDir()
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "config", "user.email", "test@example.com")
	runGitCommand(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "note.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	runGitCommand(t, repo, "add", "note.txt")
	runGitCommand(t, repo, "commit", "-m", "initial")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	defer os.Chdir(oldWD) //nolint:errcheck

	background := NewBackgroundTaskManager(repo)
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	tool := &AgentTool{
		Provider:   &captureAgentProvider{responses: []string{"done"}},
		Registry:   registry.New(),
		Background: background,
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{ProjectRoot: repo, AllowedDirs: []string{repo}, PermissionMode: permissionModeDefault}})
	summary, err := tool.runSubAgentWithOptions(context.Background(), "agent-bg-clean", agentcontract.Input{
		Prompt:    "inspect",
		Isolation: "worktree",
	}, nil, agentLoopOptions{})
	if err != nil {
		t.Fatalf("runSubAgent: %v", err)
	}
	if summary.WorktreePath != "" || summary.WorktreeBranch != "" || summary.CWD != "" {
		t.Fatalf("expected clean background worktree metadata to be cleared, got %#v", summary)
	}
	record, ok := background.store.Get("agent-bg-clean")
	if !ok {
		t.Fatal("expected background agent record")
	}
	if record.AgentMetadata == nil {
		t.Fatal("expected persisted agent metadata")
	}
	if record.AgentMetadata.WorktreePath != "" || record.AgentMetadata.WorktreeBranch != "" || record.AgentMetadata.CWD != "" {
		t.Fatalf("expected persisted metadata to clear deleted worktree, got %#v", record.AgentMetadata)
	}
	expectedPath := filepath.Join(storepaths.RuntimeServiceDir(repo, "worktrees"), "agents", "agent-bg-clean")
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Fatalf("expected clean background worktree to be removed, stat err=%v", err)
	}
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}
