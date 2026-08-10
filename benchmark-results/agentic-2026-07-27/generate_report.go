package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/i18n"
)

const (
	localTaskCount  = 5
	localModel      = "gpt-5.6-sol"
	localEffort     = "xhigh"
	localTimeout    = 1800
	inputRate       = 5.0
	cachedInputRate = 0.5
	cacheWriteRate  = 6.25
	outputRate      = 30.0
)

var localAgents = []string{"codex", "luban"}

var expectedBinarySHA256 = map[string]string{
	"codex": "134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477",
	"luban": "67659bdec7cb978cb59b0be10657c3239e71daae939175920b36b6b7cd114fe9",
}

var localTasks = []string{
	"danielmiessler__Fabric-2098",
	"openai__openai-agents-js-375",
	"kubernetes__kube-state-metrics-2926",
	"skim-rs__skim-1044",
	"include-what-you-use__include-what-you-use-1991",
}

//go:embed local5-report.html.tmpl
var reportFiles embed.FS

type options struct {
	rootPath     string
	baselinePath string
	repoRootPath string
	outputPath   string
	allowPartial bool
}

type usageData struct {
	InputTokens              int64  `json:"input_tokens"`
	CachedInputTokens        int64  `json:"cached_input_tokens"`
	CacheCreationInputTokens int64  `json:"cache_creation_input_tokens"`
	OutputTokens             int64  `json:"output_tokens"`
	ReasoningOutputTokens    *int64 `json:"reasoning_output_tokens"`
}

type patchData struct {
	FilesChanged int      `json:"files_changed"`
	Files        []string `json:"files"`
	Additions    int      `json:"additions"`
	Deletions    int      `json:"deletions"`
}

type runSummary struct {
	InstanceID            string         `json:"instance_id"`
	Language              string         `json:"language"`
	Agent                 string         `json:"agent"`
	Model                 string         `json:"model"`
	ReasoningEffort       string         `json:"reasoning_effort"`
	StartedAtUnix         float64        `json:"started_at_unix"`
	ElapsedSeconds        float64        `json:"elapsed_seconds"`
	TimeoutSeconds        int            `json:"timeout_seconds"`
	TimedOut              bool           `json:"timed_out"`
	ExitCode              int            `json:"exit_code"`
	Usage                 usageData      `json:"usage"`
	EstimatedCostUSD      float64        `json:"estimated_cost_usd"`
	ToolCalls             int            `json:"tool_calls"`
	ToolCallsByType       map[string]int `json:"tool_calls_by_type"`
	Patch                 patchData      `json:"patch"`
	LLMCalls              *int           `json:"llm_calls"`
	LLMSuccessfulCalls    *int           `json:"llm_successful_calls"`
	LLMFailedCalls        *int           `json:"llm_failed_calls"`
	ProviderRequestSecond *float64       `json:"provider_request_seconds"`
	Binary                binaryData     `json:"binary"`
	presence              runSummaryPresence
}

type runSummaryPresence struct {
	TimeoutSeconds      bool
	TimedOut            bool
	ExitCode            bool
	ToolCalls           bool
	InputTokens         bool
	CachedInputTokens   bool
	CacheCreationTokens bool
	OutputTokens        bool
	PatchFilesChanged   bool
	PatchFiles          bool
	PatchAdditions      bool
	PatchDeletions      bool
}

func (summary *runSummary) UnmarshalJSON(data []byte) error {
	type plainRunSummary runSummary
	var value plainRunSummary
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var required struct {
		TimeoutSeconds *int  `json:"timeout_seconds"`
		TimedOut       *bool `json:"timed_out"`
		ExitCode       *int  `json:"exit_code"`
		ToolCalls      *int  `json:"tool_calls"`
		Usage          struct {
			InputTokens         *int64 `json:"input_tokens"`
			CachedInputTokens   *int64 `json:"cached_input_tokens"`
			CacheCreationTokens *int64 `json:"cache_creation_input_tokens"`
			OutputTokens        *int64 `json:"output_tokens"`
		} `json:"usage"`
		Patch struct {
			FilesChanged *int      `json:"files_changed"`
			Files        *[]string `json:"files"`
			Additions    *int      `json:"additions"`
			Deletions    *int      `json:"deletions"`
		} `json:"patch"`
	}
	if err := json.Unmarshal(data, &required); err != nil {
		return err
	}
	*summary = runSummary(value)
	summary.presence = runSummaryPresence{
		TimeoutSeconds: required.TimeoutSeconds != nil, TimedOut: required.TimedOut != nil,
		ExitCode: required.ExitCode != nil, ToolCalls: required.ToolCalls != nil,
		InputTokens: required.Usage.InputTokens != nil, CachedInputTokens: required.Usage.CachedInputTokens != nil,
		CacheCreationTokens: required.Usage.CacheCreationTokens != nil, OutputTokens: required.Usage.OutputTokens != nil,
		PatchFilesChanged: required.Patch.FilesChanged != nil, PatchFiles: required.Patch.Files != nil,
		PatchAdditions: required.Patch.Additions != nil, PatchDeletions: required.Patch.Deletions != nil,
	}
	return nil
}

type binaryData struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type providerRequestRecord struct {
	Sequence        *int     `json:"sequence"`
	Method          string   `json:"method"`
	Endpoint        string   `json:"endpoint"`
	Status          *int     `json:"status"`
	ElapsedSeconds  *float64 `json:"elapsed_seconds"`
	RequestIDSHA256 string   `json:"request_id_sha256"`
}

type trajectoryEvent struct {
	Type      string `json:"type"`
	Metric    string `json:"metric"`
	Name      string `json:"name"`
	ToolUseID string `json:"tool_use_id"`
	TurnID    string `json:"turn_id"`
	Outcome   string `json:"outcome"`
	Metrics   struct {
		RevisionSealDisposition string `json:"revision_seal_disposition"`
	} `json:"metrics"`
	ToolRound *struct {
		LogicalModelVisibleCalls int     `json:"logical_model_visible_calls"`
		PhysicalChildOperations  int     `json:"physical_child_operations"`
		Fanout                   int     `json:"fanout"`
		ErrorCount               int     `json:"error_count"`
		CriticalPathMS           float64 `json:"critical_path_ms"`
	} `json:"tool_round"`
	RequestStatus *struct {
		Phase      string `json:"phase"`
		Attempt    int    `json:"attempt"`
		RetryCount int    `json:"retry_count"`
		Failed     bool   `json:"failed"`
	} `json:"request_status"`
}

type evaluationData struct {
	InstanceID                 string         `json:"instance_id"`
	Agent                      string         `json:"agent"`
	Language                   string         `json:"language"`
	Resolved                   *bool          `json:"resolved"`
	ElapsedSeconds             float64        `json:"elapsed_seconds"`
	ImagePullExitCode          *int           `json:"image_pull_exit_code"`
	TestPatchApplyExitCode     *int           `json:"test_patch_apply_exit_code"`
	SolutionPatchApplyExitCode *int           `json:"solution_patch_apply_exit_code"`
	RebuildExitCode            *int           `json:"rebuild_exit_code"`
	TestExitCode               *int           `json:"test_exit_code"`
	PrintExitCode              *int           `json:"print_exit_code"`
	DiagnosticExcludedPaths    []string       `json:"diagnostic_excluded_paths"`
	FailToPass                 failToPassData `json:"FAIL_TO_PASS"`
	PassToPass                 passToPassData `json:"PASS_TO_PASS"`
}

type evaluationAdjudication struct {
	SchemaVersion         string   `json:"schema_version"`
	InstanceID            string   `json:"instance_id"`
	Agent                 string   `json:"agent"`
	OriginalReport        string   `json:"original_report"`
	EffectiveReport       string   `json:"effective_report"`
	ReasonCode            string   `json:"reason_code"`
	ExcludedCandidatePath []string `json:"excluded_candidate_paths"`
	EffectiveResolved     bool     `json:"effective_resolved"`
}

type failToPassData struct {
	Expected *int     `json:"expected"`
	Passed   []string `json:"passed"`
	Failed   []string `json:"failed"`
	Missing  []string `json:"missing"`
}

type passToPassData struct {
	Expected     *int     `json:"expected"`
	PassedCount  *int     `json:"passed_count"`
	Failed       []string `json:"failed"`
	MissingCount *int     `json:"missing_count"`
}

type pricingData struct {
	InputPerMillionUSD       float64 `json:"input_per_million_usd"`
	CachedInputPerMillionUSD float64 `json:"cached_input_per_million_usd"`
	OutputPerMillionUSD      float64 `json:"output_per_million_usd"`
}

type experimentData struct {
	Dataset           string            `json:"dataset"`
	DatasetRevision   string            `json:"dataset_revision"`
	ParquetSHA256     map[string]string `json:"parquet_sha256"`
	Model             string            `json:"model"`
	ReasoningEffort   string            `json:"reasoning_effort"`
	PricingAssumption pricingData       `json:"pricing_assumption"`
	TimeoutSeconds    int               `json:"timeout_seconds"`
}

type legacyAggregate struct {
	Resolved              int            `json:"resolved"`
	TotalTasks            int            `json:"total_tasks"`
	ElapsedSeconds        float64        `json:"elapsed_seconds"`
	EstimatedCostUSD      float64        `json:"estimated_cost_usd"`
	ToolCalls             int            `json:"tool_calls"`
	ToolCallsByType       map[string]int `json:"tool_calls_by_type"`
	InputTokens           int64          `json:"input_tokens"`
	CachedInputTokens     int64          `json:"cached_input_tokens"`
	UncachedInputTokens   int64          `json:"uncached_input_tokens"`
	CacheRatio            float64        `json:"cache_ratio"`
	OutputTokens          int64          `json:"output_tokens"`
	ReasoningOutputTokens *int64         `json:"reasoning_output_tokens"`
}

