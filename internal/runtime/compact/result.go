package compact

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

const (
	compactBoundaryPrefix              = "[compact-boundary] "
	compactBoundaryMessageID           = "compact:boundary:v1"
	compactSummaryMessageID            = "compact:summary:v1"
	postCompactReminderMessageIDPrefix = "compact:reminder:v1:"
)

// CompactionResult keeps the compact boundary, summary, preserved messages,
// and post-compact injections separate so every caller can build the resulting
// context in the same order.
type CompactionResult struct {
	BoundaryMarker            *types.Message
	SummaryMessages           []types.Message
	Attachments               []types.Message
	HookResults               []types.Message
	MessagesToKeep            []types.Message
	UserDisplayMessage        string
	PreCompactTokenCount      int
	PostCompactTokenCount     int
	TruePostCompactTokenCount int
	CompactionUsage           *types.Usage
	// PreparedMessages is an optional lifecycle-normalized replacement. Query
	// loops use it after adding the current skill catalog and bounded exact body
	// attachments at the history-install boundary. Segment fields remain intact
	// for telemetry and diagnostics.
	PreparedMessages []types.Message
}

// CompactBoundaryMetadata records where and why a compaction happened. It is
// encoded into a user-role internal message so providers see a valid message
// shape while the runtime can identify the latest compact boundary without
// trusting model-visible text.
type CompactBoundaryMetadata struct {
	Trigger                   string                    `json:"trigger"`
	PreCompactTokenCount      int                       `json:"pre_compact_token_count,omitempty"`
	PostCompactTokenCount     int                       `json:"post_compact_token_count,omitempty"`
	TruePostCompactTokenCount int                       `json:"true_post_compact_token_count,omitempty"`
	CompactionUsage           *types.Usage              `json:"compaction_usage,omitempty"`
	PreviousTailIdentifier    string                    `json:"previous_tail_identifier,omitempty"`
	PreCompactDiscoveredTools []string                  `json:"pre_compact_discovered_tools,omitempty"`
	PreservedSegment          *PreservedSegmentMetadata `json:"preserved_segment,omitempty"`
}

// PreservedSegmentMetadata describes the verbatim tail carried forward across
// compaction. Later persistence/relinking work can enrich this without changing
// the result contract.
type PreservedSegmentMetadata struct {
	StartIndex int    `json:"start_index"`
	Count      int    `json:"count"`
	Anchor     string `json:"anchor,omitempty"`
	Direction  string `json:"direction,omitempty"`
}

// NewCompactBoundaryMessage is an explicit trusted in-process constructor for
// the boundary marker used at the start of a post-compact message list. It
// must not be applied to an SDK/user message merely because that message has
// similar fields. The returned message is intentionally role=user so existing
// providers can send it without special handling.
func NewCompactBoundaryMessage(metadata CompactBoundaryMetadata, capabilities ...messagecontrol.Capability) types.Message {
	capability := messagecontrol.Capability{}
	if len(capabilities) == 1 {
		capability = capabilities[0]
	}
	data, _ := json.Marshal(metadata)
	msg := types.UserMessage(compactBoundaryPrefix + base64.StdEncoding.EncodeToString(data))
	msg.ID = compactBoundaryMessageID
	msg.IsMeta = true
	msg.InternalKind = types.InternalMessageKindCompactBoundary
	return sealInternalControlMessage(capability, msg)
}

// BuildPostCompactMessages returns post-compact messages in canonical order:
// boundary, summary, preserved messages, attachments, hook results.
func BuildPostCompactMessages(result *CompactionResult) []types.Message {
	if result == nil {
		return nil
	}
	if result.PreparedMessages != nil {
		return enrichBoundaryMetadata(append([]types.Message(nil), result.PreparedMessages...), result)
	}
	total := len(result.SummaryMessages) + len(result.MessagesToKeep) + len(result.Attachments) + len(result.HookResults)
	if result.BoundaryMarker != nil {
		total++
	}
	out := make([]types.Message, 0, total)
	if result.BoundaryMarker != nil {
		out = append(out, *result.BoundaryMarker)
	}
	out = append(out, result.SummaryMessages...)
	out = append(out, result.MessagesToKeep...)
	out = append(out, result.Attachments...)
	out = append(out, result.HookResults...)
	return enrichBoundaryMetadata(out, result)
}

