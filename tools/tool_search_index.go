package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// ToolEntry is a flattened view of a registered tool used by the BM25 ranker
// and by callers that want a quick {name, description, hint} bundle without
// re-querying the registry on every call.
type ToolEntry struct {
	Name        string
	Description string
	SearchHint  string
	IsMCP       bool
	IsDeferred  bool
}

// ToolIndex caches a snapshot of the registered tools alongside a registry
// hash so callers (cache, ranker) can detect when a rebuild is required. The
// index is safe for concurrent reads under sync.RWMutex.
//
// TS-01: descriptions are memoised in `entries` and `byName`. Tool.Description
// is invoked exactly once per Rebuild; subsequent ranker passes consult the
// cached entry, so dynamic prompt() implementations on plugin tools cannot
// regress BM25 scoring CPU. The cache is keyed by the index hash so a
// registry mutation forces a fresh capture.
type ToolIndex struct {
	mu              sync.RWMutex
	registry        *registry.Registry
	entries         []ToolEntry
	byName          map[string]ToolEntry
	hash            string
	descriptionHash string
}

// NewToolIndex builds an index from the registry. Pass nil to start with an
// empty index that can be populated later via Rebuild.
func NewToolIndex(reg *registry.Registry) *ToolIndex {
	idx := &ToolIndex{registry: reg}
	idx.Rebuild()
	return idx
}

// Rebuild scans the live registry and replaces the index in place. Returns
// true when the snapshot changed (caller can use this to invalidate caches).
func (i *ToolIndex) Rebuild() bool {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.registry == nil {
		i.entries = nil
		i.byName = map[string]ToolEntry{}
		i.hash = ""
		return false
	}

	all := i.registry.All()
	deferred := i.registry.DeferredTools()
	deferredSet := make(map[string]struct{}, len(deferred))
	for _, t := range deferred {
		deferredSet[t.Name()] = struct{}{}
	}

	entries := make([]ToolEntry, 0, len(all))
	byName := make(map[string]ToolEntry, len(all))
	for _, t := range all {
		entry := ToolEntry{
			Name:        t.Name(),
			Description: t.Description(),
			SearchHint:  registry.DiscoveryMetadata(t).SearchHint,
			IsMCP:       strings.HasPrefix(t.Name(), "mcp__"),
		}
		if _, ok := deferredSet[t.Name()]; ok {
			entry.IsDeferred = true
		}
		entries = append(entries, entry)
		byName[strings.ToLower(entry.Name)] = entry
	}

	sort.Slice(entries, func(a, b int) bool { return entries[a].Name < entries[b].Name })
	hash := computeIndexHash(entries)

	changed := hash != i.hash
	i.entries = entries
	i.byName = byName
	i.hash = hash
	return changed
}

// Entries returns a copy of the indexed tools. Safe for concurrent reads.
func (i *ToolIndex) Entries() []ToolEntry {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]ToolEntry, len(i.entries))
	copy(out, i.entries)
	return out
}

// Lookup returns the indexed entry for the given tool name, case-insensitive.
func (i *ToolIndex) Lookup(name string) (ToolEntry, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	entry, ok := i.byName[strings.ToLower(strings.TrimSpace(name))]
	return entry, ok
}

// Hash returns the SHA-256 fingerprint of the current snapshot. Used as a
// cache invalidation key by the result cache.
func (i *ToolIndex) Hash() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.hash
}

// Len returns the number of indexed tools.
func (i *ToolIndex) Len() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.entries)
}

// computeIndexHash builds a deterministic fingerprint over the index's
// {name, description, hint} triples. Order-independent because callers sort
// before hashing.
func computeIndexHash(entries []ToolEntry) string {
	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e.Name))
		h.Write([]byte{0})
		h.Write([]byte(e.Description))
		h.Write([]byte{0})
		h.Write([]byte(e.SearchHint))
		h.Write([]byte{0x1e}) // record separator
	}
	return hex.EncodeToString(h.Sum(nil))
}

// IndexFromTools builds an index from an arbitrary list of tools. Used by
// tests that don't want to spin up a full registry.
func IndexFromTools(tools []types.Tool, deferred []types.Tool) *ToolIndex {
	deferredSet := make(map[string]struct{}, len(deferred))
	for _, t := range deferred {
		deferredSet[t.Name()] = struct{}{}
	}
	entries := make([]ToolEntry, 0, len(tools))
	byName := make(map[string]ToolEntry, len(tools))
	for _, t := range tools {
		entry := ToolEntry{
			Name:        t.Name(),
			Description: t.Description(),
			SearchHint:  registry.DiscoveryMetadata(t).SearchHint,
			IsMCP:       strings.HasPrefix(t.Name(), "mcp__"),
		}
		if _, ok := deferredSet[t.Name()]; ok {
			entry.IsDeferred = true
		}
		entries = append(entries, entry)
		byName[strings.ToLower(entry.Name)] = entry
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].Name < entries[b].Name })
	return &ToolIndex{
		entries: entries,
		byName:  byName,
		hash:    computeIndexHash(entries),
	}
}
