package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type streamingGateProvider struct {
	read *streamingExecutorTestTool

	mu        sync.Mutex
	turnIndex int
	checked   chan error
}

func newStreamingGateProvider(read *streamingExecutorTestTool) *streamingGateProvider {
	return &streamingGateProvider{
		read:    read,
		checked: make(chan error, 1),
	}
}

func (p *streamingGateProvider) Name() string { return "streaming-gate-provider" }

func (p *streamingGateProvider) ModelID() string { return "streaming-gate-model" }

func (p *streamingGateProvider) CreateStream(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	idx := p.turnIndex
	p.turnIndex++
	p.mu.Unlock()

	if idx > 0 {
		return makeStreamChan(parityTextEvents("done")...), nil
	}

	ch := make(chan types.StreamEvent)
	go func() {
		defer close(ch)
		send := func(evt types.StreamEvent) bool {
			select {
			case <-ctx.Done():
				return false
			case ch <- evt:
				return true
			}
		}
		if !send(types.StreamEvent{Type: types.EventMessageStart}) {
			return
		}
		if !send(types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse,
			ID:   "read_1",
			Name: "Read",
		}}) {
			return
		}
		if !send(types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type:        "input_json_delta",
			PartialJSON: `{"id":"read_1"}`,
		}}) {
			return
		}
		if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}) {
			return
		}
		select {
		case got := <-p.read.started:
			p.checked <- fmt.Errorf("Read started before message_stop: %s", got)
			return
		case <-time.After(50 * time.Millisecond):
			p.checked <- nil
		}
		if !send(types.StreamEvent{Type: types.EventMessageDelta, StopReason: stopReasonForParity(types.StopReasonToolUse)}) {
			return
		}
		receipt := types.NewProviderToolCommitReceipt("test", "test", "completed", []types.ProviderToolCallCommit{{
			OutputIndex: 0, CallID: "read_1", Name: "Read", RawInput: `{"id":"read_1"}`,
		}})
		_ = send(types.StreamEvent{Type: types.EventMessageStop, ProviderCommitReceipt: receipt})
	}()
	return ch, nil
}

type streamingExecutorTestTool struct {
	name           string
	concurrentSafe bool

	started chan string

	mu       sync.Mutex
	releases map[string]chan types.ToolResult
}

func newStreamingExecutorTestTool(name string, concurrentSafe bool) *streamingExecutorTestTool {
	return &streamingExecutorTestTool{
		name:           name,
		concurrentSafe: concurrentSafe,
		started:        make(chan string, 32),
		releases:       make(map[string]chan types.ToolResult),
	}
}

func (t *streamingExecutorTestTool) Name() string { return t.name }

func (t *streamingExecutorTestTool) Description() string { return "streaming executor test tool" }

func (t *streamingExecutorTestTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}

func (t *streamingExecutorTestTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ConcurrencySafe: t.concurrentSafe}
}

func (t *streamingExecutorTestTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	id, _ := input["id"].(string)
	if id == "" {
		id = t.name
	}
	release := t.releaseFor(id)
	t.started <- id
	select {
	case <-ctx.Done():
		return types.ToolResult{}, ctx.Err()
	case result := <-release:
		return result, nil
	}
}

func (t *streamingExecutorTestTool) releaseFor(id string) chan types.ToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()
	if ch := t.releases[id]; ch != nil {
		return ch
	}
	ch := make(chan types.ToolResult, 1)
	t.releases[id] = ch
	return ch
}

func (t *streamingExecutorTestTool) release(id, content string, isError bool) {
	t.releaseFor(id) <- types.ToolResult{Content: content, IsError: isError}
}

