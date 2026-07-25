package taskstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	storepaths "github.com/agent-dance/luban/internal/store/paths"
	"github.com/agent-dance/luban/internal/store/secureio"
)

var errEmptyCreateResult = errors.New("task store create invariant violated")

// Task is a durable task-list entry.
type Task struct {
	ID          string         `json:"id"`
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Owner       string         `json:"owner,omitempty"`
	Status      string         `json:"status"`
	Blocks      []string       `json:"blocks"`
	BlockedBy   []string       `json:"blockedBy"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type ScopeResolver func() string

type Store struct {
	baseDir string
	scope   ScopeResolver

	listenersMu    sync.RWMutex
	listeners      map[uint64]func()
	nextListenerID uint64
}

func New(scope ScopeResolver) *Store {
	return &Store{
		baseDir:   filepath.Join(storepaths.ConfigHomeDir(), "tasks"),
		scope:     scope,
		listeners: make(map[uint64]func()),
	}
}

func (s *Store) TaskListID() string {
	if s != nil && s.scope != nil {
		if id := strings.TrimSpace(s.scope()); id != "" {
			return id
		}
	}
	return "default"
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
			b.WriteByte('-')
		}
	}
	return b.String()
}

func (s *Store) tasksDir() string {
	return s.tasksDirForTaskList(s.TaskListID())
}

func (s *Store) tasksDirForTaskList(taskListID string) string {
	return filepath.Join(s.baseDir, sanitizeTaskPathComponent(taskListID))
}

func (s *Store) taskPath(taskID string) string {
	return s.taskPathForTaskList(s.TaskListID(), taskID)

}

func (s *Store) taskPathForTaskList(taskListID, taskID string) string {
	return filepath.Join(s.tasksDirForTaskList(taskListID), sanitizeTaskPathComponent(taskID)+".json")
}

func (s *Store) highWaterMarkPath() string {
	return filepath.Join(s.tasksDir(), ".highwatermark")
}

func (s *Store) lockPath() string {
	return filepath.Join(s.tasksDir(), ".lock")
}

func (s *Store) ensureDir() error {
	return os.MkdirAll(s.tasksDir(), 0700)
}

func (s *Store) readHighWaterMark() int {
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

func (s *Store) writeHighWaterMark(value int) error {
	return secureio.AtomicWriteFile(s.highWaterMarkPath(), []byte(strconv.Itoa(value)), 0644)
}

func (s *Store) nextTaskIDLocked() (string, error) {
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

func (s *Store) writeTaskLocked(task *Task) error {
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
	if err := secureio.AtomicWriteFile(s.taskPath(task.ID), data, 0600); err != nil {
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

func (s *Store) Create(subject, description, activeForm string, metadata map[string]any) (*Task, error) {
	value, err := secureio.WithRuntimeFileLockResult(s.lockPath(), func() (any, error) {
		id, err := s.nextTaskIDLocked()
		if err != nil {
			return nil, err
		}

		task := &Task{
			ID:          id,
			Subject:     subject,
			Description: description,
			ActiveForm:  activeForm,
			Status:      "pending",
			Blocks:      []string{},
			BlockedBy:   []string{},
			Metadata:    CloneMetadata(metadata),
		}
		if err := s.writeTaskLocked(task); err != nil {
			return nil, err
		}
		return cloneTask(task), nil
	})
	if err != nil {
		return nil, err
	}
	task, _ := value.(*Task)
	if task == nil {
		return nil, errEmptyCreateResult
	}
	s.notify()
	return task, nil
}

func (s *Store) Get(id string) (*Task, bool) {
	return s.GetFromList(s.TaskListID(), id)
}

func (s *Store) GetFromList(taskListID, id string) (*Task, bool) {
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

func (s *Store) List() []*Task {
	entries, err := os.ReadDir(s.tasksDir())
	if err != nil {
		return nil
	}

	items := make([]*Task, 0, len(entries))
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

func (s *Store) Delete(id string) (*Task, bool) {
	value, err := secureio.WithRuntimeFileLockResult(s.lockPath(), func() (any, error) {
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
	task, _ := value.(*Task)
	if err != nil || task == nil {
		return nil, false
	}
	s.notify()
	return task, true
}

func (s *Store) Update(id string, updates map[string]any) (*Task, []string, bool) {
	type result struct {
		task          *Task
		updatedFields []string
	}
	value, err := secureio.WithRuntimeFileLockResult(s.lockPath(), func() (any, error) {
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
			task.Metadata = CloneMetadata(metadata)
			updatedFields = append(updatedFields, "metadata")
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
	s.notify()
	return out.task, out.updatedFields, true
}

func (s *Store) AddBlockingEdge(fromTaskID, toTaskID string) bool {
	value, err := secureio.WithRuntimeFileLockResult(s.lockPath(), func() (any, error) {
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

		mutated := false
		if !containsString(fromTask.Blocks, toTaskID) {
			fromTask.Blocks = append(fromTask.Blocks, toTaskID)
			mutated = true
		}
		if !containsString(toTask.BlockedBy, fromTaskID) {
			toTask.BlockedBy = append(toTask.BlockedBy, fromTaskID)
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
		s.notify()
	}
	return err == nil && ok
}

// reachesLocked returns true when start can reach target by following Blocks
// edges. Caller must already hold the store lock.
func (s *Store) reachesLocked(start, target string) bool {
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

func (s *Store) readTaskLocked(id string) (*Task, bool) {
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

func decodeTaskRow(data []byte) (*Task, bool) {
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
	switch status {
	case "pending", "in_progress", "completed":
	default:
		return nil, false
	}

	task := &Task{
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

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	cp := *task
	cp.Blocks = append([]string{}, task.Blocks...)
	cp.BlockedBy = append([]string{}, task.BlockedBy...)
	cp.Metadata = CloneMetadata(task.Metadata)
	return &cp
}

func CloneMetadata(metadata map[string]any) map[string]any {
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

func (s *Store) Subscribe(listener func()) func() {
	if s == nil || listener == nil {
		return func() {}
	}
	s.listenersMu.Lock()
	s.nextListenerID++
	id := s.nextListenerID
	s.listeners[id] = listener
	s.listenersMu.Unlock()
	return func() {
		s.listenersMu.Lock()
		delete(s.listeners, id)
		s.listenersMu.Unlock()
	}
}

func (s *Store) notify() {
	if s == nil {
		return
	}
	s.listenersMu.RLock()
	listeners := make([]func(), 0, len(s.listeners))
	for _, listener := range s.listeners {
		listeners = append(listeners, listener)
	}
	s.listenersMu.RUnlock()
	for _, listener := range listeners {
		func() {
			defer func() { _ = recover() }()
			listener()
		}()
	}
}

func (s *Store) Invalidate() {
	if s != nil {
		s.notify()
	}
}
