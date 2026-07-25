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

	existing, err := readMailboxMessages(inboxFile)
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
	return msg, nil
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

func readMailboxMessages(path string) ([]Message, error) {
	messages, err := readMessages(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(messages))
	for index, message := range messages {
		if _, err := uuid.Parse(message.ID); err != nil {
			return nil, fmt.Errorf("mailbox: invalid message id at index %d: %w", index, err)
		}
		if _, duplicate := seen[message.ID]; duplicate {
			return nil, fmt.Errorf("mailbox: %w: duplicate id %s", ErrMailboxSequenceConflict, message.ID)
		}
		seen[message.ID] = struct{}{}
		if want := uint64(index + 1); message.Sequence != want {
			return nil, fmt.Errorf("mailbox: %w: sequence at index %d is %d, want %d", ErrMailboxSequenceConflict, index, message.Sequence, want)
		}
	}
	return messages, nil
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
