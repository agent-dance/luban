package tools

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openOrCreateForTest(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
}

func setMtimeForTest(path string, mt time.Time) error {
	return os.Chtimes(path, mt, mt)
}

// TestBackgroundTaskWatchdog_FlagsHardDeadline (TK-04): tasks running
// longer than HardDeadline must be flagged on Scan.
func TestBackgroundTaskWatchdog_FlagsHardDeadline(t *testing.T) {
	mgr := NewBackgroundTaskManager(t.TempDir())
	// Inject a fake task with StartedAt 30 minutes ago, status="running".
	task := &BackgroundTask{
		ID:        "synthetic-1",
		Type:      "shell",
		Status:    "running",
		StartedAt: time.Now().Add(-30 * time.Minute),
	}
	mgr.mu.Lock()
	mgr.tasks[task.ID] = task
	mgr.mu.Unlock()

	wd := NewBackgroundTaskWatchdog(mgr, BackgroundTaskWatchdogConfig{
		HardDeadline: 10 * time.Minute,
	})
	flagged := wd.Scan()
	if len(flagged) != 1 || flagged[0] != task.ID {
		t.Fatalf("expected to flag %s, got %v", task.ID, flagged)
	}
	task.mu.RLock()
	defer task.mu.RUnlock()
	if task.Error == "" {
		t.Fatal("expected diagnostic recorded in task.Error")
	}
}

// TestBackgroundTaskWatchdog_DoesNotFlagFreshTask (TK-04): tasks within
// the deadline must not be flagged.
func TestBackgroundTaskWatchdog_DoesNotFlagFreshTask(t *testing.T) {
	mgr := NewBackgroundTaskManager(t.TempDir())
	task := &BackgroundTask{
		ID:        "fresh",
		Type:      "shell",
		Status:    "running",
		StartedAt: time.Now().Add(-1 * time.Minute),
	}
	mgr.mu.Lock()
	mgr.tasks[task.ID] = task
	mgr.mu.Unlock()

	wd := NewBackgroundTaskWatchdog(mgr, BackgroundTaskWatchdogConfig{
		HardDeadline: 10 * time.Minute,
	})
	flagged := wd.Scan()
	if len(flagged) != 0 {
		t.Fatalf("expected no flagged tasks, got %v", flagged)
	}
}

// TestBackgroundTaskWatchdog_IdleStdout (TK-04): a running task whose
// output file is older than IdleTimeout must be flagged.
func TestBackgroundTaskWatchdog_IdleStdout(t *testing.T) {
	dir := t.TempDir()
	mgr := NewBackgroundTaskManager(dir)
	outputPath := filepath.Join(dir, "out.log")
	if f, err := openOrCreateForTest(outputPath); err == nil {
		_ = f.Close()
	}
	old := time.Now().Add(-30 * time.Minute)
	if err := setMtimeForTest(outputPath, old); err != nil {
		t.Fatalf("set mtime: %v", err)
	}

	task := &BackgroundTask{
		ID:         "idle",
		Type:       "shell",
		Status:     "running",
		StartedAt:  time.Now().Add(-1 * time.Minute), // not over hard deadline
		OutputPath: outputPath,
	}
	mgr.mu.Lock()
	mgr.tasks[task.ID] = task
	mgr.mu.Unlock()

	wd := NewBackgroundTaskWatchdog(mgr, BackgroundTaskWatchdogConfig{
		IdleTimeout: 10 * time.Minute,
	})
	flagged := wd.Scan()
	if len(flagged) != 1 {
		t.Fatalf("expected idle task flagged, got %v", flagged)
	}
}

// TestBackgroundTaskWatchdog_StartStop_IsClean ensures the goroutine
// shuts down within a reasonable time.
func TestBackgroundTaskWatchdog_StartStop_IsClean(t *testing.T) {
	mgr := NewBackgroundTaskManager(t.TempDir())
	wd := NewBackgroundTaskWatchdog(mgr, BackgroundTaskWatchdogConfig{
		HardDeadline: time.Hour,
		PollInterval: 50 * time.Millisecond,
	})
	wd.Start()
	time.Sleep(60 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		wd.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not stop within 2s")
	}
}
