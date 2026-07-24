package loop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func buildPermissionRequest(sessionID string, exec ToolExecutionContext, tool types.ToolUseBlock, permission types.ToolPermissionResult) PermissionRequest {
	lang := i18n.DetectOrLoadLanguage()
	request := PermissionRequest{
		SessionID:         sessionID,
		TurnID:            exec.TurnID,
		DecisionID:        "decision:" + sessionID + ":" + tool.ID,
		ToolUseID:         tool.ID,
		ToolName:          tool.Name,
		Input:             tool.Input,
		ActorID:           exec.ActorID,
		ActorType:         exec.ActorType,
		WorkUnitID:        exec.WorkUnitID,
		Kind:              "permission",
		Action:            i18n.Format(lang, i18n.KeyRuntimePermissionActionExecute, tool.Name),
		Target:            permissionTarget(lang, tool.Input),
		Impact:            strings.TrimSpace(permission.Message),
		RiskReason:        strings.TrimSpace(permission.Message),
		RuleSource:        i18n.Format(lang, i18n.KeyRuntimePermissionRuleToolContract, tool.Name),
		ApprovalScope:     i18n.Text(lang, i18n.KeyRuntimePermissionScopeInvocation),
		Choices:           []string{"allow_once", "reject", "always_allow"},
		Required:          permission.Required,
		Sandboxed:         permission.Sandboxed,
		SandboxCapability: permission.SandboxCapability,
		Message:           permission.Message,
		Suggestions:       append([]types.PermissionUpdate(nil), permission.Suggestions...),
		BlockedPath:       permission.BlockedPath,
		PolicyDecision:    clonePermissionPolicyDecision(permission.PolicyDecision),
	}
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
		if fallback, ok := tool.Input["gateFallbackReason"].(string); ok && strings.TrimSpace(fallback) != "" {
			request.ReviewDetails = append(request.ReviewDetails, i18n.Format(lang, i18n.KeyRuntimePlanAutoModeFallback, strings.TrimSpace(fallback)))
		}
	}
	return request
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
