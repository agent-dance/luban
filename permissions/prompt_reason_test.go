package permissions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestWithPromptReasonReplacesGenericRuleSourceWithConcretePolicy(t *testing.T) {
	request := PromptRequest{RiskReason: "generic", RuleSource: "tool permission policy"}
	got := withPromptReason(CheckOptions{Prompt: &request}, "protected path", "mandatory approval policy")
	if got.Prompt == nil || got.Prompt.RuleSource != "mandatory approval policy" || got.Prompt.RiskReason != "generic" {
		t.Fatalf("prompt reason/source = %+v", got.Prompt)
	}
}

func TestAskAlwaysPromptReportsPermissionModeAsRuleSource(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := NewChecker(ModeAskAlways, nil)
	var got PromptRequest
	checker.SetStructuredPromptFunc(func(_ context.Context, request PromptRequest) PromptResponse {
		got = request
		return PromptResponse{Decision: DecisionDeny, Outcome: PromptOutcomeRejected, Choice: "reject"}
	})

	checker.CheckPrompt(context.Background(), PromptRequest{
		DecisionID: "ask-always", ToolName: "CustomTool",
		Input: map[string]any{"scope": "review"}, RuleSource: "tool permission policy",
	}, CheckOptions{})

	if got.RuleSource != permissionText(i18n.KeyPermissionAskAlwaysPolicy) {
		t.Fatalf("ask-always rule source = %q, want an explanatory mode source", got.RuleSource)
	}
}

func TestConfiguredAskRuleUsesDeclaredSource(t *testing.T) {
	installNoopSafetyChecks(t)
	var rules []Rule
	if err := json.Unmarshal([]byte(`[{"tool":"CustomTool","pattern":"review","decision":2,"source":"project settings permissions.ask"}]`), &rules); err != nil {
		t.Fatal(err)
	}
	checker := NewChecker(ModeRuleBased, rules)
	var got PromptRequest
	checker.SetStructuredPromptFunc(func(_ context.Context, request PromptRequest) PromptResponse {
		got = request
		return PromptResponse{Decision: DecisionDeny, Outcome: PromptOutcomeRejected, Choice: "reject"}
	})

	checker.CheckPrompt(context.Background(), PromptRequest{
		DecisionID: "configured", ToolName: "CustomTool",
		Input: map[string]any{"scope": "review"}, RuleSource: "tool permission policy",
	}, CheckOptions{})

	if got.RuleSource != "project settings permissions.ask" {
		t.Fatalf("configured rule source = %q, want declared source", got.RuleSource)
	}
}

func TestAskRuleWithoutSourceGetsDeterministicRuleDescription(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := NewChecker(ModeRuleBased, []Rule{{Tool: "CustomTool", Pattern: "review", Decision: DecisionAsk}})
	var got PromptRequest
	checker.SetStructuredPromptFunc(func(_ context.Context, request PromptRequest) PromptResponse {
		got = request
		return PromptResponse{Decision: DecisionDeny, Outcome: PromptOutcomeRejected, Choice: "reject"}
	})

	checker.CheckPrompt(context.Background(), PromptRequest{
		DecisionID: "rule-without-source", ToolName: "CustomTool",
		Input: map[string]any{"scope": "review"}, RuleSource: "tool permission policy",
	}, CheckOptions{})

	want := permissionFormat(i18n.KeyPermissionConfiguredPatternRule, "CustomTool", "review")
	if got.RuleSource != want {
		t.Fatalf("rule source = %q, want %q", got.RuleSource, want)
	}
}
