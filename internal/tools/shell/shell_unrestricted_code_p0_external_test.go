package shell_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	shell "github.com/agent-dance/luban/internal/tools/shell"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type unrestrictedRuntimeProvider struct {
	runtime types.ToolRuntimeContext
}

func (p unrestrictedRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	return p.runtime
}

func TestUnrestrictedCodePolicyReproductionsRequireFreshApproval(t *testing.T) {
	policyContext := deterministicShellPolicyContext()
	for _, command := range []string{
		`python -c 'open("artifact", "w").write("x")'`,
		`tar -C build -xf archive.tar`,
		`rsync --backup-dir=.git source/ build/`,
		`./custom-binary --target build/output`,
	} {
		decision := shell.AnalyzeShellCommand(command, policyContext)
		if decision.Disposition != types.PolicyRequiredAsk || decision.Code != types.PolicyCodeUnrestrictedCode ||
			decision.Risk != types.PolicyRiskUnrestrictedCode || decision.Remediation == nil {
			t.Errorf("unrestricted command %q did not retain its mandatory typed gate: %#v", command, decision)
		}
	}
}

func TestUnrestrictedCodePolicyPreservesModeledKnownCommands(t *testing.T) {
	policyContext := deterministicShellPolicyContext()
	for _, command := range []string{
		`printf '%s\n' ok`,
		`cat README.md`,
		`ls -la`,
		`git rev-parse --show-toplevel`,
		`go list ./...`,
		`touch build/output`,
	} {
		if decision := shell.AnalyzeShellCommand(command, policyContext); decision.Disposition != types.PolicyAllow {
			t.Errorf("modeled command %q = %#v, want Allow", command, decision)
		}
	}
}

func TestSedAuthorityAllowsModeledGrammarAndAsksForOpaqueEffects(t *testing.T) {
	policyContext := shell.DefaultShellPolicyContext()
	// In-place sed authority now includes proof of its physical effective CWD;
	// use the real package directory instead of the synthetic policy fixture.
	for _, command := range []string{
		`sed 's/old/new/g' input.txt`,
		`sed -i 's/old/new/g;2d' output.txt`,
		`sed -i '' 's/old/new/' output.txt`,
	} {
		if decision := shell.AnalyzeShellCommand(command, policyContext); decision.Disposition != types.PolicyAllow {
			t.Errorf("modeled sed %q = %#v, want Allow", command, decision)
		}
	}
	for _, command := range []string{
		`sed 's/old/new/e' input.txt`,
		`sed -e 'w hidden-output' input.txt`,
		`sed -f edits.sed input.txt`,
		`sed "$SCRIPT" input.txt`,
	} {
		decision := shell.AnalyzeShellCommand(command, policyContext)
		if decision.Disposition != types.PolicyRequiredAsk || decision.Risk != types.PolicyRiskUnrestrictedCode {
			t.Errorf("opaque sed %q = %#v, want unrestricted RequiredAsk", command, decision)
		}
	}
}

func TestSafeBasenameCannotHideCustomExecutableAuthority(t *testing.T) {
	policyContext := deterministicShellPolicyContext()
	for _, command := range []string{
		`./cat input.txt`,
		`/tmp/cat input.txt`,
		`PATH=./bin cat input.txt`,
		`alias cat='./bin/cat'; cat input.txt`,
	} {
		decision := shell.AnalyzeShellCommand(command, policyContext)
		if decision.Disposition != types.PolicyRequiredAsk || decision.Risk != types.PolicyRiskUnrestrictedCode {
			t.Errorf("custom safe-basename command %q = %#v, want unrestricted RequiredAsk", command, decision)
		}
	}
	if decision := shell.AnalyzeShellCommand(`/usr/bin/cat input.txt`, policyContext); decision.Disposition != types.PolicyAllow {
		t.Fatalf("fixed system cat = %#v, want Allow", decision)
	}
}

func TestUnrestrictedCodeRequiredAskPrecedesBashAllowAuthorities(t *testing.T) {
	root := t.TempDir()
	command := `sh -c ':'`
	input := map[string]any{"command": command}

	tool := &shell.BashTool{
		CWD: root, AllowedDirs: []string{root},
		PermissionRules: []permissions.Rule{{Tool: "Bash", Pattern: "sh *", Decision: permissions.DecisionAllow}},
	}
	local, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil || local.Behavior != types.PermissionBehaviorAsk || !local.Required ||
		local.PolicyDecision == nil || local.PolicyDecision.Risk != types.PolicyRiskUnrestrictedCode {
		t.Fatalf("local Allow or plan approval bypassed mandatory gate: result=%#v err=%v", local, err)
	}

	for _, test := range []struct {
		name string
		rule types.PermissionRuleValue
	}{
		{name: "Bash", rule: types.PermissionRuleValue{ToolName: "Bash"}},
		{name: "Bash(sh *)", rule: types.PermissionRuleValue{ToolName: "Bash", RuleContent: "sh *"}},
	} {
		provider := unrestrictedRuntimeProvider{runtime: types.ToolRuntimeContext{
			ProjectRoot:  root,
			AllowedDirs:  []string{root},
			Interactive:  true,
			AllowedTools: map[string]bool{"Bash": true},
			AllowedRules: []types.PermissionRuleValue{test.rule},
		}}
		reg := registry.New()
		reg.SetRuntimeContextProvider(provider)
		reg.Register(&shell.BashTool{CWD: root, AllowedDirs: []string{root}})
		decision, checkErr := reg.CheckToolPermissions(context.Background(), "Bash", input, types.ToolPermissionRequest{})
		if checkErr != nil || decision.Behavior != types.PermissionBehaviorAsk || !decision.Required ||
			decision.PolicyDecision == nil || decision.PolicyDecision.Risk != types.PolicyRiskUnrestrictedCode {
			t.Fatalf("runtime Allow %s bypassed mandatory gate: result=%#v err=%v", test.name, decision, checkErr)
		}
	}
}

