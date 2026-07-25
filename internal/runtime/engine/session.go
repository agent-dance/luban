package engine

import "github.com/agent-dance/luban/types"

// SessionInfo holds lightweight metadata about a stored session.
type SessionInfo struct {
	ID        string `json:"id"`
	UpdatedAt int64  `json:"updated_at"` // Unix timestamp
	Messages  int    `json:"messages"`
}

// SessionManager persists and retrieves conversation histories.
type SessionManager interface {
	// Save writes the full message history for a session.
	Save(sessionID string, messages []types.Message) error

	// Load retrieves the message history for a session.
	// Returns ErrSessionNotFound if the session does not exist.
	Load(sessionID string) ([]types.Message, error)

	// List returns metadata for all known sessions, newest first.
	List() ([]SessionInfo, error)

	// Latest returns the ID of the most-recently updated session.
	// Returns ErrSessionNotFound if there are no sessions.
	Latest() (string, error)

	// Delete removes a session and its stored messages.
	Delete(sessionID string) error
}

// SessionMetaSaver is an optional interface that SessionManager implementations
// may provide to support saving provider/model metadata alongside sessions.
type SessionMetaSaver interface {
	// SaveProviderModel persists the provider and model used in a session.
	SaveProviderModel(sessionID, providerName, modelID string) error
}

// SessionArtifactsDirProvider optionally exposes where per-session runtime
// artifacts should be stored.
type SessionArtifactsDirProvider interface {
	ArtifactsDir(sessionID string) string
}

// SessionTranscriptPathProvider optionally exposes the readable transcript path
// for a persisted session.
type SessionTranscriptPathProvider interface {
	TranscriptPath(sessionID string) string
}

// SessionContextSaver optionally persists workspace metadata that should travel
// with a session transcript.
type SessionContextSaver interface {
	SaveSessionContext(sessionID, cwd, gitBranch string) error
}

// SessionToolUseLedgerStore optionally persists and restores tool-use
// identities that outlive model-context compaction.
type SessionToolUseLedgerStore interface {
	SaveToolUseLedger(sessionID string, seenToolUseIDs []string) error
	LoadToolUseLedger(sessionID string) ([]string, error)
}

// SessionLoadedToolStore optionally persists deferred tool schemas that were
// already surfaced before compaction or process restart.
type SessionLoadedToolStore interface {
	SaveLoadedToolNames(sessionID string, loadedToolNames []string) error
	LoadLoadedToolNames(sessionID string) ([]string, error)
}
