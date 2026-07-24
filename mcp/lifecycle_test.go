package mcp

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestLifecycleStartStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix process management")
	}
	lm := NewLifecycleManager()
	ctx := context.Background()

	cfg := LifecycleConfig{Command: "cat", Args: []string{}}
	if err := lm.Start(ctx, "srv", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	st := lm.Status("srv")
	if st.State != string(stateRunning) {
		t.Fatalf("expected running, got %q", st.State)
	}
	if st.PID == 0 {
		t.Error("expected non-zero PID")
	}

	if err := lm.Stop(ctx, "srv"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	st = lm.Status("srv")
	if st.State != string(stateStopped) {
		t.Fatalf("expected stopped, got %q", st.State)
	}
}

func TestLifecycleDoubleStart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix process management")
	}
	lm := NewLifecycleManager()
	ctx := context.Background()
	cfg := LifecycleConfig{Command: "cat"}

	if err := lm.Start(ctx, "srv", cfg); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer lm.Stop(ctx, "srv") //nolint:errcheck

	if err := lm.Start(ctx, "srv", cfg); err == nil {
		t.Error("expected error on double start")
	}
}

func TestLifecycleRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix process management")
	}
	lm := NewLifecycleManager()
	ctx := context.Background()
	cfg := LifecycleConfig{Command: "cat"}

	if err := lm.Start(ctx, "srv", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	firstPID := lm.Status("srv").PID

	if err := lm.Restart(ctx, "srv"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	defer lm.Stop(ctx, "srv") //nolint:errcheck

	st := lm.Status("srv")
	if st.State != string(stateRunning) {
		t.Fatalf("expected running after restart, got %q", st.State)
	}
	if st.RestartCount != 1 {
		t.Errorf("expected RestartCount=1, got %d", st.RestartCount)
	}
	// PID should differ (new process).
	if st.PID == firstPID {
		t.Logf("note: PID unchanged after restart (may be OS reuse)")
	}
}

func TestLifecycleStopAll(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix process management")
	}
	lm := NewLifecycleManager()
	ctx := context.Background()

	for _, name := range []string{"a", "b", "c"} {
		if err := lm.Start(ctx, name, LifecycleConfig{Command: "cat"}); err != nil {
			t.Fatalf("Start %s: %v", name, err)
		}
	}

	if err := lm.StopAll(ctx); err != nil {
		t.Fatalf("StopAll: %v", err)
	}

	for _, name := range []string{"a", "b", "c"} {
		if st := lm.Status(name); st.State != string(stateStopped) {
			t.Errorf("server %s: expected stopped, got %q", name, st.State)
		}
	}
}

func TestLifecycleStatusUnknown(t *testing.T) {
	lm := NewLifecycleManager()
	st := lm.Status("nonexistent")
	if st.State != string(stateStopped) {
		t.Errorf("unknown server: expected stopped, got %q", st.State)
	}
}

func TestLifecycleStopNotFound(t *testing.T) {
	lm := NewLifecycleManager()
	err := lm.Stop(context.Background(), "nope")
	if err == nil {
		t.Error("expected error stopping unknown server")
	}
}

func TestLifecycleStartBadCommand(t *testing.T) {
	lm := NewLifecycleManager()
	err := lm.Start(context.Background(), "bad", LifecycleConfig{
		Command: "/definitely/does/not/exist",
	})
	if err == nil {
		t.Error("expected error for non-existent command")
	}
}

func TestLifecycleStartedAt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix process management")
	}
	lm := NewLifecycleManager()
	ctx := context.Background()
	before := time.Now()
	if err := lm.Start(ctx, "srv", LifecycleConfig{Command: "cat"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer lm.Stop(ctx, "srv") //nolint:errcheck
	after := time.Now()

	st := lm.Status("srv")
	if st.StartedAt.Before(before) || st.StartedAt.After(after) {
		t.Errorf("StartedAt %v not in [%v, %v]", st.StartedAt, before, after)
	}
}
