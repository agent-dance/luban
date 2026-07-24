//go:build linux

package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const suspendOrderingHelperEnv = "GO_TUI_SUSPEND_ORDERING_HELPER"

func TestSendProcessSuspendTargetsCallingThread(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	wantThreadID := unix.Gettid()

	originalSuspendThread := linuxSuspendThread
	defer func() { linuxSuspendThread = originalSuspendThread }()
	var gotProcessID, gotThreadID int
	var gotSignal syscall.Signal
	linuxSuspendThread = func(processID, threadID int, signal syscall.Signal) error {
		gotProcessID = processID
		gotThreadID = threadID
		gotSignal = signal
		return nil
	}

	if err := sendProcessSuspend(); err != nil {
		t.Fatal(err)
	}
	if gotProcessID != os.Getpid() {
		t.Fatalf("target process = %d, want %d", gotProcessID, os.Getpid())
	}
	if gotThreadID != wantThreadID {
		t.Fatalf("target thread = %d, want current thread %d", gotThreadID, wantThreadID)
	}
	if gotSignal != syscall.SIGTSTP {
		t.Fatalf("signal = %v, want SIGTSTP", gotSignal)
	}
}

func TestSendProcessSuspendDoesNotReturnBeforeSIGCONT(t *testing.T) {
	if helperPath := os.Getenv(suspendOrderingHelperEnv); helperPath != "" {
		runSuspendOrderingHelper(helperPath)
		return
	}

	eventsPath := filepath.Join(t.TempDir(), "events")
	if err := os.WriteFile(eventsPath, []byte{0}, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestSendProcessSuspendDoesNotReturnBeforeSIGCONT$")
	command.Env = append(os.Environ(), suspendOrderingHelperEnv+"="+eventsPath)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	finished := false
	defer func() {
		if finished {
			return
		}
		_ = command.Process.Signal(syscall.SIGCONT)
		_ = command.Process.Kill()
		_ = command.Wait()
	}()

	waitForFileByte(t, eventsPath, 1)
	var status syscall.WaitStatus
	if _, err := syscall.Wait4(command.Process.Pid, &status, syscall.WUNTRACED, nil); err != nil {
		t.Fatal(err)
	}
	if !status.Stopped() || status.StopSignal() != syscall.SIGTSTP {
		t.Fatalf("helper wait status = %v, want stopped by SIGTSTP", status)
	}
	contents, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := contents[0]; got != 1 {
		t.Fatalf("sendProcessSuspend returned before SIGCONT; shared byte = %d", got)
	}

	if err := command.Process.Signal(syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	finished = true
	waitForFileByte(t, eventsPath, 2)
}

func runSuspendOrderingHelper(eventsPath string) {
	file, err := os.OpenFile(eventsPath, os.O_RDWR, 0)
	if err != nil {
		os.Exit(2)
	}
	defer file.Close()
	shared, err := syscall.Mmap(int(file.Fd()), 0, 1, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		os.Exit(3)
	}
	defer syscall.Munmap(shared)
	shared[0] = 1
	if err := sendProcessSuspend(); err != nil {
		os.Exit(4)
	}
	shared[0] = 2
}

func waitForFileByte(t *testing.T, path string, want byte) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(path)
		if err == nil && len(contents) == 1 && contents[0] == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	contents, err := os.ReadFile(path)
	t.Fatalf("timed out waiting for byte %d in %s: contents=%v err=%v", want, path, contents, err)
}
