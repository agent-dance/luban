package tools

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/registry"
	svcmcp "github.com/agent-dance/luban/services/mcp"
)

func TestTask23MCPListChangedInvalidatorsAreManagerScoped(t *testing.T) {
	transportA := newNotificationTestTransport()
	transportB := newNotificationTestTransport()
	managerA := newNotificationTestManager(t, transportA)
	managerB := newNotificationTestManager(t, transportB)
	registryA := registry.New()
	registryB := registry.New()
	RegisterDynamicMCPTools(registryA, managerA, nil)
	RegisterDynamicMCPTools(registryB, managerB, nil)

	unregisterA := RegisterMCPListChangedInvalidators(registryA, managerA, nil)
	unregisterB := RegisterMCPListChangedInvalidators(registryB, managerB, nil)
	t.Cleanup(unregisterA)
	t.Cleanup(unregisterB)

	transportA.setTools([]svcmcp.ToolDefinition{{
		Name:        "only_a",
		Description: "manager A tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	transportA.emit(t, svcmcp.NotificationToolsListChanged)

	nameA := svcmcp.BuildMCPToolName("srv", "only_a")
	oldName := svcmcp.BuildMCPToolName("srv", "old_tool")
	eventually(t, func() bool {
		return registryA.Get(nameA) != nil && registryA.Get(oldName) == nil
	})
	if registryB.Get(nameA) != nil || registryB.Get(oldName) == nil {
		t.Fatalf("manager A event mutated manager B registry")
	}

	unregisterB()
	unregisterB() // teardown is explicitly idempotent
	transportB.setTools([]svcmcp.ToolDefinition{{
		Name:        "after_b_teardown",
		Description: "must remain manager-cache only",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	transportB.emit(t, svcmcp.NotificationToolsListChanged)
	eventually(t, func() bool {
		cached, ok := managerB.Cache().Tools("srv")
		return ok && len(cached.Tools) == 1 && cached.Tools[0].Name == "after_b_teardown"
	})
	if got := registryB.Get(svcmcp.BuildMCPToolName("srv", "after_b_teardown")); got != nil {
		t.Fatalf("teardown B admitted a later registry mutation: %T", got)
	}
	if registryB.Get(oldName) == nil {
		t.Fatal("teardown B removed the last authoritative registry projection")
	}
}

func TestTask23MCPListChangedUnregisterWaitsForInflightAndRejectsLateMutation(t *testing.T) {
	transport := newNotificationTestTransport()
	manager := newNotificationTestManager(t, transport)
	reg := registry.New()
	RegisterDynamicMCPTools(reg, manager, nil)

	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		SetToolSearchInvalidator(nil)
	})
	SetToolSearchInvalidator(func(serverName string) {
		if serverName != "srv" {
			return
		}
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
	})

	unregister := RegisterMCPListChangedInvalidators(reg, manager, nil)
	transport.setTools([]svcmcp.ToolDefinition{{
		Name:        "inflight",
		Description: "blocks inside the registered callback",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	transport.emit(t, svcmcp.NotificationToolsListChanged)
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("list_changed callback did not enter")
	}

	unregistered := make(chan struct{})
	go func() {
		unregister()
		close(unregistered)
	}()
	select {
	case <-unregistered:
		t.Fatal("unregister returned while a registry callback was still in flight")
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case <-unregistered:
	case <-time.After(2 * time.Second):
		t.Fatal("unregister did not return after the in-flight callback completed")
	}
	unregister() // a second call must neither block nor mutate state

	inflightName := svcmcp.BuildMCPToolName("srv", "inflight")
	if reg.Get(inflightName) == nil {
		t.Fatal("in-flight callback did not finish its pre-teardown mutation")
	}
	transport.setTools([]svcmcp.ToolDefinition{{
		Name:        "too_late",
		Description: "must never reach the registry",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	transport.emit(t, svcmcp.NotificationToolsListChanged)
	eventually(t, func() bool {
		cached, ok := manager.Cache().Tools("srv")
		return ok && len(cached.Tools) == 1 && cached.Tools[0].Name == "too_late"
	})
	if reg.Get(svcmcp.BuildMCPToolName("srv", "too_late")) != nil || reg.Get(inflightName) == nil {
		t.Fatal("a post-unregister event changed the registry projection")
	}

	// Give any hook snapshot copied by the dispatcher a chance to run. The
	// closed gate must make such a callback inert after unregister returned.
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	<-ctx.Done()
	if reg.Get(svcmcp.BuildMCPToolName("srv", "too_late")) != nil || reg.Get(inflightName) == nil {
		t.Fatal("a copied callback performed a late registry mutation")
	}
}
