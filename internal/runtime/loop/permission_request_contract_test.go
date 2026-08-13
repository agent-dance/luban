package loop

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestPermissionRequestPreservesDecisionIdentityAndReviewContext(t *testing.T) {
	policy := types.PolicyDecision{Disposition: types.PolicyRequiredAsk, Code: "shell.policy.ask.dynamic_target"}
	tool := &permissionLifecycleTool{decision: types.ToolPermissionResult{
		Behavior:       types.PermissionBehaviorAsk,
		Message:        "writes a generated artifact outside the current work unit",
		BlockedPath:    "/workspace/generated/report.txt",
		Required:       true,
		PolicyDecision: &policy,
	}}
	reg := registry.New()
	reg.Register(tool)
	handler := &lifecyclePermissionHandler{decision: permission.PermissionAllowOnce}
	exec := executioncontract.ToolExecutionContext{
		TurnID:     "turn-9",
		ActorID:    "agent-reviewer",
		ActorType:  "reviewer",
		WorkUnitID: "work-report",
	}
	toolUse := types.ToolUseBlock{
		Type: types.ContentTypeToolUse,
		ID:   "toolu-write-7",
		Name: "Lifecycle",
		Input: map[string]any{
			"file_path": "/workspace/generated/report.txt",
			"value":     "complete",
		},
	}

	_, _, err := executeToolsConcurrently(context.Background(), reg, nil, handler, "session-42", exec, []types.ToolUseBlock{toolUse}, nil)
	if err != nil {
		t.Fatalf("executeToolsConcurrently: %v", err)
	}
	req := handler.request
	if req.DecisionID == "" {
		t.Fatal("permission request has no stable decision ID")
	}
	if req.SessionID != "session-42" || req.ToolUseID != "toolu-write-7" {
		t.Fatalf("permission request lost session/tool identity: %+v", req)
	}
	if req.ActorID != "agent-reviewer" || req.ActorType != "reviewer" || req.WorkUnitID != "work-report" {
		t.Fatalf("permission request lost actor/work-unit identity: %+v", req)
	}
	if req.Action == "" || req.Target != "/workspace/generated/report.txt" || req.Impact == "" || req.RiskReason == "" {
		t.Fatalf("permission request is not reviewable: %+v", req)
	}
	if req.RuleSource == "" || req.ApprovalScope == "" {
		t.Fatalf("permission request lost policy provenance or approval scope: %+v", req)
	}
	if !req.Required {
		t.Fatal("permission request lost the tool-specific required-ask bit")
	}
	if req.PolicyDecision == nil || req.PolicyDecision.Code != policy.Code {
		t.Fatalf("permission request lost typed policy decision: %#v", req.PolicyDecision)
	}
	if want := []string{"allow_once", "reject"}; !reflect.DeepEqual(req.Choices, want) {
		t.Fatalf("permission choices = %v, want %v", req.Choices, want)
	}
}

func TestPermissionRequestUsesToolContractAsBaselineRuleSource(t *testing.T) {
	req := buildPermissionRequest("session", executioncontract.ToolExecutionContext{}, types.ToolUseBlock{
		ID: "toolu-custom", Name: "CustomTool", Input: map[string]any{"scope": "review"},
	}, types.ToolPermissionResult{Behavior: types.PermissionBehaviorAsk})

	if req.RuleSource == "tool permission policy" || !strings.Contains(req.RuleSource, "CustomTool") {
		t.Fatalf("baseline rule source = %q, want the requesting tool contract", req.RuleSource)
	}
}

func TestPermissionRequestPreservesSandboxCapability(t *testing.T) {
	req := buildPermissionRequest("session", executioncontract.ToolExecutionContext{}, types.ToolUseBlock{
		ID: "toolu-bash", Name: "Bash", Input: map[string]any{"command": "mkdir build"},
	}, types.ToolPermissionResult{
		Behavior: types.PermissionBehaviorPassthrough, Sandboxed: true, SandboxCapability: "capability-digest",
	})
	if !req.Sandboxed || req.SandboxCapability != "capability-digest" {
		t.Fatalf("sandbox capability was not preserved: %+v", req)
	}
}

