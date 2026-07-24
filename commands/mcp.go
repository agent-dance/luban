package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	svcmcp "github.com/agent-dance/luban/services/mcp"
)

// MCPBackend is the narrow services/mcp surface used by the /mcp command.
// *services/mcp.Manager satisfies it; tests use an in-memory fake.
type MCPBackend interface {
	ServerNames() []string
	Snapshot() []svcmcp.MCPServerConnection
	HealthSnapshot() svcmcp.HealthSnapshot
	State(name string) (svcmcp.MCPServerConnection, bool)
	AddConfig(name string, cfg svcmcp.MCPServerConfig)
	SetConfigs(configs map[string]svcmcp.MCPServerConfig)
	ToggleEnabled(ctx context.Context, name string, enabled bool) (svcmcp.MCPServerConnection, error)
	Reconnect(ctx context.Context, name string) (svcmcp.MCPServerConnection, error)
}

type mcpSourceProvider interface {
	ConfigSource(name string) string
	Diagnostics() []string
}

type mcpAuthenticator interface {
	AuthURL(ctx context.Context, serverName string, cfg svcmcp.MCPServerConfig) (mcpAuthResult, error)
}

type mcpAuthResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	AuthURL string `json:"authUrl,omitempty"`
}

type mcpCmd struct {
	backend MCPBackend
	auth    mcpAuthenticator
}

// NewMCPCommand returns the /mcp management command. Passing nil uses the
// runtime settings-backed manager.
func NewMCPCommand(backend MCPBackend) Command {
	return wrapCommandPresentation(&mcpCmd{backend: backend})
}

// RegisterMCPCommand lets callers wire the command without importing services/mcp
// at the slash-command registry callsite.
func RegisterMCPCommand(r *Registry, backend MCPBackend) {
	if r == nil {
		return
	}
	r.Register(NewMCPCommand(backend))
}

func (c *mcpCmd) Name() string { return "mcp" }

func (c *mcpCmd) Aliases() []string { return nil }

func (c *mcpCmd) Description() string {
	return builtinCommandDescription("mcp")
}

func (c *mcpCmd) Execute(ctx *Context, args string) error {
	if ctx == nil {
		ctx = &Context{}
	}
	backend, err := c.resolveBackend(ctx)
	if err != nil {
		return err
	}
	authenticator := c.auth
	if authenticator == nil {
		authenticator = defaultMCPAuthenticator{language: ctx.Language}
	}

	verb, rest := splitMCPVerb(args)
	jsonOutput := false
	if verb == "--json" {
		jsonOutput = true
		verb, rest = splitMCPVerb(rest)
	}
	if strings.HasSuffix(rest, " --json") || rest == "--json" {
		jsonOutput = true
		rest = strings.TrimSpace(strings.TrimSuffix(rest, " --json"))
	}
	if verb == "" || verb == "list" || verb == "status" {
		report := buildMCPStatusReport(backend, ctx.Language)
		emitMCPReport(ctx, report, jsonOutput)
		reportCommandSucceeded(ctx)
		return nil
	}

	switch verb {
	case "get", "show":
		name := strings.TrimSpace(rest)
		if name == "" {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyMCPUsage))
			reportCommandFailed(ctx)
			return nil
		}
		report := buildMCPStatusReport(backend, ctx.Language)
		for _, server := range report.Servers {
			if server.Name == name {
				emitMCPServer(ctx, server, jsonOutput)
				reportCommandSucceeded(ctx)
				return nil
			}
		}
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPServerNotFound, name))
		reportCommandFailed(ctx)
		return nil
	case "enable":
		return c.toggle(ctx, backend, rest, true)
	case "disable":
		return c.toggle(ctx, backend, rest, false)
	case "reconnect":
		return c.reconnect(ctx, backend, rest)
	case "authenticate", "auth":
		return c.authenticate(ctx, backend, authenticator, rest, jsonOutput)
	case "diagnostics", "doctor":
		report := buildMCPStatusReport(backend, ctx.Language)
		emitMCPDiagnostics(ctx, report, jsonOutput)
		reportCommandSucceeded(ctx)
		return nil
	case "add-json":
		return c.addJSON(ctx, backend, rest)
	case "remove":
		return c.remove(ctx, backend, rest)
	case "help", "-h", "--help":
		ctx.OnEvent(mcpUsage(ctx.Language))
		reportCommandSucceeded(ctx)
		return nil
	default:
		ctx.OnEvent(mcpUsage(ctx.Language))
		reportCommandFailed(ctx)
		return nil
	}
}

