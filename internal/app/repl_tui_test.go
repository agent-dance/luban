package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agent "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"

	agentruntime "github.com/agent-dance/luban/internal/agent"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/internal/contracts/stream"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/internal/ui/tui"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type stubTUIPermissionRequester struct {
	mu        sync.Mutex
	toolName  string
	input     map[string]any
	calls     int
	active    int
	maxActive int
}

func (s *stubTUIPermissionRequester) DecisionRequest(_ context.Context, request permissions.PromptRequest) permissions.PromptResponse {
	s.mu.Lock()
	s.toolName = request.ToolName
	s.input = request.Input
	s.calls++
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	s.mu.Unlock()

	time.Sleep(5 * time.Millisecond)

	s.mu.Lock()
	s.active--
	s.mu.Unlock()
	return permissions.PromptResponse{DecisionID: request.DecisionID, Decision: permissions.DecisionAllowOnce, Outcome: permissions.PromptOutcomeApproved}
}

func TestInstallTUIPermissionPromptRoutesCheckerToTUI(t *testing.T) {
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	requester := &stubTUIPermissionRequester{}
	installTUIPermissionPrompt(checker, requester)

	decision := checker.CheckPrompt(context.Background(), permissions.PromptRequest{
		DecisionID: "test.permission.write", ToolName: "Write", Input: map[string]any{"file_path": "out.txt"}, Kind: permissions.PromptKindPermission,
	}, permissions.CheckOptions{}).Decision
	if decision != permissions.DecisionAllow {
		t.Fatalf("expected checker to allow once-approved TUI prompt, got %v", decision)
	}

	requester.mu.Lock()
	defer requester.mu.Unlock()
	if requester.calls != 1 {
		t.Fatalf("expected one TUI permission request, got %d", requester.calls)
	}
	if requester.toolName != "Write" {
		t.Fatalf("expected tool Write, got %q", requester.toolName)
	}
	if requester.input["file_path"] != "out.txt" {
		t.Fatalf("expected input file_path out.txt, got %v", requester.input["file_path"])
	}
}

