package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func task14ActivePlanState(t *testing.T, plan string) (*PlanState, string) {
	t.Helper()
	root := t.TempDir()
	planPath := filepath.Join(root, ".claude", "plans", "task14.md")
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(planPath, []byte(plan), 0o600); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	state := NewPlanState(root)
	if err := state.enterWithSnapshot(planPath, map[string]any{"permission_mode": "default"}); err != nil {
		t.Fatalf("enter plan state: %v", err)
	}
	return state, planPath
}

// TS ref: src/tools/ExitPlanModeTool/ExitPlanModeV2Tool.ts:221-238.
func TestExitPlanModeTask14_LocalExitRequiresApprovalBeforeMutation(t *testing.T) {
	state, planPath := task14ActivePlanState(t, "# Plan\n\nImplement it.")
	tool := NewExitPlanModeTool(state)
	checker, ok := any(tool).(types.ToolPermissionChecker)
	if !ok {
		t.Fatal("ExitPlanMode must implement ToolPermissionChecker")
	}
	decision, err := checker.CheckPermissions(context.Background(), map[string]any{}, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatalf("CheckPermissions: %v", err)
	}
	if decision.Behavior != types.PermissionBehaviorAsk || decision.Message != toolPermissionText(i18n.KeyToolPermissionExitPlanConfirm) {
		t.Fatalf("decision = %#v, want ask with TS prompt", decision)
	}
	if !state.IsActive() || state.PlanFile() != planPath {
		t.Fatalf("permission preparation mutated plan state: active=%v path=%q", state.IsActive(), state.PlanFile())
	}
}

// TS ref: src/components/permissions/ExitPlanModePermissionRequest/ExitPlanModePermissionRequest.tsx:475-507.
func TestExitPlanModeTask14_RejectionPreservesPlanAndSurfacesFeedback(t *testing.T) {
	state, planPath := task14ActivePlanState(t, "# Plan\n\nNeeds review.")
	tool := NewExitPlanModeTool(state)
	mapper, ok := any(tool).(types.ToolPermissionRejectionMapper)
	if !ok {
		t.Fatal("ExitPlanMode must map rejected approval results")
	}
	input := map[string]any{"feedback": "Add rollback coverage"}
	result := mapper.MapToolPermissionRejection(input, "toolu_exit", "")
	if !result.IsError || !strings.Contains(result.Content, "Add rollback coverage") {
		t.Fatalf("rejection result lost feedback: %#v", result)
	}
	if !state.IsActive() || state.PlanFile() != planPath {
		t.Fatalf("rejection mutated plan state: active=%v path=%q", state.IsActive(), state.PlanFile())
	}
}

// TS ref: src/components/permissions/ExitPlanModePermissionRequest/ExitPlanModePermissionRequest.tsx:199-212.
func TestExitPlanModeTask14_UnreadablePlanFailsClosed(t *testing.T) {
	state, planPath := task14ActivePlanState(t, "# Plan")
	if err := os.Remove(planPath); err != nil {
		t.Fatalf("remove plan: %v", err)
	}
	tool := NewExitPlanModeTool(state)
	checker, ok := any(tool).(types.ToolPermissionChecker)
	if !ok {
		t.Fatal("ExitPlanMode must validate the plan before approval")
	}
	_, err := checker.CheckPermissions(context.Background(), map[string]any{}, types.ToolPermissionRequest{})
	if err == nil {
		t.Fatal("missing plan must return a strict read error")
	}
	if !state.IsActive() || state.PlanFile() != planPath {
		t.Fatalf("read failure cleared plan mode: active=%v path=%q", state.IsActive(), state.PlanFile())
	}
}

// TS ref: src/tools/ExitPlanModeTool/ExitPlanModeV2Tool.ts:110-141,419-491.
func TestExitPlanModeTask14_TypedOutputAndApprovedPlanMapper(t *testing.T) {
	state, planPath := task14ActivePlanState(t, "# Approved\n\nShip it.")
	tool := NewExitPlanModeTool(state)
	result, err := executeApprovedToolForTest(t, tool, map[string]any{})
	if err != nil {
		t.Fatalf("execute approved exit: %v", err)
	}
	if result.IsError {
		t.Fatalf("approved exit failed: %#v", result)
	}
	if strings.HasPrefix(strings.TrimSpace(result.Content), "{") {
		t.Fatalf("model-visible result leaked Go JSON: %q", result.Content)
	}
	if !strings.Contains(result.Content, "User has approved your plan") ||
		!strings.Contains(result.Content, "## Approved Plan:") ||
		!strings.Contains(result.Content, planPath) {
		t.Fatalf("result does not match TS approved-plan branch: %q", result.Content)
	}
	if result.Data == nil {
		t.Fatal("typed TS V2 output data is missing")
	}
}

