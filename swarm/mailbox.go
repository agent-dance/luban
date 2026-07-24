package swarm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/google/uuid"
)

// safeName matches only alphanumeric, underscore, and hyphen characters.
// Rejects path separators, dots-only, and control characters to prevent
// path traversal attacks (Finding #1).
var safeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// validateName checks that a name is safe for use in filesystem paths.
func validateName(name, label string) error {
	if name == "" {
		return fmt.Errorf("%s must not be empty", label)
	}
	if len(name) > 64 {
		return fmt.Errorf("%s too long (max 64 chars)", label)
	}
	if !safeName.MatchString(name) {
		return fmt.Errorf("%s %q contains invalid characters (only alphanumeric, underscore, hyphen allowed)", label, name)
	}
	return nil
}

// Message is a unit of communication between leader and teammates.
type Message struct {
	ID        string `json:"id,omitempty"`
	Sequence  uint64 `json:"sequence,omitempty"`
	From      string `json:"from"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
	Read      bool   `json:"read"`
	Color     string `json:"color,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

// AnyMailboxSequence disables the compare-and-swap revision check in SendCAS.
// Callers that retry an uncertain write should still reuse the same Message.ID.
const AnyMailboxSequence uint64 = ^uint64(0)

// ErrMailboxSequenceConflict reports a stale compare-and-swap revision or a
// reused message UUID whose immutable payload does not match the stored one.
var ErrMailboxSequenceConflict = errors.New("mailbox sequence conflict")

// NewMessageID returns the stable UUID carried by one logical mailbox message.
// Generate it once, before a retry loop, and reuse it for every attempt.
func NewMessageID() string { return uuid.NewString() }

// Mailbox provides file-based message passing between leader and teammates.
// Inbox path: ~/.luban-code/teams/{teamName}/inboxes/{agentName}.json
type Mailbox struct {
	baseDir string // ~/.luban-code/teams/{teamName}
}

// NewMailbox creates a Mailbox rooted at the LUBAN Code team directory.
// Returns an error if teamName contains unsafe characters.
func NewMailbox(teamName string) (*Mailbox, error) {
	if err := validateName(teamName, "team name"); err != nil {
		return nil, err
	}
	home, err := userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("mailbox: cannot determine home directory: %w", err)
	}
	return &Mailbox{
		baseDir: filepath.Join(home, brand.ConfigDirName, "teams", teamName),
	}, nil
}

// inboxPath returns the inbox file path for the given agent.
func (m *Mailbox) inboxPath(agentName string) (string, error) {
	if err := validateName(agentName, "agent name"); err != nil {
		return "", err
	}
	p := filepath.Join(m.baseDir, "inboxes", agentName+".json")
	// Defence-in-depth: verify the path is under baseDir.
	if rel, err := filepath.Rel(m.baseDir, p); err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("inbox path escapes base directory")
	}
	return p, nil
}

// legacyInboxPath identifies the pre-alignment Go layout. Operations migrate
// messages from it into the TS-compatible inbox without dropping history.
func (m *Mailbox) legacyInboxPath(agentName string) (string, error) {
	if err := validateName(agentName, "agent name"); err != nil {
		return "", err
	}
	p := filepath.Join(m.baseDir, agentName, "inbox.json")
	if rel, err := filepath.Rel(m.baseDir, p); err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("legacy inbox path escapes base directory")
	}
	return p, nil
}

// Send writes a message to an agent's inbox.
// It acquires a file lock, reads existing messages, appends the new one,
// and atomically replaces the inbox file.
// ctx is forwarded to lockFile so the caller can cancel a blocked lock acquire.
func (m *Mailbox) Send(ctx context.Context, agentName string, msg Message) error {
	_, err := m.SendCAS(ctx, agentName, msg, AnyMailboxSequence)
	return err
}

