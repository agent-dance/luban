package remote

// Contract regression tests for Trigger request policy and output.
//
// The suite pins the HTTP-text response format, feature and policy gates,
// single-request 401 behavior, beta header, and all supported actions.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ─── Contract 1: Output is HTTP-text, not a JSON envelope ────────────────

// TestAlignmentRemoteTriggerOutputIsHTTPText asserts that the result content
// has the canonical shape `HTTP <status>\n<json>`.
func TestAlignmentRemoteTriggerOutputIsHTTPText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tool := newAlignmentRemoteTriggerTool(srv.URL, srv.Client())

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "list",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	// The status line is separated from the raw JSON body by a literal newline.
	if !strings.HasPrefix(result.Content, "HTTP ") {
		t.Errorf("RemoteTrigger output not HTTP-text — audit P3-2 (remote_trigger.go:100-103):\n  content=%q", result.Content)
	}
	if !strings.Contains(result.Content, "\n") {
		t.Errorf("RemoteTrigger output missing newline separator between status and body — audit P3-2:\n  content=%q", result.Content)
	}
}

// TestAlignmentRemoteTriggerOutputContainsStatusAndBody asserts the canonical
// text format embeds both the numeric status AND the raw JSON body.
func TestAlignmentRemoteTriggerOutputContainsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer srv.Close()

	tool := newAlignmentRemoteTriggerTool(srv.URL, srv.Client())

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "create",
		"body":   map[string]any{},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Canonical form: "HTTP 201\n{\"id\":\"x\"}".
	if !strings.Contains(result.Content, "HTTP 201") {
		t.Errorf("expected literal `HTTP 201` in output — audit P3-2:\n  content=%q", result.Content)
	}
	if !strings.Contains(result.Content, `{"id":"x"}`) {
		t.Errorf("expected raw JSON body `{\"id\":\"x\"}` in output — audit P3-2:\n  content=%q", result.Content)
	}
	// And it MUST NOT be wrapped in a JSON envelope object.
	if strings.HasPrefix(strings.TrimSpace(result.Content), "{") &&
		strings.Contains(result.Content, `"status"`) &&
		strings.Contains(result.Content, `"json"`) {
		t.Errorf("output is JSON envelope, want HTTP-text — audit P3-2:\n  content=%q", result.Content)
	}
}

// ─── Contract 2: HTTP failures surface without post-response retry ───────

