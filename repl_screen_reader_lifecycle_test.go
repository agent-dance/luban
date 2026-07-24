package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/tui"
	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
)

func TestPresentationAdapterUsesActiveRuntimeLanguage(t *testing.T) {
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.SaveLanguage(i18n.LangEN) })

	if got := semanticAggregateSummary(tui.FamilyFileRead, 2); !strings.Contains(got, "2 次操作") {
		t.Fatalf("aggregate summary did not use active language: %q", got)
	}
	got := formatCommandPresentationTerminal(commands.CommandPresentation{
		Command: "status", Outcome: commands.CommandOutcomeSucceeded,
		Display: commands.CommandDisplayReceipt, Risk: commands.CommandRiskLow,
	})
	if !strings.Contains(got, "命令 /status") || !strings.Contains(got, "展示：回执") {
		t.Fatalf("command presentation did not use active language: %q", got)
	}
	if action := presentationActionForFamily(tui.FamilyShell, "Bash"); action != "运行命令" {
		t.Fatalf("tool action did not use active language: %q", action)
	}
}

func TestScreenReaderEventHandlerEmitsTextImmediatelyInEventOrder(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	handle := makeScreenReaderEventHandler(renderer, nil, nil, ui.ToolEventContext{SessionID: "session"})

	handle(loop.Event{Type: loop.EventText, Text: "first feedback", TurnCount: 1})
	if got := output.String(); !strings.Contains(got, "first feedback") {
		t.Fatalf("first text remained buffered until turn end: %q", got)
	}
	handle(loop.Event{Type: loop.EventToolUse, TurnCount: 1, ToolUse: &types.ToolUseBlock{ID: "tool-1", Name: "Read", Input: map[string]any{"file_path": "a.go"}}})
	handle(loop.Event{Type: loop.EventToolResult, TurnCount: 1, ToolResult: &types.ToolResultBlock{ToolUseID: "tool-1", Content: "result evidence"}})
	handle(loop.Event{Type: loop.EventTurnEnd, TurnCount: 1})

	text := output.String()
	textAt := strings.Index(text, "first feedback")
	toolAt := strings.Index(text, "Tool update. Tool: Read")
	runningAt := strings.Index(text, "State: running")
	resultAt := strings.Index(text, "State: succeeded")
	if textAt < 0 || toolAt <= textAt || runningAt <= toolAt || resultAt <= runningAt {
		t.Fatalf("screen-reader event order = text:%d tool:%d running:%d result:%d\n%s", textAt, toolAt, runningAt, resultAt, text)
	}
}

