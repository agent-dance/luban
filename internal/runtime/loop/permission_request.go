package loop

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/i18n"
	permissioncontract "github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

func buildPermissionRequest(sessionID string, exec executioncontract.ToolExecutionContext, tool types.ToolUseBlock, permission types.ToolPermissionResult) permissioncontract.PermissionRequest {
	lang := i18n.DetectOrLoadLanguage()
	request := permissioncontract.PermissionRequest{
		SessionID:              sessionID,
		TurnID:                 exec.TurnID,
		DecisionID:             "decision:" + sessionID + ":" + tool.ID,
		ToolUseID:              tool.ID,
		ToolName:               tool.Name,
		Input:                  tool.Input,
		ActorID:                exec.ActorID,
		ActorType:              exec.ActorType,
		WorkUnitID:             exec.WorkUnitID,
		Kind:                   "permission",
		Action:                 i18n.Format(lang, i18n.KeyRuntimePermissionActionExecute, tool.Name),
		Target:                 permissionTarget(lang, tool.Input),
		Impact:                 strings.TrimSpace(permission.Message),
		RiskReason:             strings.TrimSpace(permission.Message),
		RuleSource:             i18n.Format(lang, i18n.KeyRuntimePermissionRuleToolContract, tool.Name),
		ApprovalScope:          i18n.Text(lang, i18n.KeyRuntimePermissionScopeInvocation),
		Choices:                []string{"allow_once", "reject", "always_allow"},
		Required:               permission.Required,
		ToolLocalReadOnlyAllow: permission.ToolLocalReadOnlyAllow,
		Sandboxed:              permission.Sandboxed,
		SandboxCapability:      permission.SandboxCapability,
		Message:                permission.Message,
		Suggestions:            append([]types.PermissionUpdate(nil), permission.Suggestions...),
		BlockedPath:            permission.BlockedPath,
		PolicyDecision:         clonePermissionPolicyDecision(permission.PolicyDecision),
	}
	request.Description = request.Action
	if request.ActorID == "" {
		request.ActorID = "assistant"
	}
	if request.Impact == "" {
		request.Impact = i18n.Text(lang, i18n.KeyRuntimePermissionImpactDefault)
	}
	if permission.Required {
		request.RuleSource = i18n.Text(lang, i18n.KeyRuntimePermissionRuleRequired)
		// A bypass-immune decision is valid for this invocation only. Do not
		// present a persistent "always allow" choice that the execution gate is
		// required to ignore.
		request.Choices = []string{"allow_once", "reject"}
	}
	if tool.Name == "ExitPlanMode" {
		request.Kind = "plan"
		request.Action = i18n.Text(lang, i18n.KeyRuntimePlanActionExecute)
		request.Description = request.Action
		request.Impact = i18n.Text(lang, i18n.KeyRuntimePlanImpactExecute)
		request.RiskReason = i18n.Text(lang, i18n.KeyRuntimePlanRiskExecute)
		request.RuleSource = i18n.Text(lang, i18n.KeyRuntimePlanRuleGate)
		request.ApprovalScope = i18n.Text(lang, i18n.KeyRuntimePlanScopeTransition)
		request.Choices = []string{"execute", "stay_in_plan"}
		if body, ok := tool.Input["plan"].(string); ok {
			request.Body = body
		}
		if path, ok := tool.Input["planFilePath"].(string); ok && strings.TrimSpace(path) != "" {
			request.Target = path
		}
		if mode, ok := tool.Input["postApprovalMode"].(string); ok {
			request.PostMode = strings.TrimSpace(mode)
		}
		if allowed, ok := tool.Input["allowedPrompts"]; ok {
			if encoded, err := json.MarshalIndent(allowed, "", "  "); err == nil && string(encoded) != "null" && string(encoded) != "[]" {
				request.ReviewDetails = append(request.ReviewDetails, i18n.Format(lang, i18n.KeyRuntimePlanAllowedPrompts, string(encoded)))
			}
		}
	}
	if request.Kind == "permission" {
		request.ReviewDetails = append(request.ReviewDetails, permissionReviewDetails(lang, request.Target, tool, permission)...)
	}
	return request
}

