package tools

// mcp_tools_alignment_test.go — post-parity assertions for historical
// alignment_audit.md gaps
// for MCPTool / ListMcpResourcesTool / ReadMcpResourceTool.
//
// Audit reference (P2-6):
//   - MCP model-facing tools must use `mcp__<srv>__<tool>` with normalized
//     name parts, matching the TypeScript mcpStringUtils contract.
//   - No auth.go module under gosrc/services/mcp/
//   - ReadResource drops uri / mimeType from the result envelope
//   - ListResources returns concatenated text rather than a struct
//   - No OAuth 401 handshake / PKCE flow
//
// These tests started as RED coverage; task_18 keeps them as executable
// post-parity assertions and removes stale pre-parity wording.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	mcppkg "github.com/agent-dance/luban/mcp"
	mcpsvc "github.com/agent-dance/luban/services/mcp"
	"github.com/creachadair/jrpc2/handler"
)

// ─── Gap 1: Tool-name namespace must use double underscores ────────────

// TestAlignmentMCPNamespaceDoubleUnderscore asserts the canonical TS namespace
// `mcp__<server>__<tool>` with normalized server and tool parts.
func TestAlignmentMCPNamespaceDoubleUnderscore(t *testing.T) {
	probe := mcppkg.MCPTool{
		ToolName:     mcpsvc.BuildMCPToolName("my server", "my tool"),
		OriginalName: "my_tool",
	}
	if !strings.Contains(probe.ToolName, "mcp__my_server__my_tool") {
		t.Errorf("namespace = %q, want double-underscore mcp__<srv>__<tool>", probe.ToolName)
	}
}

// TestAlignmentMCPNamespaceNoSingleUnderscore asserts that the canonical
// builder does not emit the legacy ambiguous single-underscore shape.
func TestAlignmentMCPNamespaceNoSingleUnderscore(t *testing.T) {
	produced := mcpsvc.BuildMCPToolName("canary", "tool")
	if produced != "mcp__canary__tool" {
		t.Fatalf("namespace shape = %q, want canonical double-underscore form", produced)
	}
	legacyPrefix := strings.Join([]string{"mcp", "canary"}, "_") + "_"
	if strings.HasPrefix(produced, legacyPrefix) {
		t.Errorf("namespace shape %q regressed to legacy single-underscore form", produced)
	}
}

// ─── Gap 2: gosrc/services/mcp/auth.go must exist ────────────────────────

// TestAlignmentMCPAuthModuleExists asserts that an auth.go module lives
// under gosrc/services/mcp. Today no `services/mcp/` directory exists at
// all. The audit calls for OAuth401 + PKCE handling there.
func TestAlignmentMCPAuthModuleExists(t *testing.T) {
	candidates := []string{
		"gosrc/services/mcp/auth.go",
		"../services/mcp/auth.go",
		"../../services/mcp/auth.go",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return
		}
	}
	t.Errorf("services/mcp/auth.go not found — audit P2-6 requires an auth module for OAuth 401 / PKCE handling")
}

// ─── Gap 3: ReadMcpResource must preserve uri / mimeType ────────────────

