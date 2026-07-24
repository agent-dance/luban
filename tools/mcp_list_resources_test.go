package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/registry"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
)

type task11MCPFixture struct {
	mu           sync.Mutex
	capabilities svcmcp.ServerCapabilities
	resources    []svcmcp.Resource
	listError    string
	// listErrorAfter permits startup prefetches to succeed before a later
	// resources/list call exercises reconnect behavior. Zero means fail every
	// request when listError is non-empty.
	listErrorAfter       int
	transportClosedAfter int
	resourceListCalls    int
	readResult           map[string]any
	readURI              string
}

func (f *task11MCPFixture) resourceCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resourceListCalls
}

type task11MCPTransport struct {
	fixture *task11MCPFixture
	recv    chan svcmcp.JSONRPCMessage
	closed  chan struct{}
	once    sync.Once
}

func newTask11MCPTransport(fixture *task11MCPFixture) *task11MCPTransport {
	return &task11MCPTransport{
		fixture: fixture,
		recv:    make(chan svcmcp.JSONRPCMessage, 8),
		closed:  make(chan struct{}),
	}
}

func (t *task11MCPTransport) Send(ctx context.Context, msg svcmcp.JSONRPCMessage) error {
	if len(msg.ID) == 0 {
		return nil
	}
	var result any
	switch msg.Method {
	case "initialize":
		t.fixture.mu.Lock()
		capabilities := t.fixture.capabilities
		t.fixture.mu.Unlock()
		if capabilities == nil {
			capabilities = svcmcp.ServerCapabilities{"resources": map[string]any{"listChanged": true}}
		}
		result = map[string]any{
			"protocolVersion": svcmcp.MCPProtocolVersion,
			"capabilities":    capabilities,
			"serverInfo":      map[string]any{"name": "task11", "version": "test"},
		}
	case "resources/list":
		t.fixture.mu.Lock()
		t.fixture.resourceListCalls++
		calls := t.fixture.resourceListCalls
		listError := t.fixture.listError
		listErrorAfter := t.fixture.listErrorAfter
		transportClosedAfter := t.fixture.transportClosedAfter
		resources := append([]svcmcp.Resource(nil), t.fixture.resources...)
		t.fixture.mu.Unlock()
		if transportClosedAfter > 0 && calls > transportClosedAfter {
			return svcmcp.ErrTransportClosed
		}
		if listError != "" && calls > listErrorAfter {
			response, err := svcmcp.NewErrorMessage(msg.ID, -32000, listError, nil)
			if err != nil {
				return err
			}
			select {
			case t.recv <- response:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-t.closed:
				return svcmcp.ErrTransportClosed
			}
		}
		result = map[string]any{"resources": resources}
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		_ = json.Unmarshal(msg.Params, &params)
		t.fixture.readURI = params.URI
		if t.fixture.readResult != nil {
			result = t.fixture.readResult
		} else {
			result = map[string]any{"contents": []map[string]any{{"uri": params.URI, "text": "ok"}}}
		}
	default:
		result = map[string]any{}
	}
	response, err := svcmcp.NewResultMessage(msg.ID, result)
	if err != nil {
		return err
	}
	select {
	case t.recv <- response:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closed:
		return svcmcp.ErrTransportClosed
	}
}

func (t *task11MCPTransport) Receive(ctx context.Context) (svcmcp.JSONRPCMessage, error) {
	select {
	case msg := <-t.recv:
		return msg, nil
	case <-ctx.Done():
		return svcmcp.JSONRPCMessage{}, ctx.Err()
	case <-t.closed:
		return svcmcp.JSONRPCMessage{}, svcmcp.ErrTransportClosed
	}
}

func (t *task11MCPTransport) Close() error {
	t.once.Do(func() { close(t.closed) })
	return nil
}

func newTask11ServiceManager(fixtures map[string]*task11MCPFixture) *svcmcp.Manager {
	manager := svcmcp.NewManager(svcmcp.WithTransportFactory(func(ctx context.Context, name string, cfg svcmcp.MCPServerConfig, opts svcmcp.TransportBuildOptions) (svcmcp.Transport, error) {
		fixture := fixtures[name]
		if fixture == nil {
			fixture = &task11MCPFixture{}
		}
		return newTask11MCPTransport(fixture), nil
	}))
	for name := range fixtures {
		manager.AddConfig(name, svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "fake"})
	}
	return manager
}

func connectTask11ServiceManager(t *testing.T, manager *svcmcp.Manager) {
	t.Helper()
	if _, err := manager.ConnectAll(context.Background()); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
}

