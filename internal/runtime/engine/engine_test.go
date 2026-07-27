package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/contracts/stream"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// ============================================================
// Mock provider
// ============================================================

// mockProvider implements provider.Provider for tests.
// When responses is non-empty, each CreateStream call consumes one entry.
// When exhausted, defaultFn is called; if nil a plain "ok" text stream is returned.
type mockProvider struct {
	mu         sync.Mutex
	name       string
	modelID    string
	callCount  int
	lastParams provider.Params
	responses  [][]types.StreamEvent // consumed one-per-CreateStream call
	defaultFn  func(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error)
}

func (m *mockProvider) Name() string    { return m.name }
func (m *mockProvider) ModelID() string { return m.modelID }

func (m *mockProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	m.mu.Lock()
	idx := m.callCount
	m.callCount++
	m.lastParams = params
	m.mu.Unlock()

	if idx < len(m.responses) {
		evs := m.responses[idx]
		ch := make(chan types.StreamEvent, len(evs)+1)
		for _, ev := range evs {
			ch <- ev
		}
		close(ch)
		return ch, nil
	}

	if m.defaultFn != nil {
		return m.defaultFn(ctx, params)
	}

	// Default: non-empty text response so the empty-response retry does not fire.
	return makeTextStreamCh("ok"), nil
}

// ============================================================
// Stream event helpers
// ============================================================

// textEvents returns stream events that produce a single text block with the given text.
func textEvents(text string) []types.StreamEvent {
	return []types.StreamEvent{
		{
			Type:         types.EventContentBlockStart,
			Index:        0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
		},
		{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: text},
		},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
}

// toolCallEvents returns stream events that produce a tool_use block.
func toolCallEvents(toolID, toolName string, input map[string]any) []types.StreamEvent {
	raw, _ := json.Marshal(input)
	return []types.StreamEvent{
		{
			Type:  types.EventContentBlockStart,
			Index: 0,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse,
				ID:   toolID,
				Name: toolName,
			},
		},
		{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(raw)},
		},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
}

// makeTextStreamCh returns a closed, buffered channel pre-filled with text events.
func makeTextStreamCh(text string) <-chan types.StreamEvent {
	evs := textEvents(text)
	ch := make(chan types.StreamEvent, len(evs))
	for _, ev := range evs {
		ch <- ev
	}
	close(ch)
	return ch
}

// ============================================================
// In-memory SessionManager
// ============================================================

type memorySessionManager struct {
	mu        sync.Mutex
	sessions  map[string][]types.Message
	saveCalls int
}

func newMemorySessionManager() *memorySessionManager {
	return &memorySessionManager{sessions: make(map[string][]types.Message)}
}

func (m *memorySessionManager) Save(id string, msgs []types.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveCalls++
	cp := make([]types.Message, len(msgs))
	copy(cp, msgs)
	m.sessions[id] = cp
	return nil
}

func (m *memorySessionManager) Load(id string) ([]types.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
	}
	cp := make([]types.Message, len(msgs))
	copy(cp, msgs)
	return cp, nil
}

func (m *memorySessionManager) List() ([]SessionInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SessionInfo, 0, len(m.sessions))
	for id, msgs := range m.sessions {
		out = append(out, SessionInfo{ID: id, Messages: len(msgs)})
	}
	return out, nil
}

func (m *memorySessionManager) Latest() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.sessions {
		return id, nil
	}
	return "", ErrSessionNotFound
}

func (m *memorySessionManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, id)
	}
	delete(m.sessions, id)
	return nil
}

// ============================================================
// denyAllHandler: permission.PermissionHandler that always denies
// ============================================================

type denyAllHandler struct{}

func (denyAllHandler) Check(_ context.Context, _ permission.PermissionRequest) (permission.PermissionDecision, error) {
	return permission.PermissionDeny, nil
}

// ============================================================
// Test helper: drain a query channel with a timeout
// ============================================================

func drainEvents(t *testing.T, ch <-chan Event, timeout time.Duration) []Event {
	t.Helper()
	var events []Event
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-deadline:
			t.Fatal("timed out waiting for events to drain")
			return nil
		}
	}
}

// ============================================================
// Tests
// ============================================================

// 1. New(Config{}) without a provider must return ErrNoProvider.
func TestNewEngine_RequiresProvider(t *testing.T) {
	_, err := New(Config{})
	if !errors.Is(err, ErrNoProvider) {
		t.Fatalf("expected ErrNoProvider, got: %v", err)
	}
}

// 2. New with just Provider must fill in sensible defaults.
func TestNewEngine_Defaults(t *testing.T) {
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	e, err := New(Config{Provider: p})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if e.cfg.MaxTurns != 100 {
		t.Errorf("MaxTurns: want 100, got %d", e.cfg.MaxTurns)
	}
	if e.cfg.EventBufferSize != 64 {
		t.Errorf("EventBufferSize: want 64, got %d", e.cfg.EventBufferSize)
	}
	if e.cfg.Registry == nil {
		t.Error("Registry must not be nil")
	}
	if _, ok := e.cfg.Permission.(permission.AllowAllHandler); !ok {
		t.Errorf("Permission: want permission.AllowAllHandler, got %T", e.cfg.Permission)
	}
}

