package localbench

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
)

type fakeExecutor struct {
	mu         sync.Mutex
	prepared   int
	runAgents  []string
	evalAgents []string
	failCodex  bool
}

func (executor *fakeExecutor) Prepare(_ context.Context, _, _ string, withCodex bool) (PreparedEnvironment, error) {
	executor.mu.Lock()
	executor.prepared++
	executor.mu.Unlock()
	binaries := []BinaryIdentity{{Name: "luban", Version: "workspace-build", SHA256: strings.Repeat("b", 64)}}
	if withCodex {
		binaries = append([]BinaryIdentity{{Name: "codex", Version: "codex-cli 0.145.0", SHA256: strings.Repeat("a", 64)}}, binaries...)
	}
	return PreparedEnvironment{
		GatewayOrigin: "https://gateway.example.invalid", EvaluatorEngine: "local-docker",
		Binaries: binaries,
	}, nil
}

func (executor *fakeExecutor) RunAgent(_ context.Context, _ string, task TaskSelection, agent string, timeout int) (RunSummary, error) {
	executor.mu.Lock()
	executor.runAgents = append(executor.runAgents, agent)
	fail := agent == "codex" && executor.failCodex
	executor.mu.Unlock()
	if fail {
		return RunSummary{}, errors.New("injected Codex failure")
	}
	calls := 2
	if agent == "codex" {
		calls = 3
	}
	binarySHA := strings.Repeat("b", 64)
	binaryVersion := "workspace-build"
	if agent == "codex" {
		binarySHA = strings.Repeat("a", 64)
		binaryVersion = "codex-cli 0.145.0"
	}
	return RunSummary{
		InstanceID: task.InstanceID, Language: task.Language, Agent: agent,
		Model: ModelID, ReasoningEffort: ReasoningEffort, StartedAt: time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC),
		ElapsedSeconds: 20, TimeoutSeconds: timeout, Usage: Usage{InputTokens: 1000, CachedInputTokens: 800, OutputTokens: 100},
		EstimatedCostUSD: .01, ToolEvents: 2, ToolEventsByType: map[string]int{"Inspect": 2},
		Patch:    PatchStats{FilesChanged: 1, Files: []string{"a.go"}, Additions: 1},
		LLMCalls: calls, LLMSuccessfulCalls: calls, ProviderRequestSeconds: 18,
		Binary:       BinaryIdentity{Name: agent, Version: binaryVersion, SHA256: binarySHA},
		EvidenceRoot: filepath.ToSlash(filepath.Join("raw", "runs", task.InstanceID, agent)),
	}, nil
}

func (executor *fakeExecutor) Evaluate(_ context.Context, _ string, task TaskSelection, agent string, _ int) (Evaluation, error) {
	executor.mu.Lock()
	executor.evalAgents = append(executor.evalAgents, agent)
	executor.mu.Unlock()
	return Evaluation{
		InstanceID: task.InstanceID, Language: task.Language, Agent: agent, Resolved: true, ElapsedSeconds: 5,
		FailToPass:   TestPartition{Expected: 1, Passed: []string{"f2p"}, Failed: []string{}, Missing: []string{}},
		PassToPass:   TestPartition{Expected: 1, PassedCount: 1, Failed: []string{}, MissingCount: 0},
		EvidenceRoot: filepath.ToSlash(filepath.Join("raw", "evaluation", task.InstanceID, agent)),
	}, nil
}

