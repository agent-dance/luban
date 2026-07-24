package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mcpsvc "github.com/agent-dance/luban/services/mcp"
	"github.com/creachadair/jrpc2/handler"
)

type mcpParityContract struct {
	Key        string
	TSSource   string
	GoSurface  string
	Status     string
	UnskipWhen string
	Acceptance string
}

var mcpParityContracts = []mcpParityContract{
	{
		Key:        "config_schema_name_normalization_env_expansion",
		TSSource:   "../src/services/mcp/types.ts; ../src/services/mcp/config.ts; ../src/services/mcp/normalization.ts; ../src/services/mcp/envExpansion.ts",
		GoSurface:  "gosrc/mcp/mcp.go ServerConfig; gosrc/tools/mcp_tools.go LoadFromSettings",
		Status:     "pending-red-skip",
		UnskipWhen: "task_02 implements TS-compatible schema parsing, normalizeNameForMCP, and env expansion",
		Acceptance: "stdio/sse/http/ws/sdk/claudeai-proxy config fixtures parse without live network or credentials",
	},
	{
		Key:        "dynamic_tool_names",
		TSSource:   "../src/services/mcp/mcpStringUtils.ts; ../src/services/mcp/client.ts fetchToolsForClient",
		GoSurface:  "gosrc/mcp/mcp.go ListTools; gosrc/mcp/sse.go ListTools; gosrc/registry_setup.go",
		Status:     "pending-red-skip",
		UnskipWhen: "task_08 replaces the generic MCPTool-only surface with model-visible mcp__server__tool registrations",
		Acceptance: "model-facing MCP tools use mcp__<normalized-server>__<normalized-tool> and keep original tool name for tools/call",
	},
	{
		Key:        "tool_invocation_result_envelope",
		TSSource:   "../src/tools/MCPTool/MCPTool.ts; ../src/services/mcp/client.ts processMCPResult/callMCPTool",
		GoSurface:  "gosrc/tools/mcp_tools.go MCPTool.Execute; gosrc/tools/mcp_render.go",
		Status:     "enforced",
		Acceptance: "tools/call returns a JSON envelope preserving content[] and isError rather than flattened text",
	},
	{
		Key:        "resource_list_read_envelopes",
		TSSource:   "../src/tools/ListMcpResourcesTool/ListMcpResourcesTool.ts; ../src/tools/ReadMcpResourceTool/ReadMcpResourceTool.ts",
		GoSurface:  "gosrc/tools/mcp_tools.go ListMcpResourcesTool/ReadMcpResourceTool",
		Status:     "enforced",
		Acceptance: "resources/list and resources/read preserve uri, mimeType, description, and contents[] envelopes",
	},
	{
		Key:        "list_changed_invalidation",
		TSSource:   "../src/services/mcp/client.ts onclose/list_changed cache invalidation",
		GoSurface:  "gosrc/tools/mcp_notifications.go",
		Status:     "enforced",
		Acceptance: "tools/list_changed refreshes cached tools and resources/list_changed exposes an invalidation flag/callback",
	},
	{
		Key:        "oauth_needs_auth_hint",
		TSSource:   "../src/services/mcp/client.ts McpAuthError; ../src/services/mcp/auth.ts",
		GoSurface:  "gosrc/services/mcp/auth.go; gosrc/tools/mcp_tools.go mcpHTTPCall",
		Status:     "enforced",
		Acceptance: "HTTP 401 returns a structured oauth_required hint and PKCE helpers use S256",
	},
	{
		Key:        "capabilities_gating",
		TSSource:   "../src/services/mcp/types.ts ConnectedMCPServer.capabilities; ../src/services/mcp/client.ts fetchToolsForClient/fetchResourcesForClient",
		GoSurface:  "gosrc/tools/mcp_tools.go MCPManager; gosrc/services/mcp/registry.go",
		Status:     "pending-red-skip",
		UnskipWhen: "task_07 tracks per-server capabilities and task_11 gates resources on capabilities.resources",
		Acceptance: "tools/resources/prompts are listed or hidden according to server-advertised capabilities",
	},
	{
		Key:        "tool_annotations",
		TSSource:   "../src/services/mcp/client.ts fetchToolsForClient annotations/readOnlyHint/destructiveHint/searchHint",
		GoSurface:  "gosrc/mcp/mcp.go MCPTool; gosrc/tools/tool_search_index.go",
		Status:     "pending-red-skip",
		UnskipWhen: "task_08 maps MCP annotations into Go tool metadata and permission/read-only surfaces",
		Acceptance: "readOnlyHint, destructiveHint, searchHint, and alwaysLoad affect concurrency, policy, and discovery",
	},
	{
		Key:        "binary_resource_persistence",
		TSSource:   "../src/tools/ReadMcpResourceTool/ReadMcpResourceTool.ts; ../src/services/mcp/client.ts persistBlobToTextBlock",
		GoSurface:  "gosrc/tools/mcp_render.go; gosrc/services/mcp/client.go",
		Status:     "pending-red-skip",
		UnskipWhen: "task_10/task_11 persist binary blobs to disk and return blobSavedTo/text notice instead of inline base64",
		Acceptance: "binary resources are decoded, persisted locally with mime-derived extension, and never dumped into model context",
	},
	{
		Key:        "reconnect_session_cache_clearing",
		TSSource:   "../src/services/mcp/client.ts isMcpSessionExpiredError/onclose cache deletion",
		GoSurface:  "gosrc/tools/mcp_sse_reconnect.go; gosrc/tools/mcp_stdio_restart.go",
		Status:     "pending-red-skip",
		UnskipWhen: "task_14 clears connection/tool/resource caches on close, session expiry, and successful reconnect",
		Acceptance: "stale tools/resources disappear after onclose, JSON-RPC -32001/HTTP 404, and SSE reconnect",
	},
	{
		Key:        "prompts_commands_skills",
		TSSource:   "../src/services/mcp/client.ts fetchCommandsForClient; ../src/skills/mcpSkillBuilders.ts",
		GoSurface:  "gosrc/skills/mcp_prompts.go",
		Status:     "pending-red-skip",
		UnskipWhen: "task_12 implements MCP prompts, slash commands, and skill exposure",
		Acceptance: "MCP prompts become commands/skills with mcp__server__prompt names and deterministic schemas",
	},
	{
		Key:        "permissions_policy_channel_relay",
		TSSource:   "../src/services/mcp/channelAllowlist.ts; ../src/services/mcp/channelPermissions.ts; ../src/services/mcp/elicitationHandler.ts",
		GoSurface:  "gosrc/permissions; gosrc/tools/mcp_tools.go",
		Status:     "pending-red-skip",
		UnskipWhen: "task_15 wires MCP permission policy, approval routing, and channel/elicitation relay",
		Acceptance: "MCP calls respect policy allow/deny/ask decisions and surface elicitation without bypassing approvals",
	},
}

