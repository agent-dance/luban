package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
)

func TestAgenticV2WorkspacePromptUsesExactVisibleCatalog(t *testing.T) {
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)

	workspace := buildWorkspacePrompt("", deps.Registry, t.TempDir())
	if workspace.catalogErr != nil {
		t.Fatal(workspace.catalogErr)
	}
	want := []string{"Inspect", "ApplyPatch", "Run"}
	if got := workspace.toolSnapshot.Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("workspace visible tools = %v, want %v", got, want)
	}
	if !workspace.generated {
		t.Fatal("V2 workspace prompt is not bound to its visible snapshot")
	}
	for _, hiddenGuidance := range []string{"use the Agent tool", "TaskCreate tool"} {
		if strings.Contains(workspace.system, hiddenGuidance) {
			t.Fatalf("workspace prompt advertises hidden tool guidance %q", hiddenGuidance)
		}
	}
}

func TestAgenticV2WorkspacePromptRemainsExactWithRunOnlyExecution(t *testing.T) {
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	deps.RuntimeScope.SetAllowedTools([]string{"Run"})

	workspace := buildWorkspacePrompt("", deps.Registry, t.TempDir())
	if workspace.catalogErr != nil {
		t.Fatal(workspace.catalogErr)
	}
	want := []string{"Inspect", "ApplyPatch", "Run"}
	if got := workspace.toolSnapshot.Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("workspace visible tools = %v, want fixed Agentic V2 contract %v", got, want)
	}
	if deps.Registry.IsToolEnabled(deps.Registry.Get("Inspect")) {
		t.Fatal("Inspect execution unexpectedly bypassed the Run-only allow-list")
	}
}
