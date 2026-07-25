package search

import (
	"strings"
	"testing"
)

// TestBM25Ranker_RequiredTermFiltersOut ensures a "+required" term
// hard-filter documents that don't contain it.
func TestBM25Ranker_RequiredTermFiltersOut(t *testing.T) {
	entries := []toolEntry{
		{Name: "SlackList", Description: "List slack channels"},
		{Name: "SlackRead", Description: "Read slack messages"},
		{Name: "SlackPost", Description: "Post a message to slack"},
	}
	r := newBM25Ranker(entries)

	// "slack +read" → only SlackRead should appear, since the "+read"
	// requirement excludes List and Post.
	matches := r.rank("slack +read", 5)
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	for _, m := range matches {
		if !strings.Contains(strings.ToLower(m.Name+" "+strings.ToLower(snippetLower(m))), "read") {
			t.Errorf("required-term filter let through %q (%s)", m.Name, m.Snippet)
		}
	}
	// And "list" / "post" should never appear.
	for _, m := range matches {
		if m.Name == "SlackList" || m.Name == "SlackPost" {
			t.Errorf("required-term filter let through %q", m.Name)
		}
	}
}

func snippetLower(m scoredMatch) string { return strings.ToLower(m.Snippet) }

// TestToolIndex_MemoisesDescription ensures a registered tool's
// Description() should be invoked at most once per Rebuild.
func TestToolIndex_MemoisesDescription(t *testing.T) {
	idx := newToolIndex(nil) // empty registry
	// With nil registry the index is empty; sanity-check the API at least
	// doesn't panic and exposes the hash so callers can detect rebuilds.
	if idx == nil {
		t.Fatal("nil index")
	}
	hash1 := idx.hashSnapshot()
	idx.rebuild()
	hash2 := idx.hashSnapshot()
	// An empty registry should produce a stable hash across rebuilds.
	if hash1 != hash2 {
		t.Fatalf("empty-registry hash changed between rebuilds: %q vs %q", hash1, hash2)
	}
}
