package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
)

// decodeLines decodes each non-empty line in buf as a JSON object and returns
// the slice of decoded maps.
func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid JSON line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestJSONRenderer_Text(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	r.Text("Hello, world")

	lines := decodeLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := lines[0]["type"]; got != "text" {
		t.Errorf("type = %q, want %q", got, "text")
	}
	if got := lines[0]["content"]; got != "Hello, world" {
		t.Errorf("content = %q, want %q", got, "Hello, world")
	}
}

func TestJSONRenderer_Thinking(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	r.Thinking("deep thought")

	lines := decodeLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := lines[0]["type"]; got != "thinking" {
		t.Errorf("type = %q, want %q", got, "thinking")
	}
	if got := lines[0]["content"]; got != "deep thought" {
		t.Errorf("content = %q, want %q", got, "deep thought")
	}
}

func TestJSONRenderer_ToolCall(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	r.RenderToolCall(presentation.ToolEventContext{}, types.ToolUseBlock{Name: "Bash", Input: map[string]any{"command": "ls -la"}})

	lines := decodeLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := lines[0]["type"]; got != "tool_use" {
		t.Errorf("type = %q, want %q", got, "tool_use")
	}
	if got := lines[0]["name"]; got != "Bash" {
		t.Errorf("name = %q, want %q", got, "Bash")
	}
	input, ok := lines[0]["input"].(map[string]any)
	if !ok {
		t.Fatalf("input is not an object: %T", lines[0]["input"])
	}
	if got := input["command"]; got != "ls -la" {
		t.Errorf("input.command = %q, want %q", got, "ls -la")
	}
}

func TestJSONRenderer_ToolResult(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	r.RenderToolResult(presentation.ToolEventContext{}, types.ToolResultBlock{Content: "file.txt", Outcome: types.ToolOutcomeSucceeded})

	lines := decodeLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := lines[0]["type"]; got != "tool_result" {
		t.Errorf("type = %q, want %q", got, "tool_result")
	}
	if got := lines[0]["output"]; got != "file.txt" {
		t.Errorf("output = %q, want %q", got, "file.txt")
	}
	if got := lines[0]["is_error"]; got != false {
		t.Errorf("is_error = %v, want false", got)
	}
}

func TestJSONRenderer_ToolResultWithoutOutcomeFailsClosed(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	r.RenderToolResult(presentation.ToolEventContext{}, types.ToolResultBlock{Content: "untyped result"})

	lines := decodeLines(t, &buf)
	if len(lines) != 1 || lines[0]["type"] != "error" || lines[0]["kind"] != "runtime_error" {
		t.Fatalf("missing-outcome result = %#v, want semantic runtime error", lines)
	}
	for _, retired := range []string{"output", "content", "data", "metadata"} {
		if _, exists := lines[0][retired]; exists {
			t.Fatalf("missing-outcome result retained flat field %q: %#v", retired, lines[0])
		}
	}
}

