package mcp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// MCPPolicyEntry is one allowedMcpServers/deniedMcpServers entry from managed
// settings. Exactly one selector is normally populated.
type MCPPolicyEntry struct {
	ServerName    string   `json:"serverName,omitempty"`
	ServerCommand []string `json:"serverCommand,omitempty"`
	ServerURL     string   `json:"serverUrl,omitempty"`
}

// MCPPolicySettings is the MCP-relevant managed settings subset.
type MCPPolicySettings struct {
	AllowedMCPServers          []MCPPolicyEntry `json:"allowedMcpServers,omitempty"`
	AllowedMCPServersSet       bool             `json:"-"`
	DeniedMCPServers           []MCPPolicyEntry `json:"deniedMcpServers,omitempty"`
	AllowManagedMCPServersOnly bool             `json:"allowManagedMcpServersOnly,omitempty"`
}

// MCPPolicyDecision records why a server was allowed or blocked.
type MCPPolicyDecision struct {
	Allowed bool
	Reason  string
	Entry   *MCPPolicyEntry
}

// MCPPolicyFilterResult is returned by FilterMCPServersByPolicy.
type MCPPolicyFilterResult struct {
	Allowed map[string]MCPServerConfig
	Blocked map[string]MCPPolicyDecision
}

// IsMCPServerAllowedByPolicy mirrors the TypeScript managed policy semantics:
// deny entries win, nil allowlist means unrestricted, empty allowlist blocks all,
// and command/url allowlists are fail-closed for their transport family.
func IsMCPServerAllowedByPolicy(serverName string, config MCPServerConfig, settings MCPPolicySettings) MCPPolicyDecision {
	if entry := matchingDeniedMCPPolicyEntry(serverName, config, settings.DeniedMCPServers); entry != nil {
		return MCPPolicyDecision{Allowed: false, Reason: "server is explicitly blocked by enterprise policy", Entry: entry}
	}
	if !settings.AllowedMCPServersSet {
		return MCPPolicyDecision{Allowed: true, Reason: "no managed MCP allowlist configured"}
	}
	if len(settings.AllowedMCPServers) == 0 {
		return MCPPolicyDecision{Allowed: false, Reason: "managed MCP allowlist is empty"}
	}

	command := serverCommandArray(config)
	serverURL := serverURL(config)
	hasCommandEntries := hasPolicyCommandEntries(settings.AllowedMCPServers)
	hasURLEntries := hasPolicyURLEntries(settings.AllowedMCPServers)

	switch {
	case command != nil:
		if hasCommandEntries {
			if entry := matchingCommandEntry(command, settings.AllowedMCPServers); entry != nil {
				return MCPPolicyDecision{Allowed: true, Reason: "server command is allowed by enterprise policy", Entry: entry}
			}
			return MCPPolicyDecision{Allowed: false, Reason: "server command is not on managed MCP allowlist"}
		}
		if entry := matchingNameEntry(serverName, settings.AllowedMCPServers); entry != nil {
			return MCPPolicyDecision{Allowed: true, Reason: "server name is allowed by enterprise policy", Entry: entry}
		}
		return MCPPolicyDecision{Allowed: false, Reason: "server name is not on managed MCP allowlist"}
	case serverURL != "":
		if hasURLEntries {
			if entry := matchingURLEntry(serverURL, settings.AllowedMCPServers); entry != nil {
				return MCPPolicyDecision{Allowed: true, Reason: "server URL is allowed by enterprise policy", Entry: entry}
			}
			return MCPPolicyDecision{Allowed: false, Reason: "server URL is not on managed MCP allowlist"}
		}
		if entry := matchingNameEntry(serverName, settings.AllowedMCPServers); entry != nil {
			return MCPPolicyDecision{Allowed: true, Reason: "server name is allowed by enterprise policy", Entry: entry}
		}
		return MCPPolicyDecision{Allowed: false, Reason: "server name is not on managed MCP allowlist"}
	default:
		if entry := matchingNameEntry(serverName, settings.AllowedMCPServers); entry != nil {
			return MCPPolicyDecision{Allowed: true, Reason: "server name is allowed by enterprise policy", Entry: entry}
		}
		return MCPPolicyDecision{Allowed: false, Reason: "server name is not on managed MCP allowlist"}
	}
}

// FilterMCPServersByPolicy drops policy-blocked servers before callers start
// subprocesses or remote connections. SDK placeholders are exempt, matching the
// TypeScript process boundary.
func FilterMCPServersByPolicy(configs map[string]MCPServerConfig, settings MCPPolicySettings) MCPPolicyFilterResult {
	result := MCPPolicyFilterResult{
		Allowed: make(map[string]MCPServerConfig, len(configs)),
		Blocked: make(map[string]MCPPolicyDecision),
	}
	for name, config := range configs {
		if config.Type == TransportSDK {
			result.Allowed[name] = config
			continue
		}
		decision := IsMCPServerAllowedByPolicy(name, config, settings)
		if decision.Allowed {
			result.Allowed[name] = config
		} else {
			result.Blocked[name] = decision
		}
	}
	return result
}

// ProjectApprovalStatus mirrors getProjectMcpServerStatus.
type ProjectApprovalStatus string

const (
	ProjectApprovalApproved ProjectApprovalStatus = "approved"
	ProjectApprovalRejected ProjectApprovalStatus = "rejected"
	ProjectApprovalPending  ProjectApprovalStatus = "pending"
)

// ProjectApprovalSettings captures the settings and runtime gates that affect
// project .mcp.json approval.
type ProjectApprovalSettings struct {
	EnabledMCPJSONServers  []string
	DisabledMCPJSONServers []string
	AutoApproveAllMCPJSON  bool
	ProjectSettingsEnabled bool
	BypassPermissionPrompt bool
	NonInteractiveSession  bool
}