func TestInstallTUIPermissionPromptSerializesConcurrentPrompts(t *testing.T) {
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	requester := &stubTUIPermissionRequester{}
	installTUIPermissionPrompt(checker, requester)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			decision := checker.CheckPrompt(context.Background(), permissions.PromptRequest{
				DecisionID: fmt.Sprintf("test.permission.write.%d", i), ToolName: "Write",
				Input: map[string]any{"file_path": fmt.Sprintf("out-%d.txt", i)}, Kind: permissions.PromptKindPermission,
			}, permissions.CheckOptions{}).Decision
			if decision != permissions.DecisionAllow {
				t.Errorf("expected prompt %d to allow, got %v", i, decision)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	requester.mu.Lock()
	defer requester.mu.Unlock()
	if requester.maxActive != 1 {
		t.Fatalf("expected serialized TUI prompts, saw %d active prompts", requester.maxActive)
	}
	if requester.calls != 8 {
		t.Fatalf("expected 8 prompts, got %d", requester.calls)
	}
}

type recordingTUIInfoRenderer struct {
	messages chan string
}

func (r recordingTUIInfoRenderer) Info(message string) {
	r.messages <- message
}

type sessionAwareRecordingTUIInfoRenderer struct {
	recordingTUIInfoRenderer
	sessionID string
}

func (r sessionAwareRecordingTUIInfoRenderer) VisibleSessionID() string { return r.sessionID }

type stateAwareRecordingTUIInfoRenderer struct {
	state     *tui.AppState
	sessionID string
	messages  chan string
}

func (r *stateAwareRecordingTUIInfoRenderer) Info(message string) {
	r.state.AppendMessage(tui.Message{Kind: tui.MsgInfo, Text: message})
	select {
	case r.messages <- message:
	default:
	}
}

func (r *stateAwareRecordingTUIInfoRenderer) VisibleSessionID() string { return r.sessionID }

func (r *stateAwareRecordingTUIInfoRenderer) HasSubagentObservation(sessionID, parentToolUseID string) bool {
	for _, observation := range r.state.Observations.Snapshot() {
		if observation.SessionID == sessionID && observation.ToolUseID == parentToolUseID && strings.EqualFold(observation.ToolName, "Agent") {
			return true
		}
	}
	return false
}

func (r *stateAwareRecordingTUIInfoRenderer) AcknowledgeSubagentResult(taskID string) bool {
	return r.state.AcknowledgeActivity("background:"+taskID) == nil
}

type directTUIActivityApp struct{ state *tui.AppState }

func (a directTUIActivityApp) State() *tui.AppState { return a.state }
func (a directTUIActivityApp) UpdateSync(fn func()) bool {
	fn()
	return true
}

type recordingSpinnerRenderer struct {
	ui.QuietRenderer

	mu       sync.Mutex
	active   map[string]int
	stopRuns []int
}

type goalUsageRecordingRenderer struct {
	ui.QuietRenderer
	usageCalls   int
	contextBars  int
	goalEpoch    uint64
	goalStatus   string
	goalText     string
	goalCriteria []stream.GoalCriterionStatusEvent
}

func (r *goalUsageRecordingRenderer) Usage(*types.Usage) { r.usageCalls++ }
func (r *goalUsageRecordingRenderer) ContextBar(int, int) {
	r.contextBars++
}

func (r *goalUsageRecordingRenderer) GoalStatusAtEpoch(epoch uint64, event stream.GoalStatusEvent) {
	r.goalEpoch = epoch
	r.goalStatus = event.Status
	r.goalText = event.Objective
	r.goalCriteria = append([]stream.GoalCriterionStatusEvent(nil), event.Criteria...)
}

func TestTUIEventHandlerRecordsGoalEvaluationUsageWithoutRenderingATurn(t *testing.T) {
	renderer := &goalUsageRecordingRenderer{}
	tracker := ui.NewCostTracker("claude-opus-4-5")
	tracker.RecordTurnUsageForProviderModel("", "", types.Usage{InputTokens: 100, OutputTokens: 20}, time.Second)
	before := tracker.LastTurn()
	contextReads := 0
	handler, cleanup := makeTUIEventHandler(renderer, tracker, func() (int, int) {
		contextReads++
		return 200_000, 100
	})
	t.Cleanup(cleanup)

	handler(stream.Event{Type: stream.EventGoalEvaluation, Usage: &types.Usage{InputTokens: 7, OutputTokens: 3}})

	if input, output, _, _ := tracker.TotalUsage(); input != 107 || output != 23 {
		t.Fatalf("tracker totals = %d/%d, want 107/23", input, output)
	}
	if tracker.LastTurn() == nil || before == nil || *tracker.LastTurn() != *before {
		t.Fatalf("goal evaluation changed main turn state: before=%+v after=%+v", before, tracker.LastTurn())
	}
	if renderer.usageCalls != 0 || renderer.contextBars != 0 || contextReads != 0 {
		t.Fatalf("goal evaluation rendered main-turn usage: usage=%d bars=%d context_reads=%d", renderer.usageCalls, renderer.contextBars, contextReads)
	}
}

func TestTUIEventHandlerMarksCompactionAfterCompactionUsage(t *testing.T) {
	tracker := ui.NewCostTracker("conversation-model")
	handler, cleanup := makeTUIEventHandler(&goalUsageRecordingRenderer{}, tracker, nil)
	t.Cleanup(cleanup)

	handler(stream.Event{
		Type: stream.EventProviderUsage, Usage: &types.Usage{InputTokens: 1000, CacheReadInputTokens: 800},
		Metadata: map[string]any{"kind": "compaction", "status": "success"},
	})
	handler(stream.Event{Type: stream.EventCompactBoundary, Compact: &stream.CompactBoundaryEvent{Trigger: "auto"}})

	hasCompacted, input, cacheRead := tracker.CompactionBaseline()
	if !hasCompacted || input != 1000 || cacheRead != 800 {
		t.Fatalf("compaction baseline = %t/%d/%d, want true/1000/800", hasCompacted, input, cacheRead)
	}
}

func TestTUIEventHandlerAccountsDiscardedAttemptAndFallbackTurnOnce(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{CostCurrency: "USD", ID: "primary-model", Provider: "priced", CostPer1MIn: 1, CostPer1MOut: 2})
	catalog.Register(provider.ModelInfo{CostCurrency: "USD", ID: "fallback-model", Provider: "priced", CostPer1MIn: 9, CostPer1MOut: 11})
	tracker := ui.NewCostTracker("primary-model")
	tracker.SetCatalog(catalog)
	tracker.SetProvider("priced")

	handler, cleanup := makeTUIEventHandler(&goalUsageRecordingRenderer{}, tracker, nil)
	t.Cleanup(cleanup)
	handler(stream.Event{
		Type:     stream.EventProviderUsage,
		Usage:    &types.Usage{InputTokens: 1_000_000},
		Metadata: map[string]any{"provider": "priced", "model": "primary-model", "kind": "provider_attempt"},
	})
	handler(stream.Event{
		Type:     stream.EventTurnEnd,
		Usage:    &types.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
		Metadata: map[string]any{"provider": "priced", "model": "fallback-model"},
	})

	if got := tracker.TotalCost(); got != 21 {
		t.Fatalf("session cost = %.4f, want 21.0000", got)
	}
	if last := tracker.LastTurn(); last == nil || last.Model != "fallback-model" || last.CostUSD != 20 {
		t.Fatalf("final fallback turn = %+v", last)
	}
}

