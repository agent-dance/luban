package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

// ─── Index ────────────────────────────────────────────────────────────────────

func TestToolIndex_BuildAndLookup(t *testing.T) {
	tools := []toolEntryStub{
		{name: "Read", desc: "read files from disk"},
		{name: "Write", desc: "write files to disk"},
		{name: "Grep", desc: "search file contents"},
	}
	idx := IndexFromTools(stubsToTypedTools(tools), nil)
	if idx.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", idx.Len())
	}
	got, ok := idx.Lookup("Grep")
	if !ok {
		t.Fatal("expected Grep lookup to hit")
	}
	if got.Name != "Grep" {
		t.Errorf("expected Name=Grep, got %q", got.Name)
	}
	if idx.Hash() == "" {
		t.Error("expected non-empty hash")
	}
}

func TestToolIndex_HashChangesWithCorpus(t *testing.T) {
	a := IndexFromTools(stubsToTypedTools([]toolEntryStub{
		{name: "Read", desc: "read"},
	}), nil)
	b := IndexFromTools(stubsToTypedTools([]toolEntryStub{
		{name: "Read", desc: "read"},
		{name: "Write", desc: "write"},
	}), nil)
	if a.Hash() == b.Hash() {
		t.Fatal("expected different hashes for different corpora")
	}
}

func TestToolIndex_HashStableForSameCorpus(t *testing.T) {
	tools := []toolEntryStub{
		{name: "Read", desc: "read"},
		{name: "Write", desc: "write"},
	}
	a := IndexFromTools(stubsToTypedTools(tools), nil)
	b := IndexFromTools(stubsToTypedTools(tools), nil)
	if a.Hash() != b.Hash() {
		t.Fatalf("expected stable hash, got %s vs %s", a.Hash(), b.Hash())
	}
}

// ─── BM25 ranker ──────────────────────────────────────────────────────────────

func TestBM25Ranker_RanksByRelevance(t *testing.T) {
	entries := []ToolEntry{
		{Name: "WebFetch", Description: "fetch a URL and process its contents"},
		{Name: "Read", Description: "read a local file from disk"},
		{Name: "Grep", Description: "search file contents using regex"},
	}
	r := NewBM25Ranker(entries)
	matches := r.Rank("fetch url contents", 3)
	if len(matches) == 0 {
		t.Fatal("expected matches for 'fetch url contents'")
	}
	if matches[0].Name != "WebFetch" {
		t.Errorf("expected WebFetch top match, got %q (full=%v)", matches[0].Name, matches)
	}
}

func TestBM25Ranker_TokeniseStripsStopwords(t *testing.T) {
	tokens := TokeniseForBM25("The quick brown fox jumps over the lazy dog")
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
	entries := []ToolEntry{
		{Name: "AlphaTool", Description: "alpha"},
		{Name: "BetaTool", Description: "alpha beta"},
		{Name: "GammaTool", Description: "alpha gamma delta"},
		{Name: "DeltaTool", Description: "alpha delta epsilon"},
	}
	r := NewBM25Ranker(entries)
	matches := r.Rank("alpha", 2)
	if len(matches) != 2 {
		t.Errorf("expected 2 results, got %d", len(matches))
	}
}

func TestBM25Ranker_DescendingScores(t *testing.T) {
	entries := []ToolEntry{
		{Name: "A", Description: "fetch fetch fetch web url content"},
		{Name: "B", Description: "fetch web"},
		{Name: "C", Description: "unrelated tool"},
	}
	r := NewBM25Ranker(entries)
	matches := r.Rank("fetch web url", 5)
	for i := 1; i < len(matches); i++ {
		if matches[i].Score > matches[i-1].Score {
			t.Errorf("scores not descending: %v", matches)
			break
		}
	}
}

// ─── Cache ────────────────────────────────────────────────────────────────────

func TestToolSearchCache_HitAndMiss(t *testing.T) {
	c := NewToolSearchCache(8, 5*time.Minute)
	key := ToolSearchCacheKey{Query: "abc", Registry: "h1", Limit: 5}
	if _, ok := c.Get(key); ok {
		t.Fatal("expected miss")
	}
	c.Set(key, []ScoredMatch{{Name: "X", Score: 1}})
	got, ok := c.Get(key)
	if !ok || len(got) != 1 || got[0].Name != "X" {
		t.Fatalf("expected hit, got ok=%v matches=%v", ok, got)
	}
}

func TestToolSearchCache_TTLExpires(t *testing.T) {
	c := NewToolSearchCache(8, 50*time.Millisecond)
	key := ToolSearchCacheKey{Query: "q", Registry: "h"}
	c.Set(key, []ScoredMatch{{Name: "X"}})
	time.Sleep(60 * time.Millisecond)
	if _, ok := c.Get(key); ok {
		t.Fatal("expected expired entry to miss")
	}
}

func TestToolSearchCache_RegistryHashMismatchEvicts(t *testing.T) {
	c := NewToolSearchCache(8, 5*time.Minute)
	c.Set(ToolSearchCacheKey{Query: "q", Registry: "h1"}, []ScoredMatch{{Name: "X"}})
	if _, ok := c.Get(ToolSearchCacheKey{Query: "q", Registry: "h2"}); ok {
		t.Fatal("expected mismatch hash to miss")
	}
}

