package compact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/agent-dance/luban/types"
)

// SessionMemoryCompactConfig mirrors the TS session-memory compact thresholds.
type SessionMemoryCompactConfig struct {
	MinTokens            int
	MinTextBlockMessages int
	MaxTokens            int
}

var DefaultSessionMemoryCompactConfig = SessionMemoryCompactConfig{
	MinTokens:            10_000,
	MinTextBlockMessages: 5,
	MaxTokens:            40_000,
}

var (
	sessionMemoryConfigMu sync.RWMutex
	sessionMemoryConfig   = DefaultSessionMemoryCompactConfig
)

// SetSessionMemoryCompactConfig updates the process-local session-memory
// compact thresholds. Non-positive values are ignored to preserve defaults.
func SetSessionMemoryCompactConfig(cfg SessionMemoryCompactConfig) {
	sessionMemoryConfigMu.Lock()
	defer sessionMemoryConfigMu.Unlock()
	if cfg.MinTokens > 0 {
		sessionMemoryConfig.MinTokens = cfg.MinTokens
	}
	if cfg.MinTextBlockMessages > 0 {
		sessionMemoryConfig.MinTextBlockMessages = cfg.MinTextBlockMessages
	}
	if cfg.MaxTokens > 0 {
		sessionMemoryConfig.MaxTokens = cfg.MaxTokens
	}
}

func GetSessionMemoryCompactConfig() SessionMemoryCompactConfig {
	sessionMemoryConfigMu.RLock()
	defer sessionMemoryConfigMu.RUnlock()
	return sessionMemoryConfig
}

func ResetSessionMemoryCompactConfig() {
	sessionMemoryConfigMu.Lock()
	defer sessionMemoryConfigMu.Unlock()
	sessionMemoryConfig = DefaultSessionMemoryCompactConfig
}

const sessionMemoryMessageAnchorVersion = 1

// ErrSessionMemoryAnchorInvalid marks a session-memory snapshot whose summary
// boundary cannot be proven against the exact current history.
var ErrSessionMemoryAnchorInvalid = errors.New("compact.session_memory.anchor_invalid")

// SessionMemoryMessageAnchor is a durable, content-addressed reference to the
// logical message through which a session-memory summary was extracted.
//
// LogicalOrdinal is the zero-based logical-message position in the exact
// history observed by the extractor. Contiguous assistant fragments that
// share a non-empty message ID form one logical message. ContentDigest binds
// every persisted field of every fragment in that group. All fields are
// required during restore; MessageID alone is never authorization to compact.
type SessionMemoryMessageAnchor struct {
	Version        uint8      `json:"Version"`
	MessageID      string     `json:"MessageID"`
	Role           types.Role `json:"Role"`
	LogicalOrdinal uint64     `json:"LogicalOrdinal"`
	FragmentCount  uint64     `json:"FragmentCount"`
	ContentDigest  string     `json:"ContentDigest"`
}

// SessionMemorySnapshot is the pre-extracted session memory state used by
// compaction. Go currently has no upstream extractor, so the default provider
// returns Available=false instead of fabricating memory content.
//
// LastSummarizedMessageID is retained only to decode legacy snapshots. An
// ID-only snapshot cannot prove which content was summarized and therefore
// fails closed. A fresh producer must call
// WithCapturedLastSummarizedMessageAnchor while it owns the authoritative
// extraction history. A producer migrating a legacy record may call
// WithLastSummarizedMessageAnchor only while it still owns that original
// history and the legacy ID is non-empty. Restore never synthesizes a digest
// from potentially stale history.
type SessionMemorySnapshot struct {
	Available                   bool                        `json:"Available"`
	Content                     string                      `json:"Content"`
	LastSummarizedMessageID     string                      `json:"LastSummarizedMessageID,omitempty"`
	LastSummarizedMessageAnchor *SessionMemoryMessageAnchor `json:"LastSummarizedMessageAnchor,omitempty"`
}

