package tui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// SessionIdentity identifies one presentation epoch. Epoch is deliberately
// excluded from persisted observation IDs: resuming the same session must not
// rename its durable history.
type SessionIdentity struct {
	Namespace       string
	SessionID       string
	Epoch           uint64
	controlScope    messagecontrol.Scope
	controlScopeSet bool
}

// WithInternalControlScope binds persisted transcript projection to the
// authoritative manifest scope. Without this private capability, bound bearer
// receipts are treated as ordinary visible data; fresh unbound test/runtime
// descriptors retain the compatibility behavior used before persistence.
func (i SessionIdentity) WithInternalControlScope(capability messagecontrol.Capability, scope messagecontrol.Scope) SessionIdentity {
	i.controlScope = messagecontrol.Scope{}
	i.controlScopeSet = false
	if capability.Valid() && scope.Bound() {
		i.controlScope = scope
		i.controlScopeSet = true
	}
	return i
}

func (i SessionIdentity) isInternalRuntimeMessage(message types.Message) bool {
	if i.controlScopeSet {
		return message.IsInternalRuntimeMessageForScope(i.controlScope, false)
	}
	return message.IsInternalRuntimeMessageForScope(messagecontrol.Scope{}, true)
}

func (i SessionIdentity) isTrustedSkillInvocation(message types.Message) bool {
	if i.controlScopeSet {
		return message.IsTrustedSkillInvocationMessageForScope(i.controlScope, false)
	}
	return message.IsTrustedSkillInvocationMessageForScope(messagecontrol.Scope{}, true)
}

// SessionProjection is a complete, immutable-by-convention presentation of a
// persisted model transcript. Details contains the lossless result evidence
// referenced by Observations.
type SessionProjection struct {
	Messages     []Message
	Observations []Observation
	// Aggregates is nil for legacy deterministic projections. Exact
	// checkpoints always provide a non-nil slice, including when empty, so
	// ApplySessionSnapshot can restore the captured projection without
	// re-running a newer display policy.
	Aggregates []ObservationAggregate
	Details    DetailStore
}

type persistedBlock struct {
	messageIndex int
	blockIndex   int
	message      types.Message
	block        types.ContentBlock
}

// ProjectPersistedMessages deterministically converts model messages into a
// visible transcript and observation graph. Association is based only on an
// exact, unique ToolUseID; message order and adjacency are never correlation
// signals.
func ProjectPersistedMessages(identity SessionIdentity, persisted []types.Message, details DetailStore) (SessionProjection, error) {
	return ProjectPersistedMessagesInLanguage(i18n.DetectOrLoadLanguage(), identity, persisted, details)
}

// ProjectPersistedMessagesInLanguage projects user-visible persisted block
// labels in the active runtime language while preserving raw evidence.
func ProjectPersistedMessagesInLanguage(lang i18n.Language, identity SessionIdentity, persisted []types.Message, details DetailStore) (SessionProjection, error) {
	if details == nil {
		details = NewMemoryDetailStore()
	}

	blocks := flattenPersistedBlocks(identity, persisted)
	callCounts := make(map[string]int)
	calls := make(map[string]types.ToolUseBlock)
	for _, item := range blocks {
		if call, ok := item.block.(types.ToolUseBlock); ok && call.ID != "" {
			callCounts[call.ID]++
			calls[call.ID] = call
		}
	}

	projection := SessionProjection{
		Messages:     make([]Message, 0, len(blocks)),
		Observations: make([]Observation, 0, len(blocks)),
		Details:      details,
	}
	observationIndexes := make(map[string]int)
	messageIndexes := make(map[string]int)
	for _, item := range blocks {
		message, observation, joinsUniqueCall, err := projectPersistedBlock(lang, identity, item, callCounts, calls, details)
		if err != nil {
			return SessionProjection{}, err
		}
		if message != nil {
			if joinsUniqueCall {
				if index, exists := messageIndexes[message.ObservationID]; exists {
					mergePersistedToolMessage(&projection.Messages[index], *message)
				} else {
					messageIndexes[message.ObservationID] = len(projection.Messages)
					projection.Messages = append(projection.Messages, *message)
				}
			} else {
				projection.Messages = append(projection.Messages, *message)
			}
		}
		if observation == nil {
			continue
		}

		if joinsUniqueCall {
			if index, exists := observationIndexes[observation.ID]; exists {
				existing := &projection.Observations[index]
				mergePersistedToolObservation(existing, *observation)
				continue
			}
			observationIndexes[observation.ID] = len(projection.Observations)
		}
		projection.Observations = append(projection.Observations, *observation)
	}

	return projection, nil
}

