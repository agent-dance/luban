package search

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// MCPServerVisibilityState is the ToolSearch-safe subset of MCP connection
// health. It intentionally uses strings to keep this package independent of
// MCP runtime internals.
type MCPServerVisibilityState struct {
	Name                 string
	State                string
	ReconnectAttempt     int
	MaxReconnectAttempts int
	Error                string
}

var (
	toolSearchSelectPattern = regexp.MustCompile(`(?i)^select:(.+)$`)
)

func currentMCPServerVisibilityStates(provider func() []MCPServerVisibilityState) []MCPServerVisibilityState {
	seen := map[string]struct{}{}
	out := make([]MCPServerVisibilityState, 0)
	if provider != nil {
		for _, state := range provider() {
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
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// toolSearchTool discovers deferred tools and returns structured tool
// references so the loop can expose those tools on subsequent turns.
//
// Execution wires through:
//
//	registry → toolIndex (hash) → bm25Ranker → scoredMatch list →
//	  toolSearchCache → ToolReferenceBlock output
//
// Each step has its own helper file in this package; toolSearchTool stitches
// them together and is the only entry point from the loop.
type toolSearchTool struct {
	registry  *registry.Registry
	mcpStates func() []MCPServerVisibilityState

	mu     sync.Mutex
	index  *toolIndex
	ranker *bm25Ranker
	cache  *toolSearchCache
}

// NewToolSearch creates a search tool bound to one registry and its MCP
// visibility projection. Neither dependency is shared through global state.
func NewToolSearch(reg *registry.Registry, mcpStates func() []MCPServerVisibilityState) *toolSearchTool {
	if reg == nil {
		reg = registry.New()
	}
	return &toolSearchTool{registry: reg, mcpStates: mcpStates}
}

func (t *toolSearchTool) Name() string { return "ToolSearch" }

func (t *toolSearchTool) Description() string {
	return toolPromptText(i18n.KeyToolSearchCatalogDescription)
}

func (t *toolSearchTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, Search: true, ConcurrencySafe: true}
}

func (t *toolSearchTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": toolPromptText(i18n.KeyToolSearchCatalogInputQueryDescription),
			},
			"max_results": map[string]any{
				"type":        "number",
				"description": toolPromptText(i18n.KeyToolSearchCatalogInputMaxResultsDescription),
				"minimum":     1,
				"maximum":     50,
			},
		},
		"query",
	)
}

// WithRegistry returns a fresh search tool bound to a scoped registry. Search
// indexes and caches are intentionally not shared because their tool sets may
// differ.
func (t *toolSearchTool) WithRegistry(reg *registry.Registry) types.Tool {
	return NewToolSearch(reg, t.mcpStates)
}

// Invalidate drops the cached index, ranker, and query results. MCP
// list_changed notifications call this after dynamic registry updates so the
// next search rebuilds from the fresh tool set immediately.
func (t *toolSearchTool) Invalidate() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.index = nil
	t.ranker = nil
	if t.cache != nil {
		t.cache.invalidate("")
	}
}

// refreshIndexLocked lazily builds the tool index and BM25 ranker. Registry
// mutation owners call Invalidate, so repeated queries reuse descriptions and
// tokenisation until an explicit catalog change occurs. Must hold t.mu.
func (t *toolSearchTool) refreshIndexLocked() {
	if t.index != nil && t.ranker != nil {
		return
	}
	t.index = newToolIndex(t.registry)
	deferredEntries := make([]toolEntry, 0)
	for _, e := range t.index.entriesSnapshot() {
		if e.IsDeferred {
			deferredEntries = append(deferredEntries, e)
		}
	}
	t.ranker = newBM25Ranker(deferredEntries)
}

func (t *toolSearchTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := toolbase.ParseStrictInputOrError[toolSearchInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}

	query := strings.TrimSpace(in.Query)
	if query == "" {
		return types.ToolResult{Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeRequiredFieldMissing, "query"), IsError: true}, nil
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

	reg := t.registry
	if reg == nil {
		reg = registry.New()
	}
	deferredTools := reg.DeferredTools()
	allTools := reg.All()

	// Direct-select branch is keyword-free and bypasses ranking entirely.
	selectMatch := toolSearchSelectPattern.FindStringSubmatch(query)
	if len(selectMatch) == 2 {
		requested := splitCSV(selectMatch[1])
		found, missing := resolveSelectedTools(requested, deferredTools, allTools)
		return buildToolSearchResult(query, len(deferredTools), found, missing, nil, t.mcpStates), nil
	}

	t.mu.Lock()
	t.refreshIndexLocked()
	indexHash := t.index.hashSnapshot()
	if t.cache == nil {
		t.cache = newToolSearchCache(0, 0)
	}
	cache := t.cache
	ranker := t.ranker
	t.mu.Unlock()

	// Drop any cache entries that no longer match the live registry hash.
	cache.invalidate(indexHash)

	cacheKey := toolSearchCacheKey{
		query:    query,
		registry: indexHash,
		limit:    limit,
	}

	matches, hit := cache.get(cacheKey)
	if !hit {
		matches = ranker.rank(query, limit)
		cache.set(cacheKey, matches)
	}

	found := make([]string, 0, len(matches))
	for _, m := range matches {
		found = append(found, m.Name)
	}

	result := buildToolSearchResult(query, len(deferredTools), found, nil, matches, t.mcpStates)
	return result, nil
}