type legacyResults struct {
	ReportSchema     string                     `json:"report_schema"`
	TaskOrder        []string                   `json:"task_order"`
	Experiment       experimentData             `json:"experiment"`
	Aggregates       map[string]legacyAggregate `json:"aggregates"`
	SecurityIncident struct {
		Observed    string `json:"observed"`
		Impact      string `json:"impact"`
		Remediation string `json:"remediation"`
	} `json:"security_incident"`
}

type artifactLink struct {
	Label string
	Href  string
}

type namedCount struct {
	Name  string
	Count int
}

type taskRunData struct {
	Agent                  string
	RunAvailable           bool
	EvaluationAvailable    bool
	Adjudicated            bool
	Resolved               bool
	ElapsedSeconds         float64
	ProviderRequestSeconds float64
	LLMCalls               int
	LLMSuccessful          int
	LLMFailed              int
	InputTokens            int64
	CachedTokens           int64
	CacheWriteTokens       int64
	OutputTokens           int64
	CacheRatio             float64
	EstimatedCost          float64
	RunnerEstimatedCost    float64
	ToolCalls              int
	ToolTypes              []namedCount
	FilesChanged           int
	TimedOut               bool
	ExitCode               int
	BinaryPath             string
	BinarySHA256           string
	RawArtifactLinks       []artifactLink
}

type taskData struct {
	ID            string
	Language      string
	Runs          []taskRunData
	GoldAvailable bool
	GoldLink      artifactLink
}

type agentData struct {
	ID                     string
	Resolved               int
	Tasks                  int
	RunsObserved           int
	EvaluationsObserved    int
	PairsObserved          int
	Score                  float64
	ElapsedSeconds         float64
	ProviderRequestSeconds float64
	LLMCalls               int
	LLMSuccessful          int
	LLMFailed              int
	InputTokens            int64
	CachedTokens           int64
	CacheWriteTokens       int64
	UncachedTokens         int64
	OutputTokens           int64
	CacheRatio             float64
	EstimatedCost          float64
	RunnerEstimatedCost    float64
	ToolCalls              int
	ToolTypes              []namedCount
	BinaryPath             string
	BinarySHA256           string
	RawResolved            int
	RawEvaluationsObserved int
	AdjudicatedResolved    int
}

type headlineData struct {
	Available       bool
	LubanResolved   int
	CodexResolved   int
	Tasks           int
	LubanCalls      int
	CodexCalls      int
	CallsDelta      string
	WallDelta       string
	CostDelta       string
	CachePointDelta string
	LubanCached     int64
	CodexCached     int64
	Adjudicated     bool
}

type efficiencyDiagnosis struct {
	Available                  bool
	OldOuterToolEvents         int
	OldToolRounds              int
	OldUsageRounds             int
	NewLogicalToolEvents       int
	NewToolRounds              int
	NewPhysicalChildOperations int
	NewProviderPOSTs           int
	LogicalToolDelta           string
	PhysicalOperationDelta     string
	ToolRoundDelta             string
	ProviderRoundDelta         string
	Succeeded                  int
	Partial                    int
	Failed                     int
	TimedOut                   int
	Cancelled                  int
	InspectCalls               int
	InspectPartial             int
	ProviderSeconds            float64
	WallSeconds                float64
	ProviderShare              float64
	TransportRetries           int
	ToolRoundErrors            int
	RunRevisionBound           int
	RunCommittedUnverified     int
	RunWithoutRevisionSeal     int
	ToolCriticalPathSeconds    float64
	CompletionTailCalls        int
	CompletionTailSeconds      float64
	CompletionTailCost         float64
	CompletionRejections       int
}

type optimizationEvidenceRow struct {
	CopyKey  string
	Before   string
	After    string
	Change   string
	ScopeKey string
	Sources  []artifactLink
}

type sharedPassMetrics struct {
	LLMCalls               int
	ElapsedSeconds         float64
	ProviderRequestSeconds float64
	InputTokens            int64
	CachedTokens           int64
	CacheWriteTokens       int64
	OutputTokens           int64
	CacheRatio             float64
	EstimatedCost          float64
	ToolCalls              int
	ToolTypes              []namedCount
}

type sharedPassMetricRow struct {
	Metric string
	Codex  string
	Luban  string
	Ratio  string
	Delta  string
}

type sharedPassTaskData struct {
	ID         string
	Language   string
	Codex      sharedPassMetrics
	Luban      sharedPassMetrics
	Rows       []sharedPassMetricRow
	CodexLinks []artifactLink
	LubanLinks []artifactLink
}

type sharedPassData struct {
	Available            bool
	TaskCount            int
	Codex                sharedPassMetrics
	Luban                sharedPassMetrics
	Rows                 []sharedPassMetricRow
	Tasks                []sharedPassTaskData
	LubanLongerSlower    bool
	CodexInputPerCall    string
	LubanInputPerCall    string
	InputPerCallDelta    string
	CodexProviderPerCall string
	LubanProviderPerCall string
	ProviderPerCallDelta string
	CodexOutputPerCall   string
	LubanOutputPerCall   string
	OutputPerCallDelta   string
}

type changeRow struct {
	MetricKey string
	Before    string
	After     string
	Change    string
	Tone      string
}

type evidenceCard struct {
	CopyKey  string
	Evidence string
}

type optimizationCard struct {
	CopyKey string
	Sources []artifactLink
}

type reportData struct {
	HTMLLanguage             string
	Model                    string
	ReasoningEffort          string
	EvidenceCutoff           time.Time
	TaskCount                int
	AgentSlotCount           int
	Complete                 bool
	Dataset                  string
	DatasetRevision          string
	TimeoutSeconds           int
	GoldObserved             int
	RunSlotsObserved         int
	EvaluationSlotsObserved  int
	PairSlotsObserved        int
	Agents                   []agentData
	Tasks                    []taskData
	LubanChanges             []changeRow
	HistoricalToolRatio      float64
	HistoricalCards          []evidenceCard
	Optimizations            []optimizationCard
	BaselineHref             string
	MetadataHref             string
	CurrentRootHref          string
	Headline                 headlineData
	Efficiency               efficiencyDiagnosis
	OptimizationEvidence     []optimizationEvidenceRow
	BaselineIncidentObserved bool
	SharedPass               sharedPassData
	AdjudicationsObserved    int
}

func main() {
	commandIO := cli.ProcessCommandIO()
	os.Exit(runMain(os.Args[1:], commandIO.Stdout, commandIO.Stderr))
}

func runMain(arguments []string, stdout, stderr io.Writer) int {
	language := i18n.DetectOrLoadLanguage()
	parsed, help, err := parseOptions(language, arguments)
	if help {
		writeUsage(stdout, language)
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyAgenticReportCLIError, err))
		return 2
	}
	if err := generateReport(parsed, language); err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyAgenticReportCLIError, err))
		return 1
	}
	_, _ = fmt.Fprintln(stdout, i18n.Format(language, i18n.KeyAgenticReportCLISuccess, parsed.outputPath))
	return 0
}

func parseOptions(language i18n.Language, arguments []string) (options, bool, error) {
	var result options
	set := flag.NewFlagSet("local5-report", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&result.rootPath, "root", "", i18n.Text(language, i18n.KeyAgenticReportCLIFlagInput))
	set.StringVar(&result.baselinePath, "baseline", "", i18n.Text(language, i18n.KeyAgenticReportCLIFlagInput))
	set.StringVar(&result.repoRootPath, "repo-root", ".", i18n.Text(language, i18n.KeyAgenticReportCLIFlagInput))
	set.StringVar(&result.outputPath, "output", "", i18n.Text(language, i18n.KeyAgenticReportCLIFlagOutput))
	set.BoolVar(&result.allowPartial, "allow-partial", false, i18n.Text(language, i18n.KeyAgenticLocal5CaveatIncompleteRefusal))
	if err := set.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, true, nil
		}
		return options{}, false, err
	}
	if set.NArg() != 0 || result.rootPath == "" || result.baselinePath == "" || result.outputPath == "" {
		return options{}, false, i18n.NewError(i18n.KeyAgenticReportCLIRequired)
	}
	for _, path := range []*string{&result.rootPath, &result.baselinePath, &result.repoRootPath, &result.outputPath} {
		absolute, err := filepath.Abs(*path)
		if err != nil {
			return options{}, false, err
		}
		*path = absolute
	}
	return result, false, nil
}

func writeUsage(writer io.Writer, language i18n.Language) {
	_, _ = fmt.Fprintf(writer, "--root PATH\n--baseline PATH\n--repo-root PATH\n--output PATH\t%s\n--allow-partial\t%s\n",
		i18n.Text(language, i18n.KeyAgenticReportCLIFlagOutput),
		i18n.Text(language, i18n.KeyAgenticLocal5CaveatIncompleteRefusal))
}

func generateReport(options options, language i18n.Language) error {
	data, err := compileReport(options, language)
	if err != nil {
		return err
	}
	if !data.Complete && !options.allowPartial {
		return i18n.NewError(i18n.KeyAgenticLocal5CaveatIncompleteRefusal)
	}
	parent := filepath.Dir(options.outputPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".local5-report-*.html")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return err
	}
	if err := renderReport(temporary, data, language); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, options.outputPath); err != nil {
		return err
	}
	committed = true
	return nil
}

