package loop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	"github.com/agent-dance/luban/internal/runtime/flight"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

const (
	flightAcceptanceCurrentMutationVerified = "current_workspace_mutation_verified"
	flightInvariantNoPendingToolIntent      = "no_pending_tool_intent"
	flightVerificationPolicyID              = "agentic_v2_current_workspace_verification_v1"

	flightDispositionVerificationRequired   = "verification_required"
	flightDispositionCompletedVerified      = "completed_verified"
	flightDispositionCompletedReadOnly      = "completed_read_only"
	flightDispositionIncompleteUnverified   = "incomplete_unverified"
	flightDispositionBlockedRuntime         = "blocked_runtime"
	flightDispositionBlockedRepeatedFailure = "blocked_repeated_failure"
)

type agenticFlightTerminalAction uint8

const (
	agenticFlightTerminalContinue agenticFlightTerminalAction = iota
	agenticFlightTerminalComplete
	agenticFlightTerminalBlocked
)

type agenticFlightTerminalDecision struct {
	Action      agenticFlightTerminalAction
	Disposition string
	Blocker     flight.CompletionBlocker
}

// agenticFlightController is query-local by design. A workspacerevision
// receipt is process-local authority and must never be inferred from restored
// transcript metadata or carried into a later user query.
type agenticFlightController struct {
	state                 flight.FlightState
	currentRevision       workspacerevision.Receipt
	deferredMutationEpoch flight.Epoch
	deferred              bool
	mutationAttempted     bool
	nextExecutionSequence uint64
	pendingExecutions     map[string]agenticFlightExecutionIntent
}

type agenticFlightExecutionIntent struct {
	sequence    uint64
	fingerprint flight.Fingerprint
}

func newAgenticFlightController(reg *registry.Registry, snapshot QueryConfigSnapshot, queryID string, messages []types.Message) (*agenticFlightController, error) {
	if !isAgenticV2FlightProfile(reg) {
		return nil, nil
	}
	workspaceIdentity := digestFlightValues("workspace", snapshot.ProjectRoot, snapshot.CWD, snapshot.SessionID)
	state, err := flight.NewState(flight.StateSpec{
		WorkspaceInstanceID:      string(workspaceIdentity),
		WorkspaceDigest:          digestFlightValues("query_baseline", string(workspaceIdentity), queryID),
		TaskDigest:               digestFlightMessages(messages),
		VerificationConfigDigest: digestFlightValues("verification_policy", flightVerificationPolicyID),
		AcceptanceCriteria:       []string{flightAcceptanceCurrentMutationVerified},
		Invariants:               []string{flightInvariantNoPendingToolIntent},
	})
	if err != nil {
		return nil, err
	}
	return &agenticFlightController{state: state, pendingExecutions: make(map[string]agenticFlightExecutionIntent)}, nil
}

func isAgenticV2FlightProfile(reg *registry.Registry) bool {
	if reg == nil {
		return false
	}
	mutation, mutationOK := reg.Get("ApplyPatch").(workspacerevision.MutationTool)
	verification, verificationOK := reg.Get("Run").(workspacerevision.VerificationTool)
	if reg.Get("Inspect") == nil || !mutationOK || !verificationOK ||
		!mutation.ProvidesWorkspaceRevisionBarrier() || !verification.ConsumesWorkspaceRevisionBarrier() {
		return false
	}
	for _, legacy := range []string{"Bash", "PowerShell", "Read", "Write", "Edit", "Glob", "Grep"} {
		if reg.Get(legacy) != nil {
			return false
		}
	}
	return true
}

func (controller *agenticFlightController) bindVerificationContext(ctx context.Context) context.Context {
	if controller == nil || !controller.currentRevision.Valid() {
		return ctx
	}
	return workspacerevision.WithReceipt(ctx, controller.currentRevision)
}

