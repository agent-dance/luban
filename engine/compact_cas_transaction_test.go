package engine

import (
	"context"
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

type failNextSaveSessionManager struct {
	*memorySessionManager

	failureMu sync.Mutex
	nextError error
}

func (m *failNextSaveSessionManager) Save(sessionID string, messages []types.Message) error {
	m.failureMu.Lock()
	err := m.nextError
	m.nextError = nil
	m.failureMu.Unlock()
	if err != nil {
		return err
	}
	return m.memorySessionManager.Save(sessionID, messages)
}

func compactCASTestMessages(marker string) []types.Message {
	messages := make([]types.Message, 0, 24)
	for range 12 {
		messages = append(messages,
			types.UserMessage(marker+" question"),
			types.AssistantMessage(marker+" answer"),
		)
	}
	return messages
}

func compactCASTestEvents(events []loop.Event) (boundaries int, terminalStages []string) {
	for _, event := range events {
		if event.Type == loop.EventCompactBoundary {
			boundaries++
		}
		if event.Type != loop.EventProgress || event.Progress == nil {
			continue
		}
		switch event.Progress.Stage {
		case "compact_end", "compact_failed", "compact_cancelled":
			terminalStages = append(terminalStages, event.Progress.Stage)
		}
	}
	return boundaries, terminalStages
}

func requireCompactPersistenceFailureEvents(t *testing.T, events []loop.Event, privateCauses ...error) {
	t.Helper()
	boundaries, terminalStages := compactCASTestEvents(events)
	if boundaries != 0 || !reflect.DeepEqual(terminalStages, []string{"compact_failed"}) {
		t.Fatalf("failed compact publication boundaries/terminals = %d/%v, want 0/[compact_failed]; events=%+v", boundaries, terminalStages, events)
	}
	for _, event := range events {
		if event.Type != loop.EventProgress || event.Progress == nil || event.Progress.Stage != "compact_failed" {
			continue
		}
		publicDiagnostic, _ := event.Progress.Metadata["error"].(string)
		if strings.TrimSpace(publicDiagnostic) == "" {
			t.Fatalf("failed compact terminal omitted semantic public diagnostic: %+v", event)
		}
		for _, cause := range privateCauses {
			if cause != nil && strings.Contains(publicDiagnostic, cause.Error()) {
				t.Fatalf("failed compact terminal leaked private cause %q: %+v", cause, event)
			}
		}
	}
}

func TestManualCompactSaveFailureRestoresDurableViewBeforeNextQuery(t *testing.T) {
	const sessionID = "compact-save-rollback"
	initial := compactCASTestMessages("committed original")
	base := newMemorySessionManager()
	if err := base.Save(sessionID, initial); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected save failure")
	sessions := &failNextSaveSessionManager{memorySessionManager: base}

	var (
		callsMu      sync.Mutex
		providerCall int
		queryParams  provider.Params
	)
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	p.defaultFn = func(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
		callsMu.Lock()
		call := providerCall
		providerCall++
		if call == 1 {
			queryParams = params
		}
		callsMu.Unlock()
		if call == 0 {
			return makeTextStreamCh(`{"schema":"compact-summary/v2","summary":"uncommitted compact view"}`), nil
		}
		return makeTextStreamCh("query completed"), nil
	}

	e, err := New(Config{Provider: p, Sessions: sessions, MaxContextTokens: 100_000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	sessions.failureMu.Lock()
	sessions.nextError = injected
	sessions.failureMu.Unlock()

	var compactEvents []loop.Event
	if err := e.CompactWithEvents(context.Background(), sessionID, "", func(event loop.Event) {
		compactEvents = append(compactEvents, event)
	}); !errors.Is(err, injected) {
		t.Fatalf("Compact error = %v, want injected save failure", err)
	}
	requireCompactPersistenceFailureEvents(t, compactEvents, injected)
	conv := e.convs[e.currentConversationKey(sessionID)]
	if got := conv.ql.Messages(); !reflect.DeepEqual(got, initial) {
		t.Fatalf("live view after failed save = %#v, want durable pre-image", got)
	}

	events, err := e.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "query after failed compact"})
	if err != nil {
		t.Fatal(err)
	}
	drainEvents(t, events, 5*time.Second)
	callsMu.Lock()
	sampled := task26EngineMessagesText(queryParams.Messages)
	callsMu.Unlock()
	if !strings.Contains(sampled, "committed original") || strings.Contains(sampled, "uncommitted compact view") {
		t.Fatalf("query sampled a non-authoritative view: %s", sampled)
	}
}

func TestManualCompactCleanupFailureRollsBackBeforePersistenceAndNextQuery(t *testing.T) {
	const sessionID = "compact-cleanup-rollback"
	initial := compactCASTestMessages("cleanup committed source")
	sessions := newMemorySessionManager()
	if err := sessions.Save(sessionID, initial); err != nil {
		t.Fatal(err)
	}
	cleanupErr := errors.New("private cleanup failure")
	var (
		callsMu      sync.Mutex
		providerCall int
		queryParams  provider.Params
	)
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	p.defaultFn = func(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
		callsMu.Lock()
		call := providerCall
		providerCall++
		if call == 1 {
			queryParams = params
		}
		callsMu.Unlock()
		if call == 0 {
			return makeTextStreamCh(`{"schema":"compact-summary/v2","summary":"cleanup replacement summary"}`), nil
		}
		return makeTextStreamCh("query after cleanup rollback"), nil
	}
	e, err := New(Config{Provider: p, Sessions: sessions, MaxContextTokens: 100_000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	conv := e.convs[e.currentConversationKey(sessionID)]
	cleanupCalls := 0
	rollbackLoop := loop.New(p, nil, loop.Config{
		MaxContextTokens: 100_000,
		SessionID:        sessionID,
		PostCompactCleanup: func(context.Context) error {
			cleanupCalls++
			return cleanupErr
		},
	})
	rollbackLoop.SetMessages(initial)
	conv.ql = rollbackLoop

	var compactEvents []loop.Event
	compactErr := e.CompactWithEvents(context.Background(), sessionID, "", func(event loop.Event) {
		compactEvents = append(compactEvents, event)
	})
	if !errors.Is(compactErr, cleanupErr) {
		t.Fatalf("Compact error = %v, want cleanup failure", compactErr)
	}
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
	}
	requireCompactPersistenceFailureEvents(t, compactEvents, cleanupErr)
	if got := conv.ql.Messages(); !reflect.DeepEqual(got, initial) {
		t.Fatalf("live view after cleanup failure = %#v, want pre-image %#v", got, initial)
	}
	if sessions.saveCalls != 1 {
		t.Fatalf("session save calls after pre-CAS cleanup failure = %d, want seed save only", sessions.saveCalls)
	}

	events, err := e.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "query after cleanup failure"})
	if err != nil {
		t.Fatal(err)
	}
	drainEvents(t, events, 5*time.Second)
	callsMu.Lock()
	sampled := task26EngineMessagesText(queryParams.Messages)
	callsMu.Unlock()
	if !strings.Contains(sampled, "cleanup committed source") || strings.Contains(sampled, "cleanup replacement summary") {
		t.Fatalf("next provider sampled cleanup-failed replacement: %s", sampled)
	}
}

