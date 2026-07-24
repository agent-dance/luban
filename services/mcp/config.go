package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// ParseOptions controls MCP config parsing.
type ParseOptions struct {
	Scope      ConfigScope
	ExpandVars bool
	FilePath   string
}

var serverNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ParseMCPConfig parses a settings/.mcp.json-compatible MCP config payload.
func ParseMCPConfig(data []byte, opts ParseOptions) (*MCPConfigParseResult, error) {
	if opts.Scope == "" {
		opts.Scope = ScopeLocal
	}

	var settings SettingsConfig
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("%s: %w", mcpText(i18n.KeyMCPValidationInvalidJSON), err)
	}
	return parseSettingsConfig(settings, opts), nil
}

// ParseMCPConfigFile parses MCP config from a file path.
func ParseMCPConfigFile(path string, opts ParseOptions) (*MCPConfigParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if opts.FilePath == "" {
		opts.FilePath = path
	}
	return ParseMCPConfig(data, opts)
}

func parseSettingsConfig(settings SettingsConfig, opts ParseOptions) *MCPConfigParseResult {
	servers := make(map[string]MCPServerConfig)
	if settings.MCPServers == nil {
		settings.MCPServers = map[string]MCPServerConfig{}
	}
	result := &MCPConfigParseResult{
		Config: MCPJSONConfig{
			MCPServers: make(map[string]MCPServerConfig, len(settings.MCPServers)),
		},
		Servers:                servers,
		EnabledMCPServers:      append([]string(nil), settings.EnabledMCPServers...),
		DisabledMCPServers:     append([]string(nil), settings.DisabledMCPServers...),
		EnabledMCPJSONServers:  append([]string(nil), settings.EnabledMCPJSONServers...),
		DisabledMCPJSONServers: append([]string(nil), settings.DisabledMCPJSONServers...),
		AutoApproveAllMCPJSON:  settings.AutoApproveAllMCPJSON,
		UnknownSettingsFields:  settings.Unknown,
	}

	for name, config := range settings.MCPServers {
		config.Scope = opts.Scope
		if opts.ExpandVars {
			var missing []string
			config, missing = ExpandEnvVarsInConfig(config)
			if len(missing) > 0 {
				joined := strings.Join(missing, ", ")
				result.Errors = append(result.Errors, validationError(opts, name, "warning", mcpFormat(i18n.KeyMCPValidationMissingEnv, joined), mcpFormat(i18n.KeyMCPValidationSetEnv, joined)))
			}
		}
		result.Errors = append(result.Errors, validateServer(name, config, opts)...)
		result.Servers[name] = config
		result.Config.MCPServers[name] = config
	}

	return result
}

func validateServer(name string, config MCPServerConfig, opts ParseOptions) []ValidationError {
	errorsOut := []ValidationError{}
	if err := ValidateServerName(name, opts.Scope); err != nil {
		errorsOut = append(errorsOut, validationError(opts, name, "fatal", err.Error(), ""))
	}
	switch config.Type {
	case "", TransportStdio:
		if strings.TrimSpace(config.Command) == "" {
			errorsOut = append(errorsOut, validationError(opts, name, "fatal", mcpText(i18n.KeyMCPValidationCommandEmpty), mcpFormat(i18n.KeyMCPValidationSetField, "mcpServers."+name+".command")))
		}
	case TransportSSE, TransportHTTP, TransportWebSocket:
		if err := validateRemoteURL(config.URL, config.Type); err != nil {
			errorsOut = append(errorsOut, validationError(opts, name, "fatal", err.Error(), mcpFormat(i18n.KeyMCPValidationSetURL, "mcpServers."+name+".url")))
		}
		errorsOut = append(errorsOut, validateOAuth(name, config.OAuth, opts)...)
	case TransportSSEIDE, TransportWebSocketIDE:
		if err := validateRemoteURL(config.URL, config.Type); err != nil {
			errorsOut = append(errorsOut, validationError(opts, name, "fatal", err.Error(), mcpFormat(i18n.KeyMCPValidationSetURL, "mcpServers."+name+".url")))
		}
		if strings.TrimSpace(config.IDEName) == "" {
			errorsOut = append(errorsOut, validationError(opts, name, "fatal", mcpText(i18n.KeyMCPValidationIDENameEmpty), mcpFormat(i18n.KeyMCPValidationSetField, "mcpServers."+name+".ideName")))
		}
	case TransportSDK:
		if strings.TrimSpace(config.Name) == "" {
			errorsOut = append(errorsOut, validationError(opts, name, "fatal", mcpText(i18n.KeyMCPValidationNameEmpty), mcpFormat(i18n.KeyMCPValidationSetField, "mcpServers."+name+".name")))
		}
	case TransportClaudeAIProxy:
		if err := validateRemoteURL(config.URL, config.Type); err != nil {
			errorsOut = append(errorsOut, validationError(opts, name, "fatal", err.Error(), mcpFormat(i18n.KeyMCPValidationSetURL, "mcpServers."+name+".url")))
		}
		if strings.TrimSpace(config.ID) == "" {
			errorsOut = append(errorsOut, validationError(opts, name, "fatal", mcpText(i18n.KeyMCPValidationIDEmpty), mcpFormat(i18n.KeyMCPValidationSetField, "mcpServers."+name+".id")))
		}
	default:
		errorsOut = append(errorsOut, validationError(opts, name, "fatal", mcpFormat(i18n.KeyMCPValidationTransportInvalid, config.Type), mcpText(i18n.KeyMCPValidationUseTransport)))
	}
	return errorsOut
}

