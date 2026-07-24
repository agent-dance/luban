package tools

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

// toolSearchCacheTTL mirrors the TS reference (5 minutes per query/registry pair).
const toolSearchCacheTTL = 5 * time.Minute

// toolSearchCacheCapacity caps the LRU. 256 entries keeps memory under ~1MB.
const toolSearchCacheCapacity = 256

// ToolSearchCacheKey scopes a cache lookup to (query, registryHash, limit,
// embedding-flag).
type ToolSearchCacheKey struct {
	Query    string
	Registry string
	Limit    int
	UseEmbed bool
}

func (k ToolSearchCacheKey) String() string {
	h := sha256.New()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(k.Query))))
	h.Write([]byte{0x1f})
	h.Write([]byte(k.Registry))
	h.Write([]byte{0x1f})
	if k.UseEmbed {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
	h.Write([]byte{0x1f})
	// limit varies output ordering, so include it
	h.Write([]byte{byte(k.Limit & 0xff), byte((k.Limit >> 8) & 0xff)})
	return hex.EncodeToString(h.Sum(nil))
}

type toolSearchCacheEntry struct {
	key       string
	matches   []ScoredMatch
	expiresAt time.Time
	registry  string
}

// ToolSearchCache is an LRU+TTL cache keyed by (query, registry hash). Entries
// older than toolSearchCacheTTL are treated as misses; entries created against
// a now-stale registry hash are evicted on access.
type ToolSearchCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	order    *list.List
	now      func() time.Time
}

// NewToolSearchCache builds a cache. Pass capacity<=0 to use the default.
func NewToolSearchCache(capacity int, ttl time.Duration) *ToolSearchCache {
	if capacity <= 0 {
		capacity = toolSearchCacheCapacity
	}
	if ttl <= 0 {
		ttl = toolSearchCacheTTL
	}
	return &ToolSearchCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
		now:      time.Now,
	}
}

// Get returns cached matches when present, fresh, and bound to the same registry
// hash; otherwise (nil, false).
func (c *ToolSearchCache) Get(key ToolSearchCacheKey) ([]ScoredMatch, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	idx := key.String()
	elem, ok := c.items[idx]
	if !ok {
		return nil, false
	}
	entry := elem.Value.(*toolSearchCacheEntry)
	if c.now().After(entry.expiresAt) {
		c.removeLocked(elem)
		return nil, false
	}
	if entry.registry != key.Registry {
		c.removeLocked(elem)
		return nil, false
	}

	c.order.MoveToFront(elem)
	out := make([]ScoredMatch, len(entry.matches))
	copy(out, entry.matches)
	return out, true
}

// Set stores matches under the key, evicting LRU when the capacity is hit.
func (c *ToolSearchCache) Set(key ToolSearchCacheKey, matches []ScoredMatch) {
	c.mu.Lock()
	defer c.mu.Unlock()

	idx := key.String()
	stored := make([]ScoredMatch, len(matches))
	copy(stored, matches)
	entry := &toolSearchCacheEntry{
		key:       idx,
		matches:   stored,
		expiresAt: c.now().Add(c.ttl),
		registry:  key.Registry,
	}
	if elem, exists := c.items[idx]; exists {
		elem.Value = entry
		c.order.MoveToFront(elem)
		return
	}
	elem := c.order.PushFront(entry)
	c.items[idx] = elem

	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest == nil {
			break
		}
		c.removeLocked(oldest)
	}
}

// Invalidate purges every entry whose registry hash != current. Cheap when
// registry is unchanged.
func (c *ToolSearchCache) Invalidate(registryHash string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if registryHash == "" {
		c.clearLocked()
		return
	}

	stale := make([]*list.Element, 0)
	for elem := c.order.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*toolSearchCacheEntry)
		if entry.registry != registryHash {
			stale = append(stale, elem)
		}
	}
	for _, e := range stale {
		c.removeLocked(e)
	}
}

// Len returns the number of resident entries (including expired ones until
// they are accessed). Used by tests.
func (c *ToolSearchCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Keys returns a snapshot of resident cache keys for tests/debugging.
func (c *ToolSearchCache) Keys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.items))
	for k := range c.items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (c *ToolSearchCache) removeLocked(elem *list.Element) {
	entry := elem.Value.(*toolSearchCacheEntry)
	c.order.Remove(elem)
	delete(c.items, entry.key)
}

func (c *ToolSearchCache) clearLocked() {
	c.items = make(map[string]*list.Element, c.capacity)
	c.order = list.New()
}
