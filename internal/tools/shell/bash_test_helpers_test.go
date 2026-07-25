package shell

import (
	"context"
	"os/exec"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
)

func executeApprovedBashForTest(t *testing.T, tool *BashTool, input map[string]any) (types.ToolResult, error) {
	t.Helper()
	permission, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatalf("Bash permission preflight: %v", err)
	}
	ctx := approvalcommit.Bind(context.Background(), tool.Name(), input, permission.ExecutionPolicyCode)
	return tool.Execute(ctx, input)
}

func highestBashSecuritySeverityForTest(findings []BashSecurityFinding) BashSecuritySeverity {
	var highest BashSecuritySeverity
	for _, finding := range findings {
		if finding.Severity > highest {
			highest = finding.Severity
		}
	}
	return highest
}

func shellPolicyDecisionTextForTest(decision types.PolicyDecision) string {
	if decision.Disposition == types.PolicyAllow {
		return ""
	}
	return decision.Code
}

func matchBashRuleForTest(command string, rules []permissions.Rule) (permissions.Decision, *permissions.Rule) {
	decision, matched, _ := matchBashRuleDetailed(command, rules)
	return decision, matched
}

func buildBashCommand(tool *BashTool, ctx context.Context, input bashInput, command string) (*exec.Cmd, error) {
	scope := tool.executionScopeSnapshot()
	semantics := ClassifyCommand(command)
	return tool.buildCommandWithSemanticsAtScope(ctx, input, command, semantics, IsReadOnlyCommand(command, semantics), scope)
}
