package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type ExitPlanModeStatus string

const (
	ExitPlanModeApproved ExitPlanModeStatus = "approved"
	ExitPlanModeAwaiting ExitPlanModeStatus = "awaiting"
	ExitPlanModeRejected ExitPlanModeStatus = "rejected"
)

// ExitPlanModeTool prepares a plan for approval, then commits the permission
// and PlanState transition only after the shared permission lifecycle approves.
type ExitPlanModeTool struct {
	State   *PlanState
	Runtime planModePermissionRuntime

	// AgentID is populated on the scoped teammate copy. Teammate approval is
	// handled in exit_plan_mode_teammate.go and bypasses the local dialog.
	AgentID          string
	PlanModeRequired bool
	TeamManager      *TeamManager
}

type exitPlanModeToolInput struct {
	AllowedPrompts []PlanAllowedPrompt `json:"allowedPrompts,omitempty"`
	Plan           *string             `json:"plan,omitempty"`
	PlanFilePath   string              `json:"planFilePath,omitempty"`
	ClearContext   bool                `json:"clearContext,omitempty"`
	HasTaskTool    bool                `json:"hasTaskTool,omitempty"`
	Feedback       string              `json:"feedback,omitempty"`
	PostMode       string              `json:"postApprovalMode,omitempty"`
	NeedsAuto      bool                `json:"needsAutoAttachment,omitempty"`
	FallbackReason string              `json:"gateFallbackReason,omitempty"`
}

// exitPlanModeResult mirrors the TS V2 data contract. Status/Feedback are
// programmatic lifecycle metadata and deliberately stay off the SDK JSON shape.
type exitPlanModeResult struct {
	Plan                   *string            `json:"plan"`
	IsAgent                bool               `json:"isAgent"`
	FilePath               string             `json:"filePath,omitempty"`
	HasTaskTool            bool               `json:"hasTaskTool,omitempty"`
	PlanWasEdited          bool               `json:"planWasEdited,omitempty"`
	AwaitingLeaderApproval bool               `json:"awaitingLeaderApproval,omitempty"`
	RequestID              string             `json:"requestId,omitempty"`
	Status                 ExitPlanModeStatus `json:"-"`
	Feedback               string             `json:"-"`
	GateFallbackReason     string             `json:"-"`
}

func NewExitPlanModeTool(state *PlanState, runtime ...planModePermissionRuntime) *ExitPlanModeTool {
	tool := &ExitPlanModeTool{State: state}
	if len(runtime) > 0 {
		tool.Runtime = runtime[0]
	}
	if state != nil && tool.Runtime != nil {
		state.bindPermissionRuntime(tool.Runtime)
	}
	return tool
}

func (t *ExitPlanModeTool) Name() string { return "ExitPlanMode" }

func (t *ExitPlanModeTool) Description() string {
	return "Prompts the user to exit plan mode and start coding"
}

func (t *ExitPlanModeTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"allowedPrompts": map[string]any{
				"type":        "array",
				"description": "Prompt-based permissions needed to implement the plan. These describe categories of actions rather than specific commands.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tool":   map[string]any{"type": "string", "enum": []string{"Bash"}},
						"prompt": map[string]any{"type": "string"},
					},
					"required": []string{"tool", "prompt"},
				},
			},
		},
		AdditionalProperties: true,
	}
}

func (t *ExitPlanModeTool) ToolContract() types.ToolContract {
	return types.ToolContract{
		OutputSchema: &types.JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"plan":                   map[string]any{"type": []string{"string", "null"}},
				"isAgent":                map[string]any{"type": "boolean"},
				"filePath":               map[string]any{"type": "string"},
				"hasTaskTool":            map[string]any{"type": "boolean"},
				"planWasEdited":          map[string]any{"type": "boolean"},
				"awaitingLeaderApproval": map[string]any{"type": "boolean"},
				"requestId":              map[string]any{"type": "string"},
			},
			Required: []string{"plan", "isAgent"},
		},
		ReadOnly:           false,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 100_000,
	}
}

func (t *ExitPlanModeTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	return !runtime.ChannelsActive && !AskUserChannelsActive()
}

func (t *ExitPlanModeTool) CheckPermissions(ctx context.Context, input map[string]any, _ types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	if t == nil || t.State == nil {
		return types.ToolPermissionResult{}, i18n.NewError(i18n.KeyToolLegacyAExitPlanStateRequired)
	}
	if strings.TrimSpace(t.AgentID) == "" && !t.State.IsActive() {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolPermissionText(i18n.KeyToolPermissionExitPlanNotActive)}, nil
	}
	normalized, err := t.NormalizeToolInput(ctx, input)
	if err != nil {
		return types.ToolPermissionResult{}, err
	}
	if strings.TrimSpace(t.AgentID) != "" {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: normalized}, nil
	}
	return types.ToolPermissionResult{
		Behavior:     types.PermissionBehaviorAsk,
		Message:      toolPermissionText(i18n.KeyToolPermissionExitPlanConfirm),
		UpdatedInput: normalized,
		Required:     true,
	}, nil
}

