package loop

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// mockProvider simulates an LLM for integration testing
type mockProvider struct {
	mu        sync.Mutex
	responses [][]types.StreamEvent // one per turn
	turnIndex int
}

func (m *mockProvider) Name() string    { return "mock" }
func (m *mockProvider) ModelID() string { return "mock-model" }

func (m *mockProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	m.mu.Lock()
	idx := m.turnIndex
	m.turnIndex++
	m.mu.Unlock()

	ch := make(chan types.StreamEvent, 64)
	go func() {
		defer close(ch)
		if idx < len(m.responses) {
			for _, ev := range m.responses[idx] {
				if ctx.Err() != nil {
					return
				}
				ch <- ev
			}
		}
	}()
	return ch, nil
}

// mockEchoTool returns whatever "text" input it receives
type mockEchoTool struct{}

func (t *mockEchoTool) Name() string        { return "Echo" }
func (t *mockEchoTool) Description() string { return "echo tool" }
func (t *mockEchoTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *mockEchoTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	text, _ := input["text"].(string)
	return types.ToolResult{Content: "echo: " + text}, nil
}
func (t *mockEchoTool) IsConcurrentSafe() bool { return true }

type mockAttachmentTool struct{}

func (t *mockAttachmentTool) Name() string        { return "Attach" }
func (t *mockAttachmentTool) Description() string { return "returns a supplemental image message" }
func (t *mockAttachmentTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *mockAttachmentTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	return types.ToolResult{
		Content: "attached",
		NewMessages: []types.Message{{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ImageBlock{
					Type: types.ContentTypeImage,
					Source: &types.ImageSource{
						Type:      "base64",
						MediaType: "image/png",
						Data:      "iVBORw0KGgo=",
					},
				},
			},
		}},
	}, nil
}
func (t *mockAttachmentTool) IsConcurrentSafe() bool { return true }

type namedMockTool struct {
	name string
	desc string
}

func (t *namedMockTool) Name() string        { return t.name }
func (t *namedMockTool) Description() string { return t.desc }
func (t *namedMockTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *namedMockTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: "ok"}, nil
}

type toolSearchLoaderMockTool struct{}

func (t *toolSearchLoaderMockTool) Name() string        { return "ToolSearch" }
func (t *toolSearchLoaderMockTool) Description() string { return "tool search" }
func (t *toolSearchLoaderMockTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *toolSearchLoaderMockTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	return types.ToolResult{
		Content: `Loaded 1 tool(s) for "select:TaskCreate": TaskCreate.`,
		ContentBlocks: []types.ContentBlock{
			types.ToolReferenceBlock{
				Type:     types.ContentTypeToolReference,
				ToolName: "TaskCreate",
			},
		},
	}, nil
}

type recordingMockProvider struct {
	mu        sync.Mutex
	responses [][]types.StreamEvent
	turnIndex int
	toolSets  [][]string
}

func (m *recordingMockProvider) Name() string    { return "mock" }
func (m *recordingMockProvider) ModelID() string { return "mock-model" }

func (m *recordingMockProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	m.mu.Lock()
	idx := m.turnIndex
	m.turnIndex++
	names := make([]string, 0, len(params.Tools))
	for _, tool := range params.Tools {
		names = append(names, tool.Name)
	}
	m.toolSets = append(m.toolSets, names)
	m.mu.Unlock()

	ch := make(chan types.StreamEvent, 64)
	go func() {
		defer close(ch)
		if idx < len(m.responses) {
			for _, ev := range m.responses[idx] {
				if ctx.Err() != nil {
					return
				}
				ch <- ev
			}
		}
	}()
	return ch, nil
}

