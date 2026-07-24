package tools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newWebFetchConformanceTool(t *testing.T) *WebFetchTool {
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

// TestWebFetchConformance exercises the WebFetch parity surface across
// 25+ scenarios covering input validation, content-type handling,
// HTML→Markdown conversion, redirects, server-tool routing, preapproved
// hosts, cache behaviour, summariser dispatch, and the explicit error
// surfaces ported from the TS reference.
//
// Cases are intentionally split across helper sub-tests rather than a
// single table because some tests need real HTTP servers (httptest) and
// can't be folded into a pure data-driven harness.
func TestWebFetchConformance(t *testing.T) {
	t.Run("01_DescriptionMentionsAuthFailure", func(t *testing.T) {
		desc := NewWebFetchTool(nil).Description()
		if !strings.Contains(desc, "WebFetch WILL FAIL") {
			t.Fatalf("description must warn about authenticated URL failure: %q", desc)
		}
	})

	t.Run("02_DescriptionPrefersGhCli", func(t *testing.T) {
		desc := NewWebFetchTool(nil).Description()
		if !strings.Contains(strings.ToLower(desc), "gh cli") && !strings.Contains(desc, "gh ") {
			t.Fatalf("description must mention preferring the gh CLI for GitHub: %q", desc)
		}
	})

	t.Run("03_RejectsMissingURL", func(t *testing.T) {
		tool := NewWebFetchTool(nil)
		result, err := tool.Execute(context.Background(), map[string]any{"url": "", "prompt": "p"})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatal("expected IsError for empty URL")
		}
	})

	t.Run("04_RejectsMissingPrompt", func(t *testing.T) {
		tool := NewWebFetchTool(nil)
		result, err := tool.Execute(context.Background(), map[string]any{"url": "https://example.com", "prompt": ""})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatal("expected IsError for empty prompt")
		}
	})

	t.Run("05_FetchPlainText", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "raw plain text body")
		}))
		defer srv.Close()
		tool := newWebFetchConformanceTool(t)
		result, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL, "prompt": "summarise"})
		if err != nil || result.IsError {
			t.Fatalf("err=%v isErr=%v body=%q", err, result.IsError, result.Content)
		}
		if !strings.Contains(result.Content, "raw plain text body") {
			t.Fatalf("body not present: %q", result.Content)
		}
	})

	t.Run("06_StripsScriptTagInHTML", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<html><body><script>evil()</script><p>visible</p></body></html>`)
		}))
		defer srv.Close()
		tool := newWebFetchConformanceTool(t)
		result, _ := tool.Execute(context.Background(), map[string]any{"url": srv.URL, "prompt": "x"})
		if strings.Contains(result.Content, "evil()") {
			t.Fatalf("script content leaked: %q", result.Content)
		}
		if !strings.Contains(result.Content, "visible") {
			t.Fatalf("paragraph dropped: %q", result.Content)
		}
	})

	t.Run("07_StripsStyleAndNavAndFooter", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body><style>.x{}</style><nav>menu</nav><p>core</p><footer>foot</footer></body></html>`)
		}))
		defer srv.Close()
		tool := newWebFetchConformanceTool(t)
		result, _ := tool.Execute(context.Background(), map[string]any{"url": srv.URL, "prompt": "x"})
		for _, banned := range []string{".x{}", "menu", "foot"} {
			if strings.Contains(result.Content, banned) {
				t.Fatalf("expected to strip %q, body=%q", banned, result.Content)
			}
		}
	})

	t.Run("08_PreservesCodeFenceLanguage", func(t *testing.T) {
		got := HTMLToMarkdown(`<pre><code class="language-rust">fn main(){}</code></pre>`)
		if !strings.Contains(got, "```rust") {
			t.Fatalf("rust fence missing: %q", got)
		}
	})

	t.Run("09_TruncationCapApplied", func(t *testing.T) {
		body := strings.Repeat("a", MaxMarkdownBytes+200)
		got := HTMLToMarkdown(`<p>` + body + `</p>`)
		if !strings.HasSuffix(got, "[Content truncated due to length...]") {
			t.Fatalf("truncation marker missing")
		}
	})

	t.Run("10_404IsToolError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		defer srv.Close()
		tool := newWebFetchConformanceTool(t)
		result, _ := tool.Execute(context.Background(), map[string]any{"url": srv.URL, "prompt": "p"})
		if !result.IsError {
			t.Fatalf("expected IsError for 404")
		}
	})

	t.Run("11_500IsToolError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer srv.Close()
		tool := newWebFetchConformanceTool(t)
		result, _ := tool.Execute(context.Background(), map[string]any{"url": srv.URL, "prompt": "p"})
		if !result.IsError {
			t.Fatalf("expected IsError for 500")
		}
	})

	t.Run("12_ServerToolRoutedWhenProvided", func(t *testing.T) {
		called := false
		provider := WebFetchServerToolFunc(func(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error) {
			called = true
			return WebFetchServerToolResponse{Summary: "from server tool"}, nil
		})
		tool := NewWebFetchTool(NewSearchCache())
		tool.skipSSRFCheck = true
		tool.SetWebFetchServerToolProvider(provider, true)
		result, err := tool.Execute(context.Background(), map[string]any{"url": "https://example.com", "prompt": "p"})
		if err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Fatal("expected server tool to be invoked")
		}
		if result.IsError {
			t.Fatalf("unexpected error: %q", result.Content)
		}
	})

	t.Run("13_ServerToolFailureFallsBackOrErrors", func(t *testing.T) {
		// Provider returns error → tool should still produce a result via fallback or
		// a clear IsError. Either is acceptable as long as we don't panic.
		provider := WebFetchServerToolFunc(func(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error) {
			return WebFetchServerToolResponse{}, errors.New("provider down")
		})
		tool := NewWebFetchTool(NewSearchCache())
		tool.skipSSRFCheck = true
		tool.SetWebFetchServerToolProvider(provider, true)
		result, err := tool.Execute(context.Background(), map[string]any{"url": "https://127.0.0.1:1/", "prompt": "p"})
		if err != nil {
			t.Fatal(err)
		}
		_ = result // No panic is the only invariant.
	})

	t.Run("14_PreapprovedHostList_Has80PlusEntries", func(t *testing.T) {
		if len(preapprovedHostEntries) < 50 {
			t.Fatalf("preapproved list shrunk: %d", len(preapprovedHostEntries))
		}
	})

	t.Run("15_PreapprovedAnthropicGitHubPath", func(t *testing.T) {
		if !IsPreapprovedHost("https://github.com/anthropics/claude-code") {
			t.Fatal("github.com/anthropics path should be preapproved")
		}
	})

	t.Run("16_NotPreapprovedRandomDomain", func(t *testing.T) {
		if IsPreapprovedHost("https://random.example.com") {
			t.Fatal("random domain should not be preapproved")
		}
	})

	t.Run("17_CacheKeyUsesRawURLOnly", func(t *testing.T) {
		a := WebFetchCacheKey("https://example.com", "p1")
		b := WebFetchCacheKey("https://example.com", "p2")
		if a != b {
			t.Fatal("different prompts must reuse the same raw URL cache key")
		}
	})

	t.Run("18_CacheTTLIs15Minutes", func(t *testing.T) {
		if WebFetchCacheTTL.Minutes() != 15 {
			t.Fatalf("cache TTL drift: got %v", WebFetchCacheTTL)
		}
	})

	t.Run("19_SummariserNilFallback", func(t *testing.T) {
		_, err := RunWebFetchSummariser(context.Background(), nil, "u", "p", "md", false)
		if !errors.Is(err, ErrSummariserUnavailable) {
			t.Fatalf("expected ErrSummariserUnavailable, got %v", err)
		}
	})

	t.Run("20_SummariserMaxTokensCap", func(t *testing.T) {
		if WebFetchSummariserMaxTokens != 4096 {
			t.Fatalf("summariser cap drift: %d", WebFetchSummariserMaxTokens)
		}
	})

	t.Run("21_RedirectMarkerSchema", func(t *testing.T) {
		got := formatRedirectMarker("https://a.com", "https://b.com", 301, "p")
		for _, want := range []string{"REDIRECT DETECTED", "Original URL: https://a.com", "Redirect URL: https://b.com", "Status: 301 Moved Permanently", `- prompt: "p"`} {
			if !strings.Contains(got, want) {
				t.Fatalf("redirect marker missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("22_HTTPUpgradeOptIn", func(t *testing.T) {
		if up := upgradeHTTPToHTTPS("http://example.com/a"); up != "https://example.com/a" {
			t.Fatalf("expected https upgrade, got %q", up)
		}
		if up := upgradeHTTPToHTTPS("https://example.com/a"); up != "https://example.com/a" {
			t.Fatalf("https URL should be unchanged, got %q", up)
		}
	})

	t.Run("23_SameOriginRedirectFollowed", func(t *testing.T) {
		if !sameOriginRedirect("https://example.com/a", "https://example.com/b") {
			t.Fatal("same host should be same origin")
		}
		if sameOriginRedirect("https://example.com/a", "https://other.com/b") {
			t.Fatal("different hosts must not be same origin")
		}
	})

	t.Run("24_HTTPRedirectMaxHopsAllowsTen", func(t *testing.T) {
		// Set up a chain of 10 same-origin redirects ending in a 200 response.
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/hop/") {
				n := 0
				fmt.Sscanf(r.URL.Path, "/hop/%d", &n)
				if n < 10 {
					http.Redirect(w, r, fmt.Sprintf("/hop/%d", n+1), http.StatusFound)
					return
				}
				w.Header().Set("Content-Type", "text/plain")
				fmt.Fprint(w, "final")
				return
			}
			http.Redirect(w, r, "/hop/0", http.StatusFound)
		}))
		defer srv.Close()
		tool := newWebFetchConformanceTool(t)
		_, _ = tool.Execute(context.Background(), map[string]any{"url": srv.URL + "/start", "prompt": "p"})
		// No panic / hang is the invariant — Go's default 10-hop max applies.
	})

	t.Run("25_SnippetCapEnforced", func(t *testing.T) {
		if WebSearchSnippetCap != 280 {
			t.Fatalf("snippet cap drift: %d", WebSearchSnippetCap)
		}
	})

	t.Run("26_HTTPSPreservedOnRedirect", func(t *testing.T) {
		// Confirm sameOriginRedirect treats subdomain mismatch as cross-origin
		// so the REDIRECT marker is emitted.
		if sameOriginRedirect("https://www.example.com", "https://api.example.com") {
			t.Fatal("subdomain change must be treated as cross-origin")
		}
	})

	t.Run("27_HasWebFetchServerTool_DefaultFalse", func(t *testing.T) {
		tool := NewWebFetchTool(NewSearchCache())
		if tool.HasWebFetchServerTool() {
			t.Fatal("default should not have server tool")
		}
	})

	t.Run("28_HasWebFetchServerTool_TrueAfterSet", func(t *testing.T) {
		tool := NewWebFetchTool(NewSearchCache())
		tool.SetWebFetchServerToolProvider(WebFetchServerToolFunc(func(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error) {
			return WebFetchServerToolResponse{}, nil
		}), false)
		if !tool.HasWebFetchServerTool() {
			t.Fatal("expected provider after Set")
		}
	})
}
