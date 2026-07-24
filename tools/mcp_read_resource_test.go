package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/registry"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
	"github.com/creachadair/jrpc2/handler"
)

// TS evidence: ReadMcpResourceTool.ts:30-44 and :106-143 construct a new
// contents array and deliberately whitelist uri, mimeType, text, blobSavedTo.
func TestReadMcpResourceTool_TSOutputShapeDropsRawMetaAndUnknownFields(t *testing.T) {
	fixture := &task11MCPFixture{
		resources: []svcmcp.Resource{{URI: "memo://alpha", Name: "Alpha", MimeType: "text/markdown"}},
		readResult: map[string]any{
			"contents": []map[string]any{
				{
					"uri":         "memo://alpha",
					"mimeType":    "text/markdown",
					"text":        "# Alpha",
					"blob":        "must-not-win-over-text",
					"type":        "text",
					"annotations": map[string]any{"audience": []string{"assistant"}},
					"_meta":       map[string]any{"source": "fixture"},
					"unknown":     "drop-me",
				},
				{
					"uri":      "memo://empty",
					"mimeType": "application/octet-stream",
					"data":     "raw-data-is-not-a-resource-blob",
					"type":     "blob",
				},
			},
			"_meta": map[string]any{"request": "read"},
		},
	}
	manager := newTask11ServiceManager(map[string]*task11MCPFixture{"docs": fixture})
	connectTask11ServiceManager(t, manager)
	tool := NewReadMcpResourceTool(NewMCPManager(), manager)

	result, err := tool.Execute(context.Background(), map[string]any{"server": "docs", "uri": "memo://alpha"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if fixture.readURI != "memo://alpha" {
		t.Fatalf("resources/read routed uri = %q, want memo://alpha", fixture.readURI)
	}
	want := `{"contents":[{"uri":"memo://alpha","mimeType":"text/markdown","text":"# Alpha"},{"uri":"memo://empty","mimeType":"application/octet-stream"}]}`
	if result.Content != want {
		t.Fatalf("TS-normalized output mismatch:\n got: %s\nwant: %s", result.Content, want)
	}
	for _, forbidden := range []string{"_meta", "annotations", "unknown", "blob", "data", `"type"`} {
		if strings.Contains(result.Content, forbidden) {
			t.Fatalf("raw MCP field %q leaked into model output: %s", forbidden, result.Content)
		}
	}
	output, ok := result.Data.(ReadMcpResourceOutput)
	if !ok || len(output.Contents) != 2 {
		t.Fatalf("typed output = %#v, want ReadMcpResourceOutput with 2 items", result.Data)
	}
}

func TestReadMcpResourceTool_TextPreservedExactlyAndEmptyResult(t *testing.T) {
	t.Run("text is not trimmed or pretty-printed", func(t *testing.T) {
		text := " {\"b\":2,\"a\":1} \n"
		fixture := &task11MCPFixture{readResult: map[string]any{"contents": []map[string]any{{
			"uri": "memo://json", "mimeType": "application/json", "text": text,
		}}}}
		manager := newTask11ServiceManager(map[string]*task11MCPFixture{"docs": fixture})
		connectTask11ServiceManager(t, manager)
		result, err := NewReadMcpResourceTool(nil, manager).Execute(context.Background(), map[string]any{"server": "docs", "uri": "memo://json"})
		if err != nil || result.IsError {
			t.Fatalf("Execute = %#v, %v", result, err)
		}
		var output ReadMcpResourceOutput
		if err := json.Unmarshal([]byte(result.Content), &output); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if len(output.Contents) != 1 || output.Contents[0].Text == nil || *output.Contents[0].Text != text {
			t.Fatalf("text changed: %#v", output)
		}
	})

	t.Run("empty contents remains structured empty array", func(t *testing.T) {
		fixture := &task11MCPFixture{readResult: map[string]any{"contents": []any{}}}
		manager := newTask11ServiceManager(map[string]*task11MCPFixture{"docs": fixture})
		connectTask11ServiceManager(t, manager)
		result, err := NewReadMcpResourceTool(nil, manager).Execute(context.Background(), map[string]any{"server": "docs", "uri": "memo://empty"})
		if err != nil || result.IsError {
			t.Fatalf("Execute = %#v, %v", result, err)
		}
		if result.Content != `{"contents":[]}` {
			t.Fatalf("empty result = %q, want exact contents array", result.Content)
		}
	})
}

// TS evidence: mcpOutputStorage.ts:148-188 writes decoded bytes and returns a
// blobSavedTo path plus the concise saved-content message.
func TestReadMcpResourceTool_BinaryBlobSavedToDoesNotInlineBase64(t *testing.T) {
	for _, size := range []int{17, maxBinaryInlineBytes + 31} {
		size := size
		t.Run(map[bool]string{true: "large", false: "small"}[size > maxBinaryInlineBytes], func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(mcpToolResultsDirEnv, dir)
			payload := make([]byte, size)
			for i := range payload {
				payload[i] = byte(i % 251)
			}
			encoded := base64.StdEncoding.EncodeToString(payload)
			fixture := &task11MCPFixture{readResult: map[string]any{"contents": []map[string]any{{
				"uri": "file://report.pdf", "mimeType": "application/pdf", "blob": encoded,
			}}}}
			manager := newTask11ServiceManager(map[string]*task11MCPFixture{"docs": fixture})
			connectTask11ServiceManager(t, manager)
			result, err := NewReadMcpResourceTool(nil, manager).Execute(context.Background(), map[string]any{"server": "docs", "uri": "file://report.pdf"})
			if err != nil || result.IsError {
				t.Fatalf("Execute = %#v, %v", result, err)
			}
			if strings.Contains(result.Content, encoded) || strings.Contains(result.Content, `"blob"`) || strings.Contains(result.Content, `"data"`) {
				t.Fatalf("base64/raw blob field leaked: %s", result.Content)
			}
			var output ReadMcpResourceOutput
			if err := json.Unmarshal([]byte(result.Content), &output); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			if len(output.Contents) != 1 || output.Contents[0].BlobSavedTo == "" || output.Contents[0].Text == nil {
				t.Fatalf("binary output shape: %#v", output)
			}
			path := output.Contents[0].BlobSavedTo
			if filepath.Dir(path) != dir || filepath.Ext(path) != ".pdf" {
				t.Fatalf("persist path = %q, want configured dir and MIME extension", path)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read persisted bytes: %v", err)
			}
			if string(got) != string(payload) {
				t.Fatalf("persisted bytes differ: got %d bytes want %d", len(got), len(payload))
			}
			if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("persisted file mode = %v, %v; want 0600", info, err)
			}
			wantPrefix := "[Resource from docs at file://report.pdf] Binary content (application/pdf, "
			if !strings.HasPrefix(*output.Contents[0].Text, wantPrefix) || !strings.HasSuffix(*output.Contents[0].Text, " saved to "+path) {
				t.Fatalf("saved message = %q", *output.Contents[0].Text)
			}
		})
	}
}