func (c *mcpCmd) resolveBackend(ctx *Context) (MCPBackend, error) {
	if c.backend != nil {
		return c.backend, nil
	}
	if ctx != nil && ctx.MCPBackend != nil {
		return ctx.MCPBackend, nil
	}
	cwd := ctx.CWD
	if strings.TrimSpace(cwd) == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	return newRuntimeMCPBackend(cwd, ctx.Language), nil
}

func (c *mcpCmd) toggle(ctx *Context, backend MCPBackend, args string, enabled bool) error {
	target := strings.TrimSpace(args)
	if target == "" {
		target = "all"
	}
	names := backend.ServerNames()
	if target != "all" {
		if _, ok := backend.State(target); !ok {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPServerNotFound, target))
			reportCommandFailed(ctx)
			return nil
		}
		names = []string{target}
	}

	var changed []string
	hadFailure := false
	for _, name := range names {
		if isInternalMCPServer(name, backend) {
			continue
		}
		state, ok := backend.State(name)
		if ok && isManagedMCPScope(state.Config.Scope) {
			ctx.OnEvent(i18n.Format(ctx.Language, mcpToggleKey(enabled, i18n.KeyMCPManagedEnable, i18n.KeyMCPManagedDisable), name))
			hadFailure = true
			continue
		}
		if _, err := backend.ToggleEnabled(context.Background(), name, enabled); err != nil {
			ctx.OnEvent(i18n.Format(ctx.Language, mcpToggleKey(enabled, i18n.KeyMCPEnableFailed, i18n.KeyMCPDisableFailed), name, err))
			hadFailure = true
			continue
		}
		changed = append(changed, name)
	}

	if len(changed) == 0 {
		ctx.OnEvent(i18n.Text(ctx.Language, mcpToggleKey(enabled, i18n.KeyMCPNoneEnabled, i18n.KeyMCPNoneDisabled)))
		if hadFailure {
			reportCommandFailed(ctx)
		} else {
			reportCommandSucceeded(ctx)
		}
		return nil
	}
	if path, err := persistMCPEnabledState(ctx, changed, enabled); err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, mcpToggleKey(enabled, i18n.KeyMCPEnabledPersistFailed, i18n.KeyMCPDisabledPersistFailed), len(changed), err))
		hadFailure = true
	} else if path != "" {
		ctx.OnEvent(i18n.Format(ctx.Language, mcpToggleKey(enabled, i18n.KeyMCPEnabledSaved, i18n.KeyMCPDisabledSaved), len(changed), path))
	} else {
		ctx.OnEvent(i18n.Format(ctx.Language, mcpToggleKey(enabled, i18n.KeyMCPEnabled, i18n.KeyMCPDisabled), len(changed)))
	}
	if hadFailure {
		reportCommandFailed(ctx)
	} else {
		reportCommandSucceeded(ctx)
	}
	return nil
}

func (c *mcpCmd) reconnect(ctx *Context, backend MCPBackend, args string) error {
	name := strings.TrimSpace(args)
	if name == "" {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyMCPUsageReconnect))
		reportCommandFailed(ctx)
		return nil
	}
	state, err := backend.Reconnect(context.Background(), name)
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPReconnectFailed, name, err))
		reportCommandFailed(ctx)
		return nil
	}
	switch state.Type {
	case svcmcp.MCPStateConnected:
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPReconnectSuccess, name, len(state.Tools), len(state.Resources), len(state.Prompts)))
		reportCommandSucceeded(ctx)
	case svcmcp.MCPStateNeedsAuth:
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPReconnectNeedsAuth, name, name))
		reportCommandFailed(ctx)
	case svcmcp.MCPStateDisabled:
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPReconnectDisabled, name, name))
		reportCommandFailed(ctx)
	default:
		if state.Error != "" {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPReconnectStateFailed, name, state.Error))
		} else {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPReconnectUnexpectedState, name, localizedMCPState(ctx.Language, state.Type)))
		}
		reportCommandFailed(ctx)
	}
	return nil
}