func compileReport(options options, language i18n.Language) (reportData, error) {
	var baseline legacyResults
	if err := decodeJSONFile(options.baselinePath, &baseline); err != nil {
		return reportData{}, err
	}
	metadataPath := filepath.Join(options.rootPath, "raw", "metadata", "experiment.json")
	var currentExperiment experimentData
	if err := decodeJSONFile(metadataPath, &currentExperiment); err != nil {
		return reportData{}, i18n.NewError(i18n.KeyAgenticLocal5CaveatIncompleteRefusal)
	}
	if baseline.ReportSchema != "luban-agent-comparison/v1" || !equalStrings(baseline.TaskOrder, localTasks) ||
		!validExperiment(baseline.Experiment) || !equalExperiment(baseline.Experiment, currentExperiment) {
		return reportData{}, i18n.NewError(i18n.KeyAgenticLocal5CaveatIncompleteRefusal)
	}
	for _, agentID := range localAgents {
		legacy, exists := baseline.Aggregates[agentID]
		if !exists || !validLegacyAggregate(legacy) {
			return reportData{}, i18n.NewError(i18n.KeyAgenticLocal5CaveatIncompleteRefusal)
		}
	}

	data := reportData{
		HTMLLanguage: languageCode(language), Model: localModel, ReasoningEffort: localEffort,
		TaskCount: localTaskCount, AgentSlotCount: localTaskCount * len(localAgents),
		Dataset: currentExperiment.Dataset, DatasetRevision: currentExperiment.DatasetRevision, TimeoutSeconds: currentExperiment.TimeoutSeconds,
		BaselineHref:    relativeHref(filepath.Dir(options.outputPath), options.baselinePath),
		MetadataHref:    relativeHref(filepath.Dir(options.outputPath), metadataPath),
		CurrentRootHref: relativeHref(filepath.Dir(options.outputPath), filepath.Join(options.rootPath, "raw")),
	}
	aggregates := map[string]*agentData{}
	toolTotals := map[string]map[string]int{}
	for _, agentID := range localAgents {
		aggregate := &agentData{ID: agentID, Tasks: localTaskCount}
		aggregates[agentID] = aggregate
		toolTotals[agentID] = map[string]int{}
	}

	latestEvidence := evidenceUnix(options.baselinePath)
	for _, taskID := range baseline.TaskOrder {
		goldPath := filepath.Join(options.rootPath, "raw", "evaluation", taskID, "gold", "report.json")
		var gold evaluationData
		goldAvailable := decodeJSONFile(goldPath, &gold) == nil && validateGoldEvaluation(taskID, gold) == nil
		if goldAvailable {
			data.GoldObserved++
		}
		latestEvidence = math.Max(latestEvidence, evidenceUnix(goldPath))
		task := taskData{
			ID:            taskID,
			GoldAvailable: goldAvailable,
			GoldLink:      artifactLink{Label: "gold/report.json", Href: relativeHref(filepath.Dir(options.outputPath), goldPath)},
		}
		for _, agentID := range localAgents {
			summaryPath := filepath.Join(options.rootPath, "raw", "runs", taskID, agentID, "summary.json")
			originalEvaluationPath := filepath.Join(options.rootPath, "raw", "evaluation", taskID, agentID, "report.json")
			evaluationPath, adjudicationPath, adjudicated, err := resolveEvaluationPath(originalEvaluationPath, taskID, agentID)
			if err != nil {
				return reportData{}, err
			}
			var summary runSummary
			var evaluation evaluationData
			run := taskRunData{
				Agent:            agentID,
				Adjudicated:      adjudicated,
				RawArtifactLinks: rawLinks(options, taskID, agentID, summaryPath, originalEvaluationPath, evaluationPath, adjudicationPath),
			}
			aggregate := aggregates[agentID]
			providerRequestsPath := filepath.Join(filepath.Dir(summaryPath), "provider-requests.jsonl")
			if decodeJSONFile(summaryPath, &summary) == nil && validateCompletedSummary(taskID, agentID, summary, baseline.Experiment.PricingAssumption) == nil &&
				validateProviderRequestLog(providerRequestsPath, summary) == nil {
				run.RunAvailable = true
				run.ElapsedSeconds = summary.ElapsedSeconds
				run.ProviderRequestSeconds = *summary.ProviderRequestSecond
				run.LLMCalls = *summary.LLMCalls
				run.LLMSuccessful = *summary.LLMSuccessfulCalls
				run.LLMFailed = *summary.LLMFailedCalls
				run.InputTokens = summary.Usage.InputTokens
				run.CachedTokens = summary.Usage.CachedInputTokens
				run.CacheWriteTokens = summary.Usage.CacheCreationInputTokens
				run.OutputTokens = summary.Usage.OutputTokens
				run.CacheRatio = ratio(summary.Usage.CachedInputTokens, summary.Usage.InputTokens)
				run.EstimatedCost = estimateCost(summary.Usage, baseline.Experiment.PricingAssumption)
				run.RunnerEstimatedCost = summary.EstimatedCostUSD
				run.ToolCalls = summary.ToolCalls
				run.ToolTypes = sortedCounts(summary.ToolCallsByType)
				run.FilesChanged = summary.Patch.FilesChanged
				run.TimedOut = summary.TimedOut
				run.ExitCode = summary.ExitCode
				run.BinaryPath = portableReportPath(options.repoRootPath, summary.Binary.Path)
				run.BinarySHA256 = summary.Binary.SHA256
				aggregate.RunsObserved++
				data.RunSlotsObserved++
				aggregate.ElapsedSeconds += summary.ElapsedSeconds
				aggregate.ProviderRequestSeconds += *summary.ProviderRequestSecond
				aggregate.LLMCalls += *summary.LLMCalls
				aggregate.LLMSuccessful += *summary.LLMSuccessfulCalls
				aggregate.LLMFailed += *summary.LLMFailedCalls
				aggregate.InputTokens += summary.Usage.InputTokens
				aggregate.CachedTokens += summary.Usage.CachedInputTokens
				aggregate.CacheWriteTokens += summary.Usage.CacheCreationInputTokens
				aggregate.OutputTokens += summary.Usage.OutputTokens
				aggregate.EstimatedCost += run.EstimatedCost
				aggregate.RunnerEstimatedCost += summary.EstimatedCostUSD
				aggregate.ToolCalls += summary.ToolCalls
				if aggregate.BinarySHA256 == "" {
					aggregate.BinaryPath = portableReportPath(options.repoRootPath, summary.Binary.Path)
					aggregate.BinarySHA256 = summary.Binary.SHA256
				}
				for name, count := range summary.ToolCallsByType {
					toolTotals[agentID][name] += count
				}
				if task.Language == "" {
					task.Language = summary.Language
				}
				latestEvidence = math.Max(latestEvidence, summary.StartedAtUnix+summary.ElapsedSeconds)
			}
			evaluationDecoded := decodeJSONFile(evaluationPath, &evaluation) == nil
			if evaluationDecoded && validateGoldScoringProjection(taskID, gold) == nil && validateCompletedEvaluation(taskID, agentID, evaluation, gold) == nil {
				aggregate.RawEvaluationsObserved++
				if *evaluation.Resolved {
					aggregate.RawResolved++
					if adjudicated {
						aggregate.AdjudicatedResolved++
						data.AdjudicationsObserved++
					}
				}
			}
			if goldAvailable && evaluationDecoded && validateCompletedEvaluation(taskID, agentID, evaluation, gold) == nil {
				run.EvaluationAvailable = true
				run.Resolved = *evaluation.Resolved
				aggregate.EvaluationsObserved++
				data.EvaluationSlotsObserved++
				if run.RunAvailable {
					aggregate.PairsObserved++
					data.PairSlotsObserved++
					if run.Resolved {
						aggregate.Resolved++
					}
				}
			}
			latestEvidence = math.Max(latestEvidence, evidenceUnix(summaryPath))
			latestEvidence = math.Max(latestEvidence, evidenceUnix(originalEvaluationPath))
			latestEvidence = math.Max(latestEvidence, evidenceUnix(evaluationPath))
			latestEvidence = math.Max(latestEvidence, evidenceUnix(adjudicationPath))
			task.Runs = append(task.Runs, run)
		}
		data.Tasks = append(data.Tasks, task)
	}

	for _, agentID := range localAgents {
		aggregate := aggregates[agentID]
		aggregate.Score = ratioInt(aggregate.Resolved, aggregate.PairsObserved)
		aggregate.UncachedTokens = aggregate.InputTokens - aggregate.CachedTokens
		aggregate.CacheRatio = ratio(aggregate.CachedTokens, aggregate.InputTokens)
		aggregate.ToolTypes = sortedCounts(toolTotals[agentID])
		data.Agents = append(data.Agents, *aggregate)
	}
	data.Complete = data.GoldObserved == localTaskCount && data.RunSlotsObserved == localTaskCount*len(localAgents) &&
		data.EvaluationSlotsObserved == localTaskCount*len(localAgents) && data.PairSlotsObserved == localTaskCount*len(localAgents)
	data.SharedPass = buildSharedPassData(data.Tasks, data.Complete)
	data.EvidenceCutoff = unixFloatTime(latestEvidence)
	data.LubanChanges = buildLubanChanges(baseline.Aggregates["luban"], *aggregates["luban"], language)
	data.HistoricalToolRatio = ratioFloat(float64(baseline.Aggregates["luban"].ToolCalls), float64(baseline.Aggregates["codex"].ToolCalls))
	data.HistoricalCards = historicalCards(baseline.Aggregates)
	data.Optimizations = optimizationCards(options)
	data.BaselineIncidentObserved = strings.TrimSpace(baseline.SecurityIncident.Observed) != ""
	data.Headline = buildHeadline(*aggregates["codex"], *aggregates["luban"])
	data.Efficiency = buildFixedPilotEfficiency(baseline.Aggregates["luban"], *aggregates["luban"])
	data.OptimizationEvidence = buildOptimizationEvidence(options, baseline.Aggregates["luban"], *aggregates["luban"], data.Efficiency)
	return data, nil
}

