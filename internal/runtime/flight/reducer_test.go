package flight

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestCompletionRequiresClosedIntentsCurrentConditionsAndReceipt(t *testing.T) {
	state := testState(t, LedgerLimits{})
	state = mustReduce(t, state, IntentOpened{ID: "intent-1", ExecutionSequence: 1, ActionFingerprint: "verify-0"})
	if decision := CompletionGate(state); decision.Blocker != CompletionIntentOpen || decision.OpenIntents != 1 {
		t.Fatalf("open-intent gate = %+v", decision)
	}

	var verificationEffects ReduceEffects
	state, verificationEffects = mustReduceEffects(t, state, ToolExecuted{Facts: passingVerification(
		"verify-0", "intent-1", 1, 0, "workspace-0", "verification-evidence-0",
	)})
	if verificationEffects.ReceiptDisposition != ReceiptIssued || verificationEffects.Receipt == nil ||
		state.VerificationReceipt == nil || state.VerifiedEpoch != 0 {
		t.Fatalf("verification effects/state = %+v / %+v", verificationEffects, state)
	}
	if decision := CompletionGate(state); decision.Blocker != CompletionAcceptanceIncomplete || decision.PendingAcceptance != 2 {
		t.Fatalf("unevaluated acceptance gate = %+v", decision)
	}

	state = mustReduce(t, state, ConditionsEvaluated{
		Acceptance: []ConditionEvaluation{
			currentCondition("criterion-build", ConditionSatisfied, state, "acceptance-build"),
			currentCondition("criterion-tests", ConditionSatisfied, state, "acceptance-tests"),
		},
		Invariants: []ConditionEvaluation{
			currentCondition("invariant-i18n", ConditionUnsatisfied, state, "invariant-failed"),
		},
	})
	if decision := CompletionGate(state); decision.Blocker != CompletionInvariantIncomplete || decision.ViolatedInvariants != 1 {
		t.Fatalf("violated-invariant gate = %+v", decision)
	}
	state = mustReduce(t, state, ConditionsEvaluated{Invariants: []ConditionEvaluation{
		currentCondition("invariant-i18n", ConditionSatisfied, state, "invariant-passed"),
	}})
	if decision := CompletionGate(state); !decision.Allowed || decision.Blocker != CompletionReady {
		t.Fatalf("ready gate = %+v", decision)
	}
	state = mustReduce(t, state, TerminalRequested{Disposition: TerminalCompleted})
	if state.TerminalDisposition != TerminalCompleted {
		t.Fatalf("terminal disposition = %q", state.TerminalDisposition)
	}
	if err := Validate(state); err != nil {
		t.Fatalf("completed state invalid: %v", err)
	}
}

func TestCommittedMultiFileMutationAdvancesExactlyOnceAndInvalidatesVerification(t *testing.T) {
	state := completionReadyState(t)
	state = mustReduce(t, state, IntentOpened{ID: "patch-intent", ExecutionSequence: 2, ActionFingerprint: "patch-call-1"})
	facts := ToolExecutionFacts{
		ToolID: "ApplyPatch", IntentID: "patch-intent", ExecutionSequence: 2, Invoked: true, EffectScope: EffectScopeWorkspaceWrite,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationCommitted,
		VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-0", AfterDigest: "workspace-1", ActionFingerprint: "patch-call-1",
		Files: []FileFact{
			{Identity: "file-a", BeforeDigest: "a0", AfterDigest: "a1", MutationOutcome: MutationCommitted},
			{Identity: "file-b", BeforeDigest: "b0", AfterDigest: "b1", MutationOutcome: MutationCommitted},
		},
	}
	beforeRevision := state.LedgerRevision
	mutated, effects := mustReduceEffects(t, state, ToolExecuted{Facts: facts})
	if mutated.MutationEpoch != 1 || mutated.WorkspaceRevision != 1 || mutated.LedgerRevision != beforeRevision+1 {
		t.Fatalf("mutation generations = mutation:%d workspace:%d ledger:%d", mutated.MutationEpoch, mutated.WorkspaceRevision, mutated.LedgerRevision)
	}
	if !effects.MutationAdvanced || !effects.VerificationInvalidated || mutated.VerificationReceipt != nil || mutated.VerifiedEpoch != 0 {
		t.Fatalf("mutation invalidation = %+v / receipt=%+v verified=%d", effects, mutated.VerificationReceipt, mutated.VerifiedEpoch)
	}
	if len(mutated.Files) != 2 || len(mutated.PendingIntents) != 0 {
		t.Fatalf("file/intent ledger = files:%+v intents:%+v", mutated.Files, mutated.PendingIntents)
	}
	for _, condition := range append(append([]ConditionState{}, mutated.Acceptance.Criteria...), mutated.Invariants.Conditions...) {
		if condition.Outcome != ConditionPending {
			t.Fatalf("condition survived mutation: %+v", condition)
		}
	}
	if mutated.Actions[len(mutated.Actions)-1].MutationEpochBefore != 0 || mutated.Actions[len(mutated.Actions)-1].MutationEpochAfter != 1 {
		t.Fatalf("action epoch record = %+v", mutated.Actions[len(mutated.Actions)-1])
	}

	replayed, replayEffects := mustReduceEffects(t, mutated, ToolExecuted{Facts: facts})
	if !replayEffects.DuplicateAction || replayEffects.StateChanged || !reflect.DeepEqual(replayed, mutated) {
		t.Fatalf("idempotent replay = %+v / changed=%t", replayEffects, !reflect.DeepEqual(replayed, mutated))
	}
	conflict := facts
	conflict.AfterDigest = "workspace-conflict"
	assertTransitionError(t, mutated, ToolExecuted{Facts: conflict}, ErrorConflictingReplay)
}

