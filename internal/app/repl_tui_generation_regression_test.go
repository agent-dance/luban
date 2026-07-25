package app

import (
	"github.com/agent-dance/luban/internal/contracts/stream"
	"testing"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/internal/ui/tui"
)

type freshSessionGenerationRenderer struct {
	ui.QuietRenderer
	state *tui.AppState
	text  string
}

func (r *freshSessionGenerationRenderer) AdmitContextGeneration(ctx presentation.ToolEventContext) bool {
	return r.state.SessionID.Get() == ctx.SessionID &&
		r.state.AdmitRuntimeGeneration(ctx.SessionEpoch, ctx.ContextGeneration, ctx.ContextGenerationPersisted)
}

func (r *freshSessionGenerationRenderer) Text(text string) { r.text += text }

func TestFreshTUISessionAdmitsAssistantResponseEvents(t *testing.T) {
	const sessionID = "fresh-tui-response"
	identity := tui.SessionIdentity{SessionID: sessionID, Epoch: 1}
	projection, err := tui.ProjectPersistedMessages(identity, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tui.SessionSnapshot{Identity: identity, Projection: projection}
	if snapshot.ContextGeneration != 0 || snapshot.ContextGenerationPersisted {
		t.Fatalf("fresh context generation = %d persisted=%t, want unpersisted generation zero", snapshot.ContextGeneration, snapshot.ContextGenerationPersisted)
	}

	state := tui.NewAppState()
	if err := state.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	renderer := &freshSessionGenerationRenderer{state: state}
	base := presentation.ToolEventContext{
		SessionID:                  sessionID,
		SessionEpoch:               state.SessionEpoch.Get(),
		ContextGeneration:          state.ContextGeneration.Get(),
		ContextGenerationPersisted: state.ContextGenerationPersisted.Get(),
	}
	handle, cleanup := makeTUIEventHandler(renderer, nil, nil, base)
	t.Cleanup(cleanup)
	handle(stream.Event{Type: stream.EventText, Text: "assistant response", TurnCount: 1})

	if renderer.text != "assistant response" {
		t.Fatalf("fresh-session response event was dropped: %q", renderer.text)
	}
}
