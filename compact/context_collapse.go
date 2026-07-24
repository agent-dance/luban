package compact

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

const (
	contextCollapseStagedPrefix    = "[context-collapse-staged] "
	contextCollapseStagedMessageID = "compact:context-collapse-staged:v1"
	contextCollapseStagedKind      = types.InternalMessageKindContextCollapseStaged
)

type contextCollapsePayload struct {
	Messages         []types.Message                 `json:"messages"`
	TrustedControls  []contextCollapseTrustedControl `json:"trusted_controls,omitempty"`
	OwnerScopeDigest string                          `json:"owner_scope_digest,omitempty"`
}

type contextCollapseTrustedControl struct {
	Index       int                       `json:"index"`
	Digest      string                    `json:"digest"`
	Kind        types.InternalMessageKind `json:"kind"`
	ScopeDigest string                    `json:"scope_digest,omitempty"`
}

// ContextCollapseDrainResult is the minimal recovery projection used when a
// staged context collapse exists before a prompt-too-long failure.
type ContextCollapseDrainResult struct {
	Messages  []types.Message
	Committed int
}

// ContextCollapseProjectionResult is the pre-call projection of a staged
// collapse marker. Go does not implement the full TS context-collapse store;
// this result only applies an explicit message-backed staged marker.
type ContextCollapseProjectionResult struct {
	Messages  []types.Message
	Projected int
}

// NewContextCollapseStagedMessage is an explicit trusted in-process
// constructor that encodes a staged collapsed view. It must not be used to
// promote an SDK/user message based on matching exported fields. The full TS
// context-collapse store is intentionally unsupported in Go; this
// message-backed adapter gives the loop a deterministic projection/drain
// contract without claiming full parity.
func NewContextCollapseStagedMessage(messages []types.Message, capabilities ...messagecontrol.Capability) types.Message {
	capability := messagecontrol.Capability{}
	if len(capabilities) == 1 {
		capability = capabilities[0]
	}
	msg, _ := newContextCollapseStagedMessage(messages, capability, nil)
	return msg
}

// NewContextCollapseStagedMessageForScope constructs a staged marker whose
// outer authority and every trusted control carried by its payload belong to
// the same exact session generation. A foreign or unbound nested control is a
// hard construction failure rather than a bearer that a later loop may adopt.
func NewContextCollapseStagedMessageForScope(
	messages []types.Message,
	capability messagecontrol.Capability,
	scope messagecontrol.Scope,
) (types.Message, bool) {
	if !capability.Valid() || !scope.Bound() {
		return types.Message{}, false
	}
	return newContextCollapseStagedMessage(messages, capability, &scope)
}

func newContextCollapseStagedMessage(
	messages []types.Message,
	capability messagecontrol.Capability,
	expectedScope *messagecontrol.Scope,
) (types.Message, bool) {
	payload := contextCollapsePayload{Messages: messages}
	if expectedScope != nil {
		payload.OwnerScopeDigest = contextCollapseScopeDigest(*expectedScope)
	}
	for index, message := range messages {
		if !contextCollapseMessagePayloadSupported(message) || !contextCollapseMessageRoundTrips(message) {
			return failedContextCollapseStagedMessage(capability, expectedScope)
		}
		if !message.HasInternalControlProvenance() {
			continue
		}
		if message.InternalKind == types.InternalMessageKindContextCollapseStaged {
			return failedContextCollapseStagedMessage(capability, expectedScope)
		}
		scope, bound := message.InternalControlProvenanceScope()
		if expectedScope != nil && (!bound || !scope.Equal(*expectedScope)) {
			return types.Message{}, false
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			return types.Message{}, false
		}
		digest := sha256.Sum256(encoded)
		trusted := contextCollapseTrustedControl{
			Index: index, Digest: "sha256:" + hex.EncodeToString(digest[:]), Kind: message.InternalKind,
		}
		if bound {
			trusted.ScopeDigest = contextCollapseScopeDigest(scope)
		}
		payload.TrustedControls = append(payload.TrustedControls, trusted)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return failedContextCollapseStagedMessage(capability, expectedScope)
	}
	msg := types.UserMessage(contextCollapseStagedPrefix + base64.StdEncoding.EncodeToString(data))
	msg.ID = contextCollapseStagedMessageID
	msg.IsMeta = true
	msg.InternalKind = contextCollapseStagedKind
	return sealContextCollapseStagedMessage(msg, capability, expectedScope), true
}

func failedContextCollapseStagedMessage(
	capability messagecontrol.Capability,
	scope *messagecontrol.Scope,
) (types.Message, bool) {
	if scope != nil {
		return types.Message{}, false
	}
	return invalidContextCollapseStagedMessage(capability, nil), false
}

