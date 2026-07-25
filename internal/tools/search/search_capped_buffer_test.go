// Package search tests capped ripgrep output.
package search

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestSearchOutcomeClassifiesPartialAndTimeout(t *testing.T) {
	if got := searchOutcomeForPartialReason(globPartialStdoutCap); got != types.ToolOutcomePartial {
		t.Fatalf("stdout-cap outcome = %q, want %q", got, types.ToolOutcomePartial)
	}
	if got := searchOutcomeForPartialReason(globPartialTimeout); got != types.ToolOutcomeTimedOut {
		t.Fatalf("timeout outcome = %q, want %q", got, types.ToolOutcomeTimedOut)
	}
	if got := searchOutcomeForPartialReason(""); got != "" {
		t.Fatalf("complete outcome = %q, want empty", got)
	}
}

func TestGrepCaptureDroppedCannotClaimFullEvidence(t *testing.T) {
	completeness := grepResultCompleteness(grepPartialStdoutCap, false, false, 0, 0, 25)
	if completeness.Source != types.ToolResultCompletenessCaptureDropped || completeness.CanRetainFullEvidence() {
		t.Fatalf("stdout-cap completeness = %+v", completeness)
	}
}

func TestCappedBuffer_DropsBeyondCap(t *testing.T) {
	var b cappedBuffer
	b.cap = 8
	if n, err := b.Write([]byte("12345")); n != 5 || err != nil {
		t.Fatalf("first write: n=%d err=%v", n, err)
	}
	if b.dropped {
		t.Fatalf("dropped flag should not be set yet")
	}
	if n, err := b.Write([]byte("67890")); n != 5 || err != nil {
		t.Fatalf("second write: n=%d err=%v", n, err)
	}
	if !b.dropped {
		t.Fatalf("dropped flag should be set after exceeding cap")
	}
	if got := b.Buffer.String(); got != "12345678" {
		t.Fatalf("buffer = %q want %q", got, "12345678")
	}
	// further writes are silently swallowed
	if _, err := b.Write([]byte("xxx")); err != nil {
		t.Fatalf("write after cap: %v", err)
	}
	if got := b.Buffer.String(); got != "12345678" {
		t.Fatalf("buffer grew past cap: %q", got)
	}
}

