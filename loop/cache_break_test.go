package loop

import (
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestCacheBreakDetector_FirstCall_NoBreak(t *testing.T) {
	d := &CacheBreakDetector{}
	evt := d.Check(&types.Usage{CacheReadInputTokens: 50000})
	if evt != nil {
		t.Error("expected nil on first call (no baseline)")
	}
}

func TestCacheBreakDetector_StableCache_NoBreak(t *testing.T) {
	d := &CacheBreakDetector{}
	// First call: establish baseline
	d.Check(&types.Usage{CacheReadInputTokens: 50000})
	// Second call: stable cache (slight increase)
	evt := d.Check(&types.Usage{CacheReadInputTokens: 51000})
	if evt != nil {
		t.Error("expected nil when cache is stable")
	}
}

func TestCacheBreakDetector_SmallDrop_NoBreak(t *testing.T) {
	d := &CacheBreakDetector{}
	d.Check(&types.Usage{CacheReadInputTokens: 50000})
	// Drop of 1500 < minCacheMissTokens (2000) — should not trigger
	evt := d.Check(&types.Usage{CacheReadInputTokens: 48500})
	if evt != nil {
		t.Error("expected nil for small drop below threshold")
	}
}

func TestCacheBreakDetector_SignificantDrop_Break(t *testing.T) {
	d := &CacheBreakDetector{}
	d.Check(&types.Usage{CacheReadInputTokens: 100000})
	// Massive drop: 100K → 0
	evt := d.Check(&types.Usage{CacheReadInputTokens: 0, CacheCreationInputTokens: 95000})
	if evt == nil {
		t.Fatal("expected cache break event on massive drop")
	}
	if evt.TokenDrop != 100000 {
		t.Errorf("TokenDrop = %d, want 100000", evt.TokenDrop)
	}
	if evt.PrevCacheRead != 100000 {
		t.Errorf("PrevCacheRead = %d, want 100000", evt.PrevCacheRead)
	}
	if evt.CurrCacheRead != 0 {
		t.Errorf("CurrCacheRead = %d, want 0", evt.CurrCacheRead)
	}
	if evt.CurrCacheCreate != 95000 {
		t.Errorf("CurrCacheCreate = %d, want 95000", evt.CurrCacheCreate)
	}
}

func TestCacheBreakDetector_ExactThreshold_NoBreak(t *testing.T) {
	d := &CacheBreakDetector{}
	d.Check(&types.Usage{CacheReadInputTokens: 100000})
	// 4% drop — below 5% threshold
	evt := d.Check(&types.Usage{CacheReadInputTokens: 96000})
	if evt != nil {
		t.Error("expected nil for drop below 5% threshold")
	}
}

func TestCacheBreakDetector_JustOverThreshold_Break(t *testing.T) {
	d := &CacheBreakDetector{}
	d.Check(&types.Usage{CacheReadInputTokens: 100000})
	// 6% drop + 6K tokens > 2K min — should trigger
	evt := d.Check(&types.Usage{CacheReadInputTokens: 94000})
	if evt == nil {
		t.Fatal("expected cache break at 6% drop (>5% threshold, >2K tokens)")
	}
	if evt.CallNumber != 2 {
		t.Errorf("CallNumber = %d, want 2", evt.CallNumber)
	}
}

func TestCacheBreakDetector_NotifyCompaction_ResetsBaseline(t *testing.T) {
	d := &CacheBreakDetector{}
	d.Check(&types.Usage{CacheReadInputTokens: 100000})
	d.NotifyCompaction()
	// After compaction, drop to 0 should NOT trigger (baseline was reset)
	evt := d.Check(&types.Usage{CacheReadInputTokens: 0})
	if evt != nil {
		t.Error("expected nil after compaction reset")
	}
}

func TestCacheBreakDetector_PrevZero_NoBreak(t *testing.T) {
	d := &CacheBreakDetector{}
	d.Check(&types.Usage{CacheReadInputTokens: 0})
	// Previous was 0 — can't detect a "drop" from nothing
	evt := d.Check(&types.Usage{CacheReadInputTokens: 0})
	if evt != nil {
		t.Error("expected nil when previous cache read was 0")
	}
}

func TestCacheBreakDetector_NilUsage(t *testing.T) {
	d := &CacheBreakDetector{}
	evt := d.Check(nil)
	if evt != nil {
		t.Error("expected nil for nil usage")
	}
}

func TestCacheBreakEvent_String(t *testing.T) {
	evt := CacheBreakEvent{
		PrevCacheRead:   100000,
		CurrCacheRead:   5000,
		CurrCacheCreate: 90000,
		TokenDrop:       95000,
		DropPercent:     0.95,
		TimeSincePrev:   3 * time.Second,
		CallNumber:      5,
		ProbableCause:   "prompt content changed (high cache_creation_tokens indicates re-caching)",
	}
	s := evt.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
	t.Logf("CacheBreakEvent.String() = %s", s)
}

func TestClassifyCause_TTL1Hour(t *testing.T) {
	cause := classifyCause(2*time.Hour, 0)
	if cause == "" {
		t.Error("expected non-empty cause")
	}
	t.Logf("1h TTL cause: %s", cause)
}

func TestClassifyCause_TTL5Min(t *testing.T) {
	cause := classifyCause(6*time.Minute, 0)
	if cause == "" {
		t.Error("expected non-empty cause")
	}
	t.Logf("5min TTL cause: %s", cause)
}

func TestClassifyCause_PromptChanged(t *testing.T) {
	cause := classifyCause(10*time.Second, 50000)
	if cause == "" {
		t.Error("expected non-empty cause")
	}
	t.Logf("prompt changed cause: %s", cause)
}

func TestClassifyCause_ServerSide(t *testing.T) {
	cause := classifyCause(10*time.Second, 0)
	if cause == "" {
		t.Error("expected non-empty cause")
	}
	t.Logf("server-side cause: %s", cause)
}
