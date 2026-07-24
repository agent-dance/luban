package tools

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// StructuredProtocolKinds enumerates the inbound mailbox JSON envelope types
// that must be routed to dedicated handlers instead of being injected into the
// LLM prompt as raw user content. Mirrors the TS isStructuredProtocolMessage
// classifier (src/utils/teammateMailbox.ts:1019-1095).
//
// SM-08: Without inbound classification, a permission_request envelope arriving
// in a worker's mailbox would be appended to its next user prompt as JSON,
// causing the model to hallucinate a textual response and breaking the
// permission round-trip protocol entirely. Centralising the prefix list here
// means every mailbox consumer can call IsStructuredProtocolMessage before it
// decides whether to feed bytes into the prompt blob.
var StructuredProtocolKinds = []string{
	"permission_request",
	"permission_response",
	"permission_decision",
	"permission_revoke",
	"sandbox_permission_request",
	"sandbox_permission_response",
	"sandbox_permission_decision",
	"shutdown_request",
	"shutdown_response",
	"shutdown_approved",
	"shutdown_rejected",
	"shutdown_ack",
	"plan_approval_request",
	"plan_approval_response",
	"mode_set_request",
	"mode_set_response",
	"team_permission_update",
	"ask_user_question",
}

// IsStructuredProtocolMessage reports whether the given mailbox text is a
// structured protocol envelope. Non-JSON or unrecognised payloads return
// (kind:"", false, nil) so callers can treat them as plain LLM content.
//
// kind is the canonical "type" field for the envelope (already lower-cased and
// trimmed) and is empty for plain text. err is non-nil only when the input
// was syntactically JSON-shaped but failed to decode — that should be surfaced
// to the caller so a malformed envelope is not silently swallowed.
func IsStructuredProtocolMessage(text string) (kind string, ok bool, err error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false, nil
	}
	// Cheap rejection: must be a JSON object.
	if trimmed[0] != '{' {
		return "", false, nil
	}
	var raw map[string]any
	if jerr := json.Unmarshal([]byte(trimmed), &raw); jerr != nil {
		// Looks like JSON but is malformed; surface the error so the caller
		// can decide whether to drop, log, or propagate.
		return "", false, jerr
	}
	rawKind, _ := raw["type"].(string)
	rawKind = strings.ToLower(strings.TrimSpace(rawKind))
	if rawKind == "" {
		return "", false, nil
	}
	for _, candidate := range StructuredProtocolKinds {
		if rawKind == candidate {
			return rawKind, true, nil
		}
	}
	return "", false, nil
}

// Envelope is retained for Go-internal approval handshakes. Public
// SendMessage deliberately does not expose this coordinator transport; its
// current TS contract is SendMessageResult in send_message_contract.go.
type Envelope struct {
	RequestID  string   `json:"request_id"`
	Kind       string   `json:"kind"`
	ReplyTo    string   `json:"reply_to"`
	Delivered  bool     `json:"delivered"`
	Recipients []string `json:"recipients"`
	ReplyText  string   `json:"reply_text"`
	Quorum     int      `json:"quorum"`
	Priority   string   `json:"priority,omitempty"`
	Message    string   `json:"message,omitempty"`
	Success    *bool    `json:"success,omitempty"`
}

func envelopeSuccess(value bool) *bool {
	return &value
}

// NewRequestID returns a unique request ID prefixed with the given kind.
// IDs use crypto/rand so two concurrent calls never collide. Falls back to a
// nanosecond-stamp suffix only when crypto/rand is unavailable (extremely rare
// on supported platforms).
func NewRequestID(prefix string) string {
	if prefix == "" {
		prefix = "req"
	}
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

// inflightEntry stores the bookkeeping the InflightTable maintains per send.
type inflightEntry struct {
	Kind       string
	Recipients []string
	ReplyTo    string
	IssuedAt   time.Time
}

// InflightTable tracks request IDs the SendMessage path has emitted so the
// Coordinator can correlate eventual replies. It is goroutine-safe.
type InflightTable struct {
	mu      sync.Mutex
	entries map[string]inflightEntry
}

// NewInflightTable returns an empty InflightTable.
func NewInflightTable() *InflightTable {
	return &InflightTable{entries: make(map[string]inflightEntry)}
}

// Register records a request ID with its associated routing metadata.
func (t *InflightTable) Register(requestID, kind, replyTo string, recipients []string) {
	if t == nil || requestID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.entries == nil {
		t.entries = make(map[string]inflightEntry)
	}
	t.entries[requestID] = inflightEntry{
		Kind:       kind,
		Recipients: append([]string(nil), recipients...),
		ReplyTo:    replyTo,
		IssuedAt:   time.Now().UTC(),
	}
}

// Has reports whether a request ID is currently tracked.
func (t *InflightTable) Has(requestID string) bool {
	if t == nil || requestID == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.entries[requestID]
	return ok
}

// Resolve removes the entry for the given request ID and returns its metadata.
func (t *InflightTable) Resolve(requestID string) (inflightEntry, bool) {
	if t == nil || requestID == "" {
		return inflightEntry{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.entries[requestID]
	if ok {
		delete(t.entries, requestID)
	}
	return entry, ok
}

// defaultInflightTable is the package-level table used by SendMessage,
// EnterPlanMode, ExitPlanMode and AskUserQuestion when no caller-supplied
// table is available. It exists so cross-tool replies can correlate without
// every call site needing to thread a table through.
var defaultInflightTable = NewInflightTable()

// DefaultInflightTable returns the package-level InflightTable used by tools.
func DefaultInflightTable() *InflightTable { return defaultInflightTable }
