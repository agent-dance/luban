package loop

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/types"
)

func TestQueryLoopCompactionParityProviderFakes(t *testing.T) {
	t.Run("prompt too long on main request is modeled without credentials", func(t *testing.T) {
		prov := newParityFakeProvider([]parityProviderTurn{{
			Error: &types.APIError{Type: "invalid_request_error", Message: "prompt is too long", Status: 400},
		}})
		ql := New(prov, newParityRegistry(t, nil), Config{MaxTurns: 1})

		err := ql.Run(context.Background(), "oversized prompt", func(Event) {})
		if err == nil || !strings.Contains(err.Error(), "prompt is too long") {
			t.Fatalf("Run error = %v, want prompt-too-long error", err)
		}
		if len(prov.Calls) != 1 {
			t.Fatalf("provider calls = %d, want 1", len(prov.Calls))
		}
	})
}

func TestQueryLoopCompactionParityRegressionSlots(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{"task_08_pre_call_auto_compact", assertPreCallAutoCompactSlot},
		{"task_08_no_tool_turn_auto_compact", assertNoToolTurnAutoCompactSlot},
		{"task_09_reactive_compact_retry", skipPendingLoopCompactionParity("task_09", "reactive compact retry after context overflow")},
		{"task_07_circuit_breaker_prevents_repeated_auto_compact", assertAutoCompactCircuitBreakerSlot},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, tt.run)
	}
}

func assertPreCallAutoCompactSlot(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("done")}})
	ql := New(prov, newParityRegistry(t, nil), Config{
		MaxTurns:         1,
		MaxContextTokens: 100,
	})
	ql.compactor = &countingCompactor{}
	ql.messages = manyUserMessages(30)

	if err := ql.runLoop(context.Background(), func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if got := ql.compactor.(*countingCompactor).calls; got != 1 {
		t.Fatalf("compactor calls = %d, want 1 before provider request", got)
	}
	if len(prov.Calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(prov.Calls))
	}
	if got := joinedText(prov.Calls[0].Messages); !strings.Contains(got, "summary for") {
		t.Fatalf("provider request did not use compacted messages: %q", got)
	}
}

func assertNoToolTurnAutoCompactSlot(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("terminal")}})
	ql := New(prov, newParityRegistry(t, nil), Config{
		MaxTurns:         1,
		MaxContextTokens: 100,
	})
	ql.compactor = &countingCompactor{}
	ql.messages = manyUserMessages(30)

	if err := ql.runLoop(context.Background(), func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if got := ql.compactor.(*countingCompactor).calls; got != 1 {
		t.Fatalf("compactor calls = %d, want 1 before no-tool assistant response", got)
	}
	if got := joinedEventTextFromMessages(ql.Messages()); !strings.Contains(got, "terminal") {
		t.Fatalf("final messages do not include terminal answer: %q", got)
	}
}

func assertAutoCompactCircuitBreakerSlot(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: parityToolUseEventsWithUsage("call_1", "Echo", `{"text":"large"}`, &types.Usage{InputTokens: 100000, OutputTokens: 10})},
		{Events: parityTextEvents("done")},
	})
	reg := newParityRegistry(t, []parityToolFixture{
		{Name: "Echo", Kind: "echo", EchoPrefix: "echo: "},
	})
	ql := New(prov, reg, Config{
		MaxTurns:         3,
		MaxContextTokens: 100,
	})
	ql.compactor = &countingCompactor{}
	for i := 0; i < compact.MaxConsecutiveAutocompactFailures; i++ {
		ql.ctxWindow.RecordCompactFailure()
	}

	err := ql.Run(context.Background(), "run tool", func(Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if got := ql.compactor.(*countingCompactor).calls; got != 0 {
		t.Fatalf("compactor calls = %d, want 0 after circuit breaker trips", got)
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("provider calls = %d, want 2; circuit breaker should continue without compacting", len(prov.Calls))
	}
	if got := joinedEventTextFromMessages(ql.Messages()); !strings.Contains(got, "done") {
		t.Fatalf("final messages do not include terminal answer: %q", got)
	}
}

func skipPendingLoopCompactionParity(taskID, behavior string) func(*testing.T) {
	return func(t *testing.T) {
		t.Skipf("TODO(%s): %s is not implemented yet; parity slot is reserved", taskID, behavior)
	}
}

type countingCompactor struct {
	calls int
}

func (c *countingCompactor) Compact(_ context.Context, messages []types.Message, _ int) (*compact.CompactionResult, error) {
	c.calls++
	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "auto"})
	return &compact.CompactionResult{
		BoundaryMarker:  &boundary,
		SummaryMessages: []types.Message{types.UserMessage(fmt.Sprintf("summary for %d messages", len(messages)))},
	}, nil
}

func (c *countingCompactor) CompactWithTrigger(ctx context.Context, messages []types.Message, keepRecent int, _ string) (*compact.CompactionResult, error) {
	return c.Compact(ctx, messages, keepRecent)
}

func parityToolUseEventsWithUsage(id, name, input string, usage *types.Usage) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse,
			ID:   id,
			Name: name,
		}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: input}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, Usage: usage, StopReason: stopReasonForParity(types.StopReasonToolUse)},
		{Type: types.EventMessageStop},
	}
}

func joinedEventTextFromMessages(messages []types.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		if msg.Role == types.RoleAssistant {
			b.WriteString(msg.GetText())
		}
	}
	return b.String()
}