// SendCAS appends msg when expectedSequence matches the current mailbox high
// water mark. Reusing a UUID with the same immutable payload is an idempotent
// success even when expectedSequence is stale; reusing it for different data
// is rejected. The returned Message contains the durable UUID and sequence.
func (m *Mailbox) SendCAS(ctx context.Context, agentName string, msg Message, expectedSequence uint64) (Message, error) {
	inboxFile, err := m.inboxPath(agentName)
	if err != nil {
		return Message{}, fmt.Errorf("mailbox send: %w", err)
	}

	// Ensure directory exists (0700 — owner-only).
	if err := os.MkdirAll(filepath.Dir(inboxFile), 0o700); err != nil {
		return Message{}, fmt.Errorf("mailbox send: mkdir: %w", err)
	}

	// Fill timestamp if empty.
	timestampWasEmpty := strings.TrimSpace(msg.Timestamp) == ""
	if msg.Timestamp == "" {
		msg.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(msg.ID) == "" {
		msg.ID = NewMessageID()
	}
	if _, err := uuid.Parse(msg.ID); err != nil {
		return Message{}, fmt.Errorf("mailbox send: invalid message id: %w", err)
	}
	msg.Read = false
	msg.Sequence = 0

	// Acquire file lock to prevent TOCTOU race (Finding #2).
	unlock, err := lockFile(ctx, inboxFile+".lock")
	if err != nil {
		return Message{}, fmt.Errorf("mailbox send: lock: %w", err)
	}
	defer unlock()

	existing, migration, err := m.readCompatibleMessages(agentName, inboxFile)
	if err != nil {
		return Message{}, fmt.Errorf("mailbox send: read existing: %w", err)
	}
	for _, stored := range existing {
		if stored.ID != msg.ID {
			continue
		}
		if sameMessagePayload(stored, msg, timestampWasEmpty) {
			return stored, nil
		}
		return Message{}, fmt.Errorf("mailbox send: %w: message id %s payload mismatch", ErrMailboxSequenceConflict, msg.ID)
	}
	currentSequence := mailboxHighWater(existing)
	if expectedSequence != AnyMailboxSequence && expectedSequence != currentSequence {
		return Message{}, fmt.Errorf("mailbox send: %w: expected %d, current %d", ErrMailboxSequenceConflict, expectedSequence, currentSequence)
	}
	msg.Sequence = currentSequence + 1

	messages := append(existing, msg)

	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return Message{}, fmt.Errorf("mailbox send: marshal: %w", err)
	}

	if err := atomicWrite(inboxFile, data); err != nil {
		return Message{}, err
	}
	// Keep the legacy file and its stable lock inode during mixed-version
	// operation. Deterministic IDs make every subsequent merge idempotent.
	_ = migration
	return msg, nil
}

// Read returns all messages in order without consuming them. Callers that
// display messages can use MarkAllRead; polling marks one unread message at a
// time. This preserves the TS mailbox audit history.
// If the inbox is empty or does not exist, returns an empty slice.
// ctx is forwarded to lockFile so the caller can cancel a blocked lock acquire.
func (m *Mailbox) Read(ctx context.Context, agentName string) ([]Message, error) {
	inboxFile, err := m.inboxPath(agentName)
	if err != nil {
		return nil, fmt.Errorf("mailbox read: %w", err)
	}

	// Acquire file lock.
	unlock, err := lockFile(ctx, inboxFile+".lock")
	if err != nil {
		return nil, fmt.Errorf("mailbox read: lock: %w", err)
	}
	defer unlock()

	messages, migration, err := m.readCompatibleMessages(agentName, inboxFile)
	if err != nil {
		return nil, fmt.Errorf("mailbox read: %w", err)
	}
	if migration {
		if err := writeMessages(inboxFile, messages); err != nil {
			return nil, fmt.Errorf("mailbox read: migrate legacy inbox: %w", err)
		}
	}
	return messages, nil
}

// ReadUnread returns the unread subset without mutating mailbox state.
func (m *Mailbox) ReadUnread(ctx context.Context, agentName string) ([]Message, error) {
	messages, err := m.Read(ctx, agentName)
	if err != nil {
		return nil, err
	}
	unread := make([]Message, 0, len(messages))
	for _, message := range messages {
		if !message.Read {
			unread = append(unread, message)
		}
	}
	return unread, nil
}

// MessageReceipt identifies the exact durable append a consumer processed.
// Both fields are required so an acknowledgement cannot accidentally apply to
// a migrated/replaced record that merely occupies the same array position.
type MessageReceipt struct {
	ID       string
	Sequence uint64
}

