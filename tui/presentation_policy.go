package tui

import "fmt"

// PresentationLevel is the renderer-neutral disclosure decision. It is kept
// separate from DisclosureLevel so persisted Summary/Detail/Evidence values
// remain backward compatible while D0 exists only in the visible projection.
type PresentationLevel uint8

const (
	PresentationHiddenMember PresentationLevel = iota
	PresentationFolded
	PresentationStructured
	PresentationEvidence
)

func (l PresentationLevel) String() string {
	switch l {
	case PresentationHiddenMember:
		return "hidden_member"
	case PresentationFolded:
		return "folded"
	case PresentationStructured:
		return "structured"
	case PresentationEvidence:
		return "evidence"
	default:
		return fmt.Sprintf("presentation_level_%d", l)
	}
}

type PresentationSurface string

const (
	SurfaceNone       PresentationSurface = "none"
	SurfaceTranscript PresentationSurface = "transcript"
	SurfaceWorkView   PresentationSurface = "work_view"
	SurfaceOverlay    PresentationSurface = "overlay"
	SurfacePager      PresentationSurface = "pager"
)

type CommandFamily string

const (
	FamilyUnknown   CommandFamily = "unknown"
	FamilyShell     CommandFamily = "shell"
	FamilyFileRead  CommandFamily = "file_read"
	FamilyFileWrite CommandFamily = "file_write"
	FamilySearch    CommandFamily = "search"
	FamilyWeb       CommandFamily = "web"
	FamilyMCP       CommandFamily = "mcp"
	FamilyAgent     CommandFamily = "agent"
	FamilyTask      CommandFamily = "task"
	FamilyGoal      CommandFamily = "goal"
	FamilyDecision  CommandFamily = "decision"
	FamilyMessage   CommandFamily = "message"
	FamilyTeam      CommandFamily = "team"
	FamilyCron      CommandFamily = "cron"
	FamilyWorktree  CommandFamily = "worktree"
	FamilyConfig    CommandFamily = "config"
	FamilySkill     CommandFamily = "skill"
	FamilyRemote    CommandFamily = "remote"
	FamilyInternal  CommandFamily = "internal"
)

type PresentationRisk string

const (
	RiskUnknown     PresentationRisk = "unknown"
	RiskLow         PresentationRisk = "low"
	RiskMedium      PresentationRisk = "medium"
	RiskHigh        PresentationRisk = "high"
	RiskDestructive PresentationRisk = "destructive"
)

type PresentationReason string

const (
	ReasonRedacted             PresentationReason = "redacted"
	ReasonUserFull             PresentationReason = "user_full"
	ReasonUserAudit            PresentationReason = "user_audit"
	ReasonPinnedEvidence       PresentationReason = "pinned_evidence"
	ReasonDecision             PresentationReason = "decision"
	ReasonDestructive          PresentationReason = "destructive_pre_action"
	ReasonUserOutput           PresentationReason = "user_output"
	ReasonIdentityUnreliable   PresentationReason = "identity_unreliable"
	ReasonEvidenceIntegrity    PresentationReason = "evidence_integrity_failed"
	ReasonNonSuccess           PresentationReason = "non_success"
	ReasonWarning              PresentationReason = "warning"
	ReasonRetrying             PresentationReason = "retrying"
	ReasonStalled              PresentationReason = "stalled"
	ReasonTruncated            PresentationReason = "truncated"
	ReasonScopeExpanded        PresentationReason = "scope_expanded"
	ReasonSideEffect           PresentationReason = "side_effect"
	ReasonNeedsReview          PresentationReason = "needs_review"
	ReasonPlanGate             PresentationReason = "plan_gate"
	ReasonTerminalAgent        PresentationReason = "terminal_agent"
	ReasonHighRisk             PresentationReason = "high_risk"
	ReasonExplicitCollapse     PresentationReason = "explicit_collapse"
	ReasonUserInspect          PresentationReason = "user_inspect"
	ReasonRiskUnknown          PresentationReason = "risk_unknown"
	ReasonRunning              PresentationReason = "running"
	ReasonSafeFallback         PresentationReason = "safe_fallback"
	ReasonRoutineSuccess       PresentationReason = "routine_success"
	ReasonAggregationCandidate PresentationReason = "aggregation_candidate"
	// ReasonAggregateMember is retained for persisted compatibility. The policy
	// no longer emits it: only the aggregation projection may make a member D0.
	ReasonAggregateMember  PresentationReason = "aggregate_member"
	ReasonPresentationTick PresentationReason = "presentation_tick"
)

