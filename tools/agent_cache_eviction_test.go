package tools

import (
	"sync"
	"testing"
)

type capturedAnalyticsEvent struct {
	Name    string
	Payload map[string]any
}

func TestEmitCacheEvictionHint_FiresOnFinalize(t *testing.T) {
	mu := sync.Mutex{}
	events := []capturedAnalyticsEvent{}
	SetAgentAnalyticsHook(func(name string, payload map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		copy := map[string]any{}
		for k, v := range payload {
			copy[k] = v
		}
		events = append(events, capturedAnalyticsEvent{Name: name, Payload: copy})
	})
	defer ResetAgentAnalyticsHook()

	summary := agentRunSummary{
		AgentID:       "task-123",
		AgentType:     "explorer",
		TotalTokens:   42,
		ToolUseCount:  3,
		TotalDuration: 555,
	}
	emitCacheEvictionHint(summary)

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	evt := events[0]
	if evt.Name != "tengu_cache_eviction_hint" {
		t.Fatalf("expected tengu_cache_eviction_hint, got %q", evt.Name)
	}
	if evt.Payload["scope"] != "subagent_end" {
		t.Fatalf("expected scope=subagent_end, got %v", evt.Payload["scope"])
	}
	if evt.Payload["agent_id"] != "task-123" {
		t.Fatalf("expected agent_id=task-123, got %v", evt.Payload["agent_id"])
	}
	if evt.Payload["agent_type"] != "explorer" {
		t.Fatalf("expected agent_type=explorer, got %v", evt.Payload["agent_type"])
	}
}

func TestEmitCacheEvictionHint_NoEventWithoutAgentID(t *testing.T) {
	calls := 0
	SetAgentAnalyticsHook(func(name string, payload map[string]any) { calls++ })
	defer ResetAgentAnalyticsHook()
	emitCacheEvictionHint(agentRunSummary{AgentID: ""})
	if calls != 0 {
		t.Fatalf("expected no events for empty agent ID, got %d", calls)
	}
}

func TestEmitCacheEvictionHint_NoHookIsSafe(t *testing.T) {
	ResetAgentAnalyticsHook()
	// Should not panic.
	emitCacheEvictionHint(agentRunSummary{AgentID: "x"})
}