func TestJSONRenderer_StructuredToolEventsPreserveExecutionIdentityAndOutcome(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	r.SessionInfo("session-machine", nil)
	ctx := presentation.ToolEventContext{
		SessionID: "session-machine", SessionEpoch: 4, ContextGeneration: 9,
		TurnID: "session-machine:query-1:turn-2", ActorID: "agent-reviewer",
		ActorType: "reviewer", WorkUnitID: "review-work",
	}
	presentation.DispatchToolCallEvent(r, ctx, types.ToolUseBlock{
		Type:  types.ContentTypeToolUse,
		ID:    "toolu-machine",
		Name:  "Read",
		Input: map[string]any{"file_path": "/tmp/evidence"},
	})
	presentation.DispatchToolResultEvent(r, ctx, types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu-machine",
		Content:   "partial evidence",
		ContentBlocks: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "structured evidence"},
		},
		Data:        map[string]any{"partial_count": 2},
		NewMessages: []types.Message{types.UserMessage("retained follow-up")},
		Metadata:    map[string]string{"source": "filesystem"},
		Usage:       &types.Usage{InputTokens: 7, OutputTokens: 3},
		Outcome:     types.ToolOutcomePartial,
	})

	lines := decodeLines(t, &buf)
	if len(lines) != 3 {
		t.Fatalf("events = %#v, want session_info plus two tool events", lines)
	}
	call, result := lines[1], lines[2]
	for key, want := range map[string]any{
		"session_id": "session-machine", "turn_id": "session-machine:query-1:turn-2",
		"actor_id": "agent-reviewer", "actor_type": "reviewer", "work_unit_id": "review-work",
		"tool_use_id": "toolu-machine",
	} {
		if got := call[key]; got != want {
			t.Errorf("tool call %s = %#v, want %#v", key, got, want)
		}
		if got := result[key]; got != want {
			t.Errorf("tool result %s = %#v, want %#v", key, got, want)
		}
	}
	if got := result["outcome"]; got != string(types.ToolOutcomePartial) {
		t.Errorf("outcome = %#v, want partial", got)
	}
	if result["schema_version"] != types.RuntimeEventSchemaVersion || result["audience"] != "sdk" ||
		result["redaction_level"] != "strict" || result["context_generation"] != float64(9) || result["event_id"] == "" {
		t.Errorf("typed runtime result projection = %#v", result)
	}
	if result["public_key"] != "runtime.tool_result.public_summary" || result["code"] != "tool.result" {
		t.Errorf("typed runtime result semantics = %#v", result)
	}
	if got := result["output"]; got != "structured evidence" {
		t.Errorf("output = %#v, want structured text projection", got)
	}
	if got := result["content"]; got != "partial evidence" {
		t.Errorf("content = %#v, want exact raw content", got)
	}
	metadata, ok := result["metadata"].(map[string]any)
	if !ok || metadata["source"] != "filesystem" {
		t.Errorf("metadata = %#v, want source identity", result["metadata"])
	}
	data, ok := result["data"].(map[string]any)
	if !ok || data["partial_count"] != float64(2) {
		t.Errorf("data = %#v, want typed payload", result["data"])
	}
	if blocks, ok := result["content_blocks"].([]any); !ok || len(blocks) != 1 {
		t.Errorf("content_blocks = %#v, want one block", result["content_blocks"])
	}
	if messages, ok := result["new_messages"].([]any); !ok || len(messages) != 1 {
		t.Errorf("new_messages = %#v, want retained follow-up", result["new_messages"])
	}
	usage, ok := result["usage"].(map[string]any)
	if !ok || usage["input_tokens"] != float64(7) || usage["output_tokens"] != float64(3) {
		t.Errorf("usage = %#v, want exact token fields", result["usage"])
	}
}

