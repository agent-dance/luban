package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatWebSearchToolResult_IncludesQueryAndReminder(t *testing.T) {
	res := []WebSearchResult{
		{Title: "Go", URL: "https://go.dev", Snippet: "The Go Programming Language"},
	}
	got := FormatWebSearchToolResult("golang", res)
	if !strings.HasPrefix(got, `Web search results for query: "golang"`) {
		t.Fatalf("missing query header: %q", got)
	}
	if !strings.Contains(got, SourcesReminder()) {
		t.Fatalf("missing sources reminder: %q", got)
	}
	if !strings.Contains(got, "Links: ") {
		t.Fatalf("missing Links section: %q", got)
	}
}

func TestFormatWebSearchToolResult_LinksAreJSON(t *testing.T) {
	res := []WebSearchResult{
		{Title: "Go", URL: "https://go.dev", Snippet: "lang"},
		{Title: "", URL: "https://example.com"},
	}
	got := FormatWebSearchToolResult("q", res)
	idx := strings.Index(got, "Links: ")
	if idx < 0 {
		t.Fatalf("no Links header")
	}
	tail := got[idx+len("Links: "):]
	end := strings.Index(tail, "\n\n")
	if end < 0 {
		t.Fatalf("Links payload not terminated: %q", tail)
	}
	payload := tail[:end]
	var parsed []map[string]string
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		t.Fatalf("Links not valid JSON: %v\npayload=%q", err, payload)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(parsed))
	}
	if parsed[0]["url"] != "https://go.dev" || parsed[0]["title"] != "Go" {
		t.Fatalf("unexpected first entry: %+v", parsed[0])
	}
}

func TestFormatWebSearchToolResult_EmptyResultsStillCarriesReminder(t *testing.T) {
	got := FormatWebSearchToolResult("nothing", nil)
	if !strings.Contains(got, SourcesReminder()) {
		t.Fatalf("reminder dropped on empty results")
	}
	if strings.Contains(got, "Links:") || !strings.HasPrefix(got, `Web search results for query: "nothing"`) {
		t.Fatalf("empty TS output must contain only header/reminder, got %q", got)
	}
}

func TestSourcesReminder_Verbatim(t *testing.T) {
	want := "REMINDER: You MUST include the sources above in your response to the user using markdown hyperlinks."
	if got := SourcesReminder(); got != want {
		t.Fatalf("SourcesReminder text drift:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestFormatSourcesSection_RendersBullets(t *testing.T) {
	res := []WebSearchResult{
		{Title: "Go", URL: "https://go.dev"},
		{Title: "", URL: "https://example.com"},
	}
	got := FormatSourcesSection(res)
	if !strings.HasPrefix(got, "Sources:\n") {
		t.Fatalf("missing Sources header: %q", got)
	}
	if !strings.Contains(got, "- [Go](https://go.dev)") {
		t.Fatalf("missing first bullet: %q", got)
	}
	if !strings.Contains(got, "- [https://example.com](https://example.com)") {
		t.Fatalf("title fallback to URL missing: %q", got)
	}
}

func TestFormatSourcesSection_EmptyReturnsEmpty(t *testing.T) {
	if got := FormatSourcesSection(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestCitationBlocksJSON_Roundtrip(t *testing.T) {
	block := WebSearchToolResultBlock{
		Type: "web_search_tool_result",
		Content: []WebSearchResultBlock{
			{URL: "https://go.dev", Title: "Go", Snippet: "lang"},
		},
	}
	encoded := CitationBlocksJSON(block)
	var parsed WebSearchToolResultBlock
	if err := json.Unmarshal([]byte(encoded), &parsed); err != nil {
		t.Fatalf("citation JSON not parseable: %v", err)
	}
	if parsed.Type != "web_search_tool_result" || len(parsed.Content) != 1 {
		t.Fatalf("citation block lost shape: %+v", parsed)
	}
}
