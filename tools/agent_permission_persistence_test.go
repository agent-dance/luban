package tools

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type permissionPersistenceTestProvider struct {
	started chan struct{}
	once    sync.Once
}

func (p *permissionPersistenceTestProvider) Name() string { return "permission-persistence-test" }

func (p *permissionPersistenceTestProvider) ModelID() string {
	return "permission-persistence-test-model"
}

func (p *permissionPersistenceTestProvider) CreateStream(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	if p.started != nil {
		p.once.Do(func() { close(p.started) })
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

type permissionPersistenceCaptureHandler struct {
	mu       sync.Mutex
	requests []loop.PermissionRequest
}

func (h *permissionPersistenceCaptureHandler) Check(_ context.Context, req loop.PermissionRequest) (loop.PermissionDecision, error) {
	h.mu.Lock()
	h.requests = append(h.requests, req)
	h.mu.Unlock()
	return loop.PermissionAllow, nil
}

func (h *permissionPersistenceCaptureHandler) lastRequest(t *testing.T) loop.PermissionRequest {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.requests) == 0 {
		t.Fatal("permission handler received no requests")
	}
	return h.requests[len(h.requests)-1]
}

func permissionPersistenceSnapshot(root, mode string) types.ToolRuntimeContext {
	return types.ToolRuntimeContext{
		SessionID:      "parent-session",
		ProjectRoot:    root,
		AllowedDirs:    []string{root, root + "/shared"},
		Interactive:    true,
		AgentID:        "parent-agent",
		ChannelsActive: true,
		PermissionMode: mode,
		Provider:       "openai",
		Model:          "parent-model",
		Features:       map[string]bool{types.ToolFeatureTeams: true},
		AllowedTools:   map[string]bool{"Read": true},
		DeniedTools:    map[string]bool{"Write": true},
		AllowedRules:   []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "git status"}},
		DeniedRules:    []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "rm -rf /"}},
		AskRules:       []types.PermissionRuleValue{{ToolName: "Write"}},
	}
}

func TestAgentSessionMetadataCloneDeepCopiesPermissionSnapshot(t *testing.T) {
	root := t.TempDir()
	snapshot := permissionPersistenceSnapshot(root, "default")
	metadata := agentSessionMetadata{PermissionSnapshot: &snapshot}

	cloned := cloneAgentSessionMetadata(metadata)
	if cloned.PermissionSnapshot == metadata.PermissionSnapshot {
		t.Fatal("metadata clone retained the original permission snapshot pointer")
	}
	if !reflect.DeepEqual(*cloned.PermissionSnapshot, snapshot) {
		t.Fatalf("cloned permission snapshot = %#v, want %#v", *cloned.PermissionSnapshot, snapshot)
	}

	metadata.PermissionSnapshot.AllowedDirs[0] = "mutated-original"
	metadata.PermissionSnapshot.Features[types.ToolFeatureTeams] = false
	metadata.PermissionSnapshot.AllowedTools["Read"] = false
	metadata.PermissionSnapshot.DeniedTools["Write"] = false
	metadata.PermissionSnapshot.AllowedRules[0].RuleContent = "mutated-original"
	metadata.PermissionSnapshot.DeniedRules[0].RuleContent = "mutated-original"
	metadata.PermissionSnapshot.AskRules[0].ToolName = "mutated-original"

	if cloned.PermissionSnapshot.AllowedDirs[0] != root || !cloned.PermissionSnapshot.Features[types.ToolFeatureTeams] || !cloned.PermissionSnapshot.AllowedTools["Read"] || !cloned.PermissionSnapshot.DeniedTools["Write"] {
		t.Fatalf("cloned permission snapshot followed original slice/map mutations: %#v", *cloned.PermissionSnapshot)
	}
	if cloned.PermissionSnapshot.AllowedRules[0].RuleContent != "git status" || cloned.PermissionSnapshot.DeniedRules[0].RuleContent != "rm -rf /" || cloned.PermissionSnapshot.AskRules[0].ToolName != "Write" {
		t.Fatalf("cloned permission snapshot followed original rule mutations: %#v", *cloned.PermissionSnapshot)
	}

	cloned.PermissionSnapshot.AllowedDirs[1] = "mutated-clone"
	cloned.PermissionSnapshot.Features[types.ToolFeatureTeams] = true
	cloned.PermissionSnapshot.AllowedRules[0].ToolName = "mutated-clone"
	if metadata.PermissionSnapshot.AllowedDirs[1] == "mutated-clone" || metadata.PermissionSnapshot.AllowedRules[0].ToolName == "mutated-clone" {
		t.Fatal("original permission snapshot followed clone mutations")
	}
}

