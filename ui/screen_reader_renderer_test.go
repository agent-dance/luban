package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
)

type screenReaderTestBuffer struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	updated chan struct{}
}

func newScreenReaderTestBuffer() *screenReaderTestBuffer {
	return &screenReaderTestBuffer{updated: make(chan struct{}, 1)}
}

func (b *screenReaderTestBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	n, err := b.buffer.Write(value)
	b.mu.Unlock()
	select {
	case b.updated <- struct{}{}:
	default:
	}
	return n, err
}

func (b *screenReaderTestBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func waitForScreenReaderOutput(t *testing.T, output *screenReaderTestBuffer, want string) string {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		if text := output.String(); strings.Contains(text, want) {
			return text
		}
		select {
		case <-output.updated:
		case <-timer.C:
			t.Fatalf("screen-reader output did not contain %q:\n%s", want, output.String())
		}
	}
}

func TestScreenReaderRendererIsLinearAndNamesCriticalToolAndHookState(t *testing.T) {
	var output bytes.Buffer
	renderer := NewScreenReaderRenderer(&output, strings.NewReader(""))
	ctx := ToolEventContext{SessionID: "session", TurnID: "session:turn-3", WorkUnitID: "verify", ActorID: "agent-7"}
	renderer.RenderToolCall(ctx, types.ToolUseBlock{ID: "tool-9", Name: "Bash", Input: map[string]any{"command": "go test ./..."}})
	renderer.RenderToolResult(ctx, types.ToolResultBlock{ToolUseID: "tool-9", Content: "line one\nline two", Outcome: types.ToolOutcomePartial})
	renderer.RenderHookSummary(ctx, HookSummary{ExecutionID: "hook:session:turn-3:PostSampling", Name: "PostSampling", Status: "blocked", Summary: "policy denied"})
	beforeSpinner := output.Len()
	stop := renderer.SpinnerStart("Bash")
	stop()

	text := output.String()
	for _, want := range []string{"Tool started: Bash", "tool-9", "session:turn-3", "verify", "agent-7", "go test ./...", "Outcome: partial", "line one\nline two", "Hook finished: PostSampling", "Status: blocked", "policy denied"} {
		if !strings.Contains(text, want) {
			t.Fatalf("linear output omitted %q:\n%s", want, text)
		}
	}
	if output.Len() != beforeSpinner {
		t.Fatalf("static spinner changed append-only output: %q", output.String()[beforeSpinner:])
	}
	if strings.ContainsAny(text, "\x1b\r") {
		t.Fatalf("screen reader output contains cursor/control sequence: %q", text)
	}
}

func TestScreenReaderStructuredToolResultDoesNotInferMissingOutcome(t *testing.T) {
	var output bytes.Buffer
	renderer := NewScreenReaderRenderer(&output, strings.NewReader(""))
	renderer.RenderToolResult(ToolEventContext{}, types.ToolResultBlock{
		ToolUseID: "tool-missing-outcome", IsError: true, Content: `{"status":"failed"}`,
	})
	text := output.String()
	if !strings.Contains(text, "Outcome: unknown") || strings.Contains(text, "Outcome: failed") {
		t.Fatalf("screen reader inferred outcome from IsError/payload: %s", text)
	}
}

func TestScreenReaderDecisionReadsFullPlanAndEmitsSubmitReceipt(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	output := newScreenReaderTestBuffer()
	renderer := NewScreenReaderRenderer(output, reader)
	request := permissions.PromptRequest{
		DecisionID: "plan-1", ExecutionSessionID: "agent-session-7", Kind: permissions.PromptKindPlan, ActorID: "agent-planner", ActorType: "planner", WorkUnitID: "work-plan",
		Action: "Execute plan", Target: "workspace", Impact: "modify files", RiskLevel: 3, RiskReason: "writes code", RuleSource: "plan gate", ApprovalScope: "this plan only",
		Body: "step one\nstep two", ReviewDetails: []string{"Allowed prompt: run tests", "Allowed prompt: edit files"}, PostMode: "acceptEdits",
		Choices: []string{"execute", "stay_in_plan"},
	}
	responses := make(chan permissions.PromptResponse, 1)
	go func() { responses <- renderer.DecisionRequest(context.Background(), request) }()
	waitForScreenReaderOutput(t, output, "Decision choice: ")
	if _, err := io.WriteString(writer, "decision plan-1 2\n"); err != nil {
		t.Fatal(err)
	}
	response := <-responses
	if response.DecisionID != request.DecisionID || response.Outcome != permissions.PromptOutcomeRejected || response.Choice != "stay_in_plan" {
		t.Fatalf("decision response = %#v", response)
	}
	text := output.String()
	for _, want := range []string{"Decision required", "agent-planner", "work-plan", "Execution session: agent-session-7", "step one\nstep two", "Allowed prompt: run tests", "acceptEdits", "Choice 1: Execute (execute)", "Choice 2: Stay in Plan (stay_in_plan)", "Decision receipt", "Outcome: rejected", "Choice: Stay in Plan"} {
		if !strings.Contains(text, want) {
			t.Fatalf("decision output omitted %q:\n%s", want, text)
		}
	}
	if strings.ContainsAny(text, "\x1b\r") {
		t.Fatalf("decision output contains cursor/control sequence: %q", text)
	}
}

