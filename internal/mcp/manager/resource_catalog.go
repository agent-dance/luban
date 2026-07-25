package manager

import (
	"context"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
)

// ListResources returns the current server's cached resource catalogue or
// fetches and atomically publishes it. If authority changes during I/O, the
// stale result is discarded and the operation retries against the live client.
func (m *Manager) ListResources(ctx context.Context, name string) (catalog.ListResourcesResult, error) {
	if m == nil || m.cache == nil {
		return catalog.ListResourcesResult{}, i18n.NewError(i18n.KeyServicesMCPNilManager)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return catalog.ListResourcesResult{}, err
		}

		m.mu.Lock()
		state, ok := m.states[name]
		if !ok {
			m.mu.Unlock()
			return catalog.ListResourcesResult{}, i18n.NewError(i18n.KeyServicesMCPServerNotConfigured, name)
		}
		if state.Type != MCPStateConnected || state.Client == nil {
			m.mu.Unlock()
			return catalog.ListResourcesResult{}, mcptransport.NewTransportClosedError(i18n.KeyServicesMCPTransportClientClosedReason, mcptransport.ErrTransportClosed)
		}
		client := state.Client
		if _, current := m.currentListChangedStateLocked(ListChangedEvent{ServerName: name, client: client}); !current {
			m.mu.Unlock()
			return catalog.ListResourcesResult{}, mcptransport.NewTransportClosedError(i18n.KeyServicesMCPTransportClientClosedReason, mcptransport.ErrTransportClosed)
		}
		m.cache.mu.RLock()
		cached, cachedOK := m.cache.resources[name]
		if cachedOK {
			cached = catalog.CloneListResourcesResult(cached)
		}
		m.cache.mu.RUnlock()
		m.mu.Unlock()
		if cachedOK {
			return cached, nil
		}

		result, err := client.ListResourcesResult(ctx)
		if err != nil {
			return catalog.ListResourcesResult{}, err
		}
		if m.storeResources(name, client, result) {
			return catalog.CloneListResourcesResult(*result), nil
		}
	}
}

// storeResources atomically publishes a resources/list result to the cache and
// authoritative connection state. Results from a replaced client are rejected.
func (m *Manager) storeResources(name string, client *mcptransport.Client, result *catalog.ListResourcesResult) bool {
	if m == nil || m.cache == nil || name == "" || client == nil || result == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.currentListChangedStateLocked(ListChangedEvent{ServerName: name, client: client})
	if !ok {
		return false
	}

	cloned := catalog.CloneListResourcesResult(*result)
	m.cache.mu.Lock()
	defer m.cache.mu.Unlock()
	if m.cache.resources == nil {
		m.cache.resources = make(map[string]catalog.ListResourcesResult)
	}
	m.cache.resources[name] = cloned
	state.Resources = catalog.CloneListResourcesResult(cloned).Resources
	m.setStateLocked(state)
	return true
}