func TestNewAgentSessionMetadataPersistsCompletePermissionSnapshotAndRouting(t *testing.T) {
	root := t.TempDir()
	snapshot := permissionPersistenceSnapshot(root, "bypassPermissions")
	tool := &AgentTool{
		Provider: &permissionPersistenceTestProvider{},
		Registry: registry.New(),
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: snapshot})

	bundle, err := tool.buildSubAgentLoopWithOptions("agent-persist-new", AgentInput{
		Prompt: "inspect",
	}, agentLoopOptions{
		Profile:               &agentProfile{Name: "general-purpose"},
		ApprovalRouting:       approvalRouteParentSession,
		PresentationSessionID: "presentation-session",
	})
	if err != nil {
		t.Fatalf("build subagent loop: %v", err)
	}
	defer runAgentCleanup(bundle.Cleanup)

	if bundle.Metadata.PermissionSnapshot == nil {
		t.Fatal("new agent metadata omitted the parent permission snapshot")
	}
	if !reflect.DeepEqual(*bundle.Metadata.PermissionSnapshot, snapshot) {
		t.Fatalf("persisted permission snapshot = %#v, want %#v", *bundle.Metadata.PermissionSnapshot, snapshot)
	}
	if bundle.Metadata.ApprovalRouting != approvalRouteParentSession || bundle.Metadata.PresentationSessionID != "presentation-session" {
		t.Fatalf("persisted approval route = %q/%q", bundle.Metadata.ApprovalRouting, bundle.Metadata.PresentationSessionID)
	}

	snapshot.AllowedDirs[0] = "foreground-mutated"
	snapshot.Features[types.ToolFeatureTeams] = false
	if bundle.Metadata.PermissionSnapshot.AllowedDirs[0] != root || !bundle.Metadata.PermissionSnapshot.Features[types.ToolFeatureTeams] {
		t.Fatalf("persisted permission snapshot followed foreground mutation: %#v", *bundle.Metadata.PermissionSnapshot)
	}
}

func TestRestoreLegacyAgentMetadataWithoutPermissionSnapshotFailsClosed(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(manager.Shutdown)
	parentPermissions := &permissionPersistenceCaptureHandler{}
	provider := &permissionPersistenceTestProvider{}
	tool := &AgentTool{
		Provider:          provider,
		Registry:          registry.New(),
		Background:        manager,
		PermissionHandler: parentPermissions,
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: permissionPersistenceSnapshot(root, "bypassPermissions")})

	record := RuntimeTaskRecord{
		ID:     "agent-legacy-mode",
		Type:   backgroundTaskTypeLocalAgent,
		Status: "completed",
		AgentInput: &AgentInput{
			Prompt:       "continue",
			SubagentType: "general-purpose",
		},
		AgentMetadata: &agentSessionMetadata{
			AgentType:       "general-purpose",
			Provider:        provider.Name(),
			Model:           provider.ModelID(),
			CWD:             root,
			Mode:            "default",
			ApprovalRouting: approvalRouteFailClosed,
		},
	}

	session, _, err := tool.RestoreAgentSessionFromRecord(record.ID, record)
	if !errors.Is(err, errAgentPermissionSnapshotUnavailable) {
		t.Fatalf("legacy restore error = %v, want unavailable snapshot", err)
	}
	if session != nil {
		t.Fatal("legacy record with model-controlled mode must not resume")
	}
	if len(parentPermissions.requests) != 0 {
		t.Fatalf("legacy restore reached parent permissions: %#v", parentPermissions.requests)
	}
}

func TestRestoreAgentRejectsModifiedOrCrossProcessPermissionMetadata(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(manager.Shutdown)
	provider := &permissionPersistenceTestProvider{}
	tool := &AgentTool{Provider: provider, Registry: registry.New(), Background: manager}
	snapshot := permissionPersistenceSnapshot(root, "default")
	record := RuntimeTaskRecord{
		ID: "agent-tampered-resume", Type: backgroundTaskTypeLocalAgent, Status: "completed",
		AgentInput: &AgentInput{Prompt: "continue", SubagentType: "general-purpose"},
		AgentMetadata: &agentSessionMetadata{
			AgentType: "general-purpose", CWD: root, Mode: "default",
			PermissionSnapshot: &snapshot, ApprovalRouting: approvalRouteFailClosed,
		},
	}
	manager.rememberTrustedAgentResume(record.ID, *record.AgentInput, *record.AgentMetadata)

	tampered := record
	tamperedMetadata := cloneAgentSessionMetadata(*record.AgentMetadata)
	tamperedMetadata.PermissionSnapshot.AllowedDirs = []string{"/"}
	tampered.AgentMetadata = &tamperedMetadata
	if _, _, err := tool.RestoreAgentSessionFromRecord(record.ID, tampered); !errors.Is(err, errAgentResumeContextUntrusted) {
		t.Fatalf("tampered resume error = %v", err)
	}

	freshManager := NewBackgroundTaskManager(root)
	t.Cleanup(freshManager.Shutdown)
	freshTool := &AgentTool{Provider: provider, Registry: registry.New(), Background: freshManager}
	if _, _, err := freshTool.RestoreAgentSessionFromRecord(record.ID, record); !errors.Is(err, errAgentResumeContextUntrusted) {
		t.Fatalf("cross-process-style resume error = %v", err)
	}
}

