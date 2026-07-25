package agent

import (
	"context"
	"path/filepath"
	"testing"

	toolfile "github.com/agent-dance/luban/internal/tools/file"
	toolinteraction "github.com/agent-dance/luban/internal/tools/interaction"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestPinnedAgentPlanGateDoesNotFollowForegroundSession(t *testing.T) {
	origin := t.TempDir()
	foreground := t.TempDir()
	foregroundPlan, err := toolinteraction.NewPlanState(foreground)
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.New()
	reg.Register(&toolfile.FileWriteTool{AllowedDirs: []string{origin}, PlanState: foregroundPlan})
	snapshot := types.ToolRuntimeContext{ProjectRoot: origin, AllowedDirs: []string{origin}, PermissionMode: "default"}
	childRuntime := agentRuntimeContextProvider{snapshot: cloneToolRuntimeContext(snapshot), agentID: "agent-plan"}
	pinRegistryForAgentRuntime(reg, childRuntime, snapshot)
	reg.SetRuntimeContextProvider(childRuntime)

	// The foreground enters plan mode after the child registry exists. The
	// background child must keep its launch-time plan gate.
	if err := foregroundPlan.Enter(filepath.Join(foreground, "plan.md")); err != nil {
		t.Fatal(err)
	}
	childWrite, ok := reg.Get("Write").(*toolfile.FileWriteTool)
	if !ok {
		t.Fatalf("child Write = %T", reg.Get("Write"))
	}
	decision, err := childWrite.CheckPermissions(context.Background(), map[string]any{
		"file_path": filepath.Join(origin, "result.txt"), "content": "ok",
	}, types.ToolPermissionRequest{Runtime: childRuntime.ToolRuntimeContext()})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Behavior == types.PermissionBehaviorDeny {
		t.Fatalf("child plan gate followed foreground PlanState: %+v", decision)
	}
}
