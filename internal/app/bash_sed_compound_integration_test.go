package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/approvalcommit"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	shell "github.com/agent-dance/luban/internal/tools/shell"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func p0bWriteFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func p0bReadEvidence(t *testing.T, dir, path string, state *toolfile.ReadFileState) {
	t.Helper()
	result, err := (&toolfile.FileReadTool{AllowedDirs: []string{dir}, ReadState: state}).Execute(
		context.Background(), map[string]any{"file_path": path},
	)
	if err != nil || result.IsError {
		t.Fatalf("production Read evidence failed: result=%+v err=%v", result, err)
	}
}

func requireIntegrationBashAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash is not on PATH: %v", err)
	}
	if err := exec.Command("bash", "-c", "true").Run(); err != nil {
		t.Skipf("bash is present but not runnable: %v", err)
	}
}

func executeApprovedIntegrationBash(
	t *testing.T,
	ctx context.Context,
	tool *shell.BashTool,
	input map[string]any,
) (types.ToolResultBlock, error) {
	t.Helper()
	reg := registry.New()
	reg.Register(tool)
	preflight, err := reg.CheckToolPermissions(ctx, tool.Name(), input, types.ToolPermissionRequest{
		SessionID:     "bash-integration-session",
		TurnID:        "bash-integration-turn",
		ToolUseID:     "bash-integration-use",
		ApprovalEpoch: "bash-integration-epoch",
	})
	if err != nil {
		return types.ToolResultBlock{}, err
	}
	if preflight.Behavior == types.PermissionBehaviorDeny {
		return reg.ExecuteToolWithError(ctx, tool.Name(), input)
	}
	token := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant,
		tool.Name(),
		input,
		preflight.PermissionBinding,
		preflight.ExecutionPolicyCode,
	)
	if token == "" {
		t.Fatalf("Bash permission preflight did not produce an execution receipt: %+v", preflight)
	}
	approved := approvalcommit.WithPending(ctx, approvalcommit.Pending{
		Token: token, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	})
	return reg.ExecuteToolWithError(approved, tool.Name(), input)
}

func p0SedCommand(script, target string) string {
	if runtime.GOOS == "darwin" {
		return "sed -i '' '" + script + "' " + target
	}
	return "sed -i '" + script + "' " + target
}

func TestP0BSedEffectiveCWDRejectsUnreadRealTarget(t *testing.T) {
	requireIntegrationBashAvailable(t)
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(sub, "victim.txt")
	p0bWriteFixture(t, target, "alpha\n")
	state := toolfile.NewReadFileState()
	tool := &shell.BashTool{
		CWD: root, AllowedDirs: []string{root}, FileMutations: toolfile.NewFileMutationCoordinator(state),
	}
	command := "cd sub && " + p0SedCommand("s/alpha/forged/", "victim.txt")
	result, err := executeApprovedIntegrationBash(t, context.Background(), tool, map[string]any{"command": command})
	if err != nil || !result.IsError {
		t.Fatalf("unread effective target was not rejected: result=%+v err=%v", result, err)
	}
	raw, readErr := os.ReadFile(target)
	if readErr != nil || string(raw) != "alpha\n" {
		t.Fatalf("rejected command changed real target: content=%q err=%v", raw, readErr)
	}
	if _, exists := state.GetForContext(context.Background(), target); exists {
		t.Fatal("rejected command published evidence for unread target")
	}
}

func TestP0BSedUnprovableSequenceIsApprovalOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	static := p0SedCommand("s/alpha/beta/", "victim.txt")
	commands := map[string]string{
		"unconditional-cd": "cd sub; " + static,
		"preceding-copy":   "cp other.txt victim.txt; " + static,
		"glob-target":      p0SedCommand("s/alpha/beta/", "*.txt"),
		"script-file":      "sed -i -f edits.sed victim.txt",
		"conditional":      "if true; then " + static + "; fi",
		"asynchronous":     static + " &",
	}
	tool := &shell.BashTool{CWD: root, AllowedDirs: []string{root}}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			preflight, err := tool.CheckPermissions(context.Background(), map[string]any{"command": command}, types.ToolPermissionRequest{})
			if err != nil || preflight.Behavior != types.PermissionBehaviorAsk || preflight.PolicyDecision == nil ||
				preflight.PolicyDecision.Disposition != types.PolicyRequiredAsk {
				t.Fatalf("unprovable sed sequence did not require approval: preflight=%+v err=%v", preflight, err)
			}
		})
	}
}

