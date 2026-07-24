package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

func TestSendMessageTask23_DescriptionSchemaVisibilityAndReadOnly(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewSendMessageTool(mgr)
	if tool.Description() != "Send a message to another agent" {
		t.Fatalf("Description = %q", tool.Description())
	}
	if !tool.Schema().RejectsUnknownFields() {
		t.Fatal("SendMessage schema must reject unknown root fields")
	}
	plain := map[string]any{"to": "worker", "summary": "send current status", "message": "hello"}
	structured := map[string]any{"to": "worker", "message": map[string]any{"type": "shutdown_request"}}
	if !tool.IsReadOnlyInput(plain) || tool.ToolMetadata(plain).Write {
		t.Fatalf("plain SendMessage metadata = %#v", tool.ToolMetadata(plain))
	}
	if tool.IsReadOnlyInput(structured) || !tool.ToolMetadata(structured).Write {
		t.Fatalf("structured SendMessage metadata = %#v", tool.ToolMetadata(structured))
	}

	scope := NewRuntimeScope(t.TempDir(), true)
	reg := registry.New()
	reg.SetRuntimeContextProvider(scope)
	reg.Register(tool)
	scope.SetFeatureGate(types.ToolFeatureTeams, false)
	if reg.IsToolEnabled(tool) {
		t.Fatal("SendMessage remained visible with agent swarms disabled")
	}
	scope.SetFeatureGate(types.ToolFeatureTeams, true)
	if !reg.IsToolEnabled(tool) {
		t.Fatal("SendMessage was hidden with agent swarms enabled")
	}
	if metadata := reg.ToolMetadata(tool.Name(), plain); !metadata.ReadOnly || metadata.Write {
		t.Fatalf("registry plain metadata = %#v", metadata)
	}
	if metadata := reg.ToolMetadata(tool.Name(), structured); metadata.ReadOnly || !metadata.Write {
		t.Fatalf("registry structured metadata = %#v", metadata)
	}
}

func TestSendMessageTask23_StrictStructuredInput(t *testing.T) {
	mgr := newTestManager(t)
	mgr.Runtime = NewRuntimeScope(t.TempDir(), true)
	tool := NewSendMessageTool(mgr)
	for _, input := range []map[string]any{
		{"to": "worker", "message": map[string]any{"type": "ask_user_question"}},
		{"to": "worker", "message": map[string]any{"type": "shutdown_request", "extra": "nope"}},
		{"to": "worker", "message": map[string]any{"type": "shutdown_request", "reason": 7}},
		{"to": teamLeadName, "message": map[string]any{"type": "shutdown_response", "request_id": "r", "approve": "maybe"}},
		{"to": "worker", "message": map[string]any{"type": "plan_approval_response", "request_id": "r", "approve": true, "permission_mode": "bypassPermissions"}},
	} {
		result, err := tool.Execute(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatalf("invalid structured input accepted: %#v -> %#v", input, result)
		}
	}
}

func TestSendMessageTask23_ColorAndTeamMailboxDirect(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "color-team", []any{map[string]any{"id": "worker-1", "role": "worker"}})
	cfg, err := swarm.LoadTeamConfig("color-team")
	if err != nil {
		t.Fatal(err)
	}
	for i := range cfg.Members {
		switch cfg.Members[i].Name {
		case teamLeadName:
			cfg.Members[i].Color = "blue"
		case "worker-1":
			cfg.Members[i].Color = "red"
		}
	}
	if err := swarm.SaveTeamConfig(cfg); err != nil {
		t.Fatal(err)
	}
	result, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to": "worker-1", "summary": "share the current status", "message": "hello",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute = %#v, err=%v", result, err)
	}
	data := result.Data.(SendMessageResult)
	if data.Routing == nil || data.Routing.SenderColor != "blue" || data.Routing.TargetColor != "red" {
		t.Fatalf("routing colors = %#v", data.Routing)
	}
	messages := readMailboxMessages(t, "color-team", "worker-1")
	if len(messages) != 1 || messages[0].Color != "blue" || messages[0].Read {
		t.Fatalf("mailbox color/read = %#v", messages)
	}
}