// NormalizeToolInput is the SDK/hook-facing projection. It injects the plan
// and planFilePath while keeping the model schema limited to allowedPrompts.
func (t *ExitPlanModeTool) NormalizeToolInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if t == nil || t.State == nil {
		return nil, i18n.NewError(i18n.KeyToolLegacyAExitPlanStateRequired)
	}
	updated := cloneExitPlanModeInput(input)
	planPath := strings.TrimSpace(t.State.PlanFile())
	if planPath == "" && strings.TrimSpace(t.AgentID) != "" {
		planPath = t.teammatePlanFilePath()
	}
	if supplied, _ := updated["planFilePath"].(string); strings.TrimSpace(supplied) != "" {
		if planPath != "" && filepathClean(supplied) != filepathClean(planPath) {
			return nil, i18n.NewError(i18n.KeyToolLegacyAExitPlanPathMismatch)
		}
		planPath = supplied
	}
	if planPath == "" {
		return nil, i18n.NewError(i18n.KeyToolLegacyAExitPlanNoActiveFile)
	}
	if _, supplied := updated["plan"]; !supplied {
		data, err := os.ReadFile(planPath)
		if err == nil {
			updated["plan"] = string(data)
		} else if strings.TrimSpace(t.AgentID) != "" && os.IsNotExist(err) {
			if plan := teammatePlanFromExecutionContext(ctx); strings.TrimSpace(plan) != "" {
				updated["plan"] = plan
			} else {
				return nil, i18n.WrapError(i18n.KeyToolLegacyAExitPlanReadFile, err, planPath)
			}
		} else {
			return nil, i18n.WrapError(i18n.KeyToolLegacyAExitPlanReadFile, err, planPath)
		}
	}
	updated["planFilePath"] = planPath
	if strings.TrimSpace(t.AgentID) == "" {
		restoreMode, needsAuto, fallbackReason := t.plannedRestoreMode(ctx)
		updated["postApprovalMode"] = restoreMode
		updated["needsAutoAttachment"] = needsAuto
		if fallbackReason != "" {
			updated["gateFallbackReason"] = fallbackReason
		} else {
			delete(updated, "gateFallbackReason")
		}
	}
	if _, err := decodeExitPlanModeInput(updated); err != nil {
		return nil, err
	}
	return updated, nil
}

func (t *ExitPlanModeTool) BackfillObservableInput(input map[string]any) (map[string]any, error) {
	return t.NormalizeToolInput(context.Background(), input)
}

func cloneExitPlanModeInput(input map[string]any) map[string]any {
	updated := make(map[string]any, len(input)+2)
	for key, value := range input {
		updated[key] = value
	}
	return updated
}

func filepathClean(path string) string {
	return filepath.Clean(strings.TrimSpace(path))
}

func decodeExitPlanModeInput(input map[string]any) (exitPlanModeToolInput, error) {
	var in exitPlanModeToolInput
	data, err := json.Marshal(input)
	if err != nil {
		return in, i18n.WrapError(i18n.KeyToolLegacyAExitPlanMarshalInput, err)
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return in, i18n.WrapError(i18n.KeyToolLegacyAExitPlanInvalidInput, err)
	}
	for _, prompt := range in.AllowedPrompts {
		if prompt.Tool != "Bash" || strings.TrimSpace(prompt.Prompt) == "" {
			return in, i18n.NewError(i18n.KeyToolLegacyAExitPlanInvalidPrompts)
		}
	}
	return in, nil
}

