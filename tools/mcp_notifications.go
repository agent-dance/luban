package tools

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/agent-dance/luban/registry"
	svcmcp "github.com/agent-dance/luban/services/mcp"
)

// mcp_notifications.go — MCP-04 list_changed notification handler.
//
// MCP servers can emit `notifications/tools/list_changed` and
// `notifications/resources/list_changed` to signal that their
// advertised catalogues have shifted. Without subscribing to these,
// hot-added tools / resources stay invisible until the agent restarts.
//
// Strategy:
//
//   * MCPManager holds a per-server `notificationDispatcher` that
//     refreshes the cached `tools` slice on tools/list_changed and
//     fires the registered `ToolSearchInvalidator` so the tool-search
//     index is rebuilt on next use.
//   * resources/list_changed flips a flag that ListMcpResourcesTool can
//     check to skip its cache. Today we don't cache resource lists, so
//     the flag is informational.
//
// The dispatcher is intentionally lightweight: callers (typically the
// jrpc2 protocol layer) invoke HandleNotification(server, method,
// params) when a frame arrives.

// ToolSearchInvalidator is the callback fired when an MCP server's tool
// catalogue changes. The tool-search subsystem should rebuild its index
// (or at minimum its registry hash) on the next use.
type ToolSearchInvalidator func(serverName string)

// ResourceListInvalidator is the analogue for resources/list_changed.
type ResourceListInvalidator func(serverName string)

// PromptListInvalidator is fired for prompts/list_changed.
type PromptListInvalidator func(serverName string)

var (
	mcpNotificationMu        sync.RWMutex
	toolSearchInvalidator    ToolSearchInvalidator
	resourceListInvalidator  ResourceListInvalidator
	promptListInvalidator    PromptListInvalidator
	resourcesChangedFlag     map[string]bool
	resourcesChangedFlagInit sync.Once
)

// SetToolSearchInvalidator registers the callback fired when a server
// emits notifications/tools/list_changed. Pass nil to clear.
func SetToolSearchInvalidator(fn ToolSearchInvalidator) {
	mcpNotificationMu.Lock()
	toolSearchInvalidator = fn
	mcpNotificationMu.Unlock()
}

// SetResourceListInvalidator registers the callback fired when a server
// emits notifications/resources/list_changed. Pass nil to clear.
func SetResourceListInvalidator(fn ResourceListInvalidator) {
	mcpNotificationMu.Lock()
	resourceListInvalidator = fn
	mcpNotificationMu.Unlock()
}

// SetPromptListInvalidator registers the callback fired when a server emits
// notifications/prompts/list_changed. Pass nil to clear.
func SetPromptListInvalidator(fn PromptListInvalidator) {
	mcpNotificationMu.Lock()
	promptListInvalidator = fn
	mcpNotificationMu.Unlock()
}

func ensureResourcesChangedMap() {
	resourcesChangedFlagInit.Do(func() {
		resourcesChangedFlag = make(map[string]bool)
	})
}

// ResourcesChanged reports whether `serverName` has emitted a
// resources/list_changed notification since the last call to
// ConsumeResourcesChanged. Useful for ListMcpResourcesTool to know
// whether to bypass any cached catalog.
func ResourcesChanged(serverName string) bool {
	mcpNotificationMu.RLock()
	defer mcpNotificationMu.RUnlock()
	ensureResourcesChangedMap()
	return resourcesChangedFlag[serverName]
}

// ConsumeResourcesChanged atomically reads-and-clears the resources-
// changed flag for `serverName`.
func ConsumeResourcesChanged(serverName string) bool {
	mcpNotificationMu.Lock()
	defer mcpNotificationMu.Unlock()
	ensureResourcesChangedMap()
	v := resourcesChangedFlag[serverName]
	if v {
		delete(resourcesChangedFlag, serverName)
	}
	return v
}

// HandleMCPNotification dispatches a JSON-RPC notification to the
// appropriate handler. method is the JSON-RPC method (e.g.
// "notifications/tools/list_changed"); params is the notification's raw
// JSON payload (currently unused but reserved for future fields).
//
// Unknown methods are silently ignored — the protocol layer is allowed
// to deliver every notification frame here without filtering.
func (m *MCPManager) HandleMCPNotification(ctx context.Context, serverName, method string, params json.RawMessage) {
	switch method {
	case svcmcp.NotificationToolsListChanged:
		m.refreshServerTools(ctx, serverName)
		fireToolSearchInvalidator(serverName)
	case svcmcp.NotificationResourcesListChanged:
		m.mu.RLock()
		lifecycle := m.lifecycle
		m.mu.RUnlock()
		if lifecycle != nil {
			_ = lifecycle.Publish(ctx, RuntimeLifecycleEvent{
				Type:     LifecycleMCPResourcesChanged,
				EntityID: serverName,
				ToolName: "ListMcpResourcesTool",
				Status:   "invalidated",
				Payload: map[string]any{
					"method": method,
					"params": params,
				},
			})
		}
		mcpNotificationMu.Lock()
		ensureResourcesChangedMap()
		resourcesChangedFlag[serverName] = true
		fn := resourceListInvalidator
		mcpNotificationMu.Unlock()
		if fn != nil {
			defer func() { _ = recover() }()
			fn(serverName)
		}
	case svcmcp.NotificationPromptsListChanged:
		mcpNotificationMu.RLock()
		fn := promptListInvalidator
		mcpNotificationMu.RUnlock()
		if fn != nil {
			defer func() { _ = recover() }()
			fn(serverName)
		}
	}
}

