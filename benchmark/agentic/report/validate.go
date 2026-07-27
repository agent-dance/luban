package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var (
	reportIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]*$`)
	hex64Pattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hex40Pattern    = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

func LoadInput(path string) (Input, OptimizationLedger, error) {
	var input Input
	if err := decodeStrictFile(path, &input); err != nil {
		return Input{}, OptimizationLedger{}, fmt.Errorf("decode report input: %w", err)
	}
	if err := ValidateInput(input); err != nil {
		return Input{}, OptimizationLedger{}, err
	}
	base := filepath.Dir(path)
	optimizationPath, err := resolveConfigPath(base, input.OptimizationLedger.Path)
	if err != nil {
		return Input{}, OptimizationLedger{}, fmt.Errorf("optimization ledger path: %w", err)
	}
	digest, err := hashFile(optimizationPath)
	if err != nil {
		return Input{}, OptimizationLedger{}, fmt.Errorf("hash optimization ledger: %w", err)
	}
	if digest != input.OptimizationLedger.SHA256 {
		return Input{}, OptimizationLedger{}, errors.New("optimization ledger SHA-256 does not match report input")
	}
	var ledger OptimizationLedger
	if err := decodeStrictFile(optimizationPath, &ledger); err != nil {
		return Input{}, OptimizationLedger{}, fmt.Errorf("decode optimization ledger: %w", err)
	}
	if err := ValidateOptimizationLedger(ledger); err != nil {
		return Input{}, OptimizationLedger{}, err
	}
	return input, ledger, nil
}

func ValidateInput(input Input) error {
	var problems []string
	if input.SchemaVersion != InputSchemaVersion {
		problems = append(problems, "schema_version must be "+InputSchemaVersion)
	}
	if strings.TrimSpace(input.Report.Title) == "" || strings.TrimSpace(input.Report.Benchmark) == "" || strings.TrimSpace(input.Report.BenchmarkVersion) == "" {
		problems = append(problems, "report title, benchmark, and benchmark_version are required")
	}
	if input.Report.BenchmarkContractID != BenchmarkContractDeepSWEV11Pilot5 && input.Report.BenchmarkContractID != BenchmarkContractDeepSWEV11Full113 {
		problems = append(problems, "report.benchmark_contract_id is not a registered exact contract ID")
	}
	if !slices.Contains([]string{"en", "zh-CN", "de", "ja", "ko", "ru"}, input.Report.Language) {
		problems = append(problems, "report.language is unsupported")
	}
	validateID("report.baseline_agent_id", input.Report.BaselineAgentID, &problems)
	validateID("report.contender_agent_id", input.Report.ContenderAgentID, &problems)
	if input.Report.BaselineAgentID == input.Report.ContenderAgentID {
		problems = append(problems, "report baseline and contender agents must differ")
	}
	if input.Report.AsOf.IsZero() {
		problems = append(problems, "report.as_of is required")
	}
	if input.Statistics.ConfidenceLevel != ReportConfidenceLevel || input.Statistics.Method != ReportStatisticsMethod || input.Statistics.Resamples != ReportStatisticsResamples || input.Statistics.Seed != ReportStatisticsSeed {
		problems = append(problems, "statistics differs from the frozen comparative-inference contract")
	}
	if len(input.ArtifactSources) == 0 && len(input.DiagnosticExperiments) == 0 {
		problems = append(problems, "at least one artifact source or diagnostic experiment is required")
	}
	if strings.TrimSpace(input.OptimizationLedger.Path) == "" || !hex64Pattern.MatchString(input.OptimizationLedger.SHA256) {
		problems = append(problems, "optimization_ledger requires path and SHA-256")
	}
	ids := map[string]struct{}{}
	formalSources, pilotSources := 0, 0
	for index, source := range input.ArtifactSources {
		prefix := fmt.Sprintf("artifact_sources[%d]", index)
		validateID(prefix+".id", source.ID, &problems)
		if _, exists := ids[source.ID]; exists {
			problems = append(problems, prefix+".id is duplicated")
		}
		ids[source.ID] = struct{}{}
		if source.Class != ClassPilot && source.Class != ClassFormal {
			problems = append(problems, prefix+".class must be pilot or formal")
		}
		if source.Class == ClassFormal {
			formalSources++
		} else if source.Class == ClassPilot {
			pilotSources++
		}
		if strings.TrimSpace(source.Label) == "" || strings.TrimSpace(source.Description) == "" {
			problems = append(problems, prefix+" label and description are required")
		}
		if err := validateConfigPath(source.Root); err != nil {
			problems = append(problems, prefix+".root "+err.Error())
		}
		if !hex64Pattern.MatchString(source.LedgerFileSHA256) {
			problems = append(problems, prefix+".ledger_file_sha256 must be a SHA-256")
		}
	}
	if formalSources > 1 {
		problems = append(problems, "at most one formal artifact source is allowed for an unambiguous verdict")
	}
	switch input.Report.BenchmarkContractID {
	case BenchmarkContractDeepSWEV11Full113:
		if formalSources != 1 || pilotSources != 0 {
			problems = append(problems, "full113 benchmark contract requires exactly one formal source and no pilot sources")
		}
	case BenchmarkContractDeepSWEV11Pilot5:
		if formalSources != 0 {
			problems = append(problems, "pilot5 development contract cannot contain a formal source")
		}
	}
	for index, experiment := range input.DiagnosticExperiments {
		prefix := fmt.Sprintf("diagnostic_experiments[%d]", index)
		validateID(prefix+".id", experiment.ID, &problems)
		if _, exists := ids[experiment.ID]; exists {
			problems = append(problems, prefix+".id is duplicated")
		}
		ids[experiment.ID] = struct{}{}
		if experiment.Class != ClassDiagnosticCanary {
			problems = append(problems, prefix+".class must be diagnostic_canary")
		}
		if strings.TrimSpace(experiment.Label) == "" || strings.TrimSpace(experiment.Description) == "" || strings.TrimSpace(experiment.SourceNote) == "" {
			problems = append(problems, prefix+" label, description, and source_note are required")
		}
		if len(experiment.Runs) == 0 {
			problems = append(problems, prefix+".runs is required")
		}
		seenRuns := map[string]struct{}{}
		for runIndex, run := range experiment.Runs {
			runPrefix := fmt.Sprintf("%s.runs[%d]", prefix, runIndex)
			validateDiagnosticRun(runPrefix, run, &problems)
			key := fmt.Sprintf("%s/%s/%d", run.TaskID, run.AgentID, run.Repetition)
			if _, exists := seenRuns[key]; exists {
				problems = append(problems, runPrefix+" duplicates a task/agent/repetition")
			}
			seenRuns[key] = struct{}{}
		}
	}
	seenReferences := map[string]struct{}{}
	for index, reference := range input.PublicReferences {
		prefix := fmt.Sprintf("public_references[%d]", index)
		validateID(prefix+".id", reference.ID, &problems)
		if _, exists := seenReferences[reference.ID]; exists {
			problems = append(problems, prefix+".id is duplicated")
		}
		seenReferences[reference.ID] = struct{}{}
		validatePublicReference(prefix, reference, &problems)
		if !input.Report.AsOf.IsZero() && reference.AccessedAt.After(input.Report.AsOf) {
			problems = append(problems, prefix+".accessed_at is later than report.as_of")
		}
	}
	for index, annotation := range input.FailureAnnotations {
		prefix := fmt.Sprintf("failure_annotations[%d]", index)
		validateID(prefix+".experiment_id", annotation.ExperimentID, &problems)
		validateID(prefix+".task_id", annotation.TaskID, &problems)
		validateID(prefix+".agent_id", annotation.AgentID, &problems)
		if annotation.Repetition < 0 || !validFailureCategory(annotation.Category) || strings.TrimSpace(annotation.Summary) == "" || len(annotation.Evidence) == 0 {
			problems = append(problems, prefix+" requires non-negative repetition, category, summary, and evidence")
		}
	}
	for index, command := range input.Reproduction {
		prefix := fmt.Sprintf("reproduction[%d]", index)
		if strings.TrimSpace(command.Label) == "" || len(command.Argv) == 0 {
			problems = append(problems, prefix+" label and argv are required")
		}
		for _, arg := range command.Argv {
			if arg == "" || strings.ContainsAny(arg, "\r\n\x00") {
				problems = append(problems, prefix+".argv contains an empty or control-bearing argument")
			}
		}
	}
	if len(input.Limitations) == 0 {
		problems = append(problems, "limitations must explicitly state at least one limitation")
	}
	for index, limitation := range input.Limitations {
		if strings.TrimSpace(limitation) == "" {
			problems = append(problems, fmt.Sprintf("limitations[%d] is empty", index))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid report input: %s", strings.Join(problems, "; "))
	}
	return nil
}

func ValidateOptimizationLedger(ledger OptimizationLedger) error {
	var problems []string
	if ledger.SchemaVersion != OptimizationSchemaVersion {
		problems = append(problems, "schema_version must be "+OptimizationSchemaVersion)
	}
	seen := map[string]struct{}{}
	for index, entry := range ledger.Entries {
		prefix := fmt.Sprintf("entries[%d]", index)
		validateID(prefix+".id", entry.ID, &problems)
		if _, exists := seen[entry.ID]; exists {
			problems = append(problems, prefix+".id is duplicated")
		}
		seen[entry.ID] = struct{}{}
		if strings.TrimSpace(entry.Title) == "" || strings.TrimSpace(entry.Summary) == "" || strings.TrimSpace(entry.DesignDefect) == "" ||
			strings.TrimSpace(entry.Mechanism) == "" || strings.TrimSpace(entry.Value) == "" || strings.TrimSpace(entry.ExpectedEffect) == "" || strings.TrimSpace(entry.ObservedEffect) == "" {
			problems = append(problems, prefix+" title, summary, design_defect, mechanism, value, expected_effect, and observed_effect are required")
		}
		if len(entry.Risks) == 0 || len(entry.Implementation) == 0 || len(entry.Metrics) == 0 {
			problems = append(problems, prefix+" risks, implementation, and metrics are required")
		}
		validateEndpoint(prefix+".before", entry.Before, &problems)
		validateEndpoint(prefix+".after", entry.After, &problems)
		if entry.Before == entry.After {
			problems = append(problems, prefix+" before and after endpoints must differ")
		}
		if entry.EvidenceClass != ClassDiagnosticCanary && entry.EvidenceClass != ClassPilot && entry.EvidenceClass != ClassFormal {
			problems = append(problems, prefix+".evidence_class is invalid")
		}
		if !slices.Contains([]AttributionScope{AttributionDiagnosticAssociation, AttributionCausalFeatureAblation, AttributionDesignRationale}, entry.AttributionScope) {
			problems = append(problems, prefix+".attribution_scope is invalid")
		}
		if !slices.Contains([]MeasurementLayer{MeasurementControllerEndToEnd, MeasurementTrial, MeasurementAgent, MeasurementProvider, MeasurementTool, MeasurementMixed}, entry.MeasurementLayer) {
			problems = append(problems, prefix+".measurement_layer is invalid")
		}
		if !slices.Contains([]EvidenceGrade{EvidenceMeasuredAblation, EvidenceDiagnosticBundle, EvidenceNotRun}, entry.EvidenceGrade) {
			problems = append(problems, prefix+".evidence_grade is invalid")
		}
		for confounderIndex, confounder := range entry.Confounders {
			if strings.TrimSpace(confounder) == "" {
				problems = append(problems, fmt.Sprintf("%s.confounders[%d] is empty", prefix, confounderIndex))
			}
		}
		switch entry.EvidenceGrade {
		case EvidenceMeasuredAblation:
			if entry.AttributionScope != AttributionCausalFeatureAblation || entry.Ablation.Status != AblationMeasured || len(entry.Confounders) != 0 {
				problems = append(problems, prefix+" measured_ablation requires causal_feature_ablation, a measured endpoint, and no unresolved confounders")
			}
		case EvidenceDiagnosticBundle:
			if entry.AttributionScope != AttributionDiagnosticAssociation || entry.Ablation.Status != AblationNotRun || len(entry.Confounders) == 0 {
				problems = append(problems, prefix+" diagnostic_bundle requires diagnostic_association, ablation not_run, and explicit confounders")
			}
		case EvidenceNotRun:
			if entry.AttributionScope == AttributionCausalFeatureAblation || entry.Ablation.Status == AblationMeasured {
				problems = append(problems, prefix+" not_run evidence cannot claim causal attribution or a measured ablation")
			}
		}
		validateAblation(prefix+".ablation", entry.Ablation, &problems)
		seenMetrics := map[ComparisonMetric]struct{}{}
		for _, metric := range entry.Metrics {
			if !validComparisonMetric(metric) {
				problems = append(problems, prefix+".metrics contains an unsupported metric")
			}
			if _, exists := seenMetrics[metric]; exists {
				problems = append(problems, prefix+".metrics contains a duplicate")
			}
			seenMetrics[metric] = struct{}{}
		}
		for implementationIndex, implementation := range entry.Implementation {
			implementationPrefix := fmt.Sprintf("%s.implementation[%d]", prefix, implementationIndex)
			if err := validateRelativePath(implementation.Path); err != nil {
				problems = append(problems, implementationPrefix+".path "+err.Error())
			}
			if !hex64Pattern.MatchString(implementation.SHA256) {
				problems = append(problems, implementationPrefix+".sha256 is required and must be valid")
			}
			if strings.TrimSpace(implementation.Summary) == "" {
				problems = append(problems, implementationPrefix+".summary is required")
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid optimization ledger: %s", strings.Join(problems, "; "))
	}
	return nil
}

func validateDiagnosticRun(prefix string, run DiagnosticRun, problems *[]string) {
	validateID(prefix+".task_id", run.TaskID, problems)
	validateID(prefix+".agent_id", run.AgentID, problems)
	if run.Repetition < 0 || strings.TrimSpace(run.Variant) == "" || strings.TrimSpace(run.Provider) == "" || strings.TrimSpace(run.Model) == "" || strings.TrimSpace(run.ReasoningEffort) == "" {
		*problems = append(*problems, prefix+" requires variant, provider, model, reasoning_effort, and non-negative repetition")
	}
	validateOptionalMetrics(prefix+".metrics", run.Metrics, problems)
	seen := map[string]struct{}{}
	for index, tool := range run.Tools {
		toolPrefix := fmt.Sprintf("%s.tools[%d]", prefix, index)
		if strings.TrimSpace(tool.Name) == "" {
			*problems = append(*problems, toolPrefix+".name is required")
		}
		if _, exists := seen[tool.Name]; exists {
			*problems = append(*problems, toolPrefix+".name is duplicated")
		}
		seen[tool.Name] = struct{}{}
		validateNonNegativeInt(toolPrefix+".calls", tool.Calls, problems)
		validateNonNegativeInt(toolPrefix+".errors", tool.Errors, problems)
		validateNonNegativeInt64(toolPrefix+".duration_ms", tool.DurationMS, problems)
		if tool.Calls != nil && tool.Errors != nil && *tool.Errors > *tool.Calls {
			*problems = append(*problems, toolPrefix+" errors exceed calls")
		}
	}
}

func validateOptionalMetrics(prefix string, metrics OptionalMetrics, problems *[]string) {
	validateNonNegativeFloat(prefix+".wall_time_seconds", metrics.WallTimeSeconds, problems)
	validateNonNegativeFloat(prefix+".trial_duration_seconds", metrics.TrialDurationSeconds, problems)
	for name, value := range map[string]*int{
		"llm_calls_started": metrics.LLMCallsStarted, "provider_rounds": metrics.ProviderRounds,
		"provider_errors": metrics.ProviderErrors, "tool_bearing_rounds": metrics.ToolBearingRounds,
		"tool_invocations": metrics.ToolInvocations, "physical_tool_operations": metrics.PhysicalToolOperations,
		"native_events": metrics.NativeEvents, "tool_errors": metrics.ToolErrors,
	} {
		validateNonNegativeInt(prefix+"."+name, value, problems)
	}
	for name, value := range map[string]*int64{
		"tool_critical_path_ms": metrics.ToolCriticalPathMS, "tool_total_latency_ms": metrics.ToolTotalLatencyMS,
		"tool_queue_ms": metrics.ToolQueueMS, "input_tokens": metrics.InputTokens,
		"cached_input_tokens": metrics.CachedInputTokens, "cache_write_input_tokens": metrics.CacheWriteInputTokens,
		"output_tokens":           metrics.OutputTokens,
		"reasoning_output_tokens": metrics.ReasoningOutputTokens,
	} {
		validateNonNegativeInt64(prefix+"."+name, value, problems)
	}
	validateNonNegativeFloat(prefix+".comparable_cost", metrics.ComparableCost, problems)
	if metrics.ComparableCost != nil && metrics.ComparableCostBasis != comparableCostBasisFrozen && metrics.ComparableCostBasis != ComparableCostBasisDevelopmentNonBilling {
		*problems = append(*problems, prefix+".comparable_cost requires the frozen all-transport basis")
	}
	if metrics.ComparableCost == nil && metrics.ComparableCostBasis != "" {
		*problems = append(*problems, prefix+".comparable_cost_basis requires comparable_cost")
	}
	validateNonNegativeFloat(prefix+".provider_reported_cost", metrics.ProviderReportedCost, problems)
	if metrics.InputTokens != nil && metrics.CachedInputTokens != nil && *metrics.CachedInputTokens > *metrics.InputTokens {
		*problems = append(*problems, prefix+" cached input exceeds input tokens")
	}
	if metrics.InputTokens != nil && metrics.CachedInputTokens != nil && metrics.CacheWriteInputTokens != nil &&
		*metrics.CachedInputTokens+*metrics.CacheWriteInputTokens > *metrics.InputTokens {
		*problems = append(*problems, prefix+" cached plus cache-write input exceeds input tokens")
	}
	if metrics.RequestCache != nil && (metrics.RequestCache.Observed < 1 || metrics.RequestCache.Hits < 0 || metrics.RequestCache.Hits > metrics.RequestCache.Observed) {
		*problems = append(*problems, prefix+".request_cache has invalid coverage")
	}
}

func validatePublicReference(prefix string, reference PublicReference, problems *[]string) {
	if strings.TrimSpace(reference.Benchmark) == "" || strings.TrimSpace(reference.Version) == "" || strings.TrimSpace(reference.Agent) == "" || strings.TrimSpace(reference.Model) == "" || strings.TrimSpace(reference.Notes) == "" || reference.AccessedAt.IsZero() {
		*problems = append(*problems, prefix+" benchmark, version, agent, model, notes, and accessed_at are required")
	}
	if parsed, err := url.Parse(reference.SourceURL); err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		*problems = append(*problems, prefix+".source_url must be HTTPS")
	}
	if reference.ComputedArtifact != "" && reference.ComputedArtifact != computedDeepSWEGPT56SolXHighReference {
		*problems = append(*problems, prefix+".computed_artifact is not registered")
	}
	if reference.Score != nil || reference.Passed != nil || reference.Total != nil || reference.CostPerTask != nil || reference.MinutesPerTask != nil || reference.TurnsPerTask != nil || reference.TokensPerTask != nil || reference.TokenWeightedCacheHit != nil || len(reference.Components) != 0 {
		*problems = append(*problems, prefix+" metrics must be computed from a registered local artifact")
	}
	validateRatio(prefix+".score", reference.Score, problems)
	validateRatio(prefix+".token_weighted_cache_hit", reference.TokenWeightedCacheHit, problems)
	validateNonNegativeFloat(prefix+".cost_per_task", reference.CostPerTask, problems)
	validateNonNegativeFloat(prefix+".minutes_per_task", reference.MinutesPerTask, problems)
	validateNonNegativeFloat(prefix+".turns_per_task", reference.TurnsPerTask, problems)
	validateNonNegativeFloat(prefix+".tokens_per_task", reference.TokensPerTask, problems)
	if (reference.Passed == nil) != (reference.Total == nil) {
		*problems = append(*problems, prefix+" passed and total must be provided together")
	} else if reference.Passed != nil && (*reference.Total < 1 || *reference.Passed < 0 || *reference.Passed > *reference.Total) {
		*problems = append(*problems, prefix+" passed/total is invalid")
	} else if reference.Score != nil && reference.Passed != nil {
		derived := float64(*reference.Passed) / float64(*reference.Total)
		if math.Abs(*reference.Score-derived) > 1e-6 {
			*problems = append(*problems, prefix+" score differs from passed/total")
		}
	}
	for index, component := range reference.Components {
		if strings.TrimSpace(component.Name) == "" {
			*problems = append(*problems, fmt.Sprintf("%s.components[%d].name is required", prefix, index))
		}
		validateRatio(fmt.Sprintf("%s.components[%d].score", prefix, index), component.Score, problems)
	}
}

func validateAblation(prefix string, ablation Ablation, problems *[]string) {
	switch ablation.Status {
	case AblationMeasured:
		if ablation.Endpoint == nil {
			*problems = append(*problems, prefix+" measured ablation requires endpoint")
		} else {
			validateEndpoint(prefix+".endpoint", *ablation.Endpoint, problems)
		}
	case AblationNotRun, AblationNA:
		if ablation.Endpoint != nil {
			*problems = append(*problems, prefix+" unmeasured ablation cannot include endpoint")
		}
	default:
		*problems = append(*problems, prefix+".status is invalid")
	}
	if strings.TrimSpace(ablation.Note) == "" {
		*problems = append(*problems, prefix+".note is required")
	}
}

func validateEndpoint(prefix string, endpoint ExperimentEndpoint, problems *[]string) {
	validateID(prefix+".experiment_id", endpoint.ExperimentID, problems)
	validateID(prefix+".agent_id", endpoint.AgentID, problems)
}

func validateID(name, value string, problems *[]string) {
	if !reportIDPattern.MatchString(value) {
		*problems = append(*problems, name+" is invalid")
	}
}

func validateConfigPath(path string) error {
	if strings.TrimSpace(path) == "" || filepath.Clean(path) != path || path == "." {
		return errors.New("must be a non-empty clean path")
	}
	return nil
}

func validateRelativePath(path string) error {
	if strings.TrimSpace(path) == "" || filepath.IsAbs(path) || filepath.Clean(path) != path || path == "." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return errors.New("must be a clean relative path")
	}
	return nil
}

func resolveConfigPath(base, path string) (string, error) {
	if err := validateConfigPath(path); err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return filepath.Abs(path)
}

func validateNonNegativeInt(name string, value *int, problems *[]string) {
	if value != nil && *value < 0 {
		*problems = append(*problems, name+" cannot be negative")
	}
}

func validateNonNegativeInt64(name string, value *int64, problems *[]string) {
	if value != nil && *value < 0 {
		*problems = append(*problems, name+" cannot be negative")
	}
}

func validateNonNegativeFloat(name string, value *float64, problems *[]string) {
	if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
		*problems = append(*problems, name+" must be finite and non-negative")
	}
}

func validateRatio(name string, value *float64, problems *[]string) {
	if value != nil && (*value < 0 || *value > 1 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
		*problems = append(*problems, name+" must be between zero and one")
	}
}

func validFailureCategory(category FailureCategory) bool {
	return slices.Contains([]FailureCategory{
		FailureImplementation, FailureIncomplete, FailureRegression, FailureValidation,
		FailureTimeout, FailureInfrastructure, FailureProtocol, FailureUnknown,
	}, category)
}

func validComparisonMetric(metric ComparisonMetric) bool {
	return slices.Contains([]ComparisonMetric{
		MetricPassRate, MetricWallTime, MetricTrialDuration, MetricLLMCallsStarted, MetricProviderRounds, MetricProviderErrors, MetricToolBearingRounds,
		MetricToolInvocations, MetricPhysicalToolOperations, MetricNativeEvents,
		MetricToolErrors, MetricInputTokens, MetricCachedInputTokens, MetricCacheWriteInputTokens, MetricUncachedInputTokens, MetricOutputTokens,
		MetricReasoningTokens, MetricTokenCacheHit, MetricRequestCacheHit,
		MetricComparableCost, MetricProviderCost,
	}, metric)
}

func decodeStrictFile(path string, destination any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