func TestScreenReaderGroupsRoutineToolMembersAndKeepsVisibleSummary(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	handle := makeScreenReaderEventHandler(renderer, nil, nil, ui.ToolEventContext{SessionID: "session", ActorID: "agent", WorkUnitID: "research"})
	for index, id := range []string{"read-a", "read-b"} {
		call := types.ToolUseBlock{ID: id, Name: "Read", Input: map[string]any{"file_path": fmt.Sprintf("%c.go", 'a'+index)}}
		handle(loop.Event{Type: loop.EventToolUse, TurnCount: 1, TurnID: "turn-group", ActorID: "agent", WorkUnitID: "research", ToolUse: &call})
		result := types.ToolResultBlock{ToolUseID: id, Content: "retained " + id, Outcome: types.ToolOutcomeSucceeded}
		handle(loop.Event{Type: loop.EventToolResult, TurnCount: 1, TurnID: "turn-group", ActorID: "agent", WorkUnitID: "research", ToolResult: &result})
	}
	handle(loop.Event{Type: loop.EventTurnEnd, TurnCount: 1, TurnID: "turn-group", ActorID: "agent", WorkUnitID: "research"})

	got := output.String()
	for _, want := range []string{"Tool: Aggregate", "Read - 2 operations", "Details available"} {
		if !strings.Contains(got, want) {
			t.Fatalf("grouped screen-reader output missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "State: running.") != 1 || strings.Count(got, "State: succeeded.") != 1 {
		t.Fatalf("routine members were not coalesced into one running and one terminal transition:\n%s", got)
	}
}

func TestScreenReaderPromotesFailureOutOfRoutineGroup(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	handle := makeScreenReaderEventHandler(renderer, nil, nil, ui.ToolEventContext{SessionID: "session", ActorID: "agent", WorkUnitID: "research"})
	for _, id := range []string{"safe-a", "failed", "safe-b"} {
		call := types.ToolUseBlock{ID: id, Name: "Read", Input: map[string]any{"file_path": id + ".go"}}
		handle(loop.Event{Type: loop.EventToolUse, TurnCount: 1, TurnID: "turn", ToolUse: &call})
		result := types.ToolResultBlock{ToolUseID: id, Content: "retained " + id, Outcome: types.ToolOutcomeSucceeded}
		if id == "failed" {
			result.IsError = true
			result.Outcome = types.ToolOutcomeFailed
			result.Content = "permission denied"
		}
		handle(loop.Event{Type: loop.EventToolResult, TurnCount: 1, TurnID: "turn", ToolResult: &result})
	}
	handle(loop.Event{Type: loop.EventTurnEnd, TurnCount: 1, TurnID: "turn"})
	got := output.String()
	if !strings.Contains(got, "State: failed.") || !strings.Contains(got, "permission denied") || strings.Contains(got, "Read - 2 operations") || strings.Contains(got, "Tool: Aggregate") {
		t.Fatalf("failure was swallowed by routine group:\n%s", got)
	}
	if strings.Count(got, "State: succeeded.") != 2 {
		t.Fatalf("safe events on opposite sides of failure were not kept independent:\n%s", got)
	}
}

func TestScreenReaderEventHandlerRecordsGoalEvaluationUsageWithoutRenderingATurn(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	tracker := ui.NewCostTracker("claude-opus-4-5")
	tracker.RecordTurnUsage(types.Usage{InputTokens: 100, OutputTokens: 20}, time.Second)
	before := tracker.LastTurn()
	contextReads := 0
	handle := makeScreenReaderEventHandler(renderer, tracker, func() (int, int) {
		contextReads++
		return 200_000, 100
	}, ui.ToolEventContext{SessionID: "session"})

	handle(loop.Event{
		Type:     loop.EventGoalEvaluation,
		Usage:    &types.Usage{InputTokens: 7, OutputTokens: 3},
		Metadata: map[string]any{"model": "goal-evaluator-model"},
	})

	if input, outputTokens, _, _ := tracker.TotalUsage(); input != 107 || outputTokens != 23 {
		t.Fatalf("tracker totals = %d/%d, want 107/23", input, outputTokens)
	}
	if tracker.TurnCount() != 1 || tracker.LastTurn() == nil || before == nil || *tracker.LastTurn() != *before {
		t.Fatalf("goal evaluation changed main turn state: count=%d before=%+v after=%+v", tracker.TurnCount(), before, tracker.LastTurn())
	}
	if contextReads != 0 {
		t.Fatalf("goal evaluation refreshed context %d times", contextReads)
	}
	entries := tracker.PerModelCosts()
	if len(entries) != 2 || entries[0].Model != "claude-opus-4-5" || entries[1].Model != "goal-evaluator-model" || entries[1].InputTokens != 7 || entries[1].OutputTokens != 3 || entries[1].TurnCount != 0 {
		t.Fatalf("goal evaluation model attribution = %+v", entries)
	}
	for _, unexpected := range []string{"Token usage:", "Context usage:", "Cost:"} {
		if strings.Contains(output.String(), unexpected) {
			t.Fatalf("goal evaluation rendered %q as a main turn: %q", unexpected, output.String())
		}
	}
}

func TestScreenReaderEventHandlerMarksCompactionUsageBaseline(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	tracker := ui.NewCostTracker("claude-opus-4-5")
	handle := makeScreenReaderEventHandler(renderer, tracker, nil, ui.ToolEventContext{SessionID: "session"})

	handle(loop.Event{
		Type: loop.EventProviderUsage, Usage: &types.Usage{InputTokens: 1000, CacheReadInputTokens: 800},
		Metadata: map[string]any{"kind": "compaction", "status": "success"},
	})
	handle(loop.Event{Type: loop.EventCompactBoundary, Compact: &loop.CompactBoundaryEvent{Trigger: "manual"}})

	hasCompacted, input, cacheRead := tracker.CompactionBaseline()
	if !hasCompacted || input != 1000 || cacheRead != 800 {
		t.Fatalf("compaction baseline = %t/%d/%d, want true/1000/800", hasCompacted, input, cacheRead)
	}
}

func TestScreenReaderEventHandlerDoesNotDoubleCountDuplicateCompactionBoundary(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	tracker := ui.NewCostTracker("claude-opus-4-5")
	tracker.RecordTurnUsage(types.Usage{InputTokens: 1500, OutputTokens: 80}, 0)
	handle := makeScreenReaderEventHandler(renderer, tracker, nil, ui.ToolEventContext{SessionID: "session", TurnID: "turn-2"})
	boundary := loop.Event{Type: loop.EventCompactBoundary, Compact: &loop.CompactBoundaryEvent{Trigger: "manual"}}

	handle(boundary)
	handle(boundary)

	usage := tracker.ConversationUsage()
	if usage.CompactionCount != 1 || usage.CompletedInputTokens != 1500 || usage.CompletedOutputTokens != 80 {
		t.Fatalf("duplicate boundary changed conversation usage: %+v", usage)
	}
}

func TestScreenReaderEventHandlerLinearizesCompactionLifecycleOnce(t *testing.T) {
	for _, trigger := range []string{"auto", "reactive", "manual"} {
		t.Run(trigger, func(t *testing.T) {
			var output bytes.Buffer
			renderer := ui.NewScreenReaderRenderer(&output, nil)
			defer renderer.Close()
			handle := makeScreenReaderEventHandler(renderer, nil, nil, ui.ToolEventContext{SessionID: "session"})
			metadata := map[string]any{"trigger": trigger}
			boundary := &loop.CompactBoundaryEvent{
				Trigger: trigger, PreCompactTokenCount: 1200, PostCompactTokenCount: 300, TruePostCompactTokenCount: 280,
			}

			handle(loop.Event{Type: loop.EventProgress, TurnID: "turn-3", Progress: &loop.ProgressEvent{Stage: "compact_start", Message: "compacting", Metadata: metadata}})
			// Multiple producers or retries may repeat receipts. Append-only output
			// must linearize one start, boundary, and terminal receipt per identity.
			handle(loop.Event{Type: loop.EventProgress, TurnID: "turn-3", Progress: &loop.ProgressEvent{Stage: "compact_start", Message: "compacting", Metadata: metadata}})
			handle(loop.Event{Type: loop.EventCompactBoundary, TurnID: "turn-3", Compact: boundary})
			handle(loop.Event{Type: loop.EventCompactBoundary, TurnID: "turn-3", Compact: boundary})
			handle(loop.Event{Type: loop.EventProgress, TurnID: "turn-3", Progress: &loop.ProgressEvent{Stage: "compact_end", Message: "idle", Metadata: metadata}})
			handle(loop.Event{Type: loop.EventProgress, TurnID: "turn-3", Progress: &loop.ProgressEvent{Stage: "compact_end", Message: "idle", Metadata: metadata}})

			text := output.String()
			for _, want := range []string{"Compaction started", "trigger " + trigger, "1200", "280", "discarded 920", "Compaction completed"} {
				if !strings.Contains(text, want) {
					t.Fatalf("screen-reader compaction output omitted %q:\n%s", want, text)
				}
			}
			for _, receipt := range []string{"Compaction started", "Compaction boundary", "Compaction completed"} {
				if got := strings.Count(text, receipt); got != 1 {
					t.Fatalf("%s receipt count = %d, want 1:\n%s", receipt, got, text)
				}
			}
			boundaryAt := strings.Index(text, "Compaction boundary")
			completedAt := strings.Index(text, "Compaction completed")
			if boundaryAt < 0 || completedAt <= boundaryAt {
				t.Fatalf("screen-reader compaction completed before its boundary: boundary=%d completed=%d\n%s", boundaryAt, completedAt, text)
			}
		})
	}
}

func TestScreenReaderEventHandlerDistinguishesCompactionFailureAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, stage, status, want string
	}{
		{name: "failed", stage: "compact_failed", status: "failed", want: "Compaction failed"},
		{name: "cancelled", stage: "compact_cancelled", status: "cancelled", want: "Compaction cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			renderer := ui.NewScreenReaderRenderer(&output, nil)
			defer renderer.Close()
			handle := makeScreenReaderEventHandler(renderer, nil, nil, ui.ToolEventContext{SessionID: "session"})
			event := loop.Event{Type: loop.EventProgress, TurnID: "turn-4", Progress: &loop.ProgressEvent{
				Stage: tc.stage, Message: tc.status, Metadata: map[string]any{"trigger": "reactive"},
			}}
			handle(event)
			handle(event)
			if text := output.String(); !strings.Contains(text, tc.want) || !strings.Contains(text, "trigger reactive") || strings.Count(text, tc.want) != 1 {
				t.Fatalf("screen-reader %s receipt = %q, want %q with trigger", tc.name, text, tc.want)
			}
		})
	}
}