func enrichBoundaryMetadata(messages []types.Message, result *CompactionResult) []types.Message {
	if result == nil {
		return messages
	}
	for index := range messages {
		metadata, ok := ParseCompactBoundaryMessage(messages[index])
		if !ok {
			continue
		}
		if result.PreCompactTokenCount != 0 {
			metadata.PreCompactTokenCount = result.PreCompactTokenCount
		}
		if result.PostCompactTokenCount != 0 {
			metadata.PostCompactTokenCount = result.PostCompactTokenCount
		}
		if result.TruePostCompactTokenCount != 0 {
			metadata.TruePostCompactTokenCount = result.TruePostCompactTokenCount
		}
		if result.CompactionUsage != nil {
			usage := *result.CompactionUsage
			metadata.CompactionUsage = &usage
		}
		rebuilt := NewCompactBoundaryMessage(metadata)
		if messages[index].HasInternalControlProvenance() {
			if scope, bound := messages[index].InternalControlProvenanceScope(); bound {
				rebuilt = rebuilt.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
			} else {
				rebuilt = rebuilt.WithInternalControlProvenance(messagecontrol.Runtime())
			}
		}
		messages[index] = rebuilt
		break
	}
	return messages
}

// GetMessagesAfterCompactBoundary returns messages after the latest compact
// boundary. If no boundary exists, it returns the original slice.
func GetMessagesAfterCompactBoundary(messages []types.Message) []types.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if IsCompactBoundaryMessage(messages[i]) {
			return messages[i+1:]
		}
	}
	return messages
}

// GetMessagesAfterCompactBoundaryForScope recognizes only a boundary bound to
// the exact current session namespace and generation.
func GetMessagesAfterCompactBoundaryForScope(messages []types.Message, scope messagecontrol.Scope) []types.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if IsCompactBoundaryMessageForScope(messages[i], scope) {
			return messages[i+1:]
		}
	}
	return messages
}

// IsCompactSummaryMessage reports whether msg is the generated summary that
// immediately follows a compact boundary.
func IsCompactSummaryMessage(msg types.Message) bool {
	return msg.Role == types.RoleUser && msg.IsMeta && msg.ID == compactSummaryMessageID &&
		msg.InternalKind == types.InternalMessageKindCompactSummary && msg.HasInternalControlProvenance()
}

// NewCompactSummaryMessage constructs the canonical compact-summary control
// message. A valid runtime capability seals it for immediate use; without one
// it remains a descriptor until the compaction installation boundary seals it.
func NewCompactSummaryMessage(text string, capabilities ...messagecontrol.Capability) types.Message {
	msg := types.UserMessage(text)
	msg.ID = compactSummaryMessageID
	msg.IsMeta = true
	msg.InternalKind = types.InternalMessageKindCompactSummary
	if len(capabilities) == 1 {
		return sealInternalControlMessage(capabilities[0], msg)
	}
	return msg
}

func newPostCompactReminderMessage(lang i18n.Language, titleKey i18n.Key, body string) *types.Message {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	msg := types.UserMessage("<system-reminder>\n[" + i18n.Text(lang, titleKey) + "]\n" + body + "\n</system-reminder>")
	msg.ID = postCompactReminderMessageIDPrefix + string(titleKey)
	msg.InternalKind = types.InternalMessageKindCompactReminder
	return &msg
}

