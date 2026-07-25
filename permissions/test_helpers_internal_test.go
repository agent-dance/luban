package permissions

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/types"
)

func checkDecision(checker *Checker, toolName string, input map[string]any) Decision {
	return checkDecisionWithOptions(checker, toolName, input, CheckOptions{})
}

func checkDecisionWithOptions(checker *Checker, toolName string, input map[string]any, opts CheckOptions) Decision {
	return checker.CheckPrompt(context.Background(), PromptRequest{
		DecisionID: "test.permission",
		ToolName:   toolName,
		Input:      input,
		Kind:       PromptKindPermission,
	}, opts).Decision
}

func installNoopSafetyChecks(t *testing.T) {
	t.Helper()
	SetSafetyConfig(SafetyConfig{
		ShellPolicyAnalyzer: func(string, types.PolicyContext) types.PolicyDecision {
			return types.PolicyDecision{Disposition: types.PolicyAllow}
		},
	})
	t.Cleanup(func() { SetSafetyConfig(SafetyConfig{}) })
}