func TestRunCreatesUniqueDateGroupedStructuredReport(t *testing.T) {
	root := t.TempDir()
	clock := func() time.Time { return time.Date(2026, 7, 27, 9, 8, 7, 0, time.FixedZone("CST", 8*60*60)) }
	resultsRoot := filepath.Join(root, "benchmark-results")
	options := Options{RepositoryRoot: root, ResultsRoot: resultsRoot, TaskSize: 2, WithCodex: true, Now: clock}
	executor := &fakeExecutor{}
	first, err := Run(context.Background(), options, executor, i18n.LangZH)
	if err != nil {
		t.Fatal(err)
	}
	options.WithCodex = false
	second, err := Run(context.Background(), options, executor, i18n.LangZH)
	if err != nil {
		t.Fatal(err)
	}
	if first.RunRoot == second.RunRoot || !strings.Contains(first.RunRoot, filepath.Join("agentic-2026-07-27", "run-20260727-090807-n2")) || !strings.HasSuffix(second.RunRoot, "-2") {
		t.Fatalf("run roots = %q, %q", first.RunRoot, second.RunRoot)
	}
	var result BenchmarkResult
	raw, err := os.ReadFile(first.ResultPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "complete" || len(result.Tasks) != 2 || len(result.Runs) != 4 || len(result.Evaluations) != 4 || len(result.GoldEvaluations) != 2 || len(result.Aggregates) != 2 {
		t.Fatalf("result coverage = %#v", result)
	}
	htmlRaw, err := os.ReadFile(first.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlRaw)
	for _, fragment := range []string{"Codex 与 Luban", "2/2 (100.0%)", "LLM", "benchmark.json", "raw/runs/", "codex-cli 0.145.0", "已刷新 Codex 基线"} {
		if !strings.Contains(html, fragment) {
			t.Errorf("report is missing %q", fragment)
		}
	}
	if strings.Contains(html, root) {
		t.Fatalf("report contains local absolute root %q", root)
	}
	for _, path := range []string{first.CodexReportPath, filepath.Join(first.RunRoot, currentCodexBaselineJSON), filepath.Join(resultsRoot, currentCodexBaselineJSON), filepath.Join(resultsRoot, currentCodexBaselineHTML)} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected baseline artifact %q: %v", path, err)
		}
	}
	codexHTML, err := os.ReadFile(first.CodexReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(codexHTML), "codex-cli 0.145.0") || strings.Contains(string(codexHTML), root) {
		t.Fatalf("standalone Codex report does not disclose a relative, versioned snapshot")
	}
	var reused BenchmarkResult
	secondRaw, err := os.ReadFile(second.ResultPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(secondRaw, &reused); err != nil {
		t.Fatal(err)
	}
	if reused.CodexBaseline.Refreshed || reused.CodexBaseline.CodexVersion != "codex-cli 0.145.0" || reused.CodexBaseline.SourceRunID != result.RunID {
		t.Fatalf("reused baseline = %#v", reused.CodexBaseline)
	}
	secondHTML, err := os.ReadFile(second.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(secondHTML), "本次未重新运行 Codex") || strings.Contains(string(secondHTML), root) {
		t.Fatalf("reused report does not disclose its historical baseline")
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if got := countString(executor.runAgents, "codex"); got != 2 {
		t.Fatalf("Codex agent calls = %d, want only the two refresh calls; all=%v", got, executor.runAgents)
	}
	if got := countString(executor.evalAgents, "codex"); got != 2 {
		t.Fatalf("Codex evaluation calls = %d, want only the two refresh calls; all=%v", got, executor.evalAgents)
	}
}

func TestCheckedInCodexBaselineIsCompleteAndVersioned(t *testing.T) {
	const checkedInBaselineTaskCount = 5
	repositoryRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	resultsRoot := filepath.Join(repositoryRoot, "benchmark-results")
	tasks, err := loadSelection(checkedInBaselineTaskCount)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := loadCodexBaseline(resultsRoot, tasks)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.snapshot.Binary.Version == "" || baseline.snapshot.Aggregate.RunsObserved != checkedInBaselineTaskCount || baseline.snapshot.Aggregate.EvaluationsObserved != checkedInBaselineTaskCount || baseline.snapshot.Aggregate.Resolved != 3 {
		t.Fatalf("checked-in baseline = %#v", baseline.snapshot)
	}
	if !resolvedEvaluation(baseline.snapshot.Evaluations, "skim-rs__skim-1044", "codex") {
		t.Fatalf("checked-in baseline does not include the skim adjudication")
	}
	html, err := os.ReadFile(filepath.Join(resultsRoot, currentCodexBaselineHTML))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), baseline.snapshot.Binary.Version) || !strings.Contains(string(html), "3/5 (60.0%)") ||
		!strings.Contains(string(html), "通过（诊断裁决）") || strings.Contains(string(html), repositoryRoot) {
		t.Fatalf("checked-in Codex report does not identify its version with relative paths")
	}
}

func TestRunRequiresBaselineBeforePreparing(t *testing.T) {
	executor := &fakeExecutor{}
	_, err := Run(context.Background(), Options{RepositoryRoot: t.TempDir(), ResultsRoot: filepath.Join(t.TempDir(), "results"), TaskSize: 1}, executor, i18n.LangEN)
	if err == nil {
		t.Fatal("Run() unexpectedly succeeded")
	}
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyLocalBenchmarkBaselineRequired {
		t.Fatalf("Run() error = %#v, %v", info, err)
	}
	if executor.prepared != 0 {
		t.Fatalf("Prepare calls = %d, want 0", executor.prepared)
	}
}

func TestRunRejectsBaselineWithInsufficientTaskCoverage(t *testing.T) {
	root := t.TempDir()
	resultsRoot := filepath.Join(root, "results")
	executor := &fakeExecutor{}
	clock := func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC) }
	if _, err := Run(context.Background(), Options{RepositoryRoot: root, ResultsRoot: resultsRoot, TaskSize: 1, WithCodex: true, Now: clock}, executor, i18n.LangEN); err != nil {
		t.Fatal(err)
	}
	preparedBefore := executor.prepared
	_, err := Run(context.Background(), Options{RepositoryRoot: root, ResultsRoot: resultsRoot, TaskSize: 2, Now: clock}, executor, i18n.LangEN)
	if err == nil {
		t.Fatal("Run() unexpectedly accepted a smaller baseline")
	}
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyLocalBenchmarkBaselineIncompatible {
		t.Fatalf("Run() error = %#v, %v", info, err)
	}
	if executor.prepared != preparedBefore {
		t.Fatalf("Prepare ran before rejecting baseline: %d -> %d", preparedBefore, executor.prepared)
	}
}

