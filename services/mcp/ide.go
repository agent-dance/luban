package mcp

import (
	"context"
	"encoding/json"

	"github.com/agent-dance/luban/i18n"
)

const (
	ideAuthorizationHeader = "X-Claude-Code-Ide-Authorization"
)

var allowedIDERawTools = map[string]struct{}{
	"executeCode":    {},
	"getDiagnostics": {},
}

// NewIDETransport builds the internal IDE transport variants. IDE endpoints
// are local extension channels, so they do not use the OAuth TokenSource; the
// optional IDE auth token is propagated as the internal header used by the TS
// WebSocket path.
func NewIDETransport(ctx context.Context, name string, config MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
	headers := IDEHeaders(config)
	switch config.Type {
	case TransportSSEIDE:
		return NewSSETransport(SSEConfig{
			BaseURL:        config.URL,
			HTTPClient:     opts.HTTPClient,
			Headers:        headers,
			HeaderProvider: opts.HeaderProvider,
			ServerName:     name,
			UserAgent:      opts.UserAgent,
		})
	case TransportWebSocketIDE:
		return NewWebSocketTransport(ctx, WebSocketTransportConfig{
			URL:            config.URL,
			Headers:        headers,
			HeaderProvider: opts.HeaderProvider,
			ServerName:     name,
			UserAgent:      opts.UserAgent,
		})
	default:
		return nil, &UnsupportedIntegrationError{
			ServerName: name,
			Transport:  config.Type,
			Reason:     mcpText(i18n.KeyMCPNotIDETransport),
		}
	}
}

// IDEHeaders returns the static headers for sse-ide/ws-ide transports.
func IDEHeaders(config MCPServerConfig) map[string]string {
	headers := cloneStringMap(config.Headers)
	if config.AuthToken != "" {
		if headers == nil {
			headers = map[string]string{}
		}
		headers[ideAuthorizationHeader] = config.AuthToken
	}
	return headers
}

// FilterIDEListToolsResult preserves the TS IDE allow-list. The TS layer
// filters fully-qualified mcp__ide__* tools; at the service layer we keep only
// the raw tools that will become those names.
func FilterIDEListToolsResult(config MCPServerConfig, result *ListToolsResult) *ListToolsResult {
	if result == nil || !IsIDETransport(config.Type) {
		return result
	}
	filtered := cloneListToolsResult(*result)
	filtered.Tools = filtered.Tools[:0]
	for _, tool := range result.Tools {
		if _, ok := allowedIDERawTools[tool.Name]; ok {
			filtered.Tools = append(filtered.Tools, cloneToolDefinition(tool))
		}
	}
	return &filtered
}

// IsIDETransport reports whether transport is one of the internal IDE MCP
// variants.
func IsIDETransport(transport TransportType) bool {
	return transport == TransportSSEIDE || transport == TransportWebSocketIDE
}

// VSCodeLogEventHandler receives log_event notifications from the VS Code SDK
// MCP server.
type VSCodeLogEventHandler func(eventName string, eventData map[string]any)

// SetupVSCodeSDKMCP installs the VS Code SDK notification bridge and sends the
// initial experiment_gates notification. Callers supply concrete gates so this
// package does not import analytics or feature-flag state.
func SetupVSCodeSDKMCP(ctx context.Context, client *Client, gates map[string]any, onLogEvent VSCodeLogEventHandler) error {
	if client == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client.SetNotificationHandler("log_event", func(_ context.Context, msg JSONRPCMessage) {
		if onLogEvent == nil {
			return
		}
		var payload struct {
			EventName string         `json:"eventName"`
			EventData map[string]any `json:"eventData"`
		}
		if len(msg.Params) == 0 || json.Unmarshal(msg.Params, &payload) != nil || payload.EventName == "" {
			return
		}
		if payload.EventData == nil {
			payload.EventData = map[string]any{}
		}
		onLogEvent(payload.EventName, payload.EventData)
	})
	if gates == nil {
		gates = map[string]any{}
	}
	return client.Notify(ctx, "experiment_gates", map[string]any{"gates": gates})
}
