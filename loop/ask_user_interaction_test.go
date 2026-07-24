package loop_test

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/types"
)

type askUserGenericPermissionHandler struct{ called bool }

type askUserInteractiveRuntime struct{}

func (askUserInteractiveRuntime) ToolRuntimeContext() types.ToolRuntimeContext {
	return types.ToolRuntimeContext{SessionID: "session", Interactive: true}
}

func (h *askUserGenericPermissionHandler) Check(context.Context, loop.PermissionRequest) (loop.PermissionDecision, error) {
	h.called = true
	return loop.PermissionDeny, nil
}

func TestAskUserToolUsePerformsExactlyOneStructuredInteraction(t *testing.T) {
	tool := &tools.AskUserQuestionTool{}
	interactions := 0
	tool.SetInteractionRequester(tools.AskUserInteractionRequesterFunc(func(_ context.Context, request tools.AskUserInteractionRequest) (tools.AskUserInteractionResponse, error) {
		interactions++
		if request.SessionID != "session" || request.TurnID != "turn" || request.ToolUseID != "ask-tool" || request.ActorID != "assistant" || request.WorkUnitID != "work" {
			t.Fatalf("AskUser identity = %+v", request)
		}
		return tools.AskUserInteractionResponse{
			RequestID: request.RequestID, Outcome: tools.AskUserInteractionCompleted,
			Answers: map[string]tools.AnswerSelection{"Choose?": {Selection: []string{"Alpha"}}},
		}, nil
	}))
	reg := registry.New()
	reg.SetRuntimeContextProvider(askUserInteractiveRuntime{})
	reg.Register(tool)
	handler := &askUserGenericPermissionHandler{}
	executor := loop.NewStreamingToolExecutor(context.Background(), reg, nil, handler, "session", loop.ToolExecutionContext{
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
