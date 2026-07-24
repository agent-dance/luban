package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// TranscriptToolSegment is a render-time container for consecutive structured
// tool observations. It deliberately does not reuse ObservationAggregate:
// transcript segments are bounded by assistant/user prose and may contain
// heterogeneous tools, while observation aggregates join repeated operations
// with one semantic intent.
type TranscriptToolSegment struct {
	ID              string
	Messages        []Message
	ActorID         string
	WorkUnitID      string
	Alert           bool
	IssueCount      int
	DefaultExpanded bool
}

// TranscriptRenderItem contains either one ordinary message or one tool
// segment. Exactly one field is non-nil.
type TranscriptRenderItem struct {
	Message *Message
	Segment *TranscriptToolSegment
	// Start and End are indices in the input transcript (End is exclusive).
	// They let viewport projection happen after grouping without losing scroll
	// anchors at message boundaries.
	Start int
	End   int
}

// BuildTranscriptToolSegments groups two or more consecutive structured tool
// observations between visible transcript rows when they belong to the same
// actor and work stream. Presentation depth, lifecycle, and outcome affect how
// members render after expansion, but never split an otherwise contiguous run.
// Top-level assistant tool-only turns use a synthetic per-turn work-unit
// identity; those identities are normalized to their query scope so internal
// model turns do not fragment one visible segment. Empty or explicitly hidden
// internal rows are transparent. Message order is preserved exactly, so
// asynchronous result completion cannot reorder calls that already have stable
// transcript anchors. Agent observations remain independent lifecycle segments.
func BuildTranscriptToolSegments(messages []Message) []TranscriptRenderItem {
	items := make([]TranscriptRenderItem, 0, len(messages))
	pending := make([]Message, 0, 4)

	pendingStart := 0
	flush := func(end int) {
		if len(pending) == 0 {
			return
		}
		if len(pending) == 1 {
			message := pending[0]
			items = append(items, TranscriptRenderItem{Message: &message, Start: pendingStart, End: end})
		} else {
			segment := newTranscriptToolSegment(pending)
			items = append(items, TranscriptRenderItem{Segment: &segment, Start: pendingStart, End: end})
		}
		pending = pending[:0]
	}

	for index, message := range messages {
		if !isTranscriptToolObservation(message) {
			if isTransparentTranscriptInternalRow(message) {
				continue
			}
			flush(index)
			copy := message
			items = append(items, TranscriptRenderItem{Message: &copy, Start: index, End: index + 1})
			continue
		}
		if len(pending) > 0 && !sameTranscriptToolScope(pending[0], message) {
			flush(index)
		}
		if len(pending) == 0 {
			pendingStart = index
		}
		pending = append(pending, message)
	}
	flush(len(messages))
	return items
}

func isTranscriptToolObservation(message Message) bool {
	if message.ObservationID == "" {
		return false
	}
	// Agent and its legacy Task alias are first-class work units with their own
	// lifecycle cards. Folding either wrapper into a generic tool group bypasses
	// that card and can briefly expose the provider-facing async launch envelope.
	if CommandFamilyForTool(strings.TrimSpace(message.ToolName)) == FamilyAgent {
		return false
	}
	return message.Kind == MsgToolCall || message.Kind == MsgToolResult
}

func sameTranscriptToolScope(first, next Message) bool {
	return first.SessionID == next.SessionID && first.ActorID == next.ActorID &&
		transcriptToolWorkStream(first) == transcriptToolWorkStream(next)
}

func transcriptToolWorkStream(message Message) string {
	workUnitID := strings.TrimSpace(message.WorkUnitID)
	if strings.TrimSpace(message.ActorID) != "assistant" {
		return workUnitID
	}
	sessionID := strings.TrimSpace(message.SessionID)
	if sessionID == "" {
		return workUnitID
	}
	// queryTurnIdentity assigns a top-level assistant work unit shaped as
	// "<session>:query-<query-id>:turn-<n>". The turn suffix is an execution
	// round, not a user-visible transcript boundary. Keep the query prefix so a
	// later user request and every explicit subagent work unit remain isolated.
	turnMarker := strings.LastIndex(workUnitID, ":turn-")
	queryPrefix := sessionID + ":query-"
	if turnMarker <= len(queryPrefix) || !strings.HasPrefix(workUnitID, queryPrefix) ||
		!decimalTranscriptTurn(workUnitID[turnMarker+len(":turn-"):]) {
		return workUnitID
	}
	return workUnitID[:turnMarker]
}

func decimalTranscriptTurn(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func isTransparentTranscriptInternalRow(message Message) bool {
	if isTranscriptToolObservation(message) {
		return false
	}
	if message.PresentationHidden {
		return true
	}
	if strings.TrimSpace(message.Text) != "" {
		return false
	}
	switch message.Kind {
	case MsgAssistant, MsgAssistantThinking, MsgSystem, MsgInfo, MsgSuccess, MsgWarning:
		return true
	default:
		return false
	}
}

// transcriptObservationOngoing reports whether an observation is the latest
// visible transcript information. Tool and Agent segments stay open until a
// later visible row gives the reader enough context to move past them. Hidden
// bookkeeping and empty internal rows do not close the segment.
func transcriptObservationOngoing(messages []Message, observationID, toolUseID string) bool {
	observationID = strings.TrimSpace(observationID)
	toolUseID = strings.TrimSpace(toolUseID)
	if observationID == "" && toolUseID == "" {
		return false
	}

	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if isTransparentTranscriptInternalRow(message) {
			continue
		}
		if observationID != "" {
			return message.ObservationID == observationID
		}
		return message.ToolUseID == toolUseID
	}
	return false
}

