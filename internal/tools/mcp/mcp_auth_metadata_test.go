package mcp

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestMcpAuthToolDeclaresCanonicalMetadata(t *testing.T) {
	tool := &McpAuthTool{ServerName: "test"}
	if got, want := tool.ToolMetadata(nil), (types.ToolMetadata{}); got != want {
		t.Fatalf("ToolMetadata() = %#v, want %#v", got, want)
	}
}