func buildHeadline(codex, luban agentData) headlineData {
	result := headlineData{
		Available:       codex.RawEvaluationsObserved == localTaskCount && luban.RawEvaluationsObserved == localTaskCount && codex.RunsObserved == localTaskCount && luban.RunsObserved == localTaskCount,
		LubanResolved:   luban.RawResolved,
		CodexResolved:   codex.RawResolved,
		Tasks:           localTaskCount,
		LubanCalls:      luban.LLMCalls,
		CodexCalls:      codex.LLMCalls,
		CallsDelta:      percentDelta(float64(codex.LLMCalls), float64(luban.LLMCalls)),
		WallDelta:       percentDelta(codex.ElapsedSeconds, luban.ElapsedSeconds),
		CostDelta:       percentDelta(codex.EstimatedCost, luban.EstimatedCost),
		CachePointDelta: pointDelta(codex.CacheRatio, luban.CacheRatio),
		LubanCached:     luban.CachedTokens,
		CodexCached:     codex.CachedTokens,
		Adjudicated:     codex.AdjudicatedResolved > 0 || luban.AdjudicatedResolved > 0,
	}
	return result
}

func buildSharedPassData(tasks []taskData, complete bool) sharedPassData {
	if !complete || len(tasks) == 0 {
		return sharedPassData{}
	}
	result := sharedPassData{}
	seenTasks := make(map[string]struct{}, len(tasks))
	codexTools := map[string]int{}
	lubanTools := map[string]int{}
	for _, task := range tasks {
		if strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.Language) == "" || !task.GoldAvailable || len(task.Runs) != len(localAgents) {
			return sharedPassData{}
		}
		if _, exists := seenTasks[task.ID]; exists {
			return sharedPassData{}
		}
		seenTasks[task.ID] = struct{}{}
		runs := map[string]taskRunData{}
		for _, run := range task.Runs {
			if (run.Agent != "codex" && run.Agent != "luban") || !run.RunAvailable || !run.EvaluationAvailable {
				return sharedPassData{}
			}
			if _, duplicate := runs[run.Agent]; duplicate {
				return sharedPassData{}
			}
			runs[run.Agent] = run
		}
		codex, codexExists := runs["codex"]
		luban, lubanExists := runs["luban"]
		if !codexExists || !lubanExists {
			return sharedPassData{}
		}
		if !codex.Resolved || !luban.Resolved {
			continue
		}
		codexMetrics := sharedPassMetricsFromRun(codex)
		lubanMetrics := sharedPassMetricsFromRun(luban)
		result.Tasks = append(result.Tasks, sharedPassTaskData{
			ID: task.ID, Language: task.Language, Codex: codexMetrics, Luban: lubanMetrics,
			Rows:       buildSharedPassRows(codexMetrics, lubanMetrics, 0),
			CodexLinks: codex.RawArtifactLinks, LubanLinks: luban.RawArtifactLinks,
		})
		addSharedPassMetrics(&result.Codex, codexMetrics, codexTools)
		addSharedPassMetrics(&result.Luban, lubanMetrics, lubanTools)
	}
	if len(result.Tasks) == 0 {
		return sharedPassData{}
	}
	result.TaskCount = len(result.Tasks)
	result.Codex.CacheRatio = ratio(result.Codex.CachedTokens, result.Codex.InputTokens)
	result.Luban.CacheRatio = ratio(result.Luban.CachedTokens, result.Luban.InputTokens)
	result.Codex.ToolTypes = sortedCounts(codexTools)
	result.Luban.ToolTypes = sortedCounts(lubanTools)
	if result.Codex.LLMCalls <= 0 || result.Luban.LLMCalls <= 0 || result.Codex.InputTokens <= 0 || result.Luban.InputTokens <= 0 {
		return sharedPassData{}
	}
	result.Rows = buildSharedPassRows(result.Codex, result.Luban, result.TaskCount)

	codexInputPerCall := float64(result.Codex.InputTokens) / float64(result.Codex.LLMCalls)
	lubanInputPerCall := float64(result.Luban.InputTokens) / float64(result.Luban.LLMCalls)
	codexProviderPerCall := result.Codex.ProviderRequestSeconds / float64(result.Codex.LLMCalls)
	lubanProviderPerCall := result.Luban.ProviderRequestSeconds / float64(result.Luban.LLMCalls)
	codexOutputPerCall := float64(result.Codex.OutputTokens) / float64(result.Codex.LLMCalls)
	lubanOutputPerCall := float64(result.Luban.OutputTokens) / float64(result.Luban.LLMCalls)
	result.CodexInputPerCall = decimalValue(codexInputPerCall)
	result.LubanInputPerCall = decimalValue(lubanInputPerCall)
	result.InputPerCallDelta = percentDelta(codexInputPerCall, lubanInputPerCall)
	result.CodexProviderPerCall = preciseSecondsValue(codexProviderPerCall)
	result.LubanProviderPerCall = preciseSecondsValue(lubanProviderPerCall)
	result.ProviderPerCallDelta = percentDelta(codexProviderPerCall, lubanProviderPerCall)
	result.CodexOutputPerCall = decimalValue(codexOutputPerCall)
	result.LubanOutputPerCall = decimalValue(lubanOutputPerCall)
	result.OutputPerCallDelta = percentDelta(codexOutputPerCall, lubanOutputPerCall)
	result.LubanLongerSlower = math.Abs(lubanInputPerCall/codexInputPerCall-1) <= 0.1 &&
		lubanProviderPerCall > codexProviderPerCall && lubanOutputPerCall > codexOutputPerCall
	result.Available = true
	return result
}

func sharedPassMetricsFromRun(run taskRunData) sharedPassMetrics {
	return sharedPassMetrics{
		LLMCalls: run.LLMCalls, ElapsedSeconds: run.ElapsedSeconds, ProviderRequestSeconds: run.ProviderRequestSeconds,
		InputTokens: run.InputTokens, CachedTokens: run.CachedTokens, CacheWriteTokens: run.CacheWriteTokens,
		OutputTokens: run.OutputTokens, CacheRatio: run.CacheRatio, EstimatedCost: run.EstimatedCost,
		ToolCalls: run.ToolCalls, ToolTypes: append([]namedCount(nil), run.ToolTypes...),
	}
}

func addSharedPassMetrics(total *sharedPassMetrics, value sharedPassMetrics, toolTotals map[string]int) {
	total.LLMCalls += value.LLMCalls
	total.ElapsedSeconds += value.ElapsedSeconds
	total.ProviderRequestSeconds += value.ProviderRequestSeconds
	total.InputTokens += value.InputTokens
	total.CachedTokens += value.CachedTokens
	total.CacheWriteTokens += value.CacheWriteTokens
	total.OutputTokens += value.OutputTokens
	total.EstimatedCost += value.EstimatedCost
	total.ToolCalls += value.ToolCalls
	for _, tool := range value.ToolTypes {
		toolTotals[tool.Name] += tool.Count
	}
}

func buildSharedPassRows(codex, luban sharedPassMetrics, solvedTasks int) []sharedPassMetricRow {
	rows := []sharedPassMetricRow{
		sharedPassRow("meter_recorded_POST_/responses", float64(codex.LLMCalls), float64(luban.LLMCalls), integerFloatValue, false),
		sharedPassRow("wall_seconds", codex.ElapsedSeconds, luban.ElapsedSeconds, secondsValue, false),
		sharedPassRow("provider_seconds", codex.ProviderRequestSeconds, luban.ProviderRequestSeconds, secondsValue, false),
		sharedPassRow("input_tokens", float64(codex.InputTokens), float64(luban.InputTokens), integerFloatValue, false),
		sharedPassRow("cached_input_tokens", float64(codex.CachedTokens), float64(luban.CachedTokens), integerFloatValue, false),
		sharedPassRow("cache_creation_input_tokens", float64(codex.CacheWriteTokens), float64(luban.CacheWriteTokens), integerFloatValue, false),
		sharedPassRow("output_tokens", float64(codex.OutputTokens), float64(luban.OutputTokens), integerFloatValue, false),
		sharedPassRow("token_weighted_cache_ratio", codex.CacheRatio, luban.CacheRatio, percentValue, true),
		sharedPassRow("same_gateway_comparable_cost", codex.EstimatedCost, luban.EstimatedCost, costValue, false),
		sharedPassRow("logical_tool_events*", float64(codex.ToolCalls), float64(luban.ToolCalls), integerFloatValue, false),
	}
	if solvedTasks > 0 {
		taskCount := float64(solvedTasks)
		rows = append(rows,
			sharedPassRow("wall_seconds_per_solved_task", codex.ElapsedSeconds/taskCount, luban.ElapsedSeconds/taskCount, secondsValue, false),
			sharedPassRow("cost_per_solved_task", codex.EstimatedCost/taskCount, luban.EstimatedCost/taskCount, costValue, false),
			sharedPassRow("provider_seconds_per_POST", codex.ProviderRequestSeconds/float64(codex.LLMCalls), luban.ProviderRequestSeconds/float64(luban.LLMCalls), preciseSecondsValue, false),
			sharedPassRow("input_tokens_per_POST", float64(codex.InputTokens)/float64(codex.LLMCalls), float64(luban.InputTokens)/float64(luban.LLMCalls), decimalValue, false),
			sharedPassRow("output_tokens_per_POST", float64(codex.OutputTokens)/float64(codex.LLMCalls), float64(luban.OutputTokens)/float64(luban.LLMCalls), decimalValue, false),
		)
	}
	return rows
}

