package localbench

import "time"

const (
	ResultSchemaVersion = "agentic-local-benchmark/v1"
	DatasetName         = "SWE-bench-Live/MultiLang"
	DatasetRevision     = "608f7ae9ab8ea1f9f0d030fe04562cf6bd1a0c8b"
	ModelID             = "gpt-5.6-sol"
	ReasoningEffort     = "xhigh"
)

var representativeOrder = []string{
	"danielmiessler__Fabric-2098",
	"openai__openai-agents-js-375",
	"kubernetes__kube-state-metrics-2926",
	"skim-rs__skim-1044",
	"include-what-you-use__include-what-you-use-1991",
}

type catalogTask struct {
	InstanceID string `json:"instance_id"`
	Language   string `json:"language"`
}

type TaskSelection struct {
	InstanceID string `json:"instance_id"`
	Language   string `json:"language"`
}

type Pricing struct {
	InputPerMillionUSD       float64 `json:"input_per_million_usd"`
	CachedInputPerMillionUSD float64 `json:"cached_input_per_million_usd"`
	CacheWritePerMillionUSD  float64 `json:"cache_write_per_million_usd"`
	OutputPerMillionUSD      float64 `json:"output_per_million_usd"`
}

func FrozenPricing() Pricing {
	return Pricing{InputPerMillionUSD: 5, CachedInputPerMillionUSD: .5, CacheWritePerMillionUSD: 6.25, OutputPerMillionUSD: 30}
}

type BinaryIdentity struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type Usage struct {
	InputTokens              int64  `json:"input_tokens"`
	CachedInputTokens        int64  `json:"cached_input_tokens"`
	CacheCreationInputTokens int64  `json:"cache_creation_input_tokens"`
	OutputTokens             int64  `json:"output_tokens"`
	ReasoningOutputTokens    *int64 `json:"reasoning_output_tokens,omitempty"`
}

type PatchStats struct {
	FilesChanged int      `json:"files_changed"`
	Files        []string `json:"files"`
	Additions    int      `json:"additions"`
	Deletions    int      `json:"deletions"`
}

type RunSummary struct {
	InstanceID             string         `json:"instance_id"`
	Language               string         `json:"language"`
	Agent                  string         `json:"agent"`
	Model                  string         `json:"model"`
	ReasoningEffort        string         `json:"reasoning_effort"`
	StartedAt              time.Time      `json:"started_at"`
	ElapsedSeconds         float64        `json:"elapsed_seconds"`
	TimeoutSeconds         int            `json:"timeout_seconds"`
	TimedOut               bool           `json:"timed_out"`
	ExitCode               int            `json:"exit_code"`
	Usage                  Usage          `json:"usage"`
	EstimatedCostUSD       float64        `json:"estimated_cost_usd"`
	ToolEvents             int            `json:"tool_events"`
	ToolEventsByType       map[string]int `json:"tool_events_by_type"`
	Patch                  PatchStats     `json:"patch"`
	LLMCalls               int            `json:"llm_calls"`
	LLMSuccessfulCalls     int            `json:"llm_successful_calls"`
	LLMFailedCalls         int            `json:"llm_failed_calls"`
	ProviderRequestSeconds float64        `json:"provider_request_seconds"`
	Binary                 BinaryIdentity `json:"binary"`
	EvidenceRoot           string         `json:"evidence_root"`
}

type TestPartition struct {
	Expected     int      `json:"expected"`
	Passed       []string `json:"passed,omitempty"`
	PassedCount  int      `json:"passed_count,omitempty"`
	Failed       []string `json:"failed"`
	Missing      []string `json:"missing,omitempty"`
	MissingCount int      `json:"missing_count,omitempty"`
}

type Evaluation struct {
	InstanceID     string        `json:"instance_id"`
	Language       string        `json:"language"`
	Agent          string        `json:"agent"`
	Resolved       bool          `json:"resolved"`
	ElapsedSeconds float64       `json:"elapsed_seconds"`
	FailToPass     TestPartition `json:"FAIL_TO_PASS"`
	PassToPass     TestPartition `json:"PASS_TO_PASS"`
	EvidenceRoot   string        `json:"evidence_root"`
}

type Failure struct {
	Stage        string `json:"stage"`
	InstanceID   string `json:"instance_id,omitempty"`
	Agent        string `json:"agent,omitempty"`
	Code         string `json:"code"`
	EvidencePath string `json:"evidence_path,omitempty"`
}

type Aggregate struct {
	Agent                  string         `json:"agent"`
	TasksSelected          int            `json:"tasks_selected"`
	RunsObserved           int            `json:"runs_observed"`
	EvaluationsObserved    int            `json:"evaluations_observed"`
	Resolved               int            `json:"resolved"`
	TaskDurationSeconds    float64        `json:"task_duration_seconds"`
	ProviderRequestSeconds float64        `json:"provider_request_seconds"`
	LLMCalls               int            `json:"llm_calls"`
	LLMSuccessfulCalls     int            `json:"llm_successful_calls"`
	LLMFailedCalls         int            `json:"llm_failed_calls"`
	InputTokens            int64          `json:"input_tokens"`
	CachedInputTokens      int64          `json:"cached_input_tokens"`
	CacheWriteInputTokens  int64          `json:"cache_write_input_tokens"`
	OutputTokens           int64          `json:"output_tokens"`
	CacheHitRatio          float64        `json:"cache_hit_ratio"`
	EstimatedCostUSD       float64        `json:"estimated_cost_usd"`
	ToolEvents             int            `json:"tool_events"`
	ToolEventsByType       map[string]int `json:"tool_events_by_type"`
}

type BenchmarkResult struct {
	SchemaVersion    string           `json:"schema_version"`
	Status           string           `json:"status"`
	RunID            string           `json:"run_id"`
	StartedAt        time.Time        `json:"started_at"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`
	Dataset          string           `json:"dataset"`
	DatasetRevision  string           `json:"dataset_revision"`
	SelectionPolicy  string           `json:"selection_policy"`
	Tasks            []TaskSelection  `json:"tasks"`
	Model            string           `json:"model"`
	ReasoningEffort  string           `json:"reasoning_effort"`
	GatewayOrigin    string           `json:"gateway_origin"`
	EvaluatorEngine  string           `json:"evaluator_engine"`
	AgentTimeout     int              `json:"agent_timeout_seconds"`
	EvaluatorTimeout int              `json:"evaluator_timeout_seconds"`
	Pricing          Pricing          `json:"pricing"`
	Binaries         []BinaryIdentity `json:"binaries"`
	Runs             []RunSummary     `json:"runs"`
	Evaluations      []Evaluation     `json:"evaluations"`
	GoldEvaluations  []Evaluation     `json:"gold_evaluations"`
	Aggregates       []Aggregate      `json:"aggregates"`
	SharedPass       []TaskSelection  `json:"shared_pass_tasks"`
	Failures         []Failure        `json:"failures"`
}
