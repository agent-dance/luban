package flight

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
)

// Reduce applies one event atomically and returns a detached next state plus
// scheduler-facing effects. Rejected transitions leave the input unchanged.
func Reduce(current FlightState, event Event) (FlightState, ReduceEffects, error) {
	base := current.Clone()
	effects := ReduceEffects{LedgerRevision: base.LedgerRevision, ReceiptDisposition: ReceiptNone}
	if err := Validate(base); err != nil {
		return base, effects, err
	}
	if event == nil {
		return base, effects, transitionError(ErrorInvalidEvent)
	}

	var next FlightState
	var err error
	switch typed := event.(type) {
	case IntentOpened:
		next, effects, err = reduceIntentOpened(base, typed, effects)
	case *IntentOpened:
		if typed == nil {
			return base, effects, transitionError(ErrorInvalidEvent)
		}
		next, effects, err = reduceIntentOpened(base, *typed, effects)
	case IntentClosed:
		next, effects, err = reduceIntentClosed(base, typed, effects)
	case *IntentClosed:
		if typed == nil {
			return base, effects, transitionError(ErrorInvalidEvent)
		}
		next, effects, err = reduceIntentClosed(base, *typed, effects)
	case ToolExecuted:
		next, effects, err = reduceToolExecuted(base, typed.Facts, effects)
	case *ToolExecuted:
		if typed == nil {
			return base, effects, transitionError(ErrorInvalidEvent)
		}
		next, effects, err = reduceToolExecuted(base, typed.Facts, effects)
	case ConditionsEvaluated:
		next, effects, err = reduceConditionsEvaluated(base, typed, effects)
	case *ConditionsEvaluated:
		if typed == nil {
			return base, effects, transitionError(ErrorInvalidEvent)
		}
		next, effects, err = reduceConditionsEvaluated(base, *typed, effects)
	case ReceiptPresented:
		next, effects, err = reduceReceiptPresented(base, typed.Receipt, effects)
	case *ReceiptPresented:
		if typed == nil {
			return base, effects, transitionError(ErrorInvalidEvent)
		}
		next, effects, err = reduceReceiptPresented(base, typed.Receipt, effects)
	case TerminalRequested:
		next, effects, err = reduceTerminalRequested(base, typed, effects)
	case *TerminalRequested:
		if typed == nil {
			return base, effects, transitionError(ErrorInvalidEvent)
		}
		next, effects, err = reduceTerminalRequested(base, *typed, effects)
	default:
		return base, effects, transitionError(ErrorInvalidEvent)
	}
	if err != nil {
		return base, ReduceEffects{LedgerRevision: base.LedgerRevision, ReceiptDisposition: ReceiptNone}, err
	}
	if err := Validate(next); err != nil {
		return base, ReduceEffects{LedgerRevision: base.LedgerRevision, ReceiptDisposition: ReceiptNone}, err
	}
	return next.Clone(), effects, nil
}

func reduceIntentOpened(state FlightState, event IntentOpened, effects ReduceEffects) (FlightState, ReduceEffects, error) {
	if state.TerminalDisposition != TerminalRunning {
		return state, effects, transitionError(ErrorTerminalState)
	}
	if !validOpaque(event.ID) || event.ExecutionSequence == 0 || !validFingerprint(event.ActionFingerprint) {
		return state, effects, transitionError(ErrorInvalidEvent)
	}
	if intentIndex(state.PendingIntents, event.ID) >= 0 {
		return state, effects, transitionError(ErrorIntentExists)
	}
	if len(state.PendingIntents) >= state.Limits.Intents {
		return state, effects, transitionError(ErrorIntentLimit)
	}
	for _, intent := range state.PendingIntents {
		if intent.ExecutionSequence == event.ExecutionSequence {
			return state, effects, transitionError(ErrorIntentExists)
		}
	}
	if event.ExecutionSequence <= state.LastExecutionSequence {
		return state, effects, transitionError(ErrorStaleExecution)
	}
	revision, ok := nextRevision(state.LedgerRevision)
	if !ok {
		return state, effects, transitionError(ErrorRevisionOverflow)
	}
	state.PendingIntents = append(state.PendingIntents, IntentState{
		ID: event.ID, ExecutionSequence: event.ExecutionSequence, ActionFingerprint: event.ActionFingerprint, OpenedRevision: revision,
	})
	return finishChange(state, effects, revision), effectsWithRevision(effects, revision), nil
}