func TestMCPParityContractMatrixCoversTask01HighPriorityGaps(t *testing.T) {
	required := map[string]bool{
		"config_schema_name_normalization_env_expansion": false,
		"dynamic_tool_names":                             false,
		"tool_invocation_result_envelope":                false,
		"resource_list_read_envelopes":                   false,
		"list_changed_invalidation":                      false,
		"oauth_needs_auth_hint":                          false,
		"capabilities_gating":                            false,
		"tool_annotations":                               false,
		"binary_resource_persistence":                    false,
		"reconnect_session_cache_clearing":               false,
		"prompts_commands_skills":                        false,
		"permissions_policy_channel_relay":               false,
	}
	seen := make(map[string]bool)
	for _, c := range mcpParityContracts {
		if c.Key == "" || c.TSSource == "" || c.GoSurface == "" || c.Status == "" || c.Acceptance == "" {
			t.Fatalf("contract %+v has empty required fields", c)
		}
		if seen[c.Key] {
			t.Fatalf("duplicate contract key %q", c.Key)
		}
		seen[c.Key] = true
		if _, ok := required[c.Key]; ok {
			required[c.Key] = true
		}
		switch c.Status {
		case "enforced":
		case "pending-red-skip":
			if c.UnskipWhen == "" {
				t.Fatalf("pending contract %q must document its unskip condition", c.Key)
			}
		default:
			t.Fatalf("contract %q has unknown status %q", c.Key, c.Status)
		}
	}
	for key, ok := range required {
		if !ok {
			t.Fatalf("missing task_01 MCP parity contract for %q", key)
		}
	}
}

