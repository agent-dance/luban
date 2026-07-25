package manager

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	"github.com/agent-dance/luban/internal/mcp/protocol"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
)

type blockingListTransport struct {
	*managerTestTransport
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (t *blockingListTransport) Send(ctx context.Context, message protocol.JSONRPCMessage) error {
	if message.Method == "tools/list" {
		t.once.Do(func() { close(t.started) })
		select {
		case <-t.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return t.managerTestTransport.Send(ctx, message)
}

func TestListChangedRejectsLateWorkspaceClientCatalog(t *testing.T) {
	manager := NewManager()
	configA := normalizeManagerConfig("shared", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace-a", Scope: catalog.ScopeProject})
	configB := normalizeManagerConfig("shared", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace-b", Scope: catalog.ScopeProject})
	manager.AddConfig("shared", configB)

	transportA := newManagerTestTransport("shared", 1, nil)
	clientA, err := mcptransport.NewClient(context.Background(), transportA)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientA.Close() })

	transportB := newManagerTestTransport("shared", 2, nil)
	clientB, err := mcptransport.NewClient(context.Background(), transportB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientB.Close() })
	stateB, catalogB, err := manager.connectedStateFromClient(context.Background(), "shared", configB, clientB)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.publishConnectionCatalogLocked(catalogB)
	manager.setStateLocked(stateB)
	manager.mu.Unlock()

	for _, kind := range []ListChangedKind{ListChangedTools, ListChangedResources, ListChangedPrompts} {
		event := manager.cache.refreshListChanged(context.Background(), "shared", kind, clientA)
		if event.Err != nil {
			t.Fatalf("old client %s refresh failed before authority check: %v", kind, event.Err)
		}
		if event.authoritative {
			t.Fatalf("old workspace %s refresh was accepted", kind)
		}
	}
	assertWorkspaceBCatalog(t, manager)

	// Exact client identity alone is insufficient: if the configured workspace
	// changes before its state is replaced, the config hash fence still rejects
	// the old result.
	manager.mu.Lock()
	manager.states["shared"] = MCPServerConnection{
		Name:       "shared",
		Type:       MCPStateConnected,
		Config:     configA,
		ConfigHash: catalog.HashMCPConfig(configA),
		Client:     clientA,
	}
	manager.mu.Unlock()
	event := manager.cache.refreshListChanged(context.Background(), "shared", ListChangedTools, clientA)
	if event.authoritative {
		t.Fatal("old workspace config was accepted despite the target config hash")
	}
	assertWorkspaceBCache(t, manager.cache)
}

func TestListChangedPublishesCurrentClientCatalog(t *testing.T) {
	manager := NewManager()
	config := normalizeManagerConfig("shared", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace", Scope: catalog.ScopeProject})
	manager.AddConfig("shared", config)
	transport := newManagerTestTransport("shared", 1, nil)
	client, err := mcptransport.NewClient(context.Background(), transport)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	state, catalog, err := manager.connectedStateFromClient(context.Background(), "shared", config, client)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.publishConnectionCatalogLocked(catalog)
	manager.setStateLocked(state)
	manager.mu.Unlock()

	transport.tools = newManagerTestTransport("shared", 2, nil).tools
	transport.resources = newManagerTestTransport("shared", 2, nil).resources
	transport.prompts = newManagerTestTransport("shared", 2, nil).prompts
	for _, kind := range []ListChangedKind{ListChangedTools, ListChangedResources, ListChangedPrompts} {
		event := manager.cache.refreshListChanged(context.Background(), "shared", kind, client)
		if event.Err != nil || !event.authoritative {
			t.Fatalf("current client %s refresh: authoritative=%v err=%v", kind, event.authoritative, event.Err)
		}
	}
	assertWorkspaceBCatalog(t, manager)
}

func TestListChangedPendingRefreshUsesNewestClient(t *testing.T) {
	manager := NewManager()
	config := normalizeManagerConfig("shared", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace-b", Scope: catalog.ScopeProject})
	manager.AddConfig("shared", config)

	oldTransport := &blockingListTransport{
		managerTestTransport: newManagerTestTransport("shared", 1, nil),
		started:              make(chan struct{}),
		release:              make(chan struct{}),
	}
	oldClient, err := mcptransport.NewClient(context.Background(), oldTransport)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = oldClient.Close() })

	currentTransport := newManagerTestTransport("shared", 2, nil)
	currentClient, err := mcptransport.NewClient(context.Background(), currentTransport)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = currentClient.Close() })
	state, catalog, err := manager.connectedStateFromClient(context.Background(), "shared", config, currentClient)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.publishConnectionCatalogLocked(catalog)
	manager.setStateLocked(state)
	manager.mu.Unlock()
	currentTransport.tools = newManagerTestTransport("shared", 3, nil).tools

	hooks := make(chan ListChangedEvent, 2)
	unregister := RegisterListChangedHook(func(event ListChangedEvent) {
		if event.Manager == manager && event.ServerName == "shared" && event.Kind == ListChangedTools {
			hooks <- event
		}
	})
	t.Cleanup(unregister)

	manager.cache.scheduleListChangedRefresh("shared", ListChangedTools, oldClient)
	select {
	case <-oldTransport.started:
	case <-time.After(time.Second):
		t.Fatal("old client refresh did not start")
	}
	manager.cache.scheduleListChangedRefresh("shared", ListChangedTools, currentClient)
	close(oldTransport.release)

	waitForMCPTest(t, time.Second, func() bool {
		tools, ok := manager.cache.toolsSnapshot("shared")
		return ok && len(tools.Tools) == 1 && tools.Tools[0].Name == "tool-3"
	})
	select {
	case event := <-hooks:
		if len(event.Tools) != 1 || event.Tools[0].Name != "tool-3" {
			t.Fatalf("hook received stale tools: %#v", event.Tools)
		}
	case <-time.After(time.Second):
		t.Fatal("current client refresh did not publish a hook")
	}
	select {
	case event := <-hooks:
		t.Fatalf("stale client unexpectedly published a hook: %#v", event)
	case <-time.After(20 * time.Millisecond):
	}
}

func assertWorkspaceBCatalog(t *testing.T, manager *Manager) {
	t.Helper()
	state, ok := manager.State("shared")
	if !ok || len(state.Tools) != 1 || state.Tools[0].Name != "tool-2" ||
		len(state.Resources) != 1 || state.Resources[0].URI != "memo://shared/2" ||
		len(state.Prompts) != 1 || state.Prompts[0].Name != "prompt-2" {
		t.Fatalf("manager catalog was not workspace B: %#v, found=%v", state, ok)
	}
	assertWorkspaceBCache(t, manager.cache)
}

func assertWorkspaceBCache(t *testing.T, cache *cache) {
	t.Helper()
	tools, toolsOK := cache.toolsSnapshot("shared")
	resources, resourcesOK := cache.resourcesSnapshot("shared")
	prompts, promptsOK := cache.promptsSnapshot("shared")
	if !toolsOK || len(tools.Tools) != 1 || tools.Tools[0].Name != "tool-2" ||
		!resourcesOK || len(resources.Resources) != 1 || resources.Resources[0].URI != "memo://shared/2" ||
		!promptsOK || len(prompts.Prompts) != 1 || prompts.Prompts[0].Name != "prompt-2" {
		t.Fatalf("cache catalog was not workspace B: tools=%#v/%v resources=%#v/%v prompts=%#v/%v", tools, toolsOK, resources, resourcesOK, prompts, promptsOK)
	}
}