func (controller *agenticFlightController) openToolIntents(toolUses []types.ToolUseBlock) error {
	if controller == nil {
		return nil
	}
	for _, toolUse := range toolUses {
		if !isAgenticFlightCoreTool(toolUse.Name) {
			continue
		}
		controller.nextExecutionSequence++
		intent := agenticFlightExecutionIntent{
			sequence: controller.nextExecutionSequence, fingerprint: flightActionFingerprint(toolUse),
		}
		next, _, err := flight.Reduce(controller.state, flight.IntentOpened{
			ID:                toolUse.ID,
			ExecutionSequence: intent.sequence,
			ActionFingerprint: intent.fingerprint,
		})
		if err != nil {
			return err
		}
		controller.state = next
		controller.pendingExecutions[toolUse.ID] = intent
		if toolUse.Name == "ApplyPatch" {
			controller.mutationAttempted = true
		}
	}
	return nil
}

func (controller *agenticFlightController) observeToolRound(toolUses []types.ToolUseBlock, results []types.ToolResultBlock) error {
	if controller == nil {
		return nil
	}
	byID := make(map[string]types.ToolResultBlock, len(results))
	for _, result := range results {
		byID[result.ToolUseID] = result
	}
	verificationEvidence := flight.Digest("")
	for _, toolUse := range toolUses {
		if !isAgenticFlightCoreTool(toolUse.Name) {
			continue
		}
		result, ok := byID[toolUse.ID]
		if !ok {
			break
		}
		facts, revision := controller.executionFacts(toolUse, result)
		next, effects, err := flight.Reduce(controller.state, flight.ToolExecuted{Facts: facts})
		if err != nil {
			return err
		}
		controller.state = next
		delete(controller.pendingExecutions, toolUse.ID)
		if toolUse.Name == "ApplyPatch" {
			controller.currentRevision = revision
			controller.deferredMutationEpoch = 0
			controller.deferred = false
		}
		if effects.MutationAdvanced && toolUse.Name != "ApplyPatch" {
			controller.currentRevision = revision
			controller.deferredMutationEpoch = 0
			controller.deferred = false
		}
		if effects.ReceiptDisposition == flight.ReceiptIssued && effects.Receipt != nil {
			verificationEvidence = effects.Receipt.EvidenceDigest
		}
	}
	if verificationEvidence == "" || len(controller.state.PendingIntents) != 0 || !controller.state.WorkspaceDigestKnown {
		return nil
	}
	evaluation := flight.ConditionEvaluation{
		Outcome:         flight.ConditionSatisfied,
		MutationEpoch:   controller.state.MutationEpoch,
		WorkspaceDigest: controller.state.WorkspaceDigest,
		EvidenceDigest:  verificationEvidence,
	}
	acceptance := evaluation
	acceptance.ID = flightAcceptanceCurrentMutationVerified
	invariant := evaluation
	invariant.ID = flightInvariantNoPendingToolIntent
	next, _, err := flight.Reduce(controller.state, flight.ConditionsEvaluated{
		Acceptance: []flight.ConditionEvaluation{acceptance},
		Invariants: []flight.ConditionEvaluation{invariant},
	})
	if err != nil {
		return err
	}
	controller.state = next
	return nil
}

