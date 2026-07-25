package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type fakeDynamicMCPManager struct {
	states []mcpmanager.MCPServerConnection
	err    error
}

func (m *fakeDynamicMCPManager) Snapshot() []mcpmanager.MCPServerConnection {
	out := make([]mcpmanager.MCPServerConnection, len(m.states))
	copy(out, m.states)
	return out
}

func (m *fakeDynamicMCPManager) GetOrConnect(_ context.Context, name string) (mcpmanager.MCPServerConnection, error) {
	if m.err != nil {
		return mcpmanager.MCPServerConnection{}, m.err
	}
	for _, state := range m.states {
		if state.Name == name {
			return state, nil
		}
	}
	return mcpmanager.MCPServerConnection{}, errors.New("missing fake MCP server")
}

type recordingMCPRawCaller struct {
	result json.RawMessage
	method string
	params map[string]any
}

func (r *recordingMCPRawCaller) CallRaw(_ context.Context, method string, params any, out any) error {
	r.method = method
	data, _ := json.Marshal(params)
	_ = json.Unmarshal(data, &r.params)
	switch target := out.(type) {
	case *json.RawMessage:
		*target = append(json.RawMessage(nil), r.result...)
	default:
		return json.Unmarshal(r.result, out)
	}
	return nil
}

type unrelatedMCPNamedTool struct {
	name string
	desc string
}

func (t unrelatedMCPNamedTool) Name() string        { return t.name }
func (t unrelatedMCPNamedTool) Description() string { return t.desc }
func (t unrelatedMCPNamedTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t unrelatedMCPNamedTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: "ok"}, nil
}

