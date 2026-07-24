package tools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func task36Summariser(calls *atomic.Int32) SummariserClient {
	return SummariserFunc(func(ctx context.Context, req SummariserRequest) (string, error) {
		if calls != nil {
			calls.Add(1)
		}
		return req.Prompt + ": " + req.UserPrompt, nil
	})
}

func task36HTTPTool(cache *WebFetchCache, summariser SummariserClient) *WebFetchTool {
	tool := NewWebFetchTool(cache)
	tool.skipSSRFCheck = true
	tool.SkipWebFetchPreflight = true
	tool.Summariser = summariser
	return tool
}

func TestWebFetchTask36PermissionLifecycle(t *testing.T) {
	tool := NewWebFetchTool(nil)
	input := map[string]any{"url": "https://example.com/a", "prompt": "p"}
	rule := types.PermissionRuleValue{ToolName: "WebFetch", RuleContent: "domain:example.com"}

	for _, tc := range []struct {
		name    string
		runtime types.ToolRuntimeContext
		want    types.PermissionBehavior
	}{
		{name: "default asks", want: types.PermissionBehaviorAsk},
		{name: "deny rule", runtime: types.ToolRuntimeContext{DeniedRules: []types.PermissionRuleValue{rule}}, want: types.PermissionBehaviorDeny},
		{name: "ask rule", runtime: types.ToolRuntimeContext{AskRules: []types.PermissionRuleValue{rule}}, want: types.PermissionBehaviorAsk},
		{name: "allow rule", runtime: types.ToolRuntimeContext{AllowedRules: []types.PermissionRuleValue{rule}}, want: types.PermissionBehaviorAllow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: tc.runtime})
			if err != nil {
				t.Fatal(err)
			}
			if got.Behavior != tc.want {
				t.Fatalf("behavior=%q want %q", got.Behavior, tc.want)
			}
			if got.Behavior == types.PermissionBehaviorAsk {
				if len(got.Suggestions) != 1 || got.Suggestions[0].Destination != types.PermissionDestinationLocalSettings ||
					len(got.Suggestions[0].Rules) != 1 || got.Suggestions[0].Rules[0].RuleContent != "domain:example.com" {
					t.Fatalf("missing TS addRules/localSettings suggestion: %+v", got.Suggestions)
				}
			}
		})
	}

	preapproved, err := tool.CheckPermissions(context.Background(), map[string]any{
		"url": "https://go.dev/doc/", "prompt": "p",
	}, types.ToolPermissionRequest{})
	if err != nil || preapproved.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("preapproved decision=%+v err=%v", preapproved, err)
	}

	managed := NewWebFetchTool(nil)
	managed.DisallowedDomains = []string{"go.dev"}
	denied, err := managed.CheckPermissions(context.Background(), map[string]any{
		"url": "https://go.dev/doc/", "prompt": "p",
	}, types.ToolPermissionRequest{})
	if err != nil || denied.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("managed deny must override preapproval: %+v err=%v", denied, err)
	}
}

func TestWebFetchTask36PreapprovedMatchingIsExact(t *testing.T) {
	for _, rawURL := range []string{
		"https://typed.docs.python.org/3/",
		"https://stable.go.dev/doc/",
	} {
		if IsPreapprovedHost(rawURL) {
			t.Fatalf("TS hostname-only allowlist must not expand to subdomain: %s", rawURL)
		}
	}
}

