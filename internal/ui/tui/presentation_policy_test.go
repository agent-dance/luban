package tui

import (
	"reflect"
	"testing"
)

func TestDecidePresentationPriorityTable(t *testing.T) {
	tests := []struct {
		name          string
		facts         PresentationFacts
		wantLevel     PresentationLevel
		wantSurface   PresentationSurface
		wantAggregate bool
		wantRedacted  bool
		wantReason    PresentationReason
	}{
		{
			name:      "routine read success folds",
			facts:     PresentationFacts{Family: FamilyFileRead, Outcome: OutcomeSucceeded, Risk: RiskLow, HasEvidence: true},
			wantLevel: PresentationFolded, wantSurface: SurfaceTranscript, wantAggregate: true, wantReason: ReasonRoutineSuccess,
		},
		{
			name:      "quiet repeated read remains visible aggregation candidate",
			facts:     PresentationFacts{Family: FamilyFileRead, Outcome: OutcomeSucceeded, Risk: RiskLow, HasEvidence: true, Repeated: true, Intent: PresentationIntent{Quiet: true}},
			wantLevel: PresentationFolded, wantSurface: SurfaceTranscript, wantAggregate: true, wantReason: ReasonAggregationCandidate,
		},
		{
			name:      "failure beats quiet and volume",
			facts:     PresentationFacts{Family: FamilyShell, Outcome: OutcomeFailed, HasEvidence: true, Large: true, Repeated: true, Intent: PresentationIntent{Quiet: true}},
			wantLevel: PresentationStructured, wantSurface: SurfaceTranscript, wantReason: ReasonNonSuccess,
		},
		{
			name:      "decision uses complete overlay",
			facts:     PresentationFacts{Family: FamilyDecision, Outcome: OutcomeRunning, RequiresDecision: true},
			wantLevel: PresentationEvidence, wantSurface: SurfaceOverlay, wantReason: ReasonDecision,
		},
		{
			name:      "side effect success stays structured",
			facts:     PresentationFacts{Family: FamilyFileWrite, Outcome: OutcomeSucceeded, Risk: RiskMedium, SideEffect: true, HasEvidence: true},
			wantLevel: PresentationStructured, wantSurface: SurfaceTranscript, wantReason: ReasonSideEffect,
		},
		{
			name:      "warning stays structured",
			facts:     PresentationFacts{Family: FamilyWeb, Outcome: OutcomeSucceeded, Risk: RiskLow, Warning: true, HasEvidence: true},
			wantLevel: PresentationStructured, wantSurface: SurfaceTranscript, wantReason: ReasonWarning,
		},
		{
			name:      "background running uses work view",
			facts:     PresentationFacts{Family: FamilyAgent, Outcome: OutcomeRunning, Background: true},
			wantLevel: PresentationFolded, wantSurface: SurfaceWorkView, wantReason: ReasonRunning,
		},
		{
			name:      "inspect raises routine success one level",
			facts:     PresentationFacts{Family: FamilyFileRead, Outcome: OutcomeSucceeded, Risk: RiskLow, HasEvidence: true, Intent: PresentationIntent{Inspect: true}},
			wantLevel: PresentationStructured, wantSurface: SurfaceTranscript, wantReason: ReasonUserInspect,
		},
		{
			name:      "full evidence retains redaction",
			facts:     PresentationFacts{Family: FamilyMCP, Outcome: OutcomeSucceeded, Risk: RiskLow, HasEvidence: true, Sensitive: true, Intent: PresentationIntent{Full: true}},
			wantLevel: PresentationEvidence, wantSurface: SurfacePager, wantRedacted: true, wantReason: ReasonUserFull,
		},
		{
			name:      "unknown success uses safe folded fallback",
			facts:     PresentationFacts{Family: FamilyUnknown, Outcome: OutcomeSucceeded, HasEvidence: true},
			wantLevel: PresentationFolded, wantSurface: SurfaceTranscript, wantReason: ReasonSafeFallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecidePresentation(tt.facts)
			if got.DefaultLevel != tt.wantLevel || got.EffectiveLevel != tt.wantLevel || got.Surface != tt.wantSurface {
				t.Fatalf("decision = %+v, want level=%s surface=%s", got, tt.wantLevel, tt.wantSurface)
			}
			if got.AggregationEligible != tt.wantAggregate {
				t.Fatalf("AggregationEligible = %t, want %t", got.AggregationEligible, tt.wantAggregate)
			}
			if got.Redacted != tt.wantRedacted {
				t.Fatalf("Redacted = %t, want %t", got.Redacted, tt.wantRedacted)
			}
			if !containsPresentationReason(got.Reasons, tt.wantReason) {
				t.Fatalf("Reasons = %v, want %q", got.Reasons, tt.wantReason)
			}
		})
	}
}

