package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestRemoteTrigger_PolicyForbidsBlocks (RT-02): when the
// allow_remote_sessions policy is false, RemoteTrigger refuses without
// hitting the network.
func TestRemoteTrigger_PolicyForbidsBlocks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server must not be reached when policy denies")
	}))
	defer srv.Close()

	tool := &RemoteTriggerTool{
		HTTPClient: srv.Client(),
		PolicyResolver: func(context.Context) (bool, error) {
			return false, nil // policy says no
		},
	}
	res, _ := tool.Execute(context.Background(), map[string]any{"action": "list"})
	if !res.IsError {
		t.Fatalf("expected refusal, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "allow_remote_sessions") {
		t.Fatalf("refusal missing policy reference: %s", res.Content)
	}
}

// TestRemoteTrigger_FeatureFlagDisabledBlocks (RT-03): when the feature
// flag returns disabled, the tool refuses with the kill-switch message.
func TestRemoteTrigger_FeatureFlagDisabledBlocks(t *testing.T) {
	tool := &RemoteTriggerTool{
		FeatureFlagResolver: func(context.Context) (bool, error) {
			return false, nil // feature disabled
		},
	}
	res, _ := tool.Execute(context.Background(), map[string]any{"action": "list"})
	if !res.IsError {
		t.Fatalf("expected refusal, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "tengu_surreal_dali") &&
		!strings.Contains(res.Content, "feature flag") {
		t.Fatalf("refusal missing feature-flag reference: %s", res.Content)
	}
}

// TestRemoteTrigger_FeatureFlagEnabledAllowsRequest verifies the resolver
// returning true does not block.
func TestRemoteTrigger_FeatureFlagEnabledAllowsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	tool := &RemoteTriggerTool{
		HTTPClient: srv.Client(),
		FeatureFlagResolver: func(context.Context) (bool, error) {
			return true, nil // enabled
		},
		PolicyResolver: func(context.Context) (bool, error) {
			return true, nil // allowed
		},
		AccessTokenResolver: func(context.Context) (string, error) {
			return "tok", nil
		},
		OrganizationUUIDResolver: func(context.Context, string, string) (string, error) {
			return "org", nil
		},
		BaseURLResolver: func() (string, error) { return srv.URL, nil },
	}
	res, _ := tool.Execute(context.Background(), map[string]any{"action": "list"})
	if res.IsError {
		t.Fatalf("expected success, got: %s", res.Content)
	}
}

// TestRemoteTrigger_ListDoesNotPaginate matches TS: list performs one GET and
// returns the server body unchanged, including any next_cursor.
func TestRemoteTrigger_ListDoesNotPaginate(t *testing.T) {
	var page atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch p := page.Add(1); p {
		case 1:
			_, _ = w.Write([]byte(`{"triggers":[{"id":"a"}],"next_cursor":"c2"}`))
		default:
			t.Fatalf("unexpected extra page %d", p)
		}
	}))
	defer srv.Close()

	tool := &RemoteTriggerTool{
		HTTPClient:               srv.Client(),
		AccessTokenResolver:      func(context.Context) (string, error) { return "tok", nil },
		OrganizationUUIDResolver: func(context.Context, string, string) (string, error) { return "org", nil },
		BaseURLResolver:          func() (string, error) { return srv.URL, nil },
	}
	res, _ := tool.Execute(context.Background(), map[string]any{"action": "list"})
	if res.IsError {
		t.Fatalf("expected success, got: %s", res.Content)
	}
	if got := page.Load(); got != 1 {
		t.Fatalf("expected exactly one list request, got %d", got)
	}
	if !strings.Contains(res.Content, `"next_cursor":"c2"`) {
		t.Fatalf("list body was not returned unchanged: %s", res.Content)
	}
	for _, synthetic := range []string{`"page_count"`, `"max_pages"`, `"truncated"`, `"last_cursor"`} {
		if strings.Contains(res.Content, synthetic) {
			t.Fatalf("list output contains synthetic pagination field %s: %s", synthetic, res.Content)
		}
	}
}

// TestRemoteTrigger_NoRetryOn5xx matches TS: one request is made and the 503
// response is surfaced as the tool result.
func TestRemoteTrigger_NoRetryOn5xx(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		attempts.Add(1)
		http.Error(w, `{"error":"upstream"}`, http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	tool := &RemoteTriggerTool{
		HTTPClient:               srv.Client(),
		AccessTokenResolver:      func(context.Context) (string, error) { return "tok", nil },
		OrganizationUUIDResolver: func(context.Context, string, string) (string, error) { return "org", nil },
		BaseURLResolver:          func() (string, error) { return srv.URL, nil },
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"action":     "get",
		"trigger_id": "x1",
	})
	if res.IsError {
		t.Fatalf("HTTP failures should be model-visible results, got error: %s", res.Content)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected exactly one request after 503, got %d", got)
	}
	if !strings.Contains(res.Content, "HTTP 503") || !strings.Contains(res.Content, "upstream") {
		t.Fatalf("503 body was not surfaced: %s", res.Content)
	}
}
