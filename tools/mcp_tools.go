package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/mcp"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
)

// H28: Shared HTTP client to avoid creating a new connection pool per call.
var sharedMCPHTTPClient = &http.Client{Timeout: 30 * time.Second}

// MCP-03: per-call timeout. Defaults to 60s; override per-server via the
// X-MCP-Tool-Timeout-Ms header in MCPServerConfig (not yet wired) or
// SetDefaultMCPCallTimeout. Without this an in-flight tool call could hang
// indefinitely because the connection-level 30s timeout only guards
// *establishing* the connection, not subsequent JSON-RPC frames.
const defaultMCPCallTimeout = 60 * time.Second

var (
	mcpCallTimeoutMu sync.RWMutex
	mcpCallTimeout   = defaultMCPCallTimeout
)

// SetDefaultMCPCallTimeout overrides the per-call timeout used for MCP
// tools/call, resources/list, and resources/read. Pass 0 to restore the
// default of 60s.
func SetDefaultMCPCallTimeout(d time.Duration) {
	mcpCallTimeoutMu.Lock()
	if d <= 0 {
		mcpCallTimeout = defaultMCPCallTimeout
	} else {
		mcpCallTimeout = d
	}
	mcpCallTimeoutMu.Unlock()
}

// effectiveMCPCallTimeout returns the active per-call timeout.
func effectiveMCPCallTimeout() time.Duration {
	mcpCallTimeoutMu.RLock()
	defer mcpCallTimeoutMu.RUnlock()
	return mcpCallTimeout
}

// withMCPCallTimeout wraps ctx with the configured per-call deadline. If
// the supplied ctx already has an earlier deadline, that deadline wins.
func withMCPCallTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := effectiveMCPCallTimeout()
	if existingDeadline, ok := ctx.Deadline(); ok {
		if time.Until(existingDeadline) < timeout {
			return ctx, func() {}
		}
	}
	return context.WithTimeout(ctx, timeout)
}

// MCPTokenSource resolves a Bearer token for a given MCP server URL. The audit
// (P2-6) requires the HTTP-mode fallback to consult an auth resolver so a
// services-layer TokenSource can plug in its OAuth state. The default returns
// an empty string, which we still surface as `Authorization: Bearer ` so tests
// can assert the wiring is in place even before a real OAuth flow runs.
type MCPTokenSource interface {
	TokenFor(ctx context.Context, serverURL string) (string, error)
}

// MCPTokenSourceFunc is a closure-backed MCPTokenSource.
type MCPTokenSourceFunc func(ctx context.Context, serverURL string) (string, error)

// TokenFor implements MCPTokenSource.
func (f MCPTokenSourceFunc) TokenFor(ctx context.Context, serverURL string) (string, error) {
	return f(ctx, serverURL)
}

// defaultMCPTokenSource returns an empty token, which still produces a
// well-formed `Authorization: Bearer ` header so the tool layer's wiring is
// verifiable without a live OAuth flow.
var defaultMCPTokenSource MCPTokenSource = MCPTokenSourceFunc(func(ctx context.Context, serverURL string) (string, error) {
	return "", nil
})

// SetDefaultMCPTokenSource swaps the package-level token resolver. The daemon
// or CLI bootstrap can install a real OAuth-backed source here so HTTP-mode
// MCP calls automatically attach Bearer credentials.
func SetDefaultMCPTokenSource(src MCPTokenSource) {
	if src == nil {
		return
	}
	defaultMCPTokenSource = src
}

// resolveMCPBearer asks the configured token source for a token and returns
// the value to send in the `Authorization` header. When the resolver yields
// an empty string we still send an anonymous Bearer marker so the audit's
// "wiring exists" contract is observable end-to-end.
func resolveMCPBearer(ctx context.Context, serverURL string) string {
	src := defaultMCPTokenSource
	tok := ""
	if src != nil {
		if got, err := src.TokenFor(ctx, serverURL); err == nil {
			tok = got
		}
	}
	if tok == "" {
		tok = "anonymous"
	}
	return "Bearer " + tok
}

// ─── MCPManager ───────────────────────────────────────────────────────────────

