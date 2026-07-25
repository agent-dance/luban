package shell

import (
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

	result, err := executeApprovedBashForTest(t, tool, map[string]any{
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

	result, err := executeApprovedBashForTest(t, tool, map[string]any{
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
	output, ok := result.Data.(*BashOutput)
	if !ok || output.ExitCode != 7 {
		t.Fatalf("expected structured exit code 7, got: %#v", result.Data)
	}
}

func TestBashToolBlocksStandaloneSleepWithoutBackground(t *testing.T) {
	tool := &BashTool{CWD: t.TempDir()}

	result, err := executeApprovedBashForTest(t, tool, map[string]any{
		"command": "sleep 5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected tool error, got success: %s", result.Content)
	}
	if !strings.Contains(result.Content, "run_in_background: true") {
		t.Fatalf("expected run_in_background guidance, got: %s", result.Content)
	}
}

func TestBashToolAllowsShortSleep(t *testing.T) {
	requireBashAvailable(t)
	tool := &BashTool{CWD: t.TempDir()}

	result, err := executeApprovedBashForTest(t, tool, map[string]any{
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
