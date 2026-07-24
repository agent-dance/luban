package tools

import (
	"context"
	"errors"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// FeatureFlagToolSearchEmbed is the env var that toggles embedding-based
// reranking. Mirrors TS feature flag tengu_tool_search_embed.
const FeatureFlagToolSearchEmbed = "TENGU_TOOL_SEARCH_EMBED"

// IsToolSearchEmbedEnabled reports whether the embedding reranker should run.
// Falls back to the env var; deployment plumbing can override by patching the
// flag accessor directly.
var IsToolSearchEmbedEnabled = func() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(FeatureFlagToolSearchEmbed)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// EmbedFunc is the contract every embedding backend must satisfy. Returning
// (nil, err) lets callers fall back gracefully to the BM25 ranking.
type EmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)

// ToolSearchEmbedder caches per-document embeddings keyed by tool name +
// content hash so we only recompute when the corpus changes.
type ToolSearchEmbedder struct {
	mu      sync.RWMutex
	embed   EmbedFunc
	docHash string
	docVecs map[string][]float32
	docKeys []string
}

// NewToolSearchEmbedder wraps an embedding function. Pass nil to disable
// (RerankBM25 will then return the input unchanged).
func NewToolSearchEmbedder(embed EmbedFunc) *ToolSearchEmbedder {
	return &ToolSearchEmbedder{embed: embed, docVecs: map[string][]float32{}}
}

// Reset drops all cached document vectors. Tests use this between scenarios.
func (e *ToolSearchEmbedder) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.docHash = ""
	e.docVecs = map[string][]float32{}
	e.docKeys = nil
}

// PrepareCorpus embeds the supplied tool entries. If indexHash matches the
// previous build the cache is reused (no embed call).
func (e *ToolSearchEmbedder) PrepareCorpus(ctx context.Context, indexHash string, entries []ToolEntry) error {
	if e.embed == nil {
		return errors.New("no embedding backend configured")
	}
	e.mu.Lock()
	if indexHash != "" && indexHash == e.docHash {
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()

	texts := make([]string, len(entries))
	keys := make([]string, len(entries))
	for i, ent := range entries {
		texts[i] = strings.TrimSpace(ent.Name + ": " + ent.Description + " " + ent.SearchHint)
		keys[i] = ent.Name
	}
	// TS-05: bounded retry with exponential backoff for transient embed
	// failures (rate limits, network blips). Without retry, a single 429
	// purges the cache and forces a full re-embed on the next request,
	// hammering the API harder. We try up to 3 times with 200/400/800ms
	// backoff before surfacing the error to the caller (which already
	// falls back to BM25).
	vecs, err := embedWithRetry(ctx, e.embed, texts)
	if err != nil {
		return err
	}
	if len(vecs) != len(entries) {
		return errors.New("embedding response size mismatch")
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.docHash = indexHash
	e.docVecs = make(map[string][]float32, len(entries))
	for i, key := range keys {
		e.docVecs[key] = vecs[i]
	}
	e.docKeys = keys
	return nil
}

// Rerank reorders the supplied BM25 candidates by cosine similarity to the
// query embedding. On embedding errors it returns (candidates, err) — the
// caller is expected to fall back.
func (e *ToolSearchEmbedder) Rerank(ctx context.Context, query string, candidates []ScoredMatch) ([]ScoredMatch, error) {
	if e.embed == nil {
		return candidates, errors.New("no embedding backend configured")
	}
	if len(candidates) == 0 {
		return candidates, nil
	}

	queryVec, err := e.embedQuery(ctx, query)
	if err != nil {
		return candidates, err
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	type scored struct {
		match ScoredMatch
		sim   float64
	}
	scoredList := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		vec, ok := e.docVecs[c.Name]
		if !ok {
			// Fallback: keep original BM25 score, push to bottom.
			scoredList = append(scoredList, scored{match: c, sim: -math.MaxFloat64})
			continue
		}
		sim := cosineSimilarity(queryVec, vec)
		updated := c
		updated.Score = sim
		scoredList = append(scoredList, scored{match: updated, sim: sim})
	}
	sort.SliceStable(scoredList, func(i, j int) bool {
		if scoredList[i].sim == scoredList[j].sim {
			return scoredList[i].match.Name < scoredList[j].match.Name
		}
		return scoredList[i].sim > scoredList[j].sim
	})
	out := make([]ScoredMatch, 0, len(scoredList))
	for _, s := range scoredList {
		out = append(out, s.match)
	}
	return out, nil
}

func (e *ToolSearchEmbedder) embedQuery(ctx context.Context, query string) ([]float32, error) {
	vecs, err := embedWithRetry(ctx, e.embed, []string{query})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, errors.New("empty query embedding")
	}
	return vecs[0], nil
}

// cosineSimilarity returns the cosine similarity in [-1, 1]. Empty/zero vectors
// yield 0.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// embedWithRetry runs the embed function with bounded exponential backoff so
// transient rate-limit / network errors don't immediately fall back to BM25.
// Permanent shaped errors (validation, dimension mismatch) propagate without
// retry. TS-05: matches the audit recommendation of 3 attempts at
// 200/400/800ms.
func embedWithRetry(ctx context.Context, embed EmbedFunc, texts []string) ([][]float32, error) {
	if embed == nil {
		return nil, errors.New("no embedding backend configured")
	}
	const maxAttempts = 3
	backoff := 200 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		vecs, err := embed(ctx, texts)
		if err == nil {
			return vecs, nil
		}
		lastErr = err
		if !isEmbedRetryable(err) {
			return nil, err
		}
		if attempt == maxAttempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return nil, lastErr
}

func isEmbedRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "429"),
		strings.Contains(msg, "rate limit"),
		strings.Contains(msg, "rate-limit"),
		strings.Contains(msg, "timeout"),
		strings.Contains(msg, "deadline"),
		strings.Contains(msg, "temporarily"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "eof"):
		return true
	}
	return false
}
