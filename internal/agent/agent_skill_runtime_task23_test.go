package agent

import (
	"context"
	"errors"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/internal/contracts/stream"

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

func TestTask23AgentChildRejectsRetargetedSkillAuthority(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeTask23TeamSkill(t, rootA, "authority-a")
	writeTask23TeamSkill(t, rootB, "authority-b")
	overrides, err := skills.NewFileOverrideStore(rootA, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(skills.DirSource{Dir: rootA, Source: skills.SourceProject})
	manager.SetOverrideStore(overrides)
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
	bundle, err := agent.buildSubAgentLoopWithOptions("task23-old-agent", agentcontract.Input{
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
	if err := bundle.Loop.Run(context.Background(), "inspect", func(stream.Event) {}); !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
		t.Fatalf("old-authority agent error = %v, want generation fence", err)
	}
	if calls := agentProvider.Calls(); len(calls) != 0 {
		t.Fatalf("old-authority agent reached provider after retarget: %d calls", len(calls))
	}

}

func TestTask23BackgroundFollowUpKeepsOriginSessionProjectDirAcrossRetarget(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := NewBackgroundTaskManager(rootA)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	const sessionID = "task23-background-session"
	const sessionProjectDir = "-Users-task23-origin"
	started := make(chan struct{})
	release := make(chan struct{})
	parent := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
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
	record, ok := runtimestore.NewRuntimeTaskStore(rootA).Get(snapshot.ID)
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
	manager := skills.NewManager(dirs...)
	manager.SetOverrideStore(store)
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
	authority := captureSkillAuthorityForTest(t, manager, sessionID, rootA)

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
