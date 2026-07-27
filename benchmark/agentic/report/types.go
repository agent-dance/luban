// Package report turns content-addressed agentic benchmark artifacts into a
// deterministic, self-contained HTML report. Formal and pilot measurements
// are loaded from harness artifacts; diagnostic canaries are deliberately kept
// in a separate, visibly watermarked input channel.
package report

import (
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

const (
	InputSchemaVersion                 = "agentic-bench/report-input-v1"
	OptimizationSchemaVersion          = "agentic-bench/optimization-ledger-v1"
	ReportStatisticsMethod             = "paired-task-cluster-bootstrap-v1"
	ReportStatisticsResamples          = 10_000
	ReportStatisticsSeed               = int64(20_260_726)
	ReportConfidenceLevel              = 0.95
	BenchmarkContractDeepSWEV11Pilot5  = "deepswe-v1.1-pilot5-development"
	BenchmarkContractDeepSWEV11Full113 = "deepswe-v1.1-full113"
	// ComparableCostBasisDevelopmentNonBilling identifies the development-only
	// same-gateway token estimate. It is deliberately distinct from the formal
	// all-transport basis so a pilot report cannot be mistaken for an invoice.
	ComparableCostBasisDevelopmentNonBilling = "development_non_billing_same_gateway_frozen_token_estimate"
	comparableCostBasisFrozen                = "same_gateway_frozen_rate_card_all_transport"
	comparableCostBasisUnknown               = "unknown_or_lower_bound"
)

type ExperimentClass string

const (
	ClassDiagnosticCanary ExperimentClass = "diagnostic_canary"
	ClassPilot            ExperimentClass = "pilot"
	ClassFormal           ExperimentClass = "formal"
)

// Input is the small report assembly manifest. It contains no copied formal
// scores: formal and pilot results are reloaded and cross-checked from their
// manifest, plan, state, scorecard, evidence, and artifact ledger.
type Input struct {
	SchemaVersion         string                 `json:"schema_version"`
	Report                ReportMeta             `json:"report"`
	Statistics            StatisticsSpec         `json:"statistics"`
	ArtifactSources       []ArtifactSource       `json:"artifact_sources"`
	DiagnosticExperiments []DiagnosticExperiment `json:"diagnostic_experiments"`
	OptimizationLedger    FileReference          `json:"optimization_ledger"`
	PublicReferences      []PublicReference      `json:"public_references"`
	FailureAnnotations    []FailureAnnotation    `json:"failure_annotations"`
	Reproduction          []ReproductionCommand  `json:"reproduction"`
	Limitations           []string               `json:"limitations"`
}

type ReportMeta struct {
	Title               string    `json:"title"`
	Subtitle            string    `json:"subtitle"`
	Benchmark           string    `json:"benchmark"`
	BenchmarkVersion    string    `json:"benchmark_version"`
	BenchmarkContractID string    `json:"benchmark_contract_id"`
	Language            string    `json:"language"`
	BaselineAgentID     string    `json:"baseline_agent_id"`
	ContenderAgentID    string    `json:"contender_agent_id"`
	AsOf                time.Time `json:"as_of"`
}

type StatisticsSpec struct {
	ConfidenceLevel float64 `json:"confidence_level"`
	Method          string  `json:"method"`
	Resamples       int     `json:"resamples"`
	Seed            int64   `json:"seed"`
}

// ArtifactSource points at an artifact root produced by harness.Runner. The
// file names inside the root are intentionally not configurable: manifest.json,
// plan.json, scorecard.json, plus the state/ledger paths frozen by the manifest.
type ArtifactSource struct {
	ID               string          `json:"id"`
	Label            string          `json:"label"`
	Class            ExperimentClass `json:"class"`
	Root             string          `json:"root"`
	LedgerFileSHA256 string          `json:"ledger_file_sha256"`
	Description      string          `json:"description"`
}

type FileReference struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// DiagnosticExperiment is the only accepted source for pre-formal canary
// numbers. Every optional scalar is a pointer so absent telemetry remains
// unknown instead of becoming a misleading zero after JSON decoding.
type DiagnosticExperiment struct {
	ID          string          `json:"id"`
	Label       string          `json:"label"`
	Class       ExperimentClass `json:"class"`
	Description string          `json:"description"`
	SourceNote  string          `json:"source_note"`
	Runs        []DiagnosticRun `json:"runs"`
}

type DiagnosticRun struct {
	TaskID          string             `json:"task_id"`
	AgentID         string             `json:"agent_id"`
	Variant         string             `json:"variant"`
	Provider        string             `json:"provider"`
	Model           string             `json:"model"`
	ReasoningEffort string             `json:"reasoning_effort"`
	Repetition      int                `json:"repetition"`
	Passed          *bool              `json:"passed,omitempty"`
	Metrics         OptionalMetrics    `json:"metrics"`
	Tools           []OptionalToolStat `json:"tools"`
}

type OptionalMetrics struct {
	WallTimeSeconds        *float64              `json:"wall_time_seconds,omitempty"`
	TrialDurationSeconds   *float64              `json:"trial_duration_seconds,omitempty"`
	LLMCallsStarted        *int                  `json:"llm_calls_started,omitempty"`
	ProviderRounds         *int                  `json:"provider_rounds,omitempty"`
	ProviderErrors         *int                  `json:"provider_errors,omitempty"`
	ToolBearingRounds      *int                  `json:"tool_bearing_rounds,omitempty"`
	ToolInvocations        *int                  `json:"tool_invocations,omitempty"`
	PhysicalToolOperations *int                  `json:"physical_tool_operations,omitempty"`
	NativeEvents           *int                  `json:"native_events,omitempty"`
	ToolErrors             *int                  `json:"tool_errors,omitempty"`
	ToolCriticalPathMS     *int64                `json:"tool_critical_path_ms,omitempty"`
	ToolTotalLatencyMS     *int64                `json:"tool_total_latency_ms,omitempty"`
	ToolQueueMS            *int64                `json:"tool_queue_ms,omitempty"`
	InputTokens            *int64                `json:"input_tokens,omitempty"`
	CachedInputTokens      *int64                `json:"cached_input_tokens,omitempty"`
	CacheWriteInputTokens  *int64                `json:"cache_write_input_tokens,omitempty"`
	OutputTokens           *int64                `json:"output_tokens,omitempty"`
	ReasoningOutputTokens  *int64                `json:"reasoning_output_tokens,omitempty"`
	RequestCache           *RequestCacheCoverage `json:"request_cache,omitempty"`
	ComparableCost         *float64              `json:"comparable_cost,omitempty"`
	ComparableCostBasis    string                `json:"comparable_cost_basis,omitempty"`
	ProviderReportedCost   *float64              `json:"provider_reported_cost,omitempty"`
}

type RequestCacheCoverage struct {
	Hits     int `json:"hits"`
	Observed int `json:"observed"`
}

type OptionalToolStat struct {
	Name       string `json:"name"`
	Calls      *int   `json:"calls,omitempty"`
	Errors     *int   `json:"errors,omitempty"`
	DurationMS *int64 `json:"duration_ms,omitempty"`
}

type PublicReference struct {
	ID                    string                                  `json:"id"`
	Benchmark             string                                  `json:"benchmark"`
	Version               string                                  `json:"version"`
	Agent                 string                                  `json:"agent"`
	Model                 string                                  `json:"model"`
	ReasoningEffort       string                                  `json:"reasoning_effort"`
	ComputedArtifact      string                                  `json:"computed_artifact,omitempty"`
	Score                 *float64                                `json:"score,omitempty"`
	Passed                *int                                    `json:"passed,omitempty"`
	Total                 *int                                    `json:"total,omitempty"`
	CostPerTask           *float64                                `json:"cost_per_task,omitempty"`
	MinutesPerTask        *float64                                `json:"minutes_per_task,omitempty"`
	TurnsPerTask          *float64                                `json:"turns_per_task,omitempty"`
	TokensPerTask         *float64                                `json:"tokens_per_task,omitempty"`
	TokenWeightedCacheHit *float64                                `json:"token_weighted_cache_hit,omitempty"`
	Components            []ReferenceComponent                    `json:"components"`
	SourceURL             string                                  `json:"source_url"`
	AccessedAt            time.Time                               `json:"accessed_at"`
	Notes                 string                                  `json:"notes"`
	Computed              *harness.DeepSWEPublicReferenceArtifact `json:"-"`
}

type ReferenceComponent struct {
	Name  string   `json:"name"`
	Score *float64 `json:"score,omitempty"`
}

type FailureCategory string

const (
	FailureImplementation FailureCategory = "implementation"
	FailureIncomplete     FailureCategory = "incomplete"
	FailureRegression     FailureCategory = "regression"
	FailureValidation     FailureCategory = "validation"
	FailureTimeout        FailureCategory = "timeout"
	FailureInfrastructure FailureCategory = "infrastructure"
	FailureProtocol       FailureCategory = "protocol"
	FailureUnknown        FailureCategory = "unknown"
)

type FailureAnnotation struct {
	ExperimentID string          `json:"experiment_id"`
	TaskID       string          `json:"task_id"`
	AgentID      string          `json:"agent_id"`
	Repetition   int             `json:"repetition"`
	Category     FailureCategory `json:"category"`
	Summary      string          `json:"summary"`
	Evidence     []string        `json:"evidence"`
}

// ReproductionCommand stores argv, not a shell program. Rendering applies
// POSIX quoting, preventing presentation-time interpolation or hidden commands.
type ReproductionCommand struct {
	Label string   `json:"label"`
	Argv  []string `json:"argv"`
}

type OptimizationLedger struct {
	SchemaVersion string              `json:"schema_version"`
	Entries       []OptimizationEntry `json:"entries"`
}

type OptimizationEntry struct {
	ID               string              `json:"id"`
	Title            string              `json:"title"`
	Summary          string              `json:"summary"`
	DesignDefect     string              `json:"design_defect"`
	Mechanism        string              `json:"mechanism"`
	Value            string              `json:"value"`
	AttributionScope AttributionScope    `json:"attribution_scope"`
	MeasurementLayer MeasurementLayer    `json:"measurement_layer"`
	EvidenceGrade    EvidenceGrade       `json:"evidence_grade"`
	ExpectedEffect   string              `json:"expected_effect"`
	ObservedEffect   string              `json:"observed_effect"`
	Confounders      []string            `json:"confounders"`
	Risks            []string            `json:"risks"`
	Implementation   []ImplementationRef `json:"implementation"`
	Before           ExperimentEndpoint  `json:"before"`
	After            ExperimentEndpoint  `json:"after"`
	Ablation         Ablation            `json:"ablation"`
	Metrics          []ComparisonMetric  `json:"metrics"`
	EvidenceClass    ExperimentClass     `json:"evidence_class"`
}

type AttributionScope string

const (
	AttributionDiagnosticAssociation AttributionScope = "diagnostic_association"
	AttributionCausalFeatureAblation AttributionScope = "causal_feature_ablation"
	AttributionDesignRationale       AttributionScope = "design_rationale"
)

type MeasurementLayer string

const (
	MeasurementControllerEndToEnd MeasurementLayer = "controller_end_to_end"
	MeasurementTrial              MeasurementLayer = "trial"
	MeasurementAgent              MeasurementLayer = "agent"
	MeasurementProvider           MeasurementLayer = "provider"
	MeasurementTool               MeasurementLayer = "tool"
	MeasurementMixed              MeasurementLayer = "mixed"
)

type EvidenceGrade string

const (
	EvidenceMeasuredAblation EvidenceGrade = "measured_ablation"
	EvidenceDiagnosticBundle EvidenceGrade = "diagnostic_bundle"
	EvidenceNotRun           EvidenceGrade = "not_run"
)

type ImplementationRef struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256,omitempty"`
	Summary string `json:"summary"`
}

