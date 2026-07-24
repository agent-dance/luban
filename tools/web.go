package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// ─── Cache ─────────────────────────────────────────────────────────────────────

const cacheTTL = 15 * time.Minute

type cacheEntry struct {
	results       string
	searchResults []searchResult
	expiry        time.Time
}

type searchCache struct {
	mu      sync.Mutex
	entries map[string]*cacheEntry
}

// NewSearchCache creates a new cache for web search/fetch results.
func NewSearchCache() *searchCache {
	return &searchCache{entries: make(map[string]*cacheEntry)}
}

func (c *searchCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return "", false
	}
	// H21: Delete expired entries on access to prevent unbounded memory growth.
	if time.Now().After(e.expiry) {
		delete(c.entries, key)
		return "", false
	}
	return e.results, true
}

const maxCacheEntries = 1000

func (c *searchCache) set(key string, results string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &cacheEntry{results: results, expiry: time.Now().Add(cacheTTL)}

	// H21: Evict oldest entries when cache exceeds maxCacheEntries.
	if len(c.entries) > maxCacheEntries {
		var oldestKey string
		var oldestTime time.Time
		first := true
		for k, e := range c.entries {
			if first || e.expiry.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.expiry
				first = false
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
}

func (c *searchCache) getSearchResults(key string) ([]searchResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiry) || e.searchResults == nil {
		if ok && time.Now().After(e.expiry) {
			delete(c.entries, key)
		}
		return nil, false
	}
	return append([]searchResult(nil), e.searchResults...), true
}

func (c *searchCache) setSearchResults(key string, results []searchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &cacheEntry{
		searchResults: append([]searchResult(nil), results...),
		expiry:        time.Now().Add(cacheTTL),
	}
	c.evictOldestLocked()
}

func (c *searchCache) evictOldestLocked() {
	if len(c.entries) <= maxCacheEntries {
		return
	}
	var oldestKey string
	var oldestTime time.Time
	first := true
	for key, entry := range c.entries {
		if first || entry.expiry.Before(oldestTime) {
			oldestKey, oldestTime, first = key, entry.expiry, false
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// Global cache variables removed — caches are now created in SetupRegistry and
// injected via NewWebFetchTool(cache) and NewWebSearchTool(cache). (H5)

// ─── HTTP helpers ──────────────────────────────────────────────────────────────

const (
	maxBodyBytes             = 10 << 20 // 10 MB (TS WebFetchTool body cap)
	maxOutputRune            = 50_000
	userAgent                = "ClaudeCode/1.0"
	maxRedirects             = 10
	webFetchMaxAttempts      = 2
	webFetchDefaultRetryWait = 100 * time.Millisecond
)

const defaultWebFetchPrompt = "Return the page's main content and the details most relevant to the user's request."

// WebFetchTimeout aligns with TS WebFetchTool.fetchTimeout=60s. Servers
// behind CDN edge caches sometimes need >30s for first-byte; matching the
// TS budget avoids spurious "fetch failed" surfacing for slow but reachable
// targets.
const WebFetchTimeout = 60 * time.Second

type webExecutionMode string

const (
	webExecutionModeLocalFallback  webExecutionMode = "local_fallback"
	webExecutionModeProviderNative webExecutionMode = "provider_native"
)

var (
	reScript  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reStyle   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reTag     = regexp.MustCompile(`<[^>]+>`)
	reSpaces  = regexp.MustCompile(`[ \t]{2,}`)
	reNewline = regexp.MustCompile(`\n{3,}`)
	reAnchor  = regexp.MustCompile(`(?is)<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
)

// stripHTML removes scripts, styles, and HTML tags from s.
// Anchor tags are converted to "text (href)" to preserve link context.
func stripHTML(s string) string {
	s = reScript.ReplaceAllString(s, "")
	s = reStyle.ReplaceAllString(s, "")
	// Convert anchors: preserve both visible text and href.
	s = reAnchor.ReplaceAllStringFunc(s, func(match string) string {
		groups := reAnchor.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		href := strings.TrimSpace(groups[1])
		text := strings.TrimSpace(reTag.ReplaceAllString(groups[2], ""))
		switch {
		case text == "" && href == "":
			return ""
		case text == "" || text == href:
			return href
		case href == "" || strings.HasPrefix(href, "#"):
			return text
		default:
			return text + " (" + href + ")"
		}
	})
	s = reTag.ReplaceAllString(s, "")
	s = reSpaces.ReplaceAllString(s, " ")
	s = reNewline.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + toolRuntimeText(i18n.KeyToolRuntimeWebTruncatedMarker)
}

func isHTML(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml+xml")
}

// newHTTPClient creates a client that follows up to maxRedirects redirects.
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return errors.New(toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchStoppedAfterRedirects, maxRedirects))
			}
			return nil
		},
	}
}

// ─── WebFetchTool ──────────────────────────────────────────────────────────────

// WebFetchTool fetches a URL and returns its text content.
type WebFetchTool struct {
	// cache is retained for source compatibility with older package-local
	// literals. WebFetch execution uses fetchCache; WebSearch still uses the
	// legacy searchCache type independently.
	cache         *searchCache
	fetchCache    *WebFetchCache
	fetchCacheMu  sync.Mutex
	httpClient    *http.Client // nil → create per-Execute
	mode          webExecutionMode
	providerFetch func(ctx context.Context, input WebFetchInput) (webFetchStructuredPayload, error)
	// skipSSRFCheck disables URL validation (for tests using httptest servers).
	skipSSRFCheck bool
	// serverTool is the optional Anthropic web_fetch_20250910 server-tool provider.
	serverTool WebFetchServerToolProvider

	// Summariser is the small/fast secondary model used to apply the user
	// prompt to fetched markdown (mirrors src/tools/WebFetchTool/utils.ts:484-530
	// applyPromptToMarkdown). When nil the legacy keyword-overlap heuristic
	// is used as a fallback so offline/CI runs still produce output.
	Summariser SummariserClient

	// DomainInfoEndpoint is the optional base URL for the brand/security
	// blocklist preflight (api.anthropic.com/api/web/domain_info). When
	// empty the preflight is skipped. DomainInfoClient overrides the
	// default http.Client used for the preflight.
	DomainInfoEndpoint string
	DomainInfoClient   *http.Client
	// SkipWebFetchPreflight is the Go settings equivalent of the TS
	// skipWebFetchPreflight escape hatch for restricted enterprise networks.
	SkipWebFetchPreflight bool

	// Domain restrictions (Task 7: security hardening).
	AllowedDomains    []string // nil = all allowed (whitelist)
	DisallowedDomains []string // these domains always blocked (blacklist)
}

// WithSummariser plugs a fast-model summariser into the WebFetchTool and
// returns the receiver so callers can chain. Mirrors the TS pattern of
// passing a Haiku-class client into the local fetch path.
func (w *WebFetchTool) WithSummariser(client SummariserClient) *WebFetchTool {
	w.Summariser = client
	return w
}

// NewWebFetchTool creates a WebFetchTool using a typed raw-content cache. The
// any parameter accepts *searchCache for source compatibility with embedders
// built before raw URL cache alignment; those values are not used by fetches.
func NewWebFetchTool(cache any) *WebFetchTool {
	tool := &WebFetchTool{
		cache:              NewSearchCache(),
		mode:               webExecutionModeLocalFallback,
		DomainInfoEndpoint: domainInfoDefaultEndpoint,
	}
	switch value := cache.(type) {
	case *WebFetchCache:
		tool.fetchCache = value
	case *searchCache:
		if value != nil {
			tool.cache = value
		}
	}
	if tool.fetchCache == nil {
		tool.fetchCache = NewWebFetchCache()
	}
	return tool
}

// FetchCache exposes the session-owned cache for lifecycle wiring and tests.
// Direct struct literals lazily receive a private cache.
func (w *WebFetchTool) FetchCache() *WebFetchCache {
	if w == nil {
		return nil
	}
	w.fetchCacheMu.Lock()
	defer w.fetchCacheMu.Unlock()
	if w.fetchCache == nil {
		w.fetchCache = NewWebFetchCache()
	}
	return w.fetchCache
}

// ClearWebFetchCache clears fetched content and allowed domain verdicts while
// keeping the tool usable for the rest of the session.
func (w *WebFetchTool) ClearWebFetchCache() {
	if cache := w.FetchCache(); cache != nil {
		cache.Clear()
	}
	ResetDomainInfoCache()
}

func (w *WebFetchTool) Name() string           { return "WebFetch" }
func (w *WebFetchTool) IsConcurrentSafe() bool { return true }

func (w *WebFetchTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func (w *WebFetchTool) ToolContract() types.ToolContract {
	output := types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"bytes":      map[string]any{"type": "number", "description": "Size of the fetched content in bytes"},
			"code":       map[string]any{"type": "number", "description": "HTTP response code"},
			"codeText":   map[string]any{"type": "string", "description": "HTTP response code text"},
			"result":     map[string]any{"type": "string", "description": "Processed result from applying the prompt to the content"},
			"durationMs": map[string]any{"type": "number", "description": "Time taken to fetch and process the content"},
			"url":        map[string]any{"type": "string", "description": "The URL that was fetched"},
		},
		Required: []string{"bytes", "code", "codeText", "result", "durationMs", "url"},
	}
	return types.ToolContract{OutputSchema: &output, Strict: true, ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func normalizeWebFetchInput(input map[string]any) map[string]any {
	prompt, present := input["prompt"]
	if present {
		if value, ok := prompt.(string); !ok || strings.TrimSpace(value) != "" {
			return input
		}
	}
	normalized := make(map[string]any, len(input)+1)
	for key, value := range input {
		normalized[key] = value
	}
	normalized["prompt"] = defaultWebFetchPrompt
	return normalized
}

// NormalizeToolInput tolerates compatible providers that omit the required
// prompt despite receiving the strict schema.
func (w *WebFetchTool) NormalizeToolInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return normalizeWebFetchInput(input), nil
}

// BackfillObservableInput keeps direct registry dispatch consistent with the
// query loop, which invokes NormalizeToolInput before validation.
func (w *WebFetchTool) BackfillObservableInput(input map[string]any) (map[string]any, error) {
	return normalizeWebFetchInput(input), nil
}

func (w *WebFetchTool) IsReadOnly() bool { return true }

func (w *WebFetchTool) ToAutoClassifierInput(input map[string]any) string {
	rawURL, _ := input["url"].(string)
	prompt, _ := input["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return rawURL
	}
	return rawURL + ": " + prompt
}

func (w *WebFetchTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	rawURL, _ := input["url"].(string)
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return webFetchAskDecision("input:"), nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return webFetchAskDecision("input:" + rawURL), nil
	}
	// CLI/config domain lists are a managed policy layer rather than user
	// approval rules. Managed deny/whitelist constraints are bypass-immune and
	// intentionally take precedence over the TS preapproved list.
	if err := checkDomainAllowed(rawURL, w.AllowedDomains, w.DisallowedDomains); err != nil {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: localizedWebDomainPolicyError(err), Required: true}, nil
	}
	if IsPreapprovedHost(rawURL) {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}, nil
	}
	host := strings.ToLower(parsed.Hostname())
	ruleContent := "domain:" + host
	if webFetchRuleMatches(request.Runtime.DeniedRules, ruleContent) {
		return types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorDeny,
			Message:  toolPermissionFormat(i18n.KeyToolPermissionWebFetchDenied, ruleContent),
			Required: true,
		}, nil
	}
	if webFetchRuleMatches(request.Runtime.AskRules, ruleContent) {
		decision := webFetchAskDecision(ruleContent)
		decision.Required = true
		return decision, nil
	}
	if webFetchRuleMatches(request.Runtime.AllowedRules, ruleContent) || w.AllowedDomains != nil {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}, nil
	}
	return webFetchAskDecision(ruleContent), nil
}

func webFetchRuleMatches(rules []types.PermissionRuleValue, ruleContent string) bool {
	for _, rule := range rules {
		if strings.EqualFold(strings.TrimSpace(rule.ToolName), "WebFetch") && strings.TrimSpace(rule.RuleContent) == ruleContent {
			return true
		}
	}
	return false
}

func webFetchAskDecision(ruleContent string) types.ToolPermissionResult {
	return types.ToolPermissionResult{
		Behavior: types.PermissionBehaviorAsk,
		Message:  toolPermissionText(i18n.KeyToolPermissionWebFetchPending),
		Suggestions: []types.PermissionUpdate{{
			Type:        types.PermissionUpdateAddRules,
			Destination: types.PermissionDestinationLocalSettings,
			Behavior:    types.PermissionBehaviorAllow,
			Rules: []types.PermissionRuleValue{{
				ToolName:    "WebFetch",
				RuleContent: ruleContent,
			}},
		}},
	}
}

// webFetchPromptDescription mirrors the DESCRIPTION constant exported from
// src/tools/WebFetchTool/prompt.ts so the Go-side tool prompt matches the
// TS reference verbatim. The leading "IMPORTANT" line mirrors the prefix
// added by WebFetchTool.ts (the verbatim authentication-warning paragraph).
const webFetchPromptDescription = `IMPORTANT: WebFetch WILL FAIL for authenticated or private URLs. Before using this tool, check if the URL points to an authenticated service (e.g. Google Docs, Confluence, Jira, GitHub). If so, look for a specialized MCP tool that provides authenticated access.

- Fetches content from a specified URL and processes it using an AI model
- Takes a URL and a prompt as input
- Fetches the URL content, converts HTML to markdown
- Processes the content with the prompt using a small, fast model
- Returns the model's response about the content
- Use this tool when you need to retrieve and analyze web content

Usage notes:
  - IMPORTANT: If an MCP-provided web fetch tool is available, prefer using that tool instead of this one, as it may have fewer restrictions.
  - The URL must be a fully-formed valid URL
  - HTTP URLs will be automatically upgraded to HTTPS
  - The prompt should describe what information you want to extract from the page
  - This tool is read-only and does not modify any files
  - Results may be summarized if the content is very large
  - Includes a self-cleaning 15-minute cache for faster responses when repeatedly accessing the same URL
  - When a URL redirects to a different host, the tool will inform you and provide the redirect URL in a special format. You should then make a new WebFetch request with the redirect URL to fetch the content.
  - For GitHub URLs, prefer using the gh CLI via Bash instead (e.g., gh pr view, gh issue view, gh api).`

func (w *WebFetchTool) Description() string {
	return webFetchPromptDescription
}

func (w *WebFetchTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"url": map[string]any{
				"type":        "string",
				"format":      "uri",
				"description": "The URL to fetch",
			},
			"prompt": map[string]any{
				"type":        "string",
				"minLength":   1,
				"description": "What to extract from the page (provided as context for the LLM)",
			},
		},
		"url", "prompt",
	)
}

func makeWebFetchCacheKey(rawURL string, _ ...string) string {
	return WebFetchCacheKey(rawURL)
}

func extractRelevantContent(body, prompt string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return body
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return body
	}
	segments := splitIntoSegments(body)
	if len(segments) == 0 {
		return body
	}
	keywords := promptKeywords(prompt)
	if len(keywords) == 0 {
		return body
	}
	type scoredSegment struct {
		text  string
		score int
		idx   int
	}
	var scored []scoredSegment
	for i, seg := range segments {
		score := scoreSegment(seg, keywords)
		if score > 0 {
			scored = append(scored, scoredSegment{text: seg, score: score, idx: i})
		}
	}
	if len(scored) == 0 {
		return body
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].idx < scored[j].idx
		}
		return scored[i].score > scored[j].score
	})
	maxSegments := 3
	if len(scored) < maxSegments {
		maxSegments = len(scored)
	}
	chosen := scored[:maxSegments]
	sort.Slice(chosen, func(i, j int) bool { return chosen[i].idx < chosen[j].idx })
	parts := make([]string, 0, len(chosen))
	for _, seg := range chosen {
		parts = append(parts, seg.text)
	}
	return strings.Join(parts, "\n\n")
}

func splitIntoSegments(body string) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	chunks := strings.Split(body, "\n\n")
	var out []string
	for _, c := range chunks {
		c = strings.TrimSpace(c)
		if c != "" {
			out = append(out, c)
		}
	}
	if len(out) > 0 {
		return out
	}
	lines := strings.Split(body, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func promptKeywords(prompt string) []string {
	prompt = strings.ToLower(prompt)
	repl := strings.NewReplacer(",", " ", ".", " ", ":", " ", ";", " ", "?", " ", "!", " ", "(", " ", ")", " ", "[", " ", "]", " ", "{", " ", "}", " ", "\"", " ", "'", " ", "-", " ", "_", " ", "/", " ")
	prompt = repl.Replace(prompt)
	parts := strings.Fields(prompt)
	stop := map[string]struct{}{
		"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "to": {}, "of": {}, "in": {}, "on": {}, "for": {}, "with": {},
		"find": {}, "extract": {}, "summarize": {}, "show": {}, "tell": {}, "what": {}, "does": {}, "page": {}, "this": {},
	}
	seen := map[string]struct{}{}
	var out []string
	for _, p := range parts {
		if len(p) < 3 {
			continue
		}
		if _, ok := stop[p]; ok {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func scoreSegment(segment string, keywords []string) int {
	lower := strings.ToLower(segment)
	score := 0
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			score++
		}
	}
	return score
}

type webFetchStructuredPayload struct {
	URL          string   `json:"url"`
	Prompt       string   `json:"prompt"`
	Method       string   `json:"method"`
	Summary      string   `json:"summary"`
	Snippets     []string `json:"snippets,omitempty"`
	Truncated    bool     `json:"truncated"`
	RedirectURL  string   `json:"redirect_url,omitempty"`
	RedirectCode int      `json:"redirect_code,omitempty"`
}

// WebFetchOutput mirrors WebFetchTool.outputSchema in the TS implementation.
type WebFetchOutput struct {
	Bytes      int    `json:"bytes"`
	Code       int    `json:"code"`
	CodeText   string `json:"codeText"`
	Result     string `json:"result"`
	DurationMs int64  `json:"durationMs"`
	URL        string `json:"url"`
}

type webSearchStructuredPayload struct {
	Query          string          `json:"query"`
	Method         string          `json:"method"`
	FallbackReason string          `json:"fallbackReason,omitempty"`
	Progress       []string        `json:"progress,omitempty"`
	Results        []searchResult  `json:"results,omitempty"`
	Output         WebSearchOutput `json:"-"`
	Usage          *types.Usage    `json:"-"`
}

func marshalStructuredPayload(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"marshal_error":%q}`, err.Error())
	}
	return string(b)
}

func buildWebFetchStructuredResult(url, prompt, relevant string, method webExecutionMode) types.ToolResult {
	_ = prompt // retained in the compatibility signature, never model-visible.
	return buildWebFetchResult(WebFetchOutput{
		Bytes:    len([]byte(relevant)),
		Code:     http.StatusOK,
		CodeText: http.StatusText(http.StatusOK),
		Result:   relevant,
		URL:      url,
	}, method)
}

func buildWebFetchResult(output WebFetchOutput, method webExecutionMode) types.ToolResult {
	encoded := marshalStructuredPayload(output)
	return types.ToolResult{
		Content: output.Result,
		Data:    output,
		Metadata: map[string]string{
			"bytes":                 strconv.Itoa(output.Bytes),
			"code":                  strconv.Itoa(output.Code),
			"codeText":              output.CodeText,
			"durationMs":            strconv.FormatInt(output.DurationMs, 10),
			"url":                   output.URL,
			"method":                string(method),
			"webfetch_summary_json": encoded,
		},
	}
}

func (w *WebFetchTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(WebFetchOutput)
	if !ok {
		if pointer, pointerOK := data.(*WebFetchOutput); pointerOK && pointer != nil {
			output, ok = *pointer, true
		}
	}
	if !ok {
		return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: fmt.Sprint(data), Data: data}
	}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: output.Result, Data: output}
}

func buildWebSearchStructuredResult(query string, results []searchResult, fallbackReason string, method webExecutionMode) types.ToolResult {
	output := webSearchOutputFromResults(query, results, 0, "")
	return buildWebSearchOutputResult(output, results, fallbackReason, method, nil)
}

func webSearchOutputFromResults(query string, results []searchResult, durationSeconds float64, toolUseID string) WebSearchOutput {
	output := WebSearchOutput{Query: query, Results: make([]any, 0), DurationSeconds: durationSeconds}
	if results == nil {
		return output
	}
	search := WebSearchOutputSearchResult{ToolUseID: toolUseID, Content: make([]WebSearchOutputLink, 0, len(results))}
	for _, result := range results {
		search.Content = append(search.Content, WebSearchOutputLink{Title: result.Title, URL: result.URL})
	}
	output.Results = []any{search}
	return output
}

func buildWebSearchOutputResult(output WebSearchOutput, results []searchResult, fallbackReason string, method webExecutionMode, usage *types.Usage) types.ToolResult {
	payload := webSearchStructuredPayload{
		Query:          output.Query,
		Method:         string(method),
		FallbackReason: fallbackReason,
		Results:        results,
		Output:         output,
		Usage:          usage,
	}

	// Typed server-tool block (TS parity). Emitted as a TextBlock with the
	// canonical `web_search_tool_result` JSON envelope so transcripts and
	// renderers can identify it without coupling to a Go-specific type.
	toolResultBlock := WebSearchToolResultBlock{
		Type:    "web_search_tool_result",
		Content: make([]WebSearchResultBlock, 0, len(results)),
	}
	for _, value := range output.Results {
		if search, ok := value.(WebSearchOutputSearchResult); ok {
			toolResultBlock.ToolUseID = search.ToolUseID
			break
		}
	}
	for _, r := range results {
		toolResultBlock.Content = append(toolResultBlock.Content, WebSearchResultBlock{
			URL:     r.URL,
			Title:   r.Title,
			Snippet: r.Snippet,
		})
	}

	var blocks []types.ContentBlock
	if fallbackReason != "" {
		blocks = append(blocks, types.TextBlock{Type: types.ContentTypeText, Text: fmt.Sprintf("websearch_fallback_reason=%s", fallbackReason)})
	}
	blocks = append(blocks, types.TextBlock{Type: types.ContentTypeText, Text: "websearch_result_json=" + marshalStructuredPayload(payload)})
	blocks = append(blocks, toolResultBlock)
	mapped := (&WebSearchTool{}).MapToolResultToToolResultBlock(output, "")
	return types.ToolResult{
		Content:       mapped.Content,
		ContentBlocks: blocks,
		Data:          output,
		Usage:         usage,
		Metadata: map[string]string{
			"results_count":   strconv.Itoa(len(results)),
			"search_count":    strconv.Itoa(len(output.Results)),
			"durationSeconds": strconv.FormatFloat(output.DurationSeconds, 'f', -1, 64),
			"query":           output.Query,
			"method":          string(method),
		},
	}
}

func (w *WebFetchTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	started := time.Now()
	input = normalizeWebFetchInput(input)
	in, toolErr := parseStrictInputOrError[WebFetchInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	if strings.TrimSpace(in.URL) == "" {
		return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolRuntimeWebFetchURLRequired), IsError: true}, nil
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolRuntimeWebFetchPromptRequired), IsError: true}, nil
	}
	if err := validateWebFetchURLSyntax(in.URL); err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchInvalidURL, in.URL), IsError: true}, nil
	}

	// Managed CLI domain policy remains separate from per-call approval rules.
	// Direct Execute callers still receive the same non-bypassable constraint.
	if err := checkDomainAllowed(in.URL, w.AllowedDomains, w.DisallowedDomains); err != nil {
		return types.ToolResult{Content: localizedWebDomainRuntimeError(err), IsError: true}, nil
	}

	// Provider-native extensions already apply the prompt remotely, so they do
	// not participate in the local raw-content cache or local domain preflight.
	if w.serverTool != nil && w.mode == webExecutionModeProviderNative {
		payload, err := runWebFetchServerTool(ctx, w.serverTool, in, w.AllowedDomains, w.DisallowedDomains)
		if err == nil {
			code := http.StatusOK
			if payload.RedirectCode != 0 {
				code = payload.RedirectCode
			}
			return buildWebFetchResult(WebFetchOutput{
				Bytes:      len([]byte(payload.Summary)),
				Code:       code,
				CodeText:   redirectOrHTTPStatusText(code),
				Result:     payload.Summary,
				DurationMs: time.Since(started).Milliseconds(),
				URL:        in.URL,
			}, webExecutionModeProviderNative), nil
		}
		if !errors.Is(err, ErrWebFetchServerToolUnavailable) {
			return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebError, err), IsError: true}, nil
		}
	}
	if w.mode == webExecutionModeProviderNative && w.providerFetch != nil {
		payload, err := w.providerFetch(ctx, in)
		if err != nil {
			return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebError, err), IsError: true}, nil
		}
		return buildWebFetchResult(WebFetchOutput{
			Bytes:      len([]byte(payload.Summary)),
			Code:       http.StatusOK,
			CodeText:   http.StatusText(http.StatusOK),
			Result:     payload.Summary,
			DurationMs: time.Since(started).Milliseconds(),
			URL:        in.URL,
		}, webExecutionModeProviderNative), nil
	}

	originalURL := in.URL
	cache := w.FetchCache()
	cacheKey := cache.MakeKey(originalURL)
	if cached, ok := cache.Get(cacheKey); ok {
		return w.applyWebFetchPrompt(ctx, in, cached, started, webExecutionModeLocalFallback)
	}

	// HTTP→HTTPS auto-upgrade. Mirrors the behaviour described in the TS
	// prompt so we never silently emit an http:// fetch. Skipped when
	// SSRF checks are disabled (test mode using httptest servers, which
	// only serve plain HTTP on loopback).
	if !w.skipSSRFCheck {
		if upgraded := upgradeHTTPToHTTPS(in.URL); upgraded != in.URL {
			in.URL = upgraded
		}
	}

	// DNS/IP restrictions are an explicit managed hardening layer beyond TS
	// validation. They remain enabled in production and are disabled only by
	// the package-private test switch used with httptest loopback servers.
	if !w.skipSSRFCheck {
		if err := validateURL(in.URL); err != nil {
			return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchBlockedURL, err), IsError: true}, nil
		}
	}

	// Domain security preflight is on by default for constructor/registry tools.
	// Only an explicit skip setting can disable it. Failed checks are surfaced
	// to the model and never cached.
	if PreflightDomainInfoEnabled(w) {
		if u, perr := url.Parse(in.URL); perr == nil && u.Host != "" {
			blocked, lookupErr := domainInfoLookup(ctx, w.DomainInfoClient, w.DomainInfoEndpoint, u.Hostname())
			if lookupErr != nil {
				return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebError, lookupErr), IsError: true}, nil
			}
			if blocked {
				return types.ToolResult{
					Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchDomainUnavailable, u.Hostname()),
					IsError: true,
				}, nil
			}
		}
	}

	fetchCtx, cancel := context.WithTimeout(ctx, WebFetchTimeout)
	defer cancel()

	// W6 fix: capture domain restrictions for redirect checking.
	allowedDomains := w.AllowedDomains
	disallowedDomains := w.DisallowedDomains
	skipSSRF := w.skipSSRFCheck

	client := w.httpClient
	if client == nil {
		if skipSSRF {
			client = newHTTPClient(WebFetchTimeout)
		} else {
			client = newSSRFSafeHTTPClient(WebFetchTimeout, maxRedirects)
		}
	}

	// W6 fix: wrap the client's CheckRedirect to re-validate domain
	// restrictions on every redirect hop. Cross-host redirects are
	// surfaced to the model via a REDIRECT marker rather than being
	// followed silently — matching the TS reference's behaviour.
	type redirectMarker struct {
		from string
		to   string
		code int
	}
	var crossHostRedirect *redirectMarker
	wrappedClient := *client
	wrappedClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return errors.New(toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchStoppedAfterRedirects, maxRedirects))
		}
		if len(via) == 0 {
			return nil
		}
		code := http.StatusFound
		if resp := req.Response; resp != nil {
			code = resp.StatusCode
		}
		if code != http.StatusMovedPermanently && code != http.StatusFound && code != http.StatusTemporaryRedirect && code != http.StatusPermanentRedirect {
			return http.ErrUseLastResponse
		}
		redirectURL := req.URL.String()
		from := via[len(via)-1].URL.String()
		if !sameOriginRedirect(from, redirectURL) {
			crossHostRedirect = &redirectMarker{from: from, to: redirectURL, code: code}
			return http.ErrUseLastResponse
		}
		// Re-check domain restrictions on the redirect target.
		if err := checkDomainAllowed(redirectURL, allowedDomains, disallowedDomains); err != nil {
			return fmt.Errorf("%s: %w", toolRuntimeText(i18n.KeyToolRuntimeWebFetchRedirectBlocked), err)
		}
		// Re-check SSRF on the redirect target.
		if !skipSSRF {
			if err := validateURL(redirectURL); err != nil {
				return fmt.Errorf("%s: %w", toolRuntimeText(i18n.KeyToolRuntimeWebFetchRedirectBlockedSSRF), err)
			}
		}
		return nil
	}

	var (
		resp *http.Response
		err  error
	)
	for attempt := 0; attempt < webFetchMaxAttempts; attempt++ {
		resp, err = doWebFetchRequest(fetchCtx, &wrappedClient, in.URL)
		if err != nil {
			return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchFailed, err), IsError: true}, nil
		}
		if attempt+1 >= webFetchMaxAttempts || !isTransientWebFetchStatus(resp.StatusCode) {
			break
		}
		delay := webFetchRetryDelay(resp)
		_, _ = io.CopyN(io.Discard, resp.Body, 4<<10)
		_ = resp.Body.Close()
		if err := waitForWebFetchRetry(fetchCtx, delay); err != nil {
			return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchFailed, err), IsError: true}, nil
		}
	}
	defer resp.Body.Close()

	if crossHostRedirect != nil {
		marker := formatRedirectMarker(crossHostRedirect.from, crossHostRedirect.to, crossHostRedirect.code, in.Prompt)
		return buildWebFetchResult(WebFetchOutput{
			Bytes:      len([]byte(marker)),
			Code:       crossHostRedirect.code,
			CodeText:   redirectStatusText(crossHostRedirect.code),
			Result:     marker,
			DurationMs: time.Since(started).Milliseconds(),
			URL:        originalURL,
		}, webExecutionModeLocalFallback), nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// webfetch-egress-blocked-error: corporate proxies / Anthropic
		// internal egress filter return 403 with X-Proxy-Error: blocked-by-allowlist.
		// Surface this distinctly so the model doesn't think the origin
		// is dead — it's the org's policy, not the URL.
		if px := resp.Header.Get("X-Proxy-Error"); strings.EqualFold(px, "blocked-by-allowlist") {
			return types.ToolResult{
				Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchEgressBlocked, resp.StatusCode, in.URL),
				IsError: true,
			}, nil
		}
		return types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchHTTPError, resp.StatusCode, httpResponseStatusText(resp)),
			IsError: true,
		}, nil
	}

	if resp.ContentLength > maxBodyBytes {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchResponseTooLargeWithBytes, resp.ContentLength), IsError: true}, nil
	}
	limited := io.LimitReader(resp.Body, maxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchReadResponseFailed, err), IsError: true}, nil
	}
	if len(body) > maxBodyBytes {
		return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolRuntimeWebFetchResponseTooLarge), IsError: true}, nil
	}

	contentType := resp.Header.Get("Content-Type")
	persistedPath := ""
	persistedSize := 0

	// Binary bytes are saved as a supplement, then still decoded and sent
	// through prompt application. PDFs often retain useful ASCII metadata.
	if isBinaryContentType(contentType) {
		path, perr := persistBinaryContent(body, in.URL, contentType)
		if perr == nil {
			persistedPath = path
			persistedSize = len(body)
		}
	}

	content := strings.ToValidUTF8(string(body), "\uFFFD")
	switch {
	case IsHTMLContentType(contentType):
		// Use the spec'd HTML→Markdown converter so code blocks survive
		// and stripping rules match TS exactly. The DOM-aware variant
		// is opt-in via CLAUDE_WEBFETCH_DOM_MARKDOWN=1 — it handles
		// nested lists, tables and code-block language hints far more
		// faithfully than the regex pipeline.
		if useDOMMarkdownConverter() {
			content = HTMLToMarkdownDOM(content)
		} else {
			content = HTMLToMarkdown(content)
		}
	case isHTML(contentType):
		// Belt-and-braces fallback for legacy callers; same conversion
		// path as above.
		if useDOMMarkdownConverter() {
			content = HTMLToMarkdownDOM(content)
		} else {
			content = HTMLToMarkdown(content)
		}
	}
	content = strings.TrimSpace(content)

	entry := WebFetchCacheEntry{
		Body:          content,
		ContentType:   contentType,
		ContentLength: len(content),
		CacheSize:     len(content),
		StatusCode:    resp.StatusCode,
		StatusText:    httpResponseStatusText(resp),
		URL:           originalURL,
		Bytes:         len(body),
		Truncated:     strings.HasSuffix(content, markdownTruncationMarker),
		PersistedPath: persistedPath,
		PersistedSize: persistedSize,
	}
	cache.Set(cacheKey, entry)
	return w.applyWebFetchPrompt(ctx, WebFetchInput{URL: originalURL, Prompt: in.Prompt}, entry, started, webExecutionModeLocalFallback)
}

