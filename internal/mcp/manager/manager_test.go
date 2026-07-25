package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	"github.com/agent-dance/luban/internal/mcp/protocol"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
)

const testMCPProtocolVersion = "2024-11-05"

type managerTestTransport struct {
	name string

	capabilities catalog.ServerCapabilities
	serverInfo   *catalog.ServerInfo
	instructions string
	tools        []catalog.ToolDefinition
	resources    []catalog.Resource
	prompts      []catalog.PromptDefinition

	recv     chan protocol.JSONRPCMessage
	closed   chan struct{}
	closeMu  sync.Mutex
	isClosed bool

	onClose func()
}

type managerCloseErrorTransport struct {
	*managerTestTransport
	err error
}

func (t *managerCloseErrorTransport) Close() error {
	return errors.Join(t.managerTestTransport.Close(), t.err)
}

func newManagerTestTransport(name string, generation int, onClose func()) *managerTestTransport {
	return &managerTestTransport{
		name: name,
		capabilities: catalog.ServerCapabilities{
			"tools":     map[string]any{"listChanged": true},
			"resources": map[string]any{"listChanged": true},
			"prompts":   map[string]any{"listChanged": true},
		},
		serverInfo:   &catalog.ServerInfo{Name: "server-" + name, Version: fmt.Sprintf("v%d", generation)},
		instructions: "instructions for " + name,
		tools: []catalog.ToolDefinition{{
			Name:        fmt.Sprintf("tool-%d", generation),
			Description: "test tool",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Annotations: map[string]any{
				"readOnlyHint": true,
			},
		}},
		resources: []catalog.Resource{{
			URI:         fmt.Sprintf("memo://%s/%d", name, generation),
			Name:        "memo",
			Description: "test resource",
			MimeType:    "text/plain",
		}},
		prompts: []catalog.PromptDefinition{{
			Name:        fmt.Sprintf("prompt-%d", generation),
			Description: "test prompt",
			Arguments:   []catalog.PromptArgument{{Name: "topic", Required: true}},
		}},
		recv:    make(chan protocol.JSONRPCMessage, 16),
		closed:  make(chan struct{}),
		onClose: onClose,
	}
}

func (t *managerTestTransport) Send(ctx context.Context, msg protocol.JSONRPCMessage) error {
	t.closeMu.Lock()
	closed := t.isClosed
	t.closeMu.Unlock()
	if closed {
		return mcptransport.NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, mcptransport.ErrTransportClosed, "manager test")
	}
	if len(msg.ID) == 0 {
		return nil
	}
	var result any
	switch msg.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": testMCPProtocolVersion,
			"capabilities":    t.capabilities,
			"serverInfo":      t.serverInfo,
			"instructions":    t.instructions,
		}
	case "tools/list":
		result = map[string]any{"tools": t.tools}
	case "resources/list":
		result = map[string]any{"resources": t.resources}
	case "prompts/list":
		result = map[string]any{"prompts": t.prompts}
	default:
		return fmt.Errorf("unexpected method %s", msg.Method)
	}
	response, err := protocol.NewResultMessage(msg.ID, result)
	if err != nil {
		return err
	}
	select {
	case t.recv <- response:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closed:
		return mcptransport.NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, mcptransport.ErrTransportClosed, "manager test")
	}
}

func (t *managerTestTransport) Receive(ctx context.Context) (protocol.JSONRPCMessage, error) {
	select {
	case msg := <-t.recv:
		return msg, nil
	case <-ctx.Done():
		return protocol.JSONRPCMessage{}, ctx.Err()
	case <-t.closed:
		return protocol.JSONRPCMessage{}, mcptransport.NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, mcptransport.ErrTransportClosed, "manager test")
	}
}

func (t *managerTestTransport) Close() error {
	t.closeMu.Lock()
	if !t.isClosed {
		t.isClosed = true
		close(t.closed)
		if t.onClose != nil {
			t.onClose()
		}
	}
	t.closeMu.Unlock()
	return nil
}