func TestDecidePresentationP10IsExclusive(t *testing.T) {
	tick := DecidePresentation(PresentationFacts{
		Family: FamilyUnknown, Outcome: OutcomeRunning, PresentationOnlyTick: true,
	})
	if tick.EffectiveLevel != PresentationHiddenMember || tick.Surface != SurfaceNone || tick.AggregationEligible {
		t.Fatalf("presentation-only tick = %+v, want exact D0 on no surface", tick)
	}
	if !reflect.DeepEqual(tick.Reasons, []PresentationReason{ReasonPresentationTick}) {
		t.Fatalf("tick reasons = %v", tick.Reasons)
	}

	tests := []struct {
		name  string
		facts PresentationFacts
	}{
		{name: "semantic change", facts: PresentationFacts{SemanticStateChanged: true}},
		{name: "warning", facts: PresentationFacts{Warning: true}},
		{name: "failure", facts: PresentationFacts{Outcome: OutcomeFailed}},
		{name: "decision", facts: PresentationFacts{RequiresDecision: true}},
		{name: "full intent", facts: PresentationFacts{Intent: PresentationIntent{Full: true}}},
		{name: "high risk", facts: PresentationFacts{Risk: RiskHigh}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.facts.Family = FamilyUnknown
			tt.facts.PresentationOnlyTick = true
			got := DecidePresentation(tt.facts)
			if got.EffectiveLevel == PresentationHiddenMember || got.Surface == SurfaceNone {
				t.Fatalf("material tick facts were hidden: %+v", got)
			}
		})
	}
}

func TestDecidePresentationRiskUnknownFailsClosedForAggregation(t *testing.T) {
	tests := []struct {
		name string
		risk PresentationRisk
		want bool
	}{
		{name: "zero value", risk: "", want: false},
		{name: "unknown", risk: RiskUnknown, want: false},
		{name: "medium", risk: RiskMedium, want: false},
		{name: "high", risk: RiskHigh, want: false},
		{name: "destructive", risk: RiskDestructive, want: false},
		{name: "explicit low", risk: RiskLow, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecidePresentation(PresentationFacts{Family: FamilyFileRead, Outcome: OutcomeSucceeded, Risk: tt.risk})
			if got.AggregationEligible != tt.want {
				t.Fatalf("risk %q aggregation = %t, want %t: %+v", tt.risk, got.AggregationEligible, tt.want, got)
			}
			if tt.risk == "" || tt.risk == RiskUnknown {
				if !containsPresentationReason(got.Reasons, ReasonRiskUnknown) {
					t.Fatalf("unknown risk reason missing: %+v", got)
				}
			}
		})
	}
}

func TestDecidePresentationAggregationNeverReturnsD0(t *testing.T) {
	got := DecidePresentation(PresentationFacts{
		Family: FamilySearch, Outcome: OutcomeSucceeded, Risk: RiskLow,
		Repeated: true, Intent: PresentationIntent{Quiet: true},
	})
	if !got.AggregationEligible || got.EffectiveLevel != PresentationFolded {
		t.Fatalf("aggregation candidate = %+v, want visible D1 candidate", got)
	}
	if containsPresentationReason(got.Reasons, ReasonAggregateMember) {
		t.Fatalf("policy emitted projection-owned aggregate-member reason: %+v", got)
	}
}

func TestDecidePresentationDistinguishesDestructivePreAndPostAction(t *testing.T) {
	pre := DecidePresentation(PresentationFacts{
		Family: FamilyShell, Outcome: OutcomeRunning, Risk: RiskDestructive, DestructivePreAction: true,
	})
	if pre.EffectiveLevel != PresentationEvidence || pre.Surface != SurfaceOverlay || !containsPresentationReason(pre.Reasons, ReasonDestructive) {
		t.Fatalf("destructive pre-action = %+v", pre)
	}

	post := DecidePresentation(PresentationFacts{
		Family: FamilyShell, Outcome: OutcomeFailed, Risk: RiskDestructive, SideEffect: true,
	})
	if post.EffectiveLevel != PresentationStructured || post.Surface != SurfaceTranscript || containsPresentationReason(post.Reasons, ReasonDestructive) {
		t.Fatalf("destructive post-action failure = %+v, want D2 receipt without decision overlay", post)
	}
}

func TestDecidePresentationDefaultAndEffectiveLevels(t *testing.T) {
	collapsed := DecidePresentation(PresentationFacts{
		Family: FamilyShell, Outcome: OutcomeFailed,
		Intent: PresentationIntent{ExplicitlyCollapsed: true},
	})
	if collapsed.DefaultLevel != PresentationStructured || collapsed.EffectiveLevel != PresentationFolded {
		t.Fatalf("explicitly collapsed failure = %+v", collapsed)
	}
	if !containsPresentationReason(collapsed.Reasons, ReasonExplicitCollapse) {
		t.Fatalf("explicit collapse reason missing: %+v", collapsed)
	}

	for _, facts := range []PresentationFacts{
		{Family: FamilyDecision, Outcome: OutcomeRunning, RequiresDecision: true, Intent: PresentationIntent{ExplicitlyCollapsed: true}},
		{Family: FamilyFileRead, Outcome: OutcomeSucceeded, Intent: PresentationIntent{Full: true, ExplicitlyCollapsed: true}},
	} {
		got := DecidePresentation(facts)
		if got.DefaultLevel != PresentationEvidence || got.EffectiveLevel != PresentationEvidence {
			t.Fatalf("D3 was lowered by explicit collapse: %+v", got)
		}
	}
}

