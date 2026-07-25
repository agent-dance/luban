package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	agent "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"

	agentruntime "github.com/agent-dance/luban/internal/agent"
	"github.com/agent-dance/luban/internal/contracts/stream"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/runtime/engine"
	runtimescope "github.com/agent-dance/luban/internal/runtime/scope"
	"github.com/agent-dance/luban/internal/store/session"
	toolcollaboration "github.com/agent-dance/luban/internal/tools/collaboration"
	toolinteraction "github.com/agent-dance/luban/internal/tools/interaction"
	"github.com/agent-dance/luban/internal/ui/terminal"
	tuiapp "github.com/agent-dance/luban/internal/ui/tui"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
)

type recordingDecisionRequester struct {
	request  permissions.PromptRequest
	response permissions.PromptResponse
	onPrompt func()
}

func TestBackgroundVerificationCommandUsesVerifyingPhase(t *testing.T) {
	snapshot := agent.TaskSnapshot{Type: "local_bash", Description: "run acceptance", Command: "go test -race ./..."}
	if phase := backgroundActivityPhase(snapshot, snapshot.Description, "background"); phase != tuiapp.ActivityPhaseVerifying {
		t.Fatalf("background verification phase = %q", phase)
	}
}

func TestBackgroundAgentLifecyclePreservesTypedOutcome(t *testing.T) {
	tests := []struct {
		outcome   agent.RunOutcome
		lifecycle tuiapp.ActivityLifecycle
		want      tuiapp.ObservationOutcome
		attention tuiapp.ActivityAttentionKind
	}{
		{agent.RunOutcomeSucceeded, tuiapp.ActivityLifecycleCompleted, tuiapp.OutcomeSucceeded, tuiapp.ActivityAttentionReadyForReview},
		{agent.RunOutcomePartial, tuiapp.ActivityLifecycleFailed, tuiapp.OutcomePartial, tuiapp.ActivityAttentionNone},
		{agent.RunOutcomeFailed, tuiapp.ActivityLifecycleFailed, tuiapp.OutcomeFailed, tuiapp.ActivityAttentionNone},
		{agent.RunOutcomeCancelled, tuiapp.ActivityLifecycleCancelled, tuiapp.OutcomeCancelled, tuiapp.ActivityAttentionNone},
		{agent.RunOutcomeTimedOut, tuiapp.ActivityLifecycleCancelled, tuiapp.OutcomeTimedOut, tuiapp.ActivityAttentionNone},
		{agent.RunOutcomeInterrupted, tuiapp.ActivityLifecycleFailed, tuiapp.OutcomeOrphan, tuiapp.ActivityAttentionNone},
	}
	for _, test := range tests {
		t.Run(string(test.outcome), func(t *testing.T) {
			lifecycle, outcome, attention := backgroundActivityLifecycle(agent.TaskSnapshot{Type: "local_agent", Status: "failed", Outcome: test.outcome, Detached: true})
			if lifecycle != test.lifecycle || outcome != test.want || attention.Kind != test.attention {
				t.Fatalf("terminal projection=%s/%s/%s want %s/%s/%s", lifecycle, outcome, attention.Kind, test.lifecycle, test.want, test.attention)
			}
		})
	}
}

func TestCompletedForegroundRetainedAgentIsCompleted(t *testing.T) {
	lifecycle, outcome, attention := backgroundActivityLifecycle(agent.TaskSnapshot{Type: "local_agent", Status: "completed"})
	if lifecycle != tuiapp.ActivityLifecycleCompleted || outcome != tuiapp.OutcomeSucceeded || attention.Kind != tuiapp.ActivityAttentionNone {
		t.Fatalf("completed foreground Agent activity = %s/%s/%s, want completed/succeeded/no-attention", lifecycle, outcome, attention.Kind)
	}
	lifecycle, outcome, attention = backgroundActivityLifecycle(agent.TaskSnapshot{Type: "local_agent", Status: "completed", Detached: true})
	if lifecycle != tuiapp.ActivityLifecycleCompleted || outcome != tuiapp.OutcomeSucceeded || attention.Kind != tuiapp.ActivityAttentionReadyForReview {
		t.Fatalf("completed detached Agent activity = %s/%s/%s, want completed/succeeded/ready-for-review", lifecycle, outcome, attention.Kind)
	}
	lifecycle, outcome, attention = backgroundActivityLifecycle(agent.TaskSnapshot{Type: "local_bash", Status: "completed"})
	if lifecycle != tuiapp.ActivityLifecycleCompleted || outcome != tuiapp.OutcomeSucceeded || attention.Kind != tuiapp.ActivityAttentionNone {
		t.Fatalf("completed bash activity = %s/%s/%s, want completed/succeeded/no-attention", lifecycle, outcome, attention.Kind)
	}
}

func TestBackgroundAgentActivityCarriesRunIdentityAndSemanticProgress(t *testing.T) {
	snapshot := agent.TaskSnapshot{
		ID: "agent-1", Type: "local_agent", Status: "running", Description: "inspect display",
		CurrentRunID: "run-2", Attempt: 2, BatchID: "batch-1", ParentRunID: "run-parent", AgentPath: "lead/agent-1",
		QueuedPrompts: 1, QueueReason: "dependency:active_run",
		LatestProgress: &agent.ProgressEvent{
			RunID: "run-2", Attempt: 2, SourceSequence: 7, Phase: agent.ProgressToolUse,
			MessageCount: 3, LatestTool: "Read", Detail: "reading renderer", ElapsedMs: 2500, TokensUsed: 420,
		},
	}
	event := backgroundActivityEvent(snapshot, "session", 4, "inspect display", "agent", "agent-1", nil, "task:agent-1")
	if event.RunID != "run-2" || event.Attempt != 2 || event.BatchID != "batch-1" || event.ParentRunID != "run-parent" || event.AgentPath != "lead/agent-1" {
		t.Fatalf("run identity was not projected: %+v", event)
	}
	if event.SourceSequence != 7 || event.Progress.Current != 3 || !strings.Contains(event.Progress.Message, "Read") || !strings.Contains(event.Progress.Message, "2.5s") || !strings.Contains(event.Progress.Message, "420 tokens") || !strings.Contains(event.Progress.Message, "1 queued") || !strings.Contains(event.Progress.Message, "waiting for the active run") {
		t.Fatalf("semantic progress was not projected: %+v", event.Progress)
	}
}

func TestTypedSlashCommandPresentationUpdatesOneActivityRun(t *testing.T) {
	state := tuiapp.NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(3)
	state.Activities = tuiapp.NewActivityStore(tuiapp.ActivityScope{SessionID: "session", Epoch: 3})
	var terminalOutput string
	sink := newTUICommandPresentationSink(directActivityApp{state: state}, "session", 3, "command-run-1", func(value string) { terminalOutput += value })
	sink(commands.CommandPresentation{
		Command: "doctor", Action: "diagnose", State: commands.CommandStateRunning,
		Outcome: commands.CommandOutcomeUnknown, Summary: "Inspecting runtime",
	})
	sink(commands.CommandPresentation{
		Command: "doctor", Action: "diagnose", State: commands.CommandStateCompleted,
		Outcome: commands.CommandOutcomeFailed, Result: "MCP connectivity failed",
		NextAction: "repair MCP", Display: commands.CommandDisplayInspector, Risk: commands.CommandRiskLow,
		OutcomeReliable: true, Sensitive: true, HasMore: true,
		Sections:     []commands.CommandPresentationSection{{Label: "Checks", Text: "database passed; MCP failed"}},
		EvidenceRefs: []string{"artifact://doctor/report-7"},
	})

	snapshot := state.ActivitySnapshot()
	if len(snapshot.Activities) != 1 {
		t.Fatalf("activities=%+v", snapshot.Activities)
	}
	activity := snapshot.Activities[0]
	if activity.RunID != "command-run-1" || activity.Kind != tuiapp.ActivityCommand || activity.Lifecycle != tuiapp.ActivityLifecycleFailed || activity.Outcome != tuiapp.OutcomeFailed {
		t.Fatalf("typed command activity=%+v", activity)
	}
	for _, want := range []string{"Checks=database passed; MCP failed", "Evidence references: artifact://doctor/report-7", "Display: details", "Risk: low", "Next: repair MCP", "More detail is retained.", "Sensitive values were redacted."} {
		if !strings.Contains(activity.Progress.Message, want) {
			t.Fatalf("typed command receipt missing %q: %+v", want, activity.Progress)
		}
	}
	if strings.Count(terminalOutput, "MCP connectivity failed") != 1 {
		t.Fatalf("terminal command result count/output = %q", terminalOutput)
	}
	if strings.Count(terminalOutput, "Command /doctor: failed") != 1 {
		t.Fatalf("terminal semantic receipt count/output = %q", terminalOutput)
	}
	for _, want := range []string{"Checks: database passed; MCP failed", "Evidence references: artifact://doctor/report-7"} {
		if !strings.Contains(terminalOutput, want) {
			t.Fatalf("terminal structured receipt missing %q: %q", want, terminalOutput)
		}
	}
	if len(activity.Control.DetailRefs) != 2 {
		t.Fatalf("structured command detail refs = %+v", activity.Control.DetailRefs)
	}
	details := make([]string, 0, len(activity.Control.DetailRefs))
	for _, ref := range activity.Control.DetailRefs {
		data, err := state.Details.Get(ref)
		if err != nil {
			t.Fatalf("load command detail: %v", err)
		}
		details = append(details, string(data))
	}
	if !strings.Contains(strings.Join(details, "\n"), "database passed; MCP failed") ||
		!strings.Contains(strings.Join(details, "\n"), "artifact://doctor/report-7") {
		t.Fatalf("structured command details lost: %q", details)
	}
}

