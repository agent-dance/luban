package tools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type task23TeamSkillProvider struct {
	mu    sync.Mutex
	steps [][]types.StreamEvent
	calls []provider.Params
}

func (p *task23TeamSkillProvider) Name() string    { return "task23-team-skill" }
func (p *task23TeamSkillProvider) ModelID() string { return "task23-team-model" }

func (p *task23TeamSkillProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	index := len(p.calls)
	params.Messages = append([]types.Message(nil), params.Messages...)
	p.calls = append(p.calls, params)
	var events []types.StreamEvent
	if index < len(p.steps) {
		events = append([]types.StreamEvent(nil), p.steps[index]...)
	}
	p.mu.Unlock()
	stream := make(chan types.StreamEvent, len(events))
	go func() {
		defer close(stream)
		for _, event := range events {
			select {
			case <-ctx.Done():
				return
			case stream <- event:
			}
		}
	}()
	return stream, nil
}

func (p *task23TeamSkillProvider) Calls() []provider.Params {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Params(nil), p.calls...)
}

func TestTeamChildUsesStableIsolatedSkillSessionForCatalogAndExecution(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "task23-team-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: team child session\n---\nteam body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(skills.DirSource{Dir: root, Source: skills.SourceProject})
	const parentSession = "task23-team-parent"
	snapshot, err := manager.Snapshot(parentSession)
	if err != nil || len(snapshot.Skills) != 1 {
		t.Fatalf("team skill snapshot = %+v, err=%v", snapshot.Skills, err)
	}
	row := snapshot.Skills[0]
	if changed, found := manager.SetEnabled(parentSession, row.Name, false); !changed || !found {
		t.Fatalf("disable parent overlay = changed %t, found %t", changed, found)
	}

	provider := &task23TeamSkillProvider{steps: [][]types.StreamEvent{
		task23TeamSkillToolEvents("team-skill-1", string(row.ID), row.Revision),
		task23TeamSkillToolEvents("team-skill-2", string(row.ID), row.Revision),
		task23TeamSkillTextEvents("team skill done"),
	}}
	skillTool := NewSkillTool()
	skillTool.Manager = manager
	skillTool.SessionIDResolver = func(ctx context.Context) string {
		if exec, ok := loop.ToolExecutionContextFromContext(ctx); ok {
			return exec.SessionID
		}
		return ""
	}
	skillTool.LoadedLedgerResolver = func(ctx context.Context, sessionID string, id skills.SkillID) SkillLoadedLedgerState {
		exec, ok := loop.ToolExecutionContextFromContext(ctx)
		if !ok || exec.SessionID != sessionID {
			return SkillLoadedLedgerState{}
		}
		state, resolved := exec.ResolveSkillLoadedLedger(id)
		if !resolved {
			return SkillLoadedLedgerState{}
		}
		return SkillLoadedLedgerState{
			ContextEpoch: state.ContextEpoch, LoadedContextEpoch: state.LoadedContextEpoch,
			ContentDigest: state.ContentDigest, PayloadDigest: state.PayloadDigest,
		}
	}

	mgr := newTestManagerForHome(t, t.TempDir())
	mgr.Provider = provider
	mgr.Registry = registry.New()
	mgr.Registry.Register(skillTool)
	mgr.System = "task23 team system"
	mgr.CWD = root
	mgr.SkillManager = manager
	mgr.SetSessionRuntime(TeamSessionRuntime{
		System: "task23 team system", SessionID: parentSession, CWD: root,
		ToolRuntime: types.ToolRuntimeContext{SessionID: parentSession, ProjectRoot: root, AllowedDirs: []string{root}},
	})

	create, err := NewTeamCreateTool(mgr).Execute(context.Background(), map[string]any{
		"team_name": "task23-team-runtime", "agent_type": "executor",
	})
	if err != nil || create.IsError {
		t.Fatalf("create team = %+v, err=%v", create, err)
	}
	mgr.coordinator.AddTask("exercise team child skill", 0)
	results := mgr.coordinator.Dispatch(context.Background())
	if len(results) != 1 || results[0].Error != nil || results[0].Result != "team skill done" {
		t.Fatalf("team dispatch results = %+v", results)
	}

	const childSession = "team-lead@task23-team-runtime"
	calls := provider.Calls()
	if len(calls) != 3 {
		t.Fatalf("team provider calls = %d, want 3", len(calls))
	}
	for index, call := range calls {
		if call.PromptCacheKey != parentSession {
			t.Fatalf("team call %d cache key = %q, want inherited parent lineage %q", index, call.PromptCacheKey, parentSession)
		}
	}
	first := task23TeamSkillResult(calls[1].Messages, "team-skill-1")
	second := task23TeamSkillResult(calls[2].Messages, "team-skill-2")
	if first.Metadata["envelopeKind"] != string(skills.InvocationEnvelopeFull) ||
		second.Metadata["envelopeKind"] != string(skills.InvocationEnvelopeAlreadyLoaded) {
		t.Fatalf("team envelope kinds = first %q second %q", first.Metadata["envelopeKind"], second.Metadata["envelopeKind"])
	}
	if first.Metadata["sessionID"] != childSession || second.Metadata["sessionID"] != childSession {
		t.Fatalf("team execution sessions = first %q second %q, want %q",
			first.Metadata["sessionID"], second.Metadata["sessionID"], childSession)
	}
	if manager.IsEnabled(parentSession, row.Name) || !manager.IsEnabled(childSession, row.Name) {
		t.Fatalf("team catalog/execution overlay mismatch: parent=%t child=%t",
			manager.IsEnabled(parentSession, row.Name), manager.IsEnabled(childSession, row.Name))
	}
}

