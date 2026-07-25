package search

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/registry"
)

// toolEntry is a flattened view of a registered tool used by the BM25 ranker
// and by callers that want a quick {name, description, hint} bundle without
// re-querying the registry on every call.
type toolEntry struct {
	Name        string
	Description string
	SearchHint  string
	IsMCP       bool
	IsDeferred  bool
}

// toolIndex caches a snapshot of the registered tools alongside a registry
// hash so callers (cache, ranker) can detect when a rebuild is required. The
// index is safe for concurrent reads under sync.RWMutex.
//
// Descriptions are memoised in `entries`. Tool.Description
// is invoked exactly once per Rebuild; subsequent ranker passes consult the
// cached entry, so dynamic prompt() implementations on plugin tools cannot
// regress BM25 scoring CPU. The cache is keyed by the index hash so a
// registry mutation forces a fresh capture.
type toolIndex struct {
	mu       sync.RWMutex
	registry *registry.Registry
	entries  []toolEntry
	hash     string
}

// newToolIndex builds an index from the registry. Pass nil to start with an
// empty index that can be populated later via Rebuild.
func newToolIndex(reg *registry.Registry) *toolIndex {
	idx := &toolIndex{registry: reg}
	idx.rebuild()
	return idx
}

// Rebuild scans the live registry and replaces the index in place. Returns
// true when the snapshot changed (caller can use this to invalidate caches).
func (i *toolIndex) rebuild() bool {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.registry == nil {
		i.entries = nil
		i.hash = ""
		return false
	}

	all := i.registry.All()
	deferred := i.registry.DeferredTools()
	deferredSet := make(map[string]struct{}, len(deferred))
	for _, t := range deferred {
		deferredSet[t.Name()] = struct{}{}
	}

	entries := make([]toolEntry, 0, len(all))
	for _, t := range all {
		entry := toolEntry{
			Name:        t.Name(),
			Description: t.Description(),
			SearchHint:  registry.DiscoveryMetadata(t).SearchHint,
			IsMCP:       strings.HasPrefix(t.Name(), "mcp__"),
		}
		if _, ok := deferredSet[t.Name()]; ok {
			entry.IsDeferred = true
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(a, b int) bool { return entries[a].Name < entries[b].Name })
	hash := computeIndexHash(entries)

	changed := hash != i.hash
	i.entries = entries
	i.hash = hash
	return changed
}

// Entries returns a copy of the indexed tools. Safe for concurrent reads.
func (i *toolIndex) entriesSnapshot() []toolEntry {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]toolEntry, len(i.entries))
	copy(out, i.entries)
	return out
}

// Hash returns the SHA-256 fingerprint of the current snapshot. Used as a
// cache invalidation key by the result cache.
func (i *toolIndex) hashSnapshot() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.hash
}

// computeIndexHash builds a deterministic fingerprint over the index's
// {name, description, hint} triples. Order-independent because callers sort
// before hashing.
func computeIndexHash(entries []toolEntry) string {
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
