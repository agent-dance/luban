package mcp

import (
	"context"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

const (
	defaultLocalConnectionConcurrency  = 3
	defaultRemoteConnectionConcurrency = 20
	defaultManagerUserAgent            = "ClaudeCode/1.0"
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
	Name       string             `json:"name"`
	Type       MCPConnectionState `json:"type"`
	Config     MCPServerConfig    `json:"config"`
	ConfigHash string             `json:"configHash,omitempty"`

	Client       *Client            `json:"-"`
	Capabilities ServerCapabilities `json:"capabilities,omitempty"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
	Instructions string             `json:"instructions,omitempty"`

	Tools     []ToolDefinition   `json:"tools,omitempty"`
	Resources []Resource         `json:"resources,omitempty"`
	Prompts   []PromptDefinition `json:"prompts,omitempty"`

	NeedsAuth *NeedsAuthState `json:"needsAuth,omitempty"`
	Error     string          `json:"error,omitempty"`

	ReconnectAttempt     int `json:"reconnectAttempt,omitempty"`
	MaxReconnectAttempts int `json:"maxReconnectAttempts,omitempty"`
}

// TransportBuildOptions carries shared manager dependencies into the transport
// factory without making individual transports import the manager.
type TransportBuildOptions struct {
	TokenSource    TokenSource
	HeaderProvider HeaderProvider
	CWD            string
	UserAgent      string
	HTTPClient     *http.Client
}

// TransportFactory builds a transport for a named server config.
type TransportFactory func(ctx context.Context, name string, config MCPServerConfig, opts TransportBuildOptions) (Transport, error)

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
	key       string
	client    *Client
	tools     *ListToolsResult
	resources *ListResourcesResult
	prompts   *ListPromptsResult
}

// Manager owns MCP server configs, connection state, live clients, and fetched
// catalogues. It intentionally does not register model-facing tools; task_08
// consumes this service-layer state.
type Manager struct {
	mu sync.Mutex

	registry *Registry
	cache    *Cache

	configs             map[string]MCPServerConfig
	nonWorkspaceConfigs map[string]MCPServerConfig
	disabled            map[string]bool
	states              map[string]MCPServerConnection
	inflight            map[string]*connectFlight

	reconnectPolicy       ReconnectPolicy
	reconnectTimers       map[string]context.CancelFunc
	connectionLostHandler ConnectionLostListener

	needsAuthCache *NeedsAuthCache
	tokenSource    TokenSource
	headerProvider HeaderProvider
	httpClient     *http.Client

	transportFactory TransportFactory
	clientOptions    ClientOptions

	cwd       string
	userAgent string

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

// ManagerOption customizes Manager behavior. Tests generally inject a
// TransportFactory; production uses the default transport selector.
type ManagerOption func(*Manager)

// WithRegistry installs a shared registry for state snapshots.
func WithRegistry(registry *Registry) ManagerOption {
	return func(m *Manager) {
		if registry != nil {
			m.registry = registry
		}
	}
}

// WithCache installs a shared cache bundle.
func WithCache(cache *Cache) ManagerOption {
	return func(m *Manager) {
		if cache != nil {
			m.cache = cache
		}
	}
}

// WithNeedsAuthCache installs the OAuth/needs-auth cache used during remote
// connection attempts.
func WithNeedsAuthCache(cache *NeedsAuthCache) ManagerOption {
	return func(m *Manager) {
		if cache != nil {
			m.needsAuthCache = cache
		}
	}
}

// WithTokenSource installs the token resolver used by remote transports.
func WithTokenSource(source TokenSource) ManagerOption {
	return func(m *Manager) {
		if source != nil {
			m.tokenSource = source
		}
	}
}

// WithHeaderProvider installs the dynamic headers hook used by remote transports.
func WithHeaderProvider(provider HeaderProvider) ManagerOption {
	return func(m *Manager) {
		m.headerProvider = provider
	}
}

// WithHTTPClient installs the HTTP client used by remote HTTP/SSE transports.
func WithHTTPClient(client *http.Client) ManagerOption {
	return func(m *Manager) {
		m.httpClient = client
	}
}

// WithTransportFactory installs a test or specialized transport factory.
func WithTransportFactory(factory TransportFactory) ManagerOption {
	return func(m *Manager) {
		if factory != nil {
			m.transportFactory = factory
		}
	}
}

// WithClientOptions overlays protocol client options. Zero-valued fields use
// the same defaults as NewProtocolClient.
func WithClientOptions(options ClientOptions) ManagerOption {
	return func(m *Manager) {
		m.clientOptions = options
	}
}

// WithWorkingDirectory sets the cwd used for stdio transports.
func WithWorkingDirectory(cwd string) ManagerOption {
	return func(m *Manager) {
		m.cwd = cwd
	}
}

// WithUserAgent sets the User-Agent used by remote transports.
func WithUserAgent(userAgent string) ManagerOption {
	return func(m *Manager) {
		m.userAgent = userAgent
	}
}

// WithConnectionConcurrency sets TypeScript-parity local and remote batch
// limits. Non-positive values keep defaults.
func WithConnectionConcurrency(local, remote int) ManagerOption {
	return func(m *Manager) {
		if local > 0 {
			m.localConcurrency = local
		}
		if remote > 0 {
			m.remoteConcurrency = remote
		}
	}
}

// NewManager constructs a services-layer MCP manager.
func NewManager(options ...ManagerOption) *Manager {
	m := &Manager{
		registry:            NewRegistry(),
		cache:               NewCache(),
		configs:             make(map[string]MCPServerConfig),
		nonWorkspaceConfigs: make(map[string]MCPServerConfig),
		disabled:            make(map[string]bool),
		states:              make(map[string]MCPServerConnection),
		inflight:            make(map[string]*connectFlight),
		reconnectPolicy:     DefaultReconnectPolicy(),
		reconnectTimers:     make(map[string]context.CancelFunc),
		needsAuthCache:      DefaultNeedsAuthCache,
		tokenSource:         DefaultTokenSource(),
		transportFactory:    defaultTransportFactory,
		userAgent:           defaultManagerUserAgent,
		localConcurrency:    defaultLocalConnectionConcurrency,
		remoteConcurrency:   defaultRemoteConnectionConcurrency,
	}
	for _, option := range options {
		if option != nil {
			option(m)
		}
	}
	if m.registry == nil {
		m.registry = NewRegistry()
	}
	if m.cache == nil {
		m.cache = NewCache()
	}
	m.cache.setOwner(m)
	if m.needsAuthCache == nil {
		m.needsAuthCache = DefaultNeedsAuthCache
	}
	if m.tokenSource == nil {
		m.tokenSource = DefaultTokenSource()
	}
	if m.transportFactory == nil {
		m.transportFactory = defaultTransportFactory
	}
	if strings.TrimSpace(m.userAgent) == "" {
		m.userAgent = defaultManagerUserAgent
	}
	if m.localConcurrency <= 0 {
		m.localConcurrency = defaultLocalConnectionConcurrency
	}
	if m.remoteConcurrency <= 0 {
		m.remoteConcurrency = defaultRemoteConnectionConcurrency
	}
	return m
}

// Registry returns the manager-owned registry snapshot surface.
func (m *Manager) Registry() *Registry {
	if m == nil {
		return nil
	}
	return m.registry
}

// Cache returns the manager-owned cache bundle.
func (m *Manager) Cache() *Cache {
	if m == nil {
		return nil
	}
	return m.cache
}

// LoadFromSettings reads MCP server configs from a settings.json file. Missing
// files are ignored to match the legacy tools manager.
func (m *Manager) LoadFromSettings(path string) error {
	return m.LoadFromSettingsWithScope(path, ScopeProject)
}

// LoadFromSettingsWithScope reads MCP server configs from a settings.json file
// and merges them into the manager's config set.
func (m *Manager) LoadFromSettingsWithScope(path string, scope ConfigScope) error {
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
	parsed, err := ParseMCPConfig(data, ParseOptions{
		Scope:      scope,
		ExpandVars: true,
		FilePath:   path,
	})
	if err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPParseSettings, err, path)
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
func (m *Manager) SetConfigs(configs map[string]MCPServerConfig) {
	m.setConfigs(configs, true)
}

func (m *Manager) setConfigs(configs map[string]MCPServerConfig, replaceNonWorkspace bool) {
	if m == nil {
		return
	}
	next := make(map[string]MCPServerConfig, len(configs))
	nonWorkspace := make(map[string]MCPServerConfig)
	for name, config := range configs {
		normalized := normalizeManagerConfig(name, config)
		next[name] = normalized
		if !isWorkspaceConfigScope(normalized.Scope) {
			nonWorkspace[name] = normalized
		}
	}

	var closeClients []*Client
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
			m.cache.ClearServer(name)
			m.registry.Remove(name)
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
func (m *Manager) ReplaceWorkspaceConfigs(configs map[string]MCPServerConfig) {
	if m == nil {
		return
	}
	var next map[string]MCPServerConfig
	var closeClients []*Client
	m.mu.Lock()
	next = make(map[string]MCPServerConfig, len(m.nonWorkspaceConfigs)+len(configs))
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
		m.cache.ClearServer(name)
		m.registry.Remove(name)
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

func isWorkspaceConfigScope(scope ConfigScope) bool {
	switch scope {
	case ScopeProject, ScopeLocal:
		return true
	default:
		return false
	}
}

// AddConfig registers or updates one server config without connecting.
func (m *Manager) AddConfig(name string, config MCPServerConfig) {
	if m == nil || strings.TrimSpace(name) == "" {
		return
	}
	config = normalizeManagerConfig(name, config)
	hash := HashMCPConfig(config)

	var closeClient *Client
	m.mu.Lock()
	oldConfig, hadConfig := m.configs[name]
	oldState := m.states[name]
	changed := hadConfig && HashMCPConfig(oldConfig) != hash
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
			m.cache.ClearServer(name)
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
		hash := HashMCPConfig(config)
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
			if _, cached := HasMCPNeedsAuth(m.needsAuthCache, name, config); cached {
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

		result, catalog := m.connect(connectCtx, name, config)
		cancel()
		var watchClient *Client
		if result.Type == MCPStateConnected {
			watchClient = result.Client
		}

		m.mu.Lock()
		currentConfig, configured := m.configs[name]
		currentFlight := m.inflight[name]
		stillCurrent := currentFlight == flight && !flight.invalidated && configured &&
			!m.disabled[name] && HashMCPConfig(currentConfig) == flight.configHash && currentConfig.Scope == config.Scope
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
		m.publishConnectionCatalogLocked(catalog)
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
			hash := HashMCPConfig(config)
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
	var oldClient *Client
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
	m.cache.ClearServer(name)
	m.setStateLocked(MCPServerConnection{Name: name, Type: MCPStatePending, Config: config, ConfigHash: HashMCPConfig(config)})
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
	var oldClient *Client
	m.mu.Lock()
	config, ok := m.configs[name]
	if !ok {
		m.mu.Unlock()
		return MCPServerConnection{}, i18n.NewError(i18n.KeyServicesMCPServerNotConfigured, name)
	}
	hash := HashMCPConfig(config)
	if !enabled {
		m.cancelReconnectLocked(name)
		m.invalidateConnectFlightLocked(name)
		m.disabled[name] = true
		if state := m.states[name]; state.Client != nil {
			oldClient = state.Client
		}
		m.cache.ClearServer(name)
		state := MCPServerConnection{Name: name, Type: MCPStateDisabled, Config: config, ConfigHash: hash}
		m.setStateLocked(state)
		m.mu.Unlock()
		if oldClient != nil {
			_ = oldClient.Close()
		}
		return cloneMCPServerConnection(state), nil
	}
	delete(m.disabled, name)
	m.cancelReconnectLocked(name)
	m.setStateLocked(MCPServerConnection{Name: name, Type: MCPStatePending, Config: config, ConfigHash: hash})
	m.mu.Unlock()
	return m.GetOrConnect(ctx, name)
}

// ClearServerCache removes one server's connection and fetch cache. Connected
// clients are closed when present.
func (m *Manager) ClearServerCache(name string) {
	if m == nil || name == "" {
		return
	}
	var oldClient *Client
	m.mu.Lock()
	m.cancelReconnectLocked(name)
	m.invalidateConnectFlightLocked(name)
	if state := m.states[name]; state.Client != nil {
		oldClient = state.Client
		state.Client = nil
		if state.Type == MCPStateConnected {
			state.Type = MCPStatePending
		}
		state.Tools = nil
		state.Resources = nil
		state.Prompts = nil
		m.setStateLocked(state)
	}
	m.cache.ClearServer(name)
	m.mu.Unlock()
	if oldClient != nil {
		_ = oldClient.Close()
	}
}

// Shutdown closes every live client and clears connection cache. Configs remain
// registered as pending/disabled so callers can reconnect later.
func (m *Manager) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	clients := make([]*Client, 0)
	m.mu.Lock()
	m.cancelAllReconnectLocked()
	for name := range m.inflight {
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
		m.registry.UpsertConnection(state)
		m.cache.ClearServer(name)
	}
	m.signalCatalogChangeLocked()
	m.mu.Unlock()
	for _, client := range clients {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := client.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) connect(ctx context.Context, name string, config MCPServerConfig) (MCPServerConnection, *connectionCatalogPublication) {
	hash := HashMCPConfig(config)
	if state, ok := HasMCPNeedsAuth(m.needsAuthCache, name, config); ok {
		stateCopy := state
		return MCPServerConnection{
			Name:       name,
			Type:       MCPStateNeedsAuth,
			Config:     config,
			ConfigHash: hash,
			NeedsAuth:  &stateCopy,
		}, nil
	}

	key := ServerCacheKey(name, config)
	if cached, ok := m.cache.connection(key); ok && cached != nil && cached.IsInitialized() {
		return m.connectedStateFromClient(ctx, name, config, cached)
	}

	transport, err := m.transportFactory(ctx, name, config, TransportBuildOptions{
		TokenSource:    NeedsAuthTokenSource{Base: m.tokenSource, Cache: m.needsAuthCache},
		HeaderProvider: m.headerProvider,
		CWD:            m.cwd,
		UserAgent:      m.userAgent,
		HTTPClient:     m.httpClient,
	})
	if err != nil {
		return m.failureStateFromError(name, config, err), nil
	}
	transport = wrapSessionAwareTransport(name, transport)

	client, err := NewProtocolClient(ctx, transport, m.clientOptions)
	if err != nil {
		_ = transport.Close()
		return m.failureStateFromError(name, config, err), nil
	}

	return m.connectedStateFromClient(ctx, name, config, client)
}

func (m *Manager) connectedStateFromClient(ctx context.Context, name string, config MCPServerConfig, client *Client) (MCPServerConnection, *connectionCatalogPublication) {
	hash := HashMCPConfig(config)
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
	catalog := &connectionCatalogPublication{key: ServerCacheKey(name, config), client: client}
	if capabilityExists(state.Capabilities, "tools") {
		if result, err := client.ListTools(ctx); err == nil {
			result = FilterIDEListToolsResult(config, result)
			catalog.tools = result
			state.Tools = append([]ToolDefinition(nil), cloneListToolsResult(*result).Tools...)
		}
	}
	if capabilityExists(state.Capabilities, "resources") {
		if result, err := client.ListResourcesResult(ctx); err == nil {
			catalog.resources = result
			state.Resources = append([]Resource(nil), cloneListResourcesResult(*result).Resources...)
		}
	}
	if capabilityExists(state.Capabilities, "prompts") {
		if result, err := client.ListPrompts(ctx); err == nil {
			catalog.prompts = result
			state.Prompts = append([]PromptDefinition(nil), cloneListPromptsResult(*result).Prompts...)
		}
	}
	return state, catalog
}

// publishConnectionCatalogLocked installs one connection attempt's cache only
// after its workspace/config authority has been revalidated. Caller holds
// Manager.mu so the cache publication precedes the matching state hook.
func (m *Manager) publishConnectionCatalogLocked(catalog *connectionCatalogPublication) {
	if m == nil || catalog == nil || catalog.client == nil || catalog.key == "" {
		return
	}
	name := serverNameFromCacheKey(catalog.key)
	m.cache.setConnection(catalog.key, catalog.client)
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

func (m *Manager) failureStateFromError(name string, config MCPServerConfig, err error) MCPServerConnection {
	hash := HashMCPConfig(config)
	if needsAuth, ok := RecordNeedsAuthFromError(m.needsAuthCache, name, config, err); ok {
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
		state.ConfigHash = HashMCPConfig(state.Config)
	}
	m.states[state.Name] = state
	if m.registry != nil {
		m.registry.UpsertConnection(state)
	}
	m.signalCatalogChangeLocked()
}

func defaultTransportFactory(ctx context.Context, name string, config MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch config.Type {
	case "", TransportStdio:
		if transport, handled, err := MaybeNewInProcessTransport(ctx, name, config); handled || err != nil {
			return transport, err
		}
		return NewStdioTransport(ctx, StdioConfig{
			Command: config.Command,
			Args:    append([]string(nil), config.Args...),
			Env:     cloneStringMap(config.Env),
			Dir:     opts.CWD,
		})
	case TransportHTTP:
		return NewHTTPTransport(HTTPTransportConfig{
			BaseURL:        config.URL,
			HTTPClient:     opts.HTTPClient,
			Headers:        cloneStringMap(config.Headers),
			HeaderProvider: opts.HeaderProvider,
			Auth:           opts.TokenSource,
			ServerName:     name,
			UserAgent:      opts.UserAgent,
		})
	case TransportClaudeAIProxy:
		return NewClaudeAIProxyTransport(ctx, ClaudeAIProxyTransportConfig{
			ServerName:     name,
			ServerID:       config.ID,
			URL:            config.URL,
			HTTPClient:     opts.HTTPClient,
			Headers:        cloneStringMap(config.Headers),
			HeaderProvider: opts.HeaderProvider,
			Auth:           opts.TokenSource,
			UserAgent:      opts.UserAgent,
			SessionID:      claudeAIProxySessionIDFromEnv(),
		})
	case TransportSSE:
		return NewSSETransport(SSEConfig{
			BaseURL:        config.URL,
			HTTPClient:     opts.HTTPClient,
			Headers:        cloneStringMap(config.Headers),
			HeaderProvider: opts.HeaderProvider,
			Auth:           opts.TokenSource,
			ServerName:     name,
			UserAgent:      opts.UserAgent,
		})
	case TransportSSEIDE, TransportWebSocketIDE:
		return NewIDETransport(ctx, name, config, opts)
	case TransportWebSocket:
		return NewWebSocketTransport(ctx, WebSocketTransportConfig{
			URL:            config.URL,
			Headers:        cloneStringMap(config.Headers),
			HeaderProvider: opts.HeaderProvider,
			Auth:           opts.TokenSource,
			ServerName:     name,
			UserAgent:      opts.UserAgent,
		})
	case TransportSDK:
		return NewSDKTransport(name, config)
	default:
		return nil, i18n.NewError(i18n.KeyMCPTransportUnsupported, config.Type)
	}
}

func normalizeManagerConfig(name string, config MCPServerConfig) MCPServerConfig {
	if config.Type == "" {
		config.Type = TransportStdio
	}
	if config.Type == TransportStdio && config.Args == nil {
		config.Args = []string{}
	}
	if config.Name == "" {
		config.Name = name
	}
	return config
}

func isLocalManagerTransport(config MCPServerConfig) bool {
	return config.Type == "" || config.Type == TransportStdio || config.Type == TransportSDK
}

func capabilityExists(caps ServerCapabilities, key string) bool {
	if caps == nil {
		return false
	}
	_, ok := caps[key]
	return ok
}

func cloneMCPServerConnection(in MCPServerConnection) MCPServerConnection {
	out := in
	out.Config = cloneMCPServerConfig(in.Config)
	out.Capabilities = cloneCapabilities(in.Capabilities)
	out.ServerInfo = cloneServerInfo(in.ServerInfo)
	if in.Tools != nil {
		result := cloneListToolsResult(ListToolsResult{Tools: in.Tools})
		out.Tools = result.Tools
	}
	if in.Resources != nil {
		result := cloneListResourcesResult(ListResourcesResult{Resources: in.Resources})
		out.Resources = result.Resources
	}
	if in.Prompts != nil {
		result := cloneListPromptsResult(ListPromptsResult{Prompts: in.Prompts})
		out.Prompts = result.Prompts
	}
	if in.NeedsAuth != nil {
		needsAuth := *in.NeedsAuth
		out.NeedsAuth = &needsAuth
	}
	return out
}

func cloneMCPServerConfig(in MCPServerConfig) MCPServerConfig {
	out := in
	out.Args = append([]string(nil), in.Args...)
	out.Env = cloneStringMap(in.Env)
	out.Headers = cloneStringMap(in.Headers)
	out.Unknown = cloneMap(in.Unknown)
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
