package manager

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	"github.com/agent-dance/luban/internal/mcp/protocol"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
)

const (
	notificationToolsListChanged     = "notifications/tools/list_changed"
	notificationResourcesListChanged = "notifications/resources/list_changed"
	notificationPromptsListChanged   = "notifications/prompts/list_changed"

	listChangedRefreshTimeout = 30 * time.Second
	listChangedRefreshLimit   = 4
)

type ListChangedKind string

const (
	ListChangedTools     ListChangedKind = "tools"
	ListChangedResources ListChangedKind = "resources"
	ListChangedPrompts   ListChangedKind = "prompts"
)

type ListChangedEvent struct {
	Manager        *Manager
	ServerName     string
	Kind           ListChangedKind
	PreviousCount  int
	NewCount       int
	Tools          []catalog.ToolDefinition
	Resources      []catalog.Resource
	Prompts        []catalog.PromptDefinition
	Err            error
	client         *mcptransport.Client
	toolsResult    *catalog.ListToolsResult
	resourceResult *catalog.ListResourcesResult
	promptResult   *catalog.ListPromptsResult
	authoritative  bool
}

type ListChangedHook func(ListChangedEvent)

var listChangedHooks struct {
	sync.Mutex
	nextID int
	hooks  map[int]ListChangedHook
}

var listChangedRefreshes = struct {
	sync.Mutex
	active  map[string]bool
	pending map[string]*mcptransport.Client
	limit   chan struct{}
}{
	active:  map[string]bool{},
	pending: map[string]*mcptransport.Client{},
	limit:   make(chan struct{}, listChangedRefreshLimit),
}

func RegisterListChangedHook(hook ListChangedHook) func() {
	if hook == nil {
		return func() {}
	}
	listChangedHooks.Lock()
	defer listChangedHooks.Unlock()
	if listChangedHooks.hooks == nil {
		listChangedHooks.hooks = map[int]ListChangedHook{}
	}
	listChangedHooks.nextID++
	id := listChangedHooks.nextID
	listChangedHooks.hooks[id] = hook
	var once sync.Once
	return func() {
		once.Do(func() {
			listChangedHooks.Lock()
			delete(listChangedHooks.hooks, id)
			listChangedHooks.Unlock()
		})
	}
}

func installListChangedNotificationHandlers(cache *cache, serverName string, client *mcptransport.Client) {
	serverName = strings.TrimSpace(serverName)
	if cache == nil || serverName == "" || client == nil {
		return
	}
	caps := client.GetServerCapabilities()
	if capabilityAdvertisesListChanged(caps, "tools") {
		client.SetNotificationHandler(notificationToolsListChanged, func(context.Context, protocol.JSONRPCMessage) {
			cache.scheduleListChangedRefresh(serverName, ListChangedTools, client)
		})
	}
	if capabilityAdvertisesListChanged(caps, "resources") {
		client.SetNotificationHandler(notificationResourcesListChanged, func(context.Context, protocol.JSONRPCMessage) {
			cache.scheduleListChangedRefresh(serverName, ListChangedResources, client)
		})
	}
	if capabilityAdvertisesListChanged(caps, "prompts") {
		client.SetNotificationHandler(notificationPromptsListChanged, func(context.Context, protocol.JSONRPCMessage) {
			cache.scheduleListChangedRefresh(serverName, ListChangedPrompts, client)
		})
	}
}

func (c *cache) scheduleListChangedRefresh(serverName string, kind ListChangedKind, client *mcptransport.Client) {
	if c == nil || strings.TrimSpace(serverName) == "" || client == nil {
		return
	}
	key := listChangedRefreshKey(c, serverName, kind)
	listChangedRefreshes.Lock()
	if listChangedRefreshes.active[key] {
		listChangedRefreshes.pending[key] = client
		listChangedRefreshes.Unlock()
		return
	}
	listChangedRefreshes.active[key] = true
	listChangedRefreshes.Unlock()

	go func(currentClient *mcptransport.Client) {
		for {
			listChangedRefreshes.limit <- struct{}{}
			ctx, cancel := context.WithTimeout(context.Background(), listChangedRefreshTimeout)
			event := c.refreshListChanged(ctx, serverName, kind, currentClient)
			cancel()
			<-listChangedRefreshes.limit
			if event.authoritative {
				runListChangedHooks(event)
			}

			listChangedRefreshes.Lock()
			if pendingClient := listChangedRefreshes.pending[key]; pendingClient != nil {
				delete(listChangedRefreshes.pending, key)
				currentClient = pendingClient
				listChangedRefreshes.Unlock()
				continue
			}
			delete(listChangedRefreshes.active, key)
			delete(listChangedRefreshes.pending, key)
			listChangedRefreshes.Unlock()
			return
		}
	}(client)
}

