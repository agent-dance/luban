package types

import (
	"encoding/json"
	"strings"

	"github.com/agent-dance/luban/internal/messagecontrol"
)

// ContentType represents the type of a content block
type ContentType string

const (
	ContentTypeText                ContentType = "text"
	ContentTypeToolUse             ContentType = "tool_use"
	ContentTypeToolResult          ContentType = "tool_result"
	ContentTypeToolReference       ContentType = "tool_reference"
	ContentTypeThinking            ContentType = "thinking"
	ContentTypeImage               ContentType = "image"
	ContentTypeDocument            ContentType = "document"
	ContentTypeReplacement         ContentType = "content_replacement"
	ContentTypeServerToolUse       ContentType = "server_tool_use"
	ContentTypeWebSearchToolResult ContentType = "web_search_tool_result"
)

// Role represents a message role
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleDeveloper Role = "developer"
)

// DeveloperMessageKind identifies the internal instruction carried by a
// developer-role message. Provider adapters choose how to project the role;
// this local classification is persisted for session reconstruction only.
type DeveloperMessageKind string

const (
	DeveloperMessageKindSkillCatalogSnapshot DeveloperMessageKind = "skill_catalog_snapshot"
	DeveloperMessageKindSkillCatalogDelta    DeveloperMessageKind = "skill_catalog_delta"
)

// DeveloperMessageMetadata records the catalog state represented by a
// developer-role message without putting bookkeeping into model-visible text.
// Revision is the resulting catalog revision for both snapshots and deltas.
type DeveloperMessageMetadata struct {
	Kind     DeveloperMessageKind `json:"kind"`
	Revision uint64               `json:"revision"`
}

// InternalMessageKind identifies model-visible runtime control messages
// without relying on their localized text. Empty preserves legacy sessions.
type InternalMessageKind string

const (
	InternalMessageKindOutputTokenRecovery InternalMessageKind = "output_token_recovery"
	InternalMessageKindCompactBoundary     InternalMessageKind = "compact_boundary"
	InternalMessageKindCompactSummary      InternalMessageKind = "compact_summary"
	InternalMessageKindCompactFileRecovery InternalMessageKind = "compact_file_recovery"
	InternalMessageKindCompactReminder     InternalMessageKind = "compact_reminder"
	InternalMessageKindBackgroundFollowUp  InternalMessageKind = "background_follow_up"
	InternalMessageKindFileReadSecurity    InternalMessageKind = "file_read_security"
	InternalMessageKindSkillCatalog        InternalMessageKind = "skill_catalog"
	InternalMessageKindUserContext         InternalMessageKind = "user_context"
	InternalMessageKindGoalContinuation    InternalMessageKind = "goal_continuation"
	InternalMessageKindSkillInvocation     InternalMessageKind = "skill_invocation"
	// InternalMessageKindContextCollapseStaged is an ephemeral, process-local
	// projection marker. Session persistence must reject authenticated values
	// of this kind until a durable staged-collapse store exists.
	InternalMessageKindContextCollapseStaged InternalMessageKind = "context_collapse_staged"
)

// StopReason represents why the model stopped generating
type StopReason string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonMaxTokens StopReason = "max_tokens"
	StopReasonToolUse   StopReason = "tool_use"
)

// ToolOutcome records the deterministic execution result independently from
// human-readable tool output. Empty values preserve legacy session behavior.
type ToolOutcome string

const (
	ToolOutcomeSucceeded ToolOutcome = "succeeded"
	ToolOutcomeFailed    ToolOutcome = "failed"
	ToolOutcomePartial   ToolOutcome = "partial"
	ToolOutcomeDenied    ToolOutcome = "denied"
	ToolOutcomeCancelled ToolOutcome = "cancelled"
	ToolOutcomeTimedOut  ToolOutcome = "timed_out"
)

// ContentBlock is the interface for all content block types
type ContentBlock interface {
	GetType() ContentType
}

// TextBlock represents a text content block
type TextBlock struct {
	Type ContentType `json:"type"`
	Text string      `json:"text"`
}