func TestCommandTerminalErrorAfterLegacyProgressIsVisibleOnce(t *testing.T) {
	terminal := formatCommandPresentationTerminal(commands.CommandPresentation{
		Command: "compact", Action: "compact", State: commands.CommandStateCompleted,
		Outcome: commands.CommandOutcomeTimedOut, Result: "deadline exceeded",
		Display: commands.CommandDisplayReceipt, Risk: commands.CommandRiskMedium,
	})
	if strings.Count(terminal, "deadline exceeded") != 1 {
		t.Fatalf("terminal error was hidden or duplicated after legacy progress: %q", terminal)
	}
}

func TestActivityPersistenceBacksOffAtLargeRunCounts(t *testing.T) {
	tests := []struct {
		count int
		want  time.Duration
	}{
		{count: 10, want: 250 * time.Millisecond},
		{count: 1_000, want: time.Second},
		{count: 10_000, want: 2 * time.Second},
		{count: 50_000, want: 5 * time.Second},
	}
	for _, test := range tests {
		if got := activityPersistenceInterval(test.count); got != test.want {
			t.Errorf("activityPersistenceInterval(%d)=%s, want %s", test.count, got, test.want)
		}
	}
}

func TestBackgroundAgentGroupSummaryPrioritizesExceptionsAndLatestState(t *testing.T) {
	root := t.TempDir()
	current := agent.TaskSnapshot{ID: "a", Type: "local_agent", Status: "completed", OwnerSessionID: "session", OwnerProjectRoot: root, Detached: true}
	snapshots := []agent.TaskSnapshot{
		current,
		{ID: "b", Type: "local_agent", Status: "failed", OwnerSessionID: "session", OwnerProjectRoot: root},
		{ID: "c", Type: "local_agent", Status: "running", OwnerSessionID: "session", OwnerProjectRoot: root},
		{ID: "foreign", Type: "local_agent", Status: "failed", OwnerSessionID: "other", OwnerProjectRoot: root},
	}
	got := backgroundAgentGroupSummary(snapshots, current)
	for _, want := range []string{"Agent group: 3 total", "1 failed", "1 running", "1 ready for review"} {
		if !strings.Contains(got, want) {
			t.Fatalf("group summary %q missing %q", got, want)
		}
	}
	if strings.Index(got, "failed") > strings.Index(got, "running") {
		t.Fatalf("exception was not announced before running members: %q", got)
	}
}

func TestBindTUIBackgroundActivitiesSkipsIdleAgentRegistration(t *testing.T) {
	root := t.TempDir()
	const sessionID = "idle-registration-session"
	store := runtimestore.NewRuntimeTaskStore(root)
	now := time.Now().UTC()
	for _, record := range []runtimestore.RuntimeTaskRecord{
		{
			ID: "idle-agent", Type: "local_agent", Status: "completed",
			Description: "registered but not started", OwnerSessionID: sessionID,
			OwnerProjectRoot: root, OwnerPID: os.Getpid(), Attempt: 0,
			StartedAt: now, UpdatedAt: now,
		},
		{
			ID: "running-agent", Type: "local_agent", Status: "running",
			Description: "started retained agent", OwnerSessionID: sessionID,
			OwnerProjectRoot: root, OwnerPID: os.Getpid(), CurrentRunID: "run-1", Attempt: 1,
			StartedAt: now, UpdatedAt: now,
			LatestProgress: &agent.ProgressEvent{
				AgentID: "running-agent", ParentToolUseID: "agent-call", RunID: "run-1", Attempt: 1,
			},
		},
	} {
		if err := store.Save(record); err != nil {
			t.Fatalf("save runtime task %s: %v", record.ID, err)
		}
	}

	manager := newAgentBackgroundPresentationAdapter(agentruntime.NewBackgroundTaskManager(root))
	cleanupBackgroundTaskManager(t, manager.BackgroundTaskManager)
	state := tuiapp.NewAppState()
	state.SessionID.Set(sessionID)
	state.SessionEpoch.Set(1)
	state.Activities = tuiapp.NewActivityStore(tuiapp.ActivityScope{SessionID: sessionID, Epoch: 1})
	unbind := bindTUIBackgroundActivities(manager, directActivityApp{state: state})
	unbind()

	activities := state.ActivitySnapshot().Activities
	if len(activities) != 1 {
		t.Fatalf("background activities = %#v, want only the started Agent attempt", activities)
	}
	activity := activities[0]
	if activity.ID != "background:running-agent" || activity.Attempt != 1 || activity.RunID != "run-1" || activity.Progress.ParentToolUseID != "agent-call" {
		t.Fatalf("started Agent projection = %+v", activity)
	}
}

type directActivityApp struct{ state *tuiapp.AppState }

func (a directActivityApp) State() *tuiapp.AppState { return a.state }
func (a directActivityApp) UpdateSync(fn func()) bool {
	fn()
	return true
}

type queuedActivityApp struct {
	state   *tuiapp.AppState
	updates []func()
}

type stoppedActivityApp struct{ state *tuiapp.AppState }

func (a stoppedActivityApp) State() *tuiapp.AppState { return a.state }
func (a stoppedActivityApp) UpdateSync(func()) bool  { return false }

type partialSaveSessionManager struct {
	engine.SessionManager
	saved map[string]bool
}

func (s *partialSaveSessionManager) Save(id string, _ []types.Message) error {
	s.saved[id] = true
	return errors.New("metadata persistence failed after transcript rename")
}

func (s *partialSaveSessionManager) Delete(id string) error {
	delete(s.saved, id)
	return nil
}

type partialSaveEngine struct {
	engine.Engine
	sessions engine.SessionManager
}

type clearGenerationEngine struct{ *sessionSwitcherTestEngine }

func (e *clearGenerationEngine) ContextGenerationStateForSession(sessionID, _ string) (engine.ContextGenerationState, error) {
	if e == nil || e.sessionSwitcherTestEngine == nil || e.sessions == nil {
		return engine.ContextGenerationState{}, nil
	}
	if _, ok := e.sessions.messages[sessionID]; !ok {
		return engine.ContextGenerationState{}, nil
	}
	return engine.ContextGenerationState{Generation: 1, Persisted: true}, nil
}

type manualCompactionTestEngine struct {
	engine.Engine
	sessions      *sessionSwitcherTestSessions
	after         []types.Message
	err           error
	usageEvent    *stream.Event
	boundaryEvent *stream.Event
}

func (e *manualCompactionTestEngine) Sessions() engine.SessionManager { return e.sessions }

func (e *manualCompactionTestEngine) Compact(_ context.Context, sessionID string, _ ...string) (engine.CompactResult, error) {
	if e.err != nil {
		return engine.CompactResult{}, e.err
	}
	before, _ := e.sessions.Load(sessionID)
	if err := e.sessions.Save(sessionID, e.after); err != nil {
		return engine.CompactResult{}, err
	}
	return engine.CompactResult{
		Compacted: true, BeforeMessageCount: len(before), AfterMessageCount: len(e.after),
	}, nil
}

func (e *manualCompactionTestEngine) CompactWithEvents(ctx context.Context, sessionID, customInstructions string, onEvent func(stream.Event)) (engine.CompactResult, error) {
	result, err := e.Compact(ctx, sessionID, customInstructions)
	if err != nil {
		return engine.CompactResult{}, err
	}
	if e.usageEvent != nil && onEvent != nil {
		onEvent(*e.usageEvent)
	}
	if e.boundaryEvent != nil && onEvent != nil {
		onEvent(*e.boundaryEvent)
	}
	return result, nil
}

func (e partialSaveEngine) Sessions() engine.SessionManager { return e.sessions }

