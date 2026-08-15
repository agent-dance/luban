package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolinspect "github.com/agent-dance/luban/internal/agentic/inspect"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	shell "github.com/agent-dance/luban/internal/tools/shell"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestPinnedAgentRunBindsPinnedBashAndEnvironment(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	parentBash := &shell.BashTool{CWD: parentRoot, AllowedDirs: []string{parentRoot}}
	parentBash.SetEnvironmentPolicy(nil, map[string]string{"RUN_BINDING_TEST_ROOT": parentRoot})
	parentRun := shell.NewRunTool(parentBash)
	parent := registry.New()
	parent.Register(parentBash)
	parent.Register(parentRun)

	child := parent.Clone()
	snapshot := types.ToolRuntimeContext{
		SessionID: "child-session", ProjectRoot: childRoot, AllowedDirs: []string{childRoot}, PermissionMode: "default",
	}
	provider := agentRuntimeContextProvider{snapshot: snapshot, agentID: "child", cwd: childRoot}
	pinRegistryForAgentRuntime(child, provider, snapshot)
	child.SetRuntimeContextProvider(provider)

	childBash, ok := child.Get("Bash").(*shell.BashTool)
	if !ok || childBash == parentBash {
		t.Fatalf("child Bash = %T %p, parent=%p", child.Get("Bash"), childBash, parentBash)
	}
	childRun, ok := child.Get("Run").(*shell.RunTool)
	if !ok {
		t.Fatalf("child Run = %T", child.Get("Run"))
	}
	if childRun == parentRun || childRun.Bash != childBash {
		t.Fatalf("child Run = %p bash=%p, child Bash=%p", childRun, childRun.Bash, childBash)
	}
	if childRun.Bash.CurrentCWD() != childRoot || len(childRun.Bash.CurrentAllowedDirs()) != 1 || childRun.Bash.CurrentAllowedDirs()[0] != childRoot {
		t.Fatalf("child Run scope = cwd %q dirs %v", childRun.Bash.CurrentCWD(), childRun.Bash.CurrentAllowedDirs())
	}

	input := runReadInput("pwd")
	before, err := childRun.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: snapshot})
	if err != nil || before.ExecutionPolicyCode == "" {
		t.Fatalf("child Run preflight = %+v err=%v", before, err)
	}
	parentBash.SetExecutionScope(parentRoot, []string{parentRoot})
	parentBash.SetEnvironmentPolicy(nil, map[string]string{"RUN_BINDING_TEST_ROOT": "changed-parent"})
	after, err := childRun.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: snapshot})
	if err != nil || after.ExecutionPolicyCode != before.ExecutionPolicyCode {
		t.Fatalf("child Run followed parent environment: before=%q after=%q err=%v", before.ExecutionPolicyCode, after.ExecutionPolicyCode, err)
	}
	if parentRun.Bash != parentBash {
		t.Fatal("pinning child Run changed the parent Run binding")
	}
}

func TestAgentCWDRunUsesExactSandboxedChildBash(t *testing.T) {
	requireBashAvailable(t)
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(childRoot, "child.txt"), []byte("child"), 0o644); err != nil {
		t.Fatal(err)
	}
	parentPath := filepath.Join(parentRoot, "parent.txt")
	if err := os.WriteFile(parentPath, []byte("parent"), 0o644); err != nil {
		t.Fatal(err)
	}

	parentBash := &shell.BashTool{CWD: parentRoot, AllowedDirs: []string{parentRoot}, Sandbox: &task07SandboxBackend{}}
	child := registry.New()
	child.Register(parentBash)
	child.Register(shell.NewRunTool(parentBash))
	wrapRegistryForAgentCWD(child, childRoot)

	wrapper, ok := child.Get("Bash").(*agentCWDBashToolWrapper)
	if !ok {
		t.Fatalf("child Bash = %T", child.Get("Bash"))
	}
	childRun, ok := child.Get("Run").(*shell.RunTool)
	if !ok {
		t.Fatalf("child Run = %T", child.Get("Run"))
	}
	if childRun.Bash != wrapper.BashTool || childRun.Bash == parentBash || !childRun.Bash.ForceSandbox {
		t.Fatalf("child Run binding = bash=%p wrapper=%p parent=%p", childRun.Bash, wrapper.BashTool, parentBash)
	}

	inside, err := executeApprovedRegistryToolForTest(t, child, "Run", runReadInput("child.txt"))
	if err != nil || inside.IsError || !strings.Contains(inside.TextContent(), "child") {
		t.Fatalf("child Run result=%+v err=%v", inside, err)
	}
	outsideInput := map[string]any{"steps": []any{
		map[string]any{"id": "escape", "command": map[string]any{"kind": "shell", "script": "printf escaped > " + parentPath}},
	}}
	permission, err := child.CheckToolPermissions(context.Background(), "Run", outsideInput, types.ToolPermissionRequest{})
	if err != nil || permission.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("outside Run permission=%+v err=%v", permission, err)
	}
	content, err := os.ReadFile(parentPath)
	if err != nil || string(content) != "parent" {
		t.Fatalf("parent path changed to %q err=%v", content, err)
	}
}

