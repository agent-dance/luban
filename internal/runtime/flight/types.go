// Package flight contains the deterministic correctness state machine for an
// agentic coding flight. It deliberately has no I/O and stores only bounded,
// content-free execution facts so callers can persist or replay it safely.
package flight

import "github.com/agent-dance/luban/internal/contracts/workspacerevision"

// Epoch is the logical workspace-mutation generation used to invalidate stale
// verification and condition evaluations.
type Epoch = workspacerevision.Epoch

// Revision is a monotonically increasing reducer revision.
type Revision uint64

// Digest is an opaque, caller-produced content digest.
type Digest = workspacerevision.Digest

// Fingerprint is an opaque stable identity for an action, failure, or file.
type Fingerprint string

const CurrentSchemaVersion uint16 = 1

// EffectScope classifies which state an action could affect. MutationOutcome
// is authoritative for whether the workspace mutation epoch advances.
type EffectScope string

const (
	EffectScopeNone                 EffectScope = "none"
	EffectScopeWorkspaceRead        EffectScope = "workspace_read"
	EffectScopeWorkspaceWrite       EffectScope = "workspace_write"
	EffectScopeExternal             EffectScope = "external"
	EffectScopeWorkspaceAndExternal EffectScope = "workspace_and_external"
)

func (scope EffectScope) affectsWorkspace() bool {
	return scope == EffectScopeWorkspaceRead || scope == EffectScopeWorkspaceWrite || scope == EffectScopeWorkspaceAndExternal
}

func (scope EffectScope) permitsWorkspaceMutation() bool {
	return scope == EffectScopeWorkspaceWrite || scope == EffectScopeWorkspaceAndExternal
}

// MutationOutcome is conservative by construction. Possible means execution
// may have changed the workspace even when no post-action digest is available.
// A transaction that proves it restored the pre-action digest reports None.
type MutationOutcome string

const (
	MutationNone      MutationOutcome = "none"
	MutationCommitted MutationOutcome = "committed"
	MutationPossible  MutationOutcome = "possible"
)

// ExecutionOutcome is the transport-independent tool completion result.
type ExecutionOutcome string

const (
	ExecutionSucceeded ExecutionOutcome = "succeeded"
	ExecutionFailed    ExecutionOutcome = "failed"
	ExecutionCancelled ExecutionOutcome = "cancelled"
)

// VerificationKind classifies verification evidence. Observation and diff
// review are useful evidence but cannot make a flight completion-ready.
type VerificationKind string

const (
	VerificationNone           VerificationKind = "none"
	VerificationObservation    VerificationKind = "observation"
	VerificationDiffReview     VerificationKind = "diff_review"
	VerificationBuild          VerificationKind = "build"
	VerificationStaticAnalysis VerificationKind = "static_analysis"
	VerificationTargetedTest   VerificationKind = "targeted_test"
	VerificationFullTest       VerificationKind = "full_test"
	VerificationAcceptance     VerificationKind = "acceptance"
	VerificationInvariant      VerificationKind = "invariant"
)

// Relevant reports whether a passing result is strong enough to issue a
// verification receipt. The verification configuration digest binds the
// concrete command, scope, and policy selected by the caller.
func (kind VerificationKind) Relevant() bool {
	switch kind {
	case VerificationBuild,
		VerificationStaticAnalysis,
		VerificationTargetedTest,
		VerificationFullTest,
		VerificationAcceptance,
		VerificationInvariant:
		return true
	default:
		return false
	}
}

// VerificationOutcome is independent of process success because a test runner
// can execute correctly while reporting a failed test suite.
type VerificationOutcome string

const (
	VerificationNotRun       VerificationOutcome = "not_run"
	VerificationPassed       VerificationOutcome = "passed"
	VerificationFailed       VerificationOutcome = "failed"
	VerificationInconclusive VerificationOutcome = "inconclusive"
)

// FileFact is one content-free file observation associated with an action.
// Identity must be a caller-generated path identity, never a raw path.
type FileFact struct {
	Identity        Fingerprint     `json:"identity"`
	BeforeDigest    Digest          `json:"before_digest,omitempty"`
	AfterDigest     Digest          `json:"after_digest,omitempty"`
	MutationOutcome MutationOutcome `json:"mutation_outcome"`
}

