package flight

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxLedgerEntries = 4096
	maxOpaqueLength  = 1024
)

// NewState creates a validated flight with pending conditions and a known
// initial workspace digest.
func NewState(spec StateSpec) (FlightState, error) {
	limits, ok := normalizeLimits(spec.Limits)
	if !ok || !validOpaque(spec.WorkspaceInstanceID) || !validDigest(spec.WorkspaceDigest) ||
		!validDigest(spec.TaskDigest) || !validDigest(spec.VerificationConfigDigest) || len(spec.AcceptanceCriteria) == 0 {
		return FlightState{}, transitionError(ErrorInvalidSpec)
	}
	acceptance, ok := newConditions(spec.AcceptanceCriteria)
	if !ok {
		return FlightState{}, transitionError(ErrorInvalidSpec)
	}
	invariants, ok := newConditions(spec.Invariants)
	if !ok {
		return FlightState{}, transitionError(ErrorInvalidSpec)
	}
	state := FlightState{
		SchemaVersion:            CurrentSchemaVersion,
		WorkspaceInstanceID:      spec.WorkspaceInstanceID,
		WorkspaceDigest:          spec.WorkspaceDigest,
		WorkspaceDigestKnown:     true,
		TaskDigest:               spec.TaskDigest,
		VerificationConfigDigest: spec.VerificationConfigDigest,
		Acceptance:               AcceptanceState{Criteria: acceptance},
		Invariants:               InvariantState{Conditions: invariants},
		TerminalDisposition:      TerminalRunning,
		Limits:                   limits,
	}
	if err := Validate(state); err != nil {
		return FlightState{}, err
	}
	return state, nil
}

func newConditions(ids []string) ([]ConditionState, bool) {
	if len(ids) == 0 {
		return nil, true
	}
	result := make([]ConditionState, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !validOpaque(id) {
			return nil, false
		}
		if _, exists := seen[id]; exists {
			return nil, false
		}
		seen[id] = struct{}{}
		result = append(result, ConditionState{ID: id, Outcome: ConditionPending})
	}
	return result, true
}

func normalizeLimits(input LedgerLimits) (LedgerLimits, bool) {
	defaults := DefaultLedgerLimits()
	if input.Actions == 0 {
		input.Actions = defaults.Actions
	}
	if input.Failures == 0 {
		input.Failures = defaults.Failures
	}
	if input.Evidence == 0 {
		input.Evidence = defaults.Evidence
	}
	if input.Files == 0 {
		input.Files = defaults.Files
	}
	if input.Intents == 0 {
		input.Intents = defaults.Intents
	}
	if input.RepeatedFailureTrigger == 0 {
		input.RepeatedFailureTrigger = defaults.RepeatedFailureTrigger
	}
	if input.NoProgressTrigger == 0 {
		input.NoProgressTrigger = defaults.NoProgressTrigger
	}
	for _, value := range []int{input.Actions, input.Failures, input.Evidence, input.Files, input.Intents} {
		if value <= 0 || value > maxLedgerEntries {
			return LedgerLimits{}, false
		}
	}
	if input.RepeatedFailureTrigger > maxLedgerEntries || input.NoProgressTrigger > maxLedgerEntries {
		return LedgerLimits{}, false
	}
	return input, true
}

