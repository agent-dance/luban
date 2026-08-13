package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// concurrentTool is a test tool that tracks execution order
type concurrentTool struct {
	name       string
	concurrent bool
	execOrder  *int64
}

func (t *concurrentTool) Name() string        { return t.name }
func (t *concurrentTool) Description() string { return "test tool" }
func (t *concurrentTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *concurrentTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if ctx.Err() != nil {
		return types.ToolResult{}, ctx.Err()
	}
	atomic.AddInt64(t.execOrder, 1)
	return types.ToolResult{Content: "executed " + t.name}, nil
}
func (t *concurrentTool) IsConcurrentSafe() bool { return t.concurrent }

type orderedBatchTool struct {
	name       string
	concurrent bool
	execute    func(context.Context, map[string]any) (types.ToolResult, error)
}

func (t *orderedBatchTool) Name() string        { return t.name }
func (t *orderedBatchTool) Description() string { return "test tool" }
func (t *orderedBatchTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *orderedBatchTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if t.execute != nil {
		return t.execute(ctx, input)
	}
	return types.ToolResult{Content: "executed " + t.name}, nil
}
func (t *orderedBatchTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ConcurrencySafe: t.concurrent}
}

type inputAwareBatchTool struct {
	orderedBatchTool
}

type atomicAdmissionProbeTool struct {
	name     string
	executed *atomic.Int32
}

func (t *atomicAdmissionProbeTool) Name() string        { return t.name }
func (t *atomicAdmissionProbeTool) Description() string { return "admission probe" }
func (t *atomicAdmissionProbeTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"payload": map[string]any{
			"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}},
			"required": []string{"value"}, "additionalProperties": false,
		},
	}, "payload")
}
func (t *atomicAdmissionProbeTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	t.executed.Add(1)
	return types.ToolResult{Content: "executed"}, nil
}

type fusionMutationData struct {
	receipt workspacerevision.Receipt
}

func (d fusionMutationData) WorkspaceRevisionReceipt() (workspacerevision.Receipt, bool) {
	return d.receipt, d.receipt.Valid()
}

type fusionMutationTool struct {
	ledger          *workspacerevision.Ledger
	root            string
	path            string
	fail            bool
	outcome         types.ToolOutcome
	infraErr        error
	mutateAfterSeal bool
	executed        *atomic.Int32
}

func (t *fusionMutationTool) Name() string        { return "ApplyPatch" }
func (t *fusionMutationTool) Description() string { return "test mutation" }
func (t *fusionMutationTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *fusionMutationTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true}
}
func (t *fusionMutationTool) ProvidesWorkspaceRevisionBarrier() bool { return t.ledger != nil }
func (t *fusionMutationTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	if t.executed != nil {
		t.executed.Store(1)
	}
	if t.infraErr != nil {
		return types.ToolResult{}, t.infraErr
	}
	if t.fail {
		return types.ToolResult{Content: "patch failed", IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	if t.outcome != "" && t.outcome != types.ToolOutcomeSucceeded {
		return types.ToolResult{Content: "patch did not commit", IsError: true, Outcome: t.outcome}, nil
	}
	if err := os.WriteFile(t.path, []byte("patched\n"), 0o600); err != nil {
		return types.ToolResult{}, err
	}
	receipt, err := t.ledger.Commit(t.root, []string{t.path})
	if err != nil {
		return types.ToolResult{}, err
	}
	if t.mutateAfterSeal {
		if err := os.WriteFile(t.path, []byte("intervening\n"), 0o600); err != nil {
			return types.ToolResult{}, err
		}
	}
	return types.ToolResult{Content: "patch committed", Data: fusionMutationData{receipt: receipt}, Outcome: types.ToolOutcomeSucceeded}, nil
}

type fusionVerificationTool struct {
	ledger   *workspacerevision.Ledger
	path     string
	executed *atomic.Int32
	verified *atomic.Bool
}

type deniedFusionMutationTool struct {
	*fusionMutationTool
}

type writeFusionVerificationTool struct {
	*fusionVerificationTool
}

func (t *writeFusionVerificationTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true}
}

func (t *deniedFusionMutationTool) CheckPermissions(context.Context, map[string]any, types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: "denied for test", Required: true}, nil
}

func (t *fusionVerificationTool) Name() string        { return "Run" }
func (t *fusionVerificationTool) Description() string { return "test verification" }
func (t *fusionVerificationTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *fusionVerificationTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true}
}
func (t *fusionVerificationTool) ConsumesWorkspaceRevisionBarrier() bool { return t.ledger != nil }
func (t *fusionVerificationTool) RequiresPatchCommit(input map[string]any) bool {
	required, _ := input["requires_patch_commit"].(bool)
	return required
}
func (t *fusionVerificationTool) Execute(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
	if t.executed != nil {
		t.executed.Add(1)
	}
	receipt, ok := workspacerevision.FromContext(ctx)
	if !ok || t.ledger.Validate(receipt) != nil {
		return types.ToolResult{
			Content: "revision mismatch", IsError: true, Outcome: types.ToolOutcomeFailed,
			Metadata: map[string]string{"verification.status": "revision_mismatch"},
		}, nil
	}
	content, err := os.ReadFile(t.path)
	if err != nil || string(content) != "patched\n" {
		return types.ToolResult{Content: "stale", IsError: true, Outcome: types.ToolOutcomeFailed}, err
	}
	if t.verified != nil {
		t.verified.Store(true)
	}
	return types.ToolResult{Content: "verified", Outcome: types.ToolOutcomeSucceeded, Metadata: map[string]string{
		"verification.status":        "revision_bound",
		"verification.kind":          "targeted_test",
		"verification.config_digest": string(digestFlightValues("fusion-verification-config")),
	}}, nil
}

