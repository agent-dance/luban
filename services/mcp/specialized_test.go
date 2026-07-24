package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
)

func TestSDKControlTransportConnectsThroughRegisteredBridge(t *testing.T) {
	var sawServerName string
	cleanup := RegisterSDKMessageSender("sdk-server", SDKMessageSenderFunc(func(ctx context.Context, serverName string, msg JSONRPCMessage) (*JSONRPCMessage, error) {
		sawServerName = serverName
		return specializedResponseForMessage(t, msg), nil
	}))
	defer cleanup()

	manager := NewManager()
	defer manager.Shutdown(context.Background())
	manager.AddConfig("sdk-server", MCPServerConfig{Type: TransportSDK, Name: "claude-vscode"})

	state, err := manager.GetOrConnect(context.Background(), "sdk-server")
	if err != nil {
		t.Fatalf("GetOrConnect sdk: %v", err)
	}
	if state.Type != MCPStateConnected {
		t.Fatalf("sdk state = %#v, want connected", state)
	}
	if sawServerName != "sdk-server" {
		t.Fatalf("SDK sender serverName = %q", sawServerName)
	}
	if len(state.Tools) != 1 || state.Tools[0].Name != "sdkTool" {
		t.Fatalf("sdk tools = %#v", state.Tools)
	}
}

func TestSDKTransportWithoutBridgeFailsClosed(t *testing.T) {
	manager := NewManager()
	defer manager.Shutdown(context.Background())
	manager.AddConfig("sdk-missing", MCPServerConfig{Type: TransportSDK, Name: "missing-sdk"})

	state, err := manager.GetOrConnect(context.Background(), "sdk-missing")
	if err != nil {
		t.Fatalf("GetOrConnect: %v", err)
	}
	if state.Type != MCPStateFailed {
		t.Fatalf("state = %#v, want failed", state)
	}
	want := mcpFormat(i18n.KeyMCPIntegrationUnsupportedServer, "sdk-missing", TransportSDK, mcpText(i18n.KeyMCPSDKBridgeMissing))
	if state.Error != want {
		t.Fatalf("unexpected SDK unsupported error: %q", state.Error)
	}
}

