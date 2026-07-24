package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func executeApprovedToolForTest(t *testing.T, tool types.Tool, input map[string]any) (types.ToolResultBlock, error) {
	t.Helper()
	reg := registry.New()
	reg.Register(tool)
	request := types.ToolPermissionRequest{
		SessionID: "test-session", TurnID: "turn", ToolUseID: "test-tool-use", ApprovalEpoch: "test-epoch",
	}
	permission, err := reg.CheckToolPermissions(context.Background(), tool.Name(), input, request)
	if err != nil {
		return types.ToolResultBlock{}, err
	}
	policyCode := permission.ExecutionPolicyCode
	if permission.PolicyDecision != nil {
		if policyCode == "" {
			policyCode = permission.PolicyDecision.Code
		}
	}
	if permission.UpdatedInput != nil {
		input = permission.UpdatedInput
	}
	executionGrant := reg.AuthorizePermissionGrant(
		permission.PermissionGrant, tool.Name(), input, permission.PermissionBinding, policyCode,
	)
	ctx := approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: executionGrant, Binding: permission.PermissionBinding, PolicyCode: policyCode,
	})
	return reg.ExecuteToolWithError(ctx, tool.Name(), input)
}

// ─── PlanState unit tests ─────────────────────────────────────────────────────

func TestPlanState_InitialState(t *testing.T) {
	s := NewPlanState()
	if s.IsActive() {
		t.Error("new PlanState should not be active")
	}
	if s.PlanFile() != "" {
		t.Errorf("new PlanState PlanFile should be empty, got %q", s.PlanFile())
	}
}

func TestPlanState_Enter(t *testing.T) {
	s := NewPlanState()
	s.enter("/tmp/plan.md")
	if !s.IsActive() {
		t.Error("PlanState should be active after enter()")
	}
	if s.PlanFile() != "/tmp/plan.md" {
		t.Errorf("PlanFile = %q, want %q", s.PlanFile(), "/tmp/plan.md")
	}
}

func TestPlanState_Exit(t *testing.T) {
	s := NewPlanState()
	s.enter("/tmp/plan.md")
	s.exit()
	if s.IsActive() {
		t.Error("PlanState should not be active after exit()")
	}
	if s.PlanFile() != "" {
		t.Errorf("PlanFile should be empty after exit(), got %q", s.PlanFile())
	}
}

func TestPlanState_ExitWithoutEnter(t *testing.T) {
	s := NewPlanState()
	// Should not panic
	s.exit()
	if s.IsActive() {
		t.Error("PlanState should remain inactive")
	}
}

func TestPlanState_PersistsToDisk(t *testing.T) {
	tmp := t.TempDir()
	s := NewPlanState(tmp)
	planFile := filepath.Join(tmp, ".claude", "plans", "test.md")
	if err := os.MkdirAll(filepath.Dir(planFile), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(planFile, []byte("# plan"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s.enter(planFile)
	s.SetAllowedPrompts([]PlanAllowedPrompt{{Tool: "Bash", Prompt: "run tests"}})

	loaded := NewPlanState(tmp)
	if !loaded.IsActive() {
		t.Fatal("expected persisted plan state to reload as active")
	}
	if loaded.PlanFile() != planFile {
		t.Fatalf("PlanFile = %q, want %q", loaded.PlanFile(), planFile)
	}
	prompts := loaded.AllowedPrompts()
	if len(prompts) != 1 || prompts[0].Prompt != "run tests" {
		t.Fatalf("unexpected allowed prompts: %#v", prompts)
	}
}

// ─── EnterPlanModeTool tests ──────────────────────────────────────────────────

func TestEnterPlanModeTool_Name(t *testing.T) {
	tool := NewEnterPlanModeTool(NewPlanState())
	if tool.Name() != "EnterPlanMode" {
		t.Errorf("Name() = %q, want EnterPlanMode", tool.Name())
	}
}

func TestEnterPlanModeTool_Schema(t *testing.T) {
	tool := NewEnterPlanModeTool(NewPlanState())
	schema := tool.Schema()
	if schema.Type != "object" {
		t.Errorf("Schema.Type = %q, want object", schema.Type)
	}
}

func TestEnterPlanModeTool_SetsActiveState(t *testing.T) {
	// Run from a temp dir so the plan file is created there.
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	state := NewPlanState()
	tool := NewEnterPlanModeTool(state)

	res, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}

	if !state.IsActive() {
		t.Error("plan mode should be active after EnterPlanMode")
	}
	if state.PlanFile() == "" {
		t.Error("plan file path should be set after EnterPlanMode")
	}
}

func TestEnterPlanModeTool_DoesNotCreatePlanFile(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	state := NewPlanState()
	tool := NewEnterPlanModeTool(state)

	res, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}

	planFile := state.PlanFile()
	if planFile == "" {
		t.Fatal("internal future plan file path should be set after EnterPlanMode")
	}
	if _, err := os.Stat(planFile); err == nil {
		t.Fatalf("EnterPlanMode should not create plan file on disk: %s", planFile)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat plan file: %v", err)
	}
}

func TestEnterPlanModeTool_ResponseDoesNotMentionPlanFile(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	state := NewPlanState()
	tool := NewEnterPlanModeTool(state)

	res, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if strings.Contains(res.Content, state.PlanFile()) {
		t.Errorf("response should not mention internal plan file path %q:\n%s", state.PlanFile(), res.Content)
	}
	if !strings.Contains(res.Content, "Entered plan mode") {
		t.Errorf("response should contain TS confirmation message, got:\n%s", res.Content)
	}
}

func TestEnterPlanModeTool_RejectsDuplicateEnter(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	state := NewPlanState(tmp)
	tool := NewEnterPlanModeTool(state)

	if _, err := tool.Execute(context.Background(), nil); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	res, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected duplicate enter to fail, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "already in plan mode") {
		t.Fatalf("unexpected duplicate-enter message: %s", res.Content)
	}
}

