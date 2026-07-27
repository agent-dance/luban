package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

type rendererExecutionEvidence struct{}

func (rendererExecutionEvidence) ToolExecutionEvidence() runtimeevent.ToolExecutionEvidence {
	return runtimeevent.ToolExecutionEvidence{
		LogicalExecutionCommitted: true, RevisionSealDisposition: "revision_bound",
		PhysicalSteps: []runtimeevent.PhysicalToolStepEvidence{
			{Ordinal: 1, StartedOffsetMS: 2, EndedOffsetMS: 6, DurationMS: 4, Outcome: "succeeded", StdoutBytes: 9},
		},
	}
}

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
	if lines[0]["schema_version"] != runtimeevent.MachineEventSchemaVersion || lines[0]["input_ref"] == nil || lines[0]["metrics"] == nil {
		t.Fatalf("safe tool input projection = %#v", lines[0])
	}
	if _, leaked := lines[0]["input"]; leaked {
		t.Fatalf("tool input bypassed content reference: %#v", lines[0])
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
	if lines[0]["schema_version"] != runtimeevent.MachineEventSchemaVersion || lines[0]["content_ref"] == nil || lines[0]["metrics"] == nil {
		t.Fatalf("safe tool result projection = %#v", lines[0])
	}
	if _, leaked := lines[0]["output"]; leaked {
		t.Fatalf("tool output bypassed content reference: %#v", lines[0])
	}
	if got := lines[0]["is_error"]; got != false {
		t.Errorf("is_error = %v, want false", got)
	}
}

func TestJSONRendererProjectsCompoundPhysicalExecutionEvidence(t *testing.T) {
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	r.RenderToolResult(presentation.ToolEventContext{}, types.ToolResultBlock{
		ToolUseID: "toolu-render-compound", Data: rendererExecutionEvidence{}, Outcome: types.ToolOutcomeSucceeded,
	})
	lines := decodeLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("events = %#v", lines)
	}
	metrics, ok := lines[0]["metrics"].(map[string]any)
	if !ok || metrics["logical_execution_committed"] != true || metrics["physical_child_operations"] != float64(1) ||
		metrics["revision_seal_disposition"] != "revision_bound" {
		t.Fatalf("compound execution metrics = %#v", lines[0]["metrics"])
	}
	steps, ok := metrics["physical_steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("compound physical steps = %#v", metrics["physical_steps"])
	}
	step, ok := steps[0].(map[string]any)
	if !ok || step["ordinal"] != float64(1) || step["started_offset_ms"] != float64(2) ||
		step["ended_offset_ms"] != float64(6) || step["duration_ms"] != float64(4) ||
		step["stdout_bytes"] != float64(9) || step["operation_id"] == "" {
		t.Fatalf("compound physical step = %#v", steps[0])
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
	if result["schema_version"] != runtimeevent.MachineEventSchemaVersion {
		t.Errorf("typed runtime result projection = %#v", result)
	}
	projection, ok := result["runtime_event"].(map[string]any)
	if !ok || projection["schema_version"] != types.RuntimeEventSchemaVersion || projection["audience"] != "sdk" ||
		projection["redaction_level"] != "strict" || projection["context_generation"] != float64(9) || projection["event_id"] == "" ||
		projection["public_key"] != "runtime.tool_result.public_summary" || projection["code"] != "tool.result" {
		t.Errorf("strict runtime result semantics = %#v", result)
	}
	ref, ok := result["content_ref"].(map[string]any)
	if !ok || ref["algorithm"] != "sha256" || ref["digest"] == "" || ref["scope"] != "tool_result_envelope" {
		t.Errorf("content reference = %#v", result["content_ref"])
	}
	metrics, ok := result["metrics"].(map[string]any)
	if !ok || metrics["content_bytes"] != float64(len("structured evidence")) ||
		metrics["content_block_count"] != float64(1) || metrics["data_present"] != true ||
		metrics["metadata_count"] != float64(1) || metrics["new_message_count"] != float64(1) {
		t.Errorf("tool result metrics = %#v", result["metrics"])
	}
	usage, ok := metrics["usage"].(map[string]any)
	if !ok || usage["input_tokens"] != float64(7) || usage["output_tokens"] != float64(3) {
		t.Errorf("usage metrics = %#v", metrics["usage"])
	}
	for _, raw := range []string{"output", "content", "content_blocks", "data", "metadata", "usage", "new_messages", "project_root"} {
		if _, leaked := result[raw]; leaked {
			t.Errorf("tool result retained raw field %q: %#v", raw, result)
		}
	}
}

