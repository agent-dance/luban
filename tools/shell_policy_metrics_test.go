package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/observability"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestBashPolicyConsumerEmitsBoundedMetricsWithoutCommandPayload(t *testing.T) {
	observability.Reset()
	tool := &BashTool{CWD: t.TempDir()}
	privateCommand := `rm -rf / # private-correlation-value`
	decision, err := tool.CheckPermissions(context.Background(), map[string]any{"command": privateCommand}, types.ToolPermissionRequest{})
	if err != nil || decision.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("policy decision = %#v err=%v, want deny", decision, err)
	}
	point, ok := findShellMetricPoint(observability.MetricShellPolicyDecisions, map[string]string{
		"decision": "block", "reason_class": "root",
	})
	if !ok || point.Sum != 1 {
		t.Fatalf("shell block metric = %+v ok=%v, snapshot=%+v", point, ok, observability.Snapshot())
	}
	for _, metric := range observability.Snapshot() {
		for key, value := range metric.Labels {
			if strings.Contains(key, privateCommand) || strings.Contains(value, privateCommand) ||
				strings.Contains(key, "private-correlation-value") || strings.Contains(value, "private-correlation-value") {
				t.Fatalf("command payload leaked in metric %+v", metric)
			}
		}
	}
}

func TestDirectBashPolicyBlockIsObservedWithoutPreparedPermission(t *testing.T) {
	observability.Reset()
	tool := &BashTool{CWD: t.TempDir()}
	result, err := tool.Execute(context.Background(), map[string]any{"command": `rm -rf /`})
	if err != nil || !result.IsError {
		t.Fatalf("direct result = %#v err=%v, want structured block", result, err)
	}
	point, ok := findShellMetricPoint(observability.MetricShellPolicyDecisions, map[string]string{
		"decision": "block", "reason_class": "root",
	})
	if !ok || point.Sum != 1 {
		t.Fatalf("direct shell block metric = %+v ok=%v, snapshot=%+v", point, ok, observability.Snapshot())
	}
}

func TestPreparedBashExecutionDoesNotDoubleCountPolicyDecision(t *testing.T) {
	observability.Reset()
	tool := &BashTool{CWD: t.TempDir()}
	input := map[string]any{"command": `printf ok`}
	reg := registry.New()
	reg.Register(tool)
	request := types.ToolPermissionRequest{SessionID: "metrics", TurnID: "turn", ToolUseID: "bash", ApprovalEpoch: "metrics-epoch"}
	permission, err := reg.CheckToolPermissions(context.Background(), "Bash", input, request)
	if err != nil || permission.PermissionGrant == "" {
		t.Fatalf("bound permission grant = %#v err=%v", permission, err)
	}
	policyCode := permission.ExecutionPolicyCode
	if permission.PolicyDecision != nil {
		if policyCode == "" {
			policyCode = permission.PolicyDecision.Code
		}
	}
	executionGrant := reg.AuthorizePermissionGrant(
		permission.PermissionGrant, "Bash", input, permission.PermissionBinding, policyCode,
	)
	prepared := approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: executionGrant, Binding: permission.PermissionBinding, PolicyCode: policyCode,
	})
	result, err := reg.ExecuteToolWithError(prepared, "Bash", input)
	if err != nil || result.IsError {
		t.Fatalf("prepared execution = %#v err=%v", result, err)
	}
	point, ok := findShellMetricPoint(observability.MetricShellPolicyDecisions, map[string]string{
		"decision": "allow", "reason_class": "none",
	})
	if !ok || point.Sum != 1 {
		t.Fatalf("prepared policy metric = %+v ok=%v, want one decision; snapshot=%+v", point, ok, observability.Snapshot())
	}
}

func findShellMetricPoint(name observability.MetricName, labels map[string]string) (observability.Point, bool) {
	for _, point := range observability.Snapshot() {
		if point.Name != name || len(point.Labels) != len(labels) {
			continue
		}
		match := true
		for key, value := range labels {
			if point.Labels[key] != value {
				match = false
				break
			}
		}
		if match {
			return point, true
		}
	}
	return observability.Point{}, false
}