func reduceIntentClosed(state FlightState, event IntentClosed, effects ReduceEffects) (FlightState, ReduceEffects, error) {
	if state.TerminalDisposition != TerminalRunning {
		return state, effects, transitionError(ErrorTerminalState)
	}
	if !validOpaque(event.ID) || event.ExecutionSequence == 0 || !validFingerprint(event.ActionFingerprint) ||
		(event.Resolution != IntentCompleted && event.Resolution != IntentAbandoned) {
		return state, effects, transitionError(ErrorInvalidEvent)
	}
	index := intentIndex(state.PendingIntents, event.ID)
	if index < 0 {
		return state, effects, transitionError(ErrorIntentNotFound)
	}
	if state.PendingIntents[index].ExecutionSequence != event.ExecutionSequence ||
		state.PendingIntents[index].ActionFingerprint != event.ActionFingerprint {
		return state, effects, transitionError(ErrorIntentMismatch)
	}
	revision, ok := nextRevision(state.LedgerRevision)
	if !ok {
		return state, effects, transitionError(ErrorRevisionOverflow)
	}
	state.PendingIntents = removeIntent(state.PendingIntents, index)
	return finishChange(state, effects, revision), effectsWithRevision(effects, revision), nil
}

func reduceToolExecuted(state FlightState, facts ToolExecutionFacts, effects ReduceEffects) (FlightState, ReduceEffects, error) {
	if err := validateToolFacts(facts); err != nil {
		return state, effects, err
	}
	factDigest := digestFacts(facts)
	for _, record := range state.Actions {
		if record.ExecutionSequence == facts.ExecutionSequence {
			if record.FactDigest != factDigest {
				return state, effects, transitionError(ErrorConflictingReplay)
			}
			effects.DuplicateAction = true
			return state, effects, nil
		}
		if record.ActionFingerprint == facts.ActionFingerprint {
			return state, effects, transitionError(ErrorConflictingReplay)
		}
	}
	if facts.ExecutionSequence <= state.LastExecutionSequence {
		return state, effects, transitionError(ErrorStaleExecution)
	}
	if facts.ExecutionSequence != state.LastExecutionSequence+1 {
		return state, effects, transitionError(ErrorExecutionGap)
	}
	for _, intent := range state.PendingIntents {
		if intent.ExecutionSequence == facts.ExecutionSequence && intent.ID != facts.IntentID {
			return state, effects, transitionError(ErrorIntentMismatch)
		}
	}
	if state.TerminalDisposition != TerminalRunning {
		return state, effects, transitionError(ErrorTerminalState)
	}
	if facts.IntentID != "" {
		index := intentIndex(state.PendingIntents, facts.IntentID)
		if index < 0 {
			return state, effects, transitionError(ErrorIntentNotFound)
		}
		if state.PendingIntents[index].ExecutionSequence != facts.ExecutionSequence ||
			state.PendingIntents[index].ActionFingerprint != facts.ActionFingerprint {
			return state, effects, transitionError(ErrorIntentMismatch)
		}
	}
	revision, ok := nextRevision(state.LedgerRevision)
	if !ok {
		return state, effects, transitionError(ErrorRevisionOverflow)
	}
	beforeEpoch := state.MutationEpoch
	var err error
	state, effects, err = applyWorkspaceFacts(state, facts, revision, effects)
	if err != nil {
		return state, effects, err
	}
	if err := validateFactWorkspaceEpoch(facts, state.MutationEpoch); err != nil {
		return state, effects, err
	}

	if facts.IntentID != "" {
		state.PendingIntents = removeIntent(state.PendingIntents, intentIndex(state.PendingIntents, facts.IntentID))
	}

	evidenceKind := EvidenceObservation
	if facts.VerificationKind != VerificationNone {
		evidenceKind = EvidenceVerification
	}
	evidenceEpoch := state.MutationEpoch
	if facts.VerificationKind != VerificationNone {
		evidenceEpoch = facts.VerificationEpoch
	}
	observedDigest := observedWorkspaceDigest(facts, state)
	novelEvidence := false
	if facts.EvidenceDigest != "" {
		novelEvidence = appendEvidence(&state, EvidenceRecord{
			Revision: revision, Kind: evidenceKind, Digest: facts.EvidenceDigest, MutationEpoch: evidenceEpoch,
			WorkspaceDigest: observedDigest, ActionFingerprint: facts.ActionFingerprint,
		})
	}

	state, effects, err = applyVerification(state, facts, observedDigest, revision, effects)
	if err != nil {
		return state, effects, err
	}
	for _, file := range facts.Files {
		state.Files = appendBounded(state.Files, FileRecord{
			Revision: revision, ActionFingerprint: facts.ActionFingerprint, Identity: file.Identity,
			BeforeDigest: file.BeforeDigest, AfterDigest: file.AfterDigest, MutationOutcome: file.MutationOutcome,
		}, state.Limits.Files)
	}

	if facts.ExecutionOutcome == ExecutionSucceeded {
		state.LastFailureFingerprint = ""
		state.ConsecutiveFailures = 0
	} else {
		state = recordFailure(state, facts, revision)
		if state.ConsecutiveFailures >= state.Limits.RepeatedFailureTrigger {
			effects.RepeatedFailure = true
		}
	}

	currentEvidence := novelEvidence && state.WorkspaceDigestKnown && evidenceEpoch == state.MutationEpoch && observedDigest == state.WorkspaceDigest
	effects.Progress.Mutation = effects.MutationAdvanced && facts.MutationOutcome == MutationCommitted && facts.ExecutionOutcome == ExecutionSucceeded
	effects.Progress.Evidence = currentEvidence && facts.ExecutionOutcome == ExecutionSucceeded &&
		(facts.VerificationKind == VerificationNone || facts.VerificationOutcome == VerificationPassed)
	if effects.ReceiptDisposition == ReceiptIssued {
		effects.Progress.Verification = true
	}
	if effects.Progress.Any() {
		state.NoProgressStreak = 0
	} else if state.NoProgressStreak < math.MaxUint32 {
		state.NoProgressStreak++
	}
	if state.NoProgressStreak >= state.Limits.NoProgressTrigger {
		effects.NoProgress = true
	}
	if effects.RepeatedFailure || effects.NoProgress {
		effects.RecommendedDisposition = TerminalBlocked
	}

	state.Actions = appendBounded(state.Actions, ActionRecord{
		Revision: revision, ToolID: facts.ToolID, IntentID: facts.IntentID,
		ExecutionSequence: facts.ExecutionSequence, Invoked: facts.Invoked,
		ActionFingerprint: facts.ActionFingerprint, FactDigest: factDigest,
		ExecutionOutcome: facts.ExecutionOutcome, EffectScope: facts.EffectScope, MutationOutcome: facts.MutationOutcome,
		MutationEpochBefore: beforeEpoch, MutationEpochAfter: state.MutationEpoch,
		WorkspaceDigestKnown: state.WorkspaceDigestKnown, WorkspaceDigest: state.WorkspaceDigest,
		VerificationKind: facts.VerificationKind, VerificationOutcome: facts.VerificationOutcome,
	}, state.Limits.Actions)
	state.LastExecutionSequence = facts.ExecutionSequence
	state.LedgerRevision = revision
	effects.StateChanged = true
	effects.LedgerRevision = revision
	return state, effects, nil
}

