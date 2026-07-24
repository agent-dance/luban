package engine

import (
	"context"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
)

// RateLimiter implements a token bucket algorithm for API call rate limiting.
//
// A limiter created with rate <= 0 or burst <= 0 is "unlimited": TryAcquire
// always returns true and Wait always returns immediately.
//
// All public methods are safe for concurrent use.
type RateLimiter struct {
	rate       float64 // tokens added per second (steady-state refill rate)
	burst      int     // maximum bucket capacity (also initial fill)
	tokens     float64 // current token count
	lastRefill time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a RateLimiter with the given steady-state refill rate
// (tokens per second) and burst capacity.
//
// The bucket starts full (tokens == burst).
// Pass rate <= 0 or burst <= 0 for an unlimited limiter.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		rate:       rate,
		burst:      burst,
		lastRefill: time.Now(),
	}
	if !rl.unlimited() {
		rl.tokens = float64(burst)
	}
	return rl
}

// NewRateLimiterFromEnv creates a RateLimiter whose rate is read from the
// LUBAN_CODE_RATE_LIMIT environment variable (requests per second, float).
// DeepSeek Code and Claude variables remain supported as legacy fallbacks.
// If the variable is absent, empty, or <= 0, an unlimited limiter is returned.
// The burst capacity defaults to ceil(rate), minimum 1.
func NewRateLimiterFromEnv() *RateLimiter {
	val := os.Getenv(brand.RateLimitEnv)
	if val == "" {
		val = os.Getenv(brand.LegacyDeepSeekRateLimitEnv)
	}
	if val == "" {
		val = os.Getenv(brand.LegacyRateLimitEnv)
	}
	if val == "" {
		return NewRateLimiter(0, 0)
	}
	rate, err := strconv.ParseFloat(val, 64)
	if err != nil || rate <= 0 {
		return NewRateLimiter(0, 0)
	}
	burst := int(math.Ceil(rate))
	if burst < 1 {
		burst = 1
	}
	return NewRateLimiter(rate, burst)
}

// unlimited reports whether this limiter imposes no constraints.
func (r *RateLimiter) unlimited() bool {
	return r.rate <= 0 || r.burst <= 0
}

// refill adds tokens proportional to the time elapsed since the last refill,
// capped at burst capacity. Caller must hold r.mu.
func (r *RateLimiter) refill(now time.Time) {
	elapsed := now.Sub(r.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	r.tokens += elapsed * r.rate
	if r.tokens > float64(r.burst) {
		r.tokens = float64(r.burst)
	}
	r.lastRefill = now
}

// TryAcquire attempts to consume one token without blocking.
// Returns true if a token was available and consumed.
// Always returns true for an unlimited limiter.
func (r *RateLimiter) TryAcquire() bool {
	if r.unlimited() {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refill(time.Now())
	if r.tokens >= 1 {
		r.tokens--
		return true
	}
	return false
}

// Wait blocks until one token is available, then consumes it.
// Returns nil on success or ctx.Err() if the context is cancelled first.
// Always returns nil immediately (unless ctx is already cancelled) for an
// unlimited limiter.
func (r *RateLimiter) Wait(ctx context.Context) error {
	if r.unlimited() {
		// Check for pre-cancelled context even in unlimited mode.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	for {
		// Honour cancellation before each attempt.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		r.mu.Lock()
		r.refill(time.Now())
		if r.tokens >= 1 {
			r.tokens--
			r.mu.Unlock()
			return nil
		}

		// Compute wait until next token is available.
		// (1 - tokens) / rate gives seconds until the bucket has 1 token.
		waitSecs := (1 - r.tokens) / r.rate
		r.mu.Unlock()

		// Cap at 1 s so we can re-check context cancellation regularly.
		if waitSecs > 1.0 {
			waitSecs = 1.0
		}
		waitDur := time.Duration(waitSecs * float64(time.Second))
		if waitDur < time.Millisecond {
			waitDur = time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDur):
			// Loop back to re-check.
		}
	}
}
