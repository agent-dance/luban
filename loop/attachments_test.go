package loop

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type attachmentCaptureProvider struct {
	mu        sync.Mutex
	responses [][]types.StreamEvent
	calls     []provider.Params
}

func (p *attachmentCaptureProvider) Name() string    { return "capture" }
func (p *attachmentCaptureProvider) ModelID() string { return "capture-model" }

func (p *attachmentCaptureProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	idx := len(p.calls)
	p.calls = append(p.calls, params)
	p.mu.Unlock()

	ch := make(chan types.StreamEvent, 64)
	go func() {
		defer close(ch)
		if idx >= len(p.responses) {
			return
		}
		for _, event := range p.responses[idx] {
			if ctx.Err() != nil {
				return
			}
			ch <- event
		}
	}()
	return ch, nil
}

func (p *attachmentCaptureProvider) callMessages(i int) []types.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]types.Message(nil), p.calls[i].Messages...)
}

func (p *attachmentCaptureProvider) callToolNames(i int) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	names := make([]string, 0, len(p.calls[i].Tools))
	for _, tool := range p.calls[i].Tools {
		names = append(names, tool.Name)
	}
	return names
}

type attachmentStaticTool struct {
	name        string
	content     string
	newMessages []types.Message
}

func (t *attachmentStaticTool) Name() string        { return t.name }
func (t *attachmentStaticTool) Description() string { return t.name + " tool" }
func (t *attachmentStaticTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *attachmentStaticTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: t.content, NewMessages: t.newMessages}, nil
}
func (t *attachmentStaticTool) IsConcurrentSafe() bool { return true }

type readyPrefetch struct {
	readyCalls int
	messages   []types.Message
}

func (p *readyPrefetch) Ready() bool {
	p.readyCalls++
	return p.readyCalls > 1
}
func (p *readyPrefetch) Collect(context.Context) ([]types.Message, error) {
	return append([]types.Message(nil), p.messages...), nil
}

type staticPrefetcher struct {
	pending PendingAttachmentPrefetch
}

func (p staticPrefetcher) StartMemoryPrefetch(context.Context, []types.Message) PendingAttachmentPrefetch {
	return p.pending
}
func (p staticPrefetcher) StartSkillPrefetch(context.Context, []types.Message) PendingAttachmentPrefetch {
	return p.pending
}

type immediatePrefetch struct {
	messages []types.Message
}

func (p immediatePrefetch) Ready() bool { return true }
func (p immediatePrefetch) Collect(context.Context) ([]types.Message, error) {
	return append([]types.Message(nil), p.messages...), nil
}

type registryRefresher struct {
	next *registry.Registry
}

func (r registryRefresher) RefreshTools(context.Context, *registry.Registry) (*registry.Registry, error) {
	return r.next, nil
}

type attachmentMCPState struct {
	mu      sync.Mutex
	servers []compact.MCPServerSnapshot
}

func (s *attachmentMCPState) PostCompactMCPServers() []compact.MCPServerSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]compact.MCPServerSnapshot(nil), s.servers...)
}

func (s *attachmentMCPState) Set(servers ...compact.MCPServerSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.servers = append([]compact.MCPServerSnapshot(nil), servers...)
}

type mcpStateRefresher struct {
	state *attachmentMCPState
	next  []compact.MCPServerSnapshot
}

func (r mcpStateRefresher) RefreshTools(context.Context, *registry.Registry) (*registry.Registry, error) {
	r.state.Set(r.next...)
	return nil, nil
}

func attachmentToolUseEvents(id, name string, input map[string]any) []types.StreamEvent {
	data, _ := json.Marshal(input)
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: id, Name: name}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(data)}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: attachmentStopReasonPtr(types.StopReasonToolUse)},
		{Type: types.EventMessageStop},
	}
}

func attachmentTextEvents(text string) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: attachmentStopReasonPtr(types.StopReasonEndTurn)},
		{Type: types.EventMessageStop},
	}
}

func attachmentStopReasonPtr(v types.StopReason) *types.StopReason { return &v }