func doWebFetchRequest(ctx context.Context, client *http.Client, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", toolRuntimeText(i18n.KeyToolRuntimeWebFetchInvalidURLPrefix), err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/markdown, text/html, */*")
	return client.Do(req)
}

func isTransientWebFetchStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func webFetchRetryDelay(resp *http.Response) time.Duration {
	if resp == nil {
		return webFetchDefaultRetryWait
	}
	raw := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
		delay := time.Duration(seconds) * time.Second
		if delay > 2*time.Second {
			return 2 * time.Second
		}
		return delay
	}
	if deadline, err := http.ParseTime(raw); err == nil {
		delay := time.Until(deadline)
		if delay < 0 {
			return 0
		}
		if delay > 2*time.Second {
			return 2 * time.Second
		}
		return delay
	}
	return webFetchDefaultRetryWait
}

func waitForWebFetchRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func validateWebFetchURLSyntax(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid URL")
	}
	return nil
}

func redirectOrHTTPStatusText(code int) string {
	if code >= 300 && code < 400 {
		return redirectStatusText(code)
	}
	return http.StatusText(code)
}

func httpResponseStatusText(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	prefix := strconv.Itoa(resp.StatusCode)
	if text := strings.TrimSpace(strings.TrimPrefix(resp.Status, prefix)); text != "" {
		return text
	}
	return http.StatusText(resp.StatusCode)
}