func TestSuccessfulVerificationOfPostMutationDigestCatchesUpInSameReduction(t *testing.T) {
	state := testState(t, LedgerLimits{})
	next, effects := mustReduceEffects(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "Run", ExecutionSequence: 1, Invoked: true, EffectScope: EffectScopeWorkspaceAndExternal,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationCommitted,
		VerificationKind: VerificationTargetedTest, VerificationOutcome: VerificationPassed, VerificationEpoch: 1,
		VerificationConfigDigest: "verification-config",
		BeforeDigest:             "workspace-0", AfterDigest: "workspace-1", EvidenceDigest: "post-mutation-tests",
		ActionFingerprint: "format-and-test",
		Files:             []FileFact{{Identity: "formatted-file", BeforeDigest: "file-0", AfterDigest: "file-1", MutationOutcome: MutationCommitted}},
	}})
	if !effects.MutationAdvanced || effects.ReceiptDisposition != ReceiptIssued || next.MutationEpoch != 1 || next.VerifiedEpoch != 1 ||
		next.VerificationReceipt == nil || next.VerificationReceipt.WorkspaceDigest != "workspace-1" ||
		next.VerificationReceipt.IssuedRevision != next.LedgerRevision {
		t.Fatalf("combined mutation/verification = %+v / %+v", effects, next)
	}
	if err := Validate(next); err != nil {
		t.Fatalf("combined state invalid: %v", err)
	}
}

func TestExecutionSequenceRejectsReplayAfterActionLedgerEviction(t *testing.T) {
	limits := DefaultLedgerLimits()
	limits.Actions = 2
	state := testState(t, limits)
	a := ToolExecutionFacts{
		ToolID: "ApplyPatch", ExecutionSequence: 1, Invoked: true, EffectScope: EffectScopeWorkspaceWrite,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationCommitted,
		VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-0", AfterDigest: "workspace-1", ActionFingerprint: "semantic-patch-a",
	}
	state = mustReduce(t, state, ToolExecuted{Facts: a})
	state = mustReduce(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "ApplyPatch", ExecutionSequence: 2, Invoked: true, EffectScope: EffectScopeWorkspaceWrite,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationCommitted,
		VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-1", AfterDigest: "workspace-0", ActionFingerprint: "semantic-patch-b",
	}})
	state = mustReduce(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "Inspect", ExecutionSequence: 3, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationNone,
		VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-0", AfterDigest: "workspace-0", ActionFingerprint: "semantic-inspect-c",
	}})
	if len(state.Actions) != 2 || state.Actions[0].ExecutionSequence != 2 {
		t.Fatalf("action A was not evicted: %+v", state.Actions)
	}
	assertTransitionError(t, state, ToolExecuted{Facts: a}, ErrorStaleExecution)

	newExecution := ToolExecutionFacts{
		ToolID: "Inspect", ExecutionSequence: 4, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationNone,
		VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-0", AfterDigest: "workspace-0", ActionFingerprint: "semantic-patch-a-execution-2",
	}
	next := mustReduce(t, state, ToolExecuted{Facts: newExecution})
	if next.LastExecutionSequence != 4 || next.MutationEpoch != state.MutationEpoch {
		t.Fatalf("new execution of repeated semantic action = %+v", next)
	}
}

