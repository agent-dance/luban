package tools

import (
	"testing"
)

func TestPerfetto_DisabledByDefault_NoOp(t *testing.T) {
	ResetPerfettoRegistry()
	t.Setenv("CLAUDE_CODE_PERFETTO_TRACING", "")
	RegisterPerfettoAgent("a1", "explorer", "")
	if got := PerfettoActiveAgents(); len(got) != 0 {
		t.Fatalf("expected no entries when disabled, got %d", len(got))
	}
}

func TestPerfetto_RegisterAndUnregister(t *testing.T) {
	ResetPerfettoRegistry()
	t.Setenv("CLAUDE_CODE_PERFETTO_TRACING", "1")
	RegisterPerfettoAgent("a1", "explorer", "parent-1")
	RegisterPerfettoAgent("a2", "planner", "parent-1")
	active := PerfettoActiveAgents()
	if len(active) != 2 {
		t.Fatalf("expected 2 active agents, got %d", len(active))
	}
	UnregisterPerfettoAgent("a1")
	active = PerfettoActiveAgents()
	if len(active) != 1 {
		t.Fatalf("expected 1 active after unregister, got %d", len(active))
	}
	hist := PerfettoCompletedAgents()
	if len(hist) != 1 || hist[0].AgentID != "a1" {
		t.Fatalf("expected a1 in history, got %+v", hist)
	}
	if hist[0].FinishedAt.IsZero() {
		t.Fatalf("FinishedAt should be set after unregister")
	}
	if hist[0].ParentID != "parent-1" {
		t.Fatalf("expected parent-1, got %q", hist[0].ParentID)
	}
}

func TestPerfetto_EmptyAgentIDIsNoOp(t *testing.T) {
	ResetPerfettoRegistry()
	t.Setenv("CLAUDE_CODE_PERFETTO_TRACING", "1")
	RegisterPerfettoAgent("", "x", "")
	UnregisterPerfettoAgent("")
	if len(PerfettoActiveAgents()) != 0 || len(PerfettoCompletedAgents()) != 0 {
		t.Fatalf("empty agent IDs must be ignored")
	}
}

func TestIsPerfettoTracingEnabled_Truthy(t *testing.T) {
	for _, v := range []string{"1", "true", "YES", "on"} {
		t.Setenv("CLAUDE_CODE_PERFETTO_TRACING", v)
		if !IsPerfettoTracingEnabled() {
			t.Fatalf("expected enabled for %q", v)
		}
	}
	t.Setenv("CLAUDE_CODE_PERFETTO_TRACING", "0")
	if IsPerfettoTracingEnabled() {
		t.Fatalf("expected disabled for 0")
	}
}