func validateToolFacts(facts ToolExecutionFacts) error {
	if !validOpaque(facts.ToolID) || !validOptionalOpaque(facts.IntentID) || facts.ExecutionSequence == 0 ||
		!validFingerprint(facts.ActionFingerprint) ||
		!validOptionalFingerprint(facts.FailureFingerprint) || !validOptionalDigest(facts.BeforeDigest) ||
		!validOptionalDigest(facts.AfterDigest) || !validOptionalDigest(facts.EvidenceDigest) ||
		!validOptionalDigest(facts.VerificationConfigDigest) {
		return transitionError(ErrorInvalidFacts)
	}
	if !validEffectScope(facts.EffectScope) {
		return transitionError(ErrorInvalidFacts)
	}
	if !validExecutionOutcome(facts.ExecutionOutcome) {
		return transitionError(ErrorInvalidFacts)
	}
	if !facts.Invoked && (facts.ExecutionOutcome == ExecutionSucceeded || facts.MutationOutcome != MutationNone || facts.NoMutationProven ||
		facts.VerificationKind != VerificationNone || facts.BeforeDigest != "" || facts.AfterDigest != "" ||
		facts.EvidenceDigest != "" || facts.VerificationConfigDigest != "" || len(facts.Files) != 0) {
		return transitionError(ErrorInvalidFacts)
	}
	switch facts.ExecutionOutcome {
	case ExecutionSucceeded:
		if facts.FailureFingerprint != "" {
			return transitionError(ErrorInvalidFacts)
		}
	case ExecutionFailed, ExecutionCancelled:
		if !validFingerprint(facts.FailureFingerprint) {
			return transitionError(ErrorInvalidFacts)
		}
	}
	if !validMutationOutcome(facts.MutationOutcome) {
		return transitionError(ErrorInvalidFacts)
	}
	if (facts.MutationOutcome == MutationCommitted || facts.MutationOutcome == MutationPossible) &&
		!facts.EffectScope.permitsWorkspaceMutation() {
		return transitionError(ErrorInvalidFacts)
	}
	if facts.EffectScope == EffectScopeWorkspaceRead && facts.MutationOutcome != MutationNone {
		return transitionError(ErrorInvalidFacts)
	}
	if !facts.EffectScope.affectsWorkspace() && (facts.BeforeDigest != "" || facts.AfterDigest != "") {
		return transitionError(ErrorInvalidFacts)
	}
	if facts.MutationOutcome == MutationCommitted && facts.AfterDigest == "" {
		return transitionError(ErrorInvalidFacts)
	}
	if facts.NoMutationProven && (!facts.Invoked || !facts.EffectScope.permitsWorkspaceMutation() ||
		facts.MutationOutcome != MutationNone || facts.ExecutionOutcome != ExecutionFailed) {
		return transitionError(ErrorInvalidFacts)
	}
	if facts.Invoked && facts.EffectScope.permitsWorkspaceMutation() && facts.MutationOutcome == MutationNone && !facts.NoMutationProven &&
		(facts.BeforeDigest == "" || facts.AfterDigest == "" || facts.BeforeDigest != facts.AfterDigest) {
		return transitionError(ErrorInvalidFacts)
	}
	if facts.VerificationKind == VerificationNone {
		if facts.VerificationOutcome != VerificationNotRun || facts.VerificationEpoch != 0 || facts.VerificationConfigDigest != "" {
			return transitionError(ErrorInvalidFacts)
		}
	} else {
		if !validVerificationKind(facts.VerificationKind) {
			return transitionError(ErrorInvalidFacts)
		}
		if facts.VerificationOutcome != VerificationPassed && facts.VerificationOutcome != VerificationFailed &&
			facts.VerificationOutcome != VerificationInconclusive || facts.EvidenceDigest == "" {
			return transitionError(ErrorInvalidFacts)
		}
		if facts.VerificationOutcome == VerificationPassed && facts.ExecutionOutcome != ExecutionSucceeded {
			return transitionError(ErrorInvalidFacts)
		}
		if facts.VerificationKind.Relevant() && facts.VerificationOutcome == VerificationPassed &&
			(facts.BeforeDigest == "" || facts.AfterDigest == "") {
			return transitionError(ErrorInvalidFacts)
		}
		if facts.VerificationKind.Relevant() && facts.VerificationConfigDigest == "" {
			return transitionError(ErrorInvalidFacts)
		}
	}
	seenFiles := make(map[Fingerprint]struct{}, len(facts.Files))
	for _, file := range facts.Files {
		if !validFingerprint(file.Identity) || !validOptionalDigest(file.BeforeDigest) || !validOptionalDigest(file.AfterDigest) ||
			!validMutationOutcome(file.MutationOutcome) {
			return transitionError(ErrorInvalidFacts)
		}
		if _, exists := seenFiles[file.Identity]; exists {
			return transitionError(ErrorInvalidFacts)
		}
		seenFiles[file.Identity] = struct{}{}
		if (file.MutationOutcome == MutationCommitted || file.MutationOutcome == MutationPossible) &&
			facts.MutationOutcome != MutationCommitted && facts.MutationOutcome != MutationPossible {
			return transitionError(ErrorInvalidFacts)
		}
		if file.MutationOutcome == MutationNone && file.BeforeDigest != "" && file.AfterDigest != "" &&
			file.BeforeDigest != file.AfterDigest {
			return transitionError(ErrorInvalidFacts)
		}
	}
	return nil
}