func TestMCPDynamicToolNameMetadataSchemaAndExecute(t *testing.T) {
	raw := &recordingMCPRawCaller{
		result: json.RawMessage(`{"content":[{"type":"text","text":"ok"}],"structuredContent":{"count":1},"_meta":{"trace":"abc"},"isError":false}`),
	}
	client := newMCPProtocolTestClient(t, raw)
	manager := &fakeDynamicMCPManager{states: []mcpmanager.MCPServerConnection{{
		Name:   "My Server.Name",
		Type:   mcpmanager.MCPStateConnected,
		Client: client,
	}}}
	tool := NewDynamicMCPTool(manager, "My Server.Name", catalog.ToolDefinition{
		Name:        "Search Issues!",
		Description: "Find issues",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"terms"},"limit":{"type":"number"}},"required":["query"],"description":"Search args"}`),
		Annotations: map[string]any{
			"readOnlyHint":    true,
			"destructiveHint": false,
			"openWorldHint":   true,
			"title":           "Search Issues",
		},
		Meta: map[string]any{
			"anthropic/searchHint": " issue\n search   tracker ",
			"anthropic/alwaysLoad": true,
		},
	})

	if got, want := tool.Name(), "mcp__My_Server_Name__Search_Issues_"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
	if got, want := tool.MCPServerName(), "My Server.Name"; got != want {
		t.Fatalf("MCPServerName() = %q, want %q", got, want)
	}
	if got, want := tool.MCPToolName(), "Search Issues!"; got != want {
		t.Fatalf("MCPToolName() = %q, want %q", got, want)
	}
	metadata := tool.ToolMetadata(nil)
	if !metadata.ReadOnly || !metadata.ConcurrencySafe || metadata.Search || metadata.Write || metadata.Destructive {
		t.Fatalf("ToolMetadata = %+v, want read-only and concurrent-safe", metadata)
	}
	meta := registry.DiscoveryMetadata(tool)
	if !meta.ShouldDefer || !meta.AlwaysLoad || meta.SearchHint != "issue search tracker" {
		t.Fatalf("DiscoveryMetadata = %+v, want defer+alwaysLoad+collapsed search hint", meta)
	}
	if registry.PermissionIdentity(tool) != tool.Name() {
		t.Fatalf("PermissionIdentity = %q, want dynamic full name", registry.PermissionIdentity(tool))
	}

	schema := tool.Schema()
	if schema.Type != "object" || schema.Description != "Search args" {
		t.Fatalf("Schema header = %+v", schema)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "query" {
		t.Fatalf("Schema.Required = %v, want [query]", schema.Required)
	}
	if _, ok := schema.Properties["query"].(map[string]any); !ok {
		t.Fatalf("Schema.Properties did not preserve nested query schema: %#v", schema.Properties["query"])
	}

	result, err := tool.Execute(context.Background(), map[string]any{"query": "bug"})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if raw.method != "tools/call" {
		t.Fatalf("CallRaw method = %q, want tools/call", raw.method)
	}
	if raw.params["name"] != "Search Issues!" {
		t.Fatalf("tools/call name = %#v, want original unnormalized name", raw.params["name"])
	}
	args, _ := raw.params["arguments"].(map[string]any)
	if args["query"] != "bug" {
		t.Fatalf("tools/call arguments = %#v", raw.params["arguments"])
	}
	if result.IsError || !strings.Contains(result.TextContent(), "ok") {
		t.Fatalf("Execute result = %+v", result)
	}
	if result.Metadata["mcp.serverName"] != "My Server.Name" || result.Metadata["mcp.toolName"] != "Search Issues!" {
		t.Fatalf("result metadata missing MCP identity: %#v", result.Metadata)
	}
	if !strings.Contains(result.Metadata["mcp.structuredContent"], `"count":1`) {
		t.Fatalf("structuredContent metadata missing: %#v", result.Metadata)
	}
}

var _ types.ToolMetadataProvider = (*DynamicMCPTool)(nil)

func TestMCPDynamicToolsSnapshotIncludesNeedsAuthPseudoTool(t *testing.T) {
	manager := &fakeDynamicMCPManager{states: []mcpmanager.MCPServerConnection{
		{
			Name: "docs",
			Type: mcpmanager.MCPStateConnected,
			Tools: []catalog.ToolDefinition{{
				Name:        "lookup",
				Description: "Lookup docs",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			}},
		},
		{
			Name:   "auth server",
			Type:   mcpmanager.MCPStateNeedsAuth,
			Config: catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "https://example.invalid/mcp"},
		},
	}}

	built := BuildDynamicMCPToolsFromSnapshot(manager, nil, nil)
	if len(built) != 2 {
		t.Fatalf("built %d dynamic tools, want 2", len(built))
	}
	names := map[string]types.Tool{}
	for _, tool := range built {
		names[tool.Name()] = tool
	}
	if names["mcp__docs__lookup"] == nil {
		t.Fatalf("connected MCP tool not built: %v", names)
	}
	authTool := names["mcp__auth_server__authenticate"]
	if authTool == nil {
		t.Fatalf("needs-auth pseudo-tool not built: %v", names)
	}
	registration, ok := authTool.(registry.MCPDynamicRegisteredTool)
	if !ok {
		t.Fatalf("auth pseudo-tool does not carry MCP registration marker")
	}
	if got := registration.MCPDynamicRegistration(); got.Kind != "authenticate" || got.ServerName != "auth server" {
		t.Fatalf("auth registration = %+v", got)
	}
}

func TestMCPDynamicToolDeferredVisibility(t *testing.T) {
	manager := &fakeDynamicMCPManager{states: []mcpmanager.MCPServerConnection{{
		Name: "my server",
		Type: mcpmanager.MCPStateConnected,
		Tools: []catalog.ToolDefinition{{
			Name:        "lookup",
			Description: "Lookup project documents",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Meta: map[string]any{
				"anthropic/searchHint": "project docs lookup",
			},
		}},
	}}}
	reg := registry.New()
	reg.Register(unrelatedMCPNamedTool{name: "ToolSearch", desc: "discover tools"})
	reg.SyncMCPDynamicTools(BuildDynamicMCPToolsFromSnapshot(manager, nil, nil))

	if reg.Get("MCPTool") != nil {
		t.Fatalf("test registry should not contain generic MCPTool")
	}
	if got := reg.Get("mcp__my_server__lookup"); got == nil {
		t.Fatalf("dynamic MCP tool not registered")
	}
	if defs := reg.VisibleDefinitions(nil); len(defs) != 1 || defs[0].Name != "ToolSearch" {
		t.Fatalf("dynamic MCP tool should be deferred before select, visible defs=%#v", defs)
	}

	loaded := map[string]struct{}{"mcp__my_server__lookup": {}}
	defs := reg.VisibleDefinitions(loaded)
	if len(defs) != 2 || defs[0].Name != "ToolSearch" || defs[1].Name != "mcp__my_server__lookup" {
		t.Fatalf("dynamic MCP tool should be visible after select, defs=%#v", defs)
	}
}

func TestMCPDynamicRegistrySyncDoesNotRemoveUnrelatedMCPNames(t *testing.T) {
	reg := registry.New()
	unrelated := unrelatedMCPNamedTool{name: "mcp__prompt__command", desc: "unrelated mcp command"}
	reg.Register(unrelated)

	manager := &fakeDynamicMCPManager{states: []mcpmanager.MCPServerConnection{{
		Name:  "old",
		Type:  mcpmanager.MCPStateConnected,
		Tools: []catalog.ToolDefinition{{Name: "gone", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	}}}
	reg.SyncMCPDynamicTools(BuildDynamicMCPToolsFromSnapshot(manager, nil, nil))
	if reg.Get("mcp__old__gone") == nil {
		t.Fatalf("expected old dynamic tool")
	}

	manager.states = nil
	reg.SyncMCPDynamicTools(BuildDynamicMCPToolsFromSnapshot(manager, nil, nil))
	if reg.Get("mcp__old__gone") != nil {
		t.Fatalf("stale dynamic MCP tool was not removed")
	}
	if reg.Get("mcp__prompt__command") == nil {
		t.Fatalf("unrelated mcp__ tool was removed by dynamic sync")
	}
}

func TestTruncateMCPDynamicDescriptionPreservesUTF8Runes(t *testing.T) {
	value := strings.Repeat("界", maxMCPDynamicDescriptionLength+12)
	tool := NewDynamicMCPTool(nil, "fixture", catalog.ToolDefinition{Name: "lookup", Description: value})
	got := tool.Description()
	if !utf8.ValidString(got) {
		t.Fatalf("truncated MCP tool description is invalid UTF-8: %q", got)
	}
	want := strings.Repeat("界", maxMCPDynamicDescriptionLength) + "\u2026 [truncated]"
	if got != want {
		t.Fatalf("truncated MCP tool description rune count = %d, want %d", utf8.RuneCountInString(got), utf8.RuneCountInString(want))
	}
	short := strings.Repeat("界", maxMCPDynamicDescriptionLength)
	shortTool := NewDynamicMCPTool(nil, "fixture", catalog.ToolDefinition{Name: "lookup", Description: short})
	if got := shortTool.Description(); got != short {
		t.Fatalf("MCP tool description at rune limit changed: rune count=%d", utf8.RuneCountInString(got))
	}
}
