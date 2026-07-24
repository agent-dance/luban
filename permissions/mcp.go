package permissions

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MCPPermissionInfo is parsed from mcp__<server> and
// mcp__<server>__<tool> permission identities.
type MCPPermissionInfo struct {
	ServerName string
	ToolName   string
	HasTool    bool
}

// ParseMCPPermissionIdentity mirrors the TypeScript mcpInfoFromString helper.
// The returned ToolName is optional so mcp__server can represent a server-wide
// permission rule.
func ParseMCPPermissionIdentity(identity string) (MCPPermissionInfo, bool) {
	parts := strings.Split(identity, "__")
	if len(parts) < 2 || parts[0] != "mcp" || parts[1] == "" {
		return MCPPermissionInfo{}, false
	}
	info := MCPPermissionInfo{ServerName: parts[1]}
	if len(parts) > 2 {
		info.ToolName = strings.Join(parts[2:], "__")
		info.HasTool = true
	}
	return info, true
}

// MCPPermissionRuleMatches reports whether a permission rule identity matches
// a concrete MCP tool identity. It supports the original server-wide forms:
// mcp__server and mcp__server__*.
func MCPPermissionRuleMatches(ruleIdentity, toolIdentity string) bool {
	rule, ok := ParseMCPPermissionIdentity(ruleIdentity)
	if !ok {
		return false
	}
	tool, ok := ParseMCPPermissionIdentity(toolIdentity)
	if !ok || !tool.HasTool {
		return false
	}
	if rule.ServerName != tool.ServerName {
		return false
	}
	if !rule.HasTool || rule.ToolName == "*" {
		return true
	}
	return rule.ToolName == tool.ToolName
}

// MCPPermissionRuleValid validates the MCP-specific subset of permission rule
// syntax. MCP rules are identity-only and do not support input patterns.
func MCPPermissionRuleValid(ruleIdentity string, hasInputPattern bool) error {
	if _, ok := ParseMCPPermissionIdentity(ruleIdentity); !ok {
		return nil
	}
	if hasInputPattern {
		return fmt.Errorf("MCP permission rules do not support input patterns")
	}
	return nil
}

// MCPToolPermissionIdentity returns the name permission checks must use for a
// model-facing MCP tool. If the tool is an MCP replacement with an unqualified
// display name, this keeps builtin rules such as "Write" from matching it.
func MCPToolPermissionIdentity(displayName string, serverName string, toolName string) string {
	if serverName == "" || toolName == "" {
		return displayName
	}
	return "mcp__" + normalizeMCPPermissionName(serverName) + "__" + normalizeMCPPermissionName(toolName)
}

func normalizeMCPPermissionName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// MCPAnnotationPolicy is the permission/risk view of MCP tool annotations.
type MCPAnnotationPolicy struct {
	ReadOnly       bool
	Destructive    bool
	OpenWorld      bool
	ConcurrentSafe bool
	Risk           RiskLevel
}

// ClassifyMCPAnnotations maps MCP tool annotations to Go permission metadata.
// Missing annotations stay conservative: medium risk and not concurrency-safe.
func ClassifyMCPAnnotations(annotations map[string]any) MCPAnnotationPolicy {
	readOnly := mcpAnnotationBool(annotations["readOnlyHint"])
	destructive := mcpAnnotationBool(annotations["destructiveHint"])
	openWorld := mcpAnnotationBool(annotations["openWorldHint"])

	risk := RiskMedium
	switch {
	case destructive:
		risk = RiskHigh
	case readOnly && !openWorld:
		risk = RiskLow
	}

	return MCPAnnotationPolicy{
		ReadOnly:       readOnly,
		Destructive:    destructive,
		OpenWorld:      openWorld,
		ConcurrentSafe: readOnly && !destructive,
		Risk:           risk,
	}
}

func mcpAnnotationBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	case json.Number:
		return v.String() != "0"
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}
