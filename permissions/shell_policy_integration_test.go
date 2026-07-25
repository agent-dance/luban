package permissions_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/tools/shell"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

func installUnifiedShellPolicy(t *testing.T) {
	t.Helper()
	permissions.SetSafetyConfig(permissions.SafetyConfig{ShellPolicyAnalyzer: shell.AnalyzeShellCommand})
	t.Cleanup(func() { permissions.SetSafetyConfig(permissions.SafetyConfig{}) })
}

func TestUnifiedShellPolicySafetyAndMandatoryAdapters(t *testing.T) {
	installUnifiedShellPolicy(t)
	tests := []struct {
		command   string
		safety    permissions.Decision
		mandatory permissions.Decision
	}{
		{`rm -rf /`, permissions.DecisionDeny, permissions.DecisionAllow},
		{`rm -rf "$TARGET"`, permissions.DecisionAllow, permissions.DecisionAsk},
		{`tmp=$(mktemp -d); rm -rf -- "$tmp"`, permissions.DecisionAllow, permissions.DecisionAllow},
		{`printf ok`, permissions.DecisionAllow, permissions.DecisionAllow},
	}
	for _, test := range tests {
		input := map[string]any{"command": test.command}
		if got, _ := permissions.SafetyCheck("Bash", input); got != test.safety {
			t.Errorf("SafetyCheck(%q)=%v, want %v", test.command, got, test.safety)
		}
		if got, _ := permissions.MandatoryApprovalCheck("Bash", input); got != test.mandatory {
			t.Errorf("MandatoryApprovalCheck(%q)=%v, want %v", test.command, got, test.mandatory)
		}
	}
}

func TestUnifiedShellPolicyMandatoryAskPrecedesAllowAndIsNeverCached(t *testing.T) {
	installUnifiedShellPolicy(t)
	checker := permissions.NewChecker(permissions.ModeRuleBased, []permissions.Rule{{
		Tool: "Bash", Pattern: "rm ", Decision: permissions.DecisionAllow,
	}})
	prompts := 0
	setStructuredPromptDecision(checker, func(string, map[string]any) permissions.Decision {
		prompts++
		return permissions.DecisionAllow
	})
	input := map[string]any{"command": `rm -rf "$TARGET"`}
	for i := 0; i < 2; i++ {
		if got := checkDecision(checker, "Bash", input); got != permissions.DecisionAllow {
			t.Fatalf("check %d = %v, want allow after prompt", i, got)
		}
	}
	if prompts != 2 {
		t.Fatalf("mandatory ask was cached: prompt count=%d, want 2", prompts)
	}

	decision := shell.AnalyzeShellCommand(`rm -rf "$TARGET"`, shell.DefaultShellPolicyContext())
	if decision.Disposition != types.PolicyRequiredAsk || decision.Remediation == nil {
		t.Fatalf("non-interactive decision lacks structured remediation: %#v", decision)
	}
	if got := checkDecisionWithOptions(checker, "Bash", input, permissions.CheckOptions{AvoidPrompts: true}); got != permissions.DecisionDeny {
		t.Fatalf("non-interactive mandatory ask=%v, want deny", got)
	}
}

func TestUnifiedShellPolicyRequiredAskYieldsToAutomaticMode(t *testing.T) {
	installUnifiedShellPolicy(t)
	input := map[string]any{"command": `sh cleanup.sh`}
	policy := shell.AnalyzeShellCommand(input["command"].(string), shell.DefaultShellPolicyContext())
	if !policy.IsRequiredAsk() {
		t.Fatalf("test command policy = %#v, want RequiredAsk", policy)
	}

	for _, test := range []struct {
		name        string
		checkerMode permissions.Mode
		requestMode string
	}{
		{name: "session auto", checkerMode: permissions.ModeAllowAll},
		{name: "inherited auto override", checkerMode: permissions.ModeRuleBased, requestMode: "bypassPermissions"},
	} {
		t.Run(test.name, func(t *testing.T) {
			checker := permissions.NewChecker(test.checkerMode, nil)
			prompts := 0
			setStructuredPromptDecision(checker, func(string, map[string]any) permissions.Decision {
				prompts++
				return permissions.DecisionDeny
			})
			handler := permissions.NewCLIPermissionHandler(checker)
			decision, err := handler.Check(context.Background(), permission.PermissionRequest{
				ToolName: "Bash", Input: input, Mode: test.requestMode,
				Required: true, AvoidPrompts: true, PolicyDecision: &policy,
			})
			if err != nil || decision != permission.PermissionAllow || prompts != 0 {
				t.Fatalf("automatic RequiredAsk decision=%v prompts=%d err=%v", decision, prompts, err)
			}
		})
	}

	checker := permissions.NewChecker(permissions.ModeAllowAll, nil)
	prompts := 0
	setStructuredPromptDecision(checker, func(string, map[string]any) permissions.Decision {
		prompts++
		return permissions.DecisionDeny
	})
	if decision := checkDecision(checker, "Bash", input); decision != permissions.DecisionAllow || prompts != 0 {
		t.Fatalf("direct automatic RequiredAsk decision=%v prompts=%d", decision, prompts)
	}
}

