package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/sdk"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
)

type runtimeMCPFakeManager struct {
	mu               sync.Mutex
	state            svcmcp.MCPServerConnection
	recovered        svcmcp.MCPServerConnection
	err              error
	getCalls         int
	recoverCalls     int
	markNeedsAuth    int
	lastNeedsAuthErr error
}

func (m *runtimeMCPFakeManager) Snapshot() []svcmcp.MCPServerConnection {
	m.mu.Lock()
	defer m.mu.Unlock()
	return []svcmcp.MCPServerConnection{m.state}
}

func (m *runtimeMCPFakeManager) GetOrConnect(ctx context.Context, name string) (svcmcp.MCPServerConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCalls++
	if m.err != nil {
		return svcmcp.MCPServerConnection{}, m.err
	}
	if err := ctx.Err(); err != nil {
		return svcmcp.MCPServerConnection{}, err
	}
	m.state.Name = name
	return m.state, nil
}

func (m *runtimeMCPFakeManager) RecoverExpiredSession(ctx context.Context, name string, cause error) (svcmcp.MCPServerConnection, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recoverCalls++
	if !svcmcp.IsSessionExpiredError(cause) {
		return svcmcp.MCPServerConnection{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return svcmcp.MCPServerConnection{}, true, err
	}
	m.recovered.Name = name
	m.state = m.recovered
	return m.recovered, true, nil
}

func (m *runtimeMCPFakeManager) MarkNeedsAuth(_ string, err error) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markNeedsAuth++
	m.lastNeedsAuthErr = err
	m.state.Type = svcmcp.MCPStateNeedsAuth
	return true
}

type runtimeMCPRawCaller struct {
	mu      sync.Mutex
	results []json.RawMessage
	errs    []error
	delay   time.Duration
	block   bool
	params  []map[string]any
}