func TestIDETransportHeadersAndToolFiltering(t *testing.T) {
	var gotIDEAuth atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIDEAuth.Store(r.Header.Get(ideAuthorizationHeader))
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
			return
		}
		var msg JSONRPCMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		switch msg.Method {
		case "initialize":
			writeRPCResult(t, w, msg.ID, map[string]any{
				"protocolVersion": MCPProtocolVersion,
				"capabilities":    ServerCapabilities{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "ide", "version": "1"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeRPCResult(t, w, msg.ID, map[string]any{"tools": []map[string]any{
				{"name": "executeCode", "inputSchema": map[string]any{"type": "object"}},
				{"name": "getDiagnostics", "inputSchema": map[string]any{"type": "object"}},
				{"name": "dangerousInternal", "inputSchema": map[string]any{"type": "object"}},
			}})
		default:
			t.Errorf("unexpected method %q", msg.Method)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	manager := NewManager()
	defer manager.Shutdown(context.Background())
	manager.AddConfig("ide", MCPServerConfig{
		Type:      TransportSSEIDE,
		URL:       server.URL,
		IDEName:   "VSCode",
		AuthToken: "ide-token",
	})

	state, err := manager.GetOrConnect(context.Background(), "ide")
	if err != nil {
		t.Fatalf("GetOrConnect ide: %v", err)
	}
	if state.Type != MCPStateConnected {
		t.Fatalf("ide state = %#v, want connected", state)
	}
	if got, _ := gotIDEAuth.Load().(string); got != "ide-token" {
		t.Fatalf("IDE auth header = %q", got)
	}
	if got := toolNames(state.Tools); strings.Join(got, ",") != "executeCode,getDiagnostics" {
		t.Fatalf("filtered IDE tools = %#v", got)
	}
}

func TestVSCodeSDKMCPHandlersSendExperimentGatesAndHandleLogEvents(t *testing.T) {
	clientTransport, serverTransport := CreateLinkedTransportPair()
	defer clientTransport.Close()

	gatesCh := make(chan map[string]any, 1)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		for {
			msg, err := serverTransport.Receive(context.Background())
			if err != nil {
				return
			}
			switch msg.Method {
			case "initialize":
				resp, _ := NewResultMessage(msg.ID, map[string]any{
					"protocolVersion": MCPProtocolVersion,
					"capabilities":    ServerCapabilities{},
				})
				_ = serverTransport.Send(context.Background(), resp)
			case "experiment_gates":
				var payload struct {
					Gates map[string]any `json:"gates"`
				}
				_ = json.Unmarshal(msg.Params, &payload)
				select {
				case gatesCh <- payload.Gates:
				default:
				}
			}
		}
	}()

	client, err := NewProtocolClient(context.Background(), clientTransport, ClientOptions{})
	if err != nil {
		t.Fatalf("NewProtocolClient: %v", err)
	}
	defer client.Close()

	logEventCh := make(chan string, 1)
	if err := SetupVSCodeSDKMCP(context.Background(), client, map[string]any{"tengu_vscode_cc_auth": true}, func(eventName string, eventData map[string]any) {
		logEventCh <- eventName
	}); err != nil {
		t.Fatalf("SetupVSCodeSDKMCP: %v", err)
	}
	notification, err := NewNotificationMessage("log_event", map[string]any{
		"eventName": "hello",
		"eventData": map[string]any{"ok": true},
	})
	if err != nil {
		t.Fatalf("NewNotificationMessage: %v", err)
	}
	if err := serverTransport.Send(context.Background(), notification); err != nil {
		t.Fatalf("server send log_event: %v", err)
	}

	select {
	case got := <-logEventCh:
		if got != "hello" {
			t.Fatalf("log event = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for log event")
	}
	var gotGates map[string]any
	select {
	case gotGates = <-gatesCh:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for experiment gates")
	}
	if gotGates["tengu_vscode_cc_auth"] != true {
		t.Fatalf("experiment gates = %#v", gotGates)
	}

	_ = serverTransport.Close()
	<-serverDone
}

func TestClaudeAIProxyRequiresTokenBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("CLAUDE_CODE_MCP_PROXY_URL", server.URL)
	t.Setenv("CLAUDE_CODE_MCP_PROXY_PATH", "/v1/mcp/{server_id}")

	manager := NewManager(
		WithNeedsAuthCache(NewNeedsAuthCache(time.Minute)),
		WithTokenSource(TokenSourceFunc(func(context.Context, string) (string, error) {
			return "", nil
		})),
	)
	defer manager.Shutdown(context.Background())
	manager.AddConfig("claudeai", MCPServerConfig{Type: TransportClaudeAIProxy, ID: "connector-id", URL: "https://downstream.example/mcp"})

	state, err := manager.GetOrConnect(context.Background(), "claudeai")
	if err != nil {
		t.Fatalf("GetOrConnect claudeai: %v", err)
	}
	if state.Type != MCPStateNeedsAuth {
		t.Fatalf("state = %#v, want needs-auth", state)
	}
	if requests.Load() != 0 {
		t.Fatalf("claudeai-proxy made %d unauthenticated requests", requests.Load())
	}
}

func TestClaudeAIProxyBuildsProxyURLAndAttachesBearer(t *testing.T) {
	var gotPath, gotEscapedPath, gotAuth, gotSession string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotEscapedPath = r.URL.EscapedPath()
		gotAuth = r.Header.Get("Authorization")
		gotSession = r.Header.Get(claudeAIProxySessionID)
		var msg JSONRPCMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		switch msg.Method {
		case "initialize":
			writeRPCResult(t, w, msg.ID, map[string]any{
				"protocolVersion": MCPProtocolVersion,
				"capabilities":    ServerCapabilities{},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Errorf("unexpected method %q", msg.Method)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	t.Setenv("CLAUDE_CODE_MCP_PROXY_URL", server.URL)
	t.Setenv("CLAUDE_CODE_MCP_PROXY_PATH", "/proxy/{server_id}/mcp")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "session-123")

	manager := NewManager(
		WithNeedsAuthCache(NewNeedsAuthCache(time.Minute)),
		WithTokenSource(TokenSourceFunc(func(context.Context, string) (string, error) {
			return "claudeai-token", nil
		})),
	)
	defer manager.Shutdown(context.Background())
	manager.AddConfig("claudeai", MCPServerConfig{Type: TransportClaudeAIProxy, ID: "connector/id", URL: "https://downstream.example/mcp"})

	state, err := manager.GetOrConnect(context.Background(), "claudeai")
	if err != nil {
		t.Fatalf("GetOrConnect claudeai: %v", err)
	}
	if state.Type != MCPStateConnected {
		t.Fatalf("state = %#v, want connected", state)
	}
	if gotEscapedPath != "/proxy/connector%2Fid/mcp" || gotPath != "/proxy/connector/id/mcp" {
		t.Fatalf("proxy path = %q escaped=%q", gotPath, gotEscapedPath)
	}
	if gotAuth != "Bearer claudeai-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotSession != "session-123" {
		t.Fatalf("session header = %q", gotSession)
	}
}

func TestInProcessTransportPairAndBundledServerRegistration(t *testing.T) {
	cleanup := RegisterInProcessServerFactory("computer-use", func(ctx context.Context, name string, config MCPServerConfig) (InProcessMCPServer, error) {
		return &specializedInProcessServer{tools: []ToolDefinition{{Name: "request_access", InputSchema: json.RawMessage(`{"type":"object"}`)}}}, nil
	})
	defer cleanup()

	manager := NewManager()
	defer manager.Shutdown(context.Background())
	manager.AddConfig("computer-use", MCPServerConfig{Type: TransportStdio, Command: "ignored"})
	state, err := manager.GetOrConnect(context.Background(), "computer-use")
	if err != nil {
		t.Fatalf("GetOrConnect computer-use: %v", err)
	}
	if state.Type != MCPStateConnected {
		t.Fatalf("computer-use state = %#v, want connected", state)
	}
	if len(state.Tools) != 1 || state.Tools[0].Name != "request_access" {
		t.Fatalf("computer-use tools = %#v", state.Tools)
	}
}

func TestChromeInProcessUnsupportedFailsClosed(t *testing.T) {
	manager := NewManager()
	defer manager.Shutdown(context.Background())
	manager.AddConfig("claude-in-chrome", MCPServerConfig{Type: TransportStdio, Command: "ignored"})
	state, err := manager.GetOrConnect(context.Background(), "claude-in-chrome")
	if err != nil {
		t.Fatalf("GetOrConnect chrome: %v", err)
	}
	if state.Type != MCPStateFailed {
		t.Fatalf("state = %#v, want failed", state)
	}
	want := mcpFormat(i18n.KeyMCPIntegrationUnsupportedServer, "claude-in-chrome", TransportStdio, mcpText(i18n.KeyMCPIntegrationUnavailableInBuild))
	if state.Error != want {
		t.Fatalf("chrome unsupported error = %q", state.Error)
	}
}

func specializedResponseForMessage(t *testing.T, msg JSONRPCMessage) *JSONRPCMessage {
	t.Helper()
	if len(msg.ID) == 0 {
		return nil
	}
	var result any
	switch msg.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": MCPProtocolVersion,
			"capabilities":    ServerCapabilities{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "sdk", "version": "1"},
		}
	case "tools/list":
		result = map[string]any{"tools": []map[string]any{{"name": "sdkTool", "inputSchema": map[string]any{"type": "object"}}}}
	default:
		t.Fatalf("unexpected SDK method %q", msg.Method)
	}
	resp, err := NewResultMessage(msg.ID, result)
	if err != nil {
		t.Fatalf("NewResultMessage: %v", err)
	}
	return &resp
}

func toolNames(tools []ToolDefinition) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Name)
	}
	return out
}

