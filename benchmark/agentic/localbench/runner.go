package localbench

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const selectionPolicy = "frozen-representative-order-prefix-v1"

//go:embed catalog/representative5.json worker/run_worker.py worker/evaluate_worker.py
var runtimeAssets embed.FS

type Options struct {
	RepositoryRoot   string
	ResultsRoot      string
	TaskSize         int
	WithCodex        bool
	AgentTimeout     int
	EvaluatorTimeout int
	Now              func() time.Time
	Progress         func(i18n.Key, ...any)
}

type Outcome struct {
	RunRoot         string
	ResultPath      string
	ReportPath      string
	CodexReportPath string
	LogPath         string
	Complete        bool
}

type PreparedEnvironment struct {
	GatewayOrigin   string
	EvaluatorEngine string
	Binaries        []BinaryIdentity
}

type Executor interface {
	Prepare(context.Context, string, string, bool) (PreparedEnvironment, error)
	RunAgent(context.Context, string, TaskSelection, string, int) (RunSummary, error)
	Evaluate(context.Context, string, TaskSelection, string, int) (Evaluation, error)
}

func CatalogSize() int { return len(representativeOrder) }

func Run(ctx context.Context, options Options, executor Executor, language i18n.Language) (Outcome, error) {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.AgentTimeout <= 0 {
		options.AgentTimeout = 1800
	}
	if options.EvaluatorTimeout <= 0 {
		options.EvaluatorTimeout = 2700
	}
	if options.TaskSize < 1 || options.TaskSize > CatalogSize() {
		return Outcome{}, i18n.NewError(KeyTaskSizeRange(), CatalogSize())
	}
	tasks, err := loadSelection(options.TaskSize)
	if err != nil {
		return Outcome{}, err
	}
	resultsRoot, err := absoluteResultsRoot(options.ResultsRoot)
	if err != nil {
		return Outcome{}, err
	}
	var frozen *loadedCodexBaseline
	if !options.WithCodex {
		frozen, err = loadCodexBaseline(resultsRoot, tasks)
		if err != nil {
			return Outcome{}, err
		}
	}
	startedLocal := options.Now()
	started := startedLocal.UTC()
	runRoot, runID, err := createRunRoot(resultsRoot, startedLocal, options.TaskSize)
	if err != nil {
		return Outcome{}, err
	}
	outcome := Outcome{
		RunRoot: runRoot, ResultPath: filepath.Join(runRoot, "benchmark.json"),
		ReportPath: filepath.Join(runRoot, "report.html"), LogPath: filepath.Join(runRoot, "execution.log"),
	}
	if options.WithCodex {
		outcome.CodexReportPath = filepath.Join(runRoot, "codex-report.html")
	}
	result := BenchmarkResult{
		SchemaVersion: ResultSchemaVersion, Status: "running", RunID: runID, StartedAt: started,
		Dataset: DatasetName, DatasetRevision: DatasetRevision, SelectionPolicy: selectionPolicy,
		Tasks: tasks, Model: ModelID, ReasoningEffort: ReasoningEffort,
		CodexRequested: options.WithCodex,
		AgentTimeout:   options.AgentTimeout, EvaluatorTimeout: options.EvaluatorTimeout,
		Pricing: FrozenPricing(),
	}
	persist := func() error { return writeJSONAtomic(outcome.ResultPath, result) }
	if err := persist(); err != nil {
		return outcome, err
	}
	progress(options, i18n.KeyLocalBenchmarkPreparing, runID)
	prepared, prepareErr := executor.Prepare(ctx, runRoot, options.RepositoryRoot, options.WithCodex)
	if prepareErr != nil {
		result.Failures = append(result.Failures, Failure{Stage: "prepare", Code: "prepare_failed", EvidencePath: "execution.log"})
	} else {
		result.GatewaySHA256, result.EvaluatorEngine = gatewaySHA256(prepared.GatewayOrigin), prepared.EvaluatorEngine
		result.Binaries = append(result.Binaries, prepared.Binaries...)
		if frozen != nil {
			if frozen.snapshot.GatewaySHA256 != result.GatewaySHA256 {
				prepareErr = i18n.NewError(i18n.KeyLocalBenchmarkBaselineIncompatible)
				result.Failures = append(result.Failures, Failure{Stage: "codex_baseline", Code: "gateway_mismatch", EvidencePath: frozen.reference.SnapshotPath})
			} else {
				result.CodexBaseline = baselineReference(runRoot, frozen, false)
				result.Binaries = append([]BinaryIdentity{frozen.snapshot.Binary}, result.Binaries...)
				seedCodexBaseline(&result, runRoot, frozen)
			}
		}
	}
	if prepareErr == nil {
		for _, task := range tasks {
			runs := runAgents(ctx, options, executor, runRoot, task)
			for _, value := range runs {
				if value.err != nil {
					result.Failures = append(result.Failures, Failure{Stage: "agent", InstanceID: task.InstanceID, Agent: value.agent, Code: "agent_run_failed", EvidencePath: value.evidence})
					continue
				}
				result.Runs = append(result.Runs, value.run)
			}
			progress(options, i18n.KeyLocalBenchmarkEvaluating, "gold", task.InstanceID)
			gold, goldErr := executor.Evaluate(ctx, runRoot, task, "gold", options.EvaluatorTimeout)
			if goldErr != nil {
				result.Failures = append(result.Failures, Failure{Stage: "evaluate", InstanceID: task.InstanceID, Agent: "gold", Code: "evaluation_failed", EvidencePath: evaluationLogPath(task.InstanceID, "gold")})
			} else {
				result.GoldEvaluations = append(result.GoldEvaluations, gold)
				if !gold.Resolved {
					result.Failures = append(result.Failures, Failure{Stage: "oracle", InstanceID: task.InstanceID, Agent: "gold", Code: "gold_oracle_failed", EvidencePath: filepath.ToSlash(filepath.Join(gold.EvidenceRoot, "report.json"))})
				}
			}
			for _, agent := range activeAgents(options.WithCodex) {
				if !hasRun(result.Runs, task.InstanceID, agent) {
					continue
				}
				progress(options, i18n.KeyLocalBenchmarkEvaluating, agent, task.InstanceID)
				evaluation, evaluationErr := executor.Evaluate(ctx, runRoot, task, agent, options.EvaluatorTimeout)
				if evaluationErr != nil {
					result.Failures = append(result.Failures, Failure{Stage: "evaluate", InstanceID: task.InstanceID, Agent: agent, Code: "evaluation_failed", EvidencePath: evaluationLogPath(task.InstanceID, agent)})
					continue
				}
				result.Evaluations = append(result.Evaluations, evaluation)
			}
			result.Aggregates = aggregateAll(result.Tasks, result.Runs, result.Evaluations)
			result.SharedPass = sharedPassedTasks(result.Tasks, result.Evaluations)
			if err := persist(); err != nil {
				return outcome, err
			}
		}
	}
	completed := options.Now().UTC()
	result.CompletedAt = &completed
	result.Aggregates = aggregateAll(result.Tasks, result.Runs, result.Evaluations)
	result.SharedPass = sharedPassedTasks(result.Tasks, result.Evaluations)
	var baselineErr error
	if prepareErr == nil && options.WithCodex && codexBaselineCoverage(result) {
		result.CodexBaseline, baselineErr = freezeCodexBaseline(result, resultsRoot, runRoot, language)
		if baselineErr != nil {
			result.Failures = append(result.Failures, Failure{Stage: "codex_baseline", Code: "baseline_write_failed"})
		}
	}
	result.Status = "partial"
	if prepareErr == nil && baselineErr == nil && len(result.Failures) == 0 && completeCoverage(result) && result.CodexBaseline.SnapshotSHA256 != "" {
		result.Status = "complete"
	}
	if err := persist(); err != nil {
		return outcome, err
	}
	if err := GenerateReport(outcome.ResultPath, outcome.ReportPath, language); err != nil {
		return outcome, err
	}
	outcome.Complete = result.Status == "complete"
	if prepareErr != nil {
		return outcome, prepareErr
	}
	if baselineErr != nil {
		return outcome, baselineErr
	}
	return outcome, nil
}