func applyWorkspaceFacts(state FlightState, facts ToolExecutionFacts, revision Revision, effects ReduceEffects) (FlightState, ReduceEffects, error) {
	if !facts.EffectScope.affectsWorkspace() {
		return state, effects, nil
	}
	if state.WorkspaceDigestKnown && facts.BeforeDigest != "" && facts.BeforeDigest != state.WorkspaceDigest {
		return state, effects, transitionError(ErrorStaleWorkspaceFact)
	}

	switch facts.MutationOutcome {
	case MutationCommitted, MutationPossible:
		if state.WorkspaceDigestKnown && facts.BeforeDigest == "" {
			return state, effects, transitionError(ErrorInvalidFacts)
		}
		epoch, ok := nextEpoch(state.MutationEpoch)
		if !ok {
			return state, effects, transitionError(ErrorEpochOverflow)
		}
		workspaceRevision, ok := nextRevision(state.WorkspaceRevision)
		if !ok {
			return state, effects, transitionError(ErrorRevisionOverflow)
		}
		state.MutationEpoch = epoch
		if state.VerificationGeneration == math.MaxUint64 {
			return state, effects, transitionError(ErrorGenerationOverflow)
		}
		state.VerificationGeneration++
		state.WorkspaceRevision = workspaceRevision
		state.VerificationReceipt = nil
		state.VerificationInvalidatedRevision = revision
		invalidateConditions(state.Acceptance.Criteria)
		invalidateConditions(state.Invariants.Conditions)
		effects.MutationAdvanced = true
		effects.VerificationInvalidated = true
		if facts.AfterDigest == "" {
			state.WorkspaceDigest = ""
			state.WorkspaceDigestKnown = false
		} else {
			state.WorkspaceDigest = facts.AfterDigest
			state.WorkspaceDigestKnown = true
		}
	case MutationNone:
		if state.WorkspaceDigestKnown {
			if facts.AfterDigest != "" && facts.AfterDigest != state.WorkspaceDigest {
				return state, effects, transitionError(ErrorStaleWorkspaceFact)
			}
			return state, effects, nil
		}
		observed := facts.AfterDigest
		if observed == "" {
			observed = facts.BeforeDigest
		}
		if facts.BeforeDigest != "" && facts.AfterDigest != "" && facts.BeforeDigest != facts.AfterDigest {
			return state, effects, transitionError(ErrorStaleWorkspaceFact)
		}
		if observed != "" {
			workspaceRevision, ok := nextRevision(state.WorkspaceRevision)
			if !ok {
				return state, effects, transitionError(ErrorRevisionOverflow)
			}
			state.WorkspaceDigest = observed
			state.WorkspaceDigestKnown = true
			state.WorkspaceRevision = workspaceRevision
			effects.WorkspaceReconciled = true
		}
	}
	return state, effects, nil
}

