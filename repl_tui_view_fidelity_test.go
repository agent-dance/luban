package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/tui"
	"github.com/agent-dance/luban/types"
)

func viewFidelityMessages() []types.Message {
	return []types.Message{
		types.UserMessage("inspect"),
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "read-a", Name: "Read", Input: map[string]any{"file_path": "a.go"}},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "read-b", Name: "Read", Input: map[string]any{"file_path": "b.go"}},
		}},
		types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "read-a", Content: "a", Outcome: types.ToolOutcomeSucceeded}),
		types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "read-b", Content: "b", Outcome: types.ToolOutcomeSucceeded}),
		types.AssistantMessage("done"),
	}
}

func viewFidelityState(t *testing.T, identity tui.SessionIdentity, messages []types.Message, receipt string) *tui.AppState {
	t.Helper()
	projection, err := tui.ProjectPersistedMessages(identity, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := tui.NewAppState()
	if err := state.ApplySessionSnapshot(tui.SessionSnapshot{Identity: identity, Projection: projection}); err != nil {
		t.Fatal(err)
	}
	visible := append([]tui.Message(nil), state.Messages.Get()...)
	visible = append(visible[:1], append([]tui.Message{{Kind: tui.MsgInfo, Text: receipt}}, visible[1:]...)...)
	state.Messages.Set(visible)
	for _, item := range tui.BuildTranscriptToolSegments(visible) {
		if item.Segment != nil {
			state.SetToolSegmentExpanded(item.Segment.ID, true)
			break
		}
	}
	state.SetInteractionDraft("draft")
	return state
}

func TestForkCheckpointSessionIdentityRequiresDurableExactScope(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	ref := session.Ref{ID: "missing-fork-scope", ProjectDir: projectDir}
	if _, err := forkCheckpointSessionIdentity(repo, ref, 1); !errors.Is(err, session.ErrCorruptSessionHistory) {
		t.Fatalf("missing scope error = %v, want ErrCorruptSessionHistory", err)
	}
	if err := repo.Save(ref.ID, ref.ProjectDir, []types.Message{types.UserMessage("durable")}); err != nil {
		t.Fatal(err)
	}
	if _, err := forkCheckpointSessionIdentity(repo, ref, 1); err != nil {
		t.Fatalf("durable exact scope rejected: %v", err)
	}
}

func bindViewFidelityStateToCurrentGeneration(t *testing.T, repo *session.Repository, state *tui.AppState, sessionID, projectDir string) {
	t.Helper()
	scope, err := repo.StoreForProjectDir(projectDir).MessageControlScope(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Bound() || !state.PublishContextGeneration(sessionID, state.SessionEpoch.Get(), scope.ContextGeneration()) {
		t.Fatalf("could not bind view state to current scope: bound=%v generation=%d", scope.Bound(), scope.ContextGeneration())
	}
}

func TestTaskBindingDoesNotOverwriteRestoredCheckpointBeforeNewEvent(t *testing.T) {
	state := tui.NewAppState()
	state.RefreshTasksView([]tui.TaskViewItem{{ID: "checkpoint-task", Subject: "restored", Status: "in_progress"}})
	tool := tools.NewTaskCreateTool(tools.NewTaskStore())
	unbind := bindTaskCreateViewState(tool, state)
	defer unbind()
	if items := state.TaskViewItems.Get(); len(items) != 1 || items[0].ID != "checkpoint-task" {
		t.Fatalf("startup binding overwrote restored task view: %+v", items)
	}
	result, err := tool.Execute(context.Background(), map[string]any{"subject": "new event", "description": "authoritative post-resume update"})
	if err != nil || result.IsError {
		t.Fatalf("TaskCreate: result=%+v err=%v", result, err)
	}
	if items := state.TaskViewItems.Get(); len(items) != 1 || items[0].Subject != "new event" {
		t.Fatalf("post-resume task event did not replace restored view: %+v", items)
	}
}

func TestPersistAndPrepareTUISessionUsesExactViewCheckpoint(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "resume-view"
	messages := viewFidelityMessages()
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	state := viewFidelityState(t, tui.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}, messages, "LOCAL RESUME RECEIPT")
	state.SessionUsageKnown.Set(true)
	state.SessionCostKnown.Set(false)
	bindViewFidelityStateToCurrentGeneration(t, repo, state, sessionID, projectDir)
	cfg := TUIREPLConfig{Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd}
	if err := persistTUISessionLifecycle(cfg, state); err != nil {
		t.Fatal(err)
	}

	snapshot, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, messages)
	if err != nil {
		t.Fatal(err)
	}
	target := tui.NewAppState()
	if err := target.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if !viewMessagesContain(target.Messages.Get(), "LOCAL RESUME RECEIPT") {
		t.Fatal("resume rebuilt only model transcript and lost local presentation")
	}
	if target.ActiveSessionInteraction().InputDraft != "draft" {
		t.Fatalf("resume draft = %q", target.ActiveSessionInteraction().InputDraft)
	}
	if snapshot.SessionCostKnown == nil || *snapshot.SessionCostKnown || target.SessionCostKnown.Get() {
		t.Fatalf("resume changed unknown session cost: snapshot=%v target=%v", snapshot.SessionCostKnown, target.SessionCostKnown.Get())
	}
	assertFirstViewSegmentExpanded(t, target)
}

