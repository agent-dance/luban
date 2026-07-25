package permissions

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/types"
)

func TestSubagentPermissionSnapshotIgnoresLaterForegroundRules(t *testing.T) {
	checker := NewChecker(ModeRuleBased, []Rule{{Tool: "Write", Decision: DecisionAllow}})
	checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
		return PromptResponse{Decision: DecisionDeny, Outcome: PromptOutcomeRejected}
	})
	handler := NewCLIPermissionHandler(checker)
	snapshot := types.ToolRuntimeContext{PermissionMode: "default"}

	decision, err := handler.Check(context.Background(), permission.PermissionRequest{
		SessionID: "parent", ToolName: "Write", Input: map[string]any{"file_path": "note.txt"},
		PermissionSnapshot: &snapshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision != permission.PermissionDeny {
		t.Fatalf("later foreground allow rule changed child snapshot decision: %v", decision)
	}
}

func TestSubagentPermissionSnapshotIgnoresForegroundAlwaysAllowCache(t *testing.T) {
	checker := NewChecker(ModeRuleBased, nil)
	checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
		return PromptResponse{Decision: DecisionAllow, Outcome: PromptOutcomeApproved}
	})
	handler := NewCLIPermissionHandler(checker)
	request := permission.PermissionRequest{
		SessionID: "parent", ToolName: "Write", Input: map[string]any{"file_path": "note.txt"},
	}
	if decision, err := handler.Check(context.Background(), request); err != nil || decision != permission.PermissionAllow {
		t.Fatalf("seed foreground cache: decision=%v err=%v", decision, err)
	}

	checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
		return PromptResponse{Decision: DecisionDeny, Outcome: PromptOutcomeRejected}
	})
	snapshot := types.ToolRuntimeContext{PermissionMode: "default"}
	request.PermissionSnapshot = &snapshot
	decision, err := handler.Check(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decision != permission.PermissionDeny {
		t.Fatalf("foreground cache changed child snapshot decision: %v", decision)
	}
}

func TestSubagentPermissionSnapshotRulesOverrideForegroundPolicy(t *testing.T) {
	checker := NewChecker(ModeAllowAll, nil)
	handler := NewCLIPermissionHandler(checker)
	snapshot := types.ToolRuntimeContext{
		PermissionMode: "bypassPermissions",
		DeniedRules:    []types.PermissionRuleValue{{ToolName: "Write"}},
	}
	decision, err := handler.Check(context.Background(), permission.PermissionRequest{
		SessionID: "parent", ToolName: "Write", Input: map[string]any{"file_path": "note.txt"},
		PermissionSnapshot: &snapshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision != permission.PermissionDeny {
		t.Fatalf("snapshot deny lost under foreground allow-all: %v", decision)
	}
}