func (c *mcpCmd) authenticate(ctx *Context, backend MCPBackend, authenticator mcpAuthenticator, args string, jsonOutput bool) error {
	name := strings.TrimSpace(args)
	if name == "" {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyMCPUsageAuthenticate))
		reportCommandFailed(ctx)
		return nil
	}
	state, ok := backend.State(name)
	if !ok {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPServerNotFound, name))
		reportCommandFailed(ctx)
		return nil
	}
	result, err := authenticator.AuthURL(context.Background(), name, state.Config)
	if err != nil {
		result = mcpAuthResult{Status: "error", Message: err.Error()}
	}
	if jsonOutput {
		emitJSON(ctx, result)
		if result.Status == "error" || result.Status == "unsupported" {
			reportCommandFailed(ctx)
		} else {
			reportCommandSucceeded(ctx)
		}
		return nil
	}
	if result.AuthURL != "" {
		ctx.OnEvent(fmt.Sprintf("%s\n%s\n", result.Message, result.AuthURL))
		reportCommandSucceeded(ctx)
		return nil
	}
	ctx.OnEvent(result.Message + "\n")
	if result.Status == "error" || result.Status == "unsupported" {
		reportCommandFailed(ctx)
	} else {
		reportCommandSucceeded(ctx)
	}
	return nil
}

func (c *mcpCmd) addJSON(ctx *Context, backend MCPBackend, args string) error {
	name, raw := splitMCPVerb(args)
	if name == "" || strings.TrimSpace(raw) == "" {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyMCPUsageAddJSON))
		reportCommandFailed(ctx)
		return nil
	}
	var cfg svcmcp.MCPServerConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPInvalidServerJSON, err))
		reportCommandFailed(ctx)
		return nil
	}
	cfg.Name = name
	wrapped, err := json.Marshal(map[string]any{"mcpServers": map[string]svcmcp.MCPServerConfig{name: cfg}})
	if err != nil {
		return err
	}
	parsed, err := svcmcp.ParseMCPConfig(wrapped, svcmcp.ParseOptions{Scope: svcmcp.ScopeLocal, ExpandVars: true})
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPInvalidConfigError, err))
		reportCommandFailed(ctx)
		return nil
	}
	if len(parsed.Errors) > 0 {
		ctx.OnEvent(formatMCPValidationErrors(ctx.Language, parsed.Errors))
		reportCommandFailed(ctx)
		return nil
	}
	cfg = parsed.Servers[name]
	if path, err := persistMCPServerConfig(ctx, name, cfg); err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPSaveServerFailed, name, err))
		reportCommandFailed(ctx)
		return nil
	} else {
		backend.AddConfig(name, cfg)
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPServerAdded, name, path))
	}
	reportCommandSucceeded(ctx)
	return nil
}

func (c *mcpCmd) remove(ctx *Context, backend MCPBackend, args string) error {
	name := strings.TrimSpace(args)
	if name == "" {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyMCPUsageRemove))
		reportCommandFailed(ctx)
		return nil
	}
	if _, ok := backend.State(name); !ok {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPServerNotFound, name))
		reportCommandFailed(ctx)
		return nil
	}
	path, configs, err := removeMCPServerConfig(ctx, name)
	if err != nil {
		if errors.Is(err, errMCPServerNotWritable) {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPServerNotWritable, name))
		} else {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPRemoveServerFailed, name, err))
		}
		reportCommandFailed(ctx)
		return nil
	}
	backend.SetConfigs(configs)
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPServerRemoved, name, path))
	reportCommandSucceeded(ctx)
	return nil
}

type mcpRuntimeBackend struct {
	manager     *svcmcp.Manager
	sources     map[string]string
	diagnostics []string
	language    i18n.Language
}

func newRuntimeMCPBackend(cwd string, language i18n.Language) *mcpRuntimeBackend {
	manager := svcmcp.NewManager(svcmcp.WithWorkingDirectory(cwd))
	b := &mcpRuntimeBackend{
		manager:  manager,
		sources:  make(map[string]string),
		language: language,
	}
	for _, candidate := range mcpConfigCandidates(cwd) {
		b.load(candidate.path, candidate.scope)
	}
	b.collectCommandDiagnostics()
	return b
}

func (b *mcpRuntimeBackend) load(path string, scope svcmcp.ConfigScope) {
	parsed, err := svcmcp.ParseMCPConfigFile(path, svcmcp.ParseOptions{
		Scope:      scope,
		ExpandVars: true,
		FilePath:   path,
	})
	if err != nil {
		if !os.IsNotExist(err) {
			b.diagnostics = append(b.diagnostics, fmt.Sprintf("%s: %v", path, err))
		}
		return
	}
	for _, validation := range parsed.Errors {
		b.diagnostics = append(b.diagnostics, fmt.Sprintf("%s: %s", validation.Path, validation.Message))
	}
	for name, cfg := range parsed.Servers {
		cfg.Scope = scope
		b.manager.AddConfig(name, cfg)
		b.sources[name] = path
		if parsed.IsServerDisabled(name) {
			if _, err := b.manager.ToggleEnabled(context.Background(), name, false); err != nil {
				b.diagnostics = append(b.diagnostics, i18n.Format(b.language, i18n.KeyMCPDiagnosticMarkDisabled, name, err))
			}
		}
	}
}

