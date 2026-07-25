package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

type stringRuntimeErrorCapture struct {
	QuietRenderer
	message string
}

type runtimeWarningCapture struct {
	QuietRenderer
	message string
}

func (r *runtimeWarningCapture) Warning(message string) { r.message = message }

func TestDispatchRuntimeWarningEventUsesStrictProjection(t *testing.T) {
	renderer := &runtimeWarningCapture{}
	secret := "/Users/private/.ssh/id_ed25519?token=sk-warning-secret\x1b[2J"
	warning := runtimeevent.NewWarningEvent(
		types.RuntimeIdentity{SessionID: "private-session", TurnID: "private-turn"},
		i18n.KeyRuntimeAutoCompactFailed,
		nil,
		errors.New(secret),
		map[string]any{"authorization": "Bearer private-token", "project_root": "/private/project"},
	)
	presentation.DispatchRuntimeWarningEvent(renderer, warning, i18n.LangEN, true)
	if renderer.message != i18n.Text(i18n.LangEN, i18n.KeyRuntimeAutoCompactFailed) {
		t.Fatalf("runtime-warning message = %q", renderer.message)
	}
	for _, private := range []string{secret, "sk-warning-secret", "private-token", "/private/project", "private-session", "private-turn", "\x1b[2J"} {
		if strings.Contains(renderer.message, private) {
			t.Fatalf("runtime-warning projection leaked %q: %q", private, renderer.message)
		}
	}
}

func (r *stringRuntimeErrorCapture) Error(message string) { r.message = message }

func TestRuntimeErrorAdapterPreservesPrivateCauseAndStableIdentity(t *testing.T) {
	apiError := &types.APIError{Type: "provider_private", Message: "private-api-message"}
	metadata := map[string]any{"token": "private-token"}
	event := presentation.NewRuntimeErrorEvent(presentation.ToolEventContext{
		SessionID: "session-8", SessionEpoch: 5, ContextGeneration: 13,
		TurnID: "turn-8", ActorID: "actor-8", ActorType: "reviewer", WorkUnitID: "work-8",
	}, "tool-8", "private transport message", apiError, metadata)

	if event.Kind != types.RuntimeEventKindError || event.Outcome != types.ToolOutcomeFailed ||
		event.SessionID != "session-8" || event.Epoch != 5 || event.ContextGeneration != 13 ||
		event.TurnID != "turn-8" || event.ToolUseID != "tool-8" || event.WorkUnitID != "work-8" ||
		event.ActorID != "actor-8" || event.ActorType != "reviewer" || event.EventID == "" {
		t.Fatalf("runtime adapter identity/outcome = %#v", event)
	}
	if !errors.Is(event, apiError) {
		t.Fatal("API error identity was not preserved through private cause")
	}
	if event.PrivateMetadata["token"] != "private-token" || event.PrivateMetadata["runtime_message"] != "private transport message" {
		t.Fatalf("private metadata was not retained: %#v", event.PrivateMetadata)
	}
	metadata["token"] = "mutated"
	if event.PrivateMetadata["token"] != "private-token" {
		t.Fatal("adapter retained caller-owned metadata map")
	}
}

func TestDispatchRuntimeErrorEventRejectsStringOnlyRenderer(t *testing.T) {
	renderer := &stringRuntimeErrorCapture{}
	secret := "/Users/private/.ssh/id_ed25519?token=sk-runtime-secret"
	presentation.DispatchRuntimeErrorEvent(renderer, presentation.ToolEventContext{
		SessionID: "private-session", ProjectRoot: "/private/project", TurnID: "private-turn",
		ActorID: "private-actor", WorkUnitID: "private-work",
	}, "private-tool", secret, &types.APIError{Type: "private-provider-code", Message: secret}, map[string]any{"authorization": "Bearer private-token"})

	if renderer.message != "" {
		t.Fatalf("string-only renderer received runtime error: %q", renderer.message)
	}
}