func (c *cache) refreshListChanged(ctx context.Context, serverName string, kind ListChangedKind, client *mcptransport.Client) ListChangedEvent {
	event := ListChangedEvent{Manager: c.manager(), ServerName: serverName, Kind: kind, client: client}
	switch kind {
	case ListChangedTools:
		if previous, ok := c.toolsSnapshot(serverName); ok {
			event.PreviousCount = len(previous.Tools)
		}
		result, err := client.ListTools(ctx)
		if err != nil {
			event.Err = err
			break
		}
		event.toolsResult = result
		clone := catalog.CloneListToolsResult(*result)
		event.Tools = clone.Tools
		event.NewCount = len(clone.Tools)
	case ListChangedResources:
		if previous, ok := c.resourcesSnapshot(serverName); ok {
			event.PreviousCount = len(previous.Resources)
		}
		result, err := client.ListResourcesResult(ctx)
		if err != nil {
			event.Err = err
			break
		}
		event.resourceResult = result
		clone := catalog.CloneListResourcesResult(*result)
		event.Resources = clone.Resources
		event.NewCount = len(clone.Resources)
	case ListChangedPrompts:
		if previous, ok := c.promptsSnapshot(serverName); ok {
			event.PreviousCount = len(previous.Prompts)
		}
		result, err := client.ListPrompts(ctx)
		if err != nil {
			event.Err = err
			break
		}
		event.promptResult = result
		clone := catalog.CloneListPromptsResult(*result)
		event.Prompts = clone.Prompts
		event.NewCount = len(clone.Prompts)
	}
	if event.Manager != nil {
		if event.Err != nil {
			event.authoritative = event.Manager.isCurrentListChangedEvent(event)
		} else {
			event.authoritative = event.Manager.applyListChangedEvent(event)
		}
	} else if event.Err == nil {
		event.authoritative = c.publishListChangedEvent(event)
	} else {
		event.authoritative = true
	}
	return event
}

func (c *cache) publishListChangedEvent(event ListChangedEvent) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.publishListChangedEventLocked(event)
}

// publishListChangedEventLocked installs one staged catalogue result. Caller
// holds c.mu so a manager can publish cache and state as one authority change.
func (c *cache) publishListChangedEventLocked(event ListChangedEvent) bool {
	if c == nil || event.ServerName == "" {
		return false
	}
	switch event.Kind {
	case ListChangedTools:
		if event.toolsResult == nil {
			return false
		}
		if c.tools == nil {
			c.tools = make(map[string]catalog.ListToolsResult)
		}
		c.tools[event.ServerName] = catalog.CloneListToolsResult(*event.toolsResult)
	case ListChangedResources:
		if event.resourceResult == nil {
			return false
		}
		if c.resources == nil {
			c.resources = make(map[string]catalog.ListResourcesResult)
		}
		c.resources[event.ServerName] = catalog.CloneListResourcesResult(*event.resourceResult)
	case ListChangedPrompts:
		if event.promptResult == nil {
			return false
		}
		if c.prompts == nil {
			c.prompts = make(map[string]catalog.ListPromptsResult)
		}
		c.prompts[event.ServerName] = catalog.CloneListPromptsResult(*event.promptResult)
	default:
		return false
	}
	return true
}

func runListChangedHooks(event ListChangedEvent) {
	listChangedHooks.Lock()
	hooks := make([]ListChangedHook, 0, len(listChangedHooks.hooks))
	for _, hook := range listChangedHooks.hooks {
		hooks = append(hooks, hook)
	}
	listChangedHooks.Unlock()
	for _, hook := range hooks {
		func() {
			defer func() { _ = recover() }()
			hook(event)
		}()
	}
}

func capabilityAdvertisesListChanged(caps catalog.ServerCapabilities, name string) bool {
	if caps == nil {
		return false
	}
	value, ok := caps[name]
	if !ok {
		return false
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return false
	}
	return boolFromAny(fields["listChanged"])
}

func boolFromAny(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	default:
		return false
	}
}

func listChangedRefreshKey(cache *cache, serverName string, kind ListChangedKind) string {
	return strings.Join([]string{fmt.Sprintf("%p", cache), serverName, string(kind)}, "\x00")
}
