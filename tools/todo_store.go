package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type TodoStore struct {
	mu          sync.RWMutex
	projectRoot string
	pinnedRoot  bool
	scope       *RuntimeScope
	agentID     string
}

func NewTodoStore(cwd string) *TodoStore {
	return &TodoStore{
		projectRoot: filepath.Clean(cwd),
		scope:       NewRuntimeScope(cwd, true),
	}
}

func (s *TodoStore) SetScopeResolver(scope *RuntimeScope) {
	if scope != nil {
		s.mu.Lock()
		s.scope = scope
		s.mu.Unlock()
	}
}

func (s *TodoStore) SetProjectRoot(root string) {
	if strings.TrimSpace(root) == "" {
		return
	}
	s.mu.Lock()
	s.projectRoot = filepath.Clean(root)
	s.mu.Unlock()
}

func (s *TodoStore) withAgentID(agentID string) *TodoStore {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	clone := &TodoStore{projectRoot: s.projectRoot, pinnedRoot: s.pinnedRoot, scope: s.scope, agentID: strings.TrimSpace(agentID)}
	s.mu.RUnlock()
	return clone
}

func (s *TodoStore) withAgentScope(agentID, projectRoot string) *TodoStore {
	clone := s.withAgentID(agentID)
	if clone != nil && strings.TrimSpace(projectRoot) != "" {
		clone.mu.Lock()
		clone.projectRoot = filepath.Clean(projectRoot)
		clone.pinnedRoot = true
		// A retained agent must not retain the mutable foreground scope merely
		// to resolve a root and key that are already explicit on this clone.
		clone.scope = nil
		clone.mu.Unlock()
	}
	return clone
}

func (s *TodoStore) path() string {
	s.mu.RLock()
	root, pinnedRoot, scope, explicitAgentID := s.projectRoot, s.pinnedRoot, s.scope, strings.TrimSpace(s.agentID)
	s.mu.RUnlock()
	if scope != nil {
		runtime := scope.ToolRuntimeContext()
		if !pinnedRoot && strings.TrimSpace(runtime.ProjectRoot) != "" {
			root = runtime.ProjectRoot
		}
		todoKey := explicitAgentID
		if todoKey == "" {
			todoKey = strings.TrimSpace(runtime.AgentID)
		}
		if todoKey == "" {
			todoKey = strings.TrimSpace(runtime.SessionID)
		}
		if todoKey == "" {
			todoKey = "default"
		}
		return filepath.Join(root, ".claude", "todos", sanitizeTaskPathComponent(todoKey)+".json")
	}
	if explicitAgentID != "" {
		return filepath.Join(root, ".claude", "todos", sanitizeTaskPathComponent(explicitAgentID)+".json")
	}
	todoKey := strings.TrimSpace(os.Getenv("CLAUDE_SESSION_ID"))
	if todoKey == "" {
		todoKey = "default"
	}
	return filepath.Join(root, ".claude", "todos", sanitizeTaskPathComponent(todoKey)+".json")
}

func (s *TodoStore) Load() []TodoItem {
	path := s.path()
	value, err := withRuntimeFileLockResult(path+".lock", func() (any, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		var todos []TodoItem
		if err := json.Unmarshal(data, &todos); err != nil {
			return nil, err
		}
		return append([]TodoItem{}, todos...), nil
	})
	if err != nil {
		return nil
	}
	todos, _ := value.([]TodoItem)
	return todos
}

func (s *TodoStore) Save(todos []TodoItem) error {
	path := s.path()
	return withRuntimeFileLock(path+".lock", func() error {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		data, err := json.MarshalIndent(todos, "", "  ")
		if err != nil {
			return err
		}
		return atomicWriteFile(path, data, 0644)
	})
}

// LoadAndSave atomically loads the prior list, calls fn to compute the new
// list, then persists the result — all under a single file lock. Returns the
// pre-update snapshot so the caller can emit accurate diff observers.
//
// TD-04: TS exposes oldTodos+newTodos so observer hooks can react to status
// transitions (e.g., the verification nudge). The previous Go path performed
// Load and Save as separate locked critical sections, opening a race where
// two concurrent TodoWrite calls observe identical "old" snapshots and the
// second writer's diff appears as an unrelated transition. Wrapping the
// read-modify-write in one lock removes that window.
//
// fn must be deterministic: if it returns an error, the on-disk state is
// untouched. The returned old slice is a defensive copy.
func (s *TodoStore) LoadAndSave(fn func(old []TodoItem) ([]TodoItem, error)) ([]TodoItem, []TodoItem, error) {
	path := s.path()
	type result struct {
		oldTodos []TodoItem
		newTodos []TodoItem
	}
	value, err := withRuntimeFileLockResult(path+".lock", func() (any, error) {
		var oldTodos []TodoItem
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			if jsonErr := json.Unmarshal(data, &oldTodos); jsonErr != nil {
				return nil, jsonErr
			}
		} else if !os.IsNotExist(readErr) {
			return nil, readErr
		}
		newTodos, fnErr := fn(append([]TodoItem{}, oldTodos...))
		if fnErr != nil {
			return nil, fnErr
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		marshalled, marshalErr := json.MarshalIndent(newTodos, "", "  ")
		if marshalErr != nil {
			return nil, marshalErr
		}
		if writeErr := atomicWriteFile(path, marshalled, 0644); writeErr != nil {
			return nil, writeErr
		}
		return result{oldTodos: oldTodos, newTodos: newTodos}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	r, _ := value.(result)
	return r.oldTodos, r.newTodos, nil
}
