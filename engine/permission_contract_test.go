package engine

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type capturingPermissionContractHandler struct {
	request PermissionRequest
}

type inheritedPermissionContractTool struct {
	handler  loop.PermissionHandler
	executed bool
}

func (*inheritedPermissionContractTool) Name() string { return "InheritedPermissionContract" }
func (*inheritedPermissionContractTool) Description() string {
	return "captures the child permission handler"
}
func (*inheritedPermissionContractTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *inheritedPermissionContractTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	t.executed = true
	return types.ToolResult{}, nil
}
func (t *inheritedPermissionContractTool) SetChildPermissionHandler(handler loop.PermissionHandler) {
	t.handler = handler
}

func (h *capturingPermissionContractHandler) Check(_ context.Context, req PermissionRequest) (PermissionDecision, error) {
	h.request = req
	return PermissionAllowOnce, nil
}

func TestLoopPermissionAdapterPreservesStructuredDecisionContract(t *testing.T) {
	capture := &capturingPermissionContractHandler{}
	adapter := AsLoopPermissionHandler(capture)
	policy := types.PolicyDecision{Disposition: types.PolicyRequiredAsk, Code: "shell.policy.ask.dynamic_target"}
	request := loop.PermissionRequest{
		SessionID:         "session-42",
		DecisionID:        "decision:session-42:toolu-7",
		ToolUseID:         "toolu-7",
		ToolName:          "Write",
		Input:             map[string]any{"file_path": "/workspace/out.txt"},
		ActorID:           "agent-writer",
		ActorType:         "executor",
		WorkUnitID:        "work-output",
		Kind:              "permission",
		Action:            "Write a file",
		Target:            "/workspace/out.txt",
		Impact:            "Replaces the current file contents",
		RiskReason:        "Existing data can be overwritten",
		RuleSource:        "project policy: protected outputs",
		ApprovalScope:     "this invocation",
		Choices:           []string{"allow_once", "reject", "always_allow"},
		Body:              "full review body",
		ReviewDetails:     []string{"Allowed prompts: Bash(run tests)"},
		PostMode:          "acceptEdits",
		Required:          true,
		Sandboxed:         true,
		SandboxCapability: "capability-digest",
		PolicyDecision:    &policy,
	}

	decision, err := adapter.Check(context.Background(), request)
	if err != nil {
		t.Fatalf("adapter.Check: %v", err)
	}
	if decision != loop.PermissionAllowOnce {
		t.Fatalf("adapter decision = %v, want allow once", decision)
	}
	got := capture.request
	if got.SessionID != request.SessionID || got.DecisionID != request.DecisionID || got.ToolUseID != request.ToolUseID {
		t.Fatalf("adapter lost stable IDs: %+v", got)
	}
	if got.ActorID != request.ActorID || got.ActorType != request.ActorType || got.WorkUnitID != request.WorkUnitID {
		t.Fatalf("adapter lost actor/work-unit identity: %+v", got)
	}
	if got.Kind != request.Kind || got.Action != request.Action || got.Target != request.Target || got.Impact != request.Impact {
		t.Fatalf("adapter lost proposed action details: %+v", got)
	}
	if got.RiskReason != request.RiskReason || got.RuleSource != request.RuleSource || got.ApprovalScope != request.ApprovalScope {
		t.Fatalf("adapter lost review policy details: %+v", got)
	}
	if !reflect.DeepEqual(got.Choices, request.Choices) || got.Body != request.Body || !reflect.DeepEqual(got.ReviewDetails, request.ReviewDetails) || got.PostMode != request.PostMode {
		t.Fatalf("adapter lost choices or full body: %+v", got)
	}
	if !got.Required {
		t.Fatal("adapter lost the required-ask bit")
	}
	if !got.Sandboxed || got.SandboxCapability != request.SandboxCapability {
		t.Fatalf("adapter lost sandbox capability: %+v", got)
	}
	if got.PolicyDecision == nil || got.PolicyDecision.Code != policy.Code {
		t.Fatalf("adapter lost typed policy decision: %#v", got.PolicyDecision)
	}

	request.Choices[0] = "mutated-after-check"
	if capture.request.Choices[0] == request.Choices[0] {
		t.Fatal("adapter retained the caller's mutable choices slice")
	}
	request.ReviewDetails[0] = "mutated-after-check"
	if capture.request.ReviewDetails[0] == request.ReviewDetails[0] {
		t.Fatal("adapter retained the caller's mutable review-details slice")
	}
}

func TestCoreEnginePermissionChangesPropagateToChildAgentTools(t *testing.T) {
	reg := registry.New()
	child := &inheritedPermissionContractTool{}
	reg.Register(child)
	eng, err := New(Config{
		Provider: &mockProvider{name: "permission-propagation", modelID: "test-model"},
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })

	eng.SetPermission(&capturingPermissionContractHandler{})
	if child.handler == nil {
		t.Fatal("engine permission change did not reach child-agent tool")
	}
	decision, err := child.handler.Check(context.Background(), loop.PermissionRequest{ToolName: "Bash"})
	if err != nil || decision != loop.PermissionAllowOnce {
		t.Fatalf("propagated child permission decision=%v err=%v, want allow once", decision, err)
	}

	eng.SetPermission(AllowAllHandler{})
	if child.handler == nil {
		t.Fatal("child lost the shared parent permission authority")
	}
	decision, err = child.handler.Check(context.Background(), loop.PermissionRequest{ToolName: "Bash"})
	if err != nil || decision != loop.PermissionAllow {
		t.Fatalf("full-auto propagated child decision=%v err=%v, want allow", decision, err)
	}
}

func TestCoreEnginePermissionChangesReachExistingParentConversation(t *testing.T) {
	reg := registry.New()
	tool := &inheritedPermissionContractTool{}
	reg.Register(tool)
	provider := &mockProvider{
		name:    "existing-permission-conversation",
		modelID: "test-model",
		responses: [][]types.StreamEvent{
			textEvents("ready"),
			toolCallEvents("toolu_permission_ref", tool.Name(), map[string]any{}),
			textEvents("done"),
		},
	}
	eng, err := New(Config{Provider: provider, Registry: reg, Sessions: newMemorySessionManager()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })

	first, err := eng.Query(context.Background(), QueryRequest{SessionID: "permission-ref", Message: "create conversation"})
	if err != nil {
		t.Fatalf("first Query: %v", err)
	}
	drainEvents(t, first, time.Second)

	eng.SetPermission(denyAllHandler{})
	second, err := eng.Query(context.Background(), QueryRequest{SessionID: "permission-ref", Message: "try tool"})
	if err != nil {
		t.Fatalf("second Query: %v", err)
	}
	drainEvents(t, second, time.Second)
	if tool.executed {
		t.Fatal("existing parent conversation kept the pre-change permission handler")
	}
}
