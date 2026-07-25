package manager

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
)

const (
	defaultLocalConnectionConcurrency  = 3
	defaultRemoteConnectionConcurrency = 20
)

// MCPConnectionState is the services-layer equivalent of the TypeScript
// MCPServerConnection union discriminator.
type MCPConnectionState string

const (
	MCPStatePending   MCPConnectionState = "pending"
	MCPStateConnected MCPConnectionState = "connected"
	MCPStateFailed    MCPConnectionState = "failed"
	MCPStateNeedsAuth MCPConnectionState = "needs-auth"
	MCPStateDisabled  MCPConnectionState = "disabled"
)

// MCPServerConnection is a snapshot of one server's connection state. When
// Type is connected, Client is the live protocol client; all other fields are
// safe, copy-returning metadata for tools, commands, and diagnostics.
type MCPServerConnection struct {
	Name       string                  `json:"name"`
	Type       MCPConnectionState      `json:"type"`
	Config     catalog.MCPServerConfig `json:"config"`
	ConfigHash string                  `json:"configHash,omitempty"`

	Client       *mcptransport.Client       `json:"-"`
	Capabilities catalog.ServerCapabilities `json:"capabilities,omitempty"`
	ServerInfo   *catalog.ServerInfo        `json:"serverInfo,omitempty"`
	Instructions string                     `json:"instructions,omitempty"`

	Tools     []catalog.ToolDefinition   `json:"tools,omitempty"`
	Resources []catalog.Resource         `json:"resources,omitempty"`
	Prompts   []catalog.PromptDefinition `json:"prompts,omitempty"`

	NeedsAuth *mcpauth.NeedsAuthState `json:"needsAuth,omitempty"`
	Error     string                  `json:"error,omitempty"`

	ReconnectAttempt     int `json:"reconnectAttempt,omitempty"`
	MaxReconnectAttempts int `json:"maxReconnectAttempts,omitempty"`
}

// transportBuildOptions carries shared manager dependencies into the transport
// factory without making individual transports import the manager.
type transportBuildOptions struct {
	TokenSource mcpauth.TokenSource
	CWD         string
}

type transportFactory func(ctx context.Context, name string, config catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error)

type connectFlight struct {
	done        chan struct{}
	configHash  string
	cancel      context.CancelFunc
	invalidated bool
}

// connectionCatalogPublication holds data fetched by one connection attempt
// until that attempt passes the manager's config/flight currentness fence.
// Keeping these values private prevents late workspace-A attempts from
// mutating the server-name-keyed caches before they are rejected.
type connectionCatalogPublication struct {
	serverName string
	client     *mcptransport.Client
	tools      *catalog.ListToolsResult
	resources  *catalog.ListResourcesResult
	prompts    *catalog.ListPromptsResult
}

// Manager owns MCP server configs, connection state, live clients, and fetched
// catalogues. It intentionally leaves model-facing tool registration to its
// callers.
type Manager struct {
	mu sync.Mutex

	cache *cache

	configs             map[string]catalog.MCPServerConfig
	nonWorkspaceConfigs map[string]catalog.MCPServerConfig
	disabled            map[string]bool
	states              map[string]MCPServerConnection
	inflight            map[string]*connectFlight

	reconnectPolicy reconnectPolicy
	reconnectTimers map[string]*reconnectTask
	reconnectRuns   map[*reconnectTask]struct{}

	needsAuthCache *mcpauth.NeedsAuthCache

	transportFactory transportFactory

	cwd string

	localConcurrency  int
	remoteConcurrency int

	catalogHooks       map[uint64]CatalogChangeHook
	catalogHookNext    uint64
	catalogDirty       bool
	catalogDispatching bool
}

// invalidateConnectFlightLocked revokes one in-flight connection without
// removing its rendezvous entry. Waiters remain attached until the stale
// owner finishes, at which point they retry against the current config. This
// prevents a target-workspace connection from racing the old flight through
// the server-name-keyed catalogue caches.
func (m *Manager) invalidateConnectFlightLocked(name string) {
	if m == nil {
		return
	}
	flight := m.inflight[name]
	if flight == nil {
		return
	}
	flight.invalidated = true
	if flight.cancel != nil {
		flight.cancel()
	}
}

// ManagerOption customizes production Manager behavior.
type ManagerOption func(*Manager)

