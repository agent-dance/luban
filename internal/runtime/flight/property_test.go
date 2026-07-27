package flight

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

func TestRandomizedReducerSequencesPreserveHardInvariants(t *testing.T) {
	for seed := int64(0); seed < 32; seed++ {
		random := rand.New(rand.NewSource(seed))
		state := testState(t, LedgerLimits{})
		for step := 0; step < 256; step++ {
			event := randomizedEvent(state, random.Intn(8), seed, step)
			first, firstEffects, firstErr := Reduce(state, event)
			second, secondEffects, secondErr := Reduce(state, event)
			if errorCode(firstErr) != errorCode(secondErr) || !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstEffects, secondEffects) {
				t.Fatalf("seed %d step %d produced non-deterministic reduction", seed, step)
			}
			if firstErr != nil {
				if !reflect.DeepEqual(first, state) {
					t.Fatalf("seed %d step %d rejected event mutated state", seed, step)
				}
				continue
			}
			if err := Validate(first); err != nil {
				t.Fatalf("seed %d step %d invalid state: %v", seed, step, err)
			}
			if first.VerifiedEpoch > first.MutationEpoch {
				t.Fatalf("seed %d step %d verified %d > mutation %d", seed, step, first.VerifiedEpoch, first.MutationEpoch)
			}
			if len(first.Actions) > first.Limits.Actions || len(first.Failures) > first.Limits.Failures ||
				len(first.Evidence) > first.Limits.Evidence || len(first.Files) > first.Limits.Files ||
				len(first.PendingIntents) > first.Limits.Intents {
				t.Fatalf("seed %d step %d exceeded a ledger bound", seed, step)
			}
			if tool, ok := event.(ToolExecuted); ok && !firstEffects.DuplicateAction {
				switch tool.Facts.MutationOutcome {
				case MutationCommitted, MutationPossible:
					if first.MutationEpoch != state.MutationEpoch+1 {
						t.Fatalf("seed %d step %d mutation advanced by %d", seed, step, first.MutationEpoch-state.MutationEpoch)
					}
				case MutationNone:
					if first.MutationEpoch != state.MutationEpoch {
						t.Fatalf("seed %d step %d non-mutation advanced epoch", seed, step)
					}
				}
			}
			state = first
		}
	}
}

func FuzzReducerPreservesEpochAndLedgerInvariants(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7})
	f.Add([]byte{2, 2, 2, 4, 1, 5, 4, 6, 7, 7})
	f.Add([]byte{6, 0, 6, 1, 3, 4, 5})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256 {
			data = data[:256]
		}
		state, err := NewState(StateSpec{
			WorkspaceInstanceID: "fuzz-workspace", WorkspaceDigest: "workspace-0",
			TaskDigest: "fuzz-task", VerificationConfigDigest: "fuzz-config",
			AcceptanceCriteria: []string{"criterion-build", "criterion-tests"}, Invariants: []string{"invariant-i18n"},
		})
		if err != nil {
			t.Fatal(err)
		}
		for step, value := range data {
			event := randomizedEvent(state, int(value%8), int64(value), step)
			next, effects, reduceErr := Reduce(state, event)
			if reduceErr != nil {
				if !reflect.DeepEqual(next, state) || effects.StateChanged {
					t.Fatalf("rejected event changed state at step %d", step)
				}
				continue
			}
			if err := Validate(next); err != nil {
				t.Fatalf("invalid state at step %d: %v", step, err)
			}
			if next.VerifiedEpoch > next.MutationEpoch {
				t.Fatalf("verified epoch exceeded mutation epoch at step %d", step)
			}
			state = next
		}
	})
}

