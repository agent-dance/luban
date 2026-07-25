package catalog

import (
	"encoding/json"
	"testing"

	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
)

func TestMCPServerConfigJSONDefaultsStdioArgs(t *testing.T) {
	var config MCPServerConfig
	if err := json.Unmarshal([]byte(`{"command":"node"}`), &config); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if config.Type != TransportStdio {
		t.Fatalf("type = %q, want %q", config.Type, TransportStdio)
	}
	if config.Args == nil || len(config.Args) != 0 {
		t.Fatalf("args = %#v, want non-nil empty slice", config.Args)
	}
}

func TestMCPServerConfigAuthDescriptorSetsTransportCapability(t *testing.T) {
	authConfig := &mcpauth.Config{ClientID: "client"}
	remote := MCPServerConfig{
		Type:    TransportHTTP,
		URL:     "https://mcp.example.test",
		Headers: map[string]string{"X-Tenant": "one"},
		OAuth:   authConfig,
	}.AuthDescriptor()
	if !remote.OAuthCapable || remote.Transport != "http" || remote.OAuth != authConfig {
		t.Fatalf("unexpected HTTP descriptor: %#v", remote)
	}

	websocket := MCPServerConfig{Type: TransportWebSocket, URL: "wss://mcp.example.test", OAuth: authConfig}.AuthDescriptor()
	if websocket.OAuthCapable {
		t.Fatalf("WebSocket descriptor unexpectedly OAuth-capable: %#v", websocket)
	}
}