func sharedPassRow(metric string, codex, luban float64, display func(float64) string, pointChange bool) sharedPassMetricRow {
	delta := percentDelta(codex, luban)
	if pointChange {
		delta = pointDelta(codex, luban)
	}
	return sharedPassMetricRow{Metric: metric, Codex: display(codex), Luban: display(luban), Ratio: ratioXValue(codex, luban), Delta: delta}
}

func ratioXValue(codex, luban float64) string {
	if codex == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.2fx", luban/codex)
}

func integerFloatValue(value float64) string   { return integerValue64(int64(value)) }
func decimalValue(value float64) string        { return fmt.Sprintf("%.1f", value) }
func preciseSecondsValue(value float64) string { return fmt.Sprintf("%.2fs", value) }

// buildFixedPilotEfficiency exposes the independently audited local-five
// trajectory projection. It is intentionally fail-closed to the exact frozen
// pilot aggregates; these constants are not a general benchmark estimator.
func buildFixedPilotEfficiency(before legacyAggregate, after agentData) efficiencyDiagnosis {
	compatible := before.ToolCalls == 351 && after.ToolCalls == 163 && after.LLMCalls == 181 &&
		math.Abs(after.ElapsedSeconds-7057.240) < 0.001 && math.Abs(after.ProviderRequestSeconds-6459.292001) < 0.000001
	if !compatible {
		return efficiencyDiagnosis{}
	}
	return efficiencyDiagnosis{
		Available:                  true,
		OldOuterToolEvents:         351,
		OldToolRounds:              145,
		OldUsageRounds:             150,
		NewLogicalToolEvents:       163,
		NewToolRounds:              159,
		NewPhysicalChildOperations: 225,
		NewProviderPOSTs:           181,
		LogicalToolDelta:           percentDelta(351, 163),
		PhysicalOperationDelta:     percentDelta(351, 225),
		ToolRoundDelta:             percentDelta(145, 159),
		ProviderRoundDelta:         percentDelta(150, 181),
		Succeeded:                  87,
		Partial:                    54,
		Failed:                     19,
		TimedOut:                   2,
		Cancelled:                  1,
		InspectCalls:               101,
		InspectPartial:             54,
		ProviderSeconds:            after.ProviderRequestSeconds,
		WallSeconds:                after.ElapsedSeconds,
		ProviderShare:              after.ProviderRequestSeconds / after.ElapsedSeconds,
		TransportRetries:           0,
		ToolRoundErrors:            22,
		RunRevisionBound:           5,
		RunCommittedUnverified:     37,
		RunWithoutRevisionSeal:     1,
		ToolCriticalPathSeconds:    588.810,
		CompletionTailCalls:        72,
		CompletionTailSeconds:      2912,
		CompletionTailCost:         5.2256,
		CompletionRejections:       18,
	}
}

func buildOptimizationEvidence(options options, before legacyAggregate, after agentData, efficiency efficiencyDiagnosis) []optimizationEvidenceRow {
	link := func(paths ...string) []artifactLink {
		result := make([]artifactLink, 0, len(paths))
		for _, path := range paths {
			result = append(result, artifactLink{Label: path, Href: relativeHref(filepath.Dir(options.outputPath), filepath.Join(options.repoRootPath, filepath.FromSlash(path)))})
		}
		return result
	}
	notAvailable := "N/A"
	inspectBefore := before.ToolCallsByType["Read"] + before.ToolCallsByType["Grep"] + before.ToolCallsByType["Glob"] + before.ToolCallsByType["ToolSearch"]
	rows := []optimizationEvidenceRow{
		{CopyKey: string(i18n.KeyAgenticLocal5OptimizationInspectIntegration), Before: integerValue(inspectBefore), After: integerValue(after.ToolTypesCount("Inspect")), Change: percentDelta(float64(inspectBefore), float64(after.ToolTypesCount("Inspect"))), ScopeKey: string(i18n.KeyAgenticLocal5ScopeTrajectoryDiagnostic), Sources: link("benchmark-results/agentic-2026-07-26/results.json", "benchmark-results/agentic-2026-07-27/raw")},
		{CopyKey: string(i18n.KeyAgenticLocal5OptimizationApplyPatchAtomic), Before: integerValue(before.ToolCallsByType["Edit"] + before.ToolCallsByType["Write"]), After: integerValue(after.ToolTypesCount("ApplyPatch")), Change: percentDelta(float64(before.ToolCallsByType["Edit"]+before.ToolCallsByType["Write"]), float64(after.ToolTypesCount("ApplyPatch"))), ScopeKey: string(i18n.KeyAgenticLocal5ScopeTrajectoryDiagnostic), Sources: link("internal/tools/file/apply_patch.go")},
		{CopyKey: string(i18n.KeyAgenticLocal5OptimizationRunVerification), Before: integerValue(before.ToolCallsByType["Bash"]), After: integerValue(after.ToolTypesCount("Run")), Change: percentDelta(float64(before.ToolCallsByType["Bash"]), float64(after.ToolTypesCount("Run"))), ScopeKey: string(i18n.KeyAgenticLocal5ScopeTrajectoryDiagnostic), Sources: link("internal/tools/shell/run.go")},
		{CopyKey: string(i18n.KeyAgenticLocal5OptimizationThreeToolCatalog), Before: "9924 B / ~2481 tokens", After: "3284 B / ~821 tokens", Change: "-66.9%", ScopeKey: string(i18n.KeyAgenticLocal5ScopeUnitFixture), Sources: link("prompt/agentic_v2_test.go")},
		{CopyKey: string(i18n.KeyAgenticLocal5OptimizationContinuationCacheLineage), Before: "48565 tokens", After: "3621 tokens", Change: "-92.5%", ScopeKey: string(i18n.KeyAgenticLocal5ScopeSyntheticFixture), Sources: link("internal/runtime/compact/agentic_v2_proof_test.go")},
		{CopyKey: string(i18n.KeyAgenticLocal5OptimizationPrintSessionQuartet), Before: "1 root-denied", After: "0", Change: "-100%", ScopeKey: string(i18n.KeyAgenticLocal5ScopeFieldDiagnostic), Sources: link("internal/app/printmode_session_test.go")},
		{CopyKey: string(i18n.KeyAgenticLocal5OptimizationInspectCursorCompatibility), Before: "2 zero-progress LLM calls", After: "0", Change: "-100%", ScopeKey: string(i18n.KeyAgenticLocal5ScopeFieldDiagnostic), Sources: link("internal/agentic/inspect/execute.go", "internal/agentic/inspect/cursor_wire_test.go")},
	}
	if !efficiency.Available {
		for index := 0; index < 3; index++ {
			rows[index].Before, rows[index].After, rows[index].Change = notAvailable, notAvailable, notAvailable
		}
	}
	return rows
}

func (agent agentData) ToolTypesCount(name string) int {
	for _, value := range agent.ToolTypes {
		if value.Name == name {
			return value.Count
		}
	}
	return 0
}

func validateCompletedSummary(taskID, agentID string, summary runSummary, pricing pricingData) error {
	if !summary.presence.complete() || summary.InstanceID != taskID || summary.Agent != agentID || strings.TrimSpace(summary.Language) == "" || summary.Model != localModel || summary.ReasoningEffort != localEffort ||
		summary.ElapsedSeconds <= 0 || summary.StartedAtUnix <= 0 || summary.Usage.InputTokens <= 0 ||
		summary.TimeoutSeconds != localTimeout ||
		summary.Usage.CachedInputTokens < 0 || summary.Usage.CacheCreationInputTokens < 0 ||
		summary.Usage.CachedInputTokens+summary.Usage.CacheCreationInputTokens > summary.Usage.InputTokens || summary.Usage.OutputTokens < 0 ||
		summary.LLMCalls == nil || summary.LLMSuccessfulCalls == nil || summary.LLMFailedCalls == nil ||
		*summary.LLMCalls <= 0 || *summary.LLMSuccessfulCalls < 0 || *summary.LLMFailedCalls < 0 ||
		*summary.LLMSuccessfulCalls+*summary.LLMFailedCalls != *summary.LLMCalls || summary.ToolCalls < 0 ||
		summary.ToolCallsByType == nil || summary.Patch.Files == nil || summary.Patch.FilesChanged != len(summary.Patch.Files) || summary.Patch.Additions < 0 || summary.Patch.Deletions < 0 ||
		!uniqueNonempty(summary.Patch.Files) || summary.ProviderRequestSecond == nil || *summary.ProviderRequestSecond <= 0 || strings.TrimSpace(summary.Binary.Path) == "" ||
		summary.Binary.SHA256 != expectedBinarySHA256[agentID] {
		return fmt.Errorf("local5_report:completed_summary_contract")
	}
	toolTotal := 0
	for name, count := range summary.ToolCallsByType {
		if strings.TrimSpace(name) == "" || count < 0 {
			return fmt.Errorf("local5_report:tool_metric_contract")
		}
		toolTotal += count
	}
	if toolTotal != summary.ToolCalls {
		return fmt.Errorf("local5_report:tool_metric_total")
	}
	expectedCost := runnerReportedCost(summary.Usage, pricing)
	if math.IsNaN(summary.EstimatedCostUSD) || math.IsInf(summary.EstimatedCostUSD, 0) || math.Abs(summary.EstimatedCostUSD-expectedCost) > 0.00001 {
		return fmt.Errorf("local5_report:cost_formula_mismatch")
	}
	return nil
}

