package tools

import (
	"strings"
	"testing"
	"time"
)

func TestCapSnippet_BelowCap(t *testing.T) {
	in := "short snippet"
	if got := capSnippet(in); got != in {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

func TestCapSnippet_AtCap(t *testing.T) {
	in := strings.Repeat("a", WebSearchSnippetCap)
	if got := capSnippet(in); got != in {
		t.Fatalf("at-cap snippet should be unchanged")
	}
}

func TestCapSnippet_TruncatesAndAppendsEllipsis(t *testing.T) {
	in := strings.Repeat("a", WebSearchSnippetCap+50)
	got := capSnippet(in)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", got[len(got)-5:])
	}
	// Rune length must be <= cap+1 (cap chars + ellipsis rune).
	if r := []rune(got); len(r) > WebSearchSnippetCap+1 {
		t.Fatalf("snippet too long after cap: rune len=%d", len(r))
	}
}

func TestCapSnippet_BreaksOnWordBoundary(t *testing.T) {
	// Build a string where there's a space within the lookback window so
	// truncation happens at the boundary.
	prefix := strings.Repeat("aa ", WebSearchSnippetCap/3)
	in := prefix + strings.Repeat("z", 100)
	got := capSnippet(in)
	body := strings.TrimSuffix(got, "…")
	if strings.HasSuffix(body, "z") {
		// A word-boundary cut means we shouldn't have ended in the middle of
		// the trailing "z" run.
		t.Fatalf("expected word-boundary cut, got %q", got)
	}
}

func TestCapSnippet_UnicodeSafe(t *testing.T) {
	// Each "你" rune is 3 bytes in UTF-8; ensure we slice by rune.
	in := strings.Repeat("你", WebSearchSnippetCap+10)
	got := capSnippet(in)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis, got %q", got[len(got)-3:])
	}
	if !startsWithRune(got, '你') {
		t.Fatalf("first rune should still be 你, got %q", got)
	}
}

func startsWithRune(s string, r rune) bool {
	for _, c := range s {
		return c == r
	}
	return false
}

func TestNormalizeWebSearchResults_DropsEmptyURLs(t *testing.T) {
	in := []WebSearchResult{
		{Title: "good", URL: "https://example.com", Snippet: "ok"},
		{Title: "missing url", URL: "  ", Snippet: "should drop"},
		{Title: "another", URL: "https://example.org"},
	}
	out := NormalizeWebSearchResults(in)
	if out.Type != "web_search_tool_result" {
		t.Fatalf("type mismatch: %s", out.Type)
	}
	if len(out.Content) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(out.Content), out.Content)
	}
}

func TestNormalizeWebSearchResults_TitleFallsBackToURL(t *testing.T) {
	in := []WebSearchResult{
		{Title: "  ", URL: "https://example.com/path"},
	}
	out := NormalizeWebSearchResults(in)
	if len(out.Content) != 1 || out.Content[0].Title != "https://example.com/path" {
		t.Fatalf("expected title fallback, got %+v", out.Content)
	}
}

func TestNormalizeWebSearchResults_SnippetCapped(t *testing.T) {
	long := strings.Repeat("x", WebSearchSnippetCap+200)
	in := []WebSearchResult{
		{Title: "t", URL: "https://example.com", Snippet: long},
	}
	out := NormalizeWebSearchResults(in)
	if got := []rune(out.Content[0].Snippet); len(got) > WebSearchSnippetCap+1 {
		t.Fatalf("snippet should have been capped, got rune len=%d", len(got))
	}
}

func TestSearchResultsToWebSearchResults_PopulatesMetadata(t *testing.T) {
	now := time.Now().UTC()
	in := []searchResult{
		{Title: "t", URL: "https://example.com", Snippet: "s"},
	}
	out := SearchResultsToWebSearchResults(in, "ddg", now)
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	if out[0].ProviderID != "ddg" || !out[0].FetchedAt.Equal(now) {
		t.Fatalf("metadata not propagated: %+v", out[0])
	}
}

func TestSearchResultsToWebSearchResults_TrimsAndCaps(t *testing.T) {
	long := strings.Repeat("y", WebSearchSnippetCap+50)
	in := []searchResult{
		{Title: "  spaced  ", URL: "  https://example.com  ", Snippet: long},
	}
	out := SearchResultsToWebSearchResults(in, "ddg", time.Time{})
	if out[0].Title != "spaced" {
		t.Fatalf("expected trimmed title, got %q", out[0].Title)
	}
	if out[0].URL != "https://example.com" {
		t.Fatalf("expected trimmed URL, got %q", out[0].URL)
	}
	if r := []rune(out[0].Snippet); len(r) > WebSearchSnippetCap+1 {
		t.Fatalf("expected capped snippet, got len=%d", len(r))
	}
}
