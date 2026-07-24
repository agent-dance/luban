package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
)

type fakeDynamicMCPManager struct {
	states []svcmcp.MCPServerConnection
	err    error
}

func (m *fakeDynamicMCPManager) Snapshot() []svcmcp.MCPServerConnection {
	out := make([]svcmcp.MCPServerConnection, len(m.states))
	copy(out, m.states)
	return out
}

func (m *fakeDynamicMCPManager) GetOrConnect(_ context.Context, name string) (svcmcp.MCPServerConnection, error) {
	if m.err != nil {
		return svcmcp.MCPServerConnection{}, m.err
	}
	for _, state := range m.states {
		if state.Name == name {
			return state, nil
		}
	}
	return svcmcp.MCPServerConnection{}, errors.New("missing fake MCP server")
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
	client := svcmcp.NewClient(raw, nil)
	manager := &fakeDynamicMCPManager{states: []svcmcp.MCPServerConnection{{
		Name:   "My Server.Name",
		Type:   svcmcp.MCPStateConnected,
		Client: client,
	}}}
	tool := NewDynamicMCPTool(manager, "My Server.Name", svcmcp.ToolDefinition{
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
	if !tool.IsConcurrentSafe() || !tool.IsReadOnly() || tool.IsDestructive() || !tool.IsOpenWorld() {
		t.Fatalf("annotation helpers not preserved: readOnly=%v destructive=%v openWorld=%v", tool.IsReadOnly(), tool.IsDestructive(), tool.IsOpenWorld())
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

func TestMCPDynamicToolsSnapshotIncludesNeedsAuthPseudoTool(t *testing.T) {
	manager := &fakeDynamicMCPManager{states: []svcmcp.MCPServerConnection{
		{
			Name: "docs",
			Type: svcmcp.MCPStateConnected,
			Tools: []svcmcp.ToolDefinition{{
				Name:        "lookup",
				Description: "Lookup docs",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			}},
		},
		{
			Name:   "auth server",
			Type:   svcmcp.MCPStateNeedsAuth,
			Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportHTTP, URL: "https://example.invalid/mcp"},
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

func TestMCPDynamicToolSearchSelectAndVisibility(t *testing.T) {
	manager := &fakeDynamicMCPManager{states: []svcmcp.MCPServerConnection{{
		Name: "my server",
		Type: svcmcp.MCPStateConnected,
		Tools: []svcmcp.ToolDefinition{{
			Name:        "lookup",
			Description: "Lookup project documents",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Meta: map[string]any{
				"anthropic/searchHint": "project docs lookup",
			},
		}},
	}}}
	reg := registry.New()
	search := &ToolSearchTool{Registry: reg}
	reg.Register(search)
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

	result, err := search.Execute(context.Background(), map[string]any{
		"query": "select:mcp__my_server__lookup",
	})
	if err != nil {
		t.Fatalf("ToolSearch Execute error = %v", err)
	}
	if len(result.ContentBlocks) != 1 {
		t.Fatalf("ToolSearch content blocks = %#v content=%q", result.ContentBlocks, result.Content)
	}
	ref, ok := result.ContentBlocks[0].(types.ToolReferenceBlock)
	if !ok || ref.ToolName != "mcp__my_server__lookup" {
		t.Fatalf("ToolSearch ref = %#v, want mcp__my_server__lookup", result.ContentBlocks[0])
	}

	loaded := map[string]struct{}{"mcp__my_server__lookup": {}}
	defs := reg.VisibleDefinitions(loaded)
	if len(defs) != 2 || defs[1].Name != "mcp__my_server__lookup" {
		t.Fatalf("dynamic MCP tool should be visible after select, defs=%#v", defs)
	}
}

func TestVisibilityMCPDynamicAlwaysLoadAndGenericFallback(t *testing.T) {
	manager := &fakeDynamicMCPManager{states: []svcmcp.MCPServerConnection{{
		Name: "always",
		Type: svcmcp.MCPStateConnected,
		Tools: []svcmcp.ToolDefinition{{
			Name:        "visible",
			Description: "Always visible",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Meta: map[string]any{
				"anthropic/alwaysLoad": true,
			},
		}},
	}}}
	reg := registry.New()
	reg.Register(&ToolSearchTool{Registry: reg})
	reg.Register(NewMCPTool(NewMCPManager()))
	reg.SyncMCPDynamicTools(BuildDynamicMCPToolsFromSnapshot(manager, nil, nil))

	if !registry.IsDeferredTool(reg.Get("MCPTool")) {
		t.Fatalf("generic MCPTool should be deferred as compat/admin fallback")
	}
	dynamic := reg.Get("mcp__always__visible")
	if dynamic == nil {
		t.Fatalf("always-load dynamic MCP tool not registered")
	}
	if registry.IsDeferredTool(dynamic) {
		t.Fatalf("always-load dynamic MCP tool should bypass deferral")
	}
	defs := reg.VisibleDefinitions(nil)
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	if !containsMCPDynamicTestName(names, "mcp__always__visible") {
		t.Fatalf("always-load dynamic MCP tool missing from visible defs: %v", names)
	}
	if containsMCPDynamicTestName(names, "MCPTool") {
		t.Fatalf("generic MCPTool should not be initially visible: %v", names)
	}
}

func TestMCPDynamicRegistrySyncDoesNotRemoveUnrelatedMCPNames(t *testing.T) {
	reg := registry.New()
	unrelated := unrelatedMCPNamedTool{name: "mcp__prompt__command", desc: "unrelated mcp command"}
	reg.Register(unrelated)

	manager := &fakeDynamicMCPManager{states: []svcmcp.MCPServerConnection{{
		Name:  "old",
		Type:  svcmcp.MCPStateConnected,
		Tools: []svcmcp.ToolDefinition{{Name: "gone", InputSchema: json.RawMessage(`{"type":"object"}`)}},
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

func containsMCPDynamicTestName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