func (a *queuedActivityApp) State() *tuiapp.AppState { return a.state }
func (a *queuedActivityApp) UpdateSync(fn func()) bool {
	a.updates = append(a.updates, fn)
	for len(a.updates) > 0 {
		next := a.updates[0]
		a.updates = a.updates[1:]
		next()
	}
	return true
}

func TestReleaseTUIQueryDrainsUsageBeforeAllowingSessionTransition(t *testing.T) {
	state := tuiapp.NewAppState()
	generation := state.SetQueryCancel(func() {})
	app := &queuedActivityApp{state: state}
	tracker := ui.NewCostTracker("test-model")
	tracker.RecordTurnUsageForProviderModel("", "", types.Usage{InputTokens: 1000, OutputTokens: 120, CacheReadInputTokens: 400}, 0)
	tracker.RecordTurnUsageForProviderModel("", "", types.Usage{InputTokens: 500, OutputTokens: 80, CacheReadInputTokens: 200}, 0)
	usageCommittedWhileActive := false
	app.updates = append(app.updates, func() {
		usageCommittedWhileActive = state.HasActiveQuery()
	})

	releaseTUIQueryAfterUpdates(app, generation, tracker)

	if !usageCommittedWhileActive {
		t.Fatal("query became transitionable before its queued usage committed")
	}
	if state.HasActiveQuery() {
		t.Fatal("query remained active after queued updates drained")
	}
	if got := state.SessionInputTokens.Get(); got != 500 {
		t.Fatalf("latest input after query release = %d, want 500", got)
	}
	if totals := state.ActiveSessionUsage(); totals.InputTokens != 1500 || totals.OutputTokens != 200 || totals.CacheReadTokens != 600 {
		t.Fatalf("session totals after query release = %+v, want input/output/cache 1500/200/600", totals)
	}
}

func TestFlushTUIUsageFallsBackWhenUpdateQueueHasStopped(t *testing.T) {
	state := tuiapp.NewAppState()
	tracker := ui.NewCostTracker("test-model")
	tracker.RecordTurnUsageForProviderModel("", "", types.Usage{InputTokens: 500, OutputTokens: 80, CacheReadInputTokens: 200}, 0)

	flushTUIUsageUpdates(stoppedActivityApp{state: state}, tracker)

	if got := state.SessionInputTokens.Get(); got != 500 {
		t.Fatalf("latest input after stopped-queue flush = %d, want 500", got)
	}
	if totals := state.ActiveSessionUsage(); totals.InputTokens != 500 || totals.OutputTokens != 80 || totals.CacheReadTokens != 200 {
		t.Fatalf("session totals after stopped-queue flush = %+v, want input/output/cache 500/80/200", totals)
	}
}

type recordingActivityStopper struct{ ids []string }

func (s *recordingActivityStopper) Stop(id string) (agent.TaskSnapshot, error) {
	s.ids = append(s.ids, id)
	return agent.TaskSnapshot{ID: id, Status: "cancelled"}, nil
}