func TestTUIEventHandlerRefreshesGoalStatusFromEventProjection(t *testing.T) {
	renderer := &goalUsageRecordingRenderer{}
	handler, cleanup := makeTUIEventHandler(renderer, nil, nil, presentation.ToolEventContext{SessionEpoch: 7})
	t.Cleanup(cleanup)

	handler(stream.Event{
		Type: stream.EventGoalStatus,
		GoalStatus: &stream.GoalStatusEvent{
			Status:    "active",
			Objective: "finish the release",
			Revision:  2,
			Criteria:  []stream.GoalCriterionStatusEvent{{ID: "AC-1", Text: "tests pass", Status: "pending"}},
		},
	})
	if renderer.goalEpoch != 7 || renderer.goalStatus != "active" || renderer.goalText != "finish the release" || len(renderer.goalCriteria) != 1 {
		t.Fatalf("goal projection = epoch:%d status:%q objective:%q", renderer.goalEpoch, renderer.goalStatus, renderer.goalText)
	}

	handler(stream.Event{
		Type:       stream.EventGoalStatus,
		GoalStatus: &stream.GoalStatusEvent{Status: "achieved", Objective: "finish the release"},
	})
	if renderer.goalStatus != "achieved" || renderer.goalText != "finish the release" {
		t.Fatalf("terminal goal projection was not forwarded: status:%q objective:%q", renderer.goalStatus, renderer.goalText)
	}
}

func newRecordingSpinnerRenderer() *recordingSpinnerRenderer {
	return &recordingSpinnerRenderer{active: make(map[string]int)}
}

func (r *recordingSpinnerRenderer) SpinnerStart(toolName string) func() {
	r.mu.Lock()
	r.active[toolName]++
	index := len(r.stopRuns)
	r.stopRuns = append(r.stopRuns, 0)
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.stopRuns[index]++
			r.active[toolName]--
			if r.active[toolName] == 0 {
				delete(r.active, toolName)
			}
		})
	}
}

func (r *recordingSpinnerRenderer) activeCount(toolName string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active[toolName]
}

func (r *recordingSpinnerRenderer) stopCount(index int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if index < 0 || index >= len(r.stopRuns) {
		return 0
	}
	return r.stopRuns[index]
}

func TestTUIEventHandlerTracksParallelToolSpinnersByID(t *testing.T) {
	renderer := newRecordingSpinnerRenderer()
	handler, cleanup := makeTUIEventHandler(renderer, nil, nil)
	t.Cleanup(cleanup)

	handler(stream.Event{Type: stream.EventToolUse, ToolUse: &types.ToolUseBlock{ID: "agent-1", Name: "Agent"}})
	handler(stream.Event{Type: stream.EventToolUse, ToolUse: &types.ToolUseBlock{ID: "agent-2", Name: "Agent"}})
	if got := renderer.activeCount("Agent"); got != 2 {
		t.Fatalf("active Agent spinners = %d, want 2", got)
	}

	// Complete the second call first. The handler must stop the closure associated
	// with agent-2 rather than whichever same-name spinner was started first.
	handler(stream.Event{Type: stream.EventToolResult, ToolResult: &types.ToolResultBlock{ToolUseID: "agent-2"}})
	if got := renderer.stopCount(0); got != 0 {
		t.Fatalf("first Agent spinner stop count = %d, want 0", got)
	}
	if got := renderer.stopCount(1); got != 1 {
		t.Fatalf("second Agent spinner stop count = %d, want 1", got)
	}
	if got := renderer.activeCount("Agent"); got != 1 {
		t.Fatalf("active Agent spinners after agent-2 result = %d, want 1", got)
	}

	handler(stream.Event{Type: stream.EventTurnEnd})
	if got := renderer.stopCount(0); got != 1 {
		t.Fatalf("first Agent spinner stop count after turn end = %d, want 1", got)
	}
	if got := renderer.stopCount(1); got != 1 {
		t.Fatalf("second Agent spinner stopped more than once: %d", got)
	}
	if got := renderer.activeCount("Agent"); got != 0 {
		t.Fatalf("active Agent spinners after turn end = %d, want 0", got)
	}
}