func (controller *agenticFlightController) executionFacts(toolUse types.ToolUseBlock, result types.ToolResultBlock) (flight.ToolExecutionFacts, workspacerevision.Receipt) {
	before := controller.state.WorkspaceDigest
	intent := controller.pendingExecutions[toolUse.ID]
	action := intent.fingerprint
	execution := flightExecutionOutcome(result)
	evidence := digestFlightToolResult(toolUse.Name, result)
	facts := flight.ToolExecutionFacts{
		ToolID:              toolUse.Name,
		IntentID:            toolUse.ID,
		ExecutionSequence:   intent.sequence,
		Invoked:             flightToolInvoked(toolUse.Name, result),
		EffectScope:         flight.EffectScopeWorkspaceRead,
		ExecutionOutcome:    execution,
		MutationOutcome:     flight.MutationNone,
		VerificationKind:    flight.VerificationNone,
		VerificationOutcome: flight.VerificationNotRun,
		ActionFingerprint:   action,
	}
	if facts.Invoked && controller.state.WorkspaceDigestKnown {
		facts.BeforeDigest = before
		facts.AfterDigest = before
	}
	if execution != flight.ExecutionSucceeded {
		facts.FailureFingerprint = flightFailureFingerprint(toolUse, result, execution)
	}

	switch toolUse.Name {
	case "Inspect":
		if facts.Invoked && execution == flight.ExecutionSucceeded {
			facts.EvidenceDigest = evidence
		}
		return facts, workspacerevision.Receipt{}

	case "ApplyPatch":
		facts.EffectScope = flight.EffectScopeWorkspaceWrite
		revision, committed := revisionReceiptFromResult(result)
		switch {
		case committed:
			facts.Invoked = true
			facts.MutationOutcome = flight.MutationCommitted
			facts.BeforeDigest = before
			facts.AfterDigest = revision.Digest()
			facts.EvidenceDigest = digestFlightValues("mutation", string(revision.Digest()), string(action))
			return facts, revision
		case result.Outcome == types.ToolOutcomePartial || result.Outcome == types.ToolOutcomeCancelled || result.Outcome == types.ToolOutcomeTimedOut:
			facts.Invoked = true
			facts.MutationOutcome = flight.MutationPossible
			facts.BeforeDigest = before
			facts.AfterDigest = ""
			return facts, workspacerevision.Receipt{}
		default:
			// ApplyPatch is transactional: a complete business failure has
			// either not started or has proven rollback. Partial/cancelled
			// outcomes above remain conservative MutationPossible facts.
			if facts.Invoked && controller.state.WorkspaceDigestKnown {
				facts.BeforeDigest = before
				facts.AfterDigest = before
			}
			return facts, workspacerevision.Receipt{}
		}

	case "Run":
		status := result.Metadata["verification.status"]
		verificationKind := flight.VerificationKind(result.Metadata["verification.kind"])
		verificationConfig := result.Metadata["verification.config_digest"]
		runRevision, hasRunRevision := workspaceRevisionReceiptFromData(result)
		mutationCommitted := result.Metadata["mutation.status"] == "committed" && hasRunRevision &&
			controller.state.MutationEpoch < ^flight.Epoch(0) && runRevision.Epoch() == controller.state.MutationEpoch+1
		if status == "committed_unverified" && facts.Invoked {
			// A mutating graph cannot inherit the preceding patch receipt. Until
			// its actual write set is sealed, conservatively advance the mutation
			// epoch and revoke all completion authority, regardless of whether a
			// later step in that same graph passed or failed.
			facts.EffectScope = flight.EffectScopeWorkspaceWrite
			facts.MutationOutcome = flight.MutationPossible
			facts.BeforeDigest = before
			facts.AfterDigest = ""
			return facts, workspacerevision.Receipt{}
		}
		if controller.state.MutationEpoch == 0 {
			if facts.Invoked && execution == flight.ExecutionSucceeded {
				facts.EvidenceDigest = evidence
			}
			return facts, workspacerevision.Receipt{}
		}
		verificationEpoch := controller.state.MutationEpoch
		revisionBoundCurrent := controller.currentRevision.Valid() && status == "revision_bound" &&
			controller.state.WorkspaceDigestKnown && controller.currentRevision.Digest() == controller.state.WorkspaceDigest
		if mutationCommitted {
			facts.EffectScope = flight.EffectScopeWorkspaceWrite
			facts.MutationOutcome = flight.MutationCommitted
			facts.BeforeDigest = before
			facts.AfterDigest = runRevision.Digest()
			verificationEpoch = controller.state.MutationEpoch + 1
			revisionBoundCurrent = status == "revision_bound" && runRevision.Digest() != "" &&
				result.Metadata["verification.revision_digest"] == string(runRevision.Digest())
		}
		if revisionBoundCurrent && verificationKind.Relevant() && validFlightAttestationDigest(verificationConfig) {
			facts.Invoked = true
			if !mutationCommitted {
				facts.BeforeDigest = before
				facts.AfterDigest = before
			}
			facts.VerificationKind = verificationKind
			facts.VerificationEpoch = verificationEpoch
			facts.VerificationConfigDigest = controller.state.VerificationConfigDigest
			facts.EvidenceDigest = evidence
			switch execution {
			case flight.ExecutionSucceeded:
				facts.VerificationOutcome = flight.VerificationPassed
			case flight.ExecutionCancelled:
				facts.VerificationOutcome = flight.VerificationInconclusive
			default:
				facts.VerificationOutcome = flight.VerificationFailed
			}
			return facts, runRevision
		}
		if revisionBoundCurrent {
			// A revision-bound command proves that the task-scoped workspace
			// revision stayed current, but only an executor-classified build,
			// static check, or test is completion evidence. Commands such as
			// pwd and git status remain observations and preserve the receipt.
			facts.Invoked = true
			if !mutationCommitted {
				facts.BeforeDigest = before
				facts.AfterDigest = before
			}
			facts.EvidenceDigest = evidence
			return facts, runRevision
		}
		if facts.Invoked {
			// A Run without a current post-execution revision binding cannot
			// prove the task-scoped paths unchanged. Preserve the patch, but
			// invalidate completion authority conservatively.
			facts.EffectScope = flight.EffectScopeWorkspaceWrite
			facts.MutationOutcome = flight.MutationPossible
			facts.BeforeDigest = before
			facts.AfterDigest = ""
		}
		return facts, workspacerevision.Receipt{}
	default:
		return facts, workspacerevision.Receipt{}
	}
}

