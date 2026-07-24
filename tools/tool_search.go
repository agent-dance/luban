package tools

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// PendingMCPServersFn returns the names of MCP clients still in a 'pending'
// connection state. ToolSearchTool calls this on empty results so the model
// is told the tool may show up shortly. TS-02: registries that don't track
// MCP state can leave this nil and the result is silently omitted.
var PendingMCPServersFn func() []string

// MCPServerVisibilityState is the ToolSearch-safe subset of MCP connection
// health. It intentionally uses strings so callers can bridge from either the
// legacy tools manager or the services/mcp manager without creating imports.
type MCPServerVisibilityState struct {
	Name                 string
	State                string
	ReconnectAttempt     int
	MaxReconnectAttempts int
	Error                string
}

// MCPServerStatesFn returns non-connected MCP state that should be visible
// when ToolSearch finds no current tool. Pending servers explain temporary
// absence; failed/needs-auth servers explain why the model should not retry
// blindly.
var MCPServerStatesFn func() []MCPServerVisibilityState

// SupportsToolReferenceBlocksFn reports whether the active provider supports
// the tool_reference content block type. TS-03: Bedrock and Vertex deployments
// reject the unrecognised block type, so when the resolver returns false we
// emit a plain-text fallback that lists the loaded tool names instead.
// Defaulting to nil preserves the prior behaviour (always emit reference
// blocks) for runtimes that haven't wired up provider classification.
var SupportsToolReferenceBlocksFn func() bool

var (
	toolSearchSelectPattern = regexp.MustCompile(`(?i)^select:(.+)$`)
	toolSearchCamelPattern  = regexp.MustCompile(`([a-z])([A-Z])`)
)

// SetSupportsToolReferenceBlocksFn registers the provider-mode guard used by
// TS-03. Pass nil to clear (defaults to "supported").
func SetSupportsToolReferenceBlocksFn(fn func() bool) {
	SupportsToolReferenceBlocksFn = fn
}

func toolReferenceBlocksSupported() bool {
	if SupportsToolReferenceBlocksFn == nil {
		return true
	}
	return SupportsToolReferenceBlocksFn()
}

// ToolSearchOutcomeFn receives a structured analytics event for every
// ToolSearch invocation. TS-07: TS emits tengu_tool_search_outcome with
// queryType (select|keyword|embed), matchCount, totalDeferredTools, and
// hasMatches so dashboards can detect regressions when descriptions change.
// Defaulting to nil disables emission.
var ToolSearchOutcomeFn func(event ToolSearchOutcomeEvent)

// ToolSearchOutcomeEvent is the structured analytics payload for TS-07.
type ToolSearchOutcomeEvent struct {
	QueryType           string
	MatchCount          int
	TotalDeferredTools  int
	HasMatches          bool
	UsedEmbeddingRerank bool
}

// SetToolSearchOutcomeFn registers the analytics emitter. Pass nil to clear.
func SetToolSearchOutcomeFn(fn func(event ToolSearchOutcomeEvent)) {
	ToolSearchOutcomeFn = fn
}

func emitToolSearchOutcome(queryType string, matchCount, totalDeferred int, useEmbed bool) {
	if ToolSearchOutcomeFn == nil {
		return
	}
	ToolSearchOutcomeFn(ToolSearchOutcomeEvent{
		QueryType:           queryType,
		MatchCount:          matchCount,
		TotalDeferredTools:  totalDeferred,
		HasMatches:          matchCount > 0,
		UsedEmbeddingRerank: useEmbed,
	})
}

// SetPendingMCPServersFn registers the resolver used by TS-02 to surface MCP
// clients still finishing their initial handshake. Pass nil to clear.
func SetPendingMCPServersFn(fn func() []string) {
	PendingMCPServersFn = fn
}

// SetMCPServerStatesFn registers the richer task_14 resolver used to surface
// pending/failed/needs-auth MCP server state in ToolSearch misses.
func SetMCPServerStatesFn(fn func() []MCPServerVisibilityState) {
	MCPServerStatesFn = fn
}

func currentPendingMCPServers() []string {
	if PendingMCPServersFn == nil {
		return nil
	}
	out := PendingMCPServersFn()
	if len(out) == 0 {
		return nil
	}
	cleaned := make([]string, 0, len(out))
	seen := map[string]struct{}{}
	for _, name := range out {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		cleaned = append(cleaned, name)
	}
	return cleaned
}

