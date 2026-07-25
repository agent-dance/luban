package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
	"github.com/agent-dance/luban/internal/mcp/protocol"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
	"github.com/agent-dance/luban/types"
)

type runtimeMCPFakeManager struct {
	mu               sync.Mutex
	state            mcpmanager.MCPServerConnection
	recovered        mcpmanager.MCPServerConnection
	err              error
	getCalls         int
	recoverCalls     int
	markNeedsAuth    int
	lastNeedsAuthErr error
}

func (m *runtimeMCPFakeManager) Snapshot() []mcpmanager.MCPServerConnection {
	m.mu.Lock()
	defer m.mu.Unlock()
	return []mcpmanager.MCPServerConnection{m.state}
}

func (m *runtimeMCPFakeManager) GetOrConnect(ctx context.Context, name string) (mcpmanager.MCPServerConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCalls++
	if m.err != nil {
		return mcpmanager.MCPServerConnection{}, m.err
	}
	if err := ctx.Err(); err != nil {
		return mcpmanager.MCPServerConnection{}, err
	}
	m.state.Name = name
	return m.state, nil
}

func (m *runtimeMCPFakeManager) RecoverExpiredSession(ctx context.Context, name string, cause error) (mcpmanager.MCPServerConnection, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recoverCalls++
	if !mcpmanager.IsSessionExpiredError(cause) {
		return mcpmanager.MCPServerConnection{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return mcpmanager.MCPServerConnection{}, true, err
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
	m.state.Type = mcpmanager.MCPStateNeedsAuth
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

func TestMCPDynamicToolRuntimeCallDoesNotLeakPrivateMetaAndPreservesResultMetadata(t *testing.T) {
	raw := &runtimeMCPRawCaller{results: []json.RawMessage{
		json.RawMessage(`{"content":[{"type":"text","text":"ok"}],"structuredContent":{"count":1},"_meta":{"trace":"abc"},"isError":false}`),
	}}
	manager := &runtimeMCPFakeManager{state: mcpmanager.MCPServerConnection{
		Name:   "srv",
		Type:   mcpmanager.MCPStateConnected,
		Client: newMCPProtocolTestClient(t, raw),
	}}
	tool := NewDynamicMCPTool(manager, "srv", catalog.ToolDefinition{Name: "Search Issues"})

	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		ToolUse: types.ToolUseBlock{ID: "toolu_123"},
	})
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
	if _, ok := params["_meta"]; ok {
		t.Fatalf("tools/call params leaked private _meta: %#v", params["_meta"])
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
	manager := &runtimeMCPFakeManager{state: mcpmanager.MCPServerConnection{
		Name:   "srv",
		Type:   mcpmanager.MCPStateConnected,
		Client: newMCPProtocolTestClient(t, raw),
	}}
	tool := NewDynamicMCPTool(manager, "srv", catalog.ToolDefinition{Name: "write"})

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
	expired := &mcpmanager.SessionExpiredError{
		ServerName: "srv",
		Err: &mcptransport.RemoteHTTPError{
			StatusCode: 404,
			RPCError:   &protocol.RPCError{Code: -32001, Message: "Session not found"},
		},
	}
	oldRaw := &runtimeMCPRawCaller{errs: []error{expired}}
	newRaw := &runtimeMCPRawCaller{results: []json.RawMessage{
		json.RawMessage(`{"content":[{"type":"text","text":"fresh"}]}`),
	}}
	manager := &runtimeMCPFakeManager{
		state: mcpmanager.MCPServerConnection{
			Name:   "srv",
			Type:   mcpmanager.MCPStateConnected,
			Client: newMCPProtocolTestClient(t, oldRaw),
		},
		recovered: mcpmanager.MCPServerConnection{
			Name:   "srv",
			Type:   mcpmanager.MCPStateConnected,
			Client: newMCPProtocolTestClient(t, newRaw),
		},
	}
	tool := NewDynamicMCPTool(manager, "srv", catalog.ToolDefinition{Name: "lookup"})

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
	manager := &runtimeMCPFakeManager{state: mcpmanager.MCPServerConnection{
		Name:   "srv",
		Type:   mcpmanager.MCPStateConnected,
		Client: newMCPProtocolTestClient(t, raw),
	}}
	tool := NewDynamicMCPTool(manager, "srv", catalog.ToolDefinition{Name: "slow"})

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

func TestMCPDynamicToolRuntimeAuthErrorMarksNeedsAuth(t *testing.T) {
	raw := &runtimeMCPRawCaller{errs: []error{&mcpauth.UnauthorizedError{ServerURL: "https://example.test/mcp", StatusCode: 401}}}
	manager := &runtimeMCPFakeManager{state: mcpmanager.MCPServerConnection{
		Name:   "srv",
		Type:   mcpmanager.MCPStateConnected,
		Client: newMCPProtocolTestClient(t, raw),
	}}
	tool := NewDynamicMCPTool(manager, "srv", catalog.ToolDefinition{Name: "lookup"})

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

var _ types.Tool = (*DynamicMCPTool)(nil)
