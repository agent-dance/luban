package web

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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

// ─── HTTP helpers ──────────────────────────────────────────────────────────────

const (
	maxBodyBytes             = 10 << 20 // 10 MB (TS WebFetchTool body cap)
	userAgent                = "LubanCode/1.0"
	maxRedirects             = 10
	webFetchMaxAttempts      = 2
	webFetchDefaultRetryWait = 100 * time.Millisecond
)

// WebFetchTimeout aligns with TS WebFetchTool.fetchTimeout=60s. Servers
// behind CDN edge caches sometimes need >30s for first-byte; matching the
// TS budget avoids spurious "fetch failed" surfacing for slow but reachable
// targets.
const WebFetchTimeout = 60 * time.Second

func isHTML(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml+xml")
}

// ─── WebFetchTool ──────────────────────────────────────────────────────────────

// WebFetchTool fetches a URL and returns its text content.
type WebFetchTool struct {
	fetchCache *WebFetchCache
	httpClient *http.Client

	// Summariser is the small/fast secondary model used to apply the user
	// prompt to fetched markdown (mirrors src/tools/WebFetchTool/utils.ts:484-530
	// applyPromptToMarkdown).
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

// NewWebFetchTool creates the canonical local-HTTP WebFetch tool.
func NewWebFetchTool(cache *WebFetchCache) *WebFetchTool {
	if cache == nil {
		cache = NewWebFetchCache()
	}
	return &WebFetchTool{fetchCache: cache, DomainInfoEndpoint: domainInfoDefaultEndpoint}
}

// FetchCache exposes the session-owned cache for lifecycle wiring.
func (w *WebFetchTool) FetchCache() *WebFetchCache {
	if w == nil {
		return nil
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

func (w *WebFetchTool) Name() string { return "WebFetch" }

func (w *WebFetchTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
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
	Results []searchResult
	Output  WebSearchOutput `json:"-"`
	Usage   *types.Usage    `json:"-"`
}

func buildWebFetchResult(output WebFetchOutput) types.ToolResult {
	return types.ToolResult{
		Content: output.Result,
		Data:    output,
		Metadata: map[string]string{
			"bytes":      strconv.Itoa(output.Bytes),
			"code":       strconv.Itoa(output.Code),
			"codeText":   output.CodeText,
			"durationMs": strconv.FormatInt(output.DurationMs, 10),
			"url":        output.URL,
			"method":     "local_http",
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

func buildWebSearchOutputResult(output WebSearchOutput, results []searchResult, usage *types.Usage) types.ToolResult {
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
			"method":          "provider_native",
		},
	}
}

func (w *WebFetchTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	started := time.Now()
	in, toolErr := toolbase.ParseStrictInputOrError[WebFetchInput](input)
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

	originalURL := in.URL
	cache := w.FetchCache()
	cacheKey := cache.MakeKey(originalURL)
	if cached, ok := cache.Get(cacheKey); ok {
		return w.applyWebFetchPrompt(ctx, in, cached, started)
	}

	// HTTP→HTTPS auto-upgrade ensures fetches do not silently use plaintext.
	if upgraded := upgradeHTTPToHTTPS(in.URL); upgraded != in.URL {
		in.URL = upgraded
	}
	if err := validateURL(in.URL); err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebFetchBlockedURL, err), IsError: true}, nil
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
	client := w.httpClient
	if client == nil {
		client = newSSRFSafeHTTPClient(WebFetchTimeout, maxRedirects)
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
		if err := validateURL(redirectURL); err != nil {
			return fmt.Errorf("%s: %w", toolRuntimeText(i18n.KeyToolRuntimeWebFetchRedirectBlockedSSRF), err)
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
		}), nil
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
	case isHTML(contentType):
		content = HTMLToMarkdownDOM(content)
	}
	content = strings.TrimSpace(content)

	entry := WebFetchCacheEntry{
		Body:          content,
		ContentType:   contentType,
		CacheSize:     len(content),
		StatusCode:    resp.StatusCode,
		StatusText:    httpResponseStatusText(resp),
		Bytes:         len(body),
		PersistedPath: persistedPath,
		PersistedSize: persistedSize,
	}
	cache.Set(cacheKey, entry)
	return w.applyWebFetchPrompt(ctx, WebFetchInput{URL: originalURL, Prompt: in.Prompt}, entry, started)
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

func (w *WebFetchTool) applyWebFetchPrompt(ctx context.Context, input WebFetchInput, entry WebFetchCacheEntry, started time.Time) (types.ToolResult, error) {
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
	}), nil
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
	dir := os.Getenv("LUBAN_WEBFETCH_BINARY_DIR")
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join(os.TempDir(), "luban-webfetch")
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

// searchResult represents a single web search result.
type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

// WebSearchTool searches through the active provider's native server tool.
type WebSearchTool struct {
	serverTool WebSearchServerToolProvider

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

func NewWebSearchTool() *WebSearchTool { return &WebSearchTool{} }

func (w *WebSearchTool) Name() string { return "WebSearch" }

func (w *WebSearchTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	return runtime.Features == nil || runtime.FeatureEnabled(types.ToolFeatureWebSearch)
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
const webSearchPromptTemplate = `- Allows Luban to search the web and use the results to inform responses
- Provides up-to-date information for current events and recent data
- Returns search result information formatted as search result blocks, including links as markdown hyperlinks
- Use this tool for accessing information beyond Luban's knowledge cutoff
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

// ─── Execute ───────────────────────────────────────────────────────────────────

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
	in, toolErr := toolbase.ParseStrictInputOrError[WebSearchInput](input)
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

	if len(in.AllowedDomains) == 0 && len(w.AllowedDomains) > 0 {
		in.AllowedDomains = append([]string(nil), w.AllowedDomains...)
	}
	in.BlockedDomains = append(in.BlockedDomains, w.DisallowedDomains...)
	payload, err := runWebSearchServerTool(ctx, w.serverTool, in, w.emitProgress)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return types.ToolResult{}, err
		}
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWebError, err), IsError: true}, nil
	}
	return buildWebSearchOutputResult(payload.Output, payload.Results, payload.Usage), nil
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