type screenReaderLifecycleProvider struct{}

func (screenReaderLifecycleProvider) Name() string    { return "test" }
func (screenReaderLifecycleProvider) ModelID() string { return "test-model" }
func (screenReaderLifecycleProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	return nil, nil
}

type screenReaderLifecycleEngine struct{ engine.Engine }

type screenReaderDeleteEngine struct {
	screenReaderLifecycleEngine
	repo       *session.Repository
	calls      int
	deletedID  string
	projectDir string
}

func (e *screenReaderDeleteEngine) Sessions() engine.SessionManager { return nil }

func (e *screenReaderDeleteEngine) DeleteSessionHistory(_ context.Context, sessionID, projectDir string) error {
	e.calls++
	e.deletedID = sessionID
	e.projectDir = projectDir
	return e.repo.Delete(sessionID, projectDir)
}

type screenReaderClearEngine struct{ *sessionSwitcherTestEngine }

type screenReaderBackgroundEngine struct {
	replHookEngine
	followUps    chan engine.QueryRequest
	followErr    error
	providerMu   sync.Mutex
	providerRuns int
	ready        chan struct{}
	readyOnce    sync.Once
}

func (e *screenReaderBackgroundEngine) Provider() provider.Provider {
	e.providerMu.Lock()
	e.providerRuns++
	installed := e.providerRuns >= 2
	e.providerMu.Unlock()
	if installed && e.ready != nil {
		e.readyOnce.Do(func() { close(e.ready) })
	}
	return screenReaderLifecycleProvider{}
}

func (e *screenReaderBackgroundEngine) QueryFollowUp(_ context.Context, req engine.QueryRequest) (<-chan engine.Event, error) {
	if e.followErr != nil {
		return nil, e.followErr
	}
	select {
	case e.followUps <- req:
	default:
	}
	ch := make(chan engine.Event, 2)
	ch <- engine.Event{SessionID: req.SessionID, Inner: loop.Event{Type: loop.EventText, Text: "model accepted background result"}}
	ch <- engine.Event{SessionID: req.SessionID, Final: true}
	close(ch)
	return ch, nil
}

func (screenReaderClearEngine) Provider() provider.Provider { return screenReaderLifecycleProvider{} }
func (screenReaderClearEngine) ContextUsage(string) (*engine.ContextUsageInfo, error) {
	return nil, nil
}

func (screenReaderLifecycleEngine) Provider() provider.Provider {
	return screenReaderLifecycleProvider{}
}
func (screenReaderLifecycleEngine) ContextUsage(string) (*engine.ContextUsageInfo, error) {
	return nil, nil
}

