package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	"github.com/agent-dance/luban/internal/runtime/flight"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type flightControllerFixture struct {
	controller *agenticFlightController
	ledger     *workspacerevision.Ledger
	root       string
	path       string
}

type failingFlightVerificationTool struct {
	*fusionVerificationTool
}

type planUnsafeFlightRunTool struct {
	*fusionVerificationTool
}

type successfulThenFailingFlightPatchTool struct {
	*fusionMutationTool
	calls int
}

func (t *failingFlightVerificationTool) Execute(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
	receipt, ok := workspacerevision.FromContext(ctx)
	if !ok || t.ledger.Validate(receipt) != nil {
		return types.ToolResult{
			Content: "revision mismatch", IsError: true, Outcome: types.ToolOutcomeFailed,
			Metadata: map[string]string{"verification.status": "revision_mismatch"},
		}, nil
	}
	return types.ToolResult{
		Content: "stable focused test failure", IsError: true, Outcome: types.ToolOutcomeFailed,
		Metadata: map[string]string{
			"stepCount":                  "1",
			"verification.status":        "revision_bound",
			"verification.kind":          "targeted_test",
			"verification.config_digest": string(digestFlightValues("failing-flight-verification-config")),
		},
	}, nil
}

func (t *planUnsafeFlightRunTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{
		Content: "run plan is not revision safe", IsError: true, Outcome: types.ToolOutcomeFailed,
		Metadata: map[string]string{
			"stepCount":                  "1",
			"verification.status":        "committed_unverified",
			"verification.safety_reason": "plan_not_revision_safe",
			"mutation.status":            "possible",
		},
	}, nil
}

func (t *successfulThenFailingFlightPatchTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	t.calls++
	if t.calls == 1 {
		return t.fusionMutationTool.Execute(ctx, input)
	}
	return types.ToolResult{Content: "anchor ambiguous", IsError: true, Outcome: types.ToolOutcomeFailed}, nil
}

func newFlightControllerFixture(t *testing.T) flightControllerFixture {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	reg := registry.New()
	reg.Register(parityTool{name: "Inspect", content: "inspected"})
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path})
	reg.Register(&fusionVerificationTool{ledger: ledger, path: path})
	controller, err := newAgenticFlightController(reg, QueryConfigSnapshot{
		ProjectRoot: root, CWD: root, SessionID: "flight-session",
	}, "query-1", []types.Message{types.UserMessage("change it")})
	if err != nil {
		t.Fatal(err)
	}
	if controller == nil {
		t.Fatal("Agentic V2 registry did not create a flight controller")
	}
	return flightControllerFixture{controller: controller, ledger: ledger, root: root, path: path}
}

func (fixture flightControllerFixture) patchResult(t *testing.T, id string) (types.ToolUseBlock, types.ToolResultBlock) {
	t.Helper()
	if err := os.WriteFile(fixture.path, []byte("patched\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	receipt, err := fixture.ledger.Commit(fixture.root, []string{fixture.path})
	if err != nil {
		t.Fatal(err)
	}
	use := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: id, Name: "ApplyPatch", Input: map[string]any{}}
	result := types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: id, Data: fusionMutationData{receipt: receipt},
		Outcome: types.ToolOutcomeSucceeded,
	}
	return use, result
}

func currentFlightRun(id string, outcome types.ToolOutcome) (types.ToolUseBlock, types.ToolResultBlock) {
	use := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: id, Name: "Run", Input: map[string]any{"requires_patch_commit": true}}
	result := types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: id, Outcome: outcome,
		Metadata: map[string]string{
			"verification.status":        "revision_bound",
			"verification.kind":          "targeted_test",
			"verification.config_digest": string(digestFlightValues("test-run-config")),
		},
	}
	if outcome != types.ToolOutcomeSucceeded {
		result.IsError = true
	}
	return use, result
}

func observeFlightRound(t *testing.T, controller *agenticFlightController, uses []types.ToolUseBlock, results []types.ToolResultBlock) {
	t.Helper()
	if err := controller.openToolIntents(uses); err != nil {
		t.Fatal(err)
	}
	if err := controller.observeToolRound(uses, results); err != nil {
		t.Fatal(err)
	}
}

func committedFlightToolUseEvents(t *testing.T, uses ...types.ToolUseBlock) []types.StreamEvent {
	t.Helper()
	events := []types.StreamEvent{{Type: types.EventMessageStart}}
	for index, use := range uses {
		input := use.Input
		if input == nil {
			input = map[string]any{}
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("encode flight tool input: %v", err)
		}
		events = append(events,
			types.StreamEvent{Type: types.EventContentBlockStart, Index: index, ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse, ID: use.ID, Name: use.Name,
			}},
			types.StreamEvent{Type: types.EventContentBlockDelta, Index: index, Delta: &types.ContentDelta{
				Type: "input_json_delta", PartialJSON: string(encoded),
			}},
			types.StreamEvent{Type: types.EventContentBlockStop, Index: index},
		)
	}
	return append(events,
		types.StreamEvent{Type: types.EventMessageDelta, StopReason: stopReasonForParity(types.StopReasonToolUse)},
		types.StreamEvent{Type: types.EventMessageStop},
	)
}

