package tools

import (
	"sync"
	"testing"
)

func TestEmitAgentSpawnFinishTrace_NoSink(t *testing.T) {
	SetAgentPerfettoTraceSink(nil)
	if EmitAgentSpawnTrace("a", "explore", nil) {
		t.Fatalf("expected false when no sink")
	}
	if EmitAgentFinishTrace("a", "explore", nil) {
		t.Fatalf("expected false when no sink")
	}
}

func TestEmitAgentSpawnFinishTrace_DispatchesBothPhases(t *testing.T) {
	var (
		mu     sync.Mutex
		events []AgentTraceEvent
	)
	SetAgentPerfettoTraceSink(AgentPerfettoTraceSinkFunc(func(e AgentTraceEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	}))
	t.Cleanup(func() { SetAgentPerfettoTraceSink(nil) })
	if !EmitAgentSpawnTrace("a1", "verification", map[string]string{"k": "v"}) {
		t.Fatalf("spawn emit returned false")
	}
	if !EmitAgentFinishTrace("a1", "verification", nil) {
		t.Fatalf("finish emit returned false")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 || events[0].Phase != "begin" || events[1].Phase != "end" {
		t.Fatalf("expected begin+end events, got %+v", events)
	}
	if events[0].Metadata["k"] != "v" {
		t.Fatalf("metadata lost on spawn event: %+v", events[0].Metadata)
	}
}
