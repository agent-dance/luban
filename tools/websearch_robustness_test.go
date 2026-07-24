// Package tools — robustness tests for filterResults null guard
// (websearch-null-result-guard).
package tools

import "testing"

func TestFilterResults_SkipsEmptyURL(t *testing.T) {
	results := []searchResult{
		{Title: "ok", URL: "https://example.com/a", Snippet: "x"},
		{Title: "stub", URL: "", Snippet: ""},
		{Title: "ok2", URL: "https://example.com/b", Snippet: "y"},
		{Title: "spaces", URL: "   ", Snippet: ""},
	}
	out := filterResults(results, nil, nil)
	if len(out) != 2 {
		t.Fatalf("expected 2 valid results after null guard; got %d: %+v", len(out), out)
	}
	for _, r := range out {
		if r.URL == "" {
			t.Errorf("empty-URL result leaked through: %+v", r)
		}
	}
}
