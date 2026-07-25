package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

var (
	// ErrObservationNotFound indicates that a disclosure update targeted an
	// observation that is not present in this store.
	ErrObservationNotFound = i18n.NewError(i18n.KeyTUIObservationStoreNotFound)
	// ErrMissingToolUseID indicates a malformed tool event. The event is still
	// retained as an independent orphan observation.
	ErrMissingToolUseID = i18n.NewError(i18n.KeyTUIObservationMissingToolUseID)
	// ErrToolUseIDConflict indicates that a result cannot be correlated to one
	// unique call without guessing.
	ErrToolUseIDConflict = i18n.NewError(i18n.KeyTUIObservationToolUseIDConflict)
)

// ObservationOutcome is a deterministic terminal or in-progress state. It is
// supplied by the runtime rather than inferred from result prose.
type ObservationOutcome uint8

const (
	OutcomeUnknown ObservationOutcome = iota
	OutcomeRunning
	OutcomeSucceeded
	OutcomeFailed
	OutcomePartial
	OutcomeDenied
	OutcomeCancelled
	OutcomeTimedOut
	OutcomeEscaped
	OutcomeShutdown
	OutcomeOrphan
	OutcomeConflict
)

// String returns the stable session representation for an observation outcome.
func (o ObservationOutcome) String() string {
	switch o {
	case OutcomeRunning:
		return "running"
	case OutcomeSucceeded:
		return "succeeded"
	case OutcomeFailed:
		return "failed"
	case OutcomePartial:
		return "partial"
	case OutcomeDenied:
		return "denied"
	case OutcomeCancelled:
		return "cancelled"
	case OutcomeTimedOut:
		return "timed_out"
	case OutcomeEscaped:
		return "escaped"
	case OutcomeShutdown:
		return "shutdown"
	case OutcomeOrphan:
		return "orphan"
	case OutcomeConflict:
		return "conflict"
	default:
		return "unknown"
	}
}

// DisclosureLevel controls how much of one observation is projected.
type DisclosureLevel uint8

const (
	DisclosureSummary DisclosureLevel = iota
	DisclosureDetail
	DisclosureEvidence
)

// DisclosureState is owned per observation. Global transcript visibility is a
// separate concern and must not overwrite this state.
type DisclosureState struct {
	Level      DisclosureLevel
	HasMore    bool
	UserPinned bool
}

// ToolEventContext carries stable runtime correlation and structured outcome
// fields into the presentation reducer.
type ToolEventContext struct {
	SessionID   string
	TurnID      string
	WorkUnitID  string
	ActorID     string
	ActorType   string
	Outcome     ObservationOutcome
	Language    i18n.Language
	LanguageSet bool
}

// Observation joins one tool call with all results that identify that exact
// call. ResultRefs point to immutable, lossless evidence.
type Observation struct {
	ID         string
	SessionID  string
	TurnID     string
	WorkUnitID string
	ActorID    string
	// Presentation* preserves the immutable labels shown for historical
	// evidence after a fork remaps the operational identity used by lookups and
	// actions. Empty values mean the operational value is already canonical.
	PresentationID         string `json:"presentation_id,omitempty"`
	PresentationWorkUnitID string `json:"presentation_work_unit_id,omitempty"`
	PresentationActorID    string `json:"presentation_actor_id,omitempty"`

	ToolUseID string
	ToolName  string
	ToolInput map[string]any

	Outcome    ObservationOutcome
	Disclosure DisclosureState
	// Presentation is the immutable semantic projection extracted while the
	// complete ToolResultBlock is still available. Decision records why the
	// default disclosure was selected; renderers consume it without re-parsing
	// result prose.
	Presentation FormattedPresentation  `json:"presentation,omitempty"`
	Decision     PresentationDecision   `json:"presentation_decision,omitempty"`
	Aggregation  ObservationAggregation `json:"aggregation,omitempty"`
	ResultRefs   []DetailRef
	// FullEvidenceResult is a 1-based index into ResultRefs. Zero means no
	// retained result is known to contain the complete source result.
	FullEvidenceResult int `json:"full_evidence_result,omitempty"`
	// EnvelopeRefs retain the complete structured ToolResultBlock separately
	// from its human-readable text projection.
	EnvelopeRefs []DetailRef
}