func (w *WebFetchTool) applyWebFetchPrompt(ctx context.Context, input WebFetchInput, entry WebFetchCacheEntry, started time.Time, method webExecutionMode) (types.ToolResult, error) {
	result := ""
	if IsPreapprovedHost(input.URL) && isMarkdownContentType(entry.ContentType) && len(entry.Body) < MaxMarkdownBytes {
		result = entry.Body
	} else {
		summary, err := RunWebFetchSummariser(ctx, w.Summariser, input.URL, input.Prompt, entry.Body, IsPreapprovedHost(input.URL))
		if err != nil {
			if ctx.Err() != nil {
				return types.ToolResult{}, ctx.Err()
			}
			return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebError, err), IsError: true}, nil
		}
		if ctx.Err() != nil {
			return types.ToolResult{}, ctx.Err()
		}
		result = summary
	}
	if entry.PersistedPath != "" {
		size := entry.PersistedSize
		if size <= 0 {
			size = entry.Bytes
		}
		result += toolRuntimeFormat(
			i18n.KeyToolRuntimeWebFetchBinarySaved,
			entry.ContentType,
			formatWebFetchFileSize(size),
			entry.PersistedPath,
		)
	}
	return buildWebFetchResult(WebFetchOutput{
		Bytes:      entry.Bytes,
		Code:       entry.StatusCode,
		CodeText:   entry.StatusText,
		Result:     result,
		DurationMs: time.Since(started).Milliseconds(),
		URL:        input.URL,
	}, method), nil
}

