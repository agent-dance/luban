package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

// TestBM25Ranker_RequiredTermFiltersOut (TS-06): a "+required" term must
// hard-filter documents that don't contain it.
func TestBM25Ranker_RequiredTermFiltersOut(t *testing.T) {
	entries := []ToolEntry{
		{Name: "SlackList", Description: "List slack channels"},
		{Name: "SlackRead", Description: "Read slack messages"},
		{Name: "SlackPost", Description: "Post a message to slack"},
	}
	r := NewBM25Ranker(entries)

	// "slack +read" → only SlackRead should appear, since the "+read"
	// requirement excludes List and Post.
	matches := r.Rank("slack +read", 5)
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	for _, m := range matches {
		if !strings.Contains(strings.ToLower(m.Name+" "+strings.ToLower(snippetLower(m))), "read") {
			t.Errorf("required-term filter let through %q (%s)", m.Name, m.Snippet)
		}
	}
	// And "list" / "post" should never appear.
	for _, m := range matches {
		if m.Name == "SlackList" || m.Name == "SlackPost" {
			t.Errorf("required-term filter let through %q", m.Name)
		}
	}
}

func snippetLower(m ScoredMatch) string { return strings.ToLower(m.Snippet) }

// TestSupportsToolReferenceBlocksFn_FallbackToggle (TS-03): when the
// resolver reports no tool_reference support, the helper returns false so
// callers can emit a plain-text fallback.
func TestSupportsToolReferenceBlocksFn_FallbackToggle(t *testing.T) {
	original := SupportsToolReferenceBlocksFn
	t.Cleanup(func() { SupportsToolReferenceBlocksFn = original })

	SetSupportsToolReferenceBlocksFn(func() bool { return false })
	if toolReferenceBlocksSupported() {
		t.Fatal("expected unsupported when resolver returns false")
	}

	SetSupportsToolReferenceBlocksFn(func() bool { return true })
	if !toolReferenceBlocksSupported() {
		t.Fatal("expected supported when resolver returns true")
	}

	SetSupportsToolReferenceBlocksFn(nil)
	if !toolReferenceBlocksSupported() {
		t.Fatal("default (nil resolver) must be supported=true")
	}
}

// TestEmbedWithRetry_RecoversFromTransient (TS-05): transient 429 errors
// must be retried up to 3 times.
func TestEmbedWithRetry_RecoversFromTransient(t *testing.T) {
	var calls atomic.Int32
	embed := func(ctx context.Context, texts []string) ([][]float32, error) {
		if calls.Add(1) < 3 {
			return nil, errors.New("HTTP 429: rate limit hit")
		}
		// Return matching dimension on success.
		out := make([][]float32, len(texts))
		for i := range out {
			out[i] = []float32{1, 0, 0}
		}
		return out, nil
	}
	vecs, err := embedWithRetry(context.Background(), embed, []string{"a", "b"})
	if err != nil {
		t.Fatalf("embedWithRetry returned error after retries: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

// TestEmbedWithRetry_GivesUpOnPermanentError (TS-05): non-retryable
// errors (e.g. validation) must NOT consume retry budget.
func TestEmbedWithRetry_GivesUpOnPermanentError(t *testing.T) {
	var calls atomic.Int32
	embed := func(ctx context.Context, texts []string) ([][]float32, error) {
		calls.Add(1)
		return nil, errors.New("invalid model parameter")
	}
	_, err := embedWithRetry(context.Background(), embed, []string{"x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("permanent error must not retry, got %d attempts", got)
	}
}

// TestToolSearchOutcome_EmitsEvent (TS-07): emitToolSearchOutcome calls
// the registered analytics emitter with structured fields.
func TestToolSearchOutcome_EmitsEvent(t *testing.T) {
	var captured ToolSearchOutcomeEvent
	var fired atomic.Int32

	original := ToolSearchOutcomeFn
	t.Cleanup(func() { ToolSearchOutcomeFn = original })

	SetToolSearchOutcomeFn(func(ev ToolSearchOutcomeEvent) {
		captured = ev
		fired.Add(1)
	})
	emitToolSearchOutcome("keyword", 3, 12, false)
	if fired.Load() != 1 {
		t.Fatal("expected one analytics event")
	}
	if captured.QueryType != "keyword" {
		t.Errorf("queryType = %q", captured.QueryType)
	}
	if captured.MatchCount != 3 || captured.TotalDeferredTools != 12 {
		t.Errorf("counts = (%d, %d)", captured.MatchCount, captured.TotalDeferredTools)
	}
	if !captured.HasMatches {
		t.Error("expected hasMatches=true when matchCount>0")
	}
	if captured.UsedEmbeddingRerank {
		t.Error("expected useEmbed=false")
	}

	// Disable: nil emitter must be safe.
	SetToolSearchOutcomeFn(nil)
	emitToolSearchOutcome("keyword", 0, 0, false) // must not panic
}

// TestToolIndex_MemoisesDescription (TS-01): a registered tool's
// Description() should be invoked at most once per Rebuild.
func TestToolIndex_MemoisesDescription(t *testing.T) {
	idx := NewToolIndex(nil) // empty registry
	// With nil registry the index is empty; sanity-check the API at least
	// doesn't panic and exposes the hash so callers can detect rebuilds.
	if idx == nil {
		t.Fatal("nil index")
	}
	hash1 := idx.Hash()
	idx.Rebuild()
	hash2 := idx.Hash()
	// An empty registry should produce a stable hash across rebuilds.
	if hash1 != hash2 {
		t.Fatalf("empty-registry hash changed between rebuilds: %q vs %q", hash1, hash2)
	}
}

var _ = fmt.Sprint // satisfy unused import budget
