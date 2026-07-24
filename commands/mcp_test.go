package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
)

type mcpTestQueryLoop struct{}

func (*mcpTestQueryLoop) SetMessages([]types.Message)   {}
func (*mcpTestQueryLoop) Messages() []types.Message     { return nil }
func (*mcpTestQueryLoop) Model() string                 { return "test-model" }
func (*mcpTestQueryLoop) SetModel(string)               {}
func (*mcpTestQueryLoop) ContextUsage() (int, int)      { return 0, 0 }
func (*mcpTestQueryLoop) SetProvider(provider.Provider) {}

type fakeMCPBackend struct {
	states      map[string]svcmcp.MCPServerConnection
	sources     map[string]string
	diagnostics []string
	toggles     []string
	reconnects  []string
}

func newFakeMCPBackend(states ...svcmcp.MCPServerConnection) *fakeMCPBackend {
	f := &fakeMCPBackend{
		states:  make(map[string]svcmcp.MCPServerConnection),
		sources: make(map[string]string),
	}
	for _, state := range states {
		f.states[state.Name] = state
	}
	return f
}

func (f *fakeMCPBackend) ServerNames() []string {
	names := make([]string, 0, len(f.states))
	for name := range f.states {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

func (f *fakeMCPBackend) Snapshot() []svcmcp.MCPServerConnection {
	names := f.ServerNames()
	out := make([]svcmcp.MCPServerConnection, 0, len(names))
	for _, name := range names {
		out = append(out, f.states[name])
	}
	return out
}

func (f *fakeMCPBackend) HealthSnapshot() svcmcp.HealthSnapshot {
	snapshot := svcmcp.HealthSnapshot{GeneratedAt: time.Unix(1, 0)}
	for _, state := range f.states {
		switch state.Type {
		case svcmcp.MCPStatePending:
			snapshot.Counts.Pending++
		case svcmcp.MCPStateConnected:
			snapshot.Counts.Connected++
		case svcmcp.MCPStateFailed:
			snapshot.Counts.Failed++
		case svcmcp.MCPStateNeedsAuth:
			snapshot.Counts.NeedsAuth++
		case svcmcp.MCPStateDisabled:
			snapshot.Counts.Disabled++
		}
	}
	return snapshot
}

func (f *fakeMCPBackend) State(name string) (svcmcp.MCPServerConnection, bool) {
	state, ok := f.states[name]
	return state, ok
}

func (f *fakeMCPBackend) AddConfig(name string, cfg svcmcp.MCPServerConfig) {
	f.states[name] = svcmcp.MCPServerConnection{Name: name, Type: svcmcp.MCPStatePending, Config: cfg}
}

func (f *fakeMCPBackend) SetConfigs(configs map[string]svcmcp.MCPServerConfig) {
	next := make(map[string]svcmcp.MCPServerConnection, len(configs))
	for name, cfg := range configs {
		state := f.states[name]
		if state.Name == "" {
			state = svcmcp.MCPServerConnection{Name: name, Type: svcmcp.MCPStatePending}
		}
		state.Config = cfg
		next[name] = state
	}
	f.states = next
}

func (f *fakeMCPBackend) ToggleEnabled(_ context.Context, name string, enabled bool) (svcmcp.MCPServerConnection, error) {
	state, ok := f.states[name]
	if !ok {
		return svcmcp.MCPServerConnection{}, fmt.Errorf("not found")
	}
	if enabled {
		state.Type = svcmcp.MCPStatePending
	} else {
		state.Type = svcmcp.MCPStateDisabled
	}
	f.states[name] = state
	f.toggles = append(f.toggles, fmt.Sprintf("%s=%t", name, enabled))
	return state, nil
}

func (f *fakeMCPBackend) Reconnect(_ context.Context, name string) (svcmcp.MCPServerConnection, error) {
	state, ok := f.states[name]
	if !ok {
		return svcmcp.MCPServerConnection{}, fmt.Errorf("not found")
	}
	state.Type = svcmcp.MCPStateConnected
	f.states[name] = state
	f.reconnects = append(f.reconnects, name)
	return state, nil
}

func (f *fakeMCPBackend) ConfigSource(name string) string { return f.sources[name] }

func (f *fakeMCPBackend) Diagnostics() []string { return append([]string(nil), f.diagnostics...) }

type fakeMCPAuth struct {
	result mcpAuthResult
}

func (f fakeMCPAuth) AuthURL(context.Context, string, svcmcp.MCPServerConfig) (mcpAuthResult, error) {
	return f.result, nil
}

func TestMCPListShowsStatesCountsCapabilitiesAndDiagnostics(t *testing.T) {
	backend := newFakeMCPBackend(
		svcmcp.MCPServerConnection{
			Name:         "github",
			Type:         svcmcp.MCPStateConnected,
			Config:       svcmcp.MCPServerConfig{Type: svcmcp.TransportHTTP, Scope: svcmcp.ScopeUser},
			Capabilities: svcmcp.ServerCapabilities{"tools": map[string]any{}, "resources": map[string]any{}},
			Tools:        []svcmcp.ToolDefinition{{Name: "search"}},
			Resources:    []svcmcp.Resource{{URI: "repo://one"}},
			Prompts:      []svcmcp.PromptDefinition{{Name: "triage"}},
		},
		svcmcp.MCPServerConnection{
			Name:                 "broken",
			Type:                 svcmcp.MCPStateFailed,
			Config:               svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Scope: svcmcp.ScopeLocal},
			Error:                "spawn failed",
			ReconnectAttempt:     2,
			MaxReconnectAttempts: 5,
		},
		svcmcp.MCPServerConnection{
			Name:      "remote",
			Type:      svcmcp.MCPStateNeedsAuth,
			Config:    svcmcp.MCPServerConfig{Type: svcmcp.TransportSSE, Scope: svcmcp.ScopeProject},
			NeedsAuth: &svcmcp.NeedsAuthState{Scope: "repo read", ResourceMetadataURL: "https://auth.example/.well-known/oauth-protected-resource"},
		},
		svcmcp.MCPServerConnection{
			Name:   "off",
			Type:   svcmcp.MCPStateDisabled,
			Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Scope: svcmcp.ScopeLocal},
		},
	)
	backend.sources["github"] = "/tmp/settings.json"
	backend.diagnostics = []string{"broken: command not found"}
	out := runMCPCommand(t, &mcpCmd{backend: backend}, "")
	for _, want := range []string{
		"MCP servers: 4 total",
		"github  state=connected transport=http scope=user tools=1 resources=1 prompts=1 auth=authenticated",
		"broken  state=failed transport=stdio scope=local",
		"error: spawn failed",
		"remote  state=needs-auth transport=sse scope=project",
		"warning: needs OAuth scope: repo read",
		"off  state=disabled",
		"Diagnostics:",
		"broken: command not found",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestMCPCommandCanRegisterAsSlashCommand(t *testing.T) {
	backend := newFakeMCPBackend(svcmcp.MCPServerConnection{
		Name:   "demo",
		Type:   svcmcp.MCPStatePending,
		Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio},
	})
	registry := NewRegistry()
	RegisterMCPCommand(registry, backend)
	cmd, args := registry.Parse("/mcp list")
	if cmd == nil || cmd.Name() != "mcp" || args != "list" {
		t.Fatalf("Parse(/mcp list) = (%v, %q)", cmd, args)
	}
	out := runMCPCommand(t, cmd, args)
	if !strings.Contains(out, "demo  state=pending") {
		t.Fatalf("registered command output:\n%s", out)
	}
}

func TestMCPCommandPrefersLiveContextBackend(t *testing.T) {
	backend := newFakeMCPBackend(svcmcp.MCPServerConnection{
		Name:   "live-runtime",
		Type:   svcmcp.MCPStateConnected,
		Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportHTTP},
	})
	var out strings.Builder
	ctx := &Context{
		CWD:        t.TempDir(),
		MCPBackend: backend,
		QueryLoop:  &mcpTestQueryLoop{},
		OnEvent:    func(text string) { out.WriteString(text) },
	}

	if err := NewMCPCommand(nil).Execute(ctx, "list"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if text := out.String(); !strings.Contains(text, "live-runtime  state=connected") {
		t.Fatalf("/mcp did not inspect the live context backend:\n%s", text)
	}
}

func TestDoctorPrefersLiveContextMCPBackend(t *testing.T) {
	backend := newFakeMCPBackend(svcmcp.MCPServerConnection{
		Name:   "broken-live-runtime",
		Type:   svcmcp.MCPStateFailed,
		Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio},
		Error:  "live transport failed",
	})
	var out strings.Builder
	ctx := &Context{
		CWD:        t.TempDir(),
		MCPBackend: backend,
		QueryLoop:  &mcpTestQueryLoop{},
		OnEvent:    func(text string) { out.WriteString(text) },
	}

	if err := (&doctorCmd{}).Execute(ctx, ""); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "✗ MCP: " + i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyMCPDoctorFailed, 1, 1, 0, 0, 0)
	if text := out.String(); !strings.Contains(text, want) {
		t.Fatalf("/doctor did not inspect the live context backend:\n%s", text)
	}
}

