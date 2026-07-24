package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

func runSendMessageAlignment(t *testing.T, mgr *TeamManager, input map[string]any) (types.ToolResult, map[string]any) {
	t.Helper()
	result, err := NewSendMessageTool(mgr).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("SendMessage.Execute returned infrastructure error: %v", err)
	}
	var decoded map[string]any
	if !result.IsError {
		if err := json.Unmarshal([]byte(result.Content), &decoded); err != nil {
			t.Fatalf("decode SendMessage result %q: %v", result.Content, err)
		}
	}
	return result, decoded
}

// TS ref: src/tools/SendMessageTool/SendMessageTool.ts:46-86,520-541.
func TestSendMessageAlignment_ContractAndStrictSchema(t *testing.T) {
	tool := NewSendMessageTool(newTestManager(t))
	if tool.Name() != "SendMessage" || tool.Description() != "Send a message to another agent" {
		t.Fatalf("public identity = %q / %q", tool.Name(), tool.Description())
	}
	schema := tool.Schema()
	if !schema.RejectsUnknownFields() || len(schema.Required) != 2 {
		t.Fatalf("root schema is not strict/required: %#v", schema)
	}
	if _, ok := schema.Properties["from"]; ok {
		t.Fatal("schema exposes spoofable from")
	}
	if _, ok := schema.Properties["content"]; ok {
		t.Fatal("schema exposes legacy content")
	}
	to := schema.Properties["to"].(map[string]any)
	if !strings.Contains(to["description"].(string), "uds:<socket-path>") {
		t.Fatalf("to schema omits supported UDS address: %#v", to)
	}
	message := schema.Properties["message"].(map[string]any)
	if variants := message["oneOf"].([]any); len(variants) != 4 {
		t.Fatalf("message union has %d variants, want string + 3 structured", len(variants))
	}
	contract := types.ResolveToolContract(tool)
	if !contract.Strict || contract.OutputSchema == nil || contract.MaxResultSizeChars != 100_000 {
		t.Fatalf("tool contract = %#v", contract)
	}
}

// TS ref: src/tools/SendMessageTool/SendMessageTool.ts:67-86,155-160,442-457.
func TestSendMessageAlignment_FromSpoofAndContentRejected(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "spoof-team", []any{map[string]any{"id": "worker-1", "role": "worker"}})
	for _, input := range []map[string]any{
		{"to": "worker-1", "summary": "spoof sender identity", "message": "hello", "from": teamLeadName},
		{"to": "worker-1", "content": "legacy content"},
	} {
		result, _ := runSendMessageAlignment(t, mgr, input)
		if !result.IsError || !strings.Contains(result.Content, "unknown field") {
			t.Fatalf("legacy/spoof input was accepted: %#v", result)
		}
	}

	t.Setenv("CLAUDE_CODE_AGENT_NAME", "worker-1")
	result, _ := runSendMessageAlignment(t, mgr, map[string]any{
		"to":      "worker-1",
		"message": map[string]any{"type": "plan_approval_response", "request_id": "req-spoof", "approve": true},
	})
	if !result.IsError || !strings.Contains(result.Content, "Only the team lead") {
		t.Fatalf("non-leader runtime approved a plan: %#v", result)
	}
}

// TS ref: src/tools/SendMessageTool/SendMessageTool.ts:149-188 and
// src/utils/teammateMailbox.ts:56-75.
func TestSendMessageAlignment_NoTeamDirectUsesDefaultMailboxNotEnvelope(t *testing.T) {
	mgr := newTestManager(t)
	bus := mgr.coordinator.GetBus()
	ch := bus.Subscribe("alice")
	result, decoded := runSendMessageAlignment(t, mgr, map[string]any{
		"to": "alice", "summary": "send direct status", "message": "hello",
	})
	if result.IsError || decoded["success"] != true || decoded["message"] != "Message sent to alice's inbox" {
		t.Fatalf("direct output = %#v / %s", decoded, result.Content)
	}
	for _, legacy := range []string{"request_id", "kind", "delivered", "reply_to", "reply_text", "quorum"} {
		if _, exists := decoded[legacy]; exists {
			t.Fatalf("direct output leaked legacy Envelope field %q: %#v", legacy, decoded)
		}
	}
	if _, ok := result.Data.(SendMessageResult); !ok {
		t.Fatalf("direct result is not typed: %T", result.Data)
	}
	select {
	case message := <-ch:
		t.Fatalf("public SendMessage silently used MessageBus: %#v", message)
	default:
	}
	mailbox, err := swarm.NewMailbox("default")
	if err != nil {
		t.Fatal(err)
	}
	messages, err := mailbox.Read(context.Background(), "alice")
	if err != nil || len(messages) != 1 || messages[0].From != teamLeadName || messages[0].Read {
		t.Fatalf("default mailbox = %#v, err=%v", messages, err)
	}
}

