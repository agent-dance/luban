package tools

// agent_perfetto_tracing.go is the analytics/perf hook that mirrors TS
// runAgent.ts Perfetto registry. When tracing is enabled, the runtime
// records agent spawn/finish slices alongside tool slices so chrome://
// tracing-style flame graphs can attribute time to specific agents.
// Without these registry hooks, perf investigations of slow multi-agent
// runs cannot pinpoint which agent caused a spike.
//
// The actual trace sink is pluggable via SetAgentPerfettoTraceSink; the
// default is a no-op so the hook stays cheap when tracing is off.

import (
	"sync"
	"time"
)

// AgentTraceEvent is the slice payload. Phase is "begin" or "end"
// — chrome://tracing's standard B/E event split.
type AgentTraceEvent struct {
	Phase     string // "begin" | "end"
	AgentID   string
	AgentType string
	Timestamp time.Time
	Metadata  map[string]string
}

// AgentPerfettoTraceSink receives lifecycle slices.
type AgentPerfettoTraceSink interface {
	OnAgentTrace(AgentTraceEvent)
}

// AgentPerfettoTraceSinkFunc adapts plain functions to the sink.
type AgentPerfettoTraceSinkFunc func(AgentTraceEvent)

// OnAgentTrace implements AgentPerfettoTraceSink.
func (f AgentPerfettoTraceSinkFunc) OnAgentTrace(e AgentTraceEvent) {
	f(e)
}

var (
	agentTraceMu   sync.RWMutex
	agentTraceSink AgentPerfettoTraceSink
)

// SetAgentPerfettoTraceSink installs (or clears, with nil) the trace
// sink. Safe for concurrent callers.
func SetAgentPerfettoTraceSink(s AgentPerfettoTraceSink) {
	agentTraceMu.Lock()
	defer agentTraceMu.Unlock()
	agentTraceSink = s
}

// EmitAgentSpawnTrace fires the "begin" slice. Returns true when a
// sink consumed the event.
func EmitAgentSpawnTrace(agentID, agentType string, metadata map[string]string) bool {
	return emitAgentTrace("begin", agentID, agentType, metadata)
}

// EmitAgentFinishTrace fires the "end" slice. Returns true when a
// sink consumed the event.
func EmitAgentFinishTrace(agentID, agentType string, metadata map[string]string) bool {
	return emitAgentTrace("end", agentID, agentType, metadata)
}

func emitAgentTrace(phase, agentID, agentType string, metadata map[string]string) bool {
	agentTraceMu.RLock()
	sink := agentTraceSink
	agentTraceMu.RUnlock()
	if sink == nil {
		return false
	}
	defer func() { _ = recover() }()
	sink.OnAgentTrace(AgentTraceEvent{
		Phase:     phase,
		AgentID:   agentID,
		AgentType: agentType,
		Timestamp: time.Now(),
		Metadata:  metadata,
	})
	return true
}
