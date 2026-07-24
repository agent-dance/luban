package coordinator

import (
	"testing"

	"github.com/agent-dance/luban/observability"
)

func TestStaleCompletionRecordsGenerationDropWithoutRunIdentity(t *testing.T) {
	observability.Reset()
	coordinator := NewCoordinator()
	agent := &Agent{ID: "private-agent", busy: true, runID: "run-current"}
	task := &Task{ID: "private-task", Status: TaskRunning, RunID: "run-current"}
	if coordinator.completeAssignment(task, agent, "run-stale", "ignored", nil) {
		t.Fatal("stale completion unexpectedly committed")
	}
	for _, point := range observability.Snapshot() {
		if point.Name != observability.MetricGenerationDrops || point.Labels["surface"] != "coordinator_completion" {
			continue
		}
		if point.Sum != 1 {
			t.Fatalf("generation drop sum = %v, want 1", point.Sum)
		}
		for _, value := range point.Labels {
			if value == "private-agent" || value == "private-task" || value == "run-stale" {
				t.Fatalf("private run identity leaked in %+v", point)
			}
		}
		return
	}
	t.Fatalf("coordinator generation drop missing from %+v", observability.Snapshot())
}
