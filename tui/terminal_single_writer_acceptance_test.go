package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	toolruntime "github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
	gtui "github.com/grindlemire/go-tui"
)

// TestFullscreenConcurrentFailuresUseOnlyStructuredBackBuffer is the P0
// terminal single-writer acceptance test. Background producers may enqueue
// structured state changes, but they must never write around the fullscreen
// terminal owner or borrow the conversation composer as an error surface.
func TestFullscreenConcurrentFailuresUseOnlyStructuredBackBuffer(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session-current")
	state.SessionEpoch.Set(9)
	state.ContextGeneration.Set(4)
	state.ContextGenerationPersisted.Set(true)
	root := NewRootComponentWithAdmission(state, func(string) bool { return false }, nil)
	root.input.SetText("COMPOSER_SENTINEL")
	root.input.SelectAll()
	root.input.Focus()

	current := ui.ToolEventContext{
		SessionID: "session-current", SessionEpoch: 9, ContextGeneration: 4, ContextGenerationPersisted: true,
		TurnID: "turn-current", ActorID: "assistant", WorkUnitID: "foreground",
	}
	const denialCount = 6
	for index := 0; index < denialCount; index++ {
		call := types.ToolUseBlock{ID: "bash-" + itoa(index), Name: "Bash", Input: map[string]any{"command": "dangerous"}}
		if err := state.ApplyToolCall(toolObservationContext(current, OutcomeRunning), call); err != nil {
			t.Fatal(err)
		}
	}

	// Twelve current failures plus three deliberately stale failures are all
	// produced concurrently. The lossless queue is drained by one deterministic
	// owner below, modeling the fullscreen app's back-buffer commit boundary.
	const currentEventCount = 12
	const staleEventCount = 3
	const totalEventCount = currentEventCount + staleEventCount
	queued := make(chan func(), totalEventCount)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool {
		queued <- fn
		return true
	}}

	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		t.Fatal(err)
	}
	originalStdout, originalStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutWrite, stderrWrite
	t.Cleanup(func() {
		os.Stdout, os.Stderr = originalStdout, originalStderr
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		_ = stderrRead.Close()
		_ = stderrWrite.Close()
	})

	var ready sync.WaitGroup
	var workers sync.WaitGroup
	ready.Add(totalEventCount)
	workers.Add(totalEventCount)
	start := make(chan struct{})
	launch := func(deliver func()) {
		go func() {
			defer workers.Done()
			ready.Done()
			<-start
			deliver()
		}()
	}
	for index := 0; index < denialCount; index++ {
		index := index
		launch(func() {
			renderer.RenderToolResult(current, types.ToolResultBlock{
				ToolUseID: "bash-" + itoa(index), Content: "current denial " + itoa(index),
				IsError: true, Outcome: types.ToolOutcomeDenied,
			})
		})
	}
	for index := denialCount; index < currentEventCount; index++ {
		index := index
		launch(func() {
			renderer.RuntimeErrorEvent(current, "runtime-"+itoa(index), "current runtime failure", nil, nil)
		})
	}

	staleSession := current
	staleSession.SessionID = "session-stale"
	launch(func() {
		renderer.RenderToolResult(staleSession, types.ToolResultBlock{
			ToolUseID: "bash-0", Content: "STALE_SESSION_SECRET", IsError: true, Outcome: types.ToolOutcomeDenied,
		})
	})
	staleEpoch := current
	staleEpoch.SessionEpoch--
	launch(func() {
		renderer.RuntimeErrorEvent(staleEpoch, "runtime-stale-epoch", "STALE_EPOCH_SECRET", nil, nil)
	})
	staleGeneration := current
	staleGeneration.ContextGeneration--
	launch(func() {
		renderer.RuntimeErrorEvent(staleGeneration, "runtime-stale-generation", "STALE_GENERATION_SECRET", nil, nil)
	})

	ready.Wait()
	close(start)
	workers.Wait()
	for index := 0; index < totalEventCount; index++ {
		(<-queued)()
	}

	buffer := gtui.NewBuffer(120, 60)
	root.renderAtSize(nil, 120, 60).Render(buffer, 120, 60)
	frame := buffer.String()

	os.Stdout, os.Stderr = originalStdout, originalStderr
	if err := stdoutWrite.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stderrWrite.Close(); err != nil {
		t.Fatal(err)
	}
	stdoutBytes, err := io.ReadAll(stdoutRead)
	if err != nil {
		t.Fatal(err)
	}
	stderrBytes, err := io.ReadAll(stderrRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(stdoutBytes) != 0 || len(stderrBytes) != 0 {
		t.Fatalf("fullscreen failure producers bypassed terminal owner: stdout=%q stderr=%q", stdoutBytes, stderrBytes)
	}

	if got := root.input.Text(); got != "COMPOSER_SENTINEL" {
		t.Fatalf("failure projection mutated composer: %q", got)
	}
	if got := root.input.SelectedText(); got != "COMPOSER_SENTINEL" || !root.input.IsFocused() {
		t.Fatalf("failure projection changed composer selection/focus: selected=%q focused=%v", got, root.input.IsFocused())
	}
	if !strings.Contains(frame, "COMPOSER_SENTINEL") {
		t.Fatalf("composer missing from fullscreen back-buffer:\n%s", frame)
	}
	for _, stale := range []string{"STALE_SESSION_SECRET", "STALE_EPOCH_SECRET", "STALE_GENERATION_SECRET"} {
		if strings.Contains(frame, stale) {
			t.Fatalf("stale event polluted fullscreen back-buffer with %q", stale)
		}
	}
	if got := state.Observations.Snapshot(); len(got) != currentEventCount {
		t.Fatalf("structured observation count = %d, want %d; stale identity crossed fence: %+v", len(got), currentEventCount, got)
	}
	evidence, err := state.ObservationEvidence(toolObservationID(current.SessionID, "bash-0"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(evidence), "STALE_SESSION_SECRET") {
		t.Fatalf("stale-session result modified current evidence: %s", evidence)
	}
}

type fullscreenCleanupFaultBridge struct{ cause error }

func (b fullscreenCleanupFaultBridge) Lookup(name string) (toolruntime.WorktreeHook, bool) {
	if name == "WorktreeRemove" {
		return toolruntime.WorktreeHook{Name: name, Command: "fault-injection"}, true
	}
	return toolruntime.WorktreeHook{}, false
}

func (b fullscreenCleanupFaultBridge) Run(context.Context, string, map[string]any) error {
	return b.cause
}

func (b fullscreenCleanupFaultBridge) RunWithResult(context.Context, string, map[string]any) (toolruntime.WorktreeHookResult, error) {
	return toolruntime.WorktreeHookResult{}, b.cause
}

// TestFullscreenWorktreeCleanupFaultUsesStructuredPartialResult exercises the
// production cleanup branch while a fullscreen composer and back buffer are
// active. It closes the specific regression where log.Print wrote the cleanup
// cause over the input row instead of returning an attributed tool result.
func TestFullscreenWorktreeCleanupFaultUsesStructuredPartialResult(t *testing.T) {
	appState := NewAppState()
	appState.SessionID.Set("session-worktree-cleanup")
	appState.SessionEpoch.Set(3)
	root := NewRootComponentWithAdmission(appState, func(string) bool { return false }, nil)
	root.input.SetText("WORKTREE_COMPOSER_SENTINEL")
	root.input.SelectAll()
	root.input.Focus()

	originalDir := t.TempDir()
	worktreePath := t.TempDir()
	worktreeState := &toolruntime.WorktreeState{
		SessionID: "session-worktree-cleanup", Active: true, Path: worktreePath,
		OriginalDir: originalDir, RepoRoot: originalDir, CurrentDir: worktreePath,
		CreatedHere: true, HookBased: true,
	}
	rawCause := errors.New("fullscreen-private-cleanup-cause")
	exit := &toolruntime.ExitWorktreeTool{
		State: worktreeState, Manager: toolruntime.NewWorktreeManager(), Runtime: toolruntime.NewWorktreeRuntime(worktreePath, nil),
		SessionID: func() string { return "session-worktree-cleanup" }, HookBridge: fullscreenCleanupFaultBridge{cause: rawCause},
	}
	toolContext := ui.ToolEventContext{
		SessionID: "session-worktree-cleanup", SessionEpoch: 3, TurnID: "turn-worktree-cleanup",
		ActorID: "assistant", ActorType: "main", WorkUnitID: "foreground",
	}
	call := types.ToolUseBlock{ID: "exit-worktree-cleanup", Name: "ExitWorktree", Input: map[string]any{"action": "remove", "discard_changes": true}}
	if err := appState.ApplyToolCall(toolObservationContext(toolContext, OutcomeRunning), call); err != nil {
		t.Fatal(err)
	}
	renderer := &TuiRenderer{state: appState, enqueue: func(fn func()) bool { fn(); return true }}

	var result types.ToolResult
	var executeErr error
	stdout, stderr := captureFullscreenProcessStreams(t, func() {
		result, executeErr = exit.Execute(context.Background(), call.Input)
		if executeErr == nil {
			renderer.RenderToolResult(toolContext, types.MapToolResult(exit, result, call.ID))
		}
	})
	if executeErr != nil {
		t.Fatal(executeErr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("worktree cleanup fault bypassed fullscreen owner: stdout=%q stderr=%q", stdout, stderr)
	}
	output, ok := result.Data.(toolruntime.ExitWorktreeOutput)
	if !ok || result.Outcome != types.ToolOutcomePartial || !output.CleanupIncomplete || output.CleanupIssueCount != 1 {
		t.Fatalf("cleanup fault did not produce a typed partial result: result=%#v output=%#v", result, output)
	}
	if strings.Contains(result.Content, rawCause.Error()) {
		t.Fatalf("private cleanup cause leaked into tool result: %q", result.Content)
	}
	if got := root.input.Text(); got != "WORKTREE_COMPOSER_SENTINEL" {
		t.Fatalf("cleanup fault replaced fullscreen composer: %q", got)
	}
	if got := root.input.SelectedText(); got != "WORKTREE_COMPOSER_SENTINEL" || !root.input.IsFocused() {
		t.Fatalf("cleanup fault changed composer selection/focus: selected=%q focused=%v", got, root.input.IsFocused())
	}
	observation, ok := appState.Observations.Get(toolObservationID(toolContext.SessionID, call.ID))
	if !ok || observation.Outcome != OutcomePartial {
		t.Fatalf("cleanup partial outcome was not attributed to the tool observation: %#v", observation)
	}
	buffer := gtui.NewBuffer(120, 40)
	root.renderAtSize(nil, 120, 40).Render(buffer, 120, 40)
	frame := buffer.String()
	if !strings.Contains(frame, "WORKTREE_COMPOSER_SENTINEL") || strings.Contains(frame, rawCause.Error()) {
		t.Fatalf("fullscreen back buffer is inconsistent after cleanup fault:\n%s", frame)
	}
}

func captureFullscreenProcessStreams(t *testing.T, fn func()) (string, string) {
	t.Helper()
	stdoutFile, err := os.CreateTemp(t.TempDir(), "fullscreen-stdout-*")
	if err != nil {
		t.Fatal(err)
	}
	stderrFile, err := os.CreateTemp(t.TempDir(), "fullscreen-stderr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer stdoutFile.Close()
	defer stderrFile.Close()
	originalStdout, originalStderr := os.Stdout, os.Stderr
	originalLogWriter, originalLogFlags, originalLogPrefix := log.Writer(), log.Flags(), log.Prefix()
	originalSlog := slog.Default()
	os.Stdout, os.Stderr = stdoutFile, stderrFile
	log.SetOutput(stderrFile)
	slog.SetDefault(slog.New(slog.NewTextHandler(stderrFile, nil)))
	defer func() {
		os.Stdout, os.Stderr = originalStdout, originalStderr
		log.SetOutput(originalLogWriter)
		log.SetFlags(originalLogFlags)
		log.SetPrefix(originalLogPrefix)
		slog.SetDefault(originalSlog)
	}()
	fn()
	read := func(file *os.File) string {
		if err := file.Sync(); err != nil {
			t.Fatal(err)
		}
		if _, err := file.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		var captured bytes.Buffer
		if _, err := captured.ReadFrom(file); err != nil {
			t.Fatal(err)
		}
		return captured.String()
	}
	return read(stdoutFile), read(stderrFile)
}
