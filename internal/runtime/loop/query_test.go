package loop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/types"
)

func makeStreamChan(events ...types.StreamEvent) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch
}

type testVisibleReadEvidenceReceipt struct {
	commits []string
}

func (r *testVisibleReadEvidenceReceipt) CommitVisibleReadEvidence(content string) bool {
	r.commits = append(r.commits, content)
	return true
}

func TestVisibleReadEvidenceCommitsOnlyForExactPersistedToolResult(t *testing.T) {
	receipt := &testVisibleReadEvidenceReceipt{}
	expected := types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "inspect-1",
		Content: `{"requests":[]}`, Data: receipt,
	}
	commitVisibleReadEvidenceReceipts(nil, []types.ToolResultBlock{expected})
	commitVisibleReadEvidenceReceipts([]types.Message{types.ToolResultMessage(types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: expected.ToolUseID,
		Content: expected.Content + "truncated", Data: receipt,
	})}, []types.ToolResultBlock{expected})
	commitVisibleReadEvidenceReceipts([]types.Message{types.ToolResultMessage(types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "other",
		Content: expected.Content, Data: receipt,
	})}, []types.ToolResultBlock{expected})
	if len(receipt.commits) != 0 {
		t.Fatalf("non-visible receipt committed: %#v", receipt.commits)
	}

	commitVisibleReadEvidenceReceipts([]types.Message{types.ToolResultMessage(types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: expected.ToolUseID,
		Content: expected.Content, Data: receipt,
	})}, []types.ToolResultBlock{expected})
	if len(receipt.commits) != 1 || receipt.commits[0] != expected.Content {
		t.Fatalf("exact visible receipt commits = %#v", receipt.commits)
	}
}

func TestProcessStreamTextOnly(t *testing.T) {
	ql := &QueryLoop{}
	var texts []string
	onEvent := func(e stream.Event) {
		if e.Type == stream.EventText {
			texts = append(texts, e.Text)
		}
	}

	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: "Hello "}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: "world"}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.GetText() != "Hello world" {
		t.Errorf("expected 'Hello world', got '%s'", msg.GetText())
	}
	if len(texts) != 2 {
		t.Errorf("expected 2 text events, got %d", len(texts))
	}
}

func TestProcessStreamToolUse(t *testing.T) {
	ql := &QueryLoop{}
	var events []stream.Event
	onEvent := func(e stream.Event) { events = append(events, e) }

	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse,
				ID:   "tool_123",
				Name: "Bash",
			}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"com`}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `mand":"ls -la"}`}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msg.GetToolUses()) == 0 {
		t.Fatal("expected message to have tool use")
	}
	uses := msg.GetToolUses()
	if len(uses) != 1 {
		t.Fatalf("expected 1 tool use, got %d", len(uses))
	}
	if uses[0].Name != "Bash" {
		t.Errorf("expected tool name 'Bash', got '%s'", uses[0].Name)
	}
	if uses[0].ID != "tool_123" {
		t.Errorf("expected tool ID 'tool_123', got '%s'", uses[0].ID)
	}
	cmd, ok := uses[0].Input["command"].(string)
	if !ok || cmd != "ls -la" {
		t.Errorf("expected input command 'ls -la', got '%v'", uses[0].Input["command"])
	}
}

func TestProcessStreamToolUseWithPrompt(t *testing.T) {
	// This specifically tests the Agent tool scenario from the bug report
	ql := &QueryLoop{}
	onEvent := func(e stream.Event) {}

	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse,
				ID:   "tool_agent",
				Name: "Agent",
			}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"prompt":"analyze the project structure"}`}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	uses := msg.GetToolUses()
	if len(uses) != 1 {
		t.Fatalf("expected 1 tool use, got %d", len(uses))
	}
	prompt, ok := uses[0].Input["prompt"].(string)
	if !ok {
		t.Fatalf("'prompt' field missing or not a string, input: %v", uses[0].Input)
	}
	if prompt != "analyze the project structure" {
		t.Errorf("expected prompt text, got '%s'", prompt)
	}
}

