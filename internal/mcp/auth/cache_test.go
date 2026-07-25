package auth

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

type testHTTPAuthError struct {
	status    int
	challenge *AuthChallenge
}

func (e *testHTTPAuthError) Error() string { return "remote auth failure" }
func (e *testHTTPAuthError) AuthStatusCode() int {
	return e.status
}
func (e *testHTTPAuthError) AuthChallenge() *AuthChallenge {
	return e.challenge
}

func TestRecordNeedsAuthFromErrorClassifiesUnauthorized(t *testing.T) {
	descriptor := ServerDescriptor{Transport: "http", URL: "https://mcp.example.test", OAuthCapable: true}
	cache := NewNeedsAuthCache(time.Minute)
	challenge := &AuthChallenge{Scheme: "Bearer", ResourceMetadataURL: "https://mcp.example.test/.well-known/oauth-protected-resource"}

	state, ok := RecordNeedsAuthFromError(cache, "repo", descriptor, &UnauthorizedError{
		StatusCode: http.StatusUnauthorized,
		ServerURL:  descriptor.URL,
		Challenge:  challenge,
	})
	if !ok {
		t.Fatal("auth failure was not classified")
	}
	if state.Reason != "unauthorized" || state.Challenge != challenge || state.Transport != "http" {
		t.Fatalf("unexpected needs-auth state: %#v", state)
	}
	if cached, found := LookupNeedsAuth(cache, "repo", descriptor); !found || cached.StatusCode != http.StatusUnauthorized {
		t.Fatalf("cached state = %#v, found=%v", cached, found)
	}
}

func TestRecordNeedsAuthFromErrorClassifiesInsufficientScope(t *testing.T) {
	descriptor := ServerDescriptor{Transport: "sse", URL: "https://mcp.example.test/sse", OAuthCapable: true}
	challenge := &AuthChallenge{
		ErrorCode:           "insufficient_scope",
		Scope:               "repo read",
		ResourceMetadataURL: "https://mcp.example.test/.well-known/oauth-protected-resource",
	}
	err := &testHTTPAuthError{status: http.StatusForbidden, challenge: challenge}

	state, ok := RecordNeedsAuthFromError(NewNeedsAuthCache(time.Minute), "repo", descriptor, err)
	if !ok || state.Reason != "insufficient_scope" || state.Scope != "repo read" {
		t.Fatalf("unexpected needs-auth state: %#v, classified=%v", state, ok)
	}
	if !IsRequiredError(err) {
		t.Fatal("insufficient-scope error was not recognized as auth-required")
	}
}

func TestRecordNeedsAuthFromErrorClassifiesHTTPUnauthorized(t *testing.T) {
	descriptor := ServerDescriptor{Transport: "http", URL: "https://mcp.example.test", OAuthCapable: true}
	err := &testHTTPAuthError{status: http.StatusUnauthorized, challenge: &AuthChallenge{Scheme: "Bearer"}}
	state, ok := RecordNeedsAuthFromError(NewNeedsAuthCache(time.Minute), "repo", descriptor, err)
	if !ok || state.Reason != "unauthorized" || state.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected needs-auth state: %#v, classified=%v", state, ok)
	}
}

func TestNeedsAuthCacheExpiresEntries(t *testing.T) {
	cache := NewNeedsAuthCache(time.Minute)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	cache.Put("repo|key", NeedsAuthState{Reason: "unauthorized"})
	now = now.Add(time.Minute)
	if _, ok := cache.Get("repo|key"); ok {
		t.Fatal("expired cache entry remained visible")
	}
}

func TestIsRequiredErrorRejectsUnrelatedFailures(t *testing.T) {
	if IsRequiredError(errors.New("network unavailable")) {
		t.Fatal("unrelated failure was classified as auth-required")
	}
	forbidden := &testHTTPAuthError{status: http.StatusForbidden, challenge: &AuthChallenge{ErrorCode: "access_denied"}}
	if IsRequiredError(forbidden) {
		t.Fatal("non-scope forbidden response was classified as auth-required")
	}
}
