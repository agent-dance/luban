package localbench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
)

type fakeExecutor struct{}

func (fakeExecutor) Prepare(context.Context, string, string) (PreparedEnvironment, error) {
	return PreparedEnvironment{
		GatewayOrigin: "https://gateway.example.invalid", EvaluatorEngine: "local-docker",
		Binaries: []BinaryIdentity{{Name: "codex", SHA256: strings.Repeat("a", 64)}, {Name: "luban", SHA256: strings.Repeat("b", 64)}},
	}, nil
}

func (fakeExecutor) RunAgent(_ context.Context, _ string, task TaskSelection, agent string, timeout int) (RunSummary, error) {
	calls := 2
	if agent == "codex" {
		calls = 3
	}
	return RunSummary{
		InstanceID: task.InstanceID, Language: task.Language, Agent: agent,
		Model: ModelID, ReasoningEffort: ReasoningEffort, StartedAt: time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC),
		ElapsedSeconds: 20, TimeoutSeconds: timeout, Usage: Usage{InputTokens: 1000, CachedInputTokens: 800, OutputTokens: 100},
		EstimatedCostUSD: .01, ToolEvents: 2, ToolEventsByType: map[string]int{"Inspect": 2},
		Patch:    PatchStats{FilesChanged: 1, Files: []string{"a.go"}, Additions: 1},
		LLMCalls: calls, LLMSuccessfulCalls: calls, ProviderRequestSeconds: 18,
		Binary:       BinaryIdentity{Name: agent, SHA256: strings.Repeat("c", 64)},
		EvidenceRoot: filepath.ToSlash(filepath.Join("raw", "runs", task.InstanceID, agent)),
	}, nil
}

func (fakeExecutor) Evaluate(_ context.Context, _ string, task TaskSelection, agent string, _ int) (Evaluation, error) {
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
	options := Options{RepositoryRoot: root, ResultsRoot: filepath.Join(root, "benchmark-results"), TaskSize: 2, Now: clock}
	first, err := Run(context.Background(), options, fakeExecutor{}, i18n.LangZH)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Run(context.Background(), options, fakeExecutor{}, i18n.LangZH)
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
	for _, fragment := range []string{"Codex 与 Luban", "2/2 (100.0%)", "LLM", "benchmark.json", "raw/runs/"} {
		if !strings.Contains(html, fragment) {
			t.Errorf("report is missing %q", fragment)
		}
	}
	if strings.Contains(html, root) {
		t.Fatalf("report contains local absolute root %q", root)
	}
}

func TestCatalogSelectionUsesFrozenRepresentativePrefix(t *testing.T) {
	tasks, err := loadSelection(3)
	if err != nil {
		t.Fatal(err)
	}
	for index, task := range tasks {
		if task.InstanceID != representativeOrder[index] {
			t.Fatalf("task[%d] = %q", index, task.InstanceID)
		}
	}
}