func TestMCPListJSONIsMachineReadable(t *testing.T) {
	backend := newFakeMCPBackend(svcmcp.MCPServerConnection{
		Name:   "github",
		Type:   svcmcp.MCPStateConnected,
		Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportHTTP},
		Tools:  []svcmcp.ToolDefinition{{Name: "search"}},
	})
	out := runMCPCommand(t, &mcpCmd{backend: backend}, "--json")
	var report mcpStatusReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("unmarshal report: %v\n%s", err, out)
	}
	if len(report.Servers) != 1 || report.Servers[0].Name != "github" || report.Servers[0].ToolsCount != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestMCPHumanOutputUsesRuntimeLanguageButJSONStaysStable(t *testing.T) {
	backend := newFakeMCPBackend(svcmcp.MCPServerConnection{
		Name:   "server-7",
		Type:   svcmcp.MCPStateConnected,
		Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportHTTP, Scope: svcmcp.ScopeProject},
		Tools:  []svcmcp.ToolDefinition{{Name: "search"}},
	})

	human := runMCPCommandWithLanguage(t, &mcpCmd{backend: backend}, t.TempDir(), i18n.LangZH, "list")
	for _, want := range []string{"MCP 服务器：共 1 个", "server-7", "状态=已连接", "transport=http", "范围=项目", "工具=1", "认证=已认证"} {
		if !strings.Contains(human, want) {
			t.Fatalf("localized output missing %q:\n%s", want, human)
		}
	}
	if strings.Contains(human, "state=connected") {
		t.Fatalf("human output retained English state labels:\n%s", human)
	}

	machine := runMCPCommandWithLanguage(t, &mcpCmd{backend: backend}, t.TempDir(), i18n.LangZH, "--json")
	var report mcpStatusReport
	if err := json.Unmarshal([]byte(machine), &report); err != nil {
		t.Fatalf("unmarshal localized JSON report: %v\n%s", err, machine)
	}
	if len(report.Servers) != 1 || report.Servers[0].State != svcmcp.MCPStateConnected || report.Servers[0].Scope != svcmcp.ScopeProject || report.Servers[0].AuthStatus != "authenticated" {
		t.Fatalf("machine-readable values were localized: %#v", report.Servers)
	}
}