func (r *runtimeMCPRawCaller) CallRaw(ctx context.Context, method string, params any, out any) error {
	if method != "tools/call" {
		return errors.New("unexpected method " + method)
	}
	data, _ := json.Marshal(params)
	var decoded map[string]any
	_ = json.Unmarshal(data, &decoded)
	r.mu.Lock()
	r.params = append(r.params, decoded)
	idx := len(r.params) - 1
	r.mu.Unlock()

	if r.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if r.delay > 0 {
		select {
		case <-time.After(r.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if idx < len(r.errs) && r.errs[idx] != nil {
		return r.errs[idx]
	}
	raw := json.RawMessage(`{"content":[]}`)
	if idx < len(r.results) && len(r.results[idx]) > 0 {
		raw = append(json.RawMessage(nil), r.results[idx]...)
	}
	switch target := out.(type) {
	case *json.RawMessage:
		*target = append(json.RawMessage(nil), raw...)
	default:
		return json.Unmarshal(raw, target)
	}
	return nil
}

func (r *runtimeMCPRawCaller) recordedParams(i int) map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i < 0 || i >= len(r.params) {
		return nil
	}
	return r.params[i]
}

func TestMCPDynamicToolRuntimeCallMetaEmptyArgsAndTypedResult(t *testing.T) {
	raw := &runtimeMCPRawCaller{results: []json.RawMessage{
		json.RawMessage(`{"content":[{"type":"text","text":"ok"}],"structuredContent":{"count":1},"_meta":{"trace":"abc"},"isError":false}`),
	}}
	manager := &runtimeMCPFakeManager{state: svcmcp.MCPServerConnection{
		Name:   "srv",
		Type:   svcmcp.MCPStateConnected,
		Client: svcmcp.NewClient(raw, nil),
	}}
	tool := NewDynamicMCPTool(manager, "srv", svcmcp.ToolDefinition{Name: "Search Issues"})

	ctx := WithMCPToolUseID(context.Background(), "toolu_123")
	result, err := tool.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	params := raw.recordedParams(0)
	if params["name"] != "Search Issues" {
		t.Fatalf("tools/call name = %#v, want original tool name", params["name"])
	}
	args, ok := params["arguments"].(map[string]any)
	if !ok || len(args) != 0 {
		t.Fatalf("arguments = %#v, want empty object", params["arguments"])
	}
	meta, ok := params["_meta"].(map[string]any)
	if !ok || meta["claudecode/toolUseId"] != "toolu_123" {
		t.Fatalf("_meta = %#v, want claudecode/toolUseId", params["_meta"])
	}
	if result.IsError || !strings.Contains(result.TextContent(), "ok") {
		t.Fatalf("result = %+v, want successful typed result", result)
	}
	if result.Metadata["mcp._meta"] != `{"trace":"abc"}` {
		t.Fatalf("mcp._meta metadata = %#v", result.Metadata)
	}
	if result.Metadata["mcp.structuredContent"] != `{"count":1}` {
		t.Fatalf("structuredContent metadata = %#v", result.Metadata)
	}
	if result.Metadata["mcp.toolUseId"] != "toolu_123" {
		t.Fatalf("runtime toolUseId metadata missing: %#v", result.Metadata)
	}
}

func TestMCPDynamicToolRuntimeIsErrorPreservesMeta(t *testing.T) {
	raw := &runtimeMCPRawCaller{results: []json.RawMessage{
		json.RawMessage(`{"content":[{"type":"text","text":"bad request"}],"_meta":{"retry":false},"isError":true}`),
	}}
	manager := &runtimeMCPFakeManager{state: svcmcp.MCPServerConnection{
		Name:   "srv",
		Type:   svcmcp.MCPStateConnected,
		Client: svcmcp.NewClient(raw, nil),
	}}
	tool := NewDynamicMCPTool(manager, "srv", svcmcp.ToolDefinition{Name: "write"})

	result, err := tool.Execute(context.Background(), map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if !result.IsError || !strings.Contains(result.TextContent(), "bad request") {
		t.Fatalf("result = %+v, want tool-level error", result)
	}
	if result.Metadata["mcp._meta"] != `{"retry":false}` || result.Metadata["mcp.callError"] != "true" {
		t.Fatalf("error metadata did not preserve _meta/callError: %#v", result.Metadata)
	}
}

func TestMCPDynamicToolRuntimeSessionExpiredRetriesOnce(t *testing.T) {
	expired := &svcmcp.SessionExpiredError{
		ServerName: "srv",
		Err: &svcmcp.RemoteHTTPError{
			StatusCode: 404,
			RPCError:   &svcmcp.RPCError{Code: -32001, Message: "Session not found"},
		},
	}
	oldRaw := &runtimeMCPRawCaller{errs: []error{expired}}
	newRaw := &runtimeMCPRawCaller{results: []json.RawMessage{
		json.RawMessage(`{"content":[{"type":"text","text":"fresh"}]}`),
	}}
	manager := &runtimeMCPFakeManager{
		state: svcmcp.MCPServerConnection{
			Name:   "srv",
			Type:   svcmcp.MCPStateConnected,
			Client: svcmcp.NewClient(oldRaw, nil),
		},
		recovered: svcmcp.MCPServerConnection{
			Name:   "srv",
			Type:   svcmcp.MCPStateConnected,
			Client: svcmcp.NewClient(newRaw, nil),
		},
	}
	tool := NewDynamicMCPTool(manager, "srv", svcmcp.ToolDefinition{Name: "lookup"})

	result, err := tool.Execute(context.Background(), map[string]any{"q": "x"})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if result.IsError || !strings.Contains(result.TextContent(), "fresh") {
		t.Fatalf("result = %+v, want retried success", result)
	}
	if manager.recoverCalls != 1 {
		t.Fatalf("RecoverExpiredSession calls = %d, want 1", manager.recoverCalls)
	}
	if result.Metadata["mcp.sessionRetries"] != "1" {
		t.Fatalf("session retry metadata = %#v", result.Metadata)
	}
}

func TestMCPDynamicToolRuntimeTimeoutAndContextCancel(t *testing.T) {
	t.Setenv("MCP_TOOL_TIMEOUT", "20")
	raw := &runtimeMCPRawCaller{block: true}
	manager := &runtimeMCPFakeManager{state: svcmcp.MCPServerConnection{
		Name:   "srv",
		Type:   svcmcp.MCPStateConnected,
		Client: svcmcp.NewClient(raw, nil),
	}}
	tool := NewDynamicMCPTool(manager, "srv", svcmcp.ToolDefinition{Name: "slow"})

	start := time.Now()
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("timeout should be a tool-level error, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timeout took too long")
	}
	if !result.IsError || result.Metadata["mcp.timeout"] != "true" {
		t.Fatalf("timeout result = %+v", result)
	}
	if result.Outcome != types.ToolOutcomeTimedOut {
		t.Fatalf("timeout outcome = %q, want %q", result.Outcome, types.ToolOutcomeTimedOut)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = tool.Execute(cancelled, map[string]any{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Execute error = %v, want context.Canceled", err)
	}
}

func TestMCPDynamicToolRuntimeURLElicitationAcceptDeclineCancel(t *testing.T) {
	elicitationErr := &svcmcp.RPCError{
		Code:    svcmcp.ErrorCodeURLElicitationRequired,
		Message: "open url",
		Data:    json.RawMessage(`{"elicitations":[{"mode":"url","url":"https://example.test/login","elicitationId":"e1","message":"Open login"}]}`),
	}

	t.Run("accept retries", func(t *testing.T) {
		raw := &runtimeMCPRawCaller{
			errs: []error{elicitationErr},
			results: []json.RawMessage{
				nil,
				json.RawMessage(`{"content":[{"type":"text","text":"after accept"}]}`),
			},
		}
		manager := &runtimeMCPFakeManager{state: svcmcp.MCPServerConnection{Name: "srv", Type: svcmcp.MCPStateConnected, Client: svcmcp.NewClient(raw, nil)}}
		tool := NewDynamicMCPTool(manager, "srv", svcmcp.ToolDefinition{Name: "login"})
		var seen svcmcp.URLElicitation
		ctx := WithMCPURLElicitationHandler(context.Background(), MCPURLElicitationHandlerFunc(func(_ context.Context, serverName string, elicitation svcmcp.URLElicitation) (svcmcp.ElicitationResult, error) {
			if serverName != "srv" {
				t.Fatalf("serverName = %q", serverName)
			}
			seen = elicitation
			return svcmcp.ElicitationResult{Action: "accept"}, nil
		}))

		result, err := tool.Execute(ctx, map[string]any{})
		if err != nil {
			t.Fatalf("Execute error = %v", err)
		}
		if seen.ElicitationID != "e1" || seen.URL == "" {
			t.Fatalf("handler saw elicitation = %+v", seen)
		}
		if result.IsError || !strings.Contains(result.TextContent(), "after accept") {
			t.Fatalf("result = %+v, want retried success", result)
		}
		if len(raw.params) != 2 {
			t.Fatalf("tools/call count = %d, want 2", len(raw.params))
		}
	})

	for _, action := range []string{"decline", "cancel"} {
		t.Run(action, func(t *testing.T) {
			raw := &runtimeMCPRawCaller{errs: []error{elicitationErr}}
			manager := &runtimeMCPFakeManager{state: svcmcp.MCPServerConnection{Name: "srv", Type: svcmcp.MCPStateConnected, Client: svcmcp.NewClient(raw, nil)}}
			tool := NewDynamicMCPTool(manager, "srv", svcmcp.ToolDefinition{Name: "login"})
			ctx := WithMCPURLElicitationHandler(context.Background(), MCPURLElicitationHandlerFunc(func(context.Context, string, svcmcp.URLElicitation) (svcmcp.ElicitationResult, error) {
				return svcmcp.ElicitationResult{Action: action}, nil
			}))
			result, err := tool.Execute(ctx, map[string]any{})
			if err != nil {
				t.Fatalf("Execute error = %v", err)
			}
			if !result.IsError || result.Metadata["mcp.urlElicitation"] != action {
				t.Fatalf("result = %+v, want %s URL elicitation error", result, action)
			}
			if len(raw.params) != 1 {
				t.Fatalf("tools/call count = %d, want no retry", len(raw.params))
			}
		})
	}
}

func TestMCPDynamicToolRuntimeAuthErrorMarksNeedsAuth(t *testing.T) {
	raw := &runtimeMCPRawCaller{errs: []error{&svcmcp.UnauthorizedError{ServerURL: "https://example.test/mcp", StatusCode: 401}}}
	manager := &runtimeMCPFakeManager{state: svcmcp.MCPServerConnection{
		Name:   "srv",
		Type:   svcmcp.MCPStateConnected,
		Client: svcmcp.NewClient(raw, nil),
	}}
	tool := NewDynamicMCPTool(manager, "srv", svcmcp.ToolDefinition{Name: "lookup"})

	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if !result.IsError || result.Metadata["mcp.auth"] != "needs-auth" || !strings.Contains(result.Content, "authenticate") {
		t.Fatalf("auth result = %+v", result)
	}
	if manager.markNeedsAuth != 1 || manager.lastNeedsAuthErr == nil {
		t.Fatalf("MarkNeedsAuth calls=%d err=%v, want one auth mark", manager.markNeedsAuth, manager.lastNeedsAuthErr)
	}
}

func TestMCPDynamicToolRuntimeProgressHeartbeat(t *testing.T) {
	defer SetMCPRuntimeHeartbeatInterval(0)
	SetMCPRuntimeHeartbeatInterval(5 * time.Millisecond)
	raw := &runtimeMCPRawCaller{
		delay: 25 * time.Millisecond,
		results: []json.RawMessage{
			json.RawMessage(`{"content":[{"type":"text","text":"done"}]}`),
		},
	}
	manager := &runtimeMCPFakeManager{state: svcmcp.MCPServerConnection{
		Name:   "srv",
		Type:   svcmcp.MCPStateConnected,
		Client: svcmcp.NewClient(raw, nil),
	}}
	tool := NewDynamicMCPTool(manager, "srv", svcmcp.ToolDefinition{Name: "slow"})
	emitter := sdk.NewProgressEmitter(16)
	ctx := WithMCPProgressEmitter(WithMCPToolUseID(context.Background(), "toolu_progress"), emitter)

	result, err := tool.Execute(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if result.IsError {
		t.Fatalf("result = %+v", result)
	}
	events := drainMCPProgressEvents(emitter.Events())
	if !hasMCPProgressStatus(events, "started") || !hasMCPProgressStatus(events, "running") || !hasMCPProgressStatus(events, "completed") {
		t.Fatalf("progress events = %#v, want started/running/completed", events)
	}
	if result.Metadata["mcp.progressHeartbeats"] == "0" {
		t.Fatalf("expected heartbeat metadata, got %#v", result.Metadata)
	}
}

func drainMCPProgressEvents(ch <-chan sdk.ToolProgressEvent) []sdk.ToolProgressEvent {
	var out []sdk.ToolProgressEvent
	for {
		select {
		case evt := <-ch:
			out = append(out, evt)
		default:
			return out
		}
	}
}

func hasMCPProgressStatus(events []sdk.ToolProgressEvent, status string) bool {
	for _, evt := range events {
		if evt.Status == status {
			return true
		}
	}
	return false
}

var _ types.Tool = (*DynamicMCPTool)(nil)
