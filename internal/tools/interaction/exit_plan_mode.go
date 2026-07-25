package interaction

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/internal/store/secureio"
	"github.com/agent-dance/luban/types"
)

type exitPlanModeStatus string

const (
	exitPlanModeApproved exitPlanModeStatus = "approved"
	exitPlanModeRejected exitPlanModeStatus = "rejected"
)

// ExitPlanModeTool prepares a plan for approval, then commits the permission
// and PlanState transition only after the shared permission lifecycle approves.
type ExitPlanModeTool struct {
	State   *PlanState
	Runtime planModePermissionRuntime
}

type exitPlanModeToolInput struct {
	AllowedPrompts []PlanAllowedPrompt `json:"allowedPrompts,omitempty"`
	Plan           *string             `json:"plan,omitempty"`
	PlanFilePath   string              `json:"planFilePath,omitempty"`
	ClearContext   bool                `json:"clearContext,omitempty"`
	Feedback       string              `json:"feedback,omitempty"`
	PostMode       string              `json:"postApprovalMode,omitempty"`
}

// exitPlanModeResult mirrors the TS V2 data contract. Status/Feedback are
// programmatic lifecycle metadata and deliberately stay off the SDK JSON shape.
type exitPlanModeResult struct {
	Plan          *string            `json:"plan"`
	FilePath      string             `json:"filePath,omitempty"`
	HasTaskTool   bool               `json:"hasTaskTool,omitempty"`
	PlanWasEdited bool               `json:"planWasEdited,omitempty"`
	Status        exitPlanModeStatus `json:"-"`
	Feedback      string             `json:"-"`
}

func NewExitPlanModeTool(state *PlanState, runtime planModePermissionRuntime) *ExitPlanModeTool {
	tool := &ExitPlanModeTool{State: state, Runtime: runtime}
	if state != nil && runtime != nil {
		state.bindPermissionRuntime(runtime)
	}
	return tool
}

func (t *ExitPlanModeTool) Name() string { return "ExitPlanMode" }

func (t *ExitPlanModeTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func (t *ExitPlanModeTool) Description() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolInteractionExitPlanDescription)
}

func (t *ExitPlanModeTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"allowedPrompts": map[string]any{
				"type":        "array",
				"description": i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolInteractionExitPlanPermissions),
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

func (t *ExitPlanModeTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	return strings.TrimSpace(runtime.AgentID) == ""
}

func (t *ExitPlanModeTool) CheckPermissions(ctx context.Context, input map[string]any, _ types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	if t == nil || t.State == nil {
		return types.ToolPermissionResult{}, i18n.NewError(i18n.KeyToolPlanExitStateRequired)
	}
	if !t.State.IsActive() {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPermissionExitPlanNotActive)}, nil
	}
	normalized, err := t.NormalizeToolInput(ctx, input)
	if err != nil {
		return types.ToolPermissionResult{}, err
	}
	return types.ToolPermissionResult{
		Behavior:     types.PermissionBehaviorAsk,
		Message:      i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPermissionExitPlanConfirm),
		UpdatedInput: normalized,
		Required:     true,
	}, nil
}

// NormalizeToolInput is the SDK/hook-facing projection. It injects the plan
// and planFilePath while keeping the model schema limited to allowedPrompts.
func (t *ExitPlanModeTool) NormalizeToolInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if t == nil || t.State == nil {
		return nil, i18n.NewError(i18n.KeyToolPlanExitStateRequired)
	}
	updated := cloneExitPlanModeInput(input)
	planPath := strings.TrimSpace(t.State.PlanFile())
	if supplied, _ := updated["planFilePath"].(string); strings.TrimSpace(supplied) != "" {
		if planPath != "" && filepathClean(supplied) != filepathClean(planPath) {
			return nil, i18n.NewError(i18n.KeyToolPlanExitPathMismatch)
		}
		planPath = supplied
	}
	if planPath == "" {
		return nil, i18n.NewError(i18n.KeyToolPlanExitNoActiveFile)
	}
	if _, supplied := updated["plan"]; !supplied {
		data, err := os.ReadFile(planPath)
		if err == nil {
			updated["plan"] = string(data)
		} else {
			return nil, i18n.WrapError(i18n.KeyToolPlanExitReadFile, err, planPath)
		}
	}
	updated["planFilePath"] = planPath
	updated["postApprovalMode"] = t.plannedRestoreMode()
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
		return in, i18n.WrapError(i18n.KeyToolPlanExitMarshalInput, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		return in, i18n.WrapError(i18n.KeyToolPlanExitInvalidInput, err)
	}
	for _, prompt := range in.AllowedPrompts {
		if prompt.Tool != "Bash" || strings.TrimSpace(prompt.Prompt) == "" {
			return in, i18n.NewError(i18n.KeyToolPlanExitInvalidPrompts)
		}
	}
	return in, nil
}