func TestDecidePresentationBackgroundTerminalHasTransitionSurface(t *testing.T) {
	completed := DecidePresentation(PresentationFacts{
		Family: FamilyAgent, Outcome: OutcomeSucceeded, Risk: RiskMedium,
		Background: true, TerminalAgentResult: true,
	})
	if completed.Surface != SurfaceWorkView || !reflect.DeepEqual(completed.Surfaces, []PresentationSurface{SurfaceWorkView, SurfaceTranscript}) {
		t.Fatalf("background terminal surfaces = %+v", completed)
	}

	audit := DecidePresentation(PresentationFacts{
		Family: FamilyAgent, Outcome: OutcomeFailed, Risk: RiskMedium,
		Background: true, TerminalAgentResult: true, Intent: PresentationIntent{Audit: true},
	})
	want := []PresentationSurface{SurfacePager, SurfaceWorkView, SurfaceTranscript}
	if audit.Surface != SurfacePager || !reflect.DeepEqual(audit.Surfaces, want) {
		t.Fatalf("background audit surfaces = %v, want %v", audit.Surfaces, want)
	}
}

func TestDecidePresentationCoversP3P4AndP6Facts(t *testing.T) {
	tests := []struct {
		name   string
		facts  PresentationFacts
		level  PresentationLevel
		reason PresentationReason
	}{
		{name: "directly requested output", facts: PresentationFacts{DirectlyRequestedOutput: true}, level: PresentationEvidence, reason: ReasonUserOutput},
		{name: "assistant final", facts: PresentationFacts{AssistantFinal: true}, level: PresentationEvidence, reason: ReasonUserOutput},
		{name: "identity", facts: PresentationFacts{IdentityUnreliable: true}, level: PresentationStructured, reason: ReasonIdentityUnreliable},
		{name: "evidence integrity", facts: PresentationFacts{EvidenceIntegrityFailed: true}, level: PresentationStructured, reason: ReasonEvidenceIntegrity},
		{name: "scope expanded", facts: PresentationFacts{ScopeExpanded: true}, level: PresentationStructured, reason: ReasonScopeExpanded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecidePresentation(tt.facts)
			if got.EffectiveLevel != tt.level || !containsPresentationReason(got.Reasons, tt.reason) {
				t.Fatalf("decision = %+v, want %s due to %s", got, tt.level, tt.reason)
			}
		})
	}
}

func TestDecidePresentationReasonsFollowP0ToP10Order(t *testing.T) {
	facts := PresentationFacts{
		Family: FamilyFileWrite, Outcome: OutcomePartial, Risk: RiskDestructive,
		Sensitive: true, IdentityUnreliable: true, EvidenceIntegrityFailed: true,
		Warning: true, Retrying: true, Stalled: true, Truncated: true, ScopeExpanded: true,
		SideEffect: true, NeedsReview: true, PlanGate: true, TerminalAgentResult: true,
		RequiresDecision: true, DestructivePreAction: true, DirectlyRequestedOutput: true,
		Intent: PresentationIntent{Full: true, Audit: true, PinnedEvidence: true},
	}
	want := []PresentationReason{
		ReasonRedacted,
		ReasonUserFull, ReasonUserAudit, ReasonPinnedEvidence,
		ReasonDecision, ReasonDestructive,
		ReasonUserOutput,
		ReasonIdentityUnreliable, ReasonEvidenceIntegrity,
		ReasonNonSuccess,
		ReasonWarning, ReasonRetrying, ReasonStalled, ReasonTruncated, ReasonScopeExpanded,
		ReasonSideEffect, ReasonNeedsReview, ReasonPlanGate, ReasonTerminalAgent, ReasonHighRisk,
	}
	first := DecidePresentation(facts)
	second := DecidePresentation(facts)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same facts produced different decisions:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if !reflect.DeepEqual(first.Reasons, want) {
		t.Fatalf("reason order =\n%v\nwant\n%v", first.Reasons, want)
	}
}

func TestPresentationDecisionDisclosureMapping(t *testing.T) {
	if DisclosureSummary != 0 || DisclosureDetail != 1 || DisclosureEvidence != 2 {
		t.Fatalf("persisted DisclosureLevel numeric contract changed: %d/%d/%d", DisclosureSummary, DisclosureDetail, DisclosureEvidence)
	}
	tests := []struct {
		level PresentationLevel
		want  DisclosureLevel
	}{
		{PresentationHiddenMember, DisclosureSummary},
		{PresentationFolded, DisclosureSummary},
		{PresentationStructured, DisclosureDetail},
		{PresentationEvidence, DisclosureEvidence},
	}
	for _, tt := range tests {
		current := PresentationDecision{EffectiveLevel: tt.level}
		if got := current.DisclosureLevel(); got != tt.want {
			t.Fatalf("effective level %s maps to disclosure %v, want %v", tt.level, got, tt.want)
		}
	}
}

func containsPresentationReason(reasons []PresentationReason, want PresentationReason) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