func (t *inputAwareBatchTool) ToolMetadata(input map[string]any) types.ToolMetadata {
	safe, _ := input["safe"].(bool)
	return types.ToolMetadata{ConcurrencySafe: safe}
}

func TestToolBatchAdmissionIsAtomicBeforeExecution(t *testing.T) {
	reg := registry.New()
	var firstExecuted atomic.Int32
	var secondExecuted atomic.Int32
	reg.Register(&atomicAdmissionProbeTool{name: "First", executed: &firstExecuted})
	reg.Register(&atomicAdmissionProbeTool{name: "Second", executed: &secondExecuted})

	result, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "session", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{
			{ID: "call_first", Name: "First", Input: map[string]any{"payload": map[string]any{"value": "ok"}}},
			{ID: "call_second", Name: "Second", Input: map[string]any{"payload": map[string]any{"unexpected": "invalid"}}},
		}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if firstExecuted.Load() != 0 || secondExecuted.Load() != 0 {
		t.Fatalf("invalid batch crossed execution boundary: first=%d second=%d", firstExecuted.Load(), secondExecuted.Load())
	}
	if len(result.Results) != 2 || !result.Results[0].IsError || !result.Results[1].IsError {
		t.Fatalf("atomic rejection results = %#v", result.Results)
	}
}

type recordingPermissionHandler struct {
	input map[string]any
}

func (h *recordingPermissionHandler) Check(ctx context.Context, req permission.PermissionRequest) (permission.PermissionDecision, error) {
	h.input = req.Input
	return permission.PermissionAllow, nil
}

type denyPermissionHandler struct{}

func (h denyPermissionHandler) Check(ctx context.Context, req permission.PermissionRequest) (permission.PermissionDecision, error) {
	return permission.PermissionDeny, nil
}

type errorPermissionHandler struct{ err error }

func (h errorPermissionHandler) Check(context.Context, permission.PermissionRequest) (permission.PermissionDecision, error) {
	return permission.PermissionDeny, h.err
}

func TestExecuteToolsConcurrentlyPreservesMixedExecutionOrder(t *testing.T) {
	var wrote atomic.Int32

	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Write", concurrent: false, execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
		wrote.Store(1)
		return types.ToolResult{Content: "write"}, nil
	}})
	reg.Register(&orderedBatchTool{name: "Read", concurrent: true, execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
		if wrote.Load() == 0 {
			return types.ToolResult{Content: "stale"}, nil
		}
		return types.ToolResult{Content: "fresh"}, nil
	}})

	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "t1", Name: "Write", Input: map[string]any{}},
		{Type: types.ContentTypeToolUse, ID: "t2", Name: "Read", Input: map[string]any{}},
	}

	results, _, err := executeToolsConcurrently(context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{}, toolUses, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[1].Content != "fresh" {
		t.Fatalf("read ran before write completed: got %q", results[1].Content)
	}
}

func TestExecuteToolsConcurrentlyBatchesOnlyConsecutiveSafeTools(t *testing.T) {
	var firstReadsStarted atomic.Int32
	var activeFirstReads atomic.Int32
	var writeSawActiveFirstReads atomic.Int32
	var wrote atomic.Int32
	bothFirstReadsStarted := make(chan struct{})
	releaseFirstReads := make(chan struct{})
	var closeBoth sync.Once

	blockingRead := func(name string) *orderedBatchTool {
		return &orderedBatchTool{name: name, concurrent: true, execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
			activeFirstReads.Add(1)
			defer activeFirstReads.Add(-1)
			if firstReadsStarted.Add(1) == 2 {
				closeBoth.Do(func() { close(bothFirstReadsStarted) })
			}
			select {
			case <-releaseFirstReads:
				return types.ToolResult{Content: "done " + name}, nil
			case <-ctx.Done():
				return types.ToolResult{}, ctx.Err()
			}
		}}
	}

	reg := registry.New()
	reg.Register(blockingRead("Read1"))
	reg.Register(blockingRead("Read2"))
	reg.Register(&orderedBatchTool{name: "Write", concurrent: false, execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
		writeSawActiveFirstReads.Store(activeFirstReads.Load())
		wrote.Store(1)
		return types.ToolResult{Content: "write"}, nil
	}})
	reg.Register(&orderedBatchTool{name: "Read3", concurrent: true, execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
		return types.ToolResult{Content: fmt.Sprintf("write=%t", wrote.Load() == 1)}, nil
	}})

	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "t1", Name: "Read1", Input: map[string]any{}},
		{Type: types.ContentTypeToolUse, ID: "t2", Name: "Read2", Input: map[string]any{}},
		{Type: types.ContentTypeToolUse, ID: "t3", Name: "Write", Input: map[string]any{}},
		{Type: types.ContentTypeToolUse, ID: "t4", Name: "Read3", Input: map[string]any{}},
	}

	type executionResult struct {
		results []types.ToolResultBlock
		err     error
	}
	done := make(chan executionResult, 1)
	go func() {
		results, _, err := executeToolsConcurrently(context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{}, toolUses, nil)
		done <- executionResult{results: results, err: err}
	}()

	select {
	case <-bothFirstReadsStarted:
	case <-time.After(time.Second):
		close(releaseFirstReads)
		result := <-done
		t.Fatalf("first safe batch did not run concurrently; results=%v err=%v", result.results, result.err)
	}
	close(releaseFirstReads)

	result := <-done
	if result.err != nil {
		t.Fatalf("unexpected error: %v", result.err)
	}
	if writeSawActiveFirstReads.Load() != 0 {
		t.Fatalf("write started before first safe batch completed; active reads=%d", writeSawActiveFirstReads.Load())
	}
	if result.results[3].Content != "write=true" {
		t.Fatalf("trailing read ran before write completed: got %q", result.results[3].Content)
	}
}

