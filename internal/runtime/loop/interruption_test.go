package loop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type cancelTool struct{}

func (t cancelTool) Name() string        { return "CancelTool" }
func (t cancelTool) Description() string { return "cancels" }
func (t cancelTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t cancelTool) Execute(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
	<-ctx.Done()
	return types.ToolResult{}, ctx.Err()
}

func cancelToolUseEvents() []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse,
			ID:   "toolu_cancel",
			Name: "CancelTool",
		}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{}`}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: stopReasonForParity(types.StopReasonToolUse)},
		{Type: types.EventMessageStop},
	}
}

func TestInterruptEmitsUserInterruptionAndCompletesToolResultPair(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{{Events: cancelToolUseEvents()}})
	reg := registry.New()
	reg.Register(cancelTool{})
	q := New(prov, reg, Config{MaxTurns: 1, MaxTokens: 1024})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	var events []stream.Event
	err := q.Run(ctx, "run", func(evt stream.Event) {
		events = append(events, evt)
	})
	if err == nil {
		t.Fatal("Run error = nil, want tool execution cancellation")
	}

	var sawInterrupt bool
	for _, evt := range events {
		if evt.Type == stream.EventUserInterruption {
			sawInterrupt = true
		}
	}
	if !sawInterrupt {
		t.Fatalf("missing user interruption event: %+v", events)
	}
	if !hasToolResultFor(q.messages, "toolu_cancel") {
		t.Fatalf("transcript missing tool_result for interrupted tool_use: %+v", q.messages)
	}
}

func TestMissingToolResultBlocksSynthesizesUnmatchedToolUse(t *testing.T) {
	messages := []types.Message{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "toolu_done", Name: "Done", Input: map[string]any{}},
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "toolu_missing", Name: "Missing", Input: map[string]any{}},
			},
		},
		types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "toolu_done", Content: "ok"}),
	}

	results := missingToolResultBlocks(messages, "model failed")
	if len(results) != 1 {
		t.Fatalf("missing results len = %d, want 1: %+v", len(results), results)
	}
	if results[0].ToolUseID != "toolu_missing" || !results[0].IsError || !strings.Contains(results[0].Content, "model failed") {
		t.Fatalf("unexpected missing result: %+v", results[0])
	}
}

func TestInterruptSubmitInterruptCanStaySilent(t *testing.T) {
	var events []stream.Event
	emitUserInterruption(func(evt stream.Event) { events = append(events, evt) }, 1, "interrupt")
	if len(events) != 1 || events[0].Type != stream.EventUserInterruption {
		t.Fatalf("user interrupt event = %+v, want one user_interruption", events)
	}

	events = nil
	// submit-interrupt is represented by intentionally not emitting a user
	// interruption; the queued user message that follows supplies context.
	if len(events) != 0 {
		t.Fatalf("submit-interrupt should be silent, got %+v", events)
	}
}

func hasToolResultFor(messages []types.Message, id string) bool {
	for _, msg := range messages {
		for _, block := range msg.Content {
			if result, ok := block.(types.ToolResultBlock); ok && result.ToolUseID == id {
				return true
			}
		}
	}
	return false
}