func TestTask23AgentAndTeamChildrenRejectRetargetedSkillAuthority(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeTask23TeamSkill(t, rootA, "authority-a")
	writeTask23TeamSkill(t, rootB, "authority-b")
	overrides, err := skills.NewFileOverrideStore(rootA, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManagerWithOverrideStore(overrides, skills.DirSource{Dir: rootA, Source: skills.SourceProject})
	bindingA, err := manager.SnapshotBinding("task23-parent-authority")
	if err != nil {
		t.Fatal(err)
	}

	// Agent children persist the generation captured at their launch boundary.
	// Build the child while A is authoritative, then retarget before its first
	// Run. It must fail before making any provider call or observing B.
	agentProvider := &task23TeamSkillProvider{}
	agentRegistry := registry.New()
	agent := &AgentTool{
		Provider: agentProvider, Registry: agentRegistry, SkillManager: manager,
		System: "task23 agent generation fence",
	}
	agent.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{
		SessionID: "task23-parent-authority", ProjectRoot: rootA, AllowedDirs: []string{rootA},
	}})
	bundle, err := agent.buildSubAgentLoopWithOptions("task23-old-agent", AgentInput{
		Prompt: "inspect the workspace",
	}, agentLoopOptions{
		Profile:                &agentProfile{Name: "general-purpose"},
		SkillProjectGeneration: bindingA.ProjectGeneration,
	})
	if err != nil {
		t.Fatalf("build old-authority agent: %v", err)
	}
	defer runAgentCleanup(bundle.Cleanup)

	if err := manager.ReplaceProjectSources(rootB); err != nil {
		t.Fatalf("retarget manager to B: %v", err)
	}
	if err := bundle.Loop.Run(context.Background(), "inspect", func(loop.Event) {}); !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
		t.Fatalf("old-authority agent error = %v, want generation fence", err)
	}
	if calls := agentProvider.Calls(); len(calls) != 0 {
		t.Fatalf("old-authority agent reached provider after retarget: %d calls", len(calls))
	}

	// TeamCreate captures the parent QueryLoop's private generation capability.
	// The lead closure is created under A, then dispatched only after B becomes
	// authoritative. The child must reject before provider sampling.
	if err := manager.ReplaceProjectSources(rootA); err != nil {
		t.Fatalf("restore manager to A: %v", err)
	}
	teamProvider := &task23TeamSkillProvider{steps: [][]types.StreamEvent{
		task23GenericTeamToolEvents("task23-team-create", "TeamCreate", map[string]any{
			"team_name": "task23-old-team", "agent_type": "executor",
		}),
		task23TeamSkillTextEvents("team created"),
	}}
	teamRegistry := registry.New()
	teamManager := newTestManagerForHome(t, t.TempDir())
	teamManager.Provider = teamProvider
	teamManager.Registry = teamRegistry
	teamManager.System = "task23 team generation fence"
	teamManager.CWD = rootA
	teamManager.SkillManager = manager
	teamManager.SetSessionRuntime(TeamSessionRuntime{
		System: "task23 team generation fence", SessionID: "task23-parent-authority", CWD: rootA,
		ToolRuntime: types.ToolRuntimeContext{SessionID: "task23-parent-authority", ProjectRoot: rootA, AllowedDirs: []string{rootA}},
	})
	teamRegistry.Register(NewTeamCreateTool(teamManager))
	parentLoop := loop.New(teamProvider, teamRegistry, loop.Config{
		MaxTurns: 3, SessionID: "task23-parent-authority", ProjectRoot: rootA, CWD: rootA,
		SkillManager: manager,
	})
	if err := parentLoop.Run(context.Background(), "create the team", func(loop.Event) {}); err != nil {
		t.Fatalf("create team under A: %v", err)
	}
	if err := manager.ReplaceProjectSources(rootB); err != nil {
		t.Fatalf("retarget manager to B for team: %v", err)
	}
	teamManager.coordinator.AddTask("inspect the retargeted workspace", 0)
	results := teamManager.coordinator.Dispatch(context.Background())
	if len(results) != 1 || !errors.Is(results[0].Error, skills.ErrSkillProjectGenerationChanged) {
		t.Fatalf("old-authority team results = %+v, want generation fence", results)
	}
	if calls := teamProvider.Calls(); len(calls) != 2 {
		t.Fatalf("old-authority team reached child provider: calls=%d want parent-only 2", len(calls))
	}
}

