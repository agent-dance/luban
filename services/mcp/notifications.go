package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	NotificationToolsListChanged     = "notifications/tools/list_changed"
	NotificationResourcesListChanged = "notifications/resources/list_changed"
	NotificationPromptsListChanged   = "notifications/prompts/list_changed"

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
	Manager       *Manager
	ServerName    string
	Kind          ListChangedKind
	PreviousCount int
	NewCount      int
	Tools         []ToolDefinition
	Resources     []Resource
	Prompts       []PromptDefinition
	Err           error
	client        *Client
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
	pending map[string]bool
	limit   chan struct{}
}{
	active:  map[string]bool{},
	pending: map[string]bool{},
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

func installListChangedNotificationHandlers(cache *Cache, key string, client *Client) {
	if cache == nil || client == nil {
		return
	}
	serverName := serverNameFromCacheKey(key)
	if serverName == "" {
		return
	}
	caps := client.GetServerCapabilities()
	if capabilityAdvertisesListChanged(caps, "tools") {
		client.SetNotificationHandler(NotificationToolsListChanged, func(context.Context, JSONRPCMessage) {
			cache.scheduleListChangedRefresh(serverName, ListChangedTools, client)
		})
	}
	if capabilityAdvertisesListChanged(caps, "resources") {
		client.SetNotificationHandler(NotificationResourcesListChanged, func(context.Context, JSONRPCMessage) {
			// Invalidate synchronously before scheduling the refresh so a tool call
			// immediately after the notification cannot observe stale resources.
			cache.InvalidateResources(serverName)
			cache.scheduleListChangedRefresh(serverName, ListChangedResources, client)
		})
	}
	if capabilityAdvertisesListChanged(caps, "prompts") {
		client.SetNotificationHandler(NotificationPromptsListChanged, func(context.Context, JSONRPCMessage) {
			cache.scheduleListChangedRefresh(serverName, ListChangedPrompts, client)
		})
	}
}

func (c *Cache) scheduleListChangedRefresh(serverName string, kind ListChangedKind, client *Client) {
	if c == nil || strings.TrimSpace(serverName) == "" || client == nil {
		return
	}
	key := listChangedRefreshKey(c, serverName, kind)
	listChangedRefreshes.Lock()
	if listChangedRefreshes.active[key] {
		listChangedRefreshes.pending[key] = true
		listChangedRefreshes.Unlock()
		return
	}
	listChangedRefreshes.active[key] = true
	listChangedRefreshes.Unlock()

	go func() {
		for {
			listChangedRefreshes.limit <- struct{}{}
			ctx, cancel := context.WithTimeout(context.Background(), listChangedRefreshTimeout)
			event := c.refreshListChanged(ctx, serverName, kind, client)
			cancel()
			<-listChangedRefreshes.limit
			runListChangedHooks(event)

			listChangedRefreshes.Lock()
			if listChangedRefreshes.pending[key] {
				listChangedRefreshes.pending[key] = false
				listChangedRefreshes.Unlock()
				continue
			}
			delete(listChangedRefreshes.active, key)
			delete(listChangedRefreshes.pending, key)
			listChangedRefreshes.Unlock()
			return
		}
	}()
}

func (c *Cache) refreshListChanged(ctx context.Context, serverName string, kind ListChangedKind, client *Client) ListChangedEvent {
	event := ListChangedEvent{Manager: c.manager(), ServerName: serverName, Kind: kind, client: client}
	switch kind {
	case ListChangedTools:
		if previous, ok := c.Tools(serverName); ok {
			event.PreviousCount = len(previous.Tools)
		}
		c.clearTools(serverName)
		result, err := client.ListTools(ctx)
		if err != nil {
			event.Err = err
			return event
		}
		c.setTools(serverName, result)
		clone := cloneListToolsResult(*result)
		event.Tools = clone.Tools
		event.NewCount = len(clone.Tools)
	case ListChangedResources:
		if previous, ok := c.Resources(serverName); ok {
			event.PreviousCount = len(previous.Resources)
		}
		c.clearResources(serverName)
		result, err := client.ListResourcesResult(ctx)
		if err != nil {
			event.Err = err
			return event
		}
		c.setResources(serverName, result)
		clone := cloneListResourcesResult(*result)
		event.Resources = clone.Resources
		event.NewCount = len(clone.Resources)
	case ListChangedPrompts:
		if previous, ok := c.Prompts(serverName); ok {
			event.PreviousCount = len(previous.Prompts)
		}
		c.clearPrompts(serverName)
		result, err := client.ListPrompts(ctx)
		if err != nil {
			event.Err = err
			runPromptInvalidationHooks(serverName)
			return event
		}
		c.setPrompts(serverName, result)
		clone := cloneListPromptsResult(*result)
		event.Prompts = clone.Prompts
		event.NewCount = len(clone.Prompts)
		runPromptInvalidationHooks(serverName)
	}
	if event.Manager != nil {
		event.Manager.applyListChangedEvent(event)
	}
	return event
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

func capabilityAdvertisesListChanged(caps ServerCapabilities, name string) bool {
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

func serverNameFromCacheKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	idx := strings.LastIndex(key, "-")
	if idx <= 0 || len(key)-idx-1 != 16 || !isHexString(key[idx+1:]) {
		return key
	}
	return key[:idx]
}

func isHexString(value string) bool {
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return value != ""
}

func listChangedRefreshKey(cache *Cache, serverName string, kind ListChangedKind) string {
	return strings.Join([]string{fmt.Sprintf("%p", cache), serverName, string(kind)}, "\x00")
}
