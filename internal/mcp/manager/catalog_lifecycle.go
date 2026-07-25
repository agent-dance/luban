package manager

import "github.com/agent-dance/luban/internal/mcp/catalog"

// CatalogChangeHook observes a successful connection/catalog state mutation.
// Hooks run outside Manager.mu and may safely call Snapshot or perform MCP I/O.
// Bursts are coalesced; consumers must treat the manager snapshot as the
// authority instead of interpreting the notification as an ordered delta.
type CatalogChangeHook func()

// RegisterCatalogChangeHook installs one manager-scoped lifecycle observer.
// The returned function is idempotent and removes only this registration.
func (m *Manager) RegisterCatalogChangeHook(hook CatalogChangeHook) func() {
	if m == nil || hook == nil {
		return func() {}
	}
	m.mu.Lock()
	if m.catalogHooks == nil {
		m.catalogHooks = make(map[uint64]CatalogChangeHook)
	}
	m.catalogHookNext++
	id := m.catalogHookNext
	m.catalogHooks[id] = hook
	m.mu.Unlock()

	var removed bool
	return func() {
		m.mu.Lock()
		if !removed {
			delete(m.catalogHooks, id)
			removed = true
		}
		m.mu.Unlock()
	}
}

// signalCatalogChangeLocked marks the projection dirty and starts at most one
// dispatcher. Caller holds m.mu.
func (m *Manager) signalCatalogChangeLocked() {
	if m == nil || len(m.catalogHooks) == 0 {
		return
	}
	m.catalogDirty = true
	if m.catalogDispatching {
		return
	}
	m.catalogDispatching = true
	go m.dispatchCatalogChanges()
}

func (m *Manager) dispatchCatalogChanges() {
	for {
		m.mu.Lock()
		if !m.catalogDirty {
			m.catalogDispatching = false
			m.mu.Unlock()
			return
		}
		m.catalogDirty = false
		hooks := make([]CatalogChangeHook, 0, len(m.catalogHooks))
		for _, hook := range m.catalogHooks {
			hooks = append(hooks, hook)
		}
		m.mu.Unlock()

		for _, hook := range hooks {
			func() {
				defer func() { _ = recover() }()
				hook()
			}()
		}
	}
}

func (m *Manager) isCurrentListChangedEvent(event ListChangedEvent) bool {
	if m == nil || event.client == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.currentListChangedStateLocked(event)
	return ok
}

func (m *Manager) currentListChangedStateLocked(event ListChangedEvent) (MCPServerConnection, bool) {
	state, ok := m.states[event.ServerName]
	if !ok || state.Type != MCPStateConnected || state.Client != event.client {
		return MCPServerConnection{}, false
	}
	config, configured := m.configs[event.ServerName]
	if !configured || m.disabled[event.ServerName] || state.ConfigHash != catalog.HashMCPConfig(config) || state.Config.Scope != config.Scope {
		return MCPServerConnection{}, false
	}
	return state, true
}

// applyListChangedEvent publishes a staged result only when it belongs to the
// still-current client and configuration. Manager and cache locks remain held
// through both writes, so readers cannot observe a resurrected partial update.
func (m *Manager) applyListChangedEvent(event ListChangedEvent) bool {
	if m == nil || event.Err != nil || event.client == nil || m.cache == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.currentListChangedStateLocked(event)
	if !ok {
		return false
	}
	m.cache.mu.Lock()
	defer m.cache.mu.Unlock()
	if !m.cache.publishListChangedEventLocked(event) {
		return false
	}
	switch event.Kind {
	case ListChangedTools:
		state.Tools = append([]catalog.ToolDefinition(nil), event.Tools...)
	case ListChangedResources:
		state.Resources = append([]catalog.Resource(nil), event.Resources...)
	case ListChangedPrompts:
		state.Prompts = append([]catalog.PromptDefinition(nil), event.Prompts...)
	default:
		return false
	}
	m.setStateLocked(state)
	return true
}
