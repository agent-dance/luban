package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type enterPlanModeAlignmentWriteTool struct{}

func (enterPlanModeAlignmentWriteTool) Name() string        { return "Write" }
func (enterPlanModeAlignmentWriteTool) Description() string { return "test write" }
func (enterPlanModeAlignmentWriteTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}
func (enterPlanModeAlignmentWriteTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: "wrote"}, nil
}

func runEnterPlanModeAlignment(t *testing.T) (*PlanState, string, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Chdir(tmp)

	state := NewPlanState(tmp)
	tool := NewEnterPlanModeTool(state)
	res, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("EnterPlanMode.Execute infra error: %v", err)
	}
	if res.IsError {
		t.Fatalf("EnterPlanMode.Execute returned error: %s", res.Content)
	}
	return state, res.Content, state.PlanFile()
}

func TestEnterPlanModeAlignment_SchemaIsStrictEmptyObject(t *testing.T) {
	tool := NewEnterPlanModeTool(NewPlanState(t.TempDir()))
	schema := tool.Schema()
	if schema.Type != "object" {
		t.Fatalf("Schema.Type = %q, want object", schema.Type)
	}
	if len(schema.Properties) != 0 {
		t.Fatalf("EnterPlanMode should expose no input properties, got %#v", schema.Properties)
	}
	if !schema.RejectsUnknownFields() {
		t.Fatalf("EnterPlanMode schema must reject unknown fields")
	}
}

func TestEnterPlanModeAlignment_RejectsUnknownInputBeforeExecution(t *testing.T) {
	state := NewPlanState(t.TempDir())
	tool := NewEnterPlanModeTool(state)
	reg := registry.New()
	reg.Register(tool)

	result := reg.ExecuteTool(context.Background(), "EnterPlanMode", map[string]any{"plan": "do it"})
	if !result.IsError {
		t.Fatalf("expected strict input validation error, got %#v", result)
	}
	if state.IsActive() {
		t.Fatal("invalid EnterPlanMode input must not activate plan mode")
	}
	if !strings.Contains(result.Content, "InputValidationError") ||
		!strings.Contains(result.Content, "unexpected parameter `plan`") {
		t.Fatalf("unexpected validation message: %q", result.Content)
	}
}

func TestEnterPlanModeAlignment_OutputSchemaIsMessageOnly(t *testing.T) {
	tool := NewEnterPlanModeTool(NewPlanState(t.TempDir()))
	contract := tool.ToolContract()
	if contract.OutputSchema == nil {
		t.Fatal("expected EnterPlanMode output schema")
	}
	if len(contract.OutputSchema.Properties) != 1 {
		t.Fatalf("expected message-only output schema, got %#v", contract.OutputSchema.Properties)
	}
	if _, ok := contract.OutputSchema.Properties["message"]; !ok {
		t.Fatalf("expected output schema to contain only message, got %#v", contract.OutputSchema.Properties)
	}
	if !contract.OutputSchema.RejectsUnknownFields() {
		t.Fatal("output schema should reject extra fields")
	}
}

func TestEnterPlanModeAlignment_DirectResultIsMessageOnly(t *testing.T) {
	state, content, planFile := runEnterPlanModeAlignment(t)
	if !state.IsActive() {
		t.Fatal("plan mode should be active after EnterPlanMode")
	}
	if !strings.Contains(content, "Entered plan mode") {
		t.Fatalf("expected TS confirmation message, got %q", content)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(content), &decoded); err == nil {
		t.Fatalf("EnterPlanMode direct content should not be Go-only JSON, got %#v", decoded)
	}
	if strings.Contains(content, planFile) || strings.Contains(content, "plan_path") || strings.Contains(content, "request_id") {
		t.Fatalf("EnterPlanMode content leaked internal state: %q", content)
	}
}

func TestEnterPlanModeAlignment_TypedDataIsExactMessageOnlyFixture(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	tool := NewEnterPlanModeTool(NewPlanState(tmp))
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("EnterPlanMode.Execute: err=%v result=%#v", err, result)
	}
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("marshal typed data: %v", err)
	}
	want := `{"message":"Entered plan mode. You should now focus on exploring the codebase and designing an implementation approach."}`
	if string(encoded) != want {
		t.Fatalf("typed result = %s, want %s", encoded, want)
	}
}

func TestEnterPlanModeAlignment_DoesNotCreatePlanFileOnEntry(t *testing.T) {
	_, _, planFile := runEnterPlanModeAlignment(t)
	if strings.TrimSpace(planFile) == "" {
		t.Fatal("expected internal future plan path for Go ExitPlanMode compatibility")
	}
	if _, err := os.Stat(planFile); err == nil {
		t.Fatalf("EnterPlanMode should not create plan file on entry: %s", planFile)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat internal plan path: %v", err)
	}
}

