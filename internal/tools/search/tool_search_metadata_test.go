package search

import (
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestToolSearchDeclaresCanonicalMetadata(t *testing.T) {
	tool := NewToolSearch(registry.New(), nil)
	want := types.ToolMetadata{ReadOnly: true, Search: true, ConcurrencySafe: true}
	if got := tool.ToolMetadata(nil); got != want {
		t.Fatalf("ToolMetadata() = %#v, want %#v", got, want)
	}
}
