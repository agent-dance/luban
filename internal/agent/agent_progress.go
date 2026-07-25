package agent

// This file implements observer-based progress streaming for sub-agent runs.

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
)

type agentProgressEmitterContextKey struct{}

func withAgentProgressEmitter(ctx context.Context, emitter *AgentProgressEmitter) context.Context {
	if emitter == nil {
		return ctx
	}
	return context.WithValue(ctx, agentProgressEmitterContextKey{}, emitter)
}

func agentProgressEmitterFromContext(ctx context.Context) *AgentProgressEmitter {
	if ctx == nil {
		return nil
	}
	emitter, _ := ctx.Value(agentProgressEmitterContextKey{}).(*AgentProgressEmitter)
	return emitter
}

// AgentProgressEmitter is a thread-safe observer fan-out for sub-agent runs.
// Finish publishes exactly one terminal event. Observers receive events in
// publication order.
type AgentProgressEmitter struct {
	mu              sync.Mutex
	closed          atomic.Bool
	started         time.Time
	agentID         string
	agentTyp        string
	sessionID       string
	turnID          string
	workUnitID      string
	parentToolUseID string
	runID           string
	attempt         int
	batchID         string
	sequence        uint64
	lastEvent       agentcontract.ProgressEvent
	observer        func(agentcontract.ProgressEvent)
	observers       map[uint64]func(agentcontract.ProgressEvent)
	nextObserverID  uint64
}

// ConfigureCorrelation attaches the immutable parent execution identity used
// by presentation consumers to place a child run beside its Agent tool call.
// It is independent of ConfigureRun because retained agents may start several
// runs while keeping the same parent transcript anchor.
func (e *AgentProgressEmitter) ConfigureCorrelation(sessionID, turnID, workUnitID, parentToolUseID string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.sessionID = strings.TrimSpace(sessionID)
	e.turnID = strings.TrimSpace(turnID)
	e.workUnitID = strings.TrimSpace(workUnitID)
	e.parentToolUseID = strings.TrimSpace(parentToolUseID)
	e.mu.Unlock()
}

func (e *AgentProgressEmitter) correlation() (sessionID, turnID, workUnitID, parentToolUseID string) {
	if e == nil {
		return "", "", "", ""
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sessionID, e.turnID, e.workUnitID, e.parentToolUseID
}

// ConfigureRun stamps stable run identity onto all subsequently emitted
// events. It must be called before the run starts publishing progress.
func (e *AgentProgressEmitter) ConfigureRun(runID string, attempt int, batchID string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.runID = strings.TrimSpace(runID)
	e.attempt = attempt
	e.batchID = strings.TrimSpace(batchID)
	e.mu.Unlock()
}

func (e *AgentProgressEmitter) SetObserver(observer func(agentcontract.ProgressEvent)) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.observer = observer
	e.mu.Unlock()
}

// AddObserver registers an independent progress consumer without replacing
// the retained-session observer installed through SetObserver. The returned
// function is idempotent and safe to call while a run is emitting updates.
func (e *AgentProgressEmitter) AddObserver(observer func(agentcontract.ProgressEvent)) func() {
	if e == nil || observer == nil {
		return func() {}
	}
	e.mu.Lock()
	if e.observers == nil {
		e.observers = make(map[uint64]func(agentcontract.ProgressEvent))
	}
	e.nextObserverID++
	id := e.nextObserverID
	e.observers[id] = observer
	e.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			e.mu.Lock()
			delete(e.observers, id)
			e.mu.Unlock()
		})
	}
}

func (e *AgentProgressEmitter) observerSnapshotLocked() []func(agentcontract.ProgressEvent) {
	observers := make([]func(agentcontract.ProgressEvent), 0, len(e.observers)+1)
	if e.observer != nil {
		observers = append(observers, e.observer)
	}
	for _, observer := range e.observers {
		observers = append(observers, observer)
	}
	return observers
}

// newAgentProgressEmitter constructs an emitter. The agentID/agentType fields
// are stamped onto every emitted event when not explicitly overridden.
func newAgentProgressEmitter(agentID, agentType string) *AgentProgressEmitter {
	return &AgentProgressEmitter{
		started:  time.Now(),
		agentID:  agentID,
		agentTyp: agentType,
	}
}