// TestAlignmentMCPReadResourcePreservesURI asserts that the tool's result
// content includes the URI in a structured form. Today
// ReadMcpResourceTool.Execute returns just the resource body string,
// dropping uri / mimeType (mcp_tools.go:657).
func TestAlignmentMCPReadResourcePreservesURI(t *testing.T) {
	methods := handler.Map{
		"resources/read": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"contents": []map[string]any{{
					"uri":      "file:///foo/bar.txt",
					"mimeType": "text/plain",
					"text":     "hello",
				}},
			}, nil
		}),
	}
	client := startMCPServer(t, "srv", methods)
	mgr := injectServer("srv", client, []MCPServerTool{{Name: "x"}})
	tool := NewReadMcpResourceTool(mgr)

	result, err := tool.Execute(context.Background(), map[string]any{
		"server": "srv",
		"uri":    "file:///foo/bar.txt",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	// Post-fix expectation: the result is a JSON envelope carrying uri
	// and mimeType. Today the body is plain text "hello".
	if !strings.Contains(result.Content, `"uri"`) {
		t.Errorf("ReadMcpResource result missing uri field — audit P2-6:\n  got=%q", result.Content)
	}
	if !strings.Contains(result.Content, `"mimeType"`) {
		t.Errorf("ReadMcpResource result missing mimeType field — audit P2-6")
	}
}

// TestAlignmentMCPReadResourceMimeTypePropagated asserts that the
// mimeType is parsed and surfaced. Today it is silently dropped.
func TestAlignmentMCPReadResourceMimeTypePropagated(t *testing.T) {
	methods := handler.Map{
		"resources/read": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"contents": []map[string]any{{
					"uri":      "memo://k/1",
					"mimeType": "application/json",
					"text":     `{"k":"v"}`,
				}},
			}, nil
		}),
	}
	client := startMCPServer(t, "srv", methods)
	mgr := injectServer("srv", client, nil)
	tool := NewReadMcpResourceTool(mgr)

	result, err := tool.Execute(context.Background(), map[string]any{
		"server": "srv",
		"uri":    "memo://k/1",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result.Content, "application/json") {
		t.Errorf("mimeType application/json not propagated — audit P2-6:\n  got=%q", result.Content)
	}
}

// ─── Gap 4: ListMcpResources must return a structured array ─────────────

// TestAlignmentMCPListResourcesReturnsJSONArray asserts that listing
// produces JSON, not concatenated prose ("Resources from srv:\n  - name (uri)").
// Today's mcp_tools.go:562 emits prose lines.
func TestAlignmentMCPListResourcesReturnsJSONArray(t *testing.T) {
	methods := handler.Map{
		"resources/list": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"resources": []map[string]any{
					{"uri": "memo://a", "name": "A", "mimeType": "text/plain"},
					{"uri": "memo://b", "name": "B", "mimeType": "text/plain"},
				},
			}, nil
		}),
	}
	client := startMCPServer(t, "srv", methods)
	mgr := injectServer("srv", client, nil)
	tool := NewListMcpResourcesTool(mgr)

	result, err := tool.Execute(context.Background(), map[string]any{"server": "srv"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	// Post-fix: result is a JSON array. Today it's freeform prose.
	var parsed any
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		t.Errorf("ListMcpResources returned prose, not JSON — audit P2-6 (mcp_tools.go:562):\n  content=%q\n  unmarshal err=%v", result.Content, err)
	}
}

// TestAlignmentMCPListResourcesIncludesMimeType asserts the mimeType is
// surfaced in the listing — today the prose drops it entirely.
func TestAlignmentMCPListResourcesIncludesMimeType(t *testing.T) {
	methods := handler.Map{
		"resources/list": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"resources": []map[string]any{
					{"uri": "memo://x", "name": "X", "mimeType": "image/png"},
				},
			}, nil
		}),
	}
	client := startMCPServer(t, "srv", methods)
	mgr := injectServer("srv", client, nil)
	tool := NewListMcpResourcesTool(mgr)

	result, err := tool.Execute(context.Background(), map[string]any{"server": "srv"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result.Content, "image/png") {
		t.Errorf("ListMcpResources dropped mimeType — audit P2-6:\n  content=%q", result.Content)
	}
}

// ─── Gap 5: OAuth 401 handshake exists ───────────────────────────────────

