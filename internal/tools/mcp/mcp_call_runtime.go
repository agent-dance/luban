package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/i18n"
	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
	"github.com/agent-dance/luban/types"
)

const (
	defaultMCPRuntimeToolTimeout = 100_000_000 * time.Millisecond
	maxMCPRuntimeSessionRetries  = 1
)

type dynamicMCPSessionRecoverer interface {
	RecoverExpiredSession(context.Context, string, error) (mcpmanager.MCPServerConnection, bool, error)
}

type dynamicMCPNeedsAuthMarker interface {
	MarkNeedsAuth(string, error) bool
}

type mcpCallRuntime struct {
	manager DynamicMCPManager
	server  string
	tool    string
}

func newMCPCallRuntime(manager DynamicMCPManager, serverName, toolName string) mcpCallRuntime {
	return mcpCallRuntime{
		manager: manager,
		server:  serverName,
		tool:    toolName,
	}
}

func (r mcpCallRuntime) execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.manager == nil {
		return mcpRuntimeErrorResult(toolRuntimeText(i18n.KeyToolRuntimeMCPDynamicNotInitialized), r.server, r.tool), nil
	}
	if input == nil {
		input = map[string]any{}
	}

	toolUseID := mcpToolUseIDFromContext(ctx)
	startedAt := time.Now()

	var sessionRetries int
	for attempt := 0; ; attempt++ {
		state, stateResult, err := r.ensureConnected(ctx)
		if err != nil || stateResult != nil {
			if stateResult != nil {
				addMCPRuntimeMetadata(stateResult, toolUseID, sessionRetries, time.Since(startedAt))
			}
			return derefToolResult(stateResult), err
		}

		raw, err := r.callRawOnce(ctx, state.Client, input)
		if err == nil {
			result := renderMCPCallToolResult(raw, r.server, r.tool)
			if result.IsError {
				if result.Metadata == nil {
					result.Metadata = map[string]string{}
				}
				result.Metadata["mcp.callError"] = "true"
				result.Metadata["mcp.errorClass"] = "tool_result_is_error"
			}
			addMCPRuntimeMetadata(&result, toolUseID, sessionRetries, time.Since(startedAt))
			return result, nil
		}
		if errors.Is(err, context.Canceled) || (ctx.Err() != nil && !errors.Is(err, context.DeadlineExceeded)) {
			return types.ToolResult{}, err
		}
		if mcpmanager.IsSessionExpiredError(err) && attempt < maxMCPRuntimeSessionRetries {
			if recovered, ok, recoverErr := r.recoverExpiredSession(ctx, err); ok {
				if recoverErr != nil {
					return mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPSessionRecoveryFailed, r.server, recoverErr), r.server, r.tool), nil
				}
				if recovered.Type == mcpmanager.MCPStateConnected && recovered.Client != nil {
					sessionRetries++
					continue
				}
				return mcpRuntimeStateErrorResult(recovered, r.server, r.tool), nil
			}
		}
		if mcpauth.IsRequiredError(err) {
			r.markNeedsAuth(err)
			result := mcpNeedsAuthToolResult(r.server, r.tool)
			addMCPRuntimeMetadata(&result, toolUseID, sessionRetries, time.Since(startedAt))
			return result, nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			result := mcpRuntimeTimeoutResult(r.server, r.tool, effectiveMCPRuntimeToolTimeout())
			addMCPRuntimeMetadata(&result, toolUseID, sessionRetries, time.Since(startedAt))
			return result, nil
		}

		result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPToolCallFailed, err), r.server, r.tool)
		addMCPRuntimeMetadata(&result, toolUseID, sessionRetries, time.Since(startedAt))
		return result, nil
	}
}

func (r mcpCallRuntime) ensureConnected(ctx context.Context) (mcpmanager.MCPServerConnection, *types.ToolResult, error) {
	state, err := r.manager.GetOrConnect(ctx, r.server)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return mcpmanager.MCPServerConnection{}, nil, err
		}
		result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerUnavailableReason, r.server, err), r.server, r.tool)
		return mcpmanager.MCPServerConnection{}, &result, nil
	}
	if state.Type != mcpmanager.MCPStateConnected || state.Client == nil {
		result := mcpRuntimeStateErrorResult(state, r.server, r.tool)
		return mcpmanager.MCPServerConnection{}, &result, nil
	}
	return state, nil, nil
}