// ObservationStore normalizes tool events without relying on message order or
// adjacency. All public methods are safe for concurrent use.
type ObservationStore struct {
	mu      sync.RWMutex
	details DetailStore

	observations              []Observation
	byID                      map[string]int
	callCounts                map[string]int
	aggregates                map[string]*ObservationAggregate
	aggregateKeyByObservation map[string]string
	pinned                    map[string]struct{}
	pinnedOrder               []string
	nextEvent                 uint64
}

// NewObservationStore creates an empty observation store backed by details.
func NewObservationStore(details DetailStore) *ObservationStore {
	if details == nil {
		details = NewMemoryDetailStore()
	}
	return &ObservationStore{
		details:                   details,
		byID:                      make(map[string]int),
		callCounts:                make(map[string]int),
		aggregates:                make(map[string]*ObservationAggregate),
		aggregateKeyByObservation: make(map[string]string),
		pinned:                    make(map[string]struct{}),
	}
}

// ApplyToolCall records a call. A non-empty ToolUseID is unique within its
// session; duplicates are retained as explicit conflicts and return an error.
func (s *ObservationStore) ApplyToolCall(ctx ToolEventContext, call types.ToolUseBlock) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sequence := s.allocateEventLocked()
	if call.ID == "" {
		observation := Observation{
			ID:         orphanObservationID(ctx.SessionID, "call", sequence),
			SessionID:  ctx.SessionID,
			TurnID:     ctx.TurnID,
			WorkUnitID: ctx.WorkUnitID,
			ActorID:    ctx.ActorID,
			ToolName:   call.Name,
			ToolInput:  cloneStringAnyMap(call.Input),
			Outcome:    OutcomeOrphan,
			Disclosure: DisclosureState{Level: DisclosureDetail},
		}
		applyObservationPresentationInLanguage(&observation, nil, toolEventLanguage(ctx))
		s.appendLocked(observation)
		return i18n.WrapError(i18n.KeyTUIObservationToolCallMissingID, ErrMissingToolUseID, call.Name)
	}

	stableID := toolObservationID(ctx.SessionID, call.ID)
	if s.callCounts[stableID] != 0 {
		s.callCounts[stableID]++
		observation := Observation{
			ID:         conflictObservationID(stableID, sequence),
			SessionID:  ctx.SessionID,
			TurnID:     ctx.TurnID,
			WorkUnitID: ctx.WorkUnitID,
			ActorID:    ctx.ActorID,
			ToolUseID:  call.ID,
			ToolName:   call.Name,
			ToolInput:  cloneStringAnyMap(call.Input),
			Outcome:    OutcomeConflict,
			Disclosure: DisclosureState{Level: DisclosureDetail},
		}
		applyObservationPresentationInLanguage(&observation, nil, toolEventLanguage(ctx))
		s.appendLocked(observation)
		return i18n.WrapError(i18n.KeyTUIObservationToolCallIDConflict, ErrToolUseIDConflict, call.Name, call.ID)
	}

	s.callCounts[stableID] = 1
	observation := Observation{
		ID:         stableID,
		SessionID:  ctx.SessionID,
		TurnID:     ctx.TurnID,
		WorkUnitID: ctx.WorkUnitID,
		ActorID:    ctx.ActorID,
		ToolUseID:  call.ID,
		ToolName:   call.Name,
		ToolInput:  cloneStringAnyMap(call.Input),
		Outcome:    OutcomeRunning,
		Disclosure: DisclosureState{Level: DisclosureSummary},
	}
	applyObservationPresentationInLanguage(&observation, nil, toolEventLanguage(ctx))
	s.appendLocked(observation)
	return nil
}