// ToolExecutionFacts is the trusted scheduler-to-reducer contract. The action
// fingerprint must identify one physical execution (for example, include the
// tool-call ID), while failure fingerprints intentionally group equivalent
// failures across separate executions. ExecutionSequence is allocated and
// durably bound to that fingerprint by the single-writer execution WAL. It is
// contiguous: concurrent results must be reordered before Reduce, and replay
// must retain the original sequence rather than allocate a new one.
type ToolExecutionFacts struct {
	ToolID                   string              `json:"tool_id"`
	IntentID                 string              `json:"intent_id,omitempty"`
	ExecutionSequence        uint64              `json:"execution_sequence"`
	Invoked                  bool                `json:"invoked"`
	EffectScope              EffectScope         `json:"effect_scope"`
	ExecutionOutcome         ExecutionOutcome    `json:"execution_outcome"`
	MutationOutcome          MutationOutcome     `json:"mutation_outcome"`
	VerificationKind         VerificationKind    `json:"verification_kind"`
	VerificationOutcome      VerificationOutcome `json:"verification_outcome"`
	VerificationEpoch        Epoch               `json:"verification_epoch,omitempty"`
	VerificationConfigDigest Digest              `json:"verification_config_digest,omitempty"`
	BeforeDigest             Digest              `json:"before_digest,omitempty"`
	AfterDigest              Digest              `json:"after_digest,omitempty"`
	EvidenceDigest           Digest              `json:"evidence_digest,omitempty"`
	ActionFingerprint        Fingerprint         `json:"action_fingerprint"`
	FailureFingerprint       Fingerprint         `json:"failure_fingerprint,omitempty"`
	Files                    []FileFact          `json:"files,omitempty"`
}

// ConditionOutcome represents either an acceptance criterion or an invariant
// at one exact mutation epoch and workspace digest.
type ConditionOutcome string

const (
	ConditionPending     ConditionOutcome = "pending"
	ConditionSatisfied   ConditionOutcome = "satisfied"
	ConditionUnsatisfied ConditionOutcome = "unsatisfied"
)

// ConditionState stores only semantic IDs and evidence bindings. Descriptions
// remain in the owning goal contract and never enter this telemetry-safe core.
type ConditionState struct {
	ID              string           `json:"id"`
	Outcome         ConditionOutcome `json:"outcome"`
	MutationEpoch   Epoch            `json:"mutation_epoch,omitempty"`
	WorkspaceDigest Digest           `json:"workspace_digest,omitempty"`
	EvidenceDigest  Digest           `json:"evidence_digest,omitempty"`
}

type AcceptanceState struct {
	Criteria []ConditionState `json:"criteria"`
}

type InvariantState struct {
	Conditions []ConditionState `json:"conditions,omitempty"`
}

// ConditionEvaluation is accepted as current only when both its mutation epoch
// and workspace digest match the reducer state.
type ConditionEvaluation struct {
	ID              string           `json:"id"`
	Outcome         ConditionOutcome `json:"outcome"`
	MutationEpoch   Epoch            `json:"mutation_epoch"`
	WorkspaceDigest Digest           `json:"workspace_digest"`
	EvidenceDigest  Digest           `json:"evidence_digest"`
}

// IntentState is a bounded-flight ownership marker. A terminal completion can
// never be committed while an intent remains open.
type IntentState struct {
	ID                string      `json:"id"`
	ExecutionSequence uint64      `json:"execution_sequence"`
	ActionFingerprint Fingerprint `json:"action_fingerprint,omitempty"`
	OpenedRevision    Revision    `json:"opened_revision"`
}

// IntentResolution records why a non-tool event closes an intent.
type IntentResolution string

const (
	IntentCompleted IntentResolution = "completed"
	IntentAbandoned IntentResolution = "abandoned"
)

// VerificationReceipt is a self-checking correctness certificate bound to one
// workspace instance, task, policy configuration, mutation epoch, workspace
// content, and evidence artifact. BindingDigest detects corruption; receipt
// provenance must still be enforced by the caller's first-party trust boundary.
type VerificationReceipt struct {
	WorkspaceInstanceID      string           `json:"workspace_instance_id"`
	MutationEpoch            Epoch            `json:"mutation_epoch"`
	IssuedRevision           Revision         `json:"issued_revision"`
	VerificationGeneration   uint64           `json:"verification_generation"`
	WorkspaceDigest          Digest           `json:"workspace_digest"`
	TaskDigest               Digest           `json:"task_digest"`
	VerificationConfigDigest Digest           `json:"verification_config_digest"`
	EvidenceDigest           Digest           `json:"evidence_digest"`
	Kind                     VerificationKind `json:"kind"`
	ActionFingerprint        Fingerprint      `json:"action_fingerprint"`
	BindingDigest            Digest           `json:"binding_digest"`
}

