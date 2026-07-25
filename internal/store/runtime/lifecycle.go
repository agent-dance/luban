package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/internal/store/secureio"
)

// RuntimeLifecycleEventType is the stable event vocabulary shared by tool,
// task, cron, worktree, team, and MCP lifecycle integrations.
type RuntimeLifecycleEventType string

const (
	LifecycleToolStart           RuntimeLifecycleEventType = "tool_start"
	LifecycleToolComplete        RuntimeLifecycleEventType = "tool_complete"
	LifecycleTaskCreated         RuntimeLifecycleEventType = "task_created"
	LifecycleTaskCompleted       RuntimeLifecycleEventType = "task_completed"
	LifecycleCronFire            RuntimeLifecycleEventType = "cron_fire"
	LifecycleWorktreeEnter       RuntimeLifecycleEventType = "worktree_enter"
	LifecycleWorktreeExit        RuntimeLifecycleEventType = "worktree_exit"
	LifecycleTeamCreate          RuntimeLifecycleEventType = "team_create"
	LifecycleTeamDelete          RuntimeLifecycleEventType = "team_delete"
	LifecycleMCPResourcesChanged RuntimeLifecycleEventType = "mcp_resources_changed"
)

const runtimeLifecycleSchemaVersion = 1

// RuntimeLifecycleEvent is the durable, TS-compatible lifecycle envelope.
// Payload remains intentionally open so dependent tools can add state without
// forcing a schema change in this foundation layer.
type RuntimeLifecycleEvent struct {
	ID        string                    `json:"id"`
	Type      RuntimeLifecycleEventType `json:"type"`
	SessionID string                    `json:"session_id,omitempty"`
	EntityID  string                    `json:"entity_id,omitempty"`
	ToolName  string                    `json:"tool_name,omitempty"`
	Status    string                    `json:"status,omitempty"`
	Payload   map[string]any            `json:"payload,omitempty"`
	CreatedAt time.Time                 `json:"created_at"`
}

type runtimeLifecycleFile struct {
	SchemaVersion int                     `json:"schema_version"`
	Events        []RuntimeLifecycleEvent `json:"events"`
}

// RuntimeLifecycleSink is the single side-effect boundary for notifications,
// mailbox delivery, task refresh, analytics, and other event consumers.
type RuntimeLifecycleSink interface {
	HandleLifecycleEvent(context.Context, RuntimeLifecycleEvent) error
}

// RuntimeLifecycle persists events before dispatching them to side-effect
// sinks. A process crash can therefore reconstruct state from disk without
// relying on an in-memory callback having completed.
type RuntimeLifecycle struct {
	root string
	path string

	mu    sync.RWMutex
	sinks []RuntimeLifecycleSink
}

// Root returns the immutable project root owned by this lifecycle journal.
func (l *RuntimeLifecycle) Root() string {
	if l == nil {
		return ""
	}
	return l.root
}

func NewRuntimeLifecycle(projectRoot string) *RuntimeLifecycle {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
	}
	root = filepath.Clean(root)
	lifecycle := &RuntimeLifecycle{root: root, path: runtimeLifecyclePath(root)}
	_ = secureio.EnsurePrivateRuntimeDirectory(filepath.Dir(lifecycle.path))
	return lifecycle
}

func runtimeLifecyclePath(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
	}
	return filepath.Join(root, ".luban-code", "runtime-state", "lifecycle.json")
}

func (l *RuntimeLifecycle) lockPath() string {
	if l == nil {
		return ""
	}
	return l.path + ".lock"
}

func (l *RuntimeLifecycle) Subscribe(sink RuntimeLifecycleSink) {
	if l == nil || sink == nil {
		return
	}
	l.mu.Lock()
	l.sinks = append(l.sinks, sink)
	l.mu.Unlock()
}

// Publish durably appends event, then fans the same event identity out to all
// subscribers. Sink failures are returned only after the event is persisted.
func (l *RuntimeLifecycle) Publish(ctx context.Context, event RuntimeLifecycleEvent) error {
	if l == nil {
		return nil
	}
	if err := validateLifecycleEvent(event); err != nil {
		return err
	}
	if strings.TrimSpace(event.ID) == "" {
		event.ID = newLifecycleEventID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	} else {
		event.CreatedAt = event.CreatedAt.UTC()
	}
	if event.Payload != nil {
		event.Payload = cloneLifecyclePayload(event.Payload)
	}

	if err := secureio.PreparePrivateRuntimeLock(l.lockPath()); err != nil {
		return err
	}
	alreadyPersisted := false
	if err := secureio.WithRuntimeFileLock(l.lockPath(), func() error {
		state, err := readRuntimeLifecycleFile(l.path)
		if err != nil {
			return err
		}
		for _, existing := range state.Events {
			if existing.ID == event.ID {
				alreadyPersisted = true
				return nil
			}
		}
		state.SchemaVersion = runtimeLifecycleSchemaVersion
		state.Events = append(state.Events, event)
		return writeRuntimeLifecycleFile(l.path, state)
	}); err != nil {
		return err
	}
	if alreadyPersisted {
		return nil
	}

	l.mu.RLock()
	sinks := append([]RuntimeLifecycleSink(nil), l.sinks...)
	l.mu.RUnlock()
	var sinkErrs []error
	for _, sink := range sinks {
		if err := sink.HandleLifecycleEvent(ctx, event); err != nil {
			sinkErrs = append(sinkErrs, err)
		}
	}
	return errors.Join(sinkErrs...)
}

