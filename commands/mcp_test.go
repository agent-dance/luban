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

	"github.com/agent-dance/luban/i18n"
	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type mcpTestQueryLoop struct{}

func (*mcpTestQueryLoop) SetMessagesPreservingToolUseLedger([]types.Message) {}
func (*mcpTestQueryLoop) Messages() []types.Message                          { return nil }
func (*mcpTestQueryLoop) Model() string                                      { return "test-model" }
func (*mcpTestQueryLoop) SetModel(string)                                    {}
func (*mcpTestQueryLoop) ContextUsage() (int, int)                           { return 0, 0 }
func (*mcpTestQueryLoop) SetProvider(provider.Provider)                      {}

type fakeMCPBackend struct {
	states      map[string]mcpmanager.MCPServerConnection
	sources     map[string]string
	diagnostics []string
	toggles     []string
	reconnects  []string
}

func newFakeMCPBackend(states ...mcpmanager.MCPServerConnection) *fakeMCPBackend {
	f := &fakeMCPBackend{
		states:  make(map[string]mcpmanager.MCPServerConnection),
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

func (f *fakeMCPBackend) Snapshot() []mcpmanager.MCPServerConnection {
	names := f.ServerNames()
	out := make([]mcpmanager.MCPServerConnection, 0, len(names))
	for _, name := range names {
		out = append(out, f.states[name])
	}
	return out
}

func (f *fakeMCPBackend) State(name string) (mcpmanager.MCPServerConnection, bool) {
	state, ok := f.states[name]
	return state, ok
}

func (f *fakeMCPBackend) AddConfig(name string, cfg catalog.MCPServerConfig) {
	f.states[name] = mcpmanager.MCPServerConnection{Name: name, Type: mcpmanager.MCPStatePending, Config: cfg}
}

func (f *fakeMCPBackend) SetConfigs(configs map[string]catalog.MCPServerConfig) {
	next := make(map[string]mcpmanager.MCPServerConnection, len(configs))
	for name, cfg := range configs {
		state := f.states[name]
		if state.Name == "" {
			state = mcpmanager.MCPServerConnection{Name: name, Type: mcpmanager.MCPStatePending}
		}
		state.Config = cfg
		next[name] = state
	}
	f.states = next
}

func (f *fakeMCPBackend) ToggleEnabled(_ context.Context, name string, enabled bool) (mcpmanager.MCPServerConnection, error) {
	state, ok := f.states[name]
	if !ok {
		return mcpmanager.MCPServerConnection{}, fmt.Errorf("not found")
	}
	if enabled {
		state.Type = mcpmanager.MCPStatePending
	} else {
		state.Type = mcpmanager.MCPStateDisabled
	}
	f.states[name] = state
	f.toggles = append(f.toggles, fmt.Sprintf("%s=%t", name, enabled))
	return state, nil
}

func (f *fakeMCPBackend) Reconnect(_ context.Context, name string) (mcpmanager.MCPServerConnection, error) {
	state, ok := f.states[name]
	if !ok {
		return mcpmanager.MCPServerConnection{}, fmt.Errorf("not found")
	}
	state.Type = mcpmanager.MCPStateConnected
	f.states[name] = state
	f.reconnects = append(f.reconnects, name)
	return state, nil
}

func (f *fakeMCPBackend) ConfigSource(name string) string { return f.sources[name] }

func (f *fakeMCPBackend) Diagnostics() []string { return append([]string(nil), f.diagnostics...) }

type fakeMCPAuth struct {
	result mcpAuthResult
}

func (f fakeMCPAuth) AuthURL(context.Context, string, catalog.MCPServerConfig) (mcpAuthResult, error) {
	return f.result, nil
}

func TestMCPListShowsStatesCountsCapabilitiesAndDiagnostics(t *testing.T) {
	backend := newFakeMCPBackend(
		mcpmanager.MCPServerConnection{
			Name:         "github",
			Type:         mcpmanager.MCPStateConnected,
			Config:       catalog.MCPServerConfig{Type: catalog.TransportHTTP, Scope: catalog.ScopeUser},
			Capabilities: catalog.ServerCapabilities{"tools": map[string]any{}, "resources": map[string]any{}},
			Tools:        []catalog.ToolDefinition{{Name: "search"}},
			Resources:    []catalog.Resource{{URI: "repo://one"}},
			Prompts:      []catalog.PromptDefinition{{Name: "triage"}},
		},
		mcpmanager.MCPServerConnection{
			Name:                 "broken",
			Type:                 mcpmanager.MCPStateFailed,
			Config:               catalog.MCPServerConfig{Type: catalog.TransportStdio, Scope: catalog.ScopeLocal},
			Error:                "spawn failed",
			ReconnectAttempt:     2,
			MaxReconnectAttempts: 5,
		},
		mcpmanager.MCPServerConnection{
			Name:      "remote",
			Type:      mcpmanager.MCPStateNeedsAuth,
			Config:    catalog.MCPServerConfig{Type: catalog.TransportSSE, Scope: catalog.ScopeProject},
			NeedsAuth: &mcpauth.NeedsAuthState{Scope: "repo read", ResourceMetadataURL: "https://auth.example/.well-known/oauth-protected-resource"},
		},
		mcpmanager.MCPServerConnection{
			Name:   "off",
			Type:   mcpmanager.MCPStateDisabled,
			Config: catalog.MCPServerConfig{Type: catalog.TransportStdio, Scope: catalog.ScopeLocal},
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

func TestBuildMCPStatusReportProjectsCountsFromSnapshot(t *testing.T) {
	backend := newFakeMCPBackend(
		mcpmanager.MCPServerConnection{Name: "pending", Type: mcpmanager.MCPStatePending},
		mcpmanager.MCPServerConnection{Name: "connected", Type: mcpmanager.MCPStateConnected},
		mcpmanager.MCPServerConnection{Name: "failed", Type: mcpmanager.MCPStateFailed},
		mcpmanager.MCPServerConnection{Name: "auth", Type: mcpmanager.MCPStateNeedsAuth},
		mcpmanager.MCPServerConnection{Name: "disabled", Type: mcpmanager.MCPStateDisabled},
	)

	report := buildMCPStatusReport(backend, i18n.LangEN)
	if report.Counts != (mcpStatusCounts{Pending: 1, Connected: 1, Failed: 1, NeedsAuth: 1, Disabled: 1}) {
		t.Fatalf("status counts = %#v", report.Counts)
	}
}

func TestMCPCommandCanRegisterAsSlashCommand(t *testing.T) {
	backend := newFakeMCPBackend(mcpmanager.MCPServerConnection{
		Name:   "demo",
		Type:   mcpmanager.MCPStatePending,
		Config: catalog.MCPServerConfig{Type: catalog.TransportStdio},
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
	backend := newFakeMCPBackend(mcpmanager.MCPServerConnection{
		Name:   "live-runtime",
		Type:   mcpmanager.MCPStateConnected,
		Config: catalog.MCPServerConfig{Type: catalog.TransportHTTP},
	})
	var out strings.Builder
	ctx := &Context{
		CWD:                   t.TempDir(),
		MCPBackend:            backend,
		QueryLoop:             &mcpTestQueryLoop{},
		OnCommandPresentation: captureCompletedCommand(&out),
	}

	if err := NewMCPCommand(nil).Execute(ctx, "list"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if text := out.String(); !strings.Contains(text, "live-runtime  state=connected") {
		t.Fatalf("/mcp did not inspect the live context backend:\n%s", text)
	}
}

func TestDoctorPrefersLiveContextMCPBackend(t *testing.T) {
	backend := newFakeMCPBackend(mcpmanager.MCPServerConnection{
		Name:   "broken-live-runtime",
		Type:   mcpmanager.MCPStateFailed,
		Config: catalog.MCPServerConfig{Type: catalog.TransportStdio},
		Error:  "live transport failed",
	})
	var out strings.Builder
	ctx := &Context{
		CWD:              t.TempDir(),
		MCPBackend:       backend,
		QueryLoop:        &mcpTestQueryLoop{},
		ProviderRegistry: provider.DefaultRegistry(),
		OnEvent:          func(text string) { out.WriteString(text) },
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
	backend := newFakeMCPBackend(mcpmanager.MCPServerConnection{
		Name:   "github",
		Type:   mcpmanager.MCPStateConnected,
		Config: catalog.MCPServerConfig{Type: catalog.TransportHTTP},
		Tools:  []catalog.ToolDefinition{{Name: "search"}},
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
	backend := newFakeMCPBackend(mcpmanager.MCPServerConnection{
		Name:   "server-7",
		Type:   mcpmanager.MCPStateConnected,
		Config: catalog.MCPServerConfig{Type: catalog.TransportHTTP, Scope: catalog.ScopeProject},
		Tools:  []catalog.ToolDefinition{{Name: "search"}},
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
	if len(report.Servers) != 1 || report.Servers[0].State != mcpmanager.MCPStateConnected || report.Servers[0].Scope != catalog.ScopeProject || report.Servers[0].AuthStatus != "authenticated" {
		t.Fatalf("machine-readable values were localized: %#v", report.Servers)
	}
}

func TestMCPEnableDisableAllPersistsState(t *testing.T) {
	tmp := t.TempDir()
	backend := newFakeMCPBackend(
		mcpmanager.MCPServerConnection{Name: "one", Type: mcpmanager.MCPStatePending, Config: catalog.MCPServerConfig{Type: catalog.TransportStdio}},
		mcpmanager.MCPServerConnection{Name: "two", Type: mcpmanager.MCPStateConnected, Config: catalog.MCPServerConfig{Type: catalog.TransportHTTP}},
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
	backend := newFakeMCPBackend(mcpmanager.MCPServerConnection{
		Name:   "remote",
		Type:   mcpmanager.MCPStateNeedsAuth,
		Config: catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "https://mcp.example"},
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

func TestMCPAddJSONReportsWarningAndStillRegisters(t *testing.T) {
	const variable = "LUBAN_TEST_MCP_COMMAND_WARNING_MISSING"
	unsetMCPCommandEnvForTest(t, variable)
	tmp := t.TempDir()
	backend := newFakeMCPBackend()
	out := runMCPCommandWithCWD(t, &mcpCmd{backend: backend}, tmp,
		`add-json warning-only {"type":"stdio","command":"${LUBAN_TEST_MCP_COMMAND_WARNING_MISSING}"}`)
	if !strings.Contains(out, i18n.Format(i18n.LangEN, i18n.KeyMCPValidationMissingEnv, variable)) {
		t.Fatalf("warning output omitted validation diagnostic:\n%s", out)
	}
	if !strings.Contains(out, `Added MCP server "warning-only"`) {
		t.Fatalf("warning prevented successful add:\n%s", out)
	}
	if _, ok := backend.State("warning-only"); !ok {
		t.Fatal("warning-only server was not registered")
	}
}

func TestRuntimeMCPBackendSkipsOnlyFatallyInvalidServers(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	root := t.TempDir()
	data := []byte(`{"mcpServers":{"valid":{"type":"stdio","command":"node"},"invalid":{"type":"stdio","command":""}}}`)
	if err := os.WriteFile(filepath.Join(root, ".mcp.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	backend := newRuntimeMCPBackend(root, i18n.LangEN)
	if _, ok := backend.State("valid"); !ok {
		t.Fatal("valid server was not registered")
	}
	if _, ok := backend.State("invalid"); ok {
		t.Fatal("fatally invalid server was registered")
	}
	if diagnostics := strings.Join(backend.Diagnostics(), "\n"); !strings.Contains(diagnostics, "mcpServers.invalid") {
		t.Fatalf("fatal diagnostic was silently dropped: %q", diagnostics)
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
	ctx := &Context{
		CWD: cwd, Language: language,
		OnEvent:               func(value string) { sb.WriteString(value) },
		OnCommandPresentation: captureCompletedCommand(&sb),
	}
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

func unsetMCPCommandEnvForTest(t *testing.T, key string) {
	t.Helper()
	value, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	})
}