func TestWebFetchTask36DomainInfoFailClosedAndAllowedOnlyCache(t *testing.T) {
	ResetDomainInfoCache()
	t.Cleanup(ResetDomainInfoCache)
	var calls atomic.Int32
	mode := atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		switch mode.Load() {
		case 0:
			_, _ = fmt.Fprint(w, `{"can_fetch":true}`)
		case 1:
			_, _ = fmt.Fprint(w, `{"can_fetch":false}`)
		case 2:
			http.Error(w, "unavailable", http.StatusBadGateway)
		default:
			_, _ = fmt.Fprint(w, `{"unexpected":true}`)
		}
	}))
	defer srv.Close()

	blocked, err := domainInfoLookup(context.Background(), srv.Client(), srv.URL, "allowed.example")
	if err != nil || blocked {
		t.Fatalf("allowed verdict: blocked=%v err=%v", blocked, err)
	}
	blocked, err = domainInfoLookup(context.Background(), srv.Client(), srv.URL, "allowed.example")
	if err != nil || blocked || calls.Load() != 1 {
		t.Fatalf("allowed cache miss: blocked=%v err=%v calls=%d", blocked, err, calls.Load())
	}

	mode.Store(1)
	for i := 0; i < 2; i++ {
		blocked, err = domainInfoLookup(context.Background(), srv.Client(), srv.URL, "blocked.example")
		if err != nil || !blocked {
			t.Fatalf("blocked verdict %d: blocked=%v err=%v", i, blocked, err)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("blocked verdict must not cache, calls=%d", calls.Load())
	}

	mode.Store(2)
	for i := 0; i < 2; i++ {
		if _, err = domainInfoLookup(context.Background(), srv.Client(), srv.URL, "failed.example"); err == nil {
			t.Fatalf("non-200 attempt %d must fail closed", i)
		}
	}
	if calls.Load() != 5 {
		t.Fatalf("failed verdict must not cache, calls=%d", calls.Load())
	}

	mode.Store(3)
	if _, err = domainInfoLookup(context.Background(), srv.Client(), srv.URL, "malformed.example"); err == nil {
		t.Fatal("missing can_fetch must be a check failure")
	}
}

func TestWebFetchTask36ExecuteDomainInfoDefaultAndSkip(t *testing.T) {
	var preflightCalls, originCalls atomic.Int32
	preflight := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		preflightCalls.Add(1)
		_, _ = fmt.Fprint(w, `{"can_fetch":false}`)
	}))
	defer preflight.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		originCalls.Add(1)
		_, _ = fmt.Fprint(w, "origin")
	}))
	defer origin.Close()

	tool := task36HTTPTool(NewWebFetchCacheWithTTL(time.Minute, 0), task36Summariser(nil))
	defer tool.FetchCache().Stop()
	tool.SkipWebFetchPreflight = false
	tool.DomainInfoEndpoint = preflight.URL
	tool.DomainInfoClient = preflight.Client()
	result, err := tool.Execute(context.Background(), map[string]any{"url": origin.URL, "prompt": "p"})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "unable to fetch") || originCalls.Load() != 0 {
		t.Fatalf("fail-closed result=%+v err=%v originCalls=%d", result, err, originCalls.Load())
	}

	tool.SkipWebFetchPreflight = true
	result, err = tool.Execute(context.Background(), map[string]any{"url": origin.URL, "prompt": "p"})
	if err != nil || result.IsError || originCalls.Load() != 1 {
		t.Fatalf("skip preflight result=%+v err=%v originCalls=%d", result, err, originCalls.Load())
	}
	if preflightCalls.Load() != 1 {
		t.Fatalf("unexpected preflight calls=%d", preflightCalls.Load())
	}
}