func TestTask23BackgroundFollowUpKeepsOriginSessionProjectDirAcrossRetarget(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := NewBackgroundTaskManager(rootA)
	t.Cleanup(manager.Shutdown)

	const sessionID = "task23-background-session"
	const sessionProjectDir = "-Users-task23-origin"
	started := make(chan struct{})
	release := make(chan struct{})
	parent := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		SessionID: sessionID, SessionProjectDir: sessionProjectDir, ProjectRoot: rootA, CWD: rootA,
	})
	snapshot, err := manager.StartAgentTask(parent, "inspect A", "origin workspace task", func(context.Context, io.Writer) (string, error) {
		close(started)
		<-release
		return "origin result", nil
	})
	if err != nil {
		t.Fatalf("start origin background task: %v", err)
	}
	<-started
	manager.SetProjectRoot(rootB)
	close(release)
	if _, status := manager.Wait(snapshot.ID, 5*time.Second); status != "success" {
		t.Fatalf("background status = %q", status)
	}
	record, ok := NewRuntimeTaskStore(rootA).Get(snapshot.ID)
	if !ok || record.Notification == nil {
		t.Fatalf("origin notification record missing: %+v", record)
	}
	target, ok := manager.NotificationFollowUpTarget(*record.Notification)
	if !ok || target.SessionID != sessionID || target.SessionProjectDir != sessionProjectDir || filepath.Clean(target.ProjectRoot) != filepath.Clean(rootA) {
		t.Fatalf("retargeted follow-up target = %+v ok=%v", target, ok)
	}
}

func TestTask23ProfilePreloadUsesSessionVisibilityAndPinnedGeneration(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeTask23TeamSkill(t, filepath.Join(rootA, ".luban-code", "skills"), "profile-skill")
	writeTask23TeamSkill(t, filepath.Join(rootB, ".luban-code", "skills"), "profile-skill-b")
	sessionLayer := skills.NewMemorySessionOverrideLayer()
	store, err := skills.NewFileOverrideStore(rootA, nil, sessionLayer)
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := skills.ProjectDirs(rootA)
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManagerWithOverrideStore(store, dirs...)
	const sessionID = "task23-profile-session"
	binding, err := manager.SnapshotBinding(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	var row skills.EffectiveSkill
	found := false
	for _, candidate := range binding.Snapshot.Skills {
		if candidate.Name == "profile-skill" && candidate.ShadowedBy == "" {
			row, found = candidate, true
			break
		}
	}
	if !found {
		t.Fatalf("profile skill missing: %+v", binding.Snapshot.Skills)
	}
	authority := toolSkillAuthority{sessionID: sessionID, generation: binding.ProjectGeneration, pinned: true}

	if _, found, err := resolveProfileSkill(manager, authority, row.Name); err != nil || !found {
		t.Fatalf("same-generation profile preload = found %v err %v", found, err)
	}
	lastNonOff := skills.VisibilityAuto
	if _, err := manager.SetVisibility(sessionID, skills.VisibilityOverride{
		SkillID: row.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityOff,
		LastNonOff: &lastNonOff,
	}); err != nil {
		t.Fatal(err)
	}
	if skill, found, err := resolveProfileSkill(manager, authority, row.Name); err != nil || found || skill != nil {
		t.Fatalf("session-off profile preload = skill %+v found %v err %v", skill, found, err)
	}
	if err := manager.ReplaceProjectSources(rootB); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveProfileSkill(manager, authority, row.Name); !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
		t.Fatalf("old-generation profile preload error = %v", err)
	}
}

func writeTask23TeamSkill(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("---\ndescription: " + name + "\n---\n" + name + " body\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func task23GenericTeamToolEvents(id, name string, input map[string]any) []types.StreamEvent {
	encoded, _ := json.Marshal(input)
	stop := types.StopReasonToolUse
	return []types.StreamEvent{
		{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: id, Name: name}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(encoded)}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop},
	}
}

func task23TeamSkillToolEvents(id, skillID string, revision skills.SkillRevision) []types.StreamEvent {
	input, _ := json.Marshal(map[string]any{"skill": skillID, "revision": uint64(revision)})
	stop := types.StopReasonToolUse
	return []types.StreamEvent{
		{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: id, Name: "Skill"}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(input)}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop},
	}
}

func task23TeamSkillTextEvents(text string) []types.StreamEvent {
	stop := types.StopReasonEndTurn
	return []types.StreamEvent{
		{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop},
	}
}

func task23TeamSkillResult(messages []types.Message, toolUseID string) types.ToolResultBlock {
	for _, message := range messages {
		for _, content := range message.Content {
			if result, ok := content.(types.ToolResultBlock); ok && result.ToolUseID == toolUseID {
				return result
			}
		}
	}
	return types.ToolResultBlock{}
}
