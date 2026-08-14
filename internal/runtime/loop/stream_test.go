package loop

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func TestProcessStreamThinkingBlock(t *testing.T) {
	ql := &QueryLoop{}
	var thinkingTexts []string
	onEvent := func(e stream.Event) {
		if e.Type == stream.EventThinking {
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

func TestProcessStreamCommitFlushesTextWithoutContentBlockStop(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e stream.Event) {}

	// Channel closes without sending ContentBlockStop
	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: "orphan text"}},
		// A response commit is sufficient to authorize the final accumulator.
		types.StreamEvent{Type: types.EventMessageStop},
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.GetText() != "orphan text" {
		t.Errorf("expected flushed text 'orphan text', got '%s'", msg.GetText())
	}
}

func TestProcessStreamRejectsOpenToolAtCommit(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e stream.Event) {}

	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse, ID: "orphan_tool", Name: "Bash",
			}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"command":"ls"}`}},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	msg, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err == nil || msg != nil {
		t.Fatalf("open tool commit = (%#v, %v), want fail closed", msg, err)
	}
	var partial *PartialStreamError
	if !errors.As(err, &partial) {
		t.Fatalf("error = %T, want PartialStreamError", err)
	}
}

func TestProcessStreamCommitFlushesThinkingWithoutContentBlockStop(t *testing.T) {
	ql := &QueryLoop{}
	onEvent := func(e stream.Event) {}

	// ThinkingBlock stream closes without ContentBlockStop — flush must preserve type
	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeThinking}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "thinking_delta", Thinking: "orphan thinking"}},
		types.StreamEvent{Type: types.EventMessageStop},
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
	onEvent := func(e stream.Event) {}

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
	onEvent := func(e stream.Event) {}

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
	onEvent := func(e stream.Event) {}

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
	onEvent := func(e stream.Event) {}

	stream := makeStreamChan(
		types.StreamEvent{Type: types.EventError,
			Error: &types.APIError{Type: "server_error", Message: "upstream failed"}},
	)

	_, _, _, err := ql.processStream(context.Background(), stream, 1, onEvent)
	if err == nil {
		t.Fatal("expected error from stream error event")
	}
	var partial *PartialStreamError
	if !errors.As(err, &partial) || partial.PartialBlocks != 0 {
		t.Fatalf("error = %#v, want zero-block PartialStreamError", err)
	}
	var apiErr *types.APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "upstream failed" {
		t.Errorf("expected wrapped upstream failure, got %v", err)
	}
}

func TestProcessStreamPartialTextAndToolBatchCannotCommit(t *testing.T) {
	ql := &QueryLoop{}
	providerStream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: "provisional text"}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 1,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: "call_1", Name: "Run"}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 1,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"command":"touch forbidden"}`}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 1},
		types.StreamEvent{Type: types.EventError, Error: &types.APIError{
			Type: "api_error", Message: "diagnostic prose must not imply replay",
		}},
	)

	message, _, _, err := ql.processStream(context.Background(), providerStream, 1, func(stream.Event) {})
	if err == nil {
		t.Fatal("partial response unexpectedly succeeded")
	}
	if message != nil {
		t.Fatalf("partial response returned a commit-capable message: %#v", message)
	}
	var partial *PartialStreamError
	if !errors.As(err, &partial) || partial.PartialBlocks != 2 || partial.OpenBlocks != 0 {
		t.Fatalf("partial contract = %#v, want two closed provisional blocks", err)
	}
	contract := provider.ClassifyAttemptError(err)
	if contract.Class != types.ProviderErrorClassPermanent || contract.Retryable() {
		t.Fatalf("generic response error contract = %+v, want permanent/no retry", contract)
	}
}

func TestProcessStreamCommitSurvivesTrailingDisconnect(t *testing.T) {
	ql := &QueryLoop{}
	providerStream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: "committed"}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop, ResponseID: "resp_committed"},
		types.StreamEvent{Type: types.EventError,
			Error: &types.APIError{Type: "stream_interrupted", Message: "connection closed after commit"}},
	)

	msg, _, _, err := ql.processStream(context.Background(), providerStream, 1, func(stream.Event) {})
	if err != nil {
		t.Fatalf("committed response was rolled back by trailing disconnect: %v", err)
	}
	if msg == nil || msg.GetText() != "committed" {
		t.Fatalf("message = %#v, want committed text", msg)
	}
	if ql.lastResponseID != "resp_committed" {
		t.Fatalf("lastResponseID = %q, want committed response id", ql.lastResponseID)
	}
}

func TestProcessStreamRequiresMatchingToolCommitReceipt(t *testing.T) {
	raw := `{"steps":[{"id":"check","argv":["go","test","./..."]}]}`
	call := types.ProviderToolCallCommit{
		OutputIndex: 0, ToolType: types.ToolDefinitionTypeFunction, ProviderItemID: "item-1",
		CallID: "call-1", Name: "Run", RawInput: raw,
	}
	events := func(receipt *types.ProviderCommitReceipt) <-chan types.StreamEvent {
		return makeRawStreamChan(
			types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse, ID: call.CallID, Name: call.Name, ToolType: call.ToolType,
				ProviderItemID: call.ProviderItemID, ProviderStatus: "completed",
			}},
			types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: raw}},
			types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
			types.StreamEvent{Type: types.EventMessageStop, ProviderCommitReceipt: receipt},
		)
	}
	ql := &QueryLoop{}
	if msg, _, _, err := ql.processStream(context.Background(), events(nil), 1, func(stream.Event) {}); err == nil || msg != nil {
		t.Fatalf("missing receipt = (%#v, %v), want fail closed", msg, err)
	}
	badCall := call
	badCall.RawInput += " "
	bad := types.NewProviderToolCommitReceipt("deepseek", "responses", "completed", []types.ProviderToolCallCommit{badCall})
	if msg, _, _, err := ql.processStream(context.Background(), events(bad), 1, func(stream.Event) {}); err == nil || msg != nil {
		t.Fatalf("mismatched receipt = (%#v, %v), want fail closed", msg, err)
	}
	good := types.NewProviderToolCommitReceipt("deepseek", "responses", "completed", []types.ProviderToolCallCommit{call})
	msg, _, _, err := ql.processStream(context.Background(), events(good), 1, func(stream.Event) {})
	if err != nil || msg == nil || len(msg.GetToolUses()) != 1 {
		t.Fatalf("matching receipt = (%#v, %v), want one tool", msg, err)
	}
}
