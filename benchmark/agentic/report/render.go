package report

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
	"github.com/agent-dance/luban/i18n"
)

//go:embed report.html.tmpl
var reportTemplate string

func Render(writer io.Writer, data Data) error {
	lang, htmlLanguage, err := reportLanguage(data.Meta.Language)
	if err != nil {
		return err
	}
	text := func(key string) string {
		return i18n.Text(lang, i18n.Key(key))
	}
	format := func(key string, arguments ...any) string {
		return i18n.Format(lang, i18n.Key(key), arguments...)
	}
	templateValue, err := template.New("agentic-report").Funcs(template.FuncMap{
		"tr":                         text,
		"tf":                         format,
		"htmlLanguage":               func() string { return htmlLanguage },
		"classLabel":                 func(class ExperimentClass) string { return classLabel(lang, class) },
		"classClass":                 classClass,
		"gateLabel":                  func(name string) string { return gateLabel(lang, name) },
		"gateStatusLabel":            func(status GateStatus) string { return gateStatusLabel(lang, status) },
		"metricLabel":                func(metric ComparisonMetric) string { return metricLabel(lang, metric) },
		"metricValue":                func(metric ComparisonMetric, value *float64) string { return metricValue(lang, metric, value) },
		"relativeValue":              func(value *float64) string { return relativeValue(lang, value) },
		"intValue":                   func(value *int) string { return intValue(lang, value) },
		"int64Value":                 func(value *int64) string { return int64Value(lang, value) },
		"floatValue":                 func(value *float64) string { return floatValue(lang, value) },
		"secondsValue":               func(value *float64) string { return secondsValue(lang, value) },
		"costValue":                  func(value *float64) string { return costValue(lang, value) },
		"percentValue":               func(value *float64) string { return percentValue(lang, value) },
		"passValue":                  func(value *bool) string { return passValue(lang, value) },
		"passClass":                  passClass,
		"passCountValue":             func(value *int, runs int) string { return passCountValue(lang, value, runs) },
		"textOrMissing":              func(value string) string { return textOrMissing(lang, value) },
		"timeValue":                  func(value time.Time) string { return timeValue(lang, value) },
		"ciValue":                    func(value *ConfidenceInterval, metric ComparisonMetric) string { return ciValue(lang, value, metric) },
		"passCIValue":                func(value *ConfidenceInterval) string { return ciValue(lang, value, MetricPassRate) },
		"failureLabel":               func(category FailureCategory) string { return failureLabel(lang, category) },
		"ablationLabel":              func(status AblationStatus) string { return ablationLabel(lang, status) },
		"verdictLabel":               func(status string) string { return verdictLabel(lang, status) },
		"criterionLabel":             func(value *bool) string { return criterionLabel(lang, value) },
		"criterionClass":             criterionClass,
		"svgHeight":                  svgHeight,
		"svgY":                       svgY,
		"svgX":                       svgX,
		"svgWidth":                   svgWidth,
		"svgPhaseX":                  svgPhaseX,
		"providerCostValue":          func(metrics MetricData) string { return providerCostValue(lang, metrics) },
		"catalogCostValue":           func(metrics MetricData) string { return catalogCostValue(lang, metrics) },
		"headlineCostValue":          func(metrics MetricData) string { return headlineCostValue(lang, metrics) },
		"coverageValue":              func(observed, total int) string { return coverageValue(lang, observed, total) },
		"mulNotAvailable":            func(value float64) float64 { return value * 100 },
		"publicRateValue":            func(value harness.DeepSWERate) string { return publicRateValue(lang, value) },
		"publicPassAtKValue":         func(value *harness.DeepSWEPassAtK) string { return publicPassAtKValue(lang, value) },
		"publicFourRunValue":         func(value *harness.DeepSWEFourRunStatistics) string { return publicFourRunValue(lang, value) },
		"publicRunSamples":           func(value *harness.DeepSWEFourRunStatistics) string { return publicRunSamples(lang, value) },
		"publicFailureCounts":        func(value map[harness.DeepSWEFailureCategory]int) string { return publicFailureCounts(lang, value) },
		"publicExclusionSensitivity": func(value harness.DeepSWEExclusionSensitivity) string { return publicExclusionSensitivity(lang, value) },
		"publicIntAggregate":         func(value harness.DeepSWEIntAggregate) string { return publicIntAggregateValue(lang, value) },
		"publicFloatAggregate":       func(value harness.DeepSWEFloatAggregate) string { return publicFloatAggregateValue(lang, value, "") },
		"publicCostAggregate": func(value harness.DeepSWEFloatAggregate) string {
			if value.Sum == nil || value.Mean == nil {
				return i18n.Text(lang, i18n.KeyAgenticReportCostProviderNotAvailable)
			}
			return publicFloatAggregateValue(lang, value, "$")
		},
		"boolPassValue": func(value bool) string { return passValue(lang, &value) },
		"derefFloat":    derefFloat,
		"sumFloat":      sumFloat,
		"int64Float":    int64Float,
	}).Parse(reportTemplate)
	if err != nil {
		return fmt.Errorf("parse report template: %w", err)
	}
	if err := templateValue.Execute(writer, data); err != nil {
		return fmt.Errorf("render report template: %w", err)
	}
	return nil
}

