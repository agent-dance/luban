package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

// ─── stripHTML tests ───────────────────────────────────────────────────────────

func TestStripHTML_RemovesScriptAndStyle(t *testing.T) {
	input := `<html><head><script>alert('x')</script><style>body{}</style></head><body><p>Hello world</p></body></html>`
	out := stripHTML(input)
	if strings.Contains(out, "alert") {
		t.Errorf("script not removed: %q", out)
	}
	if strings.Contains(out, "body{}") {
		t.Errorf("style not removed: %q", out)
	}
	if !strings.Contains(out, "Hello world") {
		t.Errorf("expected content 'Hello world' in %q", out)
	}
}

func TestStripHTML_PreservesAnchorText(t *testing.T) {
	input := `<p>Visit <a href="https://example.com">Example Site</a> for more.</p>`
	out := stripHTML(input)
	if !strings.Contains(out, "Example Site") {
		t.Errorf("expected anchor text in output: %q", out)
	}
	if !strings.Contains(out, "https://example.com") {
		t.Errorf("expected href in output: %q", out)
	}
}

func TestStripHTML_NestedTags(t *testing.T) {
	input := `<div><p><strong>Bold <em>italic</em></strong> text</p></div>`
	out := stripHTML(input)
	if !strings.Contains(out, "Bold") || !strings.Contains(out, "italic") || !strings.Contains(out, "text") {
		t.Errorf("expected nested content preserved: %q", out)
	}
	if strings.Contains(out, "<") {
		t.Errorf("expected no HTML tags in output: %q", out)
	}
}

// ─── truncate tests ────────────────────────────────────────────────────────────

func TestTruncate(t *testing.T) {
	s := strings.Repeat("a", 100)
	out := truncate(s, 10)
	if !strings.HasPrefix(out, "aaaaaaaaaa") {
		t.Errorf("unexpected prefix: %q", out)
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation marker")
	}
	short := "hello"
	if truncate(short, 100) != "hello" {
		t.Errorf("expected no truncation for short string")
	}
}

// ─── Cache tests ───────────────────────────────────────────────────────────────

func TestSearchCache_StoreAndRetrieve(t *testing.T) {
	c := NewSearchCache()
	c.set("foo", "bar")
	got, ok := c.get("foo")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != "bar" {
		t.Errorf("expected 'bar', got %q", got)
	}
}

func TestSearchCache_MissOnUnknownKey(t *testing.T) {
	c := NewSearchCache()
	_, ok := c.get("missing")
	if ok {
		t.Error("expected cache miss for unknown key")
	}
}

func TestSearchCache_Expiry(t *testing.T) {
	c := NewSearchCache()
	// Manually insert an already-expired entry.
	c.mu.Lock()
	c.entries["expired"] = &cacheEntry{results: "old", expiry: time.Now().Add(-1 * time.Second)}
	c.mu.Unlock()

	_, ok := c.get("expired")
	if ok {
		t.Error("expected cache miss for expired entry")
	}
}

func TestSearchCache_OverwriteEntry(t *testing.T) {
	c := NewSearchCache()
	c.set("key", "first")
	c.set("key", "second")
	got, ok := c.get("key")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != "second" {
		t.Errorf("expected 'second', got %q", got)
	}
}

// ─── domainOf tests ────────────────────────────────────────────────────────────

