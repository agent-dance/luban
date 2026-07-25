package agent

import (
	"context"
	"errors"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestBackgroundManagerReconcilesInterruptedPersistedAgent(t *testing.T) {
	root := t.TempDir()
	store := runtimestore.NewRuntimeTaskStore(root)
	started := time.Now().UTC().Add(-time.Minute)
	record := runtimestore.RuntimeTaskRecord{
		ID: "crashed-agent", Type: agentcontract.TaskTypeLocalAgent, Status: "running", OwnerPID: -1,
		StartedAt: started, UpdatedAt: started, CurrentRunID: "run-1", Attempt: 1,
		QueuedPrompts: 2, QueueReason: "dependency:active_run",
		Runs: []agentcontract.RunRecord{{RunID: "run-1", Attempt: 1, Status: "running", Outcome: agentcontract.RunOutcomeRunning, StartedAt: started, UpdatedAt: started}},
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	got, ok := store.Get(record.ID)
	if !ok {
		t.Fatal("reconciled record missing")
	}
	if got.Status != "failed" || got.Outcome != agentcontract.RunOutcomeInterrupted || got.TerminalReason != interruptedAgentTerminalReason || got.QueuedPrompts != 0 || got.QueueReason != "" || got.FinishedAt == nil {
		t.Fatalf("reconciled task=%+v", got)
	}
	if len(got.Runs) != 1 || got.Runs[0].Outcome != agentcontract.RunOutcomeInterrupted || got.Runs[0].FinishedAt == nil {
		t.Fatalf("reconciled run history=%+v", got.Runs)
	}
	snapshots := manager.Snapshots()
	if len(snapshots) != 1 || snapshots[0].Outcome != agentcontract.RunOutcomeInterrupted || snapshots[0].QueuedPrompts != 0 {
		t.Fatalf("reconciled snapshot=%+v", snapshots)
	}
}

func TestBackgroundManagerReconcilesOwnerDeathAfterStartup(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	store := runtimestore.NewRuntimeTaskStore(root)
	started := time.Now().UTC().Add(-time.Minute)
	record := runtimestore.RuntimeTaskRecord{
		ID: "late-crash", Type: agentcontract.TaskTypeLocalAgent, Status: "running", OwnerPID: os.Getpid(),
		StartedAt: started, CurrentRunID: "run-late", Attempt: 1,
		Runs: []agentcontract.RunRecord{{RunID: "run-late", Attempt: 1, Status: "running", Outcome: agentcontract.RunOutcomeRunning, StartedAt: started, UpdatedAt: started}},
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	if got := manager.ReconcileInterruptedAgentRecords(); got != 0 {
		t.Fatalf("live owner was reconciled: count=%d", got)
	}
	record.OwnerPID = -1
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	if got := manager.ReconcileInterruptedAgentRecords(); got != 1 {
		t.Fatalf("dead owner reconciliation count=%d, want 1", got)
	}
	if got := manager.ReconcileInterruptedAgentRecords(); got != 0 {
		t.Fatalf("periodic reconciliation was not idempotent: count=%d", got)
	}
	snapshot, ok := manager.Snapshot(record.ID)
	if !ok || snapshot.Outcome != agentcontract.RunOutcomeInterrupted || snapshot.TerminalReason != interruptedAgentTerminalReason {
		t.Fatalf("post-startup reconciliation snapshot=%+v ok=%v", snapshot, ok)
	}
}

func TestClassifyAgentRunTerminationKeepsTypedOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		hasOutput bool
		outcome   agentcontract.RunOutcome
		reason    string
	}{
		{name: "success", outcome: agentcontract.RunOutcomeSucceeded, reason: "completed"},
		{name: "max turns", err: &loop.MaxTurnsError{MaxTurns: 2, TurnCount: 2}, hasOutput: true, outcome: agentcontract.RunOutcomePartial, reason: "max_turns"},
		{name: "cancelled", err: context.Canceled, outcome: agentcontract.RunOutcomeCancelled, reason: "context_cancelled"},
		{name: "timed out", err: context.DeadlineExceeded, outcome: agentcontract.RunOutcomeTimedOut, reason: "deadline_exceeded"},
		{name: "interrupted", err: ErrAgentRunInterrupted, outcome: agentcontract.RunOutcomeInterrupted, reason: "runtime_interrupted"},
		{name: "partial failure", err: errors.New("provider failed"), hasOutput: true, outcome: agentcontract.RunOutcomePartial, reason: "error_after_partial_result"},
		{name: "failure", err: errors.New("provider failed"), outcome: agentcontract.RunOutcomeFailed, reason: "error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome, reason := classifyAgentRunTermination(test.err, test.hasOutput)
			if outcome != test.outcome || reason != test.reason {
				t.Fatalf("classification=%s/%s want %s/%s", outcome, reason, test.outcome, test.reason)
			}
		})
	}
}