func TestReadMcpResourceTool_BinaryPersistFailureReturnsText(t *testing.T) {
	parent := t.TempDir()
	notDir := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(notDir, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(mcpToolResultsDirEnv, notDir)
	encoded := base64.StdEncoding.EncodeToString([]byte("binary"))
	fixture := &task11MCPFixture{readResult: map[string]any{"contents": []map[string]any{{
		"uri": "memo://blob", "mimeType": "application/octet-stream", "blob": encoded,
	}}}}
	manager := newTask11ServiceManager(map[string]*task11MCPFixture{"docs": fixture})
	connectTask11ServiceManager(t, manager)
	result, err := NewReadMcpResourceTool(nil, manager).Execute(context.Background(), map[string]any{"server": "docs", "uri": "memo://blob"})
	if err != nil || result.IsError {
		t.Fatalf("Execute = %#v, %v", result, err)
	}
	var output ReadMcpResourceOutput
	if err := json.Unmarshal([]byte(result.Content), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(output.Contents) != 1 || output.Contents[0].Text == nil || !strings.HasPrefix(*output.Contents[0].Text, "Binary content could not be saved to disk: ") {
		t.Fatalf("persist failure output = %#v", output)
	}
	if output.Contents[0].BlobSavedTo != "" || strings.Contains(result.Content, encoded) {
		t.Fatalf("failed persistence leaked blob/path: %s", result.Content)
	}
}

// TS evidence: ReadMcpResourceTool.ts:78-101 distinguishes missing,
// disconnected, and resources-unsupported servers before resources/read.
func TestReadMcpResourceTool_StateAndCapabilities(t *testing.T) {
	t.Run("server not found", func(t *testing.T) {
		manager := newTask11ServiceManager(map[string]*task11MCPFixture{"known": {}})
		result, err := NewReadMcpResourceTool(nil, manager).Execute(context.Background(), map[string]any{"server": "missing", "uri": "memo://x"})
		if err != nil || !result.IsError || !strings.Contains(result.Content, `Server "missing" not found. Available servers: known`) {
			t.Fatalf("result = %#v, %v", result, err)
		}
	})

	t.Run("configured but not connected is not auto-started", func(t *testing.T) {
		fixture := &task11MCPFixture{}
		manager := newTask11ServiceManager(map[string]*task11MCPFixture{"pending": fixture})
		result, err := NewReadMcpResourceTool(nil, manager).Execute(context.Background(), map[string]any{"server": "pending", "uri": "memo://x"})
		if err != nil || !result.IsError || !strings.Contains(result.Content, `Server "pending" is not connected`) {
			t.Fatalf("result = %#v, %v", result, err)
		}
		if fixture.readURI != "" {
			t.Fatalf("pending server received resources/read for %q", fixture.readURI)
		}
	})

	t.Run("connected server without resources is rejected before RPC", func(t *testing.T) {
		fixture := &task11MCPFixture{capabilities: svcmcp.ServerCapabilities{}}
		manager := newTask11ServiceManager(map[string]*task11MCPFixture{"plain": fixture})
		connectTask11ServiceManager(t, manager)
		result, err := NewReadMcpResourceTool(nil, manager).Execute(context.Background(), map[string]any{"server": "plain", "uri": "memo://x"})
		if err != nil || !result.IsError || !strings.Contains(result.Content, `Server "plain" does not support resources`) {
			t.Fatalf("result = %#v, %v", result, err)
		}
		if fixture.readURI != "" {
			t.Fatalf("unsupported server received resources/read for %q", fixture.readURI)
		}
	})

	t.Run("legacy connection uses initialize capabilities", func(t *testing.T) {
		var reads int
		client := startMCPServer(t, "plain", handler.Map{
			"initialize": handler.New(func(context.Context, json.RawMessage) (any, error) {
				return map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}}, nil
			}),
			"resources/read": handler.New(func(context.Context, json.RawMessage) (any, error) {
				reads++
				return map[string]any{"contents": []any{}}, nil
			}),
		})
		manager := injectServer("plain", client, nil)
		result, err := NewReadMcpResourceTool(manager).Execute(context.Background(), map[string]any{"server": "plain", "uri": "memo://x"})
		if err != nil || !result.IsError || !strings.Contains(result.Content, "does not support resources") || reads != 0 {
			t.Fatalf("result = %#v, err=%v reads=%d", result, err, reads)
		}
	})
}

