package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

type environmentIgnoringSandbox struct {
	executable string
}

func (environmentIgnoringSandbox) Name() string    { return "environment-ignoring-test" }
func (environmentIgnoringSandbox) Available() bool { return true }
func (backend environmentIgnoringSandbox) SandboxCapability() (sandbox.Capability, bool) {
	return sandbox.Capability{
		Backend: backend.Name(), ExecutablePath: backend.executable, ExecutableIdentity: "stable-test-identity",
	}, true
}
func (environmentIgnoringSandbox) Command(ctx context.Context, _ sandbox.Config, name string, args ...string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, name, args...), nil
}

func TestBashEnvironmentStripsHostCredentialsFromResults(t *testing.T) {
	requireBashAvailable(t)
	secretValues := []string{
		"openai-secret-sentinel",
		"anthropic-secret-sentinel",
		"token-secret-sentinel",
		"password-secret-sentinel",
		"authorization-secret-sentinel",
		"aws-secret-sentinel",
	}
	t.Setenv("OPENAI_API_KEY", secretValues[0])
	t.Setenv("ANTHROPIC_API_KEY", secretValues[1])
	t.Setenv("LUBAN_TEST_TOKEN", secretValues[2])
	t.Setenv("LUBAN_TEST_PASSWORD", secretValues[3])
	t.Setenv("Authorization", secretValues[4])
	t.Setenv("AWS_SECRET_ACCESS_KEY", secretValues[5])
	t.Setenv("GOFLAGS", "-mod=readonly")
	t.Setenv("CARGO_TARGET_DIR", "/tmp/luban-cargo-target")

	tool := &BashTool{CWD: t.TempDir()}
	result, err := executeApprovedBashForTest(t, tool, map[string]any{"command": "env"})
	if err != nil {
		t.Fatalf("execute Bash env: %v", err)
	}
	if result.IsError {
		t.Fatal("Bash env failed")
	}
	if !strings.Contains(result.Content, "GOFLAGS=-mod=readonly") ||
		!strings.Contains(result.Content, "CARGO_TARGET_DIR=/tmp/luban-cargo-target") {
		t.Fatal("common build environment was not preserved")
	}

	block := types.MapToolResult(tool, result, "environment-test")
	serializedResult, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("serialize ToolResult: %v", err)
	}
	serializedBlock, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("serialize ToolResultBlock: %v", err)
	}
	allSurfaces := string(serializedResult) + "\n" + string(serializedBlock)
	for _, secret := range secretValues {
		if strings.Contains(result.Content, secret) || strings.Contains(allSurfaces, secret) {
			t.Fatal("credential value reached a tool result or serialized event")
		}
	}
}

func TestBashEnvironmentPolicyIsBoundToPermissionReceipt(t *testing.T) {
	requireBashAvailable(t)
	tool := &BashTool{CWD: t.TempDir()}
	tool.SetEnvironmentPolicy(nil, map[string]string{"CUSTOM_BUILD_ROOT": "/private/first-sentinel"})
	input := map[string]any{"command": "printf ok"}

	permission, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatalf("permission preflight: %v", err)
	}
	if strings.Contains(permission.ExecutionPolicyCode, "first-sentinel") {
		t.Fatal("permission policy code disclosed an environment override")
	}

	tool.SetEnvironmentPolicy(nil, map[string]string{"CUSTOM_BUILD_ROOT": "/private/second-sentinel"})
	debugSurface := fmt.Sprintf("%+v\n%#v", tool, tool)
	if strings.Contains(debugSurface, "first-sentinel") || strings.Contains(debugSurface, "second-sentinel") {
		t.Fatal("generic BashTool debug formatting disclosed an environment override")
	}
	updated, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatalf("updated permission preflight: %v", err)
	}
	if permission.ExecutionPolicyCode == updated.ExecutionPolicyCode {
		t.Fatal("environment policy mutation did not invalidate the permission authority")
	}

	ctx := approvalcommit.Bind(context.Background(), tool.Name(), input, permission.ExecutionPolicyCode)
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("execute with stale receipt: %v", err)
	}
	if !result.IsError {
		t.Fatal("stale environment-bound permission receipt authorized execution")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("serialize rejection: %v", err)
	}
	if strings.Contains(string(encoded), "first-sentinel") || strings.Contains(string(encoded), "second-sentinel") {
		t.Fatal("environment override leaked through a stale-receipt rejection")
	}
}

