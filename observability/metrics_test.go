package observability

import (
	"strings"
	"sync"
	"testing"
)

func TestCompactionMetricsHaveStableSemantics(t *testing.T) {
	collector := NewCollector()
	collector.RecordCompaction(CompactionObservation{
		Trigger: CompactionTriggerAuto, Outcome: CompactionOutcomeSuccess,
		BeforeTokens: 1000, AfterTokens: 250, BeforeMessages: 40, AfterMessages: 12,
	})
	collector.RecordCompaction(CompactionObservation{
		Trigger: CompactionTriggerReactive, Outcome: CompactionOutcomeFailure,
		BeforeTokens: 1000,
	})

	ratio := requirePoint(t, collector.Snapshot(), MetricCompactionOutputRatio, map[string]string{
		"trigger": "auto", "outcome": "success",
	})
	if ratio.Count != 1 || ratio.Sum != 0.25 || ratio.Last != 0.25 {
		t.Fatalf("output ratio = %+v, want one 0.25 observation", ratio)
	}
	reduction := requirePoint(t, collector.Snapshot(), MetricCompactionReductionRatio, map[string]string{
		"trigger": "auto", "outcome": "success",
	})
	if reduction.Count != 1 || reduction.Sum != 0.75 {
		t.Fatalf("reduction ratio = %+v, want one 0.75 observation", reduction)
	}
	if got := countPoints(collector.Snapshot(), MetricCompactionOutputRatio); got != 1 {
		t.Fatalf("ratio series = %d, failure unexpectedly emitted a ratio", got)
	}
	reduced := requirePoint(t, collector.Snapshot(), MetricCompactionSemanticResults, map[string]string{
		"trigger": "auto", "outcome": "success", "result": "reduced",
	})
	if reduced.Sum != 1 {
		t.Fatalf("reduced total = %v, want 1", reduced.Sum)
	}
	unknown := requirePoint(t, collector.Snapshot(), MetricCompactionSemanticResults, map[string]string{
		"trigger": "reactive", "outcome": "failure", "result": "unknown",
	})
	if unknown.Sum != 1 {
		t.Fatalf("failure semantic total = %v, want 1", unknown.Sum)
	}
}

func TestShellMetricsRequireExplicitRetryCorrelationAndBoundLabels(t *testing.T) {
	collector := NewCollector()
	private := "/Users/alice/private/project && rm -rf secret"
	collector.RecordShellPolicy("block", "shell.policy.block.protected:"+private)
	collector.RecordShellPolicy("block", "shell.policy.block.protected:"+private)
	if got := countPoints(collector.Snapshot(), MetricShellDenialRetries); got != 0 {
		t.Fatalf("uncorrelated decisions fabricated %d retry series", got)
	}

	collector.RecordShellDenialRetry("shell.policy.block.protected:"+private, "modified", "allow")
	retry := requirePoint(t, collector.Snapshot(), MetricShellDenialRetries, map[string]string{
		"reason_class": "protected_path", "retry": "modified", "outcome": "allow",
	})
	if retry.Sum != 1 {
		t.Fatalf("retry total = %v, want 1", retry.Sum)
	}
	for _, point := range collector.Snapshot() {
		for key, value := range point.Labels {
			if strings.Contains(key, private) || strings.Contains(value, private) {
				t.Fatalf("private shell payload leaked in metric %+v", point)
			}
		}
	}
}

func TestCounterDeltaAndSnapshotAreDetached(t *testing.T) {
	collector := NewCollector()
	collector.RecordActivityOrphans(5, ActivitySourceRestoreReconcile)
	point := requirePoint(t, collector.Snapshot(), MetricActivityOrphans, map[string]string{
		"source": "restore_reconcile",
	})
	if point.Count != 1 || point.Sum != 5 {
		t.Fatalf("orphan aggregate = %+v, want one observation totalling five", point)
	}
	point.Labels["source"] = "mutated"
	again := requirePoint(t, collector.Snapshot(), MetricActivityOrphans, map[string]string{
		"source": "restore_reconcile",
	})
	if again.Labels["source"] != "restore_reconcile" {
		t.Fatalf("snapshot mutation changed collector: %+v", again)
	}
}

func TestCollectorConcurrentRecordAndReset(t *testing.T) {
	collector := NewCollector()
	const workers = 32
	const perWorker = 250
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for i := 0; i < perWorker; i++ {
				collector.RecordActivityStaleDrop(ActivitySourceScopeFence)
			}
		}()
	}
	wait.Wait()
	point := requirePoint(t, collector.Snapshot(), MetricActivityStaleDrops, map[string]string{
		"source": "scope_fence",
	})
	if point.Sum != workers*perWorker || point.Count != workers*perWorker {
		t.Fatalf("concurrent aggregate = %+v, want %d", point, workers*perWorker)
	}
	collector.Reset()
	if got := collector.Snapshot(); len(got) != 0 {
		t.Fatalf("snapshot after reset = %+v, want empty", got)
	}
}

func TestArbitraryInputsCollapseToBoundedLabels(t *testing.T) {
	collector := NewCollector()
	private := "private-session-/Users/alice/secret"
	collector.RecordShellPolicy(private, private)
	collector.RecordRogueTerminalWrite(private, private)
	collector.RecordGenerationDrop(GenerationDropSurface(private))
	for _, point := range collector.Snapshot() {
		for key, value := range point.Labels {
			if strings.Contains(key, private) || strings.Contains(value, private) {
				t.Fatalf("arbitrary input leaked in bounded metric %+v", point)
			}
		}
	}
	requirePoint(t, collector.Snapshot(), MetricShellPolicyDecisions, map[string]string{
		"decision": "unknown", "reason_class": "other",
	})
	requirePoint(t, collector.Snapshot(), MetricTerminalRogueWrites, map[string]string{
		"stream": "unknown", "phase": "unknown",
	})
	requirePoint(t, collector.Snapshot(), MetricGenerationDrops, map[string]string{
		"surface": "tui_epoch",
	})
}

func requirePoint(t *testing.T, points []Point, name MetricName, labels map[string]string) Point {
	t.Helper()
	for _, point := range points {
		if point.Name != name || len(point.Labels) != len(labels) {
			continue
		}
		match := true
		for key, value := range labels {
			if point.Labels[key] != value {
				match = false
				break
			}
		}
		if match {
			return point
		}
	}
	t.Fatalf("metric %q labels %+v not found in %+v", name, labels, points)
	return Point{}
}

func countPoints(points []Point, name MetricName) int {
	count := 0
	for _, point := range points {
		if point.Name == name {
			count++
		}
	}
	return count
}
