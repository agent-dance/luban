package tools

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcppkg "github.com/agent-dance/luban/mcp"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"
)

// ─── test helpers ─────────────────────────────────────────────────────────────

// startMCPServer spins up an in-memory jrpc2 server and returns a connected
// mcp.Client. Default "initialize" and "notifications/initialized" handlers are
// injected when not present. Server and client are cleaned up via t.Cleanup.
func startMCPServer(t *testing.T, name string, methods handler.Map) *mcppkg.Client {
	t.Helper()
	if _, ok := methods["initialize"]; !ok {
		methods["initialize"] = handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"resources": map[string]any{},
				},
			}, nil
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

	c, err := mcppkg.NewClientFromChannel(name, channel.Line(cliR, cliW))
	if err != nil {
		t.Fatalf("NewClientFromChannel: %v", err)
	}
	t.Cleanup(func() { c.Close() }) //nolint:errcheck
	return c
}

// injectServer wires an already-established mcp.Client into a fresh MCPManager
// under the given name, with the provided tools cache. Returns the manager.
func injectServer(name string, client *mcppkg.Client, tools []MCPServerTool) *MCPManager {
	m := NewMCPManager()
	m.injectConn(name, &MCPServerConn{
		client: client,
		tools:  tools,
		ready:  true,
	})
	return m
}

// ─── MCPManager ───────────────────────────────────────────────────────────────

func TestMCPManager_NewIsEmpty(t *testing.T) {
	m := NewMCPManager()
	if names := m.ServerNames(); len(names) != 0 {
		t.Errorf("expected no servers, got %v", names)
	}
}

func TestMCPManager_AddConfig(t *testing.T) {
	m := NewMCPManager()
	m.AddConfig("myserver", MCPServerConfig{Command: "npx", Args: []string{"-y", "my-server"}})
	names := m.ServerNames()
	if len(names) != 1 || names[0] != "myserver" {
		t.Errorf("expected [myserver], got %v", names)
	}
}

func TestMCPManagerReplaceWorkspaceConfigsRestoresShadowedUserConfig(t *testing.T) {
	m := NewMCPManager()
	m.AddConfig("shared", MCPServerConfig{Command: "user-command", Scope: string(svcmcp.ScopeUser)})
	m.ReplaceWorkspaceConfigs(map[string]MCPServerConfig{
		"shared": {Command: "project-command", Scope: string(svcmcp.ScopeProject)},
	})
	m.ReplaceWorkspaceConfigs(nil)

	if names := m.ServerNames(); len(names) != 1 || names[0] != "shared" {
		t.Fatalf("shadowed user config was not restored: %v", names)
	}
}

func TestMCPManager_ServerNames_Sorted(t *testing.T) {
	m := NewMCPManager()
	m.AddConfig("zebra", MCPServerConfig{Command: "z"})
	m.AddConfig("alpha", MCPServerConfig{Command: "a"})
	m.AddConfig("mango", MCPServerConfig{Command: "m"})
	names := m.ServerNames()
	if len(names) != 3 || names[0] != "alpha" || names[1] != "mango" || names[2] != "zebra" {
		t.Errorf("unexpected order: %v", names)
	}
}

func TestMCPManager_ConnectUnconfigured(t *testing.T) {
	m := NewMCPManager()
	_, err := m.Connect("ghost")
	if err == nil {
		t.Fatal("expected error for unconfigured server")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("expected server name in error: %v", err)
	}
}

func TestMCPManager_ConnectReturnsCached(t *testing.T) {
	client := startMCPServer(t, "srv", handler.Map{})
	m := injectServer("srv", client, nil)

	conn1, err := m.Connect("srv")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	conn2, err := m.Connect("srv")
	if err != nil {
		t.Fatalf("Connect (cached): %v", err)
	}
	if conn1 != conn2 {
		t.Error("expected same connection pointer from cache")
	}
}