func currentMCPServerVisibilityStates() []MCPServerVisibilityState {
	seen := map[string]struct{}{}
	out := make([]MCPServerVisibilityState, 0)
	if MCPServerStatesFn != nil {
		for _, state := range MCPServerStatesFn() {
			state.Name = strings.TrimSpace(state.Name)
			state.State = strings.TrimSpace(state.State)
			if state.Name == "" || state.State == "" {
				continue
			}
			key := strings.ToLower(state.Name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, state)
		}
	}
	for _, name := range currentPendingMCPServers() {
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, MCPServerVisibilityState{Name: name, State: "pending"})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// ToolSearchTool discovers deferred tools and returns structured tool
// references so the loop can expose those tools on subsequent turns.
//
// Execution wires through:
//
//	registry → ToolIndex (hash) → BM25Ranker → ScoredMatch list →
//	  optional embedding rerank → ToolSearchCache → ToolReferenceBlock output
//
// Each step has its own helper file in this package; ToolSearchTool stitches
// them together and is the only entry point from the loop.
type ToolSearchTool struct {
	Registry *registry.Registry

	mu       sync.Mutex
	index    *ToolIndex
	ranker   *BM25Ranker
	cache    *ToolSearchCache
	embedder *ToolSearchEmbedder
}

func (t *ToolSearchTool) Name() string { return "ToolSearch" }

func (t *ToolSearchTool) Description() string {
	return `Fetch full schema definitions for deferred tools so they can be called.

Use "select:<tool_name>" for direct selection, or supply free-form keywords to
search. Keyword queries are scored with a BM25 ranker over each tool's name,
description, and search hint, and the top matches are returned with a
` + "`Snippet:`" + ` of the terms that matched. Each result also carries a numeric
rank score so the model can judge how strong the match is.`
}

func (t *ToolSearchTool) IsConcurrentSafe() bool { return true }

func (t *ToolSearchTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": `Query to find deferred tools. Use "select:<tool_name>" for direct selection, or keywords to search via BM25 ranking.`,
			},
			"max_results": map[string]any{
				"type":        "number",
				"description": "Maximum number of results to return (default: 5). Clamped to [1, 50].",
				"minimum":     1,
				"maximum":     50,
			},
		},
		"query",
	)
}

// Cache returns the result cache so callers can inspect or warm it. Lazily
// initialised on first access; concurrent-safe.
func (t *ToolSearchTool) Cache() *ToolSearchCache {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cache == nil {
		t.cache = NewToolSearchCache(0, 0)
	}
	return t.cache
}

// SetEmbedder plugs in an embedding-based reranker. Pass nil to clear.
func (t *ToolSearchTool) SetEmbedder(e *ToolSearchEmbedder) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.embedder = e
}

// WithRegistry returns a fresh search tool bound to a scoped registry. Search
// indexes and caches are intentionally not shared because their tool sets may
// differ; the optional embedder is safe to reuse.
func (t *ToolSearchTool) WithRegistry(reg *registry.Registry) *ToolSearchTool {
	if t == nil {
		return &ToolSearchTool{Registry: reg}
	}
	t.mu.Lock()
	embedder := t.embedder
	t.mu.Unlock()
	return &ToolSearchTool{Registry: reg, embedder: embedder}
}

// IndexHash returns the SHA-256 fingerprint of the most recent registry
// snapshot. Empty until the first Execute call lazily builds the index.
func (t *ToolSearchTool) IndexHash() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.index == nil {
		return ""
	}
	return t.index.Hash()
}

// Invalidate drops the cached index, ranker, and query results. MCP
// list_changed notifications call this after dynamic registry updates so the
// next search rebuilds from the fresh tool set immediately.
func (t *ToolSearchTool) Invalidate() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.index = nil
	t.ranker = nil
	if t.cache != nil {
		t.cache.Invalidate("")
	}
}