func TestSettledQueryBoundaryCommitsExactViewBeforeSessionBecomesForkable(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "settled-boundary"
	messages := viewFidelityMessages()
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	state := viewFidelityState(t, tui.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}, messages, "TERMINAL BOUNDARY RECEIPT")
	bindViewFidelityStateToCurrentGeneration(t, repo, state, sessionID, projectDir)
	cfg := TUIREPLConfig{Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd}
	generation := state.SetQueryCancel(func() {})
	if err := persistTUISessionLifecycle(cfg, state); err != nil {
		t.Fatal(err)
	}
	legacyProbe, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, messages)
	if err != nil {
		t.Fatal(err)
	}
	if viewMessagesContain(legacyProbe.Projection.Messages, "TERMINAL BOUNDARY RECEIPT") {
		t.Fatal("ordinary lifecycle persistence captured an in-flight query")
	}
	if err := persistSettledTUISessionLifecycle(cfg, state); err != nil {
		t.Fatal(err)
	}
	if !state.HasActiveQuery() {
		t.Fatal("query became inactive before the exact view commit completed")
	}
	exact, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, messages)
	if err != nil {
		t.Fatal(err)
	}
	if !viewMessagesContain(exact.Projection.Messages, "TERMINAL BOUNDARY RECEIPT") {
		t.Fatal("terminal boundary did not publish its exact view")
	}
	state.ClearQueryCancel(generation)
	if state.HasActiveQuery() {
		t.Fatal("query did not become inactive after the exact view commit")
	}
}

func TestPrepareExactCheckpointDoesNotDependOnLegacyEvidenceJournal(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "canonical-checkpoint"
	messages := viewFidelityMessages()
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	state := viewFidelityState(t, tui.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}, messages, "CANONICAL VIEW")
	bindViewFidelityStateToCurrentGeneration(t, repo, state, sessionID, projectDir)
	cfg := TUIREPLConfig{Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd}
	if err := persistTUISessionLifecycle(cfg, state); err != nil {
		t.Fatal(err)
	}
	journalDir := filepath.Join(repo.ArtifactsDir(sessionID, projectDir), "tui-details", ".observations")
	if err := os.MkdirAll(journalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalDir, "corrupt.json"), []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, messages)
	if err != nil {
		t.Fatalf("exact checkpoint was blocked by legacy journal parsing: %v", err)
	}
	if !viewMessagesContain(snapshot.Projection.Messages, "CANONICAL VIEW") {
		t.Fatal("prepare did not use the canonical exact checkpoint")
	}
}

