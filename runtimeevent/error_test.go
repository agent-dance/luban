package runtimeevent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestNewErrorEventRetainsPrivateEvidenceUntilAudienceProjection(t *testing.T) {
	secret := "/private/runtime/path token=sk-runtime-secret"
	apiError := &types.APIError{Type: "private_provider_error", Message: secret}
	metadata := map[string]any{"authorization": "Bearer private-token"}
	event := NewErrorEvent(types.RuntimeIdentity{
		SessionID: "session-sdk", TurnID: "turn-sdk", ToolUseID: "tool-sdk",
		ActorID: "actor-sdk", ActorType: "executor", WorkUnitID: "work-sdk",
	}, secret, apiError, metadata)

	if !errors.Is(event, apiError) || event.PrivateMetadata["authorization"] != "Bearer private-token" {
		t.Fatalf("private runtime evidence was not retained: %#v", event)
	}
	metadata["authorization"] = "mutated"
	if event.PrivateMetadata["authorization"] != "Bearer private-token" {
		t.Fatal("runtime event retained caller-owned metadata map")
	}
	projection, err := NewAudienceProjector().Project(event, ProjectionOptions{
		Audience: AudienceSDK, Redaction: RedactionStrict,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{secret, "sk-runtime-secret", "private_provider_error", "private-token", "authorization"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("SDK public projection leaked %q: %s", private, encoded)
		}
	}
}

func TestNewErrorEventProjectsOnlyAllowlistedContextFailureCode(t *testing.T) {
	const privateMessage = "private context diagnostic"
	event := NewErrorEvent(
		types.RuntimeIdentity{SessionID: "private-session"},
		privateMessage,
		&types.APIError{Type: "context_length_exceeded", Message: privateMessage, Status: 400},
		map[string]any{"private": privateMessage},
	)
	projection, err := NewAudienceProjector().Project(event, ProjectionOptions{
		Audience: AudienceUser, Redaction: RedactionStrict,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Code != "context_length_exceeded" {
		t.Fatalf("semantic context code = %q", projection.Code)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), privateMessage) || strings.Contains(string(encoded), "private-session") {
		t.Fatalf("context projection leaked private evidence: %s", encoded)
	}
}