type PresentationIntent struct {
	Quiet               bool
	Inspect             bool
	Full                bool
	Audit               bool
	PinnedEvidence      bool
	ExplicitlyCollapsed bool
}

// PresentationFacts contains only deterministic execution and user-intent
// facts. The policy must never parse human result prose to derive these fields.
type PresentationFacts struct {
	Family                  CommandFamily
	Outcome                 ObservationOutcome
	Risk                    PresentationRisk
	HasEvidence             bool
	SideEffect              bool
	NeedsReview             bool
	PlanGate                bool
	TerminalAgentResult     bool
	RequiresDecision        bool
	DestructivePreAction    bool
	Warning                 bool
	Retrying                bool
	Stalled                 bool
	Truncated               bool
	ScopeExpanded           bool
	IdentityUnreliable      bool
	EvidenceIntegrityFailed bool
	UserAuthored            bool
	AssistantFinal          bool
	DirectlyRequestedOutput bool
	PresentationOnlyTick    bool
	SemanticStateChanged    bool
	Background              bool
	Repeated                bool
	Large                   bool
	Narrow                  bool
	Sensitive               bool
	Intent                  PresentationIntent
}

type PresentationDecision struct {
	DefaultLevel        PresentationLevel     `json:"default_level"`
	EffectiveLevel      PresentationLevel     `json:"effective_level"`
	Level               PresentationLevel     `json:"level"`
	Surface             PresentationSurface   `json:"surface"`
	Surfaces            []PresentationSurface `json:"surfaces,omitempty"`
	Reasons             []PresentationReason  `json:"reasons,omitempty"`
	AggregationEligible bool                  `json:"aggregation_eligible,omitempty"`
	Redacted            bool                  `json:"redacted,omitempty"`
}

func (d PresentationDecision) DisclosureLevel() DisclosureLevel {
	level := d.EffectiveLevel
	if level == PresentationHiddenMember && d.Level != PresentationHiddenMember {
		level = d.Level
	}
	switch level {
	case PresentationStructured:
		return DisclosureDetail
	case PresentationEvidence:
		return DisclosureEvidence
	default:
		return DisclosureSummary
	}
}

