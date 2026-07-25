package tasktools

import (
	"fmt"
	"os"
	"strings"

	"github.com/agent-dance/luban/i18n"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/contracts/toolmeta"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

type TaskGetResult struct {
	Task *TaskGetResultTask `json:"task"`
}

type TaskGetResultTask struct {
	ID          string   `json:"id"`
	Subject     string   `json:"subject"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Blocks      []string `json:"blocks"`
	BlockedBy   []string `json:"blockedBy"`
}

var _ agentcontract.ScopedTool = (*TaskGetTool)(nil)

func parseTaskGetInput(input map[string]any) (taskGetInput, *types.ToolResult) {
	value, present := input[taskIDField]
	if !present {
		return taskGetInput{}, &types.ToolResult{Content: runtimeFormat(i18n.KeyToolTaskInputFieldRequired, taskIDField), IsError: true}
	}
	if _, ok := value.(string); !ok {
		return taskGetInput{}, &types.ToolResult{Content: runtimeFormat(i18n.KeyToolTaskInputFieldString, taskIDField), IsError: true}
	}
	return toolbase.ParseStrictInputOrError[taskGetInput](input)
}

func (t *TaskGetTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{
		ShouldDefer: true,
		SearchHint:  runtimeText(i18n.KeyToolTaskGetDiscoveryHint),
	}
}

func (t *TaskGetTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func (t *TaskGetTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(TaskGetResult)
	if !ok {
		if pointer, pointerOK := data.(*TaskGetResult); pointerOK && pointer != nil {
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
		Content:   taskGetModelText(output),
		Data:      output,
	}
}

func taskGetModelText(output TaskGetResult) string {
	if output.Task == nil {
		return runtimeText(i18n.KeyToolTaskNotFound)
	}
	task := output.Task
	lines := []string{
		runtimeFormat(i18n.KeyToolTaskHeading, task.ID, task.Subject),
		runtimeFormat(i18n.KeyToolTaskStatus, task.Status),
		runtimeFormat(i18n.KeyToolTaskDescription, task.Description),
	}
	if len(task.BlockedBy) > 0 {
		blockedBy := make([]string, 0, len(task.BlockedBy))
		for _, blocker := range task.BlockedBy {
			blockedBy = append(blockedBy, "#"+blocker)
		}
		lines = append(lines, runtimeFormat(i18n.KeyToolTaskGetBlockedBy, strings.Join(blockedBy, ", ")))
	}
	if len(task.Blocks) > 0 {
		blocks := make([]string, 0, len(task.Blocks))
		for _, blocked := range task.Blocks {
			blocks = append(blocks, "#"+blocked)
		}
		lines = append(lines, runtimeFormat(i18n.KeyToolTaskBlocks, strings.Join(blocks, ", ")))
	}
	return strings.Join(lines, "\n")
}

func (t *TaskGetTool) taskListID() string {
	if t == nil || t.store == nil {
		return "default"
	}
	if explicit := strings.TrimSpace(os.Getenv("LUBAN_CODE_TASK_LIST_ID")); explicit != "" {
		return explicit
	}
	if teammateTeam := strings.TrimSpace(t.inProcessTeamName); teammateTeam != "" {
		return teammateTeam
	}
	return t.store.TaskListID()
}

func (t *TaskGetTool) BindAgentScope(agentID, _ string) types.Tool {
	clone := *t
	if _, teamName, ok := strings.Cut(strings.TrimSpace(agentID), "@"); ok {
		clone.inProcessTeamName = strings.TrimSpace(teamName)
	} else {
		clone.inProcessTeamName = ""
	}
	return &clone
}