func TestUnrestrictedCodeRequiredAskYieldsToAllowAll(t *testing.T) {
	permissions.SetSafetyConfig(permissions.SafetyConfig{ShellPolicyAnalyzer: shell.AnalyzeShellCommand})
	t.Cleanup(func() { permissions.SetSafetyConfig(permissions.SafetyConfig{}) })

	checker := permissions.NewChecker(permissions.ModeAllowAll, []permissions.Rule{{
		Tool: "Bash", Pattern: "python", Decision: permissions.DecisionAllow,
	}})
	var prompts atomic.Int32
	setStructuredPromptDecision(checker, func(string, map[string]any) permissions.Decision {
		prompts.Add(1)
		return permissions.DecisionAllow
	})
	input := map[string]any{"command": `python -c 'print("ok")'`}
	request := permissions.PromptRequest{
		DecisionID: "test.permission", ToolName: "Bash", Input: input, Kind: permissions.PromptKindPermission,
	}
	for index := 0; index < 2; index++ {
		if decision := checker.CheckPrompt(context.Background(), request, permissions.CheckOptions{}).Decision; decision != permissions.DecisionAllow {
			t.Fatalf("automatic invocation %d = %v, want Allow", index, decision)
		}
	}
	if prompts.Load() != 0 {
		t.Fatalf("automatic unrestricted-code invocation prompted: prompts=%d", prompts.Load())
	}
	if decision := checker.CheckPrompt(context.Background(), request, permissions.CheckOptions{AvoidPrompts: true}).Decision; decision != permissions.DecisionAllow {
		t.Fatalf("automatic AvoidPrompts unrestricted-code decision=%v, want Allow", decision)
	}
	if prompts.Load() != 0 {
		t.Fatalf("AvoidPrompts unexpectedly displayed a prompt: prompts=%d", prompts.Load())
	}
}

func TestUnrestrictedCodeReceiptBindsTypedRisk(t *testing.T) {
	root := t.TempDir()
	reg := registry.New()
	reg.Register(&shell.BashTool{CWD: root, AllowedDirs: []string{root}})
	input := map[string]any{"command": `sh -c ':'`}
	request := types.ToolPermissionRequest{SessionID: "risk-session", TurnID: "risk-turn", ToolUseID: "risk-tool", ApprovalEpoch: "risk-epoch"}

	preflight, err := reg.CheckToolPermissions(context.Background(), "Bash", input, request)
	if err != nil || preflight.PermissionGrant == "" || preflight.PermissionBinding.PolicyRisk != types.PolicyRiskUnrestrictedCode {
		t.Fatalf("unrestricted preflight did not bind typed risk: result=%#v err=%v", preflight, err)
	}
	mutated := preflight.PermissionBinding
	mutated.PolicyRisk = types.PolicyRiskCritical
	if token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, "Bash", input, mutated, preflight.ExecutionPolicyCode); token != "" {
		t.Fatalf("risk-mutated receipt authorized token %q", token)
	}
	if token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, "Bash", input, preflight.PermissionBinding, preflight.ExecutionPolicyCode); token != "" {
		t.Fatalf("failed risk mutation did not burn preflight token %q", token)
	}
}

func TestUnrestrictedCodeReceiptConcurrentSingleUse(t *testing.T) {
	root := t.TempDir()
	reg := registry.New()
	reg.Register(&shell.BashTool{CWD: root, AllowedDirs: []string{root}})
	input := map[string]any{"command": `sh -c ':'`}
	request := types.ToolPermissionRequest{SessionID: "race-session", TurnID: "race-turn", ToolUseID: "race-tool", ApprovalEpoch: "race-epoch"}
	preflight, err := reg.CheckToolPermissions(context.Background(), "Bash", input, request)
	if err != nil || preflight.PermissionGrant == "" {
		t.Fatalf("preflight=%#v err=%v", preflight, err)
	}
	token := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, "Bash", input, preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	)
	if token == "" {
		t.Fatal("typed-risk preflight was not promoted")
	}
	prepared := approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: token, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	})

	const contenders = 100
	start := make(chan struct{})
	var successes atomic.Int32
	var wait sync.WaitGroup
	wait.Add(contenders)
	for index := 0; index < contenders; index++ {
		go func() {
			defer wait.Done()
			<-start
			result, executeErr := reg.ExecuteToolWithError(prepared, "Bash", input)
			if executeErr == nil && !result.IsError {
				successes.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatalf("one-time unrestricted-code receipt successes=%d, want 1", successes.Load())
	}
}
