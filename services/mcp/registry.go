// Package mcp services-layer registry module.
//
// Audit P2-6 calls for a registry module that owns the live set of MCP
// servers. Today the inline MCPManager in tools/mcp_tools.go fills this
// role; this module exposes a transport-agnostic surface that callers can
// share between the tool layer and any future services-layer entrypoint.
package mcp

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// ServerEntry is one row in the registry. The transport-specific fields are
// kept opaque so the same registry can hold stdio, SSE, and HTTP servers.
type ServerEntry struct {
	Name      string
	Transport string // "stdio" | "sse" | "http"
	BaseURL   string // populated for sse / http
	Command   string // populated for stdio
	Args      []string
	Env       map[string]string
	Client    *Client // services-layer client wrapping a RawCaller

	State        MCPConnectionState
	Config       MCPServerConfig
	ConfigHash   string
	Capabilities ServerCapabilities
	ServerInfo   *ServerInfo
	Instructions string
	Tools        []ToolDefinition
	Resources    []Resource
	Prompts      []PromptDefinition
	Error        string
}

// Registry is a thread-safe map of name → ServerEntry. Methods follow the
// same locking discipline as MCPManager so the two layers can coexist.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*ServerEntry
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]*ServerEntry)}
}

// Add inserts or replaces an entry by name.
func (r *Registry) Add(entry *ServerEntry) error {
	if entry == nil {
		return errors.New("services/mcp: nil registry entry")
	}
	if entry.Name == "" {
		return errors.New("services/mcp: registry entry missing name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := cloneServerEntry(entry)
	r.entries[entry.Name] = cp
	return nil
}

// Remove drops the named entry. Returns false if it wasn't present.
func (r *Registry) Remove(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[name]; !ok {
		return false
	}
	delete(r.entries, name)
	return true
}

// Get fetches a copy of the entry, or returns false.
func (r *Registry) Get(name string) (*ServerEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[name]
	if !ok {
		return nil, false
	}
	return cloneServerEntry(entry), true
}

// Names returns a sorted snapshot of registered server names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.entries))
	for n := range r.entries {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ListActive returns the snapshot of entries whose Client field is non-nil.
// Tools/UIs that want to show "currently connected" state consult this.
func (r *Registry) ListActive(ctx context.Context) []*ServerEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ServerEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		if entry.Client != nil {
			cp := *entry
			out = append(out, cloneServerEntry(&cp))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// UpsertConnection stores the registry view for one manager connection state.
func (r *Registry) UpsertConnection(state MCPServerConnection) {
	if r == nil || state.Name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := &ServerEntry{
		Name:         state.Name,
		Transport:    string(state.Config.Type),
		BaseURL:      state.Config.URL,
		Command:      state.Config.Command,
		Args:         append([]string(nil), state.Config.Args...),
		Env:          cloneStringMap(state.Config.Env),
		Client:       state.Client,
		State:        state.Type,
		Config:       cloneMCPServerConfig(state.Config),
		ConfigHash:   state.ConfigHash,
		Capabilities: cloneCapabilities(state.Capabilities),
		ServerInfo:   cloneServerInfo(state.ServerInfo),
		Instructions: state.Instructions,
		Tools:        cloneListToolsResult(ListToolsResult{Tools: state.Tools}).Tools,
		Resources:    cloneListResourcesResult(ListResourcesResult{Resources: state.Resources}).Resources,
		Prompts:      cloneListPromptsResult(ListPromptsResult{Prompts: state.Prompts}).Prompts,
		Error:        state.Error,
	}
	r.entries[state.Name] = entry
}

// Snapshot returns sorted connection-state copies from the registry.
func (r *Registry) Snapshot() []MCPServerConnection {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]MCPServerConnection, 0, len(r.entries))
	for _, entry := range r.entries {
		out = append(out, MCPServerConnection{
			Name:         entry.Name,
			Type:         entry.State,
			Config:       cloneMCPServerConfig(entry.Config),
			ConfigHash:   entry.ConfigHash,
			Client:       entry.Client,
			Capabilities: cloneCapabilities(entry.Capabilities),
			ServerInfo:   cloneServerInfo(entry.ServerInfo),
			Instructions: entry.Instructions,
			Tools:        cloneListToolsResult(ListToolsResult{Tools: entry.Tools}).Tools,
			Resources:    cloneListResourcesResult(ListResourcesResult{Resources: entry.Resources}).Resources,
			Prompts:      cloneListPromptsResult(ListPromptsResult{Prompts: entry.Prompts}).Prompts,
			Error:        entry.Error,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func cloneServerEntry(entry *ServerEntry) *ServerEntry {
	if entry == nil {
		return nil
	}
	cp := *entry
	cp.Args = append([]string(nil), entry.Args...)
	cp.Env = cloneStringMap(entry.Env)
	cp.Config = cloneMCPServerConfig(entry.Config)
	cp.Capabilities = cloneCapabilities(entry.Capabilities)
	cp.ServerInfo = cloneServerInfo(entry.ServerInfo)
	cp.Tools = cloneListToolsResult(ListToolsResult{Tools: entry.Tools}).Tools
	cp.Resources = cloneListResourcesResult(ListResourcesResult{Resources: entry.Resources}).Resources
	cp.Prompts = cloneListPromptsResult(ListPromptsResult{Prompts: entry.Prompts}).Prompts
	return &cp
}
