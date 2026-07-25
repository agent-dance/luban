// Package tools — Sources/citations rendering for WebSearch.
//
// Mirrors mapToolResultToToolResultBlockParam from
// src/tools/WebSearchTool/WebSearchTool.ts. The Go side also has to
// produce a "Web search results for query: ..." block with a JSON-encoded
// list of links and the SOURCES reminder at the tail so the model
// reliably emits a Sources: section.
package web

import (
	"github.com/agent-dance/luban/i18n"
)

// SourcesReminder returns the localized trailing instruction appended to every
// web search result block. Its function form follows runtime language switches.
func SourcesReminder() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebSearchSourcesReminder)
}