type toolSearchInput struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
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
// renders without snippets/scores.
func buildToolSearchResult(query string, totalDeferred int, found, missing []string, scoredMatches []scoredMatch, mcpStateProvider func() []MCPServerVisibilityState) types.ToolResult {
	lang := i18n.DetectOrLoadLanguage()
	if len(found) == 0 {
		text := i18n.Format(lang, i18n.KeyToolSearchCatalogNoMatches, query, totalDeferred)
		if len(missing) > 0 {
			text = i18n.Format(lang, i18n.KeyToolSearchCatalogRequestedToolsMissing, query, strings.Join(missing, ", "), totalDeferred)
		}
		// Surface MCP clients whose tools may be temporarily unavailable.
		// Pending servers are retryable; failed/needs-auth servers explain
		// why waiting will not help without /mcp or auth intervention.
		mcpStates := currentMCPServerVisibilityStates(mcpStateProvider)
		if len(mcpStates) > 0 {
			text += " " + formatMCPServerStateHint(lang, mcpStates)
		}
		res := types.ToolResult{Content: text}
		meta := map[string]string{}
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
	for _, name := range found {
		blocks = append(blocks, types.ToolReferenceBlock{
			Type:     types.ContentTypeToolReference,
			ToolName: name,
		})
	}

	var sb strings.Builder
	if len(missing) > 0 {
		sb.WriteString(i18n.Format(lang, i18n.KeyToolSearchCatalogLoadedWithMissing,
			len(found), strings.Join(found, ", "), strings.Join(missing, ", "), totalDeferred))
	} else {
		sb.WriteString(i18n.Format(lang, i18n.KeyToolSearchCatalogLoadedForQuery,
			len(found), query, strings.Join(found, ", "), totalDeferred))
	}

	// Surface BM25 snippets and scores in the textual content so the model
	// can use them as ranking justification. Audit P0-2 row.
	scoreParts := make([]string, 0, len(scoredMatches))
	matchByName := make(map[string]scoredMatch, len(scoredMatches))
	for _, m := range scoredMatches {
		matchByName[m.Name] = m
	}
	for _, name := range found {
		m, ok := matchByName[name]
		if !ok {
			continue
		}
		sb.WriteString(i18n.Format(lang, i18n.KeyToolSearchCatalogMatchScore, m.Name, m.Score))
		if m.Snippet != "" {
			sb.WriteString(i18n.Format(lang, i18n.KeyToolSearchCatalogMatchSnippet, m.Snippet))
		}
		scoreParts = append(scoreParts, fmt.Sprintf("%s=%s", m.Name, strconv.FormatFloat(m.Score, 'f', 4, 64)))
	}

	res := types.ToolResult{
		Content:       sb.String(),
		ContentBlocks: blocks,
	}
	if len(scoreParts) > 0 {
		res.Metadata = map[string]string{}
	}
	if len(scoreParts) > 0 {
		res.Metadata["scores"] = strings.Join(scoreParts, ",")
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
			label = i18n.Format(lang, i18n.KeyMCPVisibilityReconnectAttempt, state.Name, state.ReconnectAttempt, state.MaxReconnectAttempts)
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
		parts = append(parts, i18n.Format(lang, i18n.KeyMCPVisibilityStateEntry, localizedMCPState(lang, state), strings.Join(values, ", ")))
	}
	for state, values := range partsByState {
		if stringSliceContains(order, state) || len(values) == 0 {
			continue
		}
		parts = append(parts, i18n.Format(lang, i18n.KeyMCPVisibilityStateEntry, state, strings.Join(values, ", ")))
	}
	prefix := ""
	if pending := partsByState["pending"]; len(pending) > 0 {
		prefix = i18n.Format(lang, i18n.KeyMCPVisibilityPendingServers, strings.Join(pending, ", "))
	}
	return prefix + i18n.Format(lang, i18n.KeyMCPVisibilityServerStates, strings.Join(parts, "; "))
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func localizedMCPState(lang i18n.Language, state string) string {
	switch state {
	case "pending":
		return i18n.Text(lang, i18n.KeyMCPStatePending)
	case "failed":
		return i18n.Text(lang, i18n.KeyMCPStateFailed)
	case "needs-auth":
		return i18n.Text(lang, i18n.KeyMCPStateNeedsAuth)
	case "disabled":
		return i18n.Text(lang, i18n.KeyMCPStateDisabled)
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