func TestAgentCWDRunOnlyProfileGetsPrivateBashOrFailsClosed(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	parentBash := &shell.BashTool{CWD: parentRoot, AllowedDirs: []string{parentRoot}, Sandbox: &task07SandboxBackend{}}
	isolated := registry.New()
	isolated.Register(shell.NewRunTool(parentBash))
	wrapRegistryForAgentCWD(isolated, childRoot)
	run, ok := isolated.Get("Run").(*shell.RunTool)
	if !ok {
		t.Fatalf("Run-only child binding = %T", isolated.Get("Run"))
	}
	if run.Bash == parentBash || run.Bash.CurrentCWD() != childRoot || !run.Bash.ForceSandbox {
		t.Fatalf("Run-only child binding = %+v", run)
	}
	if isolated.Get("Bash") != nil {
		t.Fatalf("Run-only profile unexpectedly exposed Bash: %T", isolated.Get("Bash"))
	}

	unsafe := registry.New()
	unsafe.Register(shell.NewRunTool(&shell.BashTool{CWD: parentRoot, AllowedDirs: []string{parentRoot}}))
	wrapRegistryForAgentCWD(unsafe, childRoot)
	if unsafe.Get("Run") != nil {
		t.Fatalf("Run-only child retained unsandboxed execution: %T", unsafe.Get("Run"))
	}
}

func TestAgentRunOnlyPermissionSnapshotDoesNotReuseParentBash(t *testing.T) {
	root := t.TempDir()
	parentBash := &shell.BashTool{CWD: root, AllowedDirs: []string{root}}
	reg := registry.New()
	reg.Register(shell.NewRunTool(parentBash))
	snapshot := types.ToolRuntimeContext{DeniedRules: []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "printf *"}}}
	pinAgentRegistryPermissionRules(reg, snapshot)
	run, ok := reg.Get("Run").(*shell.RunTool)
	if !ok {
		t.Fatalf("permission-pinned Run = %T", reg.Get("Run"))
	}
	if run.Bash == parentBash {
		t.Fatalf("permission-pinned Run retained parent Bash: %+v", run)
	}
	permission, err := run.CheckPermissions(context.Background(), map[string]any{"steps": []any{
		map[string]any{"id": "print", "argv": []any{"printf", "value"}},
	}}, types.ToolPermissionRequest{})
	if err != nil || permission.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("permission-pinned Run decision=%+v err=%v", permission, err)
	}
}

func TestBackgroundAgentBaseFilterRetainsRun(t *testing.T) {
	if !agentToolAllowedByBaseFilters("Run", true, false) {
		t.Fatal("background agent filter removed Run")
	}
}

func TestAgentCWDV2CoreSharesOnlyChildReadEvidence(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	parentPath := filepath.Join(parentRoot, "target.txt")
	childPath := filepath.Join(childRoot, "target.txt")
	if err := os.WriteFile(parentPath, []byte("parent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childPath, []byte("child\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	parentState := toolfile.NewReadFileState()
	parentRuntime := agentRuntimeContextProvider{snapshot: types.ToolRuntimeContext{
		SessionID: "parent-session", ProjectRoot: parentRoot, AllowedDirs: []string{parentRoot}, PermissionMode: "acceptEdits",
	}}
	reg := registry.New()
	reg.SetRuntimeContextProvider(parentRuntime)
	reg.Register(toolinspect.New(parentRuntime, parentState))
	reg.Register(&toolfile.ApplyPatchTool{
		AllowedDirs: []string{parentRoot}, Runtime: parentRuntime, ReadState: parentState,
	})

	wrapRegistryForAgentCWD(reg, childRoot)
	inspectResult, err := executeApprovedRegistryToolForTest(t, reg, "Inspect", map[string]any{
		"requests": []any{map[string]any{"id": "source", "kind": "read", "path": "target.txt"}},
	})
	if err != nil || inspectResult.IsError {
		t.Fatalf("child Inspect result=%+v err=%v", inspectResult, err)
	}
	if _, found := parentState.GetForContext(context.Background(), childPath); found {
		t.Fatal("child Inspect wrote evidence into the parent ledger")
	}

	patchResult, err := executeApprovedRegistryToolForTest(t, reg, "ApplyPatch", map[string]any{"patch": strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: target.txt",
		"@@",
		"-child",
		"+updated",
		"*** End Patch",
	}, "\n")})
	if err != nil || patchResult.IsError {
		t.Fatalf("child ApplyPatch did not consume child Inspect evidence: result=%+v err=%v", patchResult, err)
	}
	if content, readErr := os.ReadFile(childPath); readErr != nil || string(content) != "updated\n" {
		t.Fatalf("child content=%q err=%v", content, readErr)
	}
	if content, readErr := os.ReadFile(parentPath); readErr != nil || string(content) != "parent\n" {
		t.Fatalf("parent content=%q err=%v", content, readErr)
	}
}

func runReadInput(path string) map[string]any {
	argv := []any{"cat", path}
	if path == "pwd" {
		argv = []any{"pwd"}
	}
	return map[string]any{"steps": []any{map[string]any{"id": "read", "command": map[string]any{"kind": "argv", "args": argv}}}}
}