// TestAlignmentMCPOAuth401Handshake asserts that an HTTP-mode MCP server
// returning 401 triggers an OAuth handshake. Today the tool just bubbles
// up the 401 as an error.
func TestAlignmentMCPOAuth401Handshake(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="x", as_uri="http://example/oauth"`)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	mgr := NewMCPManager()
	mgr.AddServer(&MCPServer{
		Name:    "auth-srv",
		BaseURL: srv.URL,
		Tools:   []MCPServerTool{{Name: "ping"}},
	})
	tool := NewMCPTool(mgr)

	result, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "auth-srv",
		"tool_name":   "ping",
		"arguments":   map[string]any{},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Post-fix: the tool either retries with a fresh token or surfaces a
	// structured "needs OAuth" hint. Today it returns "MCP HTTP error 401".
	if strings.Contains(result.Content, "MCP HTTP error 401") {
		t.Errorf("MCPTool did not perform OAuth handshake on 401 — audit P2-6 missing services/mcp/auth.go:\n  content=%q", result.Content)
	}
}

// ─── Gap 6: PKCE code-verifier generation ───────────────────────────────

// TestAlignmentMCPPKCEHelperExists asserts a PKCE helper (S256 challenge)
// is exposed by the auth module. Today no such helper exists.
func TestAlignmentMCPPKCEHelperExists(t *testing.T) {
	if !pkceHelperPresent() {
		t.Errorf("PKCE helper missing — audit P2-6: services/mcp/auth.go must expose S256 challenge generation")
	}
}

// pkceHelperPresent returns true once the PKCE helper exists in the
// services/mcp/auth.go module. Today no such symbol exists.
func pkceHelperPresent() bool {
	// The services/mcp/auth.go module exposes NewPKCEPair and PKCEChallengeMethod.
	// Probe via the public PKCEChallengeMethod() helper — if the module
	// is wired in, we will return true.
	return mcpsvc.PKCEChallengeMethod() == mcpsvc.PKCEChallengeMethodS256
}

// ─── Gap 7: stdio transport module decoupled from MCPTool ───────────────

// TestAlignmentMCPStdioTransportDecoupled asserts that an importable
// gosrc/services/mcp/transport_stdio.go module exists. Today the stdio
// channel is built inline in mcp.go via channel.Line.
func TestAlignmentMCPStdioTransportDecoupled(t *testing.T) {
	candidates := []string{
		"gosrc/services/mcp/transport_stdio.go",
		"../services/mcp/transport_stdio.go",
		"../../services/mcp/transport_stdio.go",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return
		}
	}
	t.Errorf("services/mcp/transport_stdio.go missing — audit P2-6: stdio transport must be a separate module")
}

// ─── Gap 8: SSE transport decoupled ─────────────────────────────────────

// TestAlignmentMCPSSETransportDecoupled asserts a transport_sse.go module
// exists under services/mcp/. Today SSE handling lives in gosrc/mcp/sse.go
// alongside the protocol layer; the audit demands a clean services-layer
// transport.
func TestAlignmentMCPSSETransportDecoupled(t *testing.T) {
	candidates := []string{
		"gosrc/services/mcp/transport_sse.go",
		"../services/mcp/transport_sse.go",
		"../../services/mcp/transport_sse.go",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return
		}
	}
	t.Errorf("services/mcp/transport_sse.go missing — audit P2-6: SSE transport must be split out")
}

// ─── Gap 9: services/mcp/client.go must exist ───────────────────────────

// TestAlignmentMCPClientModuleExists asserts the audit's required client.go
// module exists under services/mcp/. Today the client lives in gosrc/mcp/.
func TestAlignmentMCPClientModuleExists(t *testing.T) {
	candidates := []string{
		"gosrc/services/mcp/client.go",
		"../services/mcp/client.go",
		"../../services/mcp/client.go",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return
		}
	}
	t.Errorf("services/mcp/client.go missing — audit P2-6 requires services-layer client module")
}

// ─── Gap 10: services/mcp/registry.go must exist ────────────────────────

// TestAlignmentMCPRegistryModuleExists asserts the audit's required
// registry.go module exists. Today there's only the inline MCPManager.
func TestAlignmentMCPRegistryModuleExists(t *testing.T) {
	candidates := []string{
		"gosrc/services/mcp/registry.go",
		"../services/mcp/registry.go",
		"../../services/mcp/registry.go",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return
		}
	}
	t.Errorf("services/mcp/registry.go missing — audit P2-6 requires registry module")
}

// ─── Gap 11: JSON-RPC envelope on tool result is structured ─────────────

// TestAlignmentMCPCallToolReturnsStructuredEnvelope asserts that MCPTool
// returns a JSON envelope, not a raw text body. Today the production
// CallTool returns the joined text content.
func TestAlignmentMCPCallToolReturnsStructuredEnvelope(t *testing.T) {
	methods := handler.Map{
		"tools/list": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"tools": []map[string]any{{
					"name":        "echo",
					"description": "echoes",
					"inputSchema": map[string]any{"type": "object"},
				}},
			}, nil
		}),
		"tools/call": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"content": []map[string]any{{
					"type": "text",
					"text": "hello",
				}},
				"isError": false,
			}, nil
		}),
	}
	client := startMCPServer(t, "srv", methods)
	mgr := injectServer("srv", client, []MCPServerTool{{Name: "echo"}})
	tool := NewMCPTool(mgr)

	result, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "srv",
		"tool_name":   "echo",
		"arguments":   map[string]any{},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Post-fix expectation: a JSON object with `content` array. Today
	// result.Content is a plain string ("hello"), losing the envelope.
	var envelope map[string]any
	if err := json.Unmarshal([]byte(result.Content), &envelope); err != nil {
		t.Errorf("CallTool result is not a JSON envelope — audit P2-6:\n  content=%q\n  err=%v", result.Content, err)
	} else if _, ok := envelope["content"]; !ok {
		t.Errorf("CallTool envelope missing `content` key — audit P2-6:\n  envelope=%v", envelope)
	}
}

// ─── Gap 12: ListResources surface uri as URL-encoded string ────────────

// TestAlignmentMCPListResourcesPreservesURIVerbatim asserts the URIs are
// surfaced verbatim. Today the prose-rendered listing wraps them in
// parentheses, mangling URIs that contain `(` or `)`.
func TestAlignmentMCPListResourcesPreservesURIVerbatim(t *testing.T) {
	methods := handler.Map{
		"resources/list": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"resources": []map[string]any{
					{"uri": "memo://name(with-parens)", "name": "tricky", "mimeType": "text/plain"},
				},
			}, nil
		}),
	}
	client := startMCPServer(t, "srv", methods)
	mgr := injectServer("srv", client, nil)
	tool := NewListMcpResourcesTool(mgr)

	result, err := tool.Execute(context.Background(), map[string]any{"server": "srv"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result.Content, `"uri"`) {
		t.Errorf("ListResources doesn't preserve uri as a structured key — audit P2-6:\n  content=%q", result.Content)
	}
	if !strings.Contains(result.Content, `memo://name(with-parens)`) {
		t.Errorf("ListResources mangled uri with parens — audit P2-6:\n  content=%q", result.Content)
	}
}