func TestManualCompactStaleCASReloadsExternalGenerationBeforeWaitingQuery(t *testing.T) {
	const sessionID = "compact-stale-generation"
	dir := t.TempDir()
	store := session.NewFileStore(dir)
	initial := compactCASTestMessages("generation one")
	if err := store.Save(sessionID, initial); err != nil {
		t.Fatal(err)
	}
	manifest, err := store.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	sessions := newFileSessionManager(dir)

	compactProviderStarted := make(chan struct{})
	releaseCompactProvider := make(chan struct{})
	queryProviderStarted := make(chan struct{})
	var (
		callsMu      sync.Mutex
		providerCall int
		queryParams  provider.Params
	)
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	p.defaultFn = func(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
		callsMu.Lock()
		call := providerCall
		providerCall++
		callsMu.Unlock()
		switch call {
		case 0:
			close(compactProviderStarted)
			<-releaseCompactProvider
			return makeTextStreamCh(`{"schema":"compact-summary/v2","summary":"stale uncommitted summary"}`), nil
		default:
			callsMu.Lock()
			queryParams = params
			callsMu.Unlock()
			close(queryProviderStarted)
			return makeTextStreamCh("query completed"), nil
		}
	}

	e, err := New(Config{Provider: p, Sessions: sessions, MaxContextTokens: 100_000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}

	compactResult := make(chan error, 1)
	var compactEvents []loop.Event
	go func() {
		compactResult <- e.CompactWithEvents(context.Background(), sessionID, "", func(event loop.Event) {
			compactEvents = append(compactEvents, event)
		})
	}()
	awaitBarrier(t, compactProviderStarted, "compact provider")

	external := append(append([]types.Message(nil), initial...), types.AssistantMessage("external generation two"))
	if _, err := store.CommitModelContext(sessionID, manifest.ContextGeneration, external, []types.Message{external[len(external)-1]}); err != nil {
		t.Fatal(err)
	}

	type queryStartResult struct {
		events <-chan Event
		err    error
	}
	queryResult := make(chan queryStartResult, 1)
	queryCtx := newObservedDoneContext(context.Background())
	go func() {
		events, queryErr := e.Query(queryCtx, QueryRequest{SessionID: sessionID, Message: "query after external generation"})
		queryResult <- queryStartResult{events: events, err: queryErr}
	}()
	awaitBarrier(t, queryCtx.observed, "query mutation barrier")
	select {
	case <-queryProviderStarted:
		t.Fatal("query crossed the compact persistence barrier")
	default:
	}

	close(releaseCompactProvider)
	if err := awaitError(t, compactResult, "stale compact"); !errors.Is(err, session.ErrStaleContextGeneration) {
		t.Fatalf("Compact error = %v, want ErrStaleContextGeneration", err)
	}
	requireCompactPersistenceFailureEvents(t, compactEvents, session.ErrStaleContextGeneration)

	var started queryStartResult
	select {
	case started = <-queryResult:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for query to cross mutation barrier")
	}
	if started.err != nil {
		t.Fatal(started.err)
	}
	drainEvents(t, started.events, 5*time.Second)
	awaitBarrier(t, queryProviderStarted, "query provider")
	callsMu.Lock()
	sampled := task26EngineMessagesText(queryParams.Messages)
	callsMu.Unlock()
	if !strings.Contains(sampled, "external generation two") || strings.Contains(sampled, "stale uncommitted summary") {
		t.Fatalf("query did not sample the authoritative external generation: %s", sampled)
	}
}

func TestManualCompactSuccessfulSavePublishesCompactedLiveView(t *testing.T) {
	const sessionID = "compact-success-transaction"
	sessions := newMemorySessionManager()
	if err := sessions.Save(sessionID, compactCASTestMessages("success source")); err != nil {
		t.Fatal(err)
	}
	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents(`{"schema":"compact-summary/v2","summary":"committed compact summary"}`)},
	}
	e, err := New(Config{Provider: p, Sessions: sessions, MaxContextTokens: 100_000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	if err := e.Compact(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}

	conv := e.convs[e.currentConversationKey(sessionID)]
	live := task26EngineMessagesText(conv.ql.Messages())
	persisted, err := sessions.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if durable := task26EngineMessagesText(persisted); !strings.Contains(live, "committed compact summary") || live != durable {
		t.Fatalf("successful compact live/durable mismatch: live=%s durable=%s", live, durable)
	}
}

type blockingCompactFileSessionManager struct {
	*fileSessionManager

	mu      sync.Mutex
	block   bool
	started chan struct{}
	release chan struct{}
}

func (m *blockingCompactFileSessionManager) Save(sessionID string, messages []types.Message) error {
	m.mu.Lock()
	block := m.block
	if block {
		m.block = false
	}
	started, release := m.started, m.release
	m.mu.Unlock()
	if block {
		close(started)
		<-release
	}
	return m.fileSessionManager.Save(sessionID, messages)
}

func TestManualCompactBuffersLifecycleUntilGenerationCommit(t *testing.T) {
	const sessionID = "compact-event-generation-boundary"
	dir := t.TempDir()
	base := newFileSessionManager(dir)
	if err := base.store.Save(sessionID, compactCASTestMessages("generation source")); err != nil {
		t.Fatal(err)
	}
	sessions := &blockingCompactFileSessionManager{
		fileSessionManager: base,
		started:            make(chan struct{}),
		release:            make(chan struct{}),
	}
	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents(`{"schema":"compact-summary/v2","summary":"generation committed summary"}`)},
	}
	e, err := New(Config{Provider: p, Sessions: sessions, MaxContextTokens: 100_000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	beforeGeneration, err := e.ContextGeneration(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	sessions.mu.Lock()
	sessions.block = true
	sessions.mu.Unlock()

	var (
		eventsMu           sync.Mutex
		compactEvents      []loop.Event
		boundaryGeneration uint64
		callbackErr        error
	)
	compactResult := make(chan error, 1)
	go func() {
		compactResult <- e.CompactWithEvents(context.Background(), sessionID, "", func(event loop.Event) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			compactEvents = append(compactEvents, event)
			if event.Type == loop.EventCompactBoundary {
				boundaryGeneration, callbackErr = e.ContextGeneration(sessionID)
			}
		})
	}()
	awaitBarrier(t, sessions.started, "manual compact durable save")
	eventsMu.Lock()
	eventsBeforeCommit := len(compactEvents)
	eventsMu.Unlock()
	if eventsBeforeCommit != 0 {
		t.Fatalf("manual compact published %d events before durable generation commit", eventsBeforeCommit)
	}
	close(sessions.release)
	if err := awaitError(t, compactResult, "manual compact generation commit"); err != nil {
		t.Fatal(err)
	}

	eventsMu.Lock()
	events := append([]loop.Event(nil), compactEvents...)
	observedGeneration, observedErr := boundaryGeneration, callbackErr
	eventsMu.Unlock()
	boundaries, terminalStages := compactCASTestEvents(events)
	if boundaries != 1 || !reflect.DeepEqual(terminalStages, []string{"compact_end"}) {
		t.Fatalf("successful compact publication boundaries/terminals = %d/%v, want 1/[compact_end]; events=%+v", boundaries, terminalStages, events)
	}
	if observedErr != nil || observedGeneration != beforeGeneration+1 {
		t.Fatalf("boundary callback generation = %d, %v; want committed generation %d", observedGeneration, observedErr, beforeGeneration+1)
	}
}

func TestManualCompactGenerationAwareNoopKeepsCommittedGeneration(t *testing.T) {
	const sessionID = "compact-generation-aware-noop"
	dir := t.TempDir()
	sessions := newFileSessionManager(dir)
	initial := []types.Message{
		types.UserMessage("short history question"),
		types.AssistantMessage("short history answer"),
	}
	if err := sessions.store.Save(sessionID, initial); err != nil {
		t.Fatal(err)
	}
	p := &mockProvider{name: "mock", modelID: "mock-model"}
	e, err := New(Config{Provider: p, Sessions: sessions, MaxContextTokens: 100_000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	beforeGeneration, err := e.ContextGeneration(sessionID)
	if err != nil {
		t.Fatal(err)
	}

	var compactEvents []loop.Event
	if err := e.CompactWithEvents(context.Background(), sessionID, "", func(event loop.Event) {
		compactEvents = append(compactEvents, event)
	}); err != nil {
		t.Fatalf("generation-aware no-op compact: %v", err)
	}
	afterGeneration, err := e.ContextGeneration(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if afterGeneration != beforeGeneration {
		t.Fatalf("no-op compact generation = %d, want unchanged %d", afterGeneration, beforeGeneration)
	}
	boundaries, terminalStages := compactCASTestEvents(compactEvents)
	if boundaries != 0 || !reflect.DeepEqual(terminalStages, []string{"compact_end"}) {
		t.Fatalf("no-op compact publication boundaries/terminals = %d/%v, want 0/[compact_end]; events=%+v", boundaries, terminalStages, compactEvents)
	}
	if got := e.convs[e.currentConversationKey(sessionID)].ql.Messages(); !reflect.DeepEqual(got, initial) {
		t.Fatalf("no-op compact changed visible history: got=%#v want=%#v", got, initial)
	}
	p.mu.Lock()
	providerCalls := p.callCount
	p.mu.Unlock()
	if providerCalls != 0 {
		t.Fatalf("no-op compact provider calls = %d, want 0", providerCalls)
	}
}

type postCASCompactSessionManager struct {
	*memorySessionManager

	mu             sync.Mutex
	generation     uint64
	failNextSave   error
	failNextReload error
}

func (m *postCASCompactSessionManager) Save(sessionID string, messages []types.Message) error {
	if err := m.memorySessionManager.Save(sessionID, messages); err != nil {
		return err
	}
	m.mu.Lock()
	m.generation++
	generation := m.generation
	failure := m.failNextSave
	m.failNextSave = nil
	m.mu.Unlock()
	if failure != nil {
		return &session.ContextCommitError{
			Manifest: session.CompactionManifestV2{ContextGeneration: generation},
			Cause:    failure,
		}
	}
	return nil
}

func (m *postCASCompactSessionManager) Load(sessionID string) ([]types.Message, error) {
	m.mu.Lock()
	failure := m.failNextReload
	m.failNextReload = nil
	m.mu.Unlock()
	if failure != nil {
		return nil, failure
	}
	return m.memorySessionManager.Load(sessionID)
}

func (m *postCASCompactSessionManager) contextGenerationState(string, string) (ContextGenerationState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return ContextGenerationState{Generation: m.generation, Persisted: m.generation > 0}, nil
}

func TestManualCompactPostCASFailureNeverPublishesSuccessBoundary(t *testing.T) {
	for _, reloadFails := range []bool{false, true} {
		name := "reload_succeeds"
		if reloadFails {
			name = "reload_fails"
		}
		t.Run(name, func(t *testing.T) {
			const sessionID = "compact-post-cas-failure"
			base := newMemorySessionManager()
			if err := base.Save(sessionID, compactCASTestMessages("post-CAS source")); err != nil {
				t.Fatal(err)
			}
			sessions := &postCASCompactSessionManager{memorySessionManager: base, generation: 1}
			p := &mockProvider{
				name:      "mock",
				modelID:   "mock-model",
				responses: [][]types.StreamEvent{textEvents(`{"schema":"compact-summary/v2","summary":"post-CAS committed summary"}`)},
			}
			e, err := New(Config{Provider: p, Sessions: sessions, MaxContextTokens: 100_000})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Resume(context.Background(), sessionID); err != nil {
				t.Fatal(err)
			}
			postCASErr := errors.New("private post-CAS failure")
			var reloadErr error
			if reloadFails {
				reloadErr = errors.New("private authoritative reload failure")
			}
			sessions.mu.Lock()
			sessions.failNextSave = postCASErr
			sessions.failNextReload = reloadErr
			sessions.mu.Unlock()

			var compactEvents []loop.Event
			compactErr := e.CompactWithEvents(context.Background(), sessionID, "", func(event loop.Event) {
				compactEvents = append(compactEvents, event)
			})
			if !errors.Is(compactErr, postCASErr) || reloadFails && !errors.Is(compactErr, reloadErr) {
				t.Fatalf("Compact error = %v, want post-CAS/reload causes", compactErr)
			}
			requireCompactPersistenceFailureEvents(t, compactEvents, postCASErr, reloadErr)

			conv := e.convs[e.currentConversationKey(sessionID)]
			conv.mu.Lock()
			reloadRequired := conv.authoritativeReloadRequired
			conv.mu.Unlock()
			if reloadRequired != reloadFails {
				t.Fatalf("authoritative reload required = %t, want %t", reloadRequired, reloadFails)
			}
			live := task26EngineMessagesText(conv.ql.Messages())
			if reloadFails && strings.Contains(live, "post-CAS committed summary") {
				t.Fatalf("failed reload exposed post-CAS replacement as live fallback: %s", live)
			}
			if !reloadFails && !strings.Contains(live, "post-CAS committed summary") {
				t.Fatalf("successful authoritative reload omitted committed post-CAS view: %s", live)
			}
		})
	}
}

type nonAdvancingCompactGenerationManager struct {
	*memorySessionManager
	state ContextGenerationState
}

func (m *nonAdvancingCompactGenerationManager) contextGenerationState(string, string) (ContextGenerationState, error) {
	return m.state, nil
}

func TestManualCompactRequiresCommittedGenerationBeforeBoundaryPublication(t *testing.T) {
	const sessionID = "compact-generation-did-not-commit"
	base := newMemorySessionManager()
	if err := base.Save(sessionID, compactCASTestMessages("unadvanced source")); err != nil {
		t.Fatal(err)
	}
	sessions := &nonAdvancingCompactGenerationManager{
		memorySessionManager: base,
		state:                ContextGenerationState{Generation: 7, Persisted: true},
	}
	p := &mockProvider{
		name:      "mock",
		modelID:   "mock-model",
		responses: [][]types.StreamEvent{textEvents(`{"schema":"compact-summary/v2","summary":"uncommitted generation summary"}`)},
	}
	e, err := New(Config{Provider: p, Sessions: sessions, MaxContextTokens: 100_000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	var compactEvents []loop.Event
	compactErr := e.CompactWithEvents(context.Background(), sessionID, "", func(event loop.Event) {
		compactEvents = append(compactEvents, event)
	})
	if !errors.Is(compactErr, errManualCompactionGenerationNotCommitted) {
		t.Fatalf("Compact error = %v, want generation-not-committed", compactErr)
	}
	requireCompactPersistenceFailureEvents(t, compactEvents, errManualCompactionGenerationNotCommitted)
}