func TestPermissionRequestBuildsLocalizedReviewBasisWithoutPrivateCause(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "project")
	sharedRoot := filepath.Join(projectRoot, "..", "shared")
	privateCause := errors.New("private analyzer diagnostic")
	policy := types.PolicyDecision{
		Disposition:  types.PolicyRequiredAsk,
		Code:         "shell.policy.ask.dynamic_target",
		PrivateCause: privateCause,
	}
	snapshot := types.ToolRuntimeContext{
		ProjectRoot: projectRoot,
		AllowedDirs: []string{projectRoot, sharedRoot},
	}
	permissionResult := types.ToolPermissionResult{
		Behavior:        types.PermissionBehaviorAsk,
		Message:         "private analyzer diagnostic",
		Required:        true,
		PolicyDecision:  &policy,
		RuntimeSnapshot: &snapshot,
		ToolMetadata:    types.ToolMetadata{ReadOnly: true},
	}
	tool := types.ToolUseBlock{
		ID: "toolu-read", Name: "Read",
		Input: map[string]any{"file_path": "docs/../report.txt"},
	}

	req := buildPermissionRequest("session", executioncontract.ToolExecutionContext{}, tool, permissionResult)
	details := strings.Join(req.ReviewDetails, "\n")
	for _, want := range []string{
		"Normalized path: " + normalizePermissionReviewPath("report.txt", projectRoot),
		"Allowed directory: " + normalizePermissionReviewPath(projectRoot, projectRoot),
		"Allowed directory: " + normalizePermissionReviewPath(sharedRoot, projectRoot),
		"Access: read-only",
		"Matched rule: shell.policy.ask.dynamic_target",
		"Required approval scope: this invocation",
	} {
		if !strings.Contains(details, want) {
			t.Fatalf("review details %q do not contain %q", details, want)
		}
	}
	if strings.Contains(details, privateCause.Error()) {
		t.Fatalf("private analyzer cause leaked into review details: %q", details)
	}

	localized := strings.Join(permissionReviewDetails(i18n.LangZH, "report.txt", tool, permissionResult), "\n")
	if !strings.Contains(localized, "规范化路径："+normalizePermissionReviewPath("report.txt", projectRoot)) || !strings.Contains(localized, "访问类型：只读") {
		t.Fatalf("localized review details = %q", localized)
	}
}

func TestPermissionReviewDetailsDoNotRepeatAlreadyNormalizedTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project", "report.txt")
	details := permissionReviewDetails(i18n.LangEN, path, types.ToolUseBlock{
		Name: "Write", Input: map[string]any{"file_path": path},
	}, types.ToolPermissionResult{ToolMetadata: types.ToolMetadata{Write: true}})
	joined := strings.Join(details, "\n")
	if strings.Contains(joined, "Normalized path:") || !strings.Contains(joined, "Access: write") {
		t.Fatalf("review details duplicated target or lost access type: %q", joined)
	}
}

func TestPlanDecisionPreservesFullBodyAndPlanSpecificChoices(t *testing.T) {
	plan := "# Migration plan\n\n" + strings.Repeat("- verify every migration invariant\n", 256) + "END OF PLAN"
	req := buildPermissionRequest("session-plan", executioncontract.ToolExecutionContext{
		ActorID:    "assistant",
		ActorType:  "planner",
		WorkUnitID: "work-plan",
	}, types.ToolUseBlock{
		ID:   "toolu-plan-1",
		Name: "ExitPlanMode",
		Input: map[string]any{
			"plan":             plan,
			"planFilePath":     "/workspace/PLAN.md",
			"postApprovalMode": "acceptEdits",
			"allowedPrompts":   []map[string]any{{"tool": "Bash", "prompt": "run tests"}},
		},
	}, types.ToolPermissionResult{
		Behavior: types.PermissionBehaviorAsk,
		Required: true,
		Message:  "Exit plan mode?",
	})

	if req.Kind != "plan" {
		t.Fatalf("decision kind = %q, want plan", req.Kind)
	}
	if req.Body != plan {
		t.Fatalf("plan body was truncated or transformed: got %d bytes, want %d", len(req.Body), len(plan))
	}
	if req.Target != "/workspace/PLAN.md" {
		t.Fatalf("plan target = %q", req.Target)
	}
	if want := []string{"execute", "stay_in_plan"}; !reflect.DeepEqual(req.Choices, want) {
		t.Fatalf("plan choices = %v, want %v", req.Choices, want)
	}
	if req.PostMode != "acceptEdits" {
		t.Fatalf("post-approval mode = %q, want acceptEdits", req.PostMode)
	}
	if len(req.ReviewDetails) != 1 || !strings.Contains(req.ReviewDetails[0], "Allowed prompts") {
		t.Fatalf("plan review details = %v", req.ReviewDetails)
	}
}