// TS evidence: ReadMcpResourceTool.ts:49-74 and UI.tsx:16-18 define the
// read-only/classifier/result-budget/schema metadata and UI label.
func TestReadMcpResourceTool_InputAndMetadata(t *testing.T) {
	tool := NewReadMcpResourceTool(NewMCPManager())
	if tool.UserFacingName() != "readMcpResource" {
		t.Fatalf("user-facing name = %q, want TS UI label", tool.UserFacingName())
	}
	if schema := tool.Schema(); !schema.RejectsUnknownFields() || len(schema.Required) != 2 || schema.Required[0] != "server" || schema.Required[1] != "uri" {
		t.Fatalf("input schema must be strict server+uri: %#v", schema)
	}
	result, err := tool.Execute(context.Background(), map[string]any{"server_name": "legacy", "resource_uri": "memo://legacy"})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "invalid input") {
		t.Fatalf("legacy aliases must be rejected: %#v, %v", result, err)
	}
	if !tool.IsReadOnly() || !tool.IsConcurrentSafe() {
		t.Fatalf("read resource metadata must be read-only and concurrent-safe")
	}
	if got := types.ToolAutoClassifierInput(tool, map[string]any{"server": "fs", "uri": "memo://a"}); got != "fs memo://a" {
		t.Fatalf("classifier input = %q", got)
	}
	contract := types.ResolveToolContract(tool)
	if !contract.Strict || !contract.ReadOnly || !contract.ConcurrencySafe || contract.MaxResultSizeChars != 100_000 || contract.OutputSchema == nil {
		t.Fatalf("tool contract = %#v", contract)
	}
	if contract.OutputSchema.Type != "object" || !contract.OutputSchema.RejectsUnknownFields() {
		t.Fatalf("output schema = %#v", contract.OutputSchema)
	}
	definition := types.ToDefinition(tool)
	if definition.OutputSchema == nil || definition.Metadata.MaxResultSizeChars != 100_000 || !definition.Metadata.ReadOnly {
		t.Fatalf("definition metadata = %#v", definition)
	}
	discovery := registry.DiscoveryMetadata(tool)
	if !discovery.ShouldDefer || discovery.SearchHint != "read a specific MCP resource by URI" {
		t.Fatalf("discovery metadata = %#v", discovery)
	}
	block := types.MapToolResult(tool, types.ToolResult{Data: ReadMcpResourceOutput{Contents: []ReadMcpResourceContent{{URI: "memo://a"}}}}, "toolu_read")
	if block.Content != `{"contents":[{"uri":"memo://a"}]}` || block.Metadata["maxResultSizeChars"] != "100000" {
		t.Fatalf("mapped block = %#v", block)
	}
	if tool.IsResultTruncated(ReadMcpResourceOutput{Contents: []ReadMcpResourceContent{{URI: "memo://a"}}}) {
		t.Fatal("single-line JSON output must not be marked truncated")
	}
}