func TestWebFetchTask36RedirectPolicyAndOutput(t *testing.T) {
	for _, tc := range []struct {
		name, from, to string
		want           bool
	}{
		{"same", "https://example.com/a", "https://example.com/b", true},
		{"add www", "https://example.com/a", "https://www.example.com/b", true},
		{"remove www", "https://www.example.com/a", "https://example.com/b", true},
		{"protocol", "https://example.com/a", "http://example.com/b", false},
		{"port", "https://example.com/a", "https://example.com:8443/b", false},
		{"userinfo", "https://example.com/a", "https://user@example.com/b", false},
		{"subdomain", "https://example.com/a", "https://api.example.com/b", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameOriginRedirect(tc.from, tc.to); got != tc.want {
				t.Fatalf("sameOriginRedirect(%q,%q)=%v want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}

	message := formatRedirectMarker("https://a.example/old", "https://b.example/new", 308, "extract title")
	for _, want := range []string{
		"REDIRECT DETECTED: The URL redirects to a different host.",
		"Original URL: https://a.example/old",
		"Redirect URL: https://b.example/new",
		"Status: 308 Permanent Redirect",
		`- url: "https://b.example/new"`,
		`- prompt: "extract title"`,
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("redirect output missing %q:\n%s", want, message)
		}
	}

	for _, redirects := range []int{10, 11} {
		t.Run(strconv.Itoa(redirects), func(t *testing.T) {
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
				if n < redirects {
					http.Redirect(w, r, fmt.Sprintf("/%d", n+1), http.StatusFound)
					return
				}
				w.Header().Set("Content-Type", "text/plain")
				_, _ = fmt.Fprint(w, "final")
			}))
			defer srv.Close()
			cache := NewWebFetchCacheWithTTL(time.Minute, 0)
			defer cache.Stop()
			tool := task36HTTPTool(cache, task36Summariser(nil))
			result, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL + "/0", "prompt": "p"})
			if err != nil {
				t.Fatal(err)
			}
			if redirects == 10 && result.IsError {
				t.Fatalf("10 redirects should succeed: %s", result.Content)
			}
			if redirects == 11 && (!result.IsError || !strings.Contains(result.Content, "10 redirects")) {
				t.Fatalf("11 redirects should fail: %+v", result)
			}
		})
	}
}

func TestWebFetchTask36RelativeRedirectAndNonCacheableFailures(t *testing.T) {
	t.Run("relative redirect follows and caches final content", func(t *testing.T) {
		var calls atomic.Int32
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if r.URL.Path == "/start" {
				http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = fmt.Fprint(w, "relative final")
		}))
		defer srv.Close()
		cache := NewWebFetchCacheWithTTL(time.Minute, 0)
		defer cache.Stop()
		tool := task36HTTPTool(cache, task36Summariser(nil))
		result, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL + "/start", "prompt": "p"})
		if err != nil || result.IsError || !strings.Contains(result.Content, "relative final") || calls.Load() != 2 || cache.Len() != 1 {
			t.Fatalf("relative redirect result=%+v err=%v calls=%d cache=%d", result, err, calls.Load(), cache.Len())
		}
	})

	t.Run("cross-origin redirect is never cached", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, "must not fetch")
		}))
		defer target.Close()
		var calls atomic.Int32
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			http.Redirect(w, r, target.URL, http.StatusFound)
		}))
		defer origin.Close()
		cache := NewWebFetchCacheWithTTL(time.Minute, 0)
		defer cache.Stop()
		tool := task36HTTPTool(cache, task36Summariser(nil))
		for i := 0; i < 2; i++ {
			result, err := tool.Execute(context.Background(), map[string]any{"url": origin.URL, "prompt": "p"})
			if err != nil || result.IsError || !strings.Contains(result.Content, "REDIRECT DETECTED") {
				t.Fatalf("attempt %d result=%+v err=%v", i, result, err)
			}
		}
		if calls.Load() != 2 || cache.Len() != 0 {
			t.Fatalf("redirect was cached: calls=%d cache=%d", calls.Load(), cache.Len())
		}
	})

	t.Run("http errors are never cached", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			http.Error(w, "no", http.StatusServiceUnavailable)
		}))
		defer srv.Close()
		cache := NewWebFetchCacheWithTTL(time.Minute, 0)
		defer cache.Stop()
		tool := task36HTTPTool(cache, task36Summariser(nil))
		for i := 0; i < 2; i++ {
			result, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL, "prompt": "p"})
			if err != nil || !result.IsError {
				t.Fatalf("attempt %d result=%+v err=%v", i, result, err)
			}
		}
		if calls.Load() != 4 || cache.Len() != 0 {
			t.Fatalf("HTTP error was cached: calls=%d cache=%d", calls.Load(), cache.Len())
		}
	})
}