func TestForegroundAgentPreservesPreciseIncompleteOutcomes(t *testing.T) {
	t.Run("partial max turns", func(t *testing.T) {
		provider := &turnLimitAgentProvider{toolName: "Echo", toolTurns: 1}
		reg := registry.New()
		reg.Register(fakeTool{name: "Echo"})
		tool := &AgentTool{
			Provider: provider,
			Registry: reg,
			InlineProfiles: map[string]agentProfile{
				"bounded": {Name: "bounded", MaxTurns: 1},
			},
		}
		result, err := tool.Execute(context.Background(), agentExecuteInput("bounded work", map[string]any{"subagent_type": "bounded"}))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		incomplete, ok := result.Data.(AgentIncomplete)
		if !ok || incomplete.Outcome != agentcontract.RunOutcomePartial || incomplete.ResultKind() != AgentResultKindPartial || incomplete.Reason != "max_turns" || result.Outcome != types.ToolOutcomePartial {
			t.Fatalf("partial foreground result=%+v (%T)", result, result.Data)
		}
		block := types.MapToolResult(tool, result, "toolu_partial")
		if block.Outcome != types.ToolOutcomePartial {
			t.Fatalf("partial mapper lost outcome: %+v text=%q", block, block.TextContent())
		}
	})

	t.Run("deadline", func(t *testing.T) {
		provider := &task05CancelProvider{started: make(chan struct{})}
		tool := &AgentTool{Provider: provider, Registry: registry.New()}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		result, err := tool.Execute(ctx, agentExecuteInput("wait", nil))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		incomplete, ok := result.Data.(AgentIncomplete)
		if !ok || incomplete.Outcome != agentcontract.RunOutcomeTimedOut || incomplete.ResultKind() != AgentResultKindTimedOut || result.Outcome != types.ToolOutcomeTimedOut {
			t.Fatalf("deadline foreground result=%+v (%T)", result, result.Data)
		}
	})

	t.Run("interrupted", func(t *testing.T) {
		result := agentIncompleteToolResult(agentRunSummary{}, "agent", "general-purpose", "", time.Now(), ErrAgentRunInterrupted)
		incomplete, ok := result.Data.(AgentIncomplete)
		if !ok || incomplete.Outcome != agentcontract.RunOutcomeInterrupted || incomplete.ResultKind() != AgentResultKindInterrupted {
			t.Fatalf("interrupted foreground result=%+v (%T)", result, result.Data)
		}
	})
}