// TS evidence: services/mcp/client.ts:2169-2190 only installs resource helper
// tools when a connected server advertises the resources capability.
func TestReadMcpResourceTool_VisibilityRequiresConnectedResourcesServer(t *testing.T) {
	fixture := &task11MCPFixture{}
	manager := newTask11ServiceManager(map[string]*task11MCPFixture{"docs": fixture})
	tool := NewReadMcpResourceTool(nil, manager)
	if tool.IsEnabled(types.ToolRuntimeContext{}) {
		t.Fatal("pending-only MCP state must not expose ReadMcpResourceTool")
	}
	connectTask11ServiceManager(t, manager)
	if !tool.IsEnabled(types.ToolRuntimeContext{}) {
		t.Fatal("connected resources-capable server must expose ReadMcpResourceTool")
	}
}

// TS evidence: ReadMcpResourceTool.ts:94-101 always sends resources/read with
// the URI as a JSON-RPC parameter, including over HTTP transports.
func TestReadMcpResourceTool_HTTPUsesJSONRPCAndOpaqueURI(t *testing.T) {
	wantURI := "custom://authority/a/b?x=1&y=two#fragment"
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.URL.Path != "/mcp" {
			t.Errorf("HTTP MCP path = %q, want configured endpoint /mcp", r.URL.Path)
		}
		var request struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  struct {
				URI string `json:"uri"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
		}
		if request.JSONRPC != "2.0" || request.Method != "resources/read" || request.Params.URI != wantURI {
			t.Errorf("JSON-RPC request = %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": json.RawMessage(request.ID),
			"result": map[string]any{"contents": []map[string]any{{"uri": wantURI, "text": "ok", "_meta": map[string]any{"drop": true}}}},
		})
	}))
	defer server.Close()

	manager := NewMCPManager()
	manager.AddServer(&MCPServer{Name: "remote", BaseURL: server.URL + "/mcp"})
	result, err := NewReadMcpResourceTool(manager).Execute(context.Background(), map[string]any{"server": "remote", "uri": wantURI})
	if err != nil || result.IsError {
		t.Fatalf("Execute = %#v, %v", result, err)
	}
	if len(methods) != 1 || methods[0] != http.MethodPost {
		t.Fatalf("HTTP methods = %#v, want one POST and no legacy GET", methods)
	}
	want := `{"contents":[{"uri":"custom://authority/a/b?x=1&y=two#fragment","text":"ok"}]}`
	if result.Content != want {
		t.Fatalf("HTTP normalized output = %s, want %s", result.Content, want)
	}
}

func TestReadMcpResourceTool_HTTP401PreservesOAuthChallenge(t *testing.T) {
	challenge := `Bearer realm="mcp", as_uri="https://auth.example.test/oauth"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("WWW-Authenticate", challenge)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	manager := NewMCPManager()
	manager.AddServer(&MCPServer{Name: "remote", BaseURL: server.URL})
	result, err := NewReadMcpResourceTool(manager).Execute(context.Background(), map[string]any{"server": "remote", "uri": "memo://private"})
	if err != nil || !result.IsError {
		t.Fatalf("Execute = %#v, %v", result, err)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(result.Content), &envelope); err != nil {
		t.Fatalf("decode OAuth envelope: %v", err)
	}
	if envelope["error"] != "oauth_required" || envelope["www_authenticate"] != challenge || envelope["status"] != float64(http.StatusUnauthorized) {
		t.Fatalf("OAuth envelope = %#v", envelope)
	}
}

type readMCPScenario struct {
	mu              sync.Mutex
	connections     int
	reads           int
	genericReadErr  bool
	expireFirstRead bool
	closeFirstRead  bool
}

func (s *readMCPScenario) counts() (connections, reads int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections, s.reads
}

type readMCPScenarioTransport struct {
	scenario   *readMCPScenario
	connection int
	recv       chan svcmcp.JSONRPCMessage
	closed     chan struct{}
	closeOnce  sync.Once
}

func (t *readMCPScenarioTransport) Send(ctx context.Context, msg svcmcp.JSONRPCMessage) error {
	if len(msg.ID) == 0 {
		return nil
	}
	var result any
	switch msg.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": svcmcp.MCPProtocolVersion,
			"capabilities":    map[string]any{"resources": map[string]any{}},
			"serverInfo":      map[string]any{"name": "read-scenario", "version": "test"},
		}
	case "resources/list":
		result = map[string]any{"resources": []any{}}
	case "resources/read":
		t.scenario.mu.Lock()
		t.scenario.reads++
		readNumber := t.scenario.reads
		generic := t.scenario.genericReadErr
		expire := t.scenario.expireFirstRead && readNumber == 1
		closeConnection := t.scenario.closeFirstRead && readNumber == 1
		t.scenario.mu.Unlock()
		if expire {
			return &svcmcp.SessionExpiredError{ServerName: "docs", Err: errors.New("session expired")}
		}
		if closeConnection {
			return svcmcp.ErrTransportClosed
		}
		if generic {
			response, err := svcmcp.NewErrorMessage(msg.ID, -32000, "resource denied", nil)
			if err != nil {
				return err
			}
			return t.enqueue(ctx, response)
		}
		var params struct {
			URI string `json:"uri"`
		}
		_ = json.Unmarshal(msg.Params, &params)
		result = map[string]any{"contents": []map[string]any{{"uri": params.URI, "text": "recovered"}}}
	default:
		result = map[string]any{}
	}
	response, err := svcmcp.NewResultMessage(msg.ID, result)
	if err != nil {
		return err
	}
	return t.enqueue(ctx, response)
}

