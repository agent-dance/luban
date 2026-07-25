package permissions

import (
	"context"
	"os/exec"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

type advisoryMockBackend struct{}

func (advisoryMockBackend) Name() string    { return "mock" }
func (advisoryMockBackend) Available() bool { return true }
func (advisoryMockBackend) Command(context.Context, sandbox.Config, string, ...string) (*exec.Cmd, error) {
	return nil, nil
}

func installMandatorySafetyChecks(t *testing.T, fn func(string) (bool, string)) {
	t.Helper()
	SetSafetyConfig(SafetyConfig{
		ShellPolicyAnalyzer: func(command string, _ types.PolicyContext) types.PolicyDecision {
			required, reason := fn(command)
			if !required {
				return types.PolicyDecision{Disposition: types.PolicyAllow}
			}
			return types.PolicyDecision{
				Disposition: types.PolicyRequiredAsk,
				Code:        "test.required_ask",
				PublicKey:   i18n.KeyPermissionApprovalRequired,
				PublicArgs:  []any{reason},
			}
		},
	})
	t.Cleanup(func() { SetSafetyConfig(SafetyConfig{}) })
}

func TestModeAskAlwaysBashApprovalCheckerPrompts(t *testing.T) {
	installMandatorySafetyChecks(t, func(command string) (bool, string) {
		return command == "git status", "git in bare repo requires approval"
	})

	checker := NewChecker(ModeAskAlways, nil)
	promptCount := 0
	setStructuredPromptDecision(checker, func(string, map[string]any) Decision {
		promptCount++
		return DecisionAllowOnce
	})

	decision := checkDecision(checker, "Bash", map[string]any{"command": "git status"})
	if decision != DecisionAllow {
		t.Fatalf("expected DecisionAllow, got %v", decision)
	}
	if promptCount != 1 {
		t.Fatalf("expected prompt to be called once, got %d", promptCount)
	}
}

func TestModeRuleBasedAllowRuleDoesNotBypassBashApprovalChecker(t *testing.T) {
	installMandatorySafetyChecks(t, func(command string) (bool, string) {
		return command == "git status", "git in bare repo requires approval"
	})

	checker := NewChecker(ModeRuleBased, []Rule{
		{Tool: "Bash", Pattern: "git status", Decision: DecisionAllow},
	})
	promptCount := 0
	setStructuredPromptDecision(checker, func(string, map[string]any) Decision {
		promptCount++
		return DecisionAllowOnce
	})

	decision := checkDecision(checker, "Bash", map[string]any{"command": "git status"})
	if decision != DecisionAllow {
		t.Fatalf("expected DecisionAllow, got %v", decision)
	}
	if promptCount != 1 {
		t.Fatalf("expected prompt to be called once, got %d", promptCount)
	}
}

func TestSandboxAwareHandler_RespectsBashApprovalChecker(t *testing.T) {
	installMandatorySafetyChecks(t, func(command string) (bool, string) {
		return command == "git status", "git in bare repo requires approval"
	})

	checker := NewChecker(ModeAskAlways, nil)
	promptCount := 0
	setStructuredPromptDecision(checker, func(string, map[string]any) Decision {
		promptCount++
		return DecisionAllowOnce
	})

	handler := NewSandboxAwarePermissionHandler(advisoryMockBackend{}, NewCLIPermissionHandler(checker))
	decision, err := handler.Check(context.Background(), permission.PermissionRequest{
		ToolName: "Bash",
		Input:    map[string]any{"command": "git status"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != permission.PermissionAllow {
		t.Fatalf("expected PermissionAllow, got %v", decision)
	}
	if promptCount != 1 {
		t.Fatalf("expected prompt to be called once, got %d", promptCount)
	}
}
