package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestParseWWWAuthenticatePreservesOAuthFields(t *testing.T) {
	response := &http.Response{Header: http.Header{
		"WWW-Authenticate": {`Bearer realm="mcp", as_uri="https://auth.example.test", error="insufficient_scope", error_description="Need repo", scope="repo", resource_metadata="https://mcp.example.test/prm"`},
	}}
	challenge := ParseWWWAuthenticate(response)
	if challenge == nil {
		t.Fatal("challenge nil")
	}
	if challenge.ASURI != "https://auth.example.test" || challenge.ErrorCode != "insufficient_scope" || challenge.Scope != "repo" || !strings.Contains(challenge.ErrorDescription, "Need") {
		t.Fatalf("challenge fields not parsed: %#v", challenge)
	}
}

func TestTokenSourceFuncResolvesBearerToken(t *testing.T) {
	source := TokenSourceFunc(func(_ context.Context, serverURL string) (string, error) {
		if serverURL != "https://mcp.example.test" {
			t.Fatalf("server URL = %q", serverURL)
		}
		return "token", nil
	})
	token, err := source.TokenFor(context.Background(), "https://mcp.example.test")
	if err != nil || token != "token" {
		t.Fatalf("TokenFor = %q, %v", token, err)
	}
}