// WithWorkingDirectory sets the cwd used for stdio transports.
func WithWorkingDirectory(cwd string) ManagerOption {
	return func(m *Manager) {
		m.cwd = cwd
	}
}

// NewManager constructs a services-layer MCP manager.
func NewManager(options ...ManagerOption) *Manager {
	m := &Manager{
		cache:               newCache(),
		configs:             make(map[string]catalog.MCPServerConfig),
		nonWorkspaceConfigs: make(map[string]catalog.MCPServerConfig),
		disabled:            make(map[string]bool),
		states:              make(map[string]MCPServerConnection),
		inflight:            make(map[string]*connectFlight),
		reconnectPolicy:     defaultReconnectPolicy(),
		reconnectTimers:     make(map[string]*reconnectTask),
		reconnectRuns:       make(map[*reconnectTask]struct{}),
		needsAuthCache:      mcpauth.NewNeedsAuthCache(0),
		transportFactory:    defaultTransportFactory,
		localConcurrency:    defaultLocalConnectionConcurrency,
		remoteConcurrency:   defaultRemoteConnectionConcurrency,
	}
	for _, option := range options {
		if option != nil {
			option(m)
		}
	}
	if m.cache == nil {
		m.cache = newCache()
	}
	m.cache.setOwner(m)
	if m.needsAuthCache == nil {
		m.needsAuthCache = mcpauth.NewNeedsAuthCache(0)
	}
	if m.transportFactory == nil {
		m.transportFactory = defaultTransportFactory
	}
	if m.localConcurrency <= 0 {
		m.localConcurrency = defaultLocalConnectionConcurrency
	}
	if m.remoteConcurrency <= 0 {
		m.remoteConcurrency = defaultRemoteConnectionConcurrency
	}
	return m
}