func formatWebFetchFileSize(size int) string {
	if size < 1024 {
		return toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchBytes, size)
	}
	value := float64(size) / 1024
	unit := "KB"
	if value >= 1024 {
		value /= 1024
		unit = "MB"
	}
	if value >= 1024 {
		value /= 1024
		unit = "GB"
	}
	formatted := strconv.FormatFloat(value, 'f', 1, 64)
	formatted = strings.TrimSuffix(formatted, ".0")
	return formatted + unit
}

// isMarkdownContentType detects text/markdown responses (case-insensitive,
// parameter-tolerant). Mirrors the TS isBinaryContentType / preapproved
// markdown gate.
func isMarkdownContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "" {
		return false
	}
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct == "text/markdown" || ct == "text/x-markdown" ||
		strings.HasSuffix(ct, "+markdown")
}

// isBinaryContentType detects content types we should not attempt to
// stringify (PDFs, archives, images, fonts). Mirrors the TS
// isBinaryContentType helper at WebFetchTool/utils.ts:440-449.
func isBinaryContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ct == "" {
		return false
	}
	prefixes := []string{"image/", "video/", "audio/", "font/"}
	for _, p := range prefixes {
		if strings.HasPrefix(ct, p) {
			return true
		}
	}
	binTypes := []string{
		"application/pdf",
		"application/zip",
		"application/x-tar",
		"application/x-rar-compressed",
		"application/x-7z-compressed",
		"application/x-bzip",
		"application/x-bzip2",
		"application/gzip",
		"application/x-gzip",
		"application/octet-stream",
		"application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.ms-excel",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-powerpoint",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
	}
	for _, b := range binTypes {
		if ct == b {
			return true
		}
	}
	return false
}

