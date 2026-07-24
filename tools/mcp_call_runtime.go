package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/sdk"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
)

const (
	defaultMCPRuntimeToolTimeout     = 100_000_000 * time.Millisecond
	defaultMCPRuntimeHeartbeat       = 30 * time.Second
	maxMCPRuntimeSessionRetries      = 1
	maxMCPRuntimeURLElicitationRetry = 3
)

type dynamicMCPSessionRecoverer interface {
	RecoverExpiredSession(context.Context, string, error) (svcmcp.MCPServerConnection, bool, error)
}

type dynamicMCPNeedsAuthMarker interface {
	MarkNeedsAuth(string, error) bool
}

type mcpToolUseIDContextKey struct{}
type mcpProgressEmitterContextKey struct{}
type mcpURLElicitationHandlerContextKey struct{}

var (
	mcpRuntimeHeartbeatMu       sync.RWMutex
	mcpRuntimeHeartbeatInterval = defaultMCPRuntimeHeartbeat
)

// WithMCPToolUseID attaches the model tool_use id to an MCP runtime context.
// The loop context is also inspected, so this helper is mainly for tests and
// non-loop call sites.
func WithMCPToolUseID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, mcpToolUseIDContextKey{}, id)
}

// WithMCPProgressEmitter attaches the repo's SDK progress emitter to an MCP
// tool call. When no emitter is present the runtime still records heartbeat
// metadata on the final ToolResult.
func WithMCPProgressEmitter(ctx context.Context, emitter sdk.ProgressEmitter) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if emitter == nil {
		return ctx
	}
	return context.WithValue(ctx, mcpProgressEmitterContextKey{}, emitter)
}

// MCPURLElicitationHandler resolves URL elicitations raised by JSON-RPC
// -32042 errors. Returning action "accept" retries the tool call; "decline" or
// "cancel" returns a tool-level error result without retrying.
type MCPURLElicitationHandler interface {
	HandleMCPURLElicitation(ctx context.Context, serverName string, elicitation svcmcp.URLElicitation) (svcmcp.ElicitationResult, error)
}

// MCPURLElicitationHandlerFunc adapts a closure to MCPURLElicitationHandler.
type MCPURLElicitationHandlerFunc func(ctx context.Context, serverName string, elicitation svcmcp.URLElicitation) (svcmcp.ElicitationResult, error)

func (f MCPURLElicitationHandlerFunc) HandleMCPURLElicitation(ctx context.Context, serverName string, elicitation svcmcp.URLElicitation) (svcmcp.ElicitationResult, error) {
	if f == nil {
		return svcmcp.ElicitationResult{}, errors.New(toolRuntimeText(i18n.KeyToolRuntimeMCPURLElicitationHandlerNil))
	}
	return f(ctx, serverName, elicitation)
}

// WithMCPURLElicitationHandler attaches a URL elicitation resolver to a tool
// call. This is intentionally runtime-scoped so task_15 can later wire policy
// and permission hooks without changing this invocation layer.
func WithMCPURLElicitationHandler(ctx context.Context, handler MCPURLElicitationHandler) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if handler == nil {
		return ctx
	}
	return context.WithValue(ctx, mcpURLElicitationHandlerContextKey{}, handler)
}

// SetMCPRuntimeHeartbeatInterval overrides the long-running call heartbeat
// interval. It is exported for focused tests; production keeps the TS-like 30s.
func SetMCPRuntimeHeartbeatInterval(d time.Duration) {
	mcpRuntimeHeartbeatMu.Lock()
	defer mcpRuntimeHeartbeatMu.Unlock()
	if d <= 0 {
		mcpRuntimeHeartbeatInterval = defaultMCPRuntimeHeartbeat
		return
	}
	mcpRuntimeHeartbeatInterval = d
}

type mcpCallRuntime struct {
	manager   DynamicMCPManager
	server    string
	tool      string
	modelName string
}

