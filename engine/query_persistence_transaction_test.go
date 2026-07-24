package engine

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/types"
)

type queryPersistenceStarter struct {
	name  string
	start func(*CoreEngine, context.Context, QueryRequest) (<-chan Event, error)
}

var queryPersistenceStarters = []queryPersistenceStarter{
	{name: "Query", start: func(engine *CoreEngine, ctx context.Context, request QueryRequest) (<-chan Event, error) {
		return engine.Query(ctx, request)
	}},
	{name: "QueryFollowUp", start: func(engine *CoreEngine, ctx context.Context, request QueryRequest) (<-chan Event, error) {
		return engine.QueryFollowUp(ctx, request)
	}},
}

func queryPersistenceFinalEvent(t *testing.T, events []Event) Event {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("query stream emitted no events")
	}
	finalCount := 0
	var final Event
	streamed := false
	for _, event := range events {
		if event.Final {
			finalCount++
			final = event
			continue
		}
		if event.Inner.Type == loop.EventText && event.Inner.Text != "" {
			streamed = true
		}
	}
	if !streamed {
		t.Fatal("provider output was not streamed before terminal persistence failure")
	}
	if finalCount != 1 || !events[len(events)-1].Final {
		t.Fatalf("terminal event count/order = %d/%v, want exactly one final event at stream end", finalCount, events[len(events)-1].Final)
	}
	return final
}

func queryPersistenceMessagesText(messages []types.Message) string {
	encoded, _ := json.Marshal(messages)
	return string(encoded)
}

type queryFailNextSaveSessionManager struct {
	*memorySessionManager

	mu        sync.Mutex
	nextError error
}

func (m *queryFailNextSaveSessionManager) Save(sessionID string, messages []types.Message) error {
	m.mu.Lock()
	err := m.nextError
	m.nextError = nil
	m.mu.Unlock()
	if err != nil {
		return err
	}
	return m.memorySessionManager.Save(sessionID, messages)
}

func TestQueryPersistenceSaveFailureRollsBackBeforeNextQuery(t *testing.T) {
	for _, starter := range queryPersistenceStarters {
		t.Run(starter.name, func(t *testing.T) {
			const sessionID = "query-save-rollback"
			initial := []types.Message{
				types.UserMessage("committed seed question"),
				types.AssistantMessage("committed seed answer"),
			}
			base := newMemorySessionManager()
			if err := base.Save(sessionID, initial); err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected query save failure")
			sessions := &queryFailNextSaveSessionManager{memorySessionManager: base}

			var (
				paramsMu   sync.Mutex
				callParams []provider.Params
			)
			mock := &mockProvider{name: "mock", modelID: "mock-model"}
			mock.defaultFn = func(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
				paramsMu.Lock()
				call := len(callParams)
				callParams = append(callParams, params)
				paramsMu.Unlock()
				if call == 0 {
					return makeTextStreamCh("streamed response whose save fails"), nil
				}
				return makeTextStreamCh("second response"), nil
			}

			engine, err := New(Config{Provider: mock, Sessions: sessions})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Resume(context.Background(), sessionID); err != nil {
				t.Fatal(err)
			}
			sessions.mu.Lock()
			sessions.nextError = injected
			sessions.mu.Unlock()

			first, err := starter.start(engine, context.Background(), QueryRequest{SessionID: sessionID, Message: "first turn that cannot persist"})
			if err != nil {
				t.Fatal(err)
			}
			final := queryPersistenceFinalEvent(t, drainEvents(t, first, 5*time.Second))
			if !errors.Is(final.Error, injected) {
				t.Fatalf("terminal error = %v, want injected save failure", final.Error)
			}

			conv := engine.convs[engine.currentConversationKey(sessionID)]
			if got := conv.ql.Messages(); !reflect.DeepEqual(got, initial) {
				t.Fatalf("live view after failed save = %#v, want committed pre-image", got)
			}

			second, err := engine.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "second turn after rollback"})
			if err != nil {
				t.Fatal(err)
			}
			secondEvents := drainEvents(t, second, 5*time.Second)
			if final := secondEvents[len(secondEvents)-1]; !final.Final || final.Error != nil {
				t.Fatalf("second terminal event = %+v, want success", final)
			}
			paramsMu.Lock()
			sampled := queryPersistenceMessagesText(callParams[1].Messages)
			paramsMu.Unlock()
			if !strings.Contains(sampled, "committed seed answer") ||
				strings.Contains(sampled, "first turn that cannot persist") ||
				strings.Contains(sampled, "streamed response whose save fails") {
				t.Fatalf("next query sampled failed live view: %s", sampled)
			}
		})
	}
}