func reportLanguage(code string) (i18n.Language, string, error) {
	switch code {
	case "en":
		return i18n.LangEN, "en", nil
	case "zh-CN":
		return i18n.LangZH, "zh-CN", nil
	case "de":
		return i18n.LangDE, "de", nil
	case "ja":
		return i18n.LangJA, "ja", nil
	case "ko":
		return i18n.LangKO, "ko", nil
	case "ru":
		return i18n.LangRU, "ru", nil
	default:
		return i18n.LangEN, "", fmt.Errorf("unsupported report language %q", code)
	}
}

func Generate(inputPath string, writer io.Writer) error {
	data, err := Compile(inputPath)
	if err != nil {
		return err
	}
	return Render(writer, data)
}

func GenerateFile(inputPath, outputPath string) error {
	var input Input
	if err := decodeStrictFile(inputPath, &input); err != nil {
		return err
	}
	inputBase := filepath.Dir(inputPath)
	outputAbsolute, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	canonicalOutput, err := canonicalFuturePath(outputAbsolute)
	if err != nil {
		return err
	}
	for _, source := range input.ArtifactSources {
		root, resolveErr := resolveConfigPath(inputBase, source.Root)
		if resolveErr != nil {
			return resolveErr
		}
		canonicalRoot, resolveErr := filepath.EvalSymlinks(root)
		if resolveErr != nil {
			return resolveErr
		}
		canonicalRoot, resolveErr = filepath.Abs(canonicalRoot)
		if resolveErr != nil {
			return resolveErr
		}
		if pathContains(canonicalRoot, canonicalOutput) {
			return fmt.Errorf("report output must not mutate sealed artifact root %s", root)
		}
	}
	data, err := Compile(inputPath)
	if err != nil {
		return err
	}
	parent := filepath.Dir(outputAbsolute)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".agentic-report-*.html")
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
	if err := Render(temporary, data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, outputAbsolute); err != nil {
		return err
	}
	committed = true
	return nil
}

// canonicalFuturePath resolves every existing path component, including a
// symlinked parent, while retaining a not-yet-created suffix. It closes the
// containment gap where an apparently external output path aliases a sealed
// artifact directory.
func canonicalFuturePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := absolute
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return filepath.Abs(resolved)
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", resolveErr
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func pathContains(root, candidate string) bool {
	return candidate == root || strings.HasPrefix(candidate, root+string(filepath.Separator))
}