func invalidContextCollapseStagedMessage(
	capability messagecontrol.Capability,
	scope *messagecontrol.Scope,
) types.Message {
	msg := types.UserMessage(contextCollapseStagedPrefix + "{}")
	msg.ID = contextCollapseStagedMessageID
	msg.IsMeta = true
	msg.InternalKind = contextCollapseStagedKind
	return sealContextCollapseStagedMessage(msg, capability, scope)
}

func contextCollapseScopeDigest(scope messagecontrol.Scope) string {
	digest := sha256.Sum256(scope.AuthenticationPrefix())
	return "sha256:" + hex.EncodeToString(digest[:])
}

// contextCollapseMessagePayloadSupported rejects process-local sidecars that
// Message/ToolResult JSON intentionally omits, and authenticated nested
// replacement records whose authority could not be restored exactly. The
// adapter must fail closed instead of silently dropping either data or trust.
func contextCollapseMessagePayloadSupported(message types.Message) bool {
	if message.Content == nil {
		return false
	}
	for _, block := range message.Content {
		if !contextCollapseContentBlockPayloadSupported(block) {
			return false
		}
	}
	return true
}

func contextCollapseMessageRoundTrips(message types.Message) bool {
	encoded, err := json.Marshal(message)
	if err != nil {
		return false
	}
	var decoded types.Message
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return false
	}
	reencoded, err := json.Marshal(decoded)
	return err == nil && bytes.Equal(encoded, reencoded) && contextCollapseMessagePublicEqual(message, decoded)
}

func contextCollapseMessagePublicEqual(left, right types.Message) bool {
	return left.ID == right.ID && left.Role == right.Role && left.IsMeta == right.IsMeta &&
		left.InternalKind == right.InternalKind && reflect.DeepEqual(left.DeveloperMetadata, right.DeveloperMetadata) &&
		reflect.DeepEqual(left.Content, right.Content)
}

func contextCollapseContentBlockPayloadSupported(block types.ContentBlock) bool {
	switch typed := block.(type) {
	case types.TextBlock:
		return typed.Type == types.ContentTypeText
	case types.ThinkingBlock:
		return typed.Type == types.ContentTypeThinking
	case types.ToolUseBlock:
		return typed.Type == types.ContentTypeToolUse && contextCollapseJSONMapSupported(typed.Input)
	case types.ToolReferenceBlock:
		return typed.Type == types.ContentTypeToolReference
	case types.ImageBlock:
		return typed.Type == types.ContentTypeImage
	case types.DocumentBlock:
		return typed.Type == types.ContentTypeDocument
	case types.UnknownBlock:
		// UnknownBlock preservation depends on the surrounding marshal path;
		// reject it rather than risk retaining only its exported type.
		return false
	case types.ContentReplacementBlock:
		return typed.Type == types.ContentTypeReplacement && !typed.HasInternalReplacementProvenance()
	case types.ToolResultBlock:
		return contextCollapseToolResultPayloadSupported(typed)
	default:
		// The staged adapter is deliberately a closed, value-only codec. Pointer,
		// nil, and custom blocks can change dynamic type or lose private fields
		// during the JSON round trip.
		return false
	}
}

func contextCollapseJSONMapSupported(values map[string]any) bool {
	return contextCollapseJSONMapSupportedAtDepth(values, 0)
}

const contextCollapseMaxJSONDepth = 64

func contextCollapseJSONMapSupportedAtDepth(values map[string]any, depth int) bool {
	if depth > contextCollapseMaxJSONDepth {
		return false
	}
	for _, value := range values {
		if !contextCollapseJSONValueSupportedAtDepth(value, depth+1) {
			return false
		}
	}
	return true
}

func contextCollapseJSONValueSupportedAtDepth(value any, depth int) bool {
	if depth > contextCollapseMaxJSONDepth {
		return false
	}
	switch typed := value.(type) {
	case nil, bool, string, float64:
		return true
	case []any:
		if typed == nil {
			return false
		}
		for _, nested := range typed {
			if !contextCollapseJSONValueSupportedAtDepth(nested, depth+1) {
				return false
			}
		}
		return true
	case map[string]any:
		return typed != nil && contextCollapseJSONMapSupportedAtDepth(typed, depth+1)
	default:
		// encoding/json decodes interface numbers as float64 and containers as
		// []any/map[string]any. Reject other dynamic types even when their wire
		// bytes happen to match, so projection cannot silently change structure.
		return false
	}
}