func TestListMcpResourcesTool_ServiceManagerAggregatesResourcesAndSkipsDisabled(t *testing.T) {
	manager := newTask11ServiceManager(map[string]*task11MCPFixture{
		"alpha": {
			resources: []svcmcp.Resource{{
				URI:         "memo://alpha",
				Name:        "Alpha",
				Description: "A memo",
				MimeType:    "text/markdown",
				Annotations: map[string]any{"audience": "model"},
				Meta:        map[string]any{"origin": "fixture"},
			}},
		},
		"beta": {
			resources: []svcmcp.Resource{{
				URI:      "memo://beta",
				Name:     "Beta",
				MimeType: "application/json",
			}},
		},
		"off": {},
	})
	if _, err := manager.ToggleEnabled(context.Background(), "off", false); err != nil {
		t.Fatalf("ToggleEnabled(false): %v", err)
	}
	connectTask11ServiceManager(t, manager)
	tool := NewListMcpResourcesTool(NewMCPManager(), manager)

	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	var resources []mcpListedResource
	if err := json.Unmarshal([]byte(result.Content), &resources); err != nil {
		t.Fatalf("resources should be the TS output array, got %q: %v", result.Content, err)
	}
	if len(resources) != 2 {
		t.Fatalf("resources length = %d, want 2: %#v", len(resources), resources)
	}
	if resources[0].Server != "alpha" || resources[0].URI != "memo://alpha" || resources[0].MimeType != "text/markdown" {
		t.Fatalf("alpha resource missing server/uri/mimeType: %#v", resources[0])
	}
	var wire []map[string]any
	if err := json.Unmarshal([]byte(result.Content), &wire); err != nil {
		t.Fatalf("decode resource wire output: %v", err)
	}
	if _, ok := wire[0]["annotations"]; ok {
		t.Fatalf("TS output schema must strip annotations: %#v", wire[0])
	}
	if _, ok := wire[0]["_meta"]; ok {
		t.Fatalf("TS output schema must strip _meta: %#v", wire[0])
	}
	if !strings.Contains(result.Metadata["mcp.skippedServers"], `"off"`) || !strings.Contains(result.Metadata["mcp.skippedServers"], `"disabled"`) {
		t.Fatalf("disabled server skip metadata missing: %#v", result.Metadata)
	}
}

