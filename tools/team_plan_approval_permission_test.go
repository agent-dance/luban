package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPlanApprovalMessageDoesNotCarryPermissionMode(t *testing.T) {
	t.Parallel()

	if _, ok := reflect.TypeOf(structuredSendMessage{}).FieldByName("PermissionMode"); ok {
		t.Fatal("structured plan approval messages must not carry a permission mode")
	}
}

func TestPlanApprovalRejectsLegacyPermissionModeWithoutRuntime(t *testing.T) {
	tool := NewSendMessageTool(&TeamManager{})
	requestID := "pending-without-runtime"
	defaultPlanApprovalCoordinator.register(pendingPlanApproval{AgentID: "worker-nil-runtime", RequestID: requestID})
	t.Cleanup(func() { defaultPlanApprovalCoordinator.unregister(requestID) })
	for _, field := range []string{"permission_mode", "permissionMode"} {
		field := field
		t.Run(field, func(t *testing.T) {
			message := map[string]any{
				"type":       "plan_approval_response",
				"request_id": requestID,
				"approve":    true,
				field:        "bypassPermissions",
			}
			decoded, structured, err := decodeStructuredSendMessage(message)
			if err != nil || !structured {
				t.Fatalf("decode structured message: structured=%v err=%v", structured, err)
			}
			if err := validateStructuredSendMessageInput(tool, message, decoded); err == nil || !strings.Contains(err.Error(), "unsupported structured message field") {
				t.Fatalf("legacy permission field must be rejected even without a runtime, got %v", err)
			}
		})
	}
}

func TestTeammatePlanApprovalCannotChangePermissionMode(t *testing.T) {
	state, _ := task14ActivePlanState(t, "# Approved plan")
	mode := permissionModeDefault
	scope := NewRuntimeScope(t.TempDir(), false)
	scope.SetPermissionModeDispatcher(func() string { return mode }, func(next string) error {
		mode = next
		return nil
	})
	NewExitPlanModeTool(state, scope)

	requestID := "permission-invariant"
	defaultPlanApprovalCoordinator.register(pendingPlanApproval{
		AgentID: "worker-1@team", RequestID: requestID, PlanFile: state.PlanFile(), State: state,
	})
	t.Cleanup(func() { defaultPlanApprovalCoordinator.unregister(requestID) })

	resolution, err := ResolveTeammatePlanApprovalResponse(requestID, teamLeadName, true, "")
	if err != nil {
		t.Fatalf("resolve teammate approval: %v", err)
	}
	if !resolution.Approved || resolution.Awaiting {
		t.Fatalf("approval semantics were not preserved: %#v", resolution)
	}
	if mode != permissionModeDefault || scope.PermissionMode() != permissionModeDefault {
		t.Fatalf("approval changed permission mode: dispatcher=%q runtime=%q", mode, scope.PermissionMode())
	}
}

func TestPlanApprovalMailboxPayloadOmitsPermissionMode(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "permission-payload", []any{map[string]any{"id": "worker-1", "role": "worker"}})
	mgr.Runtime = NewRuntimeScope(t.TempDir(), true)
	result, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to": "worker-1",
		"message": map[string]any{
			"type": "plan_approval_response", "request_id": "plan-approved", "approve": true,
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("send plan approval: result=%#v err=%v", result, err)
	}

	messages := readMailboxMessages(t, "permission-payload", "worker-1")
	var payload map[string]any
	if err := json.Unmarshal([]byte(messages[len(messages)-1].Text), &payload); err != nil {
		t.Fatalf("decode approval payload: %v", err)
	}
	if _, ok := payload["permissionMode"]; ok {
		t.Fatalf("approval payload leaked permissionMode: %#v", payload)
	}
	if _, ok := payload["permission_mode"]; ok {
		t.Fatalf("approval payload leaked permission_mode: %#v", payload)
	}
	if payload["approved"] != true {
		t.Fatalf("approval payload lost approval semantics: %#v", payload)
	}
}