func flattenPersistedBlocks(identity SessionIdentity, messages []types.Message) []persistedBlock {
	count := 0
	for _, message := range messages {
		if identity.isInternalRuntimeMessage(message) {
			continue
		}
		count += len(message.Content)
	}
	blocks := make([]persistedBlock, 0, count)
	for messageIndex, message := range messages {
		// Developer-role catalog snapshots and deltas are model context, not a
		// human-authored user prompt or assistant reply. Keep them untouched in
		// persisted history while excluding them from transcript projection.
		// Failing closed for every developer message also prevents malformed or
		// legacy metadata from leaking internal instructions to the user.
		if identity.isInternalRuntimeMessage(message) {
			continue
		}
		for blockIndex, block := range message.Content {
			blocks = append(blocks, persistedBlock{
				messageIndex: messageIndex,
				blockIndex:   blockIndex,
				message:      message,
				block:        block,
			})
		}
	}
	return blocks
}

func projectPersistedBlock(
	lang i18n.Language,
	identity SessionIdentity,
	item persistedBlock,
	callCounts map[string]int,
	calls map[string]types.ToolUseBlock,
	details DetailStore,
) (*Message, *Observation, bool, error) {
	fallbackID := persistedObservationID(identity, item.message, item.messageIndex, item.blockIndex)
	baseObservation := Observation{
		ID:         fallbackID,
		SessionID:  identity.SessionID,
		Disclosure: DisclosureState{Level: DisclosureSummary},
	}

	switch block := item.block.(type) {
	case types.TextBlock:
		kind := MsgAssistant
		text := block.Text
		if item.message.Role != types.RoleAssistant {
			kind = MsgUser
			if identity.isTrustedSkillInvocation(item.message) {
				command, ok := SkillInvocationTranscriptCommand(block.Text)
				if !ok {
					return &Message{Kind: kind, Text: text, ObservationID: fallbackID}, &baseObservation, false, nil
				}
				text = command
			}
		}
		return &Message{Kind: kind, Text: text, ObservationID: fallbackID}, &baseObservation, false, nil

	case types.ThinkingBlock:
		baseObservation.Disclosure = DisclosureState{Level: DisclosureSummary, HasMore: block.Thinking != ""}
		return &Message{
			Kind:          MsgAssistantThinking,
			Text:          block.Thinking,
			Collapsed:     true,
			ObservationID: fallbackID,
		}, &baseObservation, false, nil

	case types.ToolUseBlock:
		observation := baseObservation
		observation.ToolUseID = block.ID
		observation.ToolName = block.Name
		observation.ToolInput = cloneStringAnyMap(block.Input)
		observation.Outcome = OutcomeRunning

		joinsUniqueCall := block.ID != "" && callCounts[block.ID] == 1
		if joinsUniqueCall {
			observation.ID = persistedToolObservationID(identity, block.ID)
		} else if block.ID == "" {
			observation.Outcome = OutcomeOrphan
			observation.Disclosure.Level = DisclosureDetail
		} else {
			observation.Outcome = OutcomeConflict
			observation.Disclosure.Level = DisclosureDetail
		}
		applyObservationPresentation(&observation, nil)
		return &Message{
			Kind:          MsgToolCall,
			Text:          observation.Presentation.Summary,
			ToolName:      block.Name,
			Input:         cloneStringAnyMap(block.Input),
			ToolUseID:     block.ID,
			ObservationID: observation.ID,
			Outcome:       observation.Outcome,
			Disclosure:    observation.Disclosure,
		}, &observation, joinsUniqueCall, nil

	case types.ToolResultBlock:
		text := block.TextContent()
		joinsUniqueCall := block.ToolUseID != "" && callCounts[block.ToolUseID] == 1
		observation := baseObservation
		observation.ToolUseID = block.ToolUseID
		if joinsUniqueCall {
			call := calls[block.ToolUseID]
			observation.ToolName = call.Name
			observation.ToolInput = cloneStringAnyMap(call.Input)
		}
		observation.Outcome = observationOutcomeForResult(block)
		if joinsUniqueCall {
			observation.ID = persistedToolObservationID(identity, block.ToolUseID)
		} else if block.ToolUseID == "" || callCounts[block.ToolUseID] == 0 {
			observation.Outcome = OutcomeOrphan
		} else {
			observation.Outcome = OutcomeConflict
		}

		ref, err := details.Put(persistedResultDetailKey(observation.ID, item.messageIndex, item.blockIndex), []byte(text))
		if err != nil {
			return nil, nil, false, i18n.WrapError(i18n.KeyTUISessionProjectionRetainToolResult, err, item.messageIndex, item.blockIndex)
		}
		observation.ResultRefs = []DetailRef{ref}
		if text != "" && toolResultCanRetainFullEvidence(block) {
			observation.FullEvidenceResult = 1
		}
		if hasStructuredToolResultEvidence(block) {
			envelope, marshalErr := marshalToolResultEvidence(block)
			if marshalErr != nil {
				return nil, nil, false, i18n.WrapError(i18n.KeyTUISessionProjectionEncodeToolResult, marshalErr, item.messageIndex, item.blockIndex)
			}
			envelopeRef, putErr := details.Put(persistedResultDetailKey(observation.ID+":envelope", item.messageIndex, item.blockIndex), envelope)
			if putErr != nil {
				return nil, nil, false, i18n.WrapError(i18n.KeyTUISessionProjectionRetainStructuredToolResult, putErr, item.messageIndex, item.blockIndex)
			}
			observation.EnvelopeRefs = []DetailRef{envelopeRef}
		}
		observation.Disclosure = defaultResultDisclosure(observation.Outcome, block.IsError, true)
		applyObservationPresentation(&observation, &block)

		// This lookup is intentionally independent of call/result order.
		call := calls[block.ToolUseID]
		toolName := call.Name
		var toolInput map[string]any
		if !joinsUniqueCall {
			toolName = ""
		} else {
			toolInput = cloneStringAnyMap(call.Input)
		}
		kind := MsgToolResult
		if joinsUniqueCall {
			kind = MsgToolCall
		}
		return &Message{
			Kind:          kind,
			Text:          text,
			ToolName:      toolName,
			Input:         toolInput,
			ToolUseID:     block.ToolUseID,
			ObservationID: observation.ID,
			DetailRefs:    []DetailRef{ref},
			IsError:       block.IsError,
			Collapsed:     !block.IsError,
			Outcome:       observation.Outcome,
			Disclosure:    observation.Disclosure,
		}, &observation, joinsUniqueCall, nil

	case types.ImageBlock:
		return projectPersistedEvidenceBlock(baseObservation, details, item, i18n.Text(lang, i18n.KeyRuntimePersistedImage), block)
	case types.DocumentBlock:
		return projectPersistedEvidenceBlock(baseObservation, details, item, i18n.Text(lang, i18n.KeyRuntimePersistedDocument), block)
	case types.ToolReferenceBlock:
		return projectPersistedEvidenceBlock(baseObservation, details, item, i18n.Format(lang, i18n.KeyRuntimePersistedTool, block.ToolName), block)
	case types.ContentReplacementBlock:
		return projectPersistedEvidenceBlock(baseObservation, details, item, i18n.Text(lang, i18n.KeyRuntimeContentReplacement), block)
	case types.UnknownBlock:
		evidence := []byte(block.Raw)
		if len(evidence) == 0 {
			evidence, _ = json.Marshal(block)
		}
		return projectPersistedEvidence(baseObservation, details, item, "["+string(block.Type)+"]", evidence)
	}

	return nil, nil, false, nil
}