func TestProcessStreamInvalidToolJSON(t *testing.T) {
	ql := &QueryLoop{}
	var gotWarning bool
	onEvent := func(e stream.Event) {
		if e.Type == stream.EventSystemWarning {
			gotWarning = true
		}
	}

	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse,
				ID:   "tool_bad",
				Name: "Bash",
			}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{invalid json`}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotWarning {
		t.Error("expected warning event for invalid JSON")
	}
	// Malformed JSON is durable audit state, never assistant prose and never an
	// executable tool call.
	uses := msg.GetToolUses()
	if len(uses) != 0 {
		t.Fatalf("expected 0 tool uses for bad JSON (should be skipped), got %d", len(uses))
	}
	invalid := msg.GetInvalidToolUses()
	if len(invalid) != 1 {
		t.Fatalf("invalid tool uses = %#v, want one structured failure", invalid)
	}
	if invalid[0].Name != "Bash" || invalid[0].ID != "tool_bad" || invalid[0].FailureKind != types.ToolInputFailureInvalidJSON || !invalid[0].Recoverable {
		t.Fatalf("invalid tool use = %#v", invalid[0])
	}
	if invalid[0].RawInput != `{invalid json` || invalid[0].InputBytes != len(`{invalid json`) || !strings.HasPrefix(invalid[0].InputDigest, "sha256:") {
		t.Fatalf("invalid tool diagnostic = %#v", invalid[0])
	}
	for _, block := range msg.Content {
		if _, ok := block.(types.TextBlock); ok {
			t.Fatalf("malformed tool input was converted into assistant prose: %#v", msg.Content)
		}
	}
}

func TestProcessStreamEmptyToolJSON(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e stream.Event) {}

	// No input_json_delta events at all
	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse,
				ID:   "tool_empty",
				Name: "Agent",
			}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	uses := msg.GetToolUses()
	if len(uses) != 1 {
		t.Fatalf("expected 1 tool use, got %d", len(uses))
	}
	// Empty input, not nil
	if uses[0].Input == nil {
		t.Error("expected non-nil empty map, got nil")
	}
}

func TestProcessStreamMixedTextAndToolUse(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e stream.Event) {}

	stream := makeStreamChan(
		// Text block
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: "Let me check."}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		// Tool use block
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 1,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse,
				ID:   "tool_1",
				Name: "Read",
			}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 1,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"file_path":"/tmp/test.go"}`}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 1},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.GetText() != "Let me check." {
		t.Errorf("expected text 'Let me check.', got '%s'", msg.GetText())
	}
	if len(msg.GetToolUses()) == 0 {
		t.Error("expected tool use in mixed message")
	}
	uses := msg.GetToolUses()
	fp, ok := uses[0].Input["file_path"].(string)
	if !ok || fp != "/tmp/test.go" {
		t.Errorf("expected file_path '/tmp/test.go', got '%v'", uses[0].Input["file_path"])
	}
}

func TestProcessStreamEmptyStream(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e stream.Event) {}

	stream := makeStreamChan() // empty, immediately closed

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err == nil {
		t.Fatal("expected an uncommitted-response error")
	}
	if msg != nil {
		t.Fatalf("uncommitted empty stream returned message: %#v", msg)
	}
	var partial *PartialStreamError
	if !errors.As(err, &partial) || !partial.SafeToReplay() || partial.PartialBlocks != 0 {
		t.Fatalf("error = %#v, want replay-safe empty PartialStreamError", err)
	}
}

func TestProcessStreamContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	ql := &QueryLoop{}
	onEvent := func(e stream.Event) {}

	// Stream with data, but context already cancelled
	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
	)

	_, _, _, err := ql.processStream(ctx, stream, 1, onEvent)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestProcessStreamInterleavedToolCalls(t *testing.T) {
	// This tests the exact bug from the screenshot: OpenAI sends
	// interleaved deltas for multiple tool_calls in the same response.
	// With the old single-variable state, tool B's JSON would get
	// appended to tool A's builder, producing invalid JSON.
	ql := &QueryLoop{}
	onEvent := func(e stream.Event) {}

	stream := makeStreamChan(
		// Tool A starts at index 1
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 1,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse, ID: "tool_a", Name: "Bash",
			}},
		// Tool B starts at index 2
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 2,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse, ID: "tool_b", Name: "Read",
			}},
		// Interleaved deltas: A, B, A, B
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 1,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"comm`}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 2,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"file`}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 1,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `and":"ls"}`}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 2,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `_path":"/tmp/x"}`}},
		// Both stop
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 1},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 2},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	uses := msg.GetToolUses()
	if len(uses) != 2 {
		t.Fatalf("expected 2 tool uses, got %d", len(uses))
	}

	// Verify each tool got its own JSON, not mixed
	var bashTool, readTool *types.ToolUseBlock
	for i := range uses {
		switch uses[i].Name {
		case "Bash":
			bashTool = &uses[i]
		case "Read":
			readTool = &uses[i]
		}
	}

	if bashTool == nil {
		t.Fatal("missing Bash tool use")
	}
	cmd, ok := bashTool.Input["command"].(string)
	if !ok || cmd != "ls" {
		t.Errorf("Bash: expected command 'ls', got %v", bashTool.Input)
	}

	if readTool == nil {
		t.Fatal("missing Read tool use")
	}
	fp, ok := readTool.Input["file_path"].(string)
	if !ok || fp != "/tmp/x" {
		t.Errorf("Read: expected file_path '/tmp/x', got %v", readTool.Input)
	}
}