func TestScreenReaderREPLInstallsBackgroundReceiptAndModelFollowUp(t *testing.T) {
	root := t.TempDir()
	manager := tools.NewBackgroundTaskManager(root)
	t.Cleanup(manager.Shutdown)
	eng := &screenReaderBackgroundEngine{followUps: make(chan engine.QueryRequest, 1), ready: make(chan struct{})}
	sessionID := "screen-background-session"
	projectDir := root
	cwd := root
	reader, writer := io.Pipe()
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, reader)
	done := make(chan error, 1)
	go func() {
		done <- RunScreenReaderREPL(context.Background(), TUIREPLConfig{
			Engine: eng, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd, BackgroundTasks: manager,
		}, renderer, nil)
	}()

	select {
	case <-eng.ready:
	case <-time.After(time.Second):
		t.Fatal("screen-reader REPL did not reach the notification-installed startup boundary")
	}
	ctx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{SessionID: sessionID, CWD: root})
	task, err := manager.StartAgentTask(ctx, "screen-reader background", "screen-reader background", func(context.Context, io.Writer) (string, error) {
		return "background evidence", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, status := manager.Wait(task.ID, time.Second); status != "success" {
		t.Fatalf("Wait status = %q", status)
	}
	select {
	case request := <-eng.followUps:
		if request.SessionID != sessionID || filepath.Clean(request.CWD) != filepath.Clean(root) || request.RuntimeEventID == "" || !strings.Contains(request.Message, "background evidence") {
			t.Fatalf("screen-reader follow-up request = %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("screen-reader mode did not install the model follow-up consumer")
	}
	if _, err := writer.Write([]byte("/exit\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{"background evidence", "model accepted background result", "Background follow-up receipt", sessionID} {
		if !strings.Contains(text, want) {
			t.Fatalf("screen-reader background output missing %q:\n%s", want, text)
		}
	}
}

func TestScreenReaderDeletedSessionFollowUpIsTerminallyAcknowledged(t *testing.T) {
	root := t.TempDir()
	manager := tools.NewBackgroundTaskManager(root)
	t.Cleanup(manager.Shutdown)
	eng := &screenReaderBackgroundEngine{followUps: make(chan engine.QueryRequest, 1), followErr: engine.ErrSessionDeleted}
	sessionID := "deleted-session"
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	cfg := TUIREPLConfig{Engine: eng, SessionID: &sessionID, BackgroundTasks: manager}
	unbind := installTUIBackgroundNotifications(manager, screenReaderSessionInfoRenderer{ScreenReaderRenderer: renderer, sessionID: &sessionID}, func(_ context.Context, notification tools.RuntimeNotification) error {
		return runScreenReaderBackgroundFollowUp(context.Background(), cfg, renderer, ui.NewCostTracker("test-model"), notification)
	})
	defer unbind()
	ctx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{SessionID: sessionID, ProjectRoot: root, CWD: root})
	task, err := manager.StartAgentTask(ctx, "deleted task", "deleted task", func(context.Context, io.Writer) (string, error) {
		return "late result", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, status := manager.Wait(task.ID, time.Second); status != "success" {
		t.Fatalf("Wait status = %q", status)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		record, ok := tools.NewRuntimeTaskStore(root).Get(task.ID)
		if ok && record.Notification != nil && record.Notification.FollowUpDeliveredAt != nil && record.Notification.DeliveredAt != nil && record.Notification.LastError == "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	record, ok := tools.NewRuntimeTaskStore(root).Get(task.ID)
	if !ok || record.Notification == nil || record.Notification.FollowUpDeliveredAt == nil || record.Notification.DeliveredAt == nil || record.Notification.LastError != "" {
		t.Fatalf("deleted-session notification remained pending: %#v", record.Notification)
	}
	if !strings.Contains(output.String(), "discarded because its session history was deleted") {
		t.Fatalf("missing linear deleted-session receipt: %s", output.String())
	}
}

func TestScreenReaderResumeUsesResolvedProjectAndPreservesPreviousPresentation(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	currentCWD := t.TempDir()
	targetCWD := t.TempDir()
	currentProject := repo.ProjectDirForCWD(currentCWD)
	targetProject := repo.ProjectDirForCWD(targetCWD)
	currentID, targetID := "current-session", "target-session"
	if err := repo.Save(currentID, currentProject, []types.Message{types.UserMessage("current")}); err != nil {
		t.Fatal(err)
	}
	previousPresentation := &session.SessionPresentationMeta{
		FocusedObservationID: "obs-focus", ScrollAnchorID: "obs-anchor", ScrollOffset: 7, InputDraft: "unfinished draft", PermissionMode: "auto",
	}
	if err := repo.SaveMeta(currentID, currentProject, session.SessionMeta{CWD: currentCWD, Presentation: previousPresentation}); err != nil {
		t.Fatal(err)
	}
	targetMessages := []types.Message{types.AssistantMessage("target transcript evidence")}
	if err := repo.Save(targetID, targetProject, targetMessages); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMeta(targetID, targetProject, session.SessionMeta{
		CWD: targetCWD, Usage: &session.SessionUsageMeta{InputTokens: 11, OutputTokens: 5}, Presentation: &session.SessionPresentationMeta{PermissionMode: "plan"},
	}); err != nil {
		t.Fatal(err)
	}

	activeID, activeProject, activeCWD := currentID, currentProject, currentCWD
	var switched commands.SessionListEntry
	cfg := TUIREPLConfig{
		Engine: screenReaderLifecycleEngine{}, Repo: repo, SessionID: &activeID, SessionProjectDir: &activeProject, CWD: &activeCWD,
		PermChecker: permissions.NewChecker(permissions.ModeAskAlways, nil), SessionTransitionMu: &sync.Mutex{},
	}
	cfg.SwitchSession = func(_ context.Context, entry commands.SessionListEntry) error {
		switched = entry
		activeID, activeProject, activeCWD = entry.ID, entry.ProjectDir, entry.CWD
		return nil
	}
	tracker := ui.NewCostTracker("test-model")
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	if err := resumeScreenReaderSession(context.Background(), cfg, renderer, tracker, targetID); err != nil {
		t.Fatal(err)
	}
	if switched.ID != targetID || switched.ProjectDir != targetProject || switched.CWD != targetCWD {
		t.Fatalf("resume used current project context: %+v", switched)
	}
	if activeID != targetID || activeProject != targetProject || activeCWD != targetCWD {
		t.Fatalf("active epoch did not switch coherently: id=%q project=%q cwd=%q", activeID, activeProject, activeCWD)
	}
	if !bytes.Contains(output.Bytes(), []byte("target transcript evidence")) {
		t.Fatalf("target transcript was not rendered: %q", output.String())
	}
	input, outputTokens, _, _ := tracker.TotalUsage()
	if input != 11 || outputTokens != 5 {
		t.Fatalf("target usage was not restored: input=%d output=%d", input, outputTokens)
	}
	currentMeta, _, err := repo.GetMeta(currentID, currentProject)
	if err != nil {
		t.Fatal(err)
	}
	if currentMeta.Presentation == nil || currentMeta.Presentation.FocusedObservationID != "obs-focus" || currentMeta.Presentation.ScrollAnchorID != "obs-anchor" || currentMeta.Presentation.ScrollOffset != 7 || currentMeta.Presentation.InputDraft != "unfinished draft" {
		t.Fatalf("screen-reader persistence erased fullscreen interaction state: %+v", currentMeta.Presentation)
	}
}

func TestScreenReaderLifecyclePersistsLatestContextAlongsideTotals(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "screen-usage"
	if err := repo.Save(sessionID, projectDir, nil); err != nil {
		t.Fatal(err)
	}
	tracker := ui.NewCostTracker("test-model")
	tracker.RecordTurnUsage(types.Usage{InputTokens: 1000, OutputTokens: 120, CacheReadInputTokens: 400}, 0)
	tracker.RecordTurnUsage(types.Usage{InputTokens: 500, OutputTokens: 80, CacheReadInputTokens: 200}, 0)
	cfg := TUIREPLConfig{
		Engine: screenReaderLifecycleEngine{}, Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir,
	}

	if err := persistScreenReaderLifecycle(cfg, tracker); err != nil {
		t.Fatal(err)
	}
	meta, _, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Usage == nil || meta.Usage.InputTokens != 1500 || meta.Usage.OutputTokens != 200 || meta.Usage.CacheReadTokens != 600 {
		t.Fatalf("persisted cumulative usage = %+v, want input/output/cache 1500/200/600", meta.Usage)
	}
	if !meta.Usage.RoundUsageKnown || meta.Usage.LastInputTokens != 500 || meta.Usage.LastOutputTokens != 80 || meta.Usage.LastCacheReadTokens != 200 {
		t.Fatalf("persisted latest context = %+v, want input/output/cache 500/80/200", meta.Usage)
	}
	if meta.Usage.CostKnown {
		t.Fatalf("unknown model pricing was persisted as a known cost: %+v", meta.Usage)
	}
	restored := ui.NewCostTracker("test-model")
	if err := restoreScreenReaderLifecycle(cfg, restored); err != nil {
		t.Fatal(err)
	}
	if restored.CostKnown() {
		t.Fatal("screen-reader lifecycle restore erased the unknown session cost")
	}
}

func TestScreenReaderClearViewDoesNotClearConversation(t *testing.T) {
	sessionID := "keep-session"
	cfg := TUIREPLConfig{SessionID: &sessionID}
	tracker := ui.NewCostTracker("test-model")
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	handled, exit, err := handleScreenReaderCommand(context.Background(), cfg, renderer, tracker, "/clear view")
	if err != nil || !handled || exit {
		t.Fatalf("clear view result: handled=%v exit=%v err=%v", handled, exit, err)
	}
	if sessionID != "keep-session" {
		t.Fatalf("clear view changed conversation identity to %q", sessionID)
	}
	if !bytes.Contains(output.Bytes(), []byte("model context unchanged")) {
		t.Fatalf("clear view receipt missing context guarantee: %q", output.String())
	}
	if strings.Count(output.String(), "model context unchanged") != 1 ||
		!strings.Contains(output.String(), "Command /clear: running view") ||
		!strings.Contains(output.String(), "Command /clear: succeeded") {
		t.Fatalf("clear view did not retain one legacy body inside typed lifecycle: %q", output.String())
	}
}

func TestScreenReaderExitUsesTypedCommandLifecycle(t *testing.T) {
	sessionID := "active"
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	handled, exit, err := handleScreenReaderCommand(context.Background(), TUIREPLConfig{SessionID: &sessionID}, renderer, ui.NewCostTracker("test"), "/exit")
	if err != nil || !handled || !exit {
		t.Fatalf("exit: handled=%t exit=%t err=%v", handled, exit, err)
	}
	if !strings.Contains(output.String(), "Command /exit: running exit") ||
		!strings.Contains(output.String(), "Command /exit: exit requested") {
		t.Fatalf("exit typed lifecycle missing: %q", output.String())
	}
}

func TestScreenReaderSpecialCommandLifecycleClassifiesCancellationAndTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "cancelled", err: context.Canceled, want: "Command /compact: cancelled"},
		{name: "timed_out", err: context.DeadlineExceeded, want: "Command /compact: timed out"},
		{name: "denied", err: os.ErrPermission, want: "Command /compact: denied"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			renderer := ui.NewScreenReaderRenderer(&output, nil)
			defer renderer.Close()
			err := runScreenReaderSpecialCommandLifecycle(renderer, "compact", "compact", "", commands.CommandOutcomeSucceeded, func() error { return test.err })
			if !errors.Is(err, test.err) || !strings.Contains(output.String(), test.want) {
				t.Fatalf("special lifecycle err=%v output=%q", err, output.String())
			}
		})
	}
}

func TestScreenReaderSpecialCommandErrorsStillEmitTypedLifecycle(t *testing.T) {
	sessionID := "active"
	for _, test := range []struct{ input, command string }{{"/export", "export"}, {"/resume", "resume"}, {"/r", "resume"}, {"/fork", "fork"}} {
		t.Run(test.input, func(t *testing.T) {
			var output bytes.Buffer
			renderer := ui.NewScreenReaderRenderer(&output, nil)
			defer renderer.Close()
			handled, exit, err := handleScreenReaderCommand(context.Background(), TUIREPLConfig{SessionID: &sessionID}, renderer, ui.NewCostTracker("test"), test.input)
			if !handled || exit || err == nil {
				t.Fatalf("%s: handled=%t exit=%t err=%v", test.input, handled, exit, err)
			}
			if !strings.Contains(output.String(), "Command /"+test.command+": running") ||
				!strings.Contains(output.String(), "Command /"+test.command+": failed") {
				t.Fatalf("%s typed lifecycle missing: %q", test.input, output.String())
			}
		})
	}
}

func TestScreenReaderHelpIncludesPermanentHistoryDeletion(t *testing.T) {
	sessionID := "active"
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	handled, exit, err := handleScreenReaderCommand(context.Background(), TUIREPLConfig{SessionID: &sessionID}, renderer, ui.NewCostTracker("test"), "/help")
	if err != nil || !handled || exit {
		t.Fatalf("help: handled=%v exit=%v err=%v", handled, exit, err)
	}
	if !strings.Contains(output.String(), "/delete-history SESSION_ID") {
		t.Fatalf("help omitted permanent deletion command: %q", output.String())
	}
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	for _, command := range registry.All() {
		if !strings.Contains(output.String(), "/"+command.Name()) {
			t.Errorf("help omitted registered command /%s: %q", command.Name(), output.String())
		}
	}
}

func TestScreenReaderRoutesEveryPreviouslyUnknownRegisteredCommandAndAlias(t *testing.T) {
	root := t.TempDir()
	repo := session.NewRepository(filepath.Join(root, "sessions"))
	projectDir := repo.ProjectDirForCWD(root)
	sessionID, cwd := "screen-reader-routing", root
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("routing")}); err != nil {
		t.Fatal(err)
	}
	routingEngine := screenReaderClearEngine{&sessionSwitcherTestEngine{sessions: &sessionSwitcherTestSessions{
		messages: map[string][]types.Message{sessionID: {types.UserMessage("routing")}},
	}}}
	cfg := TUIREPLConfig{
		Engine: routingEngine, Repo: repo, SessionID: &sessionID,
		SessionProjectDir: &projectDir, CWD: &cwd, SessionTransitionMu: &sync.Mutex{},
	}
	tests := map[string]string{
		"search": "needle", "editor": "", "mouse": "on", "activity": "list", "detail": "obs-1 summary",
		"model": "", "session": "list", "config": "list", "status": "", "context": "", "init": "",
		"review": "", "language": "show", "activities": "list", "cfg": "list", "st": "", "ctx": "",
		"setup": "", "rv": "", "lang": "show",
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			renderer := ui.NewScreenReaderRenderer(&output, nil)
			defer renderer.Close()
			input := "/" + name
			if args != "" {
				input += " " + args
			}
			handled, exit, err := handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), input)
			if !handled || exit {
				t.Fatalf("route %s: handled=%t exit=%t err=%v", input, handled, exit, err)
			}
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "unknown command") {
				t.Fatalf("registered command entered unknown route: %s: %v", input, err)
			}
			if strings.Contains(strings.ToLower(output.String()), "unknown command") {
				t.Fatalf("registered command rendered as unknown: %s: %q", input, output.String())
			}
		})
	}
}