func TestListMcpResourcesTool_Capabilities(t *testing.T) {
	fixture := &task11MCPFixture{
		capabilities: svcmcp.ServerCapabilities{"tools": map[string]any{}},
		resources:    []svcmcp.Resource{{URI: "memo://must-not-call", Name: "hidden"}},
	}
	manager := newTask11ServiceManager(map[string]*task11MCPFixture{"no-resources": fixture})
	connectTask11ServiceManager(t, manager)
	tool := NewListMcpResourcesTool(nil, manager)

	result, err := tool.Execute(context.Background(), map[string]any{"server": "no-resources"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unsupported resources capability should contribute [], got error: %s", result.Content)
	}
	want := "No resources found. MCP servers may still provide tools even if they have no resources."
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
	if calls := fixture.resourceCallCount(); calls != 0 {
		t.Fatalf("resources/list calls = %d, want 0 without capabilities.resources", calls)
	}
}

func TestListMcpResourcesTool_NonConnected(t *testing.T) {
	var factoryCalls atomic.Int32
	manager := svcmcp.NewManager(svcmcp.WithTransportFactory(func(context.Context, string, svcmcp.MCPServerConfig, svcmcp.TransportBuildOptions) (svcmcp.Transport, error) {
		factoryCalls.Add(1)
		return nil, context.Canceled
	}))
	manager.AddConfig("pending", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "must-not-start"})
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	tool := NewListMcpResourcesTool(nil, manager)

	result, err := tool.Execute(context.Background(), map[string]any{"server": "pending"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("known pending server should contribute [], got %q", result.Content)
	}
	if calls := factoryCalls.Load(); calls != 0 {
		t.Fatalf("ListMcpResourcesTool opportunistically started a pending server: factory calls=%d", calls)
	}
	want := "No resources found. MCP servers may still provide tools even if they have no resources."
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
}

func TestListMcpResourcesTool_NonConnectedStates(t *testing.T) {
	const wantEmpty = "No resources found. MCP servers may still provide tools even if they have no resources."

	t.Run("failed", func(t *testing.T) {
		var factoryCalls atomic.Int32
		manager := svcmcp.NewManager(svcmcp.WithTransportFactory(func(context.Context, string, svcmcp.MCPServerConfig, svcmcp.TransportBuildOptions) (svcmcp.Transport, error) {
			factoryCalls.Add(1)
			return nil, errors.New("connection failed")
		}))
		manager.AddConfig("failed", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "fake"})
		state, err := manager.GetOrConnect(context.Background(), "failed")
		if err != nil || state.Type != svcmcp.MCPStateFailed {
			t.Fatalf("establish failed state: state=%#v err=%v", state, err)
		}
		t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

		result, err := NewListMcpResourcesTool(nil, manager).Execute(context.Background(), map[string]any{"server": "failed"})
		if err != nil || result.IsError || result.Content != wantEmpty {
			t.Fatalf("failed state result=%#v err=%v", result, err)
		}
		if calls := factoryCalls.Load(); calls != 1 {
			t.Fatalf("failed server was opportunistically reconnected: calls=%d", calls)
		}
	})

	t.Run("needs-auth", func(t *testing.T) {
		var factoryCalls atomic.Int32
		manager := svcmcp.NewManager(
			svcmcp.WithNeedsAuthCache(svcmcp.NewNeedsAuthCache(0)),
			svcmcp.WithTransportFactory(func(context.Context, string, svcmcp.MCPServerConfig, svcmcp.TransportBuildOptions) (svcmcp.Transport, error) {
				factoryCalls.Add(1)
				return nil, &svcmcp.UnauthorizedError{StatusCode: 401, ServerURL: "https://example.test/mcp"}
			}),
		)
		manager.AddConfig("auth", svcmcp.MCPServerConfig{Type: svcmcp.TransportHTTP, URL: "https://example.test/mcp"})
		state, err := manager.GetOrConnect(context.Background(), "auth")
		if err != nil || state.Type != svcmcp.MCPStateNeedsAuth {
			t.Fatalf("establish needs-auth state: state=%#v err=%v", state, err)
		}
		t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

		result, err := NewListMcpResourcesTool(nil, manager).Execute(context.Background(), map[string]any{"server": "auth"})
		if err != nil || result.IsError || result.Content != wantEmpty {
			t.Fatalf("needs-auth state result=%#v err=%v", result, err)
		}
		if calls := factoryCalls.Load(); calls != 1 {
			t.Fatalf("needs-auth server was opportunistically reconnected: calls=%d", calls)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		var factoryCalls atomic.Int32
		manager := svcmcp.NewManager(svcmcp.WithTransportFactory(func(context.Context, string, svcmcp.MCPServerConfig, svcmcp.TransportBuildOptions) (svcmcp.Transport, error) {
			factoryCalls.Add(1)
			return nil, errors.New("must not connect")
		}))
		manager.AddConfig("disabled", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "fake"})
		state, err := manager.ToggleEnabled(context.Background(), "disabled", false)
		if err != nil || state.Type != svcmcp.MCPStateDisabled {
			t.Fatalf("establish disabled state: state=%#v err=%v", state, err)
		}
		t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

		result, err := NewListMcpResourcesTool(nil, manager).Execute(context.Background(), map[string]any{"server": "disabled"})
		if err != nil || result.IsError || result.Content != wantEmpty {
			t.Fatalf("disabled state result=%#v err=%v", result, err)
		}
		if calls := factoryCalls.Load(); calls != 0 {
			t.Fatalf("disabled server was opportunistically connected: calls=%d", calls)
		}
	})
}

func TestListMcpResourcesTool_FailureIsolation(t *testing.T) {
	healthy := &task11MCPFixture{resources: []svcmcp.Resource{{URI: "memo://healthy", Name: "Healthy"}}}
	broken := &task11MCPFixture{listError: "resource catalogue unavailable"}
	manager := newTask11ServiceManager(map[string]*task11MCPFixture{
		"broken":  broken,
		"healthy": healthy,
	})
	connectTask11ServiceManager(t, manager)
	tool := NewListMcpResourcesTool(nil, manager)

	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("one failing server must not fail aggregate listing: %s", result.Content)
	}
	var resources []mcpListedResource
	if err := json.Unmarshal([]byte(result.Content), &resources); err != nil {
		t.Fatalf("decode resources: %v (%q)", err, result.Content)
	}
	if len(resources) != 1 || resources[0].Server != "healthy" || resources[0].URI != "memo://healthy" {
		t.Fatalf("healthy server resource missing after failure isolation: %#v", resources)
	}
	if !strings.Contains(result.Metadata["mcp.skippedServers"], `"broken"`) {
		t.Fatalf("failure diagnostics missing from metadata: %#v", result.Metadata)
	}
	if calls := broken.resourceCallCount(); calls != 2 {
		t.Fatalf("application-level resources/list failure caused reconnect: calls=%d, want startup + one tool fetch", calls)
	}
}