// Emit publishes an event. It returns false only after the emitter is closed.
func (e *AgentProgressEmitter) Emit(evt agentcontract.ProgressEvent) bool {
	if e == nil {
		return false
	}
	if e.closed.Load() {
		return false
	}
	if evt.AgentID == "" {
		evt.AgentID = e.agentID
	}
	if evt.AgentType == "" {
		evt.AgentType = e.agentTyp
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}
	evt.Usage = cloneUsagePointer(evt.Usage)
	evt.LastRequestUsage = cloneUsagePointer(evt.LastRequestUsage)
	if evt.ElapsedMs == 0 && !e.started.IsZero() {
		evt.ElapsedMs = time.Since(e.started).Milliseconds()
	}
	e.mu.Lock()
	if e.closed.Load() {
		e.mu.Unlock()
		return false
	}
	if evt.RunID == "" {
		evt.RunID = e.runID
	}
	if evt.Attempt == 0 {
		evt.Attempt = e.attempt
	}
	if evt.BatchID == "" {
		evt.BatchID = e.batchID
	}
	if evt.SessionID == "" {
		evt.SessionID = e.sessionID
	}
	if evt.TurnID == "" {
		evt.TurnID = e.turnID
	}
	if evt.WorkUnitID == "" {
		evt.WorkUnitID = e.workUnitID
	}
	if evt.ParentToolUseID == "" {
		evt.ParentToolUseID = e.parentToolUseID
	}
	if evt.Provider == "" {
		evt.Provider = e.lastEvent.Provider
	}
	if evt.Model == "" {
		evt.Model = e.lastEvent.Model
	}
	if evt.Usage == nil {
		evt.Usage = cloneUsagePointer(e.lastEvent.Usage)
	}
	if evt.LastRequestUsage == nil {
		evt.LastRequestUsage = cloneUsagePointer(e.lastEvent.LastRequestUsage)
	}
	if evt.TokensUsed == 0 {
		evt.TokensUsed = e.lastEvent.TokensUsed
	}
	e.sequence++
	evt.SourceSequence = e.sequence
	e.lastEvent = evt
	e.lastEvent.Usage = cloneUsagePointer(evt.Usage)
	e.lastEvent.LastRequestUsage = cloneUsagePointer(evt.LastRequestUsage)
	observers := e.observerSnapshotLocked()
	e.mu.Unlock()
	for _, observer := range observers {
		observer(evt)
	}
	return true
}

// EmitPhase emits a single-phase event with the given message count and tool.
func (e *AgentProgressEmitter) EmitPhase(phase agentcontract.ProgressPhase, messageCount int, latestTool string) bool {
	return e.Emit(agentcontract.ProgressEvent{
		Phase:        phase,
		MessageCount: messageCount,
		LatestTool:   latestTool,
	})
}

// Finish emits a terminal phase event. Idempotent. Returns true if it actually
// performed the transition.
func (e *AgentProgressEmitter) Finish(phase agentcontract.ProgressPhase, detail string) bool {
	if e == nil {
		return false
	}
	if !e.closed.CompareAndSwap(false, true) {
		return false
	}
	e.mu.Lock()
	e.sequence++
	terminal := e.lastEvent
	terminal.Usage = cloneUsagePointer(e.lastEvent.Usage)
	terminal.LastRequestUsage = cloneUsagePointer(e.lastEvent.LastRequestUsage)
	terminal.AgentID = e.agentID
	terminal.AgentType = e.agentTyp
	terminal.SessionID = e.sessionID
	terminal.TurnID = e.turnID
	terminal.WorkUnitID = e.workUnitID
	terminal.ParentToolUseID = e.parentToolUseID
	terminal.RunID = e.runID
	terminal.Attempt = e.attempt
	terminal.BatchID = e.batchID
	terminal.SourceSequence = e.sequence
	terminal.Phase = phase
	terminal.Detail = detail
	terminal.Timestamp = time.Now()
	terminal.ElapsedMs = time.Since(e.started).Milliseconds()
	observers := e.observerSnapshotLocked()
	e.mu.Unlock()
	for _, observer := range observers {
		observer(terminal)
	}
	return true
}

// Closed reports whether Finish has been called.
func (e *AgentProgressEmitter) Closed() bool {
	if e == nil {
		return true
	}
	return e.closed.Load()
}
