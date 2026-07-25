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

func TestManagerListResourcesPublishesCacheStateAndHook(t *testing.T) {
	var transport *managerTestTransport
	manager := NewManager(withTestTransportFactory(func(context.Context, string, catalog.MCPServerConfig, transportBuildOptions) (mcptransport.Transport, error) {
		transport = newManagerTestTransport("alpha", 1, nil)
		return transport, nil
	}))
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	manager.AddConfig("alpha", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake"})
	if _, err := manager.GetOrConnect(context.Background(), "alpha"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	manager.cache.clearServer("alpha")
	transport.resources = newManagerTestTransport("alpha", 2, nil).resources

	hook := make(chan struct{}, 1)
	unregister := manager.RegisterCatalogChangeHook(func() {
		select {
		case hook <- struct{}{}:
		default:
		}
	})
	t.Cleanup(unregister)
	result, err := manager.ListResources(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if len(result.Resources) != 1 || result.Resources[0].URI != "memo://alpha/2" {
		t.Fatalf("result = %#v", result)
	}
	state, ok := manager.State("alpha")
	cached, cachedOK := manager.cache.resourcesSnapshot("alpha")
	if !ok || len(state.Resources) != 1 || state.Resources[0].URI != "memo://alpha/2" ||
		!cachedOK || len(cached.Resources) != 1 || cached.Resources[0].URI != "memo://alpha/2" {
		t.Fatalf("resources diverged: state=%#v/%v cache=%#v/%v", state.Resources, ok, cached.Resources, cachedOK)
	}
	select {
	case <-hook:
	case <-time.After(3 * time.Second):
		t.Fatal("resource publication did not signal catalog change")
	}
}

type blockingResourcesTransport struct {
	*managerTestTransport
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (t *blockingResourcesTransport) Send(ctx context.Context, msg protocol.JSONRPCMessage) error {
	if msg.Method == "resources/list" {
		t.once.Do(func() {
			close(t.started)
			<-t.release
		})
	}
	return t.managerTestTransport.Send(ctx, msg)
}

func TestManagerListResourcesDiscardsReplacedClientResult(t *testing.T) {
	manager := NewManager()
	configA := normalizeManagerConfig("shared", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace-a", Scope: catalog.ScopeProject})
	configB := normalizeManagerConfig("shared", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace-b", Scope: catalog.ScopeProject})
	manager.AddConfig("shared", configA)

	transportA := &blockingResourcesTransport{
		managerTestTransport: newManagerTestTransport("shared", 1, nil),
		started:              make(chan struct{}),
		release:              make(chan struct{}),
	}
	clientA, err := mcptransport.NewClient(context.Background(), transportA)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientA.Close() })
	manager.mu.Lock()
	manager.setStateLocked(MCPServerConnection{
		Name:         "shared",
		Type:         MCPStateConnected,
		Config:       configA,
		ConfigHash:   catalog.HashMCPConfig(configA),
		Client:       clientA,
		Capabilities: clientA.GetServerCapabilities(),
	})
	manager.mu.Unlock()
	manager.cache.clearServer("shared")

	type listResult struct {
		result catalog.ListResourcesResult
		err    error
	}
	listed := make(chan listResult, 1)
	go func() {
		result, err := manager.ListResources(context.Background(), "shared")
		listed <- listResult{result: result, err: err}
	}()
	select {
	case <-transportA.started:
	case <-time.After(3 * time.Second):
		t.Fatal("workspace A resources/list did not start")
	}

	transportB := newManagerTestTransport("shared", 2, nil)
	clientB, err := mcptransport.NewClient(context.Background(), transportB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientB.Close() })
	resultB := catalog.ListResourcesResult{Resources: transportB.resources}
	manager.mu.Lock()
	manager.configs["shared"] = configB
	manager.cache.mu.Lock()
	manager.cache.resources["shared"] = catalog.CloneListResourcesResult(resultB)
	manager.setStateLocked(MCPServerConnection{
		Name:         "shared",
		Type:         MCPStateConnected,
		Config:       configB,
		ConfigHash:   catalog.HashMCPConfig(configB),
		Client:       clientB,
		Capabilities: clientB.GetServerCapabilities(),
		Resources:    catalog.CloneListResourcesResult(resultB).Resources,
	})
	manager.cache.mu.Unlock()
	manager.mu.Unlock()
	close(transportA.release)

	select {
	case got := <-listed:
		if got.err != nil {
			t.Fatalf("list resources: %v", got.err)
		}
		if len(got.result.Resources) != 1 || got.result.Resources[0].URI != "memo://shared/2" {
			t.Fatalf("stale workspace result leaked: %#v", got.result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("list resources did not retry current authority")
	}
	state, _ := manager.State("shared")
	cached, cachedOK := manager.cache.resourcesSnapshot("shared")
	if len(state.Resources) != 1 || state.Resources[0].URI != "memo://shared/2" ||
		!cachedOK || len(cached.Resources) != 1 || cached.Resources[0].URI != "memo://shared/2" {
		t.Fatalf("workspace A changed resources: state=%#v cache=%#v/%v", state.Resources, cached.Resources, cachedOK)
	}
}