func TestResumeCommandDoesNotAppendReceiptsToRestoredTarget(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sourceID, targetID := "resume-command-source", "resume-command-target"
	if err := repo.Save(sourceID, projectDir, []types.Message{types.UserMessage("source")}); err != nil {
		t.Fatal(err)
	}
	targetMessages := []types.Message{types.UserMessage("target"), types.AssistantMessage("ORIGINAL TARGET VIEW")}
	if err := repo.Save(targetID, projectDir, targetMessages); err != nil {
		t.Fatal(err)
	}
	state := viewFidelityState(t, tui.SessionIdentity{Namespace: projectDir, SessionID: sourceID, Epoch: 1}, []types.Message{types.UserMessage("source")}, "SOURCE")
	targetProjection, err := tui.ProjectPersistedMessages(tui.SessionIdentity{Namespace: projectDir, SessionID: targetID, Epoch: 2}, targetMessages, nil)
	if err != nil {
		t.Fatal(err)
	}
	targetSnapshot := tui.SessionSnapshot{Identity: tui.SessionIdentity{Namespace: projectDir, SessionID: targetID, Epoch: 2}, Projection: targetProjection}
	store := &sessionStoreAdapter{repo: repo, currentProjectDir: func() string { return projectDir }}
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	resume := registry.Find("resume")
	if resume == nil {
		t.Fatal("resume command is not registered")
	}

	for attempt := 0; attempt < 3; attempt++ {
		startedIn := state.SessionID.Get()
		var crossed atomic.Bool
		output := newSessionScopedTUICommandOutput(state, startedIn, crossed.Load, func(message string) {
			state.AppendMessage(tui.Message{Kind: tui.MsgInfo, Text: message})
		})
		ctx := &commands.Context{
			Language: i18n.LangEN, SessionID: startedIn, SessionStore: store, CWD: cwd,
			OnEvent: output,
			ResumeSession: func(commands.SessionListEntry) error {
				if err := state.ApplySessionSnapshot(targetSnapshot); err != nil {
					return err
				}
				crossed.Store(true)
				return nil
			},
		}
		if err := resume.Execute(ctx, targetID); err != nil {
			t.Fatal(err)
		}
		if got := state.Messages.Get(); len(got) != 2 || got[1].Text != "ORIGINAL TARGET VIEW" {
			t.Fatalf("resume attempt %d polluted target view: %+v", attempt+1, got)
		}
	}
}

func TestForkSessionFromSnapshotCopiesExactSelectedViewCheckpoint(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "fork-view-source"
	messages := viewFidelityMessages()
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	state := viewFidelityState(t, tui.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}, messages, "LOCAL FORK RECEIPT")
	bindViewFidelityStateToCurrentGeneration(t, repo, state, sessionID, projectDir)
	state.SetInteractionEditor("fork draft🙂", 4)
	var opened string
	cfg := TUIREPLConfig{
		Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd,
		OpenSessionTerminal: func(_ context.Context, forkID, _, _, _ string) error { opened = forkID; return nil },
	}
	fork, err := forkSessionFromSnapshotWithApp(context.Background(), cfg, directActivityApp{state: state}, messages, len(messages))
	if err != nil {
		t.Fatal(err)
	}
	if opened != fork.ID {
		t.Fatalf("opened=%q fork=%q", opened, fork.ID)
	}
	targetMessages, err := repo.Load(fork)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := prepareTUISessionSnapshot(cfg, fork.ID, fork.ProjectDir, 1, targetMessages)
	if err != nil {
		t.Fatal(err)
	}
	target := tui.NewAppState()
	if err := target.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if !viewMessagesContain(target.Messages.Get(), "LOCAL FORK RECEIPT") {
		t.Fatal("fork lost local presentation checkpoint")
	}
	if interaction := target.ActiveSessionInteraction(); interaction.InputDraft != "fork draft🙂" || interaction.InputCursor != 4 || !interaction.InputCursorSet {
		t.Fatalf("fork lost event-loop captured editor state: %+v", interaction)
	}
	assertFirstViewSegmentExpanded(t, target)
}