// MarshalJSON prevents creation of a new available summary record that only
// carries the legacy ID. Legacy JSON remains decodable so callers can reset or
// explicitly migrate it at extraction time, but it cannot be re-persisted as
// if it were a strong record.
func (snapshot SessionMemorySnapshot) MarshalJSON() ([]byte, error) {
	if !validSessionMemorySnapshotAnchorShape(snapshot) {
		return nil, ErrSessionMemoryAnchorInvalid
	}
	type snapshotJSON SessionMemorySnapshot
	return json.Marshal(snapshotJSON(snapshot))
}

// UnmarshalJSON always replaces the complete snapshot. In particular, a
// legacy record that omits LastSummarizedMessageAnchor must clear any strong
// anchor left in a reused destination value; otherwise ID reuse could combine
// fields from two different durable records.
func (snapshot *SessionMemorySnapshot) UnmarshalJSON(data []byte) error {
	type snapshotJSON SessionMemorySnapshot
	var decoded snapshotJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*snapshot = SessionMemorySnapshot(decoded)
	return nil
}

type sessionMemoryLogicalMessage struct {
	start   int
	end     int
	ordinal uint64
}

// CaptureSessionMemoryMessageAnchor creates the durable reference that a
// session-memory producer must persist alongside its summary. If index points
// into a contiguous assistant fragment group, the returned anchor covers the
// complete group and records its logical start. Runtime control messages and
// content that cannot be serialized are not eligible anchors.
func CaptureSessionMemoryMessageAnchor(messages []types.Message, index int) (SessionMemoryMessageAnchor, bool) {
	groups := sessionMemoryLogicalMessages(messages)
	for _, group := range groups {
		if index < group.start || index >= group.end {
			continue
		}
		if sessionMemoryGroupContainsInternalMessage(messages[group.start:group.end]) {
			return SessionMemoryMessageAnchor{}, false
		}
		if messageID := messages[group.start].ID; messageID != "" && sessionMemoryMessageIDGroupCount(messages, groups, messageID) != 1 {
			return SessionMemoryMessageAnchor{}, false
		}
		digest, ok := sessionMemoryLogicalMessageDigest(messages[group.start:group.end])
		if !ok {
			return SessionMemoryMessageAnchor{}, false
		}
		return SessionMemoryMessageAnchor{
			Version:        sessionMemoryMessageAnchorVersion,
			MessageID:      messages[group.start].ID,
			Role:           messages[group.start].Role,
			LogicalOrdinal: group.ordinal,
			FragmentCount:  uint64(group.end - group.start),
			ContentDigest:  digest,
		}, true
	}
	return SessionMemoryMessageAnchor{}, false
}

// WithCapturedLastSummarizedMessageAnchor binds a newly extracted snapshot to
// the exact authoritative history observed by its producer. It is not a
// legacy migration or update API: snapshots that carry a legacy ID or an
// existing strong anchor are returned unchanged. The producer remains
// responsible for ensuring Content was extracted through the supplied index.
func (snapshot SessionMemorySnapshot) WithCapturedLastSummarizedMessageAnchor(messages []types.Message, index int) (SessionMemorySnapshot, bool) {
	if snapshot.LastSummarizedMessageID != "" || snapshot.LastSummarizedMessageAnchor != nil {
		return snapshot, false
	}
	anchor, ok := CaptureSessionMemoryMessageAnchor(messages, index)
	if !ok {
		return snapshot, false
	}
	snapshot.LastSummarizedMessageAnchor = &anchor
	return snapshot, true
}

// WithLastSummarizedMessageAnchor upgrades an in-memory legacy ID-only
// snapshot while the producer still owns the exact history used to create its
// summary. Empty legacy IDs cannot identify an authoritative source message.
// Loaded legacy records must otherwise remain fail-closed rather than being
// rebound from potentially stale current history.
func (snapshot SessionMemorySnapshot) WithLastSummarizedMessageAnchor(messages []types.Message, index int) (SessionMemorySnapshot, bool) {
	legacyID := snapshot.LastSummarizedMessageID
	if legacyID == "" || snapshot.LastSummarizedMessageAnchor != nil {
		return snapshot, false
	}
	anchor, ok := CaptureSessionMemoryMessageAnchor(messages, index)
	if !ok || anchor.MessageID == "" || anchor.MessageID != legacyID {
		return snapshot, false
	}
	snapshot.LastSummarizedMessageAnchor = &anchor
	snapshot.LastSummarizedMessageID = ""
	return snapshot, true
}