func (controller *agenticFlightController) requestFinal() (agenticFlightTerminalDecision, error) {
	if controller == nil {
		return agenticFlightTerminalDecision{Action: agenticFlightTerminalComplete}, nil
	}
	if controller.state.TerminalDisposition != flight.TerminalRunning {
		return agenticFlightTerminalDecision{
			Action: agenticFlightTerminalBlocked, Disposition: flightDispositionIncompleteUnverified,
			Blocker: flight.CompletionAlreadyTerminal,
		}, nil
	}
	if controller.state.MutationEpoch == 0 && !controller.mutationAttempted {
		return agenticFlightTerminalDecision{
			Action: agenticFlightTerminalComplete, Disposition: flightDispositionCompletedReadOnly,
			Blocker: flight.CompletionReady,
		}, nil
	}

	next, effects, err := flight.Reduce(controller.state, flight.TerminalRequested{Disposition: flight.TerminalCompleted})
	if err != nil {
		return agenticFlightTerminalDecision{}, err
	}
	if effects.Completion.Allowed && next.TerminalDisposition == flight.TerminalCompleted {
		return agenticFlightTerminalDecision{
			Action: agenticFlightTerminalComplete, Disposition: flightDispositionCompletedVerified,
			Blocker: flight.CompletionReady,
		}, nil
	}
	blocker := effects.Completion.Blocker
	if blocker == "" {
		blocker = flight.CompletionVerificationMissing
	}
	if blocker == flight.CompletionRepeatedFailure {
		return controller.blockRepeatedFailure()
	}
	if !controller.deferred || controller.deferredMutationEpoch != controller.state.MutationEpoch {
		controller.deferredMutationEpoch = controller.state.MutationEpoch
		controller.deferred = true
		return agenticFlightTerminalDecision{
			Action: agenticFlightTerminalContinue, Disposition: flightDispositionVerificationRequired,
			Blocker: blocker,
		}, nil
	}

	next, _, err = flight.Reduce(controller.state, flight.TerminalRequested{Disposition: flight.TerminalBlocked})
	if err != nil {
		return agenticFlightTerminalDecision{}, err
	}
	controller.state = next
	return agenticFlightTerminalDecision{
		Action: agenticFlightTerminalBlocked, Disposition: flightDispositionIncompleteUnverified,
		Blocker: blocker,
	}, nil
}