func TestQueryPersistenceExternalGenerationBarrierReloadsBeforeWaitingFollowUp(t *testing.T) {
	for _, starter := range queryPersistenceStarters {
		t.Run(starter.name, func(t *testing.T) {
			const sessionID = "query-stale-generation"
			dir := t.TempDir()
			store := session.NewFileStore(dir)
			initial := []types.Message{
				types.UserMessage("generation one question"),
				types.AssistantMessage("generation one answer"),
			}
			if err := store.Save(sessionID, initial); err != nil {
				t.Fatal(err)
			}
			manifest, err := store.GetCompactionManifest(sessionID)
			if err != nil {
				t.Fatal(err)
			}

			firstProviderStarted := make(chan struct{})
			releaseFirstProvider := make(chan struct{})
			secondProviderStarted := make(chan struct{})
			var (
				paramsMu     sync.Mutex
				providerCall int
				secondParams provider.Params
			)
			mock := &mockProvider{name: "mock", modelID: "mock-model"}
			mock.defaultFn = func(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
				paramsMu.Lock()
				call := providerCall
				providerCall++
				paramsMu.Unlock()
				if call == 0 {
					close(firstProviderStarted)
					<-releaseFirstProvider
					return makeTextStreamCh("stale streamed response"), nil
				}
				paramsMu.Lock()
				secondParams = params
				paramsMu.Unlock()
				close(secondProviderStarted)
				return makeTextStreamCh("waiting follow-up response"), nil
			}

			engine, err := New(Config{Provider: mock, Sessions: newFileSessionManager(dir)})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Resume(context.Background(), sessionID); err != nil {
				t.Fatal(err)
			}

			first, err := starter.start(engine, context.Background(), QueryRequest{SessionID: sessionID, Message: "turn prepared on generation one"})
			if err != nil {
				t.Fatal(err)
			}
			awaitBarrier(t, firstProviderStarted, "first query provider")

			external := append(append([]types.Message(nil), initial...), types.AssistantMessage("authoritative external generation two"))
			if _, err := store.CommitModelContext(sessionID, manifest.ContextGeneration, external, []types.Message{external[len(external)-1]}); err != nil {
				t.Fatal(err)
			}

			type queryStartResult struct {
				events <-chan Event
				err    error
			}
			waitingResult := make(chan queryStartResult, 1)
			waitingCtx := newObservedDoneContext(context.Background())
			go func() {
				events, queryErr := engine.QueryFollowUp(waitingCtx, QueryRequest{SessionID: sessionID, Message: "wait behind stale save"})
				waitingResult <- queryStartResult{events: events, err: queryErr}
			}()
			awaitBarrier(t, waitingCtx.observed, "waiting follow-up query gate")
			select {
			case <-secondProviderStarted:
				t.Fatal("waiting follow-up crossed the first query persistence barrier")
			default:
			}

			close(releaseFirstProvider)
			firstFinal := queryPersistenceFinalEvent(t, drainEvents(t, first, 5*time.Second))
			if !errors.Is(firstFinal.Error, session.ErrStaleContextGeneration) {
				t.Fatalf("first terminal error = %v, want stale generation", firstFinal.Error)
			}

			var waiting queryStartResult
			select {
			case waiting = <-waitingResult:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for follow-up to cross persistence barrier")
			}
			if waiting.err != nil {
				t.Fatal(waiting.err)
			}
			waitingEvents := drainEvents(t, waiting.events, 5*time.Second)
			if final := waitingEvents[len(waitingEvents)-1]; !final.Final || final.Error != nil {
				t.Fatalf("waiting follow-up terminal event = %+v, want success", final)
			}
			awaitBarrier(t, secondProviderStarted, "waiting follow-up provider")
			paramsMu.Lock()
			sampled := queryPersistenceMessagesText(secondParams.Messages)
			paramsMu.Unlock()
			if !strings.Contains(sampled, "authoritative external generation two") ||
				strings.Contains(sampled, "turn prepared on generation one") ||
				strings.Contains(sampled, "stale streamed response") {
				t.Fatalf("waiting follow-up sampled non-authoritative state: %s", sampled)
			}
		})
	}
}

type querySidecarFailureSessionManager struct {
	*memorySessionManager

	mu                  sync.Mutex
	ledger              []string
	failNextLedgerSave  error
	failNextLedgerLoad  error
	blockNextLedgerLoad bool
	loadStarted         chan struct{}
	releaseLoad         chan struct{}
}

func (m *querySidecarFailureSessionManager) SaveToolUseLedger(_ string, ledger []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.failNextLedgerSave; err != nil {
		m.failNextLedgerSave = nil
		return err
	}
	m.ledger = append([]string(nil), ledger...)
	return nil
}

