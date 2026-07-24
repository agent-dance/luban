package mcp

import (
	"fmt"
	"regexp"
	"strings"
)

const claudeAIServerPrefix = "claude.ai "

var (
	invalidMCPNameChars  = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	consecutiveUnderline = regexp.MustCompile(`_+`)
)

// NormalizeNameForMCP mirrors TypeScript normalizeNameForMCP.
func NormalizeNameForMCP(name string) string {
	normalized := invalidMCPNameChars.ReplaceAllString(name, "_")
	if strings.HasPrefix(name, claudeAIServerPrefix) {
		normalized = consecutiveUnderline.ReplaceAllString(normalized, "_")
		normalized = strings.Trim(normalized, "_")
	}
	return normalized
}

// MCPInfo is parsed from mcp__<server>__<tool> names.
type MCPInfo struct {
	ServerName string
	ToolName   *string
}

// MCPInfoFromString mirrors TypeScript mcpInfoFromString.
func MCPInfoFromString(toolString string) (MCPInfo, bool) {
	parts := strings.Split(toolString, "__")
	if len(parts) < 2 || parts[0] != "mcp" || parts[1] == "" {
		return MCPInfo{}, false
	}
	info := MCPInfo{ServerName: parts[1]}
	if len(parts) > 2 {
		toolName := strings.Join(parts[2:], "__")
		info.ToolName = &toolName
	}
	return info, true
}

// GetMCPPrefix returns the canonical MCP tool prefix for a server.
func GetMCPPrefix(serverName string) string {
	return "mcp__" + NormalizeNameForMCP(serverName) + "__"
}

// BuildMCPToolName builds mcp__<server>__<tool> with TS-compatible normalization.
func BuildMCPToolName(serverName, toolName string) string {
	return GetMCPPrefix(serverName) + NormalizeNameForMCP(toolName)
}

// UniqueClaudeAIServerName mirrors the claude.ai connector naming loop in TS.
// usedNormalizedNames is updated with the selected normalized name.
func UniqueClaudeAIServerName(displayName string, usedNormalizedNames map[string]struct{}) string {
	baseName := claudeAIServerPrefix + displayName
	finalName := baseName
	finalNormalized := NormalizeNameForMCP(finalName)
	count := 1
	for {
		if _, exists := usedNormalizedNames[finalNormalized]; !exists {
			if usedNormalizedNames != nil {
				usedNormalizedNames[finalNormalized] = struct{}{}
			}
			return finalName
		}
		count++
		finalName = fmt.Sprintf("%s (%d)", baseName, count)
		finalNormalized = NormalizeNameForMCP(finalName)
	}
}

// GetMCPDisplayName removes a server's MCP prefix from a full tool name.
func GetMCPDisplayName(fullName, serverName string) string {
	return strings.Replace(fullName, GetMCPPrefix(serverName), "", 1)
}

// ExtractMCPToolDisplayName strips the user-facing "server - tool (MCP)" wrapper.
func ExtractMCPToolDisplayName(userFacingName string) string {
	withoutSuffix := regexp.MustCompile(`\s*\(MCP\)\s*$`).ReplaceAllString(userFacingName, "")
	withoutSuffix = strings.TrimSpace(withoutSuffix)
	if idx := strings.Index(withoutSuffix, " - "); idx >= 0 {
		return strings.TrimSpace(withoutSuffix[idx+3:])
	}
	return withoutSuffix
}