func newMCPCallRuntime(manager DynamicMCPManager, serverName, toolName, modelName string) mcpCallRuntime {
	return mcpCallRuntime{
		manager:   manager,
		server:    serverName,
		tool:      toolName,
		modelName: modelName,
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
	progress := newMCPRuntimeProgress(ctx, r.modelName, r.server, r.tool, toolUseID)
	progress.started()
	defer progress.stop()

	var sessionRetries int
	for attempt := 0; ; attempt++ {
		state, stateResult, err := r.ensureConnected(ctx)
		if err != nil || stateResult != nil {
			progress.failed()
			if stateResult != nil {
				addMCPRuntimeMetadata(stateResult, toolUseID, sessionRetries, progress.heartbeats(), time.Since(progress.startedAt))
			}
			return derefToolResult(stateResult), err
		}

		raw, toolResult, err := r.callWithURLElicitationRetry(ctx, state.Client, input, toolUseID)
		if err == nil && toolResult == nil {
			result := renderMCPCallToolResult(raw, r.server, r.tool)
			if result.IsError {
				progress.failed()
				if result.Metadata == nil {
					result.Metadata = map[string]string{}
				}
				result.Metadata["mcp.callError"] = "true"
				result.Metadata["mcp.errorClass"] = "tool_result_is_error"
			} else {
				progress.completed()
			}
			addMCPRuntimeMetadata(&result, toolUseID, sessionRetries, progress.heartbeats(), time.Since(progress.startedAt))
			return result, nil
		}
		if toolResult != nil {
			progress.failed()
			addMCPRuntimeMetadata(toolResult, toolUseID, sessionRetries, progress.heartbeats(), time.Since(progress.startedAt))
			return *toolResult, nil
		}
		if err == nil {
			err = errors.New(toolRuntimeText(i18n.KeyToolRuntimeMCPToolCallFailedPlain))
		}
		if errors.Is(err, context.Canceled) || (ctx.Err() != nil && !errors.Is(err, context.DeadlineExceeded)) {
			progress.failed()
			return types.ToolResult{}, err
		}
		if svcmcp.IsSessionExpiredError(err) && attempt < maxMCPRuntimeSessionRetries {
			if recovered, ok, recoverErr := r.recoverExpiredSession(ctx, err); ok {
				if recoverErr != nil {
					progress.failed()
					return mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPSessionRecoveryFailed, r.server, recoverErr), r.server, r.tool), nil
				}
				if recovered.Type == svcmcp.MCPStateConnected && recovered.Client != nil {
					sessionRetries++
					continue
				}
				progress.failed()
				return mcpRuntimeStateErrorResult(recovered, r.server, r.tool), nil
			}
		}
		if svcmcp.IsAuthRequiredError(err) {
			r.markNeedsAuth(err)
			progress.failed()
			result := mcpNeedsAuthToolResult(r.server, r.tool)
			addMCPRuntimeMetadata(&result, toolUseID, sessionRetries, progress.heartbeats(), time.Since(progress.startedAt))
			return result, nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			progress.failed()
			result := mcpRuntimeTimeoutResult(r.server, r.tool, effectiveMCPRuntimeToolTimeout())
			addMCPRuntimeMetadata(&result, toolUseID, sessionRetries, progress.heartbeats(), time.Since(progress.startedAt))
			return result, nil
		}

		progress.failed()
		result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPToolCallFailed, err), r.server, r.tool)
		addMCPRuntimeMetadata(&result, toolUseID, sessionRetries, progress.heartbeats(), time.Since(progress.startedAt))
		return result, nil
	}
}

func (r mcpCallRuntime) ensureConnected(ctx context.Context) (svcmcp.MCPServerConnection, *types.ToolResult, error) {
	state, err := r.manager.GetOrConnect(ctx, r.server)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return svcmcp.MCPServerConnection{}, nil, err
		}
		result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerUnavailableReason, r.server, err), r.server, r.tool)
		return svcmcp.MCPServerConnection{}, &result, nil
	}
	if state.Type != svcmcp.MCPStateConnected || state.Client == nil {
		result := mcpRuntimeStateErrorResult(state, r.server, r.tool)
		return svcmcp.MCPServerConnection{}, &result, nil
	}
	return state, nil, nil
}