type agentResult struct {
	agent    string
	run      RunSummary
	err      error
	evidence string
}

func activeAgents(withCodex bool) []string {
	if withCodex {
		return []string{"codex", "luban"}
	}
	return []string{"luban"}
}

func runAgents(ctx context.Context, options Options, executor Executor, runRoot string, task TaskSelection) []agentResult {
	agents := activeAgents(options.WithCodex)
	values := make([]agentResult, len(agents))
	var wait sync.WaitGroup
	for index, agent := range agents {
		progress(options, i18n.KeyLocalBenchmarkRunningAgent, agent, task.InstanceID)
		wait.Add(1)
		go func(index int, agent string) {
			defer wait.Done()
			run, err := executor.RunAgent(ctx, runRoot, task, agent, options.AgentTimeout)
			values[index] = agentResult{agent: agent, run: run, err: err, evidence: agentLogPath(task.InstanceID, agent)}
		}(index, agent)
	}
	wait.Wait()
	return values
}

func progress(options Options, key i18n.Key, arguments ...any) {
	if options.Progress != nil {
		options.Progress(key, arguments...)
	}
}

func loadSelection(taskSize int) ([]TaskSelection, error) {
	raw, err := runtimeAssets.ReadFile("catalog/representative5.json")
	if err != nil {
		return nil, err
	}
	var catalog []catalogTask
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, err
	}
	byID := make(map[string]catalogTask, len(catalog))
	for _, task := range catalog {
		byID[task.InstanceID] = task
	}
	result := make([]TaskSelection, 0, taskSize)
	for _, id := range representativeOrder[:taskSize] {
		task, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("representative task %s is absent from the embedded catalog", id)
		}
		result = append(result, TaskSelection{InstanceID: task.InstanceID, Language: task.Language})
	}
	return result, nil
}