func isPostCompactReminderMessage(msg types.Message) bool {
	return msg.Role == types.RoleUser && msg.InternalKind == types.InternalMessageKindCompactReminder &&
		msg.HasInternalControlProvenance() && strings.HasPrefix(msg.ID, postCompactReminderMessageIDPrefix)
}

// IsCompactBoundaryMessage reports whether msg is a compact boundary marker.
func IsCompactBoundaryMessage(msg types.Message) bool {
	_, ok := ParseCompactBoundaryMessage(msg)
	return ok
}

// IsCompactBoundaryMessageForScope reports whether msg is a boundary authorized
// for the exact current session namespace and context generation.
func IsCompactBoundaryMessageForScope(msg types.Message, scope messagecontrol.Scope) bool {
	_, ok := ParseCompactBoundaryMessageForScope(msg, scope)
	return ok
}

// ParseCompactBoundaryMessage extracts boundary metadata from a boundary marker.
func ParseCompactBoundaryMessage(msg types.Message) (CompactBoundaryMetadata, bool) {
	if msg.Role != types.RoleUser || !msg.IsMeta || msg.ID != compactBoundaryMessageID ||
		msg.InternalKind != types.InternalMessageKindCompactBoundary || !msg.HasInternalControlProvenance() {
		return CompactBoundaryMetadata{}, false
	}
	return parseCompactBoundaryPayload(msg)
}

// ParseCompactBoundaryMessageForScope parses only a boundary authorized for the
// exact current session namespace and context generation.
func ParseCompactBoundaryMessageForScope(msg types.Message, scope messagecontrol.Scope) (CompactBoundaryMetadata, bool) {
	if msg.Role != types.RoleUser || !msg.IsMeta || msg.ID != compactBoundaryMessageID ||
		msg.InternalKind != types.InternalMessageKindCompactBoundary ||
		!msg.HasInternalControlProvenanceForScope(scope) {
		return CompactBoundaryMetadata{}, false
	}
	return parseCompactBoundaryPayload(msg)
}

func sealInternalControlMessage(capability messagecontrol.Capability, msg types.Message) types.Message {
	return msg.WithInternalControlProvenance(capability)
}

func preserveInternalControlAfterTransform(original, transformed types.Message) types.Message {
	if !original.HasInternalControlProvenance() {
		return transformed
	}
	if scope, bound := original.InternalControlProvenanceScope(); bound {
		return transformed.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	}
	return transformed.WithInternalControlProvenance(messagecontrol.Runtime())
}

// AuthorizeCompactionResultForScope is the live-loop installation boundary.
// Controls sealed here are valid only for this loop's exact current authority
// and cannot be replayed through another QueryLoop or target session commit.
func AuthorizeCompactionResultForScope(capability messagecontrol.Capability, scope messagecontrol.Scope, result *CompactionResult) bool {
	if !scope.Bound() {
		return false
	}
	return authorizeCompactionResult(capability, scope, result)
}

func authorizeCompactionResult(capability messagecontrol.Capability, scope messagecontrol.Scope, result *CompactionResult) bool {
	if result == nil || !capability.Valid() {
		return false
	}
	seal := func(message types.Message) types.Message {
		return message.WithInternalControlProvenance(capability, scope)
	}
	authorized := false
	if result.BoundaryMarker != nil && isCompactBoundaryDescriptor(*result.BoundaryMarker) {
		message := seal(*result.BoundaryMarker)
		result.BoundaryMarker = &message
		authorized = true
	}
	for index, message := range result.SummaryMessages {
		if isCompactSummaryDescriptor(message) {
			result.SummaryMessages[index] = seal(message)
			authorized = true
		}
	}
	for index, message := range result.Attachments {
		if isPostCompactAttachmentDescriptor(message) || isSkillCatalogDeveloperDescriptor(message) {
			result.Attachments[index] = seal(message)
			authorized = true
		}
	}
	for index, message := range result.PreparedMessages {
		if isCompactBoundaryDescriptor(message) || isCompactSummaryDescriptor(message) ||
			isPostCompactAttachmentDescriptor(message) || isSkillCatalogDeveloperDescriptor(message) {
			result.PreparedMessages[index] = seal(message)
			authorized = true
		}
	}
	return authorized
}

