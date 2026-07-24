package tools

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/registry"
	svcmcp "github.com/agent-dance/luban/services/mcp"
)

type notificationTestTransport struct {
	mu           sync.Mutex
	capabilities svcmcp.ServerCapabilities
	tools        []svcmcp.ToolDefinition
	resources    []svcmcp.Resource
	prompts      []svcmcp.PromptDefinition

	recv      chan svcmcp.JSONRPCMessage
	closed    chan struct{}
	closeOnce sync.Once
}

func newNotificationTestTransport() *notificationTestTransport {
	return &notificationTestTransport{
		capabilities: svcmcp.ServerCapabilities{
			"tools":     map[string]any{"listChanged": true},
			"resources": map[string]any{"listChanged": true},
			"prompts":   map[string]any{"listChanged": true},
		},
		tools: []svcmcp.ToolDefinition{{
			Name:        "old_tool",
			Description: "old searchable tool",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		resources: []svcmcp.Resource{{
			URI:      "memo://srv/old",
			Name:     "old resource",
			MimeType: "text/plain",
		}},
		prompts: []svcmcp.PromptDefinition{{
			Name:        "old_prompt",
			Description: "old prompt",
		}},
		recv:   make(chan svcmcp.JSONRPCMessage, 64),
		closed: make(chan struct{}),
	}
}

func (t *notificationTestTransport) Send(ctx context.Context, msg svcmcp.JSONRPCMessage) error {
	if len(msg.ID) == 0 {
		return nil
	}
	t.mu.Lock()
	capabilities := cloneNotificationCaps(t.capabilities)
	tools := append([]svcmcp.ToolDefinition(nil), t.tools...)
	resources := append([]svcmcp.Resource(nil), t.resources...)
	prompts := append([]svcmcp.PromptDefinition(nil), t.prompts...)
	t.mu.Unlock()

	var result any
	switch msg.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": svcmcp.MCPProtocolVersion,
			"capabilities":    capabilities,
			"serverInfo":      map[string]any{"name": "notification-test", "version": "1"},
		}
	case "tools/list":
		result = map[string]any{"tools": tools}
	case "resources/list":
		result = map[string]any{"resources": resources}
	case "prompts/list":
		result = map[string]any{"prompts": prompts}
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

func (t *notificationTestTransport) Receive(ctx context.Context) (svcmcp.JSONRPCMessage, error) {
	select {
	case msg := <-t.recv:
		return msg, nil
	case <-ctx.Done():
		return svcmcp.JSONRPCMessage{}, ctx.Err()
	case <-t.closed:
		return svcmcp.JSONRPCMessage{}, svcmcp.ErrTransportClosed
	}
}

func (t *notificationTestTransport) Close() error {
	t.closeOnce.Do(func() { close(t.closed) })
	return nil
}

func (t *notificationTestTransport) emit(tb testing.TB, method string) {
	tb.Helper()
	msg, err := svcmcp.NewNotificationMessage(method, nil)
	if err != nil {
		tb.Fatalf("NewNotificationMessage: %v", err)
	}
	select {
	case t.recv <- msg:
	case <-time.After(2 * time.Second):
		tb.Fatalf("timed out emitting %s", method)
	}
}

func (t *notificationTestTransport) setTools(tools []svcmcp.ToolDefinition) {
	t.mu.Lock()
	t.tools = append([]svcmcp.ToolDefinition(nil), tools...)
	t.mu.Unlock()
}

func (t *notificationTestTransport) setResources(resources []svcmcp.Resource) {
	t.mu.Lock()
	t.resources = append([]svcmcp.Resource(nil), resources...)
	t.mu.Unlock()
}

func (t *notificationTestTransport) setPrompts(prompts []svcmcp.PromptDefinition) {
	t.mu.Lock()
	t.prompts = append([]svcmcp.PromptDefinition(nil), prompts...)
	t.mu.Unlock()
}

func (t *notificationTestTransport) setCapabilities(caps svcmcp.ServerCapabilities) {
	t.mu.Lock()
	t.capabilities = caps
	t.mu.Unlock()
}

func TestServiceMCPToolsListChangedRefreshesDynamicRegistryAndToolSearch(t *testing.T) {
	transport := newNotificationTestTransport()
	manager := newNotificationTestManager(t, transport)

	reg := registry.New()
	RegisterDynamicMCPTools(reg, manager, nil)
	oldName := svcmcp.BuildMCPToolName("srv", "old_tool")
	if reg.Get(oldName) == nil {
		t.Fatalf("initial dynamic tool %q not registered", oldName)
	}

	search := &ToolSearchTool{Registry: reg}
	if result, err := search.Execute(context.Background(), map[string]any{"query": "old", "max_results": 1}); err != nil || result.IsError {
		t.Fatalf("prime ToolSearch result=%#v err=%v", result, err)
	}
	if search.Cache().Len() == 0 {
		t.Fatalf("expected ToolSearch cache to be primed")
	}

	var invalidations atomic.Int32
	defer SetToolSearchInvalidator(nil)
	SetToolSearchInvalidator(func(name string) {
		if name == "srv" {
			invalidations.Add(1)
		}
	})
	unregister := RegisterMCPListChangedInvalidators(reg, manager, nil, search)
	defer unregister()

	transport.setTools([]svcmcp.ToolDefinition{{
		Name:        "new_tool",
		Description: "fresh searchable tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	transport.emit(t, svcmcp.NotificationToolsListChanged)

	newName := svcmcp.BuildMCPToolName("srv", "new_tool")
	eventually(t, func() bool {
		cached, ok := manager.Cache().Tools("srv")
		return ok &&
			len(cached.Tools) == 1 &&
			cached.Tools[0].Name == "new_tool" &&
			reg.Get(newName) != nil &&
			reg.Get(oldName) == nil &&
			search.Cache().Len() == 0 &&
			invalidations.Load() > 0
	})
}

func TestServiceMCPResourcesListChangedRefreshesResourceCache(t *testing.T) {
	transport := newNotificationTestTransport()
	manager := newNotificationTestManager(t, transport)

	var invalidations atomic.Int32
	defer SetResourceListInvalidator(nil)
	SetResourceListInvalidator(func(name string) {
		if name == "srv" {
			invalidations.Add(1)
		}
	})
	unregister := RegisterMCPListChangedInvalidators(nil, nil, nil)
	defer unregister()

	transport.setResources([]svcmcp.Resource{{
		URI:         "memo://srv/new",
		Name:        "new resource",
		Description: "fresh resource",
		MimeType:    "text/plain",
	}})
	transport.emit(t, svcmcp.NotificationResourcesListChanged)

	eventually(t, func() bool {
		cached, ok := manager.Cache().Resources("srv")
		return ok &&
			len(cached.Resources) == 1 &&
			cached.Resources[0].URI == "memo://srv/new" &&
			invalidations.Load() > 0
	})

	tool := NewListMcpResourcesTool(nil, manager)
	result, err := tool.Execute(context.Background(), map[string]any{"server": "srv"})
	if err != nil {
		t.Fatalf("ListMcpResourcesTool: %v", err)
	}
	if !strings.Contains(result.Content, "memo://srv/new") || strings.Contains(result.Content, "memo://srv/old") {
		t.Fatalf("resource list did not use refreshed cache: %s", result.Content)
	}
}

func TestServiceMCPPromptsListChangedRefreshesPromptCacheAndHooks(t *testing.T) {
	transport := newNotificationTestTransport()
	manager := newNotificationTestManager(t, transport)

	var promptCacheInvalidations atomic.Int32
	unregisterPromptHook := svcmcp.RegisterPromptCacheInvalidationHook(func(name string) {
		if name == "srv" {
			promptCacheInvalidations.Add(1)
		}
	})
	defer unregisterPromptHook()

	var promptListInvalidations atomic.Int32
	defer SetPromptListInvalidator(nil)
	SetPromptListInvalidator(func(name string) {
		if name == "srv" {
			promptListInvalidations.Add(1)
		}
	})
	unregister := RegisterMCPListChangedInvalidators(nil, nil, nil)
	defer unregister()

	transport.setPrompts([]svcmcp.PromptDefinition{{
		Name:        "new_prompt",
		Description: "fresh prompt",
		Arguments:   []svcmcp.PromptArgument{{Name: "topic", Required: true}},
	}})
	transport.emit(t, svcmcp.NotificationPromptsListChanged)

	eventually(t, func() bool {
		cached, ok := manager.Cache().Prompts("srv")
		return ok &&
			len(cached.Prompts) == 1 &&
			cached.Prompts[0].Name == "new_prompt" &&
			promptCacheInvalidations.Load() > 0 &&
			promptListInvalidations.Load() > 0
	})
}

func TestServiceMCPListChangedHandlersRespectCapabilities(t *testing.T) {
	transport := newNotificationTestTransport()
	transport.setCapabilities(svcmcp.ServerCapabilities{
		"tools": map[string]any{"listChanged": false},
	})
	manager := newNotificationTestManager(t, transport)

	events := make(chan svcmcp.ListChangedEvent, 1)
	unregister := svcmcp.RegisterListChangedHook(func(event svcmcp.ListChangedEvent) {
		events <- event
	})
	defer unregister()

	transport.setTools([]svcmcp.ToolDefinition{{Name: "should_not_refresh"}})
	transport.emit(t, svcmcp.NotificationToolsListChanged)

	select {
	case event := <-events:
		t.Fatalf("unexpected list_changed event without advertised capability: %#v", event)
	case <-time.After(150 * time.Millisecond):
	}
	cached, ok := manager.Cache().Tools("srv")
	if !ok || len(cached.Tools) != 1 || cached.Tools[0].Name != "old_tool" {
		t.Fatalf("tools cache changed despite listChanged=false: %#v ok=%v", cached, ok)
	}
}

// TestMCPNotificationToolsListChanged verifies the legacy MCPManager shim
// still fires the registered ToolSearch invalidator.
func TestMCPNotificationToolsListChanged(t *testing.T) {
	var fired atomic.Int32
	defer SetToolSearchInvalidator(nil)
	SetToolSearchInvalidator(func(name string) {
		if name == "alpha" {
			fired.Add(1)
		}
	})

	mgr := NewMCPManager()
	mgr.AddServer(&MCPServer{Name: "alpha"})
	mgr.HandleMCPNotification(context.Background(), "alpha",
		svcmcp.NotificationToolsListChanged, json.RawMessage(`{}`))
	if fired.Load() != 1 {
		t.Fatalf("expected ToolSearch invalidator to fire once, got %d", fired.Load())
	}
}

// TestMCPNotificationResourcesListChanged sets the legacy resources-changed
// flag for the affected server.
func TestMCPNotificationResourcesListChanged(t *testing.T) {
	mgr := NewMCPManager()
	mgr.AddServer(&MCPServer{Name: "beta"})

	if ResourcesChanged("beta") {
		t.Fatalf("flag should start clean")
	}
	mgr.HandleMCPNotification(context.Background(), "beta",
		svcmcp.NotificationResourcesListChanged, nil)
	if !ResourcesChanged("beta") {
		t.Fatalf("flag should be set after notification")
	}
	if !ConsumeResourcesChanged("beta") {
		t.Fatalf("consume should observe true")
	}
	if ResourcesChanged("beta") {
		t.Fatalf("flag should clear after consume")
	}
}

// TestMCPNotificationUnknownMethodNoOp covers the silent-ignore branch.
func TestMCPNotificationUnknownMethodNoOp(t *testing.T) {
	defer SetToolSearchInvalidator(nil)
	SetToolSearchInvalidator(func(string) {
		t.Fatalf("invalidator should not fire for unknown methods")
	})
	mgr := NewMCPManager()
	mgr.HandleMCPNotification(context.Background(), "x",
		"notifications/cancelled", nil)
}

func newNotificationTestManager(tb testing.TB, transport *notificationTestTransport) *svcmcp.Manager {
	tb.Helper()
	manager := svcmcp.NewManager(svcmcp.WithTransportFactory(func(context.Context, string, svcmcp.MCPServerConfig, svcmcp.TransportBuildOptions) (svcmcp.Transport, error) {
		return transport, nil
	}))
	manager.AddConfig("srv", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "fake"})
	state, err := manager.GetOrConnect(context.Background(), "srv")
	if err != nil {
		tb.Fatalf("GetOrConnect: %v", err)
	}
	if state.Type != svcmcp.MCPStateConnected {
		tb.Fatalf("state = %#v, want connected", state)
	}
	tb.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	return manager
}

func eventually(tb testing.TB, ok func() bool) {
	tb.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ok() {
		return
	}
	tb.Fatalf("condition was not satisfied before timeout")
}

func cloneNotificationCaps(in svcmcp.ServerCapabilities) svcmcp.ServerCapabilities {
	out := make(svcmcp.ServerCapabilities, len(in))
	for key, value := range in {
		if fields, ok := value.(map[string]any); ok {
			copied := make(map[string]any, len(fields))
			for field, fieldValue := range fields {
				copied[field] = fieldValue
			}
			out[key] = copied
			continue
		}
		out[key] = value
	}
	return out
}