// Acknowledge marks only the named UUID/sequence pairs as read. It is
// idempotent and leaves messages appended after the consumer snapshot unread.
func (m *Mailbox) Acknowledge(ctx context.Context, agentName string, receipts []MessageReceipt) error {
	if len(receipts) == 0 {
		return nil
	}
	inboxFile, err := m.inboxPath(agentName)
	if err != nil {
		return fmt.Errorf("mailbox acknowledge: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(inboxFile), 0o700); err != nil {
		return fmt.Errorf("mailbox acknowledge: mkdir: %w", err)
	}
	unlock, err := lockFile(ctx, inboxFile+".lock")
	if err != nil {
		return fmt.Errorf("mailbox acknowledge: lock: %w", err)
	}
	defer unlock()
	messages, _, err := m.readCompatibleMessages(agentName, inboxFile)
	if err != nil {
		return fmt.Errorf("mailbox acknowledge: read: %w", err)
	}
	wanted := make(map[string]uint64, len(receipts))
	for _, receipt := range receipts {
		if _, err := uuid.Parse(receipt.ID); err != nil || receipt.Sequence == 0 {
			return fmt.Errorf("mailbox acknowledge: %w: invalid receipt", ErrMailboxSequenceConflict)
		}
		if prior, duplicate := wanted[receipt.ID]; duplicate && prior != receipt.Sequence {
			return fmt.Errorf("mailbox acknowledge: %w: conflicting receipt %s", ErrMailboxSequenceConflict, receipt.ID)
		}
		wanted[receipt.ID] = receipt.Sequence
	}
	for index := range messages {
		expected, ok := wanted[messages[index].ID]
		if !ok {
			continue
		}
		if messages[index].Sequence != expected {
			return fmt.Errorf("mailbox acknowledge: %w: id %s expected sequence %d, current %d", ErrMailboxSequenceConflict, messages[index].ID, expected, messages[index].Sequence)
		}
		messages[index].Read = true
		delete(wanted, messages[index].ID)
	}
	if len(wanted) != 0 {
		return fmt.Errorf("mailbox acknowledge: %w: receipt is not present", ErrMailboxSequenceConflict)
	}
	if err := writeMessages(inboxFile, messages); err != nil {
		return fmt.Errorf("mailbox acknowledge: write: %w", err)
	}
	return nil
}

// MarkAllRead marks every persisted message as read while preserving it.
func (m *Mailbox) MarkAllRead(ctx context.Context, agentName string) error {
	inboxFile, err := m.inboxPath(agentName)
	if err != nil {
		return fmt.Errorf("mailbox mark read: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(inboxFile), 0o700); err != nil {
		return fmt.Errorf("mailbox mark read: mkdir: %w", err)
	}
	unlock, err := lockFile(ctx, inboxFile+".lock")
	if err != nil {
		return fmt.Errorf("mailbox mark read: lock: %w", err)
	}
	defer unlock()
	messages, migration, err := m.readCompatibleMessages(agentName, inboxFile)
	if err != nil {
		return fmt.Errorf("mailbox mark read: %w", err)
	}
	if len(messages) == 0 {
		return nil
	}
	for i := range messages {
		messages[i].Read = true
	}
	if err := writeMessages(inboxFile, messages); err != nil {
		return fmt.Errorf("mailbox mark read: write: %w", err)
	}
	_ = migration
	return nil
}

// PopFirst atomically returns and marks the first unread message. Read messages
// remain in the inbox for audit/history parity with the TS mailbox.
// ctx is forwarded to lockFile so the caller can cancel a blocked lock acquire.
func (m *Mailbox) PopFirst(ctx context.Context, agentName string) (Message, bool, error) {
	inboxFile, err := m.inboxPath(agentName)
	if err != nil {
		return Message{}, false, fmt.Errorf("mailbox pop: %w", err)
	}

	// Acquire file lock — entire read-modify-write is atomic.
	unlock, err := lockFile(ctx, inboxFile+".lock")
	if err != nil {
		return Message{}, false, fmt.Errorf("mailbox pop: lock: %w", err)
	}
	defer unlock()

	messages, migration, err := m.readCompatibleMessages(agentName, inboxFile)
	if err != nil {
		return Message{}, false, fmt.Errorf("mailbox pop: %w", err)
	}
	for i := range messages {
		if messages[i].Read {
			continue
		}
		first := messages[i]
		messages[i].Read = true
		if err := writeMessages(inboxFile, messages); err != nil {
			return Message{}, false, fmt.Errorf("mailbox pop: mark read: %w", err)
		}
		_ = migration
		return first, true, nil
	}
	if migration && len(messages) > 0 {
		if err := writeMessages(inboxFile, messages); err != nil {
			return Message{}, false, fmt.Errorf("mailbox pop: migrate legacy inbox: %w", err)
		}
	}
	return Message{}, false, nil
}

// Poll watches an agent's inbox for new messages (blocking with context).
// It polls every 500 ms and returns the first message found using atomic
// PopFirst semantics — no re-queue needed.
func (m *Mailbox) Poll(ctx context.Context, agentName string) (Message, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return Message{}, ctx.Err()
		case <-ticker.C:
			msg, ok, err := m.PopFirst(ctx, agentName)
			if err != nil {
				return Message{}, fmt.Errorf("mailbox poll: %w", err)
			}
			if ok {
				return msg, nil
			}
		}
	}
}