func (r mcpCallRuntime) callRawOnce(ctx context.Context, client *mcptransport.Client, input map[string]any) (json.RawMessage, error) {
	callCtx, cancel := withMCPRuntimeToolTimeout(ctx)
	defer cancel()
	params := map[string]any{
		"name":      r.tool,
		"arguments": input,
	}
	var raw json.RawMessage
	if err := client.CallRaw(callCtx, "tools/call", params, &raw); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		raw = json.RawMessage(`{"content":[],"isError":false}`)
	}
	return raw, nil
}

func (r mcpCallRuntime) recoverExpiredSession(ctx context.Context, cause error) (mcpmanager.MCPServerConnection, bool, error) {
	recoverer, ok := r.manager.(dynamicMCPSessionRecoverer)
	if !ok {
		return mcpmanager.MCPServerConnection{}, false, nil
	}
	return recoverer.RecoverExpiredSession(ctx, r.server, cause)
}

func (r mcpCallRuntime) markNeedsAuth(cause error) {
	if marker, ok := r.manager.(dynamicMCPNeedsAuthMarker); ok {
		_ = marker.MarkNeedsAuth(r.server, cause)
	}
}

func withMCPRuntimeToolTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := effectiveMCPRuntimeToolTimeout()
	if existingDeadline, ok := ctx.Deadline(); ok && time.Until(existingDeadline) < timeout {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func effectiveMCPRuntimeToolTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MCP_TOOL_TIMEOUT"))
	if raw == "" {
		return defaultMCPRuntimeToolTimeout
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ms <= 0 {
		return defaultMCPRuntimeToolTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

func mcpToolUseIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if exec, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
		return strings.TrimSpace(exec.ToolUse.ID)
	}
	return ""
}

func mcpRuntimeStateErrorResult(state mcpmanager.MCPServerConnection, serverName, toolName string) types.ToolResult {
	if serverName == "" {
		serverName = state.Name
	}
	switch state.Type {
	case mcpmanager.MCPStateNeedsAuth:
		return mcpNeedsAuthToolResult(serverName, toolName)
	case mcpmanager.MCPStateDisabled:
		return mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerDisabled, serverName), serverName, toolName)
	case mcpmanager.MCPStateFailed:
		msg := state.Error
		if msg == "" {
			msg = toolRuntimeText(i18n.KeyToolRuntimeMCPFailedToConnect)
		}
		return mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerConnectFailed, serverName, msg), serverName, toolName)
	case mcpmanager.MCPStatePending:
		return mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerConnecting, serverName), serverName, toolName)
	default:
		return mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerNotConnected, serverName), serverName, toolName)
	}
}

func mcpNeedsAuthToolResult(serverName, toolName string) types.ToolResult {
	result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerNeedsAuth, serverName, catalog.BuildMCPToolName(serverName, "authenticate")), serverName, toolName)
	result.Metadata["mcp.auth"] = "needs-auth"
	return result
}

func mcpRuntimeTimeoutResult(serverName, toolName string, timeout time.Duration) types.ToolResult {
	result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPCallTimedOut, serverName, toolName, timeout), serverName, toolName)
	result.Metadata["mcp.timeout"] = "true"
	result.Metadata["mcp.timeoutMs"] = strconv.FormatInt(timeout.Milliseconds(), 10)
	result.Outcome = types.ToolOutcomeTimedOut
	return result
}

func mcpRuntimeErrorResult(message, serverName, toolName string) types.ToolResult {
	result := types.ToolResult{
		Content: message,
		IsError: true,
		Metadata: map[string]string{
			"mcp.serverName": serverName,
			"mcp.toolName":   toolName,
			"mcp.resultType": "runtimeError",
		},
	}
	if strings.TrimSpace(message) != "" {
		result.ContentBlocks = []types.ContentBlock{newMCPTextBlock(message)}
	}
	return result
}

func addMCPRuntimeMetadata(result *types.ToolResult, toolUseID string, sessionRetries int, elapsed time.Duration) {
	if result == nil {
		return
	}
	if result.Metadata == nil {
		result.Metadata = map[string]string{}
	}
	if strings.TrimSpace(toolUseID) != "" {
		result.Metadata["mcp.toolUseId"] = strings.TrimSpace(toolUseID)
	}
	result.Metadata["mcp.sessionRetries"] = strconv.Itoa(sessionRetries)
	result.Metadata["mcp.elapsedMs"] = strconv.FormatInt(elapsed.Milliseconds(), 10)
}

func derefToolResult(result *types.ToolResult) types.ToolResult {
	if result == nil {
		return types.ToolResult{}
	}
	return *result
}
