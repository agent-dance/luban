package loop

import (
	"context"
	"strings"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type permissionLifecycleTool struct {
	name          string
	decision      types.ToolPermissionResult
	executed      bool
	input         map[string]any
	preserveInput bool
	denyValue     string
}

type permissionLifecycleOutput struct{ Value string }

func (t *permissionLifecycleTool) Name() string {
	if t.name != "" {
		return t.name
	}
	return "Lifecycle"
}
func (t *permissionLifecycleTool) Description() string { return "test lifecycle" }
func (t *permissionLifecycleTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *permissionLifecycleTool) CheckPermissions(_ context.Context, input map[string]any, _ types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	if value, _ := input["value"].(string); t.denyValue != "" && value == t.denyValue {
		policy := types.PolicyDecision{Disposition: types.PolicyBlock, Code: "test.policy.block.changed_input"}
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: "changed input denied", PolicyDecision: &policy}, nil
	}
	decision := t.decision
	if t.preserveInput {
		decision.UpdatedInput = input
	}
	return decision, nil
}
func (t *permissionLifecycleTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	t.executed = true
	t.input = input
	value, _ := input["value"].(string)
	return types.ToolResult{Content: `{"value":"legacy"}`, Data: permissionLifecycleOutput{Value: value}}, nil
}
func (t *permissionLifecycleTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, _ := data.(permissionLifecycleOutput)
	return types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: "typed value: " + output.Value,
	}
}

type lifecyclePermissionHandler struct {
	decision permission.PermissionDecision
	called   bool
	request  permission.PermissionRequest
}

func (h *lifecyclePermissionHandler) Check(_ context.Context, request permission.PermissionRequest) (permission.PermissionDecision, error) {
	h.called = true
	h.request = request
	return h.decision, nil
}