func TestExecutionSequenceRejectsGapsAndFingerprintReuse(t *testing.T) {
	state := testState(t, LedgerLimits{})
	first := ToolExecutionFacts{
		ToolID: "Inspect", ExecutionSequence: 1, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationNone,
		VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-0", AfterDigest: "workspace-0", ActionFingerprint: "physical-call-1",
	}
	gap := first
	gap.ExecutionSequence = 2
	gap.ActionFingerprint = "physical-call-2"
	assertTransitionError(t, state, ToolExecuted{Facts: gap}, ErrorExecutionGap)
	state = mustReduce(t, state, ToolExecuted{Facts: first})
	reused := first
	reused.ExecutionSequence = 2
	assertTransitionError(t, state, ToolExecuted{Facts: reused}, ErrorConflictingReplay)
}

func TestIntentBindingRejectsDelayedResultAfterIDReuse(t *testing.T) {
	state := testState(t, LedgerLimits{})
	state = mustReduce(t, state, IntentOpened{ID: "intent-x", ExecutionSequence: 1, ActionFingerprint: "action-old"})
	state = mustReduce(t, state, IntentClosed{
		ID: "intent-x", ExecutionSequence: 1, ActionFingerprint: "action-old", Resolution: IntentAbandoned,
	})
	state = mustReduce(t, state, IntentOpened{ID: "intent-x", ExecutionSequence: 1, ActionFingerprint: "action-new"})
	assertTransitionError(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "Inspect", IntentID: "intent-x", ExecutionSequence: 1, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationNone,
		VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-0", AfterDigest: "workspace-0", ActionFingerprint: "action-old",
	}}, ErrorIntentMismatch)
}

func TestPossibleBashSideEffectsAdvanceAndTriggerBoundedStagnationSignals(t *testing.T) {
	limits := DefaultLedgerLimits()
	limits.RepeatedFailureTrigger = 3
	limits.NoProgressTrigger = 3
	state := testState(t, limits)
	var effects ReduceEffects
	for index := 1; index <= 3; index++ {
		before := Digest("")
		if state.WorkspaceDigestKnown {
			before = state.WorkspaceDigest
		}
		state, effects = mustReduceEffects(t, state, ToolExecuted{Facts: ToolExecutionFacts{
			ToolID: "Bash", ExecutionSequence: uint64(index), Invoked: true, EffectScope: EffectScopeWorkspaceAndExternal,
			ExecutionOutcome: ExecutionFailed, MutationOutcome: MutationPossible,
			VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
			BeforeDigest: before, ActionFingerprint: Fingerprint(fmt.Sprintf("bash-call-%d", index)),
			FailureFingerprint: "same-bash-failure",
		}})
	}
	if state.MutationEpoch != 3 || state.WorkspaceRevision != 3 || state.WorkspaceDigestKnown {
		t.Fatalf("possible mutation state = epoch:%d workspace-revision:%d known:%t", state.MutationEpoch, state.WorkspaceRevision, state.WorkspaceDigestKnown)
	}
	if state.ConsecutiveFailures != 3 || state.NoProgressStreak != 3 || !effects.RepeatedFailure || !effects.NoProgress ||
		effects.RecommendedDisposition != TerminalBlocked {
		t.Fatalf("stagnation state/effects = failures:%d no-progress:%d %+v", state.ConsecutiveFailures, state.NoProgressStreak, effects)
	}
	if len(state.Failures) != 3 || state.Failures[2].RecentOccurrence != 3 || state.Failures[2].ConsecutiveOccurrence != 3 {
		t.Fatalf("failure ledger = %+v", state.Failures)
	}
	if state.VerifiedEpoch > state.MutationEpoch {
		t.Fatalf("verified epoch %d exceeded mutation epoch %d", state.VerifiedEpoch, state.MutationEpoch)
	}
}