// MCPServerConfig defines an MCP server process (command + args + env).
// It is a type alias for mcp.ServerConfig so callers can use either package.
type MCPServerConfig = mcp.ServerConfig

// MCPServerTool is a tool advertised by an MCP server.
type MCPServerTool struct {
	Name        string
	Description string
}

// MCPServerConn holds a live connection to a running MCP server process.
type MCPServerConn struct {
	Config       MCPServerConfig
	client       *mcp.Client
	tools        []MCPServerTool
	capabilities svcmcp.ServerCapabilities
	ready        bool
	httpBaseURL  string // for HTTP-mode fallback (backward compat with tests)
}

// MCPManager manages real MCP server processes connected via the Model Context
// Protocol (JSON-RPC 2.0 over stdio).
type MCPManager struct {
	mu                  sync.RWMutex
	servers             map[string]*MCPServerConn  // name → active connection
	configs             map[string]MCPServerConfig // name → active config (may not be connected yet)
	nonWorkspaceConfigs map[string]MCPServerConfig // persistent layer hidden by same-name project config
	// connectStartedForTest is a package-private lifecycle seam. Tests use it
	// to observe which cloned manager owns a Connect attempt without polling a
	// subprocess side-channel. Production leaves it nil.
	connectStartedForTest func(*MCPManager, string)

	// MCP-06: deterministic priority order for tool-name conflict
	// resolution. The lowest-index server wins on a tie; later servers
	// have their tools registered with `__<server>` suffix.
	priorityOrder []string

	// MCP-02: stdio child-process restart bookkeeping. Keyed by server
	// name. The manager increments restart attempts on subprocess
	// death and gives up after maxStdioRestarts.
	restartState map[string]*stdioRestartTracker
	stdioMu      sync.Mutex
	lifecycle    *RuntimeLifecycle
}

const (
	// MCP-02: stdio restart cooldowns + cap. Mirrors TS contract.
	maxStdioRestarts = 3
)

var stdioRestartCooldowns = []time.Duration{
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
}

type stdioRestartTracker struct {
	attempts    int
	lastAttempt time.Time
	failed      bool
}

type mcpServerState struct {
	config    MCPServerConfig
	hasConfig bool
	conn      *MCPServerConn
	hasConn   bool
}

// NewMCPManager creates an empty MCPManager.
func NewMCPManager() *MCPManager {
	return &MCPManager{
		servers:             make(map[string]*MCPServerConn),
		configs:             make(map[string]MCPServerConfig),
		nonWorkspaceConfigs: make(map[string]MCPServerConfig),
		restartState:        make(map[string]*stdioRestartTracker),
	}
}

// SetProjectRoot targets the shared runtime lifecycle journal used for MCP
// cache invalidation events. It is safe to retarget on session switches.
func (m *MCPManager) SetProjectRoot(root string) {
	if m == nil || strings.TrimSpace(root) == "" {
		return
	}
	m.mu.Lock()
	m.lifecycle = NewRuntimeLifecycle(filepath.Clean(root))
	m.mu.Unlock()
}

// SetPriorityOrder configures the explicit priority order for MCP-06
// tool-name conflict resolution. When two servers expose the same logical
// tool, the lower-index server wins; the loser is registered with a
// `__<serverName>` suffix. Servers not present in the list are appended in
// their original (insertion) order.
func (m *MCPManager) SetPriorityOrder(order []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(order) == 0 {
		m.priorityOrder = nil
		return
	}
	cp := make([]string, len(order))
	copy(cp, order)
	m.priorityOrder = cp
}

// ResolveToolName applies MCP-06 conflict resolution to (server, tool).
// Returns the externally-visible tool name. The lowest-priority server for
// a given logical name keeps it bare; everyone else gets `tool__server`.
func (m *MCPManager) ResolveToolName(server, tool string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	winner, _ := m.lowestPriorityServerForLocked(tool)
	if winner == "" || winner == server {
		return tool
	}
	return tool + "__" + server
}