// ─── Gap 13: Auth resolver wired into MCP HTTP fallback ─────────────────

// TestAlignmentMCPHTTPCallSendsBearerToken asserts that the HTTP-mode
// fallback consults an auth resolver and sends Authorization headers.
// Today mcpHTTPCall sends no Authorization at all.
func TestAlignmentMCPHTTPCallSendsBearerToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	mgr := NewMCPManager()
	mgr.AddServer(&MCPServer{
		Name:    "secured-srv",
		BaseURL: srv.URL,
		Tools:   []MCPServerTool{{Name: "ping"}},
	})
	tool := NewMCPTool(mgr)

	_, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "secured-srv",
		"tool_name":   "ping",
		"arguments":   map[string]any{},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("MCP HTTP-mode call sent no bearer token — audit P2-6: services/mcp/auth.go integration missing\n  Authorization=%q", gotAuth)
	}
}

// ─── Gap 14: PKCE flow uses S256 challenge ──────────────────────────────

// TestAlignmentMCPPKCEUsesS256 asserts the post-fix challenge method is
// S256 (RFC 7636). Today no PKCE helper exists.
func TestAlignmentMCPPKCEUsesS256(t *testing.T) {
	method := pkceChallengeMethod()
	if method != "S256" {
		t.Errorf("PKCE challenge method = %q, want S256 — audit P2-6: services/mcp/auth.go missing", method)
	}
}

// pkceChallengeMethod returns the PKCE method the post-fix module
// exposes. Today no such module exists, so we return empty.
func pkceChallengeMethod() string {
	return mcpsvc.PKCEChallengeMethod()
}
