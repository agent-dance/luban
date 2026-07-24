package debug

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDebugAutoInitFailureIsRetainedPrivately(t *testing.T) {
	mu.Lock()
	previousAllTopics := allTopics
	previousLogFile := logFile
	previousLastErr := lastErr
	allTopics = true
	logFile = nil
	lastErr = nil
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		allTopics = previousAllTopics
		logFile = previousLogFile
		lastErr = previousLastErr
		mu.Unlock()
	})

	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Use an explicit invalid path through Init to establish the same retained
	// failure contract without touching stderr.
	err := Init(filepath.Join(blocked, "debug.log"))
	if err == nil {
		t.Fatal("expected debug log initialization to fail")
	}
	if LastError() == nil {
		t.Fatal("debug initialization error was not retained privately")
	}
}