func (b TextBlock) GetType() ContentType { return ContentTypeText }

// ThinkingBlock represents a thinking/reasoning content block
type ThinkingBlock struct {
	Type      ContentType `json:"type"`
	Thinking  string      `json:"thinking"`
	Signature string      `json:"signature,omitempty"`
}

func (b ThinkingBlock) GetType() ContentType { return ContentTypeThinking }

// ToolUseBlock represents a tool invocation from the assistant
type ToolUseBlock struct {
	Type  ContentType    `json:"type"`
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func (b ToolUseBlock) GetType() ContentType { return ContentTypeToolUse }

// ToolResultBlock represents the result of a tool execution
type ToolResultBlock struct {
	Type          ContentType            `json:"type"`
	ToolUseID     string                 `json:"tool_use_id"`
	Content       string                 `json:"-"`
	ContentBlocks []ContentBlock         `json:"-"`
	Data          any                    `json:"-"`
	IsError       bool                   `json:"is_error,omitempty"`
	NewMessages   []Message              `json:"-"`
	Metadata      map[string]string      `json:"-"`
	Usage         *Usage                 `json:"-"`
	Outcome       ToolOutcome            `json:"outcome,omitempty"`
	Completeness  ToolResultCompleteness `json:"-"`
}

func (b ToolResultBlock) GetType() ContentType { return ContentTypeToolResult }

func (b ToolResultBlock) HasStructuredContent() bool {
	return len(b.ContentBlocks) > 0
}

func (b ToolResultBlock) TextContent() string {
	return toolResultTextContent(b.Content, b.ContentBlocks)
}

func (b ToolResultBlock) HasMediaContent() bool {
	return toolResultHasMediaContent(b.ContentBlocks)
}

// ContentReplacementBlock is a session-local record used to reconstruct
// tool-result content replacement decisions on resume. It must be stripped
// before provider requests are built.
type ContentReplacementBlock struct {
	Type                      ContentType `json:"type"`
	Kind                      string      `json:"kind"`
	ToolUseID                 string      `json:"tool_use_id"`
	Replacement               string      `json:"replacement"`
	internalReplacementDigest [messagecontrol.DigestSize]byte
	internalReplacementScope  messagecontrol.Scope
}

func (b ContentReplacementBlock) GetType() ContentType { return ContentTypeReplacement }

func (b ContentReplacementBlock) WithInternalReplacementProvenance(capability messagecontrol.Capability, scopes ...messagecontrol.Scope) ContentReplacementBlock {
	b.internalReplacementDigest = [messagecontrol.DigestSize]byte{}
	b.internalReplacementScope = messagecontrol.Scope{}
	if !capability.Valid() || b.Kind == "" || b.ToolUseID == "" || len(scopes) > 1 {
		return b
	}
	if len(scopes) == 1 {
		b.internalReplacementScope = scopes[0]
	}
	data, err := b.serializedReplacementBytes()
	if err == nil {
		b.internalReplacementDigest, _ = messagecontrol.Authenticate(capability, data)
	}
	return b
}

func (b ContentReplacementBlock) HasInternalReplacementProvenance() bool {
	if b.internalReplacementDigest == ([messagecontrol.DigestSize]byte{}) {
		return false
	}
	data, err := b.serializedReplacementBytes()
	return err == nil && messagecontrol.Verify(data, b.internalReplacementDigest)
}

func (b ContentReplacementBlock) serializedReplacementBytes() ([]byte, error) {
	descriptor, err := b.replacementDescriptorBytes()
	if err != nil {
		return nil, err
	}
	prefix := b.internalReplacementScope.AuthenticationPrefix()
	data := make([]byte, 0, len(prefix)+len(descriptor))
	data = append(data, prefix...)
	data = append(data, descriptor...)
	return data, nil
}

// InternalReplacementProvenanceScope returns the authenticated durable scope.
// A freshly installed runtime record is valid but unbound until session commit.
func (b ContentReplacementBlock) InternalReplacementProvenanceScope() (messagecontrol.Scope, bool) {
	if !b.HasInternalReplacementProvenance() {
		return messagecontrol.Scope{}, false
	}
	return b.internalReplacementScope, b.internalReplacementScope.Bound()
}

// HasInternalReplacementProvenanceForScope accepts only an exact authority
// scope. A process-wide HMAC without a scope is deliberately not a bearer
// capability: it can be copied between live loops and must never be promoted
// by reconstruction, persistence, provider, or presentation boundaries.
func (b ContentReplacementBlock) HasInternalReplacementProvenanceForScope(scope messagecontrol.Scope, allowUnbound bool) bool {
	if !b.HasInternalReplacementProvenance() {
		return false
	}
	if !b.internalReplacementScope.Bound() {
		return false
	}
	return b.internalReplacementScope.Equal(scope)
}

func (b ContentReplacementBlock) replacementDescriptorBytes() ([]byte, error) {
	return json.Marshal(struct {
		Type        ContentType `json:"type"`
		Kind        string      `json:"kind"`
		ToolUseID   string      `json:"tool_use_id"`
		Replacement string      `json:"replacement"`
	}{b.Type, b.Kind, b.ToolUseID, b.Replacement})
}

// ToolReferenceBlock represents a deferred tool that has been discovered by
// ToolSearch and can now be surfaced to the model.
type ToolReferenceBlock struct {
	Type     ContentType `json:"type"`
	ToolName string      `json:"tool_name"`
}

func (b ToolReferenceBlock) GetType() ContentType { return ContentTypeToolReference }

// ImageBlock represents an image content block
type ImageBlock struct {
	Type   ContentType  `json:"type"`
	Source *ImageSource `json:"source"`
}

func (b ImageBlock) GetType() ContentType { return ContentTypeImage }

// DocumentBlock represents a document content block (PDF, Word, etc.).
// The Source field carries the base64-encoded document data, analogous to
// ImageBlock. Introduced so StripImagesFromMessages can replace document
// blocks with [document] markers, matching the TS stripImagesFromMessages.
type DocumentBlock struct {
	Type   ContentType     `json:"type"`
	Source *DocumentSource `json:"source,omitempty"`
}

func (b DocumentBlock) GetType() ContentType { return ContentTypeDocument }

// DocumentSource represents the source data for a document block.
type DocumentSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// UnknownBlock preserves raw JSON for content types not yet modelled in this
// client. This avoids silently coercing unknown API responses into an empty
// TextBlock and losing the original data.
type UnknownBlock struct {
	Type ContentType     `json:"type"`
	Raw  json.RawMessage `json:"-"`
}

func (b UnknownBlock) GetType() ContentType { return b.Type }

// ImageSource represents an image source
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// Message represents a conversation message
type Message struct {
	ID                string                    `json:"id,omitempty"`
	Role              Role                      `json:"role"`
	Content           []ContentBlock            `json:"content"`
	IsMeta            bool                      `json:"is_meta,omitempty"`
	DeveloperMetadata *DeveloperMessageMetadata `json:"developer_metadata,omitempty"`
	InternalKind      InternalMessageKind       `json:"internal_kind,omitempty"`
	// internalControlDigest is deliberately neither exported nor serialized.
	// It binds trusted runtime provenance to the complete serialized message,
	// so copying is safe while mutation and ordinary JSON decoding invalidate
	// the authority.
	internalControlDigest [messagecontrol.DigestSize]byte
	internalControlScope  messagecontrol.Scope
}

// WithInternalControlProvenance seals a runtime-created control message with
// a process-local capability. SDK values and JSON cannot obtain the authority
// needed to set the unexported provenance field.
func (m Message) WithInternalControlProvenance(capability messagecontrol.Capability, scopes ...messagecontrol.Scope) Message {
	m.internalControlDigest = [messagecontrol.DigestSize]byte{}
	m.internalControlScope = messagecontrol.Scope{}
	if !capability.Valid() || m.InternalKind == "" || len(scopes) > 1 {
		return m
	}
	if len(scopes) == 1 {
		m.internalControlScope = scopes[0]
	}
	data, ok := m.serializedControlBytes()
	if ok {
		m.internalControlDigest, _ = messagecontrol.Authenticate(capability, data)
	}
	return m
}

// HasInternalControlProvenance reports whether this exact message was sealed
// by the runtime and has not subsequently been mutated.
func (m Message) HasInternalControlProvenance() bool {
	if m.InternalKind == "" || m.internalControlDigest == ([messagecontrol.DigestSize]byte{}) {
		return false
	}
	data, ok := m.serializedControlBytes()
	return ok && messagecontrol.Verify(data, m.internalControlDigest)
}

func (m Message) serializedControlBytes() ([]byte, bool) {
	messageData, err := json.Marshal(m)
	if err != nil {
		return nil, false
	}
	prefix := m.internalControlScope.AuthenticationPrefix()
	data := make([]byte, 0, len(prefix)+len(messageData))
	data = append(data, prefix...)
	data = append(data, messageData...)
	return data, true
}

func (m *Message) clearInternalControlProvenance() {
	m.internalControlDigest = [messagecontrol.DigestSize]byte{}
	m.internalControlScope = messagecontrol.Scope{}
}

// InternalControlProvenanceScope returns the authenticated durable scope. A
// freshly-created runtime control is valid but unbound until session commit.
func (m Message) InternalControlProvenanceScope() (messagecontrol.Scope, bool) {
	if !m.HasInternalControlProvenance() {
		return messagecontrol.Scope{}, false
	}
	return m.internalControlScope, m.internalControlScope.Bound()
}

// HasInternalControlProvenanceForScope accepts only an exact authority scope.
// allowUnbound is retained for source compatibility but cannot turn a
// process-wide HMAC into transferable pre-commit authority.
func (m Message) HasInternalControlProvenanceForScope(scope messagecontrol.Scope, allowUnbound bool) bool {
	if !m.HasInternalControlProvenance() {
		return false
	}
	if !m.internalControlScope.Bound() {
		return false
	}
	return m.internalControlScope.Equal(scope)
}

// IsInternalRuntimeMessage reports whether this exact message carries verified
// runtime provenance. Exported descriptors (Role, IsMeta, InternalKind, ID and
// DeveloperMetadata) are intentionally not authority: SDK values and ordinary
// JSON may set all of them and must remain visible, ordinary messages.
func (m Message) IsInternalRuntimeMessage() bool {
	return m.InternalKind != InternalMessageKindSkillInvocation && m.HasInternalControlProvenance()
}

// IsInternalRuntimeMessageForScope is the provider/presentation form of
// IsInternalRuntimeMessage. Bound durable controls are privileged only in the
// exact current namespace and generation; freshly installed unbound controls
// may be accepted only at a private pre-commit boundary.
func (m Message) IsInternalRuntimeMessageForScope(scope messagecontrol.Scope, allowUnbound bool) bool {
	return m.InternalKind != InternalMessageKindSkillInvocation &&
		m.HasInternalControlProvenanceForScope(scope, allowUnbound)
}

func (m Message) IsTrustedSkillInvocationMessage() bool {
	if m.Role != RoleUser || m.InternalKind != InternalMessageKindSkillInvocation || !m.HasInternalControlProvenance() || len(m.Content) != 1 {
		return false
	}
	_, ok := m.Content[0].(TextBlock)
	return ok
}

func (m Message) IsTrustedSkillInvocationMessageForScope(scope messagecontrol.Scope, allowUnbound bool) bool {
	if !m.IsTrustedSkillInvocationMessage() {
		return false
	}
	return m.HasInternalControlProvenanceForScope(scope, allowUnbound)
}

// IsTrustedDeveloperMessage reports whether the runtime, rather than an SDK or
// decoded JSON value, authorized this exact message for a high-priority
// developer-role projection.
func (m Message) IsTrustedDeveloperMessage() bool {
	if m.Role != RoleDeveloper || !m.IsMeta || m.InternalKind != InternalMessageKindSkillCatalog ||
		m.DeveloperMetadata == nil || m.DeveloperMetadata.Revision == 0 || !m.HasInternalControlProvenance() || len(m.Content) != 1 {
		return false
	}
	if _, ok := m.Content[0].(TextBlock); !ok {
		return false
	}
	switch m.DeveloperMetadata.Kind {
	case DeveloperMessageKindSkillCatalogSnapshot, DeveloperMessageKindSkillCatalogDelta:
		return true
	default:
		return false
	}
}

func (m Message) IsTrustedDeveloperMessageForScope(scope messagecontrol.Scope, allowUnbound bool) bool {
	if !m.IsTrustedDeveloperMessage() {
		return false
	}
	return m.HasInternalControlProvenanceForScope(scope, allowUnbound)
}

func toolResultTextContent(text string, blocks []ContentBlock) string {
	if len(blocks) == 0 {
		return text
	}
	var parts []string
	hasRichContent := false
	for _, block := range blocks {
		switch typed := block.(type) {
		case TextBlock:
			if typed.Text != "" {
				parts = append(parts, typed.Text)
				hasRichContent = true
			}
		case ImageBlock:
			parts = append(parts, "[image]")
			hasRichContent = true
		case DocumentBlock:
			parts = append(parts, "[document]")
			hasRichContent = true
		case ToolReferenceBlock:
			if typed.ToolName != "" {
				parts = append(parts, "[tool:"+typed.ToolName+"]")
			}
		case UnknownBlock:
			if typed.Type != "" {
				parts = append(parts, "["+string(typed.Type)+"]")
				hasRichContent = true
			}
		}
	}
	if len(parts) == 0 {
		return text
	}
	if !hasRichContent && text != "" {
		return text
	}
	return strings.Join(parts, "\n")
}

func toolResultHasMediaContent(blocks []ContentBlock) bool {
	for _, block := range blocks {
		switch block.GetType() {
		case ContentTypeImage, ContentTypeDocument:
			return true
		}
	}
	return false
}

// UserMessage creates a new user message with text content
func UserMessage(text string) Message {
	return Message{
		Role: RoleUser,
		Content: []ContentBlock{
			TextBlock{Type: ContentTypeText, Text: text},
		},
	}
}

// AssistantMessage creates a new assistant message with text content
func AssistantMessage(text string) Message {
	return Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			TextBlock{Type: ContentTypeText, Text: text},
		},
	}
}

