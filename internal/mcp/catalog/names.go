package catalog

import (
	"regexp"
)

var invalidMCPNameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// normalizeNameForMCP converts a display name into the canonical MCP tool-name
// segment used by this client.
func normalizeNameForMCP(name string) string {
	return invalidMCPNameChars.ReplaceAllString(name, "_")
}

func getMCPPrefix(serverName string) string {
	return "mcp__" + normalizeNameForMCP(serverName) + "__"
}

// BuildMCPToolName builds mcp__<server>__<tool> with canonical normalization.
func BuildMCPToolName(serverName, toolName string) string {
	return getMCPPrefix(serverName) + normalizeNameForMCP(toolName)
}
