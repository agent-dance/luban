package catalog

import "testing"

func TestNormalizeNameForMCP(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "github", want: "github"},
		{name: "my server.name", want: "my_server_name"},
		{name: "Tool: Search/Issues!", want: "Tool__Search_Issues_"},
		{name: "already_ok-123", want: "already_ok-123"},
	}

	for _, tt := range tests {
		if got := normalizeNameForMCP(tt.name); got != tt.want {
			t.Fatalf("normalizeNameForMCP(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestBuildMCPToolName(t *testing.T) {
	fullName := BuildMCPToolName("my server.name", "Search Issues!")
	if fullName != "mcp__my_server_name__Search_Issues_" {
		t.Fatalf("BuildMCPToolName = %q", fullName)
	}
}