func TestP0BSedPrecedingMutationRequiresApprovalAndPublishesNoEvidence(t *testing.T) {
	requireIntegrationBashAvailable(t)
	root := t.TempDir()
	victim := filepath.Join(root, "victim.txt")
	other := filepath.Join(root, "other.txt")
	p0bWriteFixture(t, victim, "alpha\n")
	p0bWriteFixture(t, other, "bravo\n")
	fixed := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(victim, fixed, fixed); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(other, fixed, fixed); err != nil {
		t.Fatal(err)
	}
	state := toolfile.NewReadFileState()
	p0bReadEvidence(t, root, victim, state)
	before, ok := state.GetForContext(context.Background(), victim)
	if !ok || before.ContentDigest == "" || before.FileIdentity == nil {
		t.Fatalf("missing strong initial evidence: %+v", before)
	}
	command := "cp -p other.txt victim.txt; " + p0SedCommand("s/bravo/charl/", "victim.txt")
	tool := &shell.BashTool{
		CWD: root, AllowedDirs: []string{root}, FileMutations: toolfile.NewFileMutationCoordinator(state),
	}
	preflight, err := tool.CheckPermissions(context.Background(), map[string]any{"command": command}, types.ToolPermissionRequest{})
	if err != nil || preflight.Behavior != types.PermissionBehaviorAsk || preflight.PolicyDecision == nil ||
		preflight.PolicyDecision.Disposition != types.PolicyRequiredAsk || preflight.PolicyDecision.Risk != types.PolicyRiskUnrestrictedCode {
		t.Fatalf("preceding replacement was not approval-only: preflight=%+v err=%v", preflight, err)
	}
	direct, err := tool.Execute(context.Background(), map[string]any{"command": command})
	if err != nil || !direct.IsError {
		t.Fatalf("direct execution bypassed RequiredAsk: result=%+v err=%v", direct, err)
	}
	if raw, readErr := os.ReadFile(victim); readErr != nil || string(raw) != "alpha\n" {
		t.Fatalf("denied command changed victim: content=%q err=%v", raw, readErr)
	}

	reg := registry.New()
	reg.Register(tool)
	input := map[string]any{"command": command}
	request := types.ToolPermissionRequest{
		SessionID: "sed-session", TurnID: "sed-turn", ToolUseID: "sed-tool", ApprovalEpoch: "sed-epoch",
	}
	preflight, err = reg.CheckToolPermissions(context.Background(), "Bash", input, request)
	if err != nil || preflight.Behavior != types.PermissionBehaviorAsk || !preflight.Required || preflight.PermissionGrant == "" {
		t.Fatalf("missing mandatory sed approval: result=%+v err=%v", preflight, err)
	}
	token := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, "Bash", input, preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	)
	if token == "" {
		t.Fatal("approved sed command did not receive an execution grant")
	}
	approved := approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: token, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	})
	result, err := reg.ExecuteToolWithError(approved, "Bash", input)
	if err != nil || result.IsError {
		t.Fatalf("approved sed command failed: result=%+v err=%v", result, err)
	}
	raw, readErr := os.ReadFile(victim)
	if readErr != nil || string(raw) != "charl\n" {
		t.Fatalf("approved compound result=%q err=%v", raw, readErr)
	}
	after, ok := state.GetForContext(context.Background(), victim)
	if !ok || after.LastTool == "Bash" || after.ContentDigest != before.ContentDigest ||
		after.FileIdentity == nil || !os.SameFile(after.FileIdentity, before.FileIdentity) {
		t.Fatalf("approval-only command published or rewrote Read evidence: before=%+v after=%+v", before, after)
	}
	edit := &toolfile.FileEditTool{AllowedDirs: []string{root}, ReadState: state}
	editResult, err := edit.Execute(context.Background(), map[string]any{
		"file_path": victim, "old_string": "charl", "new_string": "forged",
	})
	data, dataOK := editResult.Data.(types.ToolErrorData)
	if err != nil || !editResult.IsError || !dataOK || data.Code == "" {
		t.Fatalf("same-stat compound result authorized Edit: result=%+v data=%+v err=%v", editResult, editResult.Data, err)
	}
}
