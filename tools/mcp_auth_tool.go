package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	coremcp "github.com/agent-dance/luban/mcp"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
)

// McpAuthOutput is the model-visible result of mcp__<server>__authenticate.
type McpAuthOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	AuthURL string `json:"authUrl,omitempty"`
}

// McpAuthTool is the needs-auth pseudo-tool surfaced while a remote MCP server
// has no usable OAuth credentials.
type McpAuthTool struct {
	ServerName string
	Config     svcmcp.MCPServerConfig
	OAuth      *svcmcp.OAuthManager

	// OnAuthenticated is called after the callback exchanges successfully.
	// task_07/task_08 can use this to reconnect and replace the pseudo-tool
	// with dynamic real MCP tools without changing this tool's OAuth flow.
	OnAuthenticated func(context.Context, string, svcmcp.MCPServerConfig) error
}

// NewMcpAuthTool constructs an authenticate pseudo-tool for a services config.
func NewMcpAuthTool(serverName string, cfg svcmcp.MCPServerConfig, manager *svcmcp.OAuthManager) *McpAuthTool {
	if manager == nil {
		manager = svcmcp.NewOAuthManager(nil, nil)
	}
	return &McpAuthTool{ServerName: serverName, Config: cfg, OAuth: manager}
}

// NewMcpAuthToolFromRuntimeConfig adapts the lower-level mcp.ServerConfig used
// by the current MCPManager into the services config shape.
func NewMcpAuthToolFromRuntimeConfig(serverName string, cfg coremcp.ServerConfig, manager *svcmcp.OAuthManager) *McpAuthTool {
	return NewMcpAuthTool(serverName, serviceMCPConfigFromRuntime(cfg), manager)
}

func serviceMCPConfigFromRuntime(cfg coremcp.ServerConfig) svcmcp.MCPServerConfig {
	out := svcmcp.MCPServerConfig{
		Type:          svcmcp.TransportType(cfg.Type),
		Command:       cfg.Command,
		Args:          append([]string(nil), cfg.Args...),
		Env:           cloneMCPAuthStringMap(cfg.Env),
		URL:           cfg.URL,
		Headers:       cloneMCPAuthStringMap(cfg.Headers),
		HeadersHelper: cfg.HeadersHelper,
		IDEName:       cfg.IDEName,
		AuthToken:     cfg.AuthToken,
		Name:          cfg.Name,
		ID:            cfg.ID,
		Scope:         svcmcp.ConfigScope(cfg.Scope),
		PluginSource:  cfg.PluginSource,
	}
	if cfg.OAuth != nil {
		out.OAuth = &svcmcp.OAuthConfig{
			ClientID:              cfg.OAuth.ClientID,
			CallbackPort:          cfg.OAuth.CallbackPort,
			AuthServerMetadataURL: cfg.OAuth.AuthServerMetadataURL,
			XAA:                   cfg.OAuth.XAA,
		}
	}
	return out
}

func cloneMCPAuthStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (t *McpAuthTool) Name() string {
	return svcmcp.BuildMCPToolName(t.ServerName, "authenticate")
}

func (t *McpAuthTool) Description() string {
	transport := string(t.Config.Type)
	if transport == "" {
		transport = "stdio"
	}
	location := transport
	if strings.TrimSpace(t.Config.URL) != "" {
		location = fmt.Sprintf("%s at %s", transport, t.Config.URL)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyMCPAuthToolDescription, t.ServerName, location)
}

func (t *McpAuthTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object", Properties: map[string]any{}}
}

func (t *McpAuthTool) IsConcurrentSafe() bool { return false }

func (t *McpAuthTool) Execute(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
	if t == nil {
		return authToolResult(McpAuthOutput{Status: "error", Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyMCPAuthToolUninitialized)}, true), nil
	}
	transport := t.Config.Type
	if transport == "" {
		transport = svcmcp.TransportStdio
	}
	switch transport {
	case svcmcp.TransportClaudeAIProxy:
		return authToolResult(McpAuthOutput{
			Status:  "unsupported",
			Message: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyMCPAuthToolClaudeConnector, t.ServerName),
		}, false), nil
	case svcmcp.TransportHTTP, svcmcp.TransportSSE:
	default:
		return authToolResult(McpAuthOutput{
			Status:  "unsupported",
			Message: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyMCPAuthToolUnsupportedTransport, t.ServerName, transport),
		}, false), nil
	}
	manager := t.OAuth
	if manager == nil {
		manager = svcmcp.NewOAuthManager(nil, nil)
	}
	flow, err := manager.StartOAuthFlow(ctx, t.ServerName, t.Config, svcmcp.OAuthFlowOptions{
		SkipBrowserOpen: true,
	})
	if err != nil {
		return authToolResult(McpAuthOutput{
			Status:  "error",
			Message: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyMCPAuthToolStartFailed, t.ServerName, err),
		}, true), nil
	}
	if t.OnAuthenticated != nil {
		go func() {
			if err := <-flow.Done(); err == nil {
				_ = t.OnAuthenticated(context.Background(), t.ServerName, t.Config)
			}
		}()
	}
	return authToolResult(McpAuthOutput{
		Status:  "auth_url",
		AuthURL: flow.AuthorizationURL,
		Message: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyMCPAuthToolAuthorizationURL, t.ServerName, flow.AuthorizationURL),
	}, false), nil
}

func authToolResult(out McpAuthOutput, isError bool) types.ToolResult {
	raw, err := json.Marshal(out)
	if err != nil {
		// i18n:allow display-literal protocol -- This JSON object is the MCP authentication tool's wire envelope; the message is the raw encoder error.
		return types.ToolResult{Content: fmt.Sprintf(`{"status":"error","message":"%s"}`, err), IsError: true}
	}
	return types.ToolResult{Content: string(raw), IsError: isError}
}