type persistedSkillInvocationWire struct {
	Type    string                        `json:"type"`
	Version int                           `json:"version"`
	Kind    skills.InvocationEnvelopeKind `json:"kind"`
	Skill   struct {
		ID       skills.SkillID       `json:"id"`
		Name     string               `json:"name"`
		Revision skills.SkillRevision `json:"revision"`
		Digest   skills.SkillDigest   `json:"digest"`
		Source   skills.SkillSource   `json:"source"`
		Locator  skills.SkillLocator  `json:"locator"`
	} `json:"skill"`
	Arguments      skills.InvocationArguments     `json:"arguments"`
	PayloadDigest  skills.InvocationPayloadDigest `json:"payload_digest"`
	PreviousDigest skills.SkillDigest             `json:"previous_digest,omitempty"`
	Body           *string                        `json:"body,omitempty"`
}

// SkillInvocationTranscriptCommand recognizes only a canonical, internally
// generated invocation envelope. It replaces the model-only body with a safe
// user-command projection after resume; malformed lookalikes remain ordinary
// user text so arbitrary JSON cannot disappear from the transcript.
// Other terminal surfaces use the same parser to avoid printing SKILL.md while
// still showing the user's command-shaped receipt.
func SkillInvocationTranscriptCommand(text string) (string, bool) {
	decoder := json.NewDecoder(bytes.NewBufferString(text))
	decoder.DisallowUnknownFields()
	var wire persistedSkillInvocationWire
	if err := decoder.Decode(&wire); err != nil {
		return "", false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", false
	}
	if wire.Type != "skill_invocation" || wire.Version != skills.InvocationEnvelopeVersion ||
		wire.Skill.ID.Validate() != nil || strings.TrimSpace(wire.Skill.Name) == "" || strings.TrimSpace(wire.Skill.Name) != wire.Skill.Name ||
		strings.IndexFunc(wire.Skill.Name, unicode.IsControl) >= 0 ||
		wire.Skill.Revision.Validate() != nil || wire.Skill.Digest.Validate() != nil ||
		wire.Skill.Source.Validate() != nil || wire.Skill.Locator.Validate() != nil ||
		wire.PayloadDigest.Validate() != nil || (!wire.Arguments.Provided && wire.Arguments.Value != "") {
		return "", false
	}
	encodedSource, ok := wire.Skill.ID.Source()
	if !ok || encodedSource != wire.Skill.Source {
		return "", false
	}
	computedID, err := skills.ComputeSkillID(wire.Skill.Source, wire.Skill.Locator)
	if err != nil || computedID != wire.Skill.ID {
		return "", false
	}
	switch wire.Kind {
	case skills.InvocationEnvelopeFull:
		if wire.Body == nil || wire.PreviousDigest != "" {
			return "", false
		}
	case skills.InvocationEnvelopeAlreadyLoaded:
		if wire.Body != nil || wire.PreviousDigest != "" {
			return "", false
		}
	case skills.InvocationEnvelopeSuperseding:
		if wire.Body == nil || wire.PreviousDigest.Validate() != nil || wire.PreviousDigest == wire.Skill.Digest {
			return "", false
		}
	default:
		return "", false
	}
	if wire.Body != nil && skills.DigestInvocationPayload(*wire.Body) != wire.PayloadDigest {
		return "", false
	}
	canonical, err := json.Marshal(wire)
	if err != nil || string(canonical) != text {
		return "", false
	}

	command := "/" + wire.Skill.Name
	if wire.Arguments.Provided {
		if wire.Arguments.Value == "" {
			return command + ` ""`, true
		}
		command += " " + wire.Arguments.Value
	}
	return command, true
}

