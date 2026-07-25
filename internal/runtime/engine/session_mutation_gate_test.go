package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type observedDoneContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func newObservedDoneContext(ctx context.Context) *observedDoneContext {
	return &observedDoneContext{Context: ctx, observed: make(chan struct{})}
}

func (c *observedDoneContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

type firstSaveBarrierSessionManager struct {
	*memorySessionManager

	mu          sync.Mutex
	saveCount   int
	saveStarted chan struct{}
	releaseSave chan struct{}
}

func newFirstSaveBarrierSessionManager() *firstSaveBarrierSessionManager {
	return &firstSaveBarrierSessionManager{
		memorySessionManager: newMemorySessionManager(),
		saveStarted:          make(chan struct{}),
		releaseSave:          make(chan struct{}),
	}
}

func (m *firstSaveBarrierSessionManager) Save(id string, messages []types.Message) error {
	m.mu.Lock()
	m.saveCount++
	first := m.saveCount == 1
	m.mu.Unlock()
	if first {
		close(m.saveStarted)
		<-m.releaseSave
	}
	return m.memorySessionManager.Save(id, messages)
}

func awaitBarrier(t *testing.T, barrier <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-barrier:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func awaitError(t *testing.T, result <-chan error, name string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
		return nil
	}
}

func seedCompactableSession(t *testing.T, sessions SessionManager, sessionID string) {
	t.Helper()
	messages := make([]types.Message, 0, 24)
	for range 12 {
		messages = append(messages,
			types.UserMessage("history question"),
			types.AssistantMessage("history answer"),
		)
	}
	if err := sessions.Save(sessionID, messages); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func TestSessionMutationGateSerializesManualCompactWithQueryLifecycle(t *testing.T) {
	starters := []struct {
		name  string
		start func(*CoreEngine, context.Context, QueryRequest) (<-chan Event, error)
	}{
		{name: "Query", start: func(e *CoreEngine, ctx context.Context, req QueryRequest) (<-chan Event, error) {
			return e.Query(ctx, req)
		}},
		{name: "QueryFollowUp", start: func(e *CoreEngine, ctx context.Context, req QueryRequest) (<-chan Event, error) {
			return e.QueryFollowUp(ctx, req)
		}},
	}

	for _, starter := range starters {
		t.Run(starter.name, func(t *testing.T) {
			const sessionID = "mutation-gate-session"
			sessions := newFirstSaveBarrierSessionManager()
			seedCompactableSession(t, sessions.memorySessionManager, sessionID)
			compactProviderStarted := make(chan struct{})
			var compactStartedOnce sync.Once
			p := &mockProvider{name: "mock", modelID: "mock-model"}
			p.defaultFn = func(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
				p.mu.Lock()
				call := p.callCount
				p.mu.Unlock()
				if call == 1 {
					return makeTextStreamCh("query complete"), nil
				}
				compactStartedOnce.Do(func() { close(compactProviderStarted) })
				return makeTextStreamCh(`{"schema":"compact-summary/v2","summary":"compact summary"}`), nil
			}

			e, err := New(Config{
				Provider:         p,
				Sessions:         sessions,
				MaxContextTokens: 100_000,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if _, err := e.Resume(context.Background(), sessionID); err != nil {
				t.Fatalf("Resume: %v", err)
			}
			queryEvents, err := starter.start(e, context.Background(), QueryRequest{
				SessionID: sessionID,
				Message:   "hold the mutation lease through save",
			})
			if err != nil {
				t.Fatalf("start query: %v", err)
			}
			awaitBarrier(t, sessions.saveStarted, "query auto-save")

			compactCtx := newObservedDoneContext(context.Background())
			compactResult := make(chan error, 1)
			go func() {
				_, compactErr := e.Compact(compactCtx, sessionID, "force provider compaction")
				compactResult <- compactErr
			}()
			awaitBarrier(t, compactCtx.observed, "manual compact mutation-gate wait")
			select {
			case <-compactProviderStarted:
				t.Fatal("manual compact entered the provider while query auto-save still held the mutation lease")
			default:
			}

			close(sessions.releaseSave)
			drainEvents(t, queryEvents, 5*time.Second)
			if err := awaitError(t, compactResult, "manual compact"); err != nil {
				t.Fatalf("Compact: %v", err)
			}
			awaitBarrier(t, compactProviderStarted, "compact provider start")
		})
	}
}

func TestSessionMutationGateWaitCancellationDoesNotLeakLease(t *testing.T) {
	const sessionID = "mutation-gate-wait-cancel"
	sessions := newFirstSaveBarrierSessionManager()
	seedCompactableSession(t, sessions.memorySessionManager, sessionID)
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	p.defaultFn = func(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
		p.mu.Lock()
		call := p.callCount
		p.mu.Unlock()
		if call == 1 {
			return makeTextStreamCh("complete"), nil
		}
		return makeTextStreamCh(`{"schema":"compact-summary/v2","summary":"compact summary"}`), nil
	}
	e, err := New(Config{Provider: p, Sessions: sessions, MaxContextTokens: 100_000})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	queryEvents, err := e.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "block in save"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	awaitBarrier(t, sessions.saveStarted, "query auto-save")

	baseCtx, cancelCompact := context.WithCancel(context.Background())
	compactCtx := newObservedDoneContext(baseCtx)
	compactResult := make(chan error, 1)
	go func() {
		_, compactErr := e.Compact(compactCtx, sessionID, "cancel while waiting")
		compactResult <- compactErr
	}()
	awaitBarrier(t, compactCtx.observed, "compact mutation-gate wait")
	cancelCompact()
	if err := awaitError(t, compactResult, "cancelled compact"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Compact error = %v, want context.Canceled", err)
	}

	close(sessions.releaseSave)
	drainEvents(t, queryEvents, 5*time.Second)
	if _, err := e.Compact(context.Background(), sessionID, "retry after cancellation"); err != nil {
		t.Fatalf("Compact after cancelled gate wait: %v", err)
	}
}

func TestSessionMutationGateReleasedAfterQueryCancellationAndError(t *testing.T) {
	tests := []struct {
		name       string
		firstCall  func(context.Context, chan<- struct{}) (<-chan types.StreamEvent, error)
		cancel     bool
		wantRunErr bool
	}{
		{
			name: "cancellation",
			firstCall: func(ctx context.Context, started chan<- struct{}) (<-chan types.StreamEvent, error) {
				close(started)
				stream := make(chan types.StreamEvent)
				go func() {
					defer close(stream)
					<-ctx.Done()
				}()
				return stream, nil
			},
			cancel:     true,
			wantRunErr: true,
		},
		{
			name: "provider error",
			firstCall: func(_ context.Context, started chan<- struct{}) (<-chan types.StreamEvent, error) {
				close(started)
				return nil, errors.New("query provider failed")
			},
			wantRunErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const sessionID = "mutation-gate-query-exit"
			started := make(chan struct{})
			sessions := newMemorySessionManager()
			seedCompactableSession(t, sessions, sessionID)
			p := &mockProvider{name: "mock", modelID: "mock-model"}
			p.defaultFn = func(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
				p.mu.Lock()
				call := p.callCount
				p.mu.Unlock()
				if call == 1 {
					return tt.firstCall(ctx, started)
				}
				return makeTextStreamCh(`{"schema":"compact-summary/v2","summary":"compact summary"}`), nil
			}
			e, err := New(Config{
				Provider:         p,
				Sessions:         sessions,
				MaxContextTokens: 100_000,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if _, err := e.Resume(context.Background(), sessionID); err != nil {
				t.Fatalf("Resume: %v", err)
			}
			queryCtx, cancelQuery := context.WithCancel(context.Background())
			queryEvents, err := e.Query(queryCtx, QueryRequest{SessionID: sessionID, Message: "query exits early"})
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			awaitBarrier(t, started, "query provider start")
			if tt.cancel {
				cancelQuery()
			} else {
				defer cancelQuery()
			}
			events := drainEvents(t, queryEvents, 5*time.Second)
			if len(events) == 0 {
				t.Fatal("query produced no terminal event")
			}
			if gotRunErr := events[len(events)-1].Error != nil; gotRunErr != tt.wantRunErr {
				t.Fatalf("terminal query error = %v, want error=%t", events[len(events)-1].Error, tt.wantRunErr)
			}

			if _, err := e.Compact(context.Background(), sessionID, "after query exit"); err != nil {
				t.Fatalf("Compact after query exit: %v", err)
			}
		})
	}
}

func TestSessionMutationGateReleasedAfterManualCompactError(t *testing.T) {
	const sessionID = "mutation-gate-compact-error"
	compactErr := errors.New("compact provider failed")
	sessions := newMemorySessionManager()
	seedCompactableSession(t, sessions, sessionID)
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	p.defaultFn = func(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
		p.mu.Lock()
		call := p.callCount
		p.mu.Unlock()
		switch call {
		case 1:
			return makeTextStreamCh("query complete"), nil
		case 2:
			return nil, compactErr
		default:
			return makeTextStreamCh("query after compact failure"), nil
		}
	}
	e, err := New(Config{
		Provider:         p,
		Sessions:         sessions,
		MaxContextTokens: 100_000,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	queryEvents, err := e.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "establish session"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	drainEvents(t, queryEvents, 5*time.Second)

	if _, err := e.Compact(context.Background(), sessionID, "force compact error"); !errors.Is(err, compactErr) {
		t.Fatalf("Compact error = %v, want %v", err, compactErr)
	}
	queryEvents, err = e.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "run after compact failure"})
	if err != nil {
		t.Fatalf("Query after compact failure: %v", err)
	}
	drainEvents(t, queryEvents, 5*time.Second)
}
