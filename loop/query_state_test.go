package loop

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestQueryStateNextTurnAndMaxTurnAccounting(t *testing.T) {
	state := newQueryState([]types.Message{types.UserMessage("hello")})

	if state.TurnCount != 0 {
		t.Fatalf("initial TurnCount = %d, want 0", state.TurnCount)
	}
	if state.Transition != QueryTransitionNextTurn {
		t.Fatalf("initial Transition = %q, want %q", state.Transition, QueryTransitionNextTurn)
	}
	if !state.shouldContinue(2) {
		t.Fatal("state should continue before any turns")
	}

	if got := state.beginNextTurn(); got != 1 {
		t.Fatalf("first turn = %d, want 1", got)
	}
	if state.Transition != QueryTransitionNextTurn {
		t.Fatalf("first transition = %q, want %q", state.Transition, QueryTransitionNextTurn)
	}
	if !state.shouldContinue(2) {
		t.Fatal("state should continue after first of two turns")
	}

	if got := state.beginNextTurn(); got != 2 {
		t.Fatalf("second turn = %d, want 2", got)
	}
	if state.shouldContinue(2) {
		t.Fatal("state should stop after reaching max turns")
	}

	err := state.maxTurnsExceeded(2)
	if err.MaxTurns != 2 || err.TurnCount != 3 {
		t.Fatalf("max turns error = %+v, want MaxTurns=2 TurnCount=3", err)
	}
}

type queryConfigMutatingProvider struct {
	loop      *QueryLoop
	calls     []provider.Params
	responses [][]types.StreamEvent
}

func (p *queryConfigMutatingProvider) Name() string    { return "mutating" }
func (p *queryConfigMutatingProvider) ModelID() string { return "mutating-model" }

func (p *queryConfigMutatingProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.calls = append(p.calls, params)
	if len(p.calls) == 1 {
		p.loop.config.Model = "mutated-model"
		p.loop.config.System = "mutated-system"
		p.loop.config.MaxTokens = 999
		p.loop.config.MaxTurns = 1
		p.loop.config.SessionID = "mutated-session"
		p.loop.config.ReasoningEffort = "high"
		p.loop.thinkingConfig = &provider.ThinkingConfig{Enabled: true, BudgetTokens: 999}
	}

	idx := len(p.calls) - 1
	ch := make(chan types.StreamEvent, 16)
	if idx < len(p.responses) {
		for _, event := range p.responses[idx] {
			ch <- event
		}
	}
	close(ch)
	return ch, nil
}

func TestQueryStateConfigSnapshotFrozenWithinRun(t *testing.T) {
	toolInput, err := json.Marshal(map[string]any{"text": "snapshot"})
	if err != nil {
		t.Fatalf("Marshal tool input: %v", err)
	}

	toolTurn := []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse,
			ID:   "call_snapshot",
			Name: "Echo",
		}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type:        "input_json_delta",
			PartialJSON: string(toolInput),
		}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}

	prov := &queryConfigMutatingProvider{
		responses: [][]types.StreamEvent{
			toolTurn,
			textEvents("done"),
		},
	}
	reg := registry.New()
	reg.Register(&mockEchoTool{})

	q := New(prov, reg, Config{
		MaxTurns:        2,
		Model:           "original-model",
		System:          "original-system",
		MaxTokens:       111,
		SessionID:       "original-session",
		ReasoningEffort: "low",
	})
	q.SetThinkingConfig(true, 1234)
	prov.loop = q

	if err := q.Run(context.Background(), "start", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(prov.calls))
	}

	second := prov.calls[1]
	if second.Model != "original-model" {
		t.Fatalf("second Model = %q, want original-model", second.Model)
	}
	if second.System != "original-system" {
		t.Fatalf("second System = %q, want original-system", second.System)
	}
	if second.MaxTokens != 111 {
		t.Fatalf("second MaxTokens = %d, want 111", second.MaxTokens)
	}
	if second.PromptCacheKey != "original-session" || !second.UsePromptCache {
		t.Fatalf("second prompt cache = key %q enabled %v, want original-session true", second.PromptCacheKey, second.UsePromptCache)
	}
	if second.ReasoningEffort != "low" {
		t.Fatalf("second ReasoningEffort = %q, want low", second.ReasoningEffort)
	}
	if second.Thinking == nil || !second.Thinking.Enabled || second.Thinking.BudgetTokens != 1234 {
		t.Fatalf("second Thinking = %+v, want enabled budget 1234", second.Thinking)
	}
}