func applyVerification(state FlightState, facts ToolExecutionFacts, observed Digest, revision Revision, effects ReduceEffects) (FlightState, ReduceEffects, error) {
	if !facts.VerificationKind.Relevant() {
		return state, effects, nil
	}
	current := state.WorkspaceDigestKnown && facts.VerificationEpoch == state.MutationEpoch && observed == state.WorkspaceDigest &&
		facts.VerificationConfigDigest == state.VerificationConfigDigest
	if !current {
		effects.StaleVerification = true
		effects.ReceiptDisposition = ReceiptStale
		return state, effects, nil
	}
	if facts.VerificationOutcome == VerificationPassed {
		receipt := sealReceipt(VerificationReceipt{
			WorkspaceInstanceID: state.WorkspaceInstanceID, MutationEpoch: state.MutationEpoch, IssuedRevision: revision,
			VerificationGeneration: state.VerificationGeneration,
			WorkspaceDigest:        state.WorkspaceDigest, TaskDigest: state.TaskDigest,
			VerificationConfigDigest: state.VerificationConfigDigest, EvidenceDigest: facts.EvidenceDigest,
			Kind: facts.VerificationKind, ActionFingerprint: facts.ActionFingerprint,
		})
		state.VerifiedEpoch = state.MutationEpoch
		state.VerificationReceipt = &receipt
		effects.ReceiptDisposition = ReceiptIssued
		effects.Receipt = cloneReceipt(&receipt)
		return state, effects, nil
	}
	if state.VerificationGeneration == math.MaxUint64 {
		return state, effects, transitionError(ErrorGenerationOverflow)
	}
	state.VerificationGeneration++
	state.VerificationInvalidatedRevision = revision
	if state.VerificationReceipt != nil {
		state.VerificationReceipt = nil
		effects.VerificationInvalidated = true
	}
	return state, effects, nil
}

