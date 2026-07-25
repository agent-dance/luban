package tasktools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	taskstore "github.com/agent-dance/luban/internal/store/tasks"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

const (
	verificationAgentType   = "verification"
	taskOutputDefaultMaxLen = 32000
	taskOutputUpperLimit    = 160000
)

func runtimeText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func runtimeFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func errorResponse(err error) types.ToolResult {
	return types.ToolResult{Content: err.Error(), IsError: true, Outcome: types.ToolOutcomeFailed}
}

func errorResponsef(format string, args ...any) types.ToolResult {
	return errorResponse(fmt.Errorf(format, args...))
}

func responseJSON(content any) (types.ToolResult, error) {
	data, err := json.Marshal(content)
	if err != nil {
		return types.ToolResult{Content: runtimeFormat(i18n.KeyToolRuntimeResponseMarshalFailed, err), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	return types.ToolResult{Content: string(data), Outcome: types.ToolOutcomeSucceeded}, nil
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func taskVerificationNudgeNeeded(tasks []*taskstore.Task) bool {
	if len(tasks) < 3 {
		return false
	}
	for _, task := range tasks {
		if task.Status != "completed" {
			return false
		}
		if containsVerificationHint(task.Subject) {
			return false
		}
	}
	return true
}

func getMaxTaskOutputLength() int {
	raw := strings.TrimSpace(os.Getenv("TASK_MAX_OUTPUT_LENGTH"))
	if raw == "" {
		return taskOutputDefaultMaxLen
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return taskOutputDefaultMaxLen
	}
	if value > taskOutputUpperLimit {
		return taskOutputUpperLimit
	}
	return value
}

func formatTaskOutputContent(output, outputPath string, forceTruncated bool) string {
	maxLen := getMaxTaskOutputLength()
	if !forceTruncated && len(output) <= maxLen {
		return output
	}

	header := runtimeFormat(i18n.KeyToolTaskOutputTruncated, outputPath)
	available := maxLen - len(header)
	if available <= 0 {
		if len(output) <= maxLen {
			return output
		}
		return output[len(output)-maxLen:]
	}
	if len(output) > available {
		output = output[len(output)-available:]
	}
	return header + output
}

type TaskCreateTool struct {
	store    *taskstore.Store
	identity IdentityResolver

	mu         sync.RWMutex
	HookRunner *hooks.Runner
}

type IdentityResolver interface {
	TeamName() string
	AgentID() string
}

func NewTaskCreateTool(store *taskstore.Store, identity IdentityResolver) *TaskCreateTool {
	return &TaskCreateTool{store: store, identity: identity}
}

func (t *TaskCreateTool) Name() string { return "TaskCreate" }

func (t *TaskCreateTool) Description() string {
	return runtimeText(i18n.KeyToolTaskCreateDescription)
}

func (t *TaskCreateTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"subject": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolTaskCreateInputSubjectDescription),
			},
			"description": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolTaskCreateInputDescriptionDescription),
			},
			"activeForm": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolTaskInputActiveFormDescription),
			},
			"metadata": map[string]any{
				"type":        "object",
				"description": runtimeText(i18n.KeyToolTaskCreateInputMetadataDescription),
			},
		},
		"subject", "description",
	)
}

func (t *TaskCreateTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := parseTaskCreateInput(input)
	if toolErr != nil {
		return *toolErr, nil
	}
	task, err := t.store.Create(in.Subject, in.Description, in.ActiveForm, in.Metadata)
	if err != nil {
		return errorResponsef("%s", runtimeFormat(i18n.KeyToolTaskCreateFailed, err)), nil
	}

	if runner := t.currentHookRunner(); runner != nil {
		if err := runTaskCreatedHook(ctx, runner, task, t.taskCreatedHookIdentity()); err != nil {
			_, _ = t.store.Delete(task.ID)
			return errorResponsef("%s", taskCreatedHookFeedback(err)), nil
		}
	}
	data := TaskCreateResult{Task: TaskCreateResultTask{ID: task.ID, Subject: task.Subject}}
	return types.ToolResult{Content: taskCreateModelText(data), Data: data}, nil
}