// TerminalDisposition is the persisted flight terminal state.
type TerminalDisposition string

const (
	TerminalRunning   TerminalDisposition = "running"
	TerminalCompleted TerminalDisposition = "completed"
	TerminalBlocked   TerminalDisposition = "blocked"
	TerminalAborted   TerminalDisposition = "aborted"
)

// CompletionBlocker is a machine-readable completion-gate result.
type CompletionBlocker string

const (
	CompletionReady                CompletionBlocker = "ready"
	CompletionStateInvalid         CompletionBlocker = "state_invalid"
	CompletionAlreadyTerminal      CompletionBlocker = "already_terminal"
	CompletionIntentOpen           CompletionBlocker = "intent_open"
	CompletionWorkspaceUnknown     CompletionBlocker = "workspace_unknown"
	CompletionAcceptanceIncomplete CompletionBlocker = "acceptance_incomplete"
	CompletionInvariantIncomplete  CompletionBlocker = "invariant_incomplete"
	CompletionVerificationMissing  CompletionBlocker = "verification_missing"
	CompletionVerificationStale    CompletionBlocker = "verification_stale"
	CompletionReceiptBinding       CompletionBlocker = "receipt_binding"
	CompletionRepeatedFailure      CompletionBlocker = "repeated_failure"
)

type CompletionDecision struct {
	Allowed            bool              `json:"allowed"`
	Blocker            CompletionBlocker `json:"blocker"`
	OpenIntents        int               `json:"open_intents,omitempty"`
	PendingAcceptance  int               `json:"pending_acceptance,omitempty"`
	PendingInvariants  int               `json:"pending_invariants,omitempty"`
	ViolatedAcceptance int               `json:"violated_acceptance,omitempty"`
	ViolatedInvariants int               `json:"violated_invariants,omitempty"`
}

// EvidenceKind classifies bounded evidence ledger entries.
type EvidenceKind string

const (
	EvidenceObservation  EvidenceKind = "observation"
	EvidenceVerification EvidenceKind = "verification"
	EvidenceAcceptance   EvidenceKind = "acceptance"
	EvidenceInvariant    EvidenceKind = "invariant"
)

type ActionRecord struct {
	Revision             Revision            `json:"revision"`
	ToolID               string              `json:"tool_id"`
	IntentID             string              `json:"intent_id,omitempty"`
	ExecutionSequence    uint64              `json:"execution_sequence"`
	Invoked              bool                `json:"invoked"`
	ActionFingerprint    Fingerprint         `json:"action_fingerprint"`
	FactDigest           Digest              `json:"fact_digest"`
	ExecutionOutcome     ExecutionOutcome    `json:"execution_outcome"`
	EffectScope          EffectScope         `json:"effect_scope"`
	MutationOutcome      MutationOutcome     `json:"mutation_outcome"`
	MutationEpochBefore  Epoch               `json:"mutation_epoch_before"`
	MutationEpochAfter   Epoch               `json:"mutation_epoch_after"`
	WorkspaceDigestKnown bool                `json:"workspace_digest_known"`
	WorkspaceDigest      Digest              `json:"workspace_digest,omitempty"`
	VerificationKind     VerificationKind    `json:"verification_kind"`
	VerificationOutcome  VerificationOutcome `json:"verification_outcome"`
}

type FailureRecord struct {
	Revision              Revision    `json:"revision"`
	ActionFingerprint     Fingerprint `json:"action_fingerprint"`
	FailureFingerprint    Fingerprint `json:"failure_fingerprint"`
	RecentOccurrence      uint32      `json:"recent_occurrence"`
	ConsecutiveOccurrence uint32      `json:"consecutive_occurrence"`
}

type EvidenceRecord struct {
	Revision          Revision         `json:"revision"`
	Kind              EvidenceKind     `json:"kind"`
	Digest            Digest           `json:"digest"`
	MutationEpoch     Epoch            `json:"mutation_epoch"`
	WorkspaceDigest   Digest           `json:"workspace_digest,omitempty"`
	ActionFingerprint Fingerprint      `json:"action_fingerprint,omitempty"`
	ConditionID       string           `json:"condition_id,omitempty"`
	ConditionOutcome  ConditionOutcome `json:"condition_outcome,omitempty"`
}