func TestToolSearchCache_InvalidateClearsStale(t *testing.T) {
	c := NewToolSearchCache(8, 5*time.Minute)
	c.Set(ToolSearchCacheKey{Query: "q1", Registry: "old"}, []ScoredMatch{{Name: "A"}})
	c.Set(ToolSearchCacheKey{Query: "q2", Registry: "current"}, []ScoredMatch{{Name: "B"}})
	c.Invalidate("current")
	if c.Len() != 1 {
		t.Fatalf("expected 1 entry retained, got %d", c.Len())
	}
}

func TestToolSearchCache_LRUEvictsOldest(t *testing.T) {
	c := NewToolSearchCache(2, 5*time.Minute)
	c.Set(ToolSearchCacheKey{Query: "q1", Registry: "h"}, []ScoredMatch{{Name: "A"}})
	c.Set(ToolSearchCacheKey{Query: "q2", Registry: "h"}, []ScoredMatch{{Name: "B"}})
	c.Set(ToolSearchCacheKey{Query: "q3", Registry: "h"}, []ScoredMatch{{Name: "C"}})
	if c.Len() != 2 {
		t.Fatalf("expected LRU cap=2, got %d", c.Len())
	}
	if _, ok := c.Get(ToolSearchCacheKey{Query: "q1", Registry: "h"}); ok {
		t.Error("expected q1 to have been evicted (LRU)")
	}
}

// ─── Embedding rerank ────────────────────────────────────────────────────────

func TestToolSearchEmbedder_RerankReordersByCosine(t *testing.T) {
	// Build a deterministic embedding stub that returns dense vectors keyed
	// off the *first character* of the input — query "F" gets close to the
	// document whose name starts with F.
	stub := func(_ context.Context, texts []string) ([][]float32, error) {
		out := make([][]float32, len(texts))
		for i, t := range texts {
			vec := make([]float32, 26)
			if len(t) > 0 {
				idx := int(strings.ToLower(t)[0:1][0]) - int('a')
				if idx >= 0 && idx < 26 {
					vec[idx] = 1
				}
			}
			out[i] = vec
		}
		return out, nil
	}

	entries := []ToolEntry{
		{Name: "fetch", Description: "fetch web url"},
		{Name: "read", Description: "read file"},
		{Name: "search", Description: "search code"},
	}
	e := NewToolSearchEmbedder(stub)
	if err := e.PrepareCorpus(context.Background(), "h1", entries); err != nil {
		t.Fatalf("PrepareCorpus: %v", err)
	}
	candidates := []ScoredMatch{
		{Name: "read", Score: 1},
		{Name: "fetch", Score: 0.5},
		{Name: "search", Score: 0.1},
	}
	got, err := e.Rerank(context.Background(), "fetching", candidates)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(got) == 0 || got[0].Name != "fetch" {
		t.Errorf("expected 'fetch' top after rerank, got %v", got)
	}
}

func TestToolSearchEmbedder_NoBackendReturnsErr(t *testing.T) {
	e := NewToolSearchEmbedder(nil)
	_, err := e.Rerank(context.Background(), "q", []ScoredMatch{{Name: "x"}})
	if err == nil {
		t.Fatal("expected error when no embedding backend configured")
	}
}

func TestToolSearchEmbedder_BackendErrorFallsThrough(t *testing.T) {
	stub := func(_ context.Context, _ []string) ([][]float32, error) {
		return nil, errors.New("upstream failure")
	}
	e := NewToolSearchEmbedder(stub)
	candidates := []ScoredMatch{{Name: "a", Score: 1}, {Name: "b", Score: 0.5}}
	got, err := e.Rerank(context.Background(), "q", candidates)
	if err == nil {
		t.Fatal("expected propagated error")
	}
	if len(got) != len(candidates) {
		t.Errorf("expected fallback candidates returned, got %v", got)
	}
}

func TestIsToolSearchEmbedEnabled_DefaultOff(t *testing.T) {
	t.Setenv(FeatureFlagToolSearchEmbed, "")
	if IsToolSearchEmbedEnabled() {
		t.Error("expected default off")
	}
	t.Setenv(FeatureFlagToolSearchEmbed, "1")
	if !IsToolSearchEmbedEnabled() {
		t.Error("expected enabled when flag=1")
	}
	t.Setenv(FeatureFlagToolSearchEmbed, "false")
	if IsToolSearchEmbedEnabled() {
		t.Error("expected disabled when flag=false")
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

type toolEntryStub struct {
	name string
	desc string
}

func (s toolEntryStub) Name() string             { return s.name }
func (s toolEntryStub) Description() string      { return s.desc }
func (s toolEntryStub) Schema() types.JSONSchema { return types.JSONSchema{Type: "object"} }
func (s toolEntryStub) Execute(_ context.Context, _ map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, nil
}

func stubsToTypedTools(stubs []toolEntryStub) []types.Tool {
	out := make([]types.Tool, 0, len(stubs))
	for _, s := range stubs {
		out = append(out, s)
	}
	return out
}
