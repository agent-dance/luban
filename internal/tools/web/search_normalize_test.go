package web

import (
	"strings"
	"testing"
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