func TestAgenticFlightPatchFinalRunFinal(t *testing.T) {
	fixture := newFlightControllerFixture(t)
	patchUse, patchResult := fixture.patchResult(t, "patch-1")
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})

	deferred, err := fixture.controller.requestFinal()
	if err != nil {
		t.Fatal(err)
	}
	if deferred.Action != agenticFlightTerminalContinue || deferred.Disposition != flightDispositionVerificationRequired {
		t.Fatalf("premature final = %+v", deferred)
	}

	runUse, runResult := currentFlightRun("run-1", types.ToolOutcomeSucceeded)
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{runUse}, []types.ToolResultBlock{runResult})
	completed, err := fixture.controller.requestFinal()
	if err != nil {
		t.Fatal(err)
	}
	if completed.Action != agenticFlightTerminalComplete || completed.Disposition != flightDispositionCompletedVerified {
		t.Fatalf("verified final = %+v; state=%+v", completed, fixture.controller.state)
	}
	if err := fixture.controller.commitFinal(completed); err != nil {
		t.Fatal(err)
	}
	if fixture.controller.state.TerminalDisposition != flight.TerminalCompleted {
		t.Fatalf("terminal disposition = %q", fixture.controller.state.TerminalDisposition)
	}
}

func TestAgenticFlightSameResponseFusionDoesNotAddTerminalDeferral(t *testing.T) {
	fixture := newFlightControllerFixture(t)
	patchUse, patchResult := fixture.patchResult(t, "patch-fused")
	runUse, runResult := currentFlightRun("run-fused", types.ToolOutcomeSucceeded)
	observeFlightRound(t, fixture.controller,
		[]types.ToolUseBlock{patchUse, runUse},
		[]types.ToolResultBlock{patchResult, runResult},
	)
	decision, err := fixture.controller.requestFinal()
	if err != nil {
		t.Fatal(err)
	}
	if decision.Disposition != flightDispositionCompletedVerified || decision.Action != agenticFlightTerminalComplete {
		t.Fatalf("fused decision = %+v", decision)
	}
}

func TestAgenticFlightStaleAndFailedRunCannotComplete(t *testing.T) {
	for _, test := range []struct {
		name            string
		outcome         types.ToolOutcome
		metadata        map[string]string
		wantAction      agenticFlightTerminalAction
		wantDisposition string
	}{
		{name: "stale", outcome: types.ToolOutcomeFailed, metadata: map[string]string{"verification.status": "revision_mismatch"},
			wantAction: agenticFlightTerminalContinue, wantDisposition: flightDispositionVerificationRequired},
		{name: "failed", outcome: types.ToolOutcomeFailed, metadata: map[string]string{
			"verification.status":        "revision_bound",
			"verification.kind":          "targeted_test",
			"verification.config_digest": string(digestFlightValues("failed-test-run-config")),
		}, wantAction: agenticFlightTerminalBlocked, wantDisposition: flightDispositionIncompleteUnverified},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFlightControllerFixture(t)
			patchUse, patchResult := fixture.patchResult(t, "patch-"+test.name)
			observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})
			runUse := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "run-" + test.name, Name: "Run", Input: map[string]any{}}
			runResult := types.ToolResultBlock{
				Type: types.ContentTypeToolResult, ToolUseID: runUse.ID, IsError: true,
				Outcome: test.outcome, Metadata: test.metadata,
			}
			observeFlightRound(t, fixture.controller, []types.ToolUseBlock{runUse}, []types.ToolResultBlock{runResult})
			decision, err := fixture.controller.requestFinal()
			if err != nil {
				t.Fatal(err)
			}
			if decision.Action != test.wantAction || decision.Disposition != test.wantDisposition {
				t.Fatalf("failed/stale verification decision = %+v", decision)
			}
			if fixture.controller.state.TerminalDisposition == flight.TerminalCompleted {
				t.Fatal("failed/stale verification became completed")
			}
		})
	}
}

func TestAgenticFlightOptionalInspectFailureAfterVerificationCanComplete(t *testing.T) {
	fixture := newFlightControllerFixture(t)
	patchUse, patchResult := fixture.patchResult(t, "patch-before-optional-inspect")
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})
	runUse, runResult := currentFlightRun("verify-before-optional-inspect", types.ToolOutcomeSucceeded)
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{runUse}, []types.ToolResultBlock{runResult})

	inspectUse := types.ToolUseBlock{
		Type: types.ContentTypeToolUse, ID: "optional-inspect", Name: "Inspect",
		Input: map[string]any{"requests": []any{map[string]any{"id": "missing", "kind": "read", "path": "optional.txt"}}},
	}
	inspectResult := types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: inspectUse.ID, Content: "optional path unavailable",
		IsError: true, Outcome: types.ToolOutcomeFailed,
	}
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{inspectUse}, []types.ToolResultBlock{inspectResult})
	decision, err := fixture.controller.requestFinal()
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != agenticFlightTerminalComplete || decision.Disposition != flightDispositionCompletedVerified {
		t.Fatalf("one optional diagnostic failure invalidated verified completion: %+v", decision)
	}
}

