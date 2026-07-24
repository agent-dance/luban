// Package tools implements the raw-content cache used by WebFetch.
//
// Mirrors the behaviour of URL_CACHE in src/tools/WebFetchTool/utils.ts:
// entries are keyed by the original URL (not the prompt), contain fetched
// markdown plus HTTP/binary metadata, expire after 15 minutes, and share a
// 50MB byte budget. Prompt application deliberately happens after lookup so
// two prompts reuse one network response while invoking the model twice.
package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// WebFetchCacheTTL is the lifetime of a cached fetch result. Mirrors TS
// CACHE_TTL_MS = 15 minutes.
const WebFetchCacheTTL = 15 * time.Minute

// webFetchCachePurgeInterval is how often the background goroutine sweeps
// the map for expired entries. The TS implementation relies on lru-cache's
// lazy invalidation; we use both lazy expiry (Get) and a pump goroutine so
// long-running daemons don't accumulate dead entries between accesses.
const webFetchCachePurgeInterval = 60 * time.Second

// WebFetchCacheMaxBytes mirrors the lru-cache `maxSize` ceiling on the TS
// side. Once the sum of cached entry sizes exceeds this we begin evicting
// the oldest entries until the new entry fits.
const WebFetchCacheMaxBytes = 50 * 1024 * 1024

// WebFetchCacheEntry is the raw fetched-content value stored per original URL.
type WebFetchCacheEntry struct {
	Body          string
	ContentType   string
	ContentLength int
	CacheSize     int
	StatusCode    int
	StatusText    string
	URL           string
	Bytes         int
	Truncated     bool
	PersistedPath string
	PersistedSize int
	StoredAt      time.Time
}

// WebFetchCache is a self-cleaning sha256(originalURL)-keyed cache with a
// hard byte-cap ceiling. Entries are evicted in oldest-first order once the
// total cached bytes exceed WebFetchCacheMaxBytes.
type WebFetchCache struct {
	mu         sync.Mutex
	entries    map[string]webFetchCacheRecord
	ttl        time.Duration
	maxBytes   int
	totalBytes int
	sequence   uint64
	stopCh     chan struct{}
	stopped    bool
	now        func() time.Time // overridable for tests
}

type webFetchCacheRecord struct {
	value   WebFetchCacheEntry
	expiry  time.Time
	size    int
	recency uint64
}

// NewWebFetchCache creates a cache with the default 15-minute TTL and starts
// the background purge goroutine. Call Stop when no longer needed.
func NewWebFetchCache() *WebFetchCache {
	return NewWebFetchCacheWithTTL(WebFetchCacheTTL, webFetchCachePurgeInterval)
}

// NewWebFetchCacheWithTTL exposes TTL/purge tunables for tests.
func NewWebFetchCacheWithTTL(ttl, purgeEvery time.Duration) *WebFetchCache {
	c := &WebFetchCache{
		entries:  make(map[string]webFetchCacheRecord),
		ttl:      ttl,
		maxBytes: WebFetchCacheMaxBytes,
		stopCh:   make(chan struct{}),
		now:      time.Now,
	}
	if purgeEvery > 0 {
		go c.run(purgeEvery)
	}
	return c
}

func (c *WebFetchCache) run(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-t.C:
			c.purge()
		}
	}
}

// Stop terminates the background purge goroutine. It is idempotent.
func (c *WebFetchCache) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	c.stopped = true
	close(c.stopCh)
}

// Clear removes every cached response without stopping the cache. It is the
// state-lifecycle hook used by /clear and session-reset integrations.
func (c *WebFetchCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = make(map[string]webFetchCacheRecord)
	c.totalBytes = 0
	c.sequence = 0
	c.mu.Unlock()
}

// MakeKey returns the canonical sha256(originalURL) digest. The variadic
// parameter is intentionally ignored so older embedders compiled against the
// former (url,prompt) signature continue to build while receiving the new
// raw-URL semantics.
func (c *WebFetchCache) MakeKey(rawURL string, _ ...string) string {
	return WebFetchCacheKey(rawURL)
}