func TestMCPManager_Shutdown(t *testing.T) {
	m := NewMCPManager()
	m.injectConn("srv", &MCPServerConn{ready: false})
	m.Shutdown()
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.servers) != 0 {
		t.Errorf("expected empty servers after Shutdown, got %d", len(m.servers))
	}
}

// ─── LoadFromSettings ─────────────────────────────────────────────────────────

func TestLoadFromSettings_MissingFile(t *testing.T) {
	m := NewMCPManager()
	err := m.LoadFromSettings(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Errorf("expected no error for missing file, got: %v", err)
	}
}

func TestLoadFromSettings_ValidFile(t *testing.T) {
	settings := `{
		"mcpServers": {
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
			},
			"github": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-github"],
				"env": {"GITHUB_TOKEN": "tok"}
			}
		}
	}`
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(settings), 0644) //nolint:errcheck

	m := NewMCPManager()
	if err := m.LoadFromSettings(path); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}

	names := m.ServerNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 servers, got %v", names)
	}
	// Names are sorted.
	if names[0] != "filesystem" || names[1] != "github" {
		t.Errorf("unexpected names: %v", names)
	}

	m.mu.RLock()
	fsCfg := m.configs["filesystem"]
	ghCfg := m.configs["github"]
	m.mu.RUnlock()

	if fsCfg.Command != "npx" {
		t.Errorf("filesystem command: want npx, got %s", fsCfg.Command)
	}
	if len(fsCfg.Args) != 3 {
		t.Errorf("filesystem args: want 3, got %d", len(fsCfg.Args))
	}
	if ghCfg.Env["GITHUB_TOKEN"] != "tok" {
		t.Errorf("github env token: want 'tok', got %q", ghCfg.Env["GITHUB_TOKEN"])
	}
}

func TestMCPConfigNameLoadFromSettingsPreservesRemoteFieldsAndScope(t *testing.T) {
	t.Setenv("MCP_TOKEN", "secret")
	settings := `{
		"mcpServers": {
			"remote": {
				"type": "http",
				"url": "https://example.com/mcp",
				"headers": {"Authorization": "Bearer ${MCP_TOKEN}"},
				"headersHelper": "op read token",
				"oauth": {
					"clientId": "client-1",
					"callbackPort": 8765,
					"authServerMetadataUrl": "https://auth.example.com/.well-known/oauth-authorization-server",
					"xaa": true
				}
			},
			"sdk": {
				"type": "sdk",
				"name": "claude-vscode"
			}
		}
	}`
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(settings), 0644) //nolint:errcheck

	m := NewMCPManager()
	if err := m.LoadFromSettings(path); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}

	m.mu.RLock()
	remote := m.configs["remote"]
	sdk := m.configs["sdk"]
	m.mu.RUnlock()

	if remote.Type != "http" || remote.URL != "https://example.com/mcp" {
		t.Fatalf("remote transport fields not preserved: %#v", remote)
	}
	if remote.Headers["Authorization"] != "Bearer secret" {
		t.Fatalf("header env expansion = %q", remote.Headers["Authorization"])
	}
	if remote.HeadersHelper != "op read token" {
		t.Fatalf("headersHelper = %q", remote.HeadersHelper)
	}
	if remote.Scope != "project" {
		t.Fatalf("scope = %q, want project", remote.Scope)
	}
	if remote.OAuth == nil || remote.OAuth.ClientID != "client-1" || remote.OAuth.CallbackPort == nil || *remote.OAuth.CallbackPort != 8765 {
		t.Fatalf("oauth fields not preserved: %#v", remote.OAuth)
	}
	if remote.OAuth.XAA == nil || !*remote.OAuth.XAA {
		t.Fatalf("oauth xaa not preserved: %#v", remote.OAuth)
	}
	if sdk.Type != "sdk" || sdk.Name != "claude-vscode" {
		t.Fatalf("sdk fields not preserved: %#v", sdk)
	}
}