// ApplyToolResult stores exact evidence before projecting the result. A result
// only updates a call when its session and ToolUseID identify exactly one call.
func (s *ObservationStore) ApplyToolResult(ctx ToolEventContext, result types.ToolResultBlock) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sequence := s.allocateEventLocked()
	stableID := toolObservationID(ctx.SessionID, result.ToolUseID)
	evidence := result.Content
	if evidence == "" {
		evidence = result.TextContent()
	}
	hasEvidence := len(evidence) > 0
	detailRef, err := s.details.Put(resultDetailKey(stableID, sequence), []byte(evidence))
	if err != nil {
		return i18n.WrapError(i18n.KeyTUIObservationRetainResultEvidence, err)
	}
	var envelopeRef DetailRef
	var envelopeErr error
	if hasStructuredToolResultEvidence(result) {
		envelope, marshalErr := marshalToolResultEvidence(result)
		if marshalErr != nil {
			envelopeErr = i18n.WrapError(i18n.KeyTUIObservationEncodeStructuredResult, marshalErr)
		} else {
			envelopeRef, err = s.details.Put(resultEnvelopeKey(stableID, sequence), envelope)
			if err != nil {
				envelopeErr = i18n.WrapError(i18n.KeyTUIObservationRetainStructuredResult, err)
			}
		}
	}
	envelopeRefs := nonZeroDetailRefs(envelopeRef)

	if result.ToolUseID == "" {
		observation := Observation{
			ID:           orphanObservationID(ctx.SessionID, "result", sequence),
			SessionID:    ctx.SessionID,
			TurnID:       ctx.TurnID,
			WorkUnitID:   ctx.WorkUnitID,
			ActorID:      ctx.ActorID,
			Outcome:      OutcomeOrphan,
			Disclosure:   DisclosureState{Level: DisclosureDetail, HasMore: hasEvidence || len(envelopeRefs) > 0},
			ResultRefs:   []DetailRef{detailRef},
			EnvelopeRefs: envelopeRefs,
		}
		if hasEvidence && toolResultCanRetainFullEvidence(result) {
			observation.FullEvidenceResult = 1
		}
		applyObservationPresentationInLanguage(&observation, &result, toolEventLanguage(ctx))
		s.appendLocked(observation)
		journalErr := s.journalObservationLocked(observation)
		if envelopeErr != nil {
			return errors.Join(envelopeErr, journalErr)
		}
		if journalErr != nil {
			return journalErr
		}
		return i18n.WrapError(i18n.KeyTUIObservationToolResultMissingID, ErrMissingToolUseID)
	}

	callCount := s.callCounts[stableID]
	index, exists := s.byID[stableID]
	if callCount != 1 || !exists {
		outcome := OutcomeOrphan
		cause := ErrObservationNotFound
		if callCount > 1 {
			outcome = OutcomeConflict
			cause = ErrToolUseIDConflict
		}
		observation := Observation{
			ID:           conflictObservationID(stableID, sequence),
			SessionID:    ctx.SessionID,
			TurnID:       ctx.TurnID,
			WorkUnitID:   ctx.WorkUnitID,
			ActorID:      ctx.ActorID,
			ToolUseID:    result.ToolUseID,
			Outcome:      outcome,
			Disclosure:   DisclosureState{Level: DisclosureDetail, HasMore: hasEvidence || len(envelopeRefs) > 0},
			ResultRefs:   []DetailRef{detailRef},
			EnvelopeRefs: envelopeRefs,
		}
		if hasEvidence && toolResultCanRetainFullEvidence(result) {
			observation.FullEvidenceResult = 1
		}
		applyObservationPresentationInLanguage(&observation, &result, toolEventLanguage(ctx))
		s.appendLocked(observation)
		journalErr := s.journalObservationLocked(observation)
		if envelopeErr != nil {
			return errors.Join(envelopeErr, journalErr)
		}
		if journalErr != nil {
			return journalErr
		}
		return i18n.WrapError(i18n.KeyTUIObservationToolResultMatchCount, cause, result.ToolUseID, callCount)
	}

	observation := &s.observations[index]
	observation.ResultRefs = append(observation.ResultRefs, detailRef)
	observation.FullEvidenceResult = 0
	if hasEvidence && toolResultCanRetainFullEvidence(result) {
		observation.FullEvidenceResult = len(observation.ResultRefs)
	}
	observation.EnvelopeRefs = append(observation.EnvelopeRefs, envelopeRefs...)
	observation.Outcome = ctx.Outcome
	observation.Disclosure.HasMore = observation.Disclosure.HasMore || hasEvidence || len(envelopeRefs) > 0
	applyObservationPresentationInLanguage(observation, &result, toolEventLanguage(ctx))
	s.updateObservationAggregateLocked(index)
	return errors.Join(envelopeErr, s.journalObservationLocked(*observation))
}