// runTaskCreatedHook invokes the blocking TaskCreated lifecycle event. A
// refusal is surfaced to TaskCreate, which deletes the freshly persisted task
// before returning the error.
func runTaskCreatedHook(ctx context.Context, runner *hooks.Runner, task *taskstore.Task, identity taskHookIdentity) error {
	if runner == nil || task == nil {
		return nil
	}
	input := hooks.HookInput{
		Type:            hooks.HookTaskCreated,
		HookEventName:   hooks.HookTaskCreated,
		TaskID:          task.ID,
		TaskSubject:     task.Subject,
		TaskDescription: task.Description,
		TeammateName:    identity.TeammateName,
		TeamName:        identity.TeamName,
		TaskOwner:       task.Owner,
		Owner:           task.Owner,
	}
	_, err := runner.RunBlockingDetailed(ctx, hooks.HookTaskCreated, input)
	return err
}

type TaskListTool struct{ store *taskstore.Store }

type TaskListResult struct {
	Tasks []TaskListResultItem `json:"tasks"`
}

type TaskListResultItem struct {
	ID        string   `json:"id"`
	Subject   string   `json:"subject"`
	Status    string   `json:"status"`
	Owner     string   `json:"owner,omitempty"`
	BlockedBy []string `json:"blockedBy"`
}

type taskListInput struct{}

func NewTaskListTool(store *taskstore.Store) *TaskListTool { return &TaskListTool{store: store} }

func (t *TaskListTool) Name() string { return "TaskList" }

func (t *TaskListTool) Description() string {
	return runtimeText(i18n.KeyToolTaskListDescription)
}

func (t *TaskListTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}

func (t *TaskListTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if _, toolErr := toolbase.ParseStrictInputOrError[taskListInput](input); toolErr != nil {
		return *toolErr, nil
	}
	allTasks := t.store.List()

	resolved := make(map[string]struct{})
	for _, task := range allTasks {
		if task.Status == "completed" {
			resolved[task.ID] = struct{}{}
		}
	}

	output := TaskListResult{Tasks: make([]TaskListResultItem, 0, len(allTasks))}
	for _, task := range allTasks {
		if value, ok := task.Metadata["_internal"]; ok {
			if internal, ok := value.(bool); ok && internal {
				continue
			}
		}

		item := TaskListResultItem{
			ID:        task.ID,
			Subject:   task.Subject,
			Status:    task.Status,
			Owner:     task.Owner,
			BlockedBy: make([]string, 0, len(task.BlockedBy)),
		}
		for _, blocker := range task.BlockedBy {
			if _, done := resolved[blocker]; !done {
				item.BlockedBy = append(item.BlockedBy, blocker)
			}
		}
		output.Tasks = append(output.Tasks, item)
	}
	return types.ToolResult{Content: taskListModelContent(output), Data: output}, nil
}

func (t *TaskListTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(TaskListResult)
	if !ok {
		return types.ToolResultBlock{
			ToolUseID: toolUseID,
			Content:   runtimeText(i18n.KeyToolTaskListInvalidResult),
			IsError:   true,
		}
	}
	return types.ToolResultBlock{ToolUseID: toolUseID, Content: taskListModelContent(output)}
}