// WebFetchCacheKey computes a raw URL cache key. URL case is preserved: TS
// Map/LRU keys use the exact original URL string, so case-sensitive paths and
// escaped octets must not collapse into the same entry.
func WebFetchCacheKey(rawURL string, _ ...string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(sum[:])
}

// Get returns the cached entry for key. The second value is false when no
// entry is present or the existing entry has expired (in which case the
// stale entry is also evicted).
func (c *WebFetchCache) Get(key string) (WebFetchCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rec, ok := c.entries[key]
	if !ok {
		return WebFetchCacheEntry{}, false
	}
	if c.now().After(rec.expiry) {
		c.deleteEntryLocked(key, rec)
		return WebFetchCacheEntry{}, false
	}
	c.sequence++
	rec.recency = c.sequence
	c.entries[key] = rec
	return rec.value, true
}

// Set writes value at key. If the cache has been Stop()ped the call is a
// no-op so callers don't need to track lifecycle. Once the cache exceeds
// WebFetchCacheMaxBytes, the oldest entries are evicted until the new
// value fits (mirrors TS lru-cache `maxSize` accounting).
func (c *WebFetchCache) Set(key string, value WebFetchCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	value.StoredAt = c.now()

	size := entrySize(value)

	// Remove an old record before capacity eviction so it cannot be selected
	// by evictOldestLocked and subtracted twice.
	if old, ok := c.entries[key]; ok {
		c.deleteEntryLocked(key, old)
	}

	// lru-cache does not retain an item larger than its complete size budget.
	if c.maxBytes > 0 && size > c.maxBytes {
		return
	}

	// Evict oldest entries until the new entry fits under the byte cap.
	for c.maxBytes > 0 && c.totalBytes+size > c.maxBytes && len(c.entries) > 0 {
		c.evictOldestLocked()
	}

	c.sequence++
	c.entries[key] = webFetchCacheRecord{
		value:   value,
		expiry:  c.now().Add(c.ttl),
		size:    size,
		recency: c.sequence,
	}
	c.totalBytes += size
}

// Len returns the number of live entries (not counting expired ones still
// physically present until the next purge or Get).
func (c *WebFetchCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// TotalBytes returns the running byte total used by the byte-cap eviction
// policy. Exposed so tests and metrics can verify the accounting.
func (c *WebFetchCache) TotalBytes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totalBytes
}

func (c *WebFetchCache) purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for k, rec := range c.entries {
		if now.After(rec.expiry) {
			c.deleteEntryLocked(k, rec)
		}
	}
}

// deleteEntryLocked removes a cache record and updates the byte total. The
// caller must hold c.mu.
func (c *WebFetchCache) deleteEntryLocked(key string, rec webFetchCacheRecord) {
	delete(c.entries, key)
	c.totalBytes -= rec.size
	if c.totalBytes < 0 {
		c.totalBytes = 0
	}
}

// evictOldestLocked drops the oldest entry by StoredAt. The caller must
// hold c.mu. This is intentionally O(N) — the cache is bounded so the
// scan stays fast in practice and avoids the bookkeeping cost of an
// auxiliary linked list.
func (c *WebFetchCache) evictOldestLocked() {
	var oldestKey string
	var oldestRec webFetchCacheRecord
	first := true
	for k, rec := range c.entries {
		if first || rec.recency < oldestRec.recency {
			oldestKey = k
			oldestRec = rec
			first = false
		}
	}
	if !first {
		c.deleteEntryLocked(oldestKey, oldestRec)
	}
}

// entrySize estimates the byte footprint of a cached entry for byte-cap
// accounting. Prefers the explicit Bytes field when set, falls back to
// ContentLength, and finally to len(Body). The result always includes a
// small fixed-size overhead so empty bodies still consume a slot.
func entrySize(v WebFetchCacheEntry) int {
	const overhead = 256 // headers + struct overhead estimate
	switch {
	case v.CacheSize > 0:
		return v.CacheSize + overhead
	case v.Bytes > 0:
		return v.Bytes + overhead
	case v.ContentLength > 0:
		return v.ContentLength + overhead
	default:
		return len(v.Body) + overhead
	}
}