func validOpaque(value string) bool {
	if value == "" || len(value) > maxOpaqueLength || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validOptionalOpaque(value string) bool {
	return value == "" || validOpaque(value)
}

func validDigest(value Digest) bool {
	return validOpaque(string(value))
}

func validOptionalDigest(value Digest) bool {
	return value == "" || validDigest(value)
}

func validFingerprint(value Fingerprint) bool {
	return validOpaque(string(value))
}

func validOptionalFingerprint(value Fingerprint) bool {
	return value == "" || validFingerprint(value)
}

// Clone returns a fully detached state value.
func (state FlightState) Clone() FlightState {
	state.Acceptance.Criteria = append([]ConditionState(nil), state.Acceptance.Criteria...)
	state.Invariants.Conditions = append([]ConditionState(nil), state.Invariants.Conditions...)
	state.PendingIntents = append([]IntentState(nil), state.PendingIntents...)
	state.Actions = append([]ActionRecord(nil), state.Actions...)
	state.Failures = append([]FailureRecord(nil), state.Failures...)
	state.Evidence = append([]EvidenceRecord(nil), state.Evidence...)
	state.Files = append([]FileRecord(nil), state.Files...)
	if state.VerificationReceipt != nil {
		receipt := *state.VerificationReceipt
		state.VerificationReceipt = &receipt
	}
	return state
}

// Validate checks all persisted hard invariants without mutating state.
func Validate(state FlightState) error {
	if err := validateBase(state); err != nil {
		return err
	}
	if state.TerminalDisposition == TerminalCompleted {
		decision := completionDecision(state, false)
		if !decision.Allowed {
			return transitionError(ErrorInvalidState)
		}
	}
	return nil
}

func validateBase(state FlightState) error {
	if state.SchemaVersion != CurrentSchemaVersion || !validOpaque(state.WorkspaceInstanceID) ||
		!validDigest(state.TaskDigest) || !validDigest(state.VerificationConfigDigest) {
		return transitionError(ErrorInvalidState)
	}
	if state.WorkspaceDigestKnown != (state.WorkspaceDigest != "") || (state.WorkspaceDigestKnown && !validDigest(state.WorkspaceDigest)) {
		return transitionError(ErrorInvalidState)
	}
	if state.VerifiedEpoch > state.MutationEpoch || state.WorkspaceRevision > state.LedgerRevision ||
		state.VerificationInvalidatedRevision > state.LedgerRevision {
		return transitionError(ErrorInvalidState)
	}
	limits, ok := normalizeLimits(state.Limits)
	if !ok || limits != state.Limits {
		return transitionError(ErrorInvalidState)
	}
	if len(state.Actions) > limits.Actions || len(state.Failures) > limits.Failures ||
		len(state.Evidence) > limits.Evidence || len(state.Files) > limits.Files || len(state.PendingIntents) > limits.Intents {
		return transitionError(ErrorInvalidState)
	}
	if state.TerminalDisposition != TerminalRunning && state.TerminalDisposition != TerminalCompleted &&
		state.TerminalDisposition != TerminalBlocked && state.TerminalDisposition != TerminalAborted {
		return transitionError(ErrorInvalidState)
	}
	if len(state.Acceptance.Criteria) == 0 || !validConditionStates(state.Acceptance.Criteria, state.MutationEpoch) ||
		!validConditionStates(state.Invariants.Conditions, state.MutationEpoch) {
		return transitionError(ErrorInvalidState)
	}
	if !validIntents(state.PendingIntents, state.LedgerRevision, state.LastExecutionSequence) || !validLedgers(state) {
		return transitionError(ErrorInvalidState)
	}
	if state.ConsecutiveFailures == 0 {
		if state.LastFailureFingerprint != "" {
			return transitionError(ErrorInvalidState)
		}
	} else if !validFingerprint(state.LastFailureFingerprint) {
		return transitionError(ErrorInvalidState)
	}
	if state.VerificationReceipt != nil {
		receipt := *state.VerificationReceipt
		if !validReceipt(receipt) || !receiptMatchesState(receipt, state) || state.VerifiedEpoch != receipt.MutationEpoch {
			return transitionError(ErrorInvalidState)
		}
	}
	return nil
}

func validConditionStates(conditions []ConditionState, maximum Epoch) bool {
	seen := make(map[string]struct{}, len(conditions))
	for _, condition := range conditions {
		if !validOpaque(condition.ID) {
			return false
		}
		if _, exists := seen[condition.ID]; exists {
			return false
		}
		seen[condition.ID] = struct{}{}
		switch condition.Outcome {
		case ConditionPending:
			if condition.MutationEpoch != 0 || condition.WorkspaceDigest != "" || condition.EvidenceDigest != "" {
				return false
			}
		case ConditionSatisfied, ConditionUnsatisfied:
			if condition.MutationEpoch > maximum || !validDigest(condition.WorkspaceDigest) || !validDigest(condition.EvidenceDigest) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validIntents(intents []IntentState, revision Revision, lastExecutionSequence uint64) bool {
	seen := make(map[string]struct{}, len(intents))
	for _, intent := range intents {
		if !validOpaque(intent.ID) || intent.ExecutionSequence <= lastExecutionSequence || !validFingerprint(intent.ActionFingerprint) ||
			intent.OpenedRevision == 0 || intent.OpenedRevision > revision {
			return false
		}
		if _, exists := seen[intent.ID]; exists {
			return false
		}
		for _, other := range intents {
			if other.ID != intent.ID && other.ExecutionSequence == intent.ExecutionSequence {
				return false
			}
		}
		seen[intent.ID] = struct{}{}
	}
	return true
}

func validLedgers(state FlightState) bool {
	actionFingerprints := make(map[Fingerprint]struct{}, len(state.Actions))
	var actionRevision Revision
	var executionSequence uint64
	for _, action := range state.Actions {
		if action.Revision == 0 || action.Revision <= actionRevision || action.Revision > state.LedgerRevision || !validOpaque(action.ToolID) ||
			action.ExecutionSequence == 0 || action.ExecutionSequence <= executionSequence ||
			!validFingerprint(action.ActionFingerprint) || !validDigest(action.FactDigest) ||
			!validOptionalOpaque(action.IntentID) || !validOptionalDigest(action.WorkspaceDigest) ||
			action.WorkspaceDigestKnown != (action.WorkspaceDigest != "") || !validEffectScope(action.EffectScope) ||
			!validExecutionOutcome(action.ExecutionOutcome) || !validMutationOutcome(action.MutationOutcome) ||
			!validVerificationKind(action.VerificationKind) || !validVerificationOutcome(action.VerificationOutcome) ||
			action.MutationEpochBefore > action.MutationEpochAfter || action.MutationEpochAfter > state.MutationEpoch {
			return false
		}
		advanced := action.MutationEpochAfter == action.MutationEpochBefore+1
		if (action.MutationOutcome == MutationCommitted || action.MutationOutcome == MutationPossible) != advanced {
			return false
		}
		if advanced && !action.EffectScope.permitsWorkspaceMutation() {
			return false
		}
		if !action.Invoked && (action.ExecutionOutcome == ExecutionSucceeded || action.MutationOutcome != MutationNone ||
			action.VerificationKind != VerificationNone) {
			return false
		}
		if (action.VerificationKind == VerificationNone) != (action.VerificationOutcome == VerificationNotRun) {
			return false
		}
		if _, exists := actionFingerprints[action.ActionFingerprint]; exists {
			return false
		}
		actionFingerprints[action.ActionFingerprint] = struct{}{}
		if executionSequence != 0 && action.ExecutionSequence != executionSequence+1 {
			return false
		}
		actionRevision = action.Revision
		executionSequence = action.ExecutionSequence
	}
	if (len(state.Actions) == 0 && state.LastExecutionSequence != 0) ||
		(len(state.Actions) > 0 && executionSequence != state.LastExecutionSequence) {
		return false
	}
	var failureRevision Revision
	for _, failure := range state.Failures {
		if failure.Revision == 0 || failure.Revision <= failureRevision || failure.Revision > state.LedgerRevision || !validFingerprint(failure.ActionFingerprint) ||
			!validFingerprint(failure.FailureFingerprint) || failure.RecentOccurrence == 0 || failure.ConsecutiveOccurrence == 0 {
			return false
		}
		failureRevision = failure.Revision
	}
	var evidenceRevision Revision
	for _, evidence := range state.Evidence {
		if evidence.Revision == 0 || evidence.Revision < evidenceRevision || evidence.Revision > state.LedgerRevision || !validDigest(evidence.Digest) ||
			evidence.MutationEpoch > state.MutationEpoch || !validOptionalDigest(evidence.WorkspaceDigest) ||
			!validOptionalFingerprint(evidence.ActionFingerprint) {
			return false
		}
		switch evidence.Kind {
		case EvidenceObservation, EvidenceVerification, EvidenceAcceptance, EvidenceInvariant:
		default:
			return false
		}
		conditionEvidence := evidence.Kind == EvidenceAcceptance || evidence.Kind == EvidenceInvariant
		if conditionEvidence != (evidence.ConditionID != "") || conditionEvidence != (evidence.ConditionOutcome != "") {
			return false
		}
		if conditionEvidence && (!validOpaque(evidence.ConditionID) ||
			(evidence.ConditionOutcome != ConditionSatisfied && evidence.ConditionOutcome != ConditionUnsatisfied)) {
			return false
		}
		evidenceRevision = evidence.Revision
	}
	var fileRevision Revision
	for _, file := range state.Files {
		if file.Revision == 0 || file.Revision < fileRevision || file.Revision > state.LedgerRevision || !validFingerprint(file.ActionFingerprint) ||
			!validFingerprint(file.Identity) || !validOptionalDigest(file.BeforeDigest) || !validOptionalDigest(file.AfterDigest) ||
			!validMutationOutcome(file.MutationOutcome) {
			return false
		}
		if file.MutationOutcome == MutationNone && file.BeforeDigest != "" && file.AfterDigest != "" && file.BeforeDigest != file.AfterDigest {
			return false
		}
		fileRevision = file.Revision
	}
	return true
}

func validEffectScope(scope EffectScope) bool {
	return scope == EffectScopeNone || scope == EffectScopeWorkspaceRead || scope == EffectScopeWorkspaceWrite ||
		scope == EffectScopeExternal || scope == EffectScopeWorkspaceAndExternal
}

func validExecutionOutcome(outcome ExecutionOutcome) bool {
	return outcome == ExecutionSucceeded || outcome == ExecutionFailed || outcome == ExecutionCancelled
}

func validMutationOutcome(outcome MutationOutcome) bool {
	return outcome == MutationNone || outcome == MutationCommitted || outcome == MutationPossible
}

func validVerificationKind(kind VerificationKind) bool {
	return kind == VerificationNone || kind == VerificationObservation || kind == VerificationDiffReview || kind == VerificationBuild ||
		kind == VerificationStaticAnalysis || kind == VerificationTargetedTest || kind == VerificationFullTest ||
		kind == VerificationAcceptance || kind == VerificationInvariant
}

func validVerificationOutcome(outcome VerificationOutcome) bool {
	return outcome == VerificationNotRun || outcome == VerificationPassed || outcome == VerificationFailed ||
		outcome == VerificationInconclusive
}

func nextRevision(current Revision) (Revision, bool) {
	if current == Revision(math.MaxUint64) {
		return 0, false
	}
	return current + 1, true
}

func nextEpoch(current Epoch) (Epoch, bool) {
	if current == Epoch(math.MaxUint64) {
		return 0, false
	}
	return current + 1, true
}

func receiptBindingDigest(receipt VerificationReceipt) Digest {
	payload := struct {
		WorkspaceInstanceID      string
		MutationEpoch            Epoch
		IssuedRevision           Revision
		VerificationGeneration   uint64
		WorkspaceDigest          Digest
		TaskDigest               Digest
		VerificationConfigDigest Digest
		EvidenceDigest           Digest
		Kind                     VerificationKind
		ActionFingerprint        Fingerprint
	}{
		WorkspaceInstanceID:      receipt.WorkspaceInstanceID,
		MutationEpoch:            receipt.MutationEpoch,
		IssuedRevision:           receipt.IssuedRevision,
		VerificationGeneration:   receipt.VerificationGeneration,
		WorkspaceDigest:          receipt.WorkspaceDigest,
		TaskDigest:               receipt.TaskDigest,
		VerificationConfigDigest: receipt.VerificationConfigDigest,
		EvidenceDigest:           receipt.EvidenceDigest,
		Kind:                     receipt.Kind,
		ActionFingerprint:        receipt.ActionFingerprint,
	}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return Digest(hex.EncodeToString(sum[:]))
}

func sealReceipt(receipt VerificationReceipt) VerificationReceipt {
	receipt.BindingDigest = receiptBindingDigest(receipt)
	return receipt
}

func validReceipt(receipt VerificationReceipt) bool {
	return validOpaque(receipt.WorkspaceInstanceID) && receipt.IssuedRevision > 0 && validDigest(receipt.WorkspaceDigest) && validDigest(receipt.TaskDigest) &&
		validDigest(receipt.VerificationConfigDigest) && validDigest(receipt.EvidenceDigest) && receipt.Kind.Relevant() &&
		validFingerprint(receipt.ActionFingerprint) && validDigest(receipt.BindingDigest) &&
		receipt.BindingDigest == receiptBindingDigest(receipt)
}

func receiptMatchesState(receipt VerificationReceipt, state FlightState) bool {
	return state.WorkspaceDigestKnown && receipt.WorkspaceInstanceID == state.WorkspaceInstanceID &&
		receipt.MutationEpoch == state.MutationEpoch && receipt.WorkspaceDigest == state.WorkspaceDigest &&
		receipt.TaskDigest == state.TaskDigest && receipt.VerificationConfigDigest == state.VerificationConfigDigest &&
		receipt.VerificationGeneration == state.VerificationGeneration &&
		receipt.IssuedRevision <= state.LedgerRevision && receipt.IssuedRevision >= state.VerificationInvalidatedRevision
}

// CompletionGate computes the strict completion decision without modifying the
// state. Completion requires current conditions and an exact receipt, not just
// numeric equality between verified and mutation epochs.
func CompletionGate(state FlightState) CompletionDecision {
	structural := state.Clone()
	structural.VerificationReceipt = nil
	if err := validateBase(structural); err != nil {
		return CompletionDecision{Blocker: CompletionStateInvalid}
	}
	return completionDecision(state, true)
}

func completionDecision(state FlightState, checkTerminal bool) CompletionDecision {
	if checkTerminal && state.TerminalDisposition != TerminalRunning && state.TerminalDisposition != TerminalCompleted {
		return CompletionDecision{Blocker: CompletionAlreadyTerminal}
	}
	if len(state.PendingIntents) > 0 {
		return CompletionDecision{Blocker: CompletionIntentOpen, OpenIntents: len(state.PendingIntents)}
	}
	if !state.WorkspaceDigestKnown {
		return CompletionDecision{Blocker: CompletionWorkspaceUnknown}
	}
	if state.ConsecutiveFailures >= state.Limits.RepeatedFailureTrigger {
		return CompletionDecision{Blocker: CompletionRepeatedFailure}
	}
	pendingAcceptance, violatedAcceptance := conditionDeficits(state.Acceptance.Criteria, state)
	if pendingAcceptance > 0 || violatedAcceptance > 0 {
		return CompletionDecision{
			Blocker: CompletionAcceptanceIncomplete, PendingAcceptance: pendingAcceptance, ViolatedAcceptance: violatedAcceptance,
		}
	}
	pendingInvariants, violatedInvariants := conditionDeficits(state.Invariants.Conditions, state)
	if pendingInvariants > 0 || violatedInvariants > 0 {
		return CompletionDecision{
			Blocker: CompletionInvariantIncomplete, PendingInvariants: pendingInvariants, ViolatedInvariants: violatedInvariants,
		}
	}
	if state.VerificationReceipt == nil {
		return CompletionDecision{Blocker: CompletionVerificationMissing}
	}
	if state.VerifiedEpoch != state.MutationEpoch || state.VerificationReceipt.MutationEpoch != state.MutationEpoch {
		return CompletionDecision{Blocker: CompletionVerificationStale}
	}
	if !validReceipt(*state.VerificationReceipt) || !receiptMatchesState(*state.VerificationReceipt, state) {
		return CompletionDecision{Blocker: CompletionReceiptBinding}
	}
	return CompletionDecision{Allowed: true, Blocker: CompletionReady}
}

func conditionDeficits(conditions []ConditionState, state FlightState) (pending, violated int) {
	for _, condition := range conditions {
		current := condition.MutationEpoch == state.MutationEpoch && condition.WorkspaceDigest == state.WorkspaceDigest
		if !current || condition.Outcome == ConditionPending {
			pending++
			continue
		}
		if condition.Outcome != ConditionSatisfied {
			violated++
		}
	}
	return pending, violated
}