// TS ref: src/tools/SendMessageTool/SendMessageTool.ts:191-207.
func TestSendMessageAlignment_BroadcastRequiresTeamAndExistingTeamFile(t *testing.T) {
	mgr := newTestManager(t)
	result, _ := runSendMessageAlignment(t, mgr, map[string]any{
		"to": "*", "summary": "broadcast current status", "message": "hello",
	})
	if !result.IsError || !strings.Contains(result.Content, "Not in a team context") {
		t.Fatalf("no-team broadcast = %#v", result)
	}
	t.Setenv("CLAUDE_CODE_TEAM_NAME", "missing-team")
	result, _ = runSendMessageAlignment(t, mgr, map[string]any{
		"to": "*", "summary": "broadcast current status", "message": "hello",
	})
	if !result.IsError || result.Content != `Team "missing-team" does not exist` {
		t.Fatalf("missing-team broadcast = %#v", result)
	}
}

// TS ref: src/tools/SendMessageTool/SendMessageTool.ts:220-265.
func TestSendMessageAlignment_BroadcastIncludesInactiveAndMatchesOutput(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "broadcast-team", []any{
		map[string]any{"id": "worker-1", "role": "worker"},
		map[string]any{"id": "worker-2", "role": "reviewer"},
	})
	cfg, err := swarm.LoadTeamConfig("broadcast-team")
	if err != nil {
		t.Fatal(err)
	}
	for i := range cfg.Members {
		if cfg.Members[i].Name == "worker-2" {
			cfg.Members[i].IsActive = false
		}
	}
	if err := swarm.SaveTeamConfig(cfg); err != nil {
		t.Fatal(err)
	}
	result, decoded := runSendMessageAlignment(t, mgr, map[string]any{
		"to": "*", "summary": "broadcast current status", "message": "hello team",
	})
	if result.IsError || decoded["message"] != "Message broadcast to 2 teammate(s): worker-1, worker-2" {
		t.Fatalf("broadcast output = %#v / %s", decoded, result.Content)
	}
	recipients := decoded["recipients"].([]any)
	if len(recipients) != 2 || recipients[0] != "worker-1" || recipients[1] != "worker-2" {
		t.Fatalf("broadcast recipients = %#v", recipients)
	}
	if _, exists := decoded["failed"]; exists {
		t.Fatalf("TS-compatible output leaked Go partial bookkeeping: %#v", decoded)
	}
}

// TS ref: src/tools/SendMessageTool/SendMessageTool.ts:46-64,883-912.
func TestSendMessageAlignment_StructuredUnionRejectsNonCurrentTypes(t *testing.T) {
	mgr := newTestManager(t)
	for _, kind := range []string{"plan_approval_request", "shutdown_ack", "ask_user_question"} {
		result, _ := runSendMessageAlignment(t, mgr, map[string]any{
			"to": teamLeadName, "message": map[string]any{"type": kind},
		})
		if !result.IsError || !strings.Contains(result.Content, "unsupported structured message type") {
			t.Fatalf("non-current structured type %q was accepted: %#v", kind, result)
		}
	}
}

func TestSendMessageAlignment_TypedResultMapperUsesCurrentOutput(t *testing.T) {
	tool := NewSendMessageTool(newTestManager(t))
	data := SendMessageResult{Success: true, Message: "sent"}
	block := tool.MapToolResultToToolResultBlock(data, "toolu_send")
	if block.ToolUseID != "toolu_send" || block.Data == nil || block.Content != `{"success":true,"message":"sent"}` {
		t.Fatalf("mapped block = %#v", block)
	}
}
