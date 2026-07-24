package tools

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type TaskCreateResult struct {
	Task TaskCreateResultTask `json:"task"`
}

type TaskCreateResultTask struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
}

// TaskViewItem is the stable, UI-safe projection of a persisted task.
type TaskViewItem struct {
	ID        string
	Subject   string
	Status    string
	Owner     string
	BlockedBy []string
}

type TaskCreatedHookIdentity struct {
	TeammateName string
	TeamName     string
}

func parseTaskCreateInput(input map[string]any) (TaskCreateInput, *types.ToolResult) {
	for _, field := range []string{"subject", "description"} {
		if _, ok := input[field]; !ok {
			return TaskCreateInput{}, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolLegacyCInputFieldRequired, field), IsError: true}
		}
	}
	if value, ok := input["activeForm"]; ok && value == nil {
		return TaskCreateInput{}, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolLegacyCInputFieldString, "activeForm"), IsError: true}
	}
	if value, ok := input["metadata"]; ok {
		if value == nil {
			return TaskCreateInput{}, &types.ToolResult{Content: toolRuntimeText(i18n.KeyToolLegacyCInputMetadataObject), IsError: true}
		}
		if _, ok := value.(map[string]any); !ok {
			return TaskCreateInput{}, &types.ToolResult{Content: toolRuntimeText(i18n.KeyToolLegacyCInputMetadataObject), IsError: true}
		}
	}
	return parseStrictInputOrError[TaskCreateInput](input)
}

func (t *TaskCreateTool) IsConcurrentSafe() bool { return true }

func (t *TaskCreateTool) UserFacingName() string { return "TaskCreate" }

func (t *TaskCreateTool) RenderToolUseMessage() *string { return nil }

func (t *TaskCreateTool) ToAutoClassifierInput(input map[string]any) string {
	subject, _ := input["subject"].(string)
	return subject
}

func (t *TaskCreateTool) ToolDiscoveryMetadata() registry.ToolDiscoveryMetadata {
	return registry.ToolDiscoveryMetadata{
		ShouldDefer: true,
		SearchHint:  "create a task in the task list",
	}
}

func (t *TaskCreateTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func (t *TaskCreateTool) ToolContract() types.ToolContract {
	return types.ToolContract{
		OutputSchema: &types.JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"task": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":      map[string]any{"type": "string"},
						"subject": map[string]any{"type": "string"},
					},
					"required":             []string{"id", "subject"},
					"additionalProperties": false,
				},
			},
			Required:             []string{"task"},
			AdditionalProperties: false,
		},
		Strict:             true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 100_000,
	}
}

func (t *TaskCreateTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(TaskCreateResult)
	if !ok {
		if pointer, pointerOK := data.(*TaskCreateResult); pointerOK && pointer != nil {
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
		Content:   taskCreateModelText(output),
		Data:      output,
	}
}

func taskCreateModelText(output TaskCreateResult) string {
	return toolRuntimeFormat(i18n.KeyToolLegacyCTaskCreated, output.Task.ID, output.Task.Subject)
}

func (t *TaskCreateTool) SetHookRunner(runner *hooks.Runner) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.HookRunner = runner
	t.mu.Unlock()
}

func (t *TaskCreateTool) currentHookRunner() *hooks.Runner {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.HookRunner
}

func (t *TaskCreateTool) SetTaskViewNotifier(notifier func([]TaskViewItem)) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.taskViewNotifier = notifier
	t.mu.Unlock()
}

func (t *TaskCreateTool) notifyTaskView() {
	if t == nil {
		return
	}
	t.mu.RLock()
	notifier := t.taskViewNotifier
	t.mu.RUnlock()
	if notifier == nil {
		return
	}
	func() {
		defer func() { _ = recover() }()
		notifier(t.TaskViewSnapshot())
	}()
}

func (t *TaskCreateTool) TaskViewSnapshot() []TaskViewItem {
	if t == nil || t.Store == nil {
		return nil
	}
	tasks := t.Store.list()
	items := make([]TaskViewItem, 0, len(tasks))
	for _, task := range tasks {
		if internal, ok := task.Metadata["_internal"].(bool); ok && internal {
			continue
		}
		items = append(items, TaskViewItem{
			ID:        task.ID,
			Subject:   task.Subject,
			Status:    task.Status,
			Owner:     task.Owner,
			BlockedBy: append([]string(nil), task.BlockedBy...),
		})
	}
	return items
}

func (t *TaskCreateTool) taskCreatedHookIdentity() TaskCreatedHookIdentity {
	identity := TaskCreatedHookIdentity{TeammateName: strings.TrimSpace(os.Getenv("CLAUDE_CODE_AGENT_NAME"))}
	if t == nil || t.Store == nil || t.Store.scope == nil {
		return identity
	}
	identity.TeamName = t.Store.scope.TeamName()
	if identity.TeammateName == "" {
		identity.TeammateName = t.Store.scope.AgentID()
	}
	return identity
}

func taskCreatedHookFeedback(err error) string {
	if err == nil {
		return ""
	}
	reason := strings.TrimSpace(err.Error())
	var blocking *hooks.BlockingError
	if errors.As(err, &blocking) && strings.TrimSpace(blocking.Reason) != "" {
		reason = strings.TrimSpace(blocking.Reason)
	}
	return toolRuntimeFormat(i18n.KeyToolLegacyCTaskCreatedHookFeedback, reason)
}

const taskCreatePrompt = `Use this tool to create a structured task list for the current coding session. It helps track progress, organize complex tasks, and show overall progress to the user.

## When to Use This Tool

- Use it for complex work with three or more distinct steps, non-trivial planning, plan mode, explicit todo-list requests, or multiple user tasks.
- Capture new requirements as tasks, mark a task in_progress before work, and mark it completed as soon as it is done.

## When NOT to Use This Tool

- Skip it for one straightforward or trivial task, work completed in fewer than three trivial steps, and purely conversational requests.

## Task Fields

- subject: a brief actionable title in imperative form.
- description: enough detail to complete the work, including enough context for teammates when team mode is active.
- activeForm: optional present-continuous text shown while the task is in progress.

All new tasks start pending with no owner. Use TaskUpdate to assign teammates and establish dependencies through blocks/blockedBy. Check TaskList first to avoid duplicate tasks.`