func DecidePresentation(facts PresentationFacts) PresentationDecision {
	reasons := make(map[PresentationReason]struct{})
	addReason := func(reason PresentationReason) { reasons[reason] = struct{}{} }
	if facts.Sensitive {
		addReason(ReasonRedacted)
	}

	// P10 is exact, not a low floor. A contradictory fact set must be promoted
	// through the normal policy instead of allowing the tick flag to hide it.
	if facts.PresentationOnlyTick && !facts.SemanticStateChanged && !hasMaterialPresentationFact(facts) {
		addReason(ReasonPresentationTick)
		return finalizePresentationDecision(PresentationDecision{
			DefaultLevel:   PresentationHiddenMember,
			EffectiveLevel: PresentationHiddenMember,
			Level:          PresentationHiddenMember,
			Surface:        SurfaceNone,
			Surfaces:       []PresentationSurface{SurfaceNone},
			Redacted:       facts.Sensitive,
		}, reasons)
	}

	decision := PresentationDecision{
		DefaultLevel:   PresentationFolded,
		EffectiveLevel: PresentationFolded,
		Level:          PresentationFolded,
		Surface:        SurfaceTranscript,
		Surfaces:       []PresentationSurface{SurfaceTranscript},
		Redacted:       facts.Sensitive,
	}

	decision.AggregationEligible = presentationCanAggregate(facts)

	risk := facts.Risk
	if risk == "" {
		risk = RiskUnknown
	}
	if risk == RiskUnknown {
		addReason(ReasonRiskUnknown)
	}
	if facts.Family == FamilyUnknown || facts.Family == FamilyInternal {
		addReason(ReasonSafeFallback)
	} else if facts.Outcome == OutcomeSucceeded {
		addReason(ReasonRoutineSuccess)
	}
	if facts.Outcome == OutcomeRunning || facts.Outcome == OutcomeUnknown {
		addReason(ReasonRunning)
	}

	if facts.IdentityUnreliable || facts.Outcome == OutcomeOrphan || facts.Outcome == OutcomeConflict {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonIdentityUnreliable)
	}
	if facts.EvidenceIntegrityFailed {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonEvidenceIntegrity)
	}
	if isNonSuccessPresentationOutcome(facts.Outcome) {
		decision.Level = PresentationStructured
		decision.AggregationEligible = false
		addReason(ReasonNonSuccess)
	}
	if facts.Warning {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonWarning)
	}
	if facts.Retrying {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonRetrying)
	}
	if facts.Stalled {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonStalled)
	}
	if facts.Truncated {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonTruncated)
	}
	if facts.ScopeExpanded {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonScopeExpanded)
	}
	if facts.SideEffect {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonSideEffect)
	}
	if facts.NeedsReview {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonNeedsReview)
	}
	if facts.PlanGate {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonPlanGate)
	}
	if facts.TerminalAgentResult {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonTerminalAgent)
	}
	if risk == RiskHigh || risk == RiskDestructive {
		decision.Level = maxPresentationLevel(decision.Level, PresentationStructured)
		decision.AggregationEligible = false
		addReason(ReasonHighRisk)
	}

	if facts.RequiresDecision {
		decision.Level = PresentationEvidence
		decision.AggregationEligible = false
		addReason(ReasonDecision)
	}
	if facts.DestructivePreAction {
		decision.Level = PresentationEvidence
		decision.AggregationEligible = false
		addReason(ReasonDestructive)
	}
	if facts.UserAuthored || facts.AssistantFinal || facts.DirectlyRequestedOutput {
		decision.Level = PresentationEvidence
		decision.AggregationEligible = false
		addReason(ReasonUserOutput)
	}

	if facts.Intent.Full || facts.Intent.Audit || facts.Intent.PinnedEvidence {
		decision.Level = PresentationEvidence
		decision.AggregationEligible = false
		if facts.Intent.Full {
			addReason(ReasonUserFull)
		}
		if facts.Intent.Audit {
			addReason(ReasonUserAudit)
		}
		if facts.Intent.PinnedEvidence {
			addReason(ReasonPinnedEvidence)
		}
	}
	if facts.Intent.Inspect && decision.Level < PresentationEvidence {
		decision.Level++
		decision.AggregationEligible = false
		addReason(ReasonUserInspect)
	}

	if facts.Repeated && decision.Level == PresentationFolded && decision.AggregationEligible {
		addReason(ReasonAggregationCandidate)
	}

	decision.DefaultLevel = decision.Level
	decision.EffectiveLevel = decision.DefaultLevel
	if facts.Intent.ExplicitlyCollapsed && decision.DefaultLevel == PresentationStructured {
		decision.EffectiveLevel = PresentationFolded
		addReason(ReasonExplicitCollapse)
	}
	decision.Level = decision.EffectiveLevel

	switch {
	case facts.RequiresDecision || facts.DestructivePreAction:
		decision.Surface = SurfaceOverlay
		decision.Surfaces = []PresentationSurface{SurfaceOverlay}
	case decision.DefaultLevel == PresentationEvidence && (facts.Intent.Full || facts.Intent.Audit || facts.Intent.PinnedEvidence || facts.Large || facts.Narrow):
		decision.Surface = SurfacePager
		decision.Surfaces = []PresentationSurface{SurfacePager}
	case facts.Background:
		decision.Surface = SurfaceWorkView
		decision.Surfaces = []PresentationSurface{SurfaceWorkView}
	default:
		decision.Surface = SurfaceTranscript
		decision.Surfaces = []PresentationSurface{SurfaceTranscript}
	}
	if facts.Background && isTerminalPresentationOutcome(facts.Outcome) {
		decision.Surfaces = appendPresentationSurface(decision.Surfaces, SurfaceWorkView)
		decision.Surfaces = appendPresentationSurface(decision.Surfaces, SurfaceTranscript)
	}
	return finalizePresentationDecision(decision, reasons)
}