func createRunRoot(resultsRoot string, started time.Time, taskSize int) (string, string, error) {
	if resultsRoot == "" {
		resultsRoot = "benchmark-results"
	}
	absolute, err := filepath.Abs(resultsRoot)
	if err != nil {
		return "", "", err
	}
	dateRoot := filepath.Join(absolute, "agentic-"+started.Format("2006-01-02"))
	baseID := fmt.Sprintf("run-%s-n%d", started.Format("20060102-150405"), taskSize)
	if err := os.MkdirAll(dateRoot, 0o755); err != nil {
		return "", "", err
	}
	for suffix := 1; suffix < 1000; suffix++ {
		runID := baseID
		if suffix > 1 {
			runID = fmt.Sprintf("%s-%d", baseID, suffix)
		}
		runRoot := filepath.Join(dateRoot, runID)
		if err := os.Mkdir(runRoot, 0o755); err == nil {
			return runRoot, runID, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", "", err
		}
	}
	return "", "", errors.New("could not allocate a unique benchmark run directory")
}

func writeJSONAtomic(path string, value any) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".benchmark-json-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(name)
		}
	}()
	encoder := json.NewEncoder(temporary)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func hasRun(runs []RunSummary, taskID, agent string) bool {
	for _, run := range runs {
		if run.InstanceID == taskID && run.Agent == agent {
			return true
		}
	}
	return false
}

func completeCoverage(result BenchmarkResult) bool {
	want := len(result.Tasks) * 2
	return len(result.Runs) == want && len(result.Evaluations) == want && len(result.GoldEvaluations) == len(result.Tasks)
}

func aggregateAll(tasks []TaskSelection, runs []RunSummary, evaluations []Evaluation) []Aggregate {
	return []Aggregate{
		aggregateSubset("codex", tasks, runs, evaluations, nil),
		aggregateSubset("luban", tasks, runs, evaluations, nil),
	}
}