func (m *MCPManager) lowestPriorityServerForLocked(tool string) (string, []string) {
	priority := m.priorityOrder
	if len(priority) == 0 {
		// Use the configs map's deterministic name order.
		names := make([]string, 0, len(m.configs))
		for n := range m.configs {
			names = append(names, n)
		}
		sort.Strings(names)
		priority = names
	}
	candidates := make([]string, 0)
	for _, name := range priority {
		conn, ok := m.servers[name]
		if !ok {
			continue
		}
		for _, t := range conn.tools {
			if t.Name == tool {
				candidates = append(candidates, name)
				break
			}
		}
	}
	if len(candidates) == 0 {
		return "", nil
	}
	return candidates[0], candidates
}

// LoadFromSettings reads MCP server configs from a settings.json file.
// A missing file is silently ignored (non-fatal).
func (m *MCPManager) LoadFromSettings(path string) error {
	return m.LoadFromSettingsWithScope(path, svcmcp.ScopeProject)
}

// LoadFromSettingsWithScope reads MCP server configs from a settings.json file
// and tags them with the source scope used by the TypeScript config layer.
func (m *MCPManager) LoadFromSettingsWithScope(path string, scope svcmcp.ConfigScope) error {
	if scope == svcmcp.ScopeProject {
		settingsDir := filepath.Dir(filepath.Clean(path))
		m.SetProjectRoot(filepath.Dir(settingsDir))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return i18n.WrapError(i18n.KeyToolSourceSinkMCPReadSettings, err, path)
	}

	parsed, err := svcmcp.ParseMCPConfig(data, svcmcp.ParseOptions{
		Scope:      scope,
		ExpandVars: true,
		FilePath:   path,
	})
	if err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkMCPParseSettings, err, path)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for name, cfg := range parsed.Servers {
		runtimeCfg := runtimeMCPServerConfig(cfg)
		m.configs[name] = runtimeCfg
		if !isWorkspaceMCPConfigScope(runtimeCfg.Scope) {
			m.nonWorkspaceConfigs[name] = runtimeCfg
		}
	}
	return nil
}

func runtimeMCPServerConfig(cfg svcmcp.MCPServerConfig) MCPServerConfig {
	runtimeCfg := MCPServerConfig{
		Type:                string(cfg.Type),
		Command:             cfg.Command,
		Args:                append([]string(nil), cfg.Args...),
		Env:                 cloneStringMap(cfg.Env),
		URL:                 cfg.URL,
		Headers:             cloneStringMap(cfg.Headers),
		HeadersHelper:       cfg.HeadersHelper,
		IDEName:             cfg.IDEName,
		AuthToken:           cfg.AuthToken,
		IDERunningInWindows: cfg.IDERunningInWindows,
		Name:                cfg.Name,
		ID:                  cfg.ID,
		Scope:               string(cfg.Scope),
		PluginSource:        cfg.PluginSource,
	}
	if cfg.OAuth != nil {
		runtimeCfg.OAuth = &mcp.OAuthConfig{
			ClientID:              cfg.OAuth.ClientID,
			CallbackPort:          cfg.OAuth.CallbackPort,
			AuthServerMetadataURL: cfg.OAuth.AuthServerMetadataURL,
			XAA:                   cfg.OAuth.XAA,
		}
	}
	return runtimeCfg
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// AddConfig registers a server configuration without connecting.
func (m *MCPManager) AddConfig(name string, cfg MCPServerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configs[name] = cfg
	if !isWorkspaceMCPConfigScope(cfg.Scope) {
		m.nonWorkspaceConfigs[name] = cfg
	}
}

// ReplaceWorkspaceConfigs replaces project/local MCP configuration while
// retaining user, managed, enterprise, and dynamic servers. Session switches
// must not accumulate servers from previously active workspaces.
func (m *MCPManager) ReplaceWorkspaceConfigs(configs map[string]MCPServerConfig) {
	if m == nil {
		return
	}
	var closeClients []*mcp.Client
	m.mu.Lock()
	next := make(map[string]MCPServerConfig, len(m.nonWorkspaceConfigs)+len(configs))
	for name, cfg := range m.nonWorkspaceConfigs {
		next[name] = cfg
	}
	for name, cfg := range configs {
		next[name] = cfg
	}
	for name, conn := range m.servers {
		oldCfg, hadOld := m.configs[name]
		_, retained := next[name]
		_, replaced := configs[name]
		if !retained || replaced || (hadOld && isWorkspaceMCPConfigScope(oldCfg.Scope)) {
			if conn != nil && conn.client != nil {
				closeClients = append(closeClients, conn.client)
			}
			delete(m.servers, name)
		}
	}
	m.configs = next
	m.mu.Unlock()
	for _, client := range closeClients {
		_ = client.Close()
	}
}

// ReplaceWorkspaceServiceConfigs is the services-layer config adapter used by
// registry session publication so both MCP managers consume one parsed target
// snapshot.
func (m *MCPManager) ReplaceWorkspaceServiceConfigs(configs map[string]svcmcp.MCPServerConfig) {
	converted := make(map[string]MCPServerConfig, len(configs))
	for name, cfg := range configs {
		converted[name] = runtimeMCPServerConfig(cfg)
	}
	m.ReplaceWorkspaceConfigs(converted)
}

func isWorkspaceMCPConfigScope(scope string) bool {
	switch svcmcp.ConfigScope(strings.TrimSpace(scope)) {
	case svcmcp.ScopeProject, svcmcp.ScopeLocal:
		return true
	default:
		return false
	}
}

func (m *MCPManager) snapshotServer(name string) mcpServerState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := mcpServerState{}
	if cfg, ok := m.configs[name]; ok {
		state.config = cfg
		state.hasConfig = true
	}
	if conn, ok := m.servers[name]; ok {
		state.conn = conn
		state.hasConn = true
	}
	return state
}