func TestAgenticFlightRepeatedUnchangedVerificationFailureBlocks(t *testing.T) {
	fixture := newFlightControllerFixture(t)
	patchUse, patchResult := fixture.patchResult(t, "patch-before-repeated-run")
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})
	passedUse, passedResult := currentFlightRun("verify-before-repeated-run", types.ToolOutcomeSucceeded)
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{passedUse}, []types.ToolResultBlock{passedResult})

	for index := 1; index <= int(fixture.controller.state.Limits.RepeatedFailureTrigger); index++ {
		use, result := currentFlightRun("repeated-run-"+strconv.Itoa(index), types.ToolOutcomeFailed)
		use.Input = map[string]any{
			"steps":                 []any{map[string]any{"id": "test", "argv": []any{"go", "test", "./focused"}}},
			"requires_patch_commit": true,
		}
		result.Content = fmt.Sprintf("[test] status=failed exit=1 duration_ms=%d", index*137)
		observeFlightRound(t, fixture.controller, []types.ToolUseBlock{use}, []types.ToolResultBlock{result})
	}
	if !fixture.controller.repeatedFailureTriggered() {
		t.Fatal("presentation-only duration changes defeated deterministic failure detection")
	}
	decision, err := fixture.controller.requestFinal()
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != agenticFlightTerminalBlocked || decision.Disposition != flightDispositionBlockedRepeatedFailure ||
		decision.Blocker != flight.CompletionRepeatedFailure {
		t.Fatalf("repeated unchanged verification decision = %+v", decision)
	}
}

func TestAgenticFlightPreExecutionRunFailureIsNotInvoked(t *testing.T) {
	for _, status := range []string{"revision_mismatch", "patch_commit_required"} {
		t.Run(status, func(t *testing.T) {
			fixture := newFlightControllerFixture(t)
			patchUse, patchResult := fixture.patchResult(t, "patch-"+status)
			observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})
			epoch := fixture.controller.state.MutationEpoch
			digest := fixture.controller.state.WorkspaceDigest
			if !fixture.controller.currentRevision.Valid() {
				t.Fatal("test setup did not retain the committed patch receipt")
			}

			runUse := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "run-" + status, Name: "Run", Input: map[string]any{}}
			runResult := types.ToolResultBlock{
				Type: types.ContentTypeToolResult, ToolUseID: runUse.ID, IsError: true,
				Outcome: types.ToolOutcomeFailed, Metadata: map[string]string{"verification.status": status},
			}
			observeFlightRound(t, fixture.controller, []types.ToolUseBlock{runUse}, []types.ToolResultBlock{runResult})

			state := fixture.controller.state
			last := state.Actions[len(state.Actions)-1]
			if last.Invoked || last.MutationOutcome != flight.MutationNone || last.VerificationKind != flight.VerificationNone {
				t.Fatalf("pre-execution failure became execution evidence: %+v", last)
			}
			if state.MutationEpoch != epoch || state.WorkspaceDigest != digest || !state.WorkspaceDigestKnown || !fixture.controller.currentRevision.Valid() {
				t.Fatalf("pre-execution failure changed revision authority: state=%+v receipt=%+v", state, fixture.controller.currentRevision)
			}
		})
	}
}

