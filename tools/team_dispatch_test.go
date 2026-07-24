package tools

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/coordinator"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// ─── Mock provider ────────────────────────────────────────────────────────────

// mockProvider returns canned text responses for sub-agent loops.
type mockProvider struct {
	mu        sync.Mutex
	responses []string // one text response per CreateStream call
	callIdx   int
}

func (m *mockProvider) Name() string    { return "mock" }
func (m *mockProvider) ModelID() string { return "mock-model" }

func (m *mockProvider) CreateStream(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	m.mu.Lock()
	idx := m.callIdx
	m.callIdx++
	m.mu.Unlock()

	text := "(no response)"
	if idx < len(m.responses) {
		text = m.responses[idx]
	}

	ch := make(chan types.StreamEvent, 16)
	go func() {
		defer close(ch)
		ch <- types.StreamEvent{
			Type:         types.EventContentBlockStart,
			Index:        0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
		}
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: text},
		}
		ch <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
		ch <- types.StreamEvent{Type: types.EventMessageStop}
	}()
	return ch, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newTestManagerWithProvider(t *testing.T, p provider.Provider) *TeamManager {
	t.Helper()
	home := t.TempDir()
	mgr := newTestManagerForHome(t, home)
	mgr.Provider = p
	mgr.Registry = registry.New()
	mgr.System = "You are a test agent."
	return mgr
}

// ─── TeamCreate with Execute ──────────────────────────────────────────────────

func TestTeamCreate_AgentExecuteFunc_Stub(t *testing.T) {
	// No provider → stub execute
	mgr := newTestManager(t)
	tool := NewTeamCreateTool(mgr)
	tool.Execute(context.Background(), map[string]any{ //nolint:errcheck
		"team_name": "stub-team",
	})

	// Dispatch a task and verify the stub runs
	mgr.coordinator.AddTask("do something", 0)
	results := mgr.coordinator.Dispatch(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected error: %v", results[0].Error)
	}
	if !strings.Contains(results[0].Result, "team-lead@stub-team") {
		t.Errorf("expected agent id in result, got: %s", results[0].Result)
	}
}

func TestTeamCreate_AgentExecuteFunc_SubLoop(t *testing.T) {
	p := &mockProvider{responses: []string{"task done"}}
	mgr := newTestManagerWithProvider(t, p)
	tool := NewTeamCreateTool(mgr)
	tool.Execute(context.Background(), map[string]any{ //nolint:errcheck
		"team_name":  "real-team",
		"agent_type": "executor",
	})

	mgr.coordinator.AddTask("write something", 0)
	results := mgr.coordinator.Dispatch(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected error: %v", results[0].Error)
	}
	if results[0].Result != "task done" {
		t.Errorf("expected 'task done', got: %q", results[0].Result)
	}
}

// ─── TeamDispatchTool ─────────────────────────────────────────────────────────

