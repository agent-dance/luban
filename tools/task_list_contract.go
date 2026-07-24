package tools

import (
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

const taskListPrompt = `Use this tool to list all tasks in the current task list.

## When to Use This Tool

- Before starting task work, use TaskList to understand the current queue.
- Use it to identify blocked tasks, in-progress owners, and completed work.
- If a task is blocked, inspect its blockedBy list before updating or starting it.

## Output

Returns a summary of visible tasks:
- **id**: Task identifier
- **subject**: Task title
- **status**: 'pending', 'in_progress', or 'completed'
- **owner**: Optional teammate owner
- **blockedBy**: IDs of unresolved tasks that block this task

Internal tasks are hidden. Completed blockers are omitted from blockedBy so the
list reflects only actionable dependency edges.
`

func (t *TaskListTool) Prompt() string { return taskListPrompt }

func (t *TaskListTool) UserFacingName() string { return "TaskList" }

func (t *TaskListTool) RenderToolUseMessage() *string { return nil }

func (t *TaskListTool) IsReadOnly() bool { return true }

func (t *TaskListTool) ToAutoClassifierInput(map[string]any) string {
	return ""
}

func (t *TaskListTool) ToolDiscoveryMetadata() registry.ToolDiscoveryMetadata {
	return registry.ToolDiscoveryMetadata{
		ShouldDefer: true,
		SearchHint:  "list all tasks",
	}
}

func (t *TaskListTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}
