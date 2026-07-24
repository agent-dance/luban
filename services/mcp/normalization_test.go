package mcp

import "testing"

func TestNormalizeNameForMCPMatchesTypeScript(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "github", want: "github"},
		{name: "my server.name", want: "my_server_name"},
		{name: "Tool: Search/Issues!", want: "Tool__Search_Issues_"},
		{name: "already_ok-123", want: "already_ok-123"},
		{name: "claude.ai Slack Connector", want: "claude_ai_Slack_Connector"},
		{name: "claude.ai  Weird...Name!! ", want: "claude_ai_Weird_Name"},
	}

	for _, tt := range tests {
		if got := NormalizeNameForMCP(tt.name); got != tt.want {
			t.Fatalf("NormalizeNameForMCP(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestMCPToolNameBuildAndParse(t *testing.T) {
	fullName := BuildMCPToolName("my server.name", "Search Issues!")
	if fullName != "mcp__my_server_name__Search_Issues_" {
		t.Fatalf("BuildMCPToolName = %q", fullName)
	}

	info, ok := MCPInfoFromString(fullName)
	if !ok {
		t.Fatalf("MCPInfoFromString(%q) returned !ok", fullName)
	}
	if info.ServerName != "my_server_name" {
		t.Fatalf("server name = %q", info.ServerName)
	}
	if info.ToolName == nil || *info.ToolName != "Search_Issues_" {
		t.Fatalf("tool name = %#v", info.ToolName)
	}
}

func TestMCPInfoFromStringPreservesDoubleUnderscoreInToolName(t *testing.T) {
	info, ok := MCPInfoFromString("mcp__server__tool__with__parts")
	if !ok {
		t.Fatal("expected valid MCP name")
	}
	if info.ServerName != "server" {
		t.Fatalf("server = %q", info.ServerName)
	}
	if info.ToolName == nil || *info.ToolName != "tool__with__parts" {
		t.Fatalf("tool = %#v", info.ToolName)
	}
}

func TestMCPInfoFromStringRejectsInvalidPrefixOrMissingServer(t *testing.T) {
	if _, ok := MCPInfoFromString("Read"); ok {
		t.Fatal("builtin tool parsed as MCP")
	}
	if _, ok := MCPInfoFromString("mcp____tool"); ok {
		t.Fatal("missing server parsed as MCP")
	}
}

func TestUniqueClaudeAIServerNameAvoidsNormalizedCollisions(t *testing.T) {
	used := map[string]struct{}{}

	first := UniqueClaudeAIServerName("Example Server", used)
	second := UniqueClaudeAIServerName("Example.Server", used)
	third := UniqueClaudeAIServerName("Example Server", used)

	if first != "claude.ai Example Server" {
		t.Fatalf("first = %q", first)
	}
	if second != "claude.ai Example.Server (2)" {
		t.Fatalf("second = %q", second)
	}
	if third != "claude.ai Example Server (3)" {
		t.Fatalf("third = %q", third)
	}
	if _, ok := used["claude_ai_Example_Server"]; !ok {
		t.Fatalf("base normalized name not recorded: %#v", used)
	}
	if _, ok := used["claude_ai_Example_Server_2"]; !ok {
		t.Fatalf("second normalized name not recorded: %#v", used)
	}
}