func TestJSONRendererRedactsToolSecretsBeforeSerialization(t *testing.T) {
	const secret = "API_TOKEN=sk-stream-json-secret"
	var output bytes.Buffer
	renderer := ui.NewJSONRenderer(&output)
	ctx := presentation.ToolEventContext{SessionID: "session-safe", TurnID: "turn-safe"}
	call := types.ToolUseBlock{
		ID: "toolu-safe", Name: "Bash",
		Input: map[string]any{"command": "env " + secret, "nested": map[string]any{"authorization": secret}},
	}
	result := types.ToolResultBlock{
		ToolUseID: "toolu-safe", Content: secret,
		ContentBlocks: []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: secret}},
		Data: map[string]any{
			"OriginalFile": secret,
			"nested":       map[string]any{"environment": []any{map[string]any{"value": secret}}},
		},
		Metadata:    map[string]string{"authorization": secret},
		NewMessages: []types.Message{types.UserMessage(secret)},
		Outcome:     types.ToolOutcomeSucceeded,
	}
	renderer.RenderToolCall(ctx, call)
	renderer.RenderToolResult(ctx, result)

	encoded := output.String()
	for _, forbidden := range []string{
		secret, "sk-stream-json-secret", "OriginalFile", "authorization", "environment",
		`"input"`, `"output"`, `"content"`, `"content_blocks"`, `"data"`, `"metadata"`, `"new_messages"`,
	} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("stream-json tool event leaked %q: %s", forbidden, encoded)
		}
	}
	if result.Content != secret || result.Data.(map[string]any)["OriginalFile"] != secret {
		t.Fatalf("renderer mutated the model-visible tool result: %#v", result)
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

func TestJSONRenderer_ContextFailureKeepsOnlyAllowlistedMachineCode(t *testing.T) {
	const privateMessage = "private provider context diagnostic"
	var output bytes.Buffer
	renderer := ui.NewJSONRenderer(&output)
	renderer.RuntimeErrorEvent(
		presentation.ToolEventContext{},
		"",
		privateMessage,
		&types.APIError{Type: "context_length_exceeded", Message: privateMessage, Status: 400},
		map[string]any{"private": privateMessage},
	)

	lines := decodeLines(t, &output)
	if len(lines) != 1 || lines[0]["type"] != "error" ||
		lines[0]["schema_version"] != types.RuntimeEventSchemaVersion ||
		lines[0]["kind"] != "runtime_error" || lines[0]["outcome"] != "failed" ||
		lines[0]["code"] != "context_length_exceeded" {
		t.Fatalf("context terminal projection = %#v", lines)
	}
	encoded, err := json.Marshal(lines[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), privateMessage) {
		t.Fatalf("context projection leaked provider diagnostics: %s", encoded)
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

func TestJSONRenderer_AgenticMetricsAreContentFree(t *testing.T) {
	const secret = "secret command /private/workspace raw output"
	var buf bytes.Buffer
	r := ui.NewJSONRenderer(&buf)
	ctx := presentation.ToolEventContext{
		SessionID: "session-metrics", ProjectRoot: "/private/workspace",
		TurnID: "turn-metrics", ActorID: "agent-metrics", WorkUnitID: "work-metrics",
	}
	r.RenderRequestMetrics(ctx, stream.EventRequestFailed, &stream.RequestStatusEvent{
		RequestID: "request-metrics", StartedAt: "2026-07-26T00:00:00Z", EndedAt: "2026-07-26T00:00:01Z",
		Attempt: 2, MaxRetries: 3, RetryCount: 1, FirstTokenMilliseconds: 120, TotalMilliseconds: 400,
		InputTokens: 100, CacheReadInputTokens: 80, CacheWriteInputTokens: 5, OutputTokens: 10,
		Error: secret,
	})
	r.RenderToolRoundMetrics(ctx, &stream.ToolRoundMetricsEvent{
		RoundID: "turn-metrics", LogicalModelVisibleCalls: 4, PhysicalChildOperations: 4,
		Fanout: 3, BatchCount: 2, QueueMilliseconds: 8, CriticalPathMilliseconds: 50,
		TotalChildLatencyMilliseconds: 90, ErrorCount: 1,
	})

	lines := decodeLines(t, &buf)
	if len(lines) != 2 || lines[0]["metric"] != "provider_request" || lines[1]["metric"] != "tool_round" {
		t.Fatalf("metrics lines = %#v", lines)
	}
	request, ok := lines[0]["request_status"].(map[string]any)
	if !ok || request["request_id"] != "request-metrics" || request["cache_read_input_tokens"] != float64(80) || request["failed"] != true {
		t.Fatalf("request metrics = %#v", request)
	}
	round, ok := lines[1]["tool_round"].(map[string]any)
	if !ok || round["logical_model_visible_calls"] != float64(4) || round["fanout"] != float64(3) || round["error_count"] != float64(1) {
		t.Fatalf("round metrics = %#v", round)
	}
	encoded := buf.String()
	for _, forbidden := range []string{secret, "/private/workspace", "secret command", "raw output"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("telemetry leaked %q: %s", forbidden, encoded)
		}
	}
}