func TestAgenticFlightCommittedUnverifiedInvalidatesAuthority(t *testing.T) {
	for _, outcome := range []types.ToolOutcome{types.ToolOutcomeSucceeded, types.ToolOutcomeFailed} {
		t.Run(string(outcome), func(t *testing.T) {
			fixture := newFlightControllerFixture(t)
			patchUse, patchResult := fixture.patchResult(t, "patch-before-mutating-"+string(outcome))
			observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})
			verifyUse, verifyResult := currentFlightRun("verify-before-mutating-"+string(outcome), types.ToolOutcomeSucceeded)
			observeFlightRound(t, fixture.controller, []types.ToolUseBlock{verifyUse}, []types.ToolResultBlock{verifyResult})
			if fixture.controller.state.VerificationReceipt == nil {
				t.Fatal("test setup did not issue a verification receipt")
			}
			epoch := fixture.controller.state.MutationEpoch

			runUse := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "mutating-" + string(outcome), Name: "Run", Input: map[string]any{}}
			runResult := types.ToolResultBlock{
				Type: types.ContentTypeToolResult, ToolUseID: runUse.ID, Outcome: outcome,
				Metadata: map[string]string{
					"stepCount":                  "2",
					"verification.status":        "committed_unverified",
					"verification.kind":          "targeted_test",
					"verification.config_digest": string(digestFlightValues("must-not-sign")),
				},
			}
			if outcome != types.ToolOutcomeSucceeded {
				runResult.IsError = true
			}
			observeFlightRound(t, fixture.controller, []types.ToolUseBlock{runUse}, []types.ToolResultBlock{runResult})

			state := fixture.controller.state
			last := state.Actions[len(state.Actions)-1]
			if !last.Invoked || last.MutationOutcome != flight.MutationPossible || last.EffectScope != flight.EffectScopeWorkspaceWrite || last.VerificationKind != flight.VerificationNone {
				t.Fatalf("mutating Run facts=%+v", last)
			}
			if state.MutationEpoch != epoch+1 || state.VerificationReceipt != nil || state.VerifiedEpoch >= state.MutationEpoch || state.WorkspaceDigestKnown || fixture.controller.currentRevision.Valid() {
				t.Fatalf("mutating Run retained stale completion authority: state=%+v receipt=%+v", state, fixture.controller.currentRevision)
			}
			decision, err := fixture.controller.requestFinal()
			if err != nil {
				t.Fatal(err)
			}
			if decision.Action != agenticFlightTerminalBlocked || decision.Disposition != flightDispositionIncompleteUnverified ||
				decision.Blocker != flight.CompletionWorkspaceUnknown {
				t.Fatalf("mutating Run completion decision=%+v", decision)
			}
		})
	}

	t.Run("without preceding patch", func(t *testing.T) {
		fixture := newFlightControllerFixture(t)
		runUse := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "standalone-mutating-run", Name: "Run", Input: map[string]any{}}
		runResult := types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: runUse.ID, Outcome: types.ToolOutcomeSucceeded,
			Metadata: map[string]string{"stepCount": "1", "verification.status": "committed_unverified"},
		}
		observeFlightRound(t, fixture.controller, []types.ToolUseBlock{runUse}, []types.ToolResultBlock{runResult})
		state := fixture.controller.state
		if state.MutationEpoch != 1 || state.WorkspaceDigestKnown || state.VerificationReceipt != nil || fixture.controller.currentRevision.Valid() {
			t.Fatalf("standalone mutating Run was treated as read-only: %+v", state)
		}
		last := state.Actions[len(state.Actions)-1]
		if !last.Invoked || last.MutationOutcome != flight.MutationPossible {
			t.Fatalf("standalone mutating Run facts=%+v", last)
		}
	})
}

func TestAgenticFlightFailedTransactionalPatchPreservesUnknownWorkspace(t *testing.T) {
	fixture := newFlightControllerFixture(t)
	runUse := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "plan-unsafe-run", Name: "Run", Input: map[string]any{}}
	runResult := types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: runUse.ID, IsError: true, Outcome: types.ToolOutcomeFailed,
		Metadata: map[string]string{
			"stepCount":                  "1",
			"verification.status":        "committed_unverified",
			"verification.safety_reason": "plan_not_revision_safe",
			"mutation.status":            "possible",
		},
	}
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{runUse}, []types.ToolResultBlock{runResult})
	unknownEpoch := fixture.controller.state.MutationEpoch
	if fixture.controller.state.WorkspaceDigestKnown || unknownEpoch == 0 {
		t.Fatalf("committed-unverified Run did not make workspace unknown: %+v", fixture.controller.state)
	}

	patchUse := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "failed-patch", Name: "ApplyPatch", Input: map[string]any{}}
	patchResult := types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: patchUse.ID, Content: "anchor ambiguous",
		IsError: true, Outcome: types.ToolOutcomeFailed,
	}
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})
	if fixture.controller.state.WorkspaceDigestKnown || fixture.controller.state.MutationEpoch != unknownEpoch {
		t.Fatalf("failed transactional patch changed unknown workspace state: %+v", fixture.controller.state)
	}
	if len(fixture.controller.state.PendingIntents) != 0 {
		t.Fatalf("failed transactional patch left an open intent: %+v", fixture.controller.state.PendingIntents)
	}
}

func TestAgenticFlightWorkspaceUnknownStopsWithoutRecoveryChurn(t *testing.T) {
	fixture := newFlightControllerFixture(t)
	patchUse, patchResult := fixture.patchResult(t, "patch-before-unsealed-runs")
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})

	use := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "unsealed", Name: "Run", Input: map[string]any{}}
	result := types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: use.ID, Outcome: types.ToolOutcomeSucceeded,
		Metadata: map[string]string{
			"stepCount":                  "1",
			"verification.status":        "committed_unverified",
			"verification.safety_reason": "plan_not_revision_safe",
		},
	}
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{use}, []types.ToolResultBlock{result})
	decision, err := fixture.controller.requestFinal()
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != agenticFlightTerminalBlocked || decision.Blocker != flight.CompletionWorkspaceUnknown ||
		decision.Disposition != flightDispositionIncompleteUnverified {
		t.Fatalf("unknown-workspace decision = %+v", decision)
	}
}

