package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
	"github.com/agent-dance/luban/benchmark/agentic/pilot"
	"github.com/agent-dance/luban/benchmark/agentic/report"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/i18n"
)

const comparableCostBasis = report.ComparableCostBasisDevelopmentNonBilling

const (
	maxDevelopmentLedgerBytes   = int64(64 << 20)
	maxDevelopmentReceiptBytes  = int64(64 << 20)
	maxDevelopmentEvidenceBytes = int64(1 << 30)
)

// frozenDevelopmentPricing is the exact GPT-5.6 rate card frozen by the
// checked-in DeepSWE manifests. The development report deliberately owns a
// copy instead of trusting a mutable price projection in a pilot receipt.
var frozenDevelopmentPricing = harness.PricingCatalog{
	Currency:    "USD",
	UnitTokens:  1_000_000,
	EffectiveAt: time.Date(2026, time.July, 9, 0, 0, 0, 0, time.UTC),
	ObservedAt:  time.Date(2026, time.July, 26, 0, 0, 0, 0, time.UTC),
	SourceURL:   "https://developers.openai.com/api/docs/models/gpt-5.6-sol",
	Rates: []harness.PricingRate{{
		Provider: "openai", Model: "gpt-5.6-sol", Input: 5, CachedInput: .5, Output: 30,
		CacheWriteInputMultiplier: 1.25,
		RequestTiers: []harness.PricingTier{{
			Name: "long-context", ThresholdInputTokens: 272_000,
			InputMultiplier: 2, CachedInputMultiplier: 2, OutputMultiplier: 1.5,
		}},
	}},
}

type options struct {
	ledgerPath string
	inputPath  string
	htmlPath   string
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
	if err := generate(parsed, language); err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyAgenticReportCLIError, err))
		return 1
	}
	_, _ = fmt.Fprintln(stdout, i18n.Format(language, i18n.KeyAgenticReportCLISuccess, parsed.htmlPath))
	return 0
}

func parseOptions(language i18n.Language, arguments []string) (options, bool, error) {
	var result options
	set := flag.NewFlagSet("devreport", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&result.ledgerPath, "ledger", "", i18n.Text(language, i18n.KeyAgenticReportCLIFlagInput))
	set.StringVar(&result.inputPath, "output", "", i18n.Text(language, i18n.KeyAgenticReportCLIFlagInput))
	set.StringVar(&result.htmlPath, "html", "", i18n.Text(language, i18n.KeyAgenticReportCLIFlagOutput))
	if err := set.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, true, nil
		}
		return options{}, false, err
	}
	if set.NArg() != 0 || result.ledgerPath == "" || result.inputPath == "" || result.htmlPath == "" {
		return options{}, false, i18n.NewError(i18n.KeyAgenticReportCLIRequired)
	}
	paths := []*string{&result.ledgerPath, &result.inputPath, &result.htmlPath}
	for _, path := range paths {
		absolute, err := filepath.Abs(*path)
		if err != nil {
			return options{}, false, err
		}
		*path = absolute
	}
	return result, false, nil
}

func writeUsage(writer io.Writer, language i18n.Language) {
	_, _ = fmt.Fprintf(writer, "--ledger PATH\t%s\n--output PATH\t%s\n--html PATH\t%s\n",
		i18n.Text(language, i18n.KeyAgenticReportCLIFlagInput),
		i18n.Text(language, i18n.KeyAgenticReportCLIFlagInput),
		i18n.Text(language, i18n.KeyAgenticReportCLIFlagOutput))
}

func generate(options options, language i18n.Language) error {
	ledger, err := loadLedger(options.ledgerPath)
	if err != nil {
		return err
	}
	input, optimization := reportInput(ledger, options, language)
	optimizationPath := filepath.Join(filepath.Dir(options.inputPath), ".development-report-optimization-ledger.json")
	if err := harness.WriteJSONAtomic(optimizationPath, optimization, 0o644); err != nil {
		return err
	}
	optimizationSHA, err := harness.HashFile(optimizationPath)
	if err != nil {
		return err
	}
	input.OptimizationLedger = report.FileReference{Path: filepath.Base(optimizationPath), SHA256: optimizationSHA}
	if err := harness.WriteJSONAtomic(options.inputPath, input, 0o644); err != nil {
		return err
	}
	return report.GenerateFile(options.inputPath, options.htmlPath)
}

