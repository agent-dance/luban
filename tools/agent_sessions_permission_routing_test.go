package tools

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type retainedRoutingTestProvider struct {
	once    sync.Once
	started chan struct{}
}

func (p *retainedRoutingTestProvider) Name() string { return "retained-routing-test" }

func (p *retainedRoutingTestProvider) ModelID() string { return "retained-routing-test-model" }

func (p *retainedRoutingTestProvider) CreateStream(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	if p.started != nil {
		p.once.Do(func() { close(p.started) })
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

type retainedRoutingCaptureHandler struct {
	mu       sync.Mutex
	requests []loop.PermissionRequest
}

func (h *retainedRoutingCaptureHandler) Check(_ context.Context, req loop.PermissionRequest) (loop.PermissionDecision, error) {
	h.mu.Lock()
	h.requests = append(h.requests, req)
	h.mu.Unlock()
	return loop.PermissionAllow, nil
}

func (h *retainedRoutingCaptureHandler) lastRequest(t *testing.T) loop.PermissionRequest {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.requests) == 0 {
		t.Fatal("parent permission handler received no requests")
	}
	return h.requests[len(h.requests)-1]
}

type retainedRoutingFixture struct {
	manager      *BackgroundTaskManager
	session      *backgroundAgentSession
	parent       *retainedRoutingCaptureHandler
	permission   types.ToolRuntimeContext
	agentID      string
	providerDone <-chan struct{}
}

func newRetainedRoutingFixture(t *testing.T, agentID string) retainedRoutingFixture {
	t.Helper()
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(manager.Shutdown)
	provider := &retainedRoutingTestProvider{started: make(chan struct{})}
	parent := &retainedRoutingCaptureHandler{}
	permission := types.ToolRuntimeContext{
		SessionID:      "parent-session",
		ProjectRoot:    root,
		AllowedDirs:    []string{root},
		PermissionMode: "bypassPermissions",
		AllowedTools:   map[string]bool{"Read": true},
		DeniedTools:    map[string]bool{"Write": true},
		AllowedRules:   []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "git status"}},
	}
	tool := &AgentTool{
		Provider:          provider,
		Registry:          registry.New(),
		Background:        manager,
		PermissionHandler: parent,
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: permission})
	session, _, err := tool.createRetainedAgentSessionWithOptions(agentID, AgentInput{
		Prompt: "initial retained task",
		Name:   agentID,
	}, agentLoopOptions{
		Profile:               &agentProfile{Name: "general-purpose"},
		ApprovalRouting:       approvalRouteAttached,
		PresentationSessionID: "parent-session",
	})
	if err != nil {
		t.Fatalf("create retained agent session: %v", err)
	}
	initialMetadata := session.metadataSnapshot()
	if initialMetadata.ApprovalRouting != approvalRouteAttached {
		t.Fatalf("fixture route = %q, want attached", initialMetadata.ApprovalRouting)
	}
	return retainedRoutingFixture{
		manager:      manager,
		session:      session,
		parent:       parent,
		permission:   cloneToolRuntimeContext(permission),
		agentID:      agentID,
		providerDone: provider.started,
	}
}

func assertRetainedPromptDetachedRouting(t *testing.T, fixture retainedRoutingFixture) {
	t.Helper()
	session := fixture.session
	detachedMetadata := session.metadataSnapshot()
	if detachedMetadata.ApprovalRouting != approvalRouteFailClosed || detachedMetadata.PresentationSessionID != "" {
		t.Fatalf("retained session route = %q/%q, want fail_closed with no presentation session", detachedMetadata.ApprovalRouting, detachedMetadata.PresentationSessionID)
	}
	if detachedMetadata.PermissionSnapshot == nil || !reflect.DeepEqual(*detachedMetadata.PermissionSnapshot, fixture.permission) {
		t.Fatalf("retained session permission snapshot changed:\n got: %#v\nwant: %#v", detachedMetadata.PermissionSnapshot, fixture.permission)
	}

	session.task.mu.RLock()
	taskMetadata := cloneAgentSessionMetadata(*session.task.AgentMetadata)
	session.task.mu.RUnlock()
	if taskMetadata.ApprovalRouting != approvalRouteFailClosed || taskMetadata.PresentationSessionID != "" {
		t.Fatalf("task metadata route = %q/%q, want fail_closed with no presentation session", taskMetadata.ApprovalRouting, taskMetadata.PresentationSessionID)
	}
	if taskMetadata.PermissionSnapshot == nil || !reflect.DeepEqual(*taskMetadata.PermissionSnapshot, fixture.permission) {
		t.Fatalf("task permission snapshot changed:\n got: %#v\nwant: %#v", taskMetadata.PermissionSnapshot, fixture.permission)
	}

	record, ok := fixture.manager.store.Get(fixture.agentID)
	if !ok || record.AgentMetadata == nil {
		t.Fatalf("persisted retained-agent record not found: %#v", record)
	}
	if record.AgentMetadata.ApprovalRouting != approvalRouteFailClosed || record.AgentMetadata.PresentationSessionID != "" {
		t.Fatalf("persisted route = %q/%q, want fail_closed with no presentation session", record.AgentMetadata.ApprovalRouting, record.AgentMetadata.PresentationSessionID)
	}
	if record.AgentMetadata.PermissionSnapshot == nil || !reflect.DeepEqual(*record.AgentMetadata.PermissionSnapshot, fixture.permission) {
		t.Fatalf("persisted permission snapshot changed:\n got: %#v\nwant: %#v", record.AgentMetadata.PermissionSnapshot, fixture.permission)
	}

	if _, err := session.permissionHandler.Check(context.Background(), loop.PermissionRequest{
		SessionID: "child-session",
		ToolName:  "Read",
	}); err != nil {
		t.Fatalf("check retained-agent permission routing: %v", err)
	}
	req := fixture.parent.lastRequest(t)
	if req.Mode != "bypassPermissions" || !req.AvoidPrompts || req.SessionID != "child-session" {
		t.Fatalf("retained-agent permission request = %#v, want inherited mode with fail-closed routing", req)
	}
}

func TestQueueAgentPromptDetachesRetainedSessionPermissionRouting(t *testing.T) {
	fixture := newRetainedRoutingFixture(t, "agent-queue-detach")

	_, handled, err := fixture.manager.QueueAgentPrompt(fixture.agentID, "continue in background")
	if err != nil || !handled {
		t.Fatalf("QueueAgentPrompt handled=%v err=%v", handled, err)
	}
	assertRetainedPromptDetachedRouting(t, fixture)
}

func TestSendMessageDetachesRetainedSessionPermissionRouting(t *testing.T) {
	fixture := newRetainedRoutingFixture(t, "agent-send-detach")

	result, err := NewSendMessageTool(&TeamManager{Background: fixture.manager}).Execute(context.Background(), map[string]any{
		"to":      fixture.agentID,
		"summary": "continue retained agent",
		"message": "continue in background through SendMessage",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if result.IsError {
		t.Fatalf("SendMessage returned tool error: %+v", result)
	}
	assertRetainedPromptDetachedRouting(t, fixture)
}
