package tools

import (
	"sync"
	"testing"
)

func TestEmitAgentCacheEvictionHint_NoSinkReturnsFalse(t *testing.T) {
	SetAgentCacheEvictionHintSink(nil)
	if EmitAgentCacheEvictionHint("a1", "explore", 5, 1024) {
		t.Fatalf("expected false when no sink configured")
	}
}

func TestEmitAgentCacheEvictionHint_SkipsZeroValues(t *testing.T) {
	called := false
	SetAgentCacheEvictionHintSink(AgentCacheEvictionHintSinkFunc(func(h AgentCacheEvictionHint) {
		called = true
	}))
	t.Cleanup(func() { SetAgentCacheEvictionHintSink(nil) })
	if EmitAgentCacheEvictionHint("a1", "explore", 0, 0) {
		t.Fatalf("zero turns/tokens should not emit")
	}
	if called {
		t.Fatalf("sink must not be called for zero values")
	}
}

func TestEmitAgentCacheEvictionHint_DispatchesToSink(t *testing.T) {
	var (
		mu  sync.Mutex
		got AgentCacheEvictionHint
	)
	SetAgentCacheEvictionHintSink(AgentCacheEvictionHintSinkFunc(func(h AgentCacheEvictionHint) {
		mu.Lock()
		defer mu.Unlock()
		got = h
	}))
	t.Cleanup(func() { SetAgentCacheEvictionHintSink(nil) })
	if !EmitAgentCacheEvictionHint("agent_x", "verification", 7, 4096) {
		t.Fatalf("expected emit to return true")
	}
	mu.Lock()
	defer mu.Unlock()
	if got.AgentID != "agent_x" || got.AgentType != "verification" || got.TurnsObserved != 7 || got.EvictedTokens != 4096 {
		t.Fatalf("unexpected payload: %+v", got)
	}
}