// ─── ExitPlanModeTool tests ───────────────────────────────────────────────────

func TestExitPlanModeTool_Name(t *testing.T) {
	tool := NewExitPlanModeTool(NewPlanState())
	if tool.Name() != "ExitPlanMode" {
		t.Errorf("Name() = %q, want ExitPlanMode", tool.Name())
	}
}

func TestExitPlanModeTool_Schema(t *testing.T) {
	tool := NewExitPlanModeTool(NewPlanState())
	schema := tool.Schema()
	if schema.Type != "object" {
		t.Errorf("Schema.Type = %q, want object", schema.Type)
	}
}

func TestExitPlanModeTool_ErrorWhenNotActive(t *testing.T) {
	state := NewPlanState()
	tool := NewExitPlanModeTool(state)

	res, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError {
		t.Error("ExitPlanMode should return error when not in plan mode")
	}
	if !strings.Contains(res.Content, "not in plan mode") {
		t.Errorf("error message should mention not in plan mode: %q", res.Content)
	}
}

func TestExitPlanModeTool_ClearsActiveState(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	state := NewPlanState()
	enter := NewEnterPlanModeTool(state)
	exit := NewExitPlanModeTool(state)

	if _, err := enter.Execute(context.Background(), nil); err != nil {
		t.Fatalf("EnterPlanMode: %v", err)
	}
	if err := os.WriteFile(state.PlanFile(), nil, 0o600); err != nil {
		t.Fatalf("materialize empty plan: %v", err)
	}

	res, err := executeApprovedToolForTest(t, exit, nil)
	if err != nil {
		t.Fatalf("ExitPlanMode: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}

	if state.IsActive() {
		t.Error("plan mode should be inactive after ExitPlanMode")
	}
	if state.PlanFile() != "" {
		t.Errorf("PlanFile should be empty after ExitPlanMode, got %q", state.PlanFile())
	}
}

func TestExitPlanModeTool_ReturnsPlanContent(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	state := NewPlanState()
	enter := NewEnterPlanModeTool(state)
	exit := NewExitPlanModeTool(state)

	if _, err := enter.Execute(context.Background(), nil); err != nil {
		t.Fatalf("EnterPlanMode: %v", err)
	}

	// Write some plan content to the plan file.
	planContent := "## My Plan\n1. Do thing A\n2. Do thing B\n"
	if err := os.WriteFile(state.PlanFile(), []byte(planContent), 0644); err != nil {
		t.Fatalf("writing plan file: %v", err)
	}

	res, err := executeApprovedToolForTest(t, exit, nil)
	if err != nil {
		t.Fatalf("ExitPlanMode: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}

	if !strings.Contains(res.Content, "Do thing A") {
		t.Errorf("response should contain plan content, got:\n%s", res.Content)
	}
}

func TestExitPlanModeTool_IncludesAllowedPromptsSummary(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	state := NewPlanState(tmp)
	enter := NewEnterPlanModeTool(state)
	exit := NewExitPlanModeTool(state)

	if _, err := enter.Execute(context.Background(), nil); err != nil {
		t.Fatalf("EnterPlanMode: %v", err)
	}
	if err := os.WriteFile(state.PlanFile(), []byte("ship it"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	res, err := executeApprovedToolForTest(t, exit, map[string]any{
		"allowedPrompts": []any{
			map[string]any{"tool": "Bash", "prompt": "run tests"},
			map[string]any{"tool": "Bash", "prompt": "run tests"},
		},
	})
	if err != nil {
		t.Fatalf("ExitPlanMode: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	approved := state.AllowedPrompts()
	if len(approved) != 1 || !state.AllowedPromptMatches("Bash", "run tests --all") {
		t.Fatalf("approved prompts were not deduplicated and retained: %#v", approved)
	}
}

func TestExitPlanModeTool_EmptyPlanFile(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	state := NewPlanState()
	enter := NewEnterPlanModeTool(state)
	exit := NewExitPlanModeTool(state)

	if _, err := enter.Execute(context.Background(), nil); err != nil {
		t.Fatalf("EnterPlanMode: %v", err)
	}
	if err := os.WriteFile(state.PlanFile(), nil, 0o600); err != nil {
		t.Fatalf("materialize empty plan: %v", err)
	}

	// Don't write anything — plan file is empty.
	res, err := executeApprovedToolForTest(t, exit, nil)
	if err != nil {
		t.Fatalf("ExitPlanMode: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "approved exiting plan mode") {
		t.Errorf("empty plan should produce informative message, got:\n%s", res.Content)
	}
}

// ─── Round-trip integration test ──────────────────────────────────────────────

func TestPlanMode_EnterExitRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	state := NewPlanState()
	enter := NewEnterPlanModeTool(state)
	exit := NewExitPlanModeTool(state)
	ctx := context.Background()

	// 1. Enter plan mode
	enterRes, err := enter.Execute(ctx, nil)
	if err != nil || enterRes.IsError {
		t.Fatalf("EnterPlanMode failed: err=%v isError=%v msg=%s", err, enterRes.IsError, enterRes.Content)
	}
	if !state.IsActive() {
		t.Fatal("plan mode should be active")
	}

	// 2. Simulate writing a plan
	plan := "# Implementation Plan\n\nStep 1: analyse.\nStep 2: implement.\n"
	if err := os.WriteFile(state.PlanFile(), []byte(plan), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	// 3. Exit plan mode
	exitRes, err := executeApprovedToolForTest(t, exit, nil)
	if err != nil || exitRes.IsError {
		t.Fatalf("ExitPlanMode failed: err=%v isError=%v msg=%s", err, exitRes.IsError, exitRes.Content)
	}
	if state.IsActive() {
		t.Fatal("plan mode should be inactive after exit")
	}
	if !strings.Contains(exitRes.Content, "Step 1") {
		t.Errorf("exit response should contain plan content, got:\n%s", exitRes.Content)
	}

	// 4. Calling ExitPlanMode again should be an error
	res2, _ := exit.Execute(ctx, nil)
	if !res2.IsError {
		t.Error("second ExitPlanMode should return error (not in plan mode)")
	}
}

// ─── Plan mode blocks write tools ────────────────────────────────────────────

// TestPlanMode_BlocksFileWrite verifies that FileWriteTool rejects execution
// while plan mode is active.
func TestPlanMode_BlocksFileWrite(t *testing.T) {
	ps := NewPlanState()
	ps.enter("/tmp/plan.md")

	tool := &FileWriteTool{PlanState: ps}
	result, err := tool.Execute(context.Background(), map[string]any{
		"file_path": "/tmp/plan-mode-should-not-be-created.txt",
		"content":   "blocked",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true but got false; content=%v", result.Content)
	}

	// Confirm the file was NOT created.
	if _, statErr := os.Stat("/tmp/plan-mode-should-not-be-created.txt"); statErr == nil {
		os.Remove("/tmp/plan-mode-should-not-be-created.txt")
		t.Error("file was created despite plan mode being active")
	}
}

// TestPlanMode_BlocksBash verifies that BashTool rejects execution while plan
// mode is active.
func TestPlanMode_BlocksBash(t *testing.T) {
	ps := NewPlanState()
	ps.enter("/tmp/plan.md")

	tool := &BashTool{PlanState: ps}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true but got false; content=%v", result.Content)
	}
}

// TestPlanMode_AllowsFileRead verifies that FileReadTool is unaffected by plan
// mode — it has no PlanState field and should always succeed for a valid file.
func TestPlanMode_AllowsFileRead(t *testing.T) {
	// Write a temp file to read back.
	tmpFile, err := os.CreateTemp("", "plan-mode-read-test-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString("hello plan mode"); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	// FileReadTool has no PlanState — it must succeed even when plan mode is on.
	tool := &FileReadTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"file_path": tmpFile.Name(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false but got true; content=%v", result.Content)
	}
}