// TS ref: src/tools/ExitPlanModeTool/ExitPlanModeV2Tool.ts:147-183.
func TestExitPlanModeTask14_ContractAndChannelAvailability(t *testing.T) {
	state, _ := task14ActivePlanState(t, "# Plan")
	scope := NewRuntimeScope(t.TempDir(), true)
	scope.SetFeatureGate(types.ToolFeaturePlanMode, true)
	tool := NewExitPlanModeTool(state, scope)

	contract := types.ResolveToolContract(tool)
	if contract.OutputSchema == nil || contract.ReadOnly || !contract.ConcurrencySafe || contract.MaxResultSizeChars != 100_000 {
		t.Fatalf("ExitPlanMode contract mismatch: %#v", contract)
	}
	enabled, ok := any(tool).(types.ToolEnabledProvider)
	if !ok {
		t.Fatal("ExitPlanMode must implement context-sensitive availability")
	}
	if !enabled.IsEnabled(scope.ToolRuntimeContext()) {
		t.Fatal("normal local plan session must expose ExitPlanMode")
	}
	runtime := scope.ToolRuntimeContext()
	runtime.ChannelsActive = true
	if enabled.IsEnabled(runtime) {
		t.Fatal("channel session must hide ExitPlanMode to keep Enter/Exit gates symmetric")
	}
}

// TS ref: src/components/permissions/ExitPlanModePermissionRequest/ExitPlanModePermissionRequest.tsx:52-75,447-471.
func TestExitPlanModeTask14_ApprovedAllowedPromptCannotBypassUnrestrictedCodeGate(t *testing.T) {
	state, _ := task14ActivePlanState(t, "# Plan")
	tool := NewExitPlanModeTool(state)
	result, err := executeApprovedToolForTest(t, tool, map[string]any{
		"allowedPrompts": []any{map[string]any{"tool": "Bash", "prompt": "go test"}},
	})
	if err != nil || result.IsError {
		t.Fatalf("approved exit: result=%#v err=%v", result, err)
	}
	bash := &BashTool{PlanState: state}
	decision, err := bash.CheckPermissions(context.Background(), map[string]any{"command": "go test ./tools"}, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatalf("Bash CheckPermissions: %v", err)
	}
	if decision.Behavior != types.PermissionBehaviorAsk || !decision.Required || decision.PolicyDecision == nil ||
		decision.PolicyDecision.Risk != types.PolicyRiskUnrestrictedCode {
		t.Fatalf("approved plan prompt bypassed unrestricted-code gate: %#v", decision)
	}
}

func TestExitPlanModeTask14_UnapprovedAllowedPromptCannotCommit(t *testing.T) {
	state, _ := task14ActivePlanState(t, "# Plan")
	tool := NewExitPlanModeTool(state)
	reg := registry.New()
	reg.Register(tool)
	input := map[string]any{
		"allowedPrompts": []any{map[string]any{"tool": "Bash", "prompt": "go test"}},
	}
	result, err := reg.ExecuteToolWithError(context.Background(), tool.Name(), input)
	if err != nil || !result.IsError {
		t.Fatalf("unapproved exit was not rejected: result=%#v err=%v", result, err)
	}
	if !state.IsActive() {
		t.Fatal("unapproved exit changed plan state")
	}
	if state.AllowedPromptMatches("Bash", "go test ./tools") {
		t.Fatal("unapproved allowedPrompts input changed Bash authority")
	}
}

