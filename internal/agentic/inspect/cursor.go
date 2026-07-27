package inspect

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"
)

const (
	cursorLifetime     = 10 * time.Minute
	maximumCursors     = 32
	maximumCursorBytes = 64 << 20
	opaqueTokenBytes   = 24
)

type cursorStore struct {
	mu      sync.Mutex
	entries map[string]cursorEntry
	serial  atomic.Uint64
}

type cursorEntry struct {
	root                   string
	sessionID              string
	runID                  string
	cacheLineageID         string
	historyEpoch           string
	historyBound           bool
	workspaceRevision      uint64
	workspaceRevisionBound bool
	generation             string
	expiresAt              time.Time
	state                  *paginationState
	weight                 int
}

func newCursorStore() *cursorStore {
	return &cursorStore{entries: make(map[string]cursorEntry)}
}

func (s *cursorStore) generation() string {
	return "g_" + s.opaqueToken()
}

func (s *cursorStore) put(workspace workspaceSnapshot, generation string, state *paginationState) string {
	if s == nil || state == nil {
		return ""
	}
	now := time.Now()
	weight := state.estimatedBytes()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	for len(s.entries) >= maximumCursors || (len(s.entries) > 0 && s.totalWeightLocked()+weight > maximumCursorBytes) {
		var oldestToken string
		var oldest time.Time
		for token, entry := range s.entries {
			if oldestToken == "" || entry.expiresAt.Before(oldest) {
				oldestToken = token
				oldest = entry.expiresAt
			}
		}
		delete(s.entries, oldestToken)
	}
	token := "c_" + s.opaqueToken()
	s.entries[token] = cursorEntry{
		root: workspace.root, sessionID: workspace.sessionID, runID: workspace.runID,
		cacheLineageID: workspace.cacheLineageID, historyEpoch: workspace.historyEpoch,
		historyBound: workspace.historyBound, workspaceRevision: workspace.workspaceRevision,
		workspaceRevisionBound: workspace.workspaceRevisionBound, generation: generation,
		expiresAt: now.Add(cursorLifetime), state: state, weight: weight,
	}
	return token
}

func (s *cursorStore) totalWeightLocked() int {
	total := 0
	for _, entry := range s.entries {
		total += entry.weight
	}
	return total
}

// consume makes cursors one-shot. A successful continuation receives a fresh
// token when more items remain, so replay cannot retain an old workspace view.
func (s *cursorStore) consume(token string, workspace workspaceSnapshot) (cursorEntry, bool) {
	if s == nil || token == "" {
		return cursorEntry{}, false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	entry, ok := s.entries[token]
	if !ok || entry.root != workspace.root || entry.sessionID != workspace.sessionID ||
		entry.runID != workspace.runID || entry.cacheLineageID != workspace.cacheLineageID ||
		entry.historyBound != workspace.historyBound || !entry.historyBound ||
		entry.historyEpoch != workspace.historyEpoch ||
		entry.workspaceRevisionBound != workspace.workspaceRevisionBound ||
		entry.workspaceRevision != workspace.workspaceRevision || entry.state == nil {
		return cursorEntry{}, false
	}
	delete(s.entries, token)
	return entry, true
}

func (s *cursorStore) pruneLocked(now time.Time) {
	for token, entry := range s.entries {
		if !entry.expiresAt.After(now) {
			delete(s.entries, token)
		}
	}
}

func (s *cursorStore) opaqueToken() string {
	buffer := make([]byte, opaqueTokenBytes)
	if _, err := rand.Read(buffer); err == nil {
		return base64.RawURLEncoding.EncodeToString(buffer)
	}
	// crypto/rand failure is not allowed to turn safe output pagination into a
	// tool error. The fallback mixes a process-local monotonic nonce with time;
	// the token remains an opaque in-memory lookup key and carries no authority.
	serial := s.serial.Add(1)
	binary.BigEndian.PutUint64(buffer[:8], serial)
	binary.BigEndian.PutUint64(buffer[8:16], uint64(time.Now().UnixNano()))
	digest := sha256.Sum256(buffer[:16])
	return base64.RawURLEncoding.EncodeToString(digest[:opaqueTokenBytes])
}