func TestOrdinaryBackgroundAgentDefaultsToFailClosedApprovalRouting(t *testing.T) {
	root := t.TempDir()
	parentPermissions := &permissionPersistenceCaptureHandler{}
	tool := &AgentTool{
		Provider:          &permissionPersistenceTestProvider{},
		Registry:          registry.New(),
		PermissionHandler: parentPermissions,
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: permissionPersistenceSnapshot(root, "default")})
	bundle, err := tool.buildSubAgentLoopWithOptions("agent-background-default-route", AgentInput{
		Prompt:          "inspect",
		RunInBackground: true,
	}, agentLoopOptions{Profile: &agentProfile{Name: "general-purpose"}})
	if err != nil {
		t.Fatal(err)
	}
	defer runAgentCleanup(bundle.Cleanup)
	if bundle.Metadata.ApprovalRouting != approvalRouteFailClosed {
		t.Fatalf("background approval route = %q, want fail_closed", bundle.Metadata.ApprovalRouting)
	}
	if _, err := bundle.PermissionHandler.Check(context.Background(), loop.PermissionRequest{ToolName: "Read"}); err != nil {
		t.Fatal(err)
	}
	if req := parentPermissions.lastRequest(t); req.Mode != permissionModeDefault || !req.AvoidPrompts {
		t.Fatalf("background permission request = %#v", req)
	}
}