func classLabel(lang i18n.Language, class ExperimentClass) string {
	switch class {
	case ClassFormal:
		return i18n.Text(lang, i18n.KeyAgenticReportClassFormalLabel)
	case ClassPilot:
		return i18n.Text(lang, i18n.KeyAgenticReportClassPilotLabel)
	case ClassDiagnosticCanary:
		return i18n.Text(lang, i18n.KeyAgenticReportClassDiagnosticLabel)
	default:
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
}

func classClass(class ExperimentClass) string {
	switch class {
	case ClassFormal:
		return "formal"
	case ClassPilot:
		return "pilot"
	case ClassDiagnosticCanary:
		return "diagnostic"
	default:
		return "unknown"
	}
}

func gateLabel(lang i18n.Language, name string) string {
	labels := map[string]i18n.Key{
		"classification":          i18n.KeyAgenticReportGateClassification,
		"formal_score":            i18n.KeyAgenticReportGateFormalScore,
		"artifact_integrity":      i18n.KeyAgenticReportGateArtifactIntegrity,
		"scorecard_recomputed":    i18n.KeyAgenticReportGateScorecardRecomputed,
		"paired_schedule":         i18n.KeyAgenticReportGatePairedSchedule,
		"model_contract":          i18n.KeyAgenticReportGateModelContract,
		"single_agent_fairness":   i18n.KeyAgenticReportGateSingleAgentFairness,
		"network_isolation":       i18n.KeyAgenticReportGateNetworkIsolation,
		"oracle":                  i18n.KeyAgenticReportGateOracle,
		"complete_spend":          i18n.KeyAgenticReportGateCompleteSpend,
		"tool_execution_coverage": i18n.KeyAgenticReportGateToolExecution,
		"controller_duration":     i18n.KeyAgenticReportGateControllerDuration,
		"exclusion_symmetry":      i18n.KeyAgenticReportGateExclusionSymmetry,
		"storage_evidence":        i18n.KeyAgenticReportGateStorageEvidence,
		"projection_integrity":    i18n.KeyAgenticReportGateProjectionIntegrity,
	}
	if key, exists := labels[name]; exists {
		return i18n.Text(lang, key)
	}
	return name
}

func gateStatusLabel(lang i18n.Language, status GateStatus) string {
	switch status {
	case GatePass:
		return i18n.Text(lang, i18n.KeyAgenticReportStatusPass)
	case GateFail:
		return i18n.Text(lang, i18n.KeyAgenticReportStatusFail)
	default:
		return i18n.Text(lang, i18n.KeyAgenticReportStatusUnknown)
	}
}

func metricLabel(lang i18n.Language, metric ComparisonMetric) string {
	keys := map[ComparisonMetric]i18n.Key{
		MetricPassRate:               i18n.KeyAgenticReportMetricPassRate,
		MetricWallTime:               i18n.KeyAgenticReportMetricWallTimeSeconds,
		MetricTrialDuration:          i18n.KeyAgenticReportMetricTrialDurationSeconds,
		MetricLLMCallsStarted:        i18n.KeyAgenticReportMetricLLMCallsStarted,
		MetricProviderRounds:         i18n.KeyAgenticReportMetricProviderRounds,
		MetricProviderErrors:         i18n.KeyAgenticReportMetricProviderErrors,
		MetricToolBearingRounds:      i18n.KeyAgenticReportMetricToolBearingRounds,
		MetricToolInvocations:        i18n.KeyAgenticReportMetricToolInvocations,
		MetricPhysicalToolOperations: i18n.KeyAgenticReportMetricPhysicalOperations,
		MetricNativeEvents:           i18n.KeyAgenticReportMetricNativeEvents,
		MetricToolErrors:             i18n.KeyAgenticReportMetricToolErrors,
		MetricInputTokens:            i18n.KeyAgenticReportMetricInputTokens,
		MetricCachedInputTokens:      i18n.KeyAgenticReportMetricCachedInputTokens,
		MetricCacheWriteInputTokens:  i18n.KeyAgenticReportMetricCacheWriteInputTokens,
		MetricUncachedInputTokens:    i18n.KeyAgenticReportMetricUncachedInputTokens,
		MetricOutputTokens:           i18n.KeyAgenticReportMetricOutputTokens,
		MetricReasoningTokens:        i18n.KeyAgenticReportMetricReasoningOutputTokens,
		MetricTokenCacheHit:          i18n.KeyAgenticReportMetricTokenWeightedCacheHit,
		MetricRequestCacheHit:        i18n.KeyAgenticReportMetricRequestCacheHit,
		MetricComparableCost:         i18n.KeyAgenticReportMetricCatalogCost,
		MetricProviderCost:           i18n.KeyAgenticReportMetricProviderReportedCost,
	}
	if key, exists := keys[metric]; exists {
		return i18n.Text(lang, key)
	}
	return string(metric)
}

func metricValue(lang i18n.Language, metric ComparisonMetric, value *float64) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	switch metric {
	case MetricPassRate, MetricTokenCacheHit, MetricRequestCacheHit:
		return fmt.Sprintf("%.2f%%", *value*100)
	case MetricWallTime, MetricTrialDuration:
		return fmt.Sprintf("%.3f s", *value)
	case MetricComparableCost, MetricProviderCost:
		return fmt.Sprintf("$%.6f", *value)
	case MetricLLMCallsStarted, MetricProviderRounds, MetricProviderErrors, MetricToolBearingRounds,
		MetricToolInvocations, MetricPhysicalToolOperations, MetricNativeEvents, MetricToolErrors,
		MetricInputTokens, MetricCachedInputTokens, MetricCacheWriteInputTokens, MetricUncachedInputTokens, MetricOutputTokens, MetricReasoningTokens:
		return formatNumber(*value)
	default:
		return strconv.FormatFloat(*value, 'f', 3, 64)
	}
}