func contextCollapseToolResultPayloadSupported(result types.ToolResultBlock) bool {
	if result.Type != types.ContentTypeToolResult || result.NewMessages != nil || result.Data != nil || result.Metadata != nil ||
		result.Usage != nil || !result.Completeness.IsZero() {
		return false
	}
	if result.ContentBlocks != nil && len(result.ContentBlocks) == 0 {
		return false
	}
	if result.Content != "" && len(result.ContentBlocks) > 0 {
		return false
	}
	for _, nested := range result.ContentBlocks {
		if !contextCollapseContentBlockPayloadSupported(nested) {
			return false
		}
	}
	return true
}

func sealContextCollapseStagedMessage(
	msg types.Message,
	capability messagecontrol.Capability,
	scope *messagecontrol.Scope,
) types.Message {
	if scope != nil {
		return msg.WithInternalControlProvenance(capability, *scope)
	}
	return msg.WithInternalControlProvenance(capability)
}

// ProjectStagedContextCollapse applies the latest staged collapse marker before
// a provider call, replacing all messages up to that marker with its collapsed
// view and keeping the tail that followed it. If no valid staged marker exists,
// it is a no-op. This is the only Go-supported context-collapse behavior; the
// full TS staged-collapse subsystem/store remains unsupported.
func ProjectStagedContextCollapse(messages []types.Message) ContextCollapseProjectionResult {
	drained := projectLatestStagedContextCollapse(messages, nil)
	return ContextCollapseProjectionResult{Messages: drained.Messages, Projected: drained.Committed}
}

// ProjectStagedContextCollapseForScope is the live-loop projection path. It
// accepts only a marker and trusted payload controls authenticated for the
// loop's exact current scope.
func ProjectStagedContextCollapseForScope(messages []types.Message, scope messagecontrol.Scope) ContextCollapseProjectionResult {
	drained := projectLatestStagedContextCollapse(messages, &scope)
	return ContextCollapseProjectionResult{Messages: drained.Messages, Projected: drained.Committed}
}

// RecoverFromContextCollapseOverflow commits the latest staged collapse marker,
// replacing all messages up to that marker with its collapsed view and keeping
// the tail that followed it. If no staged collapse exists, it is a no-op.
func RecoverFromContextCollapseOverflow(messages []types.Message) ContextCollapseDrainResult {
	return projectLatestStagedContextCollapse(messages, nil)
}

// RecoverFromContextCollapseOverflowForScope is the live-loop overflow drain
// path and applies the same exact-scope contract as pre-call projection.
func RecoverFromContextCollapseOverflowForScope(messages []types.Message, scope messagecontrol.Scope) ContextCollapseDrainResult {
	return projectLatestStagedContextCollapse(messages, &scope)
}

func projectLatestStagedContextCollapse(messages []types.Message, expectedScope *messagecontrol.Scope) ContextCollapseDrainResult {
	if expectedScope != nil && !contextCollapseMessagesMatchScope(messages, *expectedScope) {
		return ContextCollapseDrainResult{Messages: messages}
	}
	for i := len(messages) - 1; i >= 0; i-- {
		var collapsed []types.Message
		var ok bool
		if expectedScope == nil {
			collapsed, ok = ParseContextCollapseStagedMessage(messages[i])
		} else {
			collapsed, ok = ParseContextCollapseStagedMessageForScope(messages[i], *expectedScope)
		}
		if !ok {
			continue
		}
		out := make([]types.Message, 0, len(collapsed)+len(messages)-i-1)
		out = append(out, collapsed...)
		out = append(out, messages[i+1:]...)
		return ContextCollapseDrainResult{Messages: out, Committed: 1}
	}
	return ContextCollapseDrainResult{Messages: messages}
}

func contextCollapseMessagesMatchScope(messages []types.Message, scope messagecontrol.Scope) bool {
	if !scope.Bound() {
		return false
	}
	for _, message := range messages {
		if message.HasInternalControlProvenance() && !message.HasInternalControlProvenanceForScope(scope, false) {
			return false
		}
		for _, block := range message.Content {
			if !contextCollapseContentBlockMatchesScope(block, scope) {
				return false
			}
		}
	}
	return true
}

func contextCollapseContentBlockMatchesScope(block types.ContentBlock, scope messagecontrol.Scope) bool {
	switch typed := block.(type) {
	case types.ContentReplacementBlock:
		return !typed.HasInternalReplacementProvenance() || typed.HasInternalReplacementProvenanceForScope(scope, false)
	case types.ToolResultBlock:
		for _, nested := range typed.ContentBlocks {
			if !contextCollapseContentBlockMatchesScope(nested, scope) {
				return false
			}
		}
		return contextCollapseMessagesMatchScope(typed.NewMessages, scope)
	case *types.ContentReplacementBlock, *types.ToolResultBlock:
		// Pointer controls are not accepted by the durable/session walkers and
		// must not cross a collapse installation boundary.
		return false
	default:
		return true
	}
}