func TestManagerConnectStoresStateAndCaches(t *testing.T) {
	var calls atomic.Int32
	manager := NewManager(withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
		calls.Add(1)
		return newManagerTestTransport(name, int(calls.Load()), nil), nil
	}))
	manager.AddConfig("alpha", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake"})

	pending, ok := manager.State("alpha")
	if !ok || pending.Type != MCPStatePending {
		t.Fatalf("initial state = %#v, want pending", pending)
	}

	state, err := manager.GetOrConnect(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("GetOrConnect: %v", err)
	}
	if state.Type != MCPStateConnected {
		t.Fatalf("state type = %q, want connected", state.Type)
	}
	if state.Client == nil {
		t.Fatalf("connected state missing initialized client")
	}
	if state.ServerInfo == nil || state.ServerInfo.Name != "server-alpha" || state.ServerInfo.Version != "v1" {
		t.Fatalf("serverInfo = %#v", state.ServerInfo)
	}
	if state.Instructions != "instructions for alpha" {
		t.Fatalf("instructions = %q", state.Instructions)
	}
	if _, ok := state.Capabilities["tools"]; !ok {
		t.Fatalf("capabilities missing tools: %#v", state.Capabilities)
	}
	if len(state.Tools) != 1 || state.Tools[0].Name != "tool-1" {
		t.Fatalf("tools = %#v", state.Tools)
	}
	if len(state.Resources) != 1 || state.Resources[0].URI != "memo://alpha/1" {
		t.Fatalf("resources = %#v", state.Resources)
	}
	if len(state.Prompts) != 1 || state.Prompts[0].Name != "prompt-1" {
		t.Fatalf("prompts = %#v", state.Prompts)
	}

	if tools, ok := manager.cache.toolsSnapshot("alpha"); !ok || len(tools.Tools) != 1 || tools.Tools[0].Name != "tool-1" {
		t.Fatalf("cached tools = %#v ok=%v", tools, ok)
	}
	if resources, ok := manager.cache.resourcesSnapshot("alpha"); !ok || len(resources.Resources) != 1 {
		t.Fatalf("cached resources = %#v ok=%v", resources, ok)
	}
	if prompts, ok := manager.cache.promptsSnapshot("alpha"); !ok || len(prompts.Prompts) != 1 {
		t.Fatalf("cached prompts = %#v ok=%v", prompts, ok)
	}

	again, err := manager.GetOrConnect(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("second GetOrConnect: %v", err)
	}
	if again.Type != MCPStateConnected || calls.Load() != 1 {
		t.Fatalf("cached GetOrConnect state=%#v calls=%d, want one connection", again, calls.Load())
	}
}

func TestManagerReplaceWorkspaceConfigsReconnectsStdioInTargetCWDAndPreservesUserScope(t *testing.T) {
	var mu sync.Mutex
	var builtCWDs []string
	var transports []*managerTestTransport
	manager := NewManager(
		WithWorkingDirectory("/workspace/old"),
		withTestTransportFactory(func(_ context.Context, name string, _ catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
			mu.Lock()
			defer mu.Unlock()
			transport := newManagerTestTransport(name, len(transports)+1, nil)
			builtCWDs = append(builtCWDs, opts.CWD)
			transports = append(transports, transport)
			return transport, nil
		}),
	)
	projectConfig := catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "server", Scope: catalog.ScopeProject}
	manager.AddConfig("project", projectConfig)
	manager.AddConfig("user", catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "https://example.test/mcp", Scope: catalog.ScopeUser})
	if _, err := manager.GetOrConnect(context.Background(), "project"); err != nil {
		t.Fatalf("connect initial project server: %v", err)
	}

	manager.SetWorkingDirectory("/workspace/target")
	manager.ReplaceWorkspaceConfigs(map[string]catalog.MCPServerConfig{"project": projectConfig})
	if _, err := manager.GetOrConnect(context.Background(), "project"); err != nil {
		t.Fatalf("connect target project server: %v", err)
	}

	mu.Lock()
	gotCWDs := append([]string(nil), builtCWDs...)
	gotTransports := append([]*managerTestTransport(nil), transports...)
	mu.Unlock()
	if !reflect.DeepEqual(gotCWDs, []string{"/workspace/old", "/workspace/target"}) {
		t.Fatalf("stdio transport CWDs = %v", gotCWDs)
	}
	oldClosed := false
	if len(gotTransports) > 0 {
		gotTransports[0].closeMu.Lock()
		oldClosed = gotTransports[0].isClosed
		gotTransports[0].closeMu.Unlock()
	}
	if len(gotTransports) != 2 || !oldClosed {
		t.Fatalf("old project transport not retired: transports=%d oldClosed=%v", len(gotTransports), oldClosed)
	}
	if names := manager.ServerNames(); !reflect.DeepEqual(names, []string{"project", "user"}) {
		t.Fatalf("server names = %v, want project plus preserved user", names)
	}
}