func TestListMcpResourcesTool_Cache(t *testing.T) {
	fixture := &task11MCPFixture{resources: []svcmcp.Resource{{URI: "memo://cached", Name: "Cached"}}}
	manager := newTask11ServiceManager(map[string]*task11MCPFixture{"srv": fixture})
	connectTask11ServiceManager(t, manager)
	tool := NewListMcpResourcesTool(nil, manager)

	for i := 0; i < 2; i++ {
		result, err := tool.Execute(context.Background(), map[string]any{"server": "srv"})
		if err != nil || result.IsError {
			t.Fatalf("Execute #%d: result=%#v err=%v", i+1, result, err)
		}
	}
	if calls := fixture.resourceCallCount(); calls != 1 {
		t.Fatalf("resources/list calls = %d, want one startup cache warm and no duplicate tool fetch", calls)
	}
}

func TestListMcpResourcesTool_CacheMissIsFilled(t *testing.T) {
	fixture := &task11MCPFixture{resources: []svcmcp.Resource{{URI: "memo://filled", Name: "Filled"}}}
	manager := newTask11ServiceManager(map[string]*task11MCPFixture{"srv": fixture})
	connectTask11ServiceManager(t, manager)
	manager.Cache().InvalidateResources("srv")
	tool := NewListMcpResourcesTool(nil, manager)

	for i := 0; i < 2; i++ {
		result, err := tool.Execute(context.Background(), map[string]any{"server": "srv"})
		if err != nil || result.IsError {
			t.Fatalf("Execute #%d: result=%#v err=%v", i+1, result, err)
		}
	}
	if calls := fixture.resourceCallCount(); calls != 2 {
		t.Fatalf("resources/list calls = %d, want startup warm + one cache refill", calls)
	}
}

func TestListMcpResourcesTool_Reconnect(t *testing.T) {
	oldFixture := &task11MCPFixture{
		resources:            []svcmcp.Resource{{URI: "memo://old", Name: "Old"}},
		transportClosedAfter: 1,
	}
	newFixture := &task11MCPFixture{resources: []svcmcp.Resource{{URI: "memo://new", Name: "New"}}}
	var factoryCalls atomic.Int32
	manager := svcmcp.NewManager(svcmcp.WithTransportFactory(func(context.Context, string, svcmcp.MCPServerConfig, svcmcp.TransportBuildOptions) (svcmcp.Transport, error) {
		if factoryCalls.Add(1) == 1 {
			return newTask11MCPTransport(oldFixture), nil
		}
		return newTask11MCPTransport(newFixture), nil
	}))
	manager.AddConfig("srv", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "fake"})
	connectTask11ServiceManager(t, manager)
	manager.Cache().InvalidateResources("srv")
	tool := NewListMcpResourcesTool(nil, manager)

	result, err := tool.Execute(context.Background(), map[string]any{"server": "srv"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unhealthy connected server should reconnect/refetch: %s", result.Content)
	}
	if !strings.Contains(result.Content, "memo://new") || strings.Contains(result.Content, "memo://old") {
		t.Fatalf("reconnect returned stale resources: %s", result.Content)
	}
	if calls := factoryCalls.Load(); calls != 2 {
		t.Fatalf("transport factory calls = %d, want initial connect + one reconnect", calls)
	}
}

func TestListMcpResourcesTool_InvalidInput(t *testing.T) {
	tool := NewListMcpResourcesTool(NewMCPManager())
	for name, input := range map[string]map[string]any{
		"non-string-server": {"server": 42},
		"legacy-alias":      {"server_name": "srv"},
		"unknown-field":     {"server": "srv", "extra": true},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), input)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !result.IsError || !strings.Contains(result.Content, "Invalid input") {
				t.Fatalf("invalid input = %#v, want stable tool-level Invalid input error", result)
			}
		})
	}
}