func (t *readMCPScenarioTransport) enqueue(ctx context.Context, msg svcmcp.JSONRPCMessage) error {
	select {
	case t.recv <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closed:
		return svcmcp.ErrTransportClosed
	}
}

func (t *readMCPScenarioTransport) Receive(ctx context.Context) (svcmcp.JSONRPCMessage, error) {
	select {
	case msg := <-t.recv:
		return msg, nil
	case <-ctx.Done():
		return svcmcp.JSONRPCMessage{}, ctx.Err()
	case <-t.closed:
		return svcmcp.JSONRPCMessage{}, svcmcp.ErrTransportClosed
	}
}

func (t *readMCPScenarioTransport) Close() error {
	t.closeOnce.Do(func() { close(t.closed) })
	return nil
}

func newReadMCPScenarioManager(t *testing.T, scenario *readMCPScenario) *svcmcp.Manager {
	t.Helper()
	manager := svcmcp.NewManager(svcmcp.WithTransportFactory(func(context.Context, string, svcmcp.MCPServerConfig, svcmcp.TransportBuildOptions) (svcmcp.Transport, error) {
		scenario.mu.Lock()
		scenario.connections++
		connection := scenario.connections
		scenario.mu.Unlock()
		return &readMCPScenarioTransport{
			scenario: scenario, connection: connection,
			recv: make(chan svcmcp.JSONRPCMessage, 8), closed: make(chan struct{}),
		}, nil
	}))
	manager.AddConfig("docs", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "fake"})
	if _, err := manager.GetOrConnect(context.Background(), "docs"); err != nil {
		t.Fatalf("connect scenario manager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	return manager
}

func TestReadMcpResourceTool_DoesNotReconnectArbitraryReadErrors(t *testing.T) {
	scenario := &readMCPScenario{genericReadErr: true}
	manager := newReadMCPScenarioManager(t, scenario)
	result, err := NewReadMcpResourceTool(nil, manager).Execute(context.Background(), map[string]any{"server": "docs", "uri": "memo://denied"})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "resource denied") {
		t.Fatalf("Execute = %#v, %v", result, err)
	}
	connections, reads := scenario.counts()
	if connections != 1 || reads != 1 {
		t.Fatalf("generic error caused reconnect/retry: connections=%d reads=%d", connections, reads)
	}
}