type SessionMemoryProvider interface {
	SessionMemorySnapshot(ctx context.Context) (SessionMemorySnapshot, error)
}

type SessionMemoryResetter interface {
	ResetLastSummarizedMessage(ctx context.Context) error
}

type unavailableSessionMemoryProvider struct{}

func (unavailableSessionMemoryProvider) SessionMemorySnapshot(context.Context) (SessionMemorySnapshot, error) {
	return SessionMemorySnapshot{Available: false}, nil
}

func (unavailableSessionMemoryProvider) ResetLastSummarizedMessage(context.Context) error {
	return nil
}

var defaultSessionMemoryProvider SessionMemoryProvider = unavailableSessionMemoryProvider{}

// TODO(task_14): Replace the unavailable default provider with the upstream Go
// session-memory extractor if/when that subsystem exists. Until then this is an
// explicit no-op capability gate, matching the task requirement not to fake
// session memory content.

type SessionMemoryCompactionOptions struct {
	Provider             SessionMemoryProvider
	Config               *SessionMemoryCompactConfig
	AutoCompactThreshold int
	Trigger              string
}

// TrySessionMemoryCompaction attempts the TS session-memory-first compact path.
// The returned bool is true only when Result contains a usable compaction.
func TrySessionMemoryCompaction(ctx context.Context, messages []types.Message, opts SessionMemoryCompactionOptions) (*CompactionResult, bool, error) {
	if !ShouldUseSessionMemoryCompaction() {
		return nil, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	provider := opts.Provider
	if provider == nil {
		provider = defaultSessionMemoryProvider
	}
	snapshot, err := provider.SessionMemorySnapshot(ctx)
	if err != nil {
		return nil, false, err
	}
	if !snapshot.Available || strings.TrimSpace(snapshot.Content) == "" {
		return nil, false, nil
	}

	lastSummarizedIndex, ok := resolveSessionMemoryMessageAnchor(messages, snapshot)
	if !ok {
		// An available summary with an unproven boundary must stop the whole
		// compaction attempt. Returning an ordinary no-op here would let the
		// automatic caller fall through to a different lossy compactor.
		return nil, false, ErrSessionMemoryAnchorInvalid
	}

	cfg := GetSessionMemoryCompactConfig()
	if opts.Config != nil {
		cfg = normalizedSessionMemoryCompactConfig(*opts.Config)
	}
	start := CalculateSessionMemoryMessagesToKeepIndex(messages, lastSummarizedIndex, cfg)
	messagesToKeep := filterCompactBoundaryMessages(messages[start:])

	trigger := opts.Trigger
	if trigger == "" {
		trigger = "auto"
	}
	result := createCompactionResultFromSessionMemory(messages, snapshot.Content, start, messagesToKeep, trigger)
	result.PostCompactTokenCount = NewContextWindow(0).EstimateMessages(BuildPostCompactMessages(result))
	result.TruePostCompactTokenCount = result.PostCompactTokenCount

	if opts.AutoCompactThreshold > 0 && result.PostCompactTokenCount >= opts.AutoCompactThreshold {
		return nil, false, nil
	}
	return result, true, nil
}

func resolveSessionMemoryMessageAnchor(messages []types.Message, snapshot SessionMemorySnapshot) (int, bool) {
	anchor := snapshot.LastSummarizedMessageAnchor
	if anchor == nil || !validSessionMemorySnapshotAnchorShape(snapshot) {
		return 0, false
	}

	groups := sessionMemoryLogicalMessages(messages)
	if anchor.MessageID != "" && sessionMemoryMessageIDGroupCount(messages, groups, anchor.MessageID) != 1 {
		return 0, false
	}

	matchedStart := -1
	matches := 0
	for _, group := range groups {
		if group.ordinal != anchor.LogicalOrdinal {
			continue
		}
		fragments := messages[group.start:group.end]
		if uint64(len(fragments)) != anchor.FragmentCount || len(fragments) == 0 ||
			fragments[0].ID != anchor.MessageID || fragments[0].Role != anchor.Role ||
			sessionMemoryGroupContainsInternalMessage(fragments) {
			continue
		}
		digest, ok := sessionMemoryLogicalMessageDigest(fragments)
		if !ok || digest != anchor.ContentDigest {
			continue
		}
		matches++
		matchedStart = group.start
	}
	if matches != 1 {
		return 0, false
	}
	return matchedStart, true
}

func sessionMemoryMessageIDGroupCount(messages []types.Message, groups []sessionMemoryLogicalMessage, messageID string) int {
	count := 0
	for _, group := range groups {
		if messages[group.start].ID == messageID {
			count++
		}
	}
	return count
}

func validSessionMemorySnapshotAnchorShape(snapshot SessionMemorySnapshot) bool {
	anchor := snapshot.LastSummarizedMessageAnchor
	if anchor == nil {
		return !snapshot.Available || strings.TrimSpace(snapshot.Content) == ""
	}
	if !validSessionMemoryMessageAnchor(*anchor) {
		return false
	}
	// A transitional producer may carry the old ID field together with a strong
	// anchor; disagreement is corruption. ID-only records never reach here as
	// valid available summaries.
	return snapshot.LastSummarizedMessageID == "" || snapshot.LastSummarizedMessageID == anchor.MessageID
}

func validSessionMemoryMessageAnchor(anchor SessionMemoryMessageAnchor) bool {
	if anchor.Version != sessionMemoryMessageAnchorVersion || anchor.FragmentCount == 0 {
		return false
	}
	switch anchor.Role {
	case types.RoleUser, types.RoleAssistant, types.RoleDeveloper:
	default:
		return false
	}
	const prefix = "sha256:"
	if !strings.HasPrefix(anchor.ContentDigest, prefix) || len(anchor.ContentDigest) != len(prefix)+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(anchor.ContentDigest, prefix))
	return err == nil
}

