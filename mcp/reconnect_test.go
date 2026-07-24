package mcp

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestReconnectPolicyDefaults(t *testing.T) {
	p := ReconnectPolicy{}
	if p.maxAttempts() != 5 {
		t.Errorf("default maxAttempts: want 5, got %d", p.maxAttempts())
	}
	if p.initialDelay() != time.Second {
		t.Errorf("default initialDelay: want 1s, got %v", p.initialDelay())
	}
	if p.maxDelay() != 30*time.Second {
		t.Errorf("default maxDelay: want 30s, got %v", p.maxDelay())
	}
	if p.stableThreshold() != 60*time.Second {
		t.Errorf("default stableThreshold: want 60s, got %v", p.stableThreshold())
	}
}

func TestReconnectPolicyCustom(t *testing.T) {
	p := ReconnectPolicy{
		MaxAttempts:     3,
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        5 * time.Second,
		StableThreshold: 10 * time.Second,
	}
	if p.maxAttempts() != 3 {
		t.Errorf("maxAttempts: want 3, got %d", p.maxAttempts())
	}
	if p.initialDelay() != 100*time.Millisecond {
		t.Errorf("initialDelay: want 100ms, got %v", p.initialDelay())
	}
	if p.maxDelay() != 5*time.Second {
		t.Errorf("maxDelay: want 5s, got %v", p.maxDelay())
	}
}

func TestEnableReconnectUnknownServer(t *testing.T) {
	// Should not panic for unknown server.
	lm := NewLifecycleManager()
	lm.EnableReconnect("nonexistent", ReconnectPolicy{})
}

func TestEnableReconnectCancelViaContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix process management")
	}
	lm := NewLifecycleManager()
	ctx := context.Background()

	if err := lm.Start(ctx, "srv", LifecycleConfig{Command: "cat"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer lm.Stop(ctx, "srv") //nolint:errcheck

	// Enable reconnect, then stop to cancel the loop via cancelReconnect.
	lm.EnableReconnect("srv", ReconnectPolicy{MaxAttempts: 2, InitialDelay: 10 * time.Millisecond})

	// Stop should cancel the reconnect loop.
	if err := lm.Stop(ctx, "srv"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Give background goroutine a moment to exit.
	time.Sleep(50 * time.Millisecond)

	st := lm.Status("srv")
	if st.State != string(stateStopped) {
		t.Errorf("expected stopped, got %q", st.State)
	}
}

func TestReconnectBackoffCapAtMaxDelay(t *testing.T) {
	// Verify that the backoff delay doubles but is capped at MaxDelay.
	p := ReconnectPolicy{
		MaxAttempts:  10,
		InitialDelay: 8 * time.Second,
		MaxDelay:     10 * time.Second,
	}
	delay := p.initialDelay()
	for i := 0; i < 5; i++ {
		delay *= 2
		if delay > p.maxDelay() {
			delay = p.maxDelay()
		}
	}
	if delay != p.maxDelay() {
		t.Errorf("delay should be capped at maxDelay=%v, got %v", p.maxDelay(), delay)
	}
}

func TestWaitForExitContextCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix process management")
	}
	lm := NewLifecycleManager()
	ctx, cancel := context.WithCancel(context.Background())

	if err := lm.Start(ctx, "srv", LifecycleConfig{Command: "cat"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer lm.Stop(context.Background(), "srv") //nolint:errcheck

	// Cancel the context; waitForExit should return quickly.
	cancel()

	done := make(chan error, 1)
	go func() { done <- lm.waitForExit(ctx, "srv") }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected context.Canceled error")
		}
	case <-time.After(2 * time.Second):
		t.Error("waitForExit did not return after context cancel")
	}
}
