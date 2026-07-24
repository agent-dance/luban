package mcp

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"
)

// startMCPTestClient creates a full MCP Client (with initialize handshake) backed
// by an in-memory jrpc2 server implementing the given methods. Default handlers
// for "initialize" and "notifications/initialized" are injected automatically
// unless the caller provides overrides.
func startMCPTestClient(t *testing.T, name string, methods handler.Map) *Client {
	t.Helper()
	if _, ok := methods["initialize"]; !ok {
		methods["initialize"] = handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			return map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}}, nil
		})
	}
	if _, ok := methods["notifications/initialized"]; !ok {
		methods["notifications/initialized"] = handler.New(func(ctx context.Context) error { return nil })
	}

	srvR, cliW := io.Pipe()
	cliR, srvW := io.Pipe()

	srv := jrpc2.NewServer(methods, nil)
	srv.Start(channel.Line(srvR, srvW))

	t.Cleanup(func() {
		srv.Stop()
		srvR.Close()
		srvW.Close()
		cliR.Close()
		cliW.Close()
	})

	c, err := NewClientFromChannel(name, channel.Line(cliR, cliW))
	if err != nil {
		t.Fatalf("NewClientFromChannel(%q): %v", name, err)
	}
	t.Cleanup(func() { c.Close() }) //nolint:errcheck
	return c
}

// ─── TestMCPClientInitialize ─────────────────────────────────────────────────

func TestMCPClientInitialize(t *testing.T) {
	// startMCPTestClient already performs initialize, so no error == success.
	_ = startMCPTestClient(t, "test", handler.Map{})
}

func TestMCPClientInitializeCustomVersion(t *testing.T) {
	var gotVersion string
	_ = startMCPTestClient(t, "test", handler.Map{
		"initialize": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			json.Unmarshal(params, &p) //nolint:errcheck
			gotVersion = p.ProtocolVersion
			return map[string]any{"protocolVersion": p.ProtocolVersion, "capabilities": map[string]any{}}, nil
		}),
	})
	if gotVersion != "2024-11-05" {
		t.Errorf("expected protocolVersion 2024-11-05, got %q", gotVersion)
	}
}

func TestMCPClientInitializeCapturesInstructions(t *testing.T) {
	c := startMCPTestClient(t, "docs", handler.Map{
		"initialize": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			return map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"instructions":    " Prefer official docs resources. \n",
			}, nil
		}),
	})
	if got := c.Instructions(); got != "Prefer official docs resources." {
		t.Fatalf("Instructions() = %q", got)
	}
}

// ─── TestMCPClientListTools ──────────────────────────────────────────────────

func TestMCPClientListTools(t *testing.T) {
	c := startMCPTestClient(t, "myserver", handler.Map{
		"tools/list": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			return map[string]any{
				"tools": []map[string]any{
					{
						"name":        "calculator",
						"description": "Do math",
						"inputSchema": map[string]any{"type": "object"},
					},
				},
			}, nil
		}),
	})

	tools, err := c.ListTools()
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name() != "mcp__myserver__calculator" {
		t.Errorf("expected prefixed name, got %q", tools[0].Name())
	}
	if tools[0].OriginalName != "calculator" {
		t.Errorf("expected OriginalName 'calculator', got %q", tools[0].OriginalName)
	}
	if tools[0].Description() != "Do math" {
		t.Errorf("expected 'Do math', got %q", tools[0].Description())
	}
}

func TestMCPClientListToolsEmpty(t *testing.T) {
	c := startMCPTestClient(t, "srv", handler.Map{
		"tools/list": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			return map[string]any{"tools": []any{}}, nil
		}),
	})
	tools, err := c.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

// ─── TestMCPClientCallTool ───────────────────────────────────────────────────

func TestMCPClientCallTool(t *testing.T) {
	c := startMCPTestClient(t, "calc", handler.Map{
		"tools/call": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			return map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "42"},
				},
			}, nil
		}),
	})

	result, err := c.CallTool(context.Background(), "add", map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result != "42" {
		t.Errorf("expected '42', got %q", result)
	}
}

func TestMCPClientCallToolPassesArguments(t *testing.T) {
	var gotName string
	var gotArgs map[string]any

	c := startMCPTestClient(t, "srv", handler.Map{
		"tools/call": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			json.Unmarshal(params, &p) //nolint:errcheck
			gotName = p.Name
			gotArgs = p.Arguments
			return map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}}, nil
		}),
	})

	c.CallTool(context.Background(), "myTool", map[string]any{"key": "val"}) //nolint:errcheck
	if gotName != "myTool" {
		t.Errorf("expected name 'myTool', got %q", gotName)
	}
	if gotArgs["key"] != "val" {
		t.Errorf("expected arguments.key='val', got %v", gotArgs)
	}
}

