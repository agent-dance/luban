package tools

import (
	"fmt"
	"os"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

const taskGetPrompt = `Use this tool to retrieve a task by its ID from the task list.

## When to Use This Tool

- When you need the full description and context before starting work on a task
- To understand task dependencies (what it blocks, what blocks it)
- After being assigned a task, to get complete requirements

## Output

Returns full task details:
- **subject**: Task title
- **description**: Detailed requirements and context
- **status**: 'pending', 'in_progress', or 'completed'
- **blocks**: Tasks waiting on this one to complete
- **blockedBy**: Tasks that must complete before this one can start

## Tips

- After fetching a task, verify its blockedBy list is empty before beginning work.
- Use TaskList to see all tasks in summary form.
`

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

func parseTaskGetInput(input map[string]any) (TaskGetInput, *types.ToolResult) {
	value, present := input["taskId"]
	if !present {
		return TaskGetInput{}, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolLegacyCInputFieldRequired, "taskId"), IsError: true}
	}
	if _, ok := value.(string); !ok {
		return TaskGetInput{}, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolLegacyCInputFieldString, "taskId"), IsError: true}
	}
	return parseStrictInputOrError[TaskGetInput](input)
}

func (t *TaskGetTool) Prompt() string { return taskGetPrompt }

func (t *TaskGetTool) UserFacingName() string { return "TaskGet" }

func (t *TaskGetTool) RenderToolUseMessage() *string { return nil }

func (t *TaskGetTool) IsReadOnly() bool { return true }

func (t *TaskGetTool) ToAutoClassifierInput(input map[string]any) string {
	taskID, _ := input["taskId"].(string)
	return taskID
}

func (t *TaskGetTool) ToolDiscoveryMetadata() registry.ToolDiscoveryMetadata {
	return registry.ToolDiscoveryMetadata{
		ShouldDefer: true,
		SearchHint:  "retrieve a task by ID",
	}
}

func (t *TaskGetTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func (t *TaskGetTool) ToolContract() types.ToolContract {
	taskSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":          map[string]any{"type": "string"},
			"subject":     map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
			"status":      map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
			"blocks":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"blockedBy":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []string{"id", "subject", "description", "status", "blocks", "blockedBy"},
	}
	return types.ToolContract{
		OutputSchema: &types.JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"task": map[string]any{"anyOf": []any{taskSchema, map[string]any{"type": "null"}}},
			},
			Required: []string{"task"},
		},
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 100_000,
	}
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
		return toolRuntimeText(i18n.KeyToolLegacyCTaskNotFound)
	}
	task := output.Task
	lines := []string{
		toolRuntimeFormat(i18n.KeyToolLegacyCTaskHeading, task.ID, task.Subject),
		toolRuntimeFormat(i18n.KeyToolLegacyCTaskStatus, task.Status),
		toolRuntimeFormat(i18n.KeyToolLegacyCTaskDescription, task.Description),
	}
	if len(task.BlockedBy) > 0 {
		blockedBy := make([]string, 0, len(task.BlockedBy))
		for _, blocker := range task.BlockedBy {
			blockedBy = append(blockedBy, "#"+blocker)
		}
		lines = append(lines, toolRuntimeFormat(i18n.KeyToolLegacyCTaskGetBlockedBy, strings.Join(blockedBy, ", ")))
	}
	if len(task.Blocks) > 0 {
		blocks := make([]string, 0, len(task.Blocks))
		for _, blocked := range task.Blocks {
			blocks = append(blocks, "#"+blocked)
		}
		lines = append(lines, toolRuntimeFormat(i18n.KeyToolLegacyCTaskBlocks, strings.Join(blocks, ", ")))
	}
	return strings.Join(lines, "\n")
}

func (t *TaskGetTool) taskListID() string {
	if t == nil || t.Store == nil {
		return "default"
	}
	if explicit := strings.TrimSpace(os.Getenv("CLAUDE_CODE_TASK_LIST_ID")); explicit != "" {
		return explicit
	}
	if teammateTeam := strings.TrimSpace(t.inProcessTeamName); teammateTeam != "" {
		return teammateTeam
	}
	return t.Store.taskListID()
}

func (t *TaskGetTool) withInProcessAgentID(agentID string) types.Tool {
	clone := *t
	clone.inProcessTeamName = teammateTeamName(agentID)
	return &clone
}
