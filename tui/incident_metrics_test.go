package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/observability"
	gotui "github.com/grindlemire/go-tui"
)

func TestIncidentMetricsRecordOrphansAndScopeDropsWithoutIdentityLabels(t *testing.T) {
	observability.Reset()
	store := NewActivityStore(ActivityScope{SessionID: "visible", Epoch: 2})
	if _, err := store.Restore([]Activity{{ActivityEvent: ActivityEvent{
		ID: "private-work-unit", RunID: "private-run", SessionID: "old", Epoch: 1,
		State: ActivityRunning, Outcome: OutcomeRunning, Sequence: 1,
	}}}); err != nil {
		t.Fatal(err)
	}
	if got := store.ReconcileNonTerminal(nil); got != 1 {
		t.Fatalf("reconciled = %d, want 1", got)
	}
	if err := store.Apply(ActivityEvent{
		ID: "another-private-work-unit", SessionID: "stale-session", Epoch: 1,
		State: ActivityRunning, Outcome: OutcomeRunning,
	}); !errors.Is(err, ErrActivityScopeMismatch) {
		t.Fatalf("scope error = %v, want ErrActivityScopeMismatch", err)
	}

	assertMetricSum(t, observability.MetricActivityOrphans, map[string]string{"source": "restore_reconcile"}, 1)
	assertMetricSum(t, observability.MetricActivityStaleDrops, map[string]string{"source": "scope_fence"}, 1)
	assertMetricLabelsExclude(t, "private-work-unit", "private-run", "stale-session")
}

func TestIncidentMetricsRecordEpochAndSessionFences(t *testing.T) {
	observability.Reset()
	state := NewAppState()
	state.SessionID.Set("session-a")
	state.SessionEpoch.Set(1)
	var queued func()
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { queued = fn; return true }}
	renderer.TextAtEpoch(1, "private stale text")
	state.SessionEpoch.Set(2)
	queued()
	assertMetricSum(t, observability.MetricGenerationDrops, map[string]string{"surface": "tui_epoch"}, 1)

	renderer.enqueue = func(fn func()) bool { fn(); return true }
	if renderer.TryInfoForVisibleSession("session-old", "private notification") {
		t.Fatal("stale session notification unexpectedly delivered")
	}
	assertMetricSum(t, observability.MetricGenerationDrops, map[string]string{"surface": "notification_session"}, 1)
	assertMetricLabelsExclude(t, "session-a", "session-old", "private stale text", "private notification")
}

func TestIncidentMetricsRecordLateActivitySequence(t *testing.T) {
	observability.Reset()
	store := NewActivityStore(ActivityScope{SessionID: "session", Epoch: 3})
	for _, sequence := range []uint64{10, 9} {
		if err := store.Apply(ActivityEvent{
			ID: "private-activity", SessionID: "session", Epoch: 3, Sequence: sequence,
			State: ActivityRunning, Outcome: OutcomeRunning,
		}); err != nil {
			t.Fatal(err)
		}
	}
	assertMetricSum(t, observability.MetricActivityStaleDrops, map[string]string{"source": "sequence_fence"}, 1)
	assertMetricLabelsExclude(t, "private-activity", "session")
}

func TestTerminalOwnerObserverBridgeRecordsOnlyBoundedReason(t *testing.T) {
	observability.Reset()
	if err := gotui.WriteTerminalControl([]byte("private-control-payload")); !errors.Is(err, gotui.ErrNoTerminalControlOwner) {
		t.Fatalf("write error = %v, want ErrNoTerminalControlOwner", err)
	}
	assertMetricSum(t, observability.MetricTerminalControlRejected, map[string]string{"reason": "no_owner"}, 1)
	assertMetricLabelsExclude(t, "private-control-payload")
}

func assertMetricSum(t *testing.T, name observability.MetricName, labels map[string]string, want float64) {
	t.Helper()
	for _, point := range observability.Snapshot() {
		if point.Name != name || len(point.Labels) != len(labels) {
			continue
		}
		matches := true
		for key, value := range labels {
			if point.Labels[key] != value {
				matches = false
				break
			}
		}
		if matches {
			if point.Sum != want {
				t.Fatalf("metric %s labels %+v sum = %v, want %v", name, labels, point.Sum, want)
			}
			return
		}
	}
	t.Fatalf("metric %s labels %+v not found in %+v", name, labels, observability.Snapshot())
}

func assertMetricLabelsExclude(t *testing.T, private ...string) {
	t.Helper()
	for _, point := range observability.Snapshot() {
		for key, value := range point.Labels {
			for _, secret := range private {
				if strings.Contains(key, secret) || strings.Contains(value, secret) {
					t.Fatalf("private value %q leaked in metric %+v", secret, point)
				}
			}
		}
	}
}
