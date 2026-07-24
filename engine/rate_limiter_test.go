package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── unlimited limiter ────────────────────────────────────────────────────────

func TestRateLimiter_Unlimited_TryAcquire(t *testing.T) {
	rl := NewRateLimiter(0, 0)
	for i := 0; i < 1000; i++ {
		if !rl.TryAcquire() {
			t.Fatalf("TryAcquire should always succeed for unlimited limiter (iter %d)", i)
		}
	}
}

func TestRateLimiter_Unlimited_Wait(t *testing.T) {
	rl := NewRateLimiter(0, 0)
	ctx := context.Background()
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("Wait should return nil for unlimited limiter: %v", err)
	}
}

func TestRateLimiter_Unlimited_CancelledContext(t *testing.T) {
	rl := NewRateLimiter(0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	if err := rl.Wait(ctx); err == nil {
		t.Fatal("Wait should return context error when context is already cancelled")
	}
}

// ─── burst capacity ───────────────────────────────────────────────────────────

func TestRateLimiter_BurstCapacity(t *testing.T) {
	burst := 5
	rl := NewRateLimiter(1, burst) // 1 token/s, burst=5; starts full

	// Should be able to acquire burst tokens immediately.
	for i := 0; i < burst; i++ {
		if !rl.TryAcquire() {
			t.Fatalf("TryAcquire should succeed for token %d within burst", i+1)
		}
	}
	// Next acquire should fail (bucket empty).
	if rl.TryAcquire() {
		t.Error("TryAcquire should fail when burst exhausted")
	}
}

// ─── token refill timing ──────────────────────────────────────────────────────

func TestRateLimiter_TokenRefill(t *testing.T) {
	// 10 tokens/s, burst=1; after exhausting the bucket wait ~150 ms for refill.
	rl := NewRateLimiter(10, 1)

	if !rl.TryAcquire() {
		t.Fatal("first TryAcquire should succeed (burst=1)")
	}
	if rl.TryAcquire() {
		t.Error("second TryAcquire should fail immediately")
	}

	// Wait 150 ms → at 10 tok/s we should have 1.5 tokens refilled → ≥1.
	time.Sleep(150 * time.Millisecond)
	if !rl.TryAcquire() {
		t.Error("TryAcquire should succeed after refill wait")
	}
}

// ─── Wait blocks then unblocks ────────────────────────────────────────────────

func TestRateLimiter_Wait_BlocksAndUnblocks(t *testing.T) {
	// 20 tokens/s, burst=1: after exhausting, Wait should unblock in ~50 ms.
	rl := NewRateLimiter(20, 1)

	if !rl.TryAcquire() {
		t.Fatal("first TryAcquire should succeed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 10*time.Millisecond {
		t.Errorf("Wait returned too quickly (%v) — expected at least 10 ms", elapsed)
	}
}

// ─── context cancellation ─────────────────────────────────────────────────────

func TestRateLimiter_Wait_ContextCancellation(t *testing.T) {
	// Very slow rate: 0.1 token/s → Wait would block for ~10 s.
	rl := NewRateLimiter(0.1, 1)

	// Exhaust the single token.
	if !rl.TryAcquire() {
		t.Fatal("first TryAcquire should succeed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)
	if err == nil {
		t.Fatal("Wait should return context error when cancelled")
	}
}

func TestRateLimiter_Wait_AlreadyCancelledContext(t *testing.T) {
	rl := NewRateLimiter(5, 5)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rl.Wait(ctx); err == nil {
		t.Fatal("Wait should return error for already-cancelled context")
	}
}

// ─── concurrent access ────────────────────────────────────────────────────────

func TestRateLimiter_ConcurrentTryAcquire(t *testing.T) {
	burst := 100
	rl := NewRateLimiter(float64(burst), burst)

	var acquired int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rl.TryAcquire() {
				atomic.AddInt64(&acquired, 1)
			}
		}()
	}
	wg.Wait()

	if acquired > int64(burst) {
		t.Errorf("acquired %d tokens but burst is %d", acquired, burst)
	}
}

func TestRateLimiter_ConcurrentWait(t *testing.T) {
	// 50 tokens/s, burst=50: all 50 goroutines should finish within ~1 s.
	burst := 50
	rl := NewRateLimiter(float64(burst), burst)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, burst)
	for i := 0; i < burst; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := rl.Wait(ctx); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Wait returned error: %v", err)
	}
}

