// Package tools — file_edit_history.go is the Go counterpart of
// src/utils/fileHistory.ts. It records every successful Edit/Write to an
// append-only JSON-Lines file under .claude/file-history/<sha1(path)>.jsonl
// so the runtime UI can offer "undo last change" and the transcript replay
// can reconstruct prior versions.
//
// The store is best-effort: callers ignore errors. Disk-full or permission
// errors during history append must never block the underlying edit.
package tools

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync"
)

// FileHistoryEntry is the JSON record persisted per edit. Field names match
// the TS fileHistory.ts shape so the same files can be consumed by either
// runtime.
type FileHistoryEntry struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
	Hash   string `json:"hash,omitempty"`
	Tool   string `json:"tool"`
	Ts     int64  `json:"ts"`
	// EditID is a UUID correlating this entry with the structured tool
	// result. Empty entries get a UUID assigned automatically by TrackEdit.
	EditID string `json:"editId,omitempty"`
}

// FileHistoryStore appends edit records under a per-session root directory
// (typically <repo>/.claude/file-history). The zero value is unusable —
// callers must construct via NewFileHistoryStore.
type FileHistoryStore struct {
	mu   sync.Mutex
	root string
}

// NewFileHistoryStore returns a store rooted at `root`. The directory is
// created lazily on the first append; callers don't have to mkdir it ahead
// of time.
func NewFileHistoryStore(root string) *FileHistoryStore {
	return &FileHistoryStore{root: root}
}

// SetRoot retargets future history operations to a new workspace. File,
// Edit, and Notebook share one store, so switching the store keeps all three
// tools coherent without replacing their pointers while work may be running.
func (s *FileHistoryStore) SetRoot(root string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.root = root
	s.mu.Unlock()
}

// Disabled reports whether history tracking is a no-op (root unset).
func (s *FileHistoryStore) Disabled() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	disabled := s.root == ""
	s.mu.Unlock()
	return disabled
}

// TrackEdit appends an entry to the per-path history file. The path-keyed
// filename uses sha1(absPath) so cross-platform path differences (drive
// letters, case sensitivity) don't fragment the history.
func (s *FileHistoryStore) TrackEdit(entry FileHistoryEntry) error {
	if s == nil {
		return nil
	}
	if entry.Path == "" {
		return fmt.Errorf("file_history: empty path")
	}
	if entry.Ts == 0 {
		return fmt.Errorf("file_history: zero timestamp")
	}
	if entry.Hash == "" {
		entry.Hash = HashContent(entry.After)
	}
	if entry.EditID == "" {
		entry.EditID = NewEditUUID()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == "" {
		return nil
	}

	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(entry); err != nil {
		return fmt.Errorf("file_history encode: %w", err)
	}
	if err := appendPrivateFileHistory(s.root, historyFileName(entry.Path), encoded.Bytes()); err != nil {
		return fmt.Errorf("file_history append: %w", err)
	}
	return nil
}

// ListEdits returns every recorded entry for `absPath` in chronological order.
// Equal timestamps preserve append order. A missing history file is not an error.
func (s *FileHistoryStore) ListEdits(absPath string) ([]FileHistoryEntry, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	if s.root == "" {
		s.mu.Unlock()
		return nil, nil
	}
	root := s.root
	s.mu.Unlock()
	data, found, err := readPrivateFileHistory(root, historyFileName(absPath))
	if err != nil {
		return nil, fmt.Errorf("file_history read: %w", err)
	}
	if !found || len(data) == 0 {
		return nil, nil
	}

	var entries []FileHistoryEntry
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var e FileHistoryEntry
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			// Skip malformed lines rather than failing the whole listing —
			// best-effort matches TS behaviour.
			break
		}
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Ts < entries[j].Ts
	})
	return entries, nil
}

// historyFileName returns the per-path JSONL filename, using sha1(path) so
// the same file is always grouped regardless of how the path is rendered.
func historyFileName(absPath string) string {
	sum := sha1.Sum([]byte(absPath))
	return hex.EncodeToString(sum[:]) + ".jsonl"
}

// HashContent returns a short content hash used to dedupe identical edits.
// Exposed for tests and external callers.
func HashContent(content string) string {
	sum := sha1.Sum([]byte(content))
	return hex.EncodeToString(sum[:])
}