func TestMCPParityContractTSMCPToolCallPreservesStructuredContentEnvelope(t *testing.T) {
	client := startMCPServer(t, "srv", handler.Map{
		"tools/call": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "hello", "mimeType": "text/plain"},
					{"type": "text", "text": `{"ok":true}`, "mimeType": "application/json"},
				},
				"isError": false,
			}, nil
		}),
	})
	mgr := injectServer("srv", client, []MCPServerTool{{Name: "echo", Description: "echoes"}})
	tool := NewMCPTool(mgr)

	result, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "srv",
		"tool_name":   "echo",
		"arguments":   map[string]any{"value": "hello"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	envelope := mustMCPJSONObject(t, result.Content)
	content := mustMCPJSONArrayField(t, envelope, "content")
	if len(content) != 2 {
		t.Fatalf("content length = %d, want 2", len(content))
	}
	if content[0]["type"] != "text" || content[0]["text"] != "hello" || content[0]["mimeType"] != "text/plain" {
		t.Fatalf("first content block did not preserve type/text/mimeType: %#v", content[0])
	}
	if !strings.Contains(content[1]["text"].(string), "\n") {
		t.Fatalf("application/json text should be pretty-printed for model parseability: %#v", content[1])
	}
	if envelope["isError"] != false {
		t.Fatalf("isError = %v, want false", envelope["isError"])
	}
}

func TestMCPParityContractTSResourcesListAndReadPreserveEnvelopes(t *testing.T) {
	client := startMCPServer(t, "fs", handler.Map{
		"resources/list": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"resources": []map[string]any{{
					"uri":         "memo://alpha",
					"name":        "Alpha",
					"description": "A memo",
					"mimeType":    "text/markdown",
				}},
			}, nil
		}),
		"resources/read": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"contents": []map[string]any{{
					"uri":      "memo://alpha",
					"mimeType": "text/markdown",
					"text":     "# Alpha",
				}},
			}, nil
		}),
	})
	mgr := injectServer("fs", client, nil)

	listResult, err := NewListMcpResourcesTool(mgr).Execute(context.Background(), map[string]any{"server": "fs"})
	if err != nil {
		t.Fatalf("ListMcpResources Execute: %v", err)
	}
	var resources []map[string]any
	if err := json.Unmarshal([]byte(listResult.Content), &resources); err != nil {
		t.Fatalf("ListMcpResources should return TS flat resource array, got %q: %v", listResult.Content, err)
	}
	if len(resources) != 1 {
		t.Fatalf("resources length = %d, want 1", len(resources))
	}
	if resources[0]["server"] != "fs" || resources[0]["uri"] != "memo://alpha" || resources[0]["mimeType"] != "text/markdown" || resources[0]["description"] != "A memo" {
		t.Fatalf("resource envelope did not preserve uri/mimeType/description: %#v", resources[0])
	}

	readResult, err := NewReadMcpResourceTool(mgr).Execute(context.Background(), map[string]any{"server": "fs", "uri": "memo://alpha"})
	if err != nil {
		t.Fatalf("ReadMcpResource Execute: %v", err)
	}
	contents := mustMCPJSONArrayField(t, mustMCPJSONObject(t, readResult.Content), "contents")
	if len(contents) != 1 {
		t.Fatalf("contents length = %d, want 1", len(contents))
	}
	if contents[0]["uri"] != "memo://alpha" || contents[0]["mimeType"] != "text/markdown" || contents[0]["text"] != "# Alpha" {
		t.Fatalf("read envelope did not preserve uri/mimeType/text: %#v", contents[0])
	}
}