func loadLedger(path string) (pilot.Ledger, error) {
	raw, err := readRegularFile(path, maxDevelopmentLedgerBytes)
	if err != nil {
		return pilot.Ledger{}, err
	}
	var ledger pilot.Ledger
	if err := decodeStrictJSON(raw, &ledger, "ledger"); err != nil {
		return pilot.Ledger{}, err
	}
	if ledger.SchemaVersion != pilot.LedgerSchemaVersion || ledger.FormalCompatible || ledger.Status != "complete" ||
		ledger.StartedAt.IsZero() || ledger.UpdatedAt.IsZero() || ledger.CompletedAt == nil || ledger.CompletedAt.IsZero() ||
		ledger.CompletedAt.Before(ledger.StartedAt) || ledger.CompletedAt.After(ledger.UpdatedAt) || len(ledger.Runs) != len(pilot.ExactTaskIDs)*2 {
		return pilot.Ledger{}, devError("ledger_identity_or_completion_invalid")
	}
	seen := make(map[string]map[string]struct{}, len(pilot.ExactTaskIDs))
	for key, run := range ledger.Runs {
		if run.FormalCompatible || run.State != "sealed" || run.AttemptNumber != 1 || run.ReservedAt.IsZero() || run.SealedAt.IsZero() ||
			run.ReservedAt.Before(ledger.StartedAt) || run.SealedAt.Before(run.ReservedAt) || run.SealedAt.After(*ledger.CompletedAt) ||
			run.ReceiptPath == "" || run.ReceiptSHA256 == "" || run.Execution == nil || run.Verification == nil || run.Metrics == nil {
			return pilot.Ledger{}, devError("run_not_sealed:" + key)
		}
		if harness.RunKey(run.Entry) != key || run.Entry.Repetition != 0 || run.Entry.AgentID != "codex" && run.Entry.AgentID != "luban" || !slices.Contains(pilot.ExactTaskIDs, run.Entry.TaskID) {
			return pilot.Ledger{}, devError("run_matrix_invalid:" + key)
		}
		if err := validateFrozenDevelopmentModel(run.Entry.AgentID, run.Model); err != nil {
			return pilot.Ledger{}, fmt.Errorf("%w:%s", err, key)
		}
		if err := validateAttemptReceipt(key, run, ledger); err != nil {
			return pilot.Ledger{}, err
		}
		if seen[run.Entry.TaskID] == nil {
			seen[run.Entry.TaskID] = map[string]struct{}{}
		}
		if _, duplicate := seen[run.Entry.TaskID][run.Entry.AgentID]; duplicate {
			return pilot.Ledger{}, devError("run_matrix_duplicate:" + key)
		}
		seen[run.Entry.TaskID][run.Entry.AgentID] = struct{}{}
	}
	for _, taskID := range pilot.ExactTaskIDs {
		if len(seen[taskID]) != 2 {
			return pilot.Ledger{}, devError("run_pair_incomplete:" + taskID)
		}
	}
	return ledger, nil
}

