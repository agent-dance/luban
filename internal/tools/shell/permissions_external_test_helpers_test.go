package shell_test

import (
	"context"

	"github.com/agent-dance/luban/permissions"
)

func setStructuredPromptDecision(checker *permissions.Checker, prompt func(string, map[string]any) permissions.Decision) {
	checker.SetStructuredPromptFunc(func(_ context.Context, request permissions.PromptRequest) permissions.PromptResponse {
		decision := prompt(request.ToolName, request.Input)
		outcome := permissions.PromptOutcomeApproved
		choice := "allow_once"
		if decision == permissions.DecisionAllow {
			choice = "always_allow"
		} else if decision == permissions.DecisionDeny || decision == permissions.DecisionAsk {
			outcome = permissions.PromptOutcomeRejected
			choice = "reject"
		}
		return permissions.PromptResponse{DecisionID: request.DecisionID, Decision: decision, Outcome: outcome, Choice: choice}
	})
}