func waitStarted(t *testing.T, tool *streamingExecutorTestTool, want string) {
	t.Helper()
	select {
	case got := <-tool.started:
		if got != want {
			t.Fatalf("%s started, want %s", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s to start", want)
	}
}

func assertNotStarted(t *testing.T, ch <-chan string, label string) {
	t.Helper()
	select {
	case got := <-ch:
		t.Fatalf("%s started unexpectedly: %s", label, got)
	case <-time.After(50 * time.Millisecond):
	}
}

func streamingTestUse(id, name string) types.ToolUseBlock {
	return types.ToolUseBlock{
		Type:  types.ContentTypeToolUse,
		ID:    id,
		Name:  name,
		Input: map[string]any{"id": id},
	}
}

func streamingRevisionDependentRun(id string) types.ToolUseBlock {
	tool := streamingTestUse(id, "Run")
	tool.Input["requires_patch_commit"] = true
	return tool
}

func newStreamingTestExecutor(ctx context.Context, reg *registry.Registry) *StreamingToolExecutor {
	return NewStreamingToolExecutor(ctx, reg, nil, nil, "session", executioncontract.ToolExecutionContext{
		Messages:  []types.Message{types.UserMessage("run tools")},
		SessionID: "session",
	})
}

func TestStreamingToolExecutorConcurrentSafeToolsStartTogetherAndDrainInOrder(t *testing.T) {
	ctx := context.Background()
	read := newStreamingExecutorTestTool("Read", true)
	grep := newStreamingExecutorTestTool("Grep", true)
	reg := registry.New()
	reg.Register(read)
	reg.Register(grep)
	executor := newStreamingTestExecutor(ctx, reg)
	assistant := types.Message{Role: types.RoleAssistant}

	executor.AddTool(streamingTestUse("read_1", "Read"), assistant)
	executor.AddTool(streamingTestUse("grep_1", "Grep"), assistant)

	waitStarted(t, read, "read_1")
	waitStarted(t, grep, "grep_1")

	grep.release("grep_1", "grep done first", false)
	read.release("read_1", "read done second", false)

	results, events, err := executor.RemainingResults(ctx)
	if err != nil {
		t.Fatalf("RemainingResults: %v", err)
	}
	if len(results.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results.Results))
	}
	if results.Results[0].ToolUseID != "read_1" || results.Results[1].ToolUseID != "grep_1" {
		t.Fatalf("result order = [%s %s], want [read_1 grep_1]", results.Results[0].ToolUseID, results.Results[1].ToolUseID)
	}
	if len(events) != 2 || events[0].Type != streamingToolEventResult || events[1].Type != streamingToolEventResult {
		t.Fatalf("events = %+v, want two result events", events)
	}
	if metrics := results.Metrics; metrics.PhysicalChildOperations != 2 || metrics.PeakFanout != 2 || metrics.BatchCount != 1 || metrics.ErrorCount != 0 {
		t.Fatalf("streaming metrics = %+v, want physical=2 fanout=2 batches=1 errors=0", metrics)
	}
}

func TestStreamingToolExecutorReturnsInfrastructureErrorAfterPublishingResult(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Deadline", concurrent: true, execute: func(context.Context, map[string]any) (types.ToolResult, error) {
		return types.ToolResult{}, context.DeadlineExceeded
	}})
	executor := newStreamingTestExecutor(context.Background(), reg)
	executor.AddTool(streamingTestUse("deadline-stream", "Deadline"), types.Message{Role: types.RoleAssistant})
	results, events, err := executor.RemainingResults(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RemainingResults error = %v, want deadline exceeded", err)
	}
	if len(results.Results) != 1 || results.Results[0].Outcome != types.ToolOutcomeTimedOut || len(events) != 1 || events[0].Result == nil {
		t.Fatalf("streaming infra terminal evidence missing: results=%#v events=%#v", results, events)
	}
	if results.Metrics.PhysicalChildOperations != 1 || results.Metrics.PeakFanout != 1 || results.Metrics.ErrorCount != 1 {
		t.Fatalf("streaming failure metrics = %+v", results.Metrics)
	}
}