func TestExecuteToolsConcurrently(t *testing.T) {
	var execOrder int64

	reg := registry.New()
	reg.Register(&concurrentTool{name: "SafeA", concurrent: true, execOrder: &execOrder})
	reg.Register(&concurrentTool{name: "SafeB", concurrent: true, execOrder: &execOrder})
	reg.Register(&concurrentTool{name: "Unsafe", concurrent: false, execOrder: &execOrder})

	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "t1", Name: "SafeA", Input: map[string]any{}},
		{Type: types.ContentTypeToolUse, ID: "t2", Name: "Unsafe", Input: map[string]any{}},
		{Type: types.ContentTypeToolUse, ID: "t3", Name: "SafeB", Input: map[string]any{}},
	}

	var callbackCount atomic.Int64
	results, _, err := executeToolsConcurrently(context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{}, toolUses, func(i int, r types.ToolResultBlock) {
		callbackCount.Add(1)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All 3 results present
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Results preserve original order
	if results[0].ToolUseID != "t1" {
		t.Errorf("result 0: expected t1, got %s", results[0].ToolUseID)
	}
	if results[1].ToolUseID != "t2" {
		t.Errorf("result 1: expected t2, got %s", results[1].ToolUseID)
	}
	if results[2].ToolUseID != "t3" {
		t.Errorf("result 2: expected t3, got %s", results[2].ToolUseID)
	}

	// All callbacks fired
	if callbackCount.Load() != 3 {
		t.Errorf("expected 3 callbacks, got %d", callbackCount.Load())
	}

	// All results have content
	for i, r := range results {
		if r.Content == "" {
			t.Errorf("result %d has empty content", i)
		}
	}
}

func TestToolExecutionMetricsCountSchedulerFanoutLatencyAndErrors(t *testing.T) {
	reg := registry.New()
	for _, name := range []string{"SafeMetricsA", "SafeMetricsB"} {
		name := name
		reg.Register(&orderedBatchTool{name: name, concurrent: true, execute: func(context.Context, map[string]any) (types.ToolResult, error) {
			time.Sleep(15 * time.Millisecond)
			return types.ToolResult{Content: name}, nil
		}})
	}
	reg.Register(&orderedBatchTool{name: "FailMetrics", concurrent: false, execute: func(context.Context, map[string]any) (types.ToolResult, error) {
		return types.ToolResult{Content: "failed", IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}})

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{
			{Type: types.ContentTypeToolUse, ID: "metrics-a", Name: "SafeMetricsA", Input: map[string]any{}},
			{Type: types.ContentTypeToolUse, ID: "metrics-b", Name: "SafeMetricsB", Input: map[string]any{}},
			{Type: types.ContentTypeToolUse, ID: "metrics-fail", Name: "FailMetrics", Input: map[string]any{}},
		}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	metrics := detailed.Metrics
	if metrics.PhysicalChildOperations != 3 || metrics.PeakFanout != 2 || metrics.BatchCount != 2 || metrics.ErrorCount != 1 {
		t.Fatalf("metrics = %+v, want physical=3 fanout=2 batches=2 errors=1", metrics)
	}
	if metrics.QueueDuration < 10*time.Millisecond || metrics.CriticalPathDuration < 10*time.Millisecond || metrics.TotalChildLatency < 20*time.Millisecond {
		t.Fatalf("metrics omitted measured scheduling latency: %+v", metrics)
	}
}

func TestRevisionFusionRunsVerificationAfterCommittedMutation(t *testing.T) {
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

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{
			{ID: "patch", Name: "ApplyPatch", Input: map[string]any{}},
			{ID: "verify", Name: "Run", Input: map[string]any{"requires_patch_commit": true}},
		}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if mutationExecuted.Load() != 1 || verificationExecuted.Load() != 1 || !verified.Load() {
		t.Fatalf("execution mutation=%d verification=%d verified=%t", mutationExecuted.Load(), verificationExecuted.Load(), verified.Load())
	}
	if len(detailed.Results) != 2 || detailed.Results[0].Outcome != types.ToolOutcomeSucceeded || detailed.Results[1].Outcome != types.ToolOutcomeSucceeded {
		t.Fatalf("results = %#v", detailed.Results)
	}
	metrics := detailed.Metrics
	if metrics.PhysicalChildOperations != 2 || metrics.BatchCount != 2 || metrics.RevisionFusionCount != 1 || metrics.RevisionBarrierSkips != 0 || metrics.RevisionMismatchCount != 0 || metrics.ErrorCount != 0 {
		t.Fatalf("fusion metrics = %+v", metrics)
	}
}

func TestRevisionFusionAutomaticallyBindsAdjacentRun(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	var verificationExecuted atomic.Int32
	var verified atomic.Bool
	reg := registry.New()
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path})
	reg.Register(&fusionVerificationTool{ledger: ledger, path: path, executed: &verificationExecuted, verified: &verified})

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{
			{ID: "patch", Name: "ApplyPatch", Input: map[string]any{}},
			{ID: "verify", Name: "Run", Input: map[string]any{}},
		}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if verificationExecuted.Load() != 1 || !verified.Load() {
		t.Fatalf("automatic binding execution=%d verified=%t", verificationExecuted.Load(), verified.Load())
	}
	if detailed.Metrics.RevisionFusionCount != 1 || detailed.Metrics.RevisionBarrierSkips != 0 {
		t.Fatalf("automatic binding metrics = %+v", detailed.Metrics)
	}
}

