package tools

// Alignment red tests for the ToolSearchTool, derived from alignment_audit.md
// (P0-2 ToolSearch 主流程未接入: BM25Ranker / ToolSearchCache / embedding
// reranker exist but Execute uses the simpler keyword scorer instead).
//
// Each test pins the *desired* contract from the TS reference
// (ToolSearchTool.tsx + readOnlyValidation/embedding hooks) and is expected
// to FAIL against the current Go implementation while still compiling
// cleanly.
//
// Do NOT modify production code to silence them without first reviewing the
// corresponding audit row.
//
// Run only these tests with:
//
//	go test -run ToolSearchAlignment -count=1 ./gosrc/tools/...

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// toolSearchKeyword is a tiny in-test stand-in for a deferred tool. It opts
// into the deferred discovery pool so ToolSearch will consider it during
// scoring, and exposes a SearchHint so we can exercise BM25 multi-field
// scoring.
type toolSearchKeyword struct {
	name string
	desc string
	hint string
}

func (s toolSearchKeyword) Name() string             { return s.name }
func (s toolSearchKeyword) Description() string      { return s.desc }
func (s toolSearchKeyword) Schema() types.JSONSchema { return types.JSONSchema{Type: "object"} }
func (s toolSearchKeyword) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: "ok"}, nil
}
func (s toolSearchKeyword) ToolDiscoveryMetadata() registry.ToolDiscoveryMetadata {
	return registry.ToolDiscoveryMetadata{ShouldDefer: true, SearchHint: s.hint}
}

func newToolSearchAlignmentRegistry(extra ...types.Tool) *registry.Registry {
	reg := registry.New()
	// Always register ToolSearch itself so the deferred pool is non-trivial.
	reg.Register(&ToolSearchTool{Registry: reg})
	for _, t := range extra {
		reg.Register(t)
	}
	return reg
}

// -------- task35: schema field name is `max_results` --------
//
// The task35 contract exposes max_results as the model-facing parameter. The
// older limit spelling must not be advertised as the canonical input.
func TestToolSearchAlignment_Schema_FieldIsMaxResultsNotLimit(t *testing.T) {
	tool := &ToolSearchTool{Registry: newToolSearchAlignmentRegistry()}
	schema := tool.Schema()
	if _, ok := schema.Properties["max_results"]; !ok {
		t.Errorf("schema must expose `max_results`; got keys %v", schemaKeys(schema))
	}
	if _, ok := schema.Properties["limit"]; ok {
		t.Errorf("schema must not declare `limit` as a canonical field")
	}
}

// -------- task35: max_results field has numeric bounds [1,50] --------
//
// The implementation clamps the requested result count to [1,50], and the
// schema advertises the same range on max_results.
func TestToolSearchAlignment_Schema_MaxResultsDeclaresMinAndMax(t *testing.T) {
	tool := &ToolSearchTool{Registry: newToolSearchAlignmentRegistry()}
	schema := tool.Schema()
	prop, ok := schema.Properties["max_results"].(map[string]any)
	if !ok {
		t.Fatalf("schema is missing `max_results` property; cannot assert bounds")
	}
	if _, ok := prop["minimum"]; !ok {
		t.Errorf("max_results property must declare `minimum`")
	}
	if _, ok := prop["maximum"]; !ok {
		t.Errorf("max_results property must declare `maximum`")
	}
}

