package loop

import (
	"errors"
	"testing"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/observability"
	"github.com/agent-dance/luban/types"
)

func TestRunCompactionMetricRecordsTerminalResultOnce(t *testing.T) {
	observability.Reset()
	boundary := types.UserMessage("trusted internal boundary fixture")
	result := &compact.CompactionResult{
		BoundaryMarker:            &boundary,
		SummaryMessages:           []types.Message{types.UserMessage("summary fixture")},
		MessagesToKeep:            []types.Message{types.UserMessage("tail fixture")},
		PreCompactTokenCount:      1000,
		TruePostCompactTokenCount: 250,
	}
	recordCompactionMetric("manual", make([]types.Message, 20), result, nil)
	assertLoopMetric(t, observability.MetricCompactionRuns, map[string]string{
		"trigger": "manual", "outcome": "success",
	}, 1)
	assertLoopMetric(t, observability.MetricCompactionOutputRatio, map[string]string{
		"trigger": "manual", "outcome": "success",
	}, 0.25)
	assertLoopMetric(t, observability.MetricCompactionReductionRatio, map[string]string{
		"trigger": "manual", "outcome": "success",
	}, 0.75)

	recordCompactionMetric("reactive", make([]types.Message, 20), nil, errors.New("private provider failure"))
	assertLoopMetric(t, observability.MetricCompactionRuns, map[string]string{
		"trigger": "reactive", "outcome": "failure",
	}, 1)
	for _, point := range observability.Snapshot() {
		if point.Name == observability.MetricCompactionOutputRatio && point.Labels["trigger"] == "reactive" {
			t.Fatalf("failed compaction emitted ratio: %+v", point)
		}
	}
}

func assertLoopMetric(t *testing.T, name observability.MetricName, labels map[string]string, want float64) {
	t.Helper()
	for _, point := range observability.Snapshot() {
		if point.Name != name || len(point.Labels) != len(labels) {
			continue
		}
		matched := true
		for key, value := range labels {
			if point.Labels[key] != value {
				matched = false
				break
			}
		}
		if matched {
			if point.Sum != want {
				t.Fatalf("metric %s %+v sum=%v want=%v", name, labels, point.Sum, want)
			}
			return
		}
	}
	t.Fatalf("metric %s %+v missing from %+v", name, labels, observability.Snapshot())
}