func TestRolledBackPatchDoesNotAdvanceOrInvalidateCurrentState(t *testing.T) {
	state := completionReadyState(t)
	receipt := *state.VerificationReceipt
	rolledBack, effects := mustReduceEffects(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "ApplyPatch", ExecutionSequence: 2, Invoked: true, EffectScope: EffectScopeWorkspaceWrite,
		ExecutionOutcome: ExecutionFailed, FailureFingerprint: "patch-context-miss",
		MutationOutcome: MutationNone, VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-0", AfterDigest: "workspace-0", ActionFingerprint: "patch-rollback-1",
		Files: []FileFact{{Identity: "file-a", BeforeDigest: "a0", AfterDigest: "a0", MutationOutcome: MutationNone}},
	}})
	if effects.MutationAdvanced || effects.VerificationInvalidated || rolledBack.MutationEpoch != 0 || rolledBack.WorkspaceRevision != 0 {
		t.Fatalf("rollback advanced state = %+v / %+v", effects, rolledBack)
	}
	if rolledBack.VerificationReceipt == nil || *rolledBack.VerificationReceipt != receipt {
		t.Fatalf("rollback invalidated receipt = %+v", rolledBack.VerificationReceipt)
	}
	if decision := CompletionGate(rolledBack); !decision.Allowed {
		t.Fatalf("rollback changed completion readiness = %+v", decision)
	}
}

func TestCompletionGateAllowsOneOptionalFailureButBlocksRepeatedFingerprint(t *testing.T) {
	state := completionReadyState(t)
	for index := 1; index <= int(state.Limits.RepeatedFailureTrigger); index++ {
		state = mustReduce(t, state, ToolExecuted{Facts: ToolExecutionFacts{
			ToolID: "Inspect", ExecutionSequence: uint64(index + 1), Invoked: true, EffectScope: EffectScopeWorkspaceRead,
			ExecutionOutcome: ExecutionFailed, FailureFingerprint: "unchanged-inspect-failure",
			MutationOutcome: MutationNone, VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
			BeforeDigest: "workspace-0", AfterDigest: "workspace-0",
			ActionFingerprint: Fingerprint(fmt.Sprintf("optional-inspect-%d", index)),
		}})
		decision := CompletionGate(state)
		if index < int(state.Limits.RepeatedFailureTrigger) {
			if !decision.Allowed || decision.Blocker != CompletionReady {
				t.Fatalf("optional failure %d invalidated verified completion: %+v", index, decision)
			}
			continue
		}
		if decision.Allowed || decision.Blocker != CompletionRepeatedFailure {
			t.Fatalf("repeated deterministic failure did not block completion: %+v", decision)
		}
	}
}

func TestStaleAndCrossContractReceiptsCannotCatchUpVerification(t *testing.T) {
	state := completionReadyState(t)
	oldReceipt := *state.VerificationReceipt
	mutated := mustReduce(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "ApplyPatch", ExecutionSequence: 2, Invoked: true, EffectScope: EffectScopeWorkspaceWrite,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationCommitted,
		VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-0", AfterDigest: "workspace-1", ActionFingerprint: "patch-after-receipt",
	}})
	beforeRevision := mutated.LedgerRevision
	presented, effects := mustReduceEffects(t, mutated, ReceiptPresented{Receipt: oldReceipt})
	if effects.ReceiptDisposition != ReceiptStale || effects.StateChanged || presented.LedgerRevision != beforeRevision || presented.VerificationReceipt != nil {
		t.Fatalf("stale receipt presentation = %+v / %+v", effects, presented)
	}

	delayed, delayedEffects := mustReduceEffects(t, mutated, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "Run", ExecutionSequence: 3, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationNone,
		VerificationKind: VerificationTargetedTest, VerificationOutcome: VerificationPassed, VerificationEpoch: 0,
		VerificationConfigDigest: "verification-config",
		BeforeDigest:             "workspace-1", AfterDigest: "workspace-1", EvidenceDigest: "old-test-result",
		ActionFingerprint: "delayed-verification",
	}})
	if !delayedEffects.StaleVerification || delayedEffects.ReceiptDisposition != ReceiptStale || delayed.VerificationReceipt != nil {
		t.Fatalf("delayed verification = %+v / %+v", delayedEffects, delayed.VerificationReceipt)
	}

	otherTask, err := NewState(StateSpec{
		WorkspaceInstanceID: "workspace-instance", WorkspaceDigest: "workspace-0", TaskDigest: "different-task",
		VerificationConfigDigest: "verification-config", AcceptanceCriteria: []string{"criterion-build"},
	})
	if err != nil {
		t.Fatal(err)
	}
	otherTask, effects = mustReduceEffects(t, otherTask, ReceiptPresented{Receipt: oldReceipt})
	if effects.ReceiptDisposition != ReceiptStale || otherTask.VerificationReceipt != nil {
		t.Fatalf("cross-task receipt accepted = %+v / %+v", effects, otherTask.VerificationReceipt)
	}

	corrupt := oldReceipt
	corrupt.EvidenceDigest = "tampered"
	assertTransitionError(t, mutated, ReceiptPresented{Receipt: corrupt}, ErrorInvalidEvent)
}