func TestAgenticFlightFormatterVerificationUsesFinalRevision(t *testing.T) {
	for _, outcome := range []types.ToolOutcome{types.ToolOutcomeSucceeded, types.ToolOutcomeFailed} {
		t.Run(string(outcome), func(t *testing.T) {
			fixture := newFlightControllerFixture(t)
			patchUse, patchResult := fixture.patchResult(t, "patch-before-formatter-"+string(outcome))
			observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})
			if err := os.WriteFile(fixture.path, []byte("formatted\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			finalRevision, err := fixture.ledger.Commit(fixture.root, []string{fixture.path})
			if err != nil {
				t.Fatal(err)
			}
			runUse := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "formatter-test-" + string(outcome), Name: "Run", Input: map[string]any{}}
			runResult := types.ToolResultBlock{
				Type: types.ContentTypeToolResult, ToolUseID: runUse.ID, Data: fusionMutationData{receipt: finalRevision}, Outcome: outcome,
				Metadata: map[string]string{
					"stepCount":                    "2",
					"mutation.status":              "committed",
					"verification.status":          "revision_bound",
					"verification.revision_digest": string(finalRevision.Digest()),
					"verification.kind":            "full_test",
					"verification.config_digest":   string(digestFlightValues("formatter-test-config")),
				},
			}
			if outcome != types.ToolOutcomeSucceeded {
				runResult.IsError = true
			}
			observeFlightRound(t, fixture.controller, []types.ToolUseBlock{runUse}, []types.ToolResultBlock{runResult})

			state := fixture.controller.state
			last := state.Actions[len(state.Actions)-1]
			if state.MutationEpoch != 2 || !last.Invoked || last.MutationOutcome != flight.MutationCommitted ||
				!fixture.controller.currentRevision.Valid() || fixture.controller.currentRevision.Digest() != finalRevision.Digest() {
				t.Fatalf("formatter revision was not installed: state=%+v action=%+v receipt=%+v", state, last, fixture.controller.currentRevision)
			}
			decision, decisionErr := fixture.controller.requestFinal()
			if decisionErr != nil {
				t.Fatal(decisionErr)
			}
			if outcome == types.ToolOutcomeSucceeded {
				if state.VerifiedEpoch != state.MutationEpoch || state.VerificationReceipt == nil || decision.Action != agenticFlightTerminalComplete {
					t.Fatalf("passing formatter/test did not verify final revision: state=%+v decision=%+v", state, decision)
				}
			} else if state.VerifiedEpoch >= state.MutationEpoch || state.VerificationReceipt != nil ||
				decision.Action != agenticFlightTerminalBlocked || decision.Disposition != flightDispositionIncompleteUnverified {
				t.Fatalf("failing formatter/test became completion evidence: state=%+v decision=%+v", state, decision)
			}
		})
	}
}

func TestAgenticFlightRevisionBoundObservationCannotComplete(t *testing.T) {
	fixture := newFlightControllerFixture(t)
	patchUse, patchResult := fixture.patchResult(t, "patch-observation")
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})
	epoch := fixture.controller.state.MutationEpoch

	runUse := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "run-pwd", Name: "Run", Input: map[string]any{}}
	runResult := types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: runUse.ID, Outcome: types.ToolOutcomeSucceeded,
		Metadata: map[string]string{"verification.status": "revision_bound"},
	}
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{runUse}, []types.ToolResultBlock{runResult})
	decision, err := fixture.controller.requestFinal()
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != agenticFlightTerminalBlocked || decision.Disposition != flightDispositionIncompleteUnverified {
		t.Fatalf("observation decision = %+v", decision)
	}
	if fixture.controller.state.MutationEpoch != epoch || fixture.controller.state.VerifiedEpoch != 0 || !fixture.controller.currentRevision.Valid() {
		t.Fatalf("observation changed revision authority: state=%+v receipt=%+v", fixture.controller.state, fixture.controller.currentRevision)
	}
}

func TestAgenticFlightReadOnlyFinalAndSecondDeferral(t *testing.T) {
	t.Run("read only", func(t *testing.T) {
		fixture := newFlightControllerFixture(t)
		decision, err := fixture.controller.requestFinal()
		if err != nil {
			t.Fatal(err)
		}
		if decision.Action != agenticFlightTerminalComplete || decision.Disposition != flightDispositionCompletedReadOnly {
			t.Fatalf("read-only decision = %+v", decision)
		}
	})

	t.Run("second deferral blocks", func(t *testing.T) {
		fixture := newFlightControllerFixture(t)
		patchUse, patchResult := fixture.patchResult(t, "patch-stall")
		observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})
		first, err := fixture.controller.requestFinal()
		if err != nil {
			t.Fatal(err)
		}
		second, err := fixture.controller.requestFinal()
		if err != nil {
			t.Fatal(err)
		}
		if first.Action != agenticFlightTerminalContinue || second.Action != agenticFlightTerminalBlocked ||
			second.Disposition != flightDispositionIncompleteUnverified || fixture.controller.state.TerminalDisposition != flight.TerminalBlocked {
			t.Fatalf("first=%+v second=%+v state=%+v", first, second, fixture.controller.state)
		}
	})
}