// ValidateServerName applies the add-time TypeScript name restrictions.
func ValidateServerName(name string, scope ConfigScope) error {
	if strings.TrimSpace(name) == "" {
		return errors.New(mcpText(i18n.KeyMCPValidationServerNameEmpty))
	}
	if IsReservedMCPServerName(name) {
		return fmt.Errorf("%s", mcpFormat(i18n.KeyMCPValidationServerNameReserved, name))
	}
	if scope == ScopeClaudeAI && strings.HasPrefix(name, claudeAIServerPrefix) {
		return nil
	}
	if !serverNamePattern.MatchString(name) {
		return fmt.Errorf("%s", mcpFormat(i18n.KeyMCPValidationServerNameInvalid, name))
	}
	return nil
}

// IsReservedMCPServerName mirrors the normalized claude-in-chrome reserved-name check.
func IsReservedMCPServerName(name string) bool {
	return NormalizeNameForMCP(name) == "claude-in-chrome"
}

func validateOAuth(name string, oauth *OAuthConfig, opts ParseOptions) []ValidationError {
	if oauth == nil {
		return nil
	}
	errorsOut := []ValidationError{}
	if oauth.CallbackPort != nil && *oauth.CallbackPort <= 0 {
		errorsOut = append(errorsOut, validationError(opts, name, "fatal", mcpText(i18n.KeyMCPValidationCallbackPort), mcpText(i18n.KeyMCPValidationSetCallbackPort)))
	}
	if oauth.AuthServerMetadataURL != "" {
		parsed, err := url.Parse(oauth.AuthServerMetadataURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			errorsOut = append(errorsOut, validationError(opts, name, "fatal", mcpText(i18n.KeyMCPValidationMetadataURL), mcpText(i18n.KeyMCPValidationSetMetadataURL)))
		} else if parsed.Scheme != "https" {
			errorsOut = append(errorsOut, validationError(opts, name, "fatal", mcpText(i18n.KeyMCPValidationMetadataHTTPS), mcpText(i18n.KeyMCPValidationUseMetadataHTTPS)))
		}
	}
	return errorsOut
}

func validateRemoteURL(raw string, transport TransportType) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New(mcpText(i18n.KeyMCPValidationURLEmpty))
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s", mcpFormat(i18n.KeyMCPValidationURLInvalid, raw))
	}
	switch transport {
	case TransportWebSocket, TransportWebSocketIDE:
		if parsed.Scheme != "ws" && parsed.Scheme != "wss" && parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("%s", mcpFormat(i18n.KeyMCPValidationURLInvalid, raw))
		}
	default:
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("%s", mcpFormat(i18n.KeyMCPValidationURLInvalid, raw))
		}
	}
	return nil
}

func validationError(opts ParseOptions, serverName, severity, message, suggestion string) ValidationError {
	return ValidationError{
		File:       opts.FilePath,
		Path:       "mcpServers." + serverName,
		Message:    message,
		Suggestion: suggestion,
		MCPErrorMetadata: MCPErrorMetadata{
			Scope:      opts.Scope,
			ServerName: serverName,
			Severity:   severity,
		},
	}
}

// AddScopeToServers returns a scoped copy of the given server map.
func AddScopeToServers(servers map[string]MCPServerConfig, scope ConfigScope) map[string]MCPServerConfig {
	if servers == nil {
		return map[string]MCPServerConfig{}
	}
	out := make(map[string]MCPServerConfig, len(servers))
	for name, config := range servers {
		config.Scope = scope
		out[name] = config
	}
	return out
}
