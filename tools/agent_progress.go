package tools

// agent_progress.go implements channel-based progress streaming for sub-agent
// runs (subtask agent-04 in tasks/agent.json). The TS reference emits
// 'agent-progress' events from runSubagent.ts after every assistant turn so
// the parent harness can render running totals and the latest tool use.

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/types"
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

// AgentProgressPhase enumerates the canonical lifecycle phases of an agent run.
type AgentProgressPhase string

const (
	AgentPhaseStart        AgentProgressPhase = "start"
	AgentPhaseMCPReady     AgentProgressPhase = "mcp_ready"
	AgentPhaseRunning      AgentProgressPhase = "running"
	AgentPhaseToolUse      AgentProgressPhase = "tool_use"
	AgentPhaseAssistant    AgentProgressPhase = "assistant"
	AgentPhaseCompleted    AgentProgressPhase = "completed"
	AgentPhaseError        AgentProgressPhase = "error"
	AgentPhaseAborted      AgentProgressPhase = "aborted"
	AgentPhaseBackground   AgentProgressPhase = "background"
	AgentPhaseRemoteLaunch AgentProgressPhase = "remote_launched"
)

// AgentProgressEvent mirrors the TS 'agent-progress' message payload.
type AgentProgressEvent struct {
	AgentID          string             `json:"agentId,omitempty"`
	AgentType        string             `json:"agentType,omitempty"`
	SessionID        string             `json:"sessionId,omitempty"`
	TurnID           string             `json:"turnId,omitempty"`
	WorkUnitID       string             `json:"workUnitId,omitempty"`
	ParentToolUseID  string             `json:"parentToolUseId,omitempty"`
	RunID            string             `json:"runId,omitempty"`
	Attempt          int                `json:"attempt,omitempty"`
	BatchID          string             `json:"batchId,omitempty"`
	SourceSequence   uint64             `json:"sourceSequence,omitempty"`
	DroppedCount     uint64             `json:"droppedCount,omitempty"`
	Phase            AgentProgressPhase `json:"phase"`
	MessageCount     int                `json:"messageCount"`
	LatestTool       string             `json:"latestTool,omitempty"`
	PartialText      string             `json:"partialText,omitempty"`
	ElapsedMs        int64              `json:"elapsedMs"`
	TokensUsed       int                `json:"tokensUsed"`
	Provider         string             `json:"provider,omitempty"`
	Model            string             `json:"model,omitempty"`
	Usage            *types.Usage       `json:"usage,omitempty"`
	LastRequestUsage *types.Usage       `json:"lastRequestUsage,omitempty"`
	Detail           string             `json:"detail,omitempty"`
	Timestamp        time.Time          `json:"timestamp"`
}

// AgentProgressEmitter is a buffered, thread-safe channel-based emitter that
// guarantees the channel is closed exactly once on Finish. Listeners receive
// events in publication order. Drop-on-full semantics keep slow listeners from
// blocking the agent loop.
type AgentProgressEmitter struct {
	mu              sync.Mutex
	ch              chan AgentProgressEvent
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
	dropped         uint64
	lastEvent       AgentProgressEvent
	observer        func(AgentProgressEvent)
	observers       map[uint64]func(AgentProgressEvent)
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

func (e *AgentProgressEmitter) SetObserver(observer func(AgentProgressEvent)) {
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
func (e *AgentProgressEmitter) AddObserver(observer func(AgentProgressEvent)) func() {
	if e == nil || observer == nil {
		return func() {}
	}
	e.mu.Lock()
	if e.observers == nil {
		e.observers = make(map[uint64]func(AgentProgressEvent))
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

func (e *AgentProgressEmitter) observerSnapshotLocked() []func(AgentProgressEvent) {
	observers := make([]func(AgentProgressEvent), 0, len(e.observers)+1)
	if e.observer != nil {
		observers = append(observers, e.observer)
	}
	for _, observer := range e.observers {
		observers = append(observers, observer)
	}
	return observers
}

// NewAgentProgressEmitter constructs a new emitter with a backlog of `buffer`
// events. The agentID/agentType fields are stamped onto every emitted event
// when not explicitly overridden.
func NewAgentProgressEmitter(agentID, agentType string, buffer int) *AgentProgressEmitter {
	if buffer <= 0 {
		buffer = 16
	}
	return &AgentProgressEmitter{
		ch:       make(chan AgentProgressEvent, buffer),
		started:  time.Now(),
		agentID:  agentID,
		agentTyp: agentType,
	}
}

// Channel returns the receive-only channel for consumers.
func (e *AgentProgressEmitter) Channel() <-chan AgentProgressEvent {
	if e == nil {
		return nil
	}
	return e.ch
}

// Emit publishes an event. Returns false if the emitter is closed or full
// (drop-on-full keeps the agent loop non-blocking).
func (e *AgentProgressEmitter) Emit(evt AgentProgressEvent) bool {
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
	evt.DroppedCount = e.dropped
	e.lastEvent = evt
	e.lastEvent.Usage = cloneUsagePointer(evt.Usage)
	e.lastEvent.LastRequestUsage = cloneUsagePointer(evt.LastRequestUsage)
	delivered := false
	select {
	case e.ch <- evt:
		delivered = true
	default:
		// Backpressure: drop oldest, push new.
		select {
		case <-e.ch:
			e.dropped++
		default:
		}
		evt.DroppedCount = e.dropped
		select {
		case e.ch <- evt:
			delivered = true
		default:
		}
	}
	observers := e.observerSnapshotLocked()
	e.mu.Unlock()
	for _, observer := range observers {
		observer(evt)
	}
	return delivered
}

// EmitPhase emits a single-phase event with the given message count and tool.
func (e *AgentProgressEmitter) EmitPhase(phase AgentProgressPhase, messageCount int, latestTool string) bool {
	return e.Emit(AgentProgressEvent{
		Phase:        phase,
		MessageCount: messageCount,
		LatestTool:   latestTool,
	})
}

// Finish emits a terminal phase event and closes the channel. Idempotent.
// Returns true if it actually performed the close.
func (e *AgentProgressEmitter) Finish(phase AgentProgressPhase, detail string) bool {
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
	terminal.DroppedCount = e.dropped
	terminal.Phase = phase
	terminal.Detail = detail
	terminal.Timestamp = time.Now()
	terminal.ElapsedMs = time.Since(e.started).Milliseconds()
	select {
	case e.ch <- terminal:
	default:
		// Drop one and re-attempt — terminal event must land if at all possible.
		select {
		case <-e.ch:
			e.dropped++
			terminal.DroppedCount = e.dropped
		default:
		}
		select {
		case e.ch <- terminal:
		default:
		}
	}
	close(e.ch)
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

// CollectAgentProgress drains the emitter into a slice. Useful in tests.
func CollectAgentProgress(emitter *AgentProgressEmitter) []AgentProgressEvent {
	if emitter == nil {
		return nil
	}
	out := []AgentProgressEvent{}
	for evt := range emitter.Channel() {
		out = append(out, evt)
	}
	return out
}