// refreshIndex rebuilds the ToolIndex + BM25 ranker when the registry has
// changed. Must be called with t.mu held.
func (t *ToolSearchTool) refreshIndexLocked() {
	if t.index == nil {
		t.index = NewToolIndex(t.Registry)
	} else {
		t.index.Rebuild()
	}
	deferredEntries := make([]ToolEntry, 0)
	for _, e := range t.index.Entries() {
		if e.IsDeferred {
			deferredEntries = append(deferredEntries, e)
		}
	}
	if t.ranker == nil {
		t.ranker = NewBM25Ranker(deferredEntries)
	} else {
		t.ranker.Rebuild(deferredEntries)
	}
}

func (t *ToolSearchTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := parseStrictInputOrError[ToolSearchInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}

	query := strings.TrimSpace(in.Query)
	if query == "" {
		return types.ToolResult{Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolLegacyDRequiredFieldQuoted, "query"), IsError: true}, nil
	}

	limit := in.MaxResults
	if limit <= 0 {
		limit = 5
	}
	if limit > 50 {
		limit = 50
	}
	if limit < 1 {
		limit = 1
	}

	deferredTools := t.Registry.DeferredTools()
	allTools := t.Registry.All()

	// Direct-select branch is keyword-free and bypasses ranking entirely.
	selectMatch := toolSearchSelectPattern.FindStringSubmatch(query)
	if len(selectMatch) == 2 {
		requested := splitCSV(selectMatch[1])
		found, missing := resolveSelectedTools(requested, deferredTools, allTools)
		emitToolSearchOutcome("select", len(found), len(deferredTools), false)
		return buildToolSearchResult(query, len(deferredTools), found, missing, nil, ""), nil
	}

	t.mu.Lock()
	t.refreshIndexLocked()
	indexHash := t.index.Hash()
	if t.cache == nil {
		t.cache = NewToolSearchCache(0, 0)
	}
	cache := t.cache
	ranker := t.ranker
	embedder := t.embedder
	t.mu.Unlock()

	// Drop any cache entries that no longer match the live registry hash.
	cache.Invalidate(indexHash)

	useEmbed := IsToolSearchEmbedEnabled()
	cacheKey := ToolSearchCacheKey{
		Query:    query,
		Registry: indexHash,
		Limit:    limit,
		UseEmbed: useEmbed,
	}

	matches, hit := cache.Get(cacheKey)
	if !hit {
		matches = ranker.Rank(query, limit)
		if useEmbed && embedder != nil {
			if err := embedder.PrepareCorpus(ctx, indexHash, t.index.Entries()); err == nil {
				if reranked, rerankErr := embedder.Rerank(ctx, query, matches); rerankErr == nil {
					matches = reranked
				}
			}
		}
		cache.Set(cacheKey, matches)
	}

	rerankTag := ""
	if useEmbed {
		rerankTag = "embed"
	}

	found := make([]string, 0, len(matches))
	for _, m := range matches {
		found = append(found, m.Name)
	}

	result := buildToolSearchResult(query, len(deferredTools), found, nil, matches, rerankTag)
	queryKind := "keyword"
	if useEmbed {
		queryKind = "embed"
	}
	emitToolSearchOutcome(queryKind, len(found), len(deferredTools), useEmbed)
	return result, nil
}

type ToolSearchInput struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

type parsedToolSearchName struct {
	parts []string
	full  string
	isMCP bool
}