func (t *ExitPlanModeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if t == nil || t.State == nil {
		return types.ToolResult{}, i18n.NewError(i18n.KeyToolPlanExitStateRequired)
	}
	if !t.State.IsActive() {
		return types.ToolResult{Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanExitNotActive), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	if approvalcommit.Consume(ctx, t.Name(), input, "") != approvalcommit.PermissionCommitValid {
		return types.ToolResult{Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPermissionExitPlanConfirm), IsError: true}, nil
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
		return types.ToolResult{}, i18n.WrapError(i18n.KeyToolPlanExitReadBeforeExit, err, planPath)
	}
	planWasEdited := plan != string(original)
	if planWasEdited {
		if _, err := persistEditedPlan(t.State.projectRootPath(), planPath, plan); err != nil {
			return types.ToolResult{}, i18n.WrapError(i18n.KeyToolPlanExitPersistEdited, err)
		}
	}

	restoreMode := in.PostMode
	allowed := filterAllowedPrompts(in.AllowedPrompts)
	if err := t.State.commitExit(allowed, restoreMode); err != nil {
		if planWasEdited {
			if rollbackErr := secureio.AtomicWriteFile(planPath, original, 0o600); rollbackErr != nil {
				return types.ToolResult{}, i18n.WrapError(i18n.KeyToolPlanExitCommitRollback, err, rollbackErr)
			}
		}
		return types.ToolResult{}, i18n.WrapError(i18n.KeyToolPlanExitCommit, err)
	}

	result := exitPlanModeResult{
		Plan:          stringPointer(plan),
		FilePath:      planPath,
		HasTaskTool:   t.runtimeHasTaskTool(),
		PlanWasEdited: planWasEdited,
		Status:        exitPlanModeApproved,
	}
	metadata := map[string]string{"exitPlanModeStatus": string(result.Status)}
	if in.ClearContext {
		metadata["clearContext"] = "true"
		metadata["restartExecution"] = "true"
	}
	return types.ToolResult{Content: exitPlanModeModelText(result), Data: result, Metadata: metadata}, nil
}

func (t *ExitPlanModeTool) plannedRestoreMode() string {
	restoreMode, _ := t.State.prePlanSnapshot()["permission_mode"].(string)
	if strings.TrimSpace(restoreMode) == "" {
		restoreMode = permissionModeDefault
	}
	return restoreMode
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
		return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanExitInvalidTypedData), IsError: true}
	}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: exitPlanModeModelText(result)}
}

func exitPlanModeModelText(result exitPlanModeResult) string {
	plan := ""
	if result.Plan != nil {
		plan = *result.Plan
	}
	if strings.TrimSpace(plan) == "" {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanExitApproved)
	}
	teamHint := ""
	if result.HasTaskTool {
		teamHint = i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanExitTeamHint)
	}
	label := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanExitLabel)
	if result.PlanWasEdited {
		label = i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanExitEditedLabel)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanExitApprovedBody, result.FilePath, teamHint, label, plan)
}

func (t *ExitPlanModeTool) MapToolPermissionRejection(input map[string]any, toolUseID, message string) types.ToolResultBlock {
	feedback, _ := input["feedback"].(string)
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		feedback = strings.TrimSpace(message)
	}
	content := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanExitRejected)
	if feedback != "" && feedback != i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPermissionExitPlanConfirm) {
		content += i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanExitFeedback, feedback)
	}
	plan := ""
	if value, ok := input["plan"].(string); ok {
		plan = value
	}
	data := exitPlanModeResult{Plan: stringPointer(plan), FilePath: t.State.PlanFile(), Status: exitPlanModeRejected, Feedback: feedback}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: content, Data: data, IsError: true, Metadata: map[string]string{"exitPlanModeStatus": string(exitPlanModeRejected)}}
}