func TestPerformTUIActivityActionControlsParallelRowsIndependently(t *testing.T) {
	state := tuiapp.NewAppState()
	for _, id := range []string{"background:a", "background:b"} {
		if err := state.ApplyActivity(tuiapp.ActivityEvent{ID: id, Lifecycle: tuiapp.ActivityLifecycleRunning, Outcome: tuiapp.OutcomeRunning, Attention: tuiapp.ActivityAttention{Kind: tuiapp.ActivityAttentionNone}, Control: tuiapp.ActivityControl{Cancelable: true}}); err != nil {
			t.Fatal(err)
		}
	}
	app := directActivityApp{state: state}
	stopper := &recordingActivityStopper{}
	for _, id := range []string{"background:a", "background:b"} {
		if _, err := performTUIActivityAction(app, stopper, id, "cancel"); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(stopper.ids, ","); got != "a,b" {
		t.Fatalf("parallel cancel targets = %q", got)
	}

	ctx := tuiapp.ToolEventContext{Outcome: tuiapp.OutcomeSucceeded}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: "tool", Content: "evidence"}); err != nil {
		t.Fatal(err)
	}
	if _, err := performTUIActivityAction(app, stopper, "tool:tool", "jump"); err != nil {
		t.Fatal(err)
	}
	activity, ok := state.GetActivity("tool:tool")
	if !ok || activity.Control.JumpTarget == "" {
		t.Fatalf("tool activity missing jump target: %+v", activity)
	}
	observationID := activity.Control.JumpTarget
	if got := state.ActiveSessionInteraction().FocusedObservationID; got != observationID {
		t.Fatalf("jump focused %q, want %q", got, observationID)
	}
	if _, err := performTUIActivityAction(app, stopper, "tool:tool", "details"); err != nil {
		t.Fatal(err)
	}
	observation, ok := state.GetObservation(observationID)
	if !ok || observation.Disclosure.Level != tuiapp.DisclosureEvidence {
		t.Fatalf("details did not open evidence: %+v", observation)
	}

	if err := state.ApplyActivity(tuiapp.ActivityEvent{
		ID: "agent:private", Kind: tuiapp.ActivityAgent, Lifecycle: tuiapp.ActivityLifecycleCompleted, Outcome: tuiapp.OutcomeSucceeded,
		Attention: tuiapp.ActivityAttention{Kind: tuiapp.ActivityAttentionNone},
		Control:   tuiapp.ActivityControl{DetailRefs: []tuiapp.DetailRef{{Source: "memory", Key: "agent/private", Size: 128}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := performTUIActivityAction(app, stopper, "agent:private", "details"); err == nil {
		t.Fatal("Agent activity unexpectedly exposed complete run details")
	}
}

func (r *recordingDecisionRequester) DecisionRequest(_ context.Context, request permissions.PromptRequest) permissions.PromptResponse {
	r.request = request
	if r.onPrompt != nil {
		r.onPrompt()
	}
	return r.response
}

func TestDeleteTUISessionHistoryRequiresStructuredApprovalAndProtectsActiveSession(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	for _, id := range []string{"active", "target"} {
		if err := repo.Save(id, projectDir, []types.Message{types.UserMessage(id)}); err != nil {
			t.Fatal(err)
		}
	}
	current := "active"
	requester := &recordingDecisionRequester{response: permissions.PromptResponse{Outcome: permissions.PromptOutcomeRejected}}
	err := deleteTUISessionHistory(context.Background(), repo, nil, &sync.Mutex{}, requester, func() string { return current }, projectDir, "target")
	if err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("rejected deletion error = %v", err)
	}
	if requester.request.Target != "target" || requester.request.RiskLevel != 3 || requester.request.ApprovalScope == "" || len(requester.request.Choices) != 2 {
		t.Fatalf("deletion decision lost structured risk facts: %+v", requester.request)
	}
	if _, err := repo.Resolve("target", projectDir); err != nil {
		t.Fatalf("rejected deletion removed target: %v", err)
	}

	requester.response = permissions.PromptResponse{Outcome: permissions.PromptOutcomeApproved, Choice: "allow_once"}
	if err := deleteTUISessionHistory(context.Background(), repo, nil, &sync.Mutex{}, requester, func() string { return current }, projectDir, "target"); err != nil {
		t.Fatalf("approved deletion: %v", err)
	}
	if _, err := repo.Resolve("target", projectDir); err == nil {
		t.Fatal("approved deletion retained target session")
	}
	if err := deleteTUISessionHistory(context.Background(), repo, nil, &sync.Mutex{}, requester, func() string { return current }, projectDir, "active"); err == nil {
		t.Fatal("active session deletion was accepted")
	}
}

func TestDeleteTUISessionHistoryRechecksActiveSessionAfterDecision(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	if err := repo.Save("target", projectDir, []types.Message{types.UserMessage("target")}); err != nil {
		t.Fatal(err)
	}
	current := "other"
	requester := &recordingDecisionRequester{
		response: permissions.PromptResponse{Outcome: permissions.PromptOutcomeApproved},
		onPrompt: func() { current = "target" },
	}
	err := deleteTUISessionHistory(context.Background(), repo, nil, &sync.Mutex{}, requester, func() string { return current }, projectDir, "target")
	if err == nil || !strings.Contains(err.Error(), "active session") {
		t.Fatalf("post-decision active-session check error = %v", err)
	}
	if _, err := repo.Resolve("target", projectDir); err != nil {
		t.Fatalf("target was deleted after becoming active: %v", err)
	}
}

func TestPersistAndRestoreActivityRunsAttentionAndViewState(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "activity-runs-session"
	messages := []types.Message{types.UserMessage("start")}
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	state := tuiapp.NewAppState()
	if err := state.ApplySessionSnapshot(tuiapp.SessionSnapshot{
		Identity:   tuiapp.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1},
		Projection: tuiapp.SessionProjection{Details: tuiapp.NewMemoryDetailStore()},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyActivity(tuiapp.ActivityEvent{
		ID: "background:agent", RunID: "run-1", Attempt: 1, SessionID: sessionID, Epoch: 1,
		Kind: tuiapp.ActivityAgent, Lifecycle: tuiapp.ActivityLifecycleCompleted, Outcome: tuiapp.OutcomeSucceeded,
		Attention: tuiapp.ActivityAttention{Kind: tuiapp.ActivityAttentionReadyForReview, Severity: tuiapp.ActivityAttentionSeverityInfo, Unread: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.AcknowledgeActivity("background:agent"); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyActivity(tuiapp.ActivityEvent{
		ID: "background:agent", RunID: "run-2", Attempt: 2, SessionID: sessionID, Epoch: 1,
		Kind: tuiapp.ActivityAgent, Lifecycle: tuiapp.ActivityLifecycleBlocked, Outcome: tuiapp.OutcomeRunning,
		Attention: tuiapp.ActivityAttention{Kind: tuiapp.ActivityAttentionNeedsInput, Severity: tuiapp.ActivityAttentionSeverityWarning, Unread: true, DecisionID: "decision-2"},
	}); err != nil {
		t.Fatal(err)
	}
	state.ActivityFocus.Set("background:agent")
	state.ActivityViewOffset.Set(4)
	cfg := TUIREPLConfig{Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir}
	if err := persistTUISessionLifecycle(cfg, state); err != nil {
		t.Fatal(err)
	}

	snapshot, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, messages)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Activities) != 2 || snapshot.ActivityFocus != "background:agent" || snapshot.ActivityViewOffset != 4 {
		t.Fatalf("prepared activity state=%+v focus=%q offset=%d", snapshot.Activities, snapshot.ActivityFocus, snapshot.ActivityViewOffset)
	}
	if snapshot.Activities[1].SupersedesRunID != "run-1" {
		t.Fatalf("restored retry supersedes=%q, want run-1", snapshot.Activities[1].SupersedesRunID)
	}
	resumed := tuiapp.NewAppState()
	if err := resumed.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	latest := resumed.ActivitySnapshot().Activities
	if len(latest) != 1 || latest[0].RunID != "run-2" || latest[0].SupersedesRunID != "run-1" || latest[0].Attention.DecisionID != "decision-2" || !latest[0].Attention.Unread {
		t.Fatalf("resumed latest activity=%+v", latest)
	}
	old, ok := resumed.Activities.GetRun("background:agent", "run-1")
	if !ok || !old.Acknowledged || old.Epoch != 2 {
		t.Fatalf("resumed old activity=%+v ok=%v", old, ok)
	}
	if resumed.ActivityFocus.Get() != "background:agent" {
		t.Fatalf("activity focus=%q", resumed.ActivityFocus.Get())
	}
	// Offset is clamped to the latest visible row count on restore.
	if resumed.ActivityViewOffset.Get() != 0 {
		t.Fatalf("activity offset=%d, want clamped 0", resumed.ActivityViewOffset.Get())
	}
}

func TestActivityTransitionIsPersistedWithoutSessionExit(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "activity-write-through"
	messages := []types.Message{types.UserMessage("start")}
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	state := tuiapp.NewAppState()
	if err := state.ApplySessionSnapshot(tuiapp.SessionSnapshot{
		Identity:   tuiapp.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1},
		Projection: tuiapp.SessionProjection{Details: tuiapp.NewMemoryDetailStore()},
	}); err != nil {
		t.Fatal(err)
	}
	cfg := TUIREPLConfig{Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, SessionTransitionMu: &sync.Mutex{}}
	unbind := bindTUIActivityPersistence(cfg, directTUIActivityApp{state: state})
	defer unbind()
	if err := state.ApplyActivity(tuiapp.ActivityEvent{
		ID: "background:agent", RunID: "run-1", Attempt: 1, SessionID: sessionID, Epoch: 1,
		Kind: tuiapp.ActivityAgent, Lifecycle: tuiapp.ActivityLifecycleBlocked, Outcome: tuiapp.OutcomeRunning,
		Attention: tuiapp.ActivityAttention{Kind: tuiapp.ActivityAttentionNeedsInput, Unread: true, DecisionID: "decision"},
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		snapshot, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, messages)
		if err == nil && len(snapshot.Activities) == 1 && snapshot.Activities[0].Attention.Unread {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("activity transition was not checkpointed: snapshot=%+v err=%v", snapshot.Activities, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSemanticViewTransitionIsCheckpointedWithoutSessionExit(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "view-write-through"
	persisted := []types.Message{types.UserMessage("start"), types.AssistantMessage("answer")}
	if err := repo.Save(sessionID, projectDir, persisted); err != nil {
		t.Fatal(err)
	}
	projection, err := tuiapp.ProjectPersistedMessages(tuiapp.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}, persisted, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := tuiapp.NewAppState()
	if err := state.ApplySessionSnapshot(tuiapp.SessionSnapshot{
		Identity: tuiapp.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}, Projection: projection,
	}); err != nil {
		t.Fatal(err)
	}
	bindViewFidelityStateToCurrentGeneration(t, repo, state, sessionID, projectDir)
	cfg := TUIREPLConfig{Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, SessionTransitionMu: &sync.Mutex{}}
	unbind := bindTUIActivityPersistence(cfg, directTUIActivityApp{state: state})
	defer unbind()
	state.AppendMessage(tuiapp.Message{Kind: tuiapp.MsgInfo, Text: "LOCAL WRITE-THROUGH RECEIPT"})

	deadline := time.Now().Add(2 * time.Second)
	for {
		snapshot, prepareErr := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, persisted)
		if prepareErr == nil {
			for _, message := range snapshot.Projection.Messages {
				if message.Text == "LOCAL WRITE-THROUGH RECEIPT" {
					return
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("semantic view transition was not checkpointed: err=%v", prepareErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPersistTUISessionLifecycleCreatesTranscriptForEmptySession(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "empty-session"
	state := tuiapp.NewAppState()
	state.SessionID.Set(sessionID)
	state.SessionNS.Set(projectDir)

	if err := persistTUISessionLifecycle(TUIREPLConfig{Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir}, state); err != nil {
		t.Fatal(err)
	}
	messages, ref, err := repo.LoadByID(sessionID, projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 || ref.ID != sessionID {
		t.Fatalf("created empty transcript = %#v, ref=%+v", messages, ref)
	}
	if _, err := prepareTUISessionSnapshot(TUIREPLConfig{Repo: repo}, sessionID, projectDir, 1, messages); err != nil {
		t.Fatalf("empty session checkpoint was not readable: %v", err)
	}
}

func TestTUISessionLifecycleRestoresLatestContextWithoutLosingTotals(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "usage-session"
	state := tuiapp.NewAppState()
	state.SessionID.Set(sessionID)
	state.SessionNS.Set(projectDir)
	state.AccumulateSessionUsage(&types.Usage{InputTokens: 1000, OutputTokens: 120, CacheReadInputTokens: 400})
	state.AccumulateSessionUsage(&types.Usage{InputTokens: 1500, OutputTokens: 80, CacheReadInputTokens: 600})
	state.MarkSessionCompacted()
	state.AccumulateSessionUsage(&types.Usage{InputTokens: 700, OutputTokens: 60, CacheReadInputTokens: 200})
	state.AccumulateSessionUsage(&types.Usage{InputTokens: 900, OutputTokens: 70, CacheReadInputTokens: 450})
	state.MarkSessionCompacted()
	state.AccumulateSessionUsage(&types.Usage{InputTokens: 600, OutputTokens: 40, CacheReadInputTokens: 300})
	state.CumulativeCost.Set(1.23)

	cfg := TUIREPLConfig{Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir}
	if err := persistTUISessionLifecycle(cfg, state); err != nil {
		t.Fatal(err)
	}
	snapshot, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Usage.InputTokens != 4700 || snapshot.Usage.OutputTokens != 370 || snapshot.Usage.CacheReadTokens != 1950 {
		t.Fatalf("restored cumulative usage = %+v, want input/output/cache 4700/370/1950", snapshot.Usage)
	}
	if snapshot.Usage.LastInputTokens != 600 || snapshot.Usage.LastOutputTokens != 40 || snapshot.Usage.LastCacheReadTokens != 300 {
		t.Fatalf("restored latest context = %+v, want input/output/cache 600/40/300", snapshot.Usage)
	}
	if !snapshot.Usage.RoundUsageKnown || !snapshot.Usage.HasCompacted || snapshot.Usage.CompactionCount != 2 ||
		snapshot.Usage.CompletedRoundInputTokens != 2400 || snapshot.Usage.CompletedRoundOutputTokens != 150 {
		t.Fatalf("restored completed round usage = %+v, want count/input/output 2/2400/150", snapshot.Usage)
	}
	if snapshot.Usage.CumulativeCost != 1.23 {
		t.Fatalf("restored cumulative cost = %f, want 1.23", snapshot.Usage.CumulativeCost)
	}
}

func TestClearTUIConversationCommitsEmptyEngineAndPresentationAndPreservesOldAudit(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	oldID := "old-session"
	oldMessages := []types.Message{types.UserMessage("retain me")}
	if err := repo.Save(oldID, projectDir, oldMessages); err != nil {
		t.Fatal(err)
	}
	sessions := &sessionSwitcherTestSessions{messages: map[string][]types.Message{oldID: oldMessages}}
	eng := &clearGenerationEngine{sessionSwitcherTestEngine: &sessionSwitcherTestEngine{sessions: sessions}}
	state := tuiapp.NewAppState()
	state.SessionID.Set(oldID)
	state.SessionNS.Set(projectDir)
	state.SessionEpoch.Set(3)
	bindViewFidelityStateToCurrentGeneration(t, repo, state, oldID, projectDir)
	state.AppendMessage(tuiapp.Message{Kind: tuiapp.MsgUser, Text: "retain me"})
	state.DecisionHistory.Set([]tuiapp.DecisionRecord{{
		Prompt:     permissions.PromptRequest{DecisionID: "audit-1", Action: "write"},
		Response:   permissions.PromptResponse{DecisionID: "audit-1", Outcome: permissions.PromptOutcomeRejected},
		ResolvedAt: time.Unix(10, 0),
	}})
	app := directActivityApp{state: state}
	runtimeScope := runtimescope.NewRuntimeScope(cwd, true)
	deps := &RegistryDeps{
		AgentTool: &agentruntime.AgentTool{}, TeamManager: toolcollaboration.NewTeamManager(nil), RuntimeScope: runtimeScope,
		SkillManager: newRegistryTestSkillManager(t, cwd),
	}
	deps.BindSessionIdentity(oldID)
	deps.UpdateSessionContext(cwd, []string{cwd})
	transitionMu := &sync.Mutex{}
	usageRestoredUnderLock := false
	cfg := TUIREPLConfig{
		Engine: eng, Repo: repo, SessionID: &oldID, SessionProjectDir: &projectDir, CWD: &cwd,
		SessionTransitionMu: transitionMu, PermChecker: permissions.NewChecker(permissions.ModeAskAlways, nil),
		PublishSessionID: deps.PublishSessionID,
		RestoreSessionUsage: func(tuiapp.SessionUsage) {
			if transitionMu.TryLock() {
				transitionMu.Unlock()
				t.Error("usage restored after transition lock was released")
				return
			}
			usageRestoredUnderLock = true
		},
	}

	newID, err := clearTUIConversation(context.Background(), cfg, app)
	if err != nil {
		t.Fatal(err)
	}
	if newID == "" || oldID != newID || state.SessionID.Get() != newID || deps.CurrentSessionID() != newID {
		t.Fatalf("clear identity = return %q pointer %q state %q registry %q", newID, oldID, state.SessionID.Get(), deps.CurrentSessionID())
	}
	if agentRuntime, scope := deps.AgentTool.SessionRuntime(), deps.RuntimeScope.ToolRuntimeContext(); agentRuntime.ToolRuntime.SessionID != newID || scope.SessionID != newID {
		t.Fatalf("clear tool-side identity disagrees: agent=%+v scope=%+v", agentRuntime, scope)
	}
	if len(eng.transcript) != 0 || len(state.Messages.Get()) != 0 || state.SessionEpoch.Get() != 4 {
		t.Fatalf("clear projections disagree: engine=%#v ui=%#v epoch=%d", eng.transcript, state.Messages.Get(), state.SessionEpoch.Get())
	}
	if state.ContextGeneration.Get() != 1 || !state.ContextGenerationPersisted.Get() {
		t.Fatalf("clear context generation = %d persisted=%v, want authoritative generation 1", state.ContextGeneration.Get(), state.ContextGenerationPersisted.Get())
	}
	if !usageRestoredUnderLock {
		t.Fatal("clear did not restore tracker usage within the transition")
	}
	oldPersisted, _, err := repo.LoadByID("old-session", projectDir)
	if err != nil || len(oldPersisted) != 1 || oldPersisted[0].GetText() != "retain me" {
		t.Fatalf("old transcript not recoverable: messages=%#v err=%v", oldPersisted, err)
	}
	oldView, restored, err := tuiapp.LoadSessionViewCheckpoint(
		repo.ArtifactsDir("old-session", projectDir), oldPersisted,
		tuiapp.SessionIdentity{Namespace: projectDir, SessionID: "old-session", Epoch: 3},
	)
	if err != nil || !restored || len(oldView.Decisions) != 1 || oldView.Decisions[0].Prompt.DecisionID != "audit-1" {
		t.Fatalf("old decision audit not recoverable: snapshot=%+v restored=%v err=%v", oldView.Decisions, restored, err)
	}
}

func TestPrepareEmptyTUISessionCleansPartiallySavedCandidate(t *testing.T) {
	const candidateID = "partial-save-candidate"
	sessions := &partialSaveSessionManager{saved: make(map[string]bool)}
	cfg := TUIREPLConfig{Engine: partialSaveEngine{sessions: sessions}}

	if _, _, err := prepareEmptyTUISession(context.Background(), cfg, candidateID, 2); err == nil {
		t.Fatal("partial session save unexpectedly succeeded")
	}
	if sessions.saved[candidateID] {
		t.Fatal("failed clear preparation left a partially persisted candidate session")
	}
}

func TestPrepareInitialTUISessionCreatesCanonicalNewSession(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "fresh-startup-session"
	sessions := &sessionSwitcherTestSessions{messages: make(map[string][]types.Message)}
	eng := &clearGenerationEngine{sessionSwitcherTestEngine: &sessionSwitcherTestEngine{sessions: sessions}}
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	cfg := TUIREPLConfig{
		Engine: eng, Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir,
		CWD: &cwd, PermChecker: checker,
	}

	snapshot, err := prepareInitialTUISessionSnapshot(
		context.Background(), cfg, sessionID, projectDir, 1, nil, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Identity.SessionID != sessionID || snapshot.Identity.Namespace != projectDir {
		t.Fatalf("fresh snapshot identity = %+v", snapshot.Identity)
	}
	if snapshot.PermissionMode != tuiapp.ModeAskEdit {
		t.Fatalf("fresh permission mode = %v, want ask", snapshot.PermissionMode)
	}
	if !snapshot.SessionCostKnown {
		t.Fatal("fresh session initialized with unknown model cost")
	}
	if snapshot.ContextGeneration != 1 || !snapshot.ContextGenerationPersisted {
		t.Fatalf("fresh context generation = %d persisted=%v", snapshot.ContextGeneration, snapshot.ContextGenerationPersisted)
	}
	if eng.commits != 1 {
		t.Fatalf("fresh engine commits = %d, want 1", eng.commits)
	}
	if _, ok := sessions.messages[sessionID]; !ok {
		t.Fatal("fresh transcript was not persisted")
	}
	restored, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, nil)
	if err != nil {
		t.Fatalf("fresh checkpoint was not durably restorable: %v", err)
	}
	if !restored.SessionCostKnown {
		t.Fatal("fresh checkpoint lost known-cost state")
	}
}

func TestPrepareInitialTUISessionDoesNotRebuildPersistedMissingCheckpoint(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "persisted-without-view"
	if err := repo.Save(sessionID, projectDir, nil); err != nil {
		t.Fatal(err)
	}

	_, err := prepareInitialTUISessionSnapshot(
		context.Background(), TUIREPLConfig{Repo: repo}, sessionID, projectDir, 1, nil, false,
	)
	if err == nil {
		t.Fatal("persisted session without a checkpoint unexpectedly loaded")
	}
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyTUISessionViewMissingCheckpoint {
		t.Fatalf("persisted missing-checkpoint error = %#v, ok=%v, err=%v", info, ok, err)
	}
}

func TestTransitionTUISessionRollbackFailurePublishesCoherentTarget(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	currentID := "old-session"
	for id, text := range map[string]string{"old-session": "old transcript", "target-session": "target transcript"} {
		messages := []types.Message{types.UserMessage(text)}
		if err := repo.Save(id, projectDir, messages); err != nil {
			t.Fatal(err)
		}
		if id == "target-session" {
			saveCanonicalTUISessionCheckpoint(t, repo, id, projectDir, messages, 1, tuiapp.DurableSessionView{PermissionMode: tuiapp.ModePlanEdit})
		}
	}
	state := tuiapp.NewAppState()
	state.SessionID.Set(currentID)
	state.SessionNS.Set(projectDir)
	state.SessionEpoch.Set(1)
	state.AppendMessage(tuiapp.Message{Kind: tuiapp.MsgUser, Text: "old transcript"})
	app := directActivityApp{state: state}
	runtimeScope := runtimescope.NewRuntimeScope(cwd, true)
	runtimeScope.SetPermissionModeDispatcher(func() string { return "default" }, func(mode string) error {
		if mode == "plan" {
			return errors.New("plan mode unavailable")
		}
		return nil
	})
	engineActive := currentID
	switchCalls := 0
	cfg := TUIREPLConfig{
		Repo: repo, SessionID: &currentID, SessionProjectDir: &projectDir, CWD: &cwd,
		SessionTransitionMu: &sync.Mutex{}, RuntimeScope: runtimeScope,
		SwitchSession: func(_ context.Context, entry commands.SessionListEntry) error {
			switchCalls++
			if switchCalls == 2 {
				return errors.New("old session reload failed")
			}
			engineActive = entry.ID
			currentID = entry.ID
			return nil
		},
	}
	store := &sessionStoreAdapter{repo: repo, currentProjectDir: func() string { return projectDir }}
	err := transitionTUISession(context.Background(), cfg, app, store, commands.SessionListEntry{ID: "target-session", ProjectDir: projectDir, CWD: cwd})
	if err == nil || !strings.Contains(err.Error(), "retained coherently") {
		t.Fatalf("transition error = %v", err)
	}
	if engineActive != "target-session" || currentID != "target-session" || state.SessionID.Get() != "target-session" {
		t.Fatalf("degraded target identity disagrees: engine=%q pointer=%q state=%q", engineActive, currentID, state.SessionID.Get())
	}
	messages := state.Messages.Get()
	if len(messages) != 1 || messages[0].Text != "target transcript" || state.Mode.Get() != tuiapp.ModeAskEdit {
		t.Fatalf("degraded target projection = messages:%#v mode:%v", messages, state.Mode.Get())
	}
}

func TestTransitionPermissionAndRollbackModeFailureFailsClosed(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	currentID := "old-session"
	for id, text := range map[string]string{"old-session": "old transcript", "target-session": "target transcript"} {
		messages := []types.Message{types.UserMessage(text)}
		if err := repo.Save(id, projectDir, messages); err != nil {
			t.Fatal(err)
		}
		if id == "target-session" {
			saveCanonicalTUISessionCheckpoint(t, repo, id, projectDir, messages, 1, tuiapp.DurableSessionView{PermissionMode: tuiapp.ModePlanEdit})
		}
	}
	state := tuiapp.NewAppState()
	state.SessionID.Set(currentID)
	state.SessionNS.Set(projectDir)
	state.SessionEpoch.Set(1)
	state.Mode.Set(tuiapp.ModeAskEdit)
	app := directActivityApp{state: state}
	runtimeScope := runtimescope.NewRuntimeScope(cwd, true)
	runtimeScope.SetPermissionModeDispatcher(func() string { return "default" }, func(mode string) error {
		return fmt.Errorf("permission dispatcher rejected %s", mode)
	})
	activeEngineID := currentID
	failClosed := false
	cfg := TUIREPLConfig{
		Repo: repo, SessionID: &currentID, SessionProjectDir: &projectDir, CWD: &cwd,
		SessionTransitionMu: &sync.Mutex{}, RuntimeScope: runtimeScope,
		SwitchSession: func(_ context.Context, entry commands.SessionListEntry) error {
			activeEngineID = entry.ID
			currentID = entry.ID
			return nil
		},
		FailClosed: func(error) { failClosed = true },
	}
	store := &sessionStoreAdapter{repo: repo, currentProjectDir: func() string { return projectDir }}
	err := transitionTUISession(context.Background(), cfg, app, store, commands.SessionListEntry{ID: "target-session", ProjectDir: projectDir, CWD: cwd})
	if err == nil || !strings.Contains(err.Error(), "mode rollback failed") {
		t.Fatalf("transition error = %v", err)
	}
	if !failClosed {
		t.Fatal("permission restore plus rollback failure did not fail closed")
	}
	if activeEngineID != "old-session" || currentID != "old-session" || state.SessionID.Get() != "old-session" || state.Mode.Get() != tuiapp.ModeAskEdit {
		t.Fatalf("failed-closed old projection disagrees: engine=%q pointer=%q state=%q mode=%v", activeEngineID, currentID, state.SessionID.Get(), state.Mode.Get())
	}
}

func TestClearCommitAndPermissionRollbackFailureFailsClosed(t *testing.T) {
	activeID := "old-session"
	commitErr := errors.New("commit unavailable")
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	base := &sessionSwitcherTestEngine{
		sessions:  &sessionSwitcherTestSessions{messages: map[string][]types.Message{activeID: {types.UserMessage("old")}}},
		commitErr: commitErr,
	}
	state := tuiapp.NewAppState()
	state.SessionID.Set(activeID)
	state.Mode.Set(tuiapp.ModeAskEdit)
	app := directActivityApp{state: state}
	runtimeMode := "default"
	runtimeScope := runtimescope.NewRuntimeScope(t.TempDir(), true)
	runtimeScope.SetPermissionModeDispatcher(func() string { return runtimeMode }, func(mode string) error {
		if mode == "default" && runtimeMode == "bypassPermissions" {
			return errors.New("default permission runtime unavailable")
		}
		runtimeMode = mode
		return nil
	})
	failClosed := false
	cfg := TUIREPLConfig{
		Engine: screenReaderClearEngine{sessionSwitcherTestEngine: base}, Repo: repo, SessionID: &activeID, SessionProjectDir: &projectDir, CWD: &cwd,
		RuntimeScope: runtimeScope, SessionTransitionMu: &sync.Mutex{},
		FailClosed: func(error) { failClosed = true },
	}

	_, err := clearTUIConversation(context.Background(), cfg, app)
	if !errors.Is(err, commitErr) || !strings.Contains(err.Error(), "failed closed") {
		t.Fatalf("clear error = %v, want commit failure with fail-closed receipt", err)
	}
	if !failClosed {
		t.Fatal("double rollback failure did not stop further runtime work")
	}
	if activeID != "old-session" || state.SessionID.Get() != "old-session" {
		t.Fatalf("failed clear changed identity: pointer=%q state=%q", activeID, state.SessionID.Get())
	}
	if runtimeMode != "bypassPermissions" || runtimeScope.PermissionMode() != "bypassPermissions" || state.Mode.Get() != tuiapp.ModeAutoEdit {
		t.Fatalf("surviving permission mode was hidden: dispatcher=%q scope=%q UI=%v", runtimeMode, runtimeScope.PermissionMode(), state.Mode.Get())
	}
	if len(base.sessions.messages) != 1 {
		t.Fatalf("failed clear retained candidate session: %#v", base.sessions.messages)
	}
}

func TestDisclosureCloseRestoresPersistedReturnPointAfterResume(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	const sessionID = "disclosure-return"
	if err := repo.Save(sessionID, projectDir, nil); err != nil {
		t.Fatal(err)
	}
	details, err := tuiapp.NewFileDetailStore(repo.ArtifactsDir(sessionID, projectDir) + string(os.PathSeparator) + "tui-details")
	if err != nil {
		t.Fatal(err)
	}
	observations := tuiapp.NewObservationStore(details)
	ctx := tuiapp.ToolEventContext{SessionID: sessionID, TurnID: "turn", Outcome: tuiapp.OutcomeSucceeded}
	if err := observations.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool-return", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	if err := observations.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: "tool-return", Content: "evidence"}); err != nil {
		t.Fatal(err)
	}
	state := tuiapp.NewAppState()
	if err := state.ApplySessionSnapshot(tuiapp.SessionSnapshot{
		Identity:   tuiapp.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1},
		Projection: tuiapp.SessionProjection{Details: details, Observations: observations.Snapshot()},
		DurableSessionView: tuiapp.DurableSessionView{
			Interaction: tuiapp.SessionInteraction{FocusedObservationID: "prior-focus", ScrollAnchorID: "prior-anchor", ScrollOffset: 11, InputDraft: "before"}, PermissionMode: tuiapp.ModeAskEdit,
		},
	}); err != nil {
		t.Fatal(err)
	}
	bindViewFidelityStateToCurrentGeneration(t, repo, state, sessionID, projectDir)
	observationID := state.ObservationSnapshot()[0].ID
	if err := state.RevealObservation(observationID, tuiapp.DisclosureEvidence); err != nil {
		t.Fatal(err)
	}
	state.SetInteractionDraft("edited while expanded")
	activeID := sessionID
	cfg := TUIREPLConfig{Repo: repo, SessionID: &activeID, SessionProjectDir: &projectDir}
	if err := persistTUISessionLifecycle(cfg, state); err != nil {
		t.Fatal(err)
	}

	snapshot, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	resumed := tuiapp.NewAppState()
	if err := resumed.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := resumed.RevealObservation(observationID, tuiapp.DisclosureSummary); err != nil {
		t.Fatal(err)
	}
	want := (tuiapp.SessionInteraction{
		FocusedObservationID: "prior-focus", ScrollAnchorID: "prior-anchor", ScrollOffset: 11,
		InputDraft: "edited while expanded", InputCursor: 21, InputCursorSet: true,
	})
	if got := resumed.ActiveSessionInteraction(); got != want {
		t.Fatalf("resumed disclosure close = %+v, want %+v", got, want)
	}
}

func TestApplyTUISessionPermissionModeSynchronizesPlanState(t *testing.T) {
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	plan, err := toolinteraction.NewPlanState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := TUIREPLConfig{PermChecker: checker, PlanState: plan}
	if err := applyTUISessionPermissionMode(cfg, tuiapp.ModePlanEdit); err != nil {
		t.Fatal(err)
	}
	if !plan.IsActive() || checker.Mode() != permissions.ModeAskAlways {
		t.Fatalf("plan restore = active %v, permission %v", plan.IsActive(), checker.Mode())
	}
	if err := applyTUISessionPermissionMode(cfg, tuiapp.ModeAutoEdit); err != nil {
		t.Fatal(err)
	}
	if plan.IsActive() || checker.Mode() != permissions.ModeAllowAll {
		t.Fatalf("auto restore = active %v, permission %v", plan.IsActive(), checker.Mode())
	}
}

type structuredToolRecorder struct {
	ui.QuietRenderer
	calls       []types.ToolUseBlock
	callContext []presentation.ToolEventContext
	results     []types.ToolResultBlock
	resultCtx   []presentation.ToolEventContext
}

type epochRejectingRecorder struct {
	structuredToolRecorder
	admitted presentation.ToolEventContext
}

type progressActivityRecorder struct {
	structuredToolRecorder
	epochs         []uint64
	events         []tuiapp.ActivityEvent
	boundaryEpochs []uint64
	boundaries     []stream.CompactBoundaryEvent
}

func (r *progressActivityRecorder) ActivityAtEpoch(epoch uint64, event tuiapp.ActivityEvent) {
	r.epochs = append(r.epochs, epoch)
	r.events = append(r.events, event)
}

func (r *progressActivityRecorder) CompactionBoundaryAtEpoch(epoch uint64, _ presentation.ToolEventContext, boundary stream.CompactBoundaryEvent) {
	r.boundaryEpochs = append(r.boundaryEpochs, epoch)
	r.boundaries = append(r.boundaries, boundary)
}

func (r *epochRejectingRecorder) AdmitContextGeneration(ctx presentation.ToolEventContext) bool {
	r.admitted = ctx
	return false
}

func (r *structuredToolRecorder) RenderToolCall(ctx presentation.ToolEventContext, call types.ToolUseBlock) {
	r.callContext = append(r.callContext, ctx)
	r.calls = append(r.calls, call)
}

func TestTUIEventHandlerRejectsStaleEpochBeforeDispatch(t *testing.T) {
	renderer := &epochRejectingRecorder{}
	handle, cleanup := makeTUIEventHandler(renderer, nil, nil, presentation.ToolEventContext{
		SessionID: "old-session", SessionEpoch: 3,
	})
	t.Cleanup(cleanup)

	handle(stream.Event{Type: stream.EventToolUse, TurnCount: 1, ToolUse: &types.ToolUseBlock{ID: "stale", Name: "Read"}})
	if renderer.admitted.SessionID != "old-session" || renderer.admitted.SessionEpoch != 3 {
		t.Fatalf("context-generation fence received %+v", renderer.admitted)
	}
	if len(renderer.calls) != 0 {
		t.Fatalf("stale event reached presentation: %+v", renderer.calls)
	}
}

func (r *structuredToolRecorder) RenderToolResult(ctx presentation.ToolEventContext, result types.ToolResultBlock) {
	r.resultCtx = append(r.resultCtx, ctx)
	r.results = append(r.results, result)
}

func TestTUIEventHandlerPreservesStructuredToolIdentity(t *testing.T) {
	renderer := &structuredToolRecorder{}
	base := presentation.ToolEventContext{SessionID: "session-a", SessionEpoch: 7}
	handle, cleanup := makeTUIEventHandler(renderer, nil, nil, base)
	t.Cleanup(cleanup)

	handle(stream.Event{
		Type:       stream.EventToolUse,
		TurnCount:  4,
		ActorID:    "agent-reviewer",
		ActorType:  "reviewer",
		WorkUnitID: "work-review",
		ToolUse: &types.ToolUseBlock{
			ID:    "toolu-a",
			Name:  "Read",
			Input: map[string]any{"file_path": "a.go"},
		},
	})
	handle(stream.Event{
		Type:       stream.EventToolResult,
		TurnCount:  4,
		ActorID:    "agent-reviewer",
		ActorType:  "reviewer",
		WorkUnitID: "work-review",
		ToolResult: &types.ToolResultBlock{
			ToolUseID: "toolu-a",
			Content:   "full evidence",
		},
	})

	if len(renderer.calls) != 1 || len(renderer.results) != 1 {
		t.Fatalf("structured events = calls:%d results:%d, want 1 each", len(renderer.calls), len(renderer.results))
	}
	if renderer.calls[0].ID != "toolu-a" || renderer.results[0].ToolUseID != "toolu-a" {
		t.Fatalf("tool identity was lost: call=%q result=%q", renderer.calls[0].ID, renderer.results[0].ToolUseID)
	}
	for _, ctx := range append(renderer.callContext, renderer.resultCtx...) {
		if ctx.SessionID != "session-a" || ctx.SessionEpoch != 7 || ctx.TurnID != "session-a:turn-4" {
			t.Fatalf("session/turn context = %+v", ctx)
		}
		if ctx.ActorID != "agent-reviewer" || ctx.ActorType != "reviewer" || ctx.WorkUnitID != "work-review" {
			t.Fatalf("actor/work-unit context = %+v", ctx)
		}
	}
}

func TestTUIEventHandlerMapsCompactionProgressToStableActivity(t *testing.T) {
	renderer := &progressActivityRecorder{}
	base := presentation.ToolEventContext{SessionID: "session-progress", SessionEpoch: 9}
	handle, cleanup := makeTUIEventHandler(renderer, nil, nil, base)
	t.Cleanup(cleanup)

	handle(stream.Event{Type: stream.EventProgress, TurnCount: 3, Progress: &stream.ProgressEvent{Stage: "compact_start", Message: "compacting"}})
	handle(stream.Event{Type: stream.EventProgress, TurnCount: 3, Progress: &stream.ProgressEvent{Stage: "compact_end", Message: "idle"}})
	if len(renderer.events) != 2 || renderer.epochs[0] != 9 || renderer.epochs[1] != 9 {
		t.Fatalf("compaction progress events = %+v epochs=%v", renderer.events, renderer.epochs)
	}
	first, last := renderer.events[0], renderer.events[1]
	if first.ID == "" || first.ID != last.ID || first.SessionID != base.SessionID || first.TurnID == "" {
		t.Fatalf("compaction progress identity is unstable: first=%+v last=%+v", first, last)
	}
	if first.Lifecycle != tuiapp.ActivityLifecycleRunning || first.Outcome != tuiapp.OutcomeRunning || last.Lifecycle != tuiapp.ActivityLifecycleCompleted || last.Outcome != tuiapp.OutcomeSucceeded {
		t.Fatalf("compaction progress lifecycle = %s/%s then %s/%s", first.Lifecycle, first.Outcome, last.Lifecycle, last.Outcome)
	}
}

func TestTUIEventHandlerForwardsCompleteCompactionBoundary(t *testing.T) {
	renderer := &progressActivityRecorder{}
	base := presentation.ToolEventContext{SessionID: "session-progress", SessionEpoch: 9, TurnID: "turn-3"}
	handle, cleanup := makeTUIEventHandler(renderer, nil, nil, base)
	t.Cleanup(cleanup)

	boundary := stream.CompactBoundaryEvent{
		Trigger: "reactive", PreCompactTokenCount: 1200, PostCompactTokenCount: 300,
		TruePostCompactTokenCount: 280, PreviousTailIdentifier: "assistant:tail",
	}
	handle(stream.Event{Type: stream.EventCompactBoundary, TurnID: "turn-3", Compact: &boundary})

	if len(renderer.boundaries) != 1 || len(renderer.boundaryEpochs) != 1 {
		t.Fatalf("compaction boundaries = %+v epochs=%v, want one forwarded boundary", renderer.boundaries, renderer.boundaryEpochs)
	}
	got := renderer.boundaries[0]
	if renderer.boundaryEpochs[0] != 9 || got.Trigger != boundary.Trigger || got.PreCompactTokenCount != boundary.PreCompactTokenCount || got.PostCompactTokenCount != boundary.PostCompactTokenCount || got.TruePostCompactTokenCount != boundary.TruePostCompactTokenCount || got.PreviousTailIdentifier != boundary.PreviousTailIdentifier {
		t.Fatalf("forwarded boundary = %+v epoch=%d, want %+v epoch=9", renderer.boundaries[0], renderer.boundaryEpochs[0], boundary)
	}
}

func TestManualCompactionEmitsOneStableLifecycleAndCompleteBoundary(t *testing.T) {
	sessionID := "session-manual"
	before := []types.Message{types.UserMessage(strings.Repeat("before ", 80)), types.AssistantMessage("tail")}
	marker := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{
		Trigger: "manual", PreCompactTokenCount: 1200, PreviousTailIdentifier: "assistant:tail",
		PreCompactDiscoveredTools: []string{"Read"},
		PreservedSegment:          &compact.PreservedSegmentMetadata{StartIndex: 1, Count: 1, Anchor: "assistant:tail", Direction: "tail"},
	})
	after := []types.Message{marker, types.UserMessage("This session is being continued from a previous conversation. Summary of conversation: compacted"), types.AssistantMessage("tail")}
	sessions := &sessionSwitcherTestSessions{messages: map[string][]types.Message{sessionID: before}}
	eng := &manualCompactionTestEngine{
		sessions: sessions,
		after:    after,
		boundaryEvent: &stream.Event{Type: stream.EventCompactBoundary, Compact: &stream.CompactBoundaryEvent{
			Trigger: "manual", PreCompactTokenCount: 1200, PostCompactTokenCount: 320,
			TruePostCompactTokenCount: 300, PreviousTailIdentifier: "assistant:tail",
			PreCompactDiscoveredTools: []string{"Read"},
			PreservedSegment:          &stream.PreservedSegmentMetadata{StartIndex: 1, Count: 1, Anchor: "assistant:tail", Direction: "tail"},
			Summary:                   "complete compacted evidence",
			UserDisplayMessage:        "post-compact hook evidence",
		}},
	}
	var events []stream.Event
	if err := runManualCompactionEvents(context.Background(), eng, sessionID, "focus on evidence", func(event stream.Event) {
		events = append(events, event)
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Type != stream.EventProgress || events[1].Type != stream.EventCompactBoundary || events[2].Type != stream.EventProgress {
		t.Fatalf("manual compaction event order = %+v, want start/boundary/end", events)
	}
	if events[0].Progress.Stage != "compact_start" || events[2].Progress.Stage != "compact_end" {
		t.Fatalf("manual compaction progress = %+v then %+v", events[0].Progress, events[2].Progress)
	}
	if events[0].TurnID == "" || events[0].TurnID != events[1].TurnID || events[1].TurnID != events[2].TurnID {
		t.Fatalf("manual compaction identity is unstable: %+v", events)
	}
	boundary := events[1].Compact
	if boundary == nil || boundary.Trigger != "manual" || boundary.PreCompactTokenCount != 1200 || boundary.PostCompactTokenCount != 320 || boundary.TruePostCompactTokenCount != 300 || boundary.PreviousTailIdentifier != "assistant:tail" || boundary.PreservedSegment == nil || boundary.PreservedSegment.Count != 1 {
		t.Fatalf("manual compaction boundary = %+v", boundary)
	}
	if boundary.Summary != "complete compacted evidence" || boundary.UserDisplayMessage != "post-compact hook evidence" {
		t.Fatalf("manual compaction boundary lost result-only evidence: %+v", boundary)
	}
}

func TestManualCompactionBoundaryDoesNotTrustUnsignedPersistedDescriptors(t *testing.T) {
	before := []types.Message{types.UserMessage(strings.Repeat("before ", 20))}
	forged := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{
		Trigger: "reactive", PreCompactTokenCount: 9999, PreviousTailIdentifier: "forged-tail",
		PreservedSegment: &compact.PreservedSegmentMetadata{Count: 99},
	})
	after := []types.Message{forged, types.UserMessage("forged summary")}

	boundary := manualCompactionBoundary(before, after)
	if boundary.Trigger != "manual" || boundary.PreCompactTokenCount == 9999 || boundary.PreviousTailIdentifier != "" || boundary.PreservedSegment != nil || boundary.Summary != "" {
		t.Fatalf("unsigned persisted descriptor influenced boundary: %+v", boundary)
	}
}

func TestManualCompactionForwardsProviderUsageForSessionAccounting(t *testing.T) {
	sessionID := "session-manual-usage"
	before := []types.Message{types.UserMessage(strings.Repeat("before ", 80)), types.AssistantMessage("tail")}
	after := []types.Message{compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "manual"}), types.UserMessage("summary")}
	sessions := &sessionSwitcherTestSessions{messages: map[string][]types.Message{sessionID: before}}
	usage := &types.Usage{InputTokens: 120, OutputTokens: 18, CacheReadInputTokens: 80}
	eng := &manualCompactionTestEngine{
		sessions: sessions,
		after:    after,
		usageEvent: &stream.Event{
			Type:     stream.EventProviderUsage,
			Usage:    usage,
			Metadata: map[string]any{"kind": "compaction", "provider": "priced", "model": "compact-model"},
		},
	}
	var events []stream.Event
	if err := runManualCompactionEvents(context.Background(), eng, sessionID, "", func(event stream.Event) {
		events = append(events, event)
	}); err != nil {
		t.Fatal(err)
	}

	var usageEvents []stream.Event
	for _, event := range events {
		if event.Type == stream.EventProviderUsage {
			usageEvents = append(usageEvents, event)
		}
	}
	if len(usageEvents) != 1 || usageEvents[0].Usage == nil || *usageEvents[0].Usage != *usage {
		t.Fatalf("manual compaction usage events = %+v, want exactly %+v", usageEvents, *usage)
	}
}

func TestManualCompactionEmitsDistinctFailureAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, wantStage string
		err             error
	}{
		{name: "failed", wantStage: "compact_failed", err: errors.New("summary failed")},
		{name: "cancelled", wantStage: "compact_cancelled", err: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := "session-" + tc.name
			sessions := &sessionSwitcherTestSessions{messages: map[string][]types.Message{sessionID: {types.UserMessage("before")}}}
			eng := &manualCompactionTestEngine{sessions: sessions, err: tc.err}
			var events []stream.Event
			err := runManualCompactionEvents(context.Background(), eng, sessionID, "", func(event stream.Event) { events = append(events, event) })
			if !errors.Is(err, tc.err) {
				t.Fatalf("manual compaction error = %v, want %v", err, tc.err)
			}
			if len(events) != 2 || events[1].Progress == nil || events[1].Progress.Stage != tc.wantStage || events[1].Progress.Metadata["error"] != tc.err.Error() {
				t.Fatalf("manual compaction terminal events = %+v", events)
			}
		})
	}
}

func TestCompactionProgressActivityPreservesDistinctTerminalOutcomes(t *testing.T) {
	ctx := presentation.ToolEventContext{SessionID: "session-progress", TurnID: "turn-3"}
	tests := []struct {
		stage         string
		wantLifecycle tuiapp.ActivityLifecycle
		wantOutcome   tuiapp.ObservationOutcome
	}{
		{stage: "compact_end", wantLifecycle: tuiapp.ActivityLifecycleCompleted, wantOutcome: tuiapp.OutcomeSucceeded},
		{stage: "compact_failed", wantLifecycle: tuiapp.ActivityLifecycleFailed, wantOutcome: tuiapp.OutcomeFailed},
		{stage: "compact_cancelled", wantLifecycle: tuiapp.ActivityLifecycleCancelled, wantOutcome: tuiapp.OutcomeCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			activity, ok := compactionProgressActivity(ctx, &stream.ProgressEvent{Stage: tt.stage, Message: tt.stage})
			if !ok {
				t.Fatalf("compaction progress stage %q was not mapped", tt.stage)
			}
			if activity.Lifecycle != tt.wantLifecycle || activity.Outcome != tt.wantOutcome {
				t.Fatalf("compaction progress %q = %s/%s, want %s/%s", tt.stage, activity.Lifecycle, activity.Outcome, tt.wantLifecycle, tt.wantOutcome)
			}
			if activity.Control.Cancelable {
				t.Fatalf("terminal compaction progress %q remained cancellable", tt.stage)
			}
		})
	}
}
