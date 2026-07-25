package catalog

import (
	"encoding/json"

	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
)

// ConfigScope identifies where an MCP server configuration came from.
type ConfigScope string

const (
	ScopeLocal   ConfigScope = "local"
	ScopeUser    ConfigScope = "user"
	ScopeProject ConfigScope = "project"
)

// TransportType is the MCP server wire transport discriminator.
type TransportType string

const (
	TransportStdio        TransportType = "stdio"
	TransportSSE          TransportType = "sse"
	TransportSSEIDE       TransportType = "sse-ide"
	TransportHTTP         TransportType = "http"
	TransportWebSocket    TransportType = "ws"
	TransportWebSocketIDE TransportType = "ws-ide"
)

// MCPServerConfig is the union of all supported MCP server config variants.
// Transport-specific validation lives in config.go.
type MCPServerConfig struct {
	Type                TransportType     `json:"type,omitempty"`
	Command             string            `json:"command,omitempty"`
	Args                []string          `json:"args,omitempty"`
	Env                 map[string]string `json:"env,omitempty"`
	URL                 string            `json:"url,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	OAuth               *mcpauth.Config   `json:"oauth,omitempty"`
	IDEName             string            `json:"ideName,omitempty"`
	IDERunningInWindows *bool             `json:"ideRunningInWindows,omitempty"`
	Scope               ConfigScope       `json:"-"`
}

// AuthDescriptor returns the auth domain's minimal view of this server. The
// config schema remains authoritative for which transports support OAuth.
func (config MCPServerConfig) AuthDescriptor() mcpauth.ServerDescriptor {
	return mcpauth.ServerDescriptor{
		Transport:    string(config.Type),
		URL:          config.URL,
		Headers:      config.Headers,
		OAuth:        config.OAuth,
		OAuthCapable: config.Type == TransportHTTP || config.Type == TransportSSE,
	}
}

// UnmarshalJSON defaults omitted type to stdio and omitted stdio args to [].
func (c *MCPServerConfig) UnmarshalJSON(data []byte) error {
	type Alias MCPServerConfig
	var raw Alias
	raw.Args = []string{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Type == "" {
		raw.Type = TransportStdio
	}
	if raw.Type == TransportStdio && raw.Args == nil {
		raw.Args = []string{}
	}
	*c = MCPServerConfig(raw)
	return nil
}

// settingsConfig is the MCP-relevant subset of settings files accepted by the
// parser. It stays private because callers consume MCPConfigParseResult.
type settingsConfig struct {
	MCPServers         map[string]MCPServerConfig `json:"mcpServers"`
	DisabledMCPServers []string                   `json:"disabledMcpServers"`
}

// ValidationError is a structured MCP configuration diagnostic.
type ValidationError struct {
	File             string           `json:"file,omitempty"`
	Path             string           `json:"path,omitempty"`
	Message          string           `json:"message"`
	Suggestion       string           `json:"suggestion,omitempty"`
	MCPErrorMetadata MCPErrorMetadata `json:"mcpErrorMetadata"`
}

// MCPErrorMetadata scopes a validation error to a server and severity.
type MCPErrorMetadata struct {
	Scope      ConfigScope `json:"scope"`
	ServerName string      `json:"serverName,omitempty"`
	Severity   string      `json:"severity"`
}

// MCPConfigParseResult is the parsed config plus source-scope metadata.
type MCPConfigParseResult struct {
	Servers            map[string]MCPServerConfig
	Errors             []ValidationError
	DisabledMCPServers []string
}

// IsServerDisabled reports whether the settings control list disables name.
func (r *MCPConfigParseResult) IsServerDisabled(name string) bool {
	if r == nil {
		return false
	}
	return stringSliceContains(r.DisabledMCPServers, name)
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