func TestIntegrationMultiTurnConversation(t *testing.T) {
	t.Parallel()

	// Turn 1: LLM responds with text only → loop ends
	p := &mockProvider{
		responses: [][]types.StreamEvent{
			{
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "text_delta", Text: "Hello!"}},
				{Type: types.EventContentBlockStop, Index: 0},
				{Type: types.EventMessageStop},
			},
		},
	}

	reg := registry.New()
	reg.Register(&mockEchoTool{})

	ql := New(p, reg, Config{MaxTurns: 10, MaxTokens: 1024})
	var texts []string
	err := ql.Run(context.Background(), "hi", func(e Event) {
		if e.Type == EventText {
			texts = append(texts, e.Text)
		}
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(texts) != 1 || texts[0] != "Hello!" {
		t.Errorf("expected ['Hello!'], got %v", texts)
	}
	// 2 messages: user + assistant
	if len(ql.Messages()) != 2 {
		t.Errorf("expected 2 messages, got %d", len(ql.Messages()))
	}
}

func TestIntegrationToolUseAndResponse(t *testing.T) {
	t.Parallel()

	toolInput, _ := json.Marshal(map[string]any{"text": "world"})

	p := &mockProvider{
		responses: [][]types.StreamEvent{
			// Turn 1: LLM calls the Echo tool
			{
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{
						Type: types.ContentTypeToolUse,
						ID:   "call_1",
						Name: "Echo",
					}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(toolInput)}},
				{Type: types.EventContentBlockStop, Index: 0},
				{Type: types.EventMessageStop},
			},
			// Turn 2: LLM sees tool result, responds with text
			{
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "text_delta", Text: "Got: echo: world"}},
				{Type: types.EventContentBlockStop, Index: 0},
				{Type: types.EventMessageStop},
			},
		},
	}

	reg := registry.New()
	reg.Register(&mockEchoTool{})

	ql := New(p, reg, Config{MaxTurns: 10, MaxTokens: 1024})

	var toolUsed bool
	var finalText string
	err := ql.Run(context.Background(), "echo world", func(e Event) {
		if e.Type == EventToolUse {
			toolUsed = true
		}
		if e.Type == EventText {
			finalText += e.Text
		}
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !toolUsed {
		t.Error("expected tool to be used")
	}
	if finalText != "Got: echo: world" {
		t.Errorf("expected 'Got: echo: world', got '%s'", finalText)
	}
	// 4 messages: user, assistant(tool_use), user(tool_result), assistant(text)
	if len(ql.Messages()) != 4 {
		t.Errorf("expected 4 messages, got %d", len(ql.Messages()))
	}
}

func TestIntegrationMaxTurnsExceeded(t *testing.T) {
	t.Parallel()

	toolInput, _ := json.Marshal(map[string]any{"text": "loop"})

	// Every turn calls the tool with a fresh provider identity → never finishes
	// and reaches the max-turn guard without violating the session ID contract.
	toolTurn := func(toolUseID string) []types.StreamEvent {
		return []types.StreamEvent{
			{Type: types.EventContentBlockStart, Index: 0,
				ContentBlock: &types.ContentDelta{
					Type: types.ContentTypeToolUse,
					ID:   toolUseID,
					Name: "Echo",
				}},
			{Type: types.EventContentBlockDelta, Index: 0,
				Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(toolInput)}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageStop},
		}
	}

	p := &mockProvider{
		responses: [][]types.StreamEvent{
			toolTurn("call_loop_1"), toolTurn("call_loop_2"), toolTurn("call_loop_3"), toolTurn("call_loop_4"), toolTurn("call_loop_5"),
		},
	}

	reg := registry.New()
	reg.Register(&mockEchoTool{})

	ql := New(p, reg, Config{MaxTurns: 3, MaxTokens: 1024})
	err := ql.Run(context.Background(), "infinite loop", func(e Event) {})

	if err == nil {
		t.Fatal("expected max turns error")
	}
	if err.Error() != "max turns (3) exceeded" {
		t.Errorf("unexpected error: %v", err)
	}
	var maxTurnsErr *MaxTurnsError
	if !errors.As(err, &maxTurnsErr) {
		t.Fatalf("expected typed MaxTurnsError, got %T", err)
	}
	if maxTurnsErr.MaxTurns != 3 || maxTurnsErr.TurnCount != 4 {
		t.Fatalf("unexpected max turns details: %+v", maxTurnsErr)
	}
}

func TestIntegrationDisableMaxTurnsRunsUntilNaturalCompletion(t *testing.T) {
	t.Parallel()

	toolInput, _ := json.Marshal(map[string]any{"text": "loop"})
	toolTurn := func(toolUseID string) []types.StreamEvent {
		return []types.StreamEvent{
			{Type: types.EventContentBlockStart, Index: 0,
				ContentBlock: &types.ContentDelta{
					Type: types.ContentTypeToolUse,
					ID:   toolUseID,
					Name: "Echo",
				}},
			{Type: types.EventContentBlockDelta, Index: 0,
				Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(toolInput)}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageStop},
		}
	}

	p := &mockProvider{
		responses: [][]types.StreamEvent{
			toolTurn("call_loop_1"),
			toolTurn("call_loop_2"),
			textEvents("done after disabled cap"),
		},
	}

	reg := registry.New()
	reg.Register(&mockEchoTool{})

	ql := New(p, reg, Config{MaxTurns: 1, DisableMaxTurns: true, MaxTokens: 1024})
	var finalText string
	err := ql.Run(context.Background(), "loop then finish", func(e Event) {
		if e.Type == EventText {
			finalText += e.Text
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if finalText != "done after disabled cap" {
		t.Fatalf("unexpected final text %q", finalText)
	}
}

func TestIntegrationContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	p := &mockProvider{
		responses: [][]types.StreamEvent{
			{
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "text_delta", Text: "should not see this"}},
				{Type: types.EventContentBlockStop, Index: 0},
				{Type: types.EventMessageStop},
			},
		},
	}

	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 10, MaxTokens: 1024})
	err := ql.Run(ctx, "hello", func(e Event) {})

	if err == nil {
		t.Fatal("expected context cancelled error")
	}
}