func aggregateSubset(agent string, tasks []TaskSelection, runs []RunSummary, evaluations []Evaluation, subset map[string]struct{}) Aggregate {
	result := Aggregate{Agent: agent, ToolEventsByType: map[string]int{}}
	selected := func(taskID string) bool {
		if subset == nil {
			return true
		}
		_, ok := subset[taskID]
		return ok
	}
	for _, task := range tasks {
		if selected(task.InstanceID) {
			result.TasksSelected++
		}
	}
	for _, run := range runs {
		if run.Agent != agent || !selected(run.InstanceID) {
			continue
		}
		result.RunsObserved++
		result.TaskDurationSeconds += run.ElapsedSeconds
		result.ProviderRequestSeconds += run.ProviderRequestSeconds
		result.LLMCalls += run.LLMCalls
		result.LLMSuccessfulCalls += run.LLMSuccessfulCalls
		result.LLMFailedCalls += run.LLMFailedCalls
		result.InputTokens += run.Usage.InputTokens
		result.CachedInputTokens += run.Usage.CachedInputTokens
		result.CacheWriteInputTokens += run.Usage.CacheCreationInputTokens
		result.OutputTokens += run.Usage.OutputTokens
		result.EstimatedCostUSD += run.EstimatedCostUSD
		result.ToolEvents += run.ToolEvents
		for name, count := range run.ToolEventsByType {
			result.ToolEventsByType[name] += count
		}
	}
	for _, evaluation := range evaluations {
		if evaluation.Agent != agent || !selected(evaluation.InstanceID) {
			continue
		}
		result.EvaluationsObserved++
		if evaluation.Resolved {
			result.Resolved++
		}
	}
	if result.InputTokens > 0 {
		result.CacheHitRatio = float64(result.CachedInputTokens) / float64(result.InputTokens)
	}
	return result
}

func sharedPassedTasks(tasks []TaskSelection, evaluations []Evaluation) []TaskSelection {
	passed := make(map[string]map[string]bool)
	for _, evaluation := range evaluations {
		if passed[evaluation.InstanceID] == nil {
			passed[evaluation.InstanceID] = map[string]bool{}
		}
		passed[evaluation.InstanceID][evaluation.Agent] = evaluation.Resolved
	}
	result := make([]TaskSelection, 0)
	for _, task := range tasks {
		if passed[task.InstanceID]["codex"] && passed[task.InstanceID]["luban"] {
			result = append(result, task)
		}
	}
	return result
}

func agentLogPath(taskID, agent string) string {
	return filepath.ToSlash(filepath.Join("raw", "runs", taskID, agent, "worker.log"))
}
func evaluationLogPath(taskID, agent string) string {
	return filepath.ToSlash(filepath.Join("raw", "evaluation", taskID, agent, "worker.log"))
}

type LocalExecutor struct {
	Python      string
	CodexBinary string
	LubanBinary string
	RuntimeRoot string
	CatalogPath string
	WorkRoot    string
	Identities  map[string]BinaryIdentity
}

func NewLocalExecutor() *LocalExecutor { return &LocalExecutor{} }