func (presence runSummaryPresence) complete() bool {
	return presence.TimeoutSeconds && presence.TimedOut && presence.ExitCode && presence.ToolCalls &&
		presence.InputTokens && presence.CachedInputTokens && presence.CacheCreationTokens && presence.OutputTokens &&
		presence.PatchFilesChanged && presence.PatchFiles && presence.PatchAdditions && presence.PatchDeletions
}

func validateProviderRequestLog(path string, summary runSummary) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	line := 0
	calls := 0
	successful := 0
	failed := 0
	seconds := float64(0)
	for scanner.Scan() {
		var record providerRequestRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil || record.Sequence == nil || *record.Sequence != line ||
			record.Status == nil || *record.Status < 100 || *record.Status > 599 || record.ElapsedSeconds == nil || *record.ElapsedSeconds < 0 ||
			(record.RequestIDSHA256 != "" && !validSHA256(record.RequestIDSHA256)) {
			return fmt.Errorf("local5_report:provider_request_log_contract")
		}
		line++
		if record.Method != "POST" || record.Endpoint != "responses" {
			continue
		}
		calls++
		seconds += *record.ElapsedSeconds
		if *record.Status >= 200 && *record.Status < 300 {
			successful++
		} else {
			failed++
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if calls != *summary.LLMCalls || successful != *summary.LLMSuccessfulCalls || failed != *summary.LLMFailedCalls ||
		math.Abs(seconds-*summary.ProviderRequestSecond) > 0.00001 {
		return fmt.Errorf("local5_report:provider_request_projection")
	}
	return nil
}

func validateGoldEvaluation(taskID string, evaluation evaluationData) error {
	if evaluation.InstanceID != taskID || evaluation.Agent != "gold" || strings.TrimSpace(evaluation.Language) == "" || evaluation.Resolved == nil || !*evaluation.Resolved ||
		evaluation.ImagePullExitCode == nil || evaluation.TestPatchApplyExitCode == nil || evaluation.RebuildExitCode == nil ||
		evaluation.TestExitCode == nil || evaluation.PrintExitCode == nil || evaluation.FailToPass.Expected == nil ||
		evaluation.FailToPass.Passed == nil || evaluation.FailToPass.Failed == nil || evaluation.FailToPass.Missing == nil ||
		evaluation.PassToPass.Expected == nil || evaluation.PassToPass.PassedCount == nil || evaluation.PassToPass.Failed == nil || evaluation.PassToPass.MissingCount == nil {
		return fmt.Errorf("local5_report:gold_evaluation_contract")
	}
	if err := validateEvaluationPartitions(evaluation); err != nil {
		return err
	}
	if len(evaluation.FailToPass.Failed) != 0 || len(evaluation.FailToPass.Missing) != 0 ||
		len(evaluation.FailToPass.Passed) != *evaluation.FailToPass.Expected || len(evaluation.PassToPass.Failed) != 0 ||
		*evaluation.PassToPass.MissingCount != 0 || *evaluation.PassToPass.PassedCount != *evaluation.PassToPass.Expected {
		return fmt.Errorf("local5_report:gold_oracle_projection")
	}
	return nil
}

func validateGoldScoringProjection(taskID string, evaluation evaluationData) error {
	if evaluation.InstanceID != taskID || evaluation.Agent != "gold" || strings.TrimSpace(evaluation.Language) == "" || evaluation.Resolved == nil || !*evaluation.Resolved ||
		evaluation.FailToPass.Expected == nil || evaluation.FailToPass.Passed == nil || evaluation.FailToPass.Failed == nil || evaluation.FailToPass.Missing == nil ||
		evaluation.PassToPass.Expected == nil || evaluation.PassToPass.PassedCount == nil || evaluation.PassToPass.Failed == nil || evaluation.PassToPass.MissingCount == nil {
		return fmt.Errorf("local5_report:gold_scoring_projection_contract")
	}
	if err := validateEvaluationPartitions(evaluation); err != nil {
		return err
	}
	if len(evaluation.FailToPass.Failed) != 0 || len(evaluation.FailToPass.Missing) != 0 ||
		len(evaluation.FailToPass.Passed) != *evaluation.FailToPass.Expected || len(evaluation.PassToPass.Failed) != 0 ||
		*evaluation.PassToPass.MissingCount != 0 || *evaluation.PassToPass.PassedCount != *evaluation.PassToPass.Expected {
		return fmt.Errorf("local5_report:gold_scoring_projection")
	}
	return nil
}

func validateCompletedEvaluation(taskID, agentID string, evaluation, gold evaluationData) error {
	if evaluation.InstanceID != taskID || evaluation.Agent != agentID || evaluation.Resolved == nil || evaluation.ElapsedSeconds < 0 ||
		evaluation.Language != gold.Language ||
		evaluation.ImagePullExitCode == nil || evaluation.TestPatchApplyExitCode == nil || evaluation.SolutionPatchApplyExitCode == nil ||
		evaluation.RebuildExitCode == nil || evaluation.TestExitCode == nil || evaluation.PrintExitCode == nil || evaluation.DiagnosticExcludedPaths == nil ||
		evaluation.FailToPass.Expected == nil || evaluation.FailToPass.Passed == nil || evaluation.FailToPass.Failed == nil || evaluation.FailToPass.Missing == nil ||
		evaluation.PassToPass.Expected == nil || evaluation.PassToPass.PassedCount == nil || evaluation.PassToPass.Failed == nil || evaluation.PassToPass.MissingCount == nil {
		return fmt.Errorf("local5_report:completed_evaluation_contract")
	}
	if err := validateEvaluationPartitions(evaluation); err != nil {
		return err
	}
	if *evaluation.FailToPass.Expected != *gold.FailToPass.Expected || *evaluation.PassToPass.Expected != *gold.PassToPass.Expected ||
		!equalStringSets(evaluationTestUniverse(evaluation), evaluationTestUniverse(gold)) {
		return fmt.Errorf("local5_report:evaluator_oracle_binding")
	}
	strictResolved := len(evaluation.FailToPass.Missing) == 0 && len(evaluation.FailToPass.Failed) == 0 &&
		len(evaluation.FailToPass.Passed) == *evaluation.FailToPass.Expected && len(evaluation.PassToPass.Failed) == 0 && *evaluation.PassToPass.MissingCount == 0
	if *evaluation.Resolved != strictResolved {
		return fmt.Errorf("local5_report:evaluator_projection_contract")
	}
	return nil
}

func resolveEvaluationPath(originalPath, taskID, agentID string) (string, string, bool, error) {
	adjudicationPath := filepath.Join(filepath.Dir(originalPath), "adjudication.json")
	var adjudication evaluationAdjudication
	if err := decodeJSONFile(adjudicationPath, &adjudication); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return originalPath, "", false, nil
		}
		return "", adjudicationPath, false, i18n.NewError(i18n.KeyAgenticLocal5CaveatIncompleteRefusal)
	}
	if adjudication.SchemaVersion != "agentic-local-evaluation-adjudication/v1" || adjudication.InstanceID != taskID ||
		adjudication.Agent != agentID || adjudication.OriginalReport != filepath.Base(originalPath) ||
		filepath.Base(adjudication.EffectiveReport) != adjudication.EffectiveReport || adjudication.EffectiveReport == "" ||
		adjudication.ReasonCode == "" || !adjudication.EffectiveResolved || len(adjudication.ExcludedCandidatePath) == 0 {
		return "", adjudicationPath, false, i18n.NewError(i18n.KeyAgenticLocal5CaveatIncompleteRefusal)
	}
	effectivePath := filepath.Join(filepath.Dir(originalPath), adjudication.EffectiveReport)
	var effective evaluationData
	if err := decodeJSONFile(effectivePath, &effective); err != nil || effective.InstanceID != taskID || effective.Agent != agentID ||
		effective.Resolved == nil || *effective.Resolved != adjudication.EffectiveResolved ||
		!equalStringSets(effective.DiagnosticExcludedPaths, adjudication.ExcludedCandidatePath) {
		return "", adjudicationPath, false, i18n.NewError(i18n.KeyAgenticLocal5CaveatIncompleteRefusal)
	}
	return effectivePath, adjudicationPath, true, nil
}

func validateEvaluationPartitions(evaluation evaluationData) error {
	if *evaluation.FailToPass.Expected <= 0 || *evaluation.PassToPass.Expected <= 0 || *evaluation.PassToPass.PassedCount < 0 || *evaluation.PassToPass.MissingCount < 0 ||
		*evaluation.FailToPass.Expected != len(evaluation.FailToPass.Passed)+len(evaluation.FailToPass.Failed)+len(evaluation.FailToPass.Missing) ||
		*evaluation.PassToPass.Expected != *evaluation.PassToPass.PassedCount+len(evaluation.PassToPass.Failed)+*evaluation.PassToPass.MissingCount {
		return fmt.Errorf("local5_report:evaluator_coverage_contract")
	}
	if !uniqueNonempty(evaluation.FailToPass.Passed, evaluation.FailToPass.Failed, evaluation.FailToPass.Missing) || !uniqueNonempty(evaluation.PassToPass.Failed) {
		return fmt.Errorf("local5_report:evaluator_partition_contract")
	}
	return nil
}