// TS ref: src/tools/ExitPlanModeTool/ExitPlanModeV2Tool.ts:263-312.
func TestExitPlanModeTask14_TeammateSubmitsPlanToLeaderMailbox(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "task14-team", []any{
		map[string]any{"id": "worker-1", "role": "worker"},
	})
	state, planPath := task14ActivePlanState(t, "# Teammate Plan\n\nImplement safely.")
	mode := permissionModePlan
	scope := NewRuntimeScope(t.TempDir(), false)
	scope.SetPermissionModeDispatcher(func() string { return mode }, func(next string) error {
		mode = next
		return nil
	})
	tool := NewExitPlanModeTool(state, scope)
	tool.AgentID = "worker-1@task14-team"
	tool.PlanModeRequired = true
	tool.TeamManager = mgr
	reg := registry.New()
	reg.Register(tool)

	result, err := reg.ExecuteToolWithError(context.Background(), tool.Name(), map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("teammate ExitPlanMode: result=%#v err=%v", result, err)
	}
	typed, ok := result.Data.(exitPlanModeResult)
	if !ok || !typed.AwaitingLeaderApproval || typed.Status != ExitPlanModeAwaiting || typed.RequestID == "" {
		t.Fatalf("teammate result is not typed awaiting state: %#v", result.Data)
	}
	if !state.IsActive() || state.PlanFile() != planPath {
		t.Fatalf("submission exited plan before leader approval: active=%v path=%q", state.IsActive(), state.PlanFile())
	}

	messages := readMailboxMessages(t, "task14-team", teamLeadName)
	if len(messages) != 1 || messages[0].From != "worker-1" {
		t.Fatalf("leader mailbox messages = %#v", messages)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(messages[0].Text), &payload); err != nil {
		t.Fatalf("decode approval request: %v", err)
	}
	for key, want := range map[string]any{
		"type": "plan_approval_request", "from": "worker-1", "planFilePath": planPath,
		"planContent": "# Teammate Plan\n\nImplement safely.", "requestId": typed.RequestID,
	} {
		if payload[key] != want {
			t.Fatalf("approval payload[%q] = %#v, want %#v; payload=%#v", key, payload[key], want, payload)
		}
	}
	if strings.TrimSpace(payload["timestamp"].(string)) == "" {
		t.Fatalf("approval request missing timestamp: %#v", payload)
	}

	if _, err := ResolveTeammatePlanApprovalResponse(typed.RequestID, "worker-2", true, ""); err == nil {
		t.Fatal("non-leader forged approval must be rejected")
	}
	response, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to": "worker-1",
		"message": map[string]any{
			"type": "plan_approval_response", "request_id": typed.RequestID,
			"approve": true,
		},
	})
	if err != nil || response.IsError {
		t.Fatalf("leader approval response: result=%#v err=%v", response, err)
	}
	approval, ok := TeammatePlanApprovalSnapshot(tool.AgentID)
	if !ok || approval.Active || !approval.Approved || approval.Awaiting || approval.PermissionMode != permissionModePlan {
		t.Fatalf("approved teammate state = %#v", approval)
	}
	if state.IsActive() || state.PlanFile() != "" || mode != permissionModePlan || scope.PermissionMode() != permissionModePlan {
		t.Fatalf("leader approval did not commit real teammate state/runtime: active=%v path=%q dispatcher=%q runtime=%q",
			state.IsActive(), state.PlanFile(), mode, scope.PermissionMode())
	}
}

// TS ref: src/hooks/useInboxPoller.ts:156-194.
func TestExitPlanModeTask14_TeammateRejectionKeepsPlanModeAndFeedback(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "task14-reject", []any{
		map[string]any{"id": "worker-2", "role": "worker"},
	})
	state, _ := task14ActivePlanState(t, "# Revise Me")
	tool := NewExitPlanModeTool(state)
	tool.AgentID = "worker-2@task14-reject"
	tool.PlanModeRequired = true
	tool.TeamManager = mgr
	reg := registry.New()
	reg.Register(tool)
	result, err := reg.ExecuteToolWithError(context.Background(), tool.Name(), map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("teammate submit: result=%#v err=%v", result, err)
	}
	typed := result.Data.(exitPlanModeResult)

	response, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to": "worker-2",
		"message": map[string]any{
			"type": "plan_approval_response", "request_id": typed.RequestID,
			"approve": false, "feedback": "Add a rollback step",
		},
	})
	if err != nil || response.IsError {
		t.Fatalf("leader rejection response: result=%#v err=%v", response, err)
	}
	rejected, ok := TeammatePlanApprovalSnapshot(tool.AgentID)
	if !ok || !rejected.Active || rejected.Approved || rejected.Awaiting || rejected.PermissionMode != permissionModePlan || rejected.Feedback != "Add a rollback step" {
		t.Fatalf("rejected teammate state = %#v", rejected)
	}
	if !state.IsActive() {
		t.Fatal("leader rejection must leave the concrete teammate PlanState active")
	}
}
