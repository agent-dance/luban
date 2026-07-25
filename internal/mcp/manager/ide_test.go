package manager

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	"github.com/agent-dance/luban/internal/mcp/protocol"
)

func TestIDETransportStaticHeadersAndToolFiltering(t *testing.T) {
	var gotIDEHeader atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIDEHeader.Store(r.Header.Get("X-IDE-Test"))
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
			return
		}
		var msg protocol.JSONRPCMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		switch msg.Method {
		case "initialize":
			writeSpecializedRPCResult(t, w, msg.ID, map[string]any{
				"protocolVersion": testMCPProtocolVersion,
				"capabilities":    catalog.ServerCapabilities{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "ide", "version": "1"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeSpecializedRPCResult(t, w, msg.ID, map[string]any{"tools": []map[string]any{
				{"name": "executeCode", "inputSchema": map[string]any{"type": "object"}},
				{"name": "getDiagnostics", "inputSchema": map[string]any{"type": "object"}},
				{"name": "dangerousInternal", "inputSchema": map[string]any{"type": "object"}},
			}})
		default:
			t.Errorf("unexpected method %q", msg.Method)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	manager := NewManager()
	defer manager.Shutdown(context.Background())
	manager.AddConfig("ide", catalog.MCPServerConfig{
		Type:    catalog.TransportSSEIDE,
		URL:     server.URL,
		IDEName: "VSCode",
		Headers: map[string]string{"X-IDE-Test": "ide-value"},
	})

	state, err := manager.GetOrConnect(context.Background(), "ide")
	if err != nil {
		t.Fatalf("GetOrConnect ide: %v", err)
	}
	if state.Type != MCPStateConnected {
		t.Fatalf("ide state = %#v, want connected", state)
	}
	if got, _ := gotIDEHeader.Load().(string); got != "ide-value" {
		t.Fatalf("IDE static header = %q", got)
	}
	if got := toolNames(state.Tools); strings.Join(got, ",") != "executeCode,getDiagnostics" {
		t.Fatalf("filtered IDE tools = %#v", got)
	}
}

func writeSpecializedRPCResult(t *testing.T, w http.ResponseWriter, id json.RawMessage, result any) {
	t.Helper()
	message, err := protocol.NewResultMessage(id, result)
	if err != nil {
		t.Fatalf("build JSON-RPC result: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(message); err != nil {
		t.Errorf("encode JSON-RPC result: %v", err)
	}
}

func toolNames(tools []catalog.ToolDefinition) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Name)
	}
	return out
}