func (m *MCPManager) restoreServer(name string, state mcpServerState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.servers[name]; ok && (!state.hasConn || current != state.conn) {
		if current.ready && current.client != nil {
			current.client.Close() //nolint:errcheck
		}
	}
	if state.hasConfig {
		m.configs[name] = state.config
	} else {
		delete(m.configs, name)
	}
	if state.hasConn {
		m.servers[name] = state.conn
	} else {
		delete(m.servers, name)
	}
}

// Connect ensures the named server is running and returns its connection.
// If already connected the cached connection is returned.
func (m *MCPManager) Connect(name string) (*MCPServerConn, error) {
	// Fast path: already connected.
	m.mu.RLock()
	conn, ok := m.servers[name]
	m.mu.RUnlock()
	if ok && conn.ready && (conn.client == nil || !conn.client.IsClosed()) {
		return conn, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring the write lock.
	if conn, ok := m.servers[name]; ok && conn.ready && (conn.client == nil || !conn.client.IsClosed()) {
		return conn, nil
	}

	cfg, ok := m.configs[name]
	if !ok {
		return nil, i18n.NewError(i18n.KeyToolSourceSinkMCPNotConfigured, name)
	}
	if m.connectStartedForTest != nil {
		m.connectStartedForTest(m, name)
	}

	// H20: Wrap NewClient in a goroutine with timeout to prevent hanging
	// if MCP server doesn't respond.
	type clientResult struct {
		client *mcp.Client
		err    error
	}
	connCh := make(chan clientResult, 1)
	go func() {
		c, err := mcp.NewClient(name, cfg)
		connCh <- clientResult{c, err}
	}()

	connTimer := time.NewTimer(30 * time.Second)
	defer connTimer.Stop()
	var client *mcp.Client
	select {
	case res := <-connCh:
		if res.err != nil {
			return nil, i18n.WrapError(i18n.KeyToolSourceSinkMCPConnect, res.err, name)
		}
		client = res.client
	case <-connTimer.C:
		return nil, i18n.NewError(i18n.KeyToolSourceSinkMCPConnectTimeout, name)
	}

	mcpTools, err := client.ListTools()
	if err != nil {
		client.Close() //nolint:errcheck
		return nil, i18n.WrapError(i18n.KeyToolSourceSinkMCPListTools, err, name)
	}

	serverTools := make([]MCPServerTool, 0, len(mcpTools))
	for _, t := range mcpTools {
		serverTools = append(serverTools, MCPServerTool{
			Name:        t.OriginalName,
			Description: t.Description(),
		})
	}

	conn = &MCPServerConn{
		Config:       cfg,
		client:       client,
		tools:        serverTools,
		capabilities: client.ServerCapabilities(),
		ready:        true,
	}
	m.servers[name] = conn
	return conn, nil
}

// ServerNames returns a sorted list of configured server names.
func (m *MCPManager) ServerNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.configs))
	for n := range m.configs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Shutdown cleanly stops all connected MCP servers.