func TestFailedCurrentVerificationRevokesEarlierReceiptAtSameEpoch(t *testing.T) {
	state := completionReadyState(t)
	passingReceipt := *state.VerificationReceipt
	failed := mustReduce(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "Run", ExecutionSequence: 2, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
		ExecutionOutcome: ExecutionFailed, FailureFingerprint: "test-suite-failed", MutationOutcome: MutationNone,
		VerificationKind: VerificationTargetedTest, VerificationOutcome: VerificationFailed, VerificationEpoch: 0,
		VerificationConfigDigest: "verification-config",
		BeforeDigest:             "workspace-0", AfterDigest: "workspace-0", EvidenceDigest: "failing-test-evidence",
		ActionFingerprint: "verify-current-failed",
	}})
	if failed.VerificationReceipt != nil || failed.VerificationInvalidatedRevision != failed.LedgerRevision {
		t.Fatalf("failed verification did not revoke receipt: %+v", failed)
	}
	restored, effects := mustReduceEffects(t, failed, ReceiptPresented{Receipt: passingReceipt})
	if effects.ReceiptDisposition != ReceiptStale || restored.VerificationReceipt != nil || effects.StateChanged {
		t.Fatalf("revoked receipt was restored: %+v / %+v", effects, restored.VerificationReceipt)
	}
}

func TestVerificationGenerationPreventsSiblingBranchReceiptResurrection(t *testing.T) {
	base := testState(t, LedgerLimits{})
	passing := mustReduce(t, base, ToolExecuted{Facts: passingVerification(
		"branch-pass", "", 1, 0, "workspace-0", "branch-pass-evidence",
	)})
	receipt := *passing.VerificationReceipt
	failing := mustReduce(t, base, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "Run", ExecutionSequence: 1, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
		ExecutionOutcome: ExecutionFailed, FailureFingerprint: "branch-test-failed", MutationOutcome: MutationNone,
		VerificationKind: VerificationTargetedTest, VerificationOutcome: VerificationFailed, VerificationEpoch: 0,
		VerificationConfigDigest: "verification-config",
		BeforeDigest:             "workspace-0", AfterDigest: "workspace-0", EvidenceDigest: "branch-fail-evidence",
		ActionFingerprint: "branch-fail",
	}})
	if passing.VerificationGeneration == failing.VerificationGeneration {
		t.Fatalf("sibling verification generations did not diverge")
	}
	next, effects := mustReduceEffects(t, failing, ReceiptPresented{Receipt: receipt})
	if effects.ReceiptDisposition != ReceiptStale || next.VerificationReceipt != nil {
		t.Fatalf("sibling receipt resurrected = %+v / %+v", effects, next.VerificationReceipt)
	}
}

func TestFirstPartyPersistedReceiptCanBeReattachedIdempotently(t *testing.T) {
	source := completionReadyState(t)
	receipt := *source.VerificationReceipt
	resumed := source.Clone()
	resumed.VerificationReceipt = nil
	resumed.VerifiedEpoch = 0
	if err := Validate(resumed); err != nil {
		t.Fatalf("resume fixture invalid: %v", err)
	}
	beforeRevision := resumed.LedgerRevision
	reattached, effects := mustReduceEffects(t, resumed, ReceiptPresented{Receipt: receipt})
	if effects.ReceiptDisposition != ReceiptAccepted || !effects.StateChanged || !effects.Progress.Verification ||
		reattached.VerificationReceipt == nil || reattached.LedgerRevision != beforeRevision+1 {
		t.Fatalf("receipt reattach = %+v / %+v", effects, reattached)
	}
	idempotent, repeatedEffects := mustReduceEffects(t, reattached, ReceiptPresented{Receipt: receipt})
	if repeatedEffects.ReceiptDisposition != ReceiptAccepted || repeatedEffects.StateChanged || !reflect.DeepEqual(idempotent, reattached) {
		t.Fatalf("receipt replay = %+v / changed=%t", repeatedEffects, !reflect.DeepEqual(idempotent, reattached))
	}
}

func TestCompletedFlightRejectsDifferentReceipt(t *testing.T) {
	state := completionReadyState(t)
	oldReceipt := *state.VerificationReceipt
	state = mustReduce(t, state, ToolExecuted{Facts: passingVerification(
		"verify-newer", "", 2, 0, "workspace-0", "newer-evidence",
	)})
	state = mustReduce(t, state, TerminalRequested{Disposition: TerminalCompleted})
	assertTransitionError(t, state, ReceiptPresented{Receipt: oldReceipt}, ErrorTerminalState)
}

