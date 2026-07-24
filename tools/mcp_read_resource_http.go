package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// mcpHTTPReadResource is the compatibility path for AddServer. Unlike the old
// REST-style GET, it sends the resource URI as an opaque JSON-RPC parameter to
// the configured MCP endpoint.
func mcpHTTPReadResource(ctx context.Context, baseURL, resourceURI, serverName string) (types.ToolResult, error) {
	requestBody, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "resources/read",
		"params":  map[string]any{"uri": resourceURI},
	})
	if err != nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadEncodeRequest, err)), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(requestBody))
	if err != nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadGenericError, err)), nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", resolveMCPBearer(ctx, baseURL))
	resp, err := sharedMCPHTTPClient.Do(req)
	if err != nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadGenericError, err)), nil
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadHTTPResponse, readErr)), nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		hint, _ := json.Marshal(map[string]any{
			"error":            "oauth_required",
			"server_url":       baseURL,
			"www_authenticate": resp.Header.Get("WWW-Authenticate"),
			"status":           resp.StatusCode,
			"message":          toolRuntimeText(i18n.KeyToolMCPReadOAuthRequired),
		})
		return types.ToolResult{Content: string(hint), IsError: true}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPHTTPError, resp.StatusCode, string(body))), nil
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data,omitempty"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadInvalidJSONRPC, err)), nil
	}
	if envelope.Error != nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadRPCFailed, envelope.Error.Code, envelope.Error.Message)), nil
	}
	if len(bytes.TrimSpace(envelope.Result)) == 0 {
		return mcpResourceErrorResult(toolRuntimeText(i18n.KeyToolMCPReadMissingResult)), nil
	}
	return renderMCPReadResourceToolResult(envelope.Result, serverName), nil
}