func taskListModelContent(output TaskListResult) string {
	if len(output.Tasks) == 0 {
		return runtimeText(i18n.KeyToolTaskNoTasks)
	}
	lines := make([]string, 0, len(output.Tasks))
	for _, task := range output.Tasks {
		line := fmt.Sprintf("#%s [%s] %s", task.ID, task.Status, task.Subject)
		if task.Owner != "" {
			line += fmt.Sprintf(" (%s)", task.Owner)
		}
		if len(task.BlockedBy) > 0 {
			blocked := make([]string, 0, len(task.BlockedBy))
			for _, id := range task.BlockedBy {
				blocked = append(blocked, "#"+id)
			}
			line += runtimeFormat(i18n.KeyToolTaskBlockedBy, strings.Join(blocked, ", "))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

type TaskUpdateTool struct {
	store               *taskstore.Store
	identity            IdentityResolver
	verificationEnabled func() bool
	HookRunner          *hooks.Runner
	Runtime             types.ToolRuntimeContextProvider

	mu sync.RWMutex
}

func NewTaskUpdateTool(store *taskstore.Store, identity IdentityResolver, verificationEnabled func() bool) *TaskUpdateTool {
	return &TaskUpdateTool{store: store, identity: identity, verificationEnabled: verificationEnabled}
}

func (t *TaskUpdateTool) Name() string { return "TaskUpdate" }

func (t *TaskUpdateTool) Description() string {
	return runtimeText(i18n.KeyToolTaskUpdateDescription)
}

func (t *TaskUpdateTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"taskId":       map[string]any{"type": "string", "description": runtimeText(i18n.KeyToolTaskUpdateInputTaskIDDescription)},
			"subject":      map[string]any{"type": "string", "description": runtimeText(i18n.KeyToolTaskUpdateInputSubjectDescription)},
			"description":  map[string]any{"type": "string", "description": runtimeText(i18n.KeyToolTaskUpdateInputDescriptionDescription)},
			"activeForm":   map[string]any{"type": "string", "description": runtimeText(i18n.KeyToolTaskInputActiveFormDescription)},
			"status":       map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "deleted"}, "description": runtimeText(i18n.KeyToolTaskUpdateInputStatusDescription)},
			"addBlocks":    map[string]any{"type": "array", "description": runtimeText(i18n.KeyToolTaskUpdateInputAddBlocksDescription), "items": map[string]any{"type": "string"}},
			"addBlockedBy": map[string]any{"type": "array", "description": runtimeText(i18n.KeyToolTaskUpdateInputAddBlockedByDescription), "items": map[string]any{"type": "string"}},
			"owner":        map[string]any{"type": "string", "description": runtimeText(i18n.KeyToolTaskUpdateInputOwnerDescription)},
			"metadata":     map[string]any{"type": "object", "description": runtimeText(i18n.KeyToolTaskUpdateInputMetadataDescription)},
		},
		"taskId",
	)
}

func parseTaskUpdateInput(input map[string]any) (taskUpdateInput, *types.ToolResult) {
	value, present := input[taskIDField]
	if !present {
		return taskUpdateInput{}, &types.ToolResult{Content: runtimeFormat(i18n.KeyToolTaskInputFieldRequired, taskIDField), IsError: true}
	}
	if _, ok := value.(string); !ok {
		return taskUpdateInput{}, &types.ToolResult{Content: runtimeFormat(i18n.KeyToolTaskInputFieldString, taskIDField), IsError: true}
	}
	for _, field := range []string{"subject", "description", "activeForm", "status", "owner"} {
		if value, ok := input[field]; ok {
			if _, isString := value.(string); !isString {
				return taskUpdateInput{}, &types.ToolResult{Content: runtimeFormat(i18n.KeyToolTaskInputFieldString, field), IsError: true}
			}
		}
	}
	for _, field := range []string{"addBlocks", "addBlockedBy"} {
		value, ok := input[field]
		if !ok {
			continue
		}
		switch values := value.(type) {
		case []any:
			for _, entry := range values {
				if _, ok := entry.(string); !ok {
					return taskUpdateInput{}, &types.ToolResult{Content: runtimeFormat(i18n.KeyToolTaskInputFieldStringArray, field), IsError: true}
				}
			}
		case []string:
		default:
			return taskUpdateInput{}, &types.ToolResult{Content: runtimeFormat(i18n.KeyToolTaskInputFieldStringArray, field), IsError: true}
		}
	}
	if value, ok := input["metadata"]; ok {
		if value == nil {
			return taskUpdateInput{}, &types.ToolResult{Content: runtimeText(i18n.KeyToolTaskInputMetadataObject), IsError: true}
		}
		if _, ok := value.(map[string]any); !ok {
			return taskUpdateInput{}, &types.ToolResult{Content: runtimeText(i18n.KeyToolTaskInputMetadataObject), IsError: true}
		}
	}
	return toolbase.ParseStrictInputOrError[taskUpdateInput](input)
}

