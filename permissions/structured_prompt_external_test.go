package permissions_test

import (
	"context"

	"github.com/agent-dance/luban/permissions"
)

func checkDecision(checker *permissions.Checker, toolName string, input map[string]any) permissions.Decision {
	return checkDecisionWithOptions(checker, toolName, input, permissions.CheckOptions{})
}

func checkDecisionWithOptions(checker *permissions.Checker, toolName string, input map[string]any, opts permissions.CheckOptions) permissions.Decision {
	return checker.CheckPrompt(context.Background(), permissions.PromptRequest{
		DecisionID: "test.permission",
		ToolName:   toolName,
		Input:      input,
		Kind:       permissions.PromptKindPermission,
	}, opts).Decision
}

func setStructuredPromptDecision(checker *permissions.Checker, prompt func(string, map[string]any) permissions.Decision) {
	checker.SetStructuredPromptFunc(func(_ context.Context, request permissions.PromptRequest) permissions.PromptResponse {
		decision := prompt(request.ToolName, request.Input)
		outcome := permissions.PromptOutcomeApproved
		choice := "allow_once"
		switch decision {
		case permissions.DecisionAllow:
			choice = "always_allow"
		case permissions.DecisionDeny, permissions.DecisionAsk:
			outcome = permissions.PromptOutcomeRejected
			choice = "reject"
		}
		return permissions.PromptResponse{
			DecisionID: request.DecisionID,
			Decision:   decision,
			Outcome:    outcome,
			Choice:     choice,
		}
	})
}
