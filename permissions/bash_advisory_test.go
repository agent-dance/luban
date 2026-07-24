package permissions

import (
	"context"
	"os/exec"
	"testing"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/sandbox"
)

type advisoryMockBackend struct{}

func (advisoryMockBackend) Name() string    { return "mock" }
func (advisoryMockBackend) Available() bool { return true }
func (advisoryMockBackend) Command(context.Context, sandbox.Config, string, ...string) (*exec.Cmd, error) {
	return nil, nil
}

func installAdvisorySafetyChecks(t *testing.T, fn func(string) (bool, string)) {
	t.Helper()
	SetSafetyConfig(SafetyConfig{
		DangerousCommandChecker: func(string) string { return "" },
		BashProtectedPathChecker: func(string) (bool, string) {
			return false, ""
		},
		BashNeedsApprovalChecker: fn,
	})
	t.Cleanup(func() { SetSafetyConfig(SafetyConfig{}) })
}

func TestModeAskAlwaysBashApprovalCheckerPrompts(t *testing.T) {
	installAdvisorySafetyChecks(t, func(command string) (bool, string) {
		return command == "git status", "git in bare repo requires approval"
	})

	checker := NewChecker(ModeAskAlways, nil)
	promptCount := 0
	checker.SetPromptFunc(func(string, map[string]any) Decision {
		promptCount++
		return DecisionAllowOnce
	})

	decision := checker.Check("Bash", map[string]any{"command": "git status"})
	if decision != DecisionAllow {
		t.Fatalf("expected DecisionAllow, got %v", decision)
	}
	if promptCount != 1 {
		t.Fatalf("expected prompt to be called once, got %d", promptCount)
	}
}

func TestModeRuleBasedAllowRuleDoesNotBypassBashApprovalChecker(t *testing.T) {
	installAdvisorySafetyChecks(t, func(command string) (bool, string) {
		return command == "git status", "git in bare repo requires approval"
	})

	checker := NewChecker(ModeRuleBased, []Rule{
		{Tool: "Bash", Pattern: "git status", Decision: DecisionAllow},
	})
	promptCount := 0
	checker.SetPromptFunc(func(string, map[string]any) Decision {
		promptCount++
		return DecisionAllowOnce
	})

	decision := checker.Check("Bash", map[string]any{"command": "git status"})
	if decision != DecisionAllow {
		t.Fatalf("expected DecisionAllow, got %v", decision)
	}
	if promptCount != 1 {
		t.Fatalf("expected prompt to be called once, got %d", promptCount)
	}
}

func TestSandboxAwareHandler_RespectsBashApprovalChecker(t *testing.T) {
	installAdvisorySafetyChecks(t, func(command string) (bool, string) {
		return command == "git status", "git in bare repo requires approval"
	})

	checker := NewChecker(ModeAskAlways, nil)
	promptCount := 0
	checker.SetPromptFunc(func(string, map[string]any) Decision {
		promptCount++
		return DecisionAllowOnce
	})

	handler := NewSandboxAwarePermissionHandler(advisoryMockBackend{}, NewCLIPermissionHandler(checker))
	decision, err := handler.Check(context.Background(), engine.PermissionRequest{
		ToolName: "Bash",
		Input:    map[string]any{"command": "git status"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != engine.PermissionAllow {
		t.Fatalf("expected PermissionAllow, got %v", decision)
	}
	if promptCount != 1 {
		t.Fatalf("expected prompt to be called once, got %d", promptCount)
	}
}