// TestAlignmentRemoteTrigger401SurfacesWithoutRetry matches TS: OAuth refresh
// happens before the request; a 401 response from the trigger API is returned
// as HTTP-text without a second upstream call.
func TestAlignmentRemoteTrigger401SurfacesWithoutRetry(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	var resolverCount int
	tool := &Trigger{
		HTTPClient: srv.Client(),
		AccessTokenResolver: func(context.Context) (string, error) {
			resolverCount++
			return "token", nil
		},
		OrganizationUUIDResolver: func(context.Context, string, string) (string, error) {
			return "org-1", nil
		},
		BaseURLResolver: func() (string, error) {
			return srv.URL, nil
		},
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "list",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if resolverCount != 1 {
		t.Errorf("AccessTokenResolver called %d times, want 1 pre-call resolution:\n  content=%q", resolverCount, result.Content)
	}
	if calls != 1 {
		t.Errorf("server saw %d calls, want exactly one trigger request", calls)
	}
	if !strings.Contains(result.Content, "HTTP 401") || !strings.Contains(result.Content, `{"error":"unauthorized"}`) {
		t.Errorf("401 response not surfaced as HTTP-text:\n  content=%q", result.Content)
	}
}

// ─── Contract 5: Beta header under the HTTP-text contract ────────────────

// TestAlignmentRemoteTriggerBetaHeaderUnderHTTPTextContract is a paired test:
// the request MUST send anthropic-beta=ccr-triggers-2026-01-30 AND the
// response surface MUST be HTTP-text.
func TestAlignmentRemoteTriggerBetaHeaderUnderHTTPTextContract(t *testing.T) {
	var gotBeta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBeta = r.Header.Get("anthropic-beta")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	tool := newAlignmentRemoteTriggerTool(srv.URL, srv.Client())

	result, err := tool.Execute(context.Background(), map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotBeta != remoteTriggerBetaHeader {
		t.Errorf("anthropic-beta = %q, want %q — audit P3-2", gotBeta, remoteTriggerBetaHeader)
	}
	// Canonical output contract:
	if !strings.HasPrefix(result.Content, "HTTP ") {
		t.Errorf("output not HTTP-text under canonical contract — audit P3-2:\n  content=%q", result.Content)
	}
}

// ─── Contract 6: All action branches honor HTTP-text output ──────────────

// TestAlignmentRemoteTriggerListActionHTTPText asserts list returns
// HTTP-text format.
func TestAlignmentRemoteTriggerListActionHTTPText(t *testing.T) {
	assertActionUsesHTTPText(t, "list", nil, http.StatusOK, `[]`)
}

// TestAlignmentRemoteTriggerGetActionHTTPText asserts get returns
// HTTP-text format and includes the trigger id in the URL.
func TestAlignmentRemoteTriggerGetActionHTTPText(t *testing.T) {
	assertActionUsesHTTPText(t,
		"get",
		map[string]any{"trigger_id": "abc-123"},
		http.StatusOK,
		`{"id":"abc-123"}`)
}

// TestAlignmentRemoteTriggerCreateActionHTTPText asserts create returns
// HTTP-text format on success.
func TestAlignmentRemoteTriggerCreateActionHTTPText(t *testing.T) {
	assertActionUsesHTTPText(t,
		"create",
		map[string]any{"body": map[string]any{"name": "x"}},
		http.StatusCreated,
		`{"id":"new"}`)
}

// TestAlignmentRemoteTriggerUpdateActionHTTPText asserts update returns
// HTTP-text format.
func TestAlignmentRemoteTriggerUpdateActionHTTPText(t *testing.T) {
	assertActionUsesHTTPText(t,
		"update",
		map[string]any{"trigger_id": "u-1", "body": map[string]any{"name": "y"}},
		http.StatusOK,
		`{"updated":true}`)
}

// TestAlignmentRemoteTriggerRunActionHTTPText asserts run returns
// HTTP-text format.
func TestAlignmentRemoteTriggerRunActionHTTPText(t *testing.T) {
	assertActionUsesHTTPText(t,
		"run",
		map[string]any{"trigger_id": "r-1"},
		http.StatusAccepted,
		`{"queued":true}`)
}

// ─── Contract 7: Error responses are also HTTP-text ──────────────────────

// TestAlignmentRemoteTriggerErrorBodyAlsoHTTPText asserts that even non-2xx
// responses are surfaced as HTTP-text.
func TestAlignmentRemoteTriggerErrorBodyAlsoHTTPText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	tool := newAlignmentRemoteTriggerTool(srv.URL, srv.Client())
	result, err := tool.Execute(context.Background(), map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// "HTTP 400\n{...}" preserves the upstream status verbatim.
	if !strings.Contains(result.Content, "HTTP 400") {
		t.Errorf("error body not surfaced as HTTP-text — audit P3-2:\n  content=%q", result.Content)
	}
	if !strings.Contains(result.Content, `{"error":"bad request"}`) {
		t.Errorf("error body lost — audit P3-2:\n  content=%q", result.Content)
	}
}

// ─── Internal helpers (do not depend on production code state) ────────────

// newAlignmentRemoteTriggerTool wires a fresh tool with the test server's
// transport and stub resolvers, mirroring the existing remote_trigger_test
// setup so the tests stay focused on output, policy, and feature gates.
func newAlignmentRemoteTriggerTool(baseURL string, client *http.Client) *Trigger {
	return &Trigger{
		HTTPClient: client,
		AccessTokenResolver: func(context.Context) (string, error) {
			return "test-token", nil
		},
		OrganizationUUIDResolver: func(context.Context, string, string) (string, error) {
			return "test-org", nil
		},
		BaseURLResolver: func() (string, error) {
			return baseURL, nil
		},
	}
}

// assertActionUsesHTTPText is a shared helper for the per-action tests. It
// runs the action through a fresh tool against a stub server and asserts the
// canonical HTTP-text contract.
func assertActionUsesHTTPText(t *testing.T, action string, extra map[string]any, status int, body string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	tool := newAlignmentRemoteTriggerTool(srv.URL, srv.Client())

	args := map[string]any{"action": action}
	for k, v := range extra {
		args[k] = v
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute(%s): %v", action, err)
	}

	// Canonical expectation: HTTP-text envelope.
	if !strings.HasPrefix(result.Content, "HTTP ") {
		t.Errorf("action=%s output not HTTP-text — audit P3-2:\n  content=%q", action, result.Content)
	}
	if !strings.Contains(result.Content, body) {
		t.Errorf("action=%s output missing body %q — audit P3-2:\n  content=%q", action, body, result.Content)
	}
	// MUST NOT be a JSON envelope.
	trimmed := strings.TrimSpace(result.Content)
	if strings.HasPrefix(trimmed, "{") &&
		strings.Contains(trimmed, `"status"`) &&
		strings.Contains(trimmed, `"json"`) {
		t.Errorf("action=%s still emits JSON envelope — audit P3-2:\n  content=%q", action, result.Content)
	}
}