func TestManagerReplaceWorkspaceConfigsRestoresShadowedUserConfig(t *testing.T) {
	manager := NewManager()
	manager.AddConfig("shared", catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "https://user.example/mcp", Scope: catalog.ScopeUser})
	manager.ReplaceWorkspaceConfigs(map[string]catalog.MCPServerConfig{
		"shared": {Type: catalog.TransportStdio, Command: "project-command", Scope: catalog.ScopeProject},
	})
	manager.ReplaceWorkspaceConfigs(nil)

	state, ok := manager.State("shared")
	if !ok || state.Config.Scope != catalog.ScopeUser || state.Config.URL != "https://user.example/mcp" {
		t.Fatalf("shadowed user config was not restored: ok=%v state=%+v", ok, state)
	}
}

func TestManagerDisabledServersNeverConnectAndCanBeEnabled(t *testing.T) {
	var calls atomic.Int32
	manager := NewManager(withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
		calls.Add(1)
		return newManagerTestTransport(name, int(calls.Load()), nil), nil
	}))
	manager.AddConfig("off", catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "http://example.test/mcp"})
	disabled, err := manager.ToggleEnabled(context.Background(), "off", false)
	if err != nil {
		t.Fatalf("ToggleEnabled(false): %v", err)
	}
	if disabled.Type != MCPStateDisabled {
		t.Fatalf("disabled state = %#v", disabled)
	}

	states, err := manager.ConnectAll(context.Background())
	if err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("disabled server connected %d times", calls.Load())
	}
	if states["off"].Type != MCPStateDisabled {
		t.Fatalf("ConnectAll disabled state = %#v", states["off"])
	}

	enabled, err := manager.ToggleEnabled(context.Background(), "off", true)
	if err != nil {
		t.Fatalf("ToggleEnabled(true): %v", err)
	}
	if enabled.Type != MCPStateConnected || calls.Load() != 1 {
		t.Fatalf("enabled state=%#v calls=%d", enabled, calls.Load())
	}
}

func TestManagerRecordsFailedAndNeedsAuthStates(t *testing.T) {
	t.Run("failed", func(t *testing.T) {
		manager := NewManager(withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
			return nil, errors.New("boom")
		}))
		manager.AddConfig("bad", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake"})
		state, err := manager.GetOrConnect(context.Background(), "bad")
		if err != nil {
			t.Fatalf("GetOrConnect: %v", err)
		}
		if state.Type != MCPStateFailed || state.Error == "" {
			t.Fatalf("failed state = %#v", state)
		}
	})

	t.Run("cached needs auth skips transport", func(t *testing.T) {
		cache := mcpauth.NewNeedsAuthCache(time.Hour)
		cfg := catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "http://example.test/mcp"}
		cache.Put(mcpauth.ServerKey("remote", cfg.AuthDescriptor()), mcpauth.NeedsAuthState{
			ServerName: "remote",
			ServerURL:  cfg.URL,
			Transport:  string(cfg.Type),
			StatusCode: 401,
			Reason:     "unauthorized",
		})
		var calls atomic.Int32
		manager := NewManager(
			withTestNeedsAuthCache(cache),
			withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
				calls.Add(1)
				return newManagerTestTransport(name, 1, nil), nil
			}),
		)
		manager.AddConfig("remote", cfg)
		state, err := manager.GetOrConnect(context.Background(), "remote")
		if err != nil {
			t.Fatalf("GetOrConnect: %v", err)
		}
		if state.Type != MCPStateNeedsAuth || state.NeedsAuth == nil {
			t.Fatalf("needs-auth state = %#v", state)
		}
		if calls.Load() != 0 {
			t.Fatalf("cached needs-auth still called transport %d times", calls.Load())
		}
	})

	t.Run("unauthorized transport records needs auth", func(t *testing.T) {
		cache := mcpauth.NewNeedsAuthCache(time.Hour)
		cfg := catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "http://example.test/mcp"}
		manager := NewManager(
			withTestNeedsAuthCache(cache),
			withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
				return nil, &mcpauth.UnauthorizedError{ServerURL: cfg.URL, StatusCode: 401}
			}),
		)
		manager.AddConfig("remote", cfg)
		state, err := manager.GetOrConnect(context.Background(), "remote")
		if err != nil {
			t.Fatalf("GetOrConnect: %v", err)
		}
		if state.Type != MCPStateNeedsAuth || state.NeedsAuth == nil {
			t.Fatalf("needs-auth state = %#v", state)
		}
		if _, ok := cache.Get(mcpauth.ServerKey("remote", cfg.AuthDescriptor())); !ok {
			t.Fatalf("needs-auth cache was not populated")
		}
	})
}