func (t *TaskUpdateTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := parseTaskUpdateInput(input)
	if toolErr != nil {
		return *toolErr, nil
	}

	taskID := in.TaskID
	if taskID == "" {
		data := TaskUpdateResult{Success: false, TaskID: taskID, UpdatedFields: []string{}, Error: runtimeText(i18n.KeyToolTaskIDRequired)}
		return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
	}

	existing, ok := t.store.Get(taskID)
	if !ok {
		data := TaskUpdateResult{Success: false, TaskID: taskID, UpdatedFields: []string{}, Error: runtimeText(i18n.KeyToolTaskNotFound)}
		return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
	}

	if in.Status != nil {
		switch *in.Status {
		case "pending", "in_progress", "completed", "deleted":
		default:
			return errorResponsef("%s", runtimeFormat(i18n.KeyToolTaskInvalidStatus, *in.Status)), nil
		}
	}

	if in.Status != nil && *in.Status == "deleted" {
		if _, ok := t.store.Delete(taskID); !ok {
			data := TaskUpdateResult{Success: false, TaskID: taskID, UpdatedFields: []string{}, Error: runtimeText(i18n.KeyToolTaskNotFound)}
			return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
		}
		data := TaskUpdateResult{
			Success:       true,
			TaskID:        taskID,
			UpdatedFields: []string{"deleted"},
			StatusChange:  &TaskUpdateStatusChange{From: existing.Status, To: "deleted"},
		}
		return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
	}

	updates := make(map[string]any)
	if in.Subject != nil && *in.Subject != existing.Subject {
		updates["subject"] = *in.Subject
	}
	if in.Description != nil && *in.Description != existing.Description {
		updates["description"] = *in.Description
	}
	if in.ActiveForm != nil && *in.ActiveForm != existing.ActiveForm {
		updates["activeForm"] = *in.ActiveForm
	}
	if in.Owner != nil && *in.Owner != existing.Owner {
		updates["owner"] = *in.Owner
	}
	if in.Status != nil && *in.Status != existing.Status {
		if *in.Status == "completed" {
			if runner := t.currentHookRunner(); runner != nil {
				if err := runTaskCompletedHook(ctx, runner, existing, t.taskUpdateHookIdentity()); err != nil {
					data := TaskUpdateResult{Success: false, TaskID: taskID, UpdatedFields: []string{}, Error: taskCompletedHookFeedback(err)}
					return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
				}
			}
		}
		updates["status"] = *in.Status
	}
	if in.Metadata != nil {
		metadata := taskstore.CloneMetadata(existing.Metadata)
		if metadata == nil {
			metadata = make(map[string]any)
		}
		for key, value := range in.Metadata {
			if value == nil {
				delete(metadata, key)
			} else {
				metadata[key] = value
			}
		}
		updates["metadata"] = metadata
	}

	task, updatedFields, ok := t.store.Update(taskID, updates)
	if !ok {
		data := TaskUpdateResult{Success: false, TaskID: taskID, UpdatedFields: []string{}, Error: runtimeText(i18n.KeyToolTaskNotFound)}
		return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
	}

	if len(in.AddBlocks) > 0 {
		newBlocks := 0
		for _, blockID := range in.AddBlocks {
			if containsString(existing.Blocks, blockID) {
				continue
			}
			if t.store.AddBlockingEdge(taskID, blockID) {
				newBlocks++
			}
		}
		if newBlocks > 0 {
			updatedFields = append(updatedFields, "blocks")
		}
	}

	if len(in.AddBlockedBy) > 0 {
		newBlockedBy := 0
		for _, blockerID := range in.AddBlockedBy {
			if containsString(existing.BlockedBy, blockerID) {
				continue
			}
			if t.store.AddBlockingEdge(blockerID, taskID) {
				newBlockedBy++
			}
		}
		if newBlockedBy > 0 {
			updatedFields = append(updatedFields, "blockedBy")
		}
	}

	if task == nil {
		data := TaskUpdateResult{Success: false, TaskID: taskID, UpdatedFields: []string{}, Error: runtimeText(i18n.KeyToolTaskNotFound)}
		return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
	}

	var statusChange *TaskUpdateStatusChange
	if in.Status != nil && *in.Status != existing.Status && containsString(updatedFields, "status") {
		statusChange = &TaskUpdateStatusChange{From: existing.Status, To: *in.Status}
	}
	verificationNudgeNeeded := false
	if in.Status != nil && *in.Status == "completed" && statusChange != nil && t.verificationNudgeRuntimeAllowed() {
		verificationNudgeNeeded = taskVerificationNudgeNeeded(t.store.List())
	}
	data := TaskUpdateResult{
		Success:                 true,
		TaskID:                  taskID,
		UpdatedFields:           updatedFields,
		StatusChange:            statusChange,
		VerificationNudgeNeeded: verificationNudgeNeeded,
	}
	return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
}