// ─── NewRateLimiterFromEnv ────────────────────────────────────────────────────

func TestNewRateLimiterFromEnv_Missing(t *testing.T) {
	t.Setenv("LUBAN_CODE_RATE_LIMIT", "")
	t.Setenv("DEEPSEEK_CODE_RATE_LIMIT", "")
	t.Setenv("CLAUDE_RATE_LIMIT", "")
	rl := NewRateLimiterFromEnv()
	if !rl.unlimited() {
		t.Error("expected unlimited limiter when env var is empty")
	}
}

func TestNewRateLimiterFromEnv_ValidRate(t *testing.T) {
	t.Setenv("LUBAN_CODE_RATE_LIMIT", "5")
	t.Setenv("DEEPSEEK_CODE_RATE_LIMIT", "")
	t.Setenv("CLAUDE_RATE_LIMIT", "")
	rl := NewRateLimiterFromEnv()
	if rl.unlimited() {
		t.Error("expected non-unlimited limiter for rate=5")
	}
	if rl.rate != 5 {
		t.Errorf("expected rate 5, got %v", rl.rate)
	}
	if rl.burst != 5 {
		t.Errorf("expected burst 5, got %d", rl.burst)
	}
}

func TestNewRateLimiterFromEnv_InvalidRate(t *testing.T) {
	t.Setenv("LUBAN_CODE_RATE_LIMIT", "not-a-number")
	t.Setenv("DEEPSEEK_CODE_RATE_LIMIT", "")
	t.Setenv("CLAUDE_RATE_LIMIT", "")
	rl := NewRateLimiterFromEnv()
	if !rl.unlimited() {
		t.Error("expected unlimited limiter for invalid env var")
	}
}

func TestNewRateLimiterFromEnv_ZeroRate(t *testing.T) {
	t.Setenv("LUBAN_CODE_RATE_LIMIT", "0")
	t.Setenv("DEEPSEEK_CODE_RATE_LIMIT", "")
	t.Setenv("CLAUDE_RATE_LIMIT", "")
	rl := NewRateLimiterFromEnv()
	if !rl.unlimited() {
		t.Error("expected unlimited limiter for rate=0")
	}
}

func TestNewRateLimiterFromEnv_NegativeRate(t *testing.T) {
	t.Setenv("LUBAN_CODE_RATE_LIMIT", "-3.5")
	t.Setenv("DEEPSEEK_CODE_RATE_LIMIT", "")
	t.Setenv("CLAUDE_RATE_LIMIT", "")
	rl := NewRateLimiterFromEnv()
	if !rl.unlimited() {
		t.Error("expected unlimited limiter for negative rate")
	}
}

func TestNewRateLimiterFromEnv_LegacyFallback(t *testing.T) {
	t.Setenv("LUBAN_CODE_RATE_LIMIT", "")
	t.Setenv("DEEPSEEK_CODE_RATE_LIMIT", "")
	t.Setenv("CLAUDE_RATE_LIMIT", "2")
	rl := NewRateLimiterFromEnv()
	if rl.unlimited() {
		t.Error("expected non-unlimited limiter from legacy fallback")
	}
	if rl.rate != 2 {
		t.Errorf("expected rate 2, got %v", rl.rate)
	}
}

func TestNewRateLimiterFromEnvDeepSeekFallbackAndPRCPrecedence(t *testing.T) {
	t.Setenv("CLAUDE_RATE_LIMIT", "1")
	t.Setenv("DEEPSEEK_CODE_RATE_LIMIT", "2")
	t.Setenv("LUBAN_CODE_RATE_LIMIT", "3")
	if got := NewRateLimiterFromEnv().rate; got != 3 {
		t.Fatalf("rate = %v, want LUBAN rate 3", got)
	}
	t.Setenv("LUBAN_CODE_RATE_LIMIT", "")
	if got := NewRateLimiterFromEnv().rate; got != 2 {
		t.Fatalf("rate = %v, want legacy DeepSeek rate 2", got)
	}
}
