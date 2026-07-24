package main

import (
	"testing"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/tui"
	"github.com/agent-dance/luban/ui"
)

type freshSessionGenerationRenderer struct {
	ui.NoOpRenderer
	state *tui.AppState
	text  string
}

func (r *freshSessionGenerationRenderer) AdmitContextGeneration(ctx ui.ToolEventContext) bool {
	return r.state.SessionID.Get() == ctx.SessionID &&
		r.state.AdmitRuntimeGeneration(ctx.SessionEpoch, ctx.ContextGeneration, ctx.ContextGenerationPersisted)
}

func (r *freshSessionGenerationRenderer) Text(text string) { r.text += text }

func TestFreshTUISessionAdmitsAssistantResponseEvents(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	const sessionID = "fresh-tui-response"

	snapshot, err := prepareTUISessionSnapshot(TUIREPLConfig{Repo: repo}, sessionID, projectDir, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextGeneration != 0 || snapshot.ContextGenerationPersisted {
		t.Fatalf("fresh context generation = %d persisted=%t, want unpersisted generation zero", snapshot.ContextGeneration, snapshot.ContextGenerationPersisted)
	}

	state := tui.NewAppState()
	if err := state.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	renderer := &freshSessionGenerationRenderer{state: state}
	base := ui.ToolEventContext{
		SessionID:                  sessionID,
		SessionEpoch:               state.SessionEpoch.Get(),
		ContextGeneration:          state.ContextGeneration.Get(),
		ContextGenerationPersisted: state.ContextGenerationPersisted.Get(),
	}
	handle, cleanup := makeTUIEventHandler(renderer, nil, nil, base)
	t.Cleanup(cleanup)
	handle(loop.Event{Type: loop.EventText, Text: "assistant response", TurnCount: 1})

	if renderer.text != "assistant response" {
		t.Fatalf("fresh-session response event was dropped: %q", renderer.text)
	}
}