// RegisterMCPListChangedInvalidators bridges services/mcp list_changed events
// into the model-facing registries owned by the tools layer. It is intentionally
// narrow: services/mcp owns protocol refresh and cache replacement; this hook
// swaps generated dynamic tools for the changed server and clears ToolSearch
// indexes so the next search sees the fresh registry.
func RegisterMCPListChangedInvalidators(reg *registry.Registry, manager DynamicMCPManager, oauth *svcmcp.OAuthManager, searches ...*ToolSearchTool) func() {
	// List-changed hooks are process-global, but the registry projection is
	// owned by one concrete production manager. Never let an event from another
	// manager rewrite this registry. The nil/nil observer-only shape is retained
	// for legacy resource/prompt invalidators; it cannot mutate a registry.
	capturedManager, managerScoped := manager.(*svcmcp.Manager)
	observerOnly := reg == nil && manager == nil

	// RegisterListChangedHook copies callbacks before invoking them. Removing a
	// hook from that global map therefore cannot, by itself, guarantee that
	// unregister has quiesced the registration. Keep the read lock for the full
	// callback so the write-side close both waits for in-flight mutations and
	// makes already-copied callbacks inert before it returns.
	var gate struct {
		sync.RWMutex
		closed bool
	}
	unregisterHook := svcmcp.RegisterListChangedHook(func(event svcmcp.ListChangedEvent) {
		gate.RLock()
		defer gate.RUnlock()
		if gate.closed {
			return
		}
		if !observerOnly && (!managerScoped || event.Manager != capturedManager) {
			return
		}
		switch event.Kind {
		case svcmcp.ListChangedTools:
			if event.Err == nil && reg != nil && manager != nil {
				reg.RemoveMCPDynamicToolsForServer(event.ServerName)
				for _, definition := range event.Tools {
					reg.Register(NewDynamicMCPTool(manager, event.ServerName, definition))
				}
			}
			invalidateToolSearches(searches)
			fireToolSearchInvalidator(event.ServerName)
		case svcmcp.ListChangedResources:
			fireResourceListInvalidator(event.ServerName)
		case svcmcp.ListChangedPrompts:
			firePromptListInvalidator(event.ServerName)
		}
		_ = oauth
	})
	var unregisterOnce sync.Once
	return func() {
		unregisterOnce.Do(func() {
			gate.Lock()
			gate.closed = true
			gate.Unlock()
			unregisterHook()
		})
	}
}

func invalidateToolSearches(searches []*ToolSearchTool) {
	for _, search := range searches {
		if search != nil {
			search.Invalidate()
		}
	}
}

func fireToolSearchInvalidator(serverName string) {
	mcpNotificationMu.RLock()
	fn := toolSearchInvalidator
	mcpNotificationMu.RUnlock()
	if fn != nil {
		defer func() { _ = recover() }()
		fn(serverName)
	}
}

func fireResourceListInvalidator(serverName string) {
	mcpNotificationMu.RLock()
	fn := resourceListInvalidator
	mcpNotificationMu.RUnlock()
	if fn != nil {
		defer func() { _ = recover() }()
		fn(serverName)
	}
}

func firePromptListInvalidator(serverName string) {
	mcpNotificationMu.RLock()
	fn := promptListInvalidator
	mcpNotificationMu.RUnlock()
	if fn != nil {
		defer func() { _ = recover() }()
		fn(serverName)
	}
}

// refreshServerTools re-runs ListTools against the named server and
// updates the cached `tools` slice on its connection. Errors are
// swallowed — a transient list failure shouldn't tear down the
// connection; the next list_changed notification will trigger another
// retry.
func (m *MCPManager) refreshServerTools(ctx context.Context, name string) {
	m.mu.Lock()
	conn, ok := m.servers[name]
	m.mu.Unlock()
	if !ok || conn == nil || conn.client == nil {
		return
	}
	tools, err := conn.client.ListTools()
	if err != nil {
		return
	}
	updated := make([]MCPServerTool, 0, len(tools))
	for _, t := range tools {
		updated = append(updated, MCPServerTool{
			Name:        t.OriginalName,
			Description: t.Description(),
		})
	}
	m.mu.Lock()
	conn.tools = updated
	m.mu.Unlock()
	_ = ctx
}
