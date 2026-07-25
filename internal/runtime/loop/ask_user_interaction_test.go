package loop_test

import (
	"context"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/runtime/loop"
	toolinteraction "github.com/agent-dance/luban/internal/tools/interaction"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type askUserGenericPermissionHandler struct{ called bool }

type askUserInteractiveRuntime struct{}

type askUserInteractionRequesterFunc func(context.Context, interaction.AskUserInteractionRequest) (interaction.AskUserInteractionResponse, error)

func (f askUserInteractionRequesterFunc) AskUserQuestions(ctx context.Context, request interaction.AskUserInteractionRequest) (interaction.AskUserInteractionResponse, error) {
	return f(ctx, request)
}

func (askUserInteractiveRuntime) ToolRuntimeContext() types.ToolRuntimeContext {
	return types.ToolRuntimeContext{SessionID: "session", Interactive: true}
}

func (h *askUserGenericPermissionHandler) Check(context.Context, permission.PermissionRequest) (permission.PermissionDecision, error) {
	h.called = true
	return permission.PermissionDeny, nil
}

func TestAskUserToolUsePerformsExactlyOneStructuredInteraction(t *testing.T) {
	tool := toolinteraction.NewAskUserQuestionTool(nil)
	interactions := 0
	tool.SetInteractionRequester(askUserInteractionRequesterFunc(func(_ context.Context, request interaction.AskUserInteractionRequest) (interaction.AskUserInteractionResponse, error) {
		interactions++
		if request.SessionID != "session" || request.TurnID != "turn" || request.ToolUseID != "ask-tool" || request.ActorID != "assistant" || request.WorkUnitID != "work" {
			t.Fatalf("AskUser identity = %+v", request)
		}
		return interaction.AskUserInteractionResponse{
			RequestID: request.RequestID, Outcome: interaction.AskUserInteractionCompleted,
			Answers: map[string]interaction.AnswerSelection{"Choose?": {Selection: []string{"Alpha"}}},
		}, nil
	}))
	reg := registry.New()
	reg.SetRuntimeContextProvider(askUserInteractiveRuntime{})
	reg.Register(tool)
	handler := &askUserGenericPermissionHandler{}
	executor := loop.NewStreamingToolExecutor(context.Background(), reg, nil, handler, "session", executioncontract.ToolExecutionContext{
		SessionID: "session", TurnID: "turn", ActorID: "assistant", ActorType: "runtime", WorkUnitID: "work",
	})
	executor.AddTool(types.ToolUseBlock{
		Type: types.ContentTypeToolUse, ID: "ask-tool", Name: "AskUserQuestion",
		Input: map[string]any{"questions": []any{map[string]any{
			"question": "Choose?", "header": "Choice", "multiSelect": false,
			"options": []any{map[string]any{"label": "Alpha", "description": "First"}, map[string]any{"label": "Beta", "description": "Second"}},
		}}},
	}, types.Message{})
	results, _, err := executor.RemainingResults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if interactions != 1 {
		t.Fatalf("structured interactions = %d, want 1", interactions)
	}
	if handler.called {
		t.Fatal("AskUser invoked generic permission handler before its structured interaction")
	}
	if len(results.Results) != 1 || results.Results[0].IsError || results.Results[0].Outcome != types.ToolOutcomeSucceeded {
		t.Fatalf("AskUser result = %+v", results.Results)
	}
}
