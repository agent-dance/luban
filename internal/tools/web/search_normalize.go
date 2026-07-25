// Package tools — canonical WebSearch server-tool result types.
package web

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// WebSearchSnippetCap mirrors the 280-char limit specified in the task
// acceptance criteria. Snippets longer than this are cut and suffixed
// with an ellipsis.
const WebSearchSnippetCap = 280

// WebSearchResult is the canonical provider-native result.
type WebSearchResult struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Snippet   string `json:"snippet,omitempty"`
	CitedText string `json:"cited_text,omitempty"`
	PageAge   string `json:"page_age,omitempty"`
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