func TestRevisionFusionSkipsRunAfterPatchBusinessFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	ledger := workspacerevision.NewLedger()
	var verificationExecuted atomic.Int32
	reg := registry.New()
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path, fail: true})
	reg.Register(&fusionVerificationTool{ledger: ledger, path: path, executed: &verificationExecuted})

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{{ID: "patch-failed", Name: "ApplyPatch", Input: map[string]any{}}, {ID: "verify-skipped", Name: "Run", Input: map[string]any{"requires_patch_commit": true}}}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if verificationExecuted.Load() != 0 {
		t.Fatalf("Run executed %d times after failed patch", verificationExecuted.Load())
	}
	if len(detailed.Results) != 2 || detailed.Results[1].Metadata["schedule.status"] != "skipped" || detailed.Results[1].Outcome != types.ToolOutcomeFailed {
		t.Fatalf("skipped result = %#v", detailed.Results)
	}
	if _, ok := detailed.Results[1].Data.(revisionBarrierSkip); !ok {
		t.Fatalf("skipped Run Data = %T", detailed.Results[1].Data)
	}
	metrics := detailed.Metrics
	if metrics.PhysicalChildOperations != 1 || metrics.RevisionFusionCount != 1 || metrics.RevisionBarrierSkips != 1 || metrics.ErrorCount != 2 {
		t.Fatalf("failed fusion metrics = %+v", metrics)
	}
}

func TestRevisionFusionAutomaticallySkipsAdjacentRunAfterPatchFailure(t *testing.T) {
	root := t.TempDir()
	ledger := workspacerevision.NewLedger()
	var verificationExecuted atomic.Int32
	reg := registry.New()
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: filepath.Join(root, "source.txt"), fail: true})
	reg.Register(&fusionVerificationTool{ledger: ledger, executed: &verificationExecuted})

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{
			{ID: "patch-failed", Name: "ApplyPatch", Input: map[string]any{}},
			{ID: "verify-skipped", Name: "Run", Input: map[string]any{}},
		}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if verificationExecuted.Load() != 0 || detailed.Results[1].Metadata["schedule.status"] != "skipped" {
		t.Fatalf("automatic skip execution=%d results=%#v", verificationExecuted.Load(), detailed.Results)
	}
	if detailed.Metrics.RevisionFusionCount != 1 || detailed.Metrics.RevisionBarrierSkips != 1 {
		t.Fatalf("automatic skip metrics = %+v", detailed.Metrics)
	}
}

func TestRevisionDependencySkipsRunAfterPartialPatch(t *testing.T) {
	root := t.TempDir()
	ledger := workspacerevision.NewLedger()
	var verificationExecuted atomic.Int32
	reg := registry.New()
	reg.Register(&fusionMutationTool{
		ledger: ledger, root: root, path: filepath.Join(root, "source.txt"), outcome: types.ToolOutcomePartial,
	})
	reg.Register(&fusionVerificationTool{ledger: ledger, executed: &verificationExecuted})

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{
			{ID: "patch-partial", Name: "ApplyPatch", Input: map[string]any{}},
			{ID: "run-after-partial", Name: "Run", Input: map[string]any{"requires_patch_commit": true}},
		}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if verificationExecuted.Load() != 0 || len(detailed.Results) != 2 || detailed.Results[1].Metadata["schedule.status"] != "skipped" {
		t.Fatalf("partial dependency execution=%d results=%#v", verificationExecuted.Load(), detailed.Results)
	}
	if detailed.Metrics.PhysicalChildOperations != 1 || detailed.Metrics.RevisionFusionCount != 1 || detailed.Metrics.RevisionBarrierSkips != 1 {
		t.Fatalf("partial dependency metrics=%+v", detailed.Metrics)
	}
}

func TestRevisionFusionMismatchCannotBecomeVerification(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	var verificationExecuted atomic.Int32
	var verified atomic.Bool
	reg := registry.New()
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path, mutateAfterSeal: true})
	reg.Register(&fusionVerificationTool{ledger: ledger, path: path, executed: &verificationExecuted, verified: &verified})

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{{ID: "patch", Name: "ApplyPatch", Input: map[string]any{}}, {ID: "verify", Name: "Run", Input: map[string]any{"requires_patch_commit": true}}}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if verificationExecuted.Load() != 1 || verified.Load() {
		t.Fatalf("verification executed=%d verified=%t", verificationExecuted.Load(), verified.Load())
	}
	if detailed.Results[1].Metadata["verification.status"] != "revision_mismatch" || detailed.Results[1].Outcome != types.ToolOutcomeFailed {
		t.Fatalf("mismatch result = %#v", detailed.Results[1])
	}
	if detailed.Metrics.RevisionMismatchCount != 1 || detailed.Metrics.ErrorCount != 1 {
		t.Fatalf("mismatch metrics = %+v", detailed.Metrics)
	}
}