func TestScreenReaderDeleteHistoryProtectsActiveSessionBeforePrompt(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	activeID := "active-delete-protected"
	if err := repo.Save(activeID, projectDir, []types.Message{types.UserMessage("keep")}); err != nil {
		t.Fatal(err)
	}
	engine := &screenReaderDeleteEngine{repo: repo}
	cfg := TUIREPLConfig{Engine: engine, Repo: repo, SessionID: &activeID, SessionProjectDir: &projectDir, SessionTransitionMu: &sync.Mutex{}}
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	handled, exit, err := handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), "/delete-history "+activeID)
	if !handled || exit || err == nil || !strings.Contains(err.Error(), "active session") {
		t.Fatalf("active delete: handled=%v exit=%v err=%v", handled, exit, err)
	}
	if strings.Contains(output.String(), "Decision required") {
		t.Fatalf("active session reached deletion prompt: %q", output.String())
	}
	if engine.calls != 0 {
		t.Fatalf("active session delete calls = %d", engine.calls)
	}
}

func TestScreenReaderDeleteHistoryRequiresStructuredApproval(t *testing.T) {
	for _, test := range []struct {
		name       string
		choice     string
		wantDelete bool
		wantText   string
	}{
		{name: "reject", choice: "2", wantText: "not deleted"},
		{name: "approve", choice: "1", wantDelete: true, wantText: "permanently deleted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := session.NewRepository(t.TempDir())
			projectDir := repo.ProjectDirForCWD(t.TempDir())
			activeID, targetID := "active-delete", "target-delete"
			for _, id := range []string{activeID, targetID} {
				if err := repo.Save(id, projectDir, []types.Message{types.UserMessage(id)}); err != nil {
					t.Fatal(err)
				}
			}
			eng := &screenReaderDeleteEngine{repo: repo}
			cfg := TUIREPLConfig{Engine: eng, Repo: repo, SessionID: &activeID, SessionProjectDir: &projectDir, SessionTransitionMu: &sync.Mutex{}}
			reader, writer := io.Pipe()
			output := &lockedScreenReaderOutput{updated: make(chan struct{}, 1)}
			renderer := ui.NewScreenReaderRenderer(output, reader)
			renderer.SetDecisionRecorder(screenReaderDecisionRecorder(cfg))
			defer func() {
				_ = writer.Close()
				_ = renderer.Close()
			}()

			done := make(chan error, 1)
			go func() {
				_, _, err := handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), "/delete-history "+targetID)
				done <- err
			}()
			waitForLockedScreenReaderText(t, output, "Decision choice: ")
			prompt := output.String()
			for _, want := range []string{
				"Action: " + i18n.Text(i18n.LangEN, i18n.KeyDeleteHistoryAction), "Target: " + targetID,
				i18n.Text(i18n.LangEN, i18n.KeyDeleteHistoryImpact), i18n.Text(i18n.LangEN, i18n.KeyDeleteHistoryRisk),
				i18n.Text(i18n.LangEN, i18n.KeyDeleteHistoryRule), "Choice 1: Allow once (allow_once)", "Choice 2: Reject (reject)",
			} {
				if !strings.Contains(prompt, want) {
					t.Fatalf("structured deletion prompt omitted %q:\n%s", want, prompt)
				}
			}
			if _, err := io.WriteString(writer, "decision decision:delete-history:"+targetID+" "+test.choice+"\n"); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("delete-history command did not complete")
			}
			if !strings.Contains(output.String(), test.wantText) {
				t.Fatalf("deletion receipt omitted %q:\n%s", test.wantText, output.String())
			}
			_, _, loadErr := repo.LoadByID(targetID, projectDir)
			if test.wantDelete {
				if loadErr == nil || eng.calls != 1 || eng.deletedID != targetID || eng.projectDir != projectDir {
					t.Fatalf("approved delete: loadErr=%v engine=%+v", loadErr, eng)
				}
			} else {
				if loadErr != nil || eng.calls != 0 {
					t.Fatalf("rejected delete mutated history: loadErr=%v calls=%d", loadErr, eng.calls)
				}
			}
		})
	}
}