func TestAgenticFlightIsQueryLocalAndDoesNotReuseReceipt(t *testing.T) {
	fixture := newFlightControllerFixture(t)
	patchUse, patchResult := fixture.patchResult(t, "patch-old-query")
	observeFlightRound(t, fixture.controller, []types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})
	if !fixture.controller.currentRevision.Valid() {
		t.Fatal("test setup did not retain current query receipt")
	}

	reg := registry.New()
	reg.Register(parityTool{name: "Inspect", content: "inspected"})
	reg.Register(&fusionMutationTool{ledger: fixture.ledger, root: fixture.root, path: fixture.path})
	verification := &fusionVerificationTool{ledger: fixture.ledger, path: fixture.path}
	reg.Register(verification)
	resumed, err := newAgenticFlightController(reg, QueryConfigSnapshot{ProjectRoot: fixture.root, CWD: fixture.root}, "query-2", []types.Message{types.UserMessage("next")})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.currentRevision.Valid() || resumed.state.MutationEpoch != 0 {
		t.Fatalf("new query reused old receipt/state: %+v", resumed.state)
	}
	if _, ok := workspacerevision.FromContext(resumed.bindVerificationContext(context.Background())); ok {
		t.Fatal("new query bound a stale receipt")
	}
	result, err := verification.Execute(resumed.bindVerificationContext(context.Background()), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Metadata["verification.status"] != "revision_mismatch" {
		t.Fatalf("receipt-less Run in a new query = %+v", result)
	}
}

func TestQueryLoopAgenticFlightDefersPrematureFinalThenCompletes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	reg := registry.New()
	reg.Register(parityTool{name: "Inspect", content: "inspected"})
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path})
	reg.Register(&fusionVerificationTool{ledger: ledger, path: path})
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: parityToolUseEventsWithUsage("patch-query", "ApplyPatch", `{}`, nil)},
		{Events: parityTextEvents("premature")},
		{Events: parityToolUseEventsWithUsage("run-query", "Run", `{}`, nil)},
		{Events: parityTextEvents("complete")},
	})
	query := New(provider, reg, Config{MaxTurns: 6, MaxTokens: 1024, ProjectRoot: root, CWD: root})
	var dispositions []string
	if err := query.Run(context.Background(), "change it", func(event stream.Event) {
		if event.Type == stream.EventProgress && event.Progress != nil && event.Progress.Stage == "agentic_flight" {
			dispositions = append(dispositions, event.Progress.Disposition)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(provider.Calls) != 4 {
		t.Fatalf("provider calls = %d, want 4", len(provider.Calls))
	}
	if len(dispositions) != 2 || dispositions[0] != flightDispositionVerificationRequired || dispositions[1] != flightDispositionCompletedVerified {
		t.Fatalf("flight dispositions = %#v", dispositions)
	}
	foundNudge := false
	for _, message := range provider.Calls[2].Messages {
		if message.InternalKind == types.InternalMessageKindFlightVerification && message.HasInternalControlProvenance() {
			foundNudge = true
		}
	}
	if !foundNudge {
		t.Fatal("verification continuation did not carry a trusted semantic runtime message")
	}
}

func TestQueryLoopAgenticFlightCrossTurnCommittedUnverifiedRunThenFailedPatchKeepsHistoryPaired(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	reg := registry.New()
	reg.Register(parityTool{name: "Inspect", content: "inspected"})
	reg.Register(&successfulThenFailingFlightPatchTool{fusionMutationTool: &fusionMutationTool{
		ledger: ledger, root: root, path: path,
	}})
	reg.Register(&planUnsafeFlightRunTool{fusionVerificationTool: &fusionVerificationTool{ledger: ledger, path: path}})
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: parityToolUseEventsWithUsage("patch-committed-cross-turn", "ApplyPatch", `{}`, nil)},
		{Events: parityToolUseEventsWithUsage("run-plan-unsafe-cross-turn", "Run", `{}`, nil)},
		{Events: parityToolUseEventsWithUsage("patch-failed-cross-turn", "ApplyPatch", `{}`, nil)},
		{Events: parityTextEvents("continue")},
	})
	query := New(provider, reg, Config{MaxTurns: 6, MaxTokens: 1024, ProjectRoot: root, CWD: root})
	if err := query.Run(context.Background(), "change it", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	if len(provider.Calls) != 4 {
		t.Fatalf("provider calls = %d, want 4", len(provider.Calls))
	}

	assertPaired := func(messages []types.Message, toolUseID string) {
		t.Helper()
		useIndex, resultIndex := -1, -1
		for messageIndex, message := range messages {
			for _, use := range message.GetToolUses() {
				if use.ID == toolUseID {
					useIndex = messageIndex
				}
			}
			for _, block := range message.Content {
				if result, ok := block.(types.ToolResultBlock); ok && result.ToolUseID == toolUseID {
					resultIndex = messageIndex
				}
			}
		}
		if useIndex < 0 || resultIndex != useIndex+1 {
			t.Fatalf("tool call %q was not followed by its result: use=%d result=%d messages=%+v", toolUseID, useIndex, resultIndex, messages)
		}
	}
	assertPaired(provider.Calls[3].Messages, "patch-failed-cross-turn")
	assertPaired(query.Messages(), "patch-failed-cross-turn")
}