func transcriptToolSegmentOngoing(messages []Message, segment TranscriptToolSegment) bool {
	if len(segment.Messages) == 0 || segment.Settled() {
		return false
	}
	latest := segment.Messages[len(segment.Messages)-1]
	return transcriptObservationOngoing(messages, latest.ObservationID, latest.ToolUseID)
}

func newTranscriptToolSegment(messages []Message) TranscriptToolSegment {
	segment := TranscriptToolSegment{
		Messages:   append([]Message(nil), messages...),
		ActorID:    messages[0].ActorID,
		WorkUnitID: messages[0].WorkUnitID,
	}
	for _, message := range messages {
		if transcriptToolOutcomeNeedsAttention(message.Outcome) {
			segment.Alert = true
			segment.IssueCount++
		}
	}
	// Every settled run defaults collapsed. Ongoing work, focused observations,
	// pinned members, show-all mode, and explicit group overrides are expanded
	// by RootComponent.toolSegmentExpanded without changing segment membership.
	segment.DefaultExpanded = false
	segment.ID = transcriptToolSegmentID(segment)
	return segment
}

// Settled reports whether every member has a structured terminal outcome.
// Unknown and running observations keep the segment live.
func (segment TranscriptToolSegment) Settled() bool {
	if len(segment.Messages) == 0 {
		return false
	}
	for _, message := range segment.Messages {
		if !isTerminalPresentationOutcome(message.Outcome) {
			return false
		}
	}
	return true
}

func transcriptToolSegmentID(segment TranscriptToolSegment) string {
	hash := sha256.New()
	// Tool-use identity is session-unique and survives /fork. Keeping the
	// disclosure key independent of SessionID and synthetic query-turn IDs lets
	// an exact fork checkpoint retain the same open/closed group state.
	_, _ = hash.Write([]byte("v2"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(segment.ActorID))
	_, _ = hash.Write([]byte{0})
	identity := segment.Messages[0].ObservationID
	if segment.Messages[0].ToolUseID != "" {
		identity = segment.Messages[0].ToolUseID
	}
	_, _ = hash.Write([]byte(identity))
	sum := hash.Sum(nil)
	return "tool-segment:" + hex.EncodeToString(sum[:8])
}

// SetToolSegmentExpanded records an explicit user preference without
// persisting derived segment membership.
func (s *AppState) SetToolSegmentExpanded(id string, expanded bool) {
	if s == nil || s.ToolSegmentExpansion == nil || id == "" {
		return
	}
	current := s.ToolSegmentExpansion.Get()
	next := make(map[string]bool, len(current)+1)
	for key, value := range current {
		next[key] = value
	}
	next[id] = expanded
	s.ToolSegmentExpansion.Set(next)
	s.bumpViewRevision()
}

// ToolSegmentExpanded reports an explicit expanded override. Derived defaults
// are applied by the renderer through toolSegmentExpansionOverride.
func (s *AppState) ToolSegmentExpanded(id string) bool {
	expanded, ok := s.toolSegmentExpansionOverride(id)
	return ok && expanded
}

func (s *AppState) toolSegmentExpansionOverride(id string) (bool, bool) {
	if s == nil || s.ToolSegmentExpansion == nil || id == "" {
		return false, false
	}
	expanded, ok := s.ToolSegmentExpansion.Get()[id]
	return expanded, ok
}

func transcriptToolOutcomeNeedsAttention(outcome ObservationOutcome) bool {
	switch outcome {
	case OutcomeFailed, OutcomePartial, OutcomeDenied, OutcomeCancelled,
		OutcomeTimedOut, OutcomeEscaped, OutcomeShutdown, OutcomeOrphan,
		OutcomeConflict:
		return true
	default:
		return false
	}
}

// Summary returns the compact group label in the active runtime language.
func (segment TranscriptToolSegment) Summary(lang i18n.Language) string {
	var summary string
	if segment.AllRead() {
		summary = i18n.Text(lang, i18n.KeyToolSegmentReadFiles)
	} else {
		summary = i18n.Format(lang, i18n.KeyToolSegmentUsedTools, len(segment.Messages))
	}
	if segment.Alert {
		summary = i18n.Format(lang, i18n.KeyToolSegmentIssues, summary, segment.IssueCount)
	}
	return strings.TrimSpace(summary)
}

// AllRead reports whether every member is a file-read family operation. It is
// intentionally computed from tool identities, not localized summaries.
func (segment TranscriptToolSegment) AllRead() bool {
	if len(segment.Messages) == 0 {
		return false
	}
	for _, message := range segment.Messages {
		if CommandFamilyForTool(message.ToolName) != FamilyFileRead {
			return false
		}
	}
	return true
}