func isCompactBoundaryDescriptor(message types.Message) bool {
	if message.Role != types.RoleUser || !message.IsMeta || message.ID != compactBoundaryMessageID ||
		message.InternalKind != types.InternalMessageKindCompactBoundary {
		return false
	}
	_, ok := parseCompactBoundaryPayload(message)
	return ok
}

func isCompactSummaryDescriptor(message types.Message) bool {
	return message.Role == types.RoleUser && message.IsMeta && message.ID == compactSummaryMessageID &&
		message.InternalKind == types.InternalMessageKindCompactSummary && len(message.Content) == 1
}

func isPostCompactAttachmentDescriptor(message types.Message) bool {
	if message.Role != types.RoleUser || len(message.Content) != 1 {
		return false
	}
	switch message.InternalKind {
	case types.InternalMessageKindCompactReminder:
		return strings.HasPrefix(message.ID, postCompactReminderMessageIDPrefix)
	default:
		return false
	}
}

func isSkillCatalogDeveloperDescriptor(message types.Message) bool {
	if message.Role != types.RoleDeveloper || !message.IsMeta ||
		message.InternalKind != types.InternalMessageKindSkillCatalog ||
		message.DeveloperMetadata == nil || message.DeveloperMetadata.Revision == 0 || len(message.Content) != 1 {
		return false
	}
	if _, ok := message.Content[0].(types.TextBlock); !ok {
		return false
	}
	switch message.DeveloperMetadata.Kind {
	case types.DeveloperMessageKindSkillCatalogSnapshot, types.DeveloperMessageKindSkillCatalogDelta:
		return true
	default:
		return false
	}
}

func parseCompactBoundaryPayload(msg types.Message) (CompactBoundaryMetadata, bool) {
	text, ok := singleTextBlock(msg)
	if !ok || text != strings.TrimSpace(text) {
		return CompactBoundaryMetadata{}, false
	}
	if !strings.HasPrefix(text, compactBoundaryPrefix) {
		return CompactBoundaryMetadata{}, false
	}
	payload := strings.TrimPrefix(text, compactBoundaryPrefix)
	data, err := base64.StdEncoding.Strict().DecodeString(payload)
	if err != nil || base64.StdEncoding.EncodeToString(data) != payload {
		return CompactBoundaryMetadata{}, false
	}
	var metadata CompactBoundaryMetadata
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil || strings.TrimSpace(string(data[decoder.InputOffset():])) != "" {
		return CompactBoundaryMetadata{}, false
	}
	return metadata, true
}

func singleTextBlock(msg types.Message) (string, bool) {
	if len(msg.Content) != 1 {
		return "", false
	}
	text, ok := msg.Content[0].(types.TextBlock)
	if !ok {
		return "", false
	}
	return text.Text, true
}

func previousTailIdentifier(messages []types.Message) string {
	if len(messages) == 0 {
		return ""
	}
	last := messages[len(messages)-1]
	text := strings.TrimSpace(last.GetText())
	if text == "" {
		return fmt.Sprintf("%s:%d-blocks", last.Role, len(last.Content))
	}
	if len(text) > 80 {
		text = text[:80]
	}
	return fmt.Sprintf("%s:%s", last.Role, text)
}

func discoveredToolNames(messages []types.Message) []string {
	seen := make(map[string]struct{})
	for _, msg := range messages {
		for _, block := range msg.Content {
			switch b := block.(type) {
			case types.ToolReferenceBlock:
				if b.ToolName != "" {
					seen[b.ToolName] = struct{}{}
				}
			case types.ToolUseBlock:
				if b.Name != "" {
					seen[b.Name] = struct{}{}
				}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