// SetDisclosure updates one observation without affecting sibling or global
// transcript disclosure state.
func (s *ObservationStore) SetDisclosure(id string, disclosure DisclosureState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("%s: %w", id, ErrObservationNotFound)
	}
	s.observations[index].Disclosure = disclosure
	s.observations[index].Decision.EffectiveLevel = presentationLevelForDisclosure(disclosure.Level)
	if disclosure.UserPinned && disclosure.Level == DisclosureEvidence {
		s.observations[index].Decision.AggregationEligible = false
		s.observations[index].Decision.Reasons = appendPresentationReason(s.observations[index].Decision.Reasons, ReasonPinnedEvidence)
	}
	s.updateObservationAggregateLocked(index)
	if disclosure.UserPinned {
		if _, exists := s.pinned[id]; !exists {
			s.pinned[id] = struct{}{}
			s.pinnedOrder = append(s.pinnedOrder, id)
		}
	} else {
		delete(s.pinned, id)
		for i := range s.pinnedOrder {
			if s.pinnedOrder[i] == id {
				s.pinnedOrder = append(s.pinnedOrder[:i], s.pinnedOrder[i+1:]...)
				break
			}
		}
	}
	return s.journalObservationLocked(s.observations[index])
}

// UpsertEvidenceObservation retains evidence produced outside the foreground
// tool stream (for example a background task) under a stable observation ID.
func (s *ObservationStore) UpsertEvidenceObservation(observation Observation, ref DetailRef) error {
	if strings.TrimSpace(observation.ID) == "" {
		return fmt.Errorf("evidence observation has empty ID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if index, ok := s.byID[observation.ID]; ok {
		existing := &s.observations[index]
		existing.Outcome = observation.Outcome
		existing.SessionID = observation.SessionID
		existing.TurnID = observation.TurnID
		existing.WorkUnitID = observation.WorkUnitID
		existing.ActorID = observation.ActorID
		existing.ToolName = observation.ToolName
		for _, current := range existing.ResultRefs {
			if current == ref {
				return s.journalObservationLocked(*existing)
			}
		}
		existing.ResultRefs = append(existing.ResultRefs, ref)
		existing.Disclosure.HasMore = true
		applyObservationPresentation(existing, nil)
		return s.journalObservationLocked(*existing)
	}
	observation.ResultRefs = append([]DetailRef(nil), ref)
	observation.Disclosure.HasMore = false
	applyObservationPresentation(&observation, nil)
	s.appendLocked(observation)
	return s.journalObservationLocked(observation)
}

// AttachResultRef adds retained evidence to an existing observation without
// recomputing its typed presentation. Agent evidence remains available to
// internal audit storage but does not advertise a user-facing detail view.
func (s *ObservationStore) AttachResultRef(id string, ref DetailRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("%s: %w", id, ErrObservationNotFound)
	}
	observation := &s.observations[index]
	for _, current := range observation.ResultRefs {
		if current == ref {
			return nil
		}
	}
	observation.ResultRefs = append(observation.ResultRefs, ref)
	observation.Disclosure.HasMore = !strings.EqualFold(strings.TrimSpace(observation.ToolName), "Agent")
	return s.journalObservationLocked(*observation)
}

