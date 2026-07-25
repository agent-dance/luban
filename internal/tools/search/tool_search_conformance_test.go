package search

import (
	"strings"
	"testing"
	"time"
)

// ─── BM25 ranker ──────────────────────────────────────────────────────────────

func TestBM25Ranker_RanksByRelevance(t *testing.T) {
	entries := []toolEntry{
		{Name: "WebFetch", Description: "fetch a URL and process its contents"},
		{Name: "Read", Description: "read a local file from disk"},
		{Name: "Grep", Description: "search file contents using regex"},
	}
	r := newBM25Ranker(entries)
	matches := r.rank("fetch url contents", 3)
	if len(matches) == 0 {
		t.Fatal("expected matches for 'fetch url contents'")
	}
	if matches[0].Name != "WebFetch" {
		t.Errorf("expected WebFetch top match, got %q (full=%v)", matches[0].Name, matches)
	}
}

func TestBM25Ranker_TokeniseStripsStopwords(t *testing.T) {
	tokens := tokeniseForBM25("The quick brown fox jumps over the lazy dog")
	for _, tok := range tokens {
		if tok == "the" || tok == "over" {
			continue
		}
	}
	got := strings.Join(tokens, ",")
	if strings.Contains(got, "the") {
		t.Errorf("expected stopword 'the' filtered, got tokens=%v", tokens)
	}
}

func TestBM25Ranker_RespectsTopK(t *testing.T) {
	entries := []toolEntry{
		{Name: "AlphaTool", Description: "alpha"},
		{Name: "BetaTool", Description: "alpha beta"},
		{Name: "GammaTool", Description: "alpha gamma delta"},
		{Name: "DeltaTool", Description: "alpha delta epsilon"},
	}
	r := newBM25Ranker(entries)
	matches := r.rank("alpha", 2)
	if len(matches) != 2 {
		t.Errorf("expected 2 results, got %d", len(matches))
	}
}

func TestBM25Ranker_DescendingScores(t *testing.T) {
	entries := []toolEntry{
		{Name: "A", Description: "fetch fetch fetch web url content"},
		{Name: "B", Description: "fetch web"},
		{Name: "C", Description: "unrelated tool"},
	}
	r := newBM25Ranker(entries)
	matches := r.rank("fetch web url", 5)
	for i := 1; i < len(matches); i++ {
		if matches[i].Score > matches[i-1].Score {
			t.Errorf("scores not descending: %v", matches)
			break
		}
	}
}

// ─── Cache ────────────────────────────────────────────────────────────────────

func TestToolSearchCache_HitAndMiss(t *testing.T) {
	c := newToolSearchCache(8, 5*time.Minute)
	key := toolSearchCacheKey{query: "abc", registry: "h1", limit: 5}
	if _, ok := c.get(key); ok {
		t.Fatal("expected miss")
	}
	c.set(key, []scoredMatch{{Name: "X", Score: 1}})
	got, ok := c.get(key)
	if !ok || len(got) != 1 || got[0].Name != "X" {
		t.Fatalf("expected hit, got ok=%v matches=%v", ok, got)
	}
}

func TestToolSearchCache_TTLExpires(t *testing.T) {
	c := newToolSearchCache(8, 50*time.Millisecond)
	key := toolSearchCacheKey{query: "q", registry: "h"}
	c.set(key, []scoredMatch{{Name: "X"}})
	time.Sleep(60 * time.Millisecond)
	if _, ok := c.get(key); ok {
		t.Fatal("expected expired entry to miss")
	}
}

func TestToolSearchCache_RegistryHashMismatchEvicts(t *testing.T) {
	c := newToolSearchCache(8, 5*time.Minute)
	c.set(toolSearchCacheKey{query: "q", registry: "h1"}, []scoredMatch{{Name: "X"}})
	if _, ok := c.get(toolSearchCacheKey{query: "q", registry: "h2"}); ok {
		t.Fatal("expected mismatch hash to miss")
	}
}

func TestToolSearchCache_InvalidateClearsStale(t *testing.T) {
	c := newToolSearchCache(8, 5*time.Minute)
	c.set(toolSearchCacheKey{query: "q1", registry: "old"}, []scoredMatch{{Name: "A"}})
	c.set(toolSearchCacheKey{query: "q2", registry: "current"}, []scoredMatch{{Name: "B"}})
	c.invalidate("current")
	if cacheLength(c) != 1 {
		t.Fatalf("expected 1 entry retained, got %d", cacheLength(c))
	}
}

func TestToolSearchCache_LRUEvictsOldest(t *testing.T) {
	c := newToolSearchCache(2, 5*time.Minute)
	c.set(toolSearchCacheKey{query: "q1", registry: "h"}, []scoredMatch{{Name: "A"}})
	c.set(toolSearchCacheKey{query: "q2", registry: "h"}, []scoredMatch{{Name: "B"}})
	c.set(toolSearchCacheKey{query: "q3", registry: "h"}, []scoredMatch{{Name: "C"}})
	if cacheLength(c) != 2 {
		t.Fatalf("expected LRU cap=2, got %d", cacheLength(c))
	}
	if _, ok := c.get(toolSearchCacheKey{query: "q1", registry: "h"}); ok {
		t.Error("expected q1 to have been evicted (LRU)")
	}
}

func cacheLength(cache *toolSearchCache) int {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return cache.order.Len()
}
