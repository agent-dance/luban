package tasktools

import (
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/toolmeta"
	"github.com/agent-dance/luban/types"
)

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

func (t *TaskUpdateTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{
		ShouldDefer: true,
		SearchHint:  runtimeText(i18n.KeyToolTaskUpdateDiscoveryHint),
	}
}

func (t *TaskUpdateTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
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
			return runtimeText(i18n.KeyToolTaskUpdateFailed)
		}
		return output.Error
	}
	suffix := ""
	if output.VerificationNudgeNeeded {
		suffix = verificationNudgeText(verificationAgentType)
	}
	if len(output.UpdatedFields) == 0 {
		return runtimeFormat(i18n.KeyToolTaskUpdated, output.TaskID, suffix)
	}
	return runtimeFormat(i18n.KeyToolTaskUpdatedFields, output.TaskID, strings.Join(output.UpdatedFields, ", "), suffix)
}
