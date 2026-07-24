package loop

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type providerGoalEvaluatorFake struct {
	mu       sync.Mutex
	params   []provider.Params
	streamFn func(context.Context, provider.Params) (<-chan types.StreamEvent, error)
	started  chan struct{}
}

func (p *providerGoalEvaluatorFake) Name() string    { return "fake" }
func (p *providerGoalEvaluatorFake) ModelID() string { return "fake-fast-model" }

func (p *providerGoalEvaluatorFake) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	p.params = append(p.params, params)
	p.mu.Unlock()
	if p.started != nil {
		select {
		case <-p.started:
		default:
			close(p.started)
		}
	}
	if p.streamFn != nil {
		return p.streamFn(ctx, params)
	}
	return goalEvaluatorStream(`{"criteria":[{"id":"AC-1","met":true,"reason":"repository inspection is complete"}],"reason":"all requested checks passed"}`, nil, types.StopReasonEndTurn), nil
}

func (p *providerGoalEvaluatorFake) lastParams(t *testing.T) provider.Params {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.params) == 0 {
		t.Fatal("provider was not called")
	}
	return p.params[len(p.params)-1]
}

func TestProviderGoalEvaluatorBuildsTranscriptOnlyRequest(t *testing.T) {
	p := &providerGoalEvaluatorFake{}
	evaluator := NewProviderGoalEvaluator(p)
	messages := []types.Message{
		types.UserMessage("inspect the repository"),
		{
			ID:   "assistant-1",
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "I will inspect it"},
				types.ThinkingBlock{Type: types.ContentTypeThinking, Thinking: "private reasoning must not be projected"},
				types.ToolUseBlock{
					Type:  types.ContentTypeToolUse,
					ID:    "tool-1",
					Name:  "Read",
					Input: map[string]any{"file_path": "README.md"},
				},
			},
		},
		types.ToolResultMessage(types.ToolResultBlock{
			ToolUseID: "tool-1",
			Content:   "README contains the expected documentation",
			Outcome:   types.ToolOutcomeSucceeded,
		}),
	}

	result, err := evaluator.Evaluate(context.Background(), newGoalEvaluationRequest(goal.Goal{
		Objective: "finish the repository inspection", Status: goal.StatusActive, Revision: 1,
		AcceptanceCriteria: []goal.AcceptanceCriterion{{ID: "AC-1", Text: "repository inspection is complete"}},
	}, messages))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result.Met || result.Reason != "all requested checks passed" || len(result.Criteria) != 1 || result.Criteria[0].ID != "AC-1" {
		t.Fatalf("result = %+v", result)
	}

	params := p.lastParams(t)
	if params.Model != "fake-fast-model" {
		t.Fatalf("Model = %q, want provider model", params.Model)
	}
	if params.MaxTokens != 1024 {
		t.Fatalf("MaxTokens = %d, want 1024", params.MaxTokens)
	}
	if len(params.Tools) != 0 || len(params.ExtraToolSchemas) != 0 || params.ToolChoice != nil {
		t.Fatalf("evaluator request exposed tools: tools=%d server_tools=%d choice=%+v", len(params.Tools), len(params.ExtraToolSchemas), params.ToolChoice)
	}
	if params.Thinking == nil || params.Thinking.Enabled {
		t.Fatalf("Thinking = %+v, want explicitly disabled", params.Thinking)
	}
	if len(params.Messages) != 1 || params.Messages[0].Role != types.RoleUser {
		t.Fatalf("Messages = %#v, want one projected user message", params.Messages)
	}
	payload := params.Messages[0].GetText()
	for _, want := range []string{
		"finish the repository inspection",
		"repository inspection is complete",
		"AC-1",
		"inspect the repository",
		"tool-1",
		"Read",
		"README.md",
		"README contains the expected documentation",
		string(types.ToolOutcomeSucceeded),
	} {
		if !strings.Contains(payload, want) {
			t.Errorf("projected evaluator payload omitted %q: %s", want, payload)
		}
	}
	if strings.Contains(payload, "private reasoning must not be projected") {
		t.Fatalf("projected evaluator payload leaked thinking: %s", payload)
	}
}

func TestProviderGoalEvaluatorUsesConfiguredConversationModel(t *testing.T) {
	p := &providerGoalEvaluatorFake{}
	evaluator := NewProviderGoalEvaluatorWithModel(p, "conversation-model")

	if _, err := evaluator.Evaluate(context.Background(), GoalEvaluationRequest{Objective: "finish"}); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := p.lastParams(t).Model; got != "conversation-model" {
		t.Fatalf("Model = %q, want configured conversation model", got)
	}
}