type specializedInProcessServer struct {
	tools []ToolDefinition
	once  sync.Once
	done  chan struct{}
}

func (s *specializedInProcessServer) Connect(ctx context.Context, transport Transport) error {
	if s.done == nil {
		s.done = make(chan struct{})
	}
	go func() {
		defer close(s.done)
		for {
			msg, err := transport.Receive(context.Background())
			if err != nil {
				return
			}
			resp := specializedInProcessResponse(msg, s.tools)
			if resp == nil {
				continue
			}
			if err := transport.Send(context.Background(), *resp); err != nil {
				return
			}
		}
	}()
	return nil
}

func (s *specializedInProcessServer) Close() error {
	s.once.Do(func() {})
	return nil
}

func specializedInProcessResponse(msg JSONRPCMessage, tools []ToolDefinition) *JSONRPCMessage {
	if len(msg.ID) == 0 {
		return nil
	}
	var result any
	switch msg.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": MCPProtocolVersion,
			"capabilities":    ServerCapabilities{"tools": map[string]any{}},
		}
	case "tools/list":
		result = ListToolsResult{Tools: tools}
	default:
		resp, _ := NewErrorMessage(msg.ID, -32601, "Method not found", nil)
		return &resp
	}
	resp, err := NewResultMessage(msg.ID, result)
	if err != nil {
		return nil
	}
	return &resp
}

func TestInProcessLinkedTransportClosesBothSides(t *testing.T) {
	a, b := CreateLinkedTransportPair()
	msg, err := NewNotificationMessage("ping", nil)
	if err != nil {
		t.Fatalf("NewNotificationMessage: %v", err)
	}
	if err := a.Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	got, err := b.Receive(context.Background())
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if got.Method != "ping" {
		t.Fatalf("method = %q", got.Method)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := b.Send(context.Background(), msg); !errors.Is(err, ErrTransportClosed) {
		t.Fatalf("peer send after close error = %v, want ErrTransportClosed", err)
	}
}
