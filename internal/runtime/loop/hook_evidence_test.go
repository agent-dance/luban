package loop

import (
	"context"
	"errors"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestToolHookSummaryInputIsImmutableAfterToolMutation(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Mutate", execute: func(_ context.Context, input map[string]any) (types.ToolResult, error) {
		input["nested"].(map[string]any)["value"] = "mutated-by-tool"
		input["added"] = true
		return types.ToolResult{Content: "done"}, nil
	}})
	runner := hooks.NewRunner([]hooks.Hook{{Type: hooks.HookPreToolUse, Command: "true", Timeout: 5}})
	toolUses := []types.ToolUseBlock{{
		Type: types.ContentTypeToolUse,
		ID:   "tool-immutable-input",
		Name: "Mutate",
		Input: map[string]any{
			"nested": map[string]any{"value": "captured"},
		},
	}}

	result, err := executeToolsConcurrentlyDetailed(context.Background(), reg, runner, nil, "session-input", executioncontract.ToolExecutionContext{
		TurnID: "turn-input", ActorID: "assistant", ActorType: "assistant", WorkUnitID: "work-input",
	}, toolUses, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.HookSummaries) != 1 {
		t.Fatalf("hook summaries = %d, want 1", len(result.HookSummaries))
	}
	input := result.HookSummaries[0].Metadata["hook_input"].(hooks.HookInput)
	nested := input.ToolInput["nested"].(map[string]any)
	if nested["value"] != "captured" {
		t.Fatalf("captured hook input changed to %v", nested["value"])
	}
	if _, ok := input.ToolInput["added"]; ok {
		t.Fatalf("captured hook input gained tool mutation: %v", input.ToolInput)
	}
}