func TestScreenReaderDecisionDistinguishesTimeoutReceipt(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	var output bytes.Buffer
	renderer := NewScreenReaderRenderer(&output, reader)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	response := renderer.DecisionRequest(ctx, permissions.PromptRequest{DecisionID: "timeout", Choices: []string{"allow_once", "reject"}})
	if response.Outcome != permissions.PromptOutcomeTimedOut {
		t.Fatalf("decision outcome = %q, want timed_out", response.Outcome)
	}
	if !strings.Contains(output.String(), "Outcome: timed out") {
		t.Fatalf("timeout receipt missing: %s", output.String())
	}
}

func TestScreenReaderRendererVisualizesUntrustedTerminalControls(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	output := newScreenReaderTestBuffer()
	renderer := NewScreenReaderRenderer(output, reader)
	renderer.Info("first\nsecond\r\x1b[2J\x00\t\u0085\u009b31m")
	renderer.RenderToolResult(ToolEventContext{ActorID: "agent\x07"}, types.ToolResultBlock{
		ToolUseID: "tool\x1b]0;owned\x07", Content: "evidence\r\nkept", Outcome: types.ToolOutcomeFailed,
	})
	responses := make(chan permissions.PromptResponse, 1)
	go func() {
		responses <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "decision\x1b[31m", ActorID: "actor\u009b", Body: "review\nbody\rhidden",
			Choices: []string{"execute", "stay_in_plan"},
		})
	}()
	waitForScreenReaderOutput(t, output, "Decision choice: ")
	if _, err := io.WriteString(writer, "decision decision\x1b[31m 2\n"); err != nil {
		t.Fatal(err)
	}
	response := <-responses
	if response.Outcome != permissions.PromptOutcomeRejected {
		t.Fatalf("decision response = %#v", response)
	}
	text := output.String()
	for _, want := range []string{"first\nsecond\\r\\x1b[2J\\x00\\t\\u0085\\u009b31m", "agent\\x07", "tool\\x1b]0;owned\\x07", "evidence\\r\nkept", "decision\\x1b[31m", "actor\\u009b", "body\\rhidden"} {
		if !strings.Contains(text, want) {
			t.Fatalf("sanitized output omitted %q:\n%s", want, text)
		}
	}
	for _, char := range text {
		if char != '\n' && (char < 0x20 || char == 0x7f || (char >= 0x80 && char <= 0x9f)) {
			t.Fatalf("output retained terminal control U+%04X: %q", char, text)
		}
	}
}