func TestIntegrationToolSupplementalMessagesAreAppended(t *testing.T) {
	t.Parallel()

	p := &mockProvider{
		responses: [][]types.StreamEvent{
			{
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{
						Type: types.ContentTypeToolUse,
						ID:   "call_attach",
						Name: "Attach",
					}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{}`}},
				{Type: types.EventContentBlockStop, Index: 0},
				{Type: types.EventMessageStop},
			},
			{
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "text_delta", Text: "done"}},
				{Type: types.EventContentBlockStop, Index: 0},
				{Type: types.EventMessageStop},
			},
		},
	}

	reg := registry.New()
	reg.Register(&mockAttachmentTool{})

	ql := New(p, reg, Config{MaxTurns: 10, MaxTokens: 1024})
	if err := ql.Run(context.Background(), "attach", func(Event) {}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := ql.Messages()
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
	img, ok := msgs[3].Content[0].(types.ImageBlock)
	if !ok {
		t.Fatalf("expected supplemental image block, got %#v", msgs[3].Content[0])
	}
	if img.Source == nil || img.Source.Data == "" {
		t.Fatalf("unexpected image source: %#v", img.Source)
	}
}

func TestIntegrationToolSearchLoadsDeferredToolsOnNextTurn(t *testing.T) {
	t.Parallel()

	toolInput, _ := json.Marshal(map[string]any{"query": "select:TaskCreate"})

	p := &recordingMockProvider{
		responses: [][]types.StreamEvent{
			{
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{
						Type: types.ContentTypeToolUse,
						ID:   "tool_search_1",
						Name: "ToolSearch",
					}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(toolInput)}},
				{Type: types.EventContentBlockStop, Index: 0},
				{Type: types.EventMessageStop},
			},
			{
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "text_delta", Text: "TaskCreate is now available"}},
				{Type: types.EventContentBlockStop, Index: 0},
				{Type: types.EventMessageStop},
			},
		},
	}

	reg := registry.New()
	reg.Register(&namedMockTool{name: "Read", desc: "read files"})
	reg.Register(&toolSearchLoaderMockTool{})
	reg.Register(&namedMockTool{name: "TaskCreate", desc: "create task"})

	ql := New(p, reg, Config{MaxTurns: 10, MaxTokens: 1024})
	err := ql.Run(context.Background(), "load task tool", func(Event) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(p.toolSets) != 2 {
		t.Fatalf("expected 2 provider turns, got %d", len(p.toolSets))
	}
	if containsString(p.toolSets[0], "TaskCreate") {
		t.Fatalf("TaskCreate should be deferred on the first turn, got %v", p.toolSets[0])
	}
	if !containsString(p.toolSets[0], "ToolSearch") {
		t.Fatalf("ToolSearch should be visible on the first turn, got %v", p.toolSets[0])
	}
	if !containsString(p.toolSets[1], "TaskCreate") {
		t.Fatalf("TaskCreate should be visible after ToolSearch loads it, got %v", p.toolSets[1])
	}
}

func TestIntegrationRestoredToolReferenceKeepsDeferredToolVisible(t *testing.T) {
	reg := registry.New()
	reg.Register(&toolSearchLoaderMockTool{})
	reg.Register(&namedMockTool{name: "TaskCreate", desc: "create task"})
	provider := &recordingMockProvider{responses: [][]types.StreamEvent{{
		{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "restored"}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: stopReasonPtr(types.StopReasonEndTurn)},
		{Type: types.EventMessageStop},
	}}}
	q := New(provider, reg, Config{MaxTurns: 1, MaxTokens: 128})
	q.SetMessages([]types.Message{
		types.UserMessage("load TaskCreate"),
		types.AssistantMessage("loading"),
		types.ToolResultMessage(types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: "tool-search-1",
			ContentBlocks: []types.ContentBlock{
				types.ToolReferenceBlock{Type: types.ContentTypeToolReference, ToolName: "TaskCreate"},
			},
		}),
	})
	if err := q.Run(context.Background(), "continue", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if len(provider.toolSets) != 1 || !containsString(provider.toolSets[0], "TaskCreate") {
		t.Fatalf("restored deferred tools = %v, want TaskCreate visible", provider.toolSets)
	}
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