func TestEnterPlanModeAlignment_RegistryMapsModelFacingInstructions(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	reg := registry.New()
	reg.Register(NewEnterPlanModeTool(NewPlanState(tmp)))

	result := reg.ExecuteTool(context.Background(), "EnterPlanMode", map[string]any{})
	if result.IsError {
		t.Fatalf("EnterPlanMode registry execution failed: %s", result.Content)
	}
	for _, want := range []string{
		"Entered plan mode. You should now focus on exploring the codebase and designing an implementation approach.",
		"In plan mode, you should:",
		"Use AskUserQuestion if you need to clarify the approach",
		"When ready, use ExitPlanMode to present your plan for approval",
		"Remember: DO NOT write or edit any files yet.",
	} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("mapped model content missing %q:\n%s", want, result.Content)
		}
	}
	if strings.Contains(result.Content, "plan_path") || strings.Contains(result.Content, "request_id") ||
		strings.Contains(result.Content, ".md") {
		t.Fatalf("mapped model content leaked Go-only state:\n%s", result.Content)
	}
}

func TestEnterPlanModeAlignment_ChannelAndAgentContextsAreUnavailable(t *testing.T) {
	channelActive := true
	SetAskUserChannelsActiveForTest(&channelActive)
	t.Cleanup(func() { SetAskUserChannelsActiveForTest(nil) })

	channelScope := NewRuntimeScope(t.TempDir(), true)
	channelRegistry := registry.New()
	channelRegistry.SetRuntimeContextProvider(channelScope)
	channelTool := NewEnterPlanModeTool(NewPlanState(t.TempDir()), channelScope)
	channelRegistry.Register(channelTool)
	if channelRegistry.IsToolEnabled(channelTool) {
		t.Fatal("EnterPlanMode must be hidden while a remote channel is active")
	}
	result, err := channelTool.Execute(context.Background(), map[string]any{})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "remote-channel") {
		t.Fatalf("direct channel-context call should be rejected: err=%v result=%#v", err, result)
	}

	channelActive = false
	SetAskUserChannelsActiveForTest(&channelActive)
	agentScope := NewRuntimeScope(t.TempDir(), true)
	agentScope.SetAgentIDFunc(func() string { return "agent-123" })
	agentRegistry := registry.New()
	agentRegistry.SetRuntimeContextProvider(agentScope)
	agentTool := NewEnterPlanModeTool(NewPlanState(t.TempDir()), agentScope)
	agentRegistry.Register(agentTool)
	if agentRegistry.IsToolEnabled(agentTool) {
		t.Fatal("EnterPlanMode must not be exposed in an agent context")
	}
	_, err = agentTool.Execute(context.Background(), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "cannot be used in agent contexts") {
		t.Fatalf("direct agent-context call should return the TS error, got %v", err)
	}
}

func TestEnterPlanModeAlignment_RemainsAvailableNonInteractive(t *testing.T) {
	scope := NewRuntimeScope(t.TempDir(), false)
	tool := NewEnterPlanModeTool(NewPlanState(t.TempDir()), scope)
	reg := registry.New()
	reg.SetRuntimeContextProvider(scope)
	reg.Register(tool)
	if !reg.IsToolEnabled(tool) {
		t.Fatal("TS EnterPlanMode has no non-interactive isEnabled gate")
	}
}

func TestEnterPlanModeAlignment_TransitionsRuntimeAndRestoresPrePlanMode(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	mode := "bypassPermissions"
	var transitions []string
	scope := NewRuntimeScope(root, true)
	scope.SetPermissionModeDispatcher(
		func() string { return mode },
		func(next string) error {
			transitions = append(transitions, next)
			mode = next
			return nil
		},
	)
	state := NewPlanState(root)
	tool := NewEnterPlanModeTool(state, scope)
	reg := registry.New()
	reg.SetRuntimeContextProvider(scope)
	reg.Register(tool)
	reg.Register(enterPlanModeAlignmentWriteTool{})

	result := reg.ExecuteTool(context.Background(), "EnterPlanMode", map[string]any{})
	if result.IsError {
		t.Fatalf("EnterPlanMode failed: %s", result.Content)
	}
	if got := scope.ToolRuntimeContext().PermissionMode; got != "plan" {
		t.Fatalf("runtime permission mode = %q, want plan", got)
	}
	snapshot := state.PrePlanState()
	if snapshot["permission_mode"] != "bypassPermissions" || snapshot["auto_mode"] != true {
		t.Fatalf("pre-plan snapshot = %#v", snapshot)
	}
	decision, err := reg.CheckToolPermissions(context.Background(), "Write", map[string]any{}, types.ToolPermissionRequest{})
	if err != nil || decision.Behavior != types.PermissionBehaviorDeny || !strings.Contains(decision.Message, "plan mode") {
		t.Fatalf("central plan permission gate did not block Write: err=%v decision=%#v", err, decision)
	}
	if err := os.WriteFile(state.PlanFile(), []byte("# Approved plan"), 0o600); err != nil {
		t.Fatalf("materialize plan before approval: %v", err)
	}

	exitResult, err := executeApprovedToolForTest(t, NewExitPlanModeTool(state), map[string]any{})
	if err != nil || exitResult.IsError {
		t.Fatalf("ExitPlanMode failed to hand off captured state: err=%v result=%#v", err, exitResult)
	}
	if mode != "bypassPermissions" || scope.ToolRuntimeContext().PermissionMode != "bypassPermissions" {
		t.Fatalf("plan exit did not restore pre-plan mode: dispatcher=%q runtime=%q", mode, scope.ToolRuntimeContext().PermissionMode)
	}
	if len(transitions) != 2 || transitions[0] != "plan" || transitions[1] != "bypassPermissions" {
		t.Fatalf("permission transitions = %#v", transitions)
	}
	if state.PrePlanState() != nil {
		t.Fatalf("pre-plan snapshot must be consumed on exit: %#v", state.PrePlanState())
	}
}