func TestLoadFromSettings_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{invalid}`), 0644) //nolint:errcheck

	m := NewMCPManager()
	if err := m.LoadFromSettings(path); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadFromSettings_EmptyMCPServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"mcpServers": {}}`), 0644) //nolint:errcheck

	m := NewMCPManager()
	if err := m.LoadFromSettings(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if names := m.ServerNames(); len(names) != 0 {
		t.Errorf("expected no servers, got %v", names)
	}
}

func TestLoadFromSettings_NoMCPServersKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"hooks": {}}`), 0644) //nolint:errcheck

	m := NewMCPManager()
	if err := m.LoadFromSettings(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if names := m.ServerNames(); len(names) != 0 {
		t.Errorf("expected no servers, got %v", names)
	}
}

// ─── MCPTool metadata ─────────────────────────────────────────────────────────

func TestMCPTool_Metadata(t *testing.T) {
	tool := NewMCPTool(NewMCPManager())
	if tool.Name() != "MCPTool" {
		t.Errorf("expected MCPTool, got %s", tool.Name())
	}
	if !tool.IsConcurrentSafe() {
		t.Error("expected IsConcurrentSafe=true")
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
	schema := tool.Schema()
	if _, ok := schema.Properties["server_name"]; !ok {
		t.Error("schema missing server_name")
	}
	if _, ok := schema.Properties["tool_name"]; !ok {
		t.Error("schema missing tool_name")
	}
	if _, ok := schema.Properties["arguments"]; !ok {
		t.Error("schema missing arguments")
	}
}

// ─── MCPTool.Execute validation ───────────────────────────────────────────────

func TestMCPTool_MissingServerName(t *testing.T) {
	tool := NewMCPTool(NewMCPManager())
	res, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "",
		"tool_name":   "do-thing",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "server_name") {
		t.Errorf("expected server_name error, got: %s", res.Content)
	}
}

func TestMCPTool_MissingToolName(t *testing.T) {
	tool := NewMCPTool(NewMCPManager())
	res, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "srv",
		"tool_name":   "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "tool_name") {
		t.Errorf("expected tool_name error, got: %s", res.Content)
	}
}

func TestMCPTool_ServerNotConfigured(t *testing.T) {
	m := NewMCPManager()
	m.AddConfig("known", MCPServerConfig{Command: "npx"})
	tool := NewMCPTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "unknown",
		"tool_name":   "do-thing",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for unconfigured server")
	}
	if !strings.Contains(res.Content, "unknown") {
		t.Errorf("expected server name in error: %s", res.Content)
	}
}

func TestMCPTool_ToolNotFound(t *testing.T) {
	client := startMCPServer(t, "srv", handler.Map{})
	m := injectServer("srv", client, []MCPServerTool{
		{Name: "tool-a", Description: "Tool A"},
		{Name: "tool-b", Description: "Tool B"},
	})
	tool := NewMCPTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "srv",
		"tool_name":   "tool-c",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for missing tool")
	}
	if !strings.Contains(res.Content, "tool-c") {
		t.Errorf("expected missing tool name in error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "tool-a") {
		t.Errorf("expected available tools listed: %s", res.Content)
	}
}

func TestMCPTool_SuccessfulCall(t *testing.T) {
	client := startMCPServer(t, "srv", handler.Map{
		"tools/call": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"content": []map[string]any{{"type": "text", "text": "result-value"}},
			}, nil
		}),
	})
	m := injectServer("srv", client, []MCPServerTool{{Name: "my-tool", Description: "does stuff"}})
	tool := NewMCPTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "srv",
		"tool_name":   "my-tool",
		"arguments":   map[string]any{"x": 42},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected tool error: %s", res.Content)
	}
	envelope := mustMCPJSONObject(t, res.Content)
	content := mustMCPJSONArrayField(t, envelope, "content")
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	if content[0]["text"] != "result-value" {
		t.Errorf("expected content[0].text='result-value', got %v", content[0]["text"])
	}
	if content[0]["type"] != "text" {
		t.Errorf("expected content[0].type='text', got %v", content[0]["type"])
	}
	if envelope["isError"] != false {
		t.Errorf("expected isError=false, got %v", envelope["isError"])
	}
}

func TestMCPTool_SkipsToolValidationWhenCacheEmpty(t *testing.T) {
	// When tools cache is nil, validation is skipped and the call goes through.
	client := startMCPServer(t, "srv", handler.Map{
		"tools/call": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"content": []map[string]any{{"type": "text", "text": "ok"}},
			}, nil
		}),
	})
	m := injectServer("srv", client, nil)
	tool := NewMCPTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "srv",
		"tool_name":   "any-tool",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error: %s", res.Content)
	}
}

func TestMCPTool_PassesArguments(t *testing.T) {
	var gotName string
	var gotArgs map[string]any

	client := startMCPServer(t, "srv", handler.Map{
		"tools/call": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			var req struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			json.Unmarshal(p, &req) //nolint:errcheck
			gotName = req.Name
			gotArgs = req.Arguments
			return map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}}, nil
		}),
	})
	m := injectServer("srv", client, nil)
	tool := NewMCPTool(m)

	tool.Execute(context.Background(), map[string]any{ //nolint:errcheck
		"server_name": "srv",
		"tool_name":   "do-thing",
		"arguments":   map[string]any{"key": "value"},
	})

	if gotName != "do-thing" {
		t.Errorf("expected tool name 'do-thing', got %q", gotName)
	}
	if gotArgs["key"] != "value" {
		t.Errorf("expected arguments.key='value', got %v", gotArgs)
	}
}

func TestMCPTool_DefaultsToEmptyArguments(t *testing.T) {
	var gotArgs map[string]any
	client := startMCPServer(t, "srv", handler.Map{
		"tools/call": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			var req struct {
				Arguments map[string]any `json:"arguments"`
			}
			json.Unmarshal(p, &req) //nolint:errcheck
			gotArgs = req.Arguments
			return map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}}, nil
		}),
	})
	m := injectServer("srv", client, nil)
	tool := NewMCPTool(m)

	tool.Execute(context.Background(), map[string]any{ //nolint:errcheck
		"server_name": "srv",
		"tool_name":   "do-thing",
		// no "arguments" key
	})
	if gotArgs == nil {
		t.Error("expected non-nil arguments map sent to server")
	}
}

// ─── ListMcpResourcesTool ─────────────────────────────────────────────────────

func TestListMcpResourcesTool_Metadata(t *testing.T) {
	tool := NewListMcpResourcesTool(NewMCPManager())
	if tool.Name() != "ListMcpResourcesTool" {
		t.Errorf("expected ListMcpResourcesTool, got %s", tool.Name())
	}
	if !tool.IsConcurrentSafe() {
		t.Error("expected IsConcurrentSafe=true")
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
	if _, ok := tool.Schema().Properties["server"]; !ok {
		t.Error("schema missing server")
	}
	metadata := tool.ToolMetadata(nil)
	if !metadata.ReadOnly || !metadata.ConcurrencySafe || metadata.MaxResultSizeChars != 100_000 {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	contract := types.ResolveToolContract(tool)
	if !contract.Strict || contract.OutputSchema == nil || contract.OutputSchema.Type != "array" {
		t.Fatalf("expected flat array output schema, got %#v", contract.OutputSchema)
	}
	if !tool.Schema().RejectsUnknownFields() {
		t.Fatalf("ListMcpResourcesTool input schema must expose only the TS server field")
	}
	if got := types.ToolAutoClassifierInput(tool, map[string]any{"server": "alpha"}); got != "alpha" {
		t.Fatalf("auto-classifier input = %q, want alpha", got)
	}
	block := types.MapToolResult(tool, types.ToolResult{Content: "[]"}, "toolu_1")
	if block.Metadata["maxResultSizeChars"] != "100000" {
		t.Fatalf("expected maxResultSizeChars metadata on result block, got %#v", block.Metadata)
	}
}

func TestListMcpResourcesTool_NoServers(t *testing.T) {
	tool := NewListMcpResourcesTool(NewMCPManager())
	res, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error: %s", res.Content)
	}
	want := "No resources found. MCP servers may still provide tools even if they have no resources."
	if res.Content != want {
		t.Errorf("content = %q, want exact TS empty-result text %q", res.Content, want)
	}
	if res.Data == nil {
		t.Fatal("empty resource result must retain typed empty-array data")
	}
}

func TestListMcpResourcesTool_ListAll(t *testing.T) {
	m := NewMCPManager()
	m.AddConfig("alpha", MCPServerConfig{Command: "npx"})
	m.AddConfig("beta", MCPServerConfig{Command: "npx"})
	tool := NewListMcpResourcesTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error: %s", res.Content)
	}
	want := "No resources found. MCP servers may still provide tools even if they have no resources."
	if res.Content != want {
		t.Errorf("configured-but-not-connected servers must contribute [], got %q", res.Content)
	}
}

func TestListMcpResourcesTool_ListAllLegacyAggregatesFlatArray(t *testing.T) {
	alpha := startMCPServer(t, "alpha", handler.Map{
		"resources/list": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{"resources": []map[string]any{{
				"uri":      "memo://alpha",
				"name":     "Alpha",
				"mimeType": "text/markdown",
			}}}, nil
		}),
	})
	beta := startMCPServer(t, "beta", handler.Map{
		"resources/list": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{"resources": []map[string]any{{
				"uri":  "memo://beta",
				"name": "Beta",
			}}}, nil
		}),
	})
	m := NewMCPManager()
	m.injectConn("alpha", &MCPServerConn{client: alpha, ready: true})
	m.injectConn("beta", &MCPServerConn{client: beta, ready: true})
	tool := NewListMcpResourcesTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	var resources []mcpListedResource
	if err := json.Unmarshal([]byte(res.Content), &resources); err != nil {
		t.Fatalf("expected TS flat resource array, got %q: %v", res.Content, err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %#v", resources)
	}
	if resources[0].Server == "" || resources[1].Server == "" {
		t.Fatalf("expected server provenance on every resource: %#v", resources)
	}
}

func TestListMcpResourcesTool_ListOneIgnoresCachedToolsAndCallsResources(t *testing.T) {
	var calls int
	client := startMCPServer(t, "fs", handler.Map{
		"resources/list": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			calls++
			return map[string]any{"resources": []map[string]any{{
				"uri":  "memo://resource",
				"name": "Resource",
			}}}, nil
		}),
	})
	m := injectServer("fs", client, []MCPServerTool{{Name: "cached-tool", Description: "not a resource"}})
	tool := NewListMcpResourcesTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{"server": "fs"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if calls != 1 {
		t.Fatalf("resources/list calls = %d, want 1", calls)
	}
	if strings.Contains(res.Content, "cached-tool") {
		t.Fatalf("cached tools leaked into resource listing: %s", res.Content)
	}
	var resources []mcpListedResource
	if err := json.Unmarshal([]byte(res.Content), &resources); err != nil {
		t.Fatalf("expected TS flat resource array, got %q: %v", res.Content, err)
	}
	if len(resources) != 1 || resources[0].Server != "fs" || resources[0].URI != "memo://resource" {
		t.Fatalf("unexpected resources: %#v", resources)
	}
}

func TestListMcpResourcesTool_ListOne_Resources(t *testing.T) {
	client := startMCPServer(t, "fs", handler.Map{
		"resources/list": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"resources": []map[string]any{
					{"uri": "file:///tmp/a.txt", "name": "a.txt", "description": "File A"},
					{"uri": "file:///tmp/b.txt", "name": "b.txt"},
				},
			}, nil
		}),
	})
	m := injectServer("fs", client, nil)
	tool := NewListMcpResourcesTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{"server": "fs"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "a.txt") {
		t.Errorf("expected a.txt in output: %s", res.Content)
	}
	if !strings.Contains(res.Content, "file:///tmp/b.txt") {
		t.Errorf("expected b.txt URI in output: %s", res.Content)
	}
}

func TestListMcpResourcesTool_ListOne_NoResources(t *testing.T) {
	client := startMCPServer(t, "srv", handler.Map{
		"resources/list": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{"resources": []any{}}, nil
		}),
	})
	m := injectServer("srv", client, nil)
	tool := NewListMcpResourcesTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{"server": "srv"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "No resources found") {
		t.Errorf("expected TS no-resources message, got %s", res.Content)
	}
}

func TestListMcpResourcesTool_UnknownServer(t *testing.T) {
	m := NewMCPManager()
	m.AddConfig("known", MCPServerConfig{Command: "npx"})
	tool := NewListMcpResourcesTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{"server": "unknown"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for unconfigured server")
	}
}

// ─── ReadMcpResourceTool ──────────────────────────────────────────────────────

func TestReadMcpResourceTool_Metadata(t *testing.T) {
	tool := NewReadMcpResourceTool(NewMCPManager())
	if tool.Name() != "ReadMcpResourceTool" {
		t.Errorf("expected ReadMcpResourceTool, got %s", tool.Name())
	}
	if !tool.IsConcurrentSafe() {
		t.Error("expected IsConcurrentSafe=true")
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
	schema := tool.Schema()
	if _, ok := schema.Properties["server"]; !ok {
		t.Error("schema missing server")
	}
	if _, ok := schema.Properties["uri"]; !ok {
		t.Error("schema missing uri")
	}
}

func TestReadMcpResourceTool_MissingServerName(t *testing.T) {
	tool := NewReadMcpResourceTool(NewMCPManager())
	res, err := tool.Execute(context.Background(), map[string]any{
		"server": "",
		"uri":    "file://test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, `Server "" not found`) {
		t.Errorf("TS accepts blank strings and performs lookup, got: %s", res.Content)
	}
}

func TestReadMcpResourceTool_BlankResourceURIIsPassedThrough(t *testing.T) {
	var gotURI string
	client := startMCPServer(t, "srv", handler.Map{
		"resources/read": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			var request struct {
				URI string `json:"uri"`
			}
			_ = json.Unmarshal(p, &request)
			gotURI = request.URI
			return map[string]any{"contents": []map[string]any{{"uri": request.URI, "text": "empty-uri"}}}, nil
		}),
	})
	tool := NewReadMcpResourceTool(injectServer("srv", client, nil))
	res, err := tool.Execute(context.Background(), map[string]any{
		"server": "srv",
		"uri":    "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError || gotURI != "" {
		t.Errorf("blank URI should reach resources/read unchanged: result=%#v uri=%q", res, gotURI)
	}
}

func TestReadMcpResourceTool_ServerNotConfigured(t *testing.T) {
	m := NewMCPManager()
	m.AddConfig("existing", MCPServerConfig{Command: "npx"})
	tool := NewReadMcpResourceTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{
		"server": "missing",
		"uri":    "file://test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for unconfigured server")
	}
	if !strings.Contains(res.Content, "missing") {
		t.Errorf("expected server name in error: %s", res.Content)
	}
}

func TestReadMcpResourceTool_SuccessfulRead(t *testing.T) {
	client := startMCPServer(t, "fs", handler.Map{
		"resources/read": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"contents": []map[string]any{
					{"uri": "file:///tmp/hello.txt", "text": "hello world"},
				},
			}, nil
		}),
	})
	m := injectServer("fs", client, nil)
	tool := NewReadMcpResourceTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{
		"server": "fs",
		"uri":    "file:///tmp/hello.txt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error: %s", res.Content)
	}
	envelope := mustMCPJSONObject(t, res.Content)
	contents := mustMCPJSONArrayField(t, envelope, "contents")
	if len(contents) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(contents))
	}
	if contents[0]["uri"] != "file:///tmp/hello.txt" {
		t.Errorf("expected uri preserved, got %v", contents[0]["uri"])
	}
	if contents[0]["text"] != "hello world" {
		t.Errorf("expected text 'hello world', got %v", contents[0]["text"])
	}
}

func TestReadMcpResourceTool_PassesURI(t *testing.T) {
	var gotURI string
	client := startMCPServer(t, "srv", handler.Map{
		"resources/read": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			var req struct {
				URI string `json:"uri"`
			}
			json.Unmarshal(p, &req) //nolint:errcheck
			gotURI = req.URI
			return map[string]any{
				"contents": []map[string]any{{"uri": req.URI, "text": "data"}},
			}, nil
		}),
	})
	m := injectServer("srv", client, nil)
	tool := NewReadMcpResourceTool(m)

	tool.Execute(context.Background(), map[string]any{ //nolint:errcheck
		"server": "srv",
		"uri":    "custom://my-resource",
	})
	if gotURI != "custom://my-resource" {
		t.Errorf("expected URI 'custom://my-resource', got %q", gotURI)
	}
}

func TestReadMcpResourceTool_MultiContent(t *testing.T) {
	client := startMCPServer(t, "srv", handler.Map{
		"resources/read": handler.New(func(ctx context.Context, p json.RawMessage) (any, error) {
			return map[string]any{
				"contents": []map[string]any{
					{"uri": "u", "text": "part1"},
					{"uri": "u", "text": "part2"},
				},
			}, nil
		}),
	})
	m := injectServer("srv", client, nil)
	tool := NewReadMcpResourceTool(m)

	res, err := tool.Execute(context.Background(), map[string]any{
		"server": "srv",
		"uri":    "u",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	envelope := mustMCPJSONObject(t, res.Content)
	contents := mustMCPJSONArrayField(t, envelope, "contents")
	if len(contents) != 2 {
		t.Fatalf("expected 2 content items, got %d", len(contents))
	}
	if contents[0]["text"] != "part1" || contents[1]["text"] != "part2" {
		t.Errorf("expected part1/part2 content items, got %#v", contents)
	}
}

// ─── Input parsing ────────────────────────────────────────────────────────────

func TestMCPToolInput_Parse(t *testing.T) {
	in, err := parseInput[MCPToolInput](map[string]any{
		"server_name": "srv",
		"tool_name":   "do-thing",
		"arguments":   map[string]any{"x": 1},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.ServerName != "srv" {
		t.Errorf("unexpected ServerName: %s", in.ServerName)
	}
	if in.ToolName != "do-thing" {
		t.Errorf("unexpected ToolName: %s", in.ToolName)
	}
	if in.Arguments["x"] == nil {
		t.Error("expected arguments.x to be set")
	}
}

func TestListMcpResourcesInput_Parse(t *testing.T) {
	in, err := parseInput[ListMcpResourcesInput](map[string]any{"server": "my-server"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Server != "my-server" {
		t.Errorf("unexpected Server: %s", in.Server)
	}
}

func TestListMcpResourcesInput_ParseEmpty(t *testing.T) {
	in, err := parseInput[ListMcpResourcesInput](map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Server != "" {
		t.Errorf("expected empty Server, got %s", in.Server)
	}
}

func TestReadMcpResourceInput_Parse(t *testing.T) {
	in, err := parseInput[ReadMcpResourceInput](map[string]any{
		"server": "srv",
		"uri":    "file://data.json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Server != "srv" {
		t.Errorf("unexpected Server: %s", in.Server)
	}
	if in.URI != "file://data.json" {
		t.Errorf("unexpected URI: %s", in.URI)
	}
}