// persistBinaryContent writes the raw bytes to a tagged file under the
// configured temp dir (or os.TempDir() when none is set) and returns the
// final path. Mirrors the TS WebFetchTool persistBinaryContent helper.
func persistBinaryContent(data []byte, sourceURL, contentType string) (string, error) {
	_ = sourceURL
	dir := os.Getenv("CLAUDE_WEBFETCH_BINARY_DIR")
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join(os.TempDir(), "claude-webfetch")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, "webfetch-*"+binaryContentExtension(contentType))
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func binaryContentExtension(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	if extensions, _ := mime.ExtensionsByType(mediaType); len(extensions) > 0 {
		return extensions[0]
	}
	switch strings.ToLower(mediaType) {
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ".pptx"
	default:
		return ".bin"
	}
}

// ─── WebSearchTool ─────────────────────────────────────────────────────────────

const (
	ddgInstantAnswerBase = "https://api.duckduckgo.com/"
	ddgLiteBase          = "https://lite.duckduckgo.com/lite/"
)

// ddgInstantResponse is the DuckDuckGo Instant Answer API JSON shape.
type ddgInstantResponse struct {
	AbstractText  string            `json:"AbstractText"`
	AbstractURL   string            `json:"AbstractURL"`
	RelatedTopics []ddgRelatedTopic `json:"RelatedTopics"`
}

type ddgRelatedTopic struct {
	Text     string            `json:"Text"`
	FirstURL string            `json:"FirstURL"`
	Topics   []ddgRelatedTopic `json:"Topics,omitempty"`
}

type webFetchNormalizedResult struct {
	Input struct {
		URL    string `json:"url"`
		Prompt string `json:"prompt"`
	} `json:"input"`
	Execution struct {
		Method      string `json:"method"`
		ResolvedURL string `json:"resolvedUrl,omitempty"`
		Truncated   bool   `json:"truncated"`
		CacheHit    bool   `json:"cacheHit"`
	} `json:"execution"`
	Content struct {
		Body string `json:"body"`
	} `json:"content"`
	Error string `json:"error,omitempty"`
}

type webSearchProgressEvent struct {
	Type  string `json:"type"`
	Query string `json:"query,omitempty"`
	Count int    `json:"count,omitempty"`
}

type webSearchNormalizedItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

type webSearchNormalizedResult struct {
	Input struct {
		Query          string   `json:"query"`
		AllowedDomains []string `json:"allowedDomains,omitempty"`
		BlockedDomains []string `json:"blockedDomains,omitempty"`
	} `json:"input"`
	Execution struct {
		Method         string `json:"method"`
		FallbackReason string `json:"fallbackReason,omitempty"`
		CacheHit       bool   `json:"cacheHit"`
	} `json:"execution"`
	Progress []webSearchProgressEvent  `json:"progress,omitempty"`
	Results  []webSearchNormalizedItem `json:"results,omitempty"`
	Error    string                    `json:"error,omitempty"`
}

// searchResult represents a single web search result.
type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

func normalizeWebFetchResult(url, prompt, body string, truncated, cacheHit bool, err error) webFetchNormalizedResult {
	var out webFetchNormalizedResult
	out.Input.URL = url
	out.Input.Prompt = prompt
	out.Execution.Method = "local_fallback"
	out.Execution.ResolvedURL = url
	out.Execution.Truncated = truncated
	out.Execution.CacheHit = cacheHit
	out.Content.Body = body
	if err != nil {
		out.Error = err.Error()
	}
	return out
}

func normalizeStructuredWebFetchToolResult(result types.ToolResult, url, prompt string, cacheHit bool) webFetchNormalizedResult {
	out := normalizeWebFetchResult(url, prompt, result.Content, strings.Contains(result.Content, "[truncated]"), cacheHit, nil)
	if typed, ok := result.Data.(WebFetchOutput); ok {
		out.Content.Body = typed.Result
		out.Execution.ResolvedURL = typed.URL
		if method := result.Metadata["method"]; method != "" {
			out.Execution.Method = method
		}
		return out
	}
	for _, block := range result.ContentBlocks {
		tb, ok := block.(types.TextBlock)
		if !ok {
			continue
		}
		const prefix = "webfetch_summary_json="
		if !strings.HasPrefix(tb.Text, prefix) {
			continue
		}
		var payload webFetchStructuredPayload
		if err := json.Unmarshal([]byte(strings.TrimPrefix(tb.Text, prefix)), &payload); err == nil {
			out.Execution.Method = payload.Method
			if payload.URL != "" {
				out.Execution.ResolvedURL = payload.URL
			}
			if payload.Summary != "" {
				out.Content.Body = payload.Summary
			}
		}
	}
	return out
}

func normalizeWebSearchResult(query string, allowedDomains, blockedDomains []string, results []searchResult, cacheHit bool, fallbackReason string, err error) webSearchNormalizedResult {
	var out webSearchNormalizedResult
	out.Input.Query = query
	out.Input.AllowedDomains = append([]string(nil), allowedDomains...)
	out.Input.BlockedDomains = append([]string(nil), blockedDomains...)
	out.Execution.Method = "local_fallback"
	out.Execution.FallbackReason = fallbackReason
	out.Execution.CacheHit = cacheHit
	out.Progress = append(out.Progress,
		webSearchProgressEvent{Type: "started"},
		webSearchProgressEvent{Type: "query_issued", Query: query},
	)
	for _, r := range results {
		out.Results = append(out.Results, webSearchNormalizedItem{Title: r.Title, URL: r.URL, Snippet: r.Snippet})
	}
	if len(results) > 0 {
		out.Progress = append(out.Progress, webSearchProgressEvent{Type: "results_received", Count: len(results)})
	}
	if err != nil {
		out.Error = err.Error()
	}
	return out
}

func normalizeStructuredWebSearchToolResult(result types.ToolResult, query string, allowedDomains, blockedDomains []string, cacheHit bool) webSearchNormalizedResult {
	out := webSearchNormalizedResult{}
	out.Input.Query = query
	out.Input.AllowedDomains = append([]string(nil), allowedDomains...)
	out.Input.BlockedDomains = append([]string(nil), blockedDomains...)
	out.Execution.Method = string(webExecutionModeLocalFallback)
	out.Execution.CacheHit = cacheHit
	for _, block := range result.ContentBlocks {
		tb, ok := block.(types.TextBlock)
		if !ok {
			continue
		}
		if strings.HasPrefix(tb.Text, "websearch_progress=") {
			kind := strings.TrimPrefix(tb.Text, "websearch_progress=")
			if strings.HasPrefix(kind, "query_issued") {
				out.Progress = append(out.Progress, webSearchProgressEvent{Type: "query_issued", Query: query})
				continue
			}
			if strings.HasPrefix(kind, "results_received") {
				count := 0
				if idx := strings.Index(kind, "count="); idx >= 0 {
					count, _ = strconv.Atoi(strings.TrimSpace(kind[idx+len("count="):]))
				}
				out.Progress = append(out.Progress, webSearchProgressEvent{Type: "results_received", Count: count})
				continue
			}
			if strings.HasPrefix(kind, "started") {
				out.Progress = append(out.Progress, webSearchProgressEvent{Type: "started"})
			}
			continue
		}
		if strings.HasPrefix(tb.Text, "websearch_fallback_reason=") {
			out.Execution.FallbackReason = strings.TrimPrefix(tb.Text, "websearch_fallback_reason=")
			continue
		}
		if strings.HasPrefix(tb.Text, "websearch_result_json=") {
			var payload webSearchStructuredPayload
			if err := json.Unmarshal([]byte(strings.TrimPrefix(tb.Text, "websearch_result_json=")), &payload); err == nil {
				if payload.Method != "" {
					out.Execution.Method = payload.Method
				}
				if payload.FallbackReason != "" {
					out.Execution.FallbackReason = payload.FallbackReason
				}
				out.Results = out.Results[:0]
				for _, r := range payload.Results {
					out.Results = append(out.Results, webSearchNormalizedItem{Title: r.Title, URL: r.URL, Snippet: r.Snippet})
				}
			}
		}
	}
	return out
}

