package tools

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestSSEBackoffSchedule covers MCP-01 base cooldown table (1s, 2s, 4s,
// 8s, 16s, 30s cap).
func TestSSEBackoffSchedule(t *testing.T) {
	cases := []struct {
		attempt int
		min     time.Duration
		max     time.Duration
	}{
		{1, time.Second, time.Second},
		{2, 2 * time.Second, 2 * time.Second},
		{3, 4 * time.Second, 4 * time.Second},
		{4, 8 * time.Second, 8 * time.Second},
		{5, 16 * time.Second, 16 * time.Second},
		{6, 30 * time.Second, 30 * time.Second}, // cap
		{99, 30 * time.Second, 30 * time.Second},
	}
	for _, c := range cases {
		got := sseBackoffBase(c.attempt)
		if got != c.min {
			t.Errorf("sseBackoffBase(%d) = %v, want %v", c.attempt, got, c.min)
		}
	}
}

// TestSSEBackoffJitterWithinRange ensures jitter is bounded ±20%.
func TestSSEBackoffJitterWithinRange(t *testing.T) {
	for i := 0; i < 50; i++ {
		base := sseBackoffBase(3) // 4s
		got := SSEBackoffWithJitter(3)
		// 4s ±20% = 3.2s..4.8s
		if got < base*8/10 || got > base*12/10 {
			t.Errorf("jitter out of range: got %v, base %v", got, base)
		}
	}
}

// TestReconnectWithBackoffSuccess verifies that a successful attempt
// resets the failure counter and returns nil.
func TestReconnectWithBackoffSuccess(t *testing.T) {
	tracker := NewSSEReconnectAttempts()
	tracker.Record("srv") // pre-populate one failure
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ReconnectWithBackoff(ctx, tracker, "srv", 1, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if tracker.Failed("srv") {
		t.Fatalf("counter should reset on success")
	}
}

// TestReconnectWithBackoffFiresLossListener verifies that crossing the
// threshold triggers the mcp.connection_lost listener exactly once.
func TestReconnectWithBackoffFiresLossListener(t *testing.T) {
	prev := sseLossThreshold()
	defer SetSSEConnectionLostThreshold(prev)
	SetSSEConnectionLostThreshold(2)

	var fired atomic.Int32
	SetSSEConnectionLossListener(func(name string) {
		if name == "srv" {
			fired.Add(1)
		}
	})
	defer SetSSEConnectionLossListener(nil)

	tracker := NewSSEReconnectAttempts()
	// Override backoff so the test runs fast.
	originalCap := sseBackoffMaxCooldown
	sseBackoffMaxCooldown = time.Millisecond
	defer func() { sseBackoffMaxCooldown = originalCap }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := ReconnectWithBackoff(ctx, tracker, "srv", 0, func(ctx context.Context) error {
		return errors.New("nope")
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var lost *sseConnectionLost
	if !errors.As(err, &lost) {
		t.Fatalf("expected connection_lost wrapper, got %T %v", err, err)
	}
	if fired.Load() != 1 {
		t.Fatalf("expected listener to fire once, got %d", fired.Load())
	}
}