func permissionReviewDetails(lang i18n.Language, displayedTarget string, tool types.ToolUseBlock, permission types.ToolPermissionResult) []string {
	details := make([]string, 0, 6)
	projectRoot := ""
	if permission.RuntimeSnapshot != nil {
		projectRoot = permission.RuntimeSnapshot.ProjectRoot
	}
	if rawPath := permissionReviewPath(tool.Input, permission.BlockedPath); rawPath != "" {
		if normalized := normalizePermissionReviewPath(rawPath, projectRoot); normalized != "" && normalized != strings.TrimSpace(displayedTarget) {
			details = append(details, i18n.Format(lang, i18n.KeyRuntimePermissionReviewNormalizedPath, normalized))
		}
	}
	if permission.RuntimeSnapshot != nil {
		seen := make(map[string]struct{}, len(permission.RuntimeSnapshot.AllowedDirs))
		for _, directory := range permission.RuntimeSnapshot.AllowedDirs {
			normalized := normalizePermissionReviewPath(directory, projectRoot)
			if normalized == "" {
				continue
			}
			if _, exists := seen[normalized]; exists {
				continue
			}
			seen[normalized] = struct{}{}
			details = append(details, i18n.Format(lang, i18n.KeyRuntimePermissionReviewAllowedDir, normalized))
		}
	}
	accessKey := i18n.KeyRuntimePermissionAccessExecute
	if permission.ToolMetadata.Write || permission.ToolMetadata.Destructive {
		accessKey = i18n.KeyRuntimePermissionAccessWrite
	} else if permission.ToolMetadata.ReadOnly {
		accessKey = i18n.KeyRuntimePermissionAccessReadOnly
	}
	details = append(details, i18n.Format(lang, i18n.KeyRuntimePermissionReviewAccess, i18n.Text(lang, accessKey)))
	if rule := matchedPermissionReviewRule(tool.Name, permission); rule != "" {
		details = append(details, i18n.Format(lang, i18n.KeyRuntimePermissionReviewMatchedRule, rule))
	}
	if permission.Required {
		details = append(details, i18n.Text(lang, i18n.KeyRuntimePermissionReviewRequiredScope))
	}
	return details
}

func permissionReviewPath(input map[string]any, blockedPath string) string {
	for _, key := range []string{"file_path", "notebook_path", "path"} {
		if value, ok := input[key]; ok {
			if path := strings.TrimSpace(fmt.Sprint(value)); path != "" {
				return path
			}
		}
	}
	return strings.TrimSpace(blockedPath)
}

func normalizePermissionReviewPath(path, projectRoot string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		if root := strings.TrimSpace(projectRoot); root != "" {
			path = filepath.Join(root, path)
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return toolbase.DisplayPath(filepath.Clean(path))
	}
	return toolbase.DisplayPath(abs)
}

func matchedPermissionReviewRule(toolName string, permission types.ToolPermissionResult) string {
	if permission.PolicyDecision != nil {
		if code := strings.TrimSpace(permission.PolicyDecision.Code); code != "" {
			return code
		}
	}
	if permission.RuntimeSnapshot == nil {
		return ""
	}
	blockedPath := strings.TrimSpace(permission.BlockedPath)
	for _, rule := range permission.RuntimeSnapshot.AskRules {
		if !strings.EqualFold(strings.TrimSpace(rule.ToolName), strings.TrimSpace(toolName)) {
			continue
		}
		content := strings.TrimSpace(rule.RuleContent)
		if blockedPath != "" && content != blockedPath {
			continue
		}
		if content != "" {
			return content
		}
		return strings.TrimSpace(rule.ToolName)
	}
	return ""
}

func clonePermissionPolicyDecision(source *types.PolicyDecision) *types.PolicyDecision {
	if source == nil {
		return nil
	}
	cloned := source.Clone()
	return &cloned
}

func permissionTarget(lang i18n.Language, input map[string]any) string {
	for _, key := range []string{"file_path", "path", "notebook_path", "command", "url", "query"} {
		if value, ok := input[key]; ok {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" {
				return text
			}
		}
	}
	return i18n.Text(lang, i18n.KeyRuntimePermissionTargetInput)
}
