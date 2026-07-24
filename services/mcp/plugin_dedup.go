package mcp

import (
	"net/url"
	"sort"
	"strings"
)

var ccrProxyPathMarkers = []string{
	"/v2/session_ingress/shttp/mcp/",
	"/v2/ccr-sessions/",
}

// MCPSuppressedDuplicate records a plugin or connector server suppressed by
// signature-based deduplication.
type MCPSuppressedDuplicate struct {
	Name        string `json:"name"`
	DuplicateOf string `json:"duplicateOf"`
}

// UnwrapCCRProxyURL extracts the original vendor URL from CCR proxy URLs.
func UnwrapCCRProxyURL(rawURL string) string {
	if !containsCCRProxyMarker(rawURL) {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if original := parsed.Query().Get("mcp_url"); original != "" {
		return original
	}
	return rawURL
}

// MCPServerSignature returns the content signature used for plugin and
// claude.ai connector deduplication. Env and headers are intentionally ignored.
func MCPServerSignature(config MCPServerConfig) (string, bool) {
	if command := serverCommandArray(config); command != nil {
		return "stdio:" + stableJSON(command), true
	}
	if rawURL := serverURL(config); rawURL != "" {
		return "url:" + UnwrapCCRProxyURL(rawURL), true
	}
	return "", false
}

// DedupPluginMCPServers removes plugin servers that duplicate manually
// configured servers or earlier plugin servers. Manual wins; first plugin wins.
func DedupPluginMCPServers(pluginServers, manualServers map[string]MCPServerConfig) (map[string]MCPServerConfig, []MCPSuppressedDuplicate) {
	manualSignatures := make(map[string]string)
	for _, name := range sortedMCPServerNames(manualServers) {
		config := manualServers[name]
		if sig, ok := MCPServerSignature(config); ok {
			if _, exists := manualSignatures[sig]; !exists {
				manualSignatures[sig] = name
			}
		}
	}

	out := make(map[string]MCPServerConfig, len(pluginServers))
	suppressed := make([]MCPSuppressedDuplicate, 0)
	seenPlugins := make(map[string]string)
	for _, name := range sortedMCPServerNames(pluginServers) {
		config := pluginServers[name]
		sig, ok := MCPServerSignature(config)
		if !ok {
			out[name] = config
			continue
		}
		if manualDup, exists := manualSignatures[sig]; exists {
			suppressed = append(suppressed, MCPSuppressedDuplicate{Name: name, DuplicateOf: manualDup})
			continue
		}
		if pluginDup, exists := seenPlugins[sig]; exists {
			suppressed = append(suppressed, MCPSuppressedDuplicate{Name: name, DuplicateOf: pluginDup})
			continue
		}
		seenPlugins[sig] = name
		out[name] = config
	}
	return out, suppressed
}

// DedupClaudeAIMCPServers removes claude.ai connectors that duplicate enabled
// manually configured servers.
func DedupClaudeAIMCPServers(claudeAIServers, manualServers map[string]MCPServerConfig) (map[string]MCPServerConfig, []MCPSuppressedDuplicate) {
	manualSignatures := make(map[string]string)
	for _, name := range sortedMCPServerNames(manualServers) {
		config := manualServers[name]
		if sig, ok := MCPServerSignature(config); ok {
			if _, exists := manualSignatures[sig]; !exists {
				manualSignatures[sig] = name
			}
		}
	}

	out := make(map[string]MCPServerConfig, len(claudeAIServers))
	suppressed := make([]MCPSuppressedDuplicate, 0)
	for _, name := range sortedMCPServerNames(claudeAIServers) {
		config := claudeAIServers[name]
		if sig, ok := MCPServerSignature(config); ok {
			if manualDup, exists := manualSignatures[sig]; exists {
				suppressed = append(suppressed, MCPSuppressedDuplicate{Name: name, DuplicateOf: manualDup})
				continue
			}
		}
		out[name] = config
	}
	return out, suppressed
}

func sortedMCPServerNames(servers map[string]MCPServerConfig) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func containsCCRProxyMarker(rawURL string) bool {
	for _, marker := range ccrProxyPathMarkers {
		if strings.Contains(rawURL, marker) {
			return true
		}
	}
	return false
}