func (m *MCPManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, conn := range m.servers {
		if conn.ready {
			conn.client.Close() //nolint:errcheck
			conn.ready = false
		}
		delete(m.servers, name)
	}
}

// injectConn installs a pre-built connection — used in tests only.
func (m *MCPManager) injectConn(name string, conn *MCPServerConn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers[name] = conn
	m.configs[name] = conn.Config
}

// ── Backward-compatible helpers (used by tests and simple HTTP-mode servers) ──

// MCPServer is a simplified server description for HTTP-mode MCP servers
// (backward compat with the original test suite and simple integrations).
type MCPServer struct {
	Name    string
	BaseURL string
	Tools   []MCPServerTool
}

// AddServer registers a simplified HTTP-mode MCP server. This is a convenience
// wrapper around injectConn that creates an MCPServerConn with the BaseURL and
// optional cached tool list but no real process or jrpc2 client.
func (m *MCPManager) AddServer(s *MCPServer) {
	conn := &MCPServerConn{
		Config:       MCPServerConfig{Command: s.BaseURL}, // stash BaseURL in Command for HTTP fallback
		tools:        s.Tools,
		capabilities: svcmcp.ServerCapabilities{"resources": map[string]any{}},
		ready:        true,
	}
	conn.httpBaseURL = s.BaseURL
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers[s.Name] = conn
	m.configs[s.Name] = conn.Config
}

// GetServer retrieves a simplified server view by name.
func (m *MCPManager) GetServer(name string) (*MCPServer, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.servers[name]
	if !ok {
		return nil, false
	}
	return &MCPServer{
		Name:    name,
		BaseURL: conn.httpBaseURL,
		Tools:   conn.tools,
	}, true
}

func (m *MCPManager) PostCompactMCPServers() []compact.MCPServerSnapshot {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.servers))
	for name, conn := range m.servers {
		if conn != nil && conn.ready {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]compact.MCPServerSnapshot, 0, len(names))
	for _, name := range names {
		conn := m.servers[name]
		toolNames := make([]string, 0, len(conn.tools))
		for _, tool := range conn.tools {
			if strings.TrimSpace(tool.Name) != "" {
				toolNames = append(toolNames, tool.Name)
			}
		}
		sort.Strings(toolNames)
		out = append(out, compact.MCPServerSnapshot{
			Name:  name,
			Tools: toolNames,
		})
	}
	return out
}

// ─── MCPTool ──────────────────────────────────────────────────────────────────

// MCPTool executes a tool on a named MCP server via the real MCP protocol.
type MCPTool struct {
	manager *MCPManager
}

// NewMCPTool constructs an MCPTool backed by the given manager.
func NewMCPTool(manager *MCPManager) *MCPTool {
	return &MCPTool{manager: manager}
}

func (t *MCPTool) Name() string           { return "MCPTool" }
func (t *MCPTool) IsConcurrentSafe() bool { return true }
func (t *MCPTool) Description() string    { return "Execute a tool provided by an MCP server" }

func (t *MCPTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"server_name": map[string]any{
				"type":        "string",
				"description": "Name of the MCP server to call",
			},
			"tool_name": map[string]any{
				"type":        "string",
				"description": "Name of the tool to execute on the server",
			},
			"arguments": map[string]any{
				"type":        "object",
				"description": "Arguments to pass to the tool (optional)",
			},
		},
		Required: []string{"server_name", "tool_name"},
	}
}