func (r mcpCallRuntime) callWithURLElicitationRetry(ctx context.Context, client *svcmcp.Client, input map[string]any, toolUseID string) (json.RawMessage, *types.ToolResult, error) {
	if client == nil {
		result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesNoActiveClient, r.server), r.server, r.tool)
		return nil, &result, nil
	}
	for attempt := 0; ; attempt++ {
		raw, err := r.callRawOnce(ctx, client, input, toolUseID)
		if err == nil {
			return raw, nil, nil
		}
		elicitations, ok := svcmcp.ExtractURLElicitations(err)
		if !ok {
			return nil, nil, err
		}
		if attempt >= maxMCPRuntimeURLElicitationRetry {
			result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPURLElicitationRetryLimit, r.tool), r.server, r.tool)
			result.Metadata["mcp.urlElicitation"] = "retry_limit"
			return nil, &result, nil
		}
		if len(elicitations) == 0 {
			result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPURLElicitationInvalid, r.tool), r.server, r.tool)
			result.Metadata["mcp.urlElicitation"] = "invalid"
			return nil, &result, nil
		}
		result, handled, handleErr := r.handleURLElicitations(ctx, elicitations)
		if handleErr != nil {
			return nil, nil, handleErr
		}
		if handled {
			return nil, result, nil
		}
	}
}

func (r mcpCallRuntime) callRawOnce(ctx context.Context, client *svcmcp.Client, input map[string]any, toolUseID string) (json.RawMessage, error) {
	callCtx, cancel := withMCPRuntimeToolTimeout(ctx)
	defer cancel()
	params := map[string]any{
		"name":      r.tool,
		"arguments": input,
	}
	if strings.TrimSpace(toolUseID) != "" {
		params["_meta"] = map[string]any{"claudecode/toolUseId": toolUseID}
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

func (r mcpCallRuntime) handleURLElicitations(ctx context.Context, elicitations []svcmcp.URLElicitation) (*types.ToolResult, bool, error) {
	handler := mcpURLElicitationHandlerFromContext(ctx)
	if handler == nil {
		result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPURLElicitationRequired, r.tool), r.server, r.tool)
		result.Metadata["mcp.urlElicitation"] = "required"
		result.Metadata["mcp.urlElicitationCount"] = strconv.Itoa(len(elicitations))
		return &result, true, nil
	}
	for _, elicitation := range elicitations {
		response, err := handler.HandleMCPURLElicitation(ctx, r.server, elicitation)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, false, err
			}
			result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPURLElicitationFailed, r.tool, err), r.server, r.tool)
			result.Metadata["mcp.urlElicitation"] = "handler_error"
			return &result, true, nil
		}
		action := normalizeMCPElicitationAction(response.Action)
		if action != "accept" {
			verb := toolRuntimeText(i18n.KeyToolRuntimeMCPURLElicitationCanceled)
			if action == "decline" {
				verb = toolRuntimeText(i18n.KeyToolRuntimeMCPURLElicitationDeclined)
			}
			result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPURLElicitationRejected, verb, r.tool), r.server, r.tool)
			result.Metadata["mcp.urlElicitation"] = action
			result.Metadata["mcp.urlElicitationId"] = elicitation.ElicitationID
			return &result, true, nil
		}
	}
	return nil, false, nil
}

func (r mcpCallRuntime) recoverExpiredSession(ctx context.Context, cause error) (svcmcp.MCPServerConnection, bool, error) {
	recoverer, ok := r.manager.(dynamicMCPSessionRecoverer)
	if !ok {
		return svcmcp.MCPServerConnection{}, false, nil
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

type mcpRuntimeProgress struct {
	emitter   sdk.ProgressEmitter
	toolUseID string
	toolName  string
	server    string
	tool      string
	startedAt time.Time

	done chan struct{}
	once sync.Once

	mu             sync.Mutex
	heartbeatCount int
}

func newMCPRuntimeProgress(ctx context.Context, modelName, serverName, toolName, toolUseID string) *mcpRuntimeProgress {
	if modelName == "" {
		modelName = svcmcp.BuildMCPToolName(serverName, toolName)
	}
	return &mcpRuntimeProgress{
		emitter:   mcpProgressEmitterFromContext(ctx),
		toolUseID: toolUseID,
		toolName:  modelName,
		server:    serverName,
		tool:      toolName,
		startedAt: time.Now(),
		done:      make(chan struct{}),
	}
}

func (p *mcpRuntimeProgress) started() {
	if p == nil {
		return
	}
	p.emit("started", 0, toolRuntimeText(i18n.KeyToolRuntimeMCPProgressStarted))
	interval := currentMCPRuntimeHeartbeatInterval()
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.mu.Lock()
				p.heartbeatCount++
				n := p.heartbeatCount
				p.mu.Unlock()
				p.emit("running", -1, toolRuntimeFormat(i18n.KeyToolRuntimeMCPProgressHeartbeat, n))
			case <-p.done:
				return
			}
		}
	}()
}