func relativeValue(lang i18n.Language, value *float64) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	return fmt.Sprintf("%+.2f%%", *value*100)
}

func intValue(lang i18n.Language, value *int) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	return formatNumber(float64(*value))
}

func int64Value(lang i18n.Language, value *int64) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	return formatNumber(float64(*value))
}

func floatValue(lang i18n.Language, value *float64) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	return strconv.FormatFloat(*value, 'f', 3, 64)
}

func secondsValue(lang i18n.Language, value *float64) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	return fmt.Sprintf("%.3f s", *value)
}

func costValue(lang i18n.Language, value *float64) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	return fmt.Sprintf("$%.6f", *value)
}

func percentValue(lang i18n.Language, value *float64) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	return fmt.Sprintf("%.2f%%", *value*100)
}

func publicRateValue(lang i18n.Language, value harness.DeepSWERate) string {
	rate := i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	if value.Rate != nil {
		rate = fmt.Sprintf("%.2f%%", *value.Rate*100)
	}
	return fmt.Sprintf("%s / %s · %s", formatNumber(value.Numerator), formatNumber(float64(value.Denominator)), rate)
}

func publicPassAtKValue(lang i18n.Language, value *harness.DeepSWEPassAtK) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	rate := i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	if value.Rate != nil {
		rate = fmt.Sprintf("%.2f%%", *value.Rate*100)
	}
	return fmt.Sprintf("passed_tasks=%s / total_tasks=%s · universe_tasks=%s · %s · %s", formatNumber(float64(value.PassedTasks)), formatNumber(float64(value.TotalTasks)), formatNumber(float64(value.UniverseTasks)), rate, value.Method)
}

func publicFourRunValue(lang i18n.Language, value *harness.DeepSWEFourRunStatistics) string {
	if value == nil || value.ConfidenceCenter == nil || value.ConfidenceLower == nil || value.ConfidenceUpper == nil || value.ConfidenceHalfWidth == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	runMean, deviation := i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing), i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	if value.RunMean != nil {
		runMean = fmt.Sprintf("%.4f%%", *value.RunMean*100)
	}
	if value.SampleStandardDeviation != nil {
		deviation = fmt.Sprintf("%.6f", *value.SampleStandardDeviation)
	}
	return fmt.Sprintf("run_mean=%s · sample_sd=%s · z=%.6f · center=%.4f%% · CI=[%.4f%%, %.4f%%] · ±%.4fpp",
		runMean, deviation, value.Z, *value.ConfidenceCenter*100, *value.ConfidenceLower*100, *value.ConfidenceUpper*100, *value.ConfidenceHalfWidth*100)
}

func publicRunSamples(lang i18n.Language, value *harness.DeepSWEFourRunStatistics) string {
	if value == nil || len(value.Runs) == 0 {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	parts := make([]string, 0, len(value.Runs))
	for _, sample := range value.Runs {
		rate := i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
		if sample.Rate != nil {
			rate = fmt.Sprintf("%.4f%%", *sample.Rate*100)
		}
		parts = append(parts, fmt.Sprintf("r%d=%d/%d (%s)", sample.Run, sample.Passed, sample.Scored, rate))
	}
	return strings.Join(parts, " · ")
}

func publicFailureCounts(lang i18n.Language, values map[harness.DeepSWEFailureCategory]int) string {
	parts := make([]string, 0, len(values))
	for category, count := range values {
		if count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", category, count))
		}
	}
	slices.Sort(parts)
	if len(parts) == 0 {
		return i18n.Text(lang, i18n.KeyAgenticReportNotApplicable)
	}
	return strings.Join(parts, " · ")
}