func TestForkSessionFromEarlierGenerationReprojectsStaleViewCheckpoint(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "fork-earlier-source"
	prefix := viewFidelityMessages()
	allMessages := append(append([]types.Message(nil), prefix...), types.UserMessage("later"), types.AssistantMessage("later answer"))
	if err := repo.Save(sessionID, projectDir, prefix); err != nil {
		t.Fatal(err)
	}
	prefixState := viewFidelityState(t, tui.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}, prefix, "EARLIER TURN RECEIPT")
	bindViewFidelityStateToCurrentGeneration(t, repo, prefixState, sessionID, projectDir)
	cfg := TUIREPLConfig{
		Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd,
		OpenSessionTerminal: func(context.Context, string, string, string, string) error { return nil },
	}
	queryGeneration := prefixState.SetQueryCancel(func() {})
	if err := persistSettledTUISessionLifecycle(cfg, prefixState); err != nil {
		t.Fatal(err)
	}
	if !prefixState.HasActiveQuery() {
		t.Fatal("settled boundary became forkable before its checkpoint commit returned")
	}
	prefixState.ClearQueryCancel(queryGeneration)
	if err := repo.Save(sessionID, projectDir, allMessages); err != nil {
		t.Fatal(err)
	}
	currentState := viewFidelityState(t, tui.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}, allMessages, "LATER TURN RECEIPT")
	bindViewFidelityStateToCurrentGeneration(t, repo, currentState, sessionID, projectDir)
	fork, err := forkSessionFromSnapshotWithView(context.Background(), cfg, currentState, allMessages, len(prefix))
	if err != nil {
		t.Fatal(err)
	}
	targetMessages, err := repo.Load(fork)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := prepareTUISessionSnapshot(cfg, fork.ID, fork.ProjectDir, 1, targetMessages)
	if err != nil {
		t.Fatal(err)
	}
	target := tui.NewAppState()
	if err := target.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if viewMessagesContain(target.Messages.Get(), "EARLIER TURN RECEIPT") || viewMessagesContain(target.Messages.Get(), "LATER TURN RECEIPT") {
		t.Fatalf("stale fork retained an unprovable frozen projection: %+v", target.Messages.Get())
	}
	if !viewMessagesContain(target.Messages.Get(), "inspect") || !viewMessagesContain(target.Messages.Get(), "done") {
		t.Fatalf("stale fork did not rebuild the selected model transcript: %+v", target.Messages.Get())
	}
	if target.ActiveSessionInteraction().InputDraft != "draft" {
		t.Fatalf("stale fork lost independently durable interaction state: %+v", target.ActiveSessionInteraction())
	}
}

func TestForkCheckpointFailureLeavesNoDiscoverableHalfFork(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "fork-cleanup-source"
	prefix := viewFidelityMessages()
	allMessages := append(append([]types.Message(nil), prefix...), types.UserMessage("later"), types.AssistantMessage("later answer"))
	if err := repo.Save(sessionID, projectDir, prefix); err != nil {
		t.Fatal(err)
	}
	prefixState := viewFidelityState(t, tui.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}, prefix, "PREFIX")
	bindViewFidelityStateToCurrentGeneration(t, repo, prefixState, sessionID, projectDir)
	if err := tui.SaveSessionViewCheckpoint(repo.ArtifactsDir(sessionID, projectDir), prefixState, prefix); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(sessionID, projectDir, allMessages); err != nil {
		t.Fatal(err)
	}
	checkpointFiles, err := filepath.Glob(filepath.Join(repo.ArtifactsDir(sessionID, projectDir), "tui-view", "checkpoints", "*.json"))
	if err != nil || len(checkpointFiles) != 1 {
		t.Fatalf("checkpoint files=%v err=%v", checkpointFiles, err)
	}
	if err := os.WriteFile(checkpointFiles[0], []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	currentState := viewFidelityState(t, tui.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}, allMessages, "CURRENT")
	bindViewFidelityStateToCurrentGeneration(t, repo, currentState, sessionID, projectDir)
	opened := false
	cfg := TUIREPLConfig{
		Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd,
		OpenSessionTerminal: func(context.Context, string, string, string, string) error { opened = true; return nil },
	}
	before, err := repo.Search(session.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := forkSessionFromSnapshotWithView(context.Background(), cfg, currentState, allMessages, len(prefix))
	if err == nil || !fork.IsZero() {
		t.Fatalf("corrupt checkpoint fork=%+v err=%v", fork, err)
	}
	if opened {
		t.Fatal("terminal opened for an incomplete fork")
	}
	after, err := repo.Search(session.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("checkpoint failure left a discoverable half-fork: before=%d after=%d", len(before), len(after))
	}
}

func viewMessagesContain(messages []tui.Message, needle string) bool {
	for _, message := range messages {
		if strings.Contains(message.Text, needle) {
			return true
		}
	}
	return false
}

func assertFirstViewSegmentExpanded(t *testing.T, state *tui.AppState) {
	t.Helper()
	for _, item := range tui.BuildTranscriptToolSegments(state.Messages.Get()) {
		if item.Segment == nil {
			continue
		}
		if !state.ToolSegmentExpanded(item.Segment.ID) {
			t.Fatalf("segment %q lost explicit expansion", item.Segment.ID)
		}
		return
	}
	t.Fatal("view has no tool segment")
}