func executeLifecycleTool(t *testing.T, tool *permissionLifecycleTool, handler permission.PermissionHandler) types.ToolResultBlock {
	t.Helper()
	reg := registry.New()
	reg.Register(tool)
	results, _, err := executeToolsConcurrently(context.Background(), reg, nil, handler, "session", executioncontract.ToolExecutionContext{}, []types.ToolUseBlock{{
		Type:  types.ContentTypeToolUse,
		ID:    "toolu_1",
		Name:  tool.Name(),
		Input: map[string]any{"value": "original"},
	}}, nil)
	if err != nil {
		t.Fatalf("executeToolsConcurrently: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	return results[0]
}

func TestPermissionLifecycleAllowUsesUpdatedInputWithoutGenericPrompt(t *testing.T) {
	tool := &permissionLifecycleTool{decision: types.ToolPermissionResult{
		Behavior:     types.PermissionBehaviorAllow,
		UpdatedInput: map[string]any{"value": "updated"},
	}}
	handler := &lifecyclePermissionHandler{decision: permission.PermissionDeny}
	result := executeLifecycleTool(t, tool, handler)

	if result.IsError || !tool.executed {
		t.Fatalf("allow decision should execute: %+v", result)
	}
	if handler.called {
		t.Fatal("tool-specific allow should not invoke the generic permission handler")
	}
	if got := tool.input["value"]; got != "updated" {
		t.Fatalf("Execute input = %v, want updated", got)
	}
}

func TestPermissionLifecycleDenyStopsBeforeExecute(t *testing.T) {
	policy := types.PolicyDecision{Disposition: types.PolicyBlock, Code: "shell.policy.block.root"}
	tool := &permissionLifecycleTool{decision: types.ToolPermissionResult{
		Behavior: types.PermissionBehaviorDeny, Message: "blocked by tool policy", PolicyDecision: &policy,
	}}
	handler := &lifecyclePermissionHandler{decision: permission.PermissionAllow}
	result := executeLifecycleTool(t, tool, handler)

	if !result.IsError || result.Content != "blocked by tool policy" {
		t.Fatalf("unexpected deny result: %+v", result)
	}
	if tool.executed || handler.called {
		t.Fatal("deny must stop before generic permission and Execute")
	}
	if decision, ok := result.Data.(types.PolicyDecision); !ok || decision.Code != policy.Code {
		t.Fatalf("deny lost typed policy decision: %#v", result.Data)
	}
}

func TestPermissionLifecycleAskForwardsSuggestions(t *testing.T) {
	suggestions := []types.PermissionUpdate{{
		Type:        types.PermissionUpdateAddRules,
		Destination: types.PermissionDestinationLocalSettings,
		Behavior:    types.PermissionBehaviorAllow,
		Rules:       []types.PermissionRuleValue{{ToolName: "Lifecycle"}},
	}}
	tool := &permissionLifecycleTool{decision: types.ToolPermissionResult{
		Behavior:    types.PermissionBehaviorAsk,
		Message:     "needs confirmation",
		Suggestions: suggestions,
	}}
	handler := &lifecyclePermissionHandler{decision: permission.PermissionAllowOnce}
	result := executeLifecycleTool(t, tool, handler)

	if result.IsError || !tool.executed || !handler.called {
		t.Fatalf("approved ask should execute: %+v", result)
	}
	if handler.request.Message != "needs confirmation" || len(handler.request.Suggestions) != 1 {
		t.Fatalf("permission request lost tool context: %+v", handler.request)
	}
}

func TestPermissionLifecycleAskWithoutHandlerFailsClosed(t *testing.T) {
	policy := types.PolicyDecision{Disposition: types.PolicyRequiredAsk, Code: "shell.policy.ask.dynamic_target"}
	tool := &permissionLifecycleTool{decision: types.ToolPermissionResult{
		Behavior: types.PermissionBehaviorAsk, Message: "needs confirmation", PolicyDecision: &policy,
	}}
	result := executeLifecycleTool(t, tool, nil)
	if !result.IsError || tool.executed {
		t.Fatalf("unhandled ask must fail closed: %+v", result)
	}
	if decision, ok := result.Data.(types.PolicyDecision); !ok || decision.Code != policy.Code {
		t.Fatalf("unhandled ask lost typed policy decision: %#v", result.Data)
	}
}

type typedAnsweringPermissionHandler struct{}

func (typedAnsweringPermissionHandler) Check(_ context.Context, request permission.PermissionRequest) (permission.PermissionDecision, error) {
	request.Input["value"] = "permission UI answer"
	return permission.PermissionAllowOnce, nil
}

func TestPermissionLifecycleAskConsumesCollectedInputAsTypedResult(t *testing.T) {
	tool := &permissionLifecycleTool{
		decision:      types.ToolPermissionResult{Behavior: types.PermissionBehaviorAsk},
		preserveInput: true,
	}
	result := executeLifecycleTool(t, tool, typedAnsweringPermissionHandler{})
	if result.IsError {
		t.Fatalf("unexpected typed lifecycle result: %#v", result)
	}
	output, ok := result.Data.(permissionLifecycleOutput)
	if !ok {
		t.Fatalf("result.Data = %T, want permissionLifecycleOutput", result.Data)
	}
	if output.Value != "permission UI answer" || tool.input["value"] != "permission UI answer" {
		t.Fatalf("collected updatedInput not preserved: %#v", output)
	}
	if !strings.Contains(result.Content, "permission UI answer") || strings.HasPrefix(result.Content, "{") {
		t.Fatalf("model-visible result was not mapped from typed data: %q", result.Content)
	}
}

func TestPermissionLifecycleRechecksHandlerMutatedInputBeforeCommit(t *testing.T) {
	tool := &permissionLifecycleTool{
		decision:      types.ToolPermissionResult{Behavior: types.PermissionBehaviorAsk},
		preserveInput: true,
		denyValue:     "permission UI answer",
	}
	result := executeLifecycleTool(t, tool, typedAnsweringPermissionHandler{})
	if !result.IsError || tool.executed {
		t.Fatalf("handler-mutated denied input executed: result=%#v", result)
	}
	decision, ok := result.Data.(types.PolicyDecision)
	if !ok || decision.Code != "test.policy.block.changed_input" {
		t.Fatalf("final policy recheck was not preserved: %#v", result.Data)
	}
}

func TestBashPermissionLifecycleRejectsAnyHandlerCommandMutation(t *testing.T) {
	tool := &permissionLifecycleTool{
		name:          "Bash",
		decision:      types.ToolPermissionResult{Behavior: types.PermissionBehaviorAsk},
		preserveInput: true,
	}
	result := executeLifecycleTool(t, tool, typedAnsweringPermissionHandler{})
	if !result.IsError || tool.executed {
		t.Fatalf("same-level Bash input mutation reused approval: result=%#v", result)
	}
}

func TestBashPassthroughWithoutPermissionHandlerFailsClosed(t *testing.T) {
	tool := &permissionLifecycleTool{
		name:     "Bash",
		decision: types.ToolPermissionResult{Behavior: types.PermissionBehaviorPassthrough},
	}
	result := executeLifecycleTool(t, tool, nil)
	if !result.IsError || tool.executed {
		t.Fatalf("Bash passthrough executed without permission handler: result=%#v", result)
	}
}