func TestJSONRenderer_HookIdentityAndRuntimeErrorPublicProjection(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	ctx := presentation.ToolEventContext{SessionID: "session-machine", ProjectRoot: "/workspace/project", TurnID: "turn-machine", ActorID: "agent-machine", ActorType: "reviewer", WorkUnitID: "work-machine"}
	r.RenderHookSummary(ctx, presentation.HookSummary{ExecutionID: "hook-machine", ToolUseID: "toolu-machine", Name: "PostToolUse", Status: "blocked", Summary: "policy blocked"})
	r.RuntimeErrorEvent(ctx, "toolu-machine", "tool failed", &types.APIError{Type: "tool_error", Message: "full failure"}, map[string]any{"retryable": false})

	lines := decodeLines(t, &buf)
	if len(lines) != 2 || lines[0]["type"] != "hook_summary" || lines[1]["type"] != "error" {
		t.Fatalf("events = %#v", lines)
	}
	for key, want := range map[string]any{
		"session_id": "session-machine", "project_root": "/workspace/project", "turn_id": "turn-machine", "actor_id": "agent-machine", "actor_type": "reviewer", "work_unit_id": "work-machine", "tool_use_id": "toolu-machine",
	} {
		if got := lines[0][key]; got != want {
			t.Errorf("%s = %#v, want %#v in %#v", key, got, want, lines[0])
		}
		if _, leaked := lines[1][key]; leaked {
			t.Errorf("default runtime-error projection leaked %s: %#v", key, lines[1])
		}
	}
	if lines[0]["hook_execution_id"] != "hook-machine" || lines[0]["status"] != "blocked" {
		t.Fatalf("hook details = %#v", lines[0])
	}
	if lines[1]["audience"] != "user" || lines[1]["schema_version"] != types.RuntimeEventSchemaVersion ||
		lines[1]["redaction_level"] != "strict" || lines[1]["kind"] != "runtime_error" ||
		lines[1]["outcome"] != string(types.ToolOutcomeFailed) || lines[1]["code"] != "runtime.operation_failed" {
		t.Fatalf("runtime error projection = %#v", lines[1])
	}
	for _, key := range []string{"event_id", "epoch", "context_generation", "public_key", "public_args", "private_cause", "private_metadata", "evidence_ref"} {
		if _, leaked := lines[1][key]; leaked {
			t.Errorf("default runtime-error projection leaked %s: %#v", key, lines[1])
		}
	}
	encoded, err := json.Marshal(lines[1])
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"tool failed", "tool_error", "full failure", "retryable"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("runtime error projection leaked %q: %s", secret, encoded)
		}
	}
}

func TestJSONRenderer_SendUserMessageUsesRuntimeEventProjection(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewJSONRenderer(&output)
	ctx := presentation.ToolEventContext{
		SessionID: "session-brief", TurnID: "turn-brief", ActorID: "assistant", ActorType: "assistant", WorkUnitID: "brief-work",
	}
	result := types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu-brief",
		Content:   "internal acknowledgement",
		ContentBlocks: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "structured acknowledgement"},
		},
		Data:        interaction.SendUserMessageOutput{Message: "Deployment complete.", SentAt: "2026-07-15T00:00:00Z"},
		Metadata:    map[string]string{"delivery": "direct"},
		Usage:       &types.Usage{InputTokens: 11, OutputTokens: 5},
		NewMessages: []types.Message{types.UserMessage("follow-up retained")},
		Outcome:     types.ToolOutcomeSucceeded,
	}
	if handled := presentation.DispatchToolResultEvent(renderer, ctx, result); !handled {
		t.Fatal("SendUserMessage result was not handled")
	}
	lines := decodeLines(t, &output)
	if len(lines) != 1 || lines[0]["type"] != "assistant_message" || lines[0]["message"] != "Deployment complete." {
		t.Fatalf("assistant event = %#v", lines)
	}
	projection, ok := lines[0]["runtime_event"].(map[string]any)
	if !ok || projection["schema_version"] != types.RuntimeEventSchemaVersion || projection["kind"] != "tool_result" ||
		projection["outcome"] != string(types.ToolOutcomeSucceeded) || projection["tool_use_id"] != result.ToolUseID ||
		projection["event_id"] == "" || projection["code"] != "tool.result" {
		t.Fatalf("assistant runtime projection = %#v", lines[0]["runtime_event"])
	}
	for _, retired := range []string{"result_envelope", "output", "content", "content_blocks", "data", "metadata", "usage", "new_messages"} {
		if _, exists := lines[0][retired]; exists {
			t.Fatalf("assistant event retained raw result field %q: %#v", retired, lines[0])
		}
	}
}