func TestFailedCodexRefreshPreservesPreviousBaseline(t *testing.T) {
	root := t.TempDir()
	resultsRoot := filepath.Join(root, "results")
	executor := &fakeExecutor{}
	firstClock := func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC) }
	options := Options{RepositoryRoot: root, ResultsRoot: resultsRoot, TaskSize: 1, WithCodex: true, Now: firstClock}
	if _, err := Run(context.Background(), options, executor, i18n.LangEN); err != nil {
		t.Fatal(err)
	}
	baselinePath := filepath.Join(resultsRoot, currentCodexBaselineJSON)
	before, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	executor.mu.Lock()
	executor.failCodex = true
	executor.mu.Unlock()
	options.Now = func() time.Time { return time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC) }
	outcome, err := Run(context.Background(), options, executor, i18n.LangEN)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Complete {
		t.Fatal("failed Codex refresh unexpectedly completed")
	}
	after, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("failed Codex refresh replaced the previous baseline")
	}
}

func countString(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

func TestCatalogSelectionUsesFrozenRepresentativePrefix(t *testing.T) {
	tasks, err := loadSelection(CatalogSize())
	if err != nil {
		t.Fatal(err)
	}
	for index, task := range tasks {
		if task.InstanceID != representativeOrder[index] {
			t.Fatalf("task[%d] = %q", index, task.InstanceID)
		}
	}
	counts := map[string]int{}
	for _, task := range tasks {
		counts[task.Language]++
	}
	for _, language := range []string{"cpp", "go", "java", "rust", "ts"} {
		if counts[language] != 4 {
			t.Errorf("%s task count = %d, want 4", language, counts[language])
		}
	}
}

func TestRepresentative20CatalogMatchesFrozenSelectionManifest(t *testing.T) {
	catalogRaw, err := runtimeAssets.ReadFile("catalog/representative20.json")
	if err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := runtimeAssets.ReadFile("catalog/representative20.selection.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog []struct {
		InstanceID       string   `json:"instance_id"`
		Language         string   `json:"language"`
		Repository       string   `json:"repo"`
		BaseCommit       string   `json:"base_commit"`
		ProblemStatement string   `json:"problem_statement"`
		Patch            string   `json:"patch"`
		TestPatch        string   `json:"test_patch"`
		DockerImage      string   `json:"docker_image"`
		FailToPass       []string `json:"FAIL_TO_PASS"`
		PassToPass       []string `json:"PASS_TO_PASS"`
	}
	if err := json.Unmarshal(catalogRaw, &catalog); err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		DatasetRevision   string            `json:"dataset_revision"`
		OrderedInstanceID []string          `json:"ordered_instance_ids"`
		ParquetSHA256     map[string]string `json:"parquet_sha256"`
		CatalogSHA256     string            `json:"catalog_sha256"`
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(catalog) != CatalogSize() || len(manifest.OrderedInstanceID) != CatalogSize() {
		t.Fatalf("catalog=%d manifest=%d want=%d", len(catalog), len(manifest.OrderedInstanceID), CatalogSize())
	}
	if manifest.DatasetRevision != DatasetRevision || fmt.Sprintf("%x", sha256.Sum256(catalogRaw)) != manifest.CatalogSHA256 {
		t.Fatalf("manifest identity does not match the embedded catalog")
	}
	if len(manifest.ParquetSHA256) != 5 {
		t.Fatalf("parquet hashes = %d, want 5", len(manifest.ParquetSHA256))
	}
	repositories := map[string]bool{}
	for index, task := range catalog {
		if task.InstanceID != representativeOrder[index] || task.InstanceID != manifest.OrderedInstanceID[index] {
			t.Fatalf("catalog[%d] identity = %q", index, task.InstanceID)
		}
		if task.Repository == "" || task.BaseCommit == "" || task.ProblemStatement == "" || task.Patch == "" || task.TestPatch == "" || task.DockerImage == "" || len(task.FailToPass) < 1 || len(task.FailToPass) > 50 || len(task.PassToPass) < 1 || len(task.PassToPass) > 1000 {
			t.Errorf("catalog[%d] is incomplete or outside the frozen bounds", index)
		}
		repository := strings.ToLower(task.Repository)
		if repositories[repository] {
			t.Errorf("catalog[%d] repeats repository %q", index, task.Repository)
		}
		repositories[repository] = true
	}
}
