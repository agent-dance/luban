package tui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/types"
)

func TestRuntimeErrorPrivateDiagnosticIsNotInDefaultMessage(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session-public")
	state.SessionEpoch.Set(1)
	secret := "SECRET_RUNTIME_SENTINEL"
	ctx := ToolEventContext{SessionID: "session-public", TurnID: "private-turn", ActorID: "private-actor", WorkUnitID: "private-work"}
	if err := state.ApplyRuntimeError(ctx, "private-tool", secret, &types.APIError{Message: secret}, map[string]any{"secret": secret}); err != nil {
		t.Fatal(err)
	}
	messages := state.Messages.Get()
	if len(messages) == 0 {
		t.Fatal("runtime error did not create a public message")
	}
	public := messages[len(messages)-1]
	if strings.Contains(public.Text, secret) || len(public.DetailRefs) != 0 {
		t.Fatalf("default message exposed private diagnostics: %+v", public)
	}
	observations := state.Observations.Snapshot()
	if len(observations) == 0 || len(observations[len(observations)-1].ResultRefs) == 0 {
		t.Fatal("private diagnostic was not retained in the audit store")
	}
	data, err := state.ReadDetail(observations[len(observations)-1].ResultRefs[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(secret)) {
		t.Fatal("default diagnostic store retained raw private cause without explicit audit authority")
	}
}

func TestRuntimeEventAuditRetainsStableIdentityAndContextGeneration(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session-generation")
	state.SessionEpoch.Set(4)
	event := presentation.NewRuntimeErrorEvent(presentation.ToolEventContext{
		SessionID: "session-generation", SessionEpoch: 4, ContextGeneration: 12,
		TurnID: "turn-12", ActorID: "actor-12", ActorType: "reviewer", WorkUnitID: "work-12",
	}, "tool-12", "private-runtime-text", &types.APIError{Message: "private-api-text"}, map[string]any{"private": "metadata"})
	event.EventID = "event-stable-12"
	if err := state.ApplyRuntimeEvent(event); err != nil {
		t.Fatal(err)
	}
	observations := state.Observations.Snapshot()
	if len(observations) != 1 || observations[0].ID != "runtime-error:event-stable-12" || len(observations[0].ResultRefs) != 1 {
		t.Fatalf("stable runtime observation = %#v", observations)
	}
	payload, err := state.ReadDetail(observations[0].ResultRefs[0])
	if err != nil {
		t.Fatal(err)
	}
	var audit map[string]any
	if err := json.Unmarshal(payload, &audit); err != nil {
		t.Fatal(err)
	}
	if audit["schema_version"] != types.RuntimeEventSchemaVersion || audit["audience"] != "audit" ||
		audit["redaction_level"] != "diagnostic" || audit["event_id"] != "event-stable-12" ||
		audit["context_generation"] != float64(12) || audit["tool_use_id"] != "tool-12" {
		t.Fatalf("diagnostic audit identity/schema = %#v", audit)
	}
	for _, secret := range []string{"private-runtime-text", "private-api-text", "metadata"} {
		if bytes.Contains(payload, []byte(secret)) {
			t.Fatalf("diagnostic audit leaked %q: %s", secret, payload)
		}
	}
	public := state.Messages.Get()
	if len(public) != 1 {
		t.Fatalf("public messages = %#v", public)
	}
	encodedPublic, err := json.Marshal(public[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-runtime-text", "private-api-text", "metadata"} {
		if bytes.Contains(encodedPublic, []byte(secret)) {
			t.Fatalf("public projection leaked %q: %s", secret, encodedPublic)
		}
	}
}

func TestRuntimeErrorAuditRetainsSafeProviderRequestDiagnostic(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangZH)
	state.SessionID.Set("session-provider-error")
	state.SessionEpoch.Set(2)
	ctx := ToolEventContext{SessionID: "session-provider-error", TurnID: "turn-provider-error"}
	if err := state.ApplyRuntimeError(ctx, "", "private wrapper", &types.APIError{
		Type: "api_error", Message: "private provider body", Status: 400,
		Provider: "openai", APIFormat: "responses", Endpoint: "https://gateway.example/…/responses",
		RequestID: "req-safe-17", SuggestedAPIFormat: "chat-completions",
	}, nil); err != nil {
		t.Fatal(err)
	}
	observations := state.Observations.Snapshot()
	if len(observations) != 1 || len(observations[0].ResultRefs) != 1 {
		t.Fatalf("provider error observation = %#v", observations)
	}
	payload, err := state.ReadDetail(observations[0].ResultRefs[0])
	if err != nil {
		t.Fatal(err)
	}
	var audit struct {
		ProviderRequest struct {
			Provider   string `json:"provider"`
			APIFormat  string `json:"api_format"`
			Endpoint   string `json:"endpoint"`
			RequestID  string `json:"request_id"`
			Suggestion string `json:"suggestion"`
		} `json:"provider_request"`
	}
	if err := json.Unmarshal(payload, &audit); err != nil {
		t.Fatal(err)
	}
	if audit.ProviderRequest.Provider != "openai" || audit.ProviderRequest.APIFormat != "responses" ||
		audit.ProviderRequest.Endpoint != "https://gateway.example/…/responses" || audit.ProviderRequest.RequestID != "req-safe-17" ||
		!strings.Contains(audit.ProviderRequest.Suggestion, "--api chat-completions") {
		t.Fatalf("stored provider request diagnostic = %#v", audit.ProviderRequest)
	}
	messages := state.Messages.Get()
	if len(messages) != 1 || strings.Contains(messages[0].Text, "gateway.example") || strings.Contains(messages[0].Text, "req-safe-17") {
		t.Fatalf("strict transcript exposed diagnostic: %#v", messages)
	}
}
