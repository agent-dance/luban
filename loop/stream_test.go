package loop

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestProcessStreamThinkingBlock(t *testing.T) {
	ql := &QueryLoop{}
	var thinkingTexts []string
	onEvent := func(e Event) {
		if e.Type == EventThinking {
			thinkingTexts = append(thinkingTexts, e.Text)
		}
	}

	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeThinking}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "thinking_delta", Thinking: "Let me think..."}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "thinking_delta", Thinking: " Step 1."}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(thinkingTexts) != 2 {
		t.Errorf("expected 2 thinking events, got %d", len(thinkingTexts))
	}
	// Check ThinkingBlock in message
	var found bool
	for _, block := range msg.Content {
		if tb, ok := block.(types.ThinkingBlock); ok {
			if tb.Thinking != "Let me think... Step 1." {
				t.Errorf("unexpected thinking: '%s'", tb.Thinking)
			}
			found = true
		}
	}
	if !found {
		t.Error("expected ThinkingBlock in message content")
	}
}

func TestProcessStreamFlushWithoutStop(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e Event) {}

	// Channel closes without sending ContentBlockStop
	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: "orphan text"}},
		// No ContentBlockStop — channel just closes
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.GetText() != "orphan text" {
		t.Errorf("expected flushed text 'orphan text', got '%s'", msg.GetText())
	}
}

func TestProcessStreamFlushToolWithoutStop(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e Event) {}

	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse, ID: "orphan_tool", Name: "Bash",
			}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"command":"ls"}`}},
		// No stop — flushed
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	uses := msg.GetToolUses()
	if len(uses) != 1 {
		t.Fatalf("expected 1 flushed tool use, got %d", len(uses))
	}
	if uses[0].Name != "Bash" {
		t.Errorf("expected Bash, got %s", uses[0].Name)
	}
}

func TestProcessStreamFlushThinkingWithoutStop(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e Event) {}

	// ThinkingBlock stream closes without ContentBlockStop — flush must preserve type
	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeThinking}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "thinking_delta", Thinking: "orphan thinking"}},
		// No ContentBlockStop — channel just closes
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Must be ThinkingBlock, not TextBlock
	var found bool
	for _, block := range msg.Content {
		if tb, ok := block.(types.ThinkingBlock); ok {
			if tb.Thinking != "orphan thinking" {
				t.Errorf("unexpected thinking content: '%s'", tb.Thinking)
			}
			found = true
		}
		if _, ok := block.(types.TextBlock); ok {
			t.Error("thinking content was incorrectly flushed as TextBlock")
		}
	}
	if !found {
		t.Error("expected ThinkingBlock in flushed content")
	}
}

func TestProcessStreamUsageTracking(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e Event) {}

	// InputTokens come from MessageStart, OutputTokens from MessageDelta
	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventMessageStart,
			Usage: &types.Usage{InputTokens: 500}},
		types.StreamEvent{Type: types.EventMessageDelta,
			Usage: &types.Usage{OutputTokens: 100}},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	_, usage, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage == nil {
		t.Fatal("expected non-nil usage")
	}
	if usage.InputTokens != 500 {
		t.Errorf("expected 500 input tokens, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 100 {
		t.Errorf("expected 100 output tokens, got %d", usage.OutputTokens)
	}
}

func TestProcessStreamMessageDeltaMergesCompleteUsage(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e Event) {}

	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventMessageDelta,
			Usage: &types.Usage{
				InputTokens:              2134,
				OutputTokens:             86,
				CacheReadInputTokens:     1920,
				CacheCreationInputTokens: 128,
			}},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	_, usage, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage == nil {
		t.Fatal("expected non-nil usage")
	}
	if usage.InputTokens != 2134 {
		t.Errorf("expected input tokens from message_delta, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 86 {
		t.Errorf("expected output tokens from message_delta, got %d", usage.OutputTokens)
	}
	if usage.CacheReadInputTokens != 1920 {
		t.Errorf("expected cache read tokens from message_delta, got %d", usage.CacheReadInputTokens)
	}
	if usage.CacheCreationInputTokens != 128 {
		t.Errorf("expected cache creation tokens from message_delta, got %d", usage.CacheCreationInputTokens)
	}
}

func TestProcessStreamMessageStartAndDeltaPreserveFullUsage(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e Event) {}

	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventMessageStart,
			Usage: &types.Usage{InputTokens: 100}},
		types.StreamEvent{Type: types.EventMessageDelta,
			Usage: &types.Usage{
				InputTokens:              2134,
				OutputTokens:             86,
				CacheReadInputTokens:     1920,
				CacheCreationInputTokens: 128,
			}},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	_, usage, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage == nil {
		t.Fatal("expected non-nil usage")
	}
	if usage.InputTokens != 2134 || usage.OutputTokens != 86 ||
		usage.CacheReadInputTokens != 1920 || usage.CacheCreationInputTokens != 128 {
		t.Fatalf("unexpected merged usage: %+v", usage)
	}
}

func TestProcessStreamErrorEvent(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e Event) {}

	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventError,
			Error: &types.APIError{Type: "server_error", Message: "upstream failed"}},
	)

	_, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err == nil {
		t.Fatal("expected error from stream error event")
	}
	if err.Error() != "upstream failed" {
		t.Errorf("expected 'upstream failed', got '%s'", err.Error())
	}
}