func evaluationTestUniverse(evaluation evaluationData) []string {
	result := make([]string, 0, *evaluation.FailToPass.Expected)
	result = append(result, evaluation.FailToPass.Passed...)
	result = append(result, evaluation.FailToPass.Failed...)
	result = append(result, evaluation.FailToPass.Missing...)
	return result
}

func uniqueNonempty(groups ...[]string) bool {
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, value := range group {
			if strings.TrimSpace(value) == "" {
				return false
			}
			if _, exists := seen[value]; exists {
				return false
			}
			seen[value] = struct{}{}
		}
	}
	return true
}

func equalStringSets(left, right []string) bool {
	if len(left) != len(right) || !uniqueNonempty(left) || !uniqueNonempty(right) {
		return false
	}
	values := make(map[string]struct{}, len(left))
	for _, value := range left {
		values[value] = struct{}{}
	}
	for _, value := range right {
		if _, exists := values[value]; !exists {
			return false
		}
	}
	return true
}

func validLegacyAggregate(value legacyAggregate) bool {
	if value.TotalTasks != localTaskCount || value.Resolved < 0 || value.Resolved > localTaskCount || value.ElapsedSeconds <= 0 ||
		value.EstimatedCostUSD < 0 || value.ToolCalls < 0 || value.InputTokens <= 0 || value.CachedInputTokens < 0 ||
		value.CachedInputTokens > value.InputTokens || value.UncachedInputTokens != value.InputTokens-value.CachedInputTokens ||
		value.OutputTokens < 0 || math.IsNaN(value.CacheRatio) || math.Abs(value.CacheRatio-ratio(value.CachedInputTokens, value.InputTokens)) > 0.000001 {
		return false
	}
	total := 0
	for name, count := range value.ToolCallsByType {
		if strings.TrimSpace(name) == "" || count < 0 {
			return false
		}
		total += count
	}
	return total == value.ToolCalls
}

func validPricing(pricing pricingData) bool {
	return pricing.InputPerMillionUSD == inputRate && pricing.CachedInputPerMillionUSD == cachedInputRate && pricing.OutputPerMillionUSD == outputRate
}

func validExperiment(value experimentData) bool {
	if value.Dataset == "" || !validSHA1(value.DatasetRevision) || value.Model != localModel || value.ReasoningEffort != localEffort ||
		value.TimeoutSeconds != localTimeout || !validPricing(value.PricingAssumption) || len(value.ParquetSHA256) != localTaskCount {
		return false
	}
	for language, digest := range value.ParquetSHA256 {
		if strings.TrimSpace(language) == "" || !validSHA256(digest) {
			return false
		}
	}
	return true
}

func equalExperiment(left, right experimentData) bool {
	if left.Dataset != right.Dataset || left.DatasetRevision != right.DatasetRevision || left.Model != right.Model ||
		left.ReasoningEffort != right.ReasoningEffort || left.TimeoutSeconds != right.TimeoutSeconds || left.PricingAssumption != right.PricingAssumption ||
		len(left.ParquetSHA256) != len(right.ParquetSHA256) {
		return false
	}
	for language, digest := range left.ParquetSHA256 {
		if right.ParquetSHA256[language] != digest {
			return false
		}
	}
	return true
}

func estimateCost(usage usageData, pricing pricingData) float64 {
	uncached := usage.InputTokens - usage.CachedInputTokens - usage.CacheCreationInputTokens
	return (float64(uncached)*pricing.InputPerMillionUSD +
		float64(usage.CachedInputTokens)*pricing.CachedInputPerMillionUSD +
		float64(usage.CacheCreationInputTokens)*cacheWriteRate +
		float64(usage.OutputTokens)*pricing.OutputPerMillionUSD) / 1_000_000
}

// runnerReportedCost reproduces the legacy runner receipt exactly. Its
// uncached base includes cache-write tokens, so the report never uses it as the
// comparable headline estimate; estimateCost applies each token class once.
func runnerReportedCost(usage usageData, pricing pricingData) float64 {
	uncached := usage.InputTokens - usage.CachedInputTokens
	return (float64(uncached)*pricing.InputPerMillionUSD +
		float64(usage.CachedInputTokens)*pricing.CachedInputPerMillionUSD +
		float64(usage.CacheCreationInputTokens)*cacheWriteRate +
		float64(usage.OutputTokens)*pricing.OutputPerMillionUSD) / 1_000_000
}

func buildLubanChanges(before legacyAggregate, after agentData, language i18n.Language) []changeRow {
	missing := i18n.Text(language, i18n.KeyAgenticReportNotApplicable)
	qualityAfter, qualityChange, qualityTone := missing, missing, "unknown"
	if after.PairsObserved == localTaskCount {
		qualityAfter = scoreValue(after.Resolved, after.PairsObserved)
		qualityChange = pointDelta(float64(before.Resolved)/float64(before.TotalTasks), after.Score)
		qualityTone = higherTone(after.Score, float64(before.Resolved)/float64(before.TotalTasks))
	}
	runAfter := func(value string) string {
		if after.RunsObserved != localTaskCount {
			return missing
		}
		return value
	}
	runChange := func(value string) string {
		if after.RunsObserved != localTaskCount {
			return missing
		}
		return value
	}
	runTone := func(value string) string {
		if after.RunsObserved != localTaskCount {
			return "unknown"
		}
		return value
	}
	return []changeRow{
		{MetricKey: string(i18n.KeyAgenticReportMetricPassRate), Before: scoreValue(before.Resolved, before.TotalTasks), After: qualityAfter, Change: qualityChange, Tone: qualityTone},
		{MetricKey: string(i18n.KeyAgenticReportMetricWallTimeSeconds), Before: secondsValue(before.ElapsedSeconds), After: runAfter(secondsValue(after.ElapsedSeconds)), Change: runChange(percentDelta(before.ElapsedSeconds, after.ElapsedSeconds)), Tone: runTone(lowerTone(after.ElapsedSeconds, before.ElapsedSeconds))},
		{MetricKey: string(i18n.KeyAgenticLocal5MetricMeterRecordedPOST), Before: missing, After: runAfter(integerValue(after.LLMCalls)), Change: missing, Tone: "unknown"},
		{MetricKey: string(i18n.KeyAgenticReportMetricInputTokens), Before: integerValue64(before.InputTokens), After: runAfter(integerValue64(after.InputTokens)), Change: runChange(percentDelta(float64(before.InputTokens), float64(after.InputTokens))), Tone: runTone(lowerTone(float64(after.InputTokens), float64(before.InputTokens)))},
		{MetricKey: string(i18n.KeyAgenticReportMetricOutputTokens), Before: integerValue64(before.OutputTokens), After: runAfter(integerValue64(after.OutputTokens)), Change: runChange(percentDelta(float64(before.OutputTokens), float64(after.OutputTokens))), Tone: runTone(lowerTone(float64(after.OutputTokens), float64(before.OutputTokens)))},
		{MetricKey: string(i18n.KeyAgenticReportMetricTokenWeightedCacheHit), Before: percentValue(before.CacheRatio), After: runAfter(percentValue(after.CacheRatio)), Change: runChange(pointDelta(before.CacheRatio, after.CacheRatio)), Tone: runTone(higherTone(after.CacheRatio, before.CacheRatio))},
		{MetricKey: string(i18n.KeyAgenticReportMetricCatalogCost), Before: costValue(before.EstimatedCostUSD), After: runAfter(costValue(after.EstimatedCost)), Change: runChange(percentDelta(before.EstimatedCostUSD, after.EstimatedCost)), Tone: runTone(lowerTone(after.EstimatedCost, before.EstimatedCostUSD))},
		{MetricKey: string(i18n.KeyAgenticReportMetricToolInvocations), Before: integerValue(before.ToolCalls), After: runAfter(integerValue(after.ToolCalls)), Change: runChange(percentDelta(float64(before.ToolCalls), float64(after.ToolCalls))), Tone: "diagnostic"},
	}
}

func historicalCards(aggregates map[string]legacyAggregate) []evidenceCard {
	luban := aggregates["luban"]
	codex := aggregates["codex"]
	inspection := luban.ToolCallsByType["Read"] + luban.ToolCallsByType["Grep"] + luban.ToolCallsByType["Glob"]
	mutation := luban.ToolCallsByType["Edit"] + luban.ToolCallsByType["Write"]
	return []evidenceCard{
		{CopyKey: string(i18n.KeyAgenticLocal5RootCauseFragmentedSurface), Evidence: fmt.Sprintf("Read+Grep+Glob=%d/%d (%.1f%%); ToolSearch=%d", inspection, luban.ToolCalls, 100*ratioInt(inspection, luban.ToolCalls), luban.ToolCallsByType["ToolSearch"])},
		{CopyKey: string(i18n.KeyAgenticLocal5RootCauseSequentialMutation), Evidence: fmt.Sprintf("Edit+Write=%d; Codex file_change=%d; taxonomy_asymmetric=true", mutation, codex.ToolCallsByType["file_change"])},
		{CopyKey: string(i18n.KeyAgenticLocal5RootCauseShellOverloading), Evidence: fmt.Sprintf("Bash=%d; Luban wall=%s; Codex wall=%s", luban.ToolCallsByType["Bash"], secondsValue(luban.ElapsedSeconds), secondsValue(codex.ElapsedSeconds))},
		{CopyKey: string(i18n.KeyAgenticLocal5RootCauseTelemetryConflation), Evidence: fmt.Sprintf("tool_event_ratio=%.2fx; quality=%d/%d vs %d/%d; output_ratio=%.2fx", ratioFloat(float64(luban.ToolCalls), float64(codex.ToolCalls)), luban.Resolved, luban.TotalTasks, codex.Resolved, codex.TotalTasks, ratioFloat(float64(luban.OutputTokens), float64(codex.OutputTokens)))},
	}
}

