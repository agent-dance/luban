package tools

import (
	"testing"
	"time"
)

func TestWebFetchCache_GetSetRoundtrip(t *testing.T) {
	c := NewWebFetchCacheWithTTL(time.Minute, 0)
	defer c.Stop()

	key := c.MakeKey("https://example.com", "summarise")
	c.Set(key, WebFetchCacheEntry{Body: "body", URL: "https://example.com", Bytes: 4})

	got, ok := c.Get(key)
	if !ok {
		t.Fatal("expected hit")
	}
	if got.Body != "body" || got.URL != "https://example.com" {
		t.Fatalf("unexpected entry: %+v", got)
	}
	if got.StoredAt.IsZero() {
		t.Fatalf("StoredAt not populated")
	}
}

func TestWebFetchCache_DifferentPromptsShareRawURLKey(t *testing.T) {
	a := WebFetchCacheKey("https://example.com", "first")
	b := WebFetchCacheKey("https://example.com", "second")
	if a != b {
		t.Fatalf("prompt must not affect raw URL cache key: %s != %s", a, b)
	}
}

func TestWebFetchCache_KeyPreservesExactOriginalURL(t *testing.T) {
	a := WebFetchCacheKey("https://Example.com/Page", "p")
	b := WebFetchCacheKey("https://example.com/Page", "p")
	if a == b {
		t.Fatalf("TS cache uses the exact original URL string")
	}
	c := WebFetchCacheKey("  https://Example.com/Page  ", "  p  ")
	if a == c {
		t.Fatalf("outer whitespace is part of the exact original cache key")
	}
}

func TestWebFetchCache_ExpiryEvicts(t *testing.T) {
	c := NewWebFetchCacheWithTTL(50*time.Millisecond, 0)
	defer c.Stop()

	now := time.Now()
	c.now = func() time.Time { return now }

	key := c.MakeKey("https://example.com", "x")
	c.Set(key, WebFetchCacheEntry{Body: "body"})

	if _, ok := c.Get(key); !ok {
		t.Fatal("expected immediate hit")
	}

	now = now.Add(time.Second)
	if _, ok := c.Get(key); ok {
		t.Fatal("expected expired entry to be evicted")
	}
	if c.Len() != 0 {
		t.Fatalf("expected len=0 after eviction, got %d", c.Len())
	}
}

func TestWebFetchCache_PurgeBackgroundEvicts(t *testing.T) {
	c := NewWebFetchCacheWithTTL(20*time.Millisecond, 10*time.Millisecond)
	defer c.Stop()

	c.Set(c.MakeKey("https://example.com", "x"), WebFetchCacheEntry{Body: "body"})
	if c.Len() != 1 {
		t.Fatalf("setup expected len=1, got %d", c.Len())
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c.Len() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected background purge to evict expired entry, len=%d", c.Len())
}

func TestWebFetchCache_StopIsIdempotent(t *testing.T) {
	c := NewWebFetchCache()
	c.Stop()
	c.Stop() // must not panic
}

func TestWebFetchCache_SetAfterStopIsNoOp(t *testing.T) {
	c := NewWebFetchCache()
	c.Stop()
	key := c.MakeKey("https://example.com", "x")
	c.Set(key, WebFetchCacheEntry{Body: "body"})
	if _, ok := c.Get(key); ok {
		t.Fatal("Set after Stop should be a no-op")
	}
}

func TestWebFetchCache_KeyDeterministic(t *testing.T) {
	a := WebFetchCacheKey("https://example.com/p", "summarise this")
	b := WebFetchCacheKey("https://example.com/p", "summarise this")
	if a != b {
		t.Fatal("identical inputs must produce identical keys")
	}
	if len(a) != 64 {
		t.Fatalf("expected sha256 hex (64 chars), got %d", len(a))
	}
}

func TestWebFetchCache_ByteCapUsesLRURecency(t *testing.T) {
	c := NewWebFetchCacheWithTTL(time.Minute, 0)
	defer c.Stop()
	c.maxBytes = 540 // two 10-byte entries plus the fixed accounting overhead
	a, b, newest := c.MakeKey("https://a.example"), c.MakeKey("https://b.example"), c.MakeKey("https://c.example")
	c.Set(a, WebFetchCacheEntry{Body: "aaaaaaaaaa", CacheSize: 10})
	c.Set(b, WebFetchCacheEntry{Body: "bbbbbbbbbb", CacheSize: 10})
	if _, ok := c.Get(a); !ok {
		t.Fatal("expected a before recency refresh")
	}
	c.Set(newest, WebFetchCacheEntry{Body: "cccccccccc", CacheSize: 10})
	if _, ok := c.Get(b); ok {
		t.Fatal("least recently used entry b should be evicted")
	}
	if _, ok := c.Get(a); !ok {
		t.Fatal("recently read entry a should remain")
	}
}
