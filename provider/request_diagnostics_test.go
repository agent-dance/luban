package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestProviderRequestDiagnosticRedactsEndpointAndCapturesRequestID(t *testing.T) {
	headers := make(http.Header)
	headers.Set("X-Request-ID", "req-safe-17")
	apiErr := annotateProviderRequestError(
		&types.APIError{Status: http.StatusBadRequest, Type: "api_error", Message: "private body"},
		"openai", "responses", "https://user:secret@gateway.example/private-token/v1/responses?api_key=secret#private", headers,
	)
	if apiErr.Provider != "openai" || apiErr.APIFormat != "responses" || apiErr.RequestID != "req-safe-17" {
		t.Fatalf("request diagnostic identity = %+v", apiErr)
	}
	if apiErr.Endpoint != "https://gateway.example/…/responses" {
		t.Fatalf("redacted endpoint = %q", apiErr.Endpoint)
	}
	for _, secret := range []string{"user", "secret", "private-token", "api_key"} {
		if strings.Contains(apiErr.Endpoint, secret) {
			t.Fatalf("redacted endpoint leaked %q: %s", secret, apiErr.Endpoint)
		}
	}
}

func TestProviderRequestIDSupportsCommonGatewayHeaders(t *testing.T) {
	for _, name := range []string{"X-Request-ID", "OpenAI-Request-ID", "Request-ID", "X-Amzn-RequestId", "CF-Ray"} {
		headers := make(http.Header)
		headers.Set(name, "req-42")
		if got := providerRequestID(headers); got != "req-42" {
			t.Fatalf("providerRequestID(%s) = %q", name, got)
		}
	}
}

func TestResponsesRequestDiagnosticsClassifyFormatSuggestionByStatus(t *testing.T) {
	for _, test := range []struct {
		status         int
		wantSuggestion bool
	}{
		{status: http.StatusBadRequest, wantSuggestion: false},
		{status: http.StatusUnauthorized, wantSuggestion: false},
		{status: http.StatusNotFound, wantSuggestion: true},
		{status: http.StatusTooManyRequests, wantSuggestion: false},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Request-ID", "req-status-42")
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(`{"error":{"type":"api_error","message":"private gateway body"}}`))
			}))
			defer server.Close()

			responses := NewResponses(Config{
				ProviderName: "custom-gateway", APIKey: "key",
				BaseURL: server.URL + "/private-token/v1", ResponsesSemantics: ResponsesSemanticsCompatible,
			})
			_, err := responses.CreateStream(context.Background(), Params{Messages: []types.Message{types.UserMessage("hello")}})
			apiErr, ok := AsAPIError(err)
			if !ok {
				t.Fatalf("CreateStream error = %v, want APIError", err)
			}
			if apiErr.Provider != "custom-gateway" || apiErr.APIFormat != "responses" ||
				apiErr.Endpoint != server.URL+"/…/responses" || apiErr.RequestID != "req-status-42" {
				t.Fatalf("request diagnostics = %+v", apiErr)
			}
			if got := apiErr.SuggestedAPIFormat == "chat-completions"; got != test.wantSuggestion {
				t.Fatalf("suggestion for status %d = %q", test.status, apiErr.SuggestedAPIFormat)
			}
		})
	}
}

func TestChatRequestDiagnosticsCaptureRedactedEndpointAndRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("OpenAI-Request-ID", "req-chat-17")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":{"type":"invalid_api_key","message":"private auth body"}}`))
	}))
	defer server.Close()

	chat := NewOpenAI(Config{
		ProviderName: "custom-gateway", APIKey: "key", BaseURL: server.URL + "/private-token/v1",
	})
	_, err := chat.CreateStream(context.Background(), Params{Messages: []types.Message{types.UserMessage("hello")}})
	apiErr, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("CreateStream error = %v, want APIError", err)
	}
	if apiErr.Provider != "custom-gateway" || apiErr.APIFormat != "chat-completions" ||
		apiErr.Endpoint != server.URL+"/…/chat/completions" || apiErr.RequestID != "req-chat-17" {
		t.Fatalf("Chat request diagnostics = %+v", apiErr)
	}
}