func (executor *LocalExecutor) Prepare(ctx context.Context, runRoot, repositoryRoot string, withCodex bool) (PreparedEnvironment, error) {
	if err := os.MkdirAll(filepath.Join(runRoot, "raw", "setup"), 0o755); err != nil {
		return PreparedEnvironment{}, err
	}
	logPath := filepath.Join(runRoot, "execution.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return PreparedEnvironment{}, err
	}
	defer logFile.Close()
	executor.Python, err = exec.LookPath("python3")
	if err != nil {
		return PreparedEnvironment{}, logCommandError(logFile, "python_lookup_failed", err)
	}
	if withCodex {
		executor.CodexBinary, err = exec.LookPath("codex")
		if err != nil {
			return PreparedEnvironment{}, logCommandError(logFile, "codex_lookup_failed", err)
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		return PreparedEnvironment{}, logCommandError(logFile, "git_lookup_failed", err)
	}
	executor.RuntimeRoot = filepath.Join(runRoot, ".runtime")
	if err := os.MkdirAll(executor.RuntimeRoot, 0o700); err != nil {
		return PreparedEnvironment{}, err
	}
	for _, name := range []string{"worker/run_worker.py", "worker/evaluate_worker.py"} {
		raw, readErr := runtimeAssets.ReadFile(name)
		if readErr != nil {
			return PreparedEnvironment{}, readErr
		}
		target := filepath.Join(executor.RuntimeRoot, filepath.Base(name))
		if writeErr := os.WriteFile(target, raw, 0o700); writeErr != nil {
			return PreparedEnvironment{}, writeErr
		}
	}
	catalogRaw, err := runtimeAssets.ReadFile("catalog/representative5.json")
	if err != nil {
		return PreparedEnvironment{}, err
	}
	executor.CatalogPath = filepath.Join(executor.RuntimeRoot, "representative5.json")
	if err := os.WriteFile(executor.CatalogPath, catalogRaw, 0o600); err != nil {
		return PreparedEnvironment{}, err
	}
	executor.LubanBinary = filepath.Join(executor.RuntimeRoot, "luban")
	buildLog := filepath.Join(runRoot, "raw", "setup", "go-build.log")
	if err := runLogged(ctx, repositoryRoot, buildLog, "go", "build", "-o", executor.LubanBinary, "."); err != nil {
		return PreparedEnvironment{}, err
	}
	executor.WorkRoot = filepath.Join(os.TempDir(), "luban-agent-benchmark-work", filepath.Base(runRoot))
	if err := os.MkdirAll(executor.WorkRoot, 0o700); err != nil {
		return PreparedEnvironment{}, err
	}
	preflightPath := filepath.Join(runRoot, "raw", "setup", "provider.json")
	preflightLog := filepath.Join(runRoot, "raw", "setup", "provider-preflight.log")
	if err := executor.runWorker(ctx, repositoryRoot, preflightLog, filepath.Join(executor.RuntimeRoot, "run_worker.py"), "--preflight", "--output", preflightPath); err != nil {
		return PreparedEnvironment{}, err
	}
	evaluatorPath := filepath.Join(runRoot, "raw", "setup", "evaluator.json")
	evaluatorLog := filepath.Join(runRoot, "raw", "setup", "evaluator-preflight.log")
	if err := executor.runWorker(ctx, repositoryRoot, evaluatorLog, filepath.Join(executor.RuntimeRoot, "evaluate_worker.py"), "--preflight", "--output", evaluatorPath); err != nil {
		return PreparedEnvironment{}, err
	}
	var provider struct {
		GatewayOrigin string `json:"gateway_origin"`
	}
	if err := decodeJSONFile(preflightPath, &provider); err != nil {
		return PreparedEnvironment{}, err
	}
	var evaluator struct {
		Engine string `json:"engine"`
	}
	if err := decodeJSONFile(evaluatorPath, &evaluator); err != nil {
		return PreparedEnvironment{}, err
	}
	lubanIdentity, err := hashBinary("luban", executor.LubanBinary)
	if err != nil {
		return PreparedEnvironment{}, err
	}
	lubanIdentity.Version = "workspace-build"
	binaries := []BinaryIdentity{lubanIdentity}
	if withCodex {
		codexIdentity, identityErr := hashBinary("codex", executor.CodexBinary)
		if identityErr != nil {
			return PreparedEnvironment{}, identityErr
		}
		codexIdentity.Version, identityErr = binaryVersion(ctx, executor.CodexBinary)
		if identityErr != nil {
			return PreparedEnvironment{}, identityErr
		}
		binaries = append([]BinaryIdentity{codexIdentity}, binaries...)
	}
	executor.Identities = make(map[string]BinaryIdentity, len(binaries))
	for _, identity := range binaries {
		executor.Identities[identity.Name] = identity
	}
	return PreparedEnvironment{GatewayOrigin: provider.GatewayOrigin, EvaluatorEngine: evaluator.Engine, Binaries: binaries}, nil
}

func (executor *LocalExecutor) RunAgent(ctx context.Context, runRoot string, task TaskSelection, agent string, timeout int) (RunSummary, error) {
	root := filepath.Join(runRoot, "raw", "runs", task.InstanceID, agent)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return RunSummary{}, err
	}
	outputPath := filepath.Join(root, "summary.json")
	logPath := filepath.Join(root, "worker.log")
	args := []string{"--catalog", executor.CatalogPath, "--result-root", runRoot, "--work-root", executor.WorkRoot, "--task", task.InstanceID, "--agent", agent, "--luban-bin", executor.LubanBinary, "--timeout", fmt.Sprint(timeout), "--output", outputPath}
	if agent == "codex" {
		args = append(args, "--codex-bin", executor.CodexBinary)
	}
	if err := executor.runWorker(ctx, "", logPath, filepath.Join(executor.RuntimeRoot, "run_worker.py"), args...); err != nil {
		return RunSummary{}, err
	}
	var result RunSummary
	if err := decodeJSONFile(outputPath, &result); err != nil {
		return RunSummary{}, err
	}
	if identity, ok := executor.Identities[agent]; ok {
		result.Binary = identity
	}
	return result, nil
}