func TestUnknownWorkspaceMustBeReconciledBeforeCurrentVerification(t *testing.T) {
	state := testState(t, LedgerLimits{})
	state = mustReduce(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "Bash", ExecutionSequence: 1, Invoked: true, EffectScope: EffectScopeWorkspaceAndExternal,
		ExecutionOutcome: ExecutionFailed, FailureFingerprint: "unknown-side-effects",
		MutationOutcome: MutationPossible, VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-0", ActionFingerprint: "bash-unknown",
	}})
	if decision := CompletionGate(state); decision.Blocker != CompletionWorkspaceUnknown {
		t.Fatalf("unknown workspace gate = %+v", decision)
	}
	reconciled, effects := mustReduceEffects(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "Inspect", ExecutionSequence: 2, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationNone,
		VerificationKind: VerificationObservation, VerificationOutcome: VerificationPassed, VerificationEpoch: 1,
		BeforeDigest: "workspace-1", AfterDigest: "workspace-1", EvidenceDigest: "reconcile-evidence",
		ActionFingerprint: "inspect-reconcile",
	}})
	if !effects.WorkspaceReconciled || !reconciled.WorkspaceDigestKnown || reconciled.WorkspaceDigest != "workspace-1" || reconciled.WorkspaceRevision != 2 {
		t.Fatalf("reconciliation = %+v / %+v", effects, reconciled)
	}
	verified, effects := mustReduceEffects(t, reconciled, ToolExecuted{Facts: passingVerification(
		"verify-1", "", 3, 1, "workspace-1", "verification-evidence-1",
	)})
	if effects.ReceiptDisposition != ReceiptIssued || verified.VerifiedEpoch != verified.MutationEpoch || verified.VerificationReceipt == nil {
		t.Fatalf("post-reconciliation verification = %+v / %+v", effects, verified.VerificationReceipt)
	}
}

func TestConditionEvaluationsAreEpochBoundAndLedgersRemainBounded(t *testing.T) {
	limits := DefaultLedgerLimits()
	limits.Actions = 2
	limits.Failures = 2
	limits.Evidence = 2
	limits.Files = 2
	state := testState(t, limits)
	for index := 1; index <= 4; index++ {
		state = mustReduce(t, state, ToolExecuted{Facts: ToolExecutionFacts{
			ToolID: "Inspect", ExecutionSequence: uint64(index), Invoked: true, EffectScope: EffectScopeWorkspaceRead,
			ExecutionOutcome: ExecutionFailed, FailureFingerprint: Fingerprint(fmt.Sprintf("failure-%d", index)),
			MutationOutcome: MutationNone, VerificationKind: VerificationObservation, VerificationOutcome: VerificationInconclusive,
			BeforeDigest: "workspace-0", AfterDigest: "workspace-0", EvidenceDigest: Digest(fmt.Sprintf("evidence-%d", index)),
			ActionFingerprint: Fingerprint(fmt.Sprintf("inspect-%d", index)),
			Files:             []FileFact{{Identity: Fingerprint(fmt.Sprintf("file-%d", index)), MutationOutcome: MutationNone}},
		}})
	}
	if len(state.Actions) != 2 || len(state.Failures) != 2 || len(state.Evidence) != 2 || len(state.Files) != 2 {
		t.Fatalf("bounded ledgers = actions:%d failures:%d evidence:%d files:%d", len(state.Actions), len(state.Failures), len(state.Evidence), len(state.Files))
	}
	if state.Actions[0].ActionFingerprint != "inspect-3" || state.Actions[1].ActionFingerprint != "inspect-4" ||
		state.Files[0].Identity != "file-3" || state.Files[1].Identity != "file-4" {
		t.Fatalf("bounded ledgers did not retain newest records: %+v / %+v", state.Actions, state.Files)
	}

	mutated := mustReduce(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "ApplyPatch", ExecutionSequence: 5, Invoked: true, EffectScope: EffectScopeWorkspaceWrite,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationCommitted,
		VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-0", AfterDigest: "workspace-1", ActionFingerprint: "patch-condition-epoch",
	}})
	stale, effects := mustReduceEffects(t, mutated, ConditionsEvaluated{Acceptance: []ConditionEvaluation{{
		ID: "criterion-build", Outcome: ConditionSatisfied, MutationEpoch: 0,
		WorkspaceDigest: "workspace-0", EvidenceDigest: "stale-acceptance",
	}}})
	if effects.StaleConditionEvaluations != 1 || effects.ConditionsUpdated != 0 || stale.Acceptance.Criteria[0].Outcome != ConditionPending {
		t.Fatalf("stale condition evaluation = %+v / %+v", effects, stale.Acceptance.Criteria[0])
	}
	current := mustReduce(t, stale, ConditionsEvaluated{Acceptance: []ConditionEvaluation{
		currentCondition("criterion-build", ConditionSatisfied, stale, "current-build"),
		currentCondition("criterion-tests", ConditionSatisfied, stale, "current-tests"),
	}})
	if current.Acceptance.Criteria[0].Outcome != ConditionSatisfied || current.Acceptance.Criteria[0].MutationEpoch != 1 {
		t.Fatalf("current condition was not accepted: %+v", current.Acceptance.Criteria)
	}
}