func TestPostToolHookCapturesInputBeforeToolMutation(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Mutate", execute: func(_ context.Context, input map[string]any) (types.ToolResult, error) {
		input["nested"].(map[string]any)["value"] = "mutated-by-tool"
		input["added"] = true
		return types.ToolResult{Content: "done"}, nil
	}})
	runner := hooks.NewRunner([]hooks.Hook{{Type: hooks.HookPostToolUse, Command: "true", Timeout: 5}})

	result, err := executeToolsConcurrentlyDetailed(context.Background(), reg, runner, nil, "session-post-input", executioncontract.ToolExecutionContext{
		TurnID: "turn-post-input", ActorID: "assistant", ActorType: "assistant", WorkUnitID: "work-post-input",
	}, []types.ToolUseBlock{{
		Type: types.ContentTypeToolUse,
		ID:   "tool-post-input",
		Name: "Mutate",
		Input: map[string]any{
			"nested": map[string]any{"value": "captured"},
		},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.HookSummaries) != 1 {
		t.Fatalf("hook summaries = %d, want 1", len(result.HookSummaries))
	}
	input := result.HookSummaries[0].Metadata["hook_input"].(hooks.HookInput)
	nested := input.ToolInput["nested"].(map[string]any)
	if nested["value"] != "captured" {
		t.Fatalf("post-hook captured input changed to %v", nested["value"])
	}
	if _, ok := input.ToolInput["added"]; ok {
		t.Fatalf("post-hook captured input gained tool mutation: %v", input.ToolInput)
	}
}

func TestToolHookSummaryOutputIsImmutableAfterToolMutation(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Mutate", execute: func(_ context.Context, input map[string]any) (types.ToolResult, error) {
		input["nested"].(map[string]any)["value"] = "mutated-by-tool"
		return types.ToolResult{Content: "done"}, nil
	}})
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookPreToolUse,
		Command: `printf '{"modified_input":{"nested":{"value":"captured"}}}'`,
		Timeout: 5,
	}})

	result, err := executeToolsConcurrentlyDetailed(context.Background(), reg, runner, nil, "session-output", executioncontract.ToolExecutionContext{
		TurnID: "turn-output", ActorID: "assistant", ActorType: "assistant", WorkUnitID: "work-output",
	}, []types.ToolUseBlock{{Type: types.ContentTypeToolUse, ID: "tool-immutable-output", Name: "Mutate", Input: map[string]any{}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.HookSummaries) != 1 {
		t.Fatalf("hook summaries = %d, want 1", len(result.HookSummaries))
	}
	output := result.HookSummaries[0].Metadata["hook_output"].(hooks.HookOutput)
	nested := output.ModifiedInput["nested"].(map[string]any)
	if nested["value"] != "captured" {
		t.Fatalf("captured hook output changed to %v", nested["value"])
	}
}

func TestPostToolHookUsesCompleteExecutionContextAndTrueToolID(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Echo", execute: func(_ context.Context, _ map[string]any) (types.ToolResult, error) {
		return types.ToolResult{Content: "done"}, nil
	}})
	runner := hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookPostToolUse, Command: "printf exact-output", Timeout: 5,
		Headers: map[string]string{"Authorization": "Bearer hidden", "X-Evidence": "visible"},
	}})

	result, err := executeToolsConcurrentlyDetailed(context.Background(), reg, runner, nil, "session-from-call", executioncontract.ToolExecutionContext{
		TurnID: "turn-7", ActorID: "agent-7", ActorType: "reviewer", WorkUnitID: "work-7",
	}, []types.ToolUseBlock{{Type: types.ContentTypeToolUse, ID: "true-tool-id", Name: "Echo", Input: map[string]any{}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.HookSummaries) != 1 {
		t.Fatalf("hook summaries = %d, want 1", len(result.HookSummaries))
	}
	summary := result.HookSummaries[0]
	input := summary.Metadata["hook_input"].(hooks.HookInput)
	if input.SessionID != "session-from-call" || input.TurnID != "turn-7" || input.ToolUseID != "true-tool-id" || input.WorkUnitID != "work-7" || input.AgentID != "agent-7" || input.AgentType != "reviewer" {
		t.Fatalf("post-hook context = %+v", input)
	}
	if summary.HookExecutionID != "hook:turn-7:PostToolUse:tool-true-tool-id:config-1:occurrence-1" || summary.ToolUseID != "true-tool-id" {
		t.Fatalf("post-hook identity = %+v", summary)
	}
	if summary.Metadata["config_id"] != "config-1" || summary.Metadata["config_index"] != 1 {
		t.Fatalf("post-hook config identity = %#v", summary.Metadata)
	}
	config := summary.Metadata["hook_config"].(hooks.Hook)
	if config.Headers["Authorization"] != "[REDACTED]" || config.Headers["X-Evidence"] != "visible" {
		t.Fatalf("post-hook config evidence = %#v", config.Headers)
	}
	output := summary.Metadata["hook_output"].(hooks.HookOutput)
	if output.Stdout != "exact-output" || output.StdoutBytes != int64(len("exact-output")) || output.StdoutTruncated {
		t.Fatalf("post-hook raw output evidence = %+v", output)
	}
}

func TestFailedPostToolHookRetainsCompleteIdentity(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Fail", execute: func(context.Context, map[string]any) (types.ToolResult, error) {
		return types.ToolResult{}, errors.New("tool failed")
	}})
	runner := hooks.NewRunner([]hooks.Hook{{Type: hooks.HookPostToolUseFailure, Command: "true", Timeout: 5}})

	result, err := executeToolsConcurrentlyDetailed(context.Background(), reg, runner, nil, "session-failure", executioncontract.ToolExecutionContext{
		TurnID: "turn-failure", ActorID: "agent-failure", ActorType: "reviewer", WorkUnitID: "work-failure",
	}, []types.ToolUseBlock{{Type: types.ContentTypeToolUse, ID: "tool-failure", Name: "Fail", Input: map[string]any{}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.HookSummaries) != 1 {
		t.Fatalf("hook summaries = %d, want 1", len(result.HookSummaries))
	}
	summary := result.HookSummaries[0]
	input := summary.Metadata["hook_input"].(hooks.HookInput)
	if input.SessionID != "session-failure" || input.TurnID != "turn-failure" || input.ToolUseID != "tool-failure" || input.WorkUnitID != "work-failure" || input.AgentID != "agent-failure" || input.AgentType != "reviewer" {
		t.Fatalf("failure hook context = %+v", input)
	}
	if summary.HookExecutionID != "hook:turn-failure:PostToolUseFailure:tool-tool-failure:config-1:occurrence-1" || summary.ToolUseID != "tool-failure" {
		t.Fatalf("failure hook identity = %+v", summary)
	}
}