func TestMCPParityContractTSListChangedInvalidatesToolAndResourceCatalogs(t *testing.T) {
	var listCalls atomic.Int32
	client := startMCPServer(t, "catalog", handler.Map{
		"tools/list": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			listCalls.Add(1)
			return map[string]any{
				"tools": []map[string]any{{
					"name":        "tool_after_change",
					"description": "refreshed",
					"inputSchema": map[string]any{"type": "object"},
				}},
			}, nil
		}),
	})
	mgr := injectServer("catalog", client, []MCPServerTool{{Name: "tool_before_change", Description: "stale"}})

	var toolInvalidations atomic.Int32
	prevToolInvalidator := currentMCPToolSearchInvalidator()
	defer SetToolSearchInvalidator(prevToolInvalidator)
	SetToolSearchInvalidator(func(serverName string) {
		if serverName == "catalog" {
			toolInvalidations.Add(1)
		}
	})

	var resourceInvalidations atomic.Int32
	prevResourceInvalidator := currentMCPResourceListInvalidator()
	defer SetResourceListInvalidator(prevResourceInvalidator)
	SetResourceListInvalidator(func(serverName string) {
		if serverName == "catalog" {
			resourceInvalidations.Add(1)
		}
	})

	mgr.HandleMCPNotification(context.Background(), "catalog", "notifications/tools/list_changed", json.RawMessage(`{}`))
	mgr.mu.RLock()
	updatedTools := append([]MCPServerTool(nil), mgr.servers["catalog"].tools...)
	mgr.mu.RUnlock()
	if len(updatedTools) != 1 || updatedTools[0].Name != "tool_after_change" {
		t.Fatalf("tools/list_changed did not refresh cached tools: %#v", updatedTools)
	}
	if listCalls.Load() != 1 {
		t.Fatalf("tools/list should be called once during refresh, got %d", listCalls.Load())
	}
	if toolInvalidations.Load() != 1 {
		t.Fatalf("tool invalidator fired %d times, want 1", toolInvalidations.Load())
	}

	mgr.HandleMCPNotification(context.Background(), "catalog", "notifications/resources/list_changed", json.RawMessage(`{}`))
	if !ResourcesChanged("catalog") {
		t.Fatalf("resources/list_changed should mark resources dirty")
	}
	if !ConsumeResourcesChanged("catalog") || ResourcesChanged("catalog") {
		t.Fatalf("ConsumeResourcesChanged should read and clear the dirty flag")
	}
	if resourceInvalidations.Load() != 1 {
		t.Fatalf("resource invalidator fired %d times, want 1", resourceInvalidations.Load())
	}
}

func TestMCPParityContractTSOAuth401SurfacesNeedsAuthHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="mcp", as_uri="https://auth.example.test/oauth"`)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	mgr := NewMCPManager()
	mgr.AddServer(&MCPServer{
		Name:    "auth-srv",
		BaseURL: server.URL,
		Tools:   []MCPServerTool{{Name: "ping", Description: "requires auth"}},
	})
	result, err := NewMCPTool(mgr).Execute(context.Background(), map[string]any{
		"server_name": "auth-srv",
		"tool_name":   "ping",
		"arguments":   map[string]any{},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("401 should be surfaced as a tool-level needs-auth error")
	}
	envelope := mustMCPJSONObject(t, result.Content)
	if envelope["error"] != "oauth_required" || envelope["status"] != float64(http.StatusUnauthorized) {
		t.Fatalf("unexpected oauth hint: %#v", envelope)
	}
	if !strings.Contains(envelope["www_authenticate"].(string), "as_uri=") {
		t.Fatalf("oauth hint must preserve WWW-Authenticate challenge: %#v", envelope)
	}
	if mcpsvc.PKCEChallengeMethod() != mcpsvc.PKCEChallengeMethodS256 {
		t.Fatalf("PKCE challenge method = %q, want S256", mcpsvc.PKCEChallengeMethod())
	}
}

func TestMCPParityContractPendingTargetBehaviorRedTests(t *testing.T) {
	pending := []mcpParityContract{}
	for _, c := range mcpParityContracts {
		if c.Status == "pending-red-skip" {
			pending = append(pending, c)
		}
	}
	if len(pending) == 0 {
		t.Fatalf("expected at least one pending target-behavior red test while task_02-task_18 are incomplete")
	}
	for _, contract := range pending {
		contract := contract
		t.Run(contract.Key, func(t *testing.T) {
			t.Skipf("TS parity target not implemented in task_01. TS source: %s. Go surface: %s. Unskip when: %s. Acceptance: %s",
				contract.TSSource, contract.GoSurface, contract.UnskipWhen, contract.Acceptance)
		})
	}
}

func mustMCPJSONObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("expected JSON object, got %q: %v", raw, err)
	}
	return out
}

func mustMCPJSONArrayField(t *testing.T, obj map[string]any, field string) []map[string]any {
	t.Helper()
	raw, ok := obj[field]
	if !ok {
		t.Fatalf("missing JSON field %q in %#v", field, obj)
	}
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("field %q is %T, want JSON array", field, raw)
	}
	out := make([]map[string]any, 0, len(items))
	for i, item := range items {
		asMap, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("field %q[%d] is %T, want JSON object", field, i, item)
		}
		out = append(out, asMap)
	}
	return out
}

func currentMCPToolSearchInvalidator() ToolSearchInvalidator {
	mcpNotificationMu.RLock()
	defer mcpNotificationMu.RUnlock()
	return toolSearchInvalidator
}

func currentMCPResourceListInvalidator() ResourceListInvalidator {
	mcpNotificationMu.RLock()
	defer mcpNotificationMu.RUnlock()
	return resourceListInvalidator
}
