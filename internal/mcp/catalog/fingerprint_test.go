package catalog

import "testing"

func TestHashMCPConfigIgnoresScope(t *testing.T) {
	a := MCPServerConfig{Type: TransportHTTP, URL: "http://example.test/mcp", Scope: ScopeProject}
	b := MCPServerConfig{Type: TransportHTTP, URL: "http://example.test/mcp", Scope: ScopeUser}
	if HashMCPConfig(a) != HashMCPConfig(b) {
		t.Fatalf("config hash should ignore scope: %s != %s", HashMCPConfig(a), HashMCPConfig(b))
	}
}

func TestHashMCPConfigChangesWithConnectionFields(t *testing.T) {
	base := MCPServerConfig{Type: TransportStdio, Command: "node", Args: []string{"server.js"}}
	changed := base
	changed.Args = []string{"other.js"}
	if HashMCPConfig(base) == HashMCPConfig(changed) {
		t.Fatal("config hash should change when connection fields change")
	}
}
