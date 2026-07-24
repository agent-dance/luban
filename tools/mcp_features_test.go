package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestMCPCallTimeoutOverride covers MCP-03 timeout configuration.
func TestMCPCallTimeoutOverride(t *testing.T) {
	original := effectiveMCPCallTimeout()
	defer SetDefaultMCPCallTimeout(original)

	SetDefaultMCPCallTimeout(7 * time.Second)
	if got := effectiveMCPCallTimeout(); got != 7*time.Second {
		t.Fatalf("expected 7s, got %v", got)
	}
	SetDefaultMCPCallTimeout(0)
	if got := effectiveMCPCallTimeout(); got != defaultMCPCallTimeout {
		t.Fatalf("zero should restore default, got %v", got)
	}
}

// TestMCPCallTimeoutPreservesEarlierDeadline ensures we don't extend an
// already-tight deadline supplied by the caller.
func TestMCPCallTimeoutPreservesEarlierDeadline(t *testing.T) {
	defer SetDefaultMCPCallTimeout(0)
	SetDefaultMCPCallTimeout(60 * time.Second)
	parent, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	ctx, c := withMCPCallTimeout(parent)
	defer c()
	d, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("expected deadline")
	}
	if time.Until(d) > 2*time.Second {
		t.Fatalf("MCP timeout should not extend caller deadline; got %v remaining", time.Until(d))
	}
}

// TestMCPRenderBinaryTruncation verifies MCP-05 binary blob truncation.
func TestMCPRenderBinaryTruncation(t *testing.T) {
	big := strings.Repeat("A", maxBinaryInlineBytes+10)
	item, _ := json.Marshal(map[string]any{
		"type":     "image",
		"mimeType": "image/png",
		"data":     big,
	})
	out := renderMCPContentItem(item)
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("rendered item not JSON: %v", err)
	}
	if got["binary_truncated"] != true {
		t.Fatalf("expected binary_truncated=true; got %v", got)
	}
	if int(got["original_size_bytes"].(float64)) != maxBinaryInlineBytes+10 {
		t.Fatalf("original_size_bytes should reflect input length")
	}
}

// TestMCPRenderJSONPrettyPrint verifies that JSON text payloads get
// pretty-printed for model parseability.
func TestMCPRenderJSONPrettyPrint(t *testing.T) {
	item, _ := json.Marshal(map[string]any{
		"type":     "text",
		"mimeType": "application/json",
		"text":     `{"a":1,"b":[2,3]}`,
	})
	out := renderMCPContentItem(item)
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("rendered item not JSON: %v", err)
	}
	pretty, ok := got["text"].(string)
	if !ok || !strings.Contains(pretty, "\n") {
		t.Fatalf("expected multi-line pretty JSON, got %q", pretty)
	}
}

// TestMCPResolveToolNameConflict covers MCP-06 conflict resolution: the
// lower-priority server keeps the bare name, others get a __<server>
// suffix.
func TestMCPResolveToolNameConflict(t *testing.T) {
	mgr := NewMCPManager()
	mgr.AddServer(&MCPServer{
		Name:  "alpha",
		Tools: []MCPServerTool{{Name: "search"}},
	})
	mgr.AddServer(&MCPServer{
		Name:  "beta",
		Tools: []MCPServerTool{{Name: "search"}},
	})
	mgr.SetPriorityOrder([]string{"alpha", "beta"})

	if got := mgr.ResolveToolName("alpha", "search"); got != "search" {
		t.Fatalf("alpha winner should keep bare name; got %q", got)
	}
	if got := mgr.ResolveToolName("beta", "search"); got != "search__beta" {
		t.Fatalf("beta loser should be suffixed; got %q", got)
	}
}

// TestMCPStdioRestartCooldownTable spot-checks the cooldown sequence.
func TestMCPStdioRestartCooldownTable(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 5 * time.Second},
		{2, 10 * time.Second},
		{3, 30 * time.Second},
		{99, 30 * time.Second}, // beyond table → reuses last entry
	}
	for _, c := range cases {
		if got := stdioRestartCooldownFor(c.attempt); got != c.want {
			t.Errorf("attempt %d: got %v, want %v", c.attempt, got, c.want)
		}
	}
}