func TestScreenReaderDecisionPreemptsAndResumesCommandInput(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	output := newScreenReaderTestBuffer()
	renderer := NewScreenReaderRenderer(output, reader)

	commandResult := make(chan string, 1)
	commandErr := make(chan error, 1)
	go func() {
		command, err := renderer.ReadCommand(context.Background())
		if err != nil {
			commandErr <- err
			return
		}
		commandResult <- command
	}()
	waitForScreenReaderOutput(t, output, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyScreenReaderInput))

	decisionResult := make(chan permissions.PromptResponse, 1)
	go func() {
		decisionResult <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "background-approval", SessionID: "session", ActorID: "agent",
			Choices: []string{"allow_once", "reject"},
		})
	}()
	waitForScreenReaderOutput(t, output, "Decision choice: ")
	ownershipText := output.String()
	suspendedAt := strings.Index(ownershipText, "Command input suspended while a decision requires attention.")
	decisionAt := strings.Index(ownershipText, "Decision required.")
	if suspendedAt < 0 || decisionAt < suspendedAt {
		t.Fatalf("decision became visible before it owned input:\n%s", ownershipText)
	}
	if _, err := io.WriteString(writer, "decision background-approval not-a-choice\n"); err != nil {
		t.Fatal(err)
	}
	waitForScreenReaderOutput(t, output, "Invalid decision choice.")
	select {
	case command := <-commandResult:
		t.Fatalf("invalid decision line leaked into command input: %q", command)
	case err := <-commandErr:
		t.Fatalf("command failed while decision retained input ownership: %v", err)
	default:
	}
	if _, err := io.WriteString(writer, "decision background-approval 1\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-decisionResult:
		if response.Outcome != permissions.PromptOutcomeApproved || response.Choice != "allow_once" {
			t.Fatalf("decision response = %#v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("decision did not receive the owned input line")
	}
	waitForScreenReaderOutput(t, output, "Command input resumed.")
	select {
	case command := <-commandResult:
		t.Fatalf("decision line leaked into command input: %q", command)
	case err := <-commandErr:
		t.Fatalf("command failed while suspended: %v", err)
	default:
	}
	if _, err := io.WriteString(writer, "status\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case command := <-commandResult:
		if command != "status" {
			t.Fatalf("resumed command = %q", command)
		}
	case err := <-commandErr:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("resumed command did not receive its input line")
	}
	text := output.String()
	receipt := strings.Index(text, "Decision receipt.")
	resumed := strings.Index(text, "Command input resumed.")
	if receipt < 0 || resumed < receipt {
		t.Fatalf("command resumed before decision receipt:\n%s", text)
	}
}

func TestScreenReaderDecisionDoesNotConsumePreDecisionCommandLine(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	output := newScreenReaderTestBuffer()
	renderer := NewScreenReaderRenderer(output, reader)

	prewriteDone := make(chan error, 1)
	go func() {
		_, err := io.WriteString(writer, "1\n")
		prewriteDone <- err
	}()
	select {
	case err := <-prewriteDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pre-decision command line was not accepted by the input reader")
	}

	responses := make(chan permissions.PromptResponse, 1)
	go func() {
		responses <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "later-decision", Choices: []string{"allow_once", "reject"},
		})
	}()
	waitForScreenReaderOutput(t, output, "Decision choice: ")
	select {
	case response := <-responses:
		t.Fatalf("pre-decision command line was consumed as approval: %#v", response)
	default:
	}
	if _, err := io.WriteString(writer, "decision later-decision 2\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-responses:
		if response.Outcome != permissions.PromptOutcomeRejected || response.Choice != "reject" {
			t.Fatalf("decision response = %#v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("decision did not receive the post-prompt line")
	}
	command, err := renderer.ReadCommand(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if command != "1" {
		t.Fatalf("reserved pre-decision command = %q, want 1", command)
	}
}

func TestScreenReaderCommandLeaseConsumesExactlyOneLine(t *testing.T) {
	renderer := NewScreenReaderRenderer(io.Discard, strings.NewReader("one\ntwo\n"))
	defer renderer.Close()
	for _, want := range []string{"one", "two"} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		got, err := renderer.ReadCommand(ctx)
		cancel()
		if err != nil {
			t.Fatalf("ReadCommand(%q): %v", want, err)
		}
		if got != want {
			t.Fatalf("ReadCommand = %q, want %q", got, want)
		}
	}
}

func TestScreenReaderCancellationRetriesUntilReadLoopStops(t *testing.T) {
	stop := make(chan struct{})
	done := make(chan struct{})
	var calls atomic.Int32
	workerDone := make(chan struct{})
	go func() {
		cancelScreenReaderReadUntilDone(stop, done, func() {
			if calls.Add(1) == 3 {
				close(done)
			}
		})
		close(workerDone)
	}()
	close(stop)
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("cancellation worker did not wait for the read loop")
	}
	if got := calls.Load(); got < 3 {
		t.Fatalf("cancel attempts = %d, want retries until read completion", got)
	}
}