func reduceConditionsEvaluated(state FlightState, event ConditionsEvaluated, effects ReduceEffects) (FlightState, ReduceEffects, error) {
	if state.TerminalDisposition != TerminalRunning {
		return state, effects, transitionError(ErrorTerminalState)
	}
	if len(event.Acceptance) == 0 && len(event.Invariants) == 0 {
		return state, effects, transitionError(ErrorInvalidEvent)
	}
	if err := validateConditionEvaluations(event.Acceptance, state.Acceptance.Criteria, state.MutationEpoch); err != nil {
		return state, effects, err
	}
	if err := validateConditionEvaluations(event.Invariants, state.Invariants.Conditions, state.MutationEpoch); err != nil {
		return state, effects, err
	}
	revision, ok := nextRevision(state.LedgerRevision)
	if !ok {
		return state, effects, transitionError(ErrorRevisionOverflow)
	}
	changed := false
	conditionProgress := false
	for _, evaluation := range event.Acceptance {
		current := state.WorkspaceDigestKnown && evaluation.MutationEpoch == state.MutationEpoch && evaluation.WorkspaceDigest == state.WorkspaceDigest
		if current {
			index := conditionIndex(state.Acceptance.Criteria, evaluation.ID)
			candidate := conditionFromEvaluation(evaluation)
			if conflictingConditionEvidence(state.Acceptance.Criteria[index], candidate) {
				return state, effects, transitionError(ErrorConflictingEvidence)
			}
			if state.Acceptance.Criteria[index] != candidate {
				if candidate.Outcome == ConditionSatisfied && state.Acceptance.Criteria[index].Outcome != ConditionSatisfied {
					conditionProgress = true
				}
				state.Acceptance.Criteria[index] = candidate
				changed = true
				effects.ConditionsUpdated++
			}
		} else {
			effects.StaleConditionEvaluations++
		}
		if appendEvidence(&state, EvidenceRecord{
			Revision: revision, Kind: EvidenceAcceptance, Digest: evaluation.EvidenceDigest,
			MutationEpoch: evaluation.MutationEpoch, WorkspaceDigest: evaluation.WorkspaceDigest,
			ConditionID: evaluation.ID, ConditionOutcome: evaluation.Outcome,
		}) {
			changed = true
		}
	}
	for _, evaluation := range event.Invariants {
		current := state.WorkspaceDigestKnown && evaluation.MutationEpoch == state.MutationEpoch && evaluation.WorkspaceDigest == state.WorkspaceDigest
		if current {
			index := conditionIndex(state.Invariants.Conditions, evaluation.ID)
			candidate := conditionFromEvaluation(evaluation)
			if conflictingConditionEvidence(state.Invariants.Conditions[index], candidate) {
				return state, effects, transitionError(ErrorConflictingEvidence)
			}
			if state.Invariants.Conditions[index] != candidate {
				if candidate.Outcome == ConditionSatisfied && state.Invariants.Conditions[index].Outcome != ConditionSatisfied {
					conditionProgress = true
				}
				state.Invariants.Conditions[index] = candidate
				changed = true
				effects.ConditionsUpdated++
			}
		} else {
			effects.StaleConditionEvaluations++
		}
		if appendEvidence(&state, EvidenceRecord{
			Revision: revision, Kind: EvidenceInvariant, Digest: evaluation.EvidenceDigest,
			MutationEpoch: evaluation.MutationEpoch, WorkspaceDigest: evaluation.WorkspaceDigest,
			ConditionID: evaluation.ID, ConditionOutcome: evaluation.Outcome,
		}) {
			changed = true
		}
	}
	if !changed {
		return state, effects, nil
	}
	state.LedgerRevision = revision
	effects.StateChanged = true
	effects.LedgerRevision = revision
	effects.Progress.Conditions = conditionProgress
	if effects.Progress.Conditions {
		state.NoProgressStreak = 0
	}
	return state, effects, nil
}