func publicExclusionSensitivity(lang i18n.Language, sensitivity harness.DeepSWEExclusionSensitivity) string {
	if sensitivity.RawAttempts <= 0 || sensitivity.ScoredAttempts < 0 || sensitivity.ExcludedAttempts < 0 || sensitivity.ScoredAttempts+sensitivity.ExcludedAttempts != sensitivity.RawAttempts {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	formatScenario := func(value harness.DeepSWEQualityScenario) string {
		return fmt.Sprintf("pooled=%s · macro=%s · pass@4=%s",
			publicRateValue(lang, value.LivePooled), publicRateValue(lang, value.TaskMacro), publicPassAtKValue(lang, value.PassAt4))
	}
	return fmt.Sprintf("rate=%s · worst_all_fail{%s} · best_all_pass{%s}",
		percentValue(lang, sensitivity.ExclusionRate),
		formatScenario(sensitivity.WorstCaseAllExcludedAsFailure),
		formatScenario(sensitivity.BestCaseAllExcludedAsPass))
}

func publicIntAggregateValue(lang i18n.Language, value harness.DeepSWEIntAggregate) string {
	coverage := coverageValue(lang, value.Observed, value.Total)
	if value.Sum == nil || value.Mean == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing) + " · " + coverage
	}
	return fmt.Sprintf("Σ=%s · μ=%.3f · %s", formatNumber(float64(*value.Sum)), *value.Mean, coverage)
}

func publicFloatAggregateValue(lang i18n.Language, value harness.DeepSWEFloatAggregate, prefix string) string {
	coverage := coverageValue(lang, value.Observed, value.Total)
	if value.Sum == nil || value.Mean == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing) + " · " + coverage
	}
	return fmt.Sprintf("Σ=%s%.6f · μ=%s%.6f · %s", prefix, *value.Sum, prefix, *value.Mean, coverage)
}

func passValue(lang i18n.Language, value *bool) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	if *value {
		return "PASS"
	}
	return "FAIL"
}

func passClass(value *bool) string {
	if value == nil {
		return "unknown"
	}
	if *value {
		return "pass"
	}
	return "fail"
}

func passCountValue(lang i18n.Language, value *int, runs int) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	return fmt.Sprintf("%s / %s", formatNumber(float64(*value)), formatNumber(float64(runs)))
}

func textOrMissing(lang i18n.Language, value string) string {
	if strings.TrimSpace(value) == "" {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	return value
}

func timeValue(lang i18n.Language, value time.Time) string {
	if value.IsZero() {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	return value.UTC().Format(time.RFC3339)
}

func ciValue(lang i18n.Language, value *ConfidenceInterval, metric ComparisonMetric) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
	}
	return fmt.Sprintf("[%s, %s] · tasks=%d · pairs=%d · bootstrap=%d · seed=%d",
		metricValue(lang, metric, &value.Lower), metricValue(lang, metric, &value.Upper), value.Tasks, value.Pairs, value.Resamples, value.Seed)
}

func failureLabel(lang i18n.Language, category FailureCategory) string {
	keys := map[FailureCategory]i18n.Key{
		FailureImplementation: i18n.KeyAgenticReportFailureImplementation,
		FailureIncomplete:     i18n.KeyAgenticReportFailureIncomplete,
		FailureRegression:     i18n.KeyAgenticReportFailureRegression,
		FailureValidation:     i18n.KeyAgenticReportFailureValidation,
		FailureTimeout:        i18n.KeyAgenticReportFailureTimeout,
		FailureInfrastructure: i18n.KeyAgenticReportFailureInfrastructure,
		FailureProtocol:       i18n.KeyAgenticReportFailureProtocol,
		FailureUnknown:        i18n.KeyAgenticReportFailureUnknown,
	}
	if key, exists := keys[category]; exists {
		return i18n.Text(lang, key)
	}
	return string(category)
}

func ablationLabel(lang i18n.Language, status AblationStatus) string {
	switch status {
	case AblationMeasured:
		return i18n.Text(lang, i18n.KeyAgenticReportAblationMeasured)
	case AblationNotRun:
		return i18n.Text(lang, i18n.KeyAgenticReportAblationNotRun)
	default:
		return i18n.Text(lang, i18n.KeyAgenticReportNotApplicable)
	}
}

func verdictLabel(lang i18n.Language, status string) string {
	switch status {
	case "verified_exceeds":
		return i18n.Text(lang, i18n.KeyAgenticReportVerdictExceeds)
	case "not_exceeds":
		return i18n.Text(lang, i18n.KeyAgenticReportVerdictNotExceeds)
	default:
		return i18n.Text(lang, i18n.KeyAgenticReportVerdictInsufficient)
	}
}

func criterionLabel(lang i18n.Language, value *bool) string {
	if value == nil {
		return i18n.Text(lang, i18n.KeyAgenticReportStatusUnknown)
	}
	if *value {
		return i18n.Text(lang, i18n.KeyAgenticReportStatusSatisfied)
	}
	return i18n.Text(lang, i18n.KeyAgenticReportStatusNotSatisfied)
}

