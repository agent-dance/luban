package manager

import "github.com/agent-dance/luban/internal/mcp/catalog"

var allowedIDERawTools = map[string]struct{}{
	"executeCode":    {},
	"getDiagnostics": {},
}

// filterIDEListToolsResult preserves the TS IDE allow-list. The TS layer
// filters fully-qualified mcp__ide__* tools; at the service layer we keep only
// the raw tools that will become those names.
func filterIDEListToolsResult(config catalog.MCPServerConfig, result *catalog.ListToolsResult) *catalog.ListToolsResult {
	if result == nil || !isIDETransport(config.Type) {
		return result
	}
	filtered := catalog.CloneListToolsResult(*result)
	filtered.Tools = filtered.Tools[:0]
	for _, tool := range result.Tools {
		if _, ok := allowedIDERawTools[tool.Name]; ok {
			cloned := catalog.CloneListToolsResult(catalog.ListToolsResult{Tools: []catalog.ToolDefinition{tool}})
			filtered.Tools = append(filtered.Tools, cloned.Tools[0])
		}
	}
	return &filtered
}

// isIDETransport reports whether transport is one of the internal IDE MCP
// variants.
func isIDETransport(transport catalog.TransportType) bool {
	return transport == catalog.TransportSSEIDE || transport == catalog.TransportWebSocketIDE
}
