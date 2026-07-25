package tasktools

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/toolmeta"
	"github.com/agent-dance/luban/internal/tools/toolbase"
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

type taskHookIdentity struct {
	TeammateName string
	TeamName     string
}

func parseTaskCreateInput(input map[string]any) (taskCreateInput, *types.ToolResult) {
	for _, field := range []string{"subject", "description"} {
		if _, ok := input[field]; !ok {
			return taskCreateInput{}, &types.ToolResult{Content: runtimeFormat(i18n.KeyToolTaskInputFieldRequired, field), IsError: true}
		}
	}
	if value, ok := input[activeFormField]; ok && value == nil {
		return taskCreateInput{}, &types.ToolResult{Content: runtimeFormat(i18n.KeyToolTaskInputFieldString, activeFormField), IsError: true}
	}
	if value, ok := input["metadata"]; ok {
		if value == nil {
			return taskCreateInput{}, &types.ToolResult{Content: runtimeText(i18n.KeyToolTaskInputMetadataObject), IsError: true}
		}
		if _, ok := value.(map[string]any); !ok {
			return taskCreateInput{}, &types.ToolResult{Content: runtimeText(i18n.KeyToolTaskInputMetadataObject), IsError: true}
		}
	}
	return toolbase.ParseStrictInputOrError[taskCreateInput](input)
}

func (t *TaskCreateTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{
		ShouldDefer: true,
		SearchHint:  runtimeText(i18n.KeyToolTaskCreateDiscoveryHint),
	}
}

func (t *TaskCreateTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
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
	return runtimeFormat(i18n.KeyToolTaskCreated, output.Task.ID, output.Task.Subject)
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

func (t *TaskCreateTool) SubscribeChanges(listener func()) func() {
	if t == nil || t.store == nil {
		return func() {}
	}
	return t.store.Subscribe(listener)
}

func (t *TaskCreateTool) TaskViewSnapshot() []TaskViewItem {
	if t == nil || t.store == nil {
		return nil
	}
	tasks := t.store.List()
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

func (t *TaskCreateTool) taskCreatedHookIdentity() taskHookIdentity {
	identity := taskHookIdentity{TeammateName: strings.TrimSpace(os.Getenv("LUBAN_CODE_AGENT_NAME"))}
	if t == nil || t.identity == nil {
		return identity
	}
	identity.TeamName = t.identity.TeamName()
	if identity.TeammateName == "" {
		identity.TeammateName = t.identity.AgentID()
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
	return runtimeFormat(i18n.KeyToolTaskCreatedHookFeedback, reason)
}