// LoadFromSettings reads project-scoped MCP server configs from a settings.json
// file. A missing file means that the project has no settings-backed servers.
// Fatal validation is atomic and blocks the load; warnings remain non-blocking.
func (m *Manager) LoadFromSettings(path string) error {
	if m == nil {
		return i18n.NewError(i18n.KeyServicesMCPNilManager)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return i18n.WrapError(i18n.KeyServicesMCPReadSettings, err, path)
	}
	parsed, err := catalog.ParseMCPConfig(data, catalog.ParseOptions{
		Scope:      catalog.ScopeProject,
		ExpandVars: true,
		FilePath:   path,
	})
	if err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPParseSettings, err, path)
	}
	if validation, ok := parsed.FirstFatalValidation(); ok {
		return &catalog.FatalConfigValidationError{Validation: validation}
	}
	for name, config := range parsed.Servers {
		m.AddConfig(name, config)
		if parsed.IsServerDisabled(name) {
			if _, err := m.ToggleEnabled(context.Background(), name, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// SetConfigs replaces the manager's config set. Removed servers are closed and
// evicted; changed hashes are reset to pending and lose connection/fetch cache.
func (m *Manager) SetConfigs(configs map[string]catalog.MCPServerConfig) {
	m.setConfigs(configs, true)
}

func (m *Manager) setConfigs(configs map[string]catalog.MCPServerConfig, replaceNonWorkspace bool) {
	if m == nil {
		return
	}
	next := make(map[string]catalog.MCPServerConfig, len(configs))
	nonWorkspace := make(map[string]catalog.MCPServerConfig)
	for name, config := range configs {
		normalized := normalizeManagerConfig(name, config)
		next[name] = normalized
		if !isWorkspaceConfigScope(normalized.Scope) {
			nonWorkspace[name] = normalized
		}
	}

	var closeClients []*mcptransport.Client
	m.mu.Lock()
	if replaceNonWorkspace {
		m.nonWorkspaceConfigs = nonWorkspace
	}
	removed := false
	for name, state := range m.states {
		if _, ok := next[name]; !ok {
			removed = true
			m.cancelReconnectLocked(name)
			m.invalidateConnectFlightLocked(name)
			if state.Client != nil {
				closeClients = append(closeClients, state.Client)
			}
			delete(m.states, name)
			delete(m.configs, name)
			delete(m.disabled, name)
			m.cache.clearServer(name)
		}
	}
	if removed {
		m.signalCatalogChangeLocked()
	}
	m.mu.Unlock()
	for _, client := range closeClients {
		_ = client.Close()
	}

	for name, config := range next {
		m.AddConfig(name, config)
	}
}

// ReplaceWorkspaceConfigs replaces project/local MCP configuration while
// preserving servers owned by user, managed, enterprise, and dynamic scopes.
// It is used when the foreground session moves to a different workspace.
func (m *Manager) ReplaceWorkspaceConfigs(configs map[string]catalog.MCPServerConfig) {
	if m == nil {
		return
	}
	var next map[string]catalog.MCPServerConfig
	var closeClients []*mcptransport.Client
	m.mu.Lock()
	next = make(map[string]catalog.MCPServerConfig, len(m.nonWorkspaceConfigs)+len(configs))
	for name, config := range m.nonWorkspaceConfigs {
		next[name] = config
	}
	removed := false
	for name, state := range m.states {
		oldConfig := m.configs[name]
		_, targetOwnsName := configs[name]
		if !isWorkspaceConfigScope(oldConfig.Scope) && !targetOwnsName {
			continue
		}
		removed = true
		m.cancelReconnectLocked(name)
		m.invalidateConnectFlightLocked(name)
		if state.Client != nil {
			closeClients = append(closeClients, state.Client)
		}
		delete(m.states, name)
		delete(m.configs, name)
		delete(m.disabled, name)
		m.cache.clearServer(name)
	}
	if removed {
		m.signalCatalogChangeLocked()
	}
	m.mu.Unlock()
	for _, client := range closeClients {
		_ = client.Close()
	}
	for name, config := range configs {
		next[name] = config
	}
	m.setConfigs(next, false)
}

// SetWorkingDirectory retargets future stdio transports without mutating the
// process-global working directory.
func (m *Manager) SetWorkingDirectory(cwd string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.cwd = strings.TrimSpace(cwd)
	m.mu.Unlock()
}

func isWorkspaceConfigScope(scope catalog.ConfigScope) bool {
	switch scope {
	case catalog.ScopeProject, catalog.ScopeLocal:
		return true
	default:
		return false
	}
}

// AddConfig registers or updates one server config without connecting.
func (m *Manager) AddConfig(name string, config catalog.MCPServerConfig) {
	if m == nil || strings.TrimSpace(name) == "" {
		return
	}
	config = normalizeManagerConfig(name, config)
	hash := catalog.HashMCPConfig(config)

	var closeClient *mcptransport.Client
	m.mu.Lock()
	oldConfig, hadConfig := m.configs[name]
	oldState := m.states[name]
	changed := hadConfig && catalog.HashMCPConfig(oldConfig) != hash
	if changed && oldState.Client != nil {
		closeClient = oldState.Client
	}
	if changed {
		m.cancelReconnectLocked(name)
		m.invalidateConnectFlightLocked(name)
	}
	m.configs[name] = config
	if !isWorkspaceConfigScope(config.Scope) {
		m.nonWorkspaceConfigs[name] = config
	}
	stateType := MCPStatePending
	if m.disabled[name] {
		stateType = MCPStateDisabled
	}
	if !hadConfig || changed || oldState.Type == "" {
		if changed {
			m.cache.clearServer(name)
		}
		m.setStateLocked(MCPServerConnection{
			Name:       name,
			Type:       stateType,
			Config:     config,
			ConfigHash: hash,
		})
	} else {
		oldState.Config = config
		oldState.ConfigHash = hash
		if m.disabled[name] {
			oldState.Type = MCPStateDisabled
			oldState.Client = nil
		}
		m.setStateLocked(oldState)
	}
	m.mu.Unlock()

	if closeClient != nil {
		_ = closeClient.Close()
	}
}

// ServerNames returns configured server names in stable order.
func (m *Manager) ServerNames() []string {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Snapshot returns sorted copies of all known server states.
func (m *Manager) Snapshot() []MCPServerConnection {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

// State returns a copy of one server state.
func (m *Manager) State(name string) (MCPServerConnection, bool) {
	if m == nil {
		return MCPServerConnection{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.states[name]
	if !ok {
		return MCPServerConnection{}, false
	}
	return cloneMCPServerConnection(state), true
}

// GetOrConnect returns a connected state when possible. Connection/auth
// failures are represented as failed or needs-auth states, matching the
// TypeScript connection manager.
func (m *Manager) GetOrConnect(ctx context.Context, name string) (MCPServerConnection, error) {
	if m == nil {
		return MCPServerConnection{}, i18n.NewError(i18n.KeyServicesMCPNilManager)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		m.mu.Lock()
		config, ok := m.configs[name]
		if !ok {
			m.mu.Unlock()
			return MCPServerConnection{}, i18n.NewError(i18n.KeyServicesMCPServerNotConfigured, name)
		}
		hash := catalog.HashMCPConfig(config)
		if m.disabled[name] {
			state := MCPServerConnection{Name: name, Type: MCPStateDisabled, Config: config, ConfigHash: hash}
			m.setStateLocked(state)
			m.mu.Unlock()
			return cloneMCPServerConnection(state), nil
		}
		if state, ok := m.states[name]; ok && state.Type == MCPStateConnected && state.Client != nil && state.ConfigHash == hash {
			out := cloneMCPServerConnection(state)
			m.mu.Unlock()
			return out, nil
		}
		if state, ok := m.states[name]; ok && state.Type == MCPStateNeedsAuth && state.ConfigHash == hash {
			if _, cached := mcpauth.LookupNeedsAuth(m.needsAuthCache, name, config.AuthDescriptor()); cached {
				out := cloneMCPServerConnection(state)
				m.mu.Unlock()
				return out, nil
			}
		}
		if flight := m.inflight[name]; flight != nil {
			done := flight.done
			m.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return MCPServerConnection{}, ctx.Err()
			}
		}
		connectCtx, cancel := context.WithCancel(ctx)
		flight := &connectFlight{done: make(chan struct{}), configHash: hash, cancel: cancel}
		m.inflight[name] = flight
		m.setStateLocked(MCPServerConnection{Name: name, Type: MCPStatePending, Config: config, ConfigHash: hash})
		m.mu.Unlock()

		result, publication := m.connect(connectCtx, name, config)
		cancel()
		var watchClient *mcptransport.Client
		if result.Type == MCPStateConnected {
			watchClient = result.Client
		}

		m.mu.Lock()
		currentConfig, configured := m.configs[name]
		currentFlight := m.inflight[name]
		stillCurrent := currentFlight == flight && !flight.invalidated && configured &&
			!m.disabled[name] && catalog.HashMCPConfig(currentConfig) == flight.configHash && currentConfig.Scope == config.Scope
		if !stillCurrent {
			if currentFlight == flight {
				delete(m.inflight, name)
			}
			close(flight.done)
			currentState, hasCurrentState := m.states[name]
			m.mu.Unlock()
			if watchClient != nil {
				_ = watchClient.Close()
			}
			if hasCurrentState {
				return cloneMCPServerConnection(currentState), nil
			}
			return MCPServerConnection{}, i18n.NewError(i18n.KeyServicesMCPServerNotConfigured, name)
		}
		m.publishConnectionCatalogLocked(publication)
		m.setStateLocked(result)
		delete(m.inflight, name)
		close(flight.done)
		m.mu.Unlock()
		if watchClient != nil {
			m.watchClientClosed(name, config, watchClient)
		}
		return cloneMCPServerConnection(result), nil
	}
}

// ConnectAll connects every configured enabled server, using separate local
// and remote concurrency ceilings.
func (m *Manager) ConnectAll(ctx context.Context) (map[string]MCPServerConnection, error) {
	if m == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilManager)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	names := m.ServerNames()
	local := make([]string, 0, len(names))
	remote := make([]string, 0, len(names))

	m.mu.Lock()
	for _, name := range names {
		config := m.configs[name]
		if m.disabled[name] {
			hash := catalog.HashMCPConfig(config)
			m.setStateLocked(MCPServerConnection{Name: name, Type: MCPStateDisabled, Config: config, ConfigHash: hash})
			continue
		}
		if isLocalManagerTransport(config) {
			local = append(local, name)
		} else {
			remote = append(remote, name)
		}
	}
	m.mu.Unlock()

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		errCh <- m.connectNames(ctx, local, m.localConcurrency)
	}()
	go func() {
		defer wg.Done()
		errCh <- m.connectNames(ctx, remote, m.remoteConcurrency)
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return m.snapshotMap(), err
		}
	}
	return m.snapshotMap(), nil
}

// Reconnect clears connection and fetch caches, closes the old client, and
// builds a fresh connection state.
func (m *Manager) Reconnect(ctx context.Context, name string) (MCPServerConnection, error) {
	if m == nil {
		return MCPServerConnection{}, i18n.NewError(i18n.KeyServicesMCPNilManager)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var oldClient *mcptransport.Client
	m.mu.Lock()
	config, ok := m.configs[name]
	if !ok {
		m.mu.Unlock()
		return MCPServerConnection{}, i18n.NewError(i18n.KeyServicesMCPServerNotConfigured, name)
	}
	if state := m.states[name]; state.Client != nil {
		oldClient = state.Client
	}
	m.cancelReconnectLocked(name)
	m.invalidateConnectFlightLocked(name)
	m.cache.clearServer(name)
	m.setStateLocked(MCPServerConnection{Name: name, Type: MCPStatePending, Config: config, ConfigHash: catalog.HashMCPConfig(config)})
	m.mu.Unlock()
	if oldClient != nil {
		_ = oldClient.Close()
	}
	return m.GetOrConnect(ctx, name)
}

// ToggleEnabled updates a server's enabled state. Disabling closes and clears
// caches; enabling moves to pending and immediately attempts a connection.
func (m *Manager) ToggleEnabled(ctx context.Context, name string, enabled bool) (MCPServerConnection, error) {
	if m == nil {
		return MCPServerConnection{}, i18n.NewError(i18n.KeyServicesMCPNilManager)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var oldClient *mcptransport.Client
	m.mu.Lock()
	config, ok := m.configs[name]
	if !ok {
		m.mu.Unlock()
		return MCPServerConnection{}, i18n.NewError(i18n.KeyServicesMCPServerNotConfigured, name)
	}
	hash := catalog.HashMCPConfig(config)
	if !enabled {
		m.cancelReconnectLocked(name)
		m.invalidateConnectFlightLocked(name)
		m.disabled[name] = true
		if state := m.states[name]; state.Client != nil {
			oldClient = state.Client
		}
		m.cache.clearServer(name)
		state := MCPServerConnection{Name: name, Type: MCPStateDisabled, Config: config, ConfigHash: hash}
		m.setStateLocked(state)
		m.mu.Unlock()
		if oldClient != nil {
			_ = oldClient.Close()
		}
		return cloneMCPServerConnection(state), nil
	}
	if !m.disabled[name] {
		m.mu.Unlock()
		return m.GetOrConnect(ctx, name)
	}
	delete(m.disabled, name)
	m.cancelReconnectLocked(name)
	m.setStateLocked(MCPServerConnection{Name: name, Type: MCPStatePending, Config: config, ConfigHash: hash})
	m.mu.Unlock()
	return m.GetOrConnect(ctx, name)
}

// Shutdown closes every live client and clears fetched catalogues. Configs
// remain registered as pending/disabled so callers can reconnect later.
func (m *Manager) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	clients := make([]*mcptransport.Client, 0)
	waiters := make([]<-chan struct{}, 0)
	m.mu.Lock()
	for task := range m.reconnectRuns {
		waiters = append(waiters, task.done)
	}
	m.cancelAllReconnectLocked()
	for name := range m.inflight {
		waiters = append(waiters, m.inflight[name].done)
		m.invalidateConnectFlightLocked(name)
	}
	for name, state := range m.states {
		if state.Client != nil {
			clients = append(clients, state.Client)
			state.Client = nil
		}
		state.Tools = nil
		state.Resources = nil
		state.Prompts = nil
		if state.Type == MCPStateConnected {
			state.Type = MCPStatePending
		}
		m.states[name] = state
		m.cache.clearServer(name)
	}
	m.signalCatalogChangeLocked()
	m.mu.Unlock()
	var shutdownErr error
	for _, client := range clients {
		if err := client.Close(); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	for _, done := range waiters {
		select {
		case <-done:
		case <-ctx.Done():
			return errors.Join(shutdownErr, ctx.Err())
		}
	}
	return shutdownErr
}

func (m *Manager) connect(ctx context.Context, name string, config catalog.MCPServerConfig) (MCPServerConnection, *connectionCatalogPublication) {
	hash := catalog.HashMCPConfig(config)
	if state, ok := mcpauth.LookupNeedsAuth(m.needsAuthCache, name, config.AuthDescriptor()); ok {
		stateCopy := state
		return MCPServerConnection{
			Name:       name,
			Type:       MCPStateNeedsAuth,
			Config:     config,
			ConfigHash: hash,
			NeedsAuth:  &stateCopy,
		}, nil
	}

	transport, err := m.transportFactory(ctx, name, config, transportBuildOptions{
		TokenSource: mcpauth.DefaultTokenSource(),
		CWD:         m.cwd,
	})
	if err != nil {
		return m.failureStateFromError(name, config, err), nil
	}
	transport = wrapSessionAwareTransport(name, transport)

	client, err := mcptransport.NewClient(ctx, transport)
	if err != nil {
		_ = transport.Close()
		return m.failureStateFromError(name, config, err), nil
	}

	state, catalog, err := m.connectedStateFromClient(ctx, name, config, client)
	if err != nil {
		_ = client.Close()
		return m.failureStateFromError(name, config, err), nil
	}
	return state, catalog
}

func (m *Manager) connectedStateFromClient(ctx context.Context, name string, config catalog.MCPServerConfig, client *mcptransport.Client) (MCPServerConnection, *connectionCatalogPublication, error) {
	hash := catalog.HashMCPConfig(config)
	state := MCPServerConnection{
		Name:         name,
		Type:         MCPStateConnected,
		Config:       config,
		ConfigHash:   hash,
		Client:       client,
		Capabilities: client.GetServerCapabilities(),
		ServerInfo:   client.GetServerInfo(),
		Instructions: client.GetInstructions(),
	}
	publication := &connectionCatalogPublication{serverName: name, client: client}
	if capabilityExists(state.Capabilities, "tools") {
		result, err := client.ListTools(ctx)
		if err != nil {
			return MCPServerConnection{}, nil, err
		}
		result = filterIDEListToolsResult(config, result)
		publication.tools = result
		state.Tools = append([]catalog.ToolDefinition(nil), catalog.CloneListToolsResult(*result).Tools...)
	}
	if capabilityExists(state.Capabilities, "resources") {
		result, err := client.ListResourcesResult(ctx)
		if err != nil {
			return MCPServerConnection{}, nil, err
		}
		publication.resources = result
		state.Resources = append([]catalog.Resource(nil), catalog.CloneListResourcesResult(*result).Resources...)
	}
	if capabilityExists(state.Capabilities, "prompts") {
		result, err := client.ListPrompts(ctx)
		if err != nil {
			return MCPServerConnection{}, nil, err
		}
		publication.prompts = result
		state.Prompts = append([]catalog.PromptDefinition(nil), catalog.CloneListPromptsResult(*result).Prompts...)
	}
	return state, publication, nil
}

// publishConnectionCatalogLocked installs one connection attempt's cache only
// after its workspace/config authority has been revalidated. Caller holds
// Manager.mu so the cache publication precedes the matching state hook.
func (m *Manager) publishConnectionCatalogLocked(catalog *connectionCatalogPublication) {
	if m == nil || catalog == nil || catalog.client == nil || catalog.serverName == "" {
		return
	}
	name := catalog.serverName
	installListChangedNotificationHandlers(m.cache, name, catalog.client)
	if catalog.tools != nil {
		m.cache.setTools(name, catalog.tools)
	}
	if catalog.resources != nil {
		m.cache.setResources(name, catalog.resources)
	}
	if catalog.prompts != nil {
		m.cache.setPrompts(name, catalog.prompts)
	}
}

func (m *Manager) failureStateFromError(name string, config catalog.MCPServerConfig, err error) MCPServerConnection {
	hash := catalog.HashMCPConfig(config)
	if needsAuth, ok := mcpauth.RecordNeedsAuthFromError(m.needsAuthCache, name, config.AuthDescriptor(), err); ok {
		stateCopy := needsAuth
		return MCPServerConnection{
			Name:       name,
			Type:       MCPStateNeedsAuth,
			Config:     config,
			ConfigHash: hash,
			NeedsAuth:  &stateCopy,
		}
	}
	return MCPServerConnection{
		Name:       name,
		Type:       MCPStateFailed,
		Config:     config,
		ConfigHash: hash,
		Error:      err.Error(),
	}
}

func (m *Manager) connectNames(ctx context.Context, names []string, concurrency int) error {
	if len(names) == 0 {
		return nil
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	errCh := make(chan error, len(names))
	var wg sync.WaitGroup
	for _, name := range names {
		name := name
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			_, err := m.GetOrConnect(ctx, name)
			if err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) snapshotLocked() []MCPServerConnection {
	out := make([]MCPServerConnection, 0, len(m.states))
	for _, state := range m.states {
		out = append(out, cloneMCPServerConnection(state))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (m *Manager) snapshotMap() map[string]MCPServerConnection {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]MCPServerConnection, len(m.states))
	for name, state := range m.states {
		out[name] = cloneMCPServerConnection(state)
	}
	return out
}

func (m *Manager) setStateLocked(state MCPServerConnection) {
	state = cloneMCPServerConnection(state)
	if state.ConfigHash == "" {
		state.ConfigHash = catalog.HashMCPConfig(state.Config)
	}
	m.states[state.Name] = state
	m.signalCatalogChangeLocked()
}

func defaultTransportFactory(ctx context.Context, _ string, config catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch config.Type {
	case "", catalog.TransportStdio:
		return mcptransport.NewStdioTransport(ctx, mcptransport.StdioConfig{
			Command: config.Command,
			Args:    append([]string(nil), config.Args...),
			Env:     cloneStringMap(config.Env),
			Dir:     opts.CWD,
		})
	case catalog.TransportHTTP:
		return mcptransport.NewHTTPTransport(mcptransport.HTTPTransportConfig{
			BaseURL: config.URL,
			Headers: cloneStringMap(config.Headers),
			Auth:    opts.TokenSource,
		})
	case catalog.TransportSSE:
		return mcptransport.NewSSETransport(mcptransport.SSEConfig{
			BaseURL: config.URL,
			Headers: cloneStringMap(config.Headers),
			Auth:    opts.TokenSource,
		})
	case catalog.TransportSSEIDE:
		return mcptransport.NewSSETransport(mcptransport.SSEConfig{
			BaseURL: config.URL,
			Headers: cloneStringMap(config.Headers),
		})
	case catalog.TransportWebSocket:
		return mcptransport.NewWebSocketTransport(ctx, mcptransport.WebSocketTransportConfig{
			URL:     config.URL,
			Headers: cloneStringMap(config.Headers),
			Auth:    opts.TokenSource,
		})
	case catalog.TransportWebSocketIDE:
		return mcptransport.NewWebSocketTransport(ctx, mcptransport.WebSocketTransportConfig{
			URL:     config.URL,
			Headers: cloneStringMap(config.Headers),
		})
	default:
		return nil, i18n.NewError(i18n.KeyMCPTransportUnsupported, config.Type)
	}
}

func normalizeManagerConfig(name string, config catalog.MCPServerConfig) catalog.MCPServerConfig {
	if config.Type == "" {
		config.Type = catalog.TransportStdio
	}
	if config.Type == catalog.TransportStdio && config.Args == nil {
		config.Args = []string{}
	}
	return config
}

func isLocalManagerTransport(config catalog.MCPServerConfig) bool {
	return config.Type == "" || config.Type == catalog.TransportStdio
}

func capabilityExists(caps catalog.ServerCapabilities, key string) bool {
	if caps == nil {
		return false
	}
	_, ok := caps[key]
	return ok
}

func cloneMCPServerConnection(in MCPServerConnection) MCPServerConnection {
	out := in
	out.Config = cloneMCPServerConfig(in.Config)
	out.Capabilities = catalog.CloneCapabilities(in.Capabilities)
	out.ServerInfo = catalog.CloneServerInfo(in.ServerInfo)
	if in.Tools != nil {
		result := catalog.CloneListToolsResult(catalog.ListToolsResult{Tools: in.Tools})
		out.Tools = result.Tools
	}
	if in.Resources != nil {
		result := catalog.CloneListResourcesResult(catalog.ListResourcesResult{Resources: in.Resources})
		out.Resources = result.Resources
	}
	if in.Prompts != nil {
		result := catalog.CloneListPromptsResult(catalog.ListPromptsResult{Prompts: in.Prompts})
		out.Prompts = result.Prompts
	}
	if in.NeedsAuth != nil {
		needsAuth := *in.NeedsAuth
		out.NeedsAuth = &needsAuth
	}
	return out
}

func cloneMCPServerConfig(in catalog.MCPServerConfig) catalog.MCPServerConfig {
	out := in
	out.Args = append([]string(nil), in.Args...)
	out.Env = cloneStringMap(in.Env)
	out.Headers = cloneStringMap(in.Headers)
	if in.OAuth != nil {
		oauth := *in.OAuth
		if in.OAuth.CallbackPort != nil {
			port := *in.OAuth.CallbackPort
			oauth.CallbackPort = &port
		}
		if in.OAuth.XAA != nil {
			xaa := *in.OAuth.XAA
			oauth.XAA = &xaa
		}
		out.OAuth = &oauth
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
