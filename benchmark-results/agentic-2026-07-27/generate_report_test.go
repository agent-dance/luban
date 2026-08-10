package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

type fixture struct {
	options options
	root    string
}

func TestGenerateReportRendersPartialEvidenceWithoutImputation(t *testing.T) {
	value := newFixture(t)
	writeRunPair(t, value, localTasks[0], "codex", true, 3)

	if err := generateReport(value.options, i18n.LangZH); err == nil {
		t.Fatal("generateReport() accepted partial evidence without --allow-partial")
	}
	if _, err := os.Stat(value.options.outputPath); !os.IsNotExist(err) {
		t.Fatalf("partial default created output: %v", err)
	}
	value.options.allowPartial = true
	if err := generateReport(value.options, i18n.LangZH); err != nil {
		t.Fatalf("generateReport() error = %v", err)
	}
	raw, err := os.ReadFile(value.options.outputPath)
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	for _, fragment := range []string{
		"本机5题初步非官方评估",
		"exact_POST_/responses",
		"HTTP POST /responses",
		"任务耗时",
		"1/10",
		"不适用",
		"2.45x",
		"summary.json",
		"internal/tools/search/inspect_bridge.go",
		"((I−C−W)×$5 + C×$0.5 + W×$6.25 + O×$30) / 1,000,000",
	} {
		if !strings.Contains(html, fragment) {
			t.Errorf("report is missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"<script", "<link ", " src=", "https://", "http://", "/tmp/codex"} {
		if strings.Contains(strings.ToLower(html), forbidden) {
			t.Errorf("report contains external-runtime marker %q", forbidden)
		}
	}

	data, err := compileReport(value.options, i18n.LangZH)
	if err != nil {
		t.Fatalf("compileReport() error = %v", err)
	}
	if data.Complete || data.RunSlotsObserved != 1 || data.EvaluationSlotsObserved != 1 || data.PairSlotsObserved != 1 || data.GoldObserved != localTaskCount {
		t.Fatalf("partial coverage = complete:%t run:%d eval:%d pair:%d gold:%d", data.Complete, data.RunSlotsObserved, data.EvaluationSlotsObserved, data.PairSlotsObserved, data.GoldObserved)
	}
	if data.SharedPass.Available {
		t.Fatalf("partial shared-pass comparison = %#v", data.SharedPass)
	}
	if got := data.LubanChanges[1].After; got != i18n.Text(i18n.LangZH, i18n.KeyAgenticReportNotApplicable) {
		t.Fatalf("partial Luban task-duration after = %q", got)
	}
}

func TestGenerateReportPromotesSameFixtureToComplete(t *testing.T) {
	value := newFixture(t)
	for taskIndex, taskID := range localTasks {
		writeRunPair(t, value, taskID, "codex", taskIndex < 2, 3)
		writeRunPair(t, value, taskID, "luban", taskIndex < 4, 2)
	}

	data, err := compileReport(value.options, i18n.LangEN)
	if err != nil {
		t.Fatalf("compileReport() error = %v", err)
	}
	if !data.Complete || data.RunSlotsObserved != 10 || data.EvaluationSlotsObserved != 10 || data.PairSlotsObserved != 10 {
		t.Fatalf("complete coverage = complete:%t run:%d eval:%d pair:%d", data.Complete, data.RunSlotsObserved, data.EvaluationSlotsObserved, data.PairSlotsObserved)
	}
	if data.Agents[0].Resolved != 2 || data.Agents[0].LLMCalls != 15 || data.Agents[1].Resolved != 4 || data.Agents[1].LLMCalls != 10 {
		t.Fatalf("aggregates = %#v", data.Agents)
	}
	if !data.Headline.Available || data.Headline.CodexResolved != 2 || data.Headline.LubanResolved != 4 {
		t.Fatalf("headline = %#v", data.Headline)
	}
	if !data.SharedPass.Available || data.SharedPass.TaskCount != 2 || len(data.SharedPass.Tasks) != 2 ||
		data.SharedPass.Tasks[0].ID != localTasks[0] || data.SharedPass.Tasks[1].ID != localTasks[1] {
		t.Fatalf("shared pass = %#v", data.SharedPass)
	}
	if data.SharedPass.Codex.LLMCalls != 6 || data.SharedPass.Luban.LLMCalls != 4 ||
		data.SharedPass.Codex.ElapsedSeconds != 40 || data.SharedPass.Luban.ElapsedSeconds != 40 ||
		data.SharedPass.Codex.InputTokens != 2000 || data.SharedPass.Luban.InputTokens != 2000 {
		t.Fatalf("shared-pass aggregates = codex:%#v luban:%#v", data.SharedPass.Codex, data.SharedPass.Luban)
	}
	wantRunCost := estimateCost(sampleUsage(), samplePricing())
	if math.Abs(data.Agents[1].EstimatedCost-5*wantRunCost) > 0.0000001 {
		t.Fatalf("Luban cost = %.8f, want %.8f", data.Agents[1].EstimatedCost, 5*wantRunCost)
	}
	if data.LubanChanges[0].After != "4/5 (80.0%)" || data.LubanChanges[0].Change != "+40.0 pp" {
		t.Fatalf("Luban quality change = %#v", data.LubanChanges[0])
	}

	if err := generateReport(value.options, i18n.LangEN); err != nil {
		t.Fatalf("generateReport() error = %v", err)
	}
	raw, err := os.ReadFile(value.options.outputPath)
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	for _, fragment := range []string{"10/10", "4/5 (80.0%)", "40.0 pp", "$0.0196", "http_2xx=10", "http_non_2xx=0", "has not demonstrated comprehensive superiority", "mutable local files", "Efficiency on tasks passed by both agents", "only the 2 tasks", "provider_seconds_per_POST", "diagnostic only"} {
		if !strings.Contains(html, fragment) {
			t.Errorf("complete report is missing %q", fragment)
		}
	}
}

func TestSharedPassIntersectionIsDynamicAndFailsClosed(t *testing.T) {
	value := newFixture(t)
	for taskIndex, taskID := range localTasks {
		writeRunPair(t, value, taskID, "codex", taskIndex == 0 || taskIndex == 2, 3)
		writeRunPair(t, value, taskID, "luban", taskIndex == 1 || taskIndex == 2, 2)
	}

	data, err := compileReport(value.options, i18n.LangEN)
	if err != nil {
		t.Fatalf("compileReport() error = %v", err)
	}
	if !data.SharedPass.Available || data.SharedPass.TaskCount != 1 || len(data.SharedPass.Tasks) != 1 || data.SharedPass.Tasks[0].ID != localTasks[2] {
		t.Fatalf("dynamic shared-pass intersection = %#v", data.SharedPass)
	}
	if data.SharedPass.Codex.LLMCalls != 3 || data.SharedPass.Luban.LLMCalls != 2 {
		t.Fatalf("dynamic shared-pass calls = codex:%d luban:%d", data.SharedPass.Codex.LLMCalls, data.SharedPass.Luban.LLMCalls)
	}

	// Corrupt a run outside the otherwise shared-passed intersection. The
	// section must close entirely instead of treating missing evidence as fail.
	summaryPath := filepath.Join(value.options.rootPath, "raw", "runs", localTasks[4], "codex", "summary.json")
	var summary runSummary
	readJSON(t, summaryPath, &summary)
	summary.EstimatedCostUSD++
	writeJSON(t, summaryPath, summary)
	data, err = compileReport(value.options, i18n.LangEN)
	if err != nil {
		t.Fatalf("compileReport() after corruption error = %v", err)
	}
	if data.Complete || data.SharedPass.Available || len(data.SharedPass.Tasks) != 0 {
		t.Fatalf("shared-pass fail-closed state = complete:%t shared:%#v", data.Complete, data.SharedPass)
	}
}

func TestFrozenPilotSharedPassProjectionMatchesReceipts(t *testing.T) {
	packageRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	data, err := compileReport(options{
		rootPath:     packageRoot,
		baselinePath: filepath.Join(packageRoot, "..", "agentic-2026-07-26", "results.json"),
		repoRootPath: filepath.Clean(filepath.Join(packageRoot, "..", "..")),
		outputPath:   filepath.Join(t.TempDir(), "report.html"),
	}, i18n.LangEN)
	if err != nil {
		t.Fatalf("compile frozen pilot: %v", err)
	}
	shared := data.SharedPass
	if !shared.Available || shared.TaskCount != 3 || len(shared.Tasks) != 3 || shared.Tasks[0].ID != localTasks[0] || shared.Tasks[1].ID != localTasks[1] || shared.Tasks[2].ID != localTasks[3] {
		t.Fatalf("frozen shared-pass set = %#v", shared)
	}
	if shared.Codex.LLMCalls != 71 || shared.Luban.LLMCalls != 109 ||
		math.Abs(shared.Codex.ElapsedSeconds-1350.830) > 0.000001 || math.Abs(shared.Luban.ElapsedSeconds-4354.811) > 0.000001 ||
		math.Abs(shared.Codex.ProviderRequestSeconds-1339.432568) > 0.000001 || math.Abs(shared.Luban.ProviderRequestSeconds-3801.706405) > 0.000001 ||
		shared.Codex.InputTokens != 3_433_804 || shared.Luban.InputTokens != 5_076_924 ||
		shared.Codex.CachedTokens != 3_193_856 || shared.Luban.CachedTokens != 4_783_104 ||
		shared.Codex.CacheWriteTokens != 0 || shared.Luban.CacheWriteTokens != 0 ||
		shared.Codex.OutputTokens != 32_613 || shared.Luban.OutputTokens != 120_504 ||
		math.Abs(shared.Codex.EstimatedCost-3.775058) > 0.000001 || math.Abs(shared.Luban.EstimatedCost-7.475772) > 0.000001 ||
		shared.Codex.ToolCalls != 63 || shared.Luban.ToolCalls != 96 {
		t.Fatalf("frozen shared-pass aggregates = codex:%#v luban:%#v", shared.Codex, shared.Luban)
	}
	if shared.CodexInputPerCall != "48363.4" || shared.LubanInputPerCall != "46577.3" || shared.InputPerCallDelta != "-3.7%" ||
		shared.CodexProviderPerCall != "18.87s" || shared.LubanProviderPerCall != "34.88s" || shared.ProviderPerCallDelta != "+84.9%" ||
		shared.CodexOutputPerCall != "459.3" || shared.LubanOutputPerCall != "1105.5" || shared.OutputPerCallDelta != "+140.7%" || !shared.LubanLongerSlower {
		t.Fatalf("frozen shared-pass normalization = %#v", shared)
	}
	if !data.Headline.Adjudicated || data.Headline.CodexResolved != 3 || data.Headline.LubanResolved != 3 || data.AdjudicationsObserved != 1 {
		t.Fatalf("frozen adjudicated headline = %#v, adjudications=%d", data.Headline, data.AdjudicationsObserved)
	}
}

func TestMalformedCurrentReceiptStaysUnavailable(t *testing.T) {
	value := newFixture(t)
	value.options.allowPartial = true
	writeRunPair(t, value, localTasks[0], "luban", true, 2)
	summaryPath := filepath.Join(value.options.rootPath, "raw", "runs", localTasks[0], "luban", "summary.json")
	var summary runSummary
	readJSON(t, summaryPath, &summary)
	summary.EstimatedCostUSD++
	writeJSON(t, summaryPath, summary)

	data, err := compileReport(value.options, i18n.LangEN)
	if err != nil {
		t.Fatalf("compileReport() error = %v", err)
	}
	if data.Complete || data.RunSlotsObserved != 0 || data.EvaluationSlotsObserved != 1 || data.PairSlotsObserved != 0 {
		t.Fatalf("malformed receipt coverage = complete:%t run:%d eval:%d pair:%d", data.Complete, data.RunSlotsObserved, data.EvaluationSlotsObserved, data.PairSlotsObserved)
	}
	if err := generateReport(value.options, i18n.LangEN); err != nil {
		t.Fatalf("partial generateReport() error = %v", err)
	}
}

func TestProviderRequestProjectionMustMatchSummary(t *testing.T) {
	value := newFixture(t)
	writeRunPair(t, value, localTasks[0], "codex", true, 3)
	logPath := filepath.Join(value.options.rootPath, "raw", "runs", localTasks[0], "codex", "provider-requests.jsonl")
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if err := os.WriteFile(logPath, []byte(strings.Join(lines[:2], "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := compileReport(value.options, i18n.LangEN)
	if err != nil {
		t.Fatalf("compileReport() error = %v", err)
	}
	if data.RunSlotsObserved != 0 || data.PairSlotsObserved != 0 || data.EvaluationSlotsObserved != 1 {
		t.Fatalf("projection mismatch coverage = run:%d pair:%d eval:%d", data.RunSlotsObserved, data.PairSlotsObserved, data.EvaluationSlotsObserved)
	}
}

func TestStrictProjectionRejectsPassToPassMissingCoverage(t *testing.T) {
	value := newFixture(t)
	for _, taskID := range localTasks {
		for _, agentID := range localAgents {
			writeRunPair(t, value, taskID, agentID, true, 2)
		}
	}
	evaluationPath := filepath.Join(value.options.rootPath, "raw", "evaluation", localTasks[0], "luban", "report.json")
	var evaluation evaluationData
	readJSON(t, evaluationPath, &evaluation)
	*evaluation.PassToPass.PassedCount = 0
	*evaluation.PassToPass.MissingCount = 1
	writeJSON(t, evaluationPath, evaluation)

	data, err := compileReport(value.options, i18n.LangEN)
	if err != nil {
		t.Fatalf("compileReport() error = %v", err)
	}
	if data.Complete || data.EvaluationSlotsObserved != 9 || data.PairSlotsObserved != 9 {
		t.Fatalf("projection coverage = complete:%t eval:%d pair:%d", data.Complete, data.EvaluationSlotsObserved, data.PairSlotsObserved)
	}
}

func TestComparableCostChargesEachInputClassOnce(t *testing.T) {
	usage := sampleUsage()
	pricing := samplePricing()
	if got, want := estimateCost(usage, pricing), 0.003925; math.Abs(got-want) > 0.000000001 {
		t.Fatalf("estimateCost() = %.9f, want %.9f", got, want)
	}
	if got, want := runnerReportedCost(usage, pricing), 0.004425; math.Abs(got-want) > 0.000000001 {
		t.Fatalf("runnerReportedCost() = %.9f, want legacy receipt %.9f", got, want)
	}
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	current := filepath.Join(root, "agentic-2026-07-27")
	baselinePath := filepath.Join(root, "agentic-2026-07-26", "results.json")
	repository := filepath.Join(root, "repo")
	output := filepath.Join(current, "report.html")
	writeJSON(t, baselinePath, sampleBaseline())
	writeJSON(t, filepath.Join(current, "raw", "metadata", "experiment.json"), sampleBaseline().Experiment)
	for _, taskID := range localTasks {
		writeJSON(t, filepath.Join(current, "raw", "evaluation", taskID, "gold", "report.json"), sampleEvaluation(taskID, "gold", true))
	}
	if err := os.MkdirAll(filepath.Join(repository, "internal", "tools", "search"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "internal", "tools", "search", "inspect_bridge.go"), []byte("package search\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return fixture{
		root: root,
		options: options{
			rootPath:     current,
			baselinePath: baselinePath,
			repoRootPath: repository,
			outputPath:   output,
		},
	}
}

func writeRunPair(t *testing.T, value fixture, taskID, agentID string, resolved bool, llmCalls int) {
	t.Helper()
	usage := sampleUsage()
	successful := llmCalls
	failed := 0
	providerSeconds := float64(llmCalls)
	reasoning := int64(12)
	toolTypes := map[string]int{"command_execution": 2}
	if agentID == "luban" {
		toolTypes = map[string]int{"Inspect": 1, "Run": 1}
	}
	summary := runSummary{
		InstanceID:            taskID,
		Language:              "go",
		Agent:                 agentID,
		Model:                 localModel,
		ReasoningEffort:       localEffort,
		StartedAtUnix:         1_754_000_000,
		ElapsedSeconds:        20,
		TimeoutSeconds:        1800,
		ExitCode:              0,
		Usage:                 usage,
		EstimatedCostUSD:      runnerReportedCost(usage, samplePricing()),
		ToolCalls:             2,
		ToolCallsByType:       toolTypes,
		Patch:                 patchData{FilesChanged: 1, Files: []string{"main.go"}, Additions: 2},
		LLMCalls:              &llmCalls,
		LLMSuccessfulCalls:    &successful,
		LLMFailedCalls:        &failed,
		ProviderRequestSecond: &providerSeconds,
		Binary:                binaryData{Path: "/tmp/" + agentID, SHA256: expectedBinarySHA256[agentID]},
	}
	summary.Usage.ReasoningOutputTokens = &reasoning
	summaryPath := filepath.Join(value.options.rootPath, "raw", "runs", taskID, agentID, "summary.json")
	writeJSON(t, summaryPath, summary)
	var requestLog strings.Builder
	for index := 0; index < llmCalls; index++ {
		record := map[string]any{
			"sequence": index, "method": "POST", "endpoint": "responses", "status": 200,
			"elapsed_seconds": 1.0, "request_id_sha256": strings.Repeat("b", 64),
		}
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		requestLog.Write(raw)
		requestLog.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(summaryPath), "provider-requests.jsonl"), []byte(requestLog.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(summaryPath), "events.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(value.options.rootPath, "raw", "evaluation", taskID, agentID, "report.json"), sampleEvaluation(taskID, agentID, resolved))
}

func TestPortableReportPath(t *testing.T) {
	repository := t.TempDir()
	inside := filepath.Join(repository, "artifacts", "luban")
	outside := filepath.Join(t.TempDir(), "codex")

	for _, test := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "repository relative", value: inside, want: "artifacts/luban"},
		{name: "external binary", value: outside, want: "codex"},
		{name: "foreign Windows binary", value: `C:\tools\codex.exe`, want: "codex.exe"},
		{name: "already relative", value: filepath.Join("artifacts", "codex"), want: "artifacts/codex"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := portableReportPath(repository, test.value); got != test.want {
				t.Fatalf("portableReportPath(%q, %q) = %q, want %q", repository, test.value, got, test.want)
			}
		})
	}
}

func sampleEvaluation(taskID, agentID string, resolved bool) evaluationData {
	zero := 0
	one := 1
	passed := []string{"case"}
	failed := []string{}
	if !resolved {
		passed = []string{}
		failed = []string{"case"}
	}
	return evaluationData{
		InstanceID:                 taskID,
		Agent:                      agentID,
		Language:                   "go",
		Resolved:                   &resolved,
		ElapsedSeconds:             5,
		ImagePullExitCode:          &zero,
		TestPatchApplyExitCode:     &zero,
		SolutionPatchApplyExitCode: &zero,
		RebuildExitCode:            &zero,
		TestExitCode:               &zero,
		PrintExitCode:              &zero,
		DiagnosticExcludedPaths:    []string{},
		FailToPass: failToPassData{
			Expected: &one,
			Passed:   passed,
			Failed:   failed,
			Missing:  []string{},
		},
		PassToPass: passToPassData{
			Expected:     &one,
			PassedCount:  &one,
			Failed:       []string{},
			MissingCount: &zero,
		},
	}
}

func sampleUsage() usageData {
	return usageData{
		InputTokens:              1000,
		CachedInputTokens:        600,
		CacheCreationInputTokens: 100,
		OutputTokens:             50,
	}
}

func samplePricing() pricingData {
	return pricingData{InputPerMillionUSD: 5, CachedInputPerMillionUSD: 0.5, OutputPerMillionUSD: 30}
}

func sampleBaseline() legacyResults {
	return legacyResults{
		ReportSchema: "luban-agent-comparison/v1",
		TaskOrder:    append([]string(nil), localTasks...),
		Experiment: experimentData{
			Dataset:           "SWE-bench-Live/MultiLang",
			DatasetRevision:   strings.Repeat("c", 40),
			ParquetSHA256:     map[string]string{"cpp": strings.Repeat("1", 64), "go": strings.Repeat("2", 64), "java": strings.Repeat("3", 64), "rust": strings.Repeat("4", 64), "ts": strings.Repeat("5", 64)},
			Model:             localModel,
			ReasoningEffort:   localEffort,
			PricingAssumption: samplePricing(),
			TimeoutSeconds:    localTimeout,
		},
		Aggregates: map[string]legacyAggregate{
			"codex": {
				Resolved: 2, TotalTasks: 5, ElapsedSeconds: 2223.512, EstimatedCostUSD: 8.288285,
				ToolCalls: 143, ToolCallsByType: map[string]int{"command_execution": 128, "file_change": 15},
				InputTokens: 8_716_033, CachedInputTokens: 8_245_760, UncachedInputTokens: 470_273,
				CacheRatio: float64(8_245_760) / 8_716_033, OutputTokens: 60_468,
			},
			"luban": {
				Resolved: 2, TotalTasks: 5, ElapsedSeconds: 3040.168, EstimatedCostUSD: 7.840923,
				ToolCalls: 351, ToolCallsByType: map[string]int{"Bash": 49, "Edit": 50, "Glob": 24, "Grep": 93, "Read": 127, "ToolSearch": 5, "Write": 3},
				InputTokens: 6_944_553, CachedInputTokens: 6_562_816, UncachedInputTokens: 381_737,
				CacheRatio: float64(6_562_816) / 6_944_553, OutputTokens: 88_361,
			},
		},
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}
