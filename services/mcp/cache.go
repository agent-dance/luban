package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
)

// HashMCPConfig returns the stable connection hash used to decide whether an
// existing MCP connection and its fetched catalogues are still valid. Scope is
// intentionally excluded by the MCPServerConfig JSON tags, matching the
// TypeScript hashMcpConfig behavior where provenance changes do not reconnect.
func HashMCPConfig(config MCPServerConfig) string {
	data, err := json.Marshal(config)
	if err != nil {
		data = []byte(fmt.Sprintf("%#v", config))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}

// ServerCacheKey mirrors the TypeScript getServerCacheKey shape: server name
// plus stable config hash. It is public so future dynamic-tool/resource
// migration tasks can share the same invalidation semantics.
func ServerCacheKey(name string, config MCPServerConfig) string {
	return name + "-" + HashMCPConfig(config)
}

// Cache owns live connection entries separately from fetched MCP catalogues.
// The manager is the only production writer; tests and follow-up task wiring
// read snapshots through the copy-returning accessors.
type Cache struct {
	mu    sync.RWMutex
	owner *Manager

	connections map[string]*Client
	tools       map[string]ListToolsResult
	resources   map[string]ListResourcesResult
	prompts     map[string]ListPromptsResult
}

func (c *Cache) setOwner(owner *Manager) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.owner = owner
	c.mu.Unlock()
}

func (c *Cache) manager() *Manager {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.owner
}

// NewCache constructs an empty MCP cache bundle.
func NewCache() *Cache {
	return &Cache{
		connections: make(map[string]*Client),
		tools:       make(map[string]ListToolsResult),
		resources:   make(map[string]ListResourcesResult),
		prompts:     make(map[string]ListPromptsResult),
	}
}

func (c *Cache) setConnection(key string, client *Client) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	if c.connections == nil {
		c.connections = make(map[string]*Client)
	}
	c.connections[key] = client
	c.mu.Unlock()
	installListChangedNotificationHandlers(c, key, client)
}

func (c *Cache) connection(key string) (*Client, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	client, ok := c.connections[key]
	return client, ok
}

func (c *Cache) deleteConnection(key string) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	delete(c.connections, key)
	c.mu.Unlock()
}

func (c *Cache) setTools(name string, result *ListToolsResult) {
	if c == nil || name == "" || result == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tools == nil {
		c.tools = make(map[string]ListToolsResult)
	}
	c.tools[name] = cloneListToolsResult(*result)
}

func (c *Cache) clearTools(name string) {
	if c == nil || name == "" {
		return
	}
	c.mu.Lock()
	delete(c.tools, name)
	c.mu.Unlock()
}

// Tools returns a copy of the cached tools/list result for name.
func (c *Cache) Tools(name string) (ListToolsResult, bool) {
	if c == nil || name == "" {
		return ListToolsResult{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result, ok := c.tools[name]
	if !ok {
		return ListToolsResult{}, false
	}
	return cloneListToolsResult(result), true
}

func (c *Cache) setResources(name string, result *ListResourcesResult) {
	if c == nil || name == "" || result == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resources == nil {
		c.resources = make(map[string]ListResourcesResult)
	}
	c.resources[name] = cloneListResourcesResult(*result)
}

// StoreResources replaces one server's cached resources/list result. The
// input is cloned so tool callers cannot mutate manager-owned cache state.
func (c *Cache) StoreResources(name string, result *ListResourcesResult) {
	c.setResources(name, result)
}

func (c *Cache) clearResources(name string) {
	if c == nil || name == "" {
		return
	}
	c.mu.Lock()
	delete(c.resources, name)
	c.mu.Unlock()
}

// InvalidateResources removes one server's resource catalogue without
// disturbing its healthy connection or unrelated tool/prompt caches.
func (c *Cache) InvalidateResources(name string) {
	c.clearResources(name)
}

// Resources returns a copy of the cached resources/list result for name.
func (c *Cache) Resources(name string) (ListResourcesResult, bool) {
	if c == nil || name == "" {
		return ListResourcesResult{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result, ok := c.resources[name]
	if !ok {
		return ListResourcesResult{}, false
	}
	return cloneListResourcesResult(result), true
}

func (c *Cache) setPrompts(name string, result *ListPromptsResult) {
	if c == nil || name == "" || result == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.prompts == nil {
		c.prompts = make(map[string]ListPromptsResult)
	}
	c.prompts[name] = cloneListPromptsResult(*result)
}

// Prompts returns a copy of the cached prompts/list result for name.
func (c *Cache) Prompts(name string) (ListPromptsResult, bool) {
	if c == nil || name == "" {
		return ListPromptsResult{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result, ok := c.prompts[name]
	if !ok {
		return ListPromptsResult{}, false
	}
	return cloneListPromptsResult(result), true
}

// ClearServer removes fetched catalogues for name and any connection cache
// entries with the same name prefix.
func (c *Cache) ClearServer(name string) {
	if c == nil || name == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.connections {
		if key == name || len(key) > len(name) && key[:len(name)+1] == name+"-" {
			delete(c.connections, key)
		}
	}
	delete(c.tools, name)
	delete(c.resources, name)
	delete(c.prompts, name)
}

func cloneListToolsResult(in ListToolsResult) ListToolsResult {
	out := ListToolsResult{
		Tools:      make([]ToolDefinition, len(in.Tools)),
		NextCursor: in.NextCursor,
		Meta:       cloneMap(in.Meta),
	}
	for i, tool := range in.Tools {
		out.Tools[i] = cloneToolDefinition(tool)
	}
	return out
}

func cloneToolDefinition(in ToolDefinition) ToolDefinition {
	return ToolDefinition{
		Name:        in.Name,
		Description: in.Description,
		InputSchema: cloneRawMessage(in.InputSchema),
		Annotations: cloneMap(in.Annotations),
		Meta:        cloneMap(in.Meta),
	}
}

func cloneListResourcesResult(in ListResourcesResult) ListResourcesResult {
	out := ListResourcesResult{
		Resources:  make([]Resource, len(in.Resources)),
		NextCursor: in.NextCursor,
		Meta:       cloneMap(in.Meta),
	}
	for i, resource := range in.Resources {
		out.Resources[i] = cloneResource(resource)
	}
	return out
}

func cloneResource(in Resource) Resource {
	return Resource{
		URI:         in.URI,
		Name:        in.Name,
		Description: in.Description,
		MimeType:    in.MimeType,
		Annotations: cloneMap(in.Annotations),
		Meta:        cloneMap(in.Meta),
	}
}

func cloneListPromptsResult(in ListPromptsResult) ListPromptsResult {
	out := ListPromptsResult{
		Prompts:    make([]PromptDefinition, len(in.Prompts)),
		NextCursor: in.NextCursor,
		Meta:       cloneMap(in.Meta),
	}
	for i, prompt := range in.Prompts {
		out.Prompts[i] = clonePromptDefinition(prompt)
	}
	return out
}

func clonePromptDefinition(in PromptDefinition) PromptDefinition {
	return PromptDefinition{
		Name:        in.Name,
		Description: in.Description,
		Arguments:   append([]PromptArgument(nil), in.Arguments...),
		Meta:        cloneMap(in.Meta),
	}
}