func TestWebFetchTask36RawURLCacheReappliesPrompts(t *testing.T) {
	var network, summaries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		network.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "shared body")
	}))
	defer srv.Close()
	cache := NewWebFetchCacheWithTTL(time.Minute, 0)
	defer cache.Stop()
	tool := task36HTTPTool(cache, task36Summariser(&summaries))

	first, _ := tool.Execute(context.Background(), map[string]any{"url": srv.URL, "prompt": "first"})
	second, _ := tool.Execute(context.Background(), map[string]any{"url": srv.URL, "prompt": "second"})
	if first.IsError || second.IsError || network.Load() != 1 || summaries.Load() != 2 {
		t.Fatalf("network=%d summaries=%d first=%+v second=%+v", network.Load(), summaries.Load(), first, second)
	}
	if !strings.HasPrefix(first.Content, "first:") || !strings.HasPrefix(second.Content, "second:") {
		t.Fatalf("prompts were not reapplied: first=%q second=%q", first.Content, second.Content)
	}
	if cache.Len() != 1 {
		t.Fatalf("raw URL cache must have one entry, got %d", cache.Len())
	}
	entry, ok := cache.Get(cache.MakeKey(srv.URL))
	if !ok || entry.Body != "shared body" || entry.StatusCode != 200 || entry.StatusText != "OK" || entry.Bytes != len("shared body") {
		t.Fatalf("raw metadata missing: %+v hit=%v", entry, ok)
	}

	tool.ClearWebFetchCache()
	if cache.Len() != 0 {
		t.Fatalf("clear lifecycle did not empty cache")
	}
}

func TestWebFetchTask36SummariserFailureAndCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "body")
	}))
	defer srv.Close()

	cache := NewWebFetchCacheWithTTL(time.Minute, 0)
	defer cache.Stop()
	tool := task36HTTPTool(cache, SummariserFunc(func(context.Context, SummariserRequest) (string, error) {
		return "", errors.New("secondary model down")
	}))
	result, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL, "prompt": "p"})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "secondary model down") || strings.Contains(result.Content, "body") {
		t.Fatalf("summariser failure was hidden: result=%+v err=%v", result, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	tool.Summariser = SummariserFunc(func(ctx context.Context, _ SummariserRequest) (string, error) {
		cancel()
		<-ctx.Done()
		return "heuristic must not escape", ctx.Err()
	})
	_, err = tool.Execute(ctx, map[string]any{"url": srv.URL, "prompt": "cancel"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation must propagate as infrastructure error, got %v", err)
	}
}

func TestWebFetchTask36BinarySummaryAndSavedNote(t *testing.T) {
	binaryDir := t.TempDir()
	t.Setenv("CLAUDE_WEBFETCH_BINARY_DIR", binaryDir)
	var summaries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = fmt.Fprint(w, "%PDF-1.7 /Title (Alignment)")
	}))
	defer srv.Close()
	cache := NewWebFetchCacheWithTTL(time.Minute, 0)
	defer cache.Stop()
	tool := task36HTTPTool(cache, task36Summariser(&summaries))
	result, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL + "/doc.pdf", "prompt": "title"})
	if err != nil || result.IsError || summaries.Load() != 1 {
		t.Fatalf("binary result=%+v err=%v summaries=%d", result, err, summaries.Load())
	}
	for _, want := range []string{"title:", "[Binary content (application/pdf,", "also saved to", ".pdf]"} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("binary result missing %q: %q", want, result.Content)
		}
	}
	entry, ok := cache.Get(cache.MakeKey(srv.URL + "/doc.pdf"))
	if !ok || entry.PersistedPath == "" || entry.PersistedSize == 0 {
		t.Fatalf("binary cache metadata missing: %+v hit=%v", entry, ok)
	}
	if _, err := os.Stat(entry.PersistedPath); err != nil {
		t.Fatalf("persisted binary missing: %v", err)
	}

	badRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(badRoot, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_WEBFETCH_BINARY_DIR", badRoot)
	other := task36HTTPTool(NewWebFetchCacheWithTTL(time.Minute, 0), task36Summariser(nil))
	defer other.FetchCache().Stop()
	result, err = other.Execute(context.Background(), map[string]any{"url": srv.URL + "/other.pdf", "prompt": "title"})
	if err != nil || result.IsError || strings.Contains(result.Content, "also saved") {
		t.Fatalf("persist failure should still summarise without false saved note: result=%+v err=%v", result, err)
	}
}

