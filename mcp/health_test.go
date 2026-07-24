package mcp

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestStartStopHealthCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix process management")
	}
	lm := NewLifecycleManager()
	ctx := context.Background()

	if err := lm.Start(ctx, "srv", LifecycleConfig{Command: "cat"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer lm.Stop(ctx, "srv") //nolint:errcheck

	// Start health check with a short interval.
	lm.StartHealthCheck("srv", 200*time.Millisecond)

	// Let it run a couple of ticks — server is alive so no error expected.
	time.Sleep(500 * time.Millisecond)

	st := lm.Status("srv")
	if st.State != string(stateRunning) {
		t.Errorf("server should still be running, got %q (lastError: %v)", st.State, st.LastError)
	}

	lm.StopHealthCheck("srv")
}

func TestHealthCheckUnknownServer(t *testing.T) {
	// Should not panic.
	lm := NewLifecycleManager()
	lm.StartHealthCheck("ghost", 100*time.Millisecond)
	lm.StopHealthCheck("ghost")
}

func TestPingRunningServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix process management")
	}
	lm := NewLifecycleManager()
	ctx := context.Background()

	if err := lm.Start(ctx, "srv", LifecycleConfig{Command: "cat"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer lm.Stop(ctx, "srv") //nolint:errcheck

	if err := lm.pingServer("srv"); err != nil {
		t.Errorf("pingServer on running process: %v", err)
	}
}

func TestPingStoppedServer(t *testing.T) {
	lm := NewLifecycleManager()
	// Server not started — ping should fail.
	if err := lm.pingServer("nonexistent"); err == nil {
		t.Error("expected error pinging non-existent server")
	}
}

func TestHealthCheckDetectsDeadProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix process management")
	}
	lm := NewLifecycleManager()
	ctx := context.Background()

	// Start a long-running process so we can kill it cleanly.
	if err := lm.Start(ctx, "srv", LifecycleConfig{Command: "cat"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Grab the managedServer to manipulate it directly.
	lm.mu.Lock()
	ms := lm.servers["srv"]
	lm.mu.Unlock()

	// Kill the process and reap it so signal(0) will fail (not a zombie).
	ms.mu.Lock()
	proc := ms.cmd.Process
	cmd := ms.cmd
	ms.mu.Unlock()

	proc.Kill()          //nolint:errcheck
	cmd.Wait()           //nolint:errcheck // reap zombie so signal(0) fails

	// Now pretend the lifecycle manager still thinks it's running.
	ms.mu.Lock()
	ms.state = stateRunning
	ms.mu.Unlock()

	// Start health check — it should detect the dead (and reaped) process.
	lm.StartHealthCheck("srv", 100*time.Millisecond)
	defer lm.StopHealthCheck("srv")

	// After enough failed pings (3 consecutive at 100ms interval = ~300ms),
	// state should flip to error. Give generous deadline for CI slowness.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if st := lm.Status("srv"); st.State == string(stateError) {
			return // test passed
		}
	}
	t.Errorf("expected server state=error after health check failures, got %q", lm.Status("srv").State)
}

func TestStopHealthCheckCancelsLoop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix process management")
	}
	lm := NewLifecycleManager()
	ctx := context.Background()

	if err := lm.Start(ctx, "srv", LifecycleConfig{Command: "cat"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer lm.Stop(ctx, "srv") //nolint:errcheck

	lm.StartHealthCheck("srv", 50*time.Millisecond)
	time.Sleep(120 * time.Millisecond)
	lm.StopHealthCheck("srv")

	// After stopping, the cancelHealth field should be nil.
	lm.mu.Lock()
	ms := lm.servers["srv"]
	lm.mu.Unlock()
	ms.mu.Lock()
	ch := ms.cancelHealth
	ms.mu.Unlock()

	if ch != nil {
		t.Error("cancelHealth should be nil after StopHealthCheck")
	}
}
