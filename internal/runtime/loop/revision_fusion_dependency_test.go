package loop

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestRevisionDependencyBindsSuccessfulPatchToMutatingRun(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	var mutationExecuted atomic.Int32
	var runExecuted atomic.Int32
	var verified atomic.Bool
	reg := registry.New()
	reg.Register(&fusionMutationTool{
		ledger: ledger, root: root, path: path, executed: &mutationExecuted,
	})
	reg.Register(&writeFusionVerificationTool{fusionVerificationTool: &fusionVerificationTool{
		ledger: ledger, path: path, executed: &runExecuted, verified: &verified,
	}})

	detailed, err := executeToolsConcurrentlyDetailed(
		context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{},
		[]types.ToolUseBlock{
			{ID: "patch", Name: "ApplyPatch", Input: map[string]any{}},
			{ID: "mutating-run", Name: "Run", Input: map[string]any{"requires_patch_commit": true}},
		}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if mutationExecuted.Load() != 1 || runExecuted.Load() != 1 || !verified.Load() {
		t.Fatalf("dependency execution mutation=%d run=%d verified=%t", mutationExecuted.Load(), runExecuted.Load(), verified.Load())
	}
	if detailed.Metrics.PhysicalChildOperations != 2 || detailed.Metrics.RevisionFusionCount != 1 || detailed.Metrics.RevisionBarrierSkips != 0 {
		t.Fatalf("dependency metrics = %+v", detailed.Metrics)
	}
}

func TestStreamingRevisionDependencyBindsSuccessfulPatchToMutatingRun(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	var mutationExecuted atomic.Int32
	var runExecuted atomic.Int32
	var verified atomic.Bool
	reg := registry.New()
	reg.Register(&fusionMutationTool{
		ledger: ledger, root: root, path: path, executed: &mutationExecuted,
	})
	reg.Register(&writeFusionVerificationTool{fusionVerificationTool: &fusionVerificationTool{
		ledger: ledger, path: path, executed: &runExecuted, verified: &verified,
	}})
	executor := newStreamingTestExecutor(context.Background(), reg)
	assistant := types.Message{Role: types.RoleAssistant}
	executor.AddTool(streamingTestUse("stream-patch", "ApplyPatch"), assistant)
	executor.AddTool(streamingRevisionDependentRun("stream-mutating-run"), assistant)

	results, _, err := executor.RemainingResults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mutationExecuted.Load() != 1 || runExecuted.Load() != 1 || !verified.Load() {
		t.Fatalf("stream dependency execution mutation=%d run=%d verified=%t", mutationExecuted.Load(), runExecuted.Load(), verified.Load())
	}
	if results.Metrics.PhysicalChildOperations != 2 || results.Metrics.RevisionFusionCount != 1 || results.Metrics.RevisionBarrierSkips != 0 {
		t.Fatalf("stream dependency metrics = %+v", results.Metrics)
	}
}