func (b *mcpRuntimeBackend) collectCommandDiagnostics() {
	for _, state := range b.manager.Snapshot() {
		if state.Config.Type != "" && state.Config.Type != svcmcp.TransportStdio {
			continue
		}
		command := strings.TrimSpace(state.Config.Command)
		if command == "" {
			continue
		}
		if strings.Contains(command, string(os.PathSeparator)) {
			if _, err := os.Stat(command); err != nil {
				b.diagnostics = append(b.diagnostics, i18n.Format(b.language, i18n.KeyMCPDiagnosticCommandMissing, state.Name, command))
			}
			continue
		}
		if _, err := exec.LookPath(command); err != nil {
			b.diagnostics = append(b.diagnostics, i18n.Format(b.language, i18n.KeyMCPDiagnosticCommandNotPath, state.Name, command))
		}
	}
}

func (b *mcpRuntimeBackend) ServerNames() []string { return b.manager.ServerNames() }

func (b *mcpRuntimeBackend) Snapshot() []svcmcp.MCPServerConnection { return b.manager.Snapshot() }

func (b *mcpRuntimeBackend) HealthSnapshot() svcmcp.HealthSnapshot {
	return b.manager.HealthSnapshot()
}

func (b *mcpRuntimeBackend) State(name string) (svcmcp.MCPServerConnection, bool) {
	return b.manager.State(name)
}

func (b *mcpRuntimeBackend) AddConfig(name string, cfg svcmcp.MCPServerConfig) {
	b.manager.AddConfig(name, cfg)
}

func (b *mcpRuntimeBackend) SetConfigs(configs map[string]svcmcp.MCPServerConfig) {
	b.manager.SetConfigs(configs)
}

func (b *mcpRuntimeBackend) ToggleEnabled(ctx context.Context, name string, enabled bool) (svcmcp.MCPServerConnection, error) {
	return b.manager.ToggleEnabled(ctx, name, enabled)
}

func (b *mcpRuntimeBackend) Reconnect(ctx context.Context, name string) (svcmcp.MCPServerConnection, error) {
	return b.manager.Reconnect(ctx, name)
}

func (b *mcpRuntimeBackend) ConfigSource(name string) string {
	if b == nil {
		return ""
	}
	return b.sources[name]
}

func (b *mcpRuntimeBackend) Diagnostics() []string {
	if b == nil || len(b.diagnostics) == 0 {
		return nil
	}
	out := append([]string(nil), b.diagnostics...)
	sort.Strings(out)
	return out
}

type mcpConfigCandidate struct {
	path  string
	scope svcmcp.ConfigScope
}

func mcpConfigCandidates(cwd string) []mcpConfigCandidate {
	return []mcpConfigCandidate{
		{path: filepath.Join(brand.LegacyUserConfigDir(), "settings.json"), scope: svcmcp.ScopeUser},
		{path: filepath.Join(brand.LegacyDeepSeekUserConfigDir(), "settings.json"), scope: svcmcp.ScopeUser},
		{path: filepath.Join(brand.UserConfigDir(), "settings.json"), scope: svcmcp.ScopeUser},
		{path: filepath.Join(cwd, ".mcp.json"), scope: svcmcp.ScopeProject},
		{path: filepath.Join(cwd, brand.LegacyConfigDirName, "settings.json"), scope: svcmcp.ScopeLocal},
		{path: filepath.Join(cwd, brand.LegacyDeepSeekConfigDirName, "settings.json"), scope: svcmcp.ScopeLocal},
		{path: filepath.Join(cwd, brand.ConfigDirName, "settings.json"), scope: svcmcp.ScopeLocal},
	}
}

type defaultMCPAuthenticator struct {
	language i18n.Language
}

