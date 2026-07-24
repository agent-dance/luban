package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func maxTokensTextEvents(text string, outputTokens int) []types.StreamEvent {
	reason := types.StopReasonMaxTokens
	return []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, Usage: &types.Usage{OutputTokens: outputTokens}, StopReason: &reason},
		{Type: types.EventMessageStop},
	}
}

func endTurnTextEvents(text string, outputTokens int) []types.StreamEvent {
	reason := types.StopReasonEndTurn
	return []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, Usage: &types.Usage{OutputTokens: outputTokens}, StopReason: &reason},
		{Type: types.EventMessageStop},
	}
}

func TestTokenBudgetDecisionContinuesBelowNinetyPercent(t *testing.T) {
	tracker := NewBudgetTracker()
	decision := CheckTokenBudget(tracker, "", 1000, 500)
	if !decision.Continue {
		t.Fatal("expected continuation below 90% of budget")
	}
	if decision.ContinuationCount != 1 {
		t.Fatalf("ContinuationCount = %d, want 1", decision.ContinuationCount)
	}
	if !strings.Contains(decision.NudgeMessage, "50%") {
		t.Fatalf("nudge message = %q, want percentage", decision.NudgeMessage)
	}
}

func TestTokenBudgetDecisionStopsAtNinetyPercentAndForSubagent(t *testing.T) {
	if decision := CheckTokenBudget(NewBudgetTracker(), "", 1000, 900); decision.Continue {
		t.Fatal("expected stop at 90% of budget")
	}
	if decision := CheckTokenBudget(NewBudgetTracker(), "agent-1", 1000, 500); decision.Continue {
		t.Fatal("expected subagent token budget to be disabled")
	}
	if decision := CheckTokenBudget(NewBudgetTracker(), "", 0, 500); decision.Continue {
		t.Fatal("expected non-positive token budget to be disabled")
	}
}

func TestTokenBudgetDecisionStopsOnDiminishingReturns(t *testing.T) {
	tracker := NewBudgetTracker()
	for _, tokens := range []int{1000, 1300, 1600} {
		if decision := CheckTokenBudget(tracker, "", 10000, tokens); !decision.Continue {
			t.Fatalf("expected warm-up continuation at %d tokens", tokens)
		}
	}
	decision := CheckTokenBudget(tracker, "", 10000, 1700)
	if decision.Continue {
		t.Fatal("expected diminishing returns stop")
	}
	if decision.CompletionEvent == nil || !decision.CompletionEvent.DiminishingReturns {
		t.Fatalf("completion event = %#v, want diminishing returns", decision.CompletionEvent)
	}
}

func TestMaxOutputTokensEscalatesBeforeRecoveryMessage(t *testing.T) {
	p := newParityFakeProvider([]parityProviderTurn{
		{Events: maxTokensTextEvents("partial", 1024)},
		{Events: endTurnTextEvents("done", 10)},
	})
	ql := New(p, registry.New(), Config{MaxTurns: 5, MaxTokens: 1024, Model: "claude-sonnet-4-6"})

	if err := ql.Run(context.Background(), "hi", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(p.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(p.Calls))
	}
	if p.Calls[0].MaxOutputTokensOverride != 0 {
		t.Fatalf("first override = %d, want 0", p.Calls[0].MaxOutputTokensOverride)
	}
	if p.Calls[1].MaxOutputTokensOverride != 64000 {
		t.Fatalf("second override = %d, want 64000", p.Calls[1].MaxOutputTokensOverride)
	}
	if len(p.Calls[1].Messages) != 1 || strings.Contains(p.Calls[1].Messages[0].GetText(), "continue") {
		t.Fatalf("escalation should retry the same request without a continue message: %#v", p.Calls[1].Messages)
	}
}

func TestMaxOutputTokensRecoveryUsesMessageAtMostThreeTimes(t *testing.T) {
	p := newParityFakeProvider([]parityProviderTurn{
		{Events: maxTokensTextEvents("first", 100)},
		{Events: maxTokensTextEvents("second", 100)},
		{Events: maxTokensTextEvents("third", 100)},
		{Events: maxTokensTextEvents("fourth", 100)},
		{Events: maxTokensTextEvents("fifth", 100)},
		{Events: endTurnTextEvents("done", 10)},
	})
	ql := New(p, registry.New(), Config{MaxTurns: 10, MaxTokens: 1024, Model: "claude-sonnet-4-6"})

	if err := ql.Run(context.Background(), "hi", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	recoveryMessages := 0
	for _, msg := range ql.messages {
		if msg.Role == types.RoleUser && strings.Contains(msg.GetText(), "Output token limit hit") {
			recoveryMessages++
		}
		if strings.Contains(msg.GetText(), "[continue from where you left off]") {
			t.Fatalf("legacy continue message should not be used: %#v", msg)
		}
	}
	if recoveryMessages != maxOutputTokensRecoveryLimit {
		t.Fatalf("recovery messages observed = %d, want %d", recoveryMessages, maxOutputTokensRecoveryLimit)
	}
	if len(p.Calls) < 3 || p.Calls[2].MaxOutputTokensOverride != 0 {
		t.Fatalf("recovery call should reset override, got calls=%d override=%d", len(p.Calls), p.Calls[2].MaxOutputTokensOverride)
	}
}

func TestTokenBudgetContinuationInjectsNudgeAfterStopHooks(t *testing.T) {
	p := newParityFakeProvider([]parityProviderTurn{
		{Events: endTurnTextEvents("part", 400)},
		{Events: endTurnTextEvents("done", 600)},
	})
	ql := New(p, registry.New(), Config{MaxTurns: 5, MaxTokens: 1024, TokenBudget: 1000})

	if err := ql.Run(context.Background(), "hi", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(p.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(p.Calls))
	}
	lastMsg := p.Calls[1].Messages[len(p.Calls[1].Messages)-1]
	if lastMsg.Role != types.RoleUser || !strings.Contains(lastMsg.GetText(), "Stopped at 40% of token target") {
		t.Fatalf("second call last message = %#v, want token budget nudge", lastMsg)
	}
}

func TestProviderParamsCarriesMaxOutputTokensOverride(t *testing.T) {
	state := newQueryState([]types.Message{types.UserMessage("hi")})
	state.MaxOutputTokensOverride = 12345
	q := New(newParityFakeProvider(nil), registry.New(), Config{})
	params := q.providerParams(state, QueryConfigSnapshot{MaxTokens: 1024}, state.Messages)
	if params.MaxOutputTokensOverride != 12345 {
		t.Fatalf("MaxOutputTokensOverride = %d, want 12345", params.MaxOutputTokensOverride)
	}
}