func (executor *LocalExecutor) Evaluate(ctx context.Context, runRoot string, task TaskSelection, agent string, timeout int) (Evaluation, error) {
	root := filepath.Join(runRoot, "raw", "evaluation", task.InstanceID, agent)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Evaluation{}, err
	}
	outputPath := filepath.Join(root, "report.json")
	logPath := filepath.Join(root, "worker.log")
	args := []string{"--catalog", executor.CatalogPath, "--result-root", runRoot, "--task", task.InstanceID, "--agent", agent, "--timeout", fmt.Sprint(timeout), "--output", outputPath}
	if err := executor.runWorker(ctx, "", logPath, filepath.Join(executor.RuntimeRoot, "evaluate_worker.py"), args...); err != nil {
		return Evaluation{}, err
	}
	var result Evaluation
	if err := decodeJSONFile(outputPath, &result); err != nil {
		return Evaluation{}, err
	}
	return result, nil
}

func (executor *LocalExecutor) runWorker(ctx context.Context, directory, logPath, script string, arguments ...string) error {
	args := append([]string{script}, arguments...)
	return runLogged(ctx, directory, logPath, executor.Python, args...)
}

func runLogged(ctx context.Context, directory, logPath, executable string, arguments ...string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	command := exec.CommandContext(ctx, executable, arguments...)
	if directory != "" {
		command.Dir = directory
	}
	command.Stdout, command.Stderr = logFile, logFile
	return command.Run()
}

func logCommandError(writer io.Writer, code string, err error) error {
	_, _ = fmt.Fprintf(writer, "%s: %v\n", code, err)
	return err
}

func decodeJSONFile(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("JSON file contains trailing content")
	}
	return nil
}

func hashBinary(name, path string) (BinaryIdentity, error) {
	file, err := os.Open(path)
	if err != nil {
		return BinaryIdentity{}, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return BinaryIdentity{}, err
	}
	return BinaryIdentity{Name: name, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func binaryVersion(ctx context.Context, path string) (string, error) {
	command := exec.CommandContext(ctx, path, "--version")
	raw, err := command.CombinedOutput()
	if err != nil {
		return "", err
	}
	version := strings.Join(strings.Fields(string(raw)), " ")
	if version == "" {
		return "", errors.New("binary version output is empty")
	}
	if len(version) > 200 {
		version = version[:200]
	}
	return version, nil
}

// KeyTaskSizeRange keeps the local package independent of stringly typed keys.
func KeyTaskSizeRange() i18n.Key { return i18n.KeyLocalBenchmarkTaskSizeRange }