func TestAgentRunCollectsSuccessfulVerificationToolEvidence(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	reg := registry.New()
	reg.Register(fakeTool{name: "Bash"})
	provider := &sequencedAgentProvider{responses: [][]types.StreamEvent{
		parallelAgentToolUseEvents(types.ToolUseBlock{
			Type: types.ContentTypeToolUse,
			ID:   "verify-1",
			Name: "Bash",
			Input: map[string]any{
				"command": "TOKEN=do-not-persist go test ./tools",
			},
		}),
		agentTextEvents("verified"),
	}}
	tool := &AgentTool{Provider: provider, Registry: reg, Background: manager}
	result, err := tool.Execute(context.Background(), agentExecuteInput("verify the implementation", nil))
	if err != nil || result.IsError {
		t.Fatalf("Execute err=%v result=%+v", err, result)
	}
	completed, ok := result.Data.(AgentCompleted)
	if !ok || completed.AgentID == "" {
		t.Fatalf("completed result=%+v (%T)", result, result.Data)
	}
	snapshot, ok := manager.Snapshot(completed.AgentID)
	if !ok || len(snapshot.VerificationRefs) != 1 || len(snapshot.Runs) != 1 || len(snapshot.Runs[0].VerificationRefs) != 1 {
		t.Fatalf("verification evidence snapshot=%+v ok=%v", snapshot, ok)
	}
	ref := snapshot.VerificationRefs[0]
	if !strings.Contains(ref, "Bash:go test") || !strings.Contains(ref, "#tool_result:verify-1") || strings.Contains(ref, "do-not-persist") {
		t.Fatalf("unsafe or incomplete verification ref=%q", ref)
	}
}

func TestAgentRunLineageUsesEachResumeRequestContext(t *testing.T) {
	first := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		TurnID: "turn-1", RunID: "parent-1", BatchID: "batch-1", ActorID: "lead", ActorType: "agent", AgentPath: "lead",
	})
	second := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		TurnID: "turn-2", RunID: "parent-2", BatchID: "batch-2", ActorID: "reviewer", ActorType: "agent", AgentPath: "reviewer",
	})
	b1, p1, path1 := agentRunLineage(first, "worker")
	b2, p2, path2 := agentRunLineage(second, "worker")
	if b1 != "batch-1" || p1 != "parent-1" || path1 != "lead/worker" {
		t.Fatalf("first lineage=%q/%q/%q", b1, p1, path1)
	}
	if b2 != "batch-2" || p2 != "parent-2" || path2 != "reviewer/worker" || path2 == path1 {
		t.Fatalf("resume lineage=%q/%q/%q", b2, p2, path2)
	}
}

func TestFinishAgentRunPersistsFormalPerRunFacts(t *testing.T) {
	started := time.Now().UTC().Add(-time.Second)
	task := &BackgroundTask{
		ID: "agent", StartedAt: started, CurrentRunID: "run-2", Outcome: agentcontract.RunOutcomeRunning,
		Runs:                    []agentcontract.RunRecord{{RunID: "run-1", Attempt: 1, Status: "completed", Result: "old"}, {RunID: "run-2", Attempt: 2, Status: "running", StartedAt: started}},
		pendingTerminalProgress: &agentcontract.ProgressEvent{RunID: "run-2", SourceSequence: 9, Phase: agentcontract.ProgressCompleted, Detail: "done"},
	}
	finished := time.Now().UTC()
	finishAgentRunLocked(task, "run-2", "completed", agentRunSummary{
		Output: "formal result", Outcome: agentcontract.RunOutcomeSucceeded, TerminalReason: "completed",
		ToolUseCount: 7, LatestToolUse: "Bash", TranscriptPath: "/tmp/run-2.jsonl",
		ArtifactRefs: []string{"/workspace/report.md"}, VerificationRefs: []string{"go test ./..."},
	}, "", finished)
	if task.pendingTerminalProgress != nil || task.LatestProgress == nil || task.LatestProgress.SourceSequence != 9 {
		t.Fatalf("terminal progress was not atomically consumed: pending=%+v latest=%+v", task.pendingTerminalProgress, task.LatestProgress)
	}
	if task.Runs[0].Result != "old" {
		t.Fatalf("prior run was overwritten: %+v", task.Runs[0])
	}
	run := task.Runs[1]
	if run.Result != "formal result" || run.ToolUseCount != 7 || run.LatestToolUse != "Bash" || run.Outcome != agentcontract.RunOutcomeSucceeded || len(run.ArtifactRefs) != 1 || len(run.VerificationRefs) != 1 || run.LatestProgress == nil {
		t.Fatalf("formal per-run facts=%+v", run)
	}
}