func waitForLockedScreenReaderText(t *testing.T, output *lockedScreenReaderOutput, want string) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for !strings.Contains(output.String(), want) {
		select {
		case <-output.updated:
		case <-timer.C:
			t.Fatalf("screen-reader output did not contain %q:\n%s", want, output.String())
		}
	}
}

func TestScreenReaderLifecyclePreservesLatestContextWithoutANewTurn(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "screen-restored-usage"
	if err := repo.Save(sessionID, projectDir, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMeta(sessionID, projectDir, session.SessionMeta{Usage: &session.SessionUsageMeta{
		InputTokens: 1500, OutputTokens: 200, CacheReadTokens: 600,
		HasCompacted: true, InputTokensAtCompact: 1000, CacheReadAtCompact: 400,
		RoundUsageKnown: true, CompactionCount: 1,
		CompletedRoundInputTokens: 1000, CompletedRoundOutputTokens: 120,
		LastInputTokens: 500, LastOutputTokens: 80, LastCacheReadTokens: 200,
	}}); err != nil {
		t.Fatal(err)
	}
	tracker := ui.NewCostTracker("test-model")
	tracker.RestoreSession("test-model", 1500, 200, 600, 0, 0, 0)
	tracker.RestoreCompactionBaseline(true, 1000, 400)
	cfg := TUIREPLConfig{
		Engine: screenReaderLifecycleEngine{}, Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir,
	}

	if err := persistScreenReaderLifecycle(cfg, tracker); err != nil {
		t.Fatal(err)
	}
	meta, _, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Usage == nil || !meta.Usage.RoundUsageKnown || meta.Usage.CompactionCount != 1 ||
		meta.Usage.CompletedRoundInputTokens != 1000 || meta.Usage.CompletedRoundOutputTokens != 120 ||
		meta.Usage.LastInputTokens != 500 || meta.Usage.LastOutputTokens != 80 || meta.Usage.LastCacheReadTokens != 200 ||
		!meta.Usage.HasCompacted || meta.Usage.InputTokensAtCompact != 1000 || meta.Usage.CacheReadAtCompact != 400 {
		t.Fatalf("persist without a new turn erased latest context: %+v", meta.Usage)
	}
}

func TestRestoreScreenReaderLifecycleRejectsCorruptMeta(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "screen-corrupt-meta"
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("preserve")}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectDir+string(os.PathSeparator)+sessionID+".meta.json", []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := TUIREPLConfig{
		Engine: screenReaderLifecycleEngine{}, Repo: repo, SessionID: &sessionID,
		SessionProjectDir: &projectDir, CWD: &cwd,
	}

	err := restoreScreenReaderLifecycle(cfg, ui.NewCostTracker("test-model"))
	if err == nil || !strings.Contains(err.Error(), i18n.Text(i18n.LangEN, i18n.KeyREPLErrorLoadScreenReaderMetadata)) {
		t.Fatalf("restore error = %v, want corrupt lifecycle rejection", err)
	}
}