func criterionClass(value *bool) string {
	if value == nil {
		return "unknown"
	}
	if *value {
		return "pass"
	}
	return "fail"
}

func providerCostValue(lang i18n.Language, metrics MetricData) string {
	if metrics.ProviderReportedCost != nil {
		return costValue(lang, metrics.ProviderReportedCost)
	}
	if metrics.ProviderCostPartial != nil {
		return fmt.Sprintf("%s · %s · coverage=%d/%d all_transport", i18n.Text(lang, i18n.KeyAgenticReportCoveragePartial), costValue(lang, metrics.ProviderCostPartial), metrics.ProviderCostObserved, metrics.ProviderCostTotal)
	}
	return i18n.Text(lang, i18n.KeyAgenticReportCostProviderNotAvailable)
}

func headlineCostValue(lang i18n.Language, metrics MetricData) string {
	if (metrics.ComparableCostBasis == comparableCostBasisFrozen || metrics.ComparableCostBasis == ComparableCostBasisDevelopmentNonBilling) && metrics.ComparableCost != nil {
		return costValue(lang, metrics.ComparableCost)
	}
	return i18n.Text(lang, i18n.KeyAgenticReportStatusUnknown)
}

func catalogCostValue(lang i18n.Language, metrics MetricData) string {
	if metrics.CatalogCost != nil {
		return costValue(lang, metrics.CatalogCost)
	}
	lowerBound := metrics.KnownCatalogCostLowerBound
	if lowerBound == nil {
		lowerBound = metrics.CatalogCostPartial
	}
	if lowerBound != nil {
		unknown := 0
		if metrics.UnknownCostAttempts != nil {
			unknown = *metrics.UnknownCostAttempts
		}
		result := i18n.Format(lang, i18n.KeyAgenticReportCostKnownLowerBound, costValue(lang, lowerBound), metrics.CostReceiptObserved, metrics.CostReceiptTotal, unknown, metrics.AllExecutedCacheWriteObserved, metrics.AllExecutedCacheWriteTotal)
		if metrics.CostIdentityUnknownAttempts != nil && *metrics.CostIdentityUnknownAttempts > 0 {
			result += fmt.Sprintf(" · cost_identity_unknown_attempts=%d", *metrics.CostIdentityUnknownAttempts)
		}
		return result
	}
	return i18n.Text(lang, i18n.KeyAgenticReportCoverageMissing)
}

func coverageValue(lang i18n.Language, observed, total int) string {
	if total == 0 {
		return i18n.Text(lang, i18n.KeyAgenticReportNotApplicable)
	}
	return fmt.Sprintf("%d/%d", observed, total)
}

func derefFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func sumFloat(left, right *float64) float64 { return derefFloat(left) + derefFloat(right) }

func int64Float(value *int64) float64 {
	if value == nil {
		return 0
	}
	return float64(*value)
}

func svgHeight(rounds []RoundData) int { return 42 + len(rounds)*26 }

func svgY(index int) int { return 32 + index*26 }

func svgX(run RunData, round RoundData) float64 {
	span := run.FinishedAt.Sub(run.StartedAt).Seconds() * 1000
	if span <= 0 {
		return 112
	}
	offset := round.StartedAt.Sub(run.StartedAt).Seconds() * 1000
	return 112 + clamp(offset/span, 0, 1)*846
}

func svgWidth(run RunData, milliseconds float64) float64 {
	span := run.FinishedAt.Sub(run.StartedAt).Seconds() * 1000
	if span <= 0 {
		return 0
	}
	return math.Max(0.7, clamp(milliseconds/span, 0, 1)*846)
}

func svgPhaseX(run RunData, round RoundData, beforeMS float64) float64 {
	return svgX(run, round) + svgWidth(run, beforeMS)
}

func clamp(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func formatNumber(value float64) string {
	rounded := math.Round(value)
	if math.Abs(value-rounded) > 1e-9 {
		return strconv.FormatFloat(value, 'f', 3, 64)
	}
	raw := strconv.FormatInt(int64(rounded), 10)
	start := 0
	if strings.HasPrefix(raw, "-") {
		start = 1
	}
	for index := len(raw) - 3; index > start; index -= 3 {
		raw = raw[:index] + "," + raw[index:]
	}
	return raw
}