func TestTUIEventHandlerMissingResultIDDoesNotGuessAnonymousSpinner(t *testing.T) {
	renderer := newRecordingSpinnerRenderer()
	handler, cleanup := makeTUIEventHandler(renderer, nil, nil)

	handler(stream.Event{Type: stream.EventToolUse, ToolUse: &types.ToolUseBlock{Name: "Read"}})
	handler(stream.Event{Type: stream.EventToolUse, ToolUse: &types.ToolUseBlock{Name: "Bash"}})
	handler(stream.Event{Type: stream.EventToolResult, ToolResult: &types.ToolResultBlock{Content: "orphan"}})
	if renderer.stopCount(0) != 0 || renderer.stopCount(1) != 0 {
		t.Fatalf("missing result ID guessed an anonymous spinner: stops=%d,%d", renderer.stopCount(0), renderer.stopCount(1))
	}
	if renderer.activeCount("Read") != 1 || renderer.activeCount("Bash") != 1 {
		t.Fatalf("missing result ID stopped a sibling: Read=%d Bash=%d", renderer.activeCount("Read"), renderer.activeCount("Bash"))
	}

	cleanup()
	if renderer.stopCount(0) != 1 || renderer.stopCount(1) != 1 {
		t.Fatalf("turn cleanup did not stop anonymous spinners exactly once: stops=%d,%d", renderer.stopCount(0), renderer.stopCount(1))
	}
}

func TestTUIEventHandlerClearsToolSpinnersOnError(t *testing.T) {
	renderer := newRecordingSpinnerRenderer()
	handler, cleanup := makeTUIEventHandler(renderer, nil, nil)
	t.Cleanup(cleanup)

	handler(stream.Event{Type: stream.EventToolUse, ToolUse: &types.ToolUseBlock{ID: "agent-1", Name: "Agent"}})
	handler(stream.Event{Type: stream.EventError, Text: "provider failed"})

	if got := renderer.activeCount("Agent"); got != 0 {
		t.Fatalf("active Agent spinners after error = %d, want 0", got)
	}
}

func TestTUIEventHandlerClearsToolSpinnersOnInterruption(t *testing.T) {
	renderer := newRecordingSpinnerRenderer()
	handler, cleanup := makeTUIEventHandler(renderer, nil, nil)
	t.Cleanup(cleanup)

	handler(stream.Event{Type: stream.EventToolUse, ToolUse: &types.ToolUseBlock{ID: "agent-1", Name: "Agent"}})
	handler(stream.Event{Type: stream.EventUserInterruption})

	if got := renderer.activeCount("Agent"); got != 0 {
		t.Fatalf("active Agent spinners after interruption = %d, want 0", got)
	}
}

func TestTUIEventHandlerCleanupClearsToolSpinners(t *testing.T) {
	renderer := newRecordingSpinnerRenderer()
	handler, cleanup := makeTUIEventHandler(renderer, nil, nil)

	handler(stream.Event{Type: stream.EventToolUse, ToolUse: &types.ToolUseBlock{ID: "agent-1", Name: "Agent"}})
	cleanup()

	if got := renderer.activeCount("Agent"); got != 0 {
		t.Fatalf("active Agent spinners after final cleanup = %d, want 0", got)
	}
	if got := renderer.stopCount(0); got != 1 {
		t.Fatalf("Agent spinner stop count after final cleanup = %d, want 1", got)
	}
}