func TestRevisionDependencyFlagKeepsIndependentRunUnbound(t *testing.T) {
	root := t.TempDir()
	ledger := workspacerevision.NewLedger()
	var verificationExecuted atomic.Int32
	reg := registry.New()
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: filepath.Join(root, "source.txt"), fail: true})
	reg.Register(&fusionVerificationTool{ledger: ledger, executed: &verificationExecuted})

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{{ID: "failed-patch", Name: "ApplyPatch", Input: map[string]any{}}, {ID: "independent-run", Name: "Run", Input: map[string]any{"requires_patch_commit": false}}}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if verificationExecuted.Load() != 1 {
		t.Fatalf("independent Run executed %d times, want 1", verificationExecuted.Load())
	}
	if detailed.Metrics.RevisionFusionCount != 0 || detailed.Metrics.RevisionBarrierSkips != 0 || detailed.Metrics.PhysicalChildOperations != 2 {
		t.Fatalf("independent metrics = %+v", detailed.Metrics)
	}
}

func TestRevisionDependencySkipsMutatingRunAfterPatchFailure(t *testing.T) {
	root := t.TempDir()
	ledger := workspacerevision.NewLedger()
	var verificationExecuted atomic.Int32
	reg := registry.New()
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: filepath.Join(root, "source.txt"), fail: true})
	reg.Register(&writeFusionVerificationTool{fusionVerificationTool: &fusionVerificationTool{ledger: ledger, executed: &verificationExecuted}})

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{{ID: "failed-patch", Name: "ApplyPatch", Input: map[string]any{}}, {ID: "mutating-run", Name: "Run", Input: map[string]any{"requires_patch_commit": true}}}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if verificationExecuted.Load() != 0 || detailed.Metrics.RevisionFusionCount != 1 || detailed.Metrics.RevisionBarrierSkips != 1 || detailed.Metrics.PhysicalChildOperations != 1 {
		t.Fatalf("mutating Run execution=%d metrics=%+v", verificationExecuted.Load(), detailed.Metrics)
	}
	if len(detailed.Results) != 2 || detailed.Results[1].Metadata["schedule.status"] != "skipped" {
		t.Fatalf("mutating Run skip result=%#v", detailed.Results)
	}
}

func TestRevisionFusionReturnsTypedSkipAfterCancelledPatch(t *testing.T) {
	root := t.TempDir()
	ledger := workspacerevision.NewLedger()
	var verificationExecuted atomic.Int32
	reg := registry.New()
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: filepath.Join(root, "source.txt"), infraErr: context.Canceled})
	reg.Register(&fusionVerificationTool{ledger: ledger, executed: &verificationExecuted})

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{{ID: "cancelled-patch", Name: "ApplyPatch", Input: map[string]any{}}, {ID: "skipped-run", Name: "Run", Input: map[string]any{"requires_patch_commit": true}}}, nil,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("execution error = %v, want context canceled", err)
	}
	if verificationExecuted.Load() != 0 || len(detailed.Results) != 2 || detailed.Results[1].Metadata["schedule.status"] != "skipped" {
		t.Fatalf("cancelled fusion execution=%d results=%#v", verificationExecuted.Load(), detailed.Results)
	}
	if detailed.Metrics.RevisionBarrierSkips != 1 || detailed.Metrics.PhysicalChildOperations != 1 {
		t.Fatalf("cancelled fusion metrics = %+v", detailed.Metrics)
	}
}

func TestRevisionFusionSkipsRunAfterDeniedPatch(t *testing.T) {
	root := t.TempDir()
	ledger := workspacerevision.NewLedger()
	var mutationExecuted atomic.Int32
	var verificationExecuted atomic.Int32
	reg := registry.New()
	reg.Register(&deniedFusionMutationTool{fusionMutationTool: &fusionMutationTool{
		ledger: ledger, root: root, path: filepath.Join(root, "source.txt"), executed: &mutationExecuted,
	}})
	reg.Register(&fusionVerificationTool{ledger: ledger, executed: &verificationExecuted})

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{{ID: "denied-patch", Name: "ApplyPatch", Input: map[string]any{}}, {ID: "skipped-run", Name: "Run", Input: map[string]any{"requires_patch_commit": true}}}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if mutationExecuted.Load() != 0 || verificationExecuted.Load() != 0 || detailed.Results[0].Outcome != types.ToolOutcomeDenied || detailed.Results[1].Metadata["schedule.status"] != "skipped" {
		t.Fatalf("denied fusion mutation=%d verification=%d results=%#v", mutationExecuted.Load(), verificationExecuted.Load(), detailed.Results)
	}
	if detailed.Metrics.RevisionBarrierSkips != 1 || detailed.Metrics.PhysicalChildOperations != 1 || detailed.Metrics.ErrorCount != 2 {
		t.Fatalf("denied fusion metrics = %+v", detailed.Metrics)
	}
}