func TestBashEnvironmentAllowlistAuthorityIsBoundWhenVariablesAreAbsent(t *testing.T) {
	tool := &BashTool{CWD: t.TempDir()}
	input := map[string]any{"command": "printf ok"}
	tool.SetEnvironmentPolicy([]string{"LUBAN_ABSENT_BUILD_SETTING_A"}, nil)
	first, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatalf("first permission preflight: %v", err)
	}
	tool.SetEnvironmentPolicy([]string{"LUBAN_ABSENT_BUILD_SETTING_B"}, nil)
	second, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatalf("second permission preflight: %v", err)
	}
	if first.ExecutionPolicyCode == second.ExecutionPolicyCode {
		t.Fatal("absent-variable allowlist mutation did not invalidate environment authority")
	}
}

func TestBashEnvironmentExplicitNonSecretDelegation(t *testing.T) {
	requireBashAvailable(t)
	t.Setenv("CUSTOM_BUILD_ROOT", "/opt/custom-build")
	tool := &BashTool{CWD: t.TempDir()}
	tool.SetEnvironmentPolicy([]string{"CUSTOM_BUILD_ROOT"}, nil)

	result, err := executeApprovedBashForTest(t, tool, map[string]any{
		"command": `printf %s "$CUSTOM_BUILD_ROOT"`,
	})
	if err != nil {
		t.Fatalf("execute delegated environment: %v", err)
	}
	if result.IsError || result.Content != "/opt/custom-build" {
		t.Fatalf("explicit environment delegation was not applied: %#v", result)
	}
}

func TestBashToolEnforcesEnvironmentWhenSandboxBackendIgnoresConfig(t *testing.T) {
	requireBashAvailable(t)
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("resolve bash: %v", err)
	}
	t.Setenv("OPENAI_API_KEY", "custom-backend-secret-sentinel")
	t.Setenv("GOFLAGS", "-mod=readonly")
	root := t.TempDir()
	tool := &BashTool{
		CWD: root, AllowedDirs: []string{root}, ForceSandbox: true,
		Sandbox: environmentIgnoringSandbox{executable: bashPath},
	}
	command, err := buildBashCommand(tool, context.Background(), bashInput{}, "env")
	if err != nil {
		t.Fatalf("build sandboxed command: %v", err)
	}
	if command.Env == nil {
		t.Fatal("third-party sandbox left child environment inheriting from the host")
	}
	joined := strings.Join(command.Env, "\n")
	if strings.Contains(joined, "custom-backend-secret-sentinel") {
		t.Fatal("third-party sandbox widened child credential authority")
	}
	if !strings.Contains(joined, "GOFLAGS=-mod=readonly") {
		t.Fatal("third-party sandbox path dropped a common build variable")
	}
}

func TestRunEnvironmentStripsHostCredentialsFromSerializedResults(t *testing.T) {
	requireBashAvailable(t)
	secretValues := []string{"run-provider-secret-sentinel", "run-token-secret-sentinel"}
	t.Setenv("OPENAI_API_KEY", secretValues[0])
	t.Setenv("LUBAN_RUN_TEST_TOKEN", secretValues[1])
	t.Setenv("GOFLAGS", "-mod=readonly")

	root := t.TempDir()
	tool := NewRunTool(&BashTool{CWD: root, AllowedDirs: []string{root}})
	result := executeApprovedRunForTest(t, tool, map[string]any{
		"steps": []any{map[string]any{"id": "environment", "argv": []any{"env"}}},
	})
	if result.IsError {
		t.Fatal("Run env failed")
	}
	if !strings.Contains(result.Content, "GOFLAGS=-mod=readonly") {
		t.Fatal("Run dropped a common build variable")
	}
	block := types.MapToolResult(tool, result, "run-environment-test")
	serializedResult, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("serialize Run ToolResult: %v", err)
	}
	serializedBlock, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("serialize Run ToolResultBlock: %v", err)
	}
	allSurfaces := result.Content + "\n" + string(serializedResult) + "\n" + string(serializedBlock)
	for _, secret := range secretValues {
		if strings.Contains(allSurfaces, secret) {
			t.Fatal("credential value reached a Run result or serialized event")
		}
	}
}
