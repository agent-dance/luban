package runtimeevent

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
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
		types.RuntimeIdentity{SessionID: "session-safe"},
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

func TestProviderRequestDiagnosticIsStructuredLocalizedAndRedacted(t *testing.T) {
	event := NewErrorEvent(
		types.RuntimeIdentity{SessionID: "private-session"},
		"private wrapper",
		&types.APIError{
			Type: "api_error", Message: "private provider body", Status: 400,
			Provider: "openai", APIFormat: "responses",
			Endpoint: "https://gateway.example/…/responses", RequestID: "req-safe-17",
			SuggestedAPIFormat: "chat-completions",
		},
		nil,
	)
	projection, err := NewAudienceProjector().Project(event, ProjectionOptions{
		Audience: AudienceAudit, Redaction: RedactionDiagnostic,
		Language: i18n.LangZH, LanguageSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := projection.ProviderRequest
	if diagnostic == nil || diagnostic.Provider != "openai" || diagnostic.APIFormat != "responses" ||
		diagnostic.Endpoint != "https://gateway.example/…/responses" || diagnostic.RequestID != "req-safe-17" ||
		!strings.Contains(diagnostic.Suggestion, "--api chat-completions") {
		t.Fatalf("provider diagnostic = %#v", diagnostic)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private wrapper", "private provider body"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("diagnostic projection leaked %q: %s", secret, encoded)
		}
	}

	public, err := NewAudienceProjector().Project(event, ProjectionOptions{
		Audience: AudienceUser, Redaction: RedactionStrict,
		Language: i18n.LangZH, LanguageSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if public.ProviderRequest != nil {
		t.Fatalf("strict public projection exposed provider diagnostic: %#v", public.ProviderRequest)
	}
}

func TestProviderRequestDiagnosticRejectsUnsanitizedExternalFields(t *testing.T) {
	event := NewErrorEvent(types.RuntimeIdentity{}, "private", &types.APIError{
		Message: "private", Provider: "openai\nsecret", APIFormat: "unknown",
		Endpoint:  "https://user:secret@gateway.example/v1/responses?token=secret",
		RequestID: "req\nsecret",
	}, nil)
	projection, err := NewAudienceProjector().Project(event, ProjectionOptions{
		Audience: AudienceAudit, Redaction: RedactionDiagnostic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.ProviderRequest != nil {
		t.Fatalf("unsanitized fields entered diagnostic projection: %#v", projection.ProviderRequest)
	}
}

func TestProviderRequestDiagnosticAuthorizationMatrix(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusNotFound,
		http.StatusTooManyRequests,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			apiError := &types.APIError{
				Type: "api_error", Message: "PRIVATE-PROVIDER-BODY", Status: status,
				Provider: "openai", APIFormat: "responses",
				Endpoint: "https://gateway.example/…/responses", RequestID: "req-safe-matrix-17",
			}
			if status == http.StatusBadRequest || status == http.StatusNotFound {
				apiError.SuggestedAPIFormat = "chat-completions"
			}
			event := NewErrorEvent(types.RuntimeIdentity{}, "PRIVATE-WRAPPER", apiError, nil)

			audit, err := NewAudienceProjector().Project(event, ProjectionOptions{
				Audience: AudienceAudit, Redaction: RedactionDiagnostic,
				Language: i18n.LangZH, LanguageSet: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			diagnostic := audit.ProviderRequest
			if diagnostic == nil || diagnostic.Provider != "openai" || diagnostic.APIFormat != "responses" ||
				diagnostic.Endpoint != "https://gateway.example/…/responses" || diagnostic.RequestID != "req-safe-matrix-17" {
				t.Fatalf("authorized diagnostic = %#v", diagnostic)
			}
			wantSuggestion := status == http.StatusBadRequest || status == http.StatusNotFound
			if got := strings.Contains(diagnostic.Suggestion, "--api chat-completions"); got != wantSuggestion {
				t.Fatalf("format suggestion for status %d = %q", status, diagnostic.Suggestion)
			}

			public, err := NewAudienceProjector().Project(event, ProjectionOptions{
				Audience: AudienceUser, Redaction: RedactionStrict,
				Language: i18n.LangZH, LanguageSet: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(public)
			if err != nil {
				t.Fatal(err)
			}
			if public.ProviderRequest != nil {
				t.Fatalf("strict projection exposed provider request: %#v", public.ProviderRequest)
			}
			for _, private := range []string{
				"PRIVATE-PROVIDER-BODY", "PRIVATE-WRAPPER", "gateway.example", "req-safe-matrix-17", "chat-completions",
			} {
				if strings.Contains(string(encoded), private) {
					t.Fatalf("strict projection leaked %q: %s", private, encoded)
				}
			}
		})
	}
}

func TestProviderRequestDiagnosticReportsExhaustedFormatsOnlyToAuthorizedAudience(t *testing.T) {
	event := NewErrorEvent(types.RuntimeIdentity{}, "PRIVATE-WRAPPER", &types.APIError{
		Type: "api_error", Message: "PRIVATE-PROVIDER-BODY", Status: http.StatusBadRequest,
		Provider: "openai", APIFormat: "chat-completions",
		Endpoint: "https://gateway.example/…/chat/completions", RequestID: "req-format-exhausted-17",
		AttemptedAPIFormats: []string{"responses", "chat-completions"},
	}, nil)
	audit, err := NewAudienceProjector().Project(event, ProjectionOptions{
		Audience: AudienceAudit, Redaction: RedactionDiagnostic,
		Language: i18n.LangZH, LanguageSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if audit.ProviderRequest == nil || len(audit.ProviderRequest.AttemptedAPIFormats) != 2 ||
		!strings.Contains(audit.ProviderRequest.Suggestion, "base URL") {
		t.Fatalf("format exhaustion diagnostic = %#v", audit.ProviderRequest)
	}
	public, err := NewAudienceProjector().Project(event, ProjectionOptions{
		Audience: AudienceUser, Redaction: RedactionStrict,
	})
	if err != nil {
		t.Fatal(err)
	}
	if public.ProviderRequest != nil || strings.Contains(public.Message, "responses") || strings.Contains(public.Message, "base URL") {
		t.Fatalf("strict projection exposed format diagnostics: %#v", public)
	}
}