func (d defaultMCPAuthenticator) AuthURL(ctx context.Context, serverName string, cfg svcmcp.MCPServerConfig) (mcpAuthResult, error) {
	transport := cfg.Type
	if transport == "" {
		transport = svcmcp.TransportStdio
	}
	switch transport {
	case svcmcp.TransportHTTP, svcmcp.TransportSSE:
		flow, err := svcmcp.NewOAuthManager(nil, nil).StartOAuthFlow(ctx, serverName, cfg, svcmcp.OAuthFlowOptions{
			SkipBrowserOpen: true,
			Timeout:         10 * time.Minute,
		})
		if err != nil {
			return mcpAuthResult{}, err
		}
		return mcpAuthResult{
			Status:  "auth_url",
			AuthURL: flow.AuthorizationURL,
			Message: i18n.Format(d.language, i18n.KeyMCPAuthOpenURL, serverName),
		}, nil
	case svcmcp.TransportClaudeAIProxy:
		return mcpAuthResult{
			Status:  "unsupported",
			Message: i18n.Format(d.language, i18n.KeyMCPAuthClaudeConnector, serverName),
			AuthURL: "https://claude.ai/settings/connectors",
		}, nil
	default:
		return mcpAuthResult{
			Status:  "unsupported",
			Message: i18n.Format(d.language, i18n.KeyMCPAuthUnsupportedTransport, serverName, transport),
		}, nil
	}
}

type mcpStatusReport struct {
	GeneratedAt time.Time           `json:"generatedAt"`
	Counts      svcmcp.HealthCounts `json:"counts"`
	Servers     []mcpServerReport   `json:"servers"`
	Diagnostics []string            `json:"diagnostics,omitempty"`
}

type mcpServerReport struct {
	Name                 string                    `json:"name"`
	State                svcmcp.MCPConnectionState `json:"state"`
	Transport            svcmcp.TransportType      `json:"transport"`
	Scope                svcmcp.ConfigScope        `json:"scope,omitempty"`
	ConfigSource         string                    `json:"configSource,omitempty"`
	Capabilities         []string                  `json:"capabilities,omitempty"`
	ToolsCount           int                       `json:"toolsCount"`
	ResourcesCount       int                       `json:"resourcesCount"`
	PromptsCount         int                       `json:"promptsCount"`
	AuthStatus           string                    `json:"authStatus"`
	LastError            string                    `json:"lastError,omitempty"`
	ReconnectAttempt     int                       `json:"reconnectAttempt,omitempty"`
	MaxReconnectAttempts int                       `json:"maxReconnectAttempts,omitempty"`
	ServerInfoName       string                    `json:"serverInfoName,omitempty"`
	ServerInfoVersion    string                    `json:"serverInfoVersion,omitempty"`
	DiagnosticWarnings   []string                  `json:"diagnosticWarnings,omitempty"`
}

func buildMCPStatusReport(backend MCPBackend, language i18n.Language) mcpStatusReport {
	health := backend.HealthSnapshot()
	report := mcpStatusReport{
		GeneratedAt: health.GeneratedAt,
		Counts:      health.Counts,
	}
	if sourceProvider, ok := backend.(mcpSourceProvider); ok {
		report.Diagnostics = sourceProvider.Diagnostics()
	}
	states := backend.Snapshot()
	report.Servers = make([]mcpServerReport, 0, len(states))
	for _, state := range states {
		row := mcpServerReport{
			Name:                 state.Name,
			State:                state.Type,
			Transport:            normalizedTransport(state.Config.Type),
			Scope:                state.Config.Scope,
			Capabilities:         capabilityNames(state.Capabilities),
			ToolsCount:           len(state.Tools),
			ResourcesCount:       len(state.Resources),
			PromptsCount:         len(state.Prompts),
			AuthStatus:           authStatus(state),
			LastError:            state.Error,
			ReconnectAttempt:     state.ReconnectAttempt,
			MaxReconnectAttempts: state.MaxReconnectAttempts,
		}
		if state.ServerInfo != nil {
			row.ServerInfoName = state.ServerInfo.Name
			row.ServerInfoVersion = state.ServerInfo.Version
		}
		if sourceProvider, ok := backend.(mcpSourceProvider); ok {
			row.ConfigSource = sourceProvider.ConfigSource(state.Name)
		}
		if state.NeedsAuth != nil {
			if state.NeedsAuth.Scope != "" {
				row.DiagnosticWarnings = append(row.DiagnosticWarnings, i18n.Format(language, i18n.KeyMCPNeedsOAuthScope, state.NeedsAuth.Scope))
			}
			if state.NeedsAuth.ResourceMetadataURL != "" {
				row.DiagnosticWarnings = append(row.DiagnosticWarnings, i18n.Format(language, i18n.KeyMCPResourceMetadata, state.NeedsAuth.ResourceMetadataURL))
			}
		}
		report.Servers = append(report.Servers, row)
	}
	sort.Slice(report.Servers, func(i, j int) bool { return report.Servers[i].Name < report.Servers[j].Name })
	return report
}