func randomizedEvent(state FlightState, choice int, seed int64, step int) Event {
	action := Fingerprint(fmt.Sprintf("action-%d-%d", seed, step))
	sequence := state.LastExecutionSequence + 1
	evidence := Digest(fmt.Sprintf("evidence-%d-%d", seed, step))
	after := Digest(fmt.Sprintf("workspace-%d-%d", seed, step))
	before := Digest("")
	if state.WorkspaceDigestKnown {
		before = state.WorkspaceDigest
	}
	if len(state.PendingIntents) > 0 && choice != 6 {
		intent := state.PendingIntents[0]
		return IntentClosed{
			ID: intent.ID, ExecutionSequence: intent.ExecutionSequence,
			ActionFingerprint: intent.ActionFingerprint, Resolution: IntentAbandoned,
		}
	}
	switch choice {
	case 0:
		if !state.WorkspaceDigestKnown {
			before = after
		}
		return ToolExecuted{Facts: ToolExecutionFacts{
			ToolID: "Inspect", ExecutionSequence: sequence, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
			ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationNone,
			VerificationKind: VerificationObservation, VerificationOutcome: VerificationPassed, VerificationEpoch: state.MutationEpoch,
			BeforeDigest: before, AfterDigest: before, EvidenceDigest: evidence, ActionFingerprint: action,
		}}
	case 1:
		return ToolExecuted{Facts: ToolExecutionFacts{
			ToolID: "ApplyPatch", ExecutionSequence: sequence, Invoked: true, EffectScope: EffectScopeWorkspaceWrite,
			ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationCommitted,
			VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
			BeforeDigest: before, AfterDigest: after, ActionFingerprint: action,
			Files: []FileFact{{Identity: Fingerprint(fmt.Sprintf("file-%d-%d", seed, step)), MutationOutcome: MutationCommitted}},
		}}
	case 2:
		return ToolExecuted{Facts: ToolExecutionFacts{
			ToolID: "Bash", ExecutionSequence: sequence, Invoked: true, EffectScope: EffectScopeWorkspaceAndExternal,
			ExecutionOutcome: ExecutionFailed, FailureFingerprint: "bash-side-effect-failure", MutationOutcome: MutationPossible,
			VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
			BeforeDigest: before, ActionFingerprint: action,
		}}
	case 3:
		return ToolExecuted{Facts: ToolExecutionFacts{
			ToolID: "ApplyPatch", ExecutionSequence: sequence, Invoked: true, EffectScope: EffectScopeWorkspaceWrite,
			ExecutionOutcome: ExecutionFailed, FailureFingerprint: "patch-rollback", MutationOutcome: MutationNone,
			VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
			BeforeDigest: before, AfterDigest: before, ActionFingerprint: action,
		}}
	case 4:
		epoch := state.MutationEpoch
		if step%3 == 0 && epoch > 0 {
			epoch--
		}
		return ToolExecuted{Facts: ToolExecutionFacts{
			ToolID: "Run", ExecutionSequence: sequence, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
			ExecutionOutcome: ExecutionSucceeded, MutationOutcome: MutationNone,
			VerificationKind: VerificationTargetedTest, VerificationOutcome: VerificationPassed, VerificationEpoch: epoch,
			VerificationConfigDigest: "verification-config",
			BeforeDigest:             before, AfterDigest: before, EvidenceDigest: evidence, ActionFingerprint: action,
		}}
	case 5:
		epoch := state.MutationEpoch
		workspace := before
		if step%3 == 0 && epoch > 0 {
			epoch--
			workspace = "stale-workspace"
		}
		return ConditionsEvaluated{Acceptance: []ConditionEvaluation{{
			ID: "criterion-build", Outcome: ConditionSatisfied, MutationEpoch: epoch,
			WorkspaceDigest: workspace, EvidenceDigest: evidence,
		}}}
	case 6:
		if len(state.PendingIntents) > 0 {
			return IntentClosed{
				ID: state.PendingIntents[0].ID, ExecutionSequence: state.PendingIntents[0].ExecutionSequence,
				ActionFingerprint: state.PendingIntents[0].ActionFingerprint, Resolution: IntentAbandoned,
			}
		}
		return IntentOpened{ID: fmt.Sprintf("intent-%d-%d", seed, step), ExecutionSequence: sequence, ActionFingerprint: action}
	default:
		return ToolExecuted{Facts: ToolExecutionFacts{
			ToolID: "Inspect", ExecutionSequence: sequence, Invoked: true, EffectScope: EffectScopeWorkspaceRead,
			ExecutionOutcome: ExecutionFailed, FailureFingerprint: "repeated-read-failure", MutationOutcome: MutationNone,
			VerificationKind: VerificationNone, VerificationOutcome: VerificationNotRun,
			BeforeDigest: before, AfterDigest: before, ActionFingerprint: action,
		}}
	}
}

func errorCode(err error) ErrorCode {
	if typed, ok := err.(*TransitionError); ok {
		return typed.Code
	}
	if err != nil {
		return ErrorCode(err.Error())
	}
	return ""
}