type FileRecord struct {
	Revision          Revision        `json:"revision"`
	ActionFingerprint Fingerprint     `json:"action_fingerprint"`
	Identity          Fingerprint     `json:"identity"`
	BeforeDigest      Digest          `json:"before_digest,omitempty"`
	AfterDigest       Digest          `json:"after_digest,omitempty"`
	MutationOutcome   MutationOutcome `json:"mutation_outcome"`
}

// LedgerLimits bounds every high-cardinality collection and the two loop
// stagnation detectors. Zero values in StateSpec select production defaults.
type LedgerLimits struct {
	Actions                int    `json:"actions"`
	Failures               int    `json:"failures"`
	Evidence               int    `json:"evidence"`
	Files                  int    `json:"files"`
	Intents                int    `json:"intents"`
	RepeatedFailureTrigger uint32 `json:"repeated_failure_trigger"`
	NoProgressTrigger      uint32 `json:"no_progress_trigger"`
}

func DefaultLedgerLimits() LedgerLimits {
	return LedgerLimits{
		Actions:                128,
		Failures:               64,
		Evidence:               128,
		Files:                  256,
		Intents:                32,
		RepeatedFailureTrigger: 3,
		NoProgressTrigger:      8,
	}
}

// StateSpec establishes immutable receipt bindings and condition IDs.
type StateSpec struct {
	WorkspaceInstanceID      string
	WorkspaceDigest          Digest
	TaskDigest               Digest
	VerificationConfigDigest Digest
	AcceptanceCriteria       []string
	Invariants               []string
	Limits                   LedgerLimits
}

// FlightState is a detached reducer value. MutationEpoch is the correctness
// generation; WorkspaceRevision is only a CAS/observation sequence and must
// never be substituted for it.
type FlightState struct {
	SchemaVersion                   uint16               `json:"schema_version"`
	WorkspaceInstanceID             string               `json:"workspace_instance_id"`
	WorkspaceDigest                 Digest               `json:"workspace_digest,omitempty"`
	WorkspaceDigestKnown            bool                 `json:"workspace_digest_known"`
	TaskDigest                      Digest               `json:"task_digest"`
	VerificationConfigDigest        Digest               `json:"verification_config_digest"`
	MutationEpoch                   Epoch                `json:"mutation_epoch"`
	VerifiedEpoch                   Epoch                `json:"verified_epoch"`
	VerificationGeneration          uint64               `json:"verification_generation"`
	WorkspaceRevision               Revision             `json:"workspace_revision"`
	LedgerRevision                  Revision             `json:"ledger_revision"`
	VerificationReceipt             *VerificationReceipt `json:"verification_receipt,omitempty"`
	VerificationInvalidatedRevision Revision             `json:"verification_invalidated_revision,omitempty"`
	Acceptance                      AcceptanceState      `json:"acceptance"`
	Invariants                      InvariantState       `json:"invariants"`
	PendingIntents                  []IntentState        `json:"pending_intents,omitempty"`
	Actions                         []ActionRecord       `json:"actions,omitempty"`
	Failures                        []FailureRecord      `json:"failures,omitempty"`
	Evidence                        []EvidenceRecord     `json:"evidence,omitempty"`
	Files                           []FileRecord         `json:"files,omitempty"`
	LastFailureFingerprint          Fingerprint          `json:"last_failure_fingerprint,omitempty"`
	ConsecutiveFailures             uint32               `json:"consecutive_failures,omitempty"`
	NoProgressStreak                uint32               `json:"no_progress_streak,omitempty"`
	LastExecutionSequence           uint64               `json:"last_execution_sequence,omitempty"`
	TerminalDisposition             TerminalDisposition  `json:"terminal_disposition"`
	Limits                          LedgerLimits         `json:"limits"`
}

// Event is a closed reducer input family.
type Event interface {
	flightEvent()
}

type IntentOpened struct {
	ID                string
	ExecutionSequence uint64
	ActionFingerprint Fingerprint
}

func (IntentOpened) flightEvent() {}

type IntentClosed struct {
	ID                string
	ExecutionSequence uint64
	ActionFingerprint Fingerprint
	Resolution        IntentResolution
}