func messageTexts(messages []types.Message) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		if text := msg.GetText(); text != "" {
			out = append(out, text)
			continue
		}
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok {
				out = append(out, "tool_result:"+tr.TextContent())
			}
		}
	}
	return out
}

func assertTextInOrder(t *testing.T, haystack string, needles []string) {
	t.Helper()
	offset := 0
	for _, needle := range needles {
		idx := strings.Index(haystack[offset:], needle)
		if idx < 0 {
			t.Fatalf("expected %q after offset %d in:\n%s", needle, offset, haystack)
		}
		offset += idx + len(needle)
	}
}

func TestAttachmentPipelineOrderBeforeNextProviderRequest(t *testing.T) {
	p := &attachmentCaptureProvider{responses: [][]types.StreamEvent{
		attachmentToolUseEvents("toolu_attach", "Attach", map[string]any{}),
		attachmentTextEvents("done"),
	}}
	reg := registry.New()
	reg.Register(&attachmentStaticTool{
		name:    "Attach",
		content: "tool output",
		newMessages: []types.Message{
			types.UserMessage("tool new message"),
		},
	})
	queue := NewMemoryCommandQueue(QueuedCommand{UUID: "cmd-1", Mode: "prompt", Content: "queued prompt", Priority: CommandPriorityNext})
	skill := immediatePrefetch{messages: []types.Message{types.UserMessage("skill discovery")}}

	ql := New(p, reg, Config{
		MaxTurns:        10,
		MaxTokens:       1024,
		CommandQueue:    queue,
		SkillPrefetcher: staticPrefetcher{pending: skill},
	})
	if err := ql.Run(context.Background(), "start", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	texts := strings.Join(messageTexts(p.callMessages(1)), "\n")
	wantOrder := []string{"tool_result:tool output", "tool new message", "queued prompt", "skill discovery"}
	last := -1
	for _, want := range wantOrder {
		idx := strings.Index(texts, want)
		if idx < 0 {
			t.Fatalf("next provider messages missing %q:\n%s", want, texts)
		}
		if idx <= last {
			t.Fatalf("message order for %q is wrong; texts:\n%s", want, texts)
		}
		last = idx
	}
	if got := queue.Started(); len(got) != 1 || got[0] != "cmd-1" {
		t.Fatalf("started lifecycle = %v, want [cmd-1]", got)
	}
	if got := queue.Completed(); len(got) != 1 || got[0] != "cmd-1" {
		t.Fatalf("completed lifecycle = %v, want [cmd-1]", got)
	}
	if got := queue.Remaining(); len(got) != 0 {
		t.Fatalf("remaining commands = %#v, want none", got)
	}
}

func TestAttachmentPipelineSleepToolDrainsLaterQueuedCommands(t *testing.T) {
	p := &attachmentCaptureProvider{responses: [][]types.StreamEvent{
		attachmentToolUseEvents("toolu_sleep", "Sleep", map[string]any{}),
		attachmentTextEvents("done"),
	}}
	reg := registry.New()
	reg.Register(&attachmentStaticTool{name: "Sleep", content: "slept"})
	queue := NewMemoryCommandQueue(QueuedCommand{UUID: "later", Mode: "task-notification", Content: "later notification", Priority: CommandPriorityLater})

	ql := New(p, reg, Config{MaxTurns: 10, MaxTokens: 1024, CommandQueue: queue})
	if err := ql.Run(context.Background(), "start", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if texts := strings.Join(messageTexts(p.callMessages(1)), "\n"); !strings.Contains(texts, "later notification") {
		t.Fatalf("Sleep turn should drain later queue, got:\n%s", texts)
	}
}

func TestAttachmentPipelineSubagentConsumesOnlyAddressedTaskNotifications(t *testing.T) {
	p := &attachmentCaptureProvider{responses: [][]types.StreamEvent{
		attachmentToolUseEvents("toolu_echo", "Echo", map[string]any{}),
		attachmentTextEvents("done"),
	}}
	reg := registry.New()
	reg.Register(&attachmentStaticTool{name: "Echo", content: "ok"})
	queue := NewMemoryCommandQueue(
		QueuedCommand{UUID: "prompt-a", Mode: "prompt", Content: "subagent prompt should not leak", AgentID: "agent-a", Priority: CommandPriorityNext},
		QueuedCommand{UUID: "task-a", Mode: "task-notification", Content: "task for agent a", AgentID: "agent-a", Priority: CommandPriorityNext},
		QueuedCommand{UUID: "task-b", Mode: "task-notification", Content: "task for agent b", AgentID: "agent-b", Priority: CommandPriorityNext},
	)

	ql := New(p, reg, Config{
		MaxTurns:     10,
		MaxTokens:    1024,
		CommandQueue: queue,
		QueryScope:   QueryScope{IsSubagent: true, AgentID: "agent-a"},
	})
	if err := ql.Run(context.Background(), "start", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	texts := strings.Join(messageTexts(p.callMessages(1)), "\n")
	if !strings.Contains(texts, "task for agent a") {
		t.Fatalf("subagent did not receive addressed task notification:\n%s", texts)
	}
	for _, forbidden := range []string{"subagent prompt should not leak", "task for agent b"} {
		if strings.Contains(texts, forbidden) {
			t.Fatalf("subagent consumed forbidden command %q:\n%s", forbidden, texts)
		}
	}
}

func TestAttachmentPipelineMemoryPrefetchDoesNotBlockAndCanBeConsumedNextTurn(t *testing.T) {
	p := &attachmentCaptureProvider{responses: [][]types.StreamEvent{
		attachmentToolUseEvents("toolu_1", "Echo", map[string]any{}),
		attachmentToolUseEvents("toolu_2", "Echo", map[string]any{}),
		attachmentTextEvents("done"),
	}}
	reg := registry.New()
	reg.Register(&attachmentStaticTool{name: "Echo", content: "ok"})
	pending := &readyPrefetch{messages: []types.Message{types.UserMessage("memory attachment")}}

	ql := New(p, reg, Config{
		MaxTurns:         10,
		MaxTokens:        1024,
		MemoryPrefetcher: staticPrefetcher{pending: pending},
	})
	if err := ql.Run(context.Background(), "start", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if texts := strings.Join(messageTexts(p.callMessages(1)), "\n"); strings.Contains(texts, "memory attachment") {
		t.Fatalf("memory prefetch was injected before Ready:\n%s", texts)
	}
	if texts := strings.Join(messageTexts(p.callMessages(2)), "\n"); !strings.Contains(texts, "memory attachment") {
		t.Fatalf("memory prefetch was not consumed on a later turn:\n%s", texts)
	}
}

func TestAttachmentPipelineRefreshToolsVisibleOnNextTurn(t *testing.T) {
	p := &attachmentCaptureProvider{responses: [][]types.StreamEvent{
		attachmentToolUseEvents("toolu_echo", "Echo", map[string]any{}),
		attachmentTextEvents("done"),
	}}
	reg := registry.New()
	reg.Register(&attachmentStaticTool{name: "Echo", content: "ok"})
	refreshed := reg.Clone()
	refreshed.Register(&attachmentStaticTool{name: "FreshMCPTool", content: "fresh"})

	ql := New(p, reg, Config{
		MaxTurns:      10,
		MaxTokens:     1024,
		ToolRefresher: registryRefresher{next: refreshed},
	})
	if err := ql.Run(context.Background(), "start", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if names := p.callToolNames(0); containsString(names, "FreshMCPTool") {
		t.Fatalf("fresh tool should not be visible before refresh: %v", names)
	}
	if names := p.callToolNames(1); !containsString(names, "FreshMCPTool") {
		t.Fatalf("fresh tool should be visible after refresh: %v", names)
	}
}

func TestMCPInstructionsDeltaNoInstructionsProducesNoPromptNoise(t *testing.T) {
	p := &attachmentCaptureProvider{responses: [][]types.StreamEvent{
		attachmentTextEvents("done"),
	}}
	state := &attachmentMCPState{}
	state.Set(
		compact.MCPServerSnapshot{Name: "empty"},
		compact.MCPServerSnapshot{Name: "blank", Instructions: " \n\t"},
	)

	ql := New(p, registry.New(), Config{MaxTurns: 1, MaxTokens: 1024, MCPState: state})
	if err := ql.Run(context.Background(), "start", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	texts := strings.Join(messageTexts(p.callMessages(0)), "\n")
	if strings.Contains(texts, "MCP Server Instructions") {
		t.Fatalf("empty MCP instructions should not be injected:\n%s", texts)
	}
}

func TestMCPInstructionsDeltaOneServer(t *testing.T) {
	p := &attachmentCaptureProvider{responses: [][]types.StreamEvent{
		attachmentTextEvents("done"),
	}}
	state := &attachmentMCPState{}
	state.Set(compact.MCPServerSnapshot{Name: "docs", Instructions: "Prefer official docs."})

	ql := New(p, registry.New(), Config{MaxTurns: 1, MaxTokens: 1024, MCPState: state})
	if err := ql.Run(context.Background(), "start", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	texts := strings.Join(messageTexts(p.callMessages(0)), "\n")
	assertTextInOrder(t, texts, []string{
		"<system-reminder>",
		"# MCP Server Instructions",
		"## docs\nPrefer official docs.",
		"</system-reminder>",
	})
}

func TestMCPInstructionsDeltaMultipleServers(t *testing.T) {
	p := &attachmentCaptureProvider{responses: [][]types.StreamEvent{
		attachmentTextEvents("done"),
	}}
	state := &attachmentMCPState{}
	state.Set(
		compact.MCPServerSnapshot{Name: "zeta", Instructions: "Use zeta."},
		compact.MCPServerSnapshot{Name: "alpha", Instructions: "Use alpha."},
	)

	ql := New(p, registry.New(), Config{MaxTurns: 1, MaxTokens: 1024, MCPState: state})
	if err := ql.Run(context.Background(), "start", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	texts := strings.Join(messageTexts(p.callMessages(0)), "\n")
	assertTextInOrder(t, texts, []string{"## alpha\nUse alpha.", "## zeta\nUse zeta."})
}

func TestMCPInstructionsDeltaLateConnectionDoesNotRewriteSystemBlocks(t *testing.T) {
	p := &attachmentCaptureProvider{responses: [][]types.StreamEvent{
		attachmentToolUseEvents("toolu_echo", "Echo", map[string]any{}),
		attachmentTextEvents("done"),
	}}
	reg := registry.New()
	reg.Register(&attachmentStaticTool{name: "Echo", content: "ok"})
	state := &attachmentMCPState{}
	staticBlocks := []prompt.SystemPromptBlock{{
		Text:       "static prompt",
		Source:     "built_in",
		Name:       "static",
		Cache:      true,
		CacheScope: "ephemeral",
	}}

	ql := New(p, reg, Config{
		MaxTurns:     10,
		MaxTokens:    1024,
		SystemBlocks: staticBlocks,
		MCPState:     state,
		ToolRefresher: mcpStateRefresher{
			state: state,
			next:  []compact.MCPServerSnapshot{{Name: "late", Instructions: "Late server guidance."}},
		},
	})
	if err := ql.Run(context.Background(), "start", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	firstTexts := strings.Join(messageTexts(p.callMessages(0)), "\n")
	if strings.Contains(firstTexts, "Late server guidance") {
		t.Fatalf("late instructions should not appear before refresh:\n%s", firstTexts)
	}
	secondTexts := strings.Join(messageTexts(p.callMessages(1)), "\n")
	if !strings.Contains(secondTexts, "## late\nLate server guidance.") {
		t.Fatalf("late instructions were not injected as a delta:\n%s", secondTexts)
	}
	if !reflect.DeepEqual(p.calls[0].SystemBlocks, p.calls[1].SystemBlocks) {
		t.Fatalf("late MCP instructions rewrote system blocks:\nfirst=%#v\nsecond=%#v", p.calls[0].SystemBlocks, p.calls[1].SystemBlocks)
	}
}