func (p *mcpRuntimeProgress) completed() {
	if p == nil {
		return
	}
	p.emit("completed", 1, toolRuntimeText(i18n.KeyToolRuntimeMCPProgressCompleted))
}

func (p *mcpRuntimeProgress) failed() {
	if p == nil {
		return
	}
	p.emit("error", -1, toolRuntimeText(i18n.KeyToolRuntimeMCPProgressFailed))
}

func (p *mcpRuntimeProgress) stop() {
	if p == nil {
		return
	}
	p.once.Do(func() { close(p.done) })
}

func (p *mcpRuntimeProgress) heartbeats() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.heartbeatCount
}

func (p *mcpRuntimeProgress) emit(status string, progress float64, message string) {
	if p == nil || p.emitter == nil {
		return
	}
	if strings.TrimSpace(p.toolUseID) != "" {
		message = strings.TrimSpace(message + " tool_use_id=" + p.toolUseID)
	}
	p.emitter.Emit(sdk.ToolProgressEvent{
		ToolName: p.toolName,
		Status:   status,
		Progress: progress,
		Message:  toolRuntimeFormat(i18n.KeyToolRuntimeMCPProgressMessage, p.server, p.tool, message),
	})
}

func currentMCPRuntimeHeartbeatInterval() time.Duration {
	mcpRuntimeHeartbeatMu.RLock()
	defer mcpRuntimeHeartbeatMu.RUnlock()
	return mcpRuntimeHeartbeatInterval
}

func mcpToolUseIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(mcpToolUseIDContextKey{}).(string); ok && strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	if exec, ok := loop.ToolExecutionContextFromContext(ctx); ok {
		return strings.TrimSpace(exec.ToolUse.ID)
	}
	return ""
}

func mcpProgressEmitterFromContext(ctx context.Context) sdk.ProgressEmitter {
	if ctx == nil {
		return nil
	}
	emitter, _ := ctx.Value(mcpProgressEmitterContextKey{}).(sdk.ProgressEmitter)
	return emitter
}

func mcpURLElicitationHandlerFromContext(ctx context.Context) MCPURLElicitationHandler {
	if ctx == nil {
		return nil
	}
	handler, _ := ctx.Value(mcpURLElicitationHandlerContextKey{}).(MCPURLElicitationHandler)
	return handler
}

func normalizeMCPElicitationAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "accept", "accepted":
		return "accept"
	case "decline", "declined":
		return "decline"
	default:
		return "cancel"
	}
}

func mcpRuntimeStateErrorResult(state svcmcp.MCPServerConnection, serverName, toolName string) types.ToolResult {
	if serverName == "" {
		serverName = state.Name
	}
	switch state.Type {
	case svcmcp.MCPStateNeedsAuth:
		return mcpNeedsAuthToolResult(serverName, toolName)
	case svcmcp.MCPStateDisabled:
		return mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerDisabled, serverName), serverName, toolName)
	case svcmcp.MCPStateFailed:
		msg := state.Error
		if msg == "" {
			msg = toolRuntimeText(i18n.KeyToolRuntimeMCPFailedToConnect)
		}
		return mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerConnectFailed, serverName, msg), serverName, toolName)
	case svcmcp.MCPStatePending:
		return mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerConnecting, serverName), serverName, toolName)
	default:
		return mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerNotConnected, serverName), serverName, toolName)
	}
}

func mcpNeedsAuthToolResult(serverName, toolName string) types.ToolResult {
	result := mcpRuntimeErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPServerNeedsAuth, serverName, svcmcp.BuildMCPToolName(serverName, "authenticate")), serverName, toolName)
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
		result.ContentBlocks = []types.ContentBlock{newTextBlock(message)}
	}
	return result
}

func addMCPRuntimeMetadata(result *types.ToolResult, toolUseID string, sessionRetries, heartbeats int, elapsed time.Duration) {
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
	result.Metadata["mcp.progressHeartbeats"] = strconv.Itoa(heartbeats)
	result.Metadata["mcp.elapsedMs"] = strconv.FormatInt(elapsed.Milliseconds(), 10)
}

func derefToolResult(result *types.ToolResult) types.ToolResult {
	if result == nil {
		return types.ToolResult{}
	}
	return *result
}