// 3. Query basic flow: text events arrive, Final event has no error, session is auto-saved.
func TestQuery_BasicFlow(t *testing.T) {
	sm := newMemorySessionManager()
	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents("hello world")},
	}

	e, err := New(Config{Provider: p, Sessions: sm})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, err := e.Query(context.Background(), QueryRequest{
		SessionID: "basic-session",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	events := drainEvents(t, ch, 5*time.Second)

	var finalEv *Event
	var textParts []string
	for i := range events {
		ev := &events[i]
		if ev.Final {
			finalEv = ev
		}
		if ev.Inner.Type == stream.EventText {
			textParts = append(textParts, ev.Inner.Text)
		}
	}

	if finalEv == nil {
		t.Fatal("no Final event received")
	}
	if finalEv.Error != nil {
		t.Errorf("Final event error: %v", finalEv.Error)
	}
	if len(textParts) == 0 {
		t.Error("expected at least one text event")
	}

	// Auto-save must have occurred.
	sm.mu.Lock()
	saves := sm.saveCalls
	sm.mu.Unlock()
	if saves == 0 {
		t.Error("session was not auto-saved after query")
	}
}

func TestQuery_SendsSystemPromptBlocks(t *testing.T) {
	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents("ok")},
	}
	e, err := New(Config{
		Provider:     p,
		Sessions:     newMemorySessionManager(),
		SystemPrompt: "plain-system",
		SystemPromptBlocks: []prompt.SystemPromptBlock{
			{Text: "first"},
			{Text: "second"},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, err := e.Query(context.Background(), QueryRequest{
		SessionID: "system-blocks-session",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	_ = drainEvents(t, ch, 5*time.Second)

	if got := p.lastParams.JoinedSystemPrompt(); got != "first\n\nsecond" {
		t.Fatalf("JoinedSystemPrompt = %q, want joined system blocks", got)
	}
	if len(p.lastParams.SystemBlocks) != 2 {
		t.Fatalf("SystemBlocks = %d, want 2", len(p.lastParams.SystemBlocks))
	}
}

func TestQuery_SendsConfiguredUserContext(t *testing.T) {
	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents("ok")},
	}
	e, err := New(Config{
		Provider:    p,
		Sessions:    newMemorySessionManager(),
		UserContext: prompt.UserContext{Instructions: "workspace instruction sentinel"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, err := e.Query(context.Background(), QueryRequest{
		SessionID: "user-context-session",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	_ = drainEvents(t, ch, 5*time.Second)

	if len(p.lastParams.Messages) < 2 || !p.lastParams.Messages[0].IsMeta {
		t.Fatalf("provider messages missing leading meta context: %#v", p.lastParams.Messages)
	}
	if !strings.Contains(p.lastParams.Messages[0].GetText(), "workspace instruction sentinel") {
		t.Fatalf("provider meta context omitted workspace instructions: %q", p.lastParams.Messages[0].GetText())
	}
}

func TestQuery_SendsConfiguredReasoningEffort(t *testing.T) {
	p := &mockProvider{
		name:      "mock",
		modelID:   "reasoning-model",
		responses: [][]types.StreamEvent{textEvents("ok")},
	}
	e, err := New(Config{
		Provider:        p,
		Sessions:        newMemorySessionManager(),
		ReasoningEffort: "medium",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, err := e.Query(context.Background(), QueryRequest{
		SessionID: "reasoning-effort-session",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	_ = drainEvents(t, ch, 5*time.Second)

	if got := p.lastParams.ReasoningEffort; got != "medium" {
		t.Fatalf("ReasoningEffort = %q, want configured default", got)
	}
}

func TestQuery_PersistsSessionContextMetadata(t *testing.T) {
	dir := t.TempDir()
	sm := newFileSessionManager(dir)
	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents("ok")},
	}

	e, err := New(Config{Provider: p, Sessions: sm, CWD: "/repo/worktree"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, err := e.Query(context.Background(), QueryRequest{
		SessionID: "meta-session",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	_ = drainEvents(t, ch, 5*time.Second)

	meta, err := sm.store.GetMeta("meta-session")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if meta.CWD != "/repo/worktree" {
		t.Fatalf("expected persisted cwd, got %q", meta.CWD)
	}
}

// 4. Query with empty SessionID must generate a UUID.
func TestQuery_NewSessionID(t *testing.T) {
	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents("hi back")},
	}
	e, err := New(Config{Provider: p, Sessions: newMemorySessionManager()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Empty SessionID → engine must assign one.
	ch, err := e.Query(context.Background(), QueryRequest{Message: "hello"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	events := drainEvents(t, ch, 5*time.Second)
	if len(events) == 0 {
		t.Fatal("no events received")
	}

	sid := events[0].SessionID
	if sid == "" {
		t.Fatal("expected non-empty SessionID on event")
	}
	// UUID v4 is 36 chars: 8-4-4-4-12 plus 4 dashes.
	if len(sid) != 36 {
		t.Errorf("session ID does not look like a UUID: %q", sid)
	}

	// All events must carry the same session ID.
	for _, ev := range events {
		if ev.SessionID != sid {
			t.Errorf("inconsistent SessionID: want %q, got %q", sid, ev.SessionID)
		}
	}
}

// 5. Query after Shutdown must return ErrShutdown.
func TestQuery_AfterShutdown(t *testing.T) {
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	e, err := New(Config{Provider: p, Sessions: newMemorySessionManager()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := e.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	_, err = e.Query(context.Background(), QueryRequest{Message: "too late"})
	if !errors.Is(err, ErrShutdown) {
		t.Fatalf("expected ErrShutdown, got: %v", err)
	}
}

// 6. Interrupt must cause an in-flight query channel to close.
func TestInterrupt_CancelsQuery(t *testing.T) {
	const sessionID = "interrupt-session"

	// ready is closed when CreateStream is first called, signalling the stream is live.
	ready := make(chan struct{})
	var readyOnce sync.Once

	p := &mockProvider{
		name:    "mock",
		modelID: "mock-model",
		defaultFn: func(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
			readyOnce.Do(func() { close(ready) })
			ch := make(chan types.StreamEvent) // unbuffered; never sends
			go func() {
				defer close(ch)
				<-ctx.Done() // unblock when context is cancelled
			}()
			return ch, nil
		},
	}

	e, err := New(Config{Provider: p, Sessions: newMemorySessionManager()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, err := e.Query(context.Background(), QueryRequest{
		SessionID: sessionID,
		Message:   "will be interrupted",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Wait for the stream to be live before interrupting.
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stream to start")
	}

	e.Interrupt(sessionID)

	// The channel must close after the interrupt.
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // success
			}
		case <-timeout:
			t.Fatal("timed out: query channel did not close after Interrupt")
		}
	}
}

// 7. Resume must load stored messages and report the correct count.
func TestResume_LoadsMessages(t *testing.T) {
	const sessionID = "resume-session"
	sm := newMemorySessionManager()

	// Pre-populate the session manager with a prior conversation.
	msgs := []types.Message{
		types.UserMessage("first question"),
		types.AssistantMessage("first answer"),
		types.UserMessage("follow-up"),
		types.AssistantMessage("follow-up answer"),
	}
	if err := sm.Save(sessionID, msgs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	p := &mockProvider{name: "mock", modelID: "mock-model"}
	e, err := New(Config{Provider: p, Sessions: sm})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	count, err := e.Resume(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if count != len(msgs) {
		t.Errorf("Resume returned %d messages, want %d", count, len(msgs))
	}
}

func TestResumeWithRuntimeContextStagesTargetWithoutChangingDefaults(t *testing.T) {
	const sessionID = "staged-runtime-session"
	sm := newMemorySessionManager()
	if err := sm.Save(sessionID, []types.Message{types.UserMessage("stored target history")}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	e, err := New(Config{
		Provider: p, Sessions: sm, SystemPrompt: "old system", CWD: "/old",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	targetHooks := hooks.NewRunner([]hooks.Hook{{Type: hooks.HookPreToolUse, Command: "target-hook"}})
	targetRuntime := RuntimeContext{
		SystemPrompt: "target system", HookRunner: targetHooks, CWD: "/target",
	}

	count, err := e.ResumeWithRuntimeContext(context.Background(), sessionID, "", targetRuntime)
	if err != nil {
		t.Fatalf("ResumeWithRuntimeContext: %v", err)
	}
	if count != 1 {
		t.Fatalf("message count = %d, want 1", count)
	}
	if e.cfg.SystemPrompt != "old system" || e.cfg.CWD != "/old" || e.cfg.HookRunner != nil {
		t.Fatalf("staged resume changed public defaults: %+v", e.cfg)
	}

	ch, err := e.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "continue target"})
	if err != nil {
		t.Fatalf("Query staged session: %v", err)
	}
	drainEvents(t, ch, 5*time.Second)
	p.mu.Lock()
	params := p.lastParams
	p.mu.Unlock()
	if params.System != "target system" {
		t.Fatalf("staged conversation system = %q, want target system", params.System)
	}
}

func TestPrepareResumeDoesNotMutateExistingConversationUntilCommit(t *testing.T) {
	const sessionID = "two-phase-resume"
	sm := newMemorySessionManager()
	if err := sm.Save(sessionID, []types.Message{types.UserMessage("old history")}); err != nil {
		t.Fatal(err)
	}
	e, err := New(Config{Provider: &mockProvider{name: "mock", modelID: "mock-model"}, Sessions: sm})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	if err := sm.Save(sessionID, []types.Message{types.UserMessage("new history")}); err != nil {
		t.Fatal(err)
	}

	prepared, err := e.PrepareResume(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	e.convsMu.RLock()
	existing := e.convs[e.currentConversationKey(sessionID)]
	e.convsMu.RUnlock()
	if got := existing.ql.Messages()[0].GetText(); got != "old history" {
		t.Fatalf("prepare mutated existing conversation to %q", got)
	}
	prepared.Abort()
	if got := existing.ql.Messages()[0].GetText(); got != "old history" {
		t.Fatalf("abort mutated existing conversation to %q", got)
	}

	prepared, err = e.PrepareResume(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := existing.ql.Messages()[0].GetText(); got != "new history" {
		t.Fatalf("commit left existing conversation at %q", got)
	}
}

func TestPreparedResumeCommitContextRejectsCancellationBeforePublication(t *testing.T) {
	const sessionID = "cancel-before-commit"
	sm := newMemorySessionManager()
	if err := sm.Save(sessionID, []types.Message{types.UserMessage("stored history")}); err != nil {
		t.Fatal(err)
	}
	e, err := New(Config{Provider: &mockProvider{name: "mock", modelID: "mock-model"}, Sessions: sm})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := e.PrepareResume(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	contextual, ok := prepared.(ContextualPreparedRuntimeContextResume)
	if !ok {
		t.Fatal("prepared resume does not support contextual commit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := contextual.CommitContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("CommitContext error = %v, want context canceled", err)
	}
	e.convsMu.RLock()
	_, published := e.convs[e.currentConversationKey(sessionID)]
	e.convsMu.RUnlock()
	if published {
		t.Fatal("cancelled contextual commit published the conversation")
	}
	prepared.Abort()
}

func TestPrepareRuntimeResumeReplacesExistingConversationRuntimeOnCommit(t *testing.T) {
	const sessionID = "replace-runtime"
	sm := newMemorySessionManager()
	if err := sm.Save(sessionID, []types.Message{types.UserMessage("history")}); err != nil {
		t.Fatal(err)
	}
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	e, err := New(Config{Provider: p, Sessions: sm, SystemPrompt: "old system"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	prepared, err := e.PrepareRuntimeContextResume(context.Background(), sessionID, "", RuntimeContext{SystemPrompt: "target system"})
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	ch, err := e.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	drainEvents(t, ch, 5*time.Second)
	p.mu.Lock()
	system := p.lastParams.System
	p.mu.Unlock()
	if system != "target system" {
		t.Fatalf("committed conversation system = %q, want target system", system)
	}
}

func TestResumeWithRuntimeContextLoadsDuplicateIDFromExplicitProject(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectA := repo.ProjectDirForCWD(filepath.Join(t.TempDir(), "project-a"))
	projectB := repo.ProjectDirForCWD(filepath.Join(t.TempDir(), "project-b"))
	const sessionID = "duplicate-session"
	if err := repo.Save(sessionID, projectA, []types.Message{types.UserMessage("project A")}); err != nil {
		t.Fatalf("save project A: %v", err)
	}
	if err := repo.Save(sessionID, projectB, []types.Message{types.UserMessage("project B")}); err != nil {
		t.Fatalf("save project B: %v", err)
	}
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	e, err := New(Config{
		Provider: p,
		Sessions: NewRepositorySessionManager(repo, func() string { return projectA }),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("prime project A conversation: %v", err)
	}

	targetRuntime := RuntimeContext{SystemPrompt: "project B system", CWD: "/project-b"}
	if _, err := e.ResumeWithRuntimeContext(context.Background(), sessionID, projectB, targetRuntime); err != nil {
		t.Fatalf("ResumeWithRuntimeContext: %v", err)
	}
	e.convsMu.RLock()
	conv := e.convs[newConversationKey(projectB, sessionID)]
	e.convsMu.RUnlock()
	if conv == nil {
		t.Fatal("staged conversation missing")
	}
	messages := conv.ql.Messages()
	if len(messages) != 1 || messages[0].GetText() != "project B" {
		t.Fatalf("loaded messages = %#v, want explicit project B transcript", messages)
	}
	if conv.projectDir != projectB {
		t.Fatalf("conversation project = %q, want %q", conv.projectDir, projectB)
	}
	if err := conv.ql.Run(context.Background(), "continue", func(stream.Event) {}); err != nil {
		t.Fatalf("run project B conversation: %v", err)
	}
	p.mu.Lock()
	params := p.lastParams
	p.mu.Unlock()
	if params.System != targetRuntime.SystemPrompt {
		t.Fatalf("duplicate-ID conversation kept project A runtime: system=%q", params.System)
	}
}

func TestBackgroundFollowUpSavesConversationToItsOriginProject(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectA := repo.ProjectDirForCWD(filepath.Join(t.TempDir(), "project-a"))
	projectB := repo.ProjectDirForCWD(filepath.Join(t.TempDir(), "project-b"))
	const sessionID = "origin-session"
	if err := repo.Save(sessionID, projectA, []types.Message{types.UserMessage("origin history")}); err != nil {
		t.Fatal(err)
	}
	currentProject := projectA
	e, err := New(Config{
		Provider: &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions: newRepositorySessionManager(repo, func() string { return currentProject }),
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := e.PrepareRuntimeContextResume(context.Background(), sessionID, projectA, RuntimeContext{CWD: filepath.Dir(projectA)})
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}

	currentProject = projectB
	ch, err := e.QueryFollowUp(context.Background(), QueryRequest{
		SessionID: sessionID, SessionProjectDir: projectA, Message: "background result",
	})
	if err != nil {
		t.Fatal(err)
	}
	drainEvents(t, ch, 5*time.Second)
	messages, err := repo.Load(session.Ref{ID: sessionID, ProjectDir: projectA})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range messages {
		found = found || message.GetText() == "background result"
	}
	if !found {
		t.Fatalf("origin transcript was not updated: messages=%#v", messages)
	}
	if _, err := repo.StoreForProjectDir(projectB).Load(sessionID); err == nil {
		t.Fatal("background follow-up created origin session ID in the current project")
	}
}

func TestDuplicateProjectConversationsRemainIndependentAcrossBackgroundFollowUp(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	rootA := t.TempDir()
	rootB := t.TempDir()
	projectA := repo.ProjectDirForCWD(rootA)
	projectB := repo.ProjectDirForCWD(rootB)
	const sessionID = "duplicate-background-session"
	if err := repo.Save(sessionID, projectA, []types.Message{types.UserMessage("project A history")}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(sessionID, projectB, []types.Message{types.UserMessage("project B history")}); err != nil {
		t.Fatal(err)
	}
	currentProject := projectA
	e, err := New(Config{
		Provider: &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions: newRepositorySessionManager(repo, func() string { return currentProject }),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		project string
		root    string
	}{{projectA, rootA}, {projectB, rootB}} {
		prepared, prepareErr := e.PrepareRuntimeContextResume(context.Background(), sessionID, target.project, RuntimeContext{CWD: target.root})
		if prepareErr != nil {
			t.Fatal(prepareErr)
		}
		if commitErr := prepared.Commit(); commitErr != nil {
			t.Fatal(commitErr)
		}
		currentProject = target.project
	}

	ch, err := e.QueryFollowUp(context.Background(), QueryRequest{
		SessionID: sessionID, SessionProjectDir: projectA, Message: "origin A completion", ProjectRoot: rootA, CWD: rootA,
	})
	if err != nil {
		t.Fatal(err)
	}
	drainEvents(t, ch, 5*time.Second)

	messagesA, err := repo.Load(session.Ref{ID: sessionID, ProjectDir: projectA})
	if err != nil {
		t.Fatal(err)
	}
	messagesB, err := repo.Load(session.Ref{ID: sessionID, ProjectDir: projectB})
	if err != nil {
		t.Fatal(err)
	}
	contains := func(messages []types.Message, text string) bool {
		for _, message := range messages {
			if message.GetText() == text {
				return true
			}
		}
		return false
	}
	if !contains(messagesA, "origin A completion") {
		t.Fatalf("origin completion missing from project A: %#v", messagesA)
	}
	if contains(messagesB, "origin A completion") {
		t.Fatalf("origin completion leaked into project B: %#v", messagesB)
	}
}

func TestDeleteSessionTombstoneIsScopedByProjectForDuplicateID(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	rootA := t.TempDir()
	rootB := t.TempDir()
	projectA := repo.ProjectDirForCWD(rootA)
	projectB := repo.ProjectDirForCWD(rootB)
	const sessionID = "duplicate-delete-session"
	for _, project := range []string{projectA, projectB} {
		if err := repo.Save(sessionID, project, []types.Message{types.UserMessage(project)}); err != nil {
			t.Fatal(err)
		}
	}
	currentProject := projectA
	e, err := New(Config{
		Provider: &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions: newRepositorySessionManager(repo, func() string { return currentProject }),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		project string
		root    string
	}{{projectA, rootA}, {projectB, rootB}} {
		prepared, prepareErr := e.PrepareRuntimeContextResume(context.Background(), sessionID, target.project, RuntimeContext{CWD: target.root})
		if prepareErr != nil {
			t.Fatal(prepareErr)
		}
		if commitErr := prepared.Commit(); commitErr != nil {
			t.Fatal(commitErr)
		}
		currentProject = target.project
	}

	if err := e.DeleteSessionHistory(context.Background(), sessionID, projectA); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Query(context.Background(), QueryRequest{
		SessionID: sessionID, SessionProjectDir: projectA, Message: "deleted A", ProjectRoot: rootA, CWD: rootA,
	}); !errors.Is(err, ErrSessionDeleted) {
		t.Fatalf("project A query error = %v, want ErrSessionDeleted", err)
	}
	ch, err := e.Query(context.Background(), QueryRequest{
		SessionID: sessionID, SessionProjectDir: projectB, Message: "project B remains", ProjectRoot: rootB, CWD: rootB,
	})
	if err != nil {
		t.Fatalf("project B was tombstoned with A: %v", err)
	}
	drainEvents(t, ch, 5*time.Second)
	if _, err := repo.Load(session.Ref{ID: sessionID, ProjectDir: projectA}); err == nil {
		t.Fatal("project A transcript survived deletion")
	}
	messagesB, err := repo.Load(session.Ref{ID: sessionID, ProjectDir: projectB})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, message := range messagesB {
		found = found || message.GetText() == "project B remains"
	}
	if !found {
		t.Fatalf("project B transcript was not independently usable: %#v", messagesB)
	}
}

func TestDeleteSessionPreventsFollowUpAndShutdownResurrection(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	const sessionID = "delete-permanently"
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("old history")}); err != nil {
		t.Fatal(err)
	}
	e, err := New(Config{
		Provider: &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions: newRepositorySessionManager(repo, func() string { return projectDir }),
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := e.PrepareRuntimeContextResume(context.Background(), sessionID, projectDir, RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	deleter, ok := any(e).(SessionHistoryDeleter)
	if !ok {
		t.Fatal("CoreEngine does not implement SessionHistoryDeleter")
	}
	if err := deleter.DeleteSessionHistory(context.Background(), sessionID, projectDir); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.StoreForProjectDir(projectDir).Load(sessionID); err == nil {
		t.Fatal("deleted transcript remains on disk")
	}
	if _, err := e.QueryFollowUp(context.Background(), QueryRequest{SessionID: sessionID, Message: "late completion"}); !errors.Is(err, ErrSessionDeleted) {
		t.Fatalf("follow-up error = %v, want ErrSessionDeleted", err)
	}
	if err := e.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.StoreForProjectDir(projectDir).Load(sessionID); err == nil {
		t.Fatal("shutdown resurrected deleted transcript")
	}
}

func TestDeleteSessionTombstoneSurvivesEngineRestart(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	root := t.TempDir()
	projectDir := repo.ProjectDirForCWD(root)
	const sessionID = "delete-survives-restart"
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("old history")}); err != nil {
		t.Fatal(err)
	}
	newEngine := func() *CoreEngine {
		e, err := New(Config{
			Provider: &mockProvider{name: "mock", modelID: "mock-model"},
			Sessions: newRepositorySessionManager(repo, func() string { return projectDir }),
		})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}

	first := newEngine()
	if err := first.DeleteSessionHistory(context.Background(), sessionID, projectDir); err != nil {
		t.Fatal(err)
	}
	if err := first.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	second := newEngine()
	defer second.Shutdown(context.Background())
	if _, err := second.QueryFollowUp(context.Background(), QueryRequest{
		SessionID: sessionID,
		Message:   "late background completion",
		CWD:       root,
	}); !errors.Is(err, ErrSessionDeleted) {
		t.Fatalf("follow-up after restart = %v, want ErrSessionDeleted", err)
	}
	if _, err := repo.StoreForProjectDir(projectDir).Load(sessionID); !errors.Is(err, session.ErrSessionDeleted) {
		t.Fatalf("deleted history was recreated after restart: %v", err)
	}
}

type cleanupFailingSessionManager struct {
	*memorySessionManager
	cleanupErr error
	deleted    bool
	deletes    int
}

func (m *cleanupFailingSessionManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletes++
	m.deleted = true
	delete(m.sessions, id)
	if m.deletes == 1 {
		return m.cleanupErr
	}
	return nil
}

func (m *cleanupFailingSessionManager) isDeleted(string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deleted, nil
}

func TestDeleteSessionCleanupErrorKeepsLogicalTombstoneAndCanRetry(t *testing.T) {
	cleanupErr := errors.New("injected cleanup failure")
	manager := &cleanupFailingSessionManager{
		memorySessionManager: newMemorySessionManager(),
		cleanupErr:           cleanupErr,
	}
	const sessionID = "cleanup-error-delete"
	manager.sessions[sessionID] = []types.Message{types.UserMessage("old")}
	e, err := New(Config{
		Provider: &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Shutdown(context.Background())
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}

	if err := e.DeleteSessionHistory(context.Background(), sessionID, ""); !errors.Is(err, cleanupErr) {
		t.Fatalf("first DeleteSessionHistory = %v, want cleanup failure", err)
	}
	if _, err := e.QueryFollowUp(context.Background(), QueryRequest{SessionID: sessionID, Message: "late"}); !errors.Is(err, ErrSessionDeleted) {
		t.Fatalf("follow-up after cleanup failure = %v, want ErrSessionDeleted", err)
	}
	if err := e.DeleteSessionHistory(context.Background(), sessionID, ""); err != nil {
		t.Fatalf("retry DeleteSessionHistory: %v", err)
	}
	if manager.deletes != 2 {
		t.Fatalf("persistent cleanup was attempted %d times, want 2", manager.deletes)
	}
}

func TestPreparedResumeCannotCommitAfterSessionDeletion(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	const sessionID = "delete-prepared-resume"
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("old history")}); err != nil {
		t.Fatal(err)
	}
	e, err := New(Config{
		Provider: &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions: newRepositorySessionManager(repo, func() string { return projectDir }),
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := e.PrepareRuntimeContextResume(context.Background(), sessionID, projectDir, RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.DeleteSessionHistory(context.Background(), sessionID, projectDir); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Commit(); !errors.Is(err, ErrSessionDeleted) {
		t.Fatalf("prepared Commit error = %v, want ErrSessionDeleted", err)
	}
	e.convsMu.RLock()
	_, published := e.convs[newConversationKey(projectDir, sessionID)]
	e.convsMu.RUnlock()
	if published {
		t.Fatal("prepared resume published a conversation after permanent deletion")
	}
	if err := e.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.StoreForProjectDir(projectDir).Load(sessionID); err == nil {
		t.Fatal("shutdown resurrected the session committed after deletion")
	}
}

func TestCompactIncludesTranscriptPathAndPreservesResumeMetadata(t *testing.T) {
	const sessionID = "compact-transcript-session"
	dir := t.TempDir()
	sm := newFileSessionManager(dir)
	msgs := make([]types.Message, 0, 24)
	for i := 0; i < 12; i++ {
		msgs = append(msgs,
			types.UserMessage(fmt.Sprintf("question %02d", i)),
			types.AssistantMessage(fmt.Sprintf("answer %02d", i)),
		)
	}
	if err := sm.Save(sessionID, msgs); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := sm.store.SaveMeta(sessionID, session.SessionMeta{Title: "custom resume title", CWD: "/repo/app"}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}

	transcriptPath := sm.TranscriptPath(sessionID)
	if transcriptPath == "" {
		t.Fatal("expected transcript path before compaction")
	}
	if _, err := os.Stat(transcriptPath); err != nil {
		t.Fatalf("expected readable transcript path %q: %v", transcriptPath, err)
	}

	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents(`{"schema":"compact-summary/v2","summary":"compact summary"}`)},
	}
	e, err := New(Config{Provider: p, Sessions: sm})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	beforeGeneration, err := e.ContextGenerationState(sessionID)
	if err != nil {
		t.Fatalf("ContextGenerationState before compact: %v", err)
	}
	result, err := e.Compact(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if !result.Compacted || result.BeforeMessageCount != len(msgs) || result.AfterMessageCount <= 0 || result.ContextGeneration != beforeGeneration.Generation+1 {
		t.Fatalf("Compact result = %#v, want committed generation %d and authoritative counts", result, beforeGeneration.Generation+1)
	}

	compacted, err := sm.Load(sessionID)
	if err != nil {
		t.Fatalf("Load compacted: %v", err)
	}
	if len(compacted) < 2 {
		t.Fatalf("expected compacted boundary and summary, got %d messages", len(compacted))
	}
	summary := compacted[1].GetText()
	resolvedTranscriptPath := transcriptPath
	if resolved, resolveErr := filepath.EvalSymlinks(transcriptPath); resolveErr == nil {
		resolvedTranscriptPath = resolved
	}
	if !strings.Contains(summary, transcriptPath) && !strings.Contains(summary, resolvedTranscriptPath) {
		t.Fatalf("summary missing transcript path %q: %q", transcriptPath, summary)
	}
	meta, err := sm.store.GetMeta(sessionID)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if meta.Title != "custom resume title" {
		t.Fatalf("resume title changed after compaction: got %q", meta.Title)
	}
	if meta.CWD != "/repo/app" {
		t.Fatalf("resume cwd changed after compaction: got %q", meta.CWD)
	}
}

func TestCompactWithoutTranscriptProviderOmitsTranscriptInstructions(t *testing.T) {
	const sessionID = "compact-memory-session"
	sm := newMemorySessionManager()
	msgs := make([]types.Message, 0, 24)
	for i := 0; i < 12; i++ {
		msgs = append(msgs,
			types.UserMessage(fmt.Sprintf("memory question %02d", i)),
			types.AssistantMessage(fmt.Sprintf("memory answer %02d", i)),
		)
	}
	if err := sm.Save(sessionID, msgs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents(`{"schema":"compact-summary/v2","summary":"compact summary"}`)},
	}
	e, err := New(Config{Provider: p, Sessions: sm})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if _, err := e.Compact(context.Background(), sessionID); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	compacted, err := sm.Load(sessionID)
	if err != nil {
		t.Fatalf("Load compacted: %v", err)
	}
	if len(compacted) < 2 {
		t.Fatalf("expected compacted boundary and summary, got %d messages", len(compacted))
	}
	summary := compacted[1].GetText()
	if strings.Contains(summary, "read the full transcript at:") {
		t.Fatalf("summary should not include transcript instructions without provider: %q", summary)
	}
}

func TestCompactCustomInstructionsAffectPrompt(t *testing.T) {
	const sessionID = "compact-custom-instructions-session"
	sm := newMemorySessionManager()
	msgs := make([]types.Message, 0, 24)
	for i := 0; i < 12; i++ {
		msgs = append(msgs,
			types.UserMessage(fmt.Sprintf("custom question %02d", i)),
			types.AssistantMessage(fmt.Sprintf("custom answer %02d", i)),
		)
	}
	if err := sm.Save(sessionID, msgs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents(`{"schema":"compact-summary/v2","summary":"custom compact summary"}`)},
	}
	e, err := New(Config{Provider: p, Sessions: sm})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if _, err := e.Compact(context.Background(), sessionID, "focus on task_12 acceptance"); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	p.mu.Lock()
	params := p.lastParams
	p.mu.Unlock()
	if len(params.Messages) == 0 {
		t.Fatal("compact provider received no messages")
	}
	promptText := params.Messages[len(params.Messages)-1].GetText()
	if !strings.Contains(promptText, "focus on task_12 acceptance") {
		t.Fatalf("compact prompt missing custom instructions: %q", promptText)
	}
}

func TestCompactAfterBoundaryCarriesPreviousSummaryWithoutRawPreBoundaryHistory(t *testing.T) {
	const sessionID = "compact-boundary-projection-session"
	sm := newFileSessionManager(t.TempDir())
	scope, scopeErr := sm.store.MessageControlScope(sessionID)
	if scopeErr != nil {
		t.Fatal(scopeErr)
	}
	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "manual"}, messagecontrol.Runtime()).
		WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	trustedSummary := compact.NewCompactSummaryMessage(
		"previous compact summary should remain in the rolling chain",
		messagecontrol.Runtime(),
	).WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	msgs := []types.Message{
		types.UserMessage("pre-boundary fact that should not be resummarized"),
		boundary,
		trustedSummary,
	}
	for i := 0; i < 24; i++ {
		msgs = append(msgs, types.UserMessage(fmt.Sprintf("new segment message %02d", i)))
	}
	if err := sm.Save(sessionID, msgs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents(`{"schema":"compact-summary/v2","summary":"new segment summary"}`)},
	}
	e, err := New(Config{Provider: p, Sessions: sm})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if _, err := e.Compact(context.Background(), sessionID, "summarize only new segment"); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	p.mu.Lock()
	params := p.lastParams
	p.mu.Unlock()
	previousSummaryIncluded := false
	for i, msg := range params.Messages {
		text := msg.GetText()
		if strings.Contains(text, "pre-boundary fact") {
			t.Fatalf("provider message %d included raw pre-boundary content: %q", i, text)
		}
		if strings.Contains(text, "previous compact summary") {
			previousSummaryIncluded = true
		}
	}
	if !previousSummaryIncluded {
		t.Fatal("rolling compaction dropped the previous summary")
	}
}

func TestManualCompactDoesNotReopenLiveReadPathAndPersistsHookResults(t *testing.T) {
	const sessionID = "manual-compact-result-session"
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	readPath := filepath.Join(dir, "keep.go")
	if err := os.WriteFile(readPath, []byte("package keep\n\nconst Answer = 42\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sm := newMemorySessionManager()
	msgs := []types.Message{
		types.UserMessage("please read a file"),
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{types.ToolUseBlock{
				Type:  types.ContentTypeToolUse,
				ID:    "toolu_read_keep",
				Name:  "Read",
				Input: map[string]any{"file_path": readPath},
			}},
		},
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{types.ToolResultBlock{
				Type:      types.ContentTypeToolResult,
				ToolUseID: "toolu_read_keep",
				Content:   "package keep\n\nconst Answer = 42\n",
				Outcome:   types.ToolOutcomeSucceeded,
			}},
		},
	}
	for i := 0; i < 12; i++ {
		msgs = append(msgs,
			types.UserMessage(fmt.Sprintf("follow-up question %02d", i)),
			types.AssistantMessage(fmt.Sprintf("follow-up answer %02d", i)),
		)
	}
	if err := sm.Save(sessionID, msgs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents(`{"schema":"compact-summary/v2","summary":"manual compact summary"}`)},
	}
	e, err := New(Config{
		Provider: p,
		Sessions: sm,
		HookRunner: hooks.NewRunner([]hooks.Hook{{
			Type:    hooks.HookSessionStart,
			Command: `printf '{"system_reminder":"manual compact hook result"}'`,
		}}),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if _, err := e.Compact(context.Background(), sessionID, "manual result parity"); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	compacted, err := sm.Load(sessionID)
	if err != nil {
		t.Fatalf("Load compacted: %v", err)
	}
	if len(compacted) < 4 {
		t.Fatalf("compacted messages = %d, want boundary, summary, preserved tail, attachments/hooks", len(compacted))
	}
	if _, ok := compact.ParseCompactBoundaryMessage(compacted[0]); !ok {
		t.Fatalf("first persisted message is not compact boundary: %q", compacted[0].GetText())
	}
	if !strings.Contains(compacted[1].GetText(), "manual compact summary") {
		t.Fatalf("summary message missing compact summary: %q", compacted[1].GetText())
	}
	var reopenedLiveFile, hasHook bool
	for _, msg := range compacted {
		text := msg.GetText()
		if strings.Contains(text, readPath) && strings.Contains(text, "const Answer = 42") {
			reopenedLiveFile = true
		}
		if strings.Contains(text, "manual compact hook result") {
			hasHook = true
		}
	}
	if reopenedLiveFile {
		t.Fatalf("manual compact reopened a live Read path without immutable evidence: %#v", compacted)
	}
	if !hasHook {
		t.Fatalf("persisted compact messages missing hook result: %#v", compacted)
	}
	if sm.saveCalls < 2 {
		t.Fatalf("session save calls = %d, want initial save plus compact persistence", sm.saveCalls)
	}
}

func TestManualCompactLLMFailureDoesNotFallbackToLastFour(t *testing.T) {
	const sessionID = "manual-compact-failure-session"
	sm := newMemorySessionManager()
	msgs := make([]types.Message, 0, 24)
	for i := 0; i < 12; i++ {
		msgs = append(msgs,
			types.UserMessage(fmt.Sprintf("failure question %02d", i)),
			types.AssistantMessage(fmt.Sprintf("failure answer %02d", i)),
		)
	}
	if err := sm.Save(sessionID, msgs); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p := &mockProvider{
		name:    "mock",
		modelID: "mock-model",
		defaultFn: func(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
			return nil, errors.New("summary provider unavailable")
		},
	}
	e, err := New(Config{Provider: p, Sessions: sm})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	result, err := e.Compact(context.Background(), sessionID, "force llm path")
	lang := i18n.DetectOrLoadLanguage()
	wantSummaryFailure := i18n.Format(lang, i18n.KeyCompactSummaryFailed,
		i18n.Format(lang, i18n.KeyCompactSummaryAPICallFailed, "summary provider unavailable"))
	if err == nil || !strings.Contains(err.Error(), "summary provider unavailable") || !strings.Contains(err.Error(), wantSummaryFailure) {
		t.Fatalf("Compact error = %v, want meaningful compact summary failure", err)
	}
	if result.Compacted || result.BeforeMessageCount != len(msgs) || result.AfterMessageCount != len(msgs) || result.ContextGeneration != 0 {
		t.Fatalf("failed compact result = %#v, want unchanged non-compacted history", result)
	}
	loaded, err := sm.Load(sessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != len(msgs) {
		t.Fatalf("messages changed after failed compact: got %d, want %d", len(loaded), len(msgs))
	}
}

// 8. SetModel must cause subsequent queries to use the new model.
func TestSetModel_ChangesModel(t *testing.T) {
	const sessionID = "model-session"

	var (
		lastModelMu sync.Mutex
		lastModel   string
	)

	p := &mockProvider{
		name:    "mock",
		modelID: "model-v1",
		defaultFn: func(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
			lastModelMu.Lock()
			lastModel = params.Model
			lastModelMu.Unlock()
			return makeTextStreamCh("response"), nil
		},
	}

	e, err := New(Config{Provider: p, Sessions: newMemorySessionManager(), Model: "model-v1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// First query creates the conversation using model-v1.
	ch1, err := e.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "q1"})
	if err != nil {
		t.Fatalf("Query 1: %v", err)
	}
	drainEvents(t, ch1, 5*time.Second)

	// Change the model for this session.
	if err := e.SetModel(sessionID, "model-v2"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	// Second query must use model-v2.
	ch2, err := e.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "q2"})
	if err != nil {
		t.Fatalf("Query 2: %v", err)
	}
	drainEvents(t, ch2, 5*time.Second)

	lastModelMu.Lock()
	got := lastModel
	lastModelMu.Unlock()

	if got != "model-v2" {
		t.Errorf("expected model 'model-v2' for second query, got %q", got)
	}
}

func TestQueryFollowUpWaitsForRunningParentWithoutCancellingIt(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	p.defaultFn = func(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
		p.mu.Lock()
		call := p.callCount
		p.mu.Unlock()
		if call > 1 {
			return makeTextStreamCh("follow-up complete"), nil
		}
		once.Do(func() { close(started) })
		ch := make(chan types.StreamEvent, 4)
		go func() {
			defer close(ch)
			select {
			case <-release:
				for _, event := range textEvents("parent complete") {
					ch <- event
				}
			case <-ctx.Done():
			}
		}()
		return ch, nil
	}
	e, err := New(Config{Provider: p, Sessions: newMemorySessionManager()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	parent, err := e.Query(context.Background(), QueryRequest{SessionID: "follow-up-session", Message: "launch background work"})
	if err != nil {
		t.Fatalf("parent Query: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("parent provider did not start")
	}
	type queryResult struct {
		ch  <-chan Event
		err error
	}
	followUpResult := make(chan queryResult, 1)
	go func() {
		ch, err := e.QueryFollowUp(context.Background(), QueryRequest{SessionID: "follow-up-session", Message: "background result"})
		followUpResult <- queryResult{ch: ch, err: err}
	}()
	select {
	case result := <-followUpResult:
		t.Fatalf("follow-up started before parent completed: err=%v", result.err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	drainEvents(t, parent, time.Second)
	var result queryResult
	select {
	case result = <-followUpResult:
	case <-time.After(time.Second):
		t.Fatal("follow-up did not start after parent completed")
	}
	if result.err != nil {
		t.Fatalf("QueryFollowUp: %v", result.err)
	}
	drainEvents(t, result.ch, time.Second)
	p.mu.Lock()
	callCount := p.callCount
	p.mu.Unlock()
	if callCount != 2 {
		t.Fatalf("provider calls = %d, want parent and follow-up", callCount)
	}
}

// 9. A permission handler that denies all tool calls must produce IsError=true tool-result events.
func TestPermissionHandler_Deny(t *testing.T) {
	const sessionID = "perm-session"

	p := &mockProvider{
		name:    "mock",
		modelID: "mock-model",
		responses: [][]types.StreamEvent{
			// Turn 1: LLM issues a Bash tool call.
			toolCallEvents("call_bash_1", "Bash", map[string]any{"command": "rm -rf /"}),
			// Turn 2: LLM responds after receiving the denied tool result.
			textEvents("I see the tool was denied."),
		},
	}

	e, err := New(Config{
		Provider:   p,
		Sessions:   newMemorySessionManager(),
		Permission: denyAllHandler{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, err := e.Query(context.Background(), QueryRequest{
		SessionID: sessionID,
		Message:   "run a dangerous command",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	events := drainEvents(t, ch, 5*time.Second)

	var sawDeniedToolResult bool
	for _, ev := range events {
		if ev.Inner.Type == stream.EventToolResult &&
			ev.Inner.ToolResult != nil &&
			ev.Inner.ToolResult.IsError {
			sawDeniedToolResult = true
			break
		}
	}

	if !sawDeniedToolResult {
		t.Error("expected a denied tool-result event (ToolResult.IsError=true) but none found")
	}
}

// 10. Shutdown must flush sessions to the manager.
func TestShutdown_FlushesSession(t *testing.T) {
	const sessionID = "flush-session"
	sm := newMemorySessionManager()

	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents("result text")},
	}

	e, err := New(Config{Provider: p, Sessions: sm})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, err := e.Query(context.Background(), QueryRequest{
		SessionID: sessionID,
		Message:   "save me",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	drainEvents(t, ch, 5*time.Second)

	// Session must be present from auto-save after query.
	if _, err := sm.Load(sessionID); err != nil {
		t.Fatalf("Load after query: %v", err)
	}

	// Shutdown triggers an additional flush.
	if err := e.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	msgs, err := sm.Load(sessionID)
	if err != nil {
		t.Fatalf("Load after shutdown: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("expected non-empty message history after shutdown flush")
	}
}

// 11. permission.AllowAllHandler must always return permission.PermissionAllow with no error.
func TestAllowAllHandler(t *testing.T) {
	tests := []struct {
		name string
		req  permission.PermissionRequest
	}{
		{"bash", permission.PermissionRequest{SessionID: "s1", ToolName: "Bash", Input: map[string]any{"command": "ls"}}},
		{"write", permission.PermissionRequest{SessionID: "s2", ToolName: "Write", Input: map[string]any{"path": "/tmp/x"}}},
		{"empty", permission.PermissionRequest{}},
	}

	h := permission.AllowAllHandler{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := h.Check(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if decision != permission.PermissionAllow {
				t.Errorf("expected permission.PermissionAllow, got %v", decision)
			}
		})
	}
}

// 12. fileSessionManager: Save, Load, List, Latest, Delete on a temp directory.
func TestFileSessionManager_CRUD(t *testing.T) {
	dir := t.TempDir()
	sm := newFileSessionManager(dir)

	const (
		id1 = "session-alpha"
		id2 = "session-beta"
	)

	// --- Empty store ---

	infos, err := sm.List()
	if err != nil {
		t.Fatalf("List (empty): %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(infos))
	}

	_, err = sm.Latest()
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Latest on empty store: want ErrSessionNotFound, got %v", err)
	}

	_, err = sm.Load(id1)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Load nonexistent: want ErrSessionNotFound, got %v", err)
	}

	// --- Save and Load id1 ---

	msgs1 := []types.Message{
		types.UserMessage("ping"),
		types.AssistantMessage("pong"),
	}
	if err := sm.Save(id1, msgs1); err != nil {
		t.Fatalf("Save id1: %v", err)
	}

	loaded, err := sm.Load(id1)
	if err != nil {
		t.Fatalf("Load id1: %v", err)
	}
	if len(loaded) != len(msgs1) {
		t.Errorf("Load id1: got %d messages, want %d", len(loaded), len(msgs1))
	}
	if loaded[0].GetText() != "ping" {
		t.Errorf("Load id1 first message: got %q, want %q", loaded[0].GetText(), "ping")
	}

	// --- Save id2, check List returns two entries ---

	msgs2 := []types.Message{types.UserMessage("hello")}
	if err := sm.Save(id2, msgs2); err != nil {
		t.Fatalf("Save id2: %v", err)
	}

	infos, err = sm.List()
	if err != nil {
		t.Fatalf("List (two sessions): %v", err)
	}
	if len(infos) != 2 {
		t.Errorf("expected 2 sessions after two saves, got %d", len(infos))
	}

	// --- Latest must return one of the two IDs ---

	latest, err := sm.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if latest != id1 && latest != id2 {
		t.Errorf("Latest returned unexpected ID: %q", latest)
	}

	// --- Extend id1 in the current generation ---

	updatedMsgs := append(append([]types.Message(nil), msgs1...), types.UserMessage("continued"))
	if err := sm.Save(id1, updatedMsgs); err != nil {
		t.Fatalf("Save id1 (extend): %v", err)
	}
	reloaded, err := sm.Load(id1)
	if err != nil {
		t.Fatalf("Load id1 after overwrite: %v", err)
	}
	if len(reloaded) != 3 || reloaded[2].GetText() != "continued" {
		t.Errorf("extend: unexpected messages: %#v", reloaded)
	}

	// --- Delete id1 ---

	if err := sm.Delete(id1); err != nil {
		t.Fatalf("Delete id1: %v", err)
	}

	_, err = sm.Load(id1)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Load after delete: want ErrSessionNotFound, got %v", err)
	}

	infos, err = sm.List()
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(infos) != 1 {
		t.Errorf("expected 1 session after delete, got %d", len(infos))
	}
	if infos[0].ID != id2 {
		t.Errorf("remaining session ID: want %q, got %q", id2, infos[0].ID)
	}

	// --- Delete nonexistent → ErrSessionNotFound ---

	err = sm.Delete("no-such-session")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Delete nonexistent: want ErrSessionNotFound, got %v", err)
	}
}
