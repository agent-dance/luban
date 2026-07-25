package interaction

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/types"
)

type testPlanRuntime struct {
	context types.ToolRuntimeContext
}

func (r *testPlanRuntime) ToolRuntimeContext() types.ToolRuntimeContext { return r.context }
func (r *testPlanRuntime) TransitionPermissionMode(mode string) error {
	r.context.PermissionMode = mode
	return nil
}
func (r *testPlanRuntime) RestorePermissionMode(mode string) error {
	r.context.PermissionMode = mode
	return nil
}

func TestPlanStateRequiresProjectRootAndReportsInvalidPersistence(t *testing.T) {
	if _, err := NewPlanState(""); err == nil {
		t.Fatal("empty project root must fail")
	}
	root := t.TempDir()
	statePath := filepath.Join(root, ".luban-code", "plan-mode.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPlanState(root); err == nil {
		t.Fatal("malformed persisted state must be reported")
	}
}

func TestPlanStatePersistsV2PrivatelyAndResumes(t *testing.T) {
	root := t.TempDir()
	state, err := NewPlanState(root)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &testPlanRuntime{context: types.ToolRuntimeContext{ProjectRoot: root, PermissionMode: "default"}}
	enter := NewEnterPlanModeTool(state, runtime)
	result, err := enter.Execute(context.Background(), map[string]any{})
	if err != nil || result.IsError || !state.IsActive() || runtime.context.PermissionMode != permissionModePlan {
		t.Fatalf("result=%+v state=%v mode=%q err=%v", result, state.IsActive(), runtime.context.PermissionMode, err)
	}
	statePath := filepath.Join(root, ".luban-code", "plan-mode.json")
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("plan state mode=%#o, want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var persisted persistedPlanState
	if err := json.Unmarshal(data, &persisted); err != nil || persisted.SchemaVersion != planStateSchemaVersion {
		t.Fatalf("persisted=%+v err=%v", persisted, err)
	}
	resumed, err := NewPlanState(root)
	if err != nil || !resumed.IsActive() || resumed.PlanFile() != state.PlanFile() {
		t.Fatalf("resumed=%+v active=%v file=%q err=%v", resumed, resumed != nil && resumed.IsActive(), resumed.PlanFile(), err)
	}
}

func TestPrepareProjectRootFailureDoesNotMutateLiveState(t *testing.T) {
	root := t.TempDir()
	state, err := NewPlanState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Enter(filepath.Join(root, "plan.md")); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	targetState := filepath.Join(target, ".luban-code", "plan-mode.json")
	if err := os.MkdirAll(filepath.Dir(targetState), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetState, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := state.PrepareProjectRoot(target); err == nil {
		t.Fatal("historical state schema must fail")
	}
	if !state.IsActive() || state.PlanFile() != filepath.Join(root, "plan.md") {
		t.Fatalf("failed prepare mutated live state: active=%v file=%q", state.IsActive(), state.PlanFile())
	}
}

func TestExitPlanModeCommitsApprovedEditedPlanAndAllowedPrompt(t *testing.T) {
	root := t.TempDir()
	state, err := NewPlanState(root)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &testPlanRuntime{context: types.ToolRuntimeContext{ProjectRoot: root, PermissionMode: "default"}}
	enter := NewEnterPlanModeTool(state, runtime)
	if result, err := enter.Execute(context.Background(), map[string]any{}); err != nil || result.IsError {
		t.Fatalf("enter result=%+v err=%v", result, err)
	}
	planPath := state.PlanFile()
	if err := os.WriteFile(planPath, []byte("# original"), 0o600); err != nil {
		t.Fatal(err)
	}
	exit := NewExitPlanModeTool(state, runtime)
	input := map[string]any{
		"plan":           "# edited",
		"allowedPrompts": []any{map[string]any{"tool": "Bash", "prompt": "go test"}},
	}
	decision, err := exit.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil || decision.Behavior != types.PermissionBehaviorAsk {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
	ctx := approvalcommit.Bind(context.Background(), exit.Name(), decision.UpdatedInput, "")
	result, err := exit.Execute(ctx, decision.UpdatedInput)
	if err != nil || result.IsError || state.IsActive() || runtime.context.PermissionMode != "default" {
		t.Fatalf("result=%+v active=%v mode=%q err=%v", result, state.IsActive(), runtime.context.PermissionMode, err)
	}
	data, err := os.ReadFile(planPath)
	if err != nil || string(data) != "# edited" {
		t.Fatalf("edited plan=%q err=%v", data, err)
	}
	if !state.AllowedPromptMatches("Bash", "go test ./...") {
		t.Fatal("approved prompt was not persisted")
	}
}