func TestConcurrentInfrastructureErrorsStillPublishEveryTerminalResult(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Deadline", concurrent: true, execute: func(context.Context, map[string]any) (types.ToolResult, error) {
		return types.ToolResult{}, context.DeadlineExceeded
	}})
	reg.Register(&orderedBatchTool{name: "Sibling", concurrent: true, execute: func(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
		<-ctx.Done()
		return types.ToolResult{}, ctx.Err()
	}})
	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "deadline-tool", Name: "Deadline", Input: map[string]any{}},
		{Type: types.ContentTypeToolUse, ID: "sibling-tool", Name: "Sibling", Input: map[string]any{}},
	}

	var mu sync.Mutex
	published := make(map[string]types.ToolResultBlock)
	results, _, err := executeToolsConcurrently(context.Background(), reg, nil, nil, "session", executioncontract.ToolExecutionContext{}, toolUses, func(_ int, result types.ToolResultBlock) {
		mu.Lock()
		defer mu.Unlock()
		if _, duplicate := published[result.ToolUseID]; duplicate {
			t.Errorf("duplicate terminal callback for %s", result.ToolUseID)
		}
		published[result.ToolUseID] = result
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("execute error = %v, want deadline exceeded", err)
	}
	if len(results) != len(toolUses) || len(published) != len(toolUses) {
		t.Fatalf("terminal results=%d published=%d, want %d", len(results), len(published), len(toolUses))
	}
	for index, toolUse := range toolUses {
		result, ok := published[toolUse.ID]
		if !ok {
			t.Fatalf("missing terminal callback for %s; returned result=%#v", toolUse.ID, results[index])
		}
		if result.ToolUseID != results[index].ToolUseID || result.Outcome == "" || !result.IsError {
			t.Fatalf("published result for %s disagrees with transcript result: published=%#v returned=%#v", toolUse.ID, result, results[index])
		}
	}
}

func TestToolResultMetadataProducesPartialAndTimedOutOutcomes(t *testing.T) {
	tests := []struct {
		name    string
		result  types.ToolResult
		want    types.ToolOutcome
		isError bool
	}{
		{name: "partial", result: types.ToolResult{Content: "some matches", Metadata: map[string]string{"partial": "true", "timed_out": "true"}}, want: types.ToolOutcomePartial},
		{name: "timeout", result: types.ToolResult{Content: "deadline", IsError: true, Metadata: map[string]string{"mcp.timeout": "true"}}, want: types.ToolOutcomeTimedOut, isError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := registry.New()
			reg.Register(&orderedBatchTool{name: "Outcome", execute: func(context.Context, map[string]any) (types.ToolResult, error) { return tc.result, nil }})
			results, _, err := executeToolsConcurrently(context.Background(), reg, nil, nil, "session", executioncontract.ToolExecutionContext{}, []types.ToolUseBlock{{ID: "outcome", Name: "Outcome"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if results[0].Outcome != tc.want || results[0].IsError != tc.isError {
				t.Fatalf("normalized result = %#v, want outcome=%s error=%v", results[0], tc.want, tc.isError)
			}
		})
	}
}

func TestPermissionHandlerErrorsRetainCancelledTimedOutAndFailedOutcomes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want types.ToolOutcome
	}{
		{name: "cancelled", err: context.Canceled, want: types.ToolOutcomeCancelled},
		{name: "timed out", err: context.DeadlineExceeded, want: types.ToolOutcomeTimedOut},
		{name: "failed", err: errors.New("permission backend unavailable"), want: types.ToolOutcomeFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := registry.New()
			reg.Register(&orderedBatchTool{name: "Write", execute: func(context.Context, map[string]any) (types.ToolResult, error) {
				t.Fatal("tool executed after permission error")
				return types.ToolResult{}, nil
			}})
			results, _, err := executeToolsConcurrently(context.Background(), reg, nil, errorPermissionHandler{err: tc.err}, "session", executioncontract.ToolExecutionContext{}, []types.ToolUseBlock{{ID: "permission", Name: "Write"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if results[0].Outcome != tc.want {
				t.Fatalf("permission result = %#v, want %s", results[0], tc.want)
			}
		})
	}
}

func TestSerialDeadlinePublishesCurrentTimedOutBeforeCancellingRemaining(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Deadline", execute: func(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
		<-ctx.Done()
		return types.ToolResult{}, ctx.Err()
	}})
	reg.Register(&orderedBatchTool{name: "Later", execute: func(context.Context, map[string]any) (types.ToolResult, error) {
		t.Fatal("later tool executed after parent deadline")
		return types.ToolResult{}, nil
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	published := make(map[string]types.ToolOutcome)
	results, _, err := executeToolsConcurrently(ctx, reg, nil, nil, "session", executioncontract.ToolExecutionContext{}, []types.ToolUseBlock{
		{ID: "deadline", Name: "Deadline"}, {ID: "later", Name: "Later"},
	}, func(_ int, result types.ToolResultBlock) { published[result.ToolUseID] = result.Outcome })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if results[0].Outcome != types.ToolOutcomeTimedOut || results[1].Outcome != types.ToolOutcomeCancelled || published["deadline"] != types.ToolOutcomeTimedOut || published["later"] != types.ToolOutcomeCancelled {
		t.Fatalf("terminal outcomes overwritten or missing: results=%#v published=%#v", results, published)
	}
}

func TestExecuteToolsNormalizesEmptyResultWithoutResultStore(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{
		name:       "Bash",
		concurrent: true,
		execute: func(context.Context, map[string]any) (types.ToolResult, error) {
			return types.ToolResult{Content: " \n\t"}, nil
		},
	})
	toolUse := types.ToolUseBlock{
		Type:  types.ContentTypeToolUse,
		ID:    "tool_silent",
		Name:  "Bash",
		Input: map[string]any{"command": "true"},
	}

	results, _, err := executeToolsConcurrently(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{}, []types.ToolUseBlock{toolUse}, nil,
	)
	if err != nil {
		t.Fatalf("executeToolsConcurrently: %v", err)
	}
	if len(results) != 1 || results[0].Content != "(Bash completed with no output)" {
		t.Fatalf("empty result = %#v", results)
	}
}

func TestAllConcurrentSafe(t *testing.T) {
	var execOrder int64

	reg := registry.New()
	reg.Register(&concurrentTool{name: "A", concurrent: true, execOrder: &execOrder})
	reg.Register(&concurrentTool{name: "B", concurrent: true, execOrder: &execOrder})

	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "t1", Name: "A", Input: map[string]any{}},
		{Type: types.ContentTypeToolUse, ID: "t2", Name: "B", Input: map[string]any{}},
	}

	results, _, err := executeToolsConcurrently(context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{}, toolUses, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestDefaultConcurrencySafety(t *testing.T) {
	reg := registry.New()
	// Read/Glob/Grep are safe by default even without implementing ConcurrentSafe
	// We can't easily test this without real tools, but verify the function exists
	if isConcurrentSafe(reg, "Read", nil) {
		// Read is safe by name but tool must be registered
		// Since no tool is registered, it returns false
	}
	if isConcurrentSafe(reg, "nonexistent", nil) {
		t.Error("nonexistent tool should not be concurrent safe")
	}
}

func TestExecuteToolsConcurrentlyMissingToolReturnsErrorResult(t *testing.T) {
	reg := registry.New()
	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "missing_1", Name: "Nope", Input: map[string]any{}},
	}

	results, _, err := executeToolsConcurrently(context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{}, toolUses, nil)
	if err != nil {
		t.Fatalf("missing tool should not fail loop: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ToolUseID != "missing_1" {
		t.Fatalf("tool_use_id = %q, want missing_1", results[0].ToolUseID)
	}
	if !results[0].IsError {
		t.Fatal("missing tool result should be marked as error")
	}
	if !strings.Contains(results[0].Content, "unknown tool") {
		t.Fatalf("missing tool content = %q", results[0].Content)
	}
}