func (m *querySidecarFailureSessionManager) LoadToolUseLedger(_ string) ([]string, error) {
	m.mu.Lock()
	if err := m.failNextLedgerLoad; err != nil {
		m.failNextLedgerLoad = nil
		m.mu.Unlock()
		return nil, err
	}
	if m.blockNextLedgerLoad {
		m.blockNextLedgerLoad = false
		started := m.loadStarted
		release := m.releaseLoad
		m.mu.Unlock()
		close(started)
		<-release
		m.mu.Lock()
	}
	ledger := append([]string(nil), m.ledger...)
	m.mu.Unlock()
	return ledger, nil
}

func TestQueryPersistenceSidecarFailureInvalidatesUntilAuthoritativeReload(t *testing.T) {
	for _, starter := range queryPersistenceStarters {
		t.Run(starter.name, func(t *testing.T) {
			const sessionID = "query-sidecar-reload"
			initial := []types.Message{
				types.UserMessage("sidecar seed question"),
				types.AssistantMessage("sidecar seed answer"),
			}
			base := newMemorySessionManager()
			if err := base.Save(sessionID, initial); err != nil {
				t.Fatal(err)
			}
			sessions := &querySidecarFailureSessionManager{
				memorySessionManager: base,
				loadStarted:          make(chan struct{}),
				releaseLoad:          make(chan struct{}),
			}
			sidecarErr := errors.New("injected post-context sidecar failure")
			reloadErr := errors.New("injected authoritative reload failure")

			var (
				paramsMu     sync.Mutex
				providerCall int
				secondParams provider.Params
			)
			mock := &mockProvider{name: "mock", modelID: "mock-model"}
			mock.defaultFn = func(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
				paramsMu.Lock()
				call := providerCall
				providerCall++
				if call == 1 {
					secondParams = params
				}
				paramsMu.Unlock()
				if call == 0 {
					return makeTextStreamCh("context committed before sidecar failure"), nil
				}
				return makeTextStreamCh("query after authoritative reload"), nil
			}

			engine, err := New(Config{Provider: mock, Sessions: sessions})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Resume(context.Background(), sessionID); err != nil {
				t.Fatal(err)
			}
			sessions.mu.Lock()
			sessions.failNextLedgerSave = sidecarErr
			sessions.failNextLedgerLoad = reloadErr
			sessions.blockNextLedgerLoad = true
			sessions.mu.Unlock()

			first, err := starter.start(engine, context.Background(), QueryRequest{SessionID: sessionID, Message: "context commit survives sidecar failure"})
			if err != nil {
				t.Fatal(err)
			}
			firstFinal := queryPersistenceFinalEvent(t, drainEvents(t, first, 5*time.Second))
			if !errors.Is(firstFinal.Error, sidecarErr) || !errors.Is(firstFinal.Error, reloadErr) {
				t.Fatalf("terminal error = %v, want sidecar and reload failures", firstFinal.Error)
			}

			conv := engine.convs[engine.currentConversationKey(sessionID)]
			conv.mu.Lock()
			reloadRequired := conv.authoritativeReloadRequired
			conv.mu.Unlock()
			if !reloadRequired || !reflect.DeepEqual(conv.ql.Messages(), initial) {
				t.Fatalf("failed reconciliation did not restore and invalidate live state: required=%v messages=%#v", reloadRequired, conv.ql.Messages())
			}

			type queryStartResult struct {
				events <-chan Event
				err    error
			}
			secondResult := make(chan queryStartResult, 1)
			go func() {
				events, queryErr := engine.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "query waits for authoritative reload"})
				secondResult <- queryStartResult{events: events, err: queryErr}
			}()
			awaitBarrier(t, sessions.loadStarted, "authoritative reload")
			paramsMu.Lock()
			callsWhileBlocked := providerCall
			paramsMu.Unlock()
			if callsWhileBlocked != 1 {
				t.Fatalf("provider calls while authoritative reload blocked = %d, want 1", callsWhileBlocked)
			}

			close(sessions.releaseLoad)
			var second queryStartResult
			select {
			case second = <-secondResult:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for query after authoritative reload")
			}
			if second.err != nil {
				t.Fatal(second.err)
			}
			secondEvents := drainEvents(t, second.events, 5*time.Second)
			if final := secondEvents[len(secondEvents)-1]; !final.Final || final.Error != nil {
				t.Fatalf("second terminal event = %+v, want success", final)
			}
			paramsMu.Lock()
			sampled := queryPersistenceMessagesText(secondParams.Messages)
			paramsMu.Unlock()
			if !strings.Contains(sampled, "context commit survives sidecar failure") ||
				!strings.Contains(sampled, "context committed before sidecar failure") {
				t.Fatalf("query did not reload committed post-CAS context: %s", sampled)
			}
		})
	}
}
