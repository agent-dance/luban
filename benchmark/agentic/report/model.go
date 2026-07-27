package report

import (
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

type Data struct {
	Meta              ReportMeta
	Statistics        StatisticsSpec
	Experiments       []ExperimentData
	PublicReferences  []PublicReference
	Optimizations     []OptimizationData
	FailureSummary    []FailureData
	Reproduction      []RenderedCommand
	Limitations       []string
	HasFormal         bool
	HasPilot          bool
	HasDiagnosticOnly bool
	// DevelopmentContract is true only for the preregistered five-task
	// optimization set. It is a hard presentation and verdict boundary: the
	// selected tasks are contaminated by iterative development and can never be
	// presented as a formal public score.
	DevelopmentContract bool
	Verdict             VerdictData
}

type VerdictData struct {
	Status   string
	Criteria []VerdictCriterion
}

type VerdictCriterion struct {
	Metric ComparisonMetric
	Passed *bool
	Detail string
}

type ExperimentData struct {
	ID              string
	Label           string
	Class           ExperimentClass
	Description     string
	SourceNote      string
	Manifest        *ManifestData
	Runs            []RunData
	Agents          []AgentData
	Comparisons     []PairedComparison
	Gates           []GateData
	Hashes          []HashData
	ToolStats       []ToolData
	ProviderRounds  []RoundData
	OrderStrata     []OrderStratumData
	PublicScorecard *harness.DeepSWEPublicScorecard
}

type ManifestData struct {
	ExperimentID                 string
	DatasetName                  string
	DatasetRepository            string
	DatasetCommit                string
	DatasetTreeSHA               string
	EvaluatorName                string
	EvaluatorCommit              string
	EvaluatorVersion             string
	EvaluatorProtocol            string
	ScoringProfile               string
	SelectionMode                string
	ExpectedTasks                int
	Repetitions                  int
	PairingSeed                  uint64
	MaxParallelPairs             int
	TaskNetwork                  string
	VerifierNetwork              string
	NetworkAttestation           string
	EgressProxyImage             string
	EgressProxyImageID           string
	HostEnvAllowlist             []string
	AgentEgressHosts             []string
	CPUs                         int
	MemoryMB                     int
	StorageMB                    int
	HostStorageGuard             harness.HostStorageGuardSpec
	GuestStorageGuard            harness.GuestStorageGuardSpec
	StoragePreflight             harness.StorageAdmissionReceipt
	AgentTimeout                 int
	VerifierTimeout              int
	PricingCurrency              string
	PricingUnit                  int64
	PricingAt                    time.Time
	PricingObservedAt            time.Time
	PricingSource                string
	ProviderOrigin               string
	ProviderSemanticsSHA         string
	ProviderObservationAuthority string
	ProviderTLSRequired          bool
	ProviderWebSocketAllowed     bool
	PricingRates                 []harness.PricingRate
	Agents                       []ManifestAgentData
}

type ManifestAgentData struct {
	ID                         string
	Provider                   string
	Model                      string
	ReasoningEffort            string
	ServiceTier                string
	ServiceTierRequestEncoding string
	BinarySHA256               string
	SourceBaseCommit           string
	SourceTreeOID              string
	SourcePatchSHA             string
	SourceArchiveSHA           string
	BuildReceiptSHA            string
	BuildArgv                  []string
	BuildToolchain             string
	Argv                       []string
	// ArchivedSourceFiles maps canonical source.tar paths to file SHA-256.
	// It is intentionally not rendered wholesale; optimization references use
	// it to prove that a cited implementation file belongs to the executed
	// source archive.
	ArchivedSourceFiles map[string]string
}

type GateStatus string

const (
	GatePass    GateStatus = "pass"
	GateFail    GateStatus = "fail"
	GateUnknown GateStatus = "unknown"
)

type GateData struct {
	Name   string
	Status GateStatus
	Detail string
}

type HashData struct {
	Name   string
	Value  string
	Source string
}

type RunData struct {
	AttemptID             string
	ExperimentID          string
	Class                 ExperimentClass
	PairID                string
	TaskID                string
	AgentID               string
	Variant               string
	Provider              string
	Model                 string
	ReasoningEffort       string
	Repetition            int
	Attempt               int
	ExecutionPosition     string
	Disposition           string
	FailureCategory       string
	Passed                *bool
	ExitClass             string
	ExitCode              *int
	AttemptStartedAt      time.Time
	ControllerStartedAt   time.Time
	ControllerFinishedAt  time.Time
	ControllerRecovered   bool
	ProviderAttemptState  string
	ProviderAttemptCount  uint64
	StartedAt             time.Time
	FinishedAt            time.Time
	TrialStartedAt        time.Time
	TrialFinishedAt       time.Time
	TrialDurationSeconds  *float64
	Metrics               MetricData
	Tools                 []ToolData
	Rounds                []RoundData
	ToolCatalogObserved   int
	NestedToolDefinitions int
	ServiceTierEvidence   harness.ServiceTierCanonicalizationEvidence
	StorageAdmission      harness.StorageAdmissionReceipt
	HostStorageEvidence   harness.StorageResourceEvidence
	GuestStorageEvidence  []harness.GuestStorageResourceEvidence
	CacheInitialState     string
	CacheEvidenceClass    string
	Failure               *FailureData
}

// OrderStratumData exposes the crossover-position sensitivity separately from
// the pooled score. Time and cost are means over observed raw attempts; their
// coverage counts remain explicit so missing telemetry is never coerced to
// zero.
type OrderStratumData struct {
	ExperimentID           string
	AgentID                string
	Position               string
	Raw                    int
	Scored                 int
	Excluded               int
	Passed                 int
	PassRate               *float64
	MeanTrialSeconds       *float64
	TrialObserved          int
	MeanComparableCost     *float64
	ComparableCostObserved int
}

type MetricData struct {
	WallTimeSeconds                  *float64
	TrialDurationSeconds             *float64
	TransportAttempts                *int
	PrewarmAttempts                  *int
	PrewarmErrors                    *int
	LLMCallsStarted                  *int
	CompletedLLMResponses            *int
	RetryAmplification               *float64
	HTTPInferenceRequests            *int
	WebSocketInferenceRequests       *int
	WebSocketConnections             *int
	PrewarmUsageObservations         *int
	PrewarmInputTokens               *int64
	PrewarmCachedInputTokens         *int64
	PrewarmOutputTokens              *int64
	PrewarmUnknownCostAttempts       *int
	CostReceiptObserved              int
	CostReceiptTotal                 int
	UnknownCostAttempts              *int
	CostIdentityUnknownAttempts      *int
	KnownCatalogCostLowerBound       *float64
	AllExecutedInputTokens           *int64
	AllExecutedCachedTokens          *int64
	AllExecutedUncachedTokens        *int64
	AllExecutedNonCachedBaseTokens   *int64
	AllExecutedOutputTokens          *int64
	AllExecutedCacheWriteInputTokens *int64
	AllExecutedCacheHit              *float64
	AllExecutedUsageObserved         int
	AllExecutedUsageTotal            int
	AllExecutedCacheWriteObserved    int
	AllExecutedCacheWriteTotal       int
	AllExecutedUnreportedCacheWrite  int
	ProviderRequests                 *int
	ProviderRounds                   *int
	ProviderErrors                   *int
	ToolBearingRounds                *int
	ToolInvocations                  *int
	ToolTraceMatched                 *int
	ToolTraceUnmatched               *int
	PhysicalToolOperations           *int
	PhysicalToolObserved             int
	PhysicalToolTotal                int
	NativeEvents                     *int
	ToolErrors                       *int
	ToolCriticalPathMS               *int64
	ToolCriticalObserved             int
	ToolCriticalTotal                int
	ToolTotalLatencyMS               *int64
	ToolTotalObserved                int
	ToolTotalTotal                   int
	ToolQueueMS                      *int64
	ToolQueueObserved                int
	ToolQueueTotal                   int
	InputTokens                      *int64
	CachedInputTokens                *int64
	CacheWriteInputTokens            *int64
	CacheMissTokens                  *int64
	OutputTokens                     *int64
	ReasoningOutputTokens            *int64
	ReasoningTokenObserved           int
	ReasoningTokenTotal              int
	CacheWriteTokenObserved          int
	CacheWriteTokenTotal             int
	UnreportedCacheWriteRounds       int
	KnownCacheWriteSurcharge         *float64
	TokenWeightedCacheHit            *float64
	RequestCacheHit                  *float64
	RequestCacheHits                 *int
	RequestCacheObserved             *int
	CachePolicyObservedRequests      *int
	CacheKeyPresentRequests          *int
	CacheUniqueKeyCount              *int
	CacheKeyTransitions              *int
	CacheLineageStable               *bool
	CatalogCost                      *float64
	CatalogCostPartial               *float64
	ComparableCost                   *float64
	ComparableCostBasis              string
	ProviderReportedCost             *float64
	ProviderCostPartial              *float64
	ProviderCostObserved             int
	ProviderCostTotal                int
	TokenUsageObserved               int
	TokenUsageTotal                  int
	ToolErrorObserved                int
	ToolErrorTotal                   int
	ToolTimingObserved               int
	ToolTimingTotal                  int
}

type ToolData struct {
	ExperimentID string
	AgentID      string
	Name         string
	Calls        *int
	Errors       *int
	DurationMS   *int64
	ErrorKnown   int
	ErrorTotal   int
	TimingKnown  int
	TimingTotal  int
}

type RoundData struct {
	ExperimentID                         string
	AgentID                              string
	TaskID                               string
	Repetition                           int
	Round                                int
	Outcome                              string
	ErrorCode                            string
	Transport                            string
	ProviderAttemptKind                  string
	TransportDisposition                 string
	RequestIDHash                        string
	ResponseIDHash                       string
	RequestedReasoningMode               string
	ReasoningModeCanonical               string
	RequestedTextVerbosity               string
	MaxOutputTokensSpecified             bool
	MaxOutputTokens                      *int64
	RequestedServiceTier                 string
	RequestedServiceTierPresent          bool
	RequestedServiceTierCanonical        string
	RequestedServiceTierRepresentation   string
	ClientCanonicalizationProofSHA256    string
	ClientAgentID                        string
	OriginalRequestBodySHA256            string
	ForwardedRequestBodySHA256           string
	OriginalRequestCanonicalSHA256       string
	ForwardedRequestCanonicalSHA256      string
	OriginalWithoutTierSHA256            string
	ForwardedWithoutTierSHA256           string
	OriginalServiceTierPresent           bool
	OriginalServiceTier                  string
	ForwardedServiceTierPresent          bool
	ForwardedServiceTier                 string
	ForwardedRequestBytes                int64
	ServiceTierTransformation            string
	ServiceTierTransformationExactDiff   bool
	ServiceTierTransformationProofSHA256 string
	ResponseServiceTier                  string
	ResponseServiceTierCanonical         string
	ServiceTierComparable                bool
	ResponseCreatedModel                 string
	ResponseModel                        string
	StartedAt                            time.Time
	FinishedAt                           time.Time
	HeadersMS                            *float64
	FirstByteMS                          *float64
	StreamMS                             *float64
	ProviderMS                           float64
	PostRoundGapMS                       *float64
	ToolCalls                            int
	PhysicalTools                        *int
	ToolCriticalMS                       *int64
	ToolTotalMS                          *int64
	ToolQueueMS                          *int64
	InputTokens                          *int64
	CachedInputTokens                    *int64
	CacheWriteTokens                     *int64
	OutputTokens                         *int64
	CacheHit                             *bool
	PromptCacheKeyPresent                bool
	PromptCacheKeyHash                   string
	CachePolicyObserved                  bool
	PromptCacheOptionsPresent            bool
	PromptCacheOptionsMode               string
	PromptCacheTTLSeconds                *int64
	PromptCacheRetentionPresent          bool
	PromptCacheRetention                 string
	CacheBreakpointCount                 int
	CacheBreakpointPositionHashes        []string
}

type AgentData struct {
	ExperimentID string
	AgentID      string
	Variant      string
	Runs         int
	Tasks        int
	Passed       *int
	PassRate     *float64
	PassCI       *ConfidenceInterval
	Metrics      MetricData
	Tools        []ToolData
}

type ConfidenceInterval struct {
	Estimate        float64
	Lower           float64
	Upper           float64
	ConfidenceLevel float64
	Method          string
	Tasks           int
	Pairs           int
	Resamples       int
	Seed            int64
}

type PairedComparison struct {
	ExperimentID  string
	Baseline      string
	Contender     string
	Tasks         int
	Pairs         int
	Metrics       []MetricComparison
	QualityWins   int
	QualityLosses int
	QualityTies   int
}

type MetricComparison struct {
	Metric         ComparisonMetric
	Baseline       *float64
	Contender      *float64
	Difference     *float64
	RelativeChange *float64
	CI             *ConfidenceInterval
	Pairs          int
	Tasks          int
	Note           string
}

type OptimizationData struct {
	Entry           OptimizationEntry
	Before          *AgentData
	After           *AgentData
	Ablation        *AgentData
	Metrics         []MetricComparison
	AblationMetrics []MetricComparison
	ClassValid      bool
}

type FailureData struct {
	ExperimentID string
	TaskID       string
	AgentID      string
	Repetition   int
	Category     FailureCategory
	Summary      string
	Evidence     []string
}

type RenderedCommand struct {
	Label   string
	Command string
}

type formalBundle struct {
	Root      string
	Manifest  harness.LoadedManifest
	Plan      harness.RunPlan
	State     harness.ExperimentState
	Scorecard harness.Scorecard
	Ledger    harness.ArtifactLedger
}