func TestInstallTUIBackgroundNotificationsShowsAgentResult(t *testing.T) {
	projectRoot := t.TempDir()
	manager := newAgentBackgroundPresentationAdapter(agentruntime.NewBackgroundTaskManager(projectRoot))
	cleanupBackgroundTaskManager(t, manager.BackgroundTaskManager)
	renderer := recordingTUIInfoRenderer{messages: make(chan string, 1)}
	followUps := make(chan agent.RuntimeNotification, 1)
	unbind := installTUIBackgroundNotifications(manager, renderer, func(_ context.Context, notification agent.RuntimeNotification) error {
		followUps <- notification
		return nil
	})
	t.Cleanup(unbind)

	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{SessionID: "parent-session"})
	snapshot, err := manager.StartAgentTask(ctx, "research", "research sectors", func(context.Context, io.Writer) (string, error) {
		return "sector analysis complete", nil
	})
	if err != nil {
		t.Fatalf("StartAgentTask: %v", err)
	}
	if _, status := manager.Wait(snapshot.ID, time.Second); status != "success" {
		t.Fatalf("Wait status = %q, want success", status)
	}
	select {
	case message := <-renderer.messages:
		if !strings.Contains(message, "sector analysis complete") || !strings.Contains(message, snapshot.ID) {
			t.Fatalf("TUI notification = %q", message)
		}
	case <-time.After(time.Second):
		t.Fatal("background completion was not sent to the TUI")
	}
	select {
	case notification := <-followUps:
		if notification.SessionID != "parent-session" {
			t.Fatalf("follow-up session = %q", notification.SessionID)
		}
		if filepath.Clean(notification.ProjectRoot) != filepath.Clean(projectRoot) {
			t.Fatalf("follow-up project root = %q, want %q", notification.ProjectRoot, projectRoot)
		}
	case <-time.After(time.Second):
		t.Fatal("background completion was not forwarded to the model bridge")
	}
}