// UpdatePresentationLifecycle projects the richer activity lifecycle onto the
// matching tool row without changing its execution outcome or retained result.
func (s *ObservationStore) UpdatePresentationLifecycle(id string, lifecycle PresentationLifecycleState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, ok := s.byID[id]
	if !ok || lifecycle == "" {
		return false
	}
	observation := &s.observations[index]
	if observation.Presentation.Lifecycle == lifecycle {
		return false
	}
	observation.Presentation.Lifecycle = lifecycle
	return true
}

// UpdateAgentResultPreview replaces an async launch projection with the
// complete typed conclusion while keeping the immutable raw evidence in the
// detail store. This lets a detached Agent finish in the original card without
// copying its provider control envelope into the transcript.
func (s *ObservationStore) UpdateAgentResultPreview(id string, lang i18n.Language, result string, outcomes ...ObservationOutcome) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("%s: %w", id, ErrObservationNotFound)
	}
	observation := &s.observations[index]
	if !strings.EqualFold(strings.TrimSpace(observation.ToolName), "Agent") {
		return nil
	}
	data := map[string]any{"content": []any{map[string]any{"type": "text", "text": result}}}
	conclusion := agentPresentationResultText(data)
	if conclusion == "" {
		return nil
	}
	outcome := OutcomeSucceeded
	if len(outcomes) > 0 && outcomes[0] != OutcomeUnknown && outcomes[0] != OutcomeRunning {
		outcome = outcomes[0]
	}
	observation.Outcome = outcome
	observation.Presentation.Family = FamilyAgent
	observation.Presentation.Outcome = outcome
	observation.Presentation.Lifecycle = PresentationLifecycleCompleted
	observation.Presentation.TerminalAgent = true
	observation.Presentation.HasMore = false
	line := i18n.Format(lang, i18n.KeyPresentationResultValue, conclusion)
	if outcome != OutcomeSucceeded {
		line = i18n.Format(lang, i18n.KeyPresentationCause, conclusion)
	}
	observation.Presentation.DetailLines = []string{line}
	observation.Disclosure.HasMore = false
	return s.journalObservationLocked(*observation)
}

// Get returns a deep copy of one observation.
func (s *ObservationStore) Get(id string) (Observation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, ok := s.byID[id]
	if !ok {
		return Observation{}, false
	}
	return cloneObservation(s.observations[index]), true
}

// Snapshot returns observations in first-seen order as independent copies.
func (s *ObservationStore) Snapshot() []Observation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := make([]Observation, len(s.observations))
	for i := range s.observations {
		snapshot[i] = cloneObservation(s.observations[i])
	}
	return snapshot
}

func (s *ObservationStore) PinnedSnapshot() []Observation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Observation, 0, len(s.pinned))
	for _, id := range s.pinnedOrder {
		if _, pinned := s.pinned[id]; !pinned {
			continue
		}
		if index, ok := s.byID[id]; ok {
			out = append(out, cloneObservation(s.observations[index]))
		}
	}
	return out
}

func (s *ObservationStore) appendLocked(observation Observation) {
	s.byID[observation.ID] = len(s.observations)
	s.observations = append(s.observations, observation)
	if observation.Disclosure.UserPinned {
		if _, exists := s.pinned[observation.ID]; !exists {
			s.pinned[observation.ID] = struct{}{}
			s.pinnedOrder = append(s.pinnedOrder, observation.ID)
		}
	}
}

func (s *ObservationStore) journalObservationLocked(observation Observation) error {
	journal, ok := s.details.(ObservationEvidenceJournal)
	if !ok || (len(observation.ResultRefs) == 0 && len(observation.EnvelopeRefs) == 0) {
		return nil
	}
	if err := journal.SaveObservationEvidence(resetProcessLocalObservationDisclosure(cloneObservation(observation))); err != nil {
		return i18n.WrapError(i18n.KeyTUIObservationRetainEvidenceIndex, err)
	}
	return nil
}

