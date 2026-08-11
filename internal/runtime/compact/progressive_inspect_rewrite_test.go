package compact

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/compactproof"
)

func TestProgressiveInspectRewritePreservesSourceMapAndExactSnippetEdges(t *testing.T) {
	originalBytes, err := json.Marshal(map[string]any{
		"requests": []any{map[string]any{
			"id": "symbols", "kind": "search", "path": "src",
			"matches": []any{map[string]any{"path": "src/graph.cc", "items": []any{map[string]any{"line": 41}, map[string]any{"line": 93}}}},
		}},
		"evidence": []any{map[string]any{
			"path": "src/graph.cc",
			"chunks": []any{map[string]any{
				"lines":   []int{40, 95},
				"content": "bool RecomputeDirty(Node* node, std::vector<Node*>* validation_nodes, std::string* err) {\n" + strings.Repeat("body();\n", 1_000) + "return true;\n}",
			}},
		}},
		"has_more_view": true, "source_truncated": true, "cursor": "cursor-opaque",
	})
	if err != nil {
		t.Fatal(err)
	}
	proof := `{"schema":"` + compactproof.SchemaVersion + `","tool":"Inspect"}`
	got, ok := progressiveInspectRewriteContent(string(originalBytes), proof)
	if !ok {
		t.Fatal("rewrite was rejected")
	}
	for _, exact := range []string{
		progressiveInspectRewriteSchema, compactproof.SchemaVersion, "src/graph.cc", "RecomputeDirty(Node* node", "return true;", "cursor-opaque",
	} {
		if !strings.Contains(got, exact) {
			t.Fatalf("rewrite omitted %q: %s", exact, got)
		}
	}
	if len(got) > progressiveInspectRewriteMaxBytes || len(got) >= len(originalBytes) {
		t.Fatalf("rewrite size = %d, original = %d", len(got), len(originalBytes))
	}
}

func TestProgressiveInspectRewriteFailsClosedWithoutStructuredEvidence(t *testing.T) {
	if got, ok := progressiveInspectRewriteContent("plain source output", `{}`); ok || got != "" {
		t.Fatalf("unstructured source projected: %q", got)
	}
}

func TestProgressiveInspectIndexPreservesEveryPathAndLineRangeWithoutSourceText(t *testing.T) {
	originalBytes, err := json.Marshal(map[string]any{
		"requests": []any{map[string]any{"id": "read", "kind": "read", "path": "src/graph.cc"}},
		"evidence": []any{
			map[string]any{"path": "src/graph.cc", "chunks": []any{
				map[string]any{"lines": []int{10, 20}, "content": strings.Repeat("first-secret ", 500)},
				map[string]any{"lines": []int{80, 95}, "content": strings.Repeat("second-secret ", 500)},
			}},
			map[string]any{"path": "src/build_test.cc", "chunks": []any{
				map[string]any{"lines": []int{100, 140}, "content": strings.Repeat("third-secret ", 500)},
			}},
		},
		"has_more_view": true, "cursor": "cursor-index",
	})
	if err != nil {
		t.Fatal(err)
	}
	proof := `{"schema":"` + compactproof.SchemaVersion + `","tool":"Inspect"}`
	got, ok := progressiveInspectIndexContent(string(originalBytes), proof)
	if !ok {
		t.Fatal("index was rejected")
	}
	for _, exact := range []string{progressiveInspectIndexSchema, compactproof.SchemaVersion, "src/graph.cc", "src/build_test.cc", "cursor-index", "[10,20]", "[80,95]", "[100,140]"} {
		if !strings.Contains(got, exact) {
			t.Fatalf("index omitted %q: %s", exact, got)
		}
	}
	for _, omitted := range []string{"first-secret", "second-secret", "third-secret"} {
		if strings.Contains(got, omitted) {
			t.Fatalf("index retained source text %q: %s", omitted, got)
		}
	}
}
