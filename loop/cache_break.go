package loop

import (
	"fmt"
	"sync"
	"time"

	"github.com/agent-dance/luban/types"
)

// minCacheMissTokens is the minimum absolute token drop to trigger a cache break alert.
// Small fluctuations (<2K) are normal due to message length changes.
const minCacheMissTokens = 2000

// cacheBreakThreshold is the relative threshold: if cacheRead drops below
// this fraction of the previous cacheRead, it's considered a cache break.
const cacheBreakThreshold = 0.95

// CacheBreakDetector monitors prompt cache usage between consecutive API calls
// and detects when the cache hit rate drops significantly, indicating that cached
// KV vectors were invalidated (a "cache break").
//
// Common causes: system prompt changed, tools changed, model changed, TTL expired.
type CacheBreakDetector struct {
	mu              sync.Mutex
	prevCacheRead   int
	prevCacheCreate int
	prevTime        time.Time
	callCount       int
	hasBaseline     bool
}

// CacheBreakEvent contains information about a detected cache break.
type CacheBreakEvent struct {
	PrevCacheRead    int
	CurrCacheRead    int
	PrevCacheCreate  int
	CurrCacheCreate  int
	TokenDrop        int
	DropPercent      float64
	TimeSincePrev    time.Duration
	CallNumber       int
	ProbableCause    string
}

// String returns a human-readable summary of the cache break event.
func (e CacheBreakEvent) String() string {
	return fmt.Sprintf("[CACHE BREAK] %s [call #%d, cache read: %dK → %dK (-%dK, -%.0f%%), creation: %dK, gap: %s]",
		e.ProbableCause,
		e.CallNumber,
		e.PrevCacheRead/1000, e.CurrCacheRead/1000,
		e.TokenDrop/1000, e.DropPercent*100,
		e.CurrCacheCreate/1000,
		e.TimeSincePrev.Round(time.Second))
}

// Check inspects the usage from the latest API response and returns a
// *CacheBreakEvent if a significant cache break was detected. Returns nil
// if no break was detected, if this is the first call (no baseline), or if
// cache read tokens were already zero (nothing to compare).
func (d *CacheBreakDetector) Check(usage *types.Usage) *CacheBreakEvent {
	if usage == nil {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.callCount++
	now := time.Now()

	currRead := usage.CacheReadInputTokens
	currCreate := usage.CacheCreationInputTokens

	defer func() {
		// Always update baseline for next comparison
		d.prevCacheRead = currRead
		d.prevCacheCreate = currCreate
		d.prevTime = now
		d.hasBaseline = true
	}()

	// Skip if no baseline yet (first call)
	if !d.hasBaseline {
		return nil
	}

	// Skip if previous cache read was zero (nothing to compare against)
	if d.prevCacheRead == 0 {
		return nil
	}

	// Calculate drop
	tokenDrop := d.prevCacheRead - currRead
	if tokenDrop < minCacheMissTokens {
		return nil // drop too small to be significant
	}

	dropPercent := float64(tokenDrop) / float64(d.prevCacheRead)
	if float64(currRead) >= float64(d.prevCacheRead)*cacheBreakThreshold {
		return nil // within normal fluctuation range (<5% drop)
	}

	// Cache break detected — determine probable cause
	timeSincePrev := now.Sub(d.prevTime)
	cause := classifyCause(timeSincePrev, currCreate)

	return &CacheBreakEvent{
		PrevCacheRead:   d.prevCacheRead,
		CurrCacheRead:   currRead,
		PrevCacheCreate: d.prevCacheCreate,
		CurrCacheCreate: currCreate,
		TokenDrop:       tokenDrop,
		DropPercent:     dropPercent,
		TimeSincePrev:   timeSincePrev,
		CallNumber:      d.callCount,
		ProbableCause:   cause,
	}
}

// NotifyCompaction resets the baseline after a context compaction, since the
// token drop is expected and should not be flagged as a cache break.
func (d *CacheBreakDetector) NotifyCompaction() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hasBaseline = false
}

// classifyCause attempts to identify why the cache broke based on timing and
// creation token patterns.
func classifyCause(timeSincePrev time.Duration, currCreate int) string {
	// TTL expiry detection
	if timeSincePrev > time.Hour {
		return "likely 1h TTL expiry (long gap between calls)"
	}
	if timeSincePrev > 5*time.Minute {
		return "likely 5min TTL expiry (idle gap)"
	}

	// High cache creation = the prompt was re-cached from scratch
	if currCreate > minCacheMissTokens {
		return "prompt content changed (high cache_creation_tokens indicates re-caching)"
	}

	// Short gap + no re-caching = server-side eviction or routing change
	return "likely server-side (prompt unchanged, short gap, low creation)"
}
