package shell_test

import (
	"context"
	shell "github.com/agent-dance/luban/internal/tools/shell"
	"testing"

	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func requireUnrestrictedCodeAsk(t *testing.T, command string, policy types.PolicyContext) {
	t.Helper()
	decision := shell.AnalyzeShellCommand(command, policy)
	if decision.Disposition != types.PolicyRequiredAsk ||
		decision.Code != types.PolicyCodeUnrestrictedCode ||
		decision.Risk != types.PolicyRiskUnrestrictedCode || decision.Remediation == nil {
		t.Fatalf("shell.AnalyzeShellCommand(%q) = %#v, want mandatory unrestricted-code approval", command, decision)
	}
}

func TestSafeAllowlistP0XXDModelsEveryOutputTarget(t *testing.T) {
	policy := deterministicShellPolicyContext()
	for _, command := range []string{
		`xxd /dev/null`,
		`xxd -r /dev/null`,
		`xxd -r /dev/null -`,
		`xxd -r /dev/null build/output.bin`,
	} {
		if decision := shell.AnalyzeShellCommand(command, policy); decision.Disposition != types.PolicyAllow {
			t.Errorf("modeled xxd command %q = %#v, want Allow", command, decision)
		}
	}
	for _, command := range []string{
		`xxd -r /dev/null .git/config`,
		`xxd /dev/null .env`,
		`TARGET=.git/config; xxd -r /dev/null "$TARGET"`,
	} {
		if decision := shell.AnalyzeShellCommand(command, policy); decision.Disposition != types.PolicyBlock {
			t.Errorf("protected xxd output %q = %#v, want Block", command, decision)
		}
	}
	for _, command := range []string{
		`xxd -r /dev/null "$TARGET"`,
		`xxd "$INPUT" build/output.bin`,
		`xxd --future-output-option build/output.bin`,
	} {
		requireUnrestrictedCodeAsk(t, command, policy)
	}
	if got := shell.ClassifyCommand(`xxd /dev/null`); got != shell.SemanticRead {
		t.Fatalf("read-only xxd semantic=%v, want read", got)
	}
	if got := shell.ClassifyCommand(`xxd -r /dev/null build/output.bin`); got != shell.SemanticWrite {
		t.Fatalf("xxd outfile semantic=%v, want write", got)
	}
}

func TestSafeAllowlistP0GoListRejectsToolchainAndOutputFlags(t *testing.T) {
	policy := deterministicShellPolicyContext()
	for _, command := range []string{
		`go version`,
		`go list ./...`,
		`go list -json -deps ./...`,
		`go list -f '{{.ImportPath}}' ./...`,
	} {
		if decision := shell.AnalyzeShellCommand(command, policy); decision.Disposition != types.PolicyAllow {
			t.Errorf("metadata-only go command %q = %#v, want Allow", command, decision)
		}
	}
	for _, command := range []string{
		`go list -export -toolexec=/tmp/evil ./...`,
		`go list -overlay=overlay.json ./...`,
		`go list -compiled ./...`,
		`go list -mod=mod ./...`,
		`go list "$FLAGS" ./...`,
		`GOFLAGS=-toolexec=/tmp/evil go list -export ./...`,
	} {
		requireUnrestrictedCodeAsk(t, command, policy)
	}
	if got := shell.ClassifyCommand(`go list -export -toolexec=/tmp/evil ./...`); got != shell.SemanticWrite {
		t.Fatalf("toolchain-bearing go list semantic=%v, want write", got)
	}
}

func TestSafeAllowlistP0GitRejectsConfiguredDelegates(t *testing.T) {
	policy := deterministicShellPolicyContext()
	for _, command := range []string{
		`git rev-parse --show-toplevel`,
		`git ls-tree HEAD`,
		`git cat-file -p HEAD`,
		`git config --get user.name`,
	} {
		if decision := shell.AnalyzeShellCommand(command, policy); decision.Disposition != types.PolicyAllow {
			t.Errorf("internal git read %q = %#v, want Allow", command, decision)
		}
	}
	for _, command := range []string{
		`git status --short`,
		`git diff --stat`,
		`git cat-file --filters HEAD:path`,
		`git cat-file --textconv HEAD:path`,
		`git tag --list 'v*'`,
		`git tag -v v1.0.0`,
		`git verify-tag v1.0.0`,
		`git -c core.fsmonitor=/tmp/evil status`,
		`GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.fsmonitor GIT_CONFIG_VALUE_0=/tmp/evil git rev-parse HEAD`,
	} {
		requireUnrestrictedCodeAsk(t, command, policy)
	}
	for command, want := range map[string]shell.CommandSemantic{
		`git status --short`:               shell.SemanticWrite,
		`git cat-file --filters HEAD:path`: shell.SemanticWrite,
		`git tag -v v1.0.0`:                shell.SemanticWrite,
		`git verify-commit HEAD`:           shell.SemanticWrite,
		`git cat-file -p HEAD`:             shell.SemanticRead,
		`git rev-parse --show-toplevel`:    shell.SemanticRead,
	} {
		if got := shell.ClassifyCommand(command); got != want {
			t.Errorf("shell.ClassifyCommand(%q)=%v, want %v", command, got, want)
		}
	}
}

func TestSafeAllowlistP0EnvironmentAndUtilityDelegatesFailClosed(t *testing.T) {
	policy := deterministicShellPolicyContext()
	for _, command := range []string{
		`LD_PRELOAD=/tmp/evil.so /usr/bin/cat README.md`,
		`env LD_LIBRARY_PATH=/tmp/evil /usr/bin/cat README.md`,
		`DYLD_INSERT_LIBRARIES=/tmp/evil.dylib /usr/bin/cat README.md`,
		`export LD_AUDIT=/tmp/evil.so; /usr/bin/cat README.md`,
		`printf -v PATH /tmp/evil; /usr/bin/cat README.md`,
		`read LD_PRELOAD; /usr/bin/cat README.md`,
		`RIPGREP_CONFIG_PATH=/tmp/rg.conf rg pattern`,
		`sort --compress-program=/tmp/evil input.txt`,
		`ss -K dst 127.0.0.1`,
		`pgrep --signal KILL worker`,
		`lsof -D b/tmp/lsof-cache`,
	} {
		requireUnrestrictedCodeAsk(t, command, policy)
	}
	for _, command := range []string{
		`FOO=bar /usr/bin/cat README.md`,
		`env -u LD_PRELOAD /usr/bin/cat README.md`,
		`sort input.txt`,
		`ss -l`,
	} {
		if decision := shell.AnalyzeShellCommand(command, policy); decision.Disposition != types.PolicyAllow {
			t.Errorf("non-delegating command %q = %#v, want Allow", command, decision)
		}
	}
	if got := shell.ClassifyCommand(`sort --compress-program=/tmp/evil input.txt`); got != shell.SemanticWrite {
		t.Fatalf("delegating sort semantic=%v, want write", got)
	}
	taintedPolicy := policy
	taintedPolicy.KnownEnvironment = map[string]string{
		"HOME": policy.HomeDir, "LD_PRELOAD": "/tmp/evil.so",
	}
	requireUnrestrictedCodeAsk(t, `/usr/bin/cat README.md`, taintedPolicy)
}

func TestSafeAllowlistP0MandatoryGateYieldsToAutomaticMode(t *testing.T) {
	command := `go list -export -toolexec=/tmp/evil ./...`
	input := map[string]any{"command": command}
	root := t.TempDir()

	tool := &shell.BashTool{
		CWD: root, AllowedDirs: []string{root},
		PermissionRules: []permissions.Rule{{Tool: "Bash", Pattern: "go list *", Decision: permissions.DecisionAllow}},
	}
	local, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil || local.Behavior != types.PermissionBehaviorAsk || !local.Required ||
		local.PolicyDecision == nil || local.PolicyDecision.Risk != types.PolicyRiskUnrestrictedCode {
		t.Fatalf("Bash Allow rule bypassed mandatory gate: result=%#v err=%v", local, err)
	}

	permissions.SetSafetyConfig(permissions.SafetyConfig{ShellPolicyAnalyzer: shell.AnalyzeShellCommand})
	t.Cleanup(func() { permissions.SetSafetyConfig(permissions.SafetyConfig{}) })
	checker := permissions.NewChecker(permissions.ModeAllowAll, []permissions.Rule{{
		Tool: "Bash", Pattern: "go list *", Decision: permissions.DecisionAllow,
	}})
	prompts := 0
	setStructuredPromptDecision(checker, func(string, map[string]any) permissions.Decision {
		prompts++
		return permissions.DecisionAllow
	})
	promptRequest := permissions.PromptRequest{
		DecisionID: "test.permission", ToolName: "Bash", Input: input, Kind: permissions.PromptKindPermission,
	}
	if got := checker.CheckPrompt(context.Background(), promptRequest, permissions.CheckOptions{}).Decision; got != permissions.DecisionAllow || prompts != 0 {
		t.Fatalf("ModeAllowAll RequiredAsk result=%v prompts=%d", got, prompts)
	}
	if got := checker.CheckPrompt(context.Background(), promptRequest, permissions.CheckOptions{}).Decision; got != permissions.DecisionAllow || prompts != 0 {
		t.Fatalf("ModeAllowAll repeated RequiredAsk result=%v prompts=%d", got, prompts)
	}

	reg := registry.New()
	reg.Register(&shell.BashTool{CWD: root, AllowedDirs: []string{root}})
	request := types.ToolPermissionRequest{
		SessionID: "delegate-session", TurnID: "delegate-turn",
		ToolUseID: "delegate-tool", ApprovalEpoch: "delegate-epoch",
	}
	preflight, err := reg.CheckToolPermissions(context.Background(), "Bash", input, request)
	if err != nil || preflight.PermissionGrant == "" ||
		preflight.PolicyDecision == nil || preflight.PolicyDecision.Code != types.PolicyCodeUnrestrictedCode ||
		preflight.PermissionBinding.PolicyRisk != types.PolicyRiskUnrestrictedCode {
		t.Fatalf("registry preflight did not bind delegate risk: result=%#v err=%v", preflight, err)
	}
	mutatedInput := map[string]any{"command": `go list ./...`}
	if token := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, "Bash", mutatedInput,
		preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	); token != "" {
		t.Fatalf("input-mutated delegate receipt authorized token %q", token)
	}
}