// resetProcessLocalObservationDisclosure removes UI-only reveal state before
// an observation crosses a persistence boundary. Evidence remains reachable
// through immutable refs, but resume and fork require a fresh explicit reveal.
func resetProcessLocalObservationDisclosure(observation Observation) Observation {
	if observation.Disclosure.Level == DisclosureEvidence {
		observation.Disclosure.Level = DisclosureSummary
	}
	observation.Disclosure.UserPinned = false
	return observation
}

func (s *ObservationStore) allocateEventLocked() uint64 {
	s.nextEvent++
	return s.nextEvent
}

func toolObservationID(sessionID, toolUseID string) string {
	return fmt.Sprintf("tool:%d:%s:%d:%s", len(sessionID), sessionID, len(toolUseID), toolUseID)
}

func orphanObservationID(sessionID, kind string, sequence uint64) string {
	return fmt.Sprintf("orphan:%d:%s:%s:%d", len(sessionID), sessionID, kind, sequence)
}

func conflictObservationID(stableID string, sequence uint64) string {
	return fmt.Sprintf("%s:conflict:%d", stableID, sequence)
}

func resultDetailKey(stableID string, sequence uint64) string {
	return fmt.Sprintf("%s:result:%d", stableID, sequence)
}

func resultEnvelopeKey(stableID string, sequence uint64) string {
	return fmt.Sprintf("%s:result-envelope:%d", stableID, sequence)
}

func marshalToolResultEvidence(result types.ToolResultBlock) ([]byte, error) {
	visibleNewMessages := userVisibleResultMessages(result.NewMessages)
	return json.Marshal(struct {
		Type          types.ContentType            `json:"type"`
		ToolUseID     string                       `json:"tool_use_id"`
		Content       string                       `json:"content"`
		ContentBlocks []types.ContentBlock         `json:"content_blocks,omitempty"`
		Data          any                          `json:"data,omitempty"`
		IsError       bool                         `json:"is_error"`
		NewMessages   []types.Message              `json:"new_messages,omitempty"`
		Metadata      map[string]string            `json:"metadata,omitempty"`
		Usage         *types.Usage                 `json:"usage,omitempty"`
		Outcome       types.ToolOutcome            `json:"outcome,omitempty"`
		Completeness  types.ToolResultCompleteness `json:"completeness,omitempty"`
	}{
		Type: result.Type, ToolUseID: result.ToolUseID, Content: result.Content,
		ContentBlocks: result.ContentBlocks, Data: jsonSafeEvidenceValue(result.Data),
		IsError: result.IsError, NewMessages: visibleNewMessages, Metadata: result.Metadata,
		Usage: result.Usage, Outcome: result.Outcome, Completeness: result.Completeness,
	})
}

func hasStructuredToolResultEvidence(result types.ToolResultBlock) bool {
	return len(result.ContentBlocks) > 0 || result.Data != nil || len(userVisibleResultMessages(result.NewMessages)) > 0 ||
		len(result.Metadata) > 0 || result.Usage != nil || !result.Completeness.IsZero()
}

func toolResultCanRetainFullEvidence(result types.ToolResultBlock) bool {
	return result.Completeness.CanRetainFullEvidence() && len(result.ContentBlocks) == 0 && len(userVisibleResultMessages(result.NewMessages)) == 0
}

func userVisibleResultMessages(messages []types.Message) []types.Message {
	// This evidence path has no authoritative session/loop scope. Treat every
	// attachment as visible ordinary evidence rather than elevating a copied
	// process-HMAC bearer into permission to hide data.
	return append([]types.Message(nil), messages...)
}

func nonZeroDetailRefs(ref DetailRef) []DetailRef {
	if ref.Source == "" {
		return nil
	}
	return []DetailRef{ref}
}

