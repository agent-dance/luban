package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolinspect "github.com/agent-dance/luban/internal/agentic/inspect"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestAgentRuntimeBindsInspectAndApplyPatchToChildEvidenceAndRoot(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	parentPath := filepath.Join(parentRoot, "target.txt")
	childPath := filepath.Join(childRoot, "target.txt")
	writeAgentInspectFixture(t, parentPath, "parent\n")
	writeAgentInspectFixture(t, childPath, "child\n")

	parentSnapshot := types.ToolRuntimeContext{
		SessionID: "parent-session", ProjectRoot: parentRoot, AllowedDirs: []string{parentRoot},
	}
	parentRuntime := agentRuntimeContextProvider{snapshot: parentSnapshot, cwd: parentRoot}
	parentState := toolfile.NewReadFileState()
	parentInspect := toolinspect.New(parentRuntime, parentState)
	parentPatch := &toolfile.ApplyPatchTool{Runtime: parentRuntime, ReadState: parentState, AllowedDirs: []string{parentRoot}}
	parentRegistry := registry.New()
	parentRegistry.Register(parentInspect)
	parentRegistry.Register(parentPatch)
	parentRegistry.SetRuntimeContextProvider(parentRuntime)

	childSnapshot := types.ToolRuntimeContext{
		SessionID: "child-session", AgentID: "child-agent", ProjectRoot: childRoot, AllowedDirs: []string{childRoot},
	}
	childRuntime := agentRuntimeContextProvider{snapshot: childSnapshot, agentID: "child-agent", cwd: childRoot}
	childRegistry := parentRegistry.Clone()
	pinRegistryForAgentRuntime(childRegistry, childRuntime, childSnapshot)
	childRegistry.SetRuntimeContextProvider(childRuntime)

	childInspect, ok := childRegistry.Get("Inspect").(*toolinspect.Tool)
	if !ok {
		t.Fatalf("child Inspect = %T", childRegistry.Get("Inspect"))
	}
	childPatch, ok := childRegistry.Get("ApplyPatch").(*toolfile.ApplyPatchTool)
	if !ok {
		t.Fatalf("child ApplyPatch = %T", childRegistry.Get("ApplyPatch"))
	}
	if childPatch.ReadState == nil || childPatch.ReadState == parentState {
		t.Fatalf("child evidence ledger was not isolated: child=%p parent=%p", childPatch.ReadState, parentState)
	}

	read, readErr := childInspect.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{"id": "target", "kind": toolinspect.KindRead, "path": "target.txt"}},
	})
	if readErr != nil || read.IsError {
		t.Fatalf("child Inspect read failed: err=%v result=%+v", readErr, read)
	}
	patched, patchErr := childPatch.Execute(context.Background(), map[string]any{"patch": strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: target.txt",
		"@@",
		"-child",
		"+changed",
		"*** End Patch",
	}, "\n")})
	if patchErr != nil || patched.IsError {
		t.Fatalf("child ApplyPatch did not share Inspect evidence: err=%v result=%+v", patchErr, patched)
	}
	if got, err := os.ReadFile(childPath); err != nil || string(got) != "changed\n" {
		t.Fatalf("child file = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(parentPath); err != nil || string(got) != "parent\n" {
		t.Fatalf("parent file changed through child runtime: %q, err=%v", got, err)
	}
	if _, found := parentState.GetForContext(context.Background(), childPath); found {
		t.Fatal("child Inspect evidence leaked into parent ledger")
	}

	escape, escapeErr := childInspect.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{"id": "escape", "kind": toolinspect.KindRead, "path": parentPath}},
	})
	if escapeErr != nil || escape.IsError {
		t.Fatalf("scoped request should fail in place: err=%v result=%+v", escapeErr, escape)
	}
	escapeOutput := escape.Data.(toolinspect.Result)
	if len(escapeOutput.Requests) != 1 || len(escapeOutput.Requests[0].Errors) == 0 || strings.Contains(escape.Content, "parent\n") {
		t.Fatalf("child Inspect crossed parent workspace: %#v", escapeOutput)
	}
}

func writeAgentInspectFixture(t testing.TB, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
