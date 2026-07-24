package tools

// remote_trigger_alignment_test.go — RED tests targeting alignment_audit.md
// gaps for RemoteTriggerTool (audit P3-2).
//
// Audit reference (P3-2):
//   - Output is JSON `{status, json}` (remote_trigger.go:100-103) but should
//     be HTTP-text `HTTP <status>\n<json>` to match TS.
//   - No `tengu_surreal_dali` feature flag guard (RemoteTriggerTool struct
//     has no FeatureFlagResolver field — see misc.go:100-105).
//   - No `allow_remote_sessions` policy guard (RemoteTriggerTool struct has
//     no PolicyResolver field).
//   - OAuth 401 refresh path not separately surfaced.
//   - Beta header constant exists but no test covers all 5 action branches
//     against the post-fix HTTP-text contract.
//
// All tests below COMPILE but ASSERT THE EXPECTED (post-fix) behaviour, so
// they MUST FAIL on the current code base.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// ─── Gap 1: Output must be HTTP-text, not JSON ────────────────────────────

// TestAlignmentRemoteTriggerOutputIsHTTPText asserts that the result content
// has the canonical TS shape `HTTP <status>\n<json>`. Today
// remote_trigger.go:100-103 emits `{"status":NN,"json":"..."}` JSON.
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

	// Post-fix expectation: result starts with "HTTP " and contains a literal
	// newline before the JSON body. Today the result is a JSON object.
	if !strings.HasPrefix(result.Content, "HTTP ") {
		t.Errorf("RemoteTrigger output not HTTP-text — audit P3-2 (remote_trigger.go:100-103):\n  content=%q", result.Content)
	}
	if !strings.Contains(result.Content, "\n") {
		t.Errorf("RemoteTrigger output missing newline separator between status and body — audit P3-2:\n  content=%q", result.Content)
	}
}

// TestAlignmentRemoteTriggerOutputContainsStatusAndBody asserts the post-fix
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

	// Post-fix: "HTTP 201\n{\"id\":\"x\"}". Today it's JSON envelope.
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

// ─── Gap 2: tengu_surreal_dali feature flag guard ─────────────────────────

// TestAlignmentRemoteTriggerFeatureFlagGate asserts that the tool consults a
// FeatureFlagResolver before issuing any HTTP traffic. Today the struct has
// no such field (misc.go:100-105) so the call always proceeds.
func TestAlignmentRemoteTriggerFeatureFlagGate(t *testing.T) {
	var contacted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	t.Setenv("CLAUDE_CODE_DISABLE_REMOTE_TRIGGER", "1")
	tool := newAlignmentRemoteTriggerTool(srv.URL, srv.Client())

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "list",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Post-fix: with the kill switch set, the tool refuses BEFORE any HTTP
	// traffic. Today the env var is unrecognised and we hit the server.
	if contacted {
		t.Errorf("RemoteTrigger ignored CLAUDE_CODE_DISABLE_REMOTE_TRIGGER feature flag — audit P3-2 (tengu_surreal_dali)")
	}
	if !result.IsError {
		t.Errorf("expected feature-flag refusal IsError, got success: %s — audit P3-2", result.Content)
	}
	low := strings.ToLower(result.Content)
	if !strings.Contains(low, "disabled") &&
		!strings.Contains(low, "feature") &&
		!strings.Contains(low, "tengu_surreal_dali") {
		t.Errorf("refusal message missing feature-flag marker — audit P3-2:\n  content=%q", result.Content)
	}
}

// TestAlignmentRemoteTriggerHasFeatureFlagResolverField asserts that the
// struct exposes a FeatureFlagResolver hook. Today no such field exists.
func TestAlignmentRemoteTriggerHasFeatureFlagResolverField(t *testing.T) {
	if !remoteTriggerExposesFeatureFlagResolver() {
		t.Errorf("RemoteTriggerTool struct has no FeatureFlagResolver field — audit P3-2 (misc.go:100-105)")
	}
}

// remoteTriggerExposesFeatureFlagResolver is a probe that returns true once
// the struct gains the resolver field. Today: false.
func remoteTriggerExposesFeatureFlagResolver() bool {
	t := reflect.TypeOf(RemoteTriggerTool{})
	_, ok := t.FieldByName("FeatureFlagResolver")
	return ok
}

// ─── Gap 3: allow_remote_sessions policy guard ────────────────────────────

// TestAlignmentRemoteTriggerPolicyGate asserts that the tool consults an
// allow_remote_sessions policy before firing. Today nothing of the sort
// is wired in.
func TestAlignmentRemoteTriggerPolicyGate(t *testing.T) {
	var contacted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	// Forbid remote sessions via env (post-fix policy resolver should read this).
	t.Setenv("CLAUDE_CODE_ALLOW_REMOTE_SESSIONS", "0")
	tool := newAlignmentRemoteTriggerTool(srv.URL, srv.Client())

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "list",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if contacted {
		t.Errorf("RemoteTrigger ignored allow_remote_sessions=false — audit P3-2 policy guard missing")
	}
	if !result.IsError {
		t.Errorf("expected policy-refusal IsError, got success: %s", result.Content)
	}
	low := strings.ToLower(result.Content)
	if !strings.Contains(low, "policy") &&
		!strings.Contains(low, "remote_sessions") &&
		!strings.Contains(low, "allow_remote") {
		t.Errorf("refusal message missing policy marker — audit P3-2:\n  content=%q", result.Content)
	}
}

// TestAlignmentRemoteTriggerHasPolicyResolverField asserts the struct
// exposes a PolicyResolver. Today: missing.
func TestAlignmentRemoteTriggerHasPolicyResolverField(t *testing.T) {
	if !remoteTriggerExposesPolicyResolver() {
		t.Errorf("RemoteTriggerTool struct has no PolicyResolver field — audit P3-2 (allow_remote_sessions)")
	}
}

// remoteTriggerExposesPolicyResolver returns true once the struct gains the
// resolver field. Today: false.
func remoteTriggerExposesPolicyResolver() bool {
	t := reflect.TypeOf(RemoteTriggerTool{})
	_, ok := t.FieldByName("PolicyResolver")
	return ok
}

// ─── Gap 4: HTTP failures are surfaced without post-response retry ────────

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
	tool := &RemoteTriggerTool{
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

// ─── Gap 5: Beta header validated under HTTP-text contract ────────────────

// TestAlignmentRemoteTriggerBetaHeaderUnderHTTPTextContract is a paired test:
// the request MUST send anthropic-beta=ccr-triggers-2026-01-30 AND the
// response surface MUST be HTTP-text. The current code passes the first half
// but fails the second.
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
	// Post-fix output contract:
	if !strings.HasPrefix(result.Content, "HTTP ") {
		t.Errorf("output not HTTP-text under canonical contract — audit P3-2:\n  content=%q", result.Content)
	}
}

// ─── Gap 6: All 5 action branches must honour HTTP-text contract ──────────

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

// ─── Gap 7: Error responses must also be HTTP-text ────────────────────────

// TestAlignmentRemoteTriggerErrorBodyAlsoHTTPText asserts that even non-2xx
// responses are surfaced as HTTP-text. Today they're equally JSON-wrapped.
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

	// Post-fix: "HTTP 400\n{...}" so the user sees the status verbatim.
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
// setup so the alignment tests stay focused on output / policy / flag gaps.
func newAlignmentRemoteTriggerTool(baseURL string, client *http.Client) *RemoteTriggerTool {
	return &RemoteTriggerTool{
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
// post-fix HTTP-text contract.
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

	// Post-fix expectation: HTTP-text envelope.
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
