package tools

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func requireBashAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash is not on PATH: %v", err)
	}
	if err := exec.Command("bash", "-c", "true").Run(); err != nil {
		t.Skipf("bash is present but not runnable: %v", err)
	}
}

func TestBashToolReturnsPlainTextOutput(t *testing.T) {
	requireBashAvailable(t)
	tool := &BashTool{CWD: t.TempDir()}

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "printf 'hello\\n'",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if result.Content != "hello" {
		t.Fatalf("unexpected bash output: %q", result.Content)
	}
}

func TestBashToolReturnsToolErrorOnCommandFailure(t *testing.T) {
	requireBashAvailable(t)
	tool := &BashTool{CWD: t.TempDir()}

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "printf 'oops\\n' >&2; exit 7",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected tool error, got success: %s", result.Content)
	}
	if !strings.Contains(result.Content, "oops") {
		t.Fatalf("expected stderr in tool error, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Exit code 7") {
		t.Fatalf("expected exit code in tool error, got: %s", result.Content)
	}
}

func TestBashToolBackgroundResultMatchesModelFacingText(t *testing.T) {
	requireBashAvailable(t)
	tmp := t.TempDir()
	tool := &BashTool{
		CWD:        tmp,
		Background: NewBackgroundTaskManager(tmp),
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"command":           "sleep 1",
		"run_in_background": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Command running in background with ID:") {
		t.Fatalf("expected background launch message, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Output is being written to:") {
		t.Fatalf("expected output path in background launch message, got: %s", result.Content)
	}
}

func TestBashToolBlocksStandaloneSleepWithoutBackground(t *testing.T) {
	tool := &BashTool{CWD: t.TempDir()}

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "sleep 5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected tool error, got success: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Blocked: standalone sleep 5.") {
		t.Fatalf("unexpected blocked-sleep message: %s", result.Content)
	}
	if !strings.Contains(result.Content, "run_in_background: true") {
		t.Fatalf("expected run_in_background guidance, got: %s", result.Content)
	}
}

func TestBashToolAllowsShortSleep(t *testing.T) {
	requireBashAvailable(t)
	tool := &BashTool{CWD: t.TempDir()}

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "sleep 1; printf done",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if result.Content != "done" {
		t.Fatalf("unexpected bash output: %q", result.Content)
	}
}
