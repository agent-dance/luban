package tools

import (
	"os"
	"strconv"
	"strings"
)

type TaskStoreEventType string

const (
	TaskStoreEventCreate TaskStoreEventType = "create"
	TaskStoreEventUpdate TaskStoreEventType = "update"
	TaskStoreEventDelete TaskStoreEventType = "delete"
	TaskStoreEventReset  TaskStoreEventType = "reset"
)

type TaskStoreEvent struct {
	Type   TaskStoreEventType `json:"type"`
	TaskID string             `json:"task_id,omitempty"`
	Task   *TaskItem          `json:"task,omitempty"`
}

type TaskStoreListener func(TaskStoreEvent) error

// Subscribe registers an in-process task mutation listener. Listener errors
// and panics are isolated from the durable mutation that triggered the event.
func (s *TaskStore) Subscribe(listener TaskStoreListener) func() {
	if s == nil || listener == nil {
		return func() {}
	}
	s.listenersMu.Lock()
	if s.listeners == nil {
		s.listeners = make(map[uint64]TaskStoreListener)
	}
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

func (s *TaskStore) notify(event TaskStoreEvent) {
	if s == nil {
		return
	}
	if event.Task != nil {
		event.Task = cloneTask(event.Task)
	}
	s.listenersMu.RLock()
	listeners := make([]TaskStoreListener, 0, len(s.listeners))
	for _, listener := range s.listeners {
		listeners = append(listeners, listener)
	}
	s.listenersMu.RUnlock()
	for _, listener := range listeners {
		func() {
			defer func() { _ = recover() }()
			_ = listener(event)
		}()
	}
}

// NotifyScopeChanged tells in-process consumers that the active task-list
// directory changed even though no row in either directory was mutated.
func (s *TaskStore) NotifyScopeChanged() {
	if s != nil {
		s.notify(TaskStoreEvent{Type: TaskStoreEventReset})
	}
}

// reset removes all persisted task records while retaining the high-water mark
// so IDs are never reused. It mirrors the TS reset lifecycle and emits one
// isolated reset notification after the durable mutation succeeds.
func (s *TaskStore) reset() error {
	err := withRuntimeFileLock(s.lockPath(), func() error {
		if err := s.ensureDir(); err != nil {
			return err
		}
		highest := s.readHighWaterMark()
		entries, err := os.ReadDir(s.tasksDir())
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			id, convErr := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".json"))
			if convErr == nil && id > highest {
				highest = id
			}
			if err := os.Remove(s.taskPath(strings.TrimSuffix(entry.Name(), ".json"))); err != nil {
				return err
			}
		}
		if highest > 0 {
			return s.writeHighWaterMark(highest)
		}
		return nil
	})
	if err == nil {
		s.notify(TaskStoreEvent{Type: TaskStoreEventReset})
	}
	return err
}
