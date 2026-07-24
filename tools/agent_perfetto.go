package tools

// agent_perfetto.go implements the lightweight agent spawn/finish registry
// the TS reference uses to feed Perfetto / chrome://tracing-style flame
// graphs (utils/telemetry/perfettoTracing.ts).
//
// Mirrors the API surface used by runAgent.ts:
//
//   if (isPerfettoTracingEnabled()) {
//     registerPerfettoAgent(agentId, agentType, parentId)
//   }
//   ...
//   unregisterPerfettoAgent(agentId)
//
// In Go we keep the bookkeeping in-process; downstream telemetry exporters
// can read the registry to attribute time slices to specific agents during
// performance investigations of slow multi-agent runs.

import (
	"os"
	"strings"
	"sync"
	"time"
)

// PerfettoAgentEntry captures one registered agent run.
type PerfettoAgentEntry struct {
	AgentID    string
	AgentType  string
	ParentID   string
	StartedAt  time.Time
	FinishedAt time.Time
}

// Duration reports the lifetime of the entry. Returns 0 while still open.
func (e PerfettoAgentEntry) Duration() time.Duration {
	if e.FinishedAt.IsZero() {
		return 0
	}
	return e.FinishedAt.Sub(e.StartedAt)
}

var (
	perfettoMu      sync.Mutex
	perfettoEntries = map[string]*PerfettoAgentEntry{}
	perfettoHistory []PerfettoAgentEntry
)

// IsPerfettoTracingEnabled mirrors TS isPerfettoTracingEnabled. Reads
// CLAUDE_CODE_PERFETTO_TRACING; truthy values enable the registry. The flag
// is consulted on every call so tests can flip it in setup.
func IsPerfettoTracingEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_CODE_PERFETTO_TRACING"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// RegisterPerfettoAgent records the spawn of a sub-agent. No-op when
// agentID is empty or tracing is disabled.
func RegisterPerfettoAgent(agentID, agentType, parentID string) {
	if !IsPerfettoTracingEnabled() {
		return
	}
	id := strings.TrimSpace(agentID)
	if id == "" {
		return
	}
	perfettoMu.Lock()
	defer perfettoMu.Unlock()
	perfettoEntries[id] = &PerfettoAgentEntry{
		AgentID:   id,
		AgentType: strings.TrimSpace(agentType),
		ParentID:  strings.TrimSpace(parentID),
		StartedAt: time.Now(),
	}
}

// UnregisterPerfettoAgent stamps the finish time and rolls the entry into
// the historical buffer for later export.
func UnregisterPerfettoAgent(agentID string) {
	id := strings.TrimSpace(agentID)
	if id == "" {
		return
	}
	perfettoMu.Lock()
	defer perfettoMu.Unlock()
	entry, ok := perfettoEntries[id]
	if !ok {
		return
	}
	entry.FinishedAt = time.Now()
	perfettoHistory = append(perfettoHistory, *entry)
	delete(perfettoEntries, id)
}

// PerfettoActiveAgents returns a snapshot of currently registered agents.
func PerfettoActiveAgents() []PerfettoAgentEntry {
	perfettoMu.Lock()
	defer perfettoMu.Unlock()
	out := make([]PerfettoAgentEntry, 0, len(perfettoEntries))
	for _, e := range perfettoEntries {
		out = append(out, *e)
	}
	return out
}

// PerfettoCompletedAgents returns the historical buffer (finished agents).
func PerfettoCompletedAgents() []PerfettoAgentEntry {
	perfettoMu.Lock()
	defer perfettoMu.Unlock()
	return append([]PerfettoAgentEntry(nil), perfettoHistory...)
}

// ResetPerfettoRegistry clears both active and historical entries. Tests
// should call this in setup so each scenario starts from a clean slate.
func ResetPerfettoRegistry() {
	perfettoMu.Lock()
	defer perfettoMu.Unlock()
	perfettoEntries = map[string]*PerfettoAgentEntry{}
	perfettoHistory = nil
}

// parentAgentIDFromContextEnv reads the parent agent ID, if any, from the
// environment variable injected by the harness when an agent spawns a
// nested sub-agent. Empty when running at the top level.
func parentAgentIDFromContextEnv() string {
	return strings.TrimSpace(os.Getenv("CLAUDE_CODE_AGENT_ID"))
}