func sessionMemoryLogicalMessages(messages []types.Message) []sessionMemoryLogicalMessage {
	groups := make([]sessionMemoryLogicalMessage, 0, len(messages))
	for start := 0; start < len(messages); {
		end := start + 1
		message := messages[start]
		if message.Role == types.RoleAssistant && message.ID != "" {
			for end < len(messages) && messages[end].Role == types.RoleAssistant && messages[end].ID == message.ID {
				end++
			}
		}
		groups = append(groups, sessionMemoryLogicalMessage{
			start:   start,
			end:     end,
			ordinal: uint64(len(groups)),
		})
		start = end
	}
	return groups
}

func sessionMemoryGroupContainsInternalMessage(messages []types.Message) bool {
	for _, message := range messages {
		if message.IsInternalRuntimeMessage() {
			return true
		}
	}
	return false
}

func sessionMemoryLogicalMessageDigest(messages []types.Message) (string, bool) {
	// The schema label makes the digest domain-specific. JSON uses Message's
	// persistence representation, so a successful session JSON round-trip
	// yields the same digest while any persisted content mutation changes it.
	payload, err := json.Marshal(struct {
		Schema   string          `json:"schema"`
		Messages []types.Message `json:"messages"`
	}{
		Schema:   "session-memory-logical-message/v1",
		Messages: messages,
	})
	if err != nil {
		return "", false
	}
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:]), true
}

// ResetSessionMemoryCompactionTracking clears last-summary tracking after a
// legacy compact replaces history. The default unavailable provider is a no-op.
func ResetSessionMemoryCompactionTracking(ctx context.Context, provider SessionMemoryProvider) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if provider == nil {
		provider = defaultSessionMemoryProvider
	}
	resetter, ok := provider.(SessionMemoryResetter)
	if !ok {
		return nil
	}
	return resetter.ResetLastSummarizedMessage(ctx)
}

func ShouldUseSessionMemoryCompaction() bool {
	if isEnvTruthy(os.Getenv("DISABLE_CLAUDE_CODE_SM_COMPACT")) {
		return false
	}
	return isEnvTruthy(os.Getenv("ENABLE_CLAUDE_CODE_SM_COMPACT"))
}