func TestQueryLoopAgenticFlightFusionAndReadOnlyDoNotAddProviderRounds(t *testing.T) {
	t.Run("fusion", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "source.txt")
		if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		ledger := workspacerevision.NewLedger()
		reg := registry.New()
		reg.Register(parityTool{name: "Inspect", content: "inspected"})
		reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path})
		reg.Register(&fusionVerificationTool{ledger: ledger, path: path})
		provider := newParityFakeProvider([]parityProviderTurn{
			{Events: committedFlightToolUseEvents(t,
				types.ToolUseBlock{ID: "patch-fused-query", Name: "ApplyPatch"},
				types.ToolUseBlock{ID: "run-fused-query", Name: "Run"},
			)},
			{Events: parityTextEvents("complete")},
		})
		query := New(provider, reg, Config{
			MaxTurns: 4, MaxTokens: 1024, TokenBudget: 1_000_000, ProjectRoot: root, CWD: root,
		})
		if err := query.Run(context.Background(), "change it", func(stream.Event) {}); err != nil {
			t.Fatal(err)
		}
		if len(provider.Calls) != 2 {
			t.Fatalf("fused provider calls = %d, want 2", len(provider.Calls))
		}
	})

	t.Run("read only", func(t *testing.T) {
		fixture := newFlightControllerFixture(t)
		reg := registry.New()
		reg.Register(parityTool{name: "Inspect", content: "inspected"})
		reg.Register(&fusionMutationTool{ledger: fixture.ledger, root: fixture.root, path: fixture.path})
		reg.Register(&fusionVerificationTool{ledger: fixture.ledger, path: fixture.path})
		provider := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("answer")}})
		query := New(provider, reg, Config{
			MaxTurns: 2, MaxTokens: 1024, TokenBudget: 1_000_000, ProjectRoot: fixture.root, CWD: fixture.root,
		})
		var disposition string
		if err := query.Run(context.Background(), "explain", func(event stream.Event) {
			if event.Progress != nil && event.Progress.Stage == "agentic_flight" {
				disposition = event.Progress.Disposition
			}
		}); err != nil {
			t.Fatal(err)
		}
		if len(provider.Calls) != 1 || disposition != flightDispositionCompletedReadOnly {
			t.Fatalf("calls=%d disposition=%q", len(provider.Calls), disposition)
		}
	})
}

func TestQueryLoopAgenticFlightSecondDeferralAndMaxTurnsAreIncomplete(t *testing.T) {
	t.Run("second deferral", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "source.txt")
		if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		ledger := workspacerevision.NewLedger()
		reg := registry.New()
		reg.Register(parityTool{name: "Inspect", content: "inspected"})
		reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path})
		reg.Register(&fusionVerificationTool{ledger: ledger, path: path})
		provider := newParityFakeProvider([]parityProviderTurn{
			{Events: parityToolUseEventsWithUsage("patch-stall-query", "ApplyPatch", `{}`, nil)},
			{Events: parityTextEvents("first")},
			{Events: parityTextEvents("second")},
		})
		query := New(provider, reg, Config{MaxTurns: 5, MaxTokens: 1024, ProjectRoot: root, CWD: root})
		var last string
		if err := query.Run(context.Background(), "change it", func(event stream.Event) {
			if event.Progress != nil && event.Progress.Stage == "agentic_flight" {
				last = event.Progress.Disposition
			}
		}); err != nil {
			t.Fatal(err)
		}
		if len(provider.Calls) != 3 || last != flightDispositionIncompleteUnverified {
			t.Fatalf("calls=%d last disposition=%q", len(provider.Calls), last)
		}
	})

	t.Run("max turns", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "source.txt")
		if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		ledger := workspacerevision.NewLedger()
		reg := registry.New()
		reg.Register(parityTool{name: "Inspect", content: "inspected"})
		reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path})
		reg.Register(&fusionVerificationTool{ledger: ledger, path: path})
		provider := newParityFakeProvider([]parityProviderTurn{{Events: parityToolUseEventsWithUsage("patch-max", "ApplyPatch", `{}`, nil)}})
		query := New(provider, reg, Config{MaxTurns: 1, MaxTokens: 1024, ProjectRoot: root, CWD: root})
		var disposition string
		err := query.Run(context.Background(), "change it", func(event stream.Event) {
			if event.Progress != nil && event.Progress.Stage == "agentic_flight" {
				disposition = event.Progress.Disposition
			}
		})
		if err == nil || disposition != flightDispositionBlockedRuntime {
			t.Fatalf("err=%v disposition=%q", err, disposition)
		}
	})
}

