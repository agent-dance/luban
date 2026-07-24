package tools

import (
	"os"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestAgentCWDCannotExpandParentAllowedDirectories(t *testing.T) {
	parentRoot := t.TempDir()
	outsideRoot := t.TempDir()
	reg := registry.New()
	tool := &AgentTool{
		Provider: &captureAgentProvider{},
		Registry: reg,
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{
		ProjectRoot:    parentRoot,
		AllowedDirs:    []string{parentRoot},
		PermissionMode: "bypassPermissions",
	}})

	bundle, err := tool.buildSubAgentLoopWithOptions("agent-cwd-escape", AgentInput{
		Prompt: "inspect outside",
		CWD:    outsideRoot,
	}, agentLoopOptions{Profile: &agentProfile{Name: "general-purpose"}})
	if err == nil {
		runAgentCleanup(bundle.Cleanup)
		t.Fatal("subagent cwd outside the parent permission scope was accepted")
	}
	if !strings.Contains(err.Error(), "outside the parent permission scope") {
		t.Fatalf("cwd escape error = %v", err)
	}
}

func TestAgentCWDMayNarrowParentAllowedDirectories(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := parentRoot + "/child"
	if err := os.MkdirAll(childRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := registry.New()
	tool := &AgentTool{Provider: &captureAgentProvider{}, Registry: reg}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{
		ProjectRoot: parentRoot,
		AllowedDirs: []string{parentRoot},
	}})

	bundle, err := tool.buildSubAgentLoopWithOptions("agent-cwd-narrow", AgentInput{
		Prompt: "inspect child",
		CWD:    childRoot,
	}, agentLoopOptions{Profile: &agentProfile{Name: "general-purpose"}})
	if err != nil {
		t.Fatalf("narrow child cwd: %v", err)
	}
	defer runAgentCleanup(bundle.Cleanup)
	if got := bundle.Metadata.CWD; got != childRoot {
		t.Fatalf("child cwd = %q, want %q", got, childRoot)
	}
}