func projectPersistedEvidenceBlock(base Observation, details DetailStore, item persistedBlock, label string, block any) (*Message, *Observation, bool, error) {
	evidence, err := json.Marshal(block)
	if err != nil {
		return nil, nil, false, i18n.WrapError(i18n.KeyTUISessionProjectionEncodeBlock, err, item.messageIndex, item.blockIndex)
	}
	return projectPersistedEvidence(base, details, item, label, evidence)
}

func projectPersistedEvidence(base Observation, details DetailStore, item persistedBlock, label string, evidence []byte) (*Message, *Observation, bool, error) {
	ref, err := details.Put(persistedResultDetailKey(base.ID, item.messageIndex, item.blockIndex), evidence)
	if err != nil {
		return nil, nil, false, i18n.WrapError(i18n.KeyTUISessionProjectionRetainBlock, err, item.messageIndex, item.blockIndex)
	}
	base.ResultRefs = []DetailRef{ref}
	base.Disclosure = DisclosureState{Level: DisclosureSummary, HasMore: true}
	return &Message{
		Kind: MsgInfo, Text: label, ObservationID: base.ID,
		DetailRefs: []DetailRef{ref}, Disclosure: base.Disclosure,
	}, &base, false, nil
}

func mergePersistedToolMessage(existing *Message, incoming Message) {
	existing.Kind = MsgToolCall
	existing.ToolUseID = incoming.ToolUseID
	existing.ObservationID = incoming.ObservationID
	if incoming.ToolName != "" {
		existing.ToolName = incoming.ToolName
	}
	if incoming.Input != nil {
		existing.Input = cloneStringAnyMap(incoming.Input)
	}
	if len(incoming.DetailRefs) == 0 {
		return
	}
	existing.Text = incoming.Text
	existing.DetailRefs = append(existing.DetailRefs, incoming.DetailRefs...)
	existing.IsError = incoming.IsError
	existing.Collapsed = incoming.Collapsed
	existing.Outcome = incoming.Outcome
	existing.Disclosure = incoming.Disclosure
}