func TestManagerReconnectClearsFetchCachesAndClosesOldClient(t *testing.T) {
	var calls atomic.Int32
	var closes atomic.Int32
	manager := NewManager(withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
		generation := int(calls.Add(1))
		return newManagerTestTransport(name, generation, func() { closes.Add(1) }), nil
	}))
	manager.AddConfig("alpha", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake"})

	first, err := manager.GetOrConnect(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("GetOrConnect: %v", err)
	}
	if first.Tools[0].Name != "tool-1" {
		t.Fatalf("first tools = %#v", first.Tools)
	}
	second, err := manager.Reconnect(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Reconnect: %v", err)
	}
	if second.Type != MCPStateConnected || second.Tools[0].Name != "tool-2" {
		t.Fatalf("reconnected state = %#v", second)
	}
	if closes.Load() != 1 {
		t.Fatalf("old client close count = %d, want 1", closes.Load())
	}
	if tools, ok := manager.cache.toolsSnapshot("alpha"); !ok || tools.Tools[0].Name != "tool-2" {
		t.Fatalf("cache after reconnect = %#v ok=%v", tools, ok)
	}
}

func TestManagerConfigHashChangeDropsStaleConnectionAndCaches(t *testing.T) {
	var calls atomic.Int32
	var closes atomic.Int32
	manager := NewManager(withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
		generation := int(calls.Add(1))
		return newManagerTestTransport(name, generation, func() { closes.Add(1) }), nil
	}))
	manager.AddConfig("alpha", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake", Args: []string{"one"}})
	if _, err := manager.GetOrConnect(context.Background(), "alpha"); err != nil {
		t.Fatalf("GetOrConnect: %v", err)
	}
	if _, ok := manager.cache.toolsSnapshot("alpha"); !ok {
		t.Fatalf("expected tools cache after first connect")
	}

	manager.AddConfig("alpha", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake", Args: []string{"two"}})
	state, ok := manager.State("alpha")
	if !ok || state.Type != MCPStatePending || state.Client != nil {
		t.Fatalf("state after config change = %#v ok=%v", state, ok)
	}
	if _, ok := manager.cache.toolsSnapshot("alpha"); ok {
		t.Fatalf("tools cache survived config hash change")
	}
	if closes.Load() != 1 {
		t.Fatalf("close count after config change = %d, want 1", closes.Load())
	}
	reconnected, err := manager.GetOrConnect(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("GetOrConnect after config change: %v", err)
	}
	if reconnected.Tools[0].Name != "tool-2" || calls.Load() != 2 {
		t.Fatalf("reconnected=%#v calls=%d", reconnected, calls.Load())
	}
}

func TestManagerShutdownClosesClientsAndLeavesReconnectableState(t *testing.T) {
	var closes atomic.Int32
	manager := NewManager(withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
		return newManagerTestTransport(name, 1, func() { closes.Add(1) }), nil
	}))
	manager.AddConfig("alpha", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake"})
	manager.AddConfig("beta", catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "http://example.test/mcp"})
	if _, err := manager.ConnectAll(context.Background()); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
	if closes.Load() != 2 {
		t.Fatalf("close count = %d, want 2", closes.Load())
	}
	for _, name := range []string{"alpha", "beta"} {
		state, ok := manager.State(name)
		if !ok || state.Type != MCPStatePending || state.Client != nil {
			t.Fatalf("state %s after shutdown = %#v ok=%v", name, state, ok)
		}
		if _, ok := manager.cache.toolsSnapshot(name); ok {
			t.Fatalf("tools cache for %s survived shutdown", name)
		}
	}
}

func TestManagerShutdownClosesEveryClientAfterCloseError(t *testing.T) {
	closeFailure := errors.New("close failed")
	var alphaCloses atomic.Int32
	var betaCloses atomic.Int32
	manager := NewManager(withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
		switch name {
		case "alpha":
			return &managerCloseErrorTransport{
				managerTestTransport: newManagerTestTransport(name, 1, func() { alphaCloses.Add(1) }),
				err:                  closeFailure,
			}, nil
		default:
			return newManagerTestTransport(name, 1, func() { betaCloses.Add(1) }), nil
		}
	}))
	manager.AddConfig("alpha", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake"})
	manager.AddConfig("beta", catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "http://example.test/mcp"})
	if _, err := manager.ConnectAll(context.Background()); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}

	err := manager.Shutdown(context.Background())
	if !errors.Is(err, closeFailure) {
		t.Fatalf("Shutdown error = %v, want close failure", err)
	}
	if alphaCloses.Load() != 1 || betaCloses.Load() != 1 {
		t.Fatalf("client close counts = alpha:%d beta:%d", alphaCloses.Load(), betaCloses.Load())
	}
	for _, name := range []string{"alpha", "beta"} {
		state, ok := manager.State(name)
		if !ok || state.Client != nil || state.Type != MCPStatePending {
			t.Fatalf("state %s after shutdown = %#v ok=%v", name, state, ok)
		}
	}
}