func TestScreenReaderDecisionPreemptsCommandAfterCommandLineDelivery(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newScreenReaderTestBuffer()
	renderer := NewScreenReaderRenderer(output, reader)
	defer renderer.Close()
	command, err := renderer.acquireInput(context.Background(), screenReaderCommandInput, "Input: ", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, "already-delivered\n"); err != nil {
		t.Fatal(err)
	}
	if got, err := command.readLine(context.Background()); err != nil || got != "already-delivered" {
		t.Fatalf("command line = %q, err=%v", got, err)
	}

	decisionResult := make(chan permissions.PromptResponse, 1)
	go func() {
		decisionResult <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "after-command", Choices: []string{"allow_once", "reject"},
		})
	}()
	waitForScreenReaderOutput(t, output, "Decision choice: ")
	if _, err := io.WriteString(writer, "decision after-command 1\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-decisionResult:
		if response.Outcome != permissions.PromptOutcomeApproved {
			t.Fatalf("decision response = %#v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("decision inherited delivered state from previous command lease")
	}
	command.release()
}

func TestScreenReaderValidDecisionDoesNotConsumeFollowingCommandTypeahead(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newScreenReaderTestBuffer()
	renderer := NewScreenReaderRenderer(output, reader)
	defer renderer.Close()
	decisionResult := make(chan permissions.PromptResponse, 1)
	go func() {
		decisionResult <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "typeahead", Choices: []string{"allow_once", "reject"},
		})
	}()
	waitForScreenReaderOutput(t, output, "Decision choice: ")
	if _, err := io.WriteString(writer, "decision typeahead 1\nstatus\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-decisionResult:
		if response.Outcome != permissions.PromptOutcomeApproved {
			t.Fatalf("decision response = %#v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("decision did not complete")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	command, err := renderer.ReadCommand(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if command != "status" {
		t.Fatalf("post-decision typeahead = %q, want status", command)
	}
}

func TestScreenReaderDecisionNeverClaimsMatchingLineBufferedBeforeLeaseActivation(t *testing.T) {
	reader, writer := io.Pipe()
	renderer := NewScreenReaderRenderer(io.Discard, reader)
	if _, err := io.WriteString(writer, "decision buffered 1\nstatus\n"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	response := renderer.DecisionRequest(ctx, permissions.PromptRequest{
		DecisionID: "buffered", Choices: []string{"allow_once", "reject"},
	})
	if response.Outcome == permissions.PromptOutcomeApproved {
		t.Fatalf("buffered decision response = %#v", response)
	}
	commandCtx, commandCancel := context.WithTimeout(context.Background(), time.Second)
	defer commandCancel()
	command, err := renderer.ReadCommand(commandCtx)
	if err != nil {
		t.Fatalf("read buffered command: %v", err)
	}
	if command != "decision buffered 1" {
		t.Fatalf("first buffered command = %q, want matching pre-decision line", command)
	}
	command, err = renderer.ReadCommand(commandCtx)
	if err != nil {
		t.Fatalf("read following buffered command: %v", err)
	}
	if command != "status" {
		t.Fatalf("following buffered command = %q, want status", command)
	}
	_ = writer.Close()
	_ = reader.Close()
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestScreenReaderRendererCloseStopsRealFileInput(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	renderer := NewScreenReaderRenderer(io.Discard, reader)
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-renderer.scannerDone:
	case <-time.After(time.Second):
		t.Fatal("real-file input goroutine survived renderer close")
	}
}

func TestScreenReaderApprovalFailsClosedWhenAuditPersistenceFails(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	output := newScreenReaderTestBuffer()
	renderer := NewScreenReaderRenderer(output, reader)
	var recorded ScreenReaderDecisionRecord
	renderer.SetDecisionRecorder(func(record ScreenReaderDecisionRecord) error {
		recorded = record
		return errors.New("disk\x1b[2J unavailable")
	})
	request := permissions.PromptRequest{
		DecisionID: "approval", SessionID: "session", Input: map[string]any{"nested": []any{"before"}},
		Choices: []string{"execute", "stay_in_plan"},
	}
	responses := make(chan permissions.PromptResponse, 1)
	go func() { responses <- renderer.DecisionRequest(context.Background(), request) }()
	waitForScreenReaderOutput(t, output, "Decision choice: ")
	if _, err := io.WriteString(writer, "decision approval 1\n"); err != nil {
		t.Fatal(err)
	}
	response := <-responses
	request.Input["nested"].([]any)[0] = "after"
	if response.Outcome != permissions.PromptOutcomeRejected || response.Decision != permissions.DecisionDeny || response.Choice != "" {
		t.Fatalf("audit failure did not fail closed: %#v", response)
	}
	if recorded.Response.Outcome != permissions.PromptOutcomeApproved || recorded.Prompt.Input["nested"].([]any)[0] != "before" {
		t.Fatalf("recorder did not receive immutable attempted approval: %+v", recorded)
	}
	text := output.String()
	for _, want := range []string{"audit persistence failed", "disk\\x1b[2J unavailable", "approval blocked", "Outcome: rejected", "Choice: none"} {
		if !strings.Contains(text, want) {
			t.Fatalf("fail-closed output omitted %q:\n%s", want, text)
		}
	}
	if strings.ContainsRune(text, '\x1b') {
		t.Fatalf("recorder error injected terminal control: %q", text)
	}
}
