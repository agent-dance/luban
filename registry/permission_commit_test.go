package registry

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/types"
)

type receiptProbeTool struct {
	executed      int
	firstConsume  approvalcommit.PermissionCommitStatus
	secondConsume approvalcommit.PermissionCommitStatus
}

func (t *receiptProbeTool) Name() string        { return "ReceiptProbe" }
func (t *receiptProbeTool) Description() string { return "receipt probe" }
func (t *receiptProbeTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *receiptProbeTool) CheckPermissions(context.Context, map[string]any, types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	return types.ToolPermissionResult{
		Behavior: types.PermissionBehaviorAsk, ExecutionPolicyCode: "probe.policy",
	}, nil
}
func (t *receiptProbeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	t.executed++
	t.firstConsume = approvalcommit.Consume(ctx, t.Name(), input, "probe.policy")
	t.secondConsume = approvalcommit.Consume(ctx, t.Name(), input, "probe.policy")
	return types.ToolResult{Content: "ok"}, nil
}

func TestPermissionCommitRequiresPromotionAndToolConsumesReceiptOnce(t *testing.T) {
	reg := New()
	tool := &receiptProbeTool{}
	reg.Register(tool)
	input := map[string]any{"value": "approved"}
	request := types.ToolPermissionRequest{
		SessionID: "session", TurnID: "turn", ToolUseID: "tool-use", ApprovalEpoch: "epoch",
	}
	preflight, err := reg.CheckToolPermissions(context.Background(), tool.Name(), input, request)
	if err != nil || preflight.PermissionGrant == "" {
		t.Fatalf("preflight=%#v err=%v", preflight, err)
	}
	unapproved := approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: preflight.PermissionGrant, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	})
	result, err := reg.ExecuteToolWithError(unapproved, tool.Name(), input)
	if err != nil || !result.IsError || tool.executed != 0 {
		t.Fatalf("preflight executed: result=%#v calls=%d err=%v", result, tool.executed, err)
	}

	preflight, err = reg.CheckToolPermissions(context.Background(), tool.Name(), input, request)
	if err != nil {
		t.Fatal(err)
	}
	executionGrant := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, tool.Name(), input, preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	)
	approved := approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: executionGrant, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	})
	result, err = reg.ExecuteToolWithError(approved, tool.Name(), input)
	if err != nil || result.IsError || tool.executed != 1 || tool.firstConsume != approvalcommit.PermissionCommitValid || tool.secondConsume != approvalcommit.PermissionCommitInvalid {
		t.Fatalf("receipt lifecycle: result=%#v tool=%#v err=%v", result, tool, err)
	}
	replay, err := reg.ExecuteToolWithError(approved, tool.Name(), input)
	if err != nil || !replay.IsError || tool.executed != 1 {
		t.Fatalf("execution grant replayed: result=%#v calls=%d err=%v", replay, tool.executed, err)
	}
}

func TestPermissionCommitTriStateRejectsPresentInvalidReceipt(t *testing.T) {
	ctx := approvalcommit.Bind(context.Background(), "ReceiptProbe", map[string]any{"value": "approved"}, "probe.policy")
	if status := approvalcommit.Consume(ctx, "ReceiptProbe", map[string]any{"value": "changed"}, "probe.policy"); status != approvalcommit.PermissionCommitInvalid {
		t.Fatalf("invalid receipt status=%v, want invalid", status)
	}
	if status := approvalcommit.Consume(ctx, "ReceiptProbe", map[string]any{"value": "changed"}, "probe.policy"); status != approvalcommit.PermissionCommitInvalid {
		t.Fatalf("consumed invalid receipt status=%v, want invalid", status)
	}
	if status := approvalcommit.Consume(context.Background(), "ReceiptProbe", nil, "probe.policy"); status != approvalcommit.PermissionCommitAbsent {
		t.Fatalf("absent receipt status=%v, want absent", status)
	}
}

func TestPermissionGrantCannotAuthorizeDifferentInputThanPreflight(t *testing.T) {
	reg := New()
	tool := &receiptProbeTool{}
	reg.Register(tool)
	request := types.ToolPermissionRequest{
		SessionID: "session", TurnID: "turn", ToolUseID: "tool-use", ApprovalEpoch: "epoch",
	}
	input := map[string]any{"value": "reviewed"}
	preflight, err := reg.CheckToolPermissions(context.Background(), tool.Name(), input, request)
	if err != nil {
		t.Fatal(err)
	}
	if token := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, tool.Name(), map[string]any{"value": "changed"},
		preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	); token != "" {
		t.Fatalf("changed input was authorized with token %q", token)
	}
}

func TestAuthorizePermissionGrantBurnsNonceOnEveryFailure(t *testing.T) {
	reg := New()
	tool := &receiptProbeTool{}
	reg.Register(tool)
	request := types.ToolPermissionRequest{
		SessionID: "session", TurnID: "turn", ToolUseID: "tool-use", ApprovalEpoch: "epoch",
	}
	input := map[string]any{"value": "reviewed"}
	preflight, err := reg.CheckToolPermissions(context.Background(), tool.Name(), input, request)
	if err != nil {
		t.Fatal(err)
	}
	badBinding := preflight.PermissionBinding
	badBinding.ToolUseID = "other"
	if token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, tool.Name(), input, badBinding, preflight.ExecutionPolicyCode); token != "" {
		t.Fatalf("invalid binding authorized token %q", token)
	}
	if token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, tool.Name(), input, preflight.PermissionBinding, preflight.ExecutionPolicyCode); token != "" {
		t.Fatalf("failed authorization did not burn nonce: %q", token)
	}
}