func TestManagerShutdownWaitsForInflightConnection(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	manager := NewManager(withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
		return nil, ctx.Err()
	}))
	manager.AddConfig("alpha", catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "http://example.test/mcp"})

	connectDone := make(chan struct{})
	go func() {
		_, _ = manager.GetOrConnect(context.Background(), "alpha")
		close(connectDone)
	}()
	<-started

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- manager.Shutdown(context.Background()) }()
	<-canceled
	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned before inflight connection exited: %v", err)
	default:
	}
	close(release)
	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	<-connectDone

	manager.mu.Lock()
	inflight := len(manager.inflight)
	manager.mu.Unlock()
	if inflight != 0 {
		t.Fatalf("inflight connections after shutdown = %d", inflight)
	}
	state, ok := manager.State("alpha")
	if !ok || state.Type != MCPStatePending || state.Client != nil {
		t.Fatalf("state after draining inflight connection = %#v ok=%v", state, ok)
	}
}

func TestManagerShutdownWaitsForReconnectLoop(t *testing.T) {
	var calls atomic.Int32
	policy := fastReconnectPolicy()
	policy.StdioCooldowns = []time.Duration{time.Second}
	manager := NewManager(
		withTestReconnectPolicy(policy),
		withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
			return newManagerTestTransport(name, int(calls.Add(1)), nil), nil
		}),
	)
	manager.AddConfig("remote", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake"})
	state, err := manager.GetOrConnect(context.Background(), "remote")
	if err != nil {
		t.Fatalf("GetOrConnect: %v", err)
	}
	if err := state.Client.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}
	waitForMCPTest(t, time.Second, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		return len(manager.reconnectTimers) == 1
	})

	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	manager.mu.Lock()
	reconnects := len(manager.reconnectTimers)
	reconnectRuns := len(manager.reconnectRuns)
	inflight := len(manager.inflight)
	manager.mu.Unlock()
	if reconnects != 0 || reconnectRuns != 0 || inflight != 0 {
		t.Fatalf("shutdown leaks: reconnects=%d reconnect runs=%d inflight=%d", reconnects, reconnectRuns, inflight)
	}
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("reconnect ran after shutdown: transport builds=%d", got)
	}
}

func TestManagerConnectAllUsesSeparateLocalAndRemoteConcurrency(t *testing.T) {
	type counters struct {
		mu      sync.Mutex
		current int
		max     int
	}
	inc := func(c *counters) {
		c.mu.Lock()
		c.current++
		if c.current > c.max {
			c.max = c.current
		}
		c.mu.Unlock()
	}
	dec := func(c *counters) {
		c.mu.Lock()
		c.current--
		c.mu.Unlock()
	}
	local := &counters{}
	remote := &counters{}

	manager := NewManager(
		withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
			counter := remote
			if isLocalManagerTransport(cfg) {
				counter = local
			}
			inc(counter)
			time.Sleep(20 * time.Millisecond)
			dec(counter)
			return newManagerTestTransport(name, 1, nil), nil
		}),
	)
	manager.localConcurrency = 1
	manager.remoteConcurrency = 2
	manager.SetConfigs(map[string]catalog.MCPServerConfig{
		"local-a":  {Type: catalog.TransportStdio, Command: "fake"},
		"local-b":  {Type: catalog.TransportStdio, Command: "fake"},
		"remote-a": {Type: catalog.TransportHTTP, URL: "http://a.example.test/mcp"},
		"remote-b": {Type: catalog.TransportHTTP, URL: "http://b.example.test/mcp"},
		"remote-c": {Type: catalog.TransportSSE, URL: "http://c.example.test/sse"},
	})
	if _, err := manager.ConnectAll(context.Background()); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	if local.max > 1 {
		t.Fatalf("local max concurrency = %d, want <= 1", local.max)
	}
	if remote.max > 2 {
		t.Fatalf("remote max concurrency = %d, want <= 2", remote.max)
	}
	if remote.max < 2 {
		t.Fatalf("remote max concurrency = %d, want to exercise parallel remote batch", remote.max)
	}
}