func TestQueryLoopAgenticFlightConsumesRepeatedFailureDisposition(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	reg := registry.New()
	reg.Register(parityTool{name: "Inspect", content: "inspected"})
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path})
	reg.Register(&failingFlightVerificationTool{fusionVerificationTool: &fusionVerificationTool{ledger: ledger, path: path}})
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: parityToolUseEventsWithUsage("patch-repeated-failure", "ApplyPatch", `{}`, nil)},
		{Events: parityToolUseEventsWithUsage("run-repeated-failure-1", "Run", `{"requires_patch_commit":true}`, nil)},
		{Events: parityToolUseEventsWithUsage("run-repeated-failure-2", "Run", `{"requires_patch_commit":true}`, nil)},
		{Events: parityToolUseEventsWithUsage("run-repeated-failure-3", "Run", `{"requires_patch_commit":true}`, nil)},
	})
	query := New(provider, reg, Config{MaxTurns: 8, MaxTokens: 1024, ProjectRoot: root, CWD: root})
	var disposition string
	err := query.Run(context.Background(), "change it", func(event stream.Event) {
		if event.Progress != nil && event.Progress.Stage == "agentic_flight" {
			disposition = event.Progress.Disposition
		}
	})
	if err == nil {
		t.Fatal("repeated deterministic failure did not stop the query")
	}
	if len(provider.Calls) != 4 || disposition != flightDispositionBlockedRepeatedFailure {
		t.Fatalf("calls=%d disposition=%q err=%v", len(provider.Calls), disposition, err)
	}
}

func TestQueryLoopAgenticFlightStopHookPreventContinuationCommitsVerifiedTerminal(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	reg := registry.New()
	reg.Register(parityTool{name: "Inspect", content: "inspected"})
	reg.Register(&fusionMutationTool{ledger: ledger, root: root, path: path})
	reg.Register(&fusionVerificationTool{ledger: ledger, path: path})
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: committedFlightToolUseEvents(t,
			types.ToolUseBlock{ID: "patch-stop-hook", Name: "ApplyPatch"},
			types.ToolUseBlock{ID: "run-stop-hook", Name: "Run", Input: map[string]any{"requires_patch_commit": true}},
		)},
		{Events: parityTextEvents("complete")},
	})
	runner := hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookStop, Command: testHookOutputCommand(`{"preventContinuation":true,"stopReason":"complete"}`), Timeout: 5,
	}})
	query := New(provider, reg, Config{
		MaxTurns: 4, MaxTokens: 1024, ProjectRoot: root, CWD: root, HookRunner: runner,
	})
	var disposition string
	if err := query.Run(context.Background(), "change it", func(event stream.Event) {
		if event.Progress != nil && event.Progress.Stage == "agentic_flight" {
			disposition = event.Progress.Disposition
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(provider.Calls) != 2 || disposition != flightDispositionCompletedVerified {
		t.Fatalf("calls=%d disposition=%q", len(provider.Calls), disposition)
	}
}

func TestAgenticInvestigationTrackerNudgeIsBoundedAndPreMutation(t *testing.T) {
	tracker := &agenticInvestigationTracker{preMutationInspects: flightInvestigationNudgeThreshold - 1}
	if tracker.takeNudge() {
		t.Fatal("nudge fired before the investigation threshold")
	}
	tracker.preMutationInspects++
	if !tracker.takeNudge() {
		t.Fatal("nudge did not fire at the investigation threshold")
	}
	if tracker.takeNudge() {
		t.Fatal("nudge fired more than once")
	}

	mutated := &agenticInvestigationTracker{
		preMutationInspects: flightInvestigationNudgeThreshold,
		mutationAttempted:   true,
	}
	if mutated.takeNudge() {
		t.Fatal("nudge fired after a mutation attempt")
	}
}

func TestAgenticInvestigationTrackerVerificationConvergenceNudgeIsBounded(t *testing.T) {
	tracker := &agenticInvestigationTracker{}
	runUse := types.ToolUseBlock{ID: "run-before-patch", Name: "Run"}
	runResult := types.ToolResultBlock{ToolUseID: runUse.ID, Outcome: types.ToolOutcomeFailed}
	tracker.observe([]types.ToolUseBlock{runUse}, []types.ToolResultBlock{runResult})
	if tracker.takeVerificationConvergenceNudge() {
		t.Fatal("verification nudge fired before a mutation")
	}

	patchUse := types.ToolUseBlock{ID: "patch", Name: "ApplyPatch"}
	patchResult := types.ToolResultBlock{ToolUseID: patchUse.ID, Outcome: types.ToolOutcomeSucceeded}
	tracker.observe([]types.ToolUseBlock{patchUse}, []types.ToolResultBlock{patchResult})
	tracker.observe([]types.ToolUseBlock{runUse}, []types.ToolResultBlock{runResult})
	if !tracker.takeVerificationConvergenceNudge() {
		t.Fatal("verification nudge did not fire after an invoked post-mutation Run")
	}
	if tracker.takeVerificationConvergenceNudge() {
		t.Fatal("verification nudge fired more than once")
	}
}