func TestAuthorizePermissionGrantBurnsNonceBeforeRuntimeAndInputValidation(t *testing.T) {
	runtime := &runtimeContextStub{ctx: types.ToolRuntimeContext{SessionID: "session"}}
	reg := New()
	reg.SetRuntimeContextProvider(runtime)
	tool := &receiptProbeTool{}
	reg.Register(tool)
	input := map[string]any{"value": "reviewed"}
	request := types.ToolPermissionRequest{SessionID: "session", TurnID: "turn", ToolUseID: "tool-use", ApprovalEpoch: "epoch"}
	issue := func(t *testing.T) types.ToolPermissionResult {
		t.Helper()
		preflight, err := reg.CheckToolPermissions(context.Background(), tool.Name(), input, request)
		if err != nil || preflight.PermissionGrant == "" {
			t.Fatalf("preflight=%#v err=%v", preflight, err)
		}
		return preflight
	}

	preflight := issue(t)
	runtime.ctx.SessionID = "other"
	if token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, tool.Name(), input, preflight.PermissionBinding, preflight.ExecutionPolicyCode); token != "" {
		t.Fatalf("stale runtime authorized token %q", token)
	}
	runtime.ctx.SessionID = "session"
	if token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, tool.Name(), input, preflight.PermissionBinding, preflight.ExecutionPolicyCode); token != "" {
		t.Fatalf("runtime failure did not burn nonce: %q", token)
	}

	preflight = issue(t)
	if token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, tool.Name(), map[string]any{"bad": func() {}}, preflight.PermissionBinding, preflight.ExecutionPolicyCode); token != "" {
		t.Fatalf("unhashable input authorized token %q", token)
	}
	if token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, tool.Name(), input, preflight.PermissionBinding, preflight.ExecutionPolicyCode); token != "" {
		t.Fatalf("input-digest failure did not burn nonce: %q", token)
	}

	preflight = issue(t)
	incomplete := preflight.PermissionBinding
	incomplete.TurnID = ""
	if token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, tool.Name(), input, incomplete, preflight.ExecutionPolicyCode); token != "" {
		t.Fatalf("incomplete binding authorized token %q", token)
	}
	if token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, tool.Name(), input, preflight.PermissionBinding, preflight.ExecutionPolicyCode); token != "" {
		t.Fatalf("binding failure did not burn nonce: %q", token)
	}
}

func TestPermissionGrantBindsRegistryToolGeneration(t *testing.T) {
	reg := New()
	oldTool := &receiptProbeTool{}
	reg.Register(oldTool)
	input := map[string]any{"value": "reviewed"}
	request := types.ToolPermissionRequest{
		SessionID: "session", TurnID: "turn", ToolUseID: "tool-use", ApprovalEpoch: "epoch",
	}
	preflight, err := reg.CheckToolPermissions(context.Background(), oldTool.Name(), input, request)
	if err != nil {
		t.Fatal(err)
	}
	reg.Register(&receiptProbeTool{})
	if token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, oldTool.Name(), input, preflight.PermissionBinding, preflight.ExecutionPolicyCode); token != "" {
		t.Fatalf("old tool generation authorized token %q", token)
	}

	preflight, err = reg.CheckToolPermissions(context.Background(), oldTool.Name(), input, request)
	if err != nil {
		t.Fatal(err)
	}
	token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, oldTool.Name(), input, preflight.PermissionBinding, preflight.ExecutionPolicyCode)
	reg.Register(&receiptProbeTool{})
	result, err := reg.ExecuteToolWithError(approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: token, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	}), oldTool.Name(), input)
	if err != nil || !result.IsError {
		t.Fatalf("execution grant crossed tool generation: result=%#v err=%v", result, err)
	}
}

func TestPermissionGrantExplicitlyBindsSeparateExecutionAndPolicyOwnerSessions(t *testing.T) {
	reg := New()
	reg.SetRuntimeContextProvider(&runtimeContextStub{ctx: types.ToolRuntimeContext{SessionID: "session-a"}})
	tool := &receiptProbeTool{}
	reg.Register(tool)
	result, err := reg.CheckToolPermissions(context.Background(), tool.Name(), map[string]any{"value": "reviewed"}, types.ToolPermissionRequest{
		SessionID: "session-b", TurnID: "turn", ToolUseID: "tool-use", ApprovalEpoch: "epoch",
	})
	if err != nil || result.PermissionGrant == "" || result.PermissionBinding.SessionID != "session-b" || result.PermissionBinding.PolicyOwnerSessionID != "session-a" {
		t.Fatalf("execution/policy owners were not explicitly separated: result=%#v err=%v", result, err)
	}
}