func TestReduceIsDetachedAndDeterministic(t *testing.T) {
	state := testState(t, LedgerLimits{})
	event := IntentOpened{ID: "intent-1", ExecutionSequence: 1, ActionFingerprint: "action-plan"}
	first, firstEffects, err := Reduce(state, event)
	if err != nil {
		t.Fatal(err)
	}
	second, secondEffects, err := Reduce(state, event)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("non-deterministic reduction = %+v/%+v vs %+v/%+v", first, firstEffects, second, secondEffects)
	}
	first.PendingIntents[0].ID = "mutated-output"
	if len(state.PendingIntents) != 0 || second.PendingIntents[0].ID != "intent-1" {
		t.Fatalf("reducer alias leaked into input or sibling output")
	}
}

func TestValidateRejectsVerifiedEpochAheadOfMutationEpoch(t *testing.T) {
	state := testState(t, LedgerLimits{})
	state.VerifiedEpoch = 1
	assertErrorCode(t, Validate(state), ErrorInvalidState)
}

func TestPassingVerificationRequiresBothWorkspaceBoundaryDigests(t *testing.T) {
	state := testState(t, LedgerLimits{})
	facts := passingVerification("missing-after", "", 1, 0, "workspace-0", "verification-evidence")
	facts.AfterDigest = ""
	assertTransitionError(t, state, ToolExecuted{Facts: facts}, ErrorInvalidFacts)
}

func TestVerificationWithDifferentPolicyDigestCannotIssueReceipt(t *testing.T) {
	state := testState(t, LedgerLimits{})
	facts := passingVerification("wrong-config", "", 1, 0, "workspace-0", "verification-evidence")
	facts.VerificationConfigDigest = "different-verification-config"
	next, effects := mustReduceEffects(t, state, ToolExecuted{Facts: facts})
	if effects.ReceiptDisposition != ReceiptStale || !effects.StaleVerification || next.VerificationReceipt != nil {
		t.Fatalf("mismatched verification config = %+v / %+v", effects, next.VerificationReceipt)
	}
}

func TestFailedEvidenceDoesNotResetStagnationAndConditionProgressDoes(t *testing.T) {
	state := testState(t, LedgerLimits{})
	state.NoProgressStreak = 2
	next, effects := mustReduceEffects(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "Inspect", ExecutionSequence: 1, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
		ExecutionOutcome: ExecutionFailed, FailureFingerprint: "failed-inspection", MutationOutcome: MutationNone,
		VerificationKind: VerificationObservation, VerificationOutcome: VerificationInconclusive, VerificationEpoch: 0,
		BeforeDigest: "workspace-0", AfterDigest: "workspace-0", EvidenceDigest: "novel-failed-evidence",
		ActionFingerprint: "failed-inspection-1",
	}})
	if effects.Progress.Evidence || next.NoProgressStreak != 3 {
		t.Fatalf("failed evidence counted as progress = %+v / streak=%d", effects, next.NoProgressStreak)
	}
	next = mustReduce(t, next, ConditionsEvaluated{Acceptance: []ConditionEvaluation{
		currentCondition("criterion-build", ConditionSatisfied, next, "condition-progress"),
	}})
	if next.NoProgressStreak != 0 {
		t.Fatalf("condition progress did not reset streak: %d", next.NoProgressStreak)
	}
}

func TestSameConditionEvidenceCannotSupportConflictingOutcomes(t *testing.T) {
	state := testState(t, LedgerLimits{})
	state = mustReduce(t, state, ConditionsEvaluated{Acceptance: []ConditionEvaluation{
		currentCondition("criterion-build", ConditionUnsatisfied, state, "same-evidence"),
	}})
	assertTransitionError(t, state, ConditionsEvaluated{Acceptance: []ConditionEvaluation{
		currentCondition("criterion-build", ConditionSatisfied, state, "same-evidence"),
	}}, ErrorConflictingEvidence)
}