// ParseContextCollapseStagedMessage decodes a staged collapse marker.
func ParseContextCollapseStagedMessage(msg types.Message) ([]types.Message, bool) {
	if msg.Role != types.RoleUser || !msg.IsMeta || msg.ID != contextCollapseStagedMessageID ||
		msg.InternalKind != contextCollapseStagedKind || !msg.HasInternalControlProvenance() {
		return nil, false
	}
	if _, bound := msg.InternalControlProvenanceScope(); bound {
		return nil, false
	}
	return parseContextCollapseStagedPayload(msg, nil)
}

// ParseContextCollapseStagedMessageForScope decodes only a marker owned by the
// exact expected session generation. Trusted controls in the payload must
// declare that same original scope and are restored directly to it; they never
// pass through a transferable unbound state.
func ParseContextCollapseStagedMessageForScope(
	msg types.Message,
	scope messagecontrol.Scope,
) ([]types.Message, bool) {
	if !scope.Bound() || msg.Role != types.RoleUser || !msg.IsMeta || msg.ID != contextCollapseStagedMessageID ||
		msg.InternalKind != contextCollapseStagedKind || !msg.HasInternalControlProvenanceForScope(scope, false) {
		return nil, false
	}
	return parseContextCollapseStagedPayload(msg, &scope)
}

// MigrateLegacyContextCollapseStagedMessage is an explicit trusted in-process
// authority. It upgrades a legacy staged record only after a session migrator
// has independently authenticated its source. Runtime projection never calls
// this compatibility path.
func MigrateLegacyContextCollapseStagedMessage(msg types.Message, capabilities ...messagecontrol.Capability) (types.Message, bool) {
	if len(capabilities) != 1 || !capabilities[0].Valid() {
		return msg, false
	}
	capability := capabilities[0]
	if msg.Role != types.RoleUser || msg.InternalKind != "" || msg.ID != "" || msg.IsMeta {
		return msg, false
	}
	if _, ok := parseContextCollapseStagedPayload(msg, nil); !ok {
		return msg, false
	}
	msg.ID = contextCollapseStagedMessageID
	msg.IsMeta = true
	msg.InternalKind = contextCollapseStagedKind
	return msg.WithInternalControlProvenance(capability), true
}

func parseContextCollapseStagedPayload(msg types.Message, expectedScope *messagecontrol.Scope) ([]types.Message, bool) {
	text, ok := singleTextBlock(msg)
	if !ok || text != strings.TrimSpace(text) {
		return nil, false
	}
	if !strings.HasPrefix(text, contextCollapseStagedPrefix) {
		return nil, false
	}
	payload := strings.TrimPrefix(text, contextCollapseStagedPrefix)
	data, err := base64.StdEncoding.Strict().DecodeString(payload)
	if err != nil || base64.StdEncoding.EncodeToString(data) != payload {
		return nil, false
	}
	var decoded contextCollapsePayload
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil || strings.TrimSpace(string(data[decoder.InputOffset():])) != "" {
		return nil, false
	}
	if len(decoded.Messages) == 0 {
		return nil, false
	}
	if expectedScope == nil {
		if decoded.OwnerScopeDigest != "" {
			return nil, false
		}
	} else if decoded.OwnerScopeDigest == "" || decoded.OwnerScopeDigest != contextCollapseScopeDigest(*expectedScope) {
		return nil, false
	}
	previous := -1
	for _, trusted := range decoded.TrustedControls {
		if trusted.Index <= previous || trusted.Index < 0 || trusted.Index >= len(decoded.Messages) || trusted.Kind == "" {
			return nil, false
		}
		if expectedScope == nil {
			// The legacy/unscoped adapter may carry only ordinary messages. A
			// trusted bearer cannot be restored without an exact owner.
			return nil, false
		}
		if trusted.ScopeDigest == "" || trusted.ScopeDigest != contextCollapseScopeDigest(*expectedScope) {
			return nil, false
		}
		previous = trusted.Index
		message := decoded.Messages[trusted.Index]
		if message.InternalKind != trusted.Kind {
			return nil, false
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			return nil, false
		}
		digest := sha256.Sum256(encoded)
		if trusted.Digest != "sha256:"+hex.EncodeToString(digest[:]) {
			return nil, false
		}
		decoded.Messages[trusted.Index] = message.WithInternalControlProvenance(messagecontrol.Runtime(), *expectedScope)
	}
	return decoded.Messages, true
}