func presentationCanAggregate(facts PresentationFacts) bool {
	if facts.Outcome != OutcomeSucceeded || facts.SideEffect || facts.NeedsReview || facts.RequiresDecision ||
		facts.PlanGate || facts.TerminalAgentResult || facts.DestructivePreAction || facts.Warning ||
		facts.Retrying || facts.Stalled || facts.Truncated || facts.ScopeExpanded || facts.Sensitive ||
		facts.IdentityUnreliable || facts.EvidenceIntegrityFailed || facts.UserAuthored || facts.AssistantFinal ||
		facts.DirectlyRequestedOutput || facts.PresentationOnlyTick ||
		facts.Risk == RiskHigh || facts.Risk == RiskDestructive || facts.Intent.Inspect || facts.Intent.Full ||
		facts.Intent.Audit || facts.Intent.PinnedEvidence {
		return false
	}
	if facts.Risk != RiskLow {
		return false
	}
	switch facts.Family {
	case FamilyFileRead, FamilySearch, FamilyWeb:
		return true
	case FamilyMCP:
		return true
	default:
		return false
	}
}

func hasMaterialPresentationFact(facts PresentationFacts) bool {
	return facts.RequiresDecision || facts.DestructivePreAction || facts.UserAuthored || facts.AssistantFinal ||
		facts.DirectlyRequestedOutput || facts.IdentityUnreliable || facts.EvidenceIntegrityFailed ||
		isNonSuccessPresentationOutcome(facts.Outcome) || facts.Warning || facts.Retrying || facts.Stalled ||
		facts.Truncated || facts.ScopeExpanded || facts.SideEffect || facts.NeedsReview || facts.PlanGate ||
		facts.TerminalAgentResult || facts.Risk == RiskHigh || facts.Risk == RiskDestructive ||
		facts.Intent.Inspect || facts.Intent.Full || facts.Intent.Audit ||
		facts.Intent.PinnedEvidence || facts.Intent.ExplicitlyCollapsed
}

func isTerminalPresentationOutcome(outcome ObservationOutcome) bool {
	return outcome != OutcomeUnknown && outcome != OutcomeRunning
}

func appendPresentationSurface(surfaces []PresentationSurface, surface PresentationSurface) []PresentationSurface {
	for _, current := range surfaces {
		if current == surface {
			return surfaces
		}
	}
	return append(surfaces, surface)
}

var presentationReasonOrder = []PresentationReason{
	// P0
	ReasonRedacted,
	// P1
	ReasonUserFull, ReasonUserAudit, ReasonPinnedEvidence,
	// P2
	ReasonDecision, ReasonDestructive,
	// P3
	ReasonUserOutput,
	// P4
	ReasonIdentityUnreliable, ReasonEvidenceIntegrity,
	// P5
	ReasonNonSuccess,
	// P6
	ReasonWarning, ReasonRetrying, ReasonStalled, ReasonTruncated, ReasonScopeExpanded,
	// P7 and explicit user modifiers.
	ReasonSideEffect, ReasonNeedsReview, ReasonPlanGate, ReasonTerminalAgent, ReasonHighRisk,
	ReasonExplicitCollapse, ReasonUserInspect,
	// P8
	ReasonRiskUnknown, ReasonSafeFallback, ReasonRunning, ReasonRoutineSuccess,
	// P9
	ReasonAggregationCandidate, ReasonAggregateMember,
	// P10
	ReasonPresentationTick,
}

func finalizePresentationDecision(decision PresentationDecision, reasons map[PresentationReason]struct{}) PresentationDecision {
	decision.Reasons = decision.Reasons[:0]
	for _, reason := range presentationReasonOrder {
		if _, ok := reasons[reason]; ok {
			decision.Reasons = append(decision.Reasons, reason)
			delete(reasons, reason)
		}
	}
	// Unknown future reason codes are deliberately omitted here. The policy owns
	// the closed reason vocabulary so persisted output stays deterministic.
	return decision
}

func isNonSuccessPresentationOutcome(outcome ObservationOutcome) bool {
	switch outcome {
	case OutcomeFailed, OutcomePartial, OutcomeDenied, OutcomeCancelled, OutcomeTimedOut, OutcomeEscaped, OutcomeShutdown, OutcomeOrphan, OutcomeConflict:
		return true
	default:
		return false
	}
}

func maxPresentationLevel(left, right PresentationLevel) PresentationLevel {
	if right > left {
		return right
	}
	return left
}