func mergePersistedToolObservation(existing *Observation, incoming Observation) {
	if incoming.ToolName != "" {
		existing.ToolName = incoming.ToolName
		existing.ToolInput = cloneStringAnyMap(incoming.ToolInput)
	}
	if len(incoming.ResultRefs) == 0 {
		return
	}
	resultOffset := len(existing.ResultRefs)
	existing.ResultRefs = append(existing.ResultRefs, incoming.ResultRefs...)
	existing.FullEvidenceResult = 0
	if incoming.FullEvidenceResult > 0 && incoming.FullEvidenceResult <= len(incoming.ResultRefs) {
		existing.FullEvidenceResult = resultOffset + incoming.FullEvidenceResult
	}
	existing.EnvelopeRefs = append(existing.EnvelopeRefs, incoming.EnvelopeRefs...)
	existing.Outcome = incoming.Outcome
	existing.Disclosure = incoming.Disclosure
	existing.Presentation = incoming.Presentation
	existing.Decision = incoming.Decision
}

func persistedToolObservationID(identity SessionIdentity, toolUseID string) string {
	return toolObservationID(identity.SessionID, toolUseID)
}

func persistedObservationID(identity SessionIdentity, message types.Message, messageIndex, blockIndex int) string {
	localID := message.ID
	if localID == "" {
		localID = fmt.Sprintf("legacy-message:%d:block:%d", messageIndex, blockIndex)
	} else {
		localID = fmt.Sprintf("message:%s:occurrence:%d:block:%d", message.ID, messageIndex, blockIndex)
	}
	return stableProjectionID(identity, "observation", localID)
}

func persistedResultDetailKey(observationID string, messageIndex, blockIndex int) string {
	return fmt.Sprintf("%s:persisted-result:%d:%d", observationID, messageIndex, blockIndex)
}

func stableProjectionID(identity SessionIdentity, kind, localID string) string {
	material := fmt.Sprintf("v1\x00%d:%s\x00%d:%s\x00%d:%s\x00%d:%s", len(identity.Namespace), identity.Namespace, len(identity.SessionID), identity.SessionID, len(kind), kind, len(localID), localID)
	digest := sha256.Sum256([]byte(material))
	return kind + ":" + hex.EncodeToString(digest[:16])
}