func TestMCPClientCallToolMultiContent(t *testing.T) {
	c := startMCPTestClient(t, "srv", handler.Map{
		"tools/call": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			return map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "hello"},
					{"type": "image", "data": "base64..."},
					{"type": "text", "text": "world"},
				},
			}, nil
		}),
	})

	result, err := c.CallTool(context.Background(), "multi", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result != "hello\nworld" {
		t.Errorf("expected 'hello\\nworld', got %q", result)
	}
}

func TestMCPClientCallToolError(t *testing.T) {
	c := startMCPTestClient(t, "srv", handler.Map{
		"tools/call": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			return map[string]any{
				"content": []map[string]any{{"type": "text", "text": "something broke"}},
				"isError": true,
			}, nil
		}),
	})

	_, err := c.CallTool(context.Background(), "broken", map[string]any{})
	if err == nil {
		t.Fatal("expected error for isError=true response")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestMCPClientCallToolContextCancel(t *testing.T) {
	c := startMCPTestClient(t, "test", handler.Map{
		"tools/call": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Second):
				return map[string]any{}, nil
			}
		}),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.CallTool(ctx, "slow", map[string]any{})
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

// ─── TestMCPClientListResources ──────────────────────────────────────────────

func TestMCPClientListResources(t *testing.T) {
	c := startMCPTestClient(t, "fs", handler.Map{
		"resources/list": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			return map[string]any{
				"resources": []map[string]any{
					{"uri": "file:///tmp/a.txt", "name": "a.txt", "description": "File A", "mimeType": "text/plain"},
					{"uri": "file:///tmp/b.txt", "name": "b.txt"},
				},
			}, nil
		}),
	})

	resources, err := c.ListResources()
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(resources))
	}
	if resources[0].URI != "file:///tmp/a.txt" {
		t.Errorf("unexpected URI: %s", resources[0].URI)
	}
	if resources[0].Description != "File A" {
		t.Errorf("unexpected Description: %s", resources[0].Description)
	}
}

func TestMCPClientListResourcesEmpty(t *testing.T) {
	c := startMCPTestClient(t, "srv", handler.Map{
		"resources/list": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			return map[string]any{"resources": []any{}}, nil
		}),
	})
	resources, err := c.ListResources()
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(resources))
	}
}

// ─── TestMCPClientReadResource ───────────────────────────────────────────────

func TestMCPClientReadResource(t *testing.T) {
	c := startMCPTestClient(t, "fs", handler.Map{
		"resources/read": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			return map[string]any{
				"contents": []map[string]any{
					{"uri": "file:///tmp/hello.txt", "text": "hello world"},
				},
			}, nil
		}),
	})

	content, err := c.ReadResource(context.Background(), "file:///tmp/hello.txt")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if content != "hello world" {
		t.Errorf("expected 'hello world', got %q", content)
	}
}

func TestMCPClientReadResourcePassesURI(t *testing.T) {
	var gotURI string
	c := startMCPTestClient(t, "srv", handler.Map{
		"resources/read": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			var p struct {
				URI string `json:"uri"`
			}
			json.Unmarshal(params, &p) //nolint:errcheck
			gotURI = p.URI
			return map[string]any{"contents": []map[string]any{{"uri": p.URI, "text": "data"}}}, nil
		}),
	})

	c.ReadResource(context.Background(), "custom://my-resource") //nolint:errcheck
	if gotURI != "custom://my-resource" {
		t.Errorf("expected URI 'custom://my-resource', got %q", gotURI)
	}
}

func TestMCPClientReadResourceMultiContent(t *testing.T) {
	c := startMCPTestClient(t, "srv", handler.Map{
		"resources/read": handler.New(func(ctx context.Context, params json.RawMessage) (any, error) {
			return map[string]any{
				"contents": []map[string]any{
					{"uri": "u", "text": "part1"},
					{"uri": "u", "text": "part2"},
				},
			}, nil
		}),
	})

	content, err := c.ReadResource(context.Background(), "u")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if content != "part1\npart2" {
		t.Errorf("expected 'part1\\npart2', got %q", content)
	}
}

// ─── TestMCPTool schema ───────────────────────────────────────────────────────

func TestMCPToolSchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"x":{"type":"number"}}}`)
	tool := &MCPTool{
		ToolName:    "test",
		ToolDesc:    "test tool",
		InputSchema: schema,
	}
	s := tool.Schema()
	if s.Type != "object" {
		t.Errorf("expected object type, got %q", s.Type)
	}
}

func TestMCPToolSchemaInvalid(t *testing.T) {
	tool := &MCPTool{
		ToolName:    "bad",
		ToolDesc:    "bad schema",
		InputSchema: json.RawMessage(`{invalid`),
	}
	s := tool.Schema()
	if s.Type != "object" {
		t.Error("expected fallback to object type for invalid schema")
	}
}
