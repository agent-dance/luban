package engine

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type capturingPermissionContractHandler struct {
	request permission.PermissionRequest
}

type inheritedPermissionContractTool struct {
	handler  permission.PermissionHandler
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
func (t *inheritedPermissionContractTool) SetChildPermissionHandler(handler permission.PermissionHandler) {
	t.handler = handler
}

func (h *capturingPermissionContractHandler) Check(_ context.Context, req permission.PermissionRequest) (permission.PermissionDecision, error) {
	h.request = req
	return permission.PermissionAllowOnce, nil
}

func TestPermissionContractPreservesStructuredDecisionContract(t *testing.T) {
	capture := &capturingPermissionContractHandler{}
	policy := types.PolicyDecision{Disposition: types.PolicyRequiredAsk, Code: "shell.policy.ask.dynamic_target"}
	request := permission.PermissionRequest{
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

	decision, err := capture.Check(context.Background(), request)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if decision != permission.PermissionAllowOnce {
		t.Fatalf("contract decision = %v, want allow once", decision)
	}
	got := capture.request
	if got.SessionID != request.SessionID || got.DecisionID != request.DecisionID || got.ToolUseID != request.ToolUseID {
		t.Fatalf("contract lost stable IDs: %+v", got)
	}
	if got.ActorID != request.ActorID || got.ActorType != request.ActorType || got.WorkUnitID != request.WorkUnitID {
		t.Fatalf("contract lost actor/work-unit identity: %+v", got)
	}
	if got.Kind != request.Kind || got.Action != request.Action || got.Target != request.Target || got.Impact != request.Impact {
		t.Fatalf("contract lost proposed action details: %+v", got)
	}
	if got.RiskReason != request.RiskReason || got.RuleSource != request.RuleSource || got.ApprovalScope != request.ApprovalScope {
		t.Fatalf("contract lost review policy details: %+v", got)
	}
	if !reflect.DeepEqual(got.Choices, request.Choices) || got.Body != request.Body || !reflect.DeepEqual(got.ReviewDetails, request.ReviewDetails) || got.PostMode != request.PostMode {
		t.Fatalf("contract lost choices or full body: %+v", got)
	}
	if !got.Required {
		t.Fatal("contract lost the required-ask bit")
	}
	if !got.Sandboxed || got.SandboxCapability != request.SandboxCapability {
		t.Fatalf("contract lost sandbox capability: %+v", got)
	}
	if got.PolicyDecision == nil || got.PolicyDecision.Code != policy.Code {
		t.Fatalf("contract lost typed policy decision: %#v", got.PolicyDecision)
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
	decision, err := child.handler.Check(context.Background(), permission.PermissionRequest{ToolName: "Bash"})
	if err != nil || decision != permission.PermissionAllowOnce {
		t.Fatalf("propagated child permission decision=%v err=%v, want allow once", decision, err)
	}

	eng.SetPermission(permission.AllowAllHandler{})
	if child.handler == nil {
		t.Fatal("child lost the shared parent permission authority")
	}
	decision, err = child.handler.Check(context.Background(), permission.PermissionRequest{ToolName: "Bash"})
	if err != nil || decision != permission.PermissionAllow {
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