func CalculateSessionMemoryMessagesToKeepIndex(messages []types.Message, lastSummarizedIndex int, cfg SessionMemoryCompactConfig) int {
	if len(messages) == 0 {
		return 0
	}
	cfg = normalizedSessionMemoryCompactConfig(cfg)
	start := len(messages)
	if lastSummarizedIndex >= 0 {
		start = lastSummarizedIndex + 1
		if start > len(messages) {
			start = len(messages)
		}
	}

	counter := NewContextWindow(0)
	totalTokens := counter.EstimateMessages(messages[start:])
	textMessages := countTextBlockMessages(messages[start:])
	if totalTokens >= cfg.MaxTokens || (totalTokens >= cfg.MinTokens && textMessages >= cfg.MinTextBlockMessages) {
		return adjustSessionMemoryTailStart(messages, start)
	}

	floor := latestCompactBoundaryFloor(messages)
	for i := start - 1; i >= floor; i-- {
		totalTokens += counter.EstimateMessages(messages[i : i+1])
		if hasTextBlocks(messages[i]) {
			textMessages++
		}
		start = i
		if totalTokens >= cfg.MaxTokens {
			break
		}
		if totalTokens >= cfg.MinTokens && textMessages >= cfg.MinTextBlockMessages {
			break
		}
	}
	return adjustSessionMemoryTailStart(messages, start)
}

func createCompactionResultFromSessionMemory(messages []types.Message, sessionMemory string, startIndex int, messagesToKeep []types.Message, trigger string) *CompactionResult {
	counter := NewContextWindow(0)
	preCompactTokenCount := counter.EstimateMessages(messages)
	if startIndex < 0 {
		startIndex = 0
	}
	if startIndex > len(messages) {
		startIndex = len(messages)
	}
	boundary := NewCompactBoundaryMessage(CompactBoundaryMetadata{
		Trigger:                   trigger,
		PreCompactTokenCount:      preCompactTokenCount,
		PreviousTailIdentifier:    previousTailIdentifier(messages),
		PreCompactDiscoveredTools: discoveredToolNames(messages),
		PreservedSegment: &PreservedSegmentMetadata{
			StartIndex: startIndex,
			Count:      len(messagesToKeep),
			Anchor:     previousTailIdentifier(messages[:startIndex]),
		},
	})
	summaryMessage := newCompactSummaryMessage(GetCompactUserSummaryMessage(sessionMemory, true, "", true))
	return &CompactionResult{
		BoundaryMarker:       &boundary,
		SummaryMessages:      []types.Message{summaryMessage},
		MessagesToKeep:       messagesToKeep,
		PreCompactTokenCount: preCompactTokenCount,
	}
}

func normalizedSessionMemoryCompactConfig(cfg SessionMemoryCompactConfig) SessionMemoryCompactConfig {
	def := DefaultSessionMemoryCompactConfig
	if cfg.MinTokens <= 0 {
		cfg.MinTokens = def.MinTokens
	}
	if cfg.MinTextBlockMessages <= 0 {
		cfg.MinTextBlockMessages = def.MinTextBlockMessages
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = def.MaxTokens
	}
	return cfg
}

func adjustSessionMemoryTailStart(messages []types.Message, start int) int {
	floor := latestCompactBoundaryFloor(messages)
	if start < floor {
		start = floor
	}
	if floor == 0 {
		return AdjustIndexToPreserveAPIInvariants(messages, start)
	}
	return floor + AdjustIndexToPreserveAPIInvariants(messages[floor:], start-floor)
}

func latestCompactBoundaryFloor(messages []types.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if IsCompactBoundaryMessage(messages[i]) {
			return i + 1
		}
	}
	return 0
}

func countTextBlockMessages(messages []types.Message) int {
	count := 0
	for _, msg := range messages {
		if hasTextBlocks(msg) {
			count++
		}
	}
	return count
}

func hasTextBlocks(msg types.Message) bool {
	for _, block := range msg.Content {
		if text, ok := block.(types.TextBlock); ok && text.Text != "" {
			return true
		}
	}
	return false
}

func filterCompactBoundaryMessages(messages []types.Message) []types.Message {
	out := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if IsCompactBoundaryMessage(msg) {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func isEnvTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}