func TestMCPEnableDisableAllPersistsState(t *testing.T) {
	tmp := t.TempDir()
	backend := newFakeMCPBackend(
		svcmcp.MCPServerConnection{Name: "one", Type: svcmcp.MCPStatePending, Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio}},
		svcmcp.MCPServerConnection{Name: "two", Type: svcmcp.MCPStateConnected, Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportHTTP}},
	)
	_ = runMCPCommandWithCWD(t, &mcpCmd{backend: backend}, tmp, "disable all")
	if got := strings.Join(backend.toggles, ","); got != "one=false,two=false" {
		t.Fatalf("toggles = %q", got)
	}
	settings := readSettingsMap(t, filepath.Join(tmp, ".luban-code", "settings.json"))
	if got := strings.Join(stringSliceFromSettings(settings["disabledMcpServers"]), ","); got != "one,two" {
		t.Fatalf("disabledMcpServers = %q", got)
	}
	_ = runMCPCommandWithCWD(t, &mcpCmd{backend: backend}, tmp, "enable two")
	settings = readSettingsMap(t, filepath.Join(tmp, ".luban-code", "settings.json"))
	if got := strings.Join(stringSliceFromSettings(settings["enabledMcpServers"]), ","); got != "two" {
		t.Fatalf("enabledMcpServers = %q", got)
	}
	if got := strings.Join(stringSliceFromSettings(settings["disabledMcpServers"]), ","); got != "one" {
		t.Fatalf("disabledMcpServers after enable = %q", got)
	}
}