func TestUnifiedShellPolicyBlockPrecedesAllowAll(t *testing.T) {
	installUnifiedShellPolicy(t)
	checker := permissions.NewChecker(permissions.ModeAllowAll, nil)
	prompted := false
	setStructuredPromptDecision(checker, func(string, map[string]any) permissions.Decision {
		prompted = true
		return permissions.DecisionAllow
	})
	if got := checkDecision(checker, "Bash", map[string]any{"command": `rm --recursive --force /`}); got != permissions.DecisionDeny {
		t.Fatalf("allow-all bypassed hard block: %v", got)
	}
	if prompted {
		t.Fatal("hard block was downgraded to an interactive prompt")
	}
}

func TestGenericRequiredRequestStillPrecedesAllowAllForSafeBash(t *testing.T) {
	installUnifiedShellPolicy(t)
	checker := permissions.NewChecker(permissions.ModeAllowAll, nil)
	prompts := 0
	setStructuredPromptDecision(checker, func(string, map[string]any) permissions.Decision {
		prompts++
		return permissions.DecisionAllowOnce
	})
	handler := permissions.NewCLIPermissionHandler(checker)
	req := permission.PermissionRequest{
		ToolName: "Bash", Input: map[string]any{"command": `printf ok`}, Required: true,
	}
	decision, err := handler.Check(context.Background(), req)
	if err != nil || decision != permission.PermissionAllowOnce || prompts != 1 {
		t.Fatalf("required safe Bash decision=%v prompts=%d err=%v", decision, prompts, err)
	}
	req.AvoidPrompts = true
	decision, err = handler.Check(context.Background(), req)
	if err != nil || decision != permission.PermissionDeny || prompts != 1 {
		t.Fatalf("non-interactive required Bash decision=%v prompts=%d err=%v", decision, prompts, err)
	}
}

func TestRestrictiveRulePrecedenceIsOrderIndependent(t *testing.T) {
	installUnifiedShellPolicy(t)
	input := map[string]any{"command": `git push origin main`}
	for _, rules := range [][]permissions.Rule{
		{{Tool: "Bash", Pattern: "git", Decision: permissions.DecisionAllow}, {Tool: "Bash", Pattern: "git push", Decision: permissions.DecisionDeny}},
		{{Tool: "Bash", Pattern: "git push", Decision: permissions.DecisionDeny}, {Tool: "Bash", Pattern: "git", Decision: permissions.DecisionAllow}},
	} {
		checker := permissions.NewChecker(permissions.ModeRuleBased, rules)
		if got := checkDecision(checker, "Bash", input); got != permissions.DecisionDeny {
			t.Fatalf("restrictive precedence depended on rule order: %v", got)
		}
	}
}

type unifiedPolicySandbox struct{}

func (unifiedPolicySandbox) Name() string    { return "unified-policy-test" }
func (unifiedPolicySandbox) Available() bool { return true }
func (unifiedPolicySandbox) Command(context.Context, sandbox.Config, string, ...string) (*exec.Cmd, error) {
	return nil, nil
}

type recordingPermissionHandler struct {
	calls int
}

func (h *recordingPermissionHandler) Check(context.Context, permission.PermissionRequest) (permission.PermissionDecision, error) {
	h.calls++
	return permission.PermissionAllow, nil
}

func TestUnifiedShellPolicySandboxCannotConsumeRequiredAsk(t *testing.T) {
	installUnifiedShellPolicy(t)
	fallback := &recordingPermissionHandler{}
	handler := permissions.NewSandboxAwarePermissionHandler(unifiedPolicySandbox{}, fallback)

	for _, disabled := range []bool{false, true} {
		decision, err := handler.Check(context.Background(), permission.PermissionRequest{
			ToolName: "Bash",
			Input: map[string]any{
				"command":                   `rm -rf "$TARGET"`,
				"dangerouslyDisableSandbox": disabled,
			},
		})
		if err != nil || decision != permission.PermissionAllow {
			t.Fatalf("sandbox required ask disabled=%v: decision=%v err=%v", disabled, decision, err)
		}
	}
	if fallback.calls != 2 {
		t.Fatalf("sandbox swallowed required asks: fallback calls=%d", fallback.calls)
	}

	before := fallback.calls
	decision, err := handler.Check(context.Background(), permission.PermissionRequest{
		ToolName: "Bash", Input: map[string]any{"command": `rm -rf /`},
	})
	if err != nil || decision != permission.PermissionDeny {
		t.Fatalf("hard block decision=%v err=%v", decision, err)
	}
	if fallback.calls != before {
		t.Fatal("hard block reached interactive fallback")
	}
}
