package search

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// toolSearchCacheTTL bounds each query/registry cache entry to five minutes.
const toolSearchCacheTTL = 5 * time.Minute

// toolSearchCacheCapacity caps the LRU. 256 entries keeps memory under ~1MB.
const toolSearchCacheCapacity = 256

// toolSearchCacheKey scopes a cache lookup to (query, registryHash, limit).
type toolSearchCacheKey struct {
	query    string
	registry string
	limit    int
}

func (k toolSearchCacheKey) digest() string {
	h := sha256.New()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(k.query))))
	h.Write([]byte{0x1f})
	h.Write([]byte(k.registry))
	h.Write([]byte{0x1f})
	// limit varies output ordering, so include it
	h.Write([]byte{byte(k.limit & 0xff), byte((k.limit >> 8) & 0xff)})
	return hex.EncodeToString(h.Sum(nil))
}

type toolSearchCacheEntry struct {
	key       string
	matches   []scoredMatch
	expiresAt time.Time
	registry  string
}

// toolSearchCache is an LRU+TTL cache keyed by (query, registry hash). Entries
// older than toolSearchCacheTTL are treated as misses; entries created against
// a now-stale registry hash are evicted on access.
type toolSearchCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	order    *list.List
	now      func() time.Time
}

// newToolSearchCache builds a cache. Pass capacity<=0 to use the default.
func newToolSearchCache(capacity int, ttl time.Duration) *toolSearchCache {
	if capacity <= 0 {
		capacity = toolSearchCacheCapacity
	}
	if ttl <= 0 {
		ttl = toolSearchCacheTTL
	}
	return &toolSearchCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
		now:      time.Now,
	}
}

// Get returns cached matches when present, fresh, and bound to the same registry
// hash; otherwise (nil, false).
func (c *toolSearchCache) get(key toolSearchCacheKey) ([]scoredMatch, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	idx := key.digest()
	elem, ok := c.items[idx]
	if !ok {
		return nil, false
	}
	entry := elem.Value.(*toolSearchCacheEntry)
	if c.now().After(entry.expiresAt) {
		c.removeLocked(elem)
		return nil, false
	}
	if entry.registry != key.registry {
		c.removeLocked(elem)
		return nil, false
	}

	c.order.MoveToFront(elem)
	out := make([]scoredMatch, len(entry.matches))
	copy(out, entry.matches)
	return out, true
}

// Set stores matches under the key, evicting LRU when the capacity is hit.
func (c *toolSearchCache) set(key toolSearchCacheKey, matches []scoredMatch) {
	c.mu.Lock()
	defer c.mu.Unlock()

	idx := key.digest()
	stored := make([]scoredMatch, len(matches))
	copy(stored, matches)
	entry := &toolSearchCacheEntry{
		key:       idx,
		matches:   stored,
		expiresAt: c.now().Add(c.ttl),
		registry:  key.registry,
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
func (c *toolSearchCache) invalidate(registryHash string) {
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

func (c *toolSearchCache) removeLocked(elem *list.Element) {
	entry := elem.Value.(*toolSearchCacheEntry)
	c.order.Remove(elem)
	delete(c.items, entry.key)
}

func (c *toolSearchCache) clearLocked() {
	c.items = make(map[string]*list.Element, c.capacity)
	c.order = list.New()
}