func TestFileDigestChangeCannotBeReportedAsNoMutation(t *testing.T) {
	state := testState(t, LedgerLimits{})
	assertTransitionError(t, state, ToolExecuted{Facts: ToolExecutionFacts{
		ToolID: "ApplyPatch", ExecutionSequence: 1, Invoked: true, EffectScope: EffectScopeWorkspaceWrite,
		ExecutionOutcome: ExecutionFailed, FailureFingerprint: "invalid-file-facts", MutationOutcome: MutationNone,
		VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
		BeforeDigest: "workspace-0", AfterDigest: "workspace-0", ActionFingerprint: "invalid-file-no-mutation",
		Files: []FileFact{{Identity: "file-a", BeforeDigest: "file-0", AfterDigest: "file-1", MutationOutcome: MutationNone}},
	}}, ErrorInvalidFacts)
}

func testState(t *testing.T, limits LedgerLimits) FlightState {
	t.Helper()
	state, err := NewState(StateSpec{
		WorkspaceInstanceID: "workspace-instance", WorkspaceDigest: "workspace-0",
		TaskDigest: "task-contract", VerificationConfigDigest: "verification-config",
		AcceptanceCriteria: []string{"criterion-build", "criterion-tests"},
		Invariants:         []string{"invariant-i18n"}, Limits: limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func completionReadyState(t *testing.T) FlightState {
	t.Helper()
	state := testState(t, LedgerLimits{})
	state = mustReduce(t, state, ToolExecuted{Facts: passingVerification(
		"verify-ready", "", 1, 0, "workspace-0", "verification-ready-evidence",
	)})
	state = mustReduce(t, state, ConditionsEvaluated{
		Acceptance: []ConditionEvaluation{
			currentCondition("criterion-build", ConditionSatisfied, state, "build-ready"),
			currentCondition("criterion-tests", ConditionSatisfied, state, "tests-ready"),
		},
		Invariants: []ConditionEvaluation{
			currentCondition("invariant-i18n", ConditionSatisfied, state, "i18n-ready"),
		},
	})
	if decision := CompletionGate(state); !decision.Allowed {
		t.Fatalf("test fixture not completion-ready: %+v", decision)
	}
	return state
}

func passingVerification(action, intent string, sequence uint64, epoch Epoch, workspace, evidence Digest) ToolExecutionFacts {
	return ToolExecutionFacts{
		ToolID: "Run", IntentID: intent, ExecutionSequence: sequence, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
		ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationNone,
		VerificationKind: VerificationTargetedTest, VerificationOutcome: VerificationPassed, VerificationEpoch: epoch,
		VerificationConfigDigest: "verification-config",
		BeforeDigest:             workspace, AfterDigest: workspace, EvidenceDigest: evidence, ActionFingerprint: Fingerprint(action),
	}
}

func currentCondition(id string, outcome ConditionOutcome, state FlightState, evidence Digest) ConditionEvaluation {
	return ConditionEvaluation{
		ID: id, Outcome: outcome, MutationEpoch: state.MutationEpoch,
		WorkspaceDigest: state.WorkspaceDigest, EvidenceDigest: evidence,
	}
}

func mustReduce(t *testing.T, state FlightState, event Event) FlightState {
	t.Helper()
	next, _, err := Reduce(state, event)
	if err != nil {
		t.Fatalf("Reduce(%T): %v", event, err)
	}
	return next
}

func mustReduceEffects(t *testing.T, state FlightState, event Event) (FlightState, ReduceEffects) {
	t.Helper()
	next, effects, err := Reduce(state, event)
	if err != nil {
		t.Fatalf("Reduce(%T): %v", event, err)
	}
	return next, effects
}

func assertTransitionError(t *testing.T, state FlightState, event Event, code ErrorCode) {
	t.Helper()
	next, effects, err := Reduce(state, event)
	assertErrorCode(t, err, code)
	if !reflect.DeepEqual(next, state) || effects.StateChanged {
		t.Fatalf("rejected transition changed state/effects: %+v / %+v", next, effects)
	}
}

func assertErrorCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	var transition *TransitionError
	if !errors.As(err, &transition) || transition.Code != code {
		t.Fatalf("error = %v, want code %s", err, code)
	}
}
