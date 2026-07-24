package tools

import (
	"reflect"
	"testing"
)

func TestNormalizeWebFetchResult_Error(t *testing.T) {
	got := normalizeWebFetchResult("https://example.com", "summarize", "", false, false, errString("boom"))
	if got.Error != "boom" {
		t.Fatalf("expected error boom, got %q", got.Error)
	}
	if got.Execution.Method != "local_fallback" {
		t.Fatalf("expected local_fallback method, got %q", got.Execution.Method)
	}
}

func TestNormalizeWebFetchResult_Success(t *testing.T) {
	got := normalizeWebFetchResult("https://example.com", "find title", "body text", true, true, nil)
	if got.Input.URL != "https://example.com" || got.Input.Prompt != "find title" {
		t.Fatalf("unexpected input: %+v", got.Input)
	}
	if got.Execution.ResolvedURL != "https://example.com" {
		t.Fatalf("expected resolved url to mirror input, got %q", got.Execution.ResolvedURL)
	}
	if !got.Execution.Truncated || !got.Execution.CacheHit {
		t.Fatalf("expected truncated and cacheHit to be true: %+v", got.Execution)
	}
	if got.Content.Body != "body text" {
		t.Fatalf("unexpected body: %q", got.Content.Body)
	}
}

func TestNormalizeWebFetchResult_ReplayFixture(t *testing.T) {
	cases := []string{
		"basic_success.json",
		"same_url_prompt_a.json",
		"same_url_prompt_b.json",
		"redirect_success.json",
		"redirect_blocked.json",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			c := loadWebFetchReplayCase(t, replayFixturePath("webfetch", "replay", name))
			got := normalizeWebFetchResult(c.Input.URL, c.Input.Prompt, c.Input.Body, c.Input.Truncated, c.Input.CacheHit, errorFromString(c.Input.Error))
			if !reflect.DeepEqual(got, c.Expected) {
				t.Fatalf("normalized webfetch mismatch\n got: %+v\nwant: %+v", got, c.Expected)
			}
		})
	}
}

func TestNormalizeWebSearchResult_Error(t *testing.T) {
	got := normalizeWebSearchResult("golang", []string{"golang.org"}, nil, nil, true, "provider_failed", errString("search failed"))
	if got.Error != "search failed" {
		t.Fatalf("expected search failed error, got %q", got.Error)
	}
	if got.Execution.FallbackReason != "provider_failed" {
		t.Fatalf("unexpected fallback reason: %q", got.Execution.FallbackReason)
	}
	if !got.Execution.CacheHit {
		t.Fatalf("expected cacheHit true")
	}
}

func TestNormalizeWebSearchResult_Success(t *testing.T) {
	results := []searchResult{{Title: "Go", URL: "https://go.dev", Snippet: "The Go Programming Language"}}
	got := normalizeWebSearchResult("golang", []string{"go.dev"}, []string{"example.com"}, results, false, "", nil)

	if got.Input.Query != "golang" {
		t.Fatalf("unexpected query: %q", got.Input.Query)
	}
	if !reflect.DeepEqual(got.Input.AllowedDomains, []string{"go.dev"}) {
		t.Fatalf("unexpected allowed domains: %+v", got.Input.AllowedDomains)
	}
	if !reflect.DeepEqual(got.Input.BlockedDomains, []string{"example.com"}) {
		t.Fatalf("unexpected blocked domains: %+v", got.Input.BlockedDomains)
	}
	if got.Execution.Method != "local_fallback" {
		t.Fatalf("unexpected method: %q", got.Execution.Method)
	}
	if len(got.Progress) != 3 || got.Progress[0].Type != "started" || got.Progress[1].Type != "query_issued" || got.Progress[2].Type != "results_received" {
		t.Fatalf("unexpected progress events: %+v", got.Progress)
	}
	if len(got.Results) != 1 || got.Results[0].URL != "https://go.dev" {
		t.Fatalf("unexpected results: %+v", got.Results)
	}
}

func TestNormalizeWebSearchResult_ReplayFixture(t *testing.T) {
	cases := []string{
		"basic_success.json",
		"allowed_blocked_conflict.json",
		"fallback_success.json",
		"same_query_allowed_a.json",
		"same_query_allowed_b.json",
		"same_query_blocked_a.json",
		"same_query_blocked_b.json",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			c := loadWebSearchReplayCase(t, replayFixturePath("websearch", "replay", name))
			got := normalizeWebSearchResult(c.Input.Query, c.Input.AllowedDomains, c.Input.BlockedDomains, c.Input.Results, c.Input.CacheHit, c.Input.FallbackReason, errorFromString(c.Input.Error))
			if !reflect.DeepEqual(got, c.Expected) {
				t.Fatalf("normalized websearch mismatch\n got: %+v\nwant: %+v", got, c.Expected)
			}
		})
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestWebSearchCacheKeyIncludesDomainFilters(t *testing.T) {
	a := makeWebSearchCacheKey("context cancellation", []string{"go.dev"}, nil)
	b := makeWebSearchCacheKey("context cancellation", []string{"golang.org"}, nil)
	c := makeWebSearchCacheKey("context cancellation", nil, []string{"pkg.go.dev"})
	d := makeWebSearchCacheKey("context cancellation", nil, []string{"example.com"})
	if a == b {
		t.Fatalf("expected different cache keys for different allowed domains: %q", a)
	}
	if c == d {
		t.Fatalf("expected different cache keys for different blocked domains: %q", c)
	}
}

func errorFromString(s string) error {
	if s == "" {
		return nil
	}
	return errString(s)
}
