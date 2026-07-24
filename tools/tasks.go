package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// TaskItem represents a single persisted todo-v2 task.
type TaskItem struct {
	ID          string         `json:"id"`
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Owner       string         `json:"owner,omitempty"`
	Status      string         `json:"status"`
	Blocks      []string       `json:"blocks"`
	BlockedBy   []string       `json:"blockedBy"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Comments    []TaskComment  `json:"-"`
	CreatedAt   int64          `json:"-"`
	UpdatedAt   int64          `json:"-"`
	CompletedAt int64          `json:"-"`
}

// TaskComment mirrors TS comment payload: free-form text plus author tag and unix-ms timestamp.
type TaskComment struct {
	Author string `json:"author,omitempty"`
	Text   string `json:"text"`
	Ts     int64  `json:"ts,omitempty"`
}

const (
	verificationAgentType   = "verification"
	taskOutputDefaultMaxLen = 32000
	taskOutputUpperLimit    = 160000
)

func (t *TaskItem) GetJSON() string {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}

type TaskStore struct {
	baseDir string
	scope   *RuntimeScope

	listenersMu    sync.RWMutex
	listeners      map[uint64]TaskStoreListener
	nextListenerID uint64
}

func NewTaskStore() *TaskStore {
	return &TaskStore{
		baseDir:   filepath.Join(defaultClaudeHomeDir(), "tasks"),
		scope:     NewRuntimeScope("", true),
		listeners: make(map[uint64]TaskStoreListener),
	}
}

func (s *TaskStore) SetScopeResolver(scope *RuntimeScope) {
	if scope != nil {
		s.scope = scope
	}
}

func defaultClaudeHomeDir() string {
	if explicit := strings.TrimSpace(os.Getenv("CLAUDE_HOME")); explicit != "" {
		return explicit
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

func defaultTaskListID() string {
	for _, value := range []string{
		os.Getenv("CLAUDE_CODE_TASK_LIST_ID"),
		os.Getenv("CLAUDE_CODE_TEAM_NAME"),
		os.Getenv("CLAUDE_SESSION_ID"),
	} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return "default"
}

func (s *TaskStore) taskListID() string {
	if s.scope != nil {
		if id := strings.TrimSpace(s.scope.TaskListID()); id != "" {
			return id
		}
	}
	return defaultTaskListID()
}

func sanitizeTaskPathComponent(input string) string {
	var b strings.Builder
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			// The TS regex does not use the unicode flag, so a non-BMP rune is
			// replaced once for each UTF-16 surrogate code unit.
			for i := 0; i < utf16.RuneLen(r); i++ {
				b.WriteByte('-')
			}
		}
	}
	return b.String()
}

func (s *TaskStore) tasksDir() string {
	return s.tasksDirForTaskList(s.taskListID())
}

func (s *TaskStore) taskPath(taskID string) string {
	return s.taskPathForTaskList(s.taskListID(), taskID)

}

func (s *TaskStore) tasksDirForTaskList(taskListID string) string {
	return filepath.Join(s.baseDir, sanitizeTaskPathComponent(taskListID))
}

// DeleteTaskList removes the durable directory for a specific task-list scope.
// It is intentionally keyed by the supplied team/session name rather than the
// current RuntimeScope so TeamDelete can clear the team list even when an
// explicit CLAUDE_CODE_TASK_LIST_ID is active.
func (s *TaskStore) DeleteTaskList(taskListID string) error {
	if s == nil {
		return nil
	}
	taskListID = strings.TrimSpace(taskListID)
	if taskListID == "" {
		return nil
	}
	if err := os.RemoveAll(s.tasksDirForTaskList(taskListID)); err != nil {
		return err
	}
	s.notify(TaskStoreEvent{Type: TaskStoreEventReset})
	return nil
}

func (s *TaskStore) taskPathForTaskList(taskListID, taskID string) string {
	return filepath.Join(s.tasksDirForTaskList(taskListID), sanitizeTaskPathComponent(taskID)+".json")
}

func (s *TaskStore) highWaterMarkPath() string {
	return filepath.Join(s.tasksDir(), ".highwatermark")
}

func (s *TaskStore) lockPath() string {
	return filepath.Join(s.tasksDir(), ".lock")
}

func (s *TaskStore) ensureDir() error {
	return os.MkdirAll(s.tasksDir(), 0755)
}

func (s *TaskStore) readHighWaterMark() int {
	data, err := os.ReadFile(s.highWaterMarkPath())
	if err != nil {
		return 0
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return value
}

func (s *TaskStore) writeHighWaterMark(value int) error {
	return atomicWriteFile(s.highWaterMarkPath(), []byte(strconv.Itoa(value)), 0644)
}

func (s *TaskStore) nextTaskIDLocked() (string, error) {
	if err := s.ensureDir(); err != nil {
		return "", err
	}

	maxID := s.readHighWaterMark()
	entries, err := os.ReadDir(s.tasksDir())
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".json") {
				continue
			}
			idStr := strings.TrimSuffix(name, ".json")
			id, convErr := strconv.Atoi(idStr)
			if convErr == nil && id > maxID {
				maxID = id
			}
		}
	}
	nextID := maxID + 1
	return strconv.Itoa(nextID), nil
}

func (s *TaskStore) writeTaskLocked(task *TaskItem) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	if task.Blocks == nil {
		task.Blocks = []string{}
	}
	if task.BlockedBy == nil {
		task.BlockedBy = []string{}
	}
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWriteFile(s.taskPath(task.ID), data, 0644); err != nil {
		return err
	}
	numericID, err := strconv.Atoi(task.ID)
	if err == nil && numericID > s.readHighWaterMark() {
		if markErr := s.writeHighWaterMark(numericID); markErr != nil {
			return markErr
		}
	}
	return nil
}

func (s *TaskStore) create(subject, description string) string {
	task, _ := s.createDetailedWithError(subject, description, "", nil)
	if task == nil {
		return ""
	}
	return task.ID
}

func (s *TaskStore) createDetailed(subject, description, activeForm string, metadata map[string]any) *TaskItem {
	task, _ := s.createDetailedWithError(subject, description, activeForm, metadata)
	return task
}

func (s *TaskStore) createDetailedWithError(subject, description, activeForm string, metadata map[string]any) (*TaskItem, error) {
	value, err := withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
		id, err := s.nextTaskIDLocked()
		if err != nil {
			return nil, err
		}

		task := &TaskItem{
			ID:          id,
			Subject:     subject,
			Description: description,
			ActiveForm:  activeForm,
			Status:      "pending",
			Blocks:      []string{},
			BlockedBy:   []string{},
			Metadata:    copyMetadata(metadata),
			CreatedAt:   time.Now().UnixMilli(),
			UpdatedAt:   time.Now().UnixMilli(),
		}
		if err := s.writeTaskLocked(task); err != nil {
			return nil, err
		}
		return cloneTask(task), nil
	})
	if err != nil {
		return nil, err
	}
	task, _ := value.(*TaskItem)
	if task == nil {
		return nil, errors.New(toolRuntimeText(i18n.KeyToolLegacyCTaskCreationEmpty))
	}
	s.notify(TaskStoreEvent{Type: TaskStoreEventCreate, TaskID: task.ID, Task: cloneTask(task)})
	return task, nil
}

func (s *TaskStore) get(id string) (*TaskItem, bool) {
	return s.getForTaskList(s.taskListID(), id)
}

func (s *TaskStore) getForTaskList(taskListID, id string) (*TaskItem, bool) {
	data, err := os.ReadFile(s.taskPathForTaskList(taskListID, id))
	if err != nil {
		return nil, false
	}
	task, ok := decodeTaskRow(data)
	if !ok {
		return nil, false
	}
	return cloneTask(task), true
}

func (s *TaskStore) list() []*TaskItem {
	entries, err := os.ReadDir(s.tasksDir())
	if err != nil {
		return nil
	}

	items := make([]*TaskItem, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}

		data, readErr := os.ReadFile(filepath.Join(s.tasksDir(), name))
		if readErr != nil {
			continue
		}

		task, ok := decodeTaskRow(data)
		if !ok {
			continue
		}
		items = append(items, cloneTask(task))
	}

	sort.Slice(items, func(i, j int) bool {
		left, errLeft := strconv.Atoi(items[i].ID)
		right, errRight := strconv.Atoi(items[j].ID)
		if errLeft == nil && errRight == nil {
			return left < right
		}
		return items[i].ID < items[j].ID
	})
	return items
}

func (s *TaskStore) upsert(id, subject, description, status string) {
	err := withRuntimeFileLock(s.lockPath(), func() error {
		task, ok := s.readTaskLocked(id)
		if !ok {
			return nil
		}
		task.Subject = subject
		task.Description = description
		task.Status = status
		return s.writeTaskLocked(task)
	})
	if err == nil {
		if task, ok := s.get(id); ok {
			s.notify(TaskStoreEvent{Type: TaskStoreEventUpdate, TaskID: id, Task: task})
		}
	}
}

func (s *TaskStore) update(id, status string) (*TaskItem, bool) {
	value, err := withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
		task, ok := s.readTaskLocked(id)
		if !ok {
			return nil, nil
		}
		now := time.Now().UnixMilli()
		if task.Status != status {
			task.Status = status
			task.UpdatedAt = now
			if status == "completed" {
				task.CompletedAt = now
			}
		}
		if err := s.writeTaskLocked(task); err != nil {
			return nil, err
		}
		if status == "completed" {
			s.unblockDependentsLocked(id)
		}
		return cloneTask(task), nil
	})
	task, _ := value.(*TaskItem)
	if err != nil || task == nil {
		return nil, false
	}
	s.notify(TaskStoreEvent{Type: TaskStoreEventUpdate, TaskID: id, Task: cloneTask(task)})
	return task, true
}

func (s *TaskStore) delete(id string) (*TaskItem, bool) {
	value, err := withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
		task, ok := s.readTaskLocked(id)
		if !ok {
			return nil, nil
		}
		if numericID, err := strconv.Atoi(id); err == nil && numericID > s.readHighWaterMark() {
			_ = s.writeHighWaterMark(numericID)
		}
		if err := os.Remove(s.taskPath(id)); err != nil {
			return nil, err
		}

		entries, err := os.ReadDir(s.tasksDir())
		if err == nil {
			for _, entry := range entries {
				name := entry.Name()
				if entry.IsDir() || !strings.HasSuffix(name, ".json") {
					continue
				}

				taskID := strings.TrimSuffix(name, ".json")
				if taskID == id {
					continue
				}

				other, ok := s.readTaskLocked(taskID)
				if !ok {
					continue
				}

				newBlocks := filterStrings(other.Blocks, id)
				newBlockedBy := filterStrings(other.BlockedBy, id)
				if len(newBlocks) == len(other.Blocks) && len(newBlockedBy) == len(other.BlockedBy) {
					continue
				}

				other.Blocks = newBlocks
				other.BlockedBy = newBlockedBy
				_ = s.writeTaskLocked(other)
			}
		}

		return cloneTask(task), nil
	})
	task, _ := value.(*TaskItem)
	if err != nil || task == nil {
		return nil, false
	}
	s.notify(TaskStoreEvent{Type: TaskStoreEventDelete, TaskID: id, Task: cloneTask(task)})
	return task, true
}

func (s *TaskStore) updateDetailed(id string, updates map[string]any) (*TaskItem, []string, bool) {
	type result struct {
		task          *TaskItem
		updatedFields []string
	}
	value, err := withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
		task, ok := s.readTaskLocked(id)
		if !ok {
			return result{}, nil
		}

		var updatedFields []string
		if subject, ok := updates["subject"].(string); ok && subject != task.Subject {
			task.Subject = subject
			updatedFields = append(updatedFields, "subject")
		}
		if description, ok := updates["description"].(string); ok && description != task.Description {
			task.Description = description
			updatedFields = append(updatedFields, "description")
		}
		if activeForm, ok := updates["activeForm"].(string); ok && activeForm != task.ActiveForm {
			task.ActiveForm = activeForm
			updatedFields = append(updatedFields, "activeForm")
		}
		if owner, ok := updates["owner"].(string); ok && owner != task.Owner {
			task.Owner = owner
			updatedFields = append(updatedFields, "owner")
		}
		if status, ok := updates["status"].(string); ok && status != task.Status {
			task.Status = status
			updatedFields = append(updatedFields, "status")
			if status == "completed" {
				task.CompletedAt = time.Now().UnixMilli()
			}
		}
		if blocks, ok := updates["blocks"].([]string); ok {
			task.Blocks = append([]string{}, blocks...)
			updatedFields = append(updatedFields, "blocks")
		}
		if blockedBy, ok := updates["blockedBy"].([]string); ok {
			task.BlockedBy = append([]string{}, blockedBy...)
			updatedFields = append(updatedFields, "blockedBy")
		}
		if metadata, ok := updates["metadata"].(map[string]any); ok {
			task.Metadata = copyMetadata(metadata)
			updatedFields = append(updatedFields, "metadata")
		}

		if len(updatedFields) > 0 {
			task.UpdatedAt = time.Now().UnixMilli()
		}

		if err := s.writeTaskLocked(task); err != nil {
			return result{}, err
		}
		return result{task: cloneTask(task), updatedFields: updatedFields}, nil
	})
	out, _ := value.(result)
	if err != nil || out.task == nil {
		return nil, nil, false
	}
	s.notify(TaskStoreEvent{Type: TaskStoreEventUpdate, TaskID: id, Task: cloneTask(out.task)})
	return out.task, out.updatedFields, true
}

// unblockDependentsLocked removes id from BlockedBy lists of every other
// stored task. Caller must already hold the store lock. Errors writing
// individual entries are tolerated so a partial failure cannot strand the
// completion that triggered the unblock.
func (s *TaskStore) unblockDependentsLocked(id string) {
	entries, err := os.ReadDir(s.tasksDir())
	if err != nil {
		return
	}
	now := time.Now().UnixMilli()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		taskID := strings.TrimSuffix(name, ".json")
		if taskID == id {
			continue
		}
		other, ok := s.readTaskLocked(taskID)
		if !ok {
			continue
		}
		newBlockedBy := filterStrings(other.BlockedBy, id)
		newBlocks := filterStrings(other.Blocks, id)
		if len(newBlockedBy) == len(other.BlockedBy) && len(newBlocks) == len(other.Blocks) {
			continue
		}
		other.BlockedBy = newBlockedBy
		other.Blocks = newBlocks
		other.UpdatedAt = now
		_ = s.writeTaskLocked(other)
	}
}

func (s *TaskStore) blockTask(fromTaskID, toTaskID string) bool {
	value, err := withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
		if fromTaskID == toTaskID {
			return false, nil
		}
		fromTask, ok := s.readTaskLocked(fromTaskID)
		if !ok {
			return false, nil
		}
		toTask, ok := s.readTaskLocked(toTaskID)
		if !ok {
			return false, nil
		}

		// Circular dep detection: walk to.Blocks transitively; if it reaches
		// from, adding the new edge would create a cycle.
		if s.reachesLocked(toTaskID, fromTaskID) {
			return false, nil
		}

		now := time.Now().UnixMilli()
		mutated := false
		if !containsString(fromTask.Blocks, toTaskID) {
			fromTask.Blocks = append(fromTask.Blocks, toTaskID)
			fromTask.UpdatedAt = now
			mutated = true
		}
		if !containsString(toTask.BlockedBy, fromTaskID) {
			toTask.BlockedBy = append(toTask.BlockedBy, fromTaskID)
			toTask.UpdatedAt = now
			mutated = true
		}

		if !mutated {
			// Edge already present; nothing to do but report success.
			return true, nil
		}
		if err := s.writeTaskLocked(fromTask); err != nil {
			return false, err
		}
		if err := s.writeTaskLocked(toTask); err != nil {
			return false, err
		}
		return true, nil
	})
	ok, _ := value.(bool)
	if err == nil && ok {
		if task, found := s.get(fromTaskID); found {
			s.notify(TaskStoreEvent{Type: TaskStoreEventUpdate, TaskID: fromTaskID, Task: task})
		}
		if task, found := s.get(toTaskID); found {
			s.notify(TaskStoreEvent{Type: TaskStoreEventUpdate, TaskID: toTaskID, Task: task})
		}
	}
	return err == nil && ok
}

// reachesLocked returns true when start can reach target by following Blocks
// edges. Caller must already hold the store lock.
func (s *TaskStore) reachesLocked(start, target string) bool {
	if start == target {
		return true
	}
	visited := map[string]bool{}
	stack := []string{start}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[id] {
			continue
		}
		visited[id] = true
		task, ok := s.readTaskLocked(id)
		if !ok {
			continue
		}
		for _, next := range task.Blocks {
			if next == target {
				return true
			}
			if !visited[next] {
				stack = append(stack, next)
			}
		}
	}
	return false
}

func (s *TaskStore) readTaskLocked(id string) (*TaskItem, bool) {
	data, err := os.ReadFile(s.taskPath(id))
	if err != nil {
		return nil, false
	}

	task, ok := decodeTaskRow(data)
	if !ok {
		return nil, false
	}
	return task, true
}

func decodeTaskRow(data []byte) (*TaskItem, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
		return nil, false
	}

	decodeRequiredString := func(name string) (string, bool) {
		value, present := raw[name]
		if !present || strings.TrimSpace(string(value)) == "null" {
			return "", false
		}
		var decoded string
		if err := json.Unmarshal(value, &decoded); err != nil {
			return "", false
		}
		return decoded, true
	}
	decodeRequiredStrings := func(name string) ([]string, bool) {
		value, present := raw[name]
		if !present || strings.TrimSpace(string(value)) == "null" {
			return nil, false
		}
		var decoded []string
		if err := json.Unmarshal(value, &decoded); err != nil || decoded == nil {
			return nil, false
		}
		return decoded, true
	}

	id, idOK := decodeRequiredString("id")
	subject, subjectOK := decodeRequiredString("subject")
	description, descriptionOK := decodeRequiredString("description")
	status, statusOK := decodeRequiredString("status")
	blocks, blocksOK := decodeRequiredStrings("blocks")
	blockedBy, blockedByOK := decodeRequiredStrings("blockedBy")
	if !idOK || !subjectOK || !descriptionOK || !statusOK || !blocksOK || !blockedByOK {
		return nil, false
	}
	if os.Getenv("USER_TYPE") == "ant" {
		switch status {
		case "open":
			status = "pending"
		case "resolved":
			status = "completed"
		case "planning", "implementing", "reviewing", "verifying":
			status = "in_progress"
		}
	}
	switch status {
	case "pending", "in_progress", "completed":
	default:
		return nil, false
	}

	task := &TaskItem{
		ID:          id,
		Subject:     subject,
		Description: description,
		Status:      status,
		Blocks:      blocks,
		BlockedBy:   blockedBy,
	}
	for name, target := range map[string]*string{"activeForm": &task.ActiveForm, "owner": &task.Owner} {
		value, present := raw[name]
		if !present {
			continue
		}
		if strings.TrimSpace(string(value)) == "null" || json.Unmarshal(value, target) != nil {
			return nil, false
		}
	}
	if value, present := raw["metadata"]; present {
		if strings.TrimSpace(string(value)) == "null" || json.Unmarshal(value, &task.Metadata) != nil || task.Metadata == nil {
			return nil, false
		}
	}
	return task, true
}

func cloneTask(task *TaskItem) *TaskItem {
	if task == nil {
		return nil
	}
	cp := *task
	cp.Blocks = append([]string{}, task.Blocks...)
	cp.BlockedBy = append([]string{}, task.BlockedBy...)
	cp.Metadata = copyMetadata(task.Metadata)
	if task.Comments != nil {
		cp.Comments = append([]TaskComment{}, task.Comments...)
	}
	return &cp
}

func copyMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cp := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cp[key] = value
	}
	return cp
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func filterStrings(items []string, target string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != target {
			out = append(out, item)
		}
	}
	return out
}

func taskVerificationNudgeNeeded(tasks []*TaskItem) bool {
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

	header := toolRuntimeFormat(i18n.KeyToolLegacyCTaskOutputTruncated, outputPath)
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
	Store *TaskStore

	mu               sync.RWMutex
	HookRunner       *hooks.Runner
	taskViewNotifier func([]TaskViewItem)
}

func NewTaskCreateTool(store *TaskStore) *TaskCreateTool { return &TaskCreateTool{Store: store} }

func (t *TaskCreateTool) Name() string { return "TaskCreate" }

func (t *TaskCreateTool) Description() string {
	return "Create a new task in the task list\n\n" + taskCreatePrompt
}

func (t *TaskCreateTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"subject": map[string]any{
				"type":        "string",
				"description": "A brief title for the task",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "What needs to be done",
			},
			"activeForm": map[string]any{
				"type":        "string",
				"description": `Present continuous form shown in spinner when in_progress (e.g., "Running tests")`,
			},
			"metadata": map[string]any{
				"type":        "object",
				"description": "Arbitrary metadata to attach to the task",
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
	task, err := t.Store.createDetailedWithError(in.Subject, in.Description, in.ActiveForm, in.Metadata)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCTaskCreateFailed, err)), nil
	}

	if runner := t.currentHookRunner(); runner != nil {
		if err := runTaskCreatedHook(ctx, runner, task, t.taskCreatedHookIdentity()); err != nil {
			_, _ = t.Store.delete(task.ID)
			return ErrorResponsef("%s", taskCreatedHookFeedback(err)), nil
		}
	}
	t.notifyTaskView()
	data := TaskCreateResult{Task: TaskCreateResultTask{ID: task.ID, Subject: task.Subject}}
	return types.ToolResult{Content: taskCreateModelText(data), Data: data}, nil
}

// runTaskCreatedHook invokes the blocking TaskCreated lifecycle event. A
// refusal is surfaced to TaskCreate, which deletes the freshly persisted task
// before returning the error.
func runTaskCreatedHook(ctx context.Context, runner *hooks.Runner, task *TaskItem, identities ...TaskCreatedHookIdentity) error {
	if runner == nil || task == nil {
		return nil
	}
	var identity TaskCreatedHookIdentity
	if len(identities) > 0 {
		identity = identities[0]
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

type TaskListTool struct{ Store *TaskStore }

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

func NewTaskListTool(store *TaskStore) *TaskListTool { return &TaskListTool{Store: store} }

func (t *TaskListTool) Name() string           { return "TaskList" }
func (t *TaskListTool) IsConcurrentSafe() bool { return true }

func (t *TaskListTool) Description() string {
	return "List all tasks in the task list"
}

func (t *TaskListTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}

func (t *TaskListTool) ToolContract() types.ToolContract {
	return types.ToolContract{
		OutputSchema: &types.JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"tasks": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":        map[string]any{"type": "string"},
							"subject":   map[string]any{"type": "string"},
							"status":    map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
							"owner":     map[string]any{"type": "string"},
							"blockedBy": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
						"required": []string{"id", "subject", "status", "blockedBy"},
					},
				},
			},
			Required: []string{"tasks"},
		},
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 100_000,
	}
}

func (t *TaskListTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if _, toolErr := parseStrictInputOrError[taskListInput](input); toolErr != nil {
		return *toolErr, nil
	}
	allTasks := t.Store.list()

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
			Content:   toolRuntimeText(i18n.KeyToolLegacyCTaskListInvalidResult),
			IsError:   true,
		}
	}
	return types.ToolResultBlock{ToolUseID: toolUseID, Content: taskListModelContent(output)}
}

func taskListModelContent(output TaskListResult) string {
	if len(output.Tasks) == 0 {
		return toolRuntimeText(i18n.KeyToolLegacyCNoTasks)
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
			line += toolRuntimeFormat(i18n.KeyToolLegacyCTaskBlockedBy, strings.Join(blocked, ", "))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

type TaskUpdateTool struct {
	Store      *TaskStore
	HookRunner *hooks.Runner
	Runtime    types.ToolRuntimeContextProvider

	mu sync.RWMutex
}

func NewTaskUpdateTool(store *TaskStore) *TaskUpdateTool { return &TaskUpdateTool{Store: store} }

func (t *TaskUpdateTool) Name() string { return "TaskUpdate" }

func (t *TaskUpdateTool) Description() string {
	return "Update a task"
}

func (t *TaskUpdateTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"taskId":       map[string]any{"type": "string", "description": "The ID of the task to update"},
			"subject":      map[string]any{"type": "string", "description": "New subject for the task"},
			"description":  map[string]any{"type": "string", "description": "New description for the task"},
			"activeForm":   map[string]any{"type": "string", "description": `Present continuous form shown in spinner when in_progress (e.g., "Running tests")`},
			"status":       map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "deleted"}, "description": "New status for the task"},
			"addBlocks":    map[string]any{"type": "array", "description": "Task IDs that this task blocks", "items": map[string]any{"type": "string"}},
			"addBlockedBy": map[string]any{"type": "array", "description": "Task IDs that block this task", "items": map[string]any{"type": "string"}},
			"owner":        map[string]any{"type": "string", "description": "New owner for the task"},
			"metadata":     map[string]any{"type": "object", "description": "Metadata keys to merge into the task. Set a key to null to delete it."},
		},
		"taskId",
	)
}

func parseTaskUpdateInput(input map[string]any) (TaskUpdateInput, *types.ToolResult) {
	value, present := input["taskId"]
	if !present {
		return TaskUpdateInput{}, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolLegacyCInputFieldRequired, "taskId"), IsError: true}
	}
	if _, ok := value.(string); !ok {
		return TaskUpdateInput{}, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolLegacyCInputFieldString, "taskId"), IsError: true}
	}
	for _, field := range []string{"subject", "description", "activeForm", "status", "owner"} {
		if value, ok := input[field]; ok {
			if _, isString := value.(string); !isString {
				return TaskUpdateInput{}, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolLegacyCInputFieldString, field), IsError: true}
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
					return TaskUpdateInput{}, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolLegacyCInputFieldStringArray, field), IsError: true}
				}
			}
		case []string:
		default:
			return TaskUpdateInput{}, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolLegacyCInputFieldStringArray, field), IsError: true}
		}
	}
	if value, ok := input["metadata"]; ok {
		if value == nil {
			return TaskUpdateInput{}, &types.ToolResult{Content: toolRuntimeText(i18n.KeyToolLegacyCInputMetadataObject), IsError: true}
		}
		if _, ok := value.(map[string]any); !ok {
			return TaskUpdateInput{}, &types.ToolResult{Content: toolRuntimeText(i18n.KeyToolLegacyCInputMetadataObject), IsError: true}
		}
	}
	return parseStrictInputOrError[TaskUpdateInput](input)
}

func (t *TaskUpdateTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := parseTaskUpdateInput(input)
	if toolErr != nil {
		return *toolErr, nil
	}

	taskID := in.TaskID
	if taskID == "" {
		data := TaskUpdateResult{Success: false, TaskID: taskID, UpdatedFields: []string{}, Error: toolRuntimeText(i18n.KeyToolLegacyCTaskIDRequired)}
		return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
	}

	existing, ok := t.Store.get(taskID)
	if !ok {
		data := TaskUpdateResult{Success: false, TaskID: taskID, UpdatedFields: []string{}, Error: toolRuntimeText(i18n.KeyToolLegacyCTaskNotFound)}
		return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
	}

	if in.Status != nil {
		switch *in.Status {
		case "pending", "in_progress", "completed", "deleted":
		default:
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCInvalidTaskStatus, *in.Status)), nil
		}
	}

	if in.Status != nil && *in.Status == "deleted" {
		if _, ok := t.Store.delete(taskID); !ok {
			data := TaskUpdateResult{Success: false, TaskID: taskID, UpdatedFields: []string{}, Error: toolRuntimeText(i18n.KeyToolLegacyCTaskNotFound)}
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
		metadata := copyMetadata(existing.Metadata)
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

	task, updatedFields, ok := t.Store.updateDetailed(taskID, updates)
	if !ok {
		data := TaskUpdateResult{Success: false, TaskID: taskID, UpdatedFields: []string{}, Error: toolRuntimeText(i18n.KeyToolLegacyCTaskNotFound)}
		return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
	}

	if len(in.AddBlocks) > 0 {
		newBlocks := 0
		for _, blockID := range in.AddBlocks {
			if containsString(existing.Blocks, blockID) {
				continue
			}
			if t.Store.blockTask(taskID, blockID) {
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
			if t.Store.blockTask(blockerID, taskID) {
				newBlockedBy++
			}
		}
		if newBlockedBy > 0 {
			updatedFields = append(updatedFields, "blockedBy")
		}
	}

	if task == nil {
		data := TaskUpdateResult{Success: false, TaskID: taskID, UpdatedFields: []string{}, Error: toolRuntimeText(i18n.KeyToolLegacyCTaskNotFound)}
		return types.ToolResult{Content: taskUpdateModelText(data), Data: data}, nil
	}

	var statusChange *TaskUpdateStatusChange
	if in.Status != nil && *in.Status != existing.Status && containsString(updatedFields, "status") {
		statusChange = &TaskUpdateStatusChange{From: existing.Status, To: *in.Status}
	}
	verificationNudgeNeeded := false
	if in.Status != nil && *in.Status == "completed" && statusChange != nil && t.verificationNudgeRuntimeAllowed() {
		verificationNudgeNeeded = taskVerificationNudgeNeeded(t.Store.list())
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

func (t *TaskUpdateTool) taskUpdateHookIdentity() TaskCreatedHookIdentity {
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

func runTaskCompletedHook(ctx context.Context, runner *hooks.Runner, task *TaskItem, identity TaskCreatedHookIdentity) error {
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
	return toolRuntimeFormat(i18n.KeyToolLegacyCTaskCompletedHookFeedback, reason)
}

func (t *TaskUpdateTool) verificationNudgeRuntimeAllowed() bool {
	if !isVerificationAgentEnabled() {
		return false
	}
	if t == nil || t.Runtime == nil {
		return true
	}
	runtime := t.Runtime.ToolRuntimeContext()
	return strings.TrimSpace(runtime.AgentID) == ""
}

type TaskGetTool struct {
	Store             *TaskStore
	inProcessTeamName string
}

func NewTaskGetTool(store *TaskStore) *TaskGetTool { return &TaskGetTool{Store: store} }

func (t *TaskGetTool) Name() string           { return "TaskGet" }
func (t *TaskGetTool) IsConcurrentSafe() bool { return true }

func (t *TaskGetTool) Description() string {
	return "Get a task by ID from the task list"
}

func (t *TaskGetTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"taskId": map[string]any{
				"type":        "string",
				"description": "The ID of the task to retrieve",
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
	task, ok := t.Store.getForTaskList(t.taskListID(), in.TaskID)
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

type TaskStopTool struct{ Background *BackgroundTaskManager }

func NewTaskStopTool(background *BackgroundTaskManager) *TaskStopTool {
	return &TaskStopTool{Background: background}
}

func (t *TaskStopTool) Name() string        { return "TaskStop" }
func (t *TaskStopTool) Aliases() []string   { return []string{"KillShell"} }
func (t *TaskStopTool) Description() string { return "Stop a running background task by ID." }

func (t *TaskStopTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The ID of the background task to stop",
			},
			"shell_id": map[string]any{
				"type":        "string",
				"description": "Deprecated: use task_id instead",
			},
		},
	)
}

func (t *TaskStopTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if t.Background == nil {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCBackgroundUnavailable)), nil
	}

	in, toolErr := parseStrictInputOrError[TaskStopInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}

	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = strings.TrimSpace(in.ShellID)
	}
	if taskID == "" {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCBackgroundIDRequired)), nil
	}

	snap, err := t.Background.Stop(taskID)
	if err != nil {
		return ErrorResponse(err), nil
	}

	command := snap.Command
	if command == "" {
		command = snap.Description
	}
	return ResponseJSON(map[string]any{
		"message":   toolRuntimeFormat(i18n.KeyToolLegacyCTaskStopped, snap.ID, command),
		"task_id":   snap.ID,
		"task_type": snap.Type,
		"command":   command,
	})
}

type TaskOutputTool struct{ Background *BackgroundTaskManager }

// TaskOutputPresentationData is the structured retrieval receipt consumed by
// presentation surfaces. Content remains the compatibility payload; display
// code must not parse its XML-like prose to determine status or offsets.
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

func NewTaskOutputTool(background *BackgroundTaskManager) *TaskOutputTool {
	return &TaskOutputTool{Background: background}
}

func (t *TaskOutputTool) Name() string           { return "TaskOutput" }
func (t *TaskOutputTool) IsConcurrentSafe() bool { return true }

func (t *TaskOutputTool) Description() string {
	return "Retrieve output from a running or completed background task."
}

func (t *TaskOutputTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The task ID to get output from",
			},
			"block": map[string]any{
				"type":        "boolean",
				"description": "Whether to wait for completion",
			},
			"timeout": map[string]any{
				"type":        "number",
				"description": "Max wait time in ms",
			},
		},
		Required: []string{"task_id"},
	}
}

func (t *TaskOutputTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if t.Background == nil {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCBackgroundUnavailable)), nil
	}

	in, toolErr := parseInputOrError[TaskOutputInput](input)
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
	var snap BackgroundTaskSnapshot
	var ok bool
	if block {
		snap, mode = t.Background.Wait(in.TaskID, timeout)
		ok = mode != "missing"
	} else {
		snap, ok = t.Background.Snapshot(in.TaskID)
		if ok {
			if snap.Status == "running" {
				mode = "not_ready"
			} else {
				mode = "success"
			}
		}
	}
	if !ok {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCBackgroundTaskNotFound, in.TaskID)), nil
	}

	outputResult, err := readBackgroundTaskOutput(snap.OutputPath, 64*1024)
	if err != nil && !os.IsNotExist(err) {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCReadTaskOutputFailed, err)), nil
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
	if outputResult.WasTruncated {
		metadata["truncated"] = "true"
		metadata["warning"] = "true"
	}
	return types.ToolResult{Content: strings.Join(parts, "\n\n"), Data: data, Metadata: metadata}, nil
}
