package goal

import (
	"context"
	"errors"
)

// ErrStaleUpdate means the goal changed after a caller captured the state it
// intended to update. Callers should reload instead of overwriting that state.
var ErrStaleUpdate = errors.New("goal update is stale")

// UpdateFunc derives the next persisted state from the latest goal. Current is
// nil when the session has no goal yet. Returning an error leaves storage
// unchanged.
type UpdateFunc func(current *Goal) (Goal, error)

// Updater provides an atomic goal read-modify-write operation.
type Updater interface {
	UpdateGoal(UpdateFunc) (Goal, error)
}

// ContextUpdater provides the same atomic operation for the session identity
// carried by a tool execution context.
type ContextUpdater interface {
	UpdateGoalForContext(context.Context, UpdateFunc) (Goal, error)
}
