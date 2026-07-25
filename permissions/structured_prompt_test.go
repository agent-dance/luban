package permissions

import "context"

func setStructuredPromptDecision(checker *Checker, prompt func(string, map[string]any) Decision) {
	checker.SetStructuredPromptFunc(func(_ context.Context, request PromptRequest) PromptResponse {
		response := responseForDecision(prompt(request.ToolName, request.Input))
		response.DecisionID = request.DecisionID
		return response
	})
}
