package mcp

import "encoding/json"

// ConfigScope identifies where an MCP server configuration came from.
type ConfigScope string

const (
	ScopeLocal      ConfigScope = "local"
	ScopeUser       ConfigScope = "user"
	ScopeProject    ConfigScope = "project"
	ScopeDynamic    ConfigScope = "dynamic"
	ScopeEnterprise ConfigScope = "enterprise"
	ScopeClaudeAI   ConfigScope = "claudeai"
	ScopeManaged    ConfigScope = "managed"
)

// TransportType is the MCP transport discriminator used by the TypeScript schema.
type TransportType string

const (
	TransportStdio         TransportType = "stdio"
	TransportSSE           TransportType = "sse"
	TransportSSEIDE        TransportType = "sse-ide"
	TransportHTTP          TransportType = "http"
	TransportWebSocket     TransportType = "ws"
	TransportWebSocketIDE  TransportType = "ws-ide"
	TransportSDK           TransportType = "sdk"
	TransportClaudeAIProxy TransportType = "claudeai-proxy"
)

// OAuthConfig mirrors the TypeScript MCP OAuth config shape.
type OAuthConfig struct {
	ClientID              string `json:"clientId,omitempty"`
	CallbackPort          *int   `json:"callbackPort,omitempty"`
	AuthServerMetadataURL string `json:"authServerMetadataUrl,omitempty"`
	XAA                   *bool  `json:"xaa,omitempty"`
}

// MCPServerConfig is the union of all TypeScript MCP server config variants.
// Transport-specific validation lives in config.go; this struct intentionally
// preserves fields that later lifecycle tasks will consume.
type MCPServerConfig struct {
	Type                TransportType     `json:"type,omitempty"`
	Command             string            `json:"command,omitempty"`
	Args                []string          `json:"args,omitempty"`
	Env                 map[string]string `json:"env,omitempty"`
	URL                 string            `json:"url,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	HeadersHelper       string            `json:"headersHelper,omitempty"`
	OAuth               *OAuthConfig      `json:"oauth,omitempty"`
	IDEName             string            `json:"ideName,omitempty"`
	AuthToken           string            `json:"authToken,omitempty"`
	IDERunningInWindows *bool             `json:"ideRunningInWindows,omitempty"`
	Name                string            `json:"name,omitempty"`
	ID                  string            `json:"id,omitempty"`
	Scope               ConfigScope       `json:"-"`
	PluginSource        string            `json:"pluginSource,omitempty"`
	Unknown             map[string]any    `json:"-"`
	raw                 map[string]json.RawMessage
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
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err == nil {
		raw.raw = fields
		raw.Unknown = unknownServerFields(fields)
	}
	*c = MCPServerConfig(raw)
	return nil
}

func unknownServerFields(fields map[string]json.RawMessage) map[string]any {
	known := map[string]struct{}{
		"type": {}, "command": {}, "args": {}, "env": {}, "url": {},
		"headers": {}, "headersHelper": {}, "oauth": {}, "ideName": {},
		"authToken": {}, "ideRunningInWindows": {}, "name": {}, "id": {},
		"pluginSource": {},
	}
	unknown := make(map[string]any)
	for key, value := range fields {
		if _, ok := known[key]; ok {
			continue
		}
		var decoded any
		if err := json.Unmarshal(value, &decoded); err == nil {
			unknown[key] = decoded
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	return unknown
}

// MCPJSONConfig mirrors .mcp.json / settings.json's mcpServers field.
type MCPJSONConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// SettingsConfig is the MCP-relevant subset of Claude settings files.
type SettingsConfig struct {
	MCPServers             map[string]MCPServerConfig `json:"mcpServers"`
	EnabledMCPServers      []string                   `json:"enabledMcpServers"`
	DisabledMCPServers     []string                   `json:"disabledMcpServers"`
	EnabledMCPJSONServers  []string                   `json:"enabledMcpjsonServers"`
	DisabledMCPJSONServers []string                   `json:"disabledMcpjsonServers"`
	AutoApproveAllMCPJSON  bool                       `json:"autoApproveAllMcpjsonServers"`
	Unknown                map[string]any             `json:"-"`
	raw                    map[string]json.RawMessage
}

// UnmarshalJSON records unknown top-level fields while preserving the known MCP fields.
func (c *SettingsConfig) UnmarshalJSON(data []byte) error {
	type Alias SettingsConfig
	var raw Alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err == nil {
		raw.raw = fields
		raw.Unknown = unknownSettingsFields(fields)
	}
	*c = SettingsConfig(raw)
	return nil
}

func unknownSettingsFields(fields map[string]json.RawMessage) map[string]any {
	known := map[string]struct{}{
		"mcpServers": {}, "enabledMcpServers": {}, "disabledMcpServers": {},
		"enabledMcpjsonServers": {}, "disabledMcpjsonServers": {},
		"autoApproveAllMcpjsonServers": {},
	}
	unknown := make(map[string]any)
	for key, value := range fields {
		if _, ok := known[key]; ok {
			continue
		}
		var decoded any
		if err := json.Unmarshal(value, &decoded); err == nil {
			unknown[key] = decoded
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	return unknown
}

// ValidationError is the Go equivalent of the MCP validation error metadata
// produced by the TypeScript parser.
type ValidationError struct {
	File              string            `json:"file,omitempty"`
	Path              string            `json:"path,omitempty"`
	Message           string            `json:"message"`
	Suggestion        string            `json:"suggestion,omitempty"`
	MCPErrorMetadata  MCPErrorMetadata  `json:"mcpErrorMetadata"`
	AdditionalDetails map[string]string `json:"additionalDetails,omitempty"`
}

// MCPErrorMetadata scopes a validation error to a server and severity.
type MCPErrorMetadata struct {
	Scope      ConfigScope `json:"scope"`
	ServerName string      `json:"serverName,omitempty"`
	Severity   string      `json:"severity"`
}

// MCPConfigParseResult is the parsed config plus source-scope metadata.
type MCPConfigParseResult struct {
	Config                 MCPJSONConfig
	Servers                map[string]MCPServerConfig
	Errors                 []ValidationError
	EnabledMCPServers      []string
	DisabledMCPServers     []string
	EnabledMCPJSONServers  []string
	DisabledMCPJSONServers []string
	AutoApproveAllMCPJSON  bool
	UnknownSettingsFields  map[string]any
}

// IsServerDisabled matches the TypeScript project-level disabledMcpServers list.
func (r *MCPConfigParseResult) IsServerDisabled(name string) bool {
	if r == nil {
		return false
	}
	return stringSliceContains(r.DisabledMCPServers, name)
}

// IsServerEnabled matches the TypeScript enabledMcpServers list.
func (r *MCPConfigParseResult) IsServerEnabled(name string) bool {
	if r == nil {
		return false
	}
	return stringSliceContains(r.EnabledMCPServers, name)
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
