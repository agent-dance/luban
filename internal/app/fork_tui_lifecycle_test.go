package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/internal/ui/tui"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type forkPickerLifecycleProvider struct{}

func (forkPickerLifecycleProvider) Name() string    { return "fork-picker" }
func (forkPickerLifecycleProvider) ModelID() string { return "fork-picker-model" }
func (forkPickerLifecycleProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	stream := make(chan types.StreamEvent)
	close(stream)
	return stream, nil
}

type forkPickerLifecycleEngine struct {
	engine.Engine
	sessions engine.SessionManager
}

func (e *forkPickerLifecycleEngine) Sessions() engine.SessionManager { return e.sessions }
func (*forkPickerLifecycleEngine) Provider() provider.Provider       { return forkPickerLifecycleProvider{} }

func TestTUIForkPickerUsesRuntimeContextAfterInputCompletes(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "fork-picker-source"
	messages := []types.Message{types.UserMessage("question"), types.AssistantMessage("answer")}
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}

	app := task28SurfaceTUIApp(t)
	t.Cleanup(func() { _ = app.Close() })
	identity := tui.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}
	projection, err := tui.ProjectPersistedMessages(identity, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.ApplySessionSnapshot(tui.SessionSnapshot{Identity: identity, Projection: projection}); err != nil {
		t.Fatal(err)
	}

	manager := engine.NewRepositorySessionManager(repo, func() string { return projectDir })
	eng := &forkPickerLifecycleEngine{sessions: manager}
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	queryLoop := &engineQueryLooper{eng: eng, sessionID: func() string { return sessionID }, model: eng.Provider().ModelID()}
	store := &sessionStoreAdapter{repo: repo, currentProjectDir: func() string { return projectDir }}
	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	t.Cleanup(cancelRuntime)

	var terminalCtx context.Context
	var openedForkID string
	cfg := TUIREPLConfig{
		Engine: eng, Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd,
		CommandMu: &sync.Mutex{}, SessionTransitionMu: &sync.Mutex{},
		OpenSessionTerminal: func(ctx context.Context, forkID, _, _, _ string) error {
			terminalCtx = ctx
			openedForkID = forkID
			return ctx.Err()
		},
	}

	submission := tuiInputSubmission{text: "/fork"}
	if !prepareTUIInputAdmission(runtimeCtx, app.State(), &submission) {
		t.Fatal("fork command input was not admitted")
	}
	handleTUIInputSubmission(
		submission.ctx, cfg, app, registry, store, queryLoop, ui.NewCostTracker(eng.Provider().ModelID()),
		submission, nil, func(fn func()) bool { fn(); return true },
	)
	// Production releases the per-input context as soon as /fork has opened
	// the picker. The later selection must remain attached to runtimeCtx.
	submission.abort()
	if !errors.Is(submission.ctx.Err(), context.Canceled) {
		t.Fatalf("input context error = %v, want context canceled", submission.ctx.Err())
	}

	picker := app.State().ForkPicker.Get()
	if picker == nil || len(picker.Entries) == 0 {
		t.Fatal("fork picker did not open")
	}
	entry := picker.Entries[0]
	app.State().ForkPicker.Set(nil)
	picker.OnSelect(entry)

	if terminalCtx == nil || openedForkID == "" {
		t.Fatal("fork selection did not reach terminal launch")
	}
	if err := terminalCtx.Err(); err != nil {
		t.Fatalf("terminal launch inherited completed input context: %v", err)
	}
	if loaded, _, err := repo.LoadByID(openedForkID, projectDir); err != nil || len(loaded) != len(messages) {
		t.Fatalf("forked session load length=%d err=%v", len(loaded), err)
	}

	cancelRuntime()
	if !errors.Is(terminalCtx.Err(), context.Canceled) {
		t.Fatalf("terminal context did not retain TUI lifecycle cancellation: %v", terminalCtx.Err())
	}
}
