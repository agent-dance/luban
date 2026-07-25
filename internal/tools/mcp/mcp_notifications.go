package mcp

import (
	"sync"

	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
	"github.com/agent-dance/luban/registry"
)

// This file bridges MCP manager list_changed notifications into tool-facing
// registries and catalog invalidators.

// RegisterMCPListChangedInvalidators bridges manager list_changed events
// into the model-facing registries owned by the tools layer. It is intentionally
// narrow: the MCP manager owns protocol refresh and cache replacement; this hook
// swaps generated dynamic tools for the changed server and clears ToolSearch
// indexes so the next search sees the fresh registry.
type toolSearchInvalidator interface {
	Invalidate()
}

func RegisterMCPListChangedInvalidators(reg *registry.Registry, manager DynamicMCPManager, oauth *mcpauth.OAuthManager, searches ...toolSearchInvalidator) func() {
	// List-changed hooks are process-global, but the registry projection is
	// owned by one concrete production manager. Never let an event from another
	// manager rewrite this registry.
	capturedManager, managerScoped := manager.(*mcpmanager.Manager)

	// RegisterListChangedHook copies callbacks before invoking them. Removing a
	// hook from that global map therefore cannot, by itself, guarantee that
	// unregister has quiesced the registration. Keep the read lock for the full
	// callback so the write-side close both waits for in-flight mutations and
	// makes already-copied callbacks inert before it returns.
	var gate struct {
		sync.RWMutex
		closed bool
	}
	unregisterHook := mcpmanager.RegisterListChangedHook(func(event mcpmanager.ListChangedEvent) {
		gate.RLock()
		defer gate.RUnlock()
		if gate.closed {
			return
		}
		if !managerScoped || event.Manager != capturedManager {
			return
		}
		switch event.Kind {
		case mcpmanager.ListChangedTools:
			if event.Err == nil && reg != nil && manager != nil {
				reg.RemoveMCPDynamicToolsForServer(event.ServerName)
				for _, definition := range event.Tools {
					reg.Register(NewDynamicMCPTool(manager, event.ServerName, definition))
				}
			}
			invalidateToolSearches(searches)
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

func invalidateToolSearches(searches []toolSearchInvalidator) {
	for _, search := range searches {
		if search != nil {
			search.Invalidate()
		}
	}
}