func TestScreenReaderClearCommitsPreparedEmptySessionAndPreservesOldLifecycle(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	activeID := "screen-old"
	oldMessages := []types.Message{types.UserMessage("screen reader old transcript")}
	if err := repo.Save(activeID, projectDir, oldMessages); err != nil {
		t.Fatal(err)
	}
	base := &sessionSwitcherTestEngine{
		sessions: &sessionSwitcherTestSessions{messages: map[string][]types.Message{activeID: oldMessages}},
	}
	eng := screenReaderClearEngine{sessionSwitcherTestEngine: base}
	publishedID := ""
	cfg := TUIREPLConfig{
		Engine: eng, Repo: repo, SessionID: &activeID, SessionProjectDir: &projectDir, CWD: &cwd,
		PermChecker: permissions.NewChecker(permissions.ModeAskAlways, nil), SessionTransitionMu: &sync.Mutex{},
		PublishSessionID: func(id string) { publishedID = id },
	}
	tracker := ui.NewCostTracker("test-model")
	tracker.RestoreSession("test-model", 9, 4, 0, 0, 0, 0)
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()

	if err := clearScreenReaderConversation(context.Background(), cfg, renderer, tracker); err != nil {
		t.Fatal(err)
	}
	if activeID == "screen-old" || publishedID != activeID || len(base.transcript) != 0 {
		t.Fatalf("screen clear did not atomically activate empty session: id=%q registry=%q transcript=%#v", activeID, publishedID, base.transcript)
	}
	oldPersisted, _, err := repo.LoadByID("screen-old", projectDir)
	if err != nil || len(oldPersisted) != 1 {
		t.Fatalf("old screen session not recoverable: messages=%#v err=%v", oldPersisted, err)
	}
	meta, _, err := repo.GetMeta("screen-old", projectDir)
	if err != nil || meta.Usage == nil || meta.Usage.InputTokens != 9 || meta.Usage.OutputTokens != 4 {
		t.Fatalf("old screen lifecycle not preserved: meta=%+v err=%v", meta, err)
	}
	input, outputTokens, _, _ := tracker.TotalUsage()
	if input != 0 || outputTokens != 0 {
		t.Fatalf("new screen session inherited old usage: input=%d output=%d", input, outputTokens)
	}
	if !bytes.Contains(output.Bytes(), []byte("Clear conversation receipt")) {
		t.Fatalf("clear receipt missing: %q", output.String())
	}
}

