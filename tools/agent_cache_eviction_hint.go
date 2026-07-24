package tools

// agent_cache_eviction_hint.go is the analytics hook that mirrors TS
// finalizeAgentTool's tengu_cache_eviction_hint emission. When a
// sub-agent's transcript exceeded the prompt-cache window during its
// run, we want to log the event so the team can tune cache strategy
// against real workloads. Without this signal, long-running agents that
// pay full prefix-cost per turn are invisible to ops.
//
// The actual analytics sink is pluggable via SetAgentCacheEvictionHintSink;
// the default is a no-op so the helper stays cheap when nobody
// listens.

import (
	"sync"
)

// AgentCacheEvictionHint is the payload emitted on agent finalize when
// the cache window was exceeded. EvictedTokens is approximate (best-
// effort from the loop's accounting).
type AgentCacheEvictionHint struct {
	AgentID       string
	AgentType     string
	TurnsObserved int
	EvictedTokens int
}

// AgentCacheEvictionHintSink is the analytics interface.
type AgentCacheEvictionHintSink interface {
	OnAgentCacheEviction(AgentCacheEvictionHint)
}

// AgentCacheEvictionHintSinkFunc adapts plain functions to the sink.
type AgentCacheEvictionHintSinkFunc func(AgentCacheEvictionHint)

// OnAgentCacheEviction implements AgentCacheEvictionHintSink.
func (f AgentCacheEvictionHintSinkFunc) OnAgentCacheEviction(h AgentCacheEvictionHint) {
	f(h)
}

var (
	cacheEvictMu   sync.RWMutex
	cacheEvictSink AgentCacheEvictionHintSink
)

// SetAgentCacheEvictionHintSink installs (or clears, with nil) the
// global sink. Safe for concurrent callers.
func SetAgentCacheEvictionHintSink(s AgentCacheEvictionHintSink) {
	cacheEvictMu.Lock()
	defer cacheEvictMu.Unlock()
	cacheEvictSink = s
}

// EmitAgentCacheEvictionHint fires the analytics signal when the agent
// loop reports more turns than fit in the prompt-cache window. Callers
// pass turnsObserved + evictedTokens; the helper short-circuits if no
// sink is configured. Returns true when the sink was actually called.
func EmitAgentCacheEvictionHint(agentID, agentType string, turnsObserved, evictedTokens int) bool {
	if turnsObserved <= 0 && evictedTokens <= 0 {
		return false
	}
	cacheEvictMu.RLock()
	sink := cacheEvictSink
	cacheEvictMu.RUnlock()
	if sink == nil {
		return false
	}
	defer func() { _ = recover() }()
	sink.OnAgentCacheEviction(AgentCacheEvictionHint{
		AgentID:       agentID,
		AgentType:     agentType,
		TurnsObserved: turnsObserved,
		EvictedTokens: evictedTokens,
	})
	return true
}