// WebSearchTool searches the web via DuckDuckGo and returns filtered results.
type WebSearchTool struct {
	cache            *searchCache
	httpClient       *http.Client // nil → create per-Execute
	instantAnswerURL string       // override for testing
	liteFallbackURL  string       // override for testing
	doInstantSearch  func(ctx context.Context, query string) ([]searchResult, error)
	doLiteSearch     func(ctx context.Context, query string) ([]searchResult, error)
	mode             webExecutionMode
	providerSearch   func(ctx context.Context, input WebSearchInput) (webSearchStructuredPayload, error)
	// serverTool is the optional Anthropic web_search_20250305 server-tool provider.
	serverTool WebSearchServerToolProvider
	// policy enforces the US-only region check + per-session rate limit.
	// Nil means permissive (legacy behaviour preserved for unit tests).
	policy *WebSearchPolicy

	// websearch-streaming-progress-events: optional callback the harness
	// wires to surface live progress chips ("Searching for: <query>", "N
	// results received") in TUI clients. Mirrors the TS onProgress
	// callback at WebSearchTool.ts:295-388. Nil = no progress events.
	OnProgress func(event WebSearchProgressEvent)

	// Domain restrictions (Task 7: security hardening).
	AllowedDomains    []string // nil = all allowed (whitelist)
	DisallowedDomains []string // these domains always blocked (blacklist)
}

// WebSearchProgressEvent is a single progress event emitted to the
// optional WebSearchTool.OnProgress callback.
type WebSearchProgressEvent struct {
	Type        string `json:"type"`
	ToolUseID   string `json:"toolUseID,omitempty"`
	Query       string `json:"query,omitempty"`
	ResultCount int    `json:"resultCount,omitempty"`
	Count       int    `json:"count,omitempty"`
}

// WebSearchOutput matches the TS tool output. Results contains string
// commentary/error entries or WebSearchOutputSearchResult values.
type WebSearchOutput struct {
	Query           string  `json:"query"`
	Results         []any   `json:"results"`
	DurationSeconds float64 `json:"durationSeconds"`
}

type WebSearchOutputSearchResult struct {
	ToolUseID string                `json:"tool_use_id"`
	Content   []WebSearchOutputLink `json:"content"`
}

type WebSearchOutputLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// NewWebSearchTool creates a WebSearchTool using the provided cache.
// If cache is nil, a new private cache is created.
func NewWebSearchTool(cache *searchCache) *WebSearchTool {
	if cache == nil {
		cache = NewSearchCache()
	}
	return &WebSearchTool{
		cache:            cache,
		instantAnswerURL: ddgInstantAnswerBase,
		liteFallbackURL:  ddgLiteBase,
		mode:             webExecutionModeLocalFallback,
	}
}

// SetPolicy installs a WebSearchPolicy that gates Execute via region and
// rate-limit checks. Pass nil (or call with a permissive policy) to disable
// gating; tests typically rely on the default nil policy.
func (w *WebSearchTool) SetPolicy(p *WebSearchPolicy) {
	w.policy = p
}

func (w *WebSearchTool) Name() string           { return "WebSearch" }
func (w *WebSearchTool) Aliases() []string      { return []string{"Search"} }
func (w *WebSearchTool) IsConcurrentSafe() bool { return true }

func (w *WebSearchTool) IsReadOnly() bool { return true }

func (w *WebSearchTool) ToolContract() types.ToolContract {
	outputSchema := types.StrictObjectSchema(map[string]any{
		"query":           map[string]any{"type": "string"},
		"results":         map[string]any{"type": "array"},
		"durationSeconds": map[string]any{"type": "number"},
	}, "query", "results", "durationSeconds")
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 100_000,
	}
}

func (w *WebSearchTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	return runtime.Features == nil || runtime.FeatureEnabled(types.ToolFeatureWebSearch)
}

func (w *WebSearchTool) ToAutoClassifierInput(input map[string]any) string {
	query, _ := input["query"].(string)
	return query
}

func (w *WebSearchTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(WebSearchOutput)
	if !ok {
		if pointer, pointerOK := data.(*WebSearchOutput); pointerOK && pointer != nil {
			output, ok = *pointer, true
		}
	}
	if !ok {
		return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: fmt.Sprint(data), Data: data}
	}

	var formatted strings.Builder
	formatted.WriteString(toolRuntimeFormat(i18n.KeyToolRuntimeWebSearchResultsForQuery, output.Query))
	formatted.WriteString("\n\n")
	for _, entry := range output.Results {
		switch value := entry.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				formatted.WriteString(strings.TrimSpace(value))
				formatted.WriteString("\n\n")
			}
		case WebSearchOutputSearchResult:
			appendWebSearchOutputLinks(&formatted, value.Content)
		case *WebSearchOutputSearchResult:
			if value != nil {
				appendWebSearchOutputLinks(&formatted, value.Content)
			}
		}
	}
	formatted.WriteString("\n")
	formatted.WriteString(SourcesReminder())
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   strings.TrimSpace(formatted.String()),
		Data:      output,
	}
}

func appendWebSearchOutputLinks(formatted *strings.Builder, links []WebSearchOutputLink) {
	if len(links) == 0 {
		formatted.WriteString(toolRuntimeText(i18n.KeyToolRuntimeWebSearchNoLinks))
		formatted.WriteString("\n\n")
		return
	}
	encoded, err := json.Marshal(links)
	if err != nil {
		formatted.WriteString(toolRuntimeText(i18n.KeyToolRuntimeWebSearchNoLinks))
		formatted.WriteString("\n\n")
		return
	}
	formatted.WriteString(toolRuntimeFormat(i18n.KeyToolRuntimeWebSearchLinks, encoded))
	formatted.WriteString("\n\n")
}

func (w *WebSearchTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, Search: true, ConcurrencySafe: true}
}

func (w *WebSearchTool) CheckPermissions(_ context.Context, _ map[string]any, _ types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	return types.ToolPermissionResult{
		Behavior: types.PermissionBehaviorPassthrough,
		Message:  toolPermissionText(i18n.KeyToolPermissionWebSearchRequired),
		Suggestions: []types.PermissionUpdate{{
			Type:        types.PermissionUpdateAddRules,
			Destination: types.PermissionDestinationLocalSettings,
			Behavior:    types.PermissionBehaviorAllow,
			Rules:       []types.PermissionRuleValue{{ToolName: "WebSearch"}},
		}},
	}, nil
}

// webSearchPromptTemplate mirrors getWebSearchPrompt() in
// src/tools/WebSearchTool/prompt.ts. The "%s" placeholder is filled with
// the current local "Month YYYY" string at render time so the model is
// reminded to use the current year in queries.
const webSearchPromptTemplate = `- Allows Claude to search the web and use the results to inform responses
- Provides up-to-date information for current events and recent data
- Returns search result information formatted as search result blocks, including links as markdown hyperlinks
- Use this tool for accessing information beyond Claude's knowledge cutoff
- Searches are performed automatically within a single API call

CRITICAL REQUIREMENT - You MUST follow this:
  - After answering the user's question, you MUST include a "Sources:" section at the end of your response
  - In the Sources section, list all relevant URLs from the search results as markdown hyperlinks: [Title](URL)
  - This is MANDATORY - never skip including sources in your response
  - Example format:

    [Your answer here]

    Sources:
    - [Source Title 1](https://example.com/1)
    - [Source Title 2](https://example.com/2)

Usage notes:
  - Domain filtering is supported to include or block specific websites
  - Web search is only available in the US

IMPORTANT - Use the correct year in search queries:
  - The current month is %s. You MUST use this year when searching for recent information, documentation, or current events.
  - Example: If the user asks for "latest React docs", search for "React documentation" with the current year, NOT last year`

// localMonthYear renders the current month/year (e.g., "May 2026") in the
// caller's local timezone, mirroring TS getLocalMonthYear().
func localMonthYear() string {
	return time.Now().Format("January 2006")
}

func (w *WebSearchTool) Description() string {
	return fmt.Sprintf(webSearchPromptTemplate, localMonthYear())
}

func (w *WebSearchTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"query": map[string]any{
				"type":        "string",
				"minLength":   2,
				"description": "The search query to use",
			},
			"allowed_domains": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Only include search results from these domains",
			},
			"blocked_domains": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Never include search results from these domains",
			},
		},
		"query",
	)
}

// ─── Parsing helpers ───────────────────────────────────────────────────────────