func TestPreToolUseModifiedInputFeedsPermissionAndExecution(t *testing.T) {
	var executedInput map[string]any
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Echo", execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
		executedInput = input
		value, _ := input["value"].(string)
		return types.ToolResult{Content: value}, nil
	}})
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookPreToolUse,
		Command: testHookOutputCommand(`{"modified_input":{"value":"hooked"}}`),
		Timeout: 5,
	}})
	perm := &recordingPermissionHandler{}
	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "tool_1", Name: "Echo", Input: map[string]any{"value": "original"}},
	}

	results, _, err := executeToolsConcurrently(context.Background(), reg, runner, perm, "session", executioncontract.ToolExecutionContext{}, toolUses, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Content != "hooked" {
		t.Fatalf("tool executed with content %q, want hooked", results[0].Content)
	}
	if perm.input["value"] != "hooked" {
		t.Fatalf("permission saw input %v, want modified input", perm.input)
	}
	if executedInput["value"] != "hooked" {
		t.Fatalf("tool saw input %v, want modified input", executedInput)
	}
}

func TestPreToolUsePermissionAllowBypassesPermissionHandler(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Echo", execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
		value, _ := input["value"].(string)
		return types.ToolResult{Content: value}, nil
	}})
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookPreToolUse,
		Command: testHookOutputCommand(`{"permissionBehavior":"allow","updatedInput":{"value":"approved"}}`),
		Timeout: 5,
	}})
	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "tool_1", Name: "Echo", Input: map[string]any{"value": "original"}},
	}

	results, _, err := executeToolsConcurrently(context.Background(), reg, runner, denyPermissionHandler{}, "session", executioncontract.ToolExecutionContext{}, toolUses, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].IsError {
		t.Fatalf("hook allow should bypass deny permission handler: %+v", results[0])
	}
	if results[0].Content != "approved" {
		t.Fatalf("tool content = %q, want approved", results[0].Content)
	}
}

func TestPostToolUseFailureRunsForErrorResultAndExecutionError(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "ErrorResult", execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
		return types.ToolResult{Content: "business failure", IsError: true}, nil
	}})
	reg.Register(&orderedBatchTool{name: "ExecutionError", execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
		return types.ToolResult{}, errors.New("boom")
	}})
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookPostToolUseFailure,
		Command: testHookOutputCommand(`{"system_reminder":"failure hook ran"}`),
		Timeout: 5,
	}})
	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "tool_1", Name: "ErrorResult", Input: map[string]any{}},
		{Type: types.ContentTypeToolUse, ID: "tool_2", Name: "ExecutionError", Input: map[string]any{}},
	}

	results, reminders, err := executeToolsConcurrently(context.Background(), reg, runner, nil, "", executioncontract.ToolExecutionContext{}, toolUses, nil)
	if err != nil {
		t.Fatalf("execution error should be surfaced as tool_result: %v", err)
	}
	if !results[0].IsError || !results[1].IsError {
		t.Fatalf("expected both results to be errors: %+v", results)
	}
	if results[1].ToolUseID != "tool_2" || !strings.Contains(results[1].Content, "boom") {
		t.Fatalf("execution error result not model-visible: %+v", results[1])
	}
	if len(reminders) != 2 {
		t.Fatalf("expected failure hook reminder for both failures, got %v", reminders)
	}
	for _, reminder := range reminders {
		if reminder != "failure hook ran" {
			t.Fatalf("unexpected reminder %q", reminder)
		}
	}
}