func TestSynchronousAgentRoutesApprovalsToCapturedParentSession(t *testing.T) {
	root := t.TempDir()
	parentPermissions := &permissionPersistenceCaptureHandler{}
	tool := &AgentTool{Provider: &permissionPersistenceTestProvider{}, Registry: registry.New(), PermissionHandler: parentPermissions}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: permissionPersistenceSnapshot(root, "default")})
	bundle, err := tool.buildSubAgentLoopWithOptions("agent-sync-route", AgentInput{Prompt: "inspect"}, agentLoopOptions{
		Profile: &agentProfile{Name: "general-purpose"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runAgentCleanup(bundle.Cleanup)
	if bundle.Metadata.ApprovalRouting != approvalRouteAttached || bundle.Metadata.PresentationSessionID != "parent-session" {
		t.Fatalf("sync approval route = %q/%q", bundle.Metadata.ApprovalRouting, bundle.Metadata.PresentationSessionID)
	}
	if _, err := bundle.PermissionHandler.Check(context.Background(), loop.PermissionRequest{SessionID: "child-session", ToolName: "Write"}); err != nil {
		t.Fatal(err)
	}
	req := parentPermissions.lastRequest(t)
	if req.SessionID != "parent-session" || req.ExecutionSessionID != "child-session" || req.AvoidPrompts {
		t.Fatalf("sync approval presentation = %#v", req)
	}
}

func TestSynchronousAgentWithoutPresentationSessionFailsClosed(t *testing.T) {
	handler := agentPermissionHandlerForSnapshot(types.ToolRuntimeContext{PermissionMode: "default"}, &permissionPersistenceCaptureHandler{}, approvalRouteAttached, agentProfile{})
	typed := handler.(*agentPermissionSnapshotHandler)
	if typed.approvalRouting != approvalRouteFailClosed || typed.presentationSessionID != "" {
		t.Fatalf("missing presentation route = %q/%q", typed.approvalRouting, typed.presentationSessionID)
	}
}

func TestApplyAgentPlanApprovalDoesNotMutatePermissionSnapshot(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	snapshot := permissionPersistenceSnapshot(root, "plan")
	sessionMetadata := agentSessionMetadata{
		Mode:               "plan",
		PermissionSnapshot: &snapshot,
		ApprovalRouting:    approvalRouteParentSession,
	}
	taskMetadata := cloneAgentSessionMetadata(sessionMetadata)
	task := &BackgroundTask{
		ID:            "agent-plan-approval",
		Type:          backgroundTaskTypeLocalAgent,
		Status:        "completed",
		done:          closedTaskDoneChannel(),
		AgentMetadata: &taskMetadata,
		origin:        manager.currentTaskOrigin(),
	}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	permissionHandler, ok := agentPermissionHandlerForSnapshot(snapshot, &permissionPersistenceCaptureHandler{}, approvalRouteParentSession, agentProfile{}, "parent-session").(*agentPermissionSnapshotHandler)
	if !ok {
		t.Fatal("permission snapshot handler was not created")
	}
	session := &backgroundAgentSession{
		parent:            parent,
		cancel:            cancel,
		metadata:          cloneAgentSessionMetadata(sessionMetadata),
		task:              task,
		manager:           manager,
		queue:             make(chan agentRunRequest, 1),
		done:              make(chan struct{}),
		permissionHandler: permissionHandler,
	}
	manager.tasks[task.ID] = task
	manager.sessions[task.ID] = session

	beforeSession := session.metadataSnapshot()
	beforeTask := cloneAgentSessionMetadata(*task.AgentMetadata)
	if !manager.ApplyAgentPlanApproval(TeammatePlanApprovalState{
		AgentID:        task.ID,
		Approved:       true,
		PermissionMode: "bypassPermissions",
	}) {
		t.Fatal("plan approval was not applied")
	}

	afterSession := session.metadataSnapshot()
	if !reflect.DeepEqual(afterSession, beforeSession) {
		t.Fatalf("plan approval mutated session metadata:\n got: %#v\nwant: %#v", afterSession, beforeSession)
	}
	if !reflect.DeepEqual(*task.AgentMetadata, beforeTask) {
		t.Fatalf("plan approval mutated persisted task metadata:\n got: %#v\nwant: %#v", *task.AgentMetadata, beforeTask)
	}
	parentCapture := &permissionPersistenceCaptureHandler{}
	session.permissionHandler.parent = parentCapture
	if _, err := session.permissionHandler.Check(context.Background(), loop.PermissionRequest{ToolName: "Write"}); err != nil {
		t.Fatalf("check permission handler after approval: %v", err)
	}
	if req := parentCapture.lastRequest(t); req.Mode != "plan" {
		t.Fatalf("plan approval changed immutable permission mode: %#v", req)
	}
}

func TestAutoBackgroundDetachPersistsFailClosedApprovalRouting(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(manager.Shutdown)
	provider := &permissionPersistenceTestProvider{started: make(chan struct{})}
	parentPermissions := &permissionPersistenceCaptureHandler{}
	tool := &AgentTool{
		Provider:          provider,
		Registry:          registry.New(),
		Background:        manager,
		PermissionHandler: parentPermissions,
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: permissionPersistenceSnapshot(root, "bypassPermissions")})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := tool.runSubAgentWithAutoBackground(ctx, "agent-auto-detach", AgentInput{
		Prompt: "keep running",
	}, 20*time.Millisecond, agentLoopOptions{
		Profile:               &agentProfile{Name: "general-purpose"},
		ApprovalRouting:       approvalRouteParentSession,
		PresentationSessionID: "parent-session",
	})
	if err != nil {
		t.Fatalf("run auto-background agent: %v", err)
	}
	if result.IsError {
		t.Fatalf("auto-background launch returned tool error: %+v", result)
	}
	select {
	case <-provider.started:
	case <-ctx.Done():
		t.Fatalf("auto-background provider did not start: %v", ctx.Err())
	}

	manager.mu.Lock()
	session := manager.sessions["agent-auto-detach"]
	task := manager.tasks["agent-auto-detach"]
	manager.mu.Unlock()
	if session == nil || task == nil {
		t.Fatal("auto-background retained session was not registered")
	}
	detachedMetadata := session.metadataSnapshot()
	if detachedMetadata.ApprovalRouting != approvalRouteFailClosed || detachedMetadata.PresentationSessionID != "" {
		t.Fatalf("detached session route = %q/%q, want fail-closed with no presentation session", detachedMetadata.ApprovalRouting, detachedMetadata.PresentationSessionID)
	}
	task.mu.RLock()
	persisted := cloneAgentSessionMetadata(*task.AgentMetadata)
	task.mu.RUnlock()
	if persisted.ApprovalRouting != approvalRouteFailClosed || persisted.PresentationSessionID != "" {
		t.Fatalf("persisted detached route = %q/%q, want fail-closed with no presentation session", persisted.ApprovalRouting, persisted.PresentationSessionID)
	}

	if _, err := session.permissionHandler.Check(context.Background(), loop.PermissionRequest{ToolName: "Read"}); err != nil {
		t.Fatalf("check detached permission handler: %v", err)
	}
	req := parentPermissions.lastRequest(t)
	if req.Mode != "bypassPermissions" || !req.AvoidPrompts {
		t.Fatalf("detached permission request lost inherited mode or fail-closed routing: %#v", req)
	}
	manager.AbortAgent("agent-auto-detach")
}
