package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	agentruntime "github.com/agent-dance/luban/internal/agent"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/loop"
	toolcollaboration "github.com/agent-dance/luban/internal/tools/collaboration"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

type appCollaborationProvider struct {
	mu    sync.Mutex
	calls int
}

func (*appCollaborationProvider) Name() string    { return "app-collaboration-test" }
func (*appCollaborationProvider) ModelID() string { return "app-collaboration-model" }

func (p *appCollaborationProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	call := p.calls
	p.calls++
	p.mu.Unlock()

	var events []types.StreamEvent
	switch call {
	case 0:
		events = appCollaborationToolEvents("create-team", "TeamCreate", map[string]any{
			"team_name": "app-collaboration",
		})
	case 2:
		events = appCollaborationToolEvents("spawn-teammate", "Agent", map[string]any{
			"description": "build release slice",
			"prompt":      "build the release slice",
			"name":        "Builder",
			"team_name":   "app-collaboration",
		})
	default:
		events = appCollaborationTextEvents("teammate done")
	}
	result := make(chan types.StreamEvent, len(events))
	for _, event := range events {
		result <- event
	}
	close(result)
	return result, nil
}

func appCollaborationToolEvents(id, name string, input map[string]any) []types.StreamEvent {
	encoded, _ := json.Marshal(input)
	stop := types.StopReasonToolUse
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse, ID: id, Name: name,
		}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type: "input_json_delta", PartialJSON: string(encoded),
		}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop},
	}
}

func appCollaborationTextEvents(text string) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
}

func TestSetupRegistryComposesAddressableTeammateLaunch(t *testing.T) {
	t.Setenv("LUBAN_CODE_EXPERIMENTAL_AGENT_TEAMS", "true")
	root := t.TempDir()
	backend := &appCollaborationProvider{}
	ref := provider.NewProviderRef(backend)
	deps := SetupRegistry(ref, root, []string{root}, sandbox.NoopBackend{}, nil)
	// Exercise the non-coding collaboration embedding explicitly; production
	// coding runs stay bound to the exact Inspect/ApplyPatch/Run profile.
	deps.Registry.SetModelToolProfile(registry.ModelToolProfileLegacy)
	t.Cleanup(deps.StopWebFetchCache)
	cleanupBackgroundTaskManager(t, deps.BackgroundTasks)
	if err := prepareInitialRegistryRuntime(deps, root, []string{root}); err != nil {
		t.Fatal(err)
	}

	const sessionID = "app-collaboration-session"
	deps.BindSessionIdentity(sessionID)
	query := loop.New(ref, deps.Registry, loop.Config{
		Model:           backend.ModelID(),
		MaxTurns:        4,
		MaxTokens:       256,
		SessionID:       sessionID,
		ProjectRoot:     root,
		CWD:             root,
		SkillManager:    deps.SkillManager,
		BackgroundTasks: appBackgroundTaskCompactAdapter{source: deps.BackgroundTasks},
	})

	create, ok := deps.Registry.Get("TeamCreate").(*toolcollaboration.TeamCreateTool)
	if !ok || create == nil {
		t.Fatalf("TeamCreate = %T, want app-composed collaboration tool", deps.Registry.Get("TeamCreate"))
	}
	if err := query.Run(context.Background(), "create the collaboration team", func(stream.Event) {}); err != nil {
		t.Fatalf("TeamCreate runtime query: %v", err)
	}
	t.Cleanup(func() { _ = swarm.DeleteTeamConfig("app-collaboration") })
	if deps.TeamManager.CurrentTeamName() != "app-collaboration" {
		t.Fatalf("app-composed TeamCreate did not publish current team: %q", deps.TeamManager.CurrentTeamName())
	}

	var agentResult *types.ToolResultBlock
	if err := query.Run(context.Background(), "spawn the release teammate", func(event stream.Event) {
		if event.Type == stream.EventToolResult && event.ToolResult != nil && event.ToolResult.ToolUseID == "spawn-teammate" {
			copy := *event.ToolResult
			agentResult = &copy
		}
	}); err != nil {
		t.Fatalf("Agent teammate runtime query: %v", err)
	}
	if agentResult == nil || agentResult.IsError {
		t.Fatalf("Agent teammate tool result = %#v", agentResult)
	}
	partial, ok := agentResult.Data.(agentruntime.AgentPartial)
	if !ok {
		t.Fatalf("teammate launch data = %T, want AgentPartial", agentResult.Data)
	}
	if partial.WireStatus != "teammate_spawned" || partial.AgentID != "Builder@app-collaboration" {
		t.Fatalf("teammate launch data = %#v", partial)
	}

	snapshot, status := deps.BackgroundTasks.Wait("Builder@app-collaboration", 2*time.Second)
	if status != "success" || snapshot.Status != "completed" {
		t.Fatalf("retained teammate status=%q snapshot=%#v", status, snapshot)
	}
	if _, ok := deps.BackgroundTasks.ResolveAgentTarget("Builder@app-collaboration"); !ok {
		t.Fatal("app-composed teammate is not addressable by its durable agent ID")
	}
	config, err := swarm.LoadTeamConfig("app-collaboration")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, member := range config.Members {
		if member.AgentID == "Builder@app-collaboration" && member.Name == "Builder" && member.IsActive {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("durable teammate missing from config: %#v", config.Members)
	}
}
