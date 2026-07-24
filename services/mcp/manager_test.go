package mcp

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
)

type managerTestTransport struct {
	name string

	capabilities ServerCapabilities
	serverInfo   *ServerInfo
	instructions string
	tools        []ToolDefinition
	resources    []Resource
	prompts      []PromptDefinition

	recv     chan JSONRPCMessage
	closed   chan struct{}
	closeMu  sync.Mutex
	isClosed bool

	onClose func()
}

func newManagerTestTransport(name string, generation int, onClose func()) *managerTestTransport {
	return &managerTestTransport{
		name: name,
		capabilities: ServerCapabilities{
			"tools":     map[string]any{"listChanged": true},
			"resources": map[string]any{"listChanged": true},
			"prompts":   map[string]any{"listChanged": true},
		},
		serverInfo:   &ServerInfo{Name: "server-" + name, Version: fmt.Sprintf("v%d", generation)},
		instructions: "instructions for " + name,
		tools: []ToolDefinition{{
			Name:        fmt.Sprintf("tool-%d", generation),
			Description: "test tool",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Annotations: map[string]any{
				"readOnlyHint": true,
			},
		}},
		resources: []Resource{{
			URI:         fmt.Sprintf("memo://%s/%d", name, generation),
			Name:        "memo",
			Description: "test resource",
			MimeType:    "text/plain",
		}},
		prompts: []PromptDefinition{{
			Name:        fmt.Sprintf("prompt-%d", generation),
			Description: "test prompt",
			Arguments:   []PromptArgument{{Name: "topic", Required: true}},
		}},
		recv:    make(chan JSONRPCMessage, 16),
		closed:  make(chan struct{}),
		onClose: onClose,
	}
}

func (t *managerTestTransport) Send(ctx context.Context, msg JSONRPCMessage) error {
	t.closeMu.Lock()
	closed := t.isClosed
	t.closeMu.Unlock()
	if closed {
		return &TransportClosedError{Reason: "manager test transport closed", Err: ErrTransportClosed}
	}
	if len(msg.ID) == 0 {
		return nil
	}
	var result any
	switch msg.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": MCPProtocolVersion,
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
	response, err := NewResultMessage(msg.ID, result)
	if err != nil {
		return err
	}
	select {
	case t.recv <- response:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closed:
		return &TransportClosedError{Reason: "manager test transport closed", Err: ErrTransportClosed}
	}
}