func (t *TaskUpdateTool) SetHookRunner(runner *hooks.Runner) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.HookRunner = runner
	t.mu.Unlock()
}

func (t *TaskUpdateTool) currentHookRunner() *hooks.Runner {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.HookRunner
}

func (t *TaskUpdateTool) taskUpdateHookIdentity() taskHookIdentity {
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

func runTaskCompletedHook(ctx context.Context, runner *hooks.Runner, task *taskstore.Task, identity taskHookIdentity) error {
	if runner == nil || task == nil {
		return nil
	}
	input := hooks.HookInput{
		Type:            hooks.HookTaskCompleted,
		HookEventName:   hooks.HookTaskCompleted,
		TaskID:          task.ID,
		TaskSubject:     task.Subject,
		TaskDescription: task.Description,
		TeammateName:    identity.TeammateName,
		TeamName:        identity.TeamName,
		TaskOwner:       task.Owner,
		Owner:           task.Owner,
	}
	_, err := runner.RunBlockingDetailed(ctx, hooks.HookTaskCompleted, input)
	return err
}

func taskCompletedHookFeedback(err error) string {
	if err == nil {
		return ""
	}
	reason := strings.TrimSpace(err.Error())
	var blocking *hooks.BlockingError
	if errors.As(err, &blocking) && strings.TrimSpace(blocking.Reason) != "" {
		reason = strings.TrimSpace(blocking.Reason)
	}
	return runtimeFormat(i18n.KeyToolTaskCompletedHookFeedback, reason)
}

func (t *TaskUpdateTool) verificationNudgeRuntimeAllowed() bool {
	if t == nil || t.verificationEnabled == nil || !t.verificationEnabled() {
		return false
	}
	if t == nil || t.Runtime == nil {
		return true
	}
	runtime := t.Runtime.ToolRuntimeContext()
	return strings.TrimSpace(runtime.AgentID) == ""
}

type TaskGetTool struct {
	store             *taskstore.Store
	inProcessTeamName string
}

func NewTaskGetTool(store *taskstore.Store) *TaskGetTool { return &TaskGetTool{store: store} }

func (t *TaskGetTool) Name() string { return "TaskGet" }

func (t *TaskGetTool) Description() string {
	return runtimeText(i18n.KeyToolTaskGetDescription)
}

func (t *TaskGetTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"taskId": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolTaskGetInputTaskIDDescription),
			},
		},
		"taskId",
	)
}