// ProjectMCPServerStatus returns approved/rejected/pending using normalized
// server names and the same fail-closed defaults as the original.
func ProjectMCPServerStatus(serverName string, settings ProjectApprovalSettings) ProjectApprovalStatus {
	normalized := NormalizeNameForMCP(serverName)
	for _, name := range settings.DisabledMCPJSONServers {
		if NormalizeNameForMCP(name) == normalized {
			return ProjectApprovalRejected
		}
	}
	for _, name := range settings.EnabledMCPJSONServers {
		if NormalizeNameForMCP(name) == normalized {
			return ProjectApprovalApproved
		}
	}
	if settings.AutoApproveAllMCPJSON {
		return ProjectApprovalApproved
	}
	if settings.ProjectSettingsEnabled && (settings.BypassPermissionPrompt || settings.NonInteractiveSession) {
		return ProjectApprovalApproved
	}
	return ProjectApprovalPending
}

// FilterApprovedProjectMCPServers returns only project-scoped servers whose
// approval status is approved. Pending and rejected servers are withheld so the
// connection manager cannot start them silently.
func FilterApprovedProjectMCPServers(configs map[string]MCPServerConfig, settings ProjectApprovalSettings) (map[string]MCPServerConfig, map[string]ProjectApprovalStatus) {
	approved := make(map[string]MCPServerConfig, len(configs))
	blocked := make(map[string]ProjectApprovalStatus)
	for name, config := range configs {
		if config.Scope != ScopeProject {
			approved[name] = config
			continue
		}
		status := ProjectMCPServerStatus(name, settings)
		if status == ProjectApprovalApproved {
			approved[name] = config
		} else {
			blocked[name] = status
		}
	}
	return approved, blocked
}

// MCPServerApprovalRequest is the UI-independent request shape for project MCP
// server approval flows.
type MCPServerApprovalRequest struct {
	ServerName string                `json:"serverName"`
	Scope      ConfigScope           `json:"scope"`
	Status     ProjectApprovalStatus `json:"status"`
	ConfigHash string                `json:"configHash"`
}

// PendingProjectApprovalRequests builds approval prompts for project servers
// that are not yet approved.
func PendingProjectApprovalRequests(configs map[string]MCPServerConfig, settings ProjectApprovalSettings) []MCPServerApprovalRequest {
	out := make([]MCPServerApprovalRequest, 0)
	for name, config := range configs {
		if config.Scope != ScopeProject {
			continue
		}
		status := ProjectMCPServerStatus(name, settings)
		if status == ProjectApprovalPending {
			out = append(out, MCPServerApprovalRequest{
				ServerName: name,
				Scope:      config.Scope,
				Status:     status,
				ConfigHash: HashMCPConfig(config),
			})
		}
	}
	return out
}

func matchingDeniedMCPPolicyEntry(serverName string, config MCPServerConfig, entries []MCPPolicyEntry) *MCPPolicyEntry {
	if entry := matchingNameEntry(serverName, entries); entry != nil {
		return entry
	}
	if command := serverCommandArray(config); command != nil {
		if entry := matchingCommandEntry(command, entries); entry != nil {
			return entry
		}
	}
	if rawURL := serverURL(config); rawURL != "" {
		if entry := matchingURLEntry(rawURL, entries); entry != nil {
			return entry
		}
	}
	return nil
}

func matchingNameEntry(serverName string, entries []MCPPolicyEntry) *MCPPolicyEntry {
	for i := range entries {
		if entries[i].ServerName == serverName {
			return &entries[i]
		}
	}
	return nil
}

func matchingCommandEntry(command []string, entries []MCPPolicyEntry) *MCPPolicyEntry {
	for i := range entries {
		if commandArraysMatch(entries[i].ServerCommand, command) {
			return &entries[i]
		}
	}
	return nil
}

func matchingURLEntry(rawURL string, entries []MCPPolicyEntry) *MCPPolicyEntry {
	for i := range entries {
		if entries[i].ServerURL != "" && urlPatternMatches(rawURL, entries[i].ServerURL) {
			return &entries[i]
		}
	}
	return nil
}

func hasPolicyCommandEntries(entries []MCPPolicyEntry) bool {
	for _, entry := range entries {
		if len(entry.ServerCommand) > 0 {
			return true
		}
	}
	return false
}

func hasPolicyURLEntries(entries []MCPPolicyEntry) bool {
	for _, entry := range entries {
		if entry.ServerURL != "" {
			return true
		}
	}
	return false
}

func commandArraysMatch(a, b []string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func serverCommandArray(config MCPServerConfig) []string {
	if config.Type != "" && config.Type != TransportStdio {
		return nil
	}
	if strings.TrimSpace(config.Command) == "" {
		return nil
	}
	command := make([]string, 0, len(config.Args)+1)
	command = append(command, config.Command)
	command = append(command, config.Args...)
	return command
}

func serverURL(config MCPServerConfig) string {
	switch config.Type {
	case TransportSSE, TransportHTTP, TransportWebSocket, TransportSSEIDE, TransportWebSocketIDE, TransportClaudeAIProxy:
		return config.URL
	default:
		return ""
	}
}

func urlPatternMatches(rawURL, pattern string) bool {
	if pattern == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if rawURL == pattern {
		return true
	}
	quoted := regexp.QuoteMeta(pattern)
	quoted = strings.ReplaceAll(quoted, `\*`, ".*")
	re, err := regexp.Compile("^" + quoted + "$")
	if err != nil {
		return false
	}
	return re.MatchString(rawURL)
}

func stableJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%#v", value)
	}
	return string(data)
}
