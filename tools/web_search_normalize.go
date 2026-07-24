// Package tools — normalisation of legacy WebSearch provider results into
// the same citation-block shape the Anthropic server tool emits, so
// downstream consumers (renderers, normalised-test fixtures) don't have
// to branch on provider.
package tools

import (
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// WebSearchSnippetCap mirrors the 280-char limit specified in the task
// acceptance criteria. Snippets longer than this are cut and suffixed
// with an ellipsis.
const WebSearchSnippetCap = 280

// WebSearchResult is the canonical struct produced by both the server
// tool path and any local provider. Title/URL/Snippet are the user-
// visible fields; FetchedAt and ProviderID are metadata used by tests
// and analytics.
type WebSearchResult struct {
	Title      string    `json:"title"`
	URL        string    `json:"url"`
	Snippet    string    `json:"snippet,omitempty"`
	CitedText  string    `json:"cited_text,omitempty"`
	PageAge    string    `json:"page_age,omitempty"`
	FetchedAt  time.Time `json:"fetched_at,omitempty"`
	ProviderID string    `json:"provider_id,omitempty"`
}

// WebSearchToolResultBlock matches the TS server-tool block shape
// `{ type: 'web_search_tool_result', content: [...] }` so the Go
// renderer can emit identical citation blocks.
type WebSearchToolResultBlock struct {
	Type      string                 `json:"type"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Content   []WebSearchResultBlock `json:"content"`
}

// GetType reports the ContentBlock type marker. The Anthropic server tool
// emits `web_search_tool_result` blocks; we surface that string verbatim
// so the typed block round-trips through ContentBlock-aware code.
func (b WebSearchToolResultBlock) GetType() types.ContentType {
	return types.ContentType("web_search_tool_result")
}

// WebSearchResultBlock is a single normalised citation block.
type WebSearchResultBlock struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	Snippet   string `json:"snippet,omitempty"`
	CitedText string `json:"cited_text,omitempty"`
	PageAge   string `json:"page_age,omitempty"`
}

// NormalizeWebSearchResults converts a heterogeneous list of provider
// results into the canonical block shape, capping snippets at the
// configured limit and dropping entries with empty URLs.
func NormalizeWebSearchResults(results []WebSearchResult) WebSearchToolResultBlock {
	out := WebSearchToolResultBlock{
		Type:    "web_search_tool_result",
		Content: make([]WebSearchResultBlock, 0, len(results)),
	}
	for _, r := range results {
		url := strings.TrimSpace(r.URL)
		if url == "" {
			continue
		}
		title := strings.TrimSpace(r.Title)
		if title == "" {
			title = url
		}
		out.Content = append(out.Content, WebSearchResultBlock{
			URL:       url,
			Title:     title,
			Snippet:   capSnippet(strings.TrimSpace(r.Snippet)),
			CitedText: strings.TrimSpace(r.CitedText),
			PageAge:   strings.TrimSpace(r.PageAge),
		})
	}
	return out
}

// SearchResultsToWebSearchResults adapts the in-package searchResult
// slice (used by the local DDG provider) to the canonical struct.
// providerID identifies where the data came from for analytics/audit.
func SearchResultsToWebSearchResults(results []searchResult, providerID string, fetchedAt time.Time) []WebSearchResult {
	out := make([]WebSearchResult, 0, len(results))
	for _, r := range results {
		out = append(out, WebSearchResult{
			Title:      strings.TrimSpace(r.Title),
			URL:        strings.TrimSpace(r.URL),
			Snippet:    capSnippet(strings.TrimSpace(r.Snippet)),
			FetchedAt:  fetchedAt,
			ProviderID: providerID,
		})
	}
	return out
}

// WebSearchToolResultError is the alternate envelope the Anthropic server
// returns when web_search_tool_result.content is an error object instead
// of an array. Mirrors src/tools/WebSearchTool/WebSearchTool.ts:115-122
// where the TS reference detects this shape and surfaces a distinct
// "Web search error: <error_code>" message instead of crashing the
// array-only parser.
type WebSearchToolResultError struct {
	Type      string `json:"type"`
	ErrorCode string `json:"error_code"`
}

// IsWebSearchToolResultErrorContent reports whether the supplied raw
// JSON value (as decoded into an interface{}) represents the error
// envelope rather than the success array.
func IsWebSearchToolResultErrorContent(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	t, _ := m["type"].(string)
	return t == "web_search_tool_result_error"
}

// FormatWebSearchToolResultError renders the error envelope into the
// human-readable string the model sees on a transient web_search
// failure (rate limit, upstream error, etc.).
func FormatWebSearchToolResultError(err WebSearchToolResultError) string {
	code := strings.TrimSpace(err.ErrorCode)
	if code == "" {
		code = "unknown"
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebSearchError, code)
}

func capSnippet(s string) string {
	if s == "" {
		return ""
	}
	// Operate on runes so Unicode characters aren't sliced mid-codepoint.
	r := []rune(s)
	if len(r) <= WebSearchSnippetCap {
		return s
	}
	cut := WebSearchSnippetCap
	// Try to break on a word boundary so the truncation reads naturally.
	for i := cut; i > cut-32 && i > 0; i-- {
		if r[i] == ' ' {
			cut = i
			break
		}
	}
	return string(r[:cut]) + "…"
}