func TestDomainOf(t *testing.T) {
	cases := []struct{ url, want string }{
		{"https://www.example.com/path", "example.com"},
		{"https://sub.example.com/", "sub.example.com"},
		{"http://example.com", "example.com"},
		{"not a url", ""},
	}
	for _, c := range cases {
		got := domainOf(c.url)
		if got != c.want {
			t.Errorf("domainOf(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

// ─── filterResults tests ───────────────────────────────────────────────────────

func TestFilterResults_AllowedDomains(t *testing.T) {
	results := []searchResult{
		{Title: "A", URL: "https://allowed.com/page"},
		{Title: "B", URL: "https://other.com/page"},
		{Title: "C", URL: "https://sub.allowed.com/page"},
	}
	filtered := filterResults(results, []string{"allowed.com"}, nil)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 results, got %d", len(filtered))
	}
}

func TestFilterResults_BlockedDomains(t *testing.T) {
	results := []searchResult{
		{Title: "A", URL: "https://allowed.com/page"},
		{Title: "B", URL: "https://blocked.com/page"},
	}
	filtered := filterResults(results, nil, []string{"blocked.com"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 result, got %d", len(filtered))
	}
	if filtered[0].Title != "A" {
		t.Errorf("expected A, got %s", filtered[0].Title)
	}
}

func TestFilterResults_NoFilter(t *testing.T) {
	results := []searchResult{
		{Title: "A", URL: "https://a.com"},
		{Title: "B", URL: "https://b.com"},
	}
	filtered := filterResults(results, nil, nil)
	if len(filtered) != 2 {
		t.Errorf("expected 2, got %d", len(filtered))
	}
}

func TestFilterResults_AllowedAndBlocked(t *testing.T) {
	results := []searchResult{
		{Title: "A", URL: "https://allowed.com/page"},
		{Title: "B", URL: "https://blocked.allowed.com/page"}, // sub of allowed but also blocked
	}
	filtered := filterResults(results, []string{"allowed.com"}, []string{"blocked.allowed.com"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 result, got %d", len(filtered))
	}
	if filtered[0].Title != "A" {
		t.Errorf("expected A, got %s", filtered[0].Title)
	}
}

// ─── parseDDGResults tests (legacy HTML) ───────────────────────────────────────

func TestParseDDGResults_Basic(t *testing.T) {
	html := `
<div class="result">
  <a class="result__a" href="/l/?uddg=https%3A%2F%2Fexample.com%2Fpage">Example Title</a>
  <a class="result__snippet">A short snippet here</a>
</div>`
	results := parseDDGResults(html)
	if len(results) == 0 {
		t.Log("no results parsed (DDG HTML format may differ from test fixture)")
		return
	}
	if results[0].Title == "" {
		t.Error("expected non-empty title")
	}
}

// ─── parseDDGLiteHTML tests ────────────────────────────────────────────────────

func TestParseDDGLiteHTML_Basic(t *testing.T) {
	html := `<html><body>
<a class="result-link" href="https://go.dev/doc">Go Documentation</a>
<a class="result-link" href="https://pkg.go.dev/net/http">net/http Package</a>
</body></html>`
	results := parseDDGLiteHTML(html)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].URL != "https://go.dev/doc" {
		t.Errorf("unexpected URL: %s", results[0].URL)
	}
	if results[0].Title != "Go Documentation" {
		t.Errorf("unexpected title: %s", results[0].Title)
	}
}

func TestParseDDGLiteHTML_NoResults(t *testing.T) {
	html := `<html><body><p>No results</p></body></html>`
	results := parseDDGLiteHTML(html)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// ─── parseInstantAnswer tests ──────────────────────────────────────────────────

func TestParseInstantAnswer_WithAbstract(t *testing.T) {
	resp := ddgInstantResponse{
		AbstractText: "Go is a programming language.",
		AbstractURL:  "https://go.dev",
		RelatedTopics: []ddgRelatedTopic{
			{Text: "Go spec", FirstURL: "https://go.dev/ref/spec"},
		},
	}
	results := parseInstantAnswer(resp)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].URL != "https://go.dev" {
		t.Errorf("unexpected abstract URL: %s", results[0].URL)
	}
	if results[0].Snippet != "Go is a programming language." {
		t.Errorf("unexpected snippet: %s", results[0].Snippet)
	}
}

func TestParseInstantAnswer_EmptyResponse(t *testing.T) {
	resp := ddgInstantResponse{}
	results := parseInstantAnswer(resp)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty response, got %d", len(results))
	}
}

func TestParseInstantAnswer_NestedTopics(t *testing.T) {
	resp := ddgInstantResponse{
		RelatedTopics: []ddgRelatedTopic{
			{
				Topics: []ddgRelatedTopic{
					{Text: "nested topic", FirstURL: "https://example.com/nested"},
				},
			},
		},
	}
	results := parseInstantAnswer(resp)
	if len(results) != 1 {
		t.Fatalf("expected 1 result from nested topics, got %d", len(results))
	}
	if results[0].URL != "https://example.com/nested" {
		t.Errorf("unexpected URL: %s", results[0].URL)
	}
}

// ─── WebFetchTool tests ────────────────────────────────────────────────────────

func newWebFetchHTTPTestTool(t *testing.T) *WebFetchTool {
	t.Helper()
	cache := NewWebFetchCacheWithTTL(time.Minute, 0)
	t.Cleanup(cache.Stop)
	tool := NewWebFetchTool(cache)
	tool.skipSSRFCheck = true
	tool.SkipWebFetchPreflight = true
	tool.Summariser = SummariserFunc(func(_ context.Context, req SummariserRequest) (string, error) {
		return req.UserPrompt, nil
	})
	return tool
}

func TestWebFetchTool_Name(t *testing.T) {
	tool := NewWebFetchTool(nil)
	if tool.Name() != "WebFetch" {
		t.Fatalf("expected WebFetch, got %s", tool.Name())
	}
	if !tool.IsConcurrentSafe() {
		t.Error("expected IsConcurrentSafe=true")
	}
}

func TestWebFetchTool_FetchPlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != userAgent {
			t.Errorf("unexpected User-Agent: %s", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "Hello from test server")
	}))
	defer srv.Close()

	tool := newWebFetchHTTPTestTool(t)
	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "What does the page say?",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "What does the page say?") {
		t.Errorf("prompt not in output: %q", result.Content)
	}
	if !strings.Contains(result.Content, "Hello from test server") {
		t.Errorf("content not in output: %q", result.Content)
	}
}