func (t *MCPTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := parseInputOrError[MCPToolInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	if strings.TrimSpace(in.ServerName) == "" {
		return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolRuntimeMCPServerNameRequired), IsError: true}, nil
	}
	if strings.TrimSpace(in.ToolName) == "" {
		return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolRuntimeMCPToolNameRequired), IsError: true}, nil
	}

	conn, err := t.manager.Connect(in.ServerName)
	if err != nil {
		available := t.manager.ServerNames()
		return types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerUnavailable,
				in.ServerName, err, strings.Join(available, ", ")),
			IsError: true,
		}, nil
	}

	// Validate tool name against the discovered list when available.
	if len(conn.tools) > 0 {
		found := false
		for _, tool := range conn.tools {
			if tool.Name == in.ToolName {
				found = true
				break
			}
		}
		if !found {
			names := make([]string, len(conn.tools))
			for i, tool := range conn.tools {
				names[i] = tool.Name
			}
			return types.ToolResult{
				Content: toolRuntimeFormat(i18n.KeyToolRuntimeMCPToolNotFound,
					in.ToolName, in.ServerName, strings.Join(names, ", ")),
				IsError: true,
			}, nil
		}
	}

	args := in.Arguments
	if args == nil {
		args = map[string]any{}
	}

	// HTTP-mode fallback for servers registered via AddServer (no jrpc2 client).
	if conn.client == nil && conn.httpBaseURL != "" {
		return mcpHTTPCall(ctx, conn.httpBaseURL, in.ToolName, args)
	}

	if conn.client == nil {
		return types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolRuntimeMCPNoActiveConnection, in.ServerName),
			IsError: true,
		}, nil
	}

	// Audit P2-6: surface the structured tools/call envelope verbatim so
	// `content` / `isError` / mimeType survive across the tool layer. The
	// CallRaw access keeps uri + mimeType intact for the model.
	// MCP-03: cap the call duration so a misbehaving server can't hang
	// the agent forever.
	callCtx, callCancel := withMCPCallTimeout(ctx)
	defer callCancel()
	var raw json.RawMessage
	if err := conn.client.CallRaw(callCtx, "tools/call", map[string]any{
		"name":      in.ToolName,
		"arguments": args,
	}, &raw); err != nil {
		return types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolRuntimeMCPToolCallFailed, err),
			IsError: true,
		}, nil
	}
	if len(raw) == 0 {
		// i18n:allow display-literal protocol -- This is the canonical empty MCP tools/call response envelope.
		return types.ToolResult{Content: `{"content":[],"isError":false}`}, nil
	}
	return types.ToolResult{Content: renderMCPCallResult(raw)}, nil
}

// mcpHTTPCall is an HTTP-mode fallback for MCP servers registered via AddServer.
func mcpHTTPCall(ctx context.Context, baseURL, toolName string, args map[string]any) (types.ToolResult, error) {
	url := strings.TrimRight(baseURL, "/") + "/tools/" + toolName
	body, _ := json.Marshal(args)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeMCPRequestFailed, err), IsError: true}, nil
	}
	req.Header.Set("Content-Type", "application/json")
	// Audit P2-6: attach Bearer credentials via the configured TokenSource so
	// the HTTP-mode fallback can ride a refreshed OAuth token. Empty token
	// still surfaces "Bearer " so the wiring is observable.
	req.Header.Set("Authorization", resolveMCPBearer(ctx, baseURL))
	resp, err := sharedMCPHTTPClient.Do(req)
	if err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeMCPRequestFailed, err), IsError: true}, nil
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		// Audit P2-6: surface a structured "needs OAuth" hint rather than
		// the raw 401 prose so the model layer can route to the handshake.
		challenge := resp.Header.Get("WWW-Authenticate")
		hint, _ := json.Marshal(map[string]any{
			"error":            "oauth_required",
			"server_url":       baseURL,
			"www_authenticate": challenge,
			"status":           resp.StatusCode,
			"message":          "MCP server requires OAuth authorization; complete handshake before retrying",
		})
		return types.ToolResult{Content: string(hint), IsError: true}, nil
	}
	if resp.StatusCode >= 400 {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeMCPHTTPError, resp.StatusCode, string(respBody)), IsError: true}, nil
	}
	return types.ToolResult{Content: string(respBody)}, nil
}
