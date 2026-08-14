package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
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

func TestMaxTokensCommitsPartialWithoutAutomaticRetry(t *testing.T) {
	p := newParityFakeProvider([]parityProviderTurn{
		{Events: maxTokensTextEvents("partial", 1024)},
		{Events: endTurnTextEvents("done", 10)},
	})
	ql := New(p, registry.New(), Config{MaxTurns: 5, MaxTokens: 1024, Model: "claude-sonnet-4-6"})

	var events []stream.Event
	if err := ql.Run(context.Background(), "hi", func(event stream.Event) { events = append(events, event) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(p.Calls) != 1 {
		t.Fatalf("CreateStream calls = %d, want 1", len(p.Calls))
	}
	if p.Calls[0].MaxOutputTokensOverride != 0 {
		t.Fatalf("first override = %d, want 0", p.Calls[0].MaxOutputTokensOverride)
	}
	messages := ql.Messages()
	if len(messages) != 2 || messages[1].GetText() != "partial" || messages[1].StopReason != types.StopReasonMaxTokens {
		t.Fatalf("durable messages = %#v, want max_tokens partial", messages)
	}
	var warnings, errors, maxTokenEnds int
	for _, event := range events {
		switch event.Type {
		case stream.EventSystemWarning:
			if event.RuntimeEvent != nil && event.RuntimeEvent.PrivateMetadata["terminal_reason"] == string(types.StopReasonMaxTokens) {
				warnings++
			}
		case stream.EventError:
			errors++
		case stream.EventTurnEnd:
			if event.TerminalReason == string(types.StopReasonMaxTokens) {
				maxTokenEnds++
			}
		}
	}
	if warnings != 1 || errors != 0 || maxTokenEnds != 1 {
		t.Fatalf("events warning/error/max_tokens turn_end = %d/%d/%d, want 1/0/1", warnings, errors, maxTokenEnds)
	}
}

func TestMaxTokensDoesNotInjectRecoveryMessages(t *testing.T) {
	p := newParityFakeProvider([]parityProviderTurn{
		{Events: maxTokensTextEvents("first", 100)},
		{Events: maxTokensTextEvents("second", 100)},
		{Events: maxTokensTextEvents("third", 100)},
		{Events: maxTokensTextEvents("fourth", 100)},
		{Events: maxTokensTextEvents("fifth", 100)},
		{Events: endTurnTextEvents("done", 10)},
	})
	ql := New(p, registry.New(), Config{MaxTurns: 10, MaxTokens: 1024, Model: "claude-sonnet-4-6"})

	if err := ql.Run(context.Background(), "hi", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(p.Calls) != 1 {
		t.Fatalf("CreateStream calls = %d, want 1", len(p.Calls))
	}
	for _, msg := range ql.messages {
		if msg.Role == types.RoleUser && strings.Contains(msg.GetText(), "Output token limit hit") {
			t.Fatalf("automatic recovery message leaked into durable history: %#v", msg)
		}
	}
}

type maxTokensCountingTool struct{ calls int }

func (t *maxTokensCountingTool) Name() string { return "Mutate" }
func (t *maxTokensCountingTool) Description() string {
	return "mutates test state" // i18n:allow test-only tool metadata
}
func (t *maxTokensCountingTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *maxTokensCountingTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	t.calls++
	return types.ToolResult{Content: "mutated"}, nil
}

func TestMaxTokensQuarantinesSyntacticallyCompleteToolCall(t *testing.T) {
	reason := types.StopReasonMaxTokens
	p := newParityFakeProvider([]parityProviderTurn{
		{Events: []types.StreamEvent{
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "safe partial"}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventContentBlockStart, Index: 1, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: "call-cutoff", Name: "Mutate"}},
			{Type: types.EventContentBlockDelta, Index: 1, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"value":1}`}},
			{Type: types.EventContentBlockStop, Index: 1},
			{Type: types.EventMessageDelta, Usage: &types.Usage{OutputTokens: 100}, StopReason: &reason},
			{Type: types.EventMessageStop, ResponseID: "response-incomplete"},
		}},
	})
	tool := &maxTokensCountingTool{}
	reg := registry.New()
	reg.Register(tool)
	ql := New(p, reg, Config{MaxTurns: 2, MaxTokens: 1024})
	var toolEvents int
	if err := ql.Run(context.Background(), "hi", func(event stream.Event) {
		if event.Type == stream.EventToolUse {
			toolEvents++
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if tool.calls != 0 || toolEvents != 0 {
		t.Fatalf("truncated tool execution calls/events = %d/%d, want 0/0", tool.calls, toolEvents)
	}
	messages := ql.Messages()
	if len(messages) != 2 || messages[1].GetText() != "safe partial" || len(messages[1].GetToolUses()) != 0 || len(messages[1].GetInvalidToolUses()) != 0 {
		t.Fatalf("quarantined message = %#v", messages)
	}
	if ql.lastResponseID != "" || !ql.disableResponseChain {
		t.Fatalf("truncated native response remained chainable: response_id=%q disabled=%v", ql.lastResponseID, ql.disableResponseChain)
	}
}

func TestTokenBudgetContinuationInjectsNudgeAfterStopHooks(t *testing.T) {
	p := newParityFakeProvider([]parityProviderTurn{
		{Events: endTurnTextEvents("part", 400)},
		{Events: endTurnTextEvents("done", 600)},
	})
	ql := New(p, registry.New(), Config{MaxTurns: 5, MaxTokens: 1024, TokenBudget: 1000})

	if err := ql.Run(context.Background(), "hi", func(stream.Event) {}); err != nil {
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