// DeveloperMessage creates an internal developer-role catalog instruction.
// Metadata is kept outside Content so provider adapters cannot expose catalog
// bookkeeping unless they deliberately opt into it.
func DeveloperMessage(text string, metadata DeveloperMessageMetadata) Message {
	return Message{
		Role: RoleDeveloper,
		Content: []ContentBlock{
			TextBlock{Type: ContentTypeText, Text: text},
		},
		IsMeta:            true,
		DeveloperMetadata: &metadata,
		InternalKind:      InternalMessageKindSkillCatalog,
	}
}

// ToolResultMessage creates a user message containing tool results
func ToolResultMessage(results ...ToolResultBlock) Message {
	content := make([]ContentBlock, len(results))
	for i, r := range results {
		r.Type = ContentTypeToolResult
		content[i] = r
	}
	return Message{
		Role:    RoleUser,
		Content: content,
	}
}

// GetText extracts all text content from a message
func (m Message) GetText() string {
	var sb strings.Builder
	for _, block := range m.Content {
		if tb, ok := block.(TextBlock); ok {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(tb.Text)
		}
	}
	return sb.String()
}

// GetToolUses extracts all tool use blocks from a message
func (m Message) GetToolUses() []ToolUseBlock {
	var uses []ToolUseBlock
	for _, block := range m.Content {
		if tu, ok := block.(ToolUseBlock); ok {
			uses = append(uses, tu)
		}
	}
	return uses
}

// HasToolUse checks if the message contains any tool use blocks
func (m Message) HasToolUse() bool {
	for _, block := range m.Content {
		if _, ok := block.(ToolUseBlock); ok {
			return true
		}
	}
	return false
}
