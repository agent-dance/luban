package registry

import "github.com/agent-dance/luban/types"

// MCPDynamicRegistration identifies model-facing tools generated from live MCP
// manager state. The registry uses this marker to replace stale dynamic MCP
// entries without touching unrelated mcp__ tools from other subsystems.
type MCPDynamicRegistration struct {
	ServerName string
	ToolName   string
	ModelName  string
	Kind       string
}

type MCPDynamicRegisteredTool interface {
	MCPDynamicRegistration() MCPDynamicRegistration
}

// SyncMCPDynamicTools atomically replaces previously registered dynamic MCP
// tools with next. Resource tools and non-MCP mcp__ commands are left alone.
func (r *Registry) SyncMCPDynamicTools(next []types.Tool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	keptOrder := make([]string, 0, len(r.order)+len(next))
	for _, name := range r.order {
		tool := r.tools[name]
		if isMCPDynamicRegistered(tool) {
			delete(r.tools, name)
			continue
		}
		keptOrder = append(keptOrder, name)
	}
	r.order = keptOrder

	for _, tool := range next {
		if tool == nil || tool.Name() == "" {
			continue
		}
		if existing, exists := r.tools[tool.Name()]; exists {
			if !isMCPDynamicRegistered(existing) {
				continue
			}
			r.tools[tool.Name()] = tool
			continue
		}
		r.tools[tool.Name()] = tool
		r.order = append(r.order, tool.Name())
	}
}

// RemoveMCPDynamicToolsForServer unregisters generated MCP tools for one
// original server name. It returns the number of removed entries.
func (r *Registry) RemoveMCPDynamicToolsForServer(serverName string) int {
	if r == nil || serverName == "" {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := 0
	keptOrder := make([]string, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]
		registration, ok := tool.(MCPDynamicRegisteredTool)
		if ok && registration.MCPDynamicRegistration().ServerName == serverName {
			delete(r.tools, name)
			removed++
			continue
		}
		keptOrder = append(keptOrder, name)
	}
	r.order = keptOrder
	return removed
}

func isMCPDynamicRegistered(tool types.Tool) bool {
	_, ok := tool.(MCPDynamicRegisteredTool)
	return ok
}
