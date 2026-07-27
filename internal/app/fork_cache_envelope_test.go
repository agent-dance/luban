package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

const forkCacheEnvelopeHelperEnv = "LUBAN_FORK_CACHE_ENVELOPE_HELPER"

type forkCacheEnvelopeProvider struct{}

func (forkCacheEnvelopeProvider) Name() string    { return "deepseek" }
func (forkCacheEnvelopeProvider) ModelID() string { return "deepseek-v4-flash" }
func (forkCacheEnvelopeProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	return nil, fmt.Errorf("fork cache envelope provider is inspection-only")
}

type forkCacheEnvelopeSnapshot struct {
	System string                 `json:"system"`
	Tools  []types.ToolDefinition `json:"tools"`
}

func TestForkCacheEnvelopeHelper(t *testing.T) {
	if os.Getenv(forkCacheEnvelopeHelperEnv) != "1" {
		t.Skip("helper process")
	}
	root := os.Getenv("LUBAN_FORK_CACHE_ENVELOPE_ROOT")
	output := os.Getenv("LUBAN_FORK_CACHE_ENVELOPE_OUTPUT")
	sessionID := os.Getenv("LUBAN_FORK_CACHE_ENVELOPE_SESSION")
	if root == "" || output == "" || sessionID == "" {
		t.Fatal("fork cache envelope helper environment is incomplete")
	}

	ref := provider.NewProviderRef(forkCacheEnvelopeProvider{})
	deps := SetupRegistry(ref, root, []string{root}, sandbox.NoopBackend{}, nil, true)
	defer stopScheduleForTest(t, deps)
	defer deps.StopWebFetchCache()
	defer stopMCPRuntimeBridgeForTest(t, deps)
	if err := prepareInitialRegistryRuntime(deps, root, []string{root}); err != nil {
		t.Fatal(err)
	}
	deps.BindSessionIdentity(sessionID)
	workspacePrompt := buildWorkspacePrompt("", deps.Registry, root)
	deps.AgentTool.System = workspacePrompt.system
	payload, err := json.Marshal(forkCacheEnvelopeSnapshot{
		System: workspacePrompt.system,
		Tools:  deps.Registry.VisibleDefinitions(map[string]struct{}{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestForkCacheEnvelopeStableAcrossFreshProcesses(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(t.TempDir(), "source.json")
	secondPath := filepath.Join(t.TempDir(), "fork.json")
	runForkCacheEnvelopeHelper(t, root, firstPath, "source-session")
	runForkCacheEnvelopeHelper(t, root, secondPath, "fork-session")

	read := func(path string) forkCacheEnvelopeSnapshot {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var snapshot forkCacheEnvelopeSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	source := read(firstPath)
	fork := read(secondPath)
	if source.System != fork.System {
		t.Fatal("fresh fork process rebuilt a different system prompt")
	}
	if reflect.DeepEqual(source.Tools, fork.Tools) {
		return
	}
	if len(source.Tools) != len(fork.Tools) {
		t.Fatalf("fresh fork process tool count = %d, source = %d", len(fork.Tools), len(source.Tools))
	}
	for index := range source.Tools {
		if !reflect.DeepEqual(source.Tools[index], fork.Tools[index]) {
			t.Fatalf("fresh fork process changed tool definition at index %d: source=%#v fork=%#v", index, source.Tools[index], fork.Tools[index])
		}
	}
	t.Fatal("fresh fork process changed serialized tool definitions")
}

func runForkCacheEnvelopeHelper(t *testing.T, root, output, sessionID string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestForkCacheEnvelopeHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		forkCacheEnvelopeHelperEnv+"=1",
		"LUBAN_FORK_CACHE_ENVELOPE_ROOT="+root,
		"LUBAN_FORK_CACHE_ENVELOPE_OUTPUT="+output,
		"LUBAN_FORK_CACHE_ENVELOPE_SESSION="+sessionID,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fresh fork envelope helper failed: %v\n%s", err, output)
	}
}