func jsonSafeEvidenceValue(value any) any {
	if value == nil {
		return nil
	}
	if _, err := json.Marshal(value); err == nil {
		return value
	}
	return fmt.Sprintf("%#v", value)
}

func defaultResultDisclosure(outcome ObservationOutcome, isError, hasEvidence bool) DisclosureState {
	if isError && (outcome == OutcomeUnknown || outcome == OutcomeSucceeded) {
		outcome = OutcomeFailed
	}
	decision := DecidePresentation(PresentationFacts{Family: FamilyUnknown, Outcome: outcome, Risk: RiskUnknown, HasEvidence: hasEvidence})
	return DisclosureState{Level: decision.DisclosureLevel(), HasMore: hasEvidence}
}

func cloneObservation(observation Observation) Observation {
	observation.ToolInput = cloneStringAnyMap(observation.ToolInput)
	observation.Presentation.Completeness = observation.Presentation.Completeness.Clone()
	observation.Presentation.DetailLines = append([]string(nil), observation.Presentation.DetailLines...)
	observation.Decision.Reasons = append([]PresentationReason(nil), observation.Decision.Reasons...)
	observation.Decision.Surfaces = append([]PresentationSurface(nil), observation.Decision.Surfaces...)
	observation.ResultRefs = append([]DetailRef(nil), observation.ResultRefs...)
	observation.EnvelopeRefs = append([]DetailRef(nil), observation.EnvelopeRefs...)
	return observation
}

// Relocalize rebuilds semantic summaries from retained typed evidence without
// changing execution facts, disclosure choices, or audit references.
func (s *ObservationStore) Relocalize(lang i18n.Language) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var joined error
	for index := range s.observations {
		observation := &s.observations[index]
		result, err := s.retainedToolResultLocked(*observation)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		aggregation := observation.Aggregation
		applyObservationPresentationInLanguage(observation, result, lang)
		observation.Aggregation = aggregation
	}
	for _, group := range s.aggregates {
		group.Summary = observationAggregateSummaryInLanguage(lang, group.Family, group.Intent, len(group.MemberIDs))
		for _, id := range group.MemberIDs {
			if index, ok := s.byID[id]; ok {
				s.observations[index].Aggregation.Summary = group.Summary
			}
		}
	}
	return joined
}

func (s *ObservationStore) retainedToolResultLocked(observation Observation) (*types.ToolResultBlock, error) {
	if len(observation.ResultRefs) == 0 {
		return nil, nil
	}
	detail, err := s.details.Get(observation.ResultRefs[len(observation.ResultRefs)-1])
	if err != nil {
		return nil, err
	}
	result := &types.ToolResultBlock{ToolUseID: observation.ToolUseID, Content: string(detail)}
	if len(observation.EnvelopeRefs) == 0 {
		return result, nil
	}
	envelope, err := s.details.Get(observation.EnvelopeRefs[len(observation.EnvelopeRefs)-1])
	if err != nil {
		return nil, err
	}
	var wire struct {
		Type          types.ContentType            `json:"type"`
		ToolUseID     string                       `json:"tool_use_id"`
		Content       string                       `json:"content"`
		ContentBlocks []json.RawMessage            `json:"content_blocks"`
		Data          any                          `json:"data"`
		IsError       bool                         `json:"is_error"`
		NewMessages   []types.Message              `json:"new_messages"`
		Metadata      map[string]string            `json:"metadata"`
		Usage         *types.Usage                 `json:"usage"`
		Outcome       types.ToolOutcome            `json:"outcome"`
		Completeness  types.ToolResultCompleteness `json:"completeness"`
	}
	if err := json.Unmarshal(envelope, &wire); err != nil {
		return nil, err
	}
	result.Type = wire.Type
	result.ToolUseID = firstNonEmptyString(wire.ToolUseID, observation.ToolUseID)
	result.Content = wire.Content
	result.Data = wire.Data
	result.IsError = wire.IsError
	result.NewMessages = userVisibleResultMessages(wire.NewMessages)
	result.Metadata = wire.Metadata
	result.Usage = wire.Usage
	result.Outcome = wire.Outcome
	result.Completeness = wire.Completeness
	if len(wire.ContentBlocks) > 0 {
		encoded, marshalErr := json.Marshal(struct {
			Role    types.Role        `json:"role"`
			Content []json.RawMessage `json:"content"`
		}{Role: types.RoleUser, Content: wire.ContentBlocks})
		if marshalErr != nil {
			return nil, marshalErr
		}
		var message types.Message
		if err := json.Unmarshal(encoded, &message); err != nil {
			return nil, err
		}
		result.ContentBlocks = message.Content
	}
	return result, nil
}