func TestGlobTimeoutReturnsCompletePartialLines(t *testing.T) {
	fake := writeFakeRipgrep(t, "printf 'first.txt\\nsecond.txt\\nincomplete'; exec sleep 5")
	withFakeRipgrep(t, fake)
	if _, err := locateRipgrep(); err != nil {
		t.Fatalf("prime fake ripgrep: %v", err)
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	res, _ := (&globTool{}).Execute(ctx, map[string]any{"pattern": "*.txt", "path": root})
	if res.IsError || !strings.Contains(res.Content, "first.txt") || !strings.Contains(res.Content, "second.txt") || strings.Contains(res.Content, "incomplete") {
		t.Fatalf("timeout partial mismatch: error=%v content=%q", res.IsError, res.Content)
	}
	if res.Metadata["truncated"] != "true" {
		t.Fatalf("timeout partial must be marked truncated: %v", res.Metadata)
	}
	if res.Outcome != types.ToolOutcomeTimedOut {
		t.Fatalf("timeout partial outcome = %q, want %q", res.Outcome, types.ToolOutcomeTimedOut)
	}
}

func TestGlobTimeoutWithoutOutputIsClearError(t *testing.T) {
	fake := writeFakeRipgrep(t, "exec sleep 5")
	withFakeRipgrep(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	res, _ := (&globTool{}).Execute(ctx, map[string]any{"pattern": "*.txt", "path": t.TempDir()})
	if !res.IsError || res.Content != toolRuntimeText(i18n.KeyToolSearchRipgrepTimedOut) {
		t.Fatalf("empty timeout mismatch: error=%v content=%q", res.IsError, res.Content)
	}
	if res.Outcome != types.ToolOutcomeTimedOut {
		t.Fatalf("empty timeout outcome = %q, want %q", res.Outcome, types.ToolOutcomeTimedOut)
	}
}

func TestRipgrepCappedStdoutDropsIncompleteTailAndMarksPartial(t *testing.T) {
	fake := writeFakeRipgrep(t, "printf 'first.txt\\nsecond.txt\\nincomplete'")
	withFakeRipgrep(t, fake)

	run, err := runRipgrepDetailedWithCap(context.Background(), []string{"--files"}, t.TempDir(), len("first.txt\nsecond.txt\ninco"))
	if err != nil {
		t.Fatalf("run fake ripgrep: %v", err)
	}
	if !run.Truncated {
		t.Fatalf("capped stdout must be marked truncated: %#v", run)
	}
	if len(run.Lines) != 2 || run.Lines[0] != "first.txt" || run.Lines[1] != "second.txt" {
		t.Fatalf("capped lines = %#v, want only complete lines", run.Lines)
	}
}

func TestGrepTimeoutReturnsSuccessfulPartialWithMetadata(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	mustWrite(t, first, "needle\n")
	mustWrite(t, second, "needle\n")
	sameTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(first, sameTime, sameTime); err != nil {
		t.Fatalf("chtime first: %v", err)
	}
	if err := os.Chtimes(second, sameTime, sameTime); err != nil {
		t.Fatalf("chtime second: %v", err)
	}
	ready := filepath.Join(t.TempDir(), "ready")
	fake := writeFakeRipgrep(t, "printf '%s\\n%s\\nincomplete' \"$LUBAN_FAKE_FIRST\" \"$LUBAN_FAKE_SECOND\"; : > \"$LUBAN_FAKE_READY\"; exec sleep 5")
	withFakeRipgrep(t, fake)
	t.Setenv("LUBAN_FAKE_FIRST", first)
	t.Setenv("LUBAN_FAKE_SECOND", second)
	t.Setenv("LUBAN_FAKE_READY", ready)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type execution struct {
		result types.ToolResult
		err    error
	}
	done := make(chan execution, 1)
	go func() {
		res, err := (&grepTool{}).Execute(ctx, map[string]any{"pattern": "needle", "path": root})
		done <- execution{result: res, err: err}
	}()
	// Package-wide and race runs can spend several seconds scheduling the fake
	// rg process under load. The marker still gates cancellation precisely; this
	// wider startup window only avoids mistaking scheduler delay for a timeout
	// partial-result failure.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fake rg did not report partial output ready")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	executed := <-done
	if executed.err != nil {
		t.Fatalf("grep infrastructure error: %v", executed.err)
	}
	res := executed.result
	if res.IsError || !strings.Contains(res.Content, "first.txt") || !strings.Contains(res.Content, "second.txt") || strings.Contains(res.Content, "incomplete") {
		t.Fatalf("grep timeout partial mismatch: error=%v content=%q", res.IsError, res.Content)
	}
	if res.Metadata["partial"] != "true" || res.Metadata["partial_reason"] != "timeout" || res.Metadata["timed_out"] != "true" {
		t.Fatalf("grep timeout partial metadata mismatch: %v", res.Metadata)
	}
	if res.Outcome != types.ToolOutcomeTimedOut {
		t.Fatalf("grep timeout partial outcome = %q, want %q", res.Outcome, types.ToolOutcomeTimedOut)
	}
}

func TestRipgrepEAGAINRetriesOnceSingleThreaded(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	fake := writeFakeRipgrep(t, "if [ ! -e \"$LUBAN_FAKE_STATE\" ]; then : > \"$LUBAN_FAKE_STATE\"; echo 'Resource temporarily unavailable (os error 11)' >&2; exit 2; fi; printf '%s\\n' \"$@\"")
	withFakeRipgrep(t, fake)
	t.Setenv("LUBAN_FAKE_STATE", state)
	run, err := runRipgrepDetailed(context.Background(), []string{"--files"}, t.TempDir())
	if err != nil {
		t.Fatalf("EAGAIN retry: %v", err)
	}
	lines := run.Lines
	joined := strings.Join(lines, " ")
	if !strings.Contains(joined, "-j") || !strings.Contains(joined, "1") || !strings.Contains(joined, "--files") {
		t.Fatalf("retry must add one-shot -j 1: %#v", lines)
	}
}

func TestGrepStdoutCapSurfacesPartialStateAndDropsTail(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	mustWrite(t, first, "needle\n")
	mustWrite(t, second, "needle\n")
	sameTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(first, sameTime, sameTime); err != nil {
		t.Fatalf("chtime first: %v", err)
	}
	if err := os.Chtimes(second, sameTime, sameTime); err != nil {
		t.Fatalf("chtime second: %v", err)
	}
	fake := writeFakeRipgrep(t, "printf '%s\\n%s\\nincomplete' \"$LUBAN_FAKE_FIRST\" \"$LUBAN_FAKE_SECOND\"")
	withFakeRipgrep(t, fake)
	t.Setenv("LUBAN_FAKE_FIRST", first)
	t.Setenv("LUBAN_FAKE_SECOND", second)
	info, err := os.Stat(root)
	if err != nil {
		t.Fatalf("stat root: %v", err)
	}
	capBytes := len(first) + 1 + len(second) + 1 + len("inco")
	result, err := runGrepWithRipgrepCap(context.Background(), grepRipgrepOptions{
		Pattern:        "needle",
		SearchPath:     root,
		SearchPathInfo: info,
		OutputMode:     "files_with_matches",
		Unlimited:      true,
		DisplayRoot:    root,
	}, capBytes)
	if err != nil {
		t.Fatalf("capped grep: %v", err)
	}
	if result.PartialReason != grepPartialStdoutCap {
		t.Fatalf("partial reason = %q, want %q", result.PartialReason, grepPartialStdoutCap)
	}
	if len(result.Lines) != 2 || result.Lines[0] != "first.txt" || result.Lines[1] != "second.txt" {
		t.Fatalf("capped Grep lines must contain only complete results: %#v", result.Lines)
	}
}

func writeFakeRipgrep(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell ripgrep fixture requires a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "rg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'ripgrep 99.0.0'; exit 0; fi\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write fake ripgrep: %v", err)
	}
	return path
}

func withFakeRipgrep(t *testing.T, path string) {
	t.Helper()
	t.Setenv("LUBAN_RG_PATH", path)
	resetRipgrepLocationForTest()
	t.Cleanup(func() {
		resetRipgrepLocationForTest()
	})
}