func TestEnterPlanModeAlignment_TransitionFailureDoesNotActivate(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	scope := NewRuntimeScope(root, true)
	scope.SetPermissionModeDispatcher(
		func() string { return "default" },
		func(string) error { return errors.New("dispatcher unavailable") },
	)
	state := NewPlanState(root)
	result, err := NewEnterPlanModeTool(state, scope).Execute(context.Background(), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "dispatcher unavailable") {
		t.Fatalf("expected dispatcher failure, got err=%v result=%#v", err, result)
	}
	if state.IsActive() || state.PrePlanState() != nil {
		t.Fatalf("failed transition mutated plan state: active=%v snapshot=%#v", state.IsActive(), state.PrePlanState())
	}
}

func TestEnterPlanModeAlignment_DuplicateEntryDoesNotLeakInternalPlanPath(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	state := NewPlanState(root)
	tool := NewEnterPlanModeTool(state)
	first, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil || first.IsError {
		t.Fatalf("first EnterPlanMode: err=%v result=%#v", err, first)
	}
	duplicate, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil || !duplicate.IsError || !strings.Contains(duplicate.Content, "already in plan mode") {
		t.Fatalf("duplicate EnterPlanMode: err=%v result=%#v", err, duplicate)
	}
	if strings.Contains(duplicate.Content, state.PlanFile()) || strings.Contains(duplicate.Content, ".md") {
		t.Fatalf("duplicate result leaked internal plan path: %q", duplicate.Content)
	}
}

func TestEnterPlanModeAlignment_ActiveStateResumesWithoutMaterializedPlan(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	scope := NewRuntimeScope(root, true)
	scope.SetPermissionModeDispatcher(func() string { return "default" }, func(string) error { return nil })
	state := NewPlanState(root)
	result, err := NewEnterPlanModeTool(state, scope).Execute(context.Background(), map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("EnterPlanMode: err=%v result=%#v", err, result)
	}
	if _, err := os.Stat(state.PlanFile()); !os.IsNotExist(err) {
		t.Fatalf("expected no materialized plan, stat err=%v", err)
	}

	resumed := NewPlanState(root)
	if !resumed.IsActive() || resumed.PlanFile() != state.PlanFile() {
		t.Fatalf("active plan mode was lost on resume: active=%v path=%q want=%q", resumed.IsActive(), resumed.PlanFile(), state.PlanFile())
	}
	if resumed.PrePlanState()["permission_mode"] != "default" {
		t.Fatalf("pre-plan state was lost on resume: %#v", resumed.PrePlanState())
	}
}

func TestEnterPlanModeAlignment_PromptAndDiscoveryMetadataMatchTS(t *testing.T) {
	t.Setenv("USER_TYPE", "")
	tool := NewEnterPlanModeTool(NewPlanState(t.TempDir()))
	for _, want := range []string{
		"Use this tool proactively when you're about to start a non-trivial implementation task.",
		"## When to Use This Tool",
		"## What Happens in Plan Mode",
		"This tool REQUIRES user approval",
	} {
		if !strings.Contains(tool.Description(), want) {
			t.Fatalf("EnterPlanMode prompt missing %q", want)
		}
	}
	metadata := registry.DiscoveryMetadata(tool)
	if !metadata.ShouldDefer || metadata.SearchHint != "switch to plan mode to design an approach before coding" {
		t.Fatalf("EnterPlanMode discovery metadata = %#v", metadata)
	}
}

func TestEnterPlanModeAlignment_InterviewVariantUsesShortResultAndAntPrompt(t *testing.T) {
	t.Setenv("USER_TYPE", "ant")
	tool := NewEnterPlanModeTool(NewPlanState(t.TempDir()))
	if !strings.Contains(tool.Description(), "genuine ambiguity") || strings.Contains(tool.Description(), "## What Happens in Plan Mode") {
		t.Fatalf("unexpected ant/interview prompt:\n%s", tool.Description())
	}
	block := tool.MapToolResultToToolResultBlock(enterPlanModeResult{Message: enterPlanModeMessage}, "toolu_plan")
	if !strings.Contains(block.Content, "Detailed workflow instructions will follow") || strings.Contains(block.Content, "In plan mode, you should:") {
		t.Fatalf("unexpected interview-phase model result:\n%s", block.Content)
	}
}