func TestProviderGoalEvaluatorParsesStrictJSONAndReportsUsage(t *testing.T) {
	usageStart := &types.Usage{InputTokens: 41, CacheReadInputTokens: 9}
	usageEnd := &types.Usage{OutputTokens: 7}
	p := &providerGoalEvaluatorFake{streamFn: func(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
		return goalEvaluatorStreamChunks([]string{
			`{"criteria":[{"id":"AC-1","met":false,`,
			`"reason":"verification is still missing"}],"reason":"verification is still missing"}`,
		}, usageStart, usageEnd, types.StopReasonEndTurn), nil
	}}

	result, err := NewProviderGoalEvaluator(p).Evaluate(context.Background(), GoalEvaluationRequest{
		Objective: "complete verification",
		Messages:  []types.Message{types.AssistantMessage("implementation complete")},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Met || result.Reason != "verification is still missing" || len(result.Criteria) != 1 || result.Criteria[0].Met {
		t.Fatalf("result = %+v", result)
	}
	if result.Usage == nil || result.Usage.InputTokens != 41 || result.Usage.OutputTokens != 7 || result.Usage.CacheReadInputTokens != 9 {
		t.Fatalf("Usage = %+v", result.Usage)
	}
}

func TestProviderGoalEvaluatorFailsClosedOnInvalidResponses(t *testing.T) {
	apiErr := &types.APIError{Type: "provider_error", Message: "stream failed"}
	tests := []struct {
		name   string
		stream <-chan types.StreamEvent
	}{
		{name: "empty", stream: goalEvaluatorStream("", nil, types.StopReasonEndTurn)},
		{name: "non JSON", stream: goalEvaluatorStream("yes", nil, types.StopReasonEndTurn)},
		{name: "malformed JSON", stream: goalEvaluatorStream(`{"met":true`, nil, types.StopReasonEndTurn)},
		{name: "missing met", stream: goalEvaluatorStream(`{"reason":"done"}`, nil, types.StopReasonEndTurn)},
		{name: "criterion missing met", stream: goalEvaluatorStream(`{"criteria":[{"id":"AC-1","reason":"done"}],"reason":"done"}`, nil, types.StopReasonEndTurn)},
		{name: "criterion missing id", stream: goalEvaluatorStream(`{"criteria":[{"met":true,"reason":"done"}],"reason":"done"}`, nil, types.StopReasonEndTurn)},
		{name: "missing reason", stream: goalEvaluatorStream(`{"met":true}`, nil, types.StopReasonEndTurn)},
		{name: "max tokens", stream: goalEvaluatorStream(`{"met":true,"reason":"done"}`, nil, types.StopReasonMaxTokens)},
		{name: "event error", stream: streamEvents(types.StreamEvent{Type: types.EventError, Error: apiErr})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &providerGoalEvaluatorFake{streamFn: func(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
				return tt.stream, nil
			}}
			result, err := NewProviderGoalEvaluator(p).Evaluate(context.Background(), GoalEvaluationRequest{
				Objective: "finish safely",
				Messages:  []types.Message{types.AssistantMessage("done")},
			})
			if err == nil {
				t.Fatalf("Evaluate returned success: %+v", result)
			}
			if result.Met {
				t.Fatalf("invalid response marked goal met: %+v", result)
			}
		})
	}
}

func TestProviderGoalEvaluatorReturnsUsageWithMalformedResponse(t *testing.T) {
	p := &providerGoalEvaluatorFake{streamFn: func(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
		return goalEvaluatorStream("not-json", &types.Usage{InputTokens: 12, OutputTokens: 2}, types.StopReasonEndTurn), nil
	}}

	result, err := NewProviderGoalEvaluator(p).Evaluate(context.Background(), GoalEvaluationRequest{Objective: "finish"})
	if err == nil {
		t.Fatal("Evaluate returned nil error")
	}
	if result.Usage == nil || result.Usage.InputTokens != 12 || result.Usage.OutputTokens != 2 {
		t.Fatalf("Usage = %+v, want usage retained on parse error", result.Usage)
	}
}

func TestProviderGoalEvaluatorCancellationDoesNotWaitForStreamClose(t *testing.T) {
	started := make(chan struct{})
	p := &providerGoalEvaluatorFake{
		started: started,
		streamFn: func(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
			return make(chan types.StreamEvent), nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewProviderGoalEvaluator(p).Evaluate(ctx, GoalEvaluationRequest{Objective: "finish"})
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider call did not start")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Evaluate blocked after context cancellation")
	}
}

func goalEvaluatorStream(text string, usage *types.Usage, stopReason types.StopReason) <-chan types.StreamEvent {
	return goalEvaluatorStreamChunks([]string{text}, usage, usage, stopReason)
}

func goalEvaluatorStreamChunks(chunks []string, startUsage, endUsage *types.Usage, stopReason types.StopReason) <-chan types.StreamEvent {
	events := []types.StreamEvent{{
		Type:    types.EventMessageStart,
		Message: &types.APIMessage{Role: types.RoleAssistant},
		Usage:   startUsage,
	}}
	for _, chunk := range chunks {
		if chunk == "" {
			continue
		}
		events = append(events, types.StreamEvent{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: chunk},
		})
	}
	events = append(events,
		types.StreamEvent{Type: types.EventMessageDelta, StopReason: &stopReason, Usage: endUsage},
		types.StreamEvent{Type: types.EventMessageStop},
	)
	return streamEvents(events...)
}

func streamEvents(events ...types.StreamEvent) <-chan types.StreamEvent {
	stream := make(chan types.StreamEvent, len(events))
	for _, event := range events {
		stream <- event
	}
	close(stream)
	return stream
}
