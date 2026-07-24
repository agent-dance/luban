// Package tools — Sources/citations rendering for WebSearch.
//
// Mirrors mapToolResultToToolResultBlockParam from
// src/tools/WebSearchTool/WebSearchTool.ts. The Go side also has to
// produce a "Web search results for query: ..." block with a JSON-encoded
// list of links and the SOURCES reminder at the tail so the model
// reliably emits a Sources: section.
package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// SourcesReminder returns the localized trailing instruction appended to every
// web search result block. Its function form follows runtime language switches.
func SourcesReminder() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebSearchSourcesReminder)
}

// FormatWebSearchToolResult builds the formatted text that ships back as
// the tool_result content for a search call. Output matches:
//
//	Web search results for query: "<query>"
//
//	Links: [{...}]
//
//	REMINDER: ...
//
// Empty result sets are still annotated with the sources reminder so the
// model knows zero hits is a valid answer rather than a missing block.
func FormatWebSearchToolResult(query string, results []WebSearchResult) string {
	var sb strings.Builder
	sb.WriteString(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebSearchResultsHeader, query))

	links := make([]map[string]string, 0, len(results))
	for _, r := range results {
		entry := map[string]string{
			"url":   r.URL,
			"title": r.Title,
		}
		links = append(links, entry)
	}
	if len(links) > 0 {
		encoded, err := json.Marshal(links)
		if err != nil {
			encoded = []byte("[]")
		}
		sb.WriteString(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebSearchLinks, string(encoded)))
	}

	sb.WriteString(SourcesReminder())
	return strings.TrimSpace(sb.String())
}

// FormatSourcesSection renders a markdown "Sources:" footer for callers
// that prefer to compose the assistant message themselves. Returns ""
// when results is empty.
func FormatSourcesSection(results []WebSearchResult) string {
	if len(results) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebSearchSourcesHeading))
	for _, r := range results {
		title := r.Title
		if title == "" {
			title = r.URL
		}
		fmt.Fprintf(&sb, "- [%s](%s)\n", title, r.URL)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// CitationBlocksJSON serialises the canonical block list as a JSON
// string, suitable for stuffing into a TextBlock for downstream parsing.
func CitationBlocksJSON(block WebSearchToolResultBlock) string {
	encoded, err := json.Marshal(block)
	if err != nil {
		return `{"type":"web_search_tool_result","content":[]}`
	}
	return string(encoded)
}