func TestMCPUnknownServerBranches(t *testing.T) {
	backend := newFakeMCPBackend()
	for _, args := range []string{"enable missing", "disable missing", "reconnect missing", "authenticate missing", "get missing"} {
		out := runMCPCommand(t, &mcpCmd{backend: backend}, args)
		if !strings.Contains(out, "not found") && !strings.Contains(out, "Failed to reconnect") {
			t.Fatalf("%q output did not report missing server:\n%s", args, out)
		}
	}
}

func TestMCPReconnectAndAuthURL(t *testing.T) {
	backend := newFakeMCPBackend(svcmcp.MCPServerConnection{
		Name:   "remote",
		Type:   svcmcp.MCPStateNeedsAuth,
		Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportHTTP, URL: "https://mcp.example"},
	})
	cmd := &mcpCmd{
		backend: backend,
		auth: fakeMCPAuth{result: mcpAuthResult{
			Status:  "auth_url",
			AuthURL: "https://auth.example/authorize",
			Message: "Open URL",
		}},
	}
	out := runMCPCommand(t, cmd, "authenticate remote")
	if !strings.Contains(out, "https://auth.example/authorize") {
		t.Fatalf("auth output missing url:\n%s", out)
	}
	out = runMCPCommand(t, cmd, "reconnect remote")
	if !strings.Contains(out, "Successfully reconnected to remote") {
		t.Fatalf("reconnect output:\n%s", out)
	}
	if got := strings.Join(backend.reconnects, ","); got != "remote" {
		t.Fatalf("reconnects = %q", got)
	}
}

func TestMCPAddJSONAndRemovePersistSettings(t *testing.T) {
	tmp := t.TempDir()
	backend := newFakeMCPBackend()
	addOut := runMCPCommandWithCWD(t, &mcpCmd{backend: backend}, tmp, `add-json demo {"type":"stdio","command":"node","args":["server.js"]}`)
	if !strings.Contains(addOut, `Added MCP server "demo"`) {
		t.Fatalf("add output:\n%s", addOut)
	}
	if _, ok := backend.State("demo"); !ok {
		t.Fatal("expected backend to contain added server")
	}
	removeOut := runMCPCommandWithCWD(t, &mcpCmd{backend: backend}, tmp, "remove demo")
	if !strings.Contains(removeOut, `Removed MCP server "demo"`) {
		t.Fatalf("remove output:\n%s", removeOut)
	}
	settings := readSettingsMap(t, filepath.Join(tmp, ".luban-code", "settings.json"))
	servers := mcpServersFromSettings(settings["mcpServers"])
	if _, ok := servers["demo"]; ok {
		t.Fatalf("demo still persisted: %#v", servers)
	}
}

func TestMCPDoctorUsesManagerDiagnostics(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	out := runMCPCommandWithCWD(t, NewMCPCommand(nil), tmp, `add-json missing {"type":"stdio","command":"definitely-not-a-real-mcp-binary"}`)
	if !strings.Contains(out, "Added MCP server") {
		t.Fatalf("add output:\n%s", out)
	}
	result := checkMCPServers(tmp)
	if result.ok {
		t.Fatalf("expected diagnostic failure for missing command: %#v", result)
	}
	if !strings.Contains(result.message, "definitely-not-a-real-mcp-binary") {
		t.Fatalf("doctor message = %q", result.message)
	}
}

func runMCPCommand(t *testing.T, cmd Command, args string) string {
	t.Helper()
	return runMCPCommandWithCWD(t, cmd, t.TempDir(), args)
}

func runMCPCommandWithCWD(t *testing.T, cmd Command, cwd, args string) string {
	t.Helper()
	return runMCPCommandWithLanguage(t, cmd, cwd, i18n.LangEN, args)
}

func runMCPCommandWithLanguage(t *testing.T, cmd Command, cwd string, language i18n.Language, args string) string {
	t.Helper()
	var sb strings.Builder
	ctx := &Context{CWD: cwd, Language: language, OnEvent: func(s string) { sb.WriteString(s) }}
	if err := cmd.Execute(ctx, args); err != nil {
		t.Fatalf("Execute(%q): %v", args, err)
	}
	return sb.String()
}

func readSettingsMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return out
}

func sortStrings(values []string) {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
}