func TestListMcpResourcesTool_TypedOutputAndModelText(t *testing.T) {
	fixture := &task11MCPFixture{resources: []svcmcp.Resource{{URI: "memo://typed", Name: "Typed"}}}
	manager := newTask11ServiceManager(map[string]*task11MCPFixture{"srv": fixture})
	connectTask11ServiceManager(t, manager)
	tool := NewListMcpResourcesTool(nil, manager)

	result, err := tool.Execute(context.Background(), map[string]any{"server": "srv"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Data == nil {
		t.Fatal("ListMcpResourcesTool must retain a typed resource-array result")
	}
	block := types.MapToolResult(tool, result, "toolu_resources")
	if block.Data == nil || block.ToolUseID != "toolu_resources" || block.Content != result.Content {
		t.Fatalf("typed result mapper mismatch: result=%#v block=%#v", result, block)
	}

	empty, err := NewListMcpResourcesTool(NewMCPManager()).Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("empty Execute: %v", err)
	}
	emptyBlock := types.MapToolResult(tool, empty, "toolu_empty")
	wantEmpty := "No resources found. MCP servers may still provide tools even if they have no resources."
	if emptyBlock.Content != wantEmpty {
		t.Fatalf("empty mapped model text = %q, want %q", emptyBlock.Content, wantEmpty)
	}
}

func TestListMcpResourcesTool_ResultBudget(t *testing.T) {
	tool := NewListMcpResourcesTool(NewMCPManager())
	provider, ok := any(tool).(interface{ IsResultTruncated(any) bool })
	if !ok {
		t.Fatal("ListMcpResourcesTool must expose the TS isResultTruncated predicate")
	}
	if provider.IsResultTruncated([]map[string]any{{"uri": "memo://short", "name": "short", "server": "srv"}}) {
		t.Fatal("short result reported truncated")
	}
	// TS checks UI line truncation separately from maxResultSizeChars. Compact
	// JSON escapes embedded newlines, so even a large one-line result is not
	// reported as UI-truncated; the 100k result budget remains authoritative.
	if provider.IsResultTruncated([]map[string]any{{
		"uri":         "memo://large",
		"name":        "large",
		"description": strings.Repeat("x", listMcpResourcesMaxResultSizeChars+1),
		"server":      "srv",
	}}) {
		t.Fatal("compact one-line JSON must not be reported as UI-line-truncated")
	}
	if contract := types.ResolveToolContract(tool); contract.MaxResultSizeChars != listMcpResourcesMaxResultSizeChars {
		t.Fatalf("max result size = %d, want %d", contract.MaxResultSizeChars, listMcpResourcesMaxResultSizeChars)
	}
}

func TestToolSearchListMcpResources(t *testing.T) {
	tool := NewListMcpResourcesTool(NewMCPManager())
	metadata := registry.DiscoveryMetadata(tool)
	if !metadata.ShouldDefer {
		t.Fatal("ListMcpResourcesTool must remain deferred")
	}
	if metadata.SearchHint != "list resources from connected MCP servers" {
		t.Fatalf("search hint = %q, want TS hint", metadata.SearchHint)
	}
	if !registry.IsDeferredTool(tool) {
		t.Fatal("registry did not classify ListMcpResourcesTool as deferred")
	}
}

func TestListMcpResourcesTool_ServiceManagerUnknownServerErrorsWithAvailableServers(t *testing.T) {
	manager := newTask11ServiceManager(map[string]*task11MCPFixture{
		"known": {resources: []svcmcp.Resource{{URI: "memo://known", Name: "Known"}}},
	})
	tool := NewListMcpResourcesTool(NewMCPManager(), manager)

	result, err := tool.Execute(context.Background(), map[string]any{"server": "missing"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected missing server error, got %q", result.Content)
	}
	if !strings.Contains(result.Content, `Server "missing" not found`) || !strings.Contains(result.Content, "known") {
		t.Fatalf("missing server error should include available servers, got %q", result.Content)
	}
}

func TestListMcpResourcesTool_LegacyListAllDoesNotListCachedTools(t *testing.T) {
	manager := NewMCPManager()
	manager.AddServer(&MCPServer{
		Name:    "legacy",
		BaseURL: "http://example.test",
		Tools:   []MCPServerTool{{Name: "tool-a", Description: "Tool A"}},
	})
	tool := NewListMcpResourcesTool(manager)

	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(result.Content, "tool-a") || strings.Contains(result.Content, "Tool A") {
		t.Fatalf("legacy list-all should not list cached tools as resources: %q", result.Content)
	}
	if !strings.Contains(result.Metadata["mcp.skippedServers"], `"legacy"`) {
		t.Fatalf("legacy skipped state should remain explainable in metadata: %#v", result.Metadata)
	}
}
