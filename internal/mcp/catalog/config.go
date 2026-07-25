package catalog

import (
	"encoding/json"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/agent-dance/luban/i18n"
	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
)

// ParseOptions controls MCP config parsing.
type ParseOptions struct {
	Scope      ConfigScope
	ExpandVars bool
	FilePath   string
}

var serverNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ParseMCPConfig parses an MCP config payload using the settings/.mcp.json shape.
func ParseMCPConfig(data []byte, opts ParseOptions) (*MCPConfigParseResult, error) {
	if opts.Scope == "" {
		opts.Scope = ScopeLocal
	}

	var settings settingsConfig
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, i18n.WrapError(i18n.KeyMCPValidationInvalidJSONCause, err)
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

func parseSettingsConfig(settings settingsConfig, opts ParseOptions) *MCPConfigParseResult {
	servers := make(map[string]MCPServerConfig)
	if settings.MCPServers == nil {
		settings.MCPServers = map[string]MCPServerConfig{}
	}
	result := &MCPConfigParseResult{
		Servers:            servers,
		DisabledMCPServers: append([]string(nil), settings.DisabledMCPServers...),
	}

	for name, config := range settings.MCPServers {
		config.Scope = opts.Scope
		if opts.ExpandVars {
			var missing []string
			config, missing = expandEnvVarsInConfig(config)
			if len(missing) > 0 {
				joined := strings.Join(missing, ", ")
				result.Errors = append(result.Errors, validationError(opts, name, validationSeverityWarning, catalogFormat(i18n.KeyMCPValidationMissingEnv, joined), catalogFormat(i18n.KeyMCPValidationSetEnv, joined)))
			}
		}
		result.Errors = append(result.Errors, validateServer(name, config, opts)...)
		result.Servers[name] = config
	}

	return result
}

func validateServer(name string, config MCPServerConfig, opts ParseOptions) []ValidationError {
	errorsOut := []ValidationError{}
	if err := validateServerName(name); err != nil {
		errorsOut = append(errorsOut, validationError(opts, name, validationSeverityFatal, err.Error(), ""))
	}
	switch config.Type {
	case "", TransportStdio:
		if strings.TrimSpace(config.Command) == "" {
			errorsOut = append(errorsOut, validationError(opts, name, validationSeverityFatal, catalogText(i18n.KeyMCPValidationCommandEmpty), catalogFormat(i18n.KeyMCPValidationSetField, "mcpServers."+name+".command")))
		}
	case TransportSSE, TransportHTTP, TransportWebSocket:
		if err := validateRemoteURL(config.URL, config.Type); err != nil {
			errorsOut = append(errorsOut, validationError(opts, name, validationSeverityFatal, err.Error(), catalogFormat(i18n.KeyMCPValidationSetURL, "mcpServers."+name+".url")))
		}
		errorsOut = append(errorsOut, validateOAuth(name, config.OAuth, opts)...)
	case TransportSSEIDE, TransportWebSocketIDE:
		if err := validateRemoteURL(config.URL, config.Type); err != nil {
			errorsOut = append(errorsOut, validationError(opts, name, validationSeverityFatal, err.Error(), catalogFormat(i18n.KeyMCPValidationSetURL, "mcpServers."+name+".url")))
		}
		if strings.TrimSpace(config.IDEName) == "" {
			errorsOut = append(errorsOut, validationError(opts, name, validationSeverityFatal, catalogText(i18n.KeyMCPValidationIDENameEmpty), catalogFormat(i18n.KeyMCPValidationSetField, "mcpServers."+name+".ideName")))
		}
	default:
		errorsOut = append(errorsOut, validationError(opts, name, validationSeverityFatal, catalogFormat(i18n.KeyMCPValidationTransportInvalid, config.Type), catalogText(i18n.KeyMCPValidationUseTransport)))
	}
	return errorsOut
}

func validateServerName(name string) error {
	if strings.TrimSpace(name) == "" {
		return i18n.NewError(i18n.KeyMCPValidationServerNameEmpty)
	}
	if !serverNamePattern.MatchString(name) {
		return i18n.NewError(i18n.KeyMCPValidationServerNameInvalid, name)
	}
	return nil
}

func validateOAuth(name string, oauth *mcpauth.Config, opts ParseOptions) []ValidationError {
	if oauth == nil {
		return nil
	}
	errorsOut := []ValidationError{}
	if oauth.CallbackPort != nil && *oauth.CallbackPort <= 0 {
		errorsOut = append(errorsOut, validationError(opts, name, validationSeverityFatal, catalogText(i18n.KeyMCPValidationCallbackPort), catalogText(i18n.KeyMCPValidationSetCallbackPort)))
	}
	if oauth.AuthServerMetadataURL != "" {
		parsed, err := url.Parse(oauth.AuthServerMetadataURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			errorsOut = append(errorsOut, validationError(opts, name, validationSeverityFatal, catalogText(i18n.KeyMCPValidationMetadataURL), catalogText(i18n.KeyMCPValidationSetMetadataURL)))
		} else if parsed.Scheme != "https" {
			errorsOut = append(errorsOut, validationError(opts, name, validationSeverityFatal, catalogText(i18n.KeyMCPValidationMetadataHTTPS), catalogText(i18n.KeyMCPValidationUseMetadataHTTPS)))
		}
	}
	return errorsOut
}

func validateRemoteURL(raw string, transport TransportType) error {
	if strings.TrimSpace(raw) == "" {
		return i18n.NewError(i18n.KeyMCPValidationURLEmpty)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return i18n.NewError(i18n.KeyMCPValidationURLInvalid, raw)
	}
	switch transport {
	case TransportWebSocket, TransportWebSocketIDE:
		if parsed.Scheme != "ws" && parsed.Scheme != "wss" && parsed.Scheme != "http" && parsed.Scheme != "https" {
			return i18n.NewError(i18n.KeyMCPValidationURLInvalid, raw)
		}
	default:
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return i18n.NewError(i18n.KeyMCPValidationURLInvalid, raw)
		}
	}
	return nil
}

func catalogText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func catalogFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
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