func validateLifecycleEvent(event RuntimeLifecycleEvent) error {
	switch event.Type {
	case LifecycleToolStart,
		LifecycleToolComplete,
		LifecycleTaskCreated,
		LifecycleTaskCompleted,
		LifecycleCronFire,
		LifecycleWorktreeEnter,
		LifecycleWorktreeExit,
		LifecycleTeamCreate,
		LifecycleTeamDelete,
		LifecycleMCPResourcesChanged:
		return nil
	case "":
		return errors.New("lifecycle event type is required")
	default:
		return fmt.Errorf("unknown lifecycle event type %q", event.Type)
	}
}

func (l *RuntimeLifecycle) Events() ([]RuntimeLifecycleEvent, error) {
	if l == nil {
		return nil, nil
	}
	if err := secureio.PreparePrivateRuntimeLock(l.lockPath()); err != nil {
		return nil, err
	}
	value, err := secureio.WithRuntimeFileLockResult(l.lockPath(), func() (any, error) {
		return readRuntimeLifecycleFile(l.path)
	})
	if err != nil {
		return nil, err
	}
	state, ok := value.(runtimeLifecycleFile)
	if !ok {
		return nil, errors.New("invalid runtime lifecycle state")
	}
	return cloneLifecycleEvents(state.Events), nil
}

// ActiveState folds the durable event history into the state that must survive
// session resume and compaction. Instantaneous cron/MCP events are history-only.
func (l *RuntimeLifecycle) ActiveState() ([]RuntimeLifecycleEvent, error) {
	events, err := l.Events()
	if err != nil {
		return nil, err
	}
	active := make(map[string]RuntimeLifecycleEvent)
	for _, event := range events {
		family, begins, ends := lifecycleFamily(event.Type)
		if family == "" || strings.TrimSpace(event.EntityID) == "" {
			continue
		}
		key := family + "\x00" + event.EntityID
		switch {
		case begins:
			active[key] = event
		case ends:
			delete(active, key)
		}
	}
	out := make([]RuntimeLifecycleEvent, 0, len(active))
	for _, event := range active {
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func lifecycleFamily(eventType RuntimeLifecycleEventType) (family string, begins, ends bool) {
	switch eventType {
	case LifecycleToolStart:
		return "tool", true, false
	case LifecycleToolComplete:
		return "tool", false, true
	case LifecycleTaskCreated:
		return "task", true, false
	case LifecycleTaskCompleted:
		return "task", false, true
	case LifecycleWorktreeEnter:
		return "worktree", true, false
	case LifecycleWorktreeExit:
		return "worktree", false, true
	case LifecycleTeamCreate:
		return "team", true, false
	case LifecycleTeamDelete:
		return "team", false, true
	default:
		return "", false, false
	}
}

func readRuntimeLifecycleFile(path string) (runtimeLifecycleFile, error) {
	if err := secureio.EnsurePrivateRuntimeDirectory(filepath.Dir(path)); err != nil {
		return runtimeLifecycleFile{}, err
	}
	data, err := secureio.ReadPrivateRuntimeRegularFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return runtimeLifecycleFile{SchemaVersion: runtimeLifecycleSchemaVersion}, nil
		}
		return runtimeLifecycleFile{}, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return runtimeLifecycleFile{SchemaVersion: runtimeLifecycleSchemaVersion}, nil
	}
	var state runtimeLifecycleFile
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return runtimeLifecycleFile{}, fmt.Errorf("decode lifecycle state: %w", err)
	}
	if state.SchemaVersion != runtimeLifecycleSchemaVersion {
		return runtimeLifecycleFile{}, fmt.Errorf("unsupported lifecycle schema version %d", state.SchemaVersion)
	}
	return state, nil
}

func writeRuntimeLifecycleFile(path string, state runtimeLifecycleFile) error {
	if err := secureio.EnsurePrivateRuntimeDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return secureio.AtomicWritePrivateRuntimeFile(path, append(body, '\n'))
}

func newLifecycleEventID() string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err == nil {
		return fmt.Sprintf("evt-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(random[:]))
	}
	return fmt.Sprintf("evt-%d", time.Now().UnixNano())
}

func cloneLifecycleEvents(events []RuntimeLifecycleEvent) []RuntimeLifecycleEvent {
	out := make([]RuntimeLifecycleEvent, len(events))
	for i, event := range events {
		out[i] = event
		out[i].Payload = cloneLifecyclePayload(event.Payload)
	}
	return out
}

func cloneLifecyclePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}