func emitMCPReport(ctx *Context, report mcpStatusReport, jsonOutput bool) {
	if jsonOutput {
		emitJSON(ctx, report)
		return
	}
	var sb strings.Builder
	if len(report.Servers) == 0 {
		sb.WriteString(i18n.Text(ctx.Language, i18n.KeyMCPNoServers))
		emit(ctx, sb.String())
		return
	}
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPServersSummary,
		len(report.Servers), report.Counts.Connected, report.Counts.Pending, report.Counts.Failed, report.Counts.NeedsAuth, report.Counts.Disabled))
	for _, server := range report.Servers {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPServerRow,
			server.Name,
			localizedMCPState(ctx.Language, server.State),
			server.Transport,
			localizedMCPScope(ctx.Language, server.Scope),
			server.ToolsCount,
			server.ResourcesCount,
			server.PromptsCount,
			localizedMCPAuthStatus(ctx.Language, server.AuthStatus)))
		if server.ReconnectAttempt > 0 {
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPReconnectProgress, server.ReconnectAttempt, server.MaxReconnectAttempts))
		}
		if server.ConfigSource != "" {
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPConfigSource, server.ConfigSource))
		}
		sb.WriteString("\n")
		if server.LastError != "" {
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPLastError, server.LastError))
		}
		for _, warning := range server.DiagnosticWarnings {
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPWarning, warning))
		}
	}
	if len(report.Diagnostics) > 0 {
		sb.WriteString("\n" + i18n.Text(ctx.Language, i18n.KeyMCPDiagnostics))
		for _, warning := range report.Diagnostics {
			sb.WriteString("- " + warning + "\n")
		}
	}
	emit(ctx, sb.String())
}

func emitMCPServer(ctx *Context, server mcpServerReport, jsonOutput bool) {
	if jsonOutput {
		emitJSON(ctx, server)
		return
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s:\n", server.Name))
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailState, localizedMCPState(ctx.Language, server.State)))
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailTransport, server.Transport))
	if server.Scope != "" {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailScope, localizedMCPScope(ctx.Language, server.Scope)))
	}
	if server.ConfigSource != "" {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailSource, server.ConfigSource))
	}
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailAuth, localizedMCPAuthStatus(ctx.Language, server.AuthStatus)))
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailTools, server.ToolsCount))
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailResources, server.ResourcesCount))
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailPrompts, server.PromptsCount))
	if len(server.Capabilities) > 0 {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailCapabilities, strings.Join(server.Capabilities, ", ")))
	}
	if server.ServerInfoName != "" || server.ServerInfoVersion != "" {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailServer, server.ServerInfoName, server.ServerInfoVersion))
	}
	if server.LastError != "" {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailLastError, server.LastError))
	}
	if server.ReconnectAttempt > 0 {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDetailReconnectAttempts, server.ReconnectAttempt, server.MaxReconnectAttempts))
	}
	for _, warning := range server.DiagnosticWarnings {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPWarning, warning))
	}
	emit(ctx, sb.String())
}

func emitMCPDiagnostics(ctx *Context, report mcpStatusReport, jsonOutput bool) {
	if jsonOutput {
		emitJSON(ctx, report)
		return
	}
	var sb strings.Builder
	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyMCPDiagnostics))
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDiagnosticCounts,
		report.Counts.Connected, report.Counts.Pending, report.Counts.Failed, report.Counts.NeedsAuth, report.Counts.Disabled))
	for _, server := range report.Servers {
		if server.State == svcmcp.MCPStateConnected {
			continue
		}
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPDiagnosticServerState, server.Name, localizedMCPState(ctx.Language, server.State)))
		if server.LastError != "" {
			sb.WriteString(" - " + server.LastError)
		}
		sb.WriteString("\n")
	}
	for _, warning := range report.Diagnostics {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyMCPWarning, warning))
	}
	emit(ctx, sb.String())
}

func emitJSON(ctx *Context, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		emit(ctx, fmt.Sprintf(`{"error":%q}`+"\n", err.Error()))
		return
	}
	emit(ctx, string(data)+"\n")
}

func emit(ctx *Context, text string) {
	if ctx != nil && ctx.OnEvent != nil {
		ctx.OnEvent(text)
	}
}

func splitMCPVerb(args string) (string, string) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", ""
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return "", ""
	}
	verb := fields[0]
	rest := strings.TrimSpace(strings.TrimPrefix(args, verb))
	return verb, rest
}