func TestScreenReaderClearCommitFailureRestoresOldPermissionMode(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	activeID := "screen-old"
	oldMessages := []types.Message{types.UserMessage("old")}
	if err := repo.Save(activeID, projectDir, oldMessages); err != nil {
		t.Fatal(err)
	}
	commitErr := errors.New("commit unavailable")
	base := &sessionSwitcherTestEngine{
		sessions:  &sessionSwitcherTestSessions{messages: map[string][]types.Message{activeID: oldMessages}},
		commitErr: commitErr,
	}
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	cfg := TUIREPLConfig{
		Engine: screenReaderClearEngine{sessionSwitcherTestEngine: base}, Repo: repo,
		SessionID: &activeID, SessionProjectDir: &projectDir, CWD: &cwd,
		PermChecker: checker, SessionTransitionMu: &sync.Mutex{},
	}
	renderer := ui.NewScreenReaderRenderer(io.Discard, nil)
	defer renderer.Close()
	err := clearScreenReaderConversation(context.Background(), cfg, renderer, ui.NewCostTracker("test-model"))
	if !errors.Is(err, commitErr) {
		t.Fatalf("clear error = %v, want commit failure", err)
	}
	if activeID != "screen-old" || checker.Mode() != permissions.ModeAskAlways || base.commits != 0 {
		t.Fatalf("failed clear leaked target state: id=%q mode=%v commits=%d", activeID, checker.Mode(), base.commits)
	}
	if len(base.sessions.messages) != 1 {
		t.Fatalf("failed clear retained candidate session: %#v", base.sessions.messages)
	}
}

func TestScreenReaderClearCommitAndPermissionRollbackFailureFailsClosed(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	activeID := "screen-old"
	oldMessages := []types.Message{types.UserMessage("old")}
	if err := repo.Save(activeID, projectDir, oldMessages); err != nil {
		t.Fatal(err)
	}
	commitErr := errors.New("commit unavailable")
	base := &sessionSwitcherTestEngine{
		sessions:  &sessionSwitcherTestSessions{messages: map[string][]types.Message{activeID: oldMessages}},
		commitErr: commitErr,
	}
	runtimeMode := "default"
	runtimeScope := tools.NewRuntimeScope(cwd, true)
	runtimeScope.SetPermissionModeDispatcher(func() string { return runtimeMode }, func(mode string) error {
		if mode == "default" && runtimeMode == "bypassPermissions" {
			return errors.New("default permission runtime unavailable")
		}
		runtimeMode = mode
		return nil
	})
	failClosed := false
	cfg := TUIREPLConfig{
		Engine: screenReaderClearEngine{sessionSwitcherTestEngine: base}, Repo: repo,
		SessionID: &activeID, SessionProjectDir: &projectDir, CWD: &cwd,
		RuntimeScope: runtimeScope, SessionTransitionMu: &sync.Mutex{},
		FailClosed: func(error) { failClosed = true },
	}
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()

	err := clearScreenReaderConversation(context.Background(), cfg, renderer, ui.NewCostTracker("test-model"))
	if !errors.Is(err, commitErr) || !strings.Contains(err.Error(), "failed closed") {
		t.Fatalf("clear error = %v, want commit failure with fail-closed receipt", err)
	}
	if !failClosed {
		t.Fatal("screen-reader clear double rollback failure did not fail closed")
	}
	if activeID != "screen-old" || runtimeMode != "bypassPermissions" || runtimeScope.PermissionMode() != "bypassPermissions" {
		t.Fatalf("failed clear hid surviving state: id=%q dispatcher=%q scope=%q", activeID, runtimeMode, runtimeScope.PermissionMode())
	}
	if !strings.Contains(output.String(), "Clear failed-closed receipt") || !strings.Contains(strings.ToLower(output.String()), "auto") {
		t.Fatalf("screen-reader did not announce surviving permission mode: %q", output.String())
	}
}

func TestScreenReaderResumePermissionRestoreAndRollbackFailureFailsClosed(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	activeID := "screen-old"
	targetID := "screen-target"
	for id, text := range map[string]string{activeID: "old", targetID: "target"} {
		if err := repo.Save(id, projectDir, []types.Message{types.UserMessage(text)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SaveMeta(targetID, projectDir, session.SessionMeta{
		CWD: cwd, Presentation: &session.SessionPresentationMeta{PermissionMode: "auto"},
	}); err != nil {
		t.Fatal(err)
	}
	runtimeMode := "default"
	targetModeErr := errors.New("target permission runtime unavailable")
	rollbackModeErr := errors.New("old permission runtime unavailable")
	runtimeScope := tools.NewRuntimeScope(cwd, true)
	runtimeScope.SetPermissionModeDispatcher(func() string { return runtimeMode }, func(mode string) error {
		switch mode {
		case "bypassPermissions":
			return targetModeErr
		case "default":
			return rollbackModeErr
		default:
			return nil
		}
	})
	activeProject := projectDir
	activeCWD := cwd
	failClosed := false
	cfg := TUIREPLConfig{
		Engine: screenReaderLifecycleEngine{}, Repo: repo,
		SessionID: &activeID, SessionProjectDir: &activeProject, CWD: &activeCWD,
		RuntimeScope: runtimeScope, SessionTransitionMu: &sync.Mutex{},
		FailClosed: func(error) { failClosed = true },
	}
	cfg.SwitchSession = func(_ context.Context, entry commands.SessionListEntry) error {
		activeID, activeProject, activeCWD = entry.ID, entry.ProjectDir, entry.CWD
		return nil
	}
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()

	err := resumeScreenReaderSession(context.Background(), cfg, renderer, ui.NewCostTracker("test-model"), targetID)
	if !errors.Is(err, targetModeErr) || !strings.Contains(err.Error(), "failed closed") {
		t.Fatalf("resume error = %v, want target failure with fail-closed receipt", err)
	}
	if !failClosed {
		t.Fatal("screen-reader resume double mode rollback failure did not fail closed")
	}
	if activeID != "screen-old" || runtimeScope.PermissionMode() != "default" {
		t.Fatalf("failed resume projection disagrees: id=%q mode=%q", activeID, runtimeScope.PermissionMode())
	}
	if !strings.Contains(output.String(), "Resume failed-closed receipt") || !strings.Contains(strings.ToLower(output.String()), "ask") {
		t.Fatalf("screen-reader did not announce surviving permission mode: %q", output.String())
	}
}
