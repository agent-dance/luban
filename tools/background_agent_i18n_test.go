package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
)

func TestBackgroundTaskTerminalCopyUsesRuntimeLanguage(t *testing.T) {
	lang := i18n.DetectOrLoadLanguage()

	cancelled := &BackgroundTask{Status: "running", done: make(chan struct{})}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	new(BackgroundTaskManager).watchShellTask(cancelled, cancelCtx, 0)
	if want := i18n.Text(lang, i18n.KeyToolBackgroundTaskCanceled); cancelled.Error != want {
		t.Fatalf("cancelled task error = %q, want %q", cancelled.Error, want)
	}

	timedOut := &BackgroundTask{Status: "running", done: make(chan struct{})}
	deadlineCtx, deadlineCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer deadlineCancel()
	new(BackgroundTaskManager).watchShellTask(timedOut, deadlineCtx, 0)
	if want := i18n.Text(lang, i18n.KeyToolBackgroundCommandTimedOut); timedOut.Error != want {
		t.Fatalf("timed-out task error = %q, want %q", timedOut.Error, want)
	}

	explicitTimeout := &BackgroundTask{Status: "running", done: make(chan struct{})}
	new(BackgroundTaskManager).watchShellTask(explicitTimeout, context.Background(), time.Nanosecond)
	if want := i18n.Format(lang, i18n.KeyToolBackgroundCommandTimedOutAfter, time.Nanosecond); explicitTimeout.Error != want {
		t.Fatalf("explicit timeout error = %q, want %q", explicitTimeout.Error, want)
	}
	if record := explicitTimeout.recordLocked(); record.Error != "" || record.TerminalReason != backgroundTerminalReasonTimeout || record.TimeoutNanos != int64(time.Nanosecond) {
		t.Fatalf("durable timeout record froze display copy: %#v", record)
	}
}

func TestRuntimeNotificationReplaysInRequestedLanguage(t *testing.T) {
	code := 7
	notification := RuntimeNotification{Kind: "task-notification", TaskID: "task-42", Status: "failed", ExitCode: &code}
	snapshot := BackgroundTaskSnapshot{ID: "task-42", Type: "local_agent", Description: "raw-label", Status: "failed", ExitCode: &code}
	english := LocalizeRuntimeNotification(i18n.LangEN, notification, snapshot)
	chinese := LocalizeRuntimeNotification(i18n.LangZH, notification, snapshot)
	if english.Message == chinese.Message || !strings.Contains(chinese.Message, "raw-label") || !strings.Contains(chinese.Message, "task-42") {
		t.Fatalf("notification did not relocalize while preserving raw values: en=%q zh=%q", english.Message, chinese.Message)
	}
}

func TestBackgroundAgentResultFallbacksUseRuntimeLanguage(t *testing.T) {
	lang := i18n.DetectOrLoadLanguage()
	completed := AgentResultFromCompleted(agentRunSummary{Output: " \n"}, "", "")
	if len(completed.Content) != 1 {
		t.Fatalf("completed content blocks = %d, want 1", len(completed.Content))
	}
	if want := i18n.Text(lang, i18n.KeyToolBackgroundAgentEmptyOutput); completed.Content[0].Text != want {
		t.Fatalf("empty-output fallback = %q, want %q", completed.Content[0].Text, want)
	}

	failed := agentFailureToolResult(context.Background(), "agent-raw-42", "general-purpose", "", time.Now(), nil)
	if want := i18n.Text(lang, i18n.KeyToolBackgroundAgentFailed); failed.Content != want {
		t.Fatalf("default failure = %q, want %q", failed.Content, want)
	}
	if data, ok := failed.Data.(AgentError); !ok || data.AgentID != "agent-raw-42" || data.Message != failed.Content {
		t.Fatalf("default failure data = %#v", failed.Data)
	}
}

func TestAgentAbortDisplayErrorPreservesCauseAndRawDetails(t *testing.T) {
	lang := i18n.DetectOrLoadLanguage()
	cancelCause := fmt.Errorf("raw-operation-42: %w", context.Canceled)
	cancelErr := agentAbortDisplayError(context.Background(), cancelCause)
	if !errors.Is(cancelErr, context.Canceled) {
		t.Fatal("cancel display error lost context.Canceled")
	}
	if want := i18n.Format(lang, i18n.KeyToolBackgroundAgentCanceledWithCause, cancelCause); cancelErr.Error() != want {
		t.Fatalf("cancel display error = %q, want %q", cancelErr, want)
	}
	if !strings.Contains(cancelErr.Error(), "raw-operation-42") {
		t.Fatalf("cancel display error lost raw cause: %q", cancelErr)
	}

	timeoutCause := fmt.Errorf("raw-provider-17: %w", context.DeadlineExceeded)
	timeoutErr := agentAbortDisplayError(context.Background(), timeoutCause)
	if !errors.Is(timeoutErr, context.DeadlineExceeded) {
		t.Fatal("timeout display error lost context.DeadlineExceeded")
	}
	if want := i18n.Format(lang, i18n.KeyToolBackgroundAgentTimedOutWithCause, timeoutCause); timeoutErr.Error() != want {
		t.Fatalf("timeout display error = %q, want %q", timeoutErr, want)
	}
}