func TestConcurrentSafeBatchRespectsMaxConcurrency(t *testing.T) {
	t.Setenv("LUBAN_CODE_MAX_TOOL_USE_CONCURRENCY", "2")

	var active atomic.Int32
	var maxActive atomic.Int32
	reg := registry.New()
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("Safe%d", i)
		reg.Register(&orderedBatchTool{name: name, concurrent: true, execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
			current := active.Add(1)
			for {
				observed := maxActive.Load()
				if current <= observed || maxActive.CompareAndSwap(observed, current) {
					break
				}
			}
			time.Sleep(25 * time.Millisecond)
			active.Add(-1)
			return types.ToolResult{Content: "ok"}, nil
		}})
	}
	toolUses := make([]types.ToolUseBlock, 0, 5)
	for i := 0; i < 5; i++ {
		toolUses = append(toolUses, types.ToolUseBlock{
			Type:  types.ContentTypeToolUse,
			ID:    fmt.Sprintf("tool_%d", i),
			Name:  fmt.Sprintf("Safe%d", i),
			Input: map[string]any{},
		})
	}

	_, _, err := executeToolsConcurrently(context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{}, toolUses, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if maxActive.Load() > 2 {
		t.Fatalf("max active executions = %d, want <= 2", maxActive.Load())
	}
}

func TestInputAwareConcurrencySafetyUsesToolInput(t *testing.T) {
	var active atomic.Int32
	var unsafeSawActive atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{})
	var closeStarted sync.Once

	reg := registry.New()
	reg.Register(&inputAwareBatchTool{orderedBatchTool: orderedBatchTool{name: "MaybeSafe", execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
		safe, _ := input["safe"].(bool)
		if !safe {
			unsafeSawActive.Store(active.Load())
			return types.ToolResult{Content: "unsafe"}, nil
		}
		active.Add(1)
		closeStarted.Do(func() { close(started) })
		defer active.Add(-1)
		select {
		case <-release:
			return types.ToolResult{Content: "safe"}, nil
		case <-ctx.Done():
			return types.ToolResult{}, ctx.Err()
		}
	}}})
	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "safe_1", Name: "MaybeSafe", Input: map[string]any{"safe": true}},
		{Type: types.ContentTypeToolUse, ID: "unsafe_1", Name: "MaybeSafe", Input: map[string]any{"safe": false}},
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := executeToolsConcurrently(context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{}, toolUses, nil)
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("safe tool did not start")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unsafeSawActive.Load() != 0 {
		t.Fatalf("input-unsafe call overlapped with safe call; active=%d", unsafeSawActive.Load())
	}
}

func TestHookStoppedContinuationPreventsNextModelCall(t *testing.T) {
	toolUseEvents := []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: "tool_1", Name: "Echo"}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"value":"x"}`}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: stopReasonForParity(types.StopReasonToolUse)},
		{Type: types.EventMessageStop},
	}
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: toolUseEvents},
		{Events: parityTextEvents("should not be requested")},
	})
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Echo", execute: func(ctx context.Context, input map[string]any) (types.ToolResult, error) {
		return types.ToolResult{Content: "ok"}, nil
	}})
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookPostToolUse,
		Command: testHookOutputCommand(`{"prevent_continuation":true,"stop_reason":"stop after tool"}`),
		Timeout: 5,
	}})
	ql := New(prov, reg, Config{MaxTurns: 5, MaxTokens: 1024, HookRunner: runner})

	if err := ql.Run(context.Background(), "go", func(stream.Event) {}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prov.Calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(prov.Calls))
	}
	messages := ql.Messages()
	last := messages[len(messages)-1].GetText()
	if !strings.Contains(last, "stop after tool") {
		t.Fatalf("expected stop reason system reminder in final message, got %q", last)
	}
}

func TestConcurrentToolsCancelledByContext(t *testing.T) {
	var execOrder int64
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	reg := registry.New()
	reg.Register(&concurrentTool{name: "A", concurrent: true, execOrder: &execOrder})
	reg.Register(&concurrentTool{name: "B", concurrent: false, execOrder: &execOrder})

	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "t1", Name: "A", Input: map[string]any{}},
		{Type: types.ContentTypeToolUse, ID: "t2", Name: "B", Input: map[string]any{}},
	}

	results, _, err := executeToolsConcurrently(ctx, reg, nil, nil, "", executioncontract.ToolExecutionContext{}, toolUses, nil)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}

	// All results should indicate cancellation
	for i, r := range results {
		if !r.IsError {
			t.Errorf("result %d: expected IsError=true for cancelled context", i)
		}
	}

	// No tools should have actually executed
	if atomic.LoadInt64(&execOrder) != 0 {
		t.Errorf("expected 0 tool executions, got %d", atomic.LoadInt64(&execOrder))
	}
}