func TestStreamingRevisionFusionPreservesDependencyAndMetrics(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	var mutationExecuted atomic.Int32
	var verificationExecuted atomic.Int32
	var verified atomic.Bool
	reg := registry.New()
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path, executed: &mutationExecuted})
	reg.Register(&fusionVerificationTool{ledger: ledger, path: path, executed: &verificationExecuted, verified: &verified})
	executor := newStreamingTestExecutor(context.Background(), reg)
	assistant := types.Message{Role: types.RoleAssistant}
	executor.AddTool(streamingTestUse("stream-patch", "ApplyPatch"), assistant)
	executor.AddTool(streamingRevisionDependentRun("stream-run"), assistant)

	results, _, err := executor.RemainingResults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mutationExecuted.Load() != 1 || verificationExecuted.Load() != 1 || !verified.Load() {
		t.Fatalf("stream fusion mutation=%d verification=%d verified=%t", mutationExecuted.Load(), verificationExecuted.Load(), verified.Load())
	}
	metrics := results.Metrics
	if metrics.PhysicalChildOperations != 2 || metrics.RevisionFusionCount != 1 || metrics.RevisionBarrierSkips != 0 || metrics.RevisionMismatchCount != 0 || metrics.ErrorCount != 0 {
		t.Fatalf("stream fusion metrics = %+v", metrics)
	}
}

func TestStreamingRevisionFusionSkipsRunAfterPatchFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	ledger := workspacerevision.NewLedger()
	var verificationExecuted atomic.Int32
	reg := registry.New()
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path, fail: true})
	reg.Register(&writeFusionVerificationTool{fusionVerificationTool: &fusionVerificationTool{ledger: ledger, path: path, executed: &verificationExecuted}})
	executor := newStreamingTestExecutor(context.Background(), reg)
	assistant := types.Message{Role: types.RoleAssistant}
	executor.AddTool(streamingTestUse("stream-patch-failed", "ApplyPatch"), assistant)
	executor.AddTool(streamingRevisionDependentRun("stream-run-skipped"), assistant)

	results, _, err := executor.RemainingResults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if verificationExecuted.Load() != 0 || len(results.Results) != 2 || results.Results[1].Metadata["schedule.status"] != "skipped" {
		t.Fatalf("stream skip execution=%d results=%#v", verificationExecuted.Load(), results.Results)
	}
	if metrics := results.Metrics; metrics.PhysicalChildOperations != 1 || metrics.RevisionFusionCount != 1 || metrics.RevisionBarrierSkips != 1 || metrics.ErrorCount != 2 {
		t.Fatalf("stream skip metrics = %+v", metrics)
	}
}

func TestStreamingToolExecutorReturnsCorrelatedLosslessHookEvidence(t *testing.T) {
	ctx := context.Background()
	read := newStreamingExecutorTestTool("Read", true)
	reg := registry.New()
	reg.Register(read)
	runner := hooks.NewRunner([]hooks.Hook{
		{Type: hooks.HookPreToolUse, Command: testHookSuccessCommand(), Timeout: 5},
		{Type: hooks.HookPostToolUse, Command: testHookSuccessCommand(), Timeout: 5},
	})
	executor := NewStreamingToolExecutor(ctx, reg, runner, nil, "session-stream", executioncontract.ToolExecutionContext{
		Messages:   []types.Message{types.UserMessage("read")},
		SessionID:  "session-stream",
		TurnID:     "session-stream:turn-4",
		ActorID:    "agent-reviewer",
		ActorType:  "reviewer",
		WorkUnitID: "review-work",
	})
	assistant := types.Message{Role: types.RoleAssistant}
	executor.AddTool(streamingTestUse("read-hook-1", "Read"), assistant)
	waitStarted(t, read, "read-hook-1")
	read.release("read-hook-1", "complete output", false)

	results, _, err := executor.RemainingResults(ctx)
	if err != nil {
		t.Fatalf("RemainingResults: %v", err)
	}
	if len(results.HookSummaries) != 2 {
		t.Fatalf("hook summaries = %d, want pre and post: %#v", len(results.HookSummaries), results.HookSummaries)
	}
	seenIDs := make(map[string]bool)
	for _, summary := range results.HookSummaries {
		if summary.HookExecutionID == "" || seenIDs[summary.HookExecutionID] {
			t.Fatalf("hook execution ID is empty or reused: %#v", summary)
		}
		seenIDs[summary.HookExecutionID] = true
		input, ok := summary.Metadata["hook_input"].(hooks.HookInput)
		if !ok {
			t.Fatalf("hook input evidence has type %T", summary.Metadata["hook_input"])
		}
		if input.SessionID != "session-stream" || input.ToolUseID != "read-hook-1" || input.TurnID != "session-stream:turn-4" || input.AgentID != "agent-reviewer" || input.WorkUnitID != "review-work" {
			t.Fatalf("hook input lost correlation: %+v", input)
		}
		if summary.Metadata["hook_output"] == nil || summary.Metadata["hook_config"] == nil {
			t.Fatalf("hook summary omitted lossless evidence: %#v", summary.Metadata)
		}
	}
}

