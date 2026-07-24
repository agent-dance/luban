package tools

// agent_cache_eviction.go implements the tengu_cache_eviction_hint analytics
// signal that the TS finalizeAgentTool emits when a sub-agent finishes.
//
// The hint tells inference-side analytics that the sub-agent's prefix-cache
// chain can be evicted; without it the team cannot see when long-running
// agents are paying full prefix-cost on every turn, and prompt-cache strategy
// can't be tuned to real workloads.
//
// Mirrors src/tools/AgentTool/agentToolUtils.ts (finalizeAgentTool, the
// `logEvent('tengu_cache_eviction_hint', ...)` block).

import (
	"sync"
)

// AgentAnalyticsHook receives analytics events as they are emitted from agent
// finalization. The payload mirrors the TS shape (string-keyed metadata).
type AgentAnalyticsHook func(event string, payload map[string]any)

var (
	agentAnalyticsMu   sync.RWMutex
	agentAnalyticsHook AgentAnalyticsHook
)

// SetAgentAnalyticsHook installs the global analytics hook used by the agent
// finalization path. Tests should call this with their own collector and
// reset to nil in cleanup.
func SetAgentAnalyticsHook(h AgentAnalyticsHook) {
	agentAnalyticsMu.Lock()
	defer agentAnalyticsMu.Unlock()
	agentAnalyticsHook = h
}

// ResetAgentAnalyticsHook clears the global hook. Convenience for tests.
func ResetAgentAnalyticsHook() {
	SetAgentAnalyticsHook(nil)
}

// emitAgentAnalyticsEvent dispatches an event to the global hook. No-op when
// no hook is installed. Recovers from any panic in the hook.
func emitAgentAnalyticsEvent(event string, payload map[string]any) {
	agentAnalyticsMu.RLock()
	hook := agentAnalyticsHook
	agentAnalyticsMu.RUnlock()
	if hook == nil {
		return
	}
	defer func() { _ = recover() }()
	hook(event, payload)
}

// emitCacheEvictionHint logs the tengu_cache_eviction_hint event for a
// finalized sub-agent. Mirrors the TS branch:
//
//	if (lastRequestId) {
//	  logEvent('tengu_cache_eviction_hint', {
//	    scope: 'subagent_end',
//	    last_request_id: lastRequestId,
//	  })
//	}
//
// In Go we don't have request IDs surfaced from the loop, so we fall back to
// the agent ID as a stable key — analytics consumers can still attribute the
// hint to a specific sub-agent run.
func emitCacheEvictionHint(summary agentRunSummary) {
	if summary.AgentID == "" {
		return
	}
	emitAgentAnalyticsEvent("tengu_cache_eviction_hint", map[string]any{
		"scope":          "subagent_end",
		"agent_id":       summary.AgentID,
		"agent_type":     summary.AgentType,
		"total_tokens":   summary.TotalTokens,
		"tool_use_count": summary.ToolUseCount,
		"duration_ms":    summary.TotalDuration,
	})
}