func validateConditionEvaluations(evaluations []ConditionEvaluation, conditions []ConditionState, maximum Epoch) error {
	seen := make(map[string]struct{}, len(evaluations))
	for _, evaluation := range evaluations {
		if !validOpaque(evaluation.ID) || (evaluation.Outcome != ConditionSatisfied && evaluation.Outcome != ConditionUnsatisfied) ||
			evaluation.MutationEpoch > maximum || !validDigest(evaluation.WorkspaceDigest) || !validDigest(evaluation.EvidenceDigest) {
			return transitionError(ErrorInvalidEvent)
		}
		if conditionIndex(conditions, evaluation.ID) < 0 {
			return transitionError(ErrorConditionNotFound)
		}
		if _, exists := seen[evaluation.ID]; exists {
			return transitionError(ErrorInvalidEvent)
		}
		seen[evaluation.ID] = struct{}{}
	}
	return nil
}

func reduceReceiptPresented(state FlightState, receipt VerificationReceipt, effects ReduceEffects) (FlightState, ReduceEffects, error) {
	if !validReceipt(receipt) {
		return state, effects, transitionError(ErrorInvalidEvent)
	}
	if state.TerminalDisposition == TerminalCompleted {
		if state.VerificationReceipt != nil && *state.VerificationReceipt == receipt && state.VerifiedEpoch == receipt.MutationEpoch {
			effects.ReceiptDisposition = ReceiptAccepted
			effects.Receipt = cloneReceipt(&receipt)
			return state, effects, nil
		}
		return state, effects, transitionError(ErrorTerminalState)
	}
	if state.TerminalDisposition != TerminalRunning {
		return state, effects, transitionError(ErrorTerminalState)
	}
	if !receiptMatchesState(receipt, state) {
		effects.ReceiptDisposition = ReceiptStale
		return state, effects, nil
	}
	effects.ReceiptDisposition = ReceiptAccepted
	effects.Receipt = cloneReceipt(&receipt)
	if state.VerificationReceipt != nil && *state.VerificationReceipt == receipt && state.VerifiedEpoch == receipt.MutationEpoch {
		return state, effects, nil
	}
	revision, ok := nextRevision(state.LedgerRevision)
	if !ok {
		return state, effects, transitionError(ErrorRevisionOverflow)
	}
	state.VerifiedEpoch = receipt.MutationEpoch
	state.VerificationReceipt = cloneReceipt(&receipt)
	appendEvidence(&state, EvidenceRecord{
		Revision: revision, Kind: EvidenceVerification, Digest: receipt.EvidenceDigest,
		MutationEpoch: receipt.MutationEpoch, WorkspaceDigest: receipt.WorkspaceDigest,
		ActionFingerprint: receipt.ActionFingerprint,
	})
	state.LedgerRevision = revision
	effects.StateChanged = true
	effects.LedgerRevision = revision
	effects.Progress.Verification = true
	state.NoProgressStreak = 0
	return state, effects, nil
}

func reduceTerminalRequested(state FlightState, event TerminalRequested, effects ReduceEffects) (FlightState, ReduceEffects, error) {
	if event.Disposition != TerminalCompleted && event.Disposition != TerminalBlocked && event.Disposition != TerminalAborted {
		return state, effects, transitionError(ErrorInvalidEvent)
	}
	if state.TerminalDisposition == event.Disposition {
		if event.Disposition == TerminalCompleted {
			effects.Completion = CompletionGate(state)
		}
		return state, effects, nil
	}
	if state.TerminalDisposition != TerminalRunning {
		return state, effects, transitionError(ErrorTerminalState)
	}
	if event.Disposition == TerminalCompleted {
		effects.Completion = CompletionGate(state)
		if !effects.Completion.Allowed {
			return state, effects, nil
		}
	}
	revision, ok := nextRevision(state.LedgerRevision)
	if !ok {
		return state, effects, transitionError(ErrorRevisionOverflow)
	}
	state.TerminalDisposition = event.Disposition
	return finishChange(state, effects, revision), effectsWithRevision(effects, revision), nil
}

func validateFactWorkspaceEpoch(facts ToolExecutionFacts, maximum Epoch) error {
	if facts.VerificationKind != VerificationNone && facts.VerificationEpoch > maximum {
		return transitionError(ErrorInvalidFacts)
	}
	return nil
}