func TestStreamingToolExecutorNonConcurrentToolIsExclusiveBarrier(t *testing.T) {
	ctx := context.Background()
	read := newStreamingExecutorTestTool("Read", true)
	bash := newStreamingExecutorTestTool("Bash", false)
	reg := registry.New()
	reg.Register(read)
	reg.Register(bash)
	executor := newStreamingTestExecutor(ctx, reg)
	assistant := types.Message{Role: types.RoleAssistant}

	executor.AddTool(streamingTestUse("read_1", "Read"), assistant)
	executor.AddTool(streamingTestUse("bash_1", "Bash"), assistant)
	executor.AddTool(streamingTestUse("read_2", "Read"), assistant)

	waitStarted(t, read, "read_1")
	assertNotStarted(t, bash.started, "Bash while prior Read is executing")
	assertNotStarted(t, read.started, "second Read behind queued Bash")

	read.release("read_1", "read one", false)
	waitStarted(t, bash, "bash_1")
	assertNotStarted(t, read.started, "second Read while Bash is executing")

	bash.release("bash_1", "bash ok", false)
	waitStarted(t, read, "read_2")
	read.release("read_2", "read two", false)

	results, _, err := executor.RemainingResults(ctx)
	if err != nil {
		t.Fatalf("RemainingResults: %v", err)
	}
	got := []string{results.Results[0].ToolUseID, results.Results[1].ToolUseID, results.Results[2].ToolUseID}
	want := []string{"read_1", "bash_1", "read_2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("result order = %v, want %v", got, want)
		}
	}
}

func TestStreamingToolExecutorBashErrorCancelsQueuedSibling(t *testing.T) {
	ctx := context.Background()
	bash := newStreamingExecutorTestTool("Bash", false)
	read := newStreamingExecutorTestTool("Read", true)
	reg := registry.New()
	reg.Register(bash)
	reg.Register(read)
	executor := newStreamingTestExecutor(ctx, reg)
	assistant := types.Message{Role: types.RoleAssistant}

	executor.AddTool(streamingTestUse("bash_1", "Bash"), assistant)
	executor.AddTool(streamingTestUse("read_1", "Read"), assistant)

	waitStarted(t, bash, "bash_1")
	assertNotStarted(t, read.started, "Read while Bash is executing")

	bash.release("bash_1", "bash failed", true)
	results, _, err := executor.RemainingResults(ctx)
	if err != nil {
		t.Fatalf("RemainingResults: %v", err)
	}
	if len(results.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results.Results))
	}
	if !results.Results[0].IsError || results.Results[0].ToolUseID != "bash_1" {
		t.Fatalf("bash result = %+v, want bash error", results.Results[0])
	}
	if !results.Results[1].IsError || results.Results[1].ToolUseID != "read_1" {
		t.Fatalf("read result = %+v, want cancelled sibling error", results.Results[1])
	}
	if got := results.Results[1].TextContent(); got == "" || got == "read one" {
		t.Fatalf("read cancellation content = %q", got)
	}
}