func (IntentClosed) flightEvent() {}

type ToolExecuted struct {
	Facts ToolExecutionFacts
}

func (ToolExecuted) flightEvent() {}

type ConditionsEvaluated struct {
	Acceptance []ConditionEvaluation
	Invariants []ConditionEvaluation
}

func (ConditionsEvaluated) flightEvent() {}

// ReceiptPresented revalidates a first-party persisted receipt after resume.
// It never manufactures a receipt; malformed or stale bindings are rejected.
type ReceiptPresented struct {
	Receipt VerificationReceipt
}

func (ReceiptPresented) flightEvent() {}

type TerminalRequested struct {
	Disposition TerminalDisposition
}

func (TerminalRequested) flightEvent() {}

// ReceiptDisposition reports the result of either a verification result or a
// persisted receipt presentation.
type ReceiptDisposition string

const (
	ReceiptNone     ReceiptDisposition = "none"
	ReceiptIssued   ReceiptDisposition = "issued"
	ReceiptAccepted ReceiptDisposition = "accepted"
	ReceiptStale    ReceiptDisposition = "stale"
)

type ProgressSignals struct {
	Mutation     bool `json:"mutation"`
	Evidence     bool `json:"evidence"`
	Verification bool `json:"verification"`
	Conditions   bool `json:"conditions"`
}

func (signals ProgressSignals) Any() bool {
	return signals.Mutation || signals.Evidence || signals.Verification || signals.Conditions
}

// ReduceEffects contains only machine-readable scheduler facts.
type ReduceEffects struct {
	StateChanged              bool                 `json:"state_changed"`
	LedgerRevision            Revision             `json:"ledger_revision"`
	MutationAdvanced          bool                 `json:"mutation_advanced"`
	WorkspaceReconciled       bool                 `json:"workspace_reconciled"`
	VerificationInvalidated   bool                 `json:"verification_invalidated"`
	ReceiptDisposition        ReceiptDisposition   `json:"receipt_disposition"`
	Receipt                   *VerificationReceipt `json:"receipt,omitempty"`
	DuplicateAction           bool                 `json:"duplicate_action"`
	StaleVerification         bool                 `json:"stale_verification"`
	ConditionsUpdated         int                  `json:"conditions_updated"`
	StaleConditionEvaluations int                  `json:"stale_condition_evaluations"`
	Progress                  ProgressSignals      `json:"progress"`
	RepeatedFailure           bool                 `json:"repeated_failure"`
	NoProgress                bool                 `json:"no_progress"`
	RecommendedDisposition    TerminalDisposition  `json:"recommended_disposition,omitempty"`
	Completion                CompletionDecision   `json:"completion"`
}

// ErrorCode is a stable internal transition rejection reason.
type ErrorCode string

const (
	ErrorInvalidSpec         ErrorCode = "invalid_spec"
	ErrorInvalidState        ErrorCode = "invalid_state"
	ErrorInvalidEvent        ErrorCode = "invalid_event"
	ErrorInvalidFacts        ErrorCode = "invalid_facts"
	ErrorStaleWorkspaceFact  ErrorCode = "stale_workspace_fact"
	ErrorConflictingReplay   ErrorCode = "conflicting_replay"
	ErrorStaleExecution      ErrorCode = "stale_execution"
	ErrorExecutionGap        ErrorCode = "execution_gap"
	ErrorIntentNotFound      ErrorCode = "intent_not_found"
	ErrorIntentExists        ErrorCode = "intent_exists"
	ErrorIntentMismatch      ErrorCode = "intent_mismatch"
	ErrorIntentLimit         ErrorCode = "intent_limit"
	ErrorConditionNotFound   ErrorCode = "condition_not_found"
	ErrorConflictingEvidence ErrorCode = "conflicting_evidence"
	ErrorTerminalState       ErrorCode = "terminal_state"
	ErrorEpochOverflow       ErrorCode = "epoch_overflow"
	ErrorGenerationOverflow  ErrorCode = "generation_overflow"
	ErrorRevisionOverflow    ErrorCode = "revision_overflow"
)

// TransitionError never contains user-visible copy.
type TransitionError struct {
	Code ErrorCode
}

func (err *TransitionError) Error() string {
	if err == nil {
		return "flight.nil"
	}
	return "flight." + string(err.Code)
}

func transitionError(code ErrorCode) error {
	return &TransitionError{Code: code}
}