func (t *TaskGetTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := parseTaskGetInput(input)
	if toolErr != nil {
		return *toolErr, nil
	}

	data := TaskGetResult{}
	task, ok := t.store.GetFromList(t.taskListID(), in.TaskID)
	if !ok {
		return types.ToolResult{Content: taskGetModelText(data), Data: data}, nil
	}
	data.Task = &TaskGetResultTask{
		ID:          task.ID,
		Subject:     task.Subject,
		Description: task.Description,
		Status:      task.Status,
		Blocks:      append([]string{}, task.Blocks...),
		BlockedBy:   append([]string{}, task.BlockedBy...),
	}
	return types.ToolResult{Content: taskGetModelText(data), Data: data}, nil
}

type TaskStopTool struct {
	background BackgroundTasks
}

type BackgroundOutput struct {
	Content      string
	WasTruncated bool
}

type BackgroundTasks interface {
	Stop(string) (agentcontract.TaskSnapshot, error)
	Wait(string, time.Duration) (agentcontract.TaskSnapshot, string)
	Snapshot(string) (agentcontract.TaskSnapshot, bool)
	ReadOutput(agentcontract.TaskSnapshot, int64) (BackgroundOutput, error)
}

func NewTaskStopTool(background BackgroundTasks) *TaskStopTool {
	return &TaskStopTool{background: background}
}

func (t *TaskStopTool) Name() string { return "TaskStop" }
func (t *TaskStopTool) Description() string {
	return runtimeText(i18n.KeyToolTaskStopDescription)
}

func (t *TaskStopTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, Destructive: true}
}

func (t *TaskStopTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolTaskStopInputTaskIDDescription),
			},
		},
		"task_id",
	)
}

func (t *TaskStopTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if t.background == nil {
		return errorResponsef("%s", runtimeText(i18n.KeyToolTaskBackgroundUnavailable)), nil
	}

	in, toolErr := toolbase.ParseStrictInputOrError[taskStopInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}

	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		return errorResponsef("%s", runtimeText(i18n.KeyToolTaskBackgroundIDRequired)), nil
	}

	snap, err := t.background.Stop(taskID)
	if err != nil {
		return errorResponse(err), nil
	}

	command := snap.Command
	if command == "" {
		command = snap.Description
	}
	return responseJSON(map[string]any{
		"message":   runtimeFormat(i18n.KeyToolTaskStopped, snap.ID, command),
		"task_id":   snap.ID,
		"task_type": snap.Type,
		"command":   command,
	})
}

type TaskOutputTool struct {
	background BackgroundTasks
}

// TaskOutputPresentationData is the structured retrieval receipt consumed by
// presentation surfaces. ToolResult.Content is the canonical model-facing
// tool protocol, while local display code consumes Data and never parses the
// XML-like model payload to determine status or offsets.
type TaskOutputPresentationData struct {
	TaskID          string `json:"task_id"`
	TaskType        string `json:"task_type"`
	RetrievalStatus string `json:"retrieval_status"`
	TaskStatus      string `json:"task_status"`
	OutputBytes     int    `json:"output_bytes"`
	StartOffset     int64  `json:"start_offset"`
	EndOffset       int64  `json:"end_offset"`
	TotalBytes      int64  `json:"total_bytes"`
	WasTruncated    bool   `json:"was_truncated"`
	Block           bool   `json:"block"`
	ExitCode        *int   `json:"exit_code,omitempty"`
}

func NewTaskOutputTool(background BackgroundTasks) *TaskOutputTool {
	return &TaskOutputTool{background: background}
}

func (t *TaskOutputTool) Name() string { return "TaskOutput" }

func (t *TaskOutputTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true}
}

func (t *TaskOutputTool) Description() string {
	return runtimeText(i18n.KeyToolTaskOutputDescription)
}

func (t *TaskOutputTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolTaskOutputInputTaskIDDescription),
			},
			"block": map[string]any{
				"type":        "boolean",
				"description": runtimeText(i18n.KeyToolTaskOutputInputBlockDescription),
			},
			"timeout": map[string]any{
				"type":        "number",
				"description": runtimeText(i18n.KeyToolTaskOutputInputTimeoutDescription),
			},
		},
		"task_id",
	)
}