func TestStreamingToolExecutorNonBashErrorDoesNotCancelIndependentSibling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errorTool := newStreamingExecutorTestTool("ErrorTool", true)
	read := newStreamingExecutorTestTool("Read", true)
	reg := registry.New()
	reg.Register(errorTool)
	reg.Register(read)
	executor := newStreamingTestExecutor(ctx, reg)
	assistant := types.Message{Role: types.RoleAssistant}

	executor.AddTool(streamingTestUse("err_1", "ErrorTool"), assistant)
	executor.AddTool(streamingTestUse("read_1", "Read"), assistant)

	waitStarted(t, errorTool, "err_1")
	waitStarted(t, read, "read_1")

	errorTool.release("err_1", "tool-level error", true)
	read.release("read_1", "read still completed", false)

	results, _, err := executor.RemainingResults(ctx)
	if err != nil {
		t.Fatalf("RemainingResults: %v", err)
	}
	if len(results.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results.Results))
	}
	if !results.Results[0].IsError || results.Results[0].ToolUseID != "err_1" {
		t.Fatalf("error tool result = %+v, want tool-level error", results.Results[0])
	}
	if results.Results[1].IsError || results.Results[1].TextContent() != "read still completed" {
		t.Fatalf("read result = %+v, want successful independent sibling", results.Results[1])
	}
}

func TestStreamingToolExecutorDiscardDropsOldResults(t *testing.T) {
	ctx := context.Background()
	read := newStreamingExecutorTestTool("Read", true)
	reg := registry.New()
	reg.Register(read)
	executor := newStreamingTestExecutor(ctx, reg)

	executor.AddTool(streamingTestUse("read_1", "Read"), types.Message{Role: types.RoleAssistant})
	waitStarted(t, read, "read_1")
	executor.Discard()
	read.release("read_1", "late result", false)

	results, events, err := executor.RemainingResults(ctx)
	if err != nil {
		t.Fatalf("RemainingResults: %v", err)
	}
	if len(results.Results) != 0 || len(events) != 0 {
		t.Fatalf("discard yielded results=%+v events=%+v, want none", results.Results, events)
	}
}

func TestQueryLoopStreamingToolsWaitForValidatedBatchBeforeStarting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read := newStreamingExecutorTestTool("Read", true)
	reg := registry.New()
	reg.Register(read)
	executor := newStreamingTestExecutor(ctx, reg)
	providerStream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse,
			ID:   "read_1",
			Name: "Read",
		}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type:        "input_json_delta",
			PartialJSON: `{"id":"read_1"}`,
		}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	msg, _, _, err := (&QueryLoop{}).processStream(ctx, providerStream, 1, func(stream.Event) {}, executor)
	if err != nil {
		t.Fatalf("processStream: %v", err)
	}
	if msg == nil || len(msg.GetToolUses()) != 1 {
		t.Fatalf("tool uses = %+v, want one", msg)
	}
	assertNotStarted(t, read.started, "Read before the complete batch was validated")

	executor.AddTool(msg.GetToolUses()[0], *msg)
	waitStarted(t, read, "read_1")
	read.release("read_1", "read ok", false)
	results, _, err := executor.RemainingResults(ctx)
	if err != nil {
		t.Fatalf("RemainingResults: %v", err)
	}
	if len(results.Results) != 1 || results.Results[0].ToolUseID != "read_1" {
		t.Fatalf("results = %+v, want read_1", results.Results)
	}
}

func TestQueryLoopStreamingToolsGateOffUsesMessageStopPath(t *testing.T) {
	ctx := context.Background()
	read := newStreamingExecutorTestTool("Read", true)
	reg := registry.New()
	reg.Register(read)
	prov := newStreamingGateProvider(read)
	ql := New(prov, reg, Config{MaxTurns: 2, StreamingToolExecution: false})

	done := make(chan error, 1)
	go func() {
		done <- ql.Run(ctx, "start", func(stream.Event) {})
	}()

	select {
	case err := <-prov.checked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("provider did not reach gate-off pre-message_stop check")
	}
	waitStarted(t, read, "read_1")
	read.release("read_1", "read ok", false)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not complete")
	}
}
