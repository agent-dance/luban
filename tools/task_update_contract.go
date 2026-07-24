package tools

import (
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

const taskUpdatePrompt = `Use this tool to update an existing task in the current task list.

## When to Use This Tool

- Mark a task in_progress before starting it, and completed as soon as it is done.
- Update subject, description, activeForm, owner, or metadata when the task changes.
- Add dependencies with addBlocks/addBlockedBy when ordering matters.
- Use status=deleted only when a task should be removed from the list.

## Important

- taskId is required and must exactly identify an existing task.
- Unknown input fields are rejected.
- Passing an empty string intentionally clears that string field.
- Metadata keys set to null are deleted.

## Examples

- {"taskId":"3","status":"in_progress","owner":"worker-a"}
- {"taskId":"3","status":"completed"}
- {"taskId":"4","addBlockedBy":["3"]}
`

type TaskUpdateStatusChange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type TaskUpdateResult struct {
	Success                 bool                    `json:"success"`
	TaskID                  string                  `json:"taskId,omitempty"`
	UpdatedFields           []string                `json:"updatedFields"`
	Error                   string                  `json:"error,omitempty"`
	StatusChange            *TaskUpdateStatusChange `json:"statusChange,omitempty"`
	VerificationNudgeNeeded bool                    `json:"verificationNudgeNeeded,omitempty"`
}

func (t *TaskUpdateTool) Prompt() string { return taskUpdatePrompt }

func (t *TaskUpdateTool) UserFacingName() string { return "TaskUpdate" }

func (t *TaskUpdateTool) RenderToolUseMessage() *string { return nil }

func (t *TaskUpdateTool) IsConcurrentSafe() bool { return true }

func (t *TaskUpdateTool) ToAutoClassifierInput(input map[string]any) string {
	parts := make([]string, 0, 3)
	for _, key := range []string{"taskId", "status", "subject"} {
		if value, ok := input[key].(string); ok && value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " ")
}

func (t *TaskUpdateTool) ToolDiscoveryMetadata() registry.ToolDiscoveryMetadata {
	return registry.ToolDiscoveryMetadata{
		ShouldDefer: true,
		SearchHint:  "update a task",
	}
}

func (t *TaskUpdateTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func (t *TaskUpdateTool) ToolContract() types.ToolContract {
	return types.ToolContract{
		OutputSchema: &types.JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"success":                 map[string]any{"type": "boolean"},
				"taskId":                  map[string]any{"type": "string"},
				"updatedFields":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"error":                   map[string]any{"type": "string"},
				"verificationNudgeNeeded": map[string]any{"type": "boolean"},
				"statusChange": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"from": map[string]any{"type": "string"},
						"to":   map[string]any{"type": "string"},
					},
					"required":             []string{"from", "to"},
					"additionalProperties": false,
				},
			},
			Required:             []string{"success", "updatedFields"},
			AdditionalProperties: false,
		},
		Strict:             true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 100_000,
	}
}

func (t *TaskUpdateTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(TaskUpdateResult)
	if !ok {
		if pointer, pointerOK := data.(*TaskUpdateResult); pointerOK && pointer != nil {
			output = *pointer
			ok = true
		}
	}
	if !ok {
		return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: fmt.Sprint(data), Data: data}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   taskUpdateModelText(output),
		Data:      output,
	}
}

func taskUpdateModelText(output TaskUpdateResult) string {
	if !output.Success {
		if strings.TrimSpace(output.Error) == "" {
			return toolRuntimeText(i18n.KeyToolLegacyCTaskUpdateFailed)
		}
		return output.Error
	}
	suffix := ""
	if output.VerificationNudgeNeeded {
		suffix = verificationNudgeText(verificationAgentType)
	}
	if len(output.UpdatedFields) == 0 {
		return toolRuntimeFormat(i18n.KeyToolLegacyCTaskUpdated, output.TaskID, suffix)
	}
	return toolRuntimeFormat(i18n.KeyToolLegacyCTaskUpdatedFields, output.TaskID, strings.Join(output.UpdatedFields, ", "), suffix)
}