func toolEventLanguage(ctx ToolEventContext) i18n.Language {
	if ctx.LanguageSet {
		return ctx.Language
	}
	return i18n.DetectOrLoadLanguage()
}

func applyObservationPresentation(observation *Observation, result *types.ToolResultBlock) {
	applyObservationPresentationInLanguage(observation, result, i18n.DetectOrLoadLanguage())
}

func applyObservationPresentationInLanguage(observation *Observation, result *types.ToolResultBlock, lang i18n.Language) {
	if observation == nil {
		return
	}
	formatted := FormatToolPresentationInLanguage(lang, observation.ToolName, observation.ToolInput, observation.Outcome, result)
	if _, ok := observation.FullEvidenceRef(); ok {
		formatted.FullEvidenceAvailable = true
		refreshCompletenessDetails(lang, &formatted)
	}
	if formatted.Outcome != OutcomeUnknown {
		observation.Outcome = formatted.Outcome
	}
	if formatted.Family == FamilyAgent {
		formatted.HasMore = false
	} else {
		formatted.HasMore = formatted.HasMore || observation.Disclosure.HasMore || len(observation.ResultRefs) > 0 || len(observation.EnvelopeRefs) > 0
	}
	facts := formatted.Facts(observation.Outcome)
	if result != nil {
		facts.Large = len(result.TextContent()) > 16*1024
	}
	if observation.Disclosure.UserPinned {
		facts.Intent.PinnedEvidence = observation.Disclosure.Level == DisclosureEvidence
		facts.Intent.Inspect = observation.Disclosure.Level == DisclosureDetail
	}
	decision := DecidePresentation(facts)
	hasMore := formatted.HasMore
	disclosure := DisclosureState{Level: decision.DisclosureLevel(), HasMore: hasMore}
	if observation.Disclosure.UserPinned {
		disclosure.Level = observation.Disclosure.Level
		disclosure.UserPinned = true
		decision.EffectiveLevel = presentationLevelForDisclosure(disclosure.Level)
	}
	observation.Presentation = formatted
	observation.Decision = decision
	observation.Disclosure = disclosure
}

// FullEvidenceRef returns a real retained reference only when the result that
// established completeness is still present. The index form survives detail
// reference rewriting during checkpoint persistence.
func (o Observation) FullEvidenceRef() (DetailRef, bool) {
	if o.FullEvidenceResult < 1 || o.FullEvidenceResult > len(o.ResultRefs) {
		return DetailRef{}, false
	}
	ref := o.ResultRefs[o.FullEvidenceResult-1]
	if ref.Source == "" || ref.Key == "" || ref.Size < 0 || ref.Digest == "" {
		return DetailRef{}, false
	}
	return ref, true
}

func presentationLevelForDisclosure(level DisclosureLevel) PresentationLevel {
	switch level {
	case DisclosureDetail:
		return PresentationStructured
	case DisclosureEvidence:
		return PresentationEvidence
	default:
		return PresentationFolded
	}
}

func appendPresentationReason(reasons []PresentationReason, reason PresentationReason) []PresentationReason {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneJSONLikeValue(value)
	}
	return cloned
}

func cloneJSONLikeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStringAnyMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i := range typed {
			cloned[i] = cloneJSONLikeValue(typed[i])
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	default:
		return value
	}
}
