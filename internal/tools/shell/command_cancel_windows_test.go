//go:build windows

package shell

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestWindowsCommandJobLifecycleDoesNotRejectStartedProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "cmd.exe", "/d", "/c", "ping -n 30 127.0.0.1 >nul")
	configureCommandCancellation(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := commandStarted(cmd); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("attach cancellation lifecycle: %v", err)
	}
	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("cancelled process did not terminate")
	}
	commandFinished(cmd)
}
