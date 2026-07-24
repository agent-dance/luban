package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

type legacyRuntimeErrorCapture struct {
	NoOpRenderer
	message string
}

type legacyRuntimeWarningCapture struct {
	NoOpRenderer
	message string
}

func (r *legacyRuntimeWarningCapture) Warning(message string) { r.message = message }

func TestDispatchRuntimeWarningEventFailsClosedForLegacyRenderer(t *testing.T) {
	renderer := &legacyRuntimeWarningCapture{}
	secret := "/Users/private/.ssh/id_ed25519?token=sk-warning-secret\x1b[2J"
	warning := runtimeevent.NewWarningEvent(
		types.RuntimeIdentity{SessionID: "private-session", TurnID: "private-turn"},
		i18n.KeyRuntimeAutoCompactFailed,
		nil,
		errors.New(secret),
		map[string]any{"authorization": "Bearer private-token", "project_root": "/private/project"},
	)
	DispatchRuntimeWarningEvent(renderer, warning, i18n.LangEN, true)
	if renderer.message != i18n.Text(i18n.LangEN, i18n.KeyRuntimeAutoCompactFailed) {
		t.Fatalf("legacy runtime-warning message = %q", renderer.message)
	}
	for _, private := range []string{secret, "sk-warning-secret", "private-token", "/private/project", "private-session", "private-turn", "\x1b[2J"} {
		if strings.Contains(renderer.message, private) {
			t.Fatalf("legacy runtime-warning projection leaked %q: %q", private, renderer.message)
		}
	}
}

func (r *legacyRuntimeErrorCapture) Error(message string) { r.message = message }

func TestRuntimeErrorAdapterPreservesPrivateCauseAndStableIdentity(t *testing.T) {
	apiError := &types.APIError{Type: "provider_private", Message: "private-api-message"}
	metadata := map[string]any{"token": "private-token"}
	event := NewRuntimeErrorEvent(ToolEventContext{
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

func TestDispatchRuntimeErrorEventFailsClosedForLegacyRenderer(t *testing.T) {
	renderer := &legacyRuntimeErrorCapture{}
	secret := "/Users/private/.ssh/id_ed25519?token=sk-runtime-secret"
	DispatchRuntimeErrorEvent(renderer, ToolEventContext{
		SessionID: "private-session", ProjectRoot: "/private/project", TurnID: "private-turn",
		ActorID: "private-actor", WorkUnitID: "private-work",
	}, "private-tool", secret, &types.APIError{Type: "private-provider-code", Message: secret}, map[string]any{"authorization": "Bearer private-token"})

	want := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary)
	if renderer.message != want {
		t.Fatalf("legacy runtime-error message = %q, want strict projection %q", renderer.message, want)
	}
	for _, private := range []string{secret, "sk-runtime-secret", "private-provider-code", "private-token", "private-session", "private-tool", "private-project"} {
		if strings.Contains(renderer.message, private) {
			t.Fatalf("legacy runtime-error projection leaked %q: %q", private, renderer.message)
		}
	}
}