// DDG HTML (legacy html.duckduckgo.com) result patterns – kept for backward compat.
var (
	reDDGResult  = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*result__a[^"]*"[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	reDDGSnippet = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
)

// parseDDGResults parses the legacy DDG HTML page (html.duckduckgo.com) for results.
func parseDDGResults(html string) []searchResult {
	titleMatches := reDDGResult.FindAllStringSubmatch(html, -1)
	snippetMatches := reDDGSnippet.FindAllStringSubmatch(html, -1)

	var results []searchResult
	for i, m := range titleMatches {
		rawURL := m[1]
		title := strings.TrimSpace(reTag.ReplaceAllString(m[2], ""))

		// DDG wraps links in a redirect; extract the real URL from uddg param.
		if parsedURL, err := url.Parse(rawURL); err == nil {
			if uddg := parsedURL.Query().Get("uddg"); uddg != "" {
				rawURL = uddg
			}
		}

		snippet := ""
		if i < len(snippetMatches) {
			snippet = strings.TrimSpace(reTag.ReplaceAllString(snippetMatches[i][1], ""))
		}

		if title == "" || rawURL == "" {
			continue
		}
		results = append(results, searchResult{Title: title, URL: rawURL, Snippet: snippet})
	}
	return results
}

// reDDGLiteResult matches result links in the DDG lite page.
var reDDGLiteResult = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*result-link[^"]*"[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)

// parseDDGLiteHTML parses the lite.duckduckgo.com HTML page for result links.
func parseDDGLiteHTML(html string) []searchResult {
	matches := reDDGLiteResult.FindAllStringSubmatch(html, -1)
	var results []searchResult
	for _, m := range matches {
		rawURL := strings.TrimSpace(m[1])
		title := strings.TrimSpace(reTag.ReplaceAllString(m[2], ""))
		if rawURL == "" || title == "" {
			continue
		}
		results = append(results, searchResult{Title: title, URL: rawURL})
	}
	return results
}

// parseInstantAnswer converts a DDG Instant Answer API response to search results.
func parseInstantAnswer(resp ddgInstantResponse) []searchResult {
	var results []searchResult
	if resp.AbstractText != "" && resp.AbstractURL != "" {
		results = append(results, searchResult{
			Title:   resp.AbstractURL,
			URL:     resp.AbstractURL,
			Snippet: resp.AbstractText,
		})
	}
	for _, topic := range resp.RelatedTopics {
		if topic.Text != "" && topic.FirstURL != "" {
			results = append(results, searchResult{
				Title:   topic.Text,
				URL:     topic.FirstURL,
				Snippet: topic.Text,
			})
		}
		// Handle nested topic groups.
		for _, sub := range topic.Topics {
			if sub.Text != "" && sub.FirstURL != "" {
				results = append(results, searchResult{
					Title:   sub.Text,
					URL:     sub.FirstURL,
					Snippet: sub.Text,
				})
			}
		}
	}
	return results
}

// ─── Tool-level domain restrictions (Task 7) ──────────────────────────────────

// matchDomain checks if host matches a domain pattern.
// Supports wildcards: "*.example.com" matches "sub.example.com" and "example.com".
func matchDomain(host, pattern string) bool {
	host = strings.ToLower(host)
	pattern = strings.ToLower(pattern)
	if strings.HasPrefix(pattern, "*.") {
		// Wildcard: *.example.com matches sub.example.com and example.com
		suffix := pattern[1:] // ".example.com"
		return strings.HasSuffix(host, suffix) || host == pattern[2:]
	}
	return host == pattern
}

type webDomainPolicyErrorKind uint8

const (
	webDomainInvalidURL webDomainPolicyErrorKind = iota + 1
	webDomainMissingHost
	webDomainBlocked
	webDomainNotAllowed
)

type webDomainPolicyError struct {
	kind  webDomainPolicyErrorKind
	host  string
	cause error
}

func (e *webDomainPolicyError) Error() string {
	switch e.kind {
	case webDomainInvalidURL:
		return fmt.Sprintf("invalid URL: %v", e.cause)
	case webDomainMissingHost:
		return "URL has no host"
	case webDomainBlocked:
		return fmt.Sprintf("domain %q is blocked by policy", e.host)
	case webDomainNotAllowed:
		return fmt.Sprintf("domain %q is not in the allowed list", e.host)
	default:
		return "domain is blocked by policy"
	}
}

func (e *webDomainPolicyError) Unwrap() error { return e.cause }

func localizedWebDomainPolicyError(err error) string {
	var policyErr *webDomainPolicyError
	if !errors.As(err, &policyErr) {
		return toolPermissionText(i18n.KeyToolPermissionWebInvalidURL)
	}
	switch policyErr.kind {
	case webDomainBlocked:
		return toolPermissionFormat(i18n.KeyToolPermissionWebDomainBlocked, policyErr.host)
	case webDomainNotAllowed:
		return toolPermissionFormat(i18n.KeyToolPermissionWebDomainNotAllowed, policyErr.host)
	default:
		return toolPermissionText(i18n.KeyToolPermissionWebInvalidURL)
	}
}

func localizedWebDomainRuntimeError(err error) string {
	var policyErr *webDomainPolicyError
	if !errors.As(err, &policyErr) {
		return toolRuntimeFormat(i18n.KeyToolRuntimeWebError, err)
	}
	switch policyErr.kind {
	case webDomainBlocked:
		return toolRuntimeFormat(i18n.KeyToolRuntimeWebError,
			toolPermissionFormat(i18n.KeyToolPermissionWebDomainBlocked, policyErr.host))
	case webDomainNotAllowed:
		return toolRuntimeFormat(i18n.KeyToolRuntimeWebError,
			toolPermissionFormat(i18n.KeyToolPermissionWebDomainNotAllowed, policyErr.host))
	default:
		return toolRuntimeFormat(i18n.KeyToolRuntimeWebError,
			toolPermissionText(i18n.KeyToolPermissionWebInvalidURL))
	}
}

// checkDomainAllowed checks if the given URL's domain is permitted.
// Returns an error if the domain is blocked by the tool-level restrictions.
// disallowed is checked first (blacklist), then allowed (whitelist).
// If allowed is nil, all non-disallowed domains are permitted.
func checkDomainAllowed(rawURL string, allowed, disallowed []string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return &webDomainPolicyError{kind: webDomainInvalidURL, cause: err}
	}
	host := u.Hostname()
	if host == "" {
		return &webDomainPolicyError{kind: webDomainMissingHost}
	}

	// Check disallowed first (blacklist)
	for _, d := range disallowed {
		if matchDomain(host, d) {
			return &webDomainPolicyError{kind: webDomainBlocked, host: host}
		}
	}

	// Check allowed (whitelist) — nil means all allowed
	if allowed != nil {
		for _, d := range allowed {
			if matchDomain(host, d) {
				return nil
			}
		}
		return &webDomainPolicyError{kind: webDomainNotAllowed, host: host}
	}

	return nil
}

// ─── Domain filtering (per-query, from tool input) ─────────────────────────────

func domainOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	// Strip leading www.
	host = strings.TrimPrefix(host, "www.")
	return host
}

func filterResults(results []searchResult, allowed, blocked []string) []searchResult {
	var out []searchResult
	for _, r := range results {
		// websearch-null-result-guard: skip entries whose URL is empty
		// (compaction round-trips can introduce {url:"",title:""} stubs).
		if strings.TrimSpace(r.URL) == "" {
			continue
		}
		d := domainOf(r.URL)
		if len(allowed) > 0 {
			found := false
			for _, a := range allowed {
				if d == strings.ToLower(a) || strings.HasSuffix(d, "."+strings.ToLower(a)) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		skip := false
		for _, b := range blocked {
			if d == strings.ToLower(b) || strings.HasSuffix(d, "."+strings.ToLower(b)) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, r)
		}
	}
	return out
}

// ─── Execute ───────────────────────────────────────────────────────────────────

func (w *WebSearchTool) client() *http.Client {
	if w.httpClient != nil {
		return w.httpClient
	}
	return newHTTPClient(15 * time.Second)
}

func (w *WebSearchTool) doInstantAnswer(ctx context.Context, query string) ([]searchResult, error) {
	apiURL := w.instantAnswerURL + "?q=" + url.QueryEscape(query) + "&format=json&no_html=1&skip_disambig=1"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := w.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}

	var apiResp ddgInstantResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}
	return parseInstantAnswer(apiResp), nil
}

func (w *WebSearchTool) doLiteFallback(ctx context.Context, query string) ([]searchResult, error) {
	liteURL := w.liteFallbackURL + "?q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, liteURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := w.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}
	return parseDDGLiteHTML(string(body)), nil
}

func makeWebSearchCacheKey(query string, allowedDomains, blockedDomains []string) string {
	normalize := func(in []string) []string {
		out := make([]string, 0, len(in))
		for _, v := range in {
			v = strings.ToLower(strings.TrimSpace(v))
			if v != "" {
				out = append(out, v)
			}
		}
		sort.Strings(out)
		return out
	}
	parts := []string{strings.ToLower(strings.TrimSpace(query))}
	parts = append(parts, "allowed="+strings.Join(normalize(allowedDomains), ","))
	parts = append(parts, "blocked="+strings.Join(normalize(blockedDomains), ","))
	return strings.Join(parts, "|")
}