func TestSendMessageTask23_PlanApprovalOmitsPermissionModeAndUsesDefaultFeedback(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "plan-team", []any{map[string]any{"id": "worker-1", "role": "worker"}})
	scope := NewRuntimeScope(t.TempDir(), true)
	scope.SetPermissionModeDispatcher(func() string { return permissionModePlan }, func(string) error { return nil })
	mgr.Runtime = scope
	tool := NewSendMessageTool(mgr)

	approved, err := tool.Execute(context.Background(), map[string]any{
		"to": "worker-1", "message": map[string]any{
			"type": "plan_approval_response", "request_id": "plan-approved", "approve": true,
		},
	})
	if err != nil || approved.IsError {
		t.Fatalf("approval = %#v, err=%v", approved, err)
	}
	messages := readMailboxMessages(t, "plan-team", "worker-1")
	var approvalPayload map[string]any
	if err := json.Unmarshal([]byte(messages[len(messages)-1].Text), &approvalPayload); err != nil {
		t.Fatal(err)
	}
	if _, ok := approvalPayload["permissionMode"]; ok {
		t.Fatalf("approval payload exposed permissionMode: %#v", approvalPayload)
	}

	rejected, err := tool.Execute(context.Background(), map[string]any{
		"to": "worker-1", "message": map[string]any{
			"type": "plan_approval_response", "request_id": "plan-rejected", "approve": false,
		},
	})
	if err != nil || rejected.IsError || !strings.Contains(rejected.Content, "Plan needs revision") {
		t.Fatalf("rejection = %#v, err=%v", rejected, err)
	}
	messages = readMailboxMessages(t, "plan-team", "worker-1")
	var rejectionPayload map[string]any
	if err := json.Unmarshal([]byte(messages[len(messages)-1].Text), &rejectionPayload); err != nil {
		t.Fatal(err)
	}
	if rejectionPayload["feedback"] != "Plan needs revision" {
		t.Fatalf("rejection payload = %#v", rejectionPayload)
	}
}

func TestSendMessageTask23_ShutdownBackendMetadataAndLifecycle(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "shutdown-team", []any{map[string]any{"id": "worker-1", "role": "worker"}})
	cfg, err := swarm.LoadTeamConfig("shutdown-team")
	if err != nil {
		t.Fatal(err)
	}
	for i := range cfg.Members {
		if cfg.Members[i].Name == "worker-1" {
			cfg.Members[i].TmuxPaneID = "%9"
			cfg.Members[i].BackendType = "tmux"
		}
	}
	if err := swarm.SaveTeamConfig(cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CODE_AGENT_NAME", "worker-1")
	t.Setenv("CLAUDE_CODE_AGENT_ID", "worker-1")
	result, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to": teamLeadName, "message": map[string]any{
			"type": "shutdown_response", "request_id": "shutdown-1", "approve": true,
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("shutdown response = %#v, err=%v", result, err)
	}
	messages := readMailboxMessages(t, "shutdown-team", teamLeadName)
	var payload map[string]any
	if err := json.Unmarshal([]byte(messages[len(messages)-1].Text), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["paneId"] != "%9" || payload["backendType"] != "tmux" {
		t.Fatalf("shutdown payload = %#v", payload)
	}
	cfg, err = swarm.LoadTeamConfig("shutdown-team")
	if err != nil {
		t.Fatal(err)
	}
	member, ok := teamMemberByIdentity(cfg, "worker-1")
	if !ok || member.IsActive {
		t.Fatalf("approved shutdown did not terminate cooperative session state: %#v", member)
	}
}

func TestSendMessageTask23_ModelOutputIsTypedCurrentShape(t *testing.T) {
	mgr := newTestManager(t)
	result, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to": "worker-1", "summary": "send a direct message", "message": "hello",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute = %#v, err=%v", result, err)
	}
	data, ok := result.Data.(SendMessageResult)
	if !ok || !data.Success || data.Routing == nil {
		t.Fatalf("typed result = %#v (%T)", result.Data, result.Data)
	}
	block := types.MapToolResult(NewSendMessageTool(mgr), result, "toolu_task23")
	if block.ToolUseID != "toolu_task23" || block.Content != result.Content || strings.Contains(block.Content, `"delivered"`) {
		t.Fatalf("model result block = %#v", block)
	}
}
