package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPParityContractTSServiceMarshalHelpersPreserveStructuredEnvelopes(t *testing.T) {
	toolCall, err := MarshalToolCall(&ToolCall{
		Content: []ToolContent{{
			Type:     "text",
			Text:     "hello",
			MimeType: "text/plain",
			URI:      "memo://tool-output",
		}},
		IsError: false,
	})
	if err != nil {
		t.Fatalf("MarshalToolCall: %v", err)
	}
	var parsedToolCall map[string]any
	if err := json.Unmarshal([]byte(toolCall), &parsedToolCall); err != nil {
		t.Fatalf("tool call envelope is not JSON: %v", err)
	}
	content := parsedToolCall["content"].([]any)[0].(map[string]any)
	if content["uri"] != "memo://tool-output" || content["mimeType"] != "text/plain" {
		t.Fatalf("tool call envelope dropped uri/mimeType: %#v", parsedToolCall)
	}

	resources, err := MarshalResources([]Resource{{
		URI:         "memo://alpha",
		Name:        "Alpha",
		Description: "A memo",
		MimeType:    "text/markdown",
	}})
	if err != nil {
		t.Fatalf("MarshalResources: %v", err)
	}
	var parsedResources map[string][]Resource
	if err := json.Unmarshal([]byte(resources), &parsedResources); err != nil {
		t.Fatalf("resources envelope is not JSON: %v", err)
	}
	if got := parsedResources["resources"][0]; got.URI != "memo://alpha" || got.MimeType != "text/markdown" || got.Description != "A memo" {
		t.Fatalf("resource envelope dropped fields: %#v", got)
	}

	contents, err := MarshalContents([]ResourceContent{{
		URI:      "memo://alpha",
		MimeType: "text/markdown",
		Text:     "# Alpha",
	}})
	if err != nil {
		t.Fatalf("MarshalContents: %v", err)
	}
	var parsedContents map[string][]ResourceContent
	if err := json.Unmarshal([]byte(contents), &parsedContents); err != nil {
		t.Fatalf("contents envelope is not JSON: %v", err)
	}
	if got := parsedContents["contents"][0]; got.URI != "memo://alpha" || got.MimeType != "text/markdown" || got.Text != "# Alpha" {
		t.Fatalf("contents envelope dropped fields: %#v", got)
	}
}

func TestMCPParityContractTSPKCEPairUsesS256(t *testing.T) {
	pair, err := NewPKCEPair()
	if err != nil {
		t.Fatalf("NewPKCEPair: %v", err)
	}
	if pair.Method != PKCEChallengeMethodS256 {
		t.Fatalf("PKCE method = %q, want %q", pair.Method, PKCEChallengeMethodS256)
	}
	if len(pair.Verifier) < 43 {
		t.Fatalf("PKCE verifier too short: %d", len(pair.Verifier))
	}
	sum := sha256.Sum256([]byte(pair.Verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if pair.Challenge != wantChallenge {
		t.Fatalf("PKCE challenge does not match S256 verifier hash")
	}
	if strings.Contains(pair.Verifier, "=") || strings.Contains(pair.Challenge, "=") {
		t.Fatalf("PKCE verifier/challenge must be unpadded base64url")
	}
}

func TestMCPParityContractTSSSEUnauthorizedReturnsParsedChallenge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="mcp", as_uri="https://auth.example.test/oauth"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	transport, err := NewSSETransport(SSEConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}
	var out map[string]any
	err = transport.CallRaw(context.Background(), "tools/list", map[string]any{}, &out)
	var unauthorized *UnauthorizedError
	if !errors.As(err, &unauthorized) {
		t.Fatalf("CallRaw error = %T %[1]v, want UnauthorizedError", err)
	}
	if unauthorized.Challenge == nil || unauthorized.Challenge.Realm != "mcp" || unauthorized.Challenge.ASURI != "https://auth.example.test/oauth" {
		t.Fatalf("unauthorized challenge was not parsed: %#v", unauthorized.Challenge)
	}
}
