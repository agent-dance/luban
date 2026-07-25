package loop

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type aggregateBudgetProvider struct {
	requests []provider.Params
	events   [][]types.StreamEvent
}

func (p *aggregateBudgetProvider) Name() string    { return "aggregate-budget" }
func (p *aggregateBudgetProvider) ModelID() string { return "aggregate-budget-model" }

func (p *aggregateBudgetProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.requests = append(p.requests, params)
	idx := len(p.requests) - 1
	events := parityTextEvents("done")
	if idx < len(p.events) {
		events = p.events[idx]
	}
	ch := make(chan types.StreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

type aggregateBudgetTool struct {
	name    string
	content string
}

func (t aggregateBudgetTool) Name() string        { return t.name }
func (t aggregateBudgetTool) Description() string { return "large aggregate budget fixture" }
func (t aggregateBudgetTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t aggregateBudgetTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: t.content}, nil
}
func (t aggregateBudgetTool) IsConcurrentSafe() bool { return true }

func TestQueryLoopAppliesAggregateToolResultBudgetBeforeNextRequest(t *testing.T) {
	prov := &aggregateBudgetProvider{events: [][]types.StreamEvent{
		aggregateToolUseEvents(aggregateToolUses(15)...),
		parityTextEvents("finished"),
	}}
	reg := registry.New()
	for _, use := range aggregateToolUses(15) {
		reg.Register(aggregateBudgetTool{name: use.Name, content: strings.Repeat(use.Name[:1], 45_000)})
	}
	ql := New(prov, reg, Config{MaxTurns: 2})
	ql.SetResultStore(compact.NewResultStore(t.TempDir()))

	if err := ql.Run(context.Background(), "run tools", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.requests) < 2 {
		t.Fatalf("provider requests = %d, want at least 2", len(prov.requests))
	}
	replaced := 0
	var seen []string
	for _, use := range aggregateToolUses(15) {
		id := use.ID
		content := findToolResultContent(prov.requests[1].Messages, id)
		seen = append(seen, id+":"+fmt.Sprint(len(content))+":"+fmt.Sprint(strings.Contains(content, "<persisted-output>")))
		if strings.Contains(content, "<persisted-output>") {
			replaced++
		}
	}
	if replaced == 0 {
		t.Fatalf("replaced results in second request = %d, want at least 1 (seen %v)", replaced, seen)
	}
	if replacements := compact.ReconstructContentReplacementStateForScope(ql.Messages(), ql.internalControlScope).Replacements; len(replacements) != replaced {
		t.Fatalf("persisted replacements = %d, want %d", len(replacements), replaced)
	}
}

func TestQueryLoopRepeatedRunsReapplyStableReplacement(t *testing.T) {
	prov := &aggregateBudgetProvider{}
	reg := registry.New()
	ql := New(prov, reg, Config{MaxTurns: 1})
	ql.SetResultStore(compact.NewResultStore(t.TempDir()))
	uses := aggregateToolUses(15)
	blocks := make([]types.ContentBlock, len(uses))
	results := make([]types.ToolResultBlock, len(uses))
	for i, use := range uses {
		blocks[i] = use
		results[i] = types.ToolResultBlock{ToolUseID: use.ID, Content: strings.Repeat(use.Name[:1], 45_000)}
	}
	ql.messages = []types.Message{
		{
			Role:    types.RoleAssistant,
			Content: blocks,
		},
		types.ToolResultMessage(results...),
	}

	if err := ql.Run(context.Background(), "first", func(stream.Event) {}); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	first := findToolResultContent(prov.requests[0].Messages, "toolu_00")
	if !strings.Contains(first, "<persisted-output>") {
		t.Fatalf("first request did not replace: %.80q", first)
	}
	if err := ql.Run(context.Background(), "second", func(stream.Event) {}); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	second := findToolResultContent(prov.requests[1].Messages, "toolu_00")
	if second != first {
		t.Fatalf("replacement changed across repeated runs\nfirst: %.80q\nsecond: %.80q", first, second)
	}
}

func TestQueryLoopResumeReconstructsAndReappliesReplacement(t *testing.T) {
	replacement := "<persisted-output>\nexact resume replacement\n</persisted-output>"
	prov := &aggregateBudgetProvider{}
	ql := New(prov, registry.New(), Config{MaxTurns: 1})
	messages := []types.Message{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{types.ToolUseBlock{
				Type: types.ContentTypeToolUse, ID: "toolu_resume", Name: "Tool", Input: map[string]any{},
			}},
		},
		compact.AppendContentReplacementRecordsForScope(
			[]types.Message{types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "toolu_resume", Content: strings.Repeat("r", 250_000)})},
			[]compact.ContentReplacementRecord{{Kind: "tool-result", ToolUseID: "toolu_resume", Replacement: replacement}},
			messagecontrol.Runtime(), ql.internalControlScope,
		)[0],
	}
	ql.SetMessages(messages)

	if err := ql.Run(context.Background(), "after resume", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := findToolResultContent(prov.requests[0].Messages, "toolu_resume")
	if got != replacement {
		t.Fatalf("resume replacement = %q, want exact stored replacement", got)
	}
	for _, msg := range prov.requests[0].Messages {
		for _, block := range msg.Content {
			if _, ok := block.(types.ContentReplacementBlock); ok {
				t.Fatalf("provider request leaked content replacement block")
			}
		}
	}
}

func aggregateToolUseEvents(uses ...types.ToolUseBlock) []types.StreamEvent {
	events := []types.StreamEvent{{Type: types.EventMessageStart}}
	for i, use := range uses {
		events = append(events,
			types.StreamEvent{Type: types.EventContentBlockStart, Index: i, ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse,
				ID:   use.ID,
				Name: use.Name,
			}},
			types.StreamEvent{Type: types.EventContentBlockDelta, Index: i, Delta: &types.ContentDelta{
				Type:        "input_json_delta",
				PartialJSON: `{}`,
			}},
			types.StreamEvent{Type: types.EventContentBlockStop, Index: i},
		)
	}
	events = append(events,
		types.StreamEvent{Type: types.EventMessageDelta, StopReason: stopReasonForParity(types.StopReasonToolUse)},
		types.StreamEvent{Type: types.EventMessageStop},
	)
	return events
}

func aggregateToolUses(n int) []types.ToolUseBlock {
	uses := make([]types.ToolUseBlock, n)
	for i := range uses {
		name := fmt.Sprintf("Tool%02d", i)
		uses[i] = types.ToolUseBlock{
			Type:  types.ContentTypeToolUse,
			ID:    fmt.Sprintf("toolu_%02d", i),
			Name:  name,
			Input: map[string]any{},
		}
	}
	return uses
}

func findToolResultContent(messages []types.Message, id string) string {
	for _, msg := range messages {
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok && tr.ToolUseID == id {
				return tr.TextContent()
			}
		}
	}
	return ""
}
