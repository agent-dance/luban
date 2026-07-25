package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func fallbackStreamEvents() []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type:      types.ContentTypeThinking,
			Signature: "sig_old_attempt",
		}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type:     "thinking_delta",
			Thinking: "orphan thinking",
		}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventContentBlockStart, Index: 1, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 1, Delta: &types.ContentDelta{Type: "text_delta", Text: "orphan text"}},
		{Type: types.EventContentBlockStop, Index: 1},
		{Type: types.EventError, Error: &types.APIError{
			Type:          "fallback_triggered",
			Message:       "high demand",
			OriginalModel: "primary-model",
			FallbackModel: "fallback-model",
		}},
	}
}

func TestFallbackTombstonesOrphanAndRetriesFallbackModel(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: fallbackStreamEvents()},
		{Events: parityTextEvents("fallback text")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 1, Model: "primary-model", MaxTokens: 1024})
	q.messages = []types.Message{
		types.UserMessage("before"),
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{types.ThinkingBlock{
				Type:      types.ContentTypeThinking,
				Thinking:  "previous protected thought",
				Signature: "sig_model_bound",
			}},
		},
	}

	var events []stream.Event
	if err := q.Run(context.Background(), "hello", func(evt stream.Event) {
		events = append(events, evt)
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(prov.Calls))
	}
	if prov.Calls[0].Model != "primary-model" {
		t.Fatalf("first model = %q, want primary-model", prov.Calls[0].Model)
	}
	if prov.Calls[1].Model != "fallback-model" {
		t.Fatalf("fallback model = %q, want fallback-model", prov.Calls[1].Model)
	}
	if hasThinkingSignature(prov.Calls[1].Messages) {
		t.Fatalf("fallback request retained thinking signature: %+v", prov.Calls[1].Messages)
	}

	var sawTombstone bool
	for _, evt := range events {
		if evt.Type == stream.EventTombstone && evt.Tombstone != nil && evt.Tombstone.Reason == "model_fallback" {
			sawTombstone = true
		}
	}
	if !sawTombstone {
		t.Fatalf("missing fallback tombstone event: %+v", events)
	}

	for _, msg := range q.messages {
		if strings.Contains(messageTextForTest(msg), "orphan text") || strings.Contains(messageTextForTest(msg), "orphan thinking") {
			t.Fatalf("orphan fallback content persisted in transcript: %+v", q.messages)
		}
	}
	if !strings.Contains(joinMessagesForTest(q.messages), "fallback text") {
		t.Fatalf("fallback response not persisted: %+v", q.messages)
	}
}

func TestFallbackTriggeredErrorFromCreateStreamUsesFallbackModel(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Error: &types.APIError{Type: "fallback_triggered", Message: "busy", OriginalModel: "primary-model", FallbackModel: "fallback-model"}},
		{Events: parityTextEvents("ok")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 1, Model: "primary-model", MaxTokens: 1024})

	if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 2 || prov.Calls[1].Model != "fallback-model" {
		t.Fatalf("calls/models = %+v, want second fallback-model", prov.Calls)
	}
}

func TestFallbackRebindsGoalEvaluatorAndUsageEventModel(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Error: &types.APIError{Type: "fallback_triggered", Message: "busy", OriginalModel: "primary-model", FallbackModel: "fallback-model"}},
		{Events: endTurnTextEvents("fallback answer", 10)},
		{Events: endTurnTextEvents(`{"met":true,"reason":"fallback answer proves completion"}`, 3)},
	})
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	q := New(prov, registry.New(), Config{
		MaxTurns: 2, Model: "primary-model", MaxTokens: 1024,
		GoalRuntime: runtime, GoalEvaluator: NewProviderGoalEvaluatorWithModel(prov, "primary-model"),
	})

	var events []stream.Event
	if err := q.Run(context.Background(), "finish", func(event stream.Event) { events = append(events, event) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 3 {
		t.Fatalf("provider calls = %d, want 3", len(prov.Calls))
	}
	if got := prov.Calls[2].Model; got != "fallback-model" {
		t.Fatalf("goal evaluator model = %q, want fallback-model", got)
	}
	for _, event := range events {
		if event.Type == stream.EventGoalEvaluation {
			if got, _ := event.Metadata["model"].(string); got != "fallback-model" {
				t.Fatalf("goal evaluator event model = %q, want fallback-model", got)
			}
			return
		}
	}
	t.Fatal("missing goal evaluator usage event")
}

func hasThinkingSignature(messages []types.Message) bool {
	for _, msg := range messages {
		for _, block := range msg.Content {
			if thinking, ok := block.(types.ThinkingBlock); ok && thinking.Signature != "" {
				return true
			}
		}
	}
	return false
}

func messageTextForTest(msg types.Message) string {
	var b strings.Builder
	for _, block := range msg.Content {
		switch typed := block.(type) {
		case types.TextBlock:
			b.WriteString(typed.Text)
		case types.ThinkingBlock:
			b.WriteString(typed.Thinking)
		case types.ToolResultBlock:
			b.WriteString(typed.TextContent())
		}
	}
	return b.String()
}

func joinMessagesForTest(messages []types.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(messageTextForTest(msg))
		b.WriteByte('\n')
	}
	return b.String()
}