type ExperimentEndpoint struct {
	ExperimentID string `json:"experiment_id"`
	AgentID      string `json:"agent_id"`
}

type AblationStatus string

const (
	AblationMeasured AblationStatus = "measured"
	AblationNotRun   AblationStatus = "not_run"
	AblationNA       AblationStatus = "not_applicable"
)

type Ablation struct {
	Status   AblationStatus      `json:"status"`
	Endpoint *ExperimentEndpoint `json:"endpoint,omitempty"`
	Note     string              `json:"note"`
}

type ComparisonMetric string

const (
	MetricPassRate               ComparisonMetric = "pass_rate"
	MetricWallTime               ComparisonMetric = "wall_time_seconds"
	MetricTrialDuration          ComparisonMetric = "trial_duration_seconds"
	MetricLLMCallsStarted        ComparisonMetric = "llm_calls_started"
	MetricProviderRounds         ComparisonMetric = "provider_rounds"
	MetricProviderErrors         ComparisonMetric = "provider_errors"
	MetricToolBearingRounds      ComparisonMetric = "tool_bearing_rounds"
	MetricToolInvocations        ComparisonMetric = "tool_invocations"
	MetricPhysicalToolOperations ComparisonMetric = "physical_tool_operations"
	MetricNativeEvents           ComparisonMetric = "native_events"
	MetricToolErrors             ComparisonMetric = "tool_errors"
	MetricInputTokens            ComparisonMetric = "input_tokens"
	MetricCachedInputTokens      ComparisonMetric = "cached_input_tokens"
	MetricCacheWriteInputTokens  ComparisonMetric = "cache_write_input_tokens"
	MetricUncachedInputTokens    ComparisonMetric = "uncached_input_tokens"
	MetricOutputTokens           ComparisonMetric = "output_tokens"
	MetricReasoningTokens        ComparisonMetric = "reasoning_output_tokens"
	MetricTokenCacheHit          ComparisonMetric = "token_weighted_cache_hit"
	MetricRequestCacheHit        ComparisonMetric = "request_cache_hit"
	MetricComparableCost         ComparisonMetric = "comparable_cost"
	MetricProviderCost           ComparisonMetric = "provider_reported_cost"
)