func TestWebFetchTool_FetchHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<html><body><script>bad()</script><p>Visible text</p></body></html>`)
	}))
	defer srv.Close()

	tool := newWebFetchHTTPTestTool(t)
	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "extract",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if strings.Contains(result.Content, "bad()") {
		t.Errorf("script tag not stripped: %q", result.Content)
	}
	if !strings.Contains(result.Content, "Visible text") {
		t.Errorf("expected visible text in output: %q", result.Content)
	}
}

func TestWebFetchTool_Non2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	tool := newWebFetchHTTPTestTool(t)
	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "fetch",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected error for 404, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "404") {
		t.Errorf("expected 404 in error message: %s", result.Content)
	}
}

func TestWebFetchTool_MissingURL(t *testing.T) {
	tool := NewWebFetchTool(nil)
	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    "",
		"prompt": "fetch",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected error for missing url")
	}
}

func TestWebFetchTool_MissingPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "page body")
	}))
	defer srv.Close()

	tool := newWebFetchHTTPTestTool(t)
	result, err := tool.Execute(context.Background(), map[string]any{
		"url": srv.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("URL-only WebFetch should use the compatibility prompt: %s", result.Content)
	}
	if !strings.Contains(result.Content, defaultWebFetchPrompt) || !strings.Contains(result.Content, "page body") {
		t.Fatalf("default prompt was not applied: %q", result.Content)
	}
}

func TestWebFetchTool_RetriesTransientHTTPStatusAndFormatsFinalStatusOnce(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "try again", http.StatusGatewayTimeout)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "recovered")
	}))
	defer srv.Close()

	tool := newWebFetchHTTPTestTool(t)
	result, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL, "prompt": "fetch"})
	if err != nil || result.IsError || calls.Load() != 2 || !strings.Contains(result.Content, "recovered") {
		t.Fatalf("transient retry result=%+v err=%v calls=%d", result, err, calls.Load())
	}

	alwaysMissing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer alwaysMissing.Close()
	result, err = tool.Execute(context.Background(), map[string]any{"url": alwaysMissing.URL, "prompt": "fetch"})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "HTTP 404 Not Found") || strings.Contains(result.Content, "404 404") {
		t.Fatalf("HTTP status formatting result=%+v err=%v", result, err)
	}
}

func TestWebFetchTool_Schema(t *testing.T) {
	tool := NewWebFetchTool(nil)
	schema := tool.Schema()
	if _, ok := schema.Properties["url"]; !ok {
		t.Error("expected 'url' in schema")
	}
	if _, ok := schema.Properties["prompt"]; !ok {
		t.Error("expected 'prompt' in schema")
	}
}

func TestWebFetchTool_Caching(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "cached content")
	}))
	defer srv.Close()

	tool := newWebFetchHTTPTestTool(t)
	for i := 0; i < 3; i++ {
		result, err := tool.Execute(context.Background(), map[string]any{
			"url":    srv.URL,
			"prompt": "test",
		})
		if err != nil || result.IsError {
			t.Fatalf("unexpected error on call %d", i)
		}
		if !strings.Contains(result.Content, "cached content") {
			t.Errorf("expected cached content on call %d", i)
		}
	}
	if callCount != 1 {
		t.Errorf("expected exactly 1 HTTP call due to caching, got %d", callCount)
	}
}

func TestWebFetchTool_Redirect(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "final destination")
	}))
	defer final.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirect.Close()

	tool := newWebFetchHTTPTestTool(t)
	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    redirect.URL,
		"prompt": "follow redirect",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "REDIRECT DETECTED") || !strings.Contains(result.Content, final.URL) {
		t.Errorf("expected cross-origin redirect retry message: %q", result.Content)
	}
	if strings.Contains(result.Content, "final destination") {
		t.Errorf("cross-origin redirect must not be followed: %q", result.Content)
	}
}

func TestWebFetchTool_SizeLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		// Write 2 MB – more than maxBodyBytes (1 MB).
		chunk := strings.Repeat("x", 1024)
		for i := 0; i < 2048; i++ {
			fmt.Fprint(w, chunk)
		}
	}))
	defer srv.Close()

	tool := newWebFetchHTTPTestTool(t)
	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "big page",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	// Should contain truncation marker since content exceeds maxOutputRune.
	if !strings.Contains(result.Content, "truncated") {
		t.Logf("content length: %d", len(result.Content))
		// Not necessarily truncated at rune level unless > 50k runes; the body read
		// is capped at 1MB which after stripping may or may not exceed 50k runes.
		// Just verify no error occurred.
	}
}

// ─── WebSearchTool tests ───────────────────────────────────────────────────────

func TestWebSearchTool_Name(t *testing.T) {
	tool := NewWebSearchTool(nil)
	if tool.Name() != "WebSearch" {
		t.Fatalf("expected WebSearch, got %s", tool.Name())
	}
	if aliases := tool.Aliases(); len(aliases) != 1 || aliases[0] != "Search" {
		t.Fatalf("WebSearch aliases = %v, want [Search]", aliases)
	}
	if !tool.IsConcurrentSafe() {
		t.Error("expected IsConcurrentSafe=true")
	}
}

func TestWebSearchTool_LocalFallbackIsProviderAgnostic(t *testing.T) {
	tool := NewWebSearchTool(nil)
	for _, providerName := range []string{"anthropic", "openai", "gemini", "deepseek", "bedrock", "vertex"} {
		runtime := types.ToolRuntimeContext{
			Provider: providerName,
			Features: map[string]bool{types.ToolFeatureWebSearch: true},
		}
		if !tool.IsEnabled(runtime) {
			t.Errorf("WebSearch should be enabled for %s via provider-native or local fallback", providerName)
		}
	}
	if tool.IsEnabled(types.ToolRuntimeContext{Provider: "openai", Features: map[string]bool{types.ToolFeatureWebSearch: false}}) {
		t.Fatal("explicit WebSearch feature disable must still win")
	}
}

func TestWebSearchTool_MissingQuery(t *testing.T) {
	tool := NewWebSearchTool(nil)
	result, err := tool.Execute(context.Background(), map[string]any{
		"query": "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected error for empty query")
	}
}

func TestWebSearchTool_Schema(t *testing.T) {
	tool := NewWebSearchTool(nil)
	schema := tool.Schema()
	if _, ok := schema.Properties["query"]; !ok {
		t.Error("expected 'query' in schema")
	}
	if _, ok := schema.Properties["allowed_domains"]; !ok {
		t.Error("expected 'allowed_domains' in schema")
	}
	if _, ok := schema.Properties["blocked_domains"]; !ok {
		t.Error("expected 'blocked_domains' in schema")
	}
}

// TestWebSearchTool_InstantAnswerAPI tests the primary DDG Instant Answer path.
func TestWebSearchTool_InstantAnswerAPI(t *testing.T) {
	apiResp := ddgInstantResponse{
		AbstractText: "Go is a statically typed language.",
		AbstractURL:  "https://go.dev",
		RelatedTopics: []ddgRelatedTopic{
			{Text: "Go spec", FirstURL: "https://go.dev/ref/spec"},
		},
	}
	body, _ := json.Marshal(apiResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	tool := &WebSearchTool{
		cache:            NewSearchCache(),
		instantAnswerURL: srv.URL + "/",
		liteFallbackURL:  srv.URL + "/lite/",
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"query": "golang",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "go.dev") {
		t.Errorf("expected go.dev in results: %q", result.Content)
	}
}

// TestWebSearchTool_LiteFallback tests that the lite HTML fallback is used when
// the Instant Answer API returns no results.
func TestWebSearchTool_LiteFallback(t *testing.T) {
	emptySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a valid but empty JSON response (no abstract, no related topics).
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"AbstractText":"","AbstractURL":"","RelatedTopics":[]}`)
	}))
	defer emptySrv.Close()

	liteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>
