// Package tools — edit_uuid.go provides the lightweight UUID helper used by
// FileEditTool, FileWriteTool, and NotebookEditTool to stamp every result
// with an `editId` for cross-session correlation in transcripts and history.
package tools

import "github.com/google/uuid"

// NewEditUUID returns a fresh RFC-4122 v4 UUID, formatted in the canonical
// 8-4-4-4-12 hex layout used by the TS reference. Returns an empty string if
// the system random source fails — callers must tolerate this rather than
// panic.
func NewEditUUID() string {
	id, err := uuid.NewRandom()
	if err != nil {
		return ""
	}
	return id.String()
}
