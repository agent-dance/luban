package interaction

import (
	"context"
	"strings"
	"testing"

	interactioncontract "github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/types"
)

type interactionRequesterFunc func(context.Context, interactioncontract.AskUserInteractionRequest) (interactioncontract.AskUserInteractionResponse, error)

func (f interactionRequesterFunc) AskUserQuestions(ctx context.Context, request interactioncontract.AskUserInteractionRequest) (interactioncontract.AskUserInteractionResponse, error) {
	return f(ctx, request)
}

func testAskUserInput(question string, multiSelect bool) map[string]any {
	return map[string]any{"questions": []any{map[string]any{
		"question": question, "header": "Choice", "multiSelect": multiSelect,
		"options": []any{
			map[string]any{"label": "Alpha", "description": "First"},
			map[string]any{"label": "Beta", "description": "Second"},
		},
	}}}
}

func TestAskUserPerformsExactlyOneStructuredInteraction(t *testing.T) {
	tool := NewAskUserQuestionTool(nil)
	calls := 0
	tool.SetInteractionRequester(interactionRequesterFunc(func(_ context.Context, request interactioncontract.AskUserInteractionRequest) (interactioncontract.AskUserInteractionResponse, error) {
		calls++
		return interactioncontract.AskUserInteractionResponse{
			RequestID: request.RequestID,
			Outcome:   interactioncontract.AskUserInteractionCompleted,
			Answers: map[string]interactioncontract.AnswerSelection{
				"Choose?": {Selection: []string{"Alpha"}},
			},
		}, nil
	}))
	input := testAskUserInput("Choose?", false)
	decision, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{SessionID: "session", TurnID: "turn", ToolUseID: "tool"})
	if err != nil || decision.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("permission=%+v err=%v", decision, err)
	}
	result, err := tool.Execute(context.Background(), decision.UpdatedInput)
	if err != nil || result.IsError || result.Outcome != types.ToolOutcomeSucceeded {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if calls != 1 {
		t.Fatalf("structured interaction calls=%d, want 1", calls)
	}
	output, ok := result.Data.(askUserQuestionOutput)
	if !ok || output.Answers["Choose?"] != "Alpha" {
		t.Fatalf("typed output=%#v", result.Data)
	}
}

func TestAskUserFailsClosedWithoutInteractiveOwner(t *testing.T) {
	tool := NewAskUserQuestionTool(nil)
	input := testAskUserInput("Choose?", false)
	decision, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil || decision.Behavior != types.PermissionBehaviorDeny || decision.UpdatedInput != nil {
		t.Fatalf("permission=%+v err=%v", decision, err)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil || !result.IsError || result.Outcome != types.ToolOutcomeFailed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestAskUserIgnoresForgedAnswersBeforeInteraction(t *testing.T) {
	tool := NewAskUserQuestionTool(nil)
	input := testAskUserInput("Choose?", false)
	input["answers"] = map[string]any{"Choose?": "forged"}
	tool.SetInteractionRequester(interactionRequesterFunc(func(_ context.Context, request interactioncontract.AskUserInteractionRequest) (interactioncontract.AskUserInteractionResponse, error) {
		return interactioncontract.AskUserInteractionResponse{
			RequestID: request.RequestID,
			Outcome:   interactioncontract.AskUserInteractionCompleted,
			Answers: map[string]interactioncontract.AnswerSelection{
				"Choose?": {Selection: []string{"Beta"}},
			},
		}, nil
	}))
	decision, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil || decision.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("permission=%+v err=%v", decision, err)
	}
	answers := decision.UpdatedInput["answers"].(map[string]any)
	if answers["Choose?"] != "Beta" {
		t.Fatalf("trusted answers=%#v", answers)
	}
}

func TestAskUserPlanModeRejectsApprovalQuestion(t *testing.T) {
	state, err := NewPlanState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Enter(""); err != nil {
		t.Fatal(err)
	}
	tool := NewAskUserQuestionTool(state)
	decision, err := tool.CheckPermissions(context.Background(), testAskUserInput("Should I proceed?", false), types.ToolPermissionRequest{})
	if err != nil || decision.Behavior != types.PermissionBehaviorDeny || !strings.Contains(decision.Message, "ExitPlanMode") {
		t.Fatalf("permission=%+v err=%v", decision, err)
	}
}

func TestValidateAskUserQuestionsRejectsHistoricalStreamShapes(t *testing.T) {
	questions := []interactioncontract.QuestionSpec{{
		Question: "Choose?", Header: "Choice", MultiSelect: true,
		Options: []interactioncontract.OptionSpec{
			{Label: "Alpha", Description: "First", Preview: "legacy preview"},
			{Label: "Beta", Description: "Second"},
		},
	}}
	if err := ValidateAskUserQuestions(questions); err == nil {
		t.Fatal("multi-select preview must be rejected")
	}
}