func mcpUsage(lang i18n.Language) string {
	return i18n.Text(lang, i18n.KeyMCPUsage)
}

func normalizedTransport(t svcmcp.TransportType) svcmcp.TransportType {
	if t == "" {
		return svcmcp.TransportStdio
	}
	return t
}

func capabilityNames(caps svcmcp.ServerCapabilities) []string {
	if len(caps) == 0 {
		return nil
	}
	names := make([]string, 0, len(caps))
	for name := range caps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func authStatus(state svcmcp.MCPServerConnection) string {
	transport := normalizedTransport(state.Config.Type)
	if state.Type == svcmcp.MCPStateNeedsAuth {
		return "needs-auth"
	}
	switch transport {
	case svcmcp.TransportHTTP, svcmcp.TransportSSE:
		if state.Type == svcmcp.MCPStateConnected && (len(state.Tools) > 0 || len(state.Resources) > 0 || len(state.Prompts) > 0) {
			return "authenticated"
		}
		if state.Config.OAuth != nil {
			return "oauth-configured"
		}
		return "unknown"
	case svcmcp.TransportClaudeAIProxy:
		return "claude.ai"
	default:
		return "not-applicable"
	}
}

func localizedMCPState(lang i18n.Language, state svcmcp.MCPConnectionState) string {
	key := i18n.Key("")
	switch state {
	case svcmcp.MCPStatePending:
		key = i18n.KeyMCPStatePending
	case svcmcp.MCPStateConnected:
		key = i18n.KeyMCPStateConnected
	case svcmcp.MCPStateFailed:
		key = i18n.KeyMCPStateFailed
	case svcmcp.MCPStateNeedsAuth:
		key = i18n.KeyMCPStateNeedsAuth
	case svcmcp.MCPStateDisabled:
		key = i18n.KeyMCPStateDisabled
	default:
		return string(state)
	}
	return i18n.Text(lang, key)
}

func localizedMCPAuthStatus(lang i18n.Language, status string) string {
	key := i18n.Key("")
	switch status {
	case "needs-auth":
		key = i18n.KeyMCPAuthStatusNeedsAuth
	case "authenticated":
		key = i18n.KeyMCPAuthStatusAuthenticated
	case "oauth-configured":
		key = i18n.KeyMCPAuthStatusConfigured
	case "unknown":
		key = i18n.KeyMCPAuthStatusUnknown
	case "not-applicable":
		key = i18n.KeyMCPAuthStatusNotApplicable
	default:
		return status
	}
	return i18n.Text(lang, key)
}

func localizedMCPScope(lang i18n.Language, scope svcmcp.ConfigScope) string {
	key := i18n.Key("")
	switch scope {
	case svcmcp.ScopeLocal:
		key = i18n.KeyMCPScopeLocal
	case svcmcp.ScopeUser:
		key = i18n.KeyMCPScopeUser
	case svcmcp.ScopeProject:
		key = i18n.KeyMCPScopeProject
	case svcmcp.ScopeDynamic:
		key = i18n.KeyMCPScopeDynamic
	case svcmcp.ScopeEnterprise:
		key = i18n.KeyMCPScopeEnterprise
	case svcmcp.ScopeClaudeAI:
		key = i18n.KeyMCPScopeClaudeAI
	case svcmcp.ScopeManaged:
		key = i18n.KeyMCPScopeManaged
	default:
		return string(scope)
	}
	return i18n.Text(lang, key)
}

func isInternalMCPServer(name string, backend MCPBackend) bool {
	state, ok := backend.State(name)
	if !ok {
		return false
	}
	return name == "ide" || state.Config.Type == svcmcp.TransportSSEIDE || state.Config.Type == svcmcp.TransportWebSocketIDE
}

func isManagedMCPScope(scope svcmcp.ConfigScope) bool {
	return scope == svcmcp.ScopeEnterprise || scope == svcmcp.ScopeManaged
}

func mcpToggleKey(enabled bool, enabledKey, disabledKey i18n.Key) i18n.Key {
	if enabled {
		return enabledKey
	}
	return disabledKey
}

func persistMCPEnabledState(ctx *Context, names []string, enabled bool) (string, error) {
	path := mcpWritableSettingsPath(ctx)
	cmd := configCmd{}
	settings, err := cmd.readSettings(path, ctx.Language)
	if err != nil {
		return path, err
	}
	enabledList := stringSliceFromSettings(settings["enabledMcpServers"])
	disabledList := stringSliceFromSettings(settings["disabledMcpServers"])
	for _, name := range names {
		if enabled {
			disabledList = removeString(disabledList, name)
			enabledList = appendUniqueString(enabledList, name)
		} else {
			enabledList = removeString(enabledList, name)
			disabledList = appendUniqueString(disabledList, name)
		}
	}
	sort.Strings(enabledList)
	sort.Strings(disabledList)
	settings["enabledMcpServers"] = enabledList
	settings["disabledMcpServers"] = disabledList
	return path, cmd.writeSettings(path, settings)
}

func persistMCPServerConfig(ctx *Context, name string, cfg svcmcp.MCPServerConfig) (string, error) {
	path := mcpWritableSettingsPath(ctx)
	cmd := configCmd{}
	settings, err := cmd.readSettings(path, ctx.Language)
	if err != nil {
		return path, err
	}
	servers := mcpServersFromSettings(settings["mcpServers"])
	servers[name] = cfg
	settings["mcpServers"] = servers
	settings["disabledMcpServers"] = removeString(stringSliceFromSettings(settings["disabledMcpServers"]), name)
	return path, cmd.writeSettings(path, settings)
}

var errMCPServerNotWritable = errors.New("mcp_server_not_writable")

func removeMCPServerConfig(ctx *Context, name string) (string, map[string]svcmcp.MCPServerConfig, error) {
	path := mcpWritableSettingsPath(ctx)
	cmd := configCmd{}
	settings, err := cmd.readSettings(path, ctx.Language)
	if err != nil {
		return path, nil, err
	}
	servers := mcpServersFromSettings(settings["mcpServers"])
	if _, ok := servers[name]; !ok {
		return path, nil, errMCPServerNotWritable
	}
	delete(servers, name)
	settings["mcpServers"] = servers
	settings["enabledMcpServers"] = removeString(stringSliceFromSettings(settings["enabledMcpServers"]), name)
	settings["disabledMcpServers"] = removeString(stringSliceFromSettings(settings["disabledMcpServers"]), name)
	if err := cmd.writeSettings(path, settings); err != nil {
		return path, nil, err
	}
	return path, servers, nil
}

func mcpWritableSettingsPath(ctx *Context) string {
	cwd := ""
	if ctx != nil {
		cwd = ctx.CWD
	}
	if strings.TrimSpace(cwd) == "" {
		cwd, _ = os.Getwd()
	}
	return filepath.Join(cwd, brand.ConfigDirName, "settings.json")
}

func mcpServersFromSettings(value any) map[string]svcmcp.MCPServerConfig {
	out := make(map[string]svcmcp.MCPServerConfig)
	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "null" {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func stringSliceFromSettings(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func removeString(values []string, value string) []string {
	out := values[:0]
	for _, existing := range values {
		if existing != value {
			out = append(out, existing)
		}
	}
	return out
}

func formatMCPValidationErrors(lang i18n.Language, errorsIn []svcmcp.ValidationError) string {
	if len(errorsIn) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(i18n.Text(lang, i18n.KeyMCPInvalidConfig))
	for _, validation := range errorsIn {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", validation.Path, validation.Message))
	}
	return sb.String()
}

func mcpDoctorCheckWithBackend(cwd string, backend MCPBackend) checkResult {
	r := checkResult{label: "MCP"}
	language := i18n.DetectOrLoadLanguage()
	if backend == nil {
		backend = newRuntimeMCPBackend(cwd, language)
	}
	report := buildMCPStatusReport(backend, language)
	if len(report.Servers) == 0 {
		r.ok = true
		r.message = i18n.Text(language, i18n.KeyMCPDoctorNoServers)
		return r
	}
	if report.Counts.Failed > 0 || report.Counts.NeedsAuth > 0 {
		r.ok = false
		r.message = i18n.Format(language, i18n.KeyMCPDoctorFailed,
			len(report.Servers), report.Counts.Failed, report.Counts.NeedsAuth, report.Counts.Pending, report.Counts.Disabled)
		return r
	}
	if len(report.Diagnostics) > 0 {
		r.ok = false
		r.message = i18n.Format(language, i18n.KeyMCPDoctorDiagnostics, len(report.Servers), strings.Join(report.Diagnostics, "; "))
		return r
	}
	r.ok = true
	r.message = i18n.Format(language, i18n.KeyMCPDoctorConfigured,
		len(report.Servers), report.Counts.Connected, report.Counts.Pending, report.Counts.Disabled)
	return r
}