func TestWebFetchTask36StrictSchemaAndTypedOutput(t *testing.T) {
	tool := task36HTTPTool(NewWebFetchCacheWithTTL(time.Minute, 0), task36Summariser(nil))
	defer tool.FetchCache().Stop()
	schema := tool.Schema()
	if !schema.RejectsUnknownFields() {
		t.Fatal("WebFetch schema must be strict")
	}
	if len(schema.Required) != 2 || schema.Required[0] != "url" || schema.Required[1] != "prompt" {
		t.Fatalf("WebFetch required fields = %v, want [url prompt]", schema.Required)
	}
	urlSchema, _ := schema.Properties["url"].(map[string]any)
	if urlSchema["format"] != "uri" {
		t.Fatalf("url schema must advertise URI format: %+v", urlSchema)
	}
	promptSchema, _ := schema.Properties["prompt"].(map[string]any)
	if promptSchema["minLength"] != 1 {
		t.Fatalf("prompt schema must reject empty provider-generated values: %+v", promptSchema)
	}
	result, err := tool.Execute(context.Background(), map[string]any{
		"url": "https://example.com", "prompt": "p", "extra": true,
	})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "unknown field") {
		t.Fatalf("direct Execute must reject unknown fields: result=%+v err=%v", result, err)
	}

	built := buildWebFetchResult(WebFetchOutput{
		Bytes: 7, Code: 200, CodeText: "OK", Result: "answer", DurationMs: 12, URL: "https://example.com",
	}, webExecutionModeLocalFallback)
	out, ok := built.Data.(WebFetchOutput)
	if !ok || out.Bytes != 7 || out.Code != 200 || out.CodeText != "OK" || out.Result != "answer" || out.DurationMs != 12 || out.URL == "" {
		t.Fatalf("typed output mismatch: %#v", built.Data)
	}
	if built.Content != "answer" || strings.Contains(built.Content, "Prompt:") {
		t.Fatalf("model content must be result-only: %q", built.Content)
	}
	block := tool.MapToolResultToToolResultBlock(built.Data, "toolu_1")
	if block.Content != "answer" || block.ToolUseID != "toolu_1" || block.Data == nil {
		t.Fatalf("mapper mismatch: %+v", block)
	}
}

func TestWebFetchTask36HTTPHeadersAndOversizedBody(t *testing.T) {
	var gotAccept, gotAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(make([]byte, maxBodyBytes+1))
	}))
	defer srv.Close()
	cache := NewWebFetchCacheWithTTL(time.Minute, 0)
	defer cache.Stop()
	tool := task36HTTPTool(cache, task36Summariser(nil))
	result, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL, "prompt": "p"})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "10 MB") {
		t.Fatalf("oversized body must fail: result=%+v err=%v", result, err)
	}
	if gotAccept != "text/markdown, text/html, */*" || gotAgent != userAgent {
		t.Fatalf("headers accept=%q agent=%q", gotAccept, gotAgent)
	}
	if cache.Len() != 0 {
		t.Fatalf("oversized response must not cache")
	}
}