func TestToolSearchAlignment_Execute_RejectsLegacyLimitField(t *testing.T) {
	tool := &ToolSearchTool{Registry: newToolSearchAlignmentRegistry()}
	result, err := tool.Execute(context.Background(), map[string]any{
		"query": "read",
		"limit": 1,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "unknown field") {
		t.Fatalf("expected strict unknown-field error for legacy limit, got %#v", result)
	}
}

// -------- audit P0-2: Execute must rank via BM25, not the keyword scorer --------
//
// We register two deferred tools whose names contain the query token but at
// different positions / token-density. BM25 ranks the tool with higher tf-idf
// for the query token first, while the current keyword scorer treats them as
// tied (both contain the literal substring once) and falls back to alphabetic
// order. We exercise this by giving them distinct token frequencies via
// SearchHint.
//
// Concretely, "EditFile" has the term "edit" once (in name); "Editor" has
// "edit" twice (name + hint), so BM25 must rank "Editor" first. The current
// keyword scorer instead returns "EditFile" first (alphabetic tiebreak).
func TestToolSearchAlignment_Execute_RanksByBM25NotKeywordTie(t *testing.T) {
	reg := newToolSearchAlignmentRegistry(
		toolSearchKeyword{name: "EditFile", desc: "rewrite file contents", hint: "rewrite"},
		toolSearchKeyword{name: "Editor", desc: "edit text", hint: "edit edit edit"},
	)
	tool := &ToolSearchTool{Registry: reg}
	res, err := tool.Execute(context.Background(), map[string]any{"query": "edit"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.ContentBlocks) < 2 {
		t.Fatalf("expected >=2 content blocks for ranking, got %d (%q)", len(res.ContentBlocks), res.Content)
	}
	first, ok := res.ContentBlocks[0].(types.ToolReferenceBlock)
	if !ok {
		t.Fatalf("first block must be a ToolReferenceBlock; got %T", res.ContentBlocks[0])
	}
	if first.ToolName != "Editor" {
		t.Errorf("BM25 must rank `Editor` (3 hits) before `EditFile` (1 hit); got first=%q", first.ToolName)
	}
}

// -------- audit P0-2: Execute exposes ranker scores in metadata --------
//
// Today Execute emits only ToolReferenceBlock entries; the audit asks for
// ScoredMatch-style output (name + score + snippet). We surface this by
// asserting Metadata["scores"] holds a comma-separated list of name=score
// pairs. Any encoding is acceptable; we just need the score signal to leak.
func TestToolSearchAlignment_Execute_OutputAdvertisesScores(t *testing.T) {
	reg := newToolSearchAlignmentRegistry(
		toolSearchKeyword{name: "EditFile", desc: "rewrite file contents"},
	)
	tool := &ToolSearchTool{Registry: reg}
	res, err := tool.Execute(context.Background(), map[string]any{"query": "edit file"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Metadata == nil {
		t.Fatalf("metadata missing")
	}
	if _, ok := res.Metadata["scores"]; !ok {
		t.Errorf("Metadata must include `scores` for ranker observability (audit P0-2); got %v", res.Metadata)
	}
}

// -------- audit P0-2: Execute populates per-match snippets --------
//
// ScoredMatch carries a Snippet field that captures the matched terms. The
// audit asks the Execute path to surface that snippet so the model can use
// it as a justification for the ranking. We accept either a metadata key or
// per-block Snippet/Reason field; the minimum signal is that the rendered
// content includes a `Snippet:` or `matched:` token, which the current
// Execute does not produce.
func TestToolSearchAlignment_Execute_OutputCarriesSnippet(t *testing.T) {
	reg := newToolSearchAlignmentRegistry(
		toolSearchKeyword{name: "Editor", desc: "edit text", hint: "edit edit"},
	)
	tool := &ToolSearchTool{Registry: reg}
	res, err := tool.Execute(context.Background(), map[string]any{"query": "edit"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	combined := strings.ToLower(res.Content)
	for _, block := range res.ContentBlocks {
		if ref, ok := block.(types.ToolReferenceBlock); ok {
			combined += " " + strings.ToLower(ref.ToolName)
		}
	}
	if !strings.Contains(combined, "snippet") && !strings.Contains(combined, "matched") {
		t.Errorf("Execute output must surface ranker snippets (audit P0-2); got content=%q", res.Content)
	}
}

// -------- audit P0-2: ToolSearchTool exposes its result cache --------
//
// The cache implementation lives in tool_search_cache.go but Execute does
// not consult it. The first observable contract is a public Cache() accessor
// on ToolSearchTool so callers can inspect / pre-populate it. Today no such
// method exists.
func TestToolSearchAlignment_Tool_HasCacheAccessor(t *testing.T) {
	type cacheHolder interface {
		Cache() *ToolSearchCache
	}
	tool := &ToolSearchTool{Registry: newToolSearchAlignmentRegistry()}
	if _, ok := any(tool).(cacheHolder); !ok {
		t.Errorf("ToolSearchTool must expose Cache() to wire tool_search_cache.go into Execute (audit P0-2)")
	}
}

// -------- audit P0-2: Execute warms the cache on first call --------
//
// We rely on the public Cache() accessor (see previous test) — when it
// exists, after one Execute the cache must hold at least one entry.
func TestToolSearchAlignment_Execute_PopulatesCacheOnFirstCall(t *testing.T) {
	type cacheHolder interface {
		Cache() *ToolSearchCache
	}
	tool := &ToolSearchTool{Registry: newToolSearchAlignmentRegistry(
		toolSearchKeyword{name: "Editor", desc: "edit text"},
	)}
	holder, ok := any(tool).(cacheHolder)
	if !ok {
		t.Skip("ToolSearchTool does not yet expose Cache(); covered by another red test")
	}
	cache := holder.Cache()
	if cache == nil {
		t.Fatalf("Cache() returned nil")
	}
	before := cache.Len()
	if _, err := tool.Execute(context.Background(), map[string]any{"query": "edit"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if cache.Len() <= before {
		t.Errorf("Execute must populate the result cache (audit P0-2); len went from %d -> %d", before, cache.Len())
	}
}

// -------- audit P0-2: registering a new tool invalidates the cache --------
//
// The cache is keyed by registryHash; mutating the registry must invalidate
// stale entries. We pre-populate the cache, then register a new tool and
// expect the entry count to drop on the next Execute call.
func TestToolSearchAlignment_Execute_CacheInvalidatedOnRegistryChange(t *testing.T) {
	type cacheHolder interface {
		Cache() *ToolSearchCache
	}
	reg := newToolSearchAlignmentRegistry(
		toolSearchKeyword{name: "Editor", desc: "edit text"},
	)
	tool := &ToolSearchTool{Registry: reg}
	holder, ok := any(tool).(cacheHolder)
	if !ok {
		t.Skip("ToolSearchTool does not yet expose Cache(); covered by another red test")
	}
	cache := holder.Cache()
	if cache == nil {
		t.Fatalf("Cache() returned nil")
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"query": "edit"}); err != nil {
		t.Fatalf("execute1: %v", err)
	}
	primed := cache.Len()
	if primed == 0 {
		t.Skip("cache was not primed by Execute; covered by separate red test")
	}

	reg.Register(toolSearchKeyword{name: "EditFile", desc: "rewrite contents"})
	if _, err := tool.Execute(context.Background(), map[string]any{"query": "edit"}); err != nil {
		t.Fatalf("execute2: %v", err)
	}

	// We expect the stale entry to have been evicted (and a new one written),
	// so the cache must contain exactly one fresh entry — never two parallel
	// hashes for the same query.
	if cache.Len() > primed {
		t.Errorf("Cache must invalidate stale entries on registry change; primed=%d, post-change=%d", primed, cache.Len())
	}
}

// -------- audit P0-2: Embedder hook is settable on the tool --------
//
// tool_search_embedding.go provides a full embedder + reranker but the
// Execute path has no setter to plug it in. This test pins the desired
// contract: ToolSearchTool exposes SetEmbedder().
func TestToolSearchAlignment_Tool_HasEmbedderHook(t *testing.T) {
	type embedHolder interface {
		SetEmbedder(*ToolSearchEmbedder)
	}
	tool := &ToolSearchTool{Registry: newToolSearchAlignmentRegistry()}
	if _, ok := any(tool).(embedHolder); !ok {
		t.Errorf("ToolSearchTool must accept an embedder via SetEmbedder() so Execute can rerank (audit P0-2)")
	}
}

// -------- audit P0-2: Embedding feature flag drives the rerank step --------
//
// When TENGU_TOOL_SEARCH_EMBED=1 AND an embedder is wired, Execute must run
// the rerank step. We surface the contract by asserting the result metadata
// announces `rerank=embed` when the flag is on. The current implementation
// never sets such a key.
func TestToolSearchAlignment_Execute_AnnouncesEmbedRerankWhenFlagOn(t *testing.T) {
	t.Setenv(FeatureFlagToolSearchEmbed, "1")
	tool := &ToolSearchTool{Registry: newToolSearchAlignmentRegistry(
		toolSearchKeyword{name: "Editor", desc: "edit text"},
	)}
	res, err := tool.Execute(context.Background(), map[string]any{"query": "edit"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := res.Metadata["rerank"]
	if got != "embed" {
		t.Errorf("Metadata[\"rerank\"] must be \"embed\" when %s=1 (audit P0-2); got %q",
			FeatureFlagToolSearchEmbed, got)
	}
}

// -------- audit P0-2: index hash is observable from the tool --------
//
// The audit calls out that index rebuilds must invalidate the cache via the
// hash. We assert ToolSearchTool exposes IndexHash() so callers can detect
// when a rebuild has happened.
func TestToolSearchAlignment_Tool_HasIndexHashAccessor(t *testing.T) {
	type hashHolder interface {
		IndexHash() string
	}
	tool := &ToolSearchTool{Registry: newToolSearchAlignmentRegistry()}
	if _, ok := any(tool).(hashHolder); !ok {
		t.Errorf("ToolSearchTool must expose IndexHash() so callers can detect index rebuilds (audit P0-2)")
	}
}

// -------- audit P0-2: registering a new tool changes the observable hash --------
func TestToolSearchAlignment_Execute_IndexHashChangesAfterRegistryMutation(t *testing.T) {
	type hashHolder interface {
		IndexHash() string
	}
	reg := newToolSearchAlignmentRegistry(
		toolSearchKeyword{name: "Editor", desc: "edit text"},
	)
	tool := &ToolSearchTool{Registry: reg}
	holder, ok := any(tool).(hashHolder)
	if !ok {
		t.Skip("ToolSearchTool does not yet expose IndexHash(); covered by another red test")
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"query": "edit"}); err != nil {
		t.Fatalf("execute1: %v", err)
	}
	before := holder.IndexHash()
	if before == "" {
		t.Fatalf("IndexHash() must be non-empty after Execute")
	}
	reg.Register(toolSearchKeyword{name: "EditFile", desc: "rewrite"})
	if _, err := tool.Execute(context.Background(), map[string]any{"query": "edit"}); err != nil {
		t.Fatalf("execute2: %v", err)
	}
	after := holder.IndexHash()
	if after == before {
		t.Errorf("IndexHash() must change when the registry gains a new tool; got %q both before and after", after)
	}
}

// -------- audit P0-2: description advertises ranker semantics --------
//
// The audit asks that the model-facing description explain *how* ToolSearch
// scores results so the model knows it returns BM25-ranked matches with a
// snippet. The current description only mentions "search" / "select".
func TestToolSearchAlignment_Description_MentionsRankerSemantics(t *testing.T) {
	tool := &ToolSearchTool{Registry: newToolSearchAlignmentRegistry()}
	desc := strings.ToLower(tool.Description())
	if !strings.Contains(desc, "rank") && !strings.Contains(desc, "score") && !strings.Contains(desc, "bm25") {
		t.Errorf("ToolSearch description should mention rank/score/bm25 semantics (audit P0-2); got %q", desc)
	}
}

// -------- audit P0-2: Schema description for `query` mentions select syntax --------
//
// Sanity lock: `query` description must teach the model the `select:<name>`
// branch. This passes today but pins the contract while we migrate to
// limit-aware ranking.
func TestToolSearchAlignment_Schema_QueryDescriptionDocumentsSelectSyntax(t *testing.T) {
	tool := &ToolSearchTool{Registry: newToolSearchAlignmentRegistry()}
	schema := tool.Schema()
	prop, ok := schema.Properties["query"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing query property")
	}
	desc, _ := prop["description"].(string)
	if !strings.Contains(strings.ToLower(desc), "select:") {
		t.Errorf("query description should document the select:<name> branch; got %q", desc)
	}
}

// guarded usage assertion to ensure imports are used even if some tests
// short-circuit via t.Errorf.
var _ types.Tool = (*ToolSearchTool)(nil)