// TS evidence: services/mcp/client.ts:1313-1397 clears expired/closed sessions
// and caches so ensureConnectedClient can build a fresh client.
func TestReadMcpResourceTool_SessionExpiredClearsCacheAndRetries(t *testing.T) {
	scenario := &readMCPScenario{expireFirstRead: true}
	manager := newReadMCPScenarioManager(t, scenario)
	result, err := NewReadMcpResourceTool(nil, manager).Execute(context.Background(), map[string]any{"server": "docs", "uri": "memo://recover"})
	if err != nil || result.IsError || !strings.Contains(result.Content, "recovered") {
		t.Fatalf("Execute = %#v, %v", result, err)
	}
	connections, reads := scenario.counts()
	if connections != 2 || reads != 2 {
		t.Fatalf("session recovery = connections=%d reads=%d, want 2/2", connections, reads)
	}
}

func TestReadMcpResourceTool_ReconnectAfterTransportClose(t *testing.T) {
	scenario := &readMCPScenario{closeFirstRead: true}
	manager := newReadMCPScenarioManager(t, scenario)
	result, err := NewReadMcpResourceTool(nil, manager).Execute(context.Background(), map[string]any{"server": "docs", "uri": "memo://recover"})
	if err != nil || result.IsError || !strings.Contains(result.Content, "recovered") {
		t.Fatalf("Execute = %#v, %v", result, err)
	}
	connections, reads := scenario.counts()
	if connections != 2 || reads != 2 {
		t.Fatalf("transport-close recovery = connections=%d reads=%d, want 2/2", connections, reads)
	}
}

func TestMCPManagerConnectDoesNotReturnClosedCachedClient(t *testing.T) {
	client := startMCPServer(t, "closed", handler.Map{})
	manager := NewMCPManager()
	manager.injectConn("closed", &MCPServerConn{
		Config: MCPServerConfig{Command: filepath.Join(t.TempDir(), "missing-server")},
		client: client,
		ready:  true,
	})
	if err := client.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}
	conn, err := manager.Connect("closed")
	if err == nil || conn != nil {
		t.Fatalf("Connect returned stale closed client: conn=%#v err=%v", conn, err)
	}
}

// Keep io imported in builds where httptest's request body assertions are
// optimized away by the compiler's inlining decisions.
var _ io.Reader