func validateAttemptReceipt(key string, run pilot.RunRecord, ledger pilot.Ledger) error {
	expectedReceiptPath := filepath.Join(run.ArtifactDir, pilot.AttemptReceiptName)
	if filepath.Clean(run.ReceiptPath) != filepath.Clean(expectedReceiptPath) {
		return devError("run_receipt_path_invalid:" + key)
	}
	if err := validateContainedPath(run.ArtifactDir, run.ReceiptPath); err != nil {
		return fmt.Errorf("%w:%s", err, key)
	}
	raw, err := readRegularFile(run.ReceiptPath, maxDevelopmentReceiptBytes)
	if err != nil {
		return fmt.Errorf("devreport:run_receipt_file_invalid:%s: %w", key, err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(raw))
	if digest != run.ReceiptSHA256 {
		return devError("run_receipt_digest_invalid:" + key)
	}
	var receipt pilot.AttemptReceipt
	if err := decodeStrictJSON(raw, &receipt, "attempt_receipt"); err != nil {
		return fmt.Errorf("%w:%s", err, key)
	}
	if receipt.SchemaVersion != pilot.AttemptReceiptSchemaVersion || receipt.FormalCompatible ||
		receipt.ManifestSHA256 != ledger.ManifestSHA256 || receipt.PlanSHA256 != ledger.PlanSHA256 ||
		receipt.RunKey != key || harness.RunKey(receipt.Entry) != key || receipt.Entry != run.Entry ||
		!reflect.DeepEqual(receipt.Model, run.Model) || receipt.SealedAt.IsZero() || !receipt.SealedAt.Equal(run.SealedAt) {
		return devError("run_receipt_identity_invalid:" + key)
	}
	if !reflect.DeepEqual(receipt.Execution, *run.Execution) ||
		!reflect.DeepEqual(receipt.Verification, *run.Verification) ||
		!reflect.DeepEqual(receipt.Metrics, *run.Metrics) {
		return devError("run_receipt_ledger_mismatch:" + key)
	}
	if receipt.Execution.Verification == nil || !reflect.DeepEqual(*receipt.Execution.Verification, receipt.Verification) {
		return devError("run_receipt_verification_mismatch:" + key)
	}
	if err := validateDevelopmentVerification(receipt.Verification); err != nil {
		return fmt.Errorf("%w:%s", err, key)
	}
	if run.NormalizedEvidencePath == "" || filepath.Clean(run.NormalizedEvidencePath) != filepath.Clean(receipt.Execution.EvidencePath) {
		return devError("run_evidence_path_mismatch:" + key)
	}
	if err := validateExecutionTiming(receipt.Execution, receipt.SealedAt); err != nil {
		return fmt.Errorf("%w:%s", err, key)
	}
	if err := validateContainedPath(run.ArtifactDir, receipt.Execution.EvidencePath); err != nil {
		return fmt.Errorf("%w:%s", err, key)
	}
	rounds, err := readStrictEvidence(receipt.Execution.EvidencePath)
	if err != nil {
		return fmt.Errorf("devreport:run_evidence_invalid:%s: %w", key, err)
	}
	if err := validateExecutionEvidenceBinding(receipt.Execution, rounds); err != nil {
		return fmt.Errorf("%w:%s", err, key)
	}
	metrics, err := harness.ValidateAndAggregateEvidence(rounds, receipt.Model, frozenDevelopmentPricing)
	if err != nil {
		return fmt.Errorf("devreport:run_evidence_aggregation_invalid:%s: %w", key, err)
	}
	if !reflect.DeepEqual(metrics, receipt.Metrics) {
		return devError("run_evidence_metrics_mismatch:" + key)
	}
	return nil
}

func validateDevelopmentVerification(verification harness.VerificationResult) error {
	if !verification.ProtocolValid || math.IsNaN(verification.Reward) || math.IsInf(verification.Reward, 0) ||
		(verification.Reward != 0 && verification.Reward != 1) {
		return devError("run_verification_invalid")
	}
	if verification.RawReward != nil && (math.IsNaN(*verification.RawReward) || math.IsInf(*verification.RawReward, 0) ||
		(*verification.RawReward != 0 && *verification.RawReward != 1)) {
		return devError("run_verification_invalid")
	}
	return nil
}

func validateFrozenDevelopmentModel(agentID string, model harness.ModelRequestSpec) error {
	wantEncoding := harness.ServiceTierEncodingExplicitDefault
	if agentID == "codex" {
		wantEncoding = harness.ServiceTierEncodingClientCanonical
	}
	if model.Provider != "openai" || model.Model != "gpt-5.6-sol" || model.ReasoningEffort != "xhigh" ||
		model.ServiceTier != harness.FormalServiceTier || model.ServiceTierRequestEncoding != wantEncoding ||
		model.TransportRequirement != harness.TransportRequirementHTTPInference {
		return devError("run_model_not_frozen")
	}
	return nil
}

func validateExecutionTiming(execution harness.AgentExecution, sealedAt time.Time) error {
	if execution.StartedAt.IsZero() || execution.FinishedAt.IsZero() || execution.FinishedAt.Before(execution.StartedAt) ||
		execution.TrialStartedAt.IsZero() || execution.TrialFinishedAt.IsZero() || execution.TrialFinishedAt.Before(execution.TrialStartedAt) ||
		execution.StartedAt.Before(execution.TrialStartedAt) || execution.FinishedAt.After(execution.TrialFinishedAt) ||
		sealedAt.Before(execution.TrialFinishedAt) {
		return devError("run_execution_timing_invalid")
	}
	return nil
}

func validateExecutionEvidenceBinding(execution harness.AgentExecution, rounds []harness.ProviderRoundEvidence) error {
	if execution.Lifecycle.ProviderAttemptState != "provider_attempt_sealed" || execution.Lifecycle.ProviderAttemptCount == 0 ||
		execution.Lifecycle.RunIdentity == "" || execution.Lifecycle.RunIdentity != execution.EvidenceRunIdentity || len(rounds) == 0 {
		return devError("run_execution_not_sealed")
	}
	seal := execution.ProviderEvidence
	if seal.StartedAttemptCount != execution.Lifecycle.ProviderAttemptCount || seal.PersistedAttemptCount != seal.StartedAttemptCount ||
		seal.RecordCount != seal.PersistedAttemptCount || seal.RecordCount != uint64(len(rounds)) ||
		seal.LastEvidenceHash == "" || seal.LastEvidenceHash != rounds[len(rounds)-1].EvidenceHash ||
		execution.EvidenceRunIdentity != rounds[0].RunIdentity {
		return devError("run_execution_evidence_binding_invalid")
	}
	return nil
}

func readStrictEvidence(path string) ([]harness.ProviderRoundEvidence, error) {
	raw, err := readRegularFile(path, maxDevelopmentEvidenceBytes)
	if err != nil {
		return nil, err
	}
	var rounds []harness.ProviderRoundEvidence
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		trimmed := bytes.TrimSpace(scanner.Bytes())
		if len(trimmed) == 0 {
			continue
		}
		var round harness.ProviderRoundEvidence
		if err := decodeStrictJSON(trimmed, &round, "provider_evidence"); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		rounds = append(rounds, round)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rounds, nil
}

func readRegularFile(path string, maximumBytes int64) ([]byte, error) {
	if path == "" || maximumBytes < 1 {
		return nil, devError("regular_file_path_invalid")
	}
	lstat, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if lstat.Mode()&os.ModeSymlink != 0 || !lstat.Mode().IsRegular() || lstat.Size() < 0 || lstat.Size() > maximumBytes {
		return nil, devError("regular_file_required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || !os.SameFile(lstat, stat) || stat.Size() > maximumBytes {
		return nil, devError("regular_file_changed")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximumBytes {
		return nil, devError("regular_file_too_large")
	}
	return raw, nil
}

func validateContainedPath(root, path string) error {
	if !filepath.IsAbs(root) || !filepath.IsAbs(path) || filepath.Clean(root) != root || filepath.Clean(path) != path {
		return devError("artifact_path_not_absolute_clean")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return devError("artifact_directory_invalid")
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return devError("artifact_path_escape")
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." || component == ".." {
			return devError("artifact_path_component_invalid")
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return devError("artifact_path_symlink")
		}
	}
	return nil
}

func decodeStrictJSON(raw []byte, target any, kind string) error {
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return fmt.Errorf("devreport:%s_duplicate_key: %w", kind, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("devreport:%s_decode_invalid: %w", kind, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return devError(kind + "_trailing_json")
	}
	return nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var scan func() error
	scan = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return devError("json_object_key_invalid")
				}
				if _, duplicate := seen[key]; duplicate {
					return devError("json_duplicate_object_key")
				}
				seen[key] = struct{}{}
				if err := scan(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return devError("json_object_invalid")
			}
		case '[':
			for decoder.More() {
				if err := scan(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return devError("json_array_invalid")
			}
		default:
			return devError("json_delimiter_invalid")
		}
		return nil
	}
	return scan()
}

func reportInput(ledger pilot.Ledger, options options, language i18n.Language) (report.Input, report.OptimizationLedger) {
	runs := make([]report.DiagnosticRun, 0, len(ledger.Runs))
	failures := make([]report.FailureAnnotation, 0, len(ledger.Runs))
	keys := make([]string, 0, len(ledger.Runs))
	for key := range ledger.Runs {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		run := ledger.Runs[key]
		runs = append(runs, diagnosticRun(run))
		if run.Verification != nil && (!run.Verification.ProtocolValid || run.Verification.Reward != 1) {
			failures = append(failures, report.FailureAnnotation{
				ExperimentID: "development-pilot-5", TaskID: run.Entry.TaskID, AgentID: run.Entry.AgentID,
				Repetition: run.Entry.Repetition, Category: report.FailureUnknown,
				Summary: i18n.Text(language, i18n.KeyAgenticReportStatusFail), Evidence: []string{run.ReceiptPath},
			})
		}
	}
	asOf := ledger.UpdatedAt
	if ledger.CompletedAt != nil {
		asOf = *ledger.CompletedAt
	}
	watermark := i18n.Text(language, i18n.KeyAgenticReportDevelopmentWatermark)
	input := report.Input{
		SchemaVersion: report.InputSchemaVersion,
		Report: report.ReportMeta{
			Title: i18n.Text(language, i18n.KeyAgenticReportTitle), Subtitle: watermark,
			Benchmark: "DeepSWE", BenchmarkVersion: "1.1", BenchmarkContractID: report.BenchmarkContractDeepSWEV11Pilot5,
			Language: reportLanguageCode(language), BaselineAgentID: "codex", ContenderAgentID: "luban", AsOf: asOf.UTC(),
		},
		Statistics: report.StatisticsSpec{
			ConfidenceLevel: report.ReportConfidenceLevel, Method: report.ReportStatisticsMethod,
			Resamples: report.ReportStatisticsResamples, Seed: report.ReportStatisticsSeed,
		},
		DiagnosticExperiments: []report.DiagnosticExperiment{{
			ID: "development-pilot-5", Label: i18n.Text(language, i18n.KeyAgenticReportClassPilotLabel),
			Class: report.ClassDiagnosticCanary, Description: i18n.Text(language, i18n.KeyAgenticReportClassPilotDescription),
			SourceNote: watermark, Runs: runs,
		}},
		FailureAnnotations: failures,
		Reproduction: []report.ReproductionCommand{{
			Label: i18n.Text(language, i18n.KeyAgenticReportSectionReproduction),
			Argv:  []string{"go", "run", "./benchmark/agentic/report/cmd/devreport", "--ledger", options.ledgerPath, "--output", options.inputPath, "--html", options.htmlPath},
		}},
		Limitations: []string{
			i18n.Text(language, i18n.KeyAgenticReportLimitationIncompatibleEvidence),
			i18n.Text(language, i18n.KeyAgenticReportLimitationFrozenCost),
		},
	}
	return input, report.OptimizationLedger{SchemaVersion: report.OptimizationSchemaVersion, Entries: []report.OptimizationEntry{}}
}

func diagnosticRun(run pilot.RunRecord) report.DiagnosticRun {
	metrics := run.Metrics
	verification := run.Verification
	execution := run.Execution
	passed := verification.ProtocolValid && verification.Reward == 1
	result := report.DiagnosticRun{
		TaskID: run.Entry.TaskID, AgentID: run.Entry.AgentID, Variant: "development-pilot",
		Provider: run.Model.Provider, Model: run.Model.Model, ReasoningEffort: run.Model.ReasoningEffort,
		Repetition: run.Entry.Repetition, Passed: &passed,
	}
	result.Metrics = report.OptionalMetrics{
		WallTimeSeconds: durationSeconds(execution.FinishedAt, execution.StartedAt),
		LLMCallsStarted: intPointer(metrics.LLMCallsStarted), ProviderRounds: intPointer(metrics.ProviderRounds),
		ProviderErrors: intPointer(metrics.ProviderErrors), ToolBearingRounds: intPointer(metrics.ToolBearingRounds),
		ToolInvocations: intPointer(metrics.ToolInvocations), ProviderReportedCost: metrics.ProviderReportedCost,
	}
	result.Metrics.TrialDurationSeconds = durationSeconds(execution.TrialFinishedAt, execution.TrialStartedAt)
	if metrics.TransportAttempts > 0 && metrics.AllExecutedUsageObservations == metrics.TransportAttempts {
		result.Metrics.InputTokens = int64Pointer(metrics.AllExecutedInputTokens)
		result.Metrics.CachedInputTokens = int64Pointer(metrics.AllExecutedCachedInputTokens)
		result.Metrics.OutputTokens = int64Pointer(metrics.AllExecutedOutputTokens)
	}
	if metrics.TransportAttempts > 0 && metrics.AllExecutedUsageObservations == metrics.TransportAttempts && metrics.AllExecutedCacheWriteObservations == metrics.TransportAttempts {
		result.Metrics.CacheWriteInputTokens = int64Pointer(metrics.AllExecutedCacheWriteInputTokens)
	}
	if metrics.ProviderRequests > 0 && metrics.PhysicalToolObservations == metrics.ProviderRequests {
		result.Metrics.PhysicalToolOperations = intPointer(metrics.PhysicalToolOperations)
	}
	if metrics.ProviderRequests > 0 && metrics.ToolCriticalObservations == metrics.ProviderRequests {
		result.Metrics.ToolCriticalPathMS = int64Pointer(metrics.ToolCriticalPathMS)
	}
	if metrics.ProviderRequests > 0 && metrics.ToolTotalObservations == metrics.ProviderRequests {
		result.Metrics.ToolTotalLatencyMS = int64Pointer(metrics.ToolTotalLatencyMS)
	}
	if metrics.ProviderRequests > 0 && metrics.ToolQueueObservations == metrics.ProviderRequests {
		result.Metrics.ToolQueueMS = int64Pointer(metrics.ToolQueueMS)
	}
	if metrics.ToolInvocations > 0 && metrics.ToolErrorObservations == metrics.ToolInvocations {
		result.Metrics.ToolErrors = intPointer(metrics.ToolErrors)
	}
	if metrics.UsageReceiptTotal > 0 && metrics.ReasoningTokenObservations == metrics.UsageReceiptTotal {
		result.Metrics.ReasoningOutputTokens = int64Pointer(metrics.ReasoningOutputTokens)
	}
	if cost := developmentComparableCost(metrics); cost != nil {
		result.Metrics.ComparableCost = cost
		result.Metrics.ComparableCostBasis = comparableCostBasis
	}
	names := make([]string, 0, len(metrics.ToolCallsByName))
	for name := range metrics.ToolCallsByName {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		calls := metrics.ToolCallsByName[name]
		tool := report.OptionalToolStat{Name: name, Calls: &calls}
		if duration, ok := metrics.ToolDurationMSByName[name]; ok && metrics.ToolDurationObservations == metrics.ToolInvocations {
			tool.DurationMS = &duration
		}
		result.Tools = append(result.Tools, tool)
	}
	return result
}

// developmentComparableCost reuses the per-request catalog projection already
// sealed in UsageMetrics. That projection applies the frozen long-context tier
// before aggregation. Subtracting the observed cache-write surcharge leaves a
// symmetric input/cached-input/output estimate even when the gateway omits
// cache-write tokens; it is intentionally not a provider-billing claim.
func developmentComparableCost(metrics *harness.UsageMetrics) *float64 {
	if metrics == nil || metrics.TransportAttempts <= 0 || metrics.AllExecutedUsageObservations != metrics.TransportAttempts || metrics.CostReceiptTotal != metrics.TransportAttempts {
		return nil
	}
	cost := metrics.KnownCatalogCostLowerBound - metrics.KnownCacheWriteSurcharge
	if cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return nil
	}
	if cost == 0 && (metrics.AllExecutedInputTokens > 0 || metrics.AllExecutedOutputTokens > 0) {
		return nil
	}
	return &cost
}

func durationSeconds(finished, started time.Time) *float64 {
	if started.IsZero() || finished.IsZero() || finished.Before(started) {
		return nil
	}
	seconds := finished.Sub(started).Seconds()
	return &seconds
}

func reportLanguageCode(language i18n.Language) string {
	switch language {
	case i18n.LangZH:
		return "zh-CN"
	case i18n.LangDE:
		return "de"
	case i18n.LangJA:
		return "ja"
	case i18n.LangKO:
		return "ko"
	case i18n.LangRU:
		return "ru"
	default:
		return "en"
	}
}

func intPointer(value int) *int       { return &value }
func int64Pointer(value int64) *int64 { return &value }

func devError(code string) error { return fmt.Errorf("devreport:%s", code) }