func (t *ExitPlanModeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if t == nil || t.State == nil {
		return types.ToolResult{}, i18n.NewError(i18n.KeyToolLegacyAExitPlanStateRequired)
	}
	if strings.TrimSpace(t.AgentID) != "" {
		return t.executeTeammateExit(ctx, input)
	}
	if !t.State.IsActive() {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyAExitPlanNotActive)), nil
	}
	if registry.ConsumePermissionCommit(ctx, t.Name(), input, "") != registry.PermissionCommitValid {
		return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolPermissionExitPlanConfirm), IsError: true}, nil
	}

	normalized := input
	if _, prepared := input["postApprovalMode"]; !prepared {
		var err error
		normalized, err = t.NormalizeToolInput(ctx, input)
		if err != nil {
			return types.ToolResult{}, err
		}
	}
	in, err := decodeExitPlanModeInput(normalized)
	if err != nil {
		return types.ToolResult{}, err
	}
	planPath := in.PlanFilePath
	plan := ""
	if in.Plan != nil {
		plan = *in.Plan
	}
	original, err := os.ReadFile(planPath)
	if err != nil {
		return types.ToolResult{}, i18n.WrapError(i18n.KeyToolLegacyAExitPlanReadBeforeExit, err, planPath)
	}
	planWasEdited := plan != string(original)
	if planWasEdited {
		if _, err := persistEditedPlan(planPath, plan); err != nil {
			return types.ToolResult{}, i18n.WrapError(i18n.KeyToolLegacyAExitPlanPersistEdited, err)
		}
	}

	restoreMode := in.PostMode
	needsAutoAttachment := in.NeedsAuto
	gateFallbackReason := in.FallbackReason
	allowed := filterAllowedPrompts(in.AllowedPrompts)
	if err := t.State.commitExit(allowed, restoreMode, needsAutoAttachment); err != nil {
		if planWasEdited {
			if rollbackErr := atomicWriteFile(planPath, original, 0o600); rollbackErr != nil {
				return types.ToolResult{}, i18n.WrapError(i18n.KeyToolLegacyAExitPlanCommitRollback, err, rollbackErr)
			}
		}
		return types.ToolResult{}, i18n.WrapError(i18n.KeyToolLegacyAExitPlanCommit, err)
	}

	result := exitPlanModeResult{
		Plan:               stringPointer(plan),
		IsAgent:            false,
		FilePath:           planPath,
		HasTaskTool:        in.HasTaskTool || t.runtimeHasTaskTool(),
		PlanWasEdited:      planWasEdited,
		Status:             ExitPlanModeApproved,
		GateFallbackReason: gateFallbackReason,
	}
	metadata := map[string]string{"exitPlanModeStatus": string(result.Status)}
	if in.ClearContext {
		metadata["clearContext"] = "true"
		metadata["restartExecution"] = "true"
	}
	if gateFallbackReason != "" {
		metadata["gateFallbackReason"] = gateFallbackReason
	}
	return types.ToolResult{Content: exitPlanModeModelText(result), Data: result, Metadata: metadata}, nil
}

func (t *ExitPlanModeTool) plannedRestoreMode(ctx context.Context) (string, bool, string) {
	restoreMode, _ := t.State.PrePlanState()["permission_mode"].(string)
	if strings.TrimSpace(restoreMode) == "" {
		restoreMode = permissionModeDefault
	}
	if restoreMode == "auto" {
		if allow, reason := autoModeGateAllow(ctx); !allow {
			return permissionModeDefault, true, reason
		}
	}
	return restoreMode, false, ""
}

func (t *ExitPlanModeTool) runtimeHasTaskTool() bool {
	if t == nil || t.Runtime == nil {
		return false
	}
	return t.Runtime.ToolRuntimeContext().FeatureEnabled(types.ToolFeatureTeams)
}

func stringPointer(value string) *string { return &value }

func (t *ExitPlanModeTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := data.(exitPlanModeResult)
	if !ok {
		if ptr, ptrOK := data.(*exitPlanModeResult); ptrOK && ptr != nil {
			result, ok = *ptr, true
		}
	}
	if !ok {
		return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: toolRuntimeText(i18n.KeyToolLegacyAExitPlanInvalidTypedData), IsError: true}
	}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: exitPlanModeModelText(result)}
}

func exitPlanModeModelText(result exitPlanModeResult) string {
	if result.AwaitingLeaderApproval {
		return toolRuntimeFormat(i18n.KeyToolLegacyAExitPlanAwaitingLeader, result.FilePath, result.RequestID)
	}
	if result.IsAgent {
		return toolRuntimeText(i18n.KeyToolLegacyAExitPlanAgentApproved)
	}
	plan := ""
	if result.Plan != nil {
		plan = *result.Plan
	}
	if strings.TrimSpace(plan) == "" {
		return toolRuntimeText(i18n.KeyToolLegacyAExitPlanApproved)
	}
	teamHint := ""
	if result.HasTaskTool {
		teamHint = toolRuntimeText(i18n.KeyToolLegacyAExitPlanTeamHint)
	}
	label := toolRuntimeText(i18n.KeyToolLegacyAExitPlanLabel)
	if result.PlanWasEdited {
		label = toolRuntimeText(i18n.KeyToolLegacyAExitPlanEditedLabel)
	}
	return toolRuntimeFormat(i18n.KeyToolLegacyAExitPlanApprovedBody, result.FilePath, teamHint, label, plan)
}

func (t *ExitPlanModeTool) MapToolPermissionRejection(input map[string]any, toolUseID, message string) types.ToolResultBlock {
	feedback, _ := input["feedback"].(string)
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		feedback = strings.TrimSpace(message)
	}
	content := toolRuntimeText(i18n.KeyToolLegacyAExitPlanRejected)
	if feedback != "" && feedback != toolRuntimeText(i18n.KeyToolPermissionExitPlanConfirm) {
		content += toolRuntimeFormat(i18n.KeyToolLegacyAExitPlanFeedback, feedback)
	}
	plan := ""
	if value, ok := input["plan"].(string); ok {
		plan = value
	}
	data := exitPlanModeResult{Plan: stringPointer(plan), FilePath: t.State.PlanFile(), Status: ExitPlanModeRejected, Feedback: feedback}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: content, Data: data, IsError: true, Metadata: map[string]string{"exitPlanModeStatus": string(ExitPlanModeRejected)}}
}
