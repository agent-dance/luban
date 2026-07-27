package report

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestCompilePreservesRecoveredUnsealedControllerAttemptAsUnknownTelemetry(t *testing.T) {
	fixture := buildFormalBundleFixture(t)
	rewriteFormalStateAndScorecard(t, fixture, func(state *harness.ExperimentState, plan harness.RunPlan) {
		makeRecoveredControllerAttempt(t, state, plan.Entries[0])
	})

	data, err := compileSyntheticFormalFixture(fixture.inputPath)
	if err != nil {
		t.Fatalf("Compile recovered controller attempt: %v", err)
	}
	run := formalRunByAttemptID(t, data, harness.RunKey(formalPlan(t, fixture).Entries[0]))
	if !run.ControllerRecovered || run.ProviderAttemptState != "provider_attempt_started_unsealed" || run.ProviderAttemptCount != 1 {
		t.Fatalf("recovered lifecycle = recovered:%v state:%q count:%d", run.ControllerRecovered, run.ProviderAttemptState, run.ProviderAttemptCount)
	}
	if run.TrialDurationSeconds != nil || run.Metrics.TrialDurationSeconds != nil || run.Metrics.WallTimeSeconds != nil {
		t.Fatalf("recovered attempt fabricated timing: trial=%v metric_trial=%v agent=%v", run.TrialDurationSeconds, run.Metrics.TrialDurationSeconds, run.Metrics.WallTimeSeconds)
	}
	if run.Metrics.ProviderRequests != nil || run.Metrics.ProviderRounds != nil || run.Metrics.ToolInvocations != nil || run.Metrics.CatalogCost != nil || run.Metrics.ProviderReportedCost != nil {
		t.Fatalf("recovered attempt fabricated inference telemetry: requests=%v rounds=%v tools=%v catalog=%v provider=%v", run.Metrics.ProviderRequests, run.Metrics.ProviderRounds, run.Metrics.ToolInvocations, run.Metrics.CatalogCost, run.Metrics.ProviderReportedCost)
	}
	assertIntMetric(t, "started transport attempts", run.Metrics.TransportAttempts, 1)
	assertIntMetric(t, "unknown-cost attempts", run.Metrics.UnknownCostAttempts, 1)
	if run.Metrics.CostReceiptObserved != 0 || run.Metrics.CostReceiptTotal != 1 || run.Metrics.AllExecutedUsageObserved != 0 || run.Metrics.AllExecutedUsageTotal != 1 {
		t.Fatalf("recovered coverage = cost %d/%d usage %d/%d, want 0/1 and 0/1", run.Metrics.CostReceiptObserved, run.Metrics.CostReceiptTotal, run.Metrics.AllExecutedUsageObserved, run.Metrics.AllExecutedUsageTotal)
	}
	var rendered bytes.Buffer
	if err := Render(&rendered, data); err != nil {
		t.Fatalf("Render recovered controller attempt: %v", err)
	}
	if strings.Contains(rendered.String(), "0001-01-01") {
		t.Fatal("rendered recovered attempt exposes a synthetic zero timestamp")
	}
}

func TestCompileRejectsFabricatedRecoveredControllerEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*harness.RunRecord)
	}{
		{
			name: "wrong infrastructure category",
			mutate: func(record *harness.RunRecord) {
				record.FailureCategory = harness.DeepSWEFailureProviderInfrastructure
			},
		},
		{
			name: "sealed lifecycle",
			mutate: func(record *harness.RunRecord) {
				record.Execution.Lifecycle.ProviderAttemptState = "provider_attempt_sealed"
			},
		},
		{
			name: "fabricated agent timing",
			mutate: func(record *harness.RunRecord) {
				record.Execution.StartedAt = record.AttemptStartedAt
				record.Execution.FinishedAt = record.AttemptStartedAt.Add(time.Second)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := buildFormalBundleFixture(t)
			rewriteFormalStateAndScorecard(t, fixture, func(state *harness.ExperimentState, plan harness.RunPlan) {
				makeRecoveredControllerAttempt(t, state, plan.Entries[0])
			})
			statePath := filepath.Join(fixture.artifactRoot, "state.json")
			plan := formalPlan(t, fixture)
			var state harness.ExperimentState
			readJSONForFormalFixture(t, statePath, &state)
			entry := plan.Entries[0]
			key := harness.RunKey(entry)
			record := state.Runs[key]
			test.mutate(&record)
			state.Runs[key] = record
			writeJSONForFormalFixture(t, statePath, state)
			repinFormalBundle(t, fixture)
			if _, err := compileSyntheticFormalFixture(fixture.inputPath); err == nil {
				t.Fatal("Compile accepted fabricated recovered controller evidence")
			}
		})
	}
}

func makeRecoveredControllerAttempt(t *testing.T, state *harness.ExperimentState, entry harness.PlanEntry) {
	t.Helper()
	key := harness.RunKey(entry)
	record, ok := state.Runs[key]
	if !ok {
		t.Fatalf("state lacks run %s", key)
	}
	startedAt := record.AttemptStartedAt.Add(-time.Second)
	record.AttemptStartedAt = startedAt
	record.Disposition = harness.DeepSWEAttemptExcluded
	record.FailureCategory = harness.DeepSWEFailureControllerInfrastructure
	record.Verification = nil
	record.Metrics = nil
	record.Execution = &harness.AgentExecution{Lifecycle: harness.AttemptLifecycle{
		SchemaVersion: "agentic-bench/attempt-lifecycle-v1", RunIdentity: formalHex("6", 64),
		ControllerStartedAt: startedAt, ProviderAttemptState: "provider_attempt_started_unsealed",
		ProviderAttemptCount: 1, Recovered: true,
	}}
	state.Runs[key] = record
}

func formalPlan(t *testing.T, fixture formalBundleFixture) harness.RunPlan {
	t.Helper()
	var plan harness.RunPlan
	readJSONForFormalFixture(t, filepath.Join(fixture.artifactRoot, "plan.json"), &plan)
	return plan
}

func formalRunByAttemptID(t *testing.T, data Data, attemptID string) RunData {
	t.Helper()
	for _, experiment := range data.Experiments {
		for _, run := range experiment.Runs {
			if run.AttemptID == attemptID {
				return run
			}
		}
	}
	t.Fatalf("report lacks run %s", attemptID)
	return RunData{}
}
