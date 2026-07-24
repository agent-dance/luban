package engine

import "errors"

// Sentinel errors returned by the engine layer.
var (
	// ErrSessionNotFound is returned when a session ID does not exist.
	ErrSessionNotFound = errors.New("engine: session not found")
	// ErrSessionDeleted prevents late background work from resurrecting history.
	ErrSessionDeleted = errors.New("engine: session history deleted")
	// ErrWorkspaceRebindUnauthorized rejects worktree transitions that do not
	// originate from the exact running QueryLoop/conversation binding.
	ErrWorkspaceRebindUnauthorized = errors.New("engine: workspace rebind unauthorized")

	// ErrShutdown is returned when an operation is attempted after Shutdown.
	ErrShutdown = errors.New("engine: already shut down")

	// ErrNoProvider is returned when no provider has been configured.
	ErrNoProvider = errors.New("engine: no provider configured")
)