// emitProgress invokes OnProgress when configured. Safe to call when nil.
func (w *WebSearchTool) emitProgress(event WebSearchProgressEvent) {
	if w == nil || w.OnProgress == nil {
		return
	}
	if event.ResultCount == 0 && event.Count != 0 {
		event.ResultCount = event.Count
	}
	w.OnProgress(event)
}

func (w *WebSearchTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	started := time.Now()
	in, toolErr := parseStrictInputOrError[WebSearchInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	if in.Query == "" {
		return webSearchValidationResult(toolRuntimeText(i18n.KeyToolRuntimeWebSearchQueryRequired), 1), nil
	}
	if utf8.RuneCountInString(in.Query) < 2 {
		return webSearchValidationResult(toolRuntimeText(i18n.KeyToolRuntimeWebSearchQueryTooShort), 1), nil
	}
	if len(in.AllowedDomains) > 0 && len(in.BlockedDomains) > 0 {
		return webSearchValidationResult(toolRuntimeText(i18n.KeyToolRuntimeWebSearchConflictingDomainFilters), 2), nil
	}
	if err := validateWebSearchDomainList(in.AllowedDomains, "allowed_domains"); err != nil {
		return webSearchValidationResult(err.Error(), 1), nil
	}
	if err := validateWebSearchDomainList(in.BlockedDomains, "blocked_domains"); err != nil {
		return webSearchValidationResult(err.Error(), 1), nil
	}

	// Policy gate (region + rate-limit). Mirrors TS WebSearchTool isEnabled
	// path: blocked region returns the verbatim TS error message; rate-limit
	// returns a transient error with the per-minute ceiling embedded.
	if w.policy != nil {
		if err := w.policy.Allow(); err != nil {
			return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebError, err), IsError: true}, nil
		}
	}

	// Anthropic web_search_20250305 server tool path. When a provider is
	// configured we prefer it because the response carries first-party
	// citation metadata that the local DDG fallback can't produce.
	if w.serverTool != nil && w.mode == webExecutionModeProviderNative {
		payload, err := runWebSearchServerTool(ctx, w.serverTool, in, w.emitProgress)
		if err == nil {
			return buildWebSearchOutputResult(payload.Output, payload.Results, "", webExecutionModeProviderNative, payload.Usage), nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return types.ToolResult{}, err
		}
		if !errors.Is(err, ErrWebSearchServerToolUnavailable) {
			return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebError, err), IsError: true}, nil
		}
	}

	if w.mode == webExecutionModeProviderNative && w.providerSearch != nil {
		payload, err := w.providerSearch(ctx, in)
		if err != nil {
			return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebError, err), IsError: true}, nil
		}
		output := payload.Output
		if output.Query == "" {
			output = webSearchOutputFromResults(in.Query, payload.Results, time.Since(started).Seconds(), "")
		}
		return buildWebSearchOutputResult(output, payload.Results, payload.FallbackReason, webExecutionModeProviderNative, payload.Usage), nil
	}

	cacheKey := makeWebSearchCacheKey(in.Query, in.AllowedDomains, in.BlockedDomains)
	if w.cache != nil {
		if cachedResults, ok := w.cache.getSearchResults(cacheKey); ok {
			output := webSearchOutputFromResults(in.Query, cachedResults, time.Since(started).Seconds(), "")
			return buildWebSearchOutputResult(output, cachedResults, "", webExecutionModeLocalFallback, nil), nil
		}
	}

	searchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	fallbackReason := ""

	// Try the official Instant Answer API first.
	instantSearch := w.doInstantAnswer
	if w.doInstantSearch != nil {
		instantSearch = w.doInstantSearch
	}
	results, err := instantSearch(searchCtx, in.Query)
	if err != nil || len(results) == 0 {
		if err != nil {
			fallbackReason = "instant_answer_error"
		} else {
			fallbackReason = "instant_answer_empty"
		}
		// Fall back to the lite HTML page.
		liteSearch := w.doLiteFallback
		if w.doLiteSearch != nil {
			liteSearch = w.doLiteSearch
		}
		results, err = liteSearch(searchCtx, in.Query)
		if err != nil {
			return types.ToolResult{
				Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebSearchFailed, err),
				IsError: true,
			}, nil
		}
	}

	// Apply domain filtering.
	// Merge tool-level restrictions with per-query filters.
	effectiveAllowed := in.AllowedDomains
	if len(w.AllowedDomains) > 0 {
		if len(effectiveAllowed) > 0 {
			// Intersect: keep only per-query domains that are also in tool-level allowed list.
			var merged []string
			for _, qa := range effectiveAllowed {
				for _, ta := range w.AllowedDomains {
					if matchDomain(strings.ToLower(qa), ta) {
						merged = append(merged, qa)
						break
					}
				}
			}
			effectiveAllowed = merged
		} else {
			effectiveAllowed = w.AllowedDomains
		}
	}
	effectiveBlocked := in.BlockedDomains
	if len(w.DisallowedDomains) > 0 {
		effectiveBlocked = append(effectiveBlocked, w.DisallowedDomains...)
	}
	results = filterResults(results, effectiveAllowed, effectiveBlocked)

	if len(results) == 0 {
		output := WebSearchOutput{Query: in.Query, Results: make([]any, 0), DurationSeconds: time.Since(started).Seconds()}
		return buildWebSearchOutputResult(output, nil, fallbackReason, webExecutionModeLocalFallback, nil), nil
	}
	if len(results) > 10 {
		results = results[:10]
	}

	if w.cache != nil {
		w.cache.setSearchResults(cacheKey, results)
	}
	output := webSearchOutputFromResults(in.Query, results, time.Since(started).Seconds(), "")
	return buildWebSearchOutputResult(output, results, fallbackReason, webExecutionModeLocalFallback, nil), nil
}

func webSearchValidationResult(message string, code int) types.ToolResult {
	return types.ToolResult{
		Content: message,
		IsError: true,
		Metadata: map[string]string{
			"errorCode": strconv.Itoa(code),
		},
	}
}

// upgradeHTTPToHTTPS rewrites an http://… URL to https://… so callers
// don't accidentally fetch over plaintext. Mirrors the upgrade in
// src/tools/WebFetchTool/utils.ts upgradeUrlToHttps.
func upgradeHTTPToHTTPS(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return rawURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return rawURL
	}
	if strings.EqualFold(parsed.Scheme, "http") {
		parsed.Scheme = "https"
		return parsed.String()
	}
	return trimmed
}

// sameOriginRedirect reports whether the redirect from src to dst stays on
// the same logical host (allowing only the www. add/remove). Mirrors
// isPermittedRedirect in TS so the Go implementation has identical
// permissions when following same-origin redirects.
func sameOriginRedirect(src, dst string) bool {
	source, sourceErr := url.Parse(src)
	target, targetErr := url.Parse(dst)
	if sourceErr != nil || targetErr != nil || source.Hostname() == "" || target.Hostname() == "" {
		return false
	}
	if !strings.EqualFold(source.Scheme, target.Scheme) || normalizedRedirectPort(source) != normalizedRedirectPort(target) || redirectHasCredentials(target) {
		return false
	}
	stripWWW := func(host string) string {
		return strings.TrimPrefix(normalizePreapprovedHost(host), "www.")
	}
	return stripWWW(source.Hostname()) == stripWWW(target.Hostname())
}

func normalizedRedirectPort(parsed *url.URL) string {
	port := parsed.Port()
	if (strings.EqualFold(parsed.Scheme, "https") && port == "443") || (strings.EqualFold(parsed.Scheme, "http") && port == "80") {
		return ""
	}
	return port
}

func redirectHasCredentials(parsed *url.URL) bool {
	if parsed == nil || parsed.User == nil {
		return false
	}
	if parsed.User.Username() != "" {
		return true
	}
	password, _ := parsed.User.Password()
	return password != ""
}

// HasWebSearchPolicy reports whether a policy has been installed.
func (w *WebSearchTool) HasWebSearchPolicy() bool {
	return w != nil && w.policy != nil
}

// validateWebSearchDomainList rejects scheme prefixes, empty entries, and
// embedded paths in the allowed_domains/blocked_domains arrays. The TS
// reference treats these as schema validation errors; the Go side used to
// forward them raw, producing dead filter rules downstream.
func validateWebSearchDomainList(values []string, fieldName string) error {
	for i, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			return errors.New(toolRuntimeFormat(i18n.KeyToolRuntimeWebSearchDomainEmpty, fieldName, i))
		}
		if strings.Contains(v, "://") {
			return errors.New(toolRuntimeFormat(i18n.KeyToolRuntimeWebSearchDomainScheme, fieldName, i, raw))
		}
		if strings.HasPrefix(strings.ToLower(v), "http:") || strings.HasPrefix(strings.ToLower(v), "https:") {
			return errors.New(toolRuntimeFormat(i18n.KeyToolRuntimeWebSearchDomainScheme, fieldName, i, raw))
		}
		if strings.ContainsAny(v, " \t\n\r") {
			return errors.New(toolRuntimeFormat(i18n.KeyToolRuntimeWebSearchDomainWhitespace, fieldName, i, raw))
		}
		if strings.HasPrefix(v, "/") {
			return errors.New(toolRuntimeFormat(i18n.KeyToolRuntimeWebSearchDomainLeadingSlash, fieldName, i, raw))
		}
	}
	return nil
}