func TestJSONRenderer_StructuredDecisionIncludesReviewFieldsAndResult(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	request := permissions.PromptRequest{
		DecisionID: "decision-json", SessionID: "session-json", ExecutionSessionID: "agent-session", TurnID: "turn-json", ToolUseID: "toolu-json",
		ToolName: "Write", Input: map[string]any{"file_path": "/protected"}, ActorID: "agent-json", ActorType: "reviewer", WorkUnitID: "work-json",
		Kind: permissions.PromptKindPermission, Action: "write", Target: "/protected", Impact: "changes data", RiskLevel: 3,
		RiskReason: "protected path", RuleSource: "mandatory approval policy", ApprovalScope: "single call", Choices: []string{"allow_once", "reject"},
		Body: "full review", ReviewDetails: []string{"path: /protected"}, PostMode: "default", Message: "approval required",
	}
	response := r.DecisionRequest(context.Background(), request)
	if response.DecisionID != request.DecisionID || response.Decision != permissions.DecisionDeny || response.Outcome != permissions.PromptOutcomeRejected {
		t.Fatalf("response = %+v, want explicit rejection", response)
	}
	lines := decodeLines(t, &buf)
	if len(lines) != 2 || lines[0]["type"] != "decision_request" || lines[1]["type"] != "decision_result" {
		t.Fatalf("decision events = %#v", lines)
	}
	for key, want := range map[string]any{
		"decision_id": "decision-json", "session_id": "session-json", "execution_session_id": "agent-session", "turn_id": "turn-json", "tool_use_id": "toolu-json",
		"actor_id": "agent-json", "actor_type": "reviewer", "work_unit_id": "work-json", "action": "write", "target": "/protected",
		"impact": "changes data", "risk_reason": "protected path", "rule_source": "mandatory approval policy", "approval_scope": "single call",
	} {
		if got := lines[0][key]; got != want {
			t.Errorf("request %s = %#v, want %#v", key, got, want)
		}
	}
	if lines[1]["outcome"] != string(permissions.PromptOutcomeRejected) || lines[1]["decision"] != "deny" {
		t.Fatalf("decision result = %#v", lines[1])
	}
}

func TestJSONRenderer_CostSummary(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	r.CostSummary(0.003, 0.12, 1200, 450)

	lines := decodeLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := lines[0]["type"]; got != "cost" {
		t.Errorf("type = %q, want %q", got, "cost")
	}
	// JSON numbers decode as float64.
	if got, ok := lines[0]["turn"].(float64); !ok || got != 0.003 {
		t.Errorf("turn = %v, want 0.003", lines[0]["turn"])
	}
	if got, ok := lines[0]["total"].(float64); !ok || got != 0.12 {
		t.Errorf("total = %v, want 0.12", lines[0]["total"])
	}
	for key, want := range map[string]any{
		"turn_scope":                 "last_request",
		"total_scope":                "cumulative_session",
		"last_request_input_tokens":  float64(1200),
		"last_request_output_tokens": float64(450),
	} {
		if got := lines[0][key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
	for _, retired := range []string{"input_tokens", "output_tokens"} {
		if _, exists := lines[0][retired]; exists {
			t.Errorf("cost event retained unscoped field %q: %#v", retired, lines[0])
		}
	}
}

func TestJSONRenderer_NewlineIsNoop(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	r.Newline()
	if buf.Len() != 0 {
		t.Errorf("Newline() wrote %q; expected nothing", buf.String())
	}
}

func TestJSONRenderer_SpinnerStartNoop(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	stop := r.SpinnerStart("Bash")
	stop()
	if buf.Len() != 0 {
		t.Errorf("SpinnerStart wrote %q; expected nothing", buf.String())
	}
}

func TestJSONRenderer_MultipleEvents(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	r.Text("chunk1")
	r.Text("chunk2")
	r.RenderToolCall(presentation.ToolEventContext{}, types.ToolUseBlock{Name: "Read", Input: map[string]any{"file_path": "/tmp/x"}})

	lines := decodeLines(t, &buf)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), buf.String())
	}
	types := []string{"text", "text", "tool_use"}
	for i, want := range types {
		if got := lines[i]["type"]; got != want {
			t.Errorf("line %d: type = %q, want %q", i, got, want)
		}
	}
}