func (t *TaskOutputTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if t.background == nil {
		return errorResponsef("%s", runtimeText(i18n.KeyToolTaskBackgroundUnavailable)), nil
	}

	in, toolErr := toolbase.ParseStrictInputOrError[taskOutputInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}

	block := in.Block
	if _, ok := input["block"]; !ok {
		block = true
	}

	timeout := 30 * time.Second
	if in.Timeout > 0 {
		timeout = time.Duration(in.Timeout) * time.Millisecond
	}

	mode := "success"
	var snap agentcontract.TaskSnapshot
	var ok bool
	if block {
		snap, mode = t.background.Wait(in.TaskID, timeout)
		ok = mode != "missing"
	} else {
		snap, ok = t.background.Snapshot(in.TaskID)
		if ok {
			if snap.Status == "running" {
				mode = "not_ready"
			} else {
				mode = "success"
			}
		}
	}
	if !ok {
		return errorResponsef("%s", runtimeFormat(i18n.KeyToolTaskBackgroundNotFound, in.TaskID)), nil
	}

	outputResult, err := t.background.ReadOutput(snap, 64*1024)
	if err != nil && !os.IsNotExist(err) {
		return errorResponsef("%s", runtimeFormat(i18n.KeyToolTaskReadOutputFailed, err)), nil
	}

	parts := []string{
		fmt.Sprintf("<retrieval_status>%s</retrieval_status>", mode),
		fmt.Sprintf("<task_id>%s</task_id>", snap.ID),
		fmt.Sprintf("<task_type>%s</task_type>", snap.Type),
		fmt.Sprintf("<status>%s</status>", snap.Status),
	}
	if snap.ExitCode != nil {
		parts = append(parts, fmt.Sprintf("<exit_code>%d</exit_code>", *snap.ExitCode))
	}
	output := outputResult.Content
	if strings.TrimSpace(output) == "" && strings.TrimSpace(snap.Result) != "" {
		output = snap.Result
	}
	if outputResult.WasTruncated || len(output) > getMaxTaskOutputLength() {
		output = formatTaskOutputContent(output, snap.OutputPath, outputResult.WasTruncated)
	}
	if strings.TrimSpace(output) != "" {
		parts = append(parts, fmt.Sprintf("<output>\n%s\n</output>", strings.TrimRight(output, "\n")))
	}
	if snap.Error != "" {
		parts = append(parts, fmt.Sprintf("<error>%s</error>", snap.Error))
	}

	startOffset := int64(0)
	totalBytes := int64(len(output))
	if info, statErr := os.Stat(snap.OutputPath); statErr == nil && info.Mode().IsRegular() {
		totalBytes = info.Size()
		visibleBytes := int64(len(outputResult.Content))
		if outputResult.WasTruncated && totalBytes > visibleBytes {
			startOffset = totalBytes - visibleBytes
		}
	}
	data := TaskOutputPresentationData{
		TaskID: snap.ID, TaskType: snap.Type, RetrievalStatus: mode, TaskStatus: snap.Status,
		OutputBytes: len(output), StartOffset: startOffset, EndOffset: startOffset + int64(len(output)),
		TotalBytes: totalBytes, WasTruncated: outputResult.WasTruncated, Block: block,
	}
	if snap.ExitCode != nil {
		code := *snap.ExitCode
		data.ExitCode = &code
	}
	metadata := map[string]string{}
	completeness := types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete}
	if outputResult.WasTruncated {
		metadata["truncated"] = "true"
		metadata["warning"] = "true"
		completeness.Source = types.ToolResultCompletenessSourceTruncated
	}
	return types.ToolResult{Content: strings.Join(parts, "\n\n"), Data: data, Metadata: metadata, Completeness: completeness}, nil
}