func digestFacts(facts ToolExecutionFacts) Digest {
	encoded, _ := json.Marshal(facts)
	sum := sha256.Sum256(encoded)
	return Digest(hex.EncodeToString(sum[:]))
}

func observedWorkspaceDigest(facts ToolExecutionFacts, state FlightState) Digest {
	if facts.AfterDigest != "" {
		return facts.AfterDigest
	}
	if facts.BeforeDigest != "" {
		return facts.BeforeDigest
	}
	if state.WorkspaceDigestKnown {
		return state.WorkspaceDigest
	}
	return ""
}

func invalidateConditions(conditions []ConditionState) {
	for index := range conditions {
		conditions[index].Outcome = ConditionPending
		conditions[index].MutationEpoch = 0
		conditions[index].WorkspaceDigest = ""
		conditions[index].EvidenceDigest = ""
	}
}

func conditionFromEvaluation(evaluation ConditionEvaluation) ConditionState {
	return ConditionState{
		ID: evaluation.ID, Outcome: evaluation.Outcome, MutationEpoch: evaluation.MutationEpoch,
		WorkspaceDigest: evaluation.WorkspaceDigest, EvidenceDigest: evaluation.EvidenceDigest,
	}
}

func conflictingConditionEvidence(current, candidate ConditionState) bool {
	return current.Outcome != ConditionPending && current.MutationEpoch == candidate.MutationEpoch &&
		current.WorkspaceDigest == candidate.WorkspaceDigest && current.EvidenceDigest == candidate.EvidenceDigest &&
		current.Outcome != candidate.Outcome
}

func conditionIndex(conditions []ConditionState, id string) int {
	for index := range conditions {
		if conditions[index].ID == id {
			return index
		}
	}
	return -1
}

func intentIndex(intents []IntentState, id string) int {
	for index := range intents {
		if intents[index].ID == id {
			return index
		}
	}
	return -1
}

func removeIntent(intents []IntentState, index int) []IntentState {
	result := make([]IntentState, 0, len(intents)-1)
	result = append(result, intents[:index]...)
	result = append(result, intents[index+1:]...)
	return result
}

func appendEvidence(state *FlightState, record EvidenceRecord) bool {
	for _, existing := range state.Evidence {
		if existing.Kind == record.Kind && existing.Digest == record.Digest && existing.MutationEpoch == record.MutationEpoch &&
			existing.WorkspaceDigest == record.WorkspaceDigest && existing.ConditionID == record.ConditionID &&
			existing.ConditionOutcome == record.ConditionOutcome {
			return false
		}
	}
	state.Evidence = appendBounded(state.Evidence, record, state.Limits.Evidence)
	return true
}

func recordFailure(state FlightState, facts ToolExecutionFacts, revision Revision) FlightState {
	if state.LastFailureFingerprint == facts.FailureFingerprint {
		if state.ConsecutiveFailures < math.MaxUint32 {
			state.ConsecutiveFailures++
		}
	} else {
		state.LastFailureFingerprint = facts.FailureFingerprint
		state.ConsecutiveFailures = 1
	}
	recent := uint32(1)
	for _, record := range state.Failures {
		if record.FailureFingerprint == facts.FailureFingerprint && recent < math.MaxUint32 {
			recent++
		}
	}
	state.Failures = appendBounded(state.Failures, FailureRecord{
		Revision: revision, ActionFingerprint: facts.ActionFingerprint, FailureFingerprint: facts.FailureFingerprint,
		RecentOccurrence: recent, ConsecutiveOccurrence: state.ConsecutiveFailures,
	}, state.Limits.Failures)
	return state
}

func appendBounded[T any](values []T, value T, limit int) []T {
	if len(values) < limit {
		return append(values, value)
	}
	result := make([]T, limit)
	copy(result, values[len(values)-limit+1:])
	result[limit-1] = value
	return result
}

func cloneReceipt(receipt *VerificationReceipt) *VerificationReceipt {
	if receipt == nil {
		return nil
	}
	copy := *receipt
	return &copy
}

func finishChange(state FlightState, _ ReduceEffects, revision Revision) FlightState {
	state.LedgerRevision = revision
	return state
}

func effectsWithRevision(effects ReduceEffects, revision Revision) ReduceEffects {
	effects.StateChanged = true
	effects.LedgerRevision = revision
	return effects
}
