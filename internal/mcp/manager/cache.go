package manager

import (
	"sync"

	"github.com/agent-dance/luban/internal/mcp/catalog"
)

// cache owns fetched MCP catalogues. Live clients remain authoritative in the
// manager's connection state and are never mirrored here.
type cache struct {
	mu    sync.RWMutex
	owner *Manager

	tools     map[string]catalog.ListToolsResult
	resources map[string]catalog.ListResourcesResult
	prompts   map[string]catalog.ListPromptsResult
}

func (c *cache) setOwner(owner *Manager) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.owner = owner
	c.mu.Unlock()
}

func (c *cache) manager() *Manager {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.owner
}

// newCache constructs an empty MCP cache bundle.
func newCache() *cache {
	return &cache{
		tools:     make(map[string]catalog.ListToolsResult),
		resources: make(map[string]catalog.ListResourcesResult),
		prompts:   make(map[string]catalog.ListPromptsResult),
	}
}

func (c *cache) setTools(name string, result *catalog.ListToolsResult) {
	if c == nil || name == "" || result == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tools == nil {
		c.tools = make(map[string]catalog.ListToolsResult)
	}
	c.tools[name] = catalog.CloneListToolsResult(*result)
}

// toolsSnapshot returns a copy of the cached tools/list result for name.
func (c *cache) toolsSnapshot(name string) (catalog.ListToolsResult, bool) {
	if c == nil || name == "" {
		return catalog.ListToolsResult{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result, ok := c.tools[name]
	if !ok {
		return catalog.ListToolsResult{}, false
	}
	return catalog.CloneListToolsResult(result), true
}

func (c *cache) setResources(name string, result *catalog.ListResourcesResult) {
	if c == nil || name == "" || result == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resources == nil {
		c.resources = make(map[string]catalog.ListResourcesResult)
	}
	c.resources[name] = catalog.CloneListResourcesResult(*result)
}

// resourcesSnapshot returns a copy of the cached resources/list result for name.
func (c *cache) resourcesSnapshot(name string) (catalog.ListResourcesResult, bool) {
	if c == nil || name == "" {
		return catalog.ListResourcesResult{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result, ok := c.resources[name]
	if !ok {
		return catalog.ListResourcesResult{}, false
	}
	return catalog.CloneListResourcesResult(result), true
}

func (c *cache) setPrompts(name string, result *catalog.ListPromptsResult) {
	if c == nil || name == "" || result == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.prompts == nil {
		c.prompts = make(map[string]catalog.ListPromptsResult)
	}
	c.prompts[name] = catalog.CloneListPromptsResult(*result)
}

// promptsSnapshot returns a copy of the cached prompts/list result for name.
func (c *cache) promptsSnapshot(name string) (catalog.ListPromptsResult, bool) {
	if c == nil || name == "" {
		return catalog.ListPromptsResult{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result, ok := c.prompts[name]
	if !ok {
		return catalog.ListPromptsResult{}, false
	}
	return catalog.CloneListPromptsResult(result), true
}

// clearServer removes fetched catalogues for name.
func (c *cache) clearServer(name string) {
	if c == nil || name == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tools, name)
	delete(c.resources, name)
	delete(c.prompts, name)
}