// readMessages deserializes a JSON inbox file.
func readMessages(path string) ([]Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var messages []Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("decode inbox %s: %w", path, err)
	}
	return messages, nil
}

func (m *Mailbox) readCompatibleMessages(agentName, inboxFile string) ([]Message, bool, error) {
	canonical, err := readMessages(inboxFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, false, err
	}
	if os.IsNotExist(err) {
		canonical = nil
	}
	canonical, canonicalChanged, err := normalizeMailboxMessages(canonical)
	if err != nil {
		return nil, false, err
	}
	legacyFile, err := m.legacyInboxPath(agentName)
	if err != nil {
		return nil, false, err
	}
	legacy, legacyErr := readMessages(legacyFile)
	if legacyErr != nil && !os.IsNotExist(legacyErr) {
		return nil, false, legacyErr
	}
	if os.IsNotExist(legacyErr) || len(legacy) == 0 {
		return canonical, canonicalChanged, nil
	}
	legacy, _, err = normalizeMailboxMessages(legacy)
	if err != nil {
		return nil, false, err
	}
	combined := make([]Message, 0, len(legacy)+len(canonical))
	if len(canonical) == 0 {
		combined = append(combined, legacy...)
	} else {
		// Once the canonical inbox has published sequences, it is authoritative.
		// Append newly discovered legacy IDs after its high-water mark so a mixed
		// old writer cannot renumber already acknowledged canonical messages.
		combined = append(combined, canonical...)
		combined = append(combined, legacy...)
	}
	combined, _, err = normalizeMailboxMessages(combined)
	if err != nil {
		return nil, false, err
	}
	return combined, true, nil
}

func normalizeMailboxMessages(messages []Message) ([]Message, bool, error) {
	if len(messages) == 0 {
		return messages, false, nil
	}
	changed := false
	occurrences := make(map[string]int)
	for i := range messages {
		fingerprint := legacyMessageFingerprint(messages[i])
		ordinal := occurrences[fingerprint]
		occurrences[fingerprint] = ordinal + 1
		if _, err := uuid.Parse(messages[i].ID); err != nil {
			messages[i].ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("mailbox:v1:%s:%d", fingerprint, ordinal))).String()
			changed = true
		}
	}

	deduped := make([]Message, 0, len(messages))
	byID := make(map[string]int, len(messages))
	for _, message := range messages {
		if index, ok := byID[message.ID]; ok {
			stored := &deduped[index]
			if !sameMessagePayload(*stored, message, false) {
				return nil, false, fmt.Errorf("mailbox: %w: duplicate id %s has conflicting payload", ErrMailboxSequenceConflict, message.ID)
			}
			stored.Read = stored.Read || message.Read
			changed = true
			continue
		}
		byID[message.ID] = len(deduped)
		deduped = append(deduped, message)
	}
	for i := range deduped {
		want := uint64(i + 1)
		if deduped[i].Sequence != want {
			deduped[i].Sequence = want
			changed = true
		}
	}
	return deduped, changed, nil
}

func legacyMessageFingerprint(message Message) string {
	payload := struct {
		From, Text, Timestamp, Color, Summary string
	}{message.From, message.Text, message.Timestamp, message.Color, message.Summary}
	data, _ := json.Marshal(payload)
	return string(data)
}

func sameMessagePayload(stored, candidate Message, ignoreCandidateTimestamp bool) bool {
	return stored.From == candidate.From && stored.Text == candidate.Text &&
		(ignoreCandidateTimestamp || stored.Timestamp == candidate.Timestamp) &&
		stored.Color == candidate.Color && stored.Summary == candidate.Summary
}

func mailboxHighWater(messages []Message) uint64 {
	var high uint64
	for _, message := range messages {
		if message.Sequence > high {
			high = message.Sequence
		}
	}
	return high
}

func writeMessages(path string, messages []Message) error {
	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

// atomicWrite writes data to path via a temp file + rename for atomicity.
// Files are created with 0600 permissions (owner-only).
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".inbox-*.tmp")
	if err != nil {
		return fmt.Errorf("atomic write: create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("atomic write: write: %w", err)
	}
	// Ensure owner-only permissions (Finding #4).
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("atomic write: chmod: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("atomic write: close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("atomic write: rename: %w", err)
	}
	return nil
}
