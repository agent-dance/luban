package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

const (
	parallelAgentParentPrompt = "delegate both independent analyses"
	parallelAgentFirstPrompt  = "analyze the first sector"
	parallelAgentSecondPrompt = "analyze the second sector"
)

// parallelAgentE2EProvider drives both the parent loop and the real child
// loops. Child streams are released only after both have started, making
// overlap a deterministic requirement instead of a timing assertion.
type parallelAgentE2EProvider struct {
	mu sync.Mutex

	childArrivals   int
	childActive     int
	maxChildActive  int
	releaseChildren chan struct{}

	parentResults []types.ToolResultBlock
}

func newParallelAgentE2EProvider() *parallelAgentE2EProvider {
	return &parallelAgentE2EProvider{releaseChildren: make(chan struct{})}
}

func (p *parallelAgentE2EProvider) Name() string    { return "parallel-agent-e2e" }
func (p *parallelAgentE2EProvider) ModelID() string { return "parallel-agent-e2e-model" }

func (p *parallelAgentE2EProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	if results := parallelAgentToolResults(params.Messages); len(results) > 0 {
		p.mu.Lock()
		p.parentResults = append([]types.ToolResultBlock(nil), results...)
		p.mu.Unlock()
		return parallelAgentEventStream(ctx, parallelAgentTextEvents("synthesis from both agents")), nil
	}

	switch parallelAgentLastText(params.Messages) {
	case parallelAgentParentPrompt:
		return parallelAgentEventStream(ctx, parallelAgentToolUseEvents(
			types.ToolUseBlock{
				Type: types.ContentTypeToolUse,
				ID:   "agent_first",
				Name: "Agent",
				Input: map[string]any{
					"description": "analyze first sector",
					"prompt":      parallelAgentFirstPrompt,
				},
			},
			types.ToolUseBlock{
				Type: types.ContentTypeToolUse,
				ID:   "agent_second",
				Name: "Agent",
				Input: map[string]any{
					"description": "analyze second sector",
					"prompt":      parallelAgentSecondPrompt,
				},
			},
		)), nil
	case parallelAgentFirstPrompt:
		return p.childStream(ctx, "first agent result"), nil
	case parallelAgentSecondPrompt:
		return p.childStream(ctx, "second agent result"), nil
	default:
		return nil, fmt.Errorf("unexpected provider messages: %#v", params.Messages)
	}
}

func (p *parallelAgentE2EProvider) childStream(ctx context.Context, result string) <-chan types.StreamEvent {
	stream := make(chan types.StreamEvent, 4)
	go func() {
		defer close(stream)

		p.mu.Lock()
		p.childArrivals++
		p.childActive++
		if p.childActive > p.maxChildActive {
			p.maxChildActive = p.childActive
		}
		if p.childArrivals == 2 {
			close(p.releaseChildren)
		}
		p.mu.Unlock()

		defer func() {
			p.mu.Lock()
			p.childActive--
			p.mu.Unlock()
		}()

		select {
		case <-ctx.Done():
			return
		case <-p.releaseChildren:
		}

		for _, event := range parallelAgentTextEvents(result) {
			select {
			case <-ctx.Done():
				return
			case stream <- event:
			}
		}
	}()
	return stream
}

func (p *parallelAgentE2EProvider) snapshot() (maxChildActive int, parentResults []types.ToolResultBlock) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxChildActive, append([]types.ToolResultBlock(nil), p.parentResults...)
}

func parallelAgentLastText(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if text := messages[i].GetText(); text != "" {
			return text
		}
	}
	return ""
}

func parallelAgentToolResults(messages []types.Message) []types.ToolResultBlock {
	for i := len(messages) - 1; i >= 0; i-- {
		var results []types.ToolResultBlock
		for _, block := range messages[i].Content {
			if result, ok := block.(types.ToolResultBlock); ok {
				results = append(results, result)
			}
		}
		if len(results) > 0 {
			return results
		}
	}
	return nil
}

func parallelAgentEventStream(ctx context.Context, events []types.StreamEvent) <-chan types.StreamEvent {
	stream := make(chan types.StreamEvent, len(events))
	go func() {
		defer close(stream)
		for _, event := range events {
			select {
			case <-ctx.Done():
				return
			case stream <- event:
			}
		}
	}()
	return stream
}

func parallelAgentTextEvents(text string) []types.StreamEvent {
	stopReason := types.StopReasonEndTurn
	return []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stopReason},
		{Type: types.EventMessageStop},
	}
}

func parallelAgentToolUseEvents(toolUses ...types.ToolUseBlock) []types.StreamEvent {
	events := make([]types.StreamEvent, 0, len(toolUses)*3+2)
	for i, toolUse := range toolUses {
		input, _ := json.Marshal(toolUse.Input)
		events = append(events,
			types.StreamEvent{
				Type:  types.EventContentBlockStart,
				Index: i,
				ContentBlock: &types.ContentDelta{
					Type: types.ContentTypeToolUse,
					ID:   toolUse.ID,
					Name: toolUse.Name,
				},
			},
			types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: i,
				Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(input)},
			},
			types.StreamEvent{Type: types.EventContentBlockStop, Index: i},
		)
	}
	stopReason := types.StopReasonToolUse
	return append(events,
		types.StreamEvent{Type: types.EventMessageDelta, StopReason: &stopReason},
		types.StreamEvent{Type: types.EventMessageStop},
	)
}

func TestAgentToolExecutesParallelForegroundCallsEndToEnd(t *testing.T) {
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "")
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "1")

	provider := newParallelAgentE2EProvider()
	reg := registry.New()
	reg.Register(&AgentTool{
		Provider: provider,
		Registry: reg,
		Model:    provider.ModelID(),
	})

	query := loop.New(provider, reg, loop.Config{
		MaxTurns:  5,
		MaxTokens: 1024,
		Model:     provider.ModelID(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var finalText string
	if err := query.Run(ctx, parallelAgentParentPrompt, func(event stream.Event) {
		if event.Type == stream.EventText {
			finalText += event.Text
		}
	}); err != nil {
		t.Fatalf("parent query: %v", err)
	}

	maxActive, results := provider.snapshot()
	if maxActive != 2 {
		t.Fatalf("maximum overlapping child runs = %d, want 2", maxActive)
	}
	if len(results) != 2 {
		t.Fatalf("parent received %d Agent results, want 2", len(results))
	}
	if results[0].ToolUseID != "agent_first" || results[1].ToolUseID != "agent_second" {
		t.Fatalf("parent result order = [%q, %q], want [agent_first, agent_second]", results[0].ToolUseID, results[1].ToolUseID)
	}
	if got := results[0].TextContent(); !strings.Contains(got, "first agent result") {
		t.Fatalf("first Agent result = %q, want child output", got)
	}
	if got := results[1].TextContent(); !strings.Contains(got, "second agent result") {
		t.Fatalf("second Agent result = %q, want child output", got)
	}
	if finalText != "synthesis from both agents" {
		t.Fatalf("final parent synthesis = %q", finalText)
	}
}