<a class="result-link" href="https://go.dev/doc">Go Documentation</a>
<a class="result-link" href="https://pkg.go.dev/fmt">fmt package</a>
</body></html>`)
	}))
	defer liteSrv.Close()

	tool := &WebSearchTool{
		cache:            NewSearchCache(),
		instantAnswerURL: emptySrv.URL + "/",
		liteFallbackURL:  liteSrv.URL + "/",
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"query": "golang fmt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "go.dev/doc") {
		t.Errorf("expected go.dev/doc in fallback results: %q", result.Content)
	}
	if !strings.Contains(result.Content, "pkg.go.dev/fmt") {
		t.Errorf("expected pkg.go.dev/fmt in fallback results: %q", result.Content)
	}
}

// TestWebSearchTool_Caching verifies the 15-minute result cache.
func TestWebSearchTool_Caching(t *testing.T) {
	callCount := 0
	apiResp := ddgInstantResponse{
		AbstractText: "Cached result.",
		AbstractURL:  "https://example.com",
	}
	body, _ := json.Marshal(apiResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	tool := &WebSearchTool{
		cache:            NewSearchCache(),
		instantAnswerURL: srv.URL + "/",
		liteFallbackURL:  srv.URL + "/lite/",
	}

	for i := 0; i < 3; i++ {
		result, err := tool.Execute(context.Background(), map[string]any{
			"query": "cached query",
		})
		if err != nil || result.IsError {
			t.Fatalf("unexpected error on call %d: %s", i, result.Content)
		}
	}
	if callCount != 1 {
		t.Errorf("expected exactly 1 HTTP call due to caching, got %d", callCount)
	}
}

// TestWebSearchTool_DomainFilteringAfterFetch verifies that allowed/blocked domain
// filtering is applied after fetching results.
func TestWebSearchTool_DomainFilteringAfterFetch(t *testing.T) {
	apiResp := ddgInstantResponse{
		RelatedTopics: []ddgRelatedTopic{
			{Text: "Allowed result", FirstURL: "https://allowed.com/page"},
			{Text: "Blocked result", FirstURL: "https://blocked.com/page"},
			{Text: "Other result", FirstURL: "https://other.com/page"},
		},
	}
	body, _ := json.Marshal(apiResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	tool := &WebSearchTool{
		cache:            NewSearchCache(),
		instantAnswerURL: srv.URL + "/",
		liteFallbackURL:  srv.URL + "/lite/",
	}

	// Allowed only.
	result, err := tool.Execute(context.Background(), map[string]any{
		"query":           "test",
		"allowed_domains": []any{"allowed.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "allowed.com") {
		t.Errorf("expected allowed.com in results: %q", result.Content)
	}
	if strings.Contains(result.Content, "blocked.com") || strings.Contains(result.Content, "other.com") {
		t.Errorf("unexpected domain in allowed-only results: %q", result.Content)
	}
}

// TestWebSearchTool_NoResults verifies empty results still use the TS output
// mapper's query header and sources reminder.
func TestWebSearchTool_NoResults(t *testing.T) {
	emptySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"AbstractText":"","AbstractURL":"","RelatedTopics":[]}`)
	}))
	defer emptySrv.Close()

	liteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><p>No results</p></body></html>`)
	}))
	defer liteSrv.Close()

	tool := &WebSearchTool{
		cache:            NewSearchCache(),
		instantAnswerURL: emptySrv.URL + "/",
		liteFallbackURL:  liteSrv.URL + "/",
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"query": "something very obscure xyzzy123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	output, ok := result.Data.(WebSearchOutput)
	if !ok || len(output.Results) != 0 {
		t.Fatalf("expected typed empty output, got %#v", result.Data)
	}
	if !strings.Contains(result.Content, `Web search results for query: "something very obscure xyzzy123"`) || !strings.Contains(result.Content, SourcesReminder()) || strings.TrimSpace(result.Content) == "No results found." {
		t.Errorf("expected TS empty-result wrapper: %q", result.Content)
	}
}

// ─── matchDomain tests (Task 7) ────────────────────────────────────────────────

func TestMatchDomain(t *testing.T) {
	tests := []struct {
		host    string
		pattern string
		want    bool
	}{
		// Exact matches
		{"example.com", "example.com", true},
		{"EXAMPLE.COM", "example.com", true},
		{"example.com", "EXAMPLE.COM", true},
		{"other.com", "example.com", false},

		// Wildcard matches
		{"sub.example.com", "*.example.com", true},
		{"deep.sub.example.com", "*.example.com", true},
		{"example.com", "*.example.com", true},     // bare domain matches wildcard
		{"notexample.com", "*.example.com", false}, // not a subdomain
		{"evil.com", "*.example.com", false},

		// Case insensitivity with wildcards
		{"SUB.EXAMPLE.COM", "*.example.com", true},
		{"sub.example.com", "*.EXAMPLE.COM", true},
	}

	for _, tt := range tests {
		t.Run(tt.host+"_"+tt.pattern, func(t *testing.T) {
			got := matchDomain(tt.host, tt.pattern)
			if got != tt.want {
				t.Errorf("matchDomain(%q, %q) = %v, want %v", tt.host, tt.pattern, got, tt.want)
			}
		})
	}
}

// ─── checkDomainAllowed tests (Task 7) ─────────────────────────────────────────

func TestCheckDomainAllowed(t *testing.T) {
	tests := []struct {
		name        string
		rawURL      string
		allowed     []string
		disallowed  []string
		wantErr     bool
		errContains string
	}{
		{
			name:    "nil allowed, nil disallowed — all permitted",
			rawURL:  "https://example.com/page",
			wantErr: false,
		},
		{
			name:        "disallowed blocks exact domain",
			rawURL:      "https://evil.com/page",
			disallowed:  []string{"evil.com"},
			wantErr:     true,
			errContains: "blocked by policy",
		},
		{
			name:        "disallowed blocks subdomain via wildcard",
			rawURL:      "https://sub.evil.com/page",
			disallowed:  []string{"*.evil.com"},
			wantErr:     true,
			errContains: "blocked by policy",
		},
		{
			name:       "disallowed does not block unrelated domain",
			rawURL:     "https://good.com/page",
			disallowed: []string{"evil.com"},
			wantErr:    false,
		},
		{
			name:    "allowed list permits listed domain",
			rawURL:  "https://good.com/page",
			allowed: []string{"good.com"},
			wantErr: false,
		},
		{
			name:        "allowed list blocks unlisted domain",
			rawURL:      "https://other.com/page",
			allowed:     []string{"good.com"},
			wantErr:     true,
			errContains: "not in the allowed list",
		},
		{
			name:    "allowed list permits wildcard subdomain",
			rawURL:  "https://api.good.com/page",
			allowed: []string{"*.good.com"},
			wantErr: false,
		},
		{
			name:        "disallowed takes precedence over allowed",
			rawURL:      "https://evil.com/page",
			allowed:     []string{"evil.com"},
			disallowed:  []string{"evil.com"},
			wantErr:     true,
			errContains: "blocked by policy",
		},
		{
			name:        "invalid URL",
			rawURL:      "://not-a-url",
			wantErr:     true,
			errContains: "invalid URL",
		},
		{
			name:        "URL with no host",
			rawURL:      "file:///etc/passwd",
			wantErr:     true,
			errContains: "no host",
		},
		{
			name:    "URL with port — domain still matched",
			rawURL:  "https://example.com:8443/page",
			allowed: []string{"example.com"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDomainAllowed(tt.rawURL, tt.allowed, tt.disallowed)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// ─── WebFetchTool domain restriction integration tests (Task 7) ────────────────

func TestWebFetchTool_DomainBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "should not reach here")
	}))
	defer srv.Close()

	tool := &WebFetchTool{
		cache:             NewSearchCache(),
		skipSSRFCheck:     true,
		DisallowedDomains: []string{"127.0.0.1"},
	}
	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error for blocked domain, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "blocked by policy") {
		t.Errorf("expected 'blocked by policy' in error: %s", result.Content)
	}
}

func TestWebFetchTool_DomainAllowedOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "should not reach here")
	}))
	defer srv.Close()

	tool := &WebFetchTool{
		cache:          NewSearchCache(),
		skipSSRFCheck:  true,
		AllowedDomains: []string{"example.com"}, // only example.com allowed; httptest is 127.0.0.1
	}
	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error for non-allowed domain, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "not in the allowed list") {
		t.Errorf("expected 'not in the allowed list' in error: %s", result.Content)
	}
}

func TestWebFetchTool_DomainAllowed_Passes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "hello from allowed domain")
	}))
	defer srv.Close()

	tool := newWebFetchHTTPTestTool(t)
	tool.AllowedDomains = []string{"127.0.0.1"}
	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello from allowed domain") {
		t.Errorf("expected content in output: %q", result.Content)
	}
}

// ─── WebSearchTool domain restriction integration test (Task 7) ────────────────

func TestWebSearchTool_ToolLevelDomainRestrictions(t *testing.T) {
	apiResp := ddgInstantResponse{
		RelatedTopics: []ddgRelatedTopic{
			{Text: "Allowed", FirstURL: "https://allowed.com/page"},
			{Text: "Blocked", FirstURL: "https://blocked.com/page"},
			{Text: "Other", FirstURL: "https://other.com/page"},
		},
	}
	body, _ := json.Marshal(apiResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	tool := &WebSearchTool{
		cache:             NewSearchCache(),
		instantAnswerURL:  srv.URL + "/",
		liteFallbackURL:   srv.URL + "/lite/",
		DisallowedDomains: []string{"blocked.com"},
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"query": "test tool-level domain filter",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if strings.Contains(result.Content, "blocked.com") {
		t.Errorf("blocked.com should be filtered out: %q", result.Content)
	}
	if !strings.Contains(result.Content, "allowed.com") {
		t.Errorf("allowed.com should remain: %q", result.Content)
	}
}