func (t *managerTestTransport) Receive(ctx context.Context) (JSONRPCMessage, error) {
	select {
	case msg := <-t.recv:
		return msg, nil
	case <-ctx.Done():
		return JSONRPCMessage{}, ctx.Err()
	case <-t.closed:
		return JSONRPCMessage{}, &TransportClosedError{Reason: "manager test transport closed", Err: ErrTransportClosed}
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

func TestManagerConnectStoresStateRegistryAndCaches(t *testing.T) {
	var calls atomic.Int32
	manager := NewManager(WithTransportFactory(func(ctx context.Context, name string, cfg MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
		calls.Add(1)
		return newManagerTestTransport(name, int(calls.Load()), nil), nil
	}))
	manager.AddConfig("alpha", MCPServerConfig{Type: TransportStdio, Command: "fake"})

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
	if state.Client == nil || !state.Client.IsInitialized() {
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

	if tools, ok := manager.Cache().Tools("alpha"); !ok || len(tools.Tools) != 1 || tools.Tools[0].Name != "tool-1" {
		t.Fatalf("cached tools = %#v ok=%v", tools, ok)
	}
	if resources, ok := manager.Cache().Resources("alpha"); !ok || len(resources.Resources) != 1 {
		t.Fatalf("cached resources = %#v ok=%v", resources, ok)
	}
	if prompts, ok := manager.Cache().Prompts("alpha"); !ok || len(prompts.Prompts) != 1 {
		t.Fatalf("cached prompts = %#v ok=%v", prompts, ok)
	}

	registryState := manager.Registry().Snapshot()
	if len(registryState) != 1 || registryState[0].Name != "alpha" || registryState[0].Type != MCPStateConnected {
		t.Fatalf("registry snapshot = %#v", registryState)
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
		WithTransportFactory(func(_ context.Context, name string, _ MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
			mu.Lock()
			defer mu.Unlock()
			transport := newManagerTestTransport(name, len(transports)+1, nil)
			builtCWDs = append(builtCWDs, opts.CWD)
			transports = append(transports, transport)
			return transport, nil
		}),
	)
	projectConfig := MCPServerConfig{Type: TransportStdio, Command: "server", Scope: ScopeProject}
	manager.AddConfig("project", projectConfig)
	manager.AddConfig("user", MCPServerConfig{Type: TransportHTTP, URL: "https://example.test/mcp", Scope: ScopeUser})
	if _, err := manager.GetOrConnect(context.Background(), "project"); err != nil {
		t.Fatalf("connect initial project server: %v", err)
	}

	manager.SetWorkingDirectory("/workspace/target")
	manager.ReplaceWorkspaceConfigs(map[string]MCPServerConfig{"project": projectConfig})
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
	manager.AddConfig("shared", MCPServerConfig{Type: TransportHTTP, URL: "https://user.example/mcp", Scope: ScopeUser})
	manager.ReplaceWorkspaceConfigs(map[string]MCPServerConfig{
		"shared": {Type: TransportStdio, Command: "project-command", Scope: ScopeProject},
	})
	manager.ReplaceWorkspaceConfigs(nil)

	state, ok := manager.State("shared")
	if !ok || state.Config.Scope != ScopeUser || state.Config.URL != "https://user.example/mcp" {
		t.Fatalf("shadowed user config was not restored: ok=%v state=%+v", ok, state)
	}
}

func TestManagerDisabledServersNeverConnectAndCanBeEnabled(t *testing.T) {
	var calls atomic.Int32
	manager := NewManager(WithTransportFactory(func(ctx context.Context, name string, cfg MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
		calls.Add(1)
		return newManagerTestTransport(name, int(calls.Load()), nil), nil
	}))
	manager.AddConfig("off", MCPServerConfig{Type: TransportHTTP, URL: "http://example.test/mcp"})
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
		manager := NewManager(WithTransportFactory(func(ctx context.Context, name string, cfg MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
			return nil, errors.New("boom")
		}))
		manager.AddConfig("bad", MCPServerConfig{Type: TransportStdio, Command: "fake"})
		state, err := manager.GetOrConnect(context.Background(), "bad")
		if err != nil {
			t.Fatalf("GetOrConnect: %v", err)
		}
		if state.Type != MCPStateFailed || state.Error == "" {
			t.Fatalf("failed state = %#v", state)
		}
	})

	t.Run("cached needs auth skips transport", func(t *testing.T) {
		cache := NewNeedsAuthCache(time.Hour)
		cfg := MCPServerConfig{Type: TransportHTTP, URL: "http://example.test/mcp"}
		cache.Put(ServerKey("remote", cfg), NeedsAuthState{
			ServerName: "remote",
			ServerURL:  cfg.URL,
			Transport:  cfg.Type,
			StatusCode: 401,
			Reason:     "unauthorized",
		})
		var calls atomic.Int32
		manager := NewManager(
			WithNeedsAuthCache(cache),
			WithTransportFactory(func(ctx context.Context, name string, cfg MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
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
		cache := NewNeedsAuthCache(time.Hour)
		cfg := MCPServerConfig{Type: TransportHTTP, URL: "http://example.test/mcp"}
		manager := NewManager(
			WithNeedsAuthCache(cache),
			WithTransportFactory(func(ctx context.Context, name string, cfg MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
				return nil, &UnauthorizedError{ServerURL: cfg.URL, StatusCode: 401}
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
		if _, ok := cache.Get(ServerKey("remote", cfg)); !ok {
			t.Fatalf("needs-auth cache was not populated")
		}
	})
}

func TestManagerReconnectClearsFetchCachesAndClosesOldClient(t *testing.T) {
	var calls atomic.Int32
	var closes atomic.Int32
	manager := NewManager(WithTransportFactory(func(ctx context.Context, name string, cfg MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
		generation := int(calls.Add(1))
		return newManagerTestTransport(name, generation, func() { closes.Add(1) }), nil
	}))
	manager.AddConfig("alpha", MCPServerConfig{Type: TransportStdio, Command: "fake"})

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
	if tools, ok := manager.Cache().Tools("alpha"); !ok || tools.Tools[0].Name != "tool-2" {
		t.Fatalf("cache after reconnect = %#v ok=%v", tools, ok)
	}
}

func TestManagerConfigHashChangeDropsStaleConnectionAndCaches(t *testing.T) {
	var calls atomic.Int32
	var closes atomic.Int32
	manager := NewManager(WithTransportFactory(func(ctx context.Context, name string, cfg MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
		generation := int(calls.Add(1))
		return newManagerTestTransport(name, generation, func() { closes.Add(1) }), nil
	}))
	manager.AddConfig("alpha", MCPServerConfig{Type: TransportStdio, Command: "fake", Args: []string{"one"}})
	if _, err := manager.GetOrConnect(context.Background(), "alpha"); err != nil {
		t.Fatalf("GetOrConnect: %v", err)
	}
	if _, ok := manager.Cache().Tools("alpha"); !ok {
		t.Fatalf("expected tools cache after first connect")
	}

	manager.AddConfig("alpha", MCPServerConfig{Type: TransportStdio, Command: "fake", Args: []string{"two"}})
	state, ok := manager.State("alpha")
	if !ok || state.Type != MCPStatePending || state.Client != nil {
		t.Fatalf("state after config change = %#v ok=%v", state, ok)
	}
	if _, ok := manager.Cache().Tools("alpha"); ok {
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

func TestHashMCPConfigIgnoresScope(t *testing.T) {
	a := MCPServerConfig{Type: TransportHTTP, URL: "http://example.test/mcp", Scope: ScopeProject}
	b := MCPServerConfig{Type: TransportHTTP, URL: "http://example.test/mcp", Scope: ScopeUser}
	if HashMCPConfig(a) != HashMCPConfig(b) {
		t.Fatalf("config hash should ignore scope: %s != %s", HashMCPConfig(a), HashMCPConfig(b))
	}
}

func TestManagerShutdownClosesClientsAndLeavesReconnectableState(t *testing.T) {
	var closes atomic.Int32
	manager := NewManager(WithTransportFactory(func(ctx context.Context, name string, cfg MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
		return newManagerTestTransport(name, 1, func() { closes.Add(1) }), nil
	}))
	manager.AddConfig("alpha", MCPServerConfig{Type: TransportStdio, Command: "fake"})
	manager.AddConfig("beta", MCPServerConfig{Type: TransportHTTP, URL: "http://example.test/mcp"})
	if _, err := manager.ConnectAll(context.Background()); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if closes.Load() != 2 {
		t.Fatalf("close count = %d, want 2", closes.Load())
	}
	for _, name := range []string{"alpha", "beta"} {
		state, ok := manager.State(name)
		if !ok || state.Type != MCPStatePending || state.Client != nil {
			t.Fatalf("state %s after shutdown = %#v ok=%v", name, state, ok)
		}
		if _, ok := manager.Cache().Tools(name); ok {
			t.Fatalf("tools cache for %s survived shutdown", name)
		}
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
		WithConnectionConcurrency(1, 2),
		WithTransportFactory(func(ctx context.Context, name string, cfg MCPServerConfig, opts TransportBuildOptions) (Transport, error) {
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
	manager.SetConfigs(map[string]MCPServerConfig{
		"local-a":  {Type: TransportStdio, Command: "fake"},
		"local-b":  {Type: TransportStdio, Command: "fake"},
		"remote-a": {Type: TransportHTTP, URL: "http://a.example.test/mcp"},
		"remote-b": {Type: TransportHTTP, URL: "http://b.example.test/mcp"},
		"remote-c": {Type: TransportSSE, URL: "http://c.example.test/sse"},
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
