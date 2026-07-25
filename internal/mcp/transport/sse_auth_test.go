package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
)

func TestSSEUnauthorizedReturnsParsedChallenge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="mcp", as_uri="https://auth.example.test/oauth"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	transport, err := NewSSETransport(SSEConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}
	_, err = NewClient(context.Background(), transport)
	var unauthorized *mcpauth.UnauthorizedError
	if !errors.As(err, &unauthorized) {
		t.Fatalf("NewClient error = %T %[1]v, want UnauthorizedError", err)
	}
	if unauthorized.Challenge == nil || unauthorized.Challenge.Realm != "mcp" || unauthorized.Challenge.ASURI != "https://auth.example.test/oauth" {
		t.Fatalf("unauthorized challenge was not parsed: %#v", unauthorized.Challenge)
	}
}