func TestRecordAgentProgressKeepsLiveOutputAcrossToolUseEvent(t *testing.T) {
	const runID = "run-sticky-output"
	task := &BackgroundTask{
		ID:           "agent-sticky-output",
		Status:       "running",
		CurrentRunID: runID,
		Runs: []agentcontract.RunRecord{{
			RunID: runID, Attempt: 1, Status: "running", Outcome: agentcontract.RunOutcomeRunning,
		}},
	}
	session := &backgroundAgentSession{task: task}

	session.recordAgentProgress(agentcontract.ProgressEvent{
		AgentID: "agent-sticky-output", RunID: runID, Attempt: 1,
		SourceSequence: 4, Phase: agentcontract.ProgressAssistant, MessageCount: 2,
		PartialText: "已取得阶段性结果",
	})
	session.recordAgentProgress(agentcontract.ProgressEvent{
		AgentID: "agent-sticky-output", RunID: runID, Attempt: 1,
		SourceSequence: 5, Phase: agentcontract.ProgressToolUse, MessageCount: 2,
		LatestTool: "WebFetch",
	})

	if task.LatestProgress == nil {
		t.Fatal("task latest progress missing")
	}
	latest := task.LatestProgress
	if latest.SourceSequence != 5 || latest.Phase != agentcontract.ProgressToolUse || latest.LatestTool != "WebFetch" {
		t.Fatalf("task latest event fields were not advanced: %+v", latest)
	}
	if latest.PartialText != "已取得阶段性结果" {
		t.Fatalf("task live output was cleared by empty tool-use progress: %+v", latest)
	}

	if len(task.Runs) != 1 || task.Runs[0].LatestProgress == nil {
		t.Fatalf("run latest progress missing: %+v", task.Runs)
	}
	runLatest := task.Runs[0].LatestProgress
	if runLatest.SourceSequence != 5 || runLatest.Phase != agentcontract.ProgressToolUse || runLatest.LatestTool != "WebFetch" {
		t.Fatalf("run latest event fields were not advanced: %+v", runLatest)
	}
	if runLatest.PartialText != "已取得阶段性结果" {
		t.Fatalf("run live output was cleared by empty tool-use progress: %+v", runLatest)
	}
}

func TestAgentTranscriptOverrideStillCreatesImmutablePerRunFiles(t *testing.T) {
	base := filepath.Join(t.TempDir(), "agent.jsonl")
	t.Setenv("LUBAN_AGENT_TRANSCRIPT", base)
	first := agentTranscriptPathForRun("run-1")
	second := agentTranscriptPathForRun("run-2")
	if first == second || first == base || second == base || !strings.Contains(first, "run-1") || !strings.Contains(second, "run-2") {
		t.Fatalf("per-run transcript paths first=%q second=%q base=%q", first, second, base)
	}
}

func TestBackgroundSnapshotSubscriptionUsesInMemoryProjection(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	updates, unsubscribe := manager.SubscribeSnapshots()
	defer unsubscribe()
	task := &BackgroundTask{ID: "live-task", Type: agentcontract.TaskTypeLocalBash, Status: "running", StartedAt: time.Now().UTC(), done: make(chan struct{})}
	manager.registerTask(task)
	task.mu.Lock()
	record := task.recordLocked()
	task.mu.Unlock()
	manager.persistRecordForTask(task, record)
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("snapshot subscriber was not notified")
	}
	snapshots := manager.InMemorySnapshots()
	if len(snapshots) != 1 || snapshots[0].ID != task.ID || snapshots[0].Status != "running" {
		t.Fatalf("in-memory snapshots=%+v", snapshots)
	}
	close(task.done)
}