func (controller *agenticFlightController) repeatedFailureTriggered() bool {
	return controller != nil &&
		controller.state.ConsecutiveFailures >= controller.state.Limits.RepeatedFailureTrigger
}

func (controller *agenticFlightController) blockRepeatedFailure() (agenticFlightTerminalDecision, error) {
	decision := agenticFlightTerminalDecision{
		Action: agenticFlightTerminalBlocked, Disposition: flightDispositionBlockedRepeatedFailure,
		Blocker: flight.CompletionRepeatedFailure,
	}
	if controller == nil {
		return decision, nil
	}
	if !controller.repeatedFailureTriggered() {
		return agenticFlightTerminalDecision{}, fmt.Errorf("flight repeated-failure block requested before trigger")
	}
	if controller.state.TerminalDisposition == flight.TerminalRunning {
		next, _, err := flight.Reduce(controller.state, flight.TerminalRequested{Disposition: flight.TerminalBlocked})
		if err != nil {
			return agenticFlightTerminalDecision{}, err
		}
		controller.state = next
	}
	return decision, nil
}

func (controller *agenticFlightController) commitFinal(decision agenticFlightTerminalDecision) error {
	if controller == nil || decision.Disposition == flightDispositionCompletedReadOnly {
		return nil
	}
	if decision.Action != agenticFlightTerminalComplete || decision.Disposition != flightDispositionCompletedVerified {
		return nil
	}
	next, effects, err := flight.Reduce(controller.state, flight.TerminalRequested{Disposition: flight.TerminalCompleted})
	if err != nil {
		return err
	}
	if !effects.Completion.Allowed || next.TerminalDisposition != flight.TerminalCompleted {
		return fmt.Errorf("flight completion commit rejected: %s", effects.Completion.Blocker)
	}
	controller.state = next
	return nil
}

func (controller *agenticFlightController) blockRuntime() (agenticFlightTerminalDecision, error) {
	decision := agenticFlightTerminalDecision{
		Action: agenticFlightTerminalBlocked, Disposition: flightDispositionBlockedRuntime,
		Blocker: flight.CompletionAlreadyTerminal,
	}
	if controller == nil {
		return decision, nil
	}
	if controller.state.TerminalDisposition == flight.TerminalRunning {
		next, _, err := flight.Reduce(controller.state, flight.TerminalRequested{Disposition: flight.TerminalBlocked})
		if err != nil {
			return agenticFlightTerminalDecision{}, err
		}
		controller.state = next
		decision.Blocker = flight.CompletionVerificationMissing
	}
	return decision, nil
}

func (controller *agenticFlightController) verificationMessage(q *QueryLoop) types.Message {
	message := types.UserMessage(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLoopVisibleFlightVerificationRequired))
	message.IsMeta = true
	message.InternalKind = types.InternalMessageKindFlightVerification
	return q.sealRuntimeControlMessage(message)
}

func (controller *agenticFlightController) mutationEpoch() uint64 {
	if controller == nil {
		return 0
	}
	return uint64(controller.state.MutationEpoch)
}

func (controller *agenticFlightController) verifiedEpoch() uint64 {
	if controller == nil {
		return 0
	}
	return uint64(controller.state.VerifiedEpoch)
}

func isAgenticFlightCoreTool(name string) bool {
	return name == "Inspect" || name == "ApplyPatch" || name == "Run"
}

func flightToolInvoked(name string, result types.ToolResultBlock) bool {
	if result.Metadata["schedule.status"] == "skipped" || result.Outcome == types.ToolOutcomeDenied {
		return false
	}
	if name == "Run" {
		switch result.Metadata["verification.status"] {
		case "revision_mismatch", "patch_commit_required":
			// Both are fail-closed pre-execution dispositions. Counting either as
			// an invocation would manufacture a mutation epoch for a process that
			// never started.
			return false
		}
		return result.Metadata["stepCount"] != "" || result.Metadata["verification.status"] != "" ||
			result.Outcome == types.ToolOutcomeSucceeded || result.Outcome == types.ToolOutcomeFailed ||
			result.Outcome == types.ToolOutcomeCancelled || result.Outcome == types.ToolOutcomeTimedOut
	}
	if result.Outcome == "" && result.IsError {
		return false
	}
	return true
}

