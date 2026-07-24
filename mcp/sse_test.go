package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// mockSSEServer is a minimal HTTP server that implements the MCP-over-SSE
// protocol used by SSEClient.  It responds to POST /message inline
// (application/json) so the test doesn't need a live SSE stream.
type mockSSEServer struct {
	mux     *http.ServeMux
	server  *httptest.Server
	idCount atomic.Int64
}

func newMockSSEServer(methods map[string]func(params json.RawMessage) (any, error)) *mockSSEServer {
	m := &mockSSEServer{mux: http.NewServeMux()}

	m.mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// notification (no id) — just return 204.
		if req.ID == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		handler, ok := methods[req.Method]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
			return
		}
		result, err := handler(req.Params)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error":   map[string]any{"code": -32000, "message": err.Error()},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		})
	})

	// SSE endpoint — just hang (tests use inline JSON responses).
	m.mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		// Block until client disconnects.
		<-r.Context().Done()
	})

	m.server = httptest.NewServer(m.mux)
	return m
}

func (m *mockSSEServer) close() { m.server.Close() }

// startTestSSEClient creates an SSEClient backed by srv.
func startTestSSEClient(t *testing.T, name string, methods map[string]func(json.RawMessage) (any, error)) *SSEClient {
	t.Helper()

	if _, ok := methods["initialize"]; !ok {
		methods["initialize"] = func(_ json.RawMessage) (any, error) {
			return map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}}, nil
		}
	}

	srv := newMockSSEServer(methods)
	t.Cleanup(func() { srv.close() })

	hc := &http.Client{Timeout: 5 * time.Second}
	c, err := NewSSEClient(name, srv.server.URL, hc)
	if err != nil {
		t.Fatalf("NewSSEClient: %v", err)
	}
	t.Cleanup(func() { c.Close() }) //nolint:errcheck
	return c
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestSSEClientInitialize(t *testing.T) {
	_ = startTestSSEClient(t, "test", map[string]func(json.RawMessage) (any, error){})
}

func TestSSEClientListTools(t *testing.T) {
	c := startTestSSEClient(t, "myserver", map[string]func(json.RawMessage) (any, error){
		"tools/list": func(_ json.RawMessage) (any, error) {
			return map[string]any{
				"tools": []map[string]any{
					{
						"name":        "greet",
						"description": "Say hello",
						"inputSchema": map[string]any{"type": "object"},
					},
				},
			}, nil
		},
	})

	tools, err := c.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].OriginalName != "greet" {
		t.Errorf("expected OriginalName 'greet', got %q", tools[0].OriginalName)
	}
	if tools[0].Name() != "mcp__myserver__greet" {
		t.Errorf("expected prefixed name, got %q", tools[0].Name())
	}
}

func TestSSEClientCallTool(t *testing.T) {
	var gotName string
	var gotArgs map[string]any

	c := startTestSSEClient(t, "calc", map[string]func(json.RawMessage) (any, error){
		"tools/call": func(params json.RawMessage) (any, error) {
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			json.Unmarshal(params, &p) //nolint:errcheck
			gotName = p.Name
			gotArgs = p.Arguments
			return map[string]any{
				"content": []map[string]any{{"type": "text", "text": "99"}},
			}, nil
		},
	})

	result, err := c.CallTool(t.Context(), "add", map[string]any{"a": 10, "b": 89})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result != "99" {
		t.Errorf("expected '99', got %q", result)
	}
	if gotName != "add" {
		t.Errorf("expected tool name 'add', got %q", gotName)
	}
	if gotArgs["a"] != float64(10) {
		t.Errorf("expected argument a=10, got %v", gotArgs["a"])
	}
}

func TestSSEClientCallToolError(t *testing.T) {
	c := startTestSSEClient(t, "srv", map[string]func(json.RawMessage) (any, error){
		"tools/call": func(_ json.RawMessage) (any, error) {
			return map[string]any{
				"content": []map[string]any{{"type": "text", "text": "boom"}},
				"isError": true,
			}, nil
		},
	})

	_, err := c.CallTool(t.Context(), "broken", map[string]any{})
	if err == nil {
		t.Fatal("expected error for isError=true response")
	}
}

func TestSSEClientListResources(t *testing.T) {
	c := startTestSSEClient(t, "fs", map[string]func(json.RawMessage) (any, error){
		"resources/list": func(_ json.RawMessage) (any, error) {
			return map[string]any{
				"resources": []map[string]any{
					{"uri": "file:///tmp/x.txt", "name": "x.txt", "description": "File X"},
				},
			}, nil
		},
	})

	resources, err := c.ListResources()
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0].URI != "file:///tmp/x.txt" {
		t.Errorf("unexpected URI: %s", resources[0].URI)
	}
}

func TestSSEClientReadResource(t *testing.T) {
	c := startTestSSEClient(t, "fs", map[string]func(json.RawMessage) (any, error){
		"resources/read": func(params json.RawMessage) (any, error) {
			var p struct {
				URI string `json:"uri"`
			}
			json.Unmarshal(params, &p) //nolint:errcheck
			return map[string]any{
				"contents": []map[string]any{
					{"uri": p.URI, "text": "file-content"},
				},
			}, nil
		},
	})

	content, err := c.ReadResource(t.Context(), "file:///tmp/y.txt")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if content != "file-content" {
		t.Errorf("expected 'file-content', got %q", content)
	}
}

func TestSSEClientRPCError(t *testing.T) {
	c := startTestSSEClient(t, "srv", map[string]func(json.RawMessage) (any, error){
		"tools/list": func(_ json.RawMessage) (any, error) {
			return nil, fmt.Errorf("internal server error")
		},
	})

	_, err := c.ListTools()
	if err == nil {
		t.Fatal("expected error from server-side RPC error")
	}
}

func TestSSEClientClose(t *testing.T) {
	c := startTestSSEClient(t, "srv", map[string]func(json.RawMessage) (any, error){})
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// Closing twice should be safe.
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestDecodeRPCResult(t *testing.T) {
	data := []byte(`{"id":1,"result":{"foo":"bar"}}`)
	var out struct {
		Foo string `json:"foo"`
	}
	if err := decodeRPCResult(data, &out); err != nil {
		t.Fatalf("decodeRPCResult: %v", err)
	}
	if out.Foo != "bar" {
		t.Errorf("expected Foo='bar', got %q", out.Foo)
	}
}

func TestDecodeRPCResultError(t *testing.T) {
	data := []byte(`{"id":1,"error":{"code":-32000,"message":"something went wrong"}}`)
	err := decodeRPCResult(data, nil)
	if err == nil {
		t.Fatal("expected error for RPC error envelope")
	}
}