func TestTeamDispatch_Success(t *testing.T) {
	p := &mockProvider{responses: []string{"result-a", "result-b"}}
	mgr := newTestManagerWithProvider(t, p)

	// TeamCreate creates the lead member; teammate spawning is handled by Agent.
	NewTeamCreateTool(mgr).Execute(context.Background(), map[string]any{ //nolint:errcheck
		"team_name":  "dispatch-team",
		"agent_type": "worker",
	})

	dispatchTool := NewTeamDispatchTool(mgr)
	res, err := dispatchTool.Execute(context.Background(), map[string]any{
		"team_id": "team-1",
		"tasks": []any{
			map[string]any{"description": "task alpha", "priority": 1},
			map[string]any{"description": "task beta", "priority": 0},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "2 task(s) executed") {
		t.Errorf("expected task count in output, got: %s", res.Content)
	}
}

func TestTeamDispatch_TeamNotFound(t *testing.T) {
	mgr := newTestManager(t)
	res, _ := NewTeamDispatchTool(mgr).Execute(context.Background(), map[string]any{
		"team_id": "team-999",
		"tasks":   []any{map[string]any{"description": "something"}},
	})
	if !res.IsError {
		t.Errorf("expected error for missing team, got: %s", res.Content)
	}
}

func TestTeamDispatch_MissingTeamID(t *testing.T) {
	mgr := newTestManager(t)
	res, _ := NewTeamDispatchTool(mgr).Execute(context.Background(), map[string]any{
		"team_id": "",
		"tasks":   []any{map[string]any{"description": "x"}},
	})
	if !res.IsError {
		t.Errorf("expected error for empty team_id, got: %s", res.Content)
	}
}

func TestTeamDispatch_EmptyTasks(t *testing.T) {
	mgr := newTestManager(t)
	// Create a team first
	NewTeamCreateTool(mgr).Execute(context.Background(), map[string]any{ //nolint:errcheck
		"name":   "t",
		"agents": []any{map[string]any{"id": "a", "role": "r"}},
	})
	res, _ := NewTeamDispatchTool(mgr).Execute(context.Background(), map[string]any{
		"team_id": "team-1",
		"tasks":   []any{},
	})
	if !res.IsError {
		t.Errorf("expected error for empty tasks, got: %s", res.Content)
	}
}

func TestTeamDispatch_EmptyTaskDescription(t *testing.T) {
	mgr := newTestManager(t)
	NewTeamCreateTool(mgr).Execute(context.Background(), map[string]any{ //nolint:errcheck
		"name":   "t",
		"agents": []any{map[string]any{"id": "a", "role": "r"}},
	})
	res, _ := NewTeamDispatchTool(mgr).Execute(context.Background(), map[string]any{
		"team_id": "team-1",
		"tasks":   []any{map[string]any{"description": ""}},
	})
	if !res.IsError {
		t.Errorf("expected error for empty task description, got: %s", res.Content)
	}
}

func TestTeamDispatchToolName(t *testing.T) {
	mgr := newTestManager(t)
	if NewTeamDispatchTool(mgr).Name() != "TeamDispatch" {
		t.Error("expected name 'TeamDispatch'")
	}
}

// ─── SendMessage with IsSubscribed ───────────────────────────────────────────

func TestSendMessage_SubscriptionCheck(t *testing.T) {
	mgr := newTestManager(t)
	// Subscribe "bob" explicitly
	mgr.coordinator.GetBus().Subscribe("bob")

	tool := NewSendMessageTool(mgr)
	res, err := tool.Execute(context.Background(), map[string]any{
		"to":      "bob",
		"summary": "send a quick greeting",
		"message": "hey",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success for subscribed agent, got: %s", res.Content)
	}
}

func TestSendMessage_NotSubscribed(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewSendMessageTool(mgr)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"to":      "ghost",
		"summary": "send a quick greeting",
		"message": "hello",
	})
	if res.IsError || !strings.Contains(res.Content, `"success":true`) {
		t.Errorf("mailbox delivery must not depend on MessageBus subscription: %s", res.Content)
	}
}

func TestSendMessage_DrainPendingReplies(t *testing.T) {
	mgr := newTestManager(t)
	bus := mgr.coordinator.GetBus()

	// Subscribe both sender and recipient
	bus.Subscribe("sender")
	bus.Subscribe("recipient")

	// Pre-load a reply into "sender"'s channel (simulates a queued reply)
	bus.Send(coordinator.Message{From: "recipient", To: "sender", Content: "reply!"}) //nolint:errcheck

	tool := NewSendMessageTool(mgr)
	res, err := tool.Execute(context.Background(), map[string]any{
		"to":      "recipient",
		"summary": "send a direct ping",
		"message": "ping",
		"from":    "sender",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "unknown field") {
		t.Fatalf("caller-supplied from must be rejected, got: %s", res.Content)
	}
	if pending := bus.Drain("sender"); len(pending) != 1 || pending[0].Content != "reply!" {
		t.Errorf("public SendMessage drained caller-selected replies: %#v", pending)
	}
}

// ─── Mock provider satisfies loop usage ──────────────────────────────────────

func TestMockProviderInterface(t *testing.T) {
	var _ provider.Provider = &mockProvider{}
}

func TestMockProvider_ReturnsText(t *testing.T) {
	p := &mockProvider{responses: []string{"hello from mock"}}
	reg := registry.New()
	ql := loop.New(p, reg, loop.Config{MaxTurns: 5, MaxTokens: 1024})

	var got string
	err := ql.Run(context.Background(), "test", func(e loop.Event) {
		if e.Type == loop.EventText {
			got += e.Text
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello from mock" {
		t.Errorf("expected 'hello from mock', got %q", got)
	}
}
