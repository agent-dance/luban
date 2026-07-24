package tools

import (
	"errors"
	"testing"
)

// fakeAgentTodoStore is a minimal in-memory store used to drive
// CleanupAgentTodosForAgent under test.
type fakeAgentTodoStore struct {
	items []TodoItem
}

func (f *fakeAgentTodoStore) LoadAndSave(mutator func(prior []TodoItem) ([]TodoItem, error)) ([]TodoItem, []TodoItem, error) {
	prior := append([]TodoItem(nil), f.items...)
	next, err := mutator(prior)
	if err != nil {
		return prior, prior, err
	}
	f.items = append([]TodoItem(nil), next...)
	return prior, next, nil
}

func TestCleanupAgentTodosForAgent_PrunesByTag(t *testing.T) {
	store := &fakeAgentTodoStore{items: []TodoItem{
		{Content: "parent task", ActiveForm: "doing parent", Status: "pending"},
		{Content: "agent work [agent:agent_x]", ActiveForm: "doing x", Status: "in_progress"},
		{Content: "another agent step", ActiveForm: "doing other [agent:agent_x]", Status: "pending"},
		{Content: "task for [agent:agent_y]", ActiveForm: "doing y", Status: "pending"},
	}}
	pruned, err := CleanupAgentTodosForAgent(store, "agent_x")
	if err != nil {
		t.Fatalf("cleanup err: %v", err)
	}
	if pruned != 2 {
		t.Fatalf("expected 2 pruned, got %d", pruned)
	}
	if len(store.items) != 2 {
		t.Fatalf("expected 2 remaining, got %d", len(store.items))
	}
	if store.items[0].Content != "parent task" {
		t.Fatalf("parent task lost: %+v", store.items)
	}
	if store.items[1].Content != "task for [agent:agent_y]" {
		t.Fatalf("y task lost: %+v", store.items)
	}
}

func TestCleanupAgentTodosForAgent_NoOpOnEmptyAgentID(t *testing.T) {
	store := &fakeAgentTodoStore{items: []TodoItem{
		{Content: "x [agent:foo]"},
	}}
	pruned, err := CleanupAgentTodosForAgent(store, "")
	if err != nil || pruned != 0 || len(store.items) != 1 {
		t.Fatalf("expected no-op, got pruned=%d items=%d err=%v", pruned, len(store.items), err)
	}
}

func TestCleanupAgentTodosForAgent_PropagatesError(t *testing.T) {
	failing := failingAgentTodoStore{err: errors.New("disk full")}
	if _, err := CleanupAgentTodosForAgent(failing, "x"); err == nil || err.Error() != "disk full" {
		t.Fatalf("expected disk full error, got %v", err)
	}
}

type failingAgentTodoStore struct {
	err error
}

func (f failingAgentTodoStore) LoadAndSave(mutator func(prior []TodoItem) ([]TodoItem, error)) ([]TodoItem, []TodoItem, error) {
	return nil, nil, f.err
}