func TestInstallTUIBackgroundNotificationsCorrelatedAgentSkipsInfoAndAcknowledgesFollowUp(t *testing.T) {
	const (
		sessionID       = "correlated-agent-session"
		taskID          = "correlated-agent"
		parentToolUseID = "agent-call"
	)
	projectRoot := t.TempDir()
	seedPendingAgentCompletion(t, projectRoot, sessionID, taskID, parentToolUseID, "correlated result")
	manager := newAgentBackgroundPresentationAdapter(agentruntime.NewBackgroundTaskManager(projectRoot))
	cleanupBackgroundTaskManager(t, manager.BackgroundTaskManager)

	state := tui.NewAppState()
	state.SessionID.Set(sessionID)
	state.SessionEpoch.Set(1)
	state.Activities = tui.NewActivityStore(tui.ActivityScope{SessionID: sessionID, Epoch: 1})
	ctx := tui.ToolEventContext{SessionID: sessionID, TurnID: "turn", WorkUnitID: "work", ActorID: "assistant"}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{
		ID: parentToolUseID, Name: "Agent", Input: map[string]any{"description": "inspect weather"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyActivity(tui.ActivityEvent{
		ID: "background:" + taskID, RunID: "run-1", Attempt: 1,
		SessionID: sessionID, Epoch: 1, TurnID: "turn", WorkUnitID: taskID,
		Kind: tui.ActivityAgent, Name: "Agent", Lifecycle: tui.ActivityLifecycleCompleted, Outcome: tui.OutcomeSucceeded,
		Attention: tui.ActivityAttention{Kind: tui.ActivityAttentionReadyForReview, Severity: tui.ActivityAttentionSeverityInfo, Unread: true},
		Progress:  tui.ActivityProgress{AgentID: taskID, ParentToolUseID: parentToolUseID, Phase: string(agent.ProgressCompleted)},
	}); err != nil {
		t.Fatal(err)
	}
	before, ok := state.GetActivity("background:" + taskID)
	if !ok || !before.Attention.Unread || before.Acknowledged {
		t.Fatalf("initial Agent attention = %+v ok=%t", before, ok)
	}
	baselineMessages := len(state.Messages.Get())
	renderer := &stateAwareRecordingTUIInfoRenderer{
		state: state, sessionID: sessionID, messages: make(chan string, 2),
	}
	followUps := make(chan agent.RuntimeNotification, 2)
	unbind := installTUIBackgroundNotifications(manager, renderer, func(_ context.Context, notification agent.RuntimeNotification) error {
		followUps <- notification
		return nil
	})
	t.Cleanup(unbind)

	notification := waitForTUIBackgroundFollowUp(t, followUps)
	if notification.TaskID != taskID {
		t.Fatalf("follow-up task = %q, want %q", notification.TaskID, taskID)
	}
	select {
	case message := <-renderer.messages:
		t.Fatalf("correlated Agent completion appended MsgInfo: %q", message)
	default:
	}
	if messages := state.Messages.Get(); len(messages) != baselineMessages {
		t.Fatalf("correlated completion changed transcript messages: before=%d after=%d (%#v)", baselineMessages, len(messages), messages)
	}
	for _, message := range state.Messages.Get() {
		if message.Kind == tui.MsgInfo {
			t.Fatalf("correlated completion appended top-level MsgInfo: %#v", message)
		}
	}
	waitForAgentAttentionAcknowledged(t, state, "background:"+taskID)
	select {
	case duplicate := <-followUps:
		t.Fatalf("correlated completion triggered duplicate follow-up: %+v", duplicate)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestInstallTUIBackgroundNotificationsKeepsUncorrelatedAndLinearFallbacks(t *testing.T) {
	t.Run("dynamic TUI without Agent observation", func(t *testing.T) {
		const (
			sessionID = "uncorrelated-agent-session"
			taskID    = "uncorrelated-agent"
		)
		projectRoot := t.TempDir()
		seedPendingAgentCompletion(t, projectRoot, sessionID, taskID, "missing-agent-call", "uncorrelated result")
		manager := newAgentBackgroundPresentationAdapter(agentruntime.NewBackgroundTaskManager(projectRoot))
		cleanupBackgroundTaskManager(t, manager.BackgroundTaskManager)

		state := tui.NewAppState()
		state.SessionID.Set(sessionID)
		state.SessionEpoch.Set(1)
		renderer := &stateAwareRecordingTUIInfoRenderer{
			state: state, sessionID: sessionID, messages: make(chan string, 2),
		}
		followUps := make(chan agent.RuntimeNotification, 2)
		unbind := installTUIBackgroundNotifications(manager, renderer, func(_ context.Context, notification agent.RuntimeNotification) error {
			followUps <- notification
			return nil
		})
		t.Cleanup(unbind)

		select {
		case message := <-renderer.messages:
			if !strings.Contains(message, "uncorrelated result") {
				t.Fatalf("uncorrelated fallback omitted result: %q", message)
			}
		case <-time.After(time.Second):
			t.Fatal("uncorrelated dynamic TUI did not receive fallback MsgInfo")
		}
		if messages := state.Messages.Get(); len(messages) != 1 || messages[0].Kind != tui.MsgInfo {
			t.Fatalf("uncorrelated fallback messages = %#v, want one MsgInfo", messages)
		}
		if notification := waitForTUIBackgroundFollowUp(t, followUps); notification.TaskID != taskID {
			t.Fatalf("uncorrelated follow-up task = %q, want %q", notification.TaskID, taskID)
		}
	})

	t.Run("linear renderer", func(t *testing.T) {
		const (
			sessionID = "linear-agent-session"
			taskID    = "linear-agent"
		)
		projectRoot := t.TempDir()
		seedPendingAgentCompletion(t, projectRoot, sessionID, taskID, "agent-call", "linear result")
		manager := newAgentBackgroundPresentationAdapter(agentruntime.NewBackgroundTaskManager(projectRoot))
		cleanupBackgroundTaskManager(t, manager.BackgroundTaskManager)

		renderer := recordingTUIInfoRenderer{messages: make(chan string, 2)}
		followUps := make(chan agent.RuntimeNotification, 2)
		unbind := installTUIBackgroundNotifications(manager, renderer, func(_ context.Context, notification agent.RuntimeNotification) error {
			followUps <- notification
			return nil
		})
		t.Cleanup(unbind)

		select {
		case message := <-renderer.messages:
			if !strings.Contains(message, "linear result") {
				t.Fatalf("linear fallback omitted result: %q", message)
			}
		case <-time.After(time.Second):
			t.Fatal("linear renderer did not receive fallback notification")
		}
		if notification := waitForTUIBackgroundFollowUp(t, followUps); notification.TaskID != taskID {
			t.Fatalf("linear follow-up task = %q, want %q", notification.TaskID, taskID)
		}
	})
}

func seedPendingAgentCompletion(t *testing.T, projectRoot, sessionID, taskID, parentToolUseID, result string) {
	t.Helper()
	now := time.Now().UTC()
	exitCode := 0
	notification := agent.RuntimeNotification{
		ID: "notification-" + taskID, Kind: "task-notification", TaskID: taskID,
		SessionID: sessionID, ProjectRoot: projectRoot,
		Status: "completed", ExitCode: &exitCode, CreatedAt: now,
	}
	record := runtimestore.RuntimeTaskRecord{
		ID: taskID, Type: "local_agent", Status: "completed", Description: "inspect weather",
		Result: result, StartedAt: now.Add(-time.Second), FinishedAt: &now, ExitCode: &exitCode,
		OwnerSessionID: sessionID, OwnerProjectRoot: projectRoot, Detached: true,
		CurrentRunID: "run-1", Attempt: 1, Outcome: agent.RunOutcomeSucceeded,
		LatestProgress: &agent.ProgressEvent{
			AgentID: taskID, ParentToolUseID: parentToolUseID, RunID: "run-1",
			Phase: agent.ProgressCompleted, SourceSequence: 1,
		},
		Notification: &notification,
	}
	if err := runtimestore.NewRuntimeTaskStore(projectRoot).Save(record); err != nil {
		t.Fatal(err)
	}
}

func waitForTUIBackgroundFollowUp(t *testing.T, followUps <-chan agent.RuntimeNotification) agent.RuntimeNotification {
	t.Helper()
	select {
	case notification := <-followUps:
		return notification
	case <-time.After(time.Second):
		t.Fatal("background completion did not trigger follow-up")
		return agent.RuntimeNotification{}
	}
}

func waitForAgentAttentionAcknowledged(t *testing.T, state *tui.AppState, activityID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		activity, ok := state.GetActivity(activityID)
		if ok && !activity.Attention.Unread && activity.Acknowledged {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	activity, ok := state.GetActivity(activityID)
	t.Fatalf("successful follow-up did not acknowledge Agent attention: %+v ok=%t", activity, ok)
}

func TestInstallTUIBackgroundNotificationsHidesSameIDFromOtherProject(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := newAgentBackgroundPresentationAdapter(agentruntime.NewBackgroundTaskManager(rootA))
	cleanupBackgroundTaskManager(t, manager.BackgroundTaskManager)
	renderer := sessionAwareRecordingTUIInfoRenderer{
		recordingTUIInfoRenderer: recordingTUIInfoRenderer{messages: make(chan string, 1)},
		sessionID:                "duplicate-session",
	}
	unbind := installTUIBackgroundNotifications(manager, renderer)
	t.Cleanup(unbind)

	started := make(chan struct{})
	release := make(chan struct{})
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{SessionID: "duplicate-session", CWD: rootA})
	snapshot, err := manager.StartAgentTask(ctx, "origin task", "origin task", func(context.Context, io.Writer) (string, error) {
		close(started)
		<-release
		return "origin result", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	manager.SetProjectRoot(rootB)
	close(release)
	if _, status := manager.Wait(snapshot.ID, time.Second); status != "success" {
		t.Fatalf("Wait status = %q", status)
	}

	select {
	case message := <-renderer.messages:
		t.Fatalf("other-project notification leaked into same-ID foreground session: %q", message)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBindTUIBackgroundActivitiesHidesSameSessionIDFromOtherProject(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := newAgentBackgroundPresentationAdapter(agentruntime.NewBackgroundTaskManager(rootA))
	cleanupBackgroundTaskManager(t, manager.BackgroundTaskManager)
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{SessionID: "duplicate-session", CWD: rootA})
	snapshot, err := manager.StartAgentTask(ctx, "origin task", "origin task", func(context.Context, io.Writer) (string, error) {
		return "origin result", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, status := manager.Wait(snapshot.ID, time.Second); status != "success" {
		t.Fatalf("Wait status = %q", status)
	}
	manager.SetProjectRoot(rootB)

	state := tui.NewAppState()
	state.SessionID.Set("duplicate-session")
	state.SessionEpoch.Set(1)
	state.Activities = tui.NewActivityStore(tui.ActivityScope{SessionID: "duplicate-session", Epoch: 1})
	unbind := bindTUIBackgroundActivities(manager, directTUIActivityApp{state: state})
	unbind()
	if activities := state.ActivitySnapshot().Activities; len(activities) != 0 {
		t.Fatalf("other-project task leaked into same-ID activity projection: %#v", activities)
	}
}

func TestBindTUIBackgroundActivitiesKeepsAgentRunEvidenceOutOfUserControls(t *testing.T) {
	root := t.TempDir()
	manager := newAgentBackgroundPresentationAdapter(agentruntime.NewBackgroundTaskManager(root))
	cleanupBackgroundTaskManager(t, manager.BackgroundTaskManager)
	const sessionID = "background-evidence-session"
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{SessionID: sessionID, CWD: root})
	started, err := manager.StartAgentTask(ctx, "inspect weather", "inspect weather", func(context.Context, io.Writer) (string, error) {
		return "typed weather result", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, status := manager.Wait(started.ID, time.Second); status != "success" {
		t.Fatalf("Wait status = %q", status)
	}

	state := tui.NewAppState()
	state.SessionID.Set(sessionID)
	state.SessionEpoch.Set(1)
	state.Activities = tui.NewActivityStore(tui.ActivityScope{SessionID: sessionID, Epoch: 1})
	unbind := bindTUIBackgroundActivities(manager, directTUIActivityApp{state: state})
	unbind()

	if messages := state.Messages.Get(); len(messages) != 0 {
		t.Fatalf("background evidence created %d top-level transcript messages: %#v", len(messages), messages)
	}
	activities := state.ActivitySnapshot().Activities
	if len(activities) != 1 {
		t.Fatalf("background activities = %#v, want one logical Agent", activities)
	}
	activity := activities[0]
	if activity.Kind != tui.ActivityAgent || activity.Lifecycle != tui.ActivityLifecycleCompleted || activity.Attention.Kind != tui.ActivityAttentionReadyForReview || !activity.Attention.Unread {
		t.Fatalf("detached Agent projection = %+v", activity)
	}
	if len(activity.Control.DetailRefs) != 0 || activityHasAction(activity, tui.ActivityDetails) {
		t.Fatalf("detached Agent exposed complete run evidence: %+v actions=%+v", activity.Control, activity.Actions)
	}
}

func TestValidateCustomEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		baseURL  string
		wantErr  bool
	}{
		{name: "provider default", provider: "openai"},
		{name: "https endpoint", provider: "vertex", baseURL: "https://proxy.example.com/v1"},
		{name: "local endpoint", provider: "ollama", baseURL: "http://localhost:11434/v1"},
		{name: "vertex requires endpoint", provider: "vertex", wantErr: true},
		{name: "reject relative URL", provider: "openai", baseURL: "/v1", wantErr: true},
		{name: "reject embedded credentials", provider: "openai", baseURL: "https://user:pass@example.com/v1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCustomEndpoint(tt.provider, tt.baseURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateCustomEndpoint(%q, %q) error = %v, wantErr %v", tt.provider, tt.baseURL, err, tt.wantErr)
			}
		})
	}
}

func TestPopulateModelPickerEntriesUsesCurrentReasoningEffort(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{CostCurrency: "USD",
		Provider:               "openai",
		ID:                     "gpt-5.5",
		Aliases:                []string{"gpt-current"},
		ReasoningEfforts:       []string{"low", "medium", "high", "xhigh"},
		DefaultReasoningEffort: "high",
	})

	picker := &tui.ModelPickerState{}
	populateModelPickerEntries(picker, catalog, "openai", "openai", "gpt-current", "high")
	if len(picker.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(picker.Entries))
	}
	if got := picker.Entries[0].ReasoningEffort; got != "high" {
		t.Fatalf("current ReasoningEffort = %q, want high", got)
	}

	populateModelPickerEntries(picker, catalog, "openai", "anthropic", "gpt-current", "high")
	if got := picker.Entries[0].ReasoningEffort; got != "high" {
		t.Fatalf("non-current ReasoningEffort = %q, want catalog default high", got)
	}
	if got := picker.Entries[0].DefaultReasoningEffort; got != "high" {
		t.Fatalf("picker DefaultReasoningEffort = %q, want high", got)
	}
}

func TestSynchronizeInitialTUIChromeRestoresRuntimeDefaults(t *testing.T) {
	state := tui.NewAppState()

	synchronizeInitialTUIChrome(state, " openai ", " gpt-5.6-sol ", " medium ")

	if got := state.Provider.Get(); got != "openai" {
		t.Fatalf("Provider = %q, want openai", got)
	}
	if got := state.Model.Get(); got != "gpt-5.6-sol" {
		t.Fatalf("Model = %q, want gpt-5.6-sol", got)
	}
	if got := state.ReasoningEffort.Get(); got != "medium" {
		t.Fatalf("ReasoningEffort = %q, want medium", got)
	}
}

func TestSynchronizeInitialTUIChromePreservesSessionModel(t *testing.T) {
	state := tui.NewAppState()
	state.Provider.Set("anthropic")
	state.Model.Set("claude-sonnet-5")

	synchronizeInitialTUIChrome(state, "openai", "gpt-5.6-sol", "medium")

	if got := state.Provider.Get(); got != "anthropic" {
		t.Fatalf("Provider = %q, want persisted provider", got)
	}
	if got := state.Model.Get(); got != "claude-sonnet-5" {
		t.Fatalf("Model = %q, want persisted model", got)
	}
	if got := state.ReasoningEffort.Get(); got != "medium" {
		t.Fatalf("ReasoningEffort = %q, want current runtime default", got)
	}
}
