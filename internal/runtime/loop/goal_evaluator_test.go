package loop

import (
	"context"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/types"
)

type goalEvaluatorContractStub struct {
	request GoalEvaluationRequest
	result  GoalEvaluationResult
}

func (s *goalEvaluatorContractStub) Evaluate(_ context.Context, request GoalEvaluationRequest) (GoalEvaluationResult, error) {
	s.request = request
	return s.result, nil
}

func (s *goalEvaluatorContractStub) GoalEvaluatorForModel(string) GoalEvaluator { return s }

var _ GoalEvaluator = (*goalEvaluatorContractStub)(nil)

func TestGoalEvaluatorContractReturnsDecisionReasonAndUsage(t *testing.T) {
	wantUsage := &types.Usage{InputTokens: 21, OutputTokens: 3}
	stub := &goalEvaluatorContractStub{result: GoalEvaluationResult{
		Reason: "the integration test has not run",
		Usage:  wantUsage,
	}}
	request := GoalEvaluationRequest{
		Objective: "finish the requested change",
		Messages:  []types.Message{types.UserMessage("start")},
	}

	var evaluator GoalEvaluator = stub
	got, err := evaluator.Evaluate(context.Background(), request)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !reflect.DeepEqual(stub.request, request) {
		t.Fatalf("request = %#v, want %#v", stub.request, request)
	}
	if got.Reason != "the integration test has not run" {
		t.Fatalf("Reason = %q", got.Reason)
	}
	if got.Usage != wantUsage {
		t.Fatalf("Usage = %#v, want original usage pointer %#v", got.Usage, wantUsage)
	}
}

func TestGoalEvaluatorRequestExposesGoalRevisionCriteriaAndMessages(t *testing.T) {
	assertExactStructFields(t, reflect.TypeOf(GoalEvaluationRequest{}), []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "Objective", typeOf: reflect.TypeOf("")},
		{name: "Revision", typeOf: reflect.TypeOf(0)},
		{name: "AcceptanceCriteria", typeOf: reflect.TypeOf([]goal.AcceptanceCriterion(nil))},
		{name: "Messages", typeOf: reflect.TypeOf([]types.Message(nil))},
	})
}

func TestGoalEvaluatorResultExposesPerCriterionResults(t *testing.T) {
	assertExactStructFields(t, reflect.TypeOf(GoalEvaluationResult{}), []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "Criteria", typeOf: reflect.TypeOf([]GoalCriterionEvaluationResult(nil))},
		{name: "Reason", typeOf: reflect.TypeOf("")},
		{name: "Usage", typeOf: reflect.TypeOf((*types.Usage)(nil))},
	})
}

func TestGoalEvaluatorRequestClonesMessagesWithoutDroppingMetadata(t *testing.T) {
	original := []types.Message{{
		ID:     "assistant-message-1",
		Role:   types.RoleAssistant,
		IsMeta: true,
		Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "analysis complete"},
			types.ToolUseBlock{
				Type:  types.ContentTypeToolUse,
				ID:    "tool-use-1",
				Name:  "Read",
				Input: map[string]any{"file_path": "README.md"},
			},
		},
	}}

	request := newGoalEvaluationRequest(goal.Goal{
		Objective: "verify the transcript", Status: goal.StatusActive, Revision: 1,
		AcceptanceCriteria: []goal.AcceptanceCriterion{{ID: "AC-1", Text: "the transcript is verified"}},
	}, original)
	if request.Objective != "verify the transcript" {
		t.Fatalf("Objective = %q", request.Objective)
	}
	if len(request.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(request.Messages))
	}
	got := request.Messages[0]
	if got.ID != "assistant-message-1" || got.Role != types.RoleAssistant || !got.IsMeta {
		t.Fatalf("message metadata was not preserved: %#v", got)
	}
	if !reflect.DeepEqual(got.Content, original[0].Content) {
		t.Fatalf("message content = %#v, want %#v", got.Content, original[0].Content)
	}

	original[0].ID = "mutated"
	original[0].Role = types.RoleUser
	original[0].IsMeta = false
	original[0].Content[0] = types.TextBlock{Type: types.ContentTypeText, Text: "mutated"}

	got = request.Messages[0]
	if got.ID != "assistant-message-1" || got.Role != types.RoleAssistant || !got.IsMeta {
		t.Fatalf("request shares message storage with caller: %#v", got)
	}
	if got.GetText() != "analysis complete" {
		t.Fatalf("request shares content storage with caller: %q", got.GetText())
	}
	if toolUses := got.GetToolUses(); len(toolUses) != 1 || toolUses[0].ID != "tool-use-1" || toolUses[0].Name != "Read" {
		t.Fatalf("tool-use transcript content was not preserved: %#v", toolUses)
	}
}

func assertExactStructFields(t *testing.T, got reflect.Type, want []struct {
	name   string
	typeOf reflect.Type
}) {
	t.Helper()
	if got.NumField() != len(want) {
		t.Fatalf("%s has %d fields, want exactly %d", got.Name(), got.NumField(), len(want))
	}
	for i, field := range want {
		gotField := got.Field(i)
		if gotField.Name != field.name || gotField.Type != field.typeOf {
			t.Fatalf("%s field %d = %s %s, want %s %s", got.Name(), i, gotField.Name, gotField.Type, field.name, field.typeOf)
		}
	}
}
