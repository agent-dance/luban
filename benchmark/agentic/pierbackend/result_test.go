package pierbackend

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/evidenceproxy"
	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestRecoverInterruptedAttemptNeverRedispatchesAfterDurableProviderWAL(t *testing.T) {
	artifactDir := t.TempDir()
	invocation := recoveryFixtureInvocation(artifactDir)
	runIdentity := strings.Repeat("a", 64)
	lifecycle := harness.AttemptLifecycle{
		SchemaVersion: "agentic-bench/attempt-lifecycle-v1", RunIdentity: runIdentity,
		ControllerStartedAt: time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC), ProviderAttemptState: "no_provider_attempt",
	}
	if err := harness.WriteJSONAtomic(filepath.Join(artifactDir, "attempt-lifecycle.json"), lifecycle, 0o600); err != nil {
		t.Fatal(err)
	}
	rawEvidence := filepath.Join(artifactDir, "metrics", "provider-http.jsonl")
	entry := evidenceproxy.AttemptStartJournalEntry{
		SchemaVersion: "agentic-bench/provider-attempt-start-v1", RunIdentity: runIdentity,
		Round: 0, StartedAt: lifecycle.ControllerStartedAt.Add(time.Second),
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.WriteBytesAtomic(evidenceproxy.AttemptJournalPath(rawEvidence), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	backend := &Backend{}
	execution, err := backend.RecoverAgent(context.Background(), invocation)
	var infrastructure harness.AttemptInfrastructureError
	if !errors.As(err, &infrastructure) || infrastructure.Category != harness.DeepSWEFailureControllerInfrastructure {
		t.Fatalf("recovery error = %T %v", err, err)
	}
	if !execution.Lifecycle.Recovered || execution.Lifecycle.ProviderAttemptCount != 1 || execution.Lifecycle.ProviderAttemptState != "provider_attempt_started_unsealed" {
		t.Fatalf("recovered lifecycle = %#v", execution.Lifecycle)
	}
	if execution.TrialStartedAt != (time.Time{}) || execution.EvidencePath != "" || execution.SubmissionPatch != "" {
		t.Fatalf("recovery fabricated unavailable trial output: %#v", execution)
	}
	if _, statErr := os.Stat(filepath.Join(artifactDir, "sealed-attempt.json")); statErr != nil {
		t.Fatal(statErr)
	}

	second, secondErr := backend.RecoverAgent(context.Background(), invocation)
	if !errors.As(secondErr, &infrastructure) || infrastructure.Category != harness.DeepSWEFailureControllerInfrastructure || !second.Lifecycle.Recovered {
		t.Fatalf("second recovery changed immutable disposition: %#v %v", second, secondErr)
	}
}

func TestRecoverInterruptedAttemptAllowsRestartOnlyBeforeWALBoundary(t *testing.T) {
	artifactDir := t.TempDir()
	invocation := recoveryFixtureInvocation(artifactDir)
	backend := &Backend{}
	if _, err := backend.RecoverAgent(context.Background(), invocation); err == nil {
		t.Fatal("missing attempt did not request a typed safe restart")
	} else {
		var safe harness.SafeRestartAttemptError
		if !errors.As(err, &safe) {
			t.Fatalf("zero-evidence recovery = %T %v", err, err)
		}
	}

	rawEvidence := filepath.Join(artifactDir, "metrics", "provider-http.jsonl")
	if err := harness.WriteBytesAtomic(rawEvidence, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.RecoverAgent(context.Background(), invocation); err == nil {
		t.Fatal("raw evidence without WAL was treated as restartable")
	} else {
		var protocol harness.AttemptProtocolError
		if !errors.As(err, &protocol) {
			t.Fatalf("raw-without-WAL recovery = %T %v", err, err)
		}
	}
}

func TestRecoverInterruptedAttemptRejectsDuplicateProviderWALRound(t *testing.T) {
	artifactDir := t.TempDir()
	invocation := recoveryFixtureInvocation(artifactDir)
	runIdentity := strings.Repeat("b", 64)
	lifecycle := harness.AttemptLifecycle{
		SchemaVersion: "agentic-bench/attempt-lifecycle-v1", RunIdentity: runIdentity,
		ControllerStartedAt: time.Now().UTC(), ProviderAttemptState: "no_provider_attempt",
	}
	if err := harness.WriteJSONAtomic(filepath.Join(artifactDir, "attempt-lifecycle.json"), lifecycle, 0o600); err != nil {
		t.Fatal(err)
	}
	entry := evidenceproxy.AttemptStartJournalEntry{
		SchemaVersion: "agentic-bench/provider-attempt-start-v1", RunIdentity: runIdentity,
		Round: 0, StartedAt: lifecycle.ControllerStartedAt,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	rawEvidence := filepath.Join(artifactDir, "metrics", "provider-http.jsonl")
	if err := harness.WriteBytesAtomic(evidenceproxy.AttemptJournalPath(rawEvidence), append(append(append([]byte(nil), raw...), '\n'), append(raw, '\n')...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Backend{}).RecoverAgent(context.Background(), invocation); err == nil {
		t.Fatal("duplicate WAL round was accepted")
	} else {
		var protocol harness.AttemptProtocolError
		if !errors.As(err, &protocol) {
			t.Fatalf("duplicate-WAL recovery = %T %v", err, err)
		}
	}
}

func recoveryFixtureInvocation(artifactDir string) harness.AgentInvocation {
	return harness.AgentInvocation{
		PlanEntry: harness.PlanEntry{PairID: "pair-a", TaskID: "task-a", AgentID: "luban", Repetition: 0},
		Task:      harness.PublicTaskView{ID: "task-a"}, Agent: harness.AgentSpec{ID: "luban"}, ArtifactDir: artifactDir,
	}
}