func flightExecutionOutcome(result types.ToolResultBlock) flight.ExecutionOutcome {
	switch result.Outcome {
	case types.ToolOutcomeSucceeded:
		return flight.ExecutionSucceeded
	case types.ToolOutcomeCancelled, types.ToolOutcomeTimedOut:
		return flight.ExecutionCancelled
	case types.ToolOutcomeFailed, types.ToolOutcomePartial, types.ToolOutcomeDenied:
		return flight.ExecutionFailed
	default:
		if result.IsError {
			return flight.ExecutionFailed
		}
		return flight.ExecutionSucceeded
	}
}

func flightActionFingerprint(toolUse types.ToolUseBlock) flight.Fingerprint {
	return digestFlightFingerprint("action", toolUse.Name, toolUse.ID)
}

// flightFailureFingerprint deliberately excludes localized result text,
// durations, provider call IDs, and other presentation noise. The canonical
// requested action plus stable machine outcome/metadata makes an unchanged
// deterministic retry recognizable while physical execution identity remains
// separately bound by flightActionFingerprint.
func flightFailureFingerprint(toolUse types.ToolUseBlock, result types.ToolResultBlock, outcome flight.ExecutionOutcome) flight.Fingerprint {
	action := toolUse.RawInput
	if action == "" {
		if encoded, err := json.Marshal(toolUse.Input); err == nil {
			action = string(encoded)
		}
	}
	values := []string{"failure", toolUse.Name, action, string(outcome), string(result.Outcome), strconv.FormatBool(result.IsError)}
	metadataKeys := make([]string, 0, len(result.Metadata))
	for key := range result.Metadata {
		switch key {
		case "duration_ms", "started_offset_ms", "ended_offset_ms", "process_duration_ms":
			continue
		}
		metadataKeys = append(metadataKeys, key)
	}
	sort.Strings(metadataKeys)
	for _, key := range metadataKeys {
		values = append(values, key, result.Metadata[key])
	}
	if toolError, ok := result.Data.(types.ToolErrorData); ok {
		values = append(values, toolError.Schema, toolError.Code, toolError.Path, strconv.FormatBool(toolError.Retryable))
	} else if toolError, ok := result.Data.(*types.ToolErrorData); ok && toolError != nil {
		values = append(values, toolError.Schema, toolError.Code, toolError.Path, strconv.FormatBool(toolError.Retryable))
	}
	return digestFlightFingerprint(values...)
}

func digestFlightMessages(messages []types.Message) flight.Digest {
	encoded, err := json.Marshal(messages)
	if err != nil {
		return digestFlightValues("task", strconv.Itoa(len(messages)))
	}
	sum := sha256.Sum256(encoded)
	return flight.Digest(hex.EncodeToString(sum[:]))
}

func digestFlightToolResult(toolName string, result types.ToolResultBlock) flight.Digest {
	metadataKeys := make([]string, 0, len(result.Metadata))
	for key := range result.Metadata {
		metadataKeys = append(metadataKeys, key)
	}
	sort.Strings(metadataKeys)
	values := []string{"tool_result", toolName, string(result.Outcome), strconv.FormatBool(result.IsError), result.TextContent()}
	for _, key := range metadataKeys {
		values = append(values, key, result.Metadata[key])
	}
	return digestFlightValues(values...)
}

func digestFlightFingerprint(values ...string) flight.Fingerprint {
	return flight.Fingerprint(digestFlightValues(values...))
}

func validFlightAttestationDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func digestFlightValues(values ...string) flight.Digest {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(strconv.Itoa(len(value))))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return flight.Digest(hex.EncodeToString(h.Sum(nil)))
}
