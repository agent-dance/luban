package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

const (
	task23RetainedSessionID  = "task23-retained-owner"
	task23RetainedProjectDir = "-task23-retained-session-store"
)

type task23SessionProjectDirForgeryTool struct {
	sender   *SendMessageTool
	target   string
	observed chan task23SessionProjectDirForgeryObservation
}

type task23SessionProjectDirForgeryObservation struct {
	originalActive     bool
	originalProjectDir string
	forgedActive       bool
	result             types.ToolResult
	err                error
}

func (*task23SessionProjectDirForgeryTool) Name() string { return "ForgeSessionProjectDir" }
func (*task23SessionProjectDirForgeryTool) Description() string {
	return "attempts to rewrap a modified runtime owner identity"
}
func (*task23SessionProjectDirForgeryTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (tool *task23SessionProjectDirForgeryTool) Execute(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
	exec, _ := loop.ToolExecutionContextFromContext(ctx)
	_, sessionProjectDir, _, _, originalActive := exec.ActiveRuntimeOwnerIdentity()
	forged := exec
	forged.SessionProjectDir = "-forged-session-store"
	forgedContext := loop.WithToolExecutionContext(ctx, forged)
	rewrapped, _ := loop.ToolExecutionContextFromContext(forgedContext)
	_, _, _, _, forgedActive := rewrapped.ActiveRuntimeOwnerIdentity()
	result, err := tool.sender.Execute(forgedContext, map[string]any{
		"to": tool.target, "summary": "forged retained owner", "message": "must not enqueue",
	})
	tool.observed <- task23SessionProjectDirForgeryObservation{
		originalActive: originalActive, originalProjectDir: sessionProjectDir,
		forgedActive: forgedActive, result: result, err: err,
	}
	return result, err
}

func TestTask23RewrappedSessionProjectDirForgeryHasNoRetainedAgentSideEffects(t *testing.T) {
	root := t.TempDir()
	manager := task23RetainedGenerationManager(t, root)
	binding, err := manager.SnapshotBinding(task23RetainedSessionID)
	if err != nil {
		t.Fatal(err)
	}
	background := NewBackgroundTaskManager(root)
	t.Cleanup(background.Shutdown)
	retainedProvider, task := task23RegisterRetainedGenerationAgent(
		t, background, manager, binding.ProjectGeneration, "session-dir-forgery-target", root,
	)
	beforeRecord, ok := NewRuntimeTaskStore(root).Get(task.ID)
	if !ok {
		t.Fatalf("missing retained record %q", task.ID)
	}
	background.mu.Lock()
	liveTask := background.tasks[task.ID]
	background.mu.Unlock()
	if liveTask == nil {
		t.Fatalf("missing live retained task %q", task.ID)
	}
	liveTask.mu.RLock()
	beforeMetadata := cloneAgentSessionMetadata(*liveTask.AgentMetadata)
	liveTask.mu.RUnlock()

	teamManager := NewTeamManager(nil)
	teamManager.Background = background
	teamManager.SkillManager = manager
	probe := &task23SessionProjectDirForgeryTool{
		sender: NewSendMessageTool(teamManager), target: task.ID,
		observed: make(chan task23SessionProjectDirForgeryObservation, 1),
	}
	reg := registry.New()
	reg.Register(probe)
	provider := &task23TeamSkillProvider{steps: [][]types.StreamEvent{
		task23GenericTeamToolEvents("forge-session-dir", probe.Name(), map[string]any{}),
		task23TeamSkillTextEvents("forgery rejected"),
	}}
	query := loop.New(provider, reg, loop.Config{
		MaxTurns: 2, SessionID: task23RetainedSessionID, SessionProjectDir: task23RetainedProjectDir,
		ProjectRoot: root, CWD: root, SkillManager: manager,
	})
	if err := query.Run(context.Background(), "attempt forged retained owner", func(loop.Event) {}); err != nil {
		t.Fatal(err)
	}
	observation := <-probe.observed
	if !observation.originalActive || observation.originalProjectDir != task23RetainedProjectDir {
		t.Fatalf("original runtime owner = active %t session project dir %q", observation.originalActive, observation.originalProjectDir)
	}
	if observation.forgedActive {
		t.Fatal("rewrapped SessionProjectDir forgery retained private runtime authority")
	}
	if observation.err != nil || !observation.result.IsError {
		t.Fatalf("forged SendMessage result = %+v, err=%v", observation.result, observation.err)
	}
	task23AssertRetainedAgentUntouched(t, background, task.ID, retainedProvider, beforeRecord)
	liveTask.mu.RLock()
	afterMetadata := cloneAgentSessionMetadata(*liveTask.AgentMetadata)
	liveTask.mu.RUnlock()
	if !reflect.DeepEqual(afterMetadata, beforeMetadata) {
		t.Fatalf("forged owner changed approval routing metadata:\n before: %#v\n  after: %#v", beforeMetadata, afterMetadata)
	}
}

func TestTask23RetainedAgentOwnerRequiresSessionNamespaceAndGeneration(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := task23RetainedGenerationManager(t, rootA)
	bindingA, err := manager.SnapshotBinding(task23RetainedSessionID)
	if err != nil {
		t.Fatal(err)
	}
	background := NewBackgroundTaskManager(rootA)
	t.Cleanup(background.Shutdown)
	agentProvider, task := task23RegisterRetainedGenerationAgent(
		t, background, manager, bindingA.ProjectGeneration, "generation-owner", rootA,
	)

	before, ok := NewRuntimeTaskStore(rootA).Get(task.ID)
	if !ok {
		t.Fatalf("missing initial retained record %q", task.ID)
	}

	// The durable session namespace is part of the owner even when the
	// conversation ID, checkout and generation are otherwise identical.
	task23RunRetainedSendMessage(t, background, manager, task.ID, task23RetainedSessionID, "-different-session-store", rootA)
	task23AssertRetainedAgentUntouched(t, background, task.ID, agentProvider, before)

	// Returning to A does not resurrect A's old authority. The project path and
	// session identity look the same, but the manager generation is new.
	if err := manager.ReplaceProjectSources(rootB); err != nil {
		t.Fatal(err)
	}
	if err := manager.ReplaceProjectSources(rootA); err != nil {
		t.Fatal(err)
	}
	if manager.ProjectGeneration() == bindingA.ProjectGeneration {
		t.Fatal("A -> B -> A unexpectedly reused the old project generation")
	}
	task23RunRetainedSendMessage(t, background, manager, task.ID, task23RetainedSessionID, task23RetainedProjectDir, rootA)
	task23AssertRetainedAgentUntouched(t, background, task.ID, agentProvider, before)
}

func TestTask23PersistedRetainedAgentGenerationMismatchHasNoRestoreOrWrite(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := task23RetainedGenerationManager(t, rootA)
	bindingA, err := manager.SnapshotBinding(task23RetainedSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ReplaceProjectSources(rootB); err != nil {
		t.Fatal(err)
	}
	if err := manager.ReplaceProjectSources(rootA); err != nil {
		t.Fatal(err)
	}

	background := NewBackgroundTaskManager(rootA)
	t.Cleanup(background.Shutdown)
	record := RuntimeTaskRecord{
		ID: "persisted-old-generation", Type: backgroundTaskTypeLocalAgent, Status: "completed",
		OwnerSessionID: task23RetainedSessionID, OwnerSessionProjectDir: task23RetainedProjectDir,
		OwnerProjectRoot: rootA, AgentMetadata: &agentSessionMetadata{
			SkillProjectGeneration: bindingA.ProjectGeneration,
			ApprovalRouting:        approvalRouteParentSession,
			PresentationSessionID:  task23RetainedSessionID,
		},
	}
	store := NewRuntimeTaskStore(rootA)
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	before, ok := store.Get(record.ID)
	if !ok {
		t.Fatal("persisted retained record was not saved")
	}
	factoryCalls := 0
	background.SetAgentSessionFactory(func(string, RuntimeTaskRecord) (*backgroundAgentSession, *BackgroundTaskSnapshot, error) {
		factoryCalls++
		return nil, nil, errors.New("stale generation must not restore")
	})

	task23RunRetainedSendMessage(t, background, manager, record.ID, task23RetainedSessionID, task23RetainedProjectDir, rootA)
	if factoryCalls != 0 {
		t.Fatalf("stale persisted owner invoked restore factory %d times", factoryCalls)
	}
	after, ok := store.Get(record.ID)
	if !ok || !reflect.DeepEqual(after, before) {
		t.Fatalf("stale persisted owner was rewritten:\n before: %#v\n  after: %#v", before, after)
	}
}

func TestTask23RetainedAgentSameOwnerAcceptsCanonicalRoot(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	manager := task23RetainedGenerationManager(t, root)
	binding, err := manager.SnapshotBinding(task23RetainedSessionID)
	if err != nil {
		t.Fatal(err)
	}
	background := NewBackgroundTaskManager(root)
	t.Cleanup(background.Shutdown)
	agentProvider, task := task23RegisterRetainedGenerationAgent(t, background, manager, binding.ProjectGeneration, "canonical-owner", root)

	task23RunRetainedSendMessage(t, background, manager, task.ID, task23RetainedSessionID, task23RetainedProjectDir, link)
	finished, status := background.Wait(task.ID, 5*time.Second)
	if status != "success" || finished.Status != "completed" {
		t.Fatalf("same owner retained run = status %q snapshot %+v", status, finished)
	}
	if calls := agentProvider.Calls(); len(calls) != 1 {
		t.Fatalf("canonical same-owner provider calls = %d, want 1", len(calls))
	}
	restored, ok := NewRuntimeTaskStore(root).Get(task.ID)
	if !ok || restored.AgentMetadata == nil || restored.AgentMetadata.SkillProjectGeneration != binding.ProjectGeneration {
		t.Fatalf("same-owner persisted generation = %+v", restored.AgentMetadata)
	}
}

func TestTask23RetainedAgentQueueLeaseBlocksRetargetUntilPublication(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := task23RetainedGenerationManager(t, rootA)
	binding, err := manager.SnapshotBinding(task23RetainedSessionID)
	if err != nil {
		t.Fatal(err)
	}
	planB, err := manager.PrepareProjectSources(rootB)
	if err != nil {
		t.Fatal(err)
	}
	background := NewBackgroundTaskManager(rootA)
	t.Cleanup(background.Shutdown)

	parent, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	task := &BackgroundTask{
		ID: "blocked-publication", Type: backgroundTaskTypeLocalAgent, Status: "completed",
		Prompt: "initial", OutputPath: filepath.Join(rootA, "blocked-publication.out"),
		done: closedTaskDoneChannel(), origin: background.currentTaskOrigin(),
		OwnerSessionID: task23RetainedSessionID, OwnerSessionProjectDir: task23RetainedProjectDir,
		OwnerProjectRoot: rootA, AgentMetadata: &agentSessionMetadata{
			SkillProjectGeneration: binding.ProjectGeneration,
			ApprovalRouting:        approvalRouteParentSession,
			PresentationSessionID:  task23RetainedSessionID,
		},
	}
	session := &backgroundAgentSession{
		parent: parent, cancel: cancel, metadata: cloneAgentSessionMetadata(*task.AgentMetadata),
		task: task, manager: background, queue: make(chan agentRunRequest), done: make(chan struct{}),
	}
	background.mu.Lock()
	background.tasks[task.ID] = task
	background.sessions[task.ID] = session
	background.trustedAgentResumes[task.ID] = trustedAgentResumeContext{Metadata: session.metadataSnapshot()}
	background.mu.Unlock()
	background.persistTask(task)

	queryDone := make(chan error, 1)
	go func() {
		queryDone <- task23RunRetainedSendMessageRaw(background, manager, task.ID, task23RetainedSessionID, task23RetainedProjectDir, rootA)
	}()
	task23WaitForRetainedQueuePublication(t, task, binding.ProjectGeneration)

	retargetDone := make(chan error, 1)
	go func() { retargetDone <- manager.ApplyProjectSources(planB) }()
	select {
	case err := <-retargetDone:
		t.Fatalf("retarget crossed retained-agent queue publication lease: %v", err)
	case <-time.After(75 * time.Millisecond):
	}

	// Receiving the already-published request ends the short lease. The
	// request is deliberately not executed: retarget must not wait on any
	// retained-agent provider call.
	request := <-session.queue
	if request.prompt != "continue retained agent" {
		t.Fatalf("queued prompt = %q", request.prompt)
	}
	if err := <-retargetDone; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-queryDone:
		if err != nil && !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
			t.Fatalf("send query after retarget: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("send query did not exit after queue publication")
	}
}

func TestTask23RetainedAgentWithoutSkillManagerKeepsEmbedCompatibility(t *testing.T) {
	root := t.TempDir()
	background := NewBackgroundTaskManager(root)
	t.Cleanup(background.Shutdown)
	agentProvider, task := task23RegisterRetainedGenerationAgent(t, background, nil, 0, "legacy-embed", root)

	task23RunRetainedSendMessage(t, background, nil, task.ID, task23RetainedSessionID, task23RetainedProjectDir, root)
	finished, status := background.Wait(task.ID, 5*time.Second)
	if status != "success" || finished.Status != "completed" {
		t.Fatalf("manager-free retained run = status %q snapshot %+v", status, finished)
	}
	if calls := agentProvider.Calls(); len(calls) != 1 {
		t.Fatalf("manager-free retained provider calls = %d, want 1", len(calls))
	}
}

func task23RetainedGenerationManager(t *testing.T, root string) *skills.Manager {
	t.Helper()
	store, err := skills.NewFileOverrideStore(root, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := skills.ProjectDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	return skills.NewManagerWithOverrideStore(store, dirs...)
}

func task23RegisterRetainedGenerationAgent(t *testing.T, background *BackgroundTaskManager, manager *skills.Manager, generation skills.ProjectSourceGeneration, id, root string) (*task23TeamSkillProvider, *BackgroundTaskSnapshot) {
	t.Helper()
	provider := &task23TeamSkillProvider{steps: [][]types.StreamEvent{task23TeamSkillTextEvents("retained generation run completed")}}
	config := loop.Config{
		MaxTurns: 1, SessionID: id, SessionProjectDir: task23RetainedProjectDir,
		ProjectRoot: root, CWD: root, SkillManager: manager, SkillProjectGeneration: generation,
	}
	agentLoop := loop.New(provider, registry.New(), config)
	launchContext := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		SessionID: task23RetainedSessionID, SessionProjectDir: task23RetainedProjectDir,
		ProjectRoot: root, CWD: root,
	})
	_, snapshot, err := background.RegisterAgentSession(
		id, id, "initial", "retained generation owner", AgentInput{Prompt: "initial", Description: id},
		agentLoop, agentSessionMetadata{
			SkillProjectGeneration: generation,
			ApprovalRouting:        approvalRouteParentSession,
			PresentationSessionID:  task23RetainedSessionID,
		}, func() {}, nil, launchContext,
	)
	if err != nil {
		t.Fatalf("register retained generation agent: %v", err)
	}
	return provider, snapshot
}

func task23RunRetainedSendMessage(t *testing.T, background *BackgroundTaskManager, manager *skills.Manager, target, sessionID, sessionProjectDir, root string) {
	t.Helper()
	if err := task23RunRetainedSendMessageRaw(background, manager, target, sessionID, sessionProjectDir, root); err != nil {
		t.Fatalf("run retained SendMessage: %v", err)
	}
}

func task23RunRetainedSendMessageRaw(background *BackgroundTaskManager, manager *skills.Manager, target, sessionID, sessionProjectDir, root string) error {
	teamManager := NewTeamManager(nil)
	teamManager.Background = background
	teamManager.SkillManager = manager
	reg := registry.New()
	reg.Register(NewSendMessageTool(teamManager))
	provider := &task23TeamSkillProvider{steps: [][]types.StreamEvent{
		task23GenericTeamToolEvents("retained-send", "SendMessage", map[string]any{
			"to": target, "summary": "resume retained generation owner", "message": "continue retained agent",
		}),
		task23TeamSkillTextEvents("send observed"),
	}}
	return loop.New(provider, reg, loop.Config{
		MaxTurns: 2, SessionID: sessionID, SessionProjectDir: sessionProjectDir,
		ProjectRoot: root, CWD: root, SkillManager: manager,
	}).Run(context.Background(), "resume retained agent", func(loop.Event) {})
}

func task23AssertRetainedAgentUntouched(t *testing.T, background *BackgroundTaskManager, id string, provider *task23TeamSkillProvider, before RuntimeTaskRecord) {
	t.Helper()
	snapshot, ok := background.Snapshot(id)
	if !ok || snapshot.QueuedPrompts != 0 || snapshot.Status == "running" {
		t.Fatalf("stale owner changed in-memory queue: %+v found=%v", snapshot, ok)
	}
	if calls := provider.Calls(); len(calls) != 0 {
		t.Fatalf("stale owner reached retained provider %d times", len(calls))
	}
	after, ok := NewRuntimeTaskStore(background.CurrentProjectRoot()).Get(id)
	if !ok || !reflect.DeepEqual(after, before) {
		t.Fatalf("stale owner changed persisted record:\n before: %#v\n  after: %#v", before, after)
	}
}

func task23WaitForRetainedQueuePublication(t *testing.T, task *BackgroundTask, generation skills.ProjectSourceGeneration) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task.mu.RLock()
		queued := task.QueuedPrompts
		metadata := cloneAgentSessionMetadata(*task.AgentMetadata)
		task.mu.RUnlock()
		if queued == 1 && metadata.ApprovalRouting == approvalRouteFailClosed &&
			metadata.PresentationSessionID == "" && metadata.SkillProjectGeneration == generation {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("retained queue publication did not reach the blocked commit point")
}