func splitCSV(value string) []string {
	raw := strings.Split(value, ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func resolveSelectedTools(requested []string, deferredTools, allTools []types.Tool) ([]string, []string) {
	found := make([]string, 0, len(requested))
	missing := make([]string, 0)
	seen := make(map[string]struct{}, len(requested))

	for _, toolName := range requested {
		tool := findToolByName(deferredTools, toolName)
		if tool == nil {
			tool = findToolByName(allTools, toolName)
		}
		if tool == nil {
			missing = append(missing, toolName)
			continue
		}
		if _, ok := seen[tool.Name()]; ok {
			continue
		}
		seen[tool.Name()] = struct{}{}
		found = append(found, tool.Name())
	}

	return found, missing
}

// buildToolSearchResult assembles the user-facing ToolResult.
//
// scoredMatches carries per-result rank scores and snippets when the keyword
// branch fed us BM25 output; the select branch passes nil and the result
// renders without snippets/scores. rerankTag is set to "embed" when the
// caller wants to advertise that an embedding rerank was attempted.
func buildToolSearchResult(query string, totalDeferred int, found, missing []string, scoredMatches []ScoredMatch, rerankTag string) types.ToolResult {
	lang := i18n.DetectOrLoadLanguage()
	if len(found) == 0 {
		text := i18n.Format(lang, i18n.KeyToolLegacyDToolSearchNoMatches, query, totalDeferred)
		if len(missing) > 0 {
			text = i18n.Format(lang, i18n.KeyToolLegacyDToolSearchRequestedMissing, query, strings.Join(missing, ", "), totalDeferred)
		}
		// Surface MCP clients whose tools may be temporarily unavailable.
		// Pending servers are retryable; failed/needs-auth servers explain
		// why waiting will not help without /mcp or auth intervention.
		mcpStates := currentMCPServerVisibilityStates()
		if len(mcpStates) > 0 {
			text += " " + formatMCPServerStateHint(lang, mcpStates)
		}
		res := types.ToolResult{Content: text}
		meta := map[string]string{}
		if rerankTag != "" {
			meta["rerank"] = rerankTag
		}
		mcpMeta := mcpServerStateMetadata(mcpStates)
		for key, value := range mcpMeta {
			meta[key] = value
		}
		if len(meta) > 0 {
			res.Metadata = meta
		}
		return res
	}

	blocks := make([]types.ContentBlock, 0, len(found))
	if toolReferenceBlocksSupported() {
		for _, name := range found {
			blocks = append(blocks, types.ToolReferenceBlock{
				Type:     types.ContentTypeToolReference,
				ToolName: name,
			})
		}
	}
	// TS-03: when the active provider does not support tool_reference blocks
	// (Bedrock / Vertex), the textual content is the only delivery channel.
	// We still emit the human-readable summary below so the model can resolve
	// names from text alone.

	var sb strings.Builder
	if len(missing) > 0 {
		sb.WriteString(i18n.Format(lang, i18n.KeyToolLegacyDToolSearchLoadedWithMissing,
			len(found), strings.Join(found, ", "), strings.Join(missing, ", "), totalDeferred))
	} else {
		sb.WriteString(i18n.Format(lang, i18n.KeyToolLegacyDToolSearchLoadedForQuery,
			len(found), query, strings.Join(found, ", "), totalDeferred))
	}

	// Surface BM25 snippets and scores in the textual content so the model
	// can use them as ranking justification. Audit P0-2 row.
	scoreParts := make([]string, 0, len(scoredMatches))
	matchByName := make(map[string]ScoredMatch, len(scoredMatches))
	for _, m := range scoredMatches {
		matchByName[m.Name] = m
	}
	for _, name := range found {
		m, ok := matchByName[name]
		if !ok {
			continue
		}
		sb.WriteString(i18n.Format(lang, i18n.KeyToolLegacyDToolSearchScore, m.Name, m.Score))
		if m.Snippet != "" {
			sb.WriteString(i18n.Format(lang, i18n.KeyToolLegacyDToolSearchSnippet, m.Snippet))
		}
		scoreParts = append(scoreParts, fmt.Sprintf("%s=%s", m.Name, strconv.FormatFloat(m.Score, 'f', 4, 64)))
	}

	res := types.ToolResult{
		Content:       sb.String(),
		ContentBlocks: blocks,
	}
	if len(scoreParts) > 0 || rerankTag != "" {
		res.Metadata = map[string]string{}
	}
	if len(scoreParts) > 0 {
		res.Metadata["scores"] = strings.Join(scoreParts, ",")
	}
	if rerankTag != "" {
		res.Metadata["rerank"] = rerankTag
	}
	return res
}

func formatMCPServerStateHint(lang i18n.Language, states []MCPServerVisibilityState) string {
	if len(states) == 0 {
		return ""
	}
	partsByState := map[string][]string{}
	order := []string{"pending", "failed", "needs-auth", "disabled"}
	for _, state := range states {
		label := state.Name
		if state.State == "pending" && state.MaxReconnectAttempts > 0 && state.ReconnectAttempt > 0 {
			label = i18n.Format(lang, i18n.KeyToolLegacyDMCPReconnect, state.Name, state.ReconnectAttempt, state.MaxReconnectAttempts)
		}
		if state.State == "failed" && state.Error != "" {
			label = fmt.Sprintf("%s (%s)", state.Name, state.Error)
		}
		partsByState[state.State] = append(partsByState[state.State], label)
	}
	parts := make([]string, 0, len(partsByState))
	for _, state := range order {
		values := partsByState[state]
		if len(values) == 0 {
			continue
		}
		parts = append(parts, i18n.Format(lang, i18n.KeyToolLegacyDMCPStateEntry, localizedMCPState(lang, state), strings.Join(values, ", ")))
	}
	for state, values := range partsByState {
		if containsString(order, state) || len(values) == 0 {
			continue
		}
		parts = append(parts, i18n.Format(lang, i18n.KeyToolLegacyDMCPStateEntry, state, strings.Join(values, ", ")))
	}
	prefix := ""
	if pending := partsByState["pending"]; len(pending) > 0 {
		prefix = i18n.Format(lang, i18n.KeyToolLegacyDMCPPendingServers, strings.Join(pending, ", "))
	}
	return prefix + i18n.Format(lang, i18n.KeyToolLegacyDMCPServerStates, strings.Join(parts, "; "))
}

func localizedMCPState(lang i18n.Language, state string) string {
	switch state {
	case "pending":
		return i18n.Text(lang, i18n.KeyToolLegacyDMCPStatePending)
	case "failed":
		return i18n.Text(lang, i18n.KeyToolLegacyDMCPStateFailed)
	case "needs-auth":
		return i18n.Text(lang, i18n.KeyToolLegacyDMCPStateNeedsAuth)
	case "disabled":
		return i18n.Text(lang, i18n.KeyToolLegacyDMCPStateDisabled)
	default:
		return state
	}
}

func mcpServerStateMetadata(states []MCPServerVisibilityState) map[string]string {
	meta := map[string]string{}
	if len(states) == 0 {
		return meta
	}
	byState := map[string][]string{}
	encoded := make([]string, 0, len(states))
	for _, state := range states {
		byState[state.State] = append(byState[state.State], state.Name)
		encoded = append(encoded, state.State+":"+state.Name)
	}
	if pending := byState["pending"]; len(pending) > 0 {
		meta["pending_mcp_servers"] = strings.Join(pending, ",")
	}
	if failed := byState["failed"]; len(failed) > 0 {
		meta["failed_mcp_servers"] = strings.Join(failed, ",")
	}
	if needsAuth := byState["needs-auth"]; len(needsAuth) > 0 {
		meta["needs_auth_mcp_servers"] = strings.Join(needsAuth, ",")
	}
	meta["mcp_server_states"] = strings.Join(encoded, ",")
	return meta
}

func findToolByName(tools []types.Tool, name string) types.Tool {
	query := strings.ToLower(strings.TrimSpace(name))
	for _, tool := range tools {
		if strings.EqualFold(tool.Name(), query) {
			return tool
		}
	}
	return nil
}

func parseToolSearchName(name string) parsedToolSearchName {
	if strings.HasPrefix(name, "mcp__") {
		withoutPrefix := strings.TrimPrefix(strings.ToLower(name), "mcp__")
		parts := strings.Split(strings.ReplaceAll(withoutPrefix, "__", "_"), "_")
		filtered := make([]string, 0, len(parts))
		for _, part := range parts {
			if part != "" {
				filtered = append(filtered, part)
			}
		}
		return parsedToolSearchName{
			parts: filtered,
			full:  strings.ReplaceAll(strings.ReplaceAll(withoutPrefix, "__", " "), "_", " "),
			isMCP: true,
		}
	}

	camelSplit := toolSearchCamelPattern.ReplaceAllString(name, `$1 $2`)
	parts := strings.Fields(strings.ToLower(strings.ReplaceAll(camelSplit, "_", " ")))
	return parsedToolSearchName{
		parts: parts,
		full:  strings.Join(parts, " "),
	}
}

// searchToolsWithKeywords is preserved for legacy callers and the older
// keyword-only conformance tests. The Execute path now routes through the
// BM25 ranker (see refreshIndexLocked + ToolSearchTool.Execute).
func searchToolsWithKeywords(query string, deferredTools, allTools []types.Tool, maxResults int) []string {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		return nil
	}

	if exact := findToolByName(deferredTools, queryLower); exact != nil {
		return []string{exact.Name()}
	}
	if exact := findToolByName(allTools, queryLower); exact != nil {
		return []string{exact.Name()}
	}

	if strings.HasPrefix(queryLower, "mcp__") && len(queryLower) > len("mcp__") {
		prefixMatches := make([]string, 0, maxResults)
		for _, tool := range deferredTools {
			if strings.HasPrefix(strings.ToLower(tool.Name()), queryLower) {
				prefixMatches = append(prefixMatches, tool.Name())
				if len(prefixMatches) >= maxResults {
					break
				}
			}
		}
		if len(prefixMatches) > 0 {
			return prefixMatches
		}
	}

	queryTerms := strings.Fields(queryLower)
	requiredTerms := make([]string, 0, len(queryTerms))
	optionalTerms := make([]string, 0, len(queryTerms))
	for _, term := range queryTerms {
		if strings.HasPrefix(term, "+") && len(term) > 1 {
			requiredTerms = append(requiredTerms, strings.TrimPrefix(term, "+"))
		} else {
			optionalTerms = append(optionalTerms, term)
		}
	}

	allTerms := queryTerms
	if len(requiredTerms) > 0 {
		allTerms = append(append([]string{}, requiredTerms...), optionalTerms...)
	}
	termPatterns := compileTermPatterns(allTerms)

	candidateTools := deferredTools
	if len(requiredTerms) > 0 {
		filtered := make([]types.Tool, 0, len(deferredTools))
		for _, tool := range deferredTools {
			parsed := parseToolSearchName(tool.Name())
			descNormalized := strings.ToLower(tool.Description())
			hintNormalized := strings.ToLower(registry.DiscoveryMetadata(tool).SearchHint)
			matchesAll := true
			for _, term := range requiredTerms {
				pattern := termPatterns[term]
				if containsSearchTerm(parsed, descNormalized, hintNormalized, term, pattern) {
					continue
				}
				matchesAll = false
				break
			}
			if matchesAll {
				filtered = append(filtered, tool)
			}
		}
		candidateTools = filtered
	}

	type scoredTool struct {
		name  string
		score int
	}
	scored := make([]scoredTool, 0, len(candidateTools))
	for _, tool := range candidateTools {
		parsed := parseToolSearchName(tool.Name())
		descNormalized := strings.ToLower(tool.Description())
		hintNormalized := strings.ToLower(registry.DiscoveryMetadata(tool).SearchHint)
		score := 0

		for _, term := range allTerms {
			pattern := termPatterns[term]
			termScore := 0
			if containsExactPart(parsed.parts, term) {
				if parsed.isMCP {
					termScore += 12
				} else {
					termScore += 10
				}
			} else if containsPartialPart(parsed.parts, term) {
				if parsed.isMCP {
					termScore += 6
				} else {
					termScore += 5
				}
			}
			if termScore == 0 && strings.Contains(parsed.full, term) {
				termScore += 3
			}
			if hintNormalized != "" && pattern.MatchString(hintNormalized) {
				termScore += 4
			}
			if pattern.MatchString(descNormalized) {
				termScore += 2
			}
			score += termScore
		}

		if score > 0 {
			scored = append(scored, scoredTool{name: tool.Name(), score: score})
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].name < scored[j].name
		}
		return scored[i].score > scored[j].score
	})

	if len(scored) > maxResults {
		scored = scored[:maxResults]
	}

	names := make([]string, 0, len(scored))
	for _, item := range scored {
		names = append(names, item.name)
	}
	return names
}

func compileTermPatterns(terms []string) map[string]*regexp.Regexp {
	patterns := make(map[string]*regexp.Regexp, len(terms))
	for _, term := range terms {
		if _, ok := patterns[term]; ok {
			continue
		}
		patterns[term] = regexp.MustCompile(`\b` + regexp.QuoteMeta(term) + `\b`)
	}
	return patterns
}

func containsExactPart(parts []string, term string) bool {
	for _, part := range parts {
		if part == term {
			return true
		}
	}
	return false
}

func containsPartialPart(parts []string, term string) bool {
	for _, part := range parts {
		if strings.Contains(part, term) {
			return true
		}
	}
	return false
}

func containsSearchTerm(parsed parsedToolSearchName, description, hint, term string, pattern *regexp.Regexp) bool {
	return containsExactPart(parsed.parts, term) ||
		containsPartialPart(parsed.parts, term) ||
		pattern.MatchString(description) ||
		(hint != "" && pattern.MatchString(hint))
}