func optimizationCards(options options) []optimizationCard {
	definitions := []struct {
		key   i18n.Key
		paths []string
	}{
		{i18n.KeyAgenticLocal5OptimizationInspectIntegration, []string{"internal/tools/search/inspect_bridge.go", "internal/tools/file/read_multiformat.go"}},
		{i18n.KeyAgenticLocal5OptimizationApplyPatchAtomic, []string{"internal/tools/file/apply_patch.go", "internal/tools/file/apply_patch_apply.go", "internal/tools/file/apply_patch_parser.go"}},
		{i18n.KeyAgenticLocal5OptimizationRunVerification, []string{"internal/tools/shell/run.go", "internal/tools/shell/run_verification.go"}},
		{i18n.KeyAgenticLocal5OptimizationThreeToolCatalog, []string{"registry/visible_snapshot.go", "internal/app/registry_setup.go", "prompt/static_sections.go"}},
		{i18n.KeyAgenticLocal5OptimizationContinuationCacheLineage, []string{"provider/responses_continuation.go", "provider/prompt_cache_scope.go", "internal/runtime/loop/revision_fusion.go"}},
		{i18n.KeyAgenticLocal5OptimizationUnifiedAttemptRetry, []string{"provider/attempt_controller.go", "provider/stream_watchdog.go", "internal/runtime/loop/flight_controller.go"}},
		{i18n.KeyAgenticLocal5OptimizationPreciseTelemetry, []string{"internal/runtime/loop/tool_operation_metrics.go", "benchmark/agentic/harness/evidence.go", "benchmark-results/agentic-2026-07-27/run_benchmark.py"}},
		{i18n.KeyAgenticLocal5OptimizationPrintSessionQuartet, []string{"internal/app/printmode.go", "internal/app/main.go", "internal/app/printmode_session_test.go"}},
		{i18n.KeyAgenticLocal5OptimizationInspectCursorCompatibility, []string{"internal/agentic/inspect/execute.go", "internal/agentic/inspect/cursor_wire_test.go"}},
	}
	result := make([]optimizationCard, 0, len(definitions))
	for _, definition := range definitions {
		card := optimizationCard{CopyKey: string(definition.key)}
		for _, path := range definition.paths {
			card.Sources = append(card.Sources, artifactLink{Label: path, Href: relativeHref(filepath.Dir(options.outputPath), filepath.Join(options.repoRootPath, filepath.FromSlash(path)))})
		}
		result = append(result, card)
	}
	return result
}

func rawLinks(options options, taskID, agentID, summaryPath, originalEvaluationPath, effectiveEvaluationPath, adjudicationPath string) []artifactLink {
	outputDir := filepath.Dir(options.outputPath)
	links := make([]artifactLink, 0, 9)
	for _, required := range []struct {
		label string
		path  string
	}{
		{label: "summary.json", path: summaryPath},
		{label: "evaluation/report.json", path: originalEvaluationPath},
	} {
		if info, err := os.Stat(required.path); err == nil && info.Mode().IsRegular() {
			links = append(links, artifactLink{Label: required.label, Href: relativeHref(outputDir, required.path)})
		}
	}
	if effectiveEvaluationPath != originalEvaluationPath {
		for _, adjudicated := range []struct {
			label string
			path  string
		}{
			{label: "evaluation/effective-report.json", path: effectiveEvaluationPath},
			{label: "evaluation/adjudication.json", path: adjudicationPath},
		} {
			if info, err := os.Stat(adjudicated.path); err == nil && info.Mode().IsRegular() {
				links = append(links, artifactLink{Label: adjudicated.label, Href: relativeHref(outputDir, adjudicated.path)})
			}
		}
	}
	runRoot := filepath.Join(options.rootPath, "raw", "runs", taskID, agentID)
	for _, name := range []string{"events.jsonl", "provider-requests.jsonl", "model.patch"} {
		path := filepath.Join(runRoot, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			links = append(links, artifactLink{Label: name, Href: relativeHref(outputDir, path)})
		}
	}
	return links
}

func decodeJSONFile(path string, target any) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		if err != nil {
			return err
		}
		return fmt.Errorf("local5_report:non_regular_input")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	return nil
}

func renderReport(writer io.Writer, data reportData, language i18n.Language) error {
	templateValue, err := template.New("local5-report").Funcs(template.FuncMap{
		"tr":        func(key string) string { return i18n.Text(language, i18n.Key(key)) },
		"tf":        func(key string, arguments ...any) string { return i18n.Format(language, i18n.Key(key), arguments...) },
		"seconds":   secondsValue,
		"percent":   percentValue,
		"cost":      costValue,
		"integer":   integerValue,
		"integer64": integerValue64,
		"timeValue": func(value time.Time) string { return value.UTC().Format(time.RFC3339) },
		"ratioX":    func(value float64) string { return fmt.Sprintf("%.2fx", value) },
		"result": func(value bool) string {
			if value {
				return i18n.Text(language, i18n.KeyAgenticReportStatusPass)
			}
			return i18n.Text(language, i18n.KeyAgenticReportStatusFail)
		},
		"resultClass": func(value bool) string {
			if value {
				return "good"
			}
			return "bad"
		},
		"score": scoreValue,
	}).ParseFS(reportFiles, "local5-report.html.tmpl")
	if err != nil {
		return err
	}
	return templateValue.ExecuteTemplate(writer, "local5-report.html.tmpl", data)
}

func sortedCounts(values map[string]int) []namedCount {
	result := make([]namedCount, 0, len(values))
	for name, count := range values {
		result = append(result, namedCount{Name: name, Count: count})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Count != result[right].Count {
			return result[left].Count > result[right].Count
		}
		return result[left].Name < result[right].Name
	})
	return result
}

func relativeHref(base, target string) string {
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return filepath.ToSlash(filepath.Base(target))
	}
	return filepath.ToSlash(relative)
}

func portableReportPath(repositoryRoot, value string) string {
	trimmed := strings.TrimSpace(value)
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." {
		return ""
	}
	if !filepath.IsAbs(cleaned) {
		foreignAbsolute := strings.HasPrefix(normalized, "/") ||
			(len(normalized) >= 3 && normalized[1] == ':' && normalized[2] == '/')
		if foreignAbsolute {
			return portablePathBase(normalized)
		}
		return filepath.ToSlash(cleaned)
	}
	repository, err := filepath.Abs(repositoryRoot)
	if err == nil {
		relative, relativeErr := filepath.Rel(repository, cleaned)
		if relativeErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(relative)
		}
	}
	return filepath.Base(cleaned)
}

func portablePathBase(value string) string {
	trimmed := strings.TrimRight(value, "/")
	if separator := strings.LastIndexByte(trimmed, '/'); separator >= 0 {
		return trimmed[separator+1:]
	}
	return trimmed
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func evidenceUnix(path string) float64 {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0
	}
	return float64(info.ModTime().UnixNano()) / float64(time.Second)
}

func ratio(numerator, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func ratioInt(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func ratioFloat(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func unixFloatTime(value float64) time.Time {
	seconds, fraction := math.Modf(value)
	return time.Unix(int64(seconds), int64(fraction*float64(time.Second))).UTC()
}

func secondsValue(value float64) string {
	minutes := int(value) / 60
	seconds := value - float64(minutes*60)
	if minutes > 0 {
		return fmt.Sprintf("%dm %.1fs", minutes, seconds)
	}
	return fmt.Sprintf("%.1fs", seconds)
}

func percentValue(value float64) string { return fmt.Sprintf("%.1f%%", value*100) }
func costValue(value float64) string    { return fmt.Sprintf("$%.4f", value) }
func integerValue(value int) string     { return fmt.Sprintf("%d", value) }
func integerValue64(value int64) string { return fmt.Sprintf("%d", value) }

func scoreValue(passed, total int) string {
	if total == 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d/%d (%.1f%%)", passed, total, 100*float64(passed)/float64(total))
}

func percentDelta(before, after float64) string {
	if before == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%+.1f%%", 100*(after/before-1))
}

func pointDelta(before, after float64) string { return fmt.Sprintf("%+.1f pp", 100*(after-before)) }

func lowerTone(after, before float64) string {
	if after < before {
		return "good"
	}
	if after > before {
		return "bad"
	}
	return "neutral"
}

func higherTone(after, before float64) string {
	if after > before {
		return "good"
	}
	if after < before {
		return "bad"
	}
	return "neutral"
}

func languageCode(language i18n.Language) string {
	if language == i18n.LangZH {
		return "zh-CN"
	}
	return language.Code()
}

func validSHA256(value string) bool {
	return validLowerHex(value, 64)
}

func validSHA1(value string) bool {
	return validLowerHex(value, 40)
}

func validLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}
