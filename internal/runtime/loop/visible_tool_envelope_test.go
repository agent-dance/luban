package loop

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	streamevent "github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type visibleEnvelopeTool struct {
	name    string
	version string
}

func (t *visibleEnvelopeTool) Name() string        { return t.name }
func (t *visibleEnvelopeTool) Description() string { return t.name + " " + t.version }
func (t *visibleEnvelopeTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"value": map[string]any{"type": "string", "description": t.version},
	}, "value")
}
func (t *visibleEnvelopeTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, nil
}

type visibleEnvelopeProvider struct {
	calls []provider.Params
}

type outsideVisibleCatalogProvider struct{}

func (*outsideVisibleCatalogProvider) Name() string    { return "openai" }
func (*outsideVisibleCatalogProvider) ModelID() string { return "gpt-visible-guard" }
func (*outsideVisibleCatalogProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	return makeStreamChan(aggregateToolUseEvents(types.ToolUseBlock{
		Type: types.ContentTypeToolUse, ID: "hidden-call", Name: "Bash", Input: map[string]any{},
	})...), nil
}

type outsideVisibleCatalogTool struct {
	executions atomic.Int32
}

func (*outsideVisibleCatalogTool) Name() string        { return "Bash" }
func (*outsideVisibleCatalogTool) Description() string { return "private dependency" }
func (*outsideVisibleCatalogTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}
func (t *outsideVisibleCatalogTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	t.executions.Add(1)
	return types.ToolResult{Content: "must not execute"}, nil
}

func (*visibleEnvelopeProvider) Name() string    { return "openai" }
func (*visibleEnvelopeProvider) ModelID() string { return "gpt-visible" }
func (p *visibleEnvelopeProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.calls = append(p.calls, params)
	responseID := "resp-" + strconv.Itoa(len(p.calls))
	events := []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "ok"}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, Usage: &types.Usage{InputTokens: 10, OutputTokens: 1}, StopReason: stopReasonPtr(types.StopReasonEndTurn)},
		{Type: types.EventMessageStop, ResponseID: responseID},
	}
	stream := make(chan types.StreamEvent, len(events))
	for _, event := range events {
		stream <- event
	}
	close(stream)
	return stream, nil
}

func TestVisibleToolSnapshotBindsPromptProviderAndCacheGeneration(t *testing.T) {
	reg := registry.New()
	reg.SetModelToolProfile(registry.ModelToolProfileAgenticV2)
	for _, name := range []string{"TaskCreate", "Agent", "Run", "Inspect", "ToolSearch", "ApplyPatch"} {
		reg.Register(&visibleEnvelopeTool{name: name, version: "v1"})
	}
	visible, err := reg.SnapshotVisibleTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	promptConfig := prompt.Config{CWD: "/repo"}
	blocks := prompt.BuildSystemPromptBlocksForDefinitions(visible.Definitions(), promptConfig)
	model := &visibleEnvelopeProvider{}
	query := New(model, reg, Config{
		MaxTurns: 1, Model: model.ModelID(), SessionID: "session-visible", CacheLineageID: "lineage-visible",
		System: blocks.JoinedText(), SystemBlocks: blocks,
		VisibleTools: visible, ToolPromptConfig: promptConfig, GeneratedToolPrompt: true,
	})

	if err := query.Run(context.Background(), "first", func(streamevent.Event) {}); err != nil {
		t.Fatal(err)
	}
	reg.Register(&visibleEnvelopeTool{name: "WebFetch", version: "v1"})
	if err := query.Run(context.Background(), "second", func(streamevent.Event) {}); err != nil {
		t.Fatal(err)
	}
	reg.Register(&visibleEnvelopeTool{name: "Run", version: "v2"})
	if err := query.Run(context.Background(), "third", func(streamevent.Event) {}); err != nil {
		t.Fatal(err)
	}

	if len(model.calls) != 3 {
		t.Fatalf("provider calls = %d, want 3", len(model.calls))
	}
	wantNames := []string{"Inspect", "ApplyPatch", "Run"}
	for index, call := range model.calls {
		names := make([]string, len(call.Tools))
		for i := range call.Tools {
			names[i] = call.Tools[i].Name
		}
		if !reflect.DeepEqual(names, wantNames) {
			t.Fatalf("call %d tools = %v, want %v", index, names, wantNames)
		}
		joined := call.JoinedSystemPrompt()
		if !strings.Contains(joined, "The complete visible catalog is Inspect, ApplyPatch, and Run") {
			t.Fatalf("call %d lost V2 guidance", index)
		}
		if strings.Contains(joined, "use the Agent tool") || strings.Contains(joined, "TaskCreate tool") {
			t.Fatalf("call %d prompt advertises a hidden tool", index)
		}
	}
	if model.calls[0].PromptCacheKey == "lineage-visible" || model.calls[0].PromptCacheKey == "" {
		t.Fatalf("catalog generation was not bound into cache affinity: %q", model.calls[0].PromptCacheKey)
	}
	if model.calls[0].PromptCacheKey != model.calls[1].PromptCacheKey {
		t.Fatal("unrelated hidden registration broke the V2 cache generation")
	}
	if model.calls[1].PreviousResponseID != "resp-1" {
		t.Fatalf("stable envelope previous_response_id = %q, want resp-1", model.calls[1].PreviousResponseID)
	}
	if model.calls[1].PromptCacheKey == model.calls[2].PromptCacheKey {
		t.Fatal("visible schema change reused the previous cache generation")
	}
	if model.calls[2].PreviousResponseID != "" {
		t.Fatalf("visible schema change reused response chain %q", model.calls[2].PreviousResponseID)
	}
	if model.calls[1].Tools[2].Description == model.calls[2].Tools[2].Description {
		t.Fatal("provider request did not receive the replacement Run schema")
	}
}

func TestProviderCannotExecuteRegisteredToolOutsideTurnCatalog(t *testing.T) {
	reg := registry.New()
	reg.SetModelToolProfile(registry.ModelToolProfileAgenticV2)
	for _, name := range []string{"Inspect", "ApplyPatch", "Run"} {
		reg.Register(&visibleEnvelopeTool{name: name, version: "v1"})
	}
	hidden := &outsideVisibleCatalogTool{}
	reg.Register(hidden)
	model := &outsideVisibleCatalogProvider{}
	query := New(model, reg, Config{
		MaxTurns: 2, Model: model.ModelID(), SessionID: "session-visible-guard",
		ProjectRoot: t.TempDir(),
	})
	var events []streamevent.Event
	err := query.Run(context.Background(), "try hidden tool", func(event streamevent.Event) {
		events = append(events, event)
	})
	var catalogErr *ToolUseCatalogError
	if !errors.As(err, &catalogErr) {
		t.Fatalf("Run() error = %v, want ToolUseCatalogError", err)
	}
	if catalogErr.ToolName != "Bash" || catalogErr.ToolUseID != "hidden-call" {
		t.Fatalf("catalog error = %#v", catalogErr)
	}
	if hidden.executions.Load() != 0 {
		t.Fatalf("private tool executions = %d, want 0", hidden.executions.Load())
	}
	foundProtocolError := false
	for _, event := range events {
		if event.Type == streamevent.EventToolUse {
			t.Fatal("outside-catalog call was emitted as an accepted tool intent")
		}
		if event.Type == streamevent.EventError && event.TerminalReason == "tool_outside_visible_catalog" {
			foundProtocolError = true
		}
	}
	if !foundProtocolError {
		t.Fatal("outside-catalog provider violation did not emit a terminal protocol error")
	}
}
