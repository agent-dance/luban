package loop

import (
	"context"
	"strings"
	"testing"

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

func TestProcessStreamTextOnly(t *testing.T) {
	ql := &QueryLoop{}
	var texts []string
	onEvent := func(e Event) {
		if e.Type == EventText {
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
	var events []Event
	onEvent := func(e Event) { events = append(events, e) }

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
	if !msg.HasToolUse() {
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
	onEvent := func(e Event) {}

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
	onEvent := func(e Event) {
		if e.Type == EventSystemWarning {
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
	// H12: malformed JSON should produce a text block instead of a tool_use block
	uses := msg.GetToolUses()
	if len(uses) != 0 {
		t.Fatalf("expected 0 tool uses for bad JSON (should be skipped), got %d", len(uses))
	}
	// Should have a text block with the skip message
	found := false
	for _, block := range msg.Content {
		if tb, ok := block.(types.TextBlock); ok {
			if strings.Contains(tb.Text, "skipped: malformed input JSON") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("expected text block with skip message for malformed JSON tool")
	}
}

func TestProcessStreamEmptyToolJSON(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e Event) {}

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
	onEvent := func(e Event) {}

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
	if !msg.HasToolUse() {
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
	onEvent := func(e Event) {}

	stream := makeStreamChan() // empty, immediately closed

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msg.Content) != 0 {
		t.Errorf("expected 0 content blocks, got %d", len(msg.Content))
	}
}

func TestProcessStreamContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	ql := &QueryLoop{}
	onEvent := func(e Event) {}

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
	onEvent := func(e Event) {}

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
