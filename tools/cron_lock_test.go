package tools

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCronSchedulerLockAcquireRelease confirms that two SchedulerLock
// instances pointed at the same file disagree on holder until one
// releases — so loser sessions can take over.
func TestCronSchedulerLockAcquireRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lock.json")
	a := NewSchedulerLock(path)
	a.SetPID(1)
	b := NewSchedulerLock(path)
	b.SetPID(2)

	got, err := a.TryAcquire()
	if err != nil || !got {
		t.Fatalf("a.TryAcquire = %v, %v", got, err)
	}
	if !a.IsHolder() {
		t.Fatalf("a should be holder")
	}

	got, err = b.TryAcquire()
	if err != nil || got {
		t.Fatalf("b.TryAcquire should fail while a holds; got %v, %v", got, err)
	}
	if b.IsHolder() {
		t.Fatalf("b should not be holder")
	}

	a.Release()
	if a.IsHolder() {
		t.Fatalf("a should no longer hold after release")
	}

	got, err = b.TryAcquire()
	if err != nil || !got {
		t.Fatalf("b.TryAcquire after a.Release = %v, %v", got, err)
	}
	b.Release()
}

// TestCronMissedTaskNotificationFenceEscape verifies that backticks in
// user-supplied prompts don't break the surrounding markdown fence.
func TestCronMissedTaskNotificationFenceEscape(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	last := now.Add(-2 * time.Hour)
	missed := []MissedRun{{
		JobID:       "abc",
		Cron:        "0 * * * *",
		Prompt:      "```evil``` payload",
		LastFiredAt: &last,
		MissedSince: now.Add(-1 * time.Hour),
		MissedCount: 2,
	}}
	out := BuildMissedTaskNotification(missed)
	if out == "" {
		t.Fatalf("expected non-empty notification")
	}
	if !strings.Contains(out, "````\n```evil``` payload\n````") {
		t.Fatalf("prompt must be preserved inside a longer dynamic fence: %q", out)
	}
	if !strings.Contains(out, "AskUserQuestion") || !strings.Contains(out, "Do NOT execute") {
		t.Fatalf("notification must require confirmation before execution: %q", out)
	}
}

// TestCronGuardDurableTeammate covers CR-04: durable cron creation is
// rejected when CLAUDE_CODE_CRON_NON_DURABLE_TEAMMATE is set.
func TestCronGuardDurableTeammate(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CRON_NON_DURABLE_TEAMMATE", "1")
	store := newTestCronStore(t)
	if err := store.guardDurableCreation(); err == nil {
		t.Fatalf("expected guard to reject durable creation under teammate flag")
	} else if !strings.Contains(err.Error(), "errorCode=4") {
		t.Fatalf("expected errorCode=4 in message: %v", err)
	}
}

// TestNextCronRunInLocationDST verifies that schedule matching honours an
// IANA timezone (CR-07). 2:30am local in `America/Los_Angeles` should
// resolve to a wall-clock 02:30 in that zone, not UTC.
func TestNextCronRunInLocationDST(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("LoadLocation failed: %v", err)
	}
	from := time.Date(2026, 3, 7, 0, 0, 0, 0, loc) // before DST start
	next, ok := nextCronRunInLocation("30 2 * * *", from, loc)
	if !ok {
		t.Fatalf("expected schedule to fire")
	}
	// next.In(loc) should have hour=2 minute=30 in the local zone.
	got := next.In(loc)
	if got.Hour() != 2 || got.Minute() != 30 {
		t.Fatalf("expected 02:30 local; got %02d:%02d", got.Hour(), got.Minute())
	}
}

// TestEscapeMarkdownFence sanity-checks the escape helper.
func TestEscapeMarkdownFence(t *testing.T) {
	in := "before ``` mid ``` end"
	out := escapeMarkdownFence(in)
	if strings.Contains(out, "```") {
		t.Fatalf("expected no raw triple-backticks: %q", out)
	}
}
