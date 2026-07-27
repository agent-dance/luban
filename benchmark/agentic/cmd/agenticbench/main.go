package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
	"github.com/agent-dance/luban/benchmark/agentic/pierbackend"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/i18n"
)

type options struct {
	manifestPath string
	backendPath  string
	workDir      string
	execute      bool
}

type preflightReport struct {
	SchemaVersion      string                  `json:"schema_version"`
	Command            string                  `json:"command"`
	ExternalExecution  bool                    `json:"external_execution"`
	ManifestSHA256     string                  `json:"manifest_sha256"`
	InventorySHA256    string                  `json:"inventory_sha256"`
	SelectedTaskIDs    []string                `json:"selected_task_ids"`
	Plan               harness.RunPlan         `json:"plan"`
	PlanSHA256         string                  `json:"plan_sha256"`
	Backend            harness.BackendSnapshot `json:"backend"`
	Agents             []harness.AgentSnapshot `json:"agents"`
	MaxAgentSeconds    int64                   `json:"max_agent_seconds"`
	MaxSetupSeconds    int64                   `json:"max_setup_seconds"`
	MaxVerifierSeconds int64                   `json:"max_verifier_seconds"`
	MaxTeardownSeconds int64                   `json:"max_teardown_seconds"`
	MaxParallelPairs   int                     `json:"max_parallel_pairs"`
	Pricing            harness.PricingCatalog  `json:"pricing"`
	ArtifactRoot       string                  `json:"artifact_root"`
}

type commandResult struct {
	SchemaVersion           string                   `json:"schema_version"`
	Command                 string                   `json:"command"`
	Status                  harness.ExperimentStatus `json:"status,omitempty"`
	ArtifactRoot            string                   `json:"artifact_root,omitempty"`
	OraclePassed            int                      `json:"oracle_passed,omitempty"`
	Scorecard               *harness.Scorecard       `json:"scorecard,omitempty"`
	Ledger                  *harness.ArtifactLedger  `json:"ledger,omitempty"`
	LockTasks               int                      `json:"lock_tasks,omitempty"`
	InventorySHA            string                   `json:"inventory_sha256,omitempty"`
	ManifestMatch           bool                     `json:"manifest_match,omitempty"`
	DatasetTreeSHA256       string                   `json:"dataset_tree_sha256,omitempty"`
	EvaluatorTreeSHA256     string                   `json:"evaluator_tree_sha256,omitempty"`
	EvaluatorManifestSHA256 string                   `json:"evaluator_manifest_sha256,omitempty"`
}

func main() {
	commandIO := cli.ProcessCommandIO()
	os.Exit(runMain(context.Background(), os.Args[1:], commandIO.Stdout, commandIO.Stderr))
}

func runMain(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	language := i18n.DetectOrLoadLanguage()
	if len(arguments) == 1 && (arguments[0] == "-h" || arguments[0] == "--help") {
		_, _ = fmt.Fprintln(stdout, i18n.Text(language, i18n.KeyBenchmarkCLIUsage))
		return 0
	}
	if len(arguments) == 0 {
		_, _ = fmt.Fprintln(stderr, i18n.Text(language, i18n.KeyBenchmarkCLIUsage))
		return 2
	}
	command := arguments[0]
	allowed := []string{"ledger", "lock", "oracle", "preflight", "resume", "run", "score"}
	if !slices.Contains(allowed, command) {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyBenchmarkCLIUnknownCommand, command))
		_, _ = fmt.Fprintln(stderr, i18n.Text(language, i18n.KeyBenchmarkCLIUsage))
		return 2
	}
	parsed, help, err := parseOptions(language, command, arguments[1:])
	if help {
		_, _ = fmt.Fprintln(stdout, i18n.Text(language, i18n.KeyBenchmarkCLIUsage))
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyBenchmarkCLIFailed, err))
		return 2
	}
	if err := executeCommand(ctx, command, parsed, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyBenchmarkCLIFailed, err))
		return 1
	}
	return 0
}

func parseOptions(language i18n.Language, command string, arguments []string) (options, bool, error) {
	result := options{
		manifestPath: os.Getenv("AGENTIC_BENCH_MANIFEST"),
		backendPath:  os.Getenv("AGENTIC_BENCH_BACKEND_CONFIG"),
		workDir:      os.Getenv("AGENTIC_BENCH_WORK_DIR"),
	}
	set := flag.NewFlagSet(command, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&result.manifestPath, "manifest", result.manifestPath, i18n.Text(language, i18n.KeyBenchmarkCLIManifestFlag))
	set.StringVar(&result.backendPath, "backend-config", result.backendPath, i18n.Text(language, i18n.KeyBenchmarkCLIBackendFlag))
	set.StringVar(&result.workDir, "work-dir", result.workDir, i18n.Text(language, i18n.KeyBenchmarkCLIWorkDirFlag))
	set.BoolVar(&result.execute, "execute", false, i18n.Text(language, i18n.KeyBenchmarkCLIExecuteFlag))
	if err := set.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, true, nil
		}
		return options{}, false, err
	}
	if set.NArg() != 0 {
		return options{}, false, i18nSemanticError(language, i18n.KeyBenchmarkCLIUsage)
	}
	if result.manifestPath == "" || result.backendPath == "" || result.workDir == "" {
		return options{}, false, i18nSemanticError(language, i18n.KeyBenchmarkCLIMissingConfig)
	}
	var err error
	result.manifestPath, err = filepath.Abs(result.manifestPath)
	if err != nil {
		return options{}, false, err
	}
	result.backendPath, err = filepath.Abs(result.backendPath)
	if err != nil {
		return options{}, false, err
	}
	result.workDir, err = filepath.Abs(result.workDir)
	return result, false, err
}

func executeCommand(ctx context.Context, command string, options options, output io.Writer) error {
	loaded, err := harness.LoadManifest(options.manifestPath)
	if err != nil {
		return err
	}
	config, err := pierbackend.LoadConfigFile(options.backendPath)
	if err != nil {
		return err
	}
	if command == "lock" {
		if !options.execute {
			return encodeJSON(output, commandResult{SchemaVersion: "agentic-bench/command-result-v1", Command: command})
		}
		if _, err := pierbackend.New(config); err != nil {
			return err
		}
		lock, inventorySHA, err := pierbackend.GenerateInventoryLock(ctx, config, loaded.Manifest)
		if err != nil {
			return err
		}
		datasetTree, err := harness.HashTree(filepath.Join(config.DatasetRepositoryRoot, loaded.Manifest.Dataset.Root))
		if err != nil {
			return err
		}
		evaluatorTree, err := harness.HashTree(filepath.Join(config.EvaluatorRepositoryRoot, loaded.Manifest.Evaluator.Root))
		if err != nil {
			return err
		}
		evaluatorManifestSHA, err := harness.HashFile(config.EvaluatorManifestPath)
		if err != nil {
			return err
		}
		return encodeJSON(output, commandResult{
			SchemaVersion: "agentic-bench/command-result-v1", Command: command,
			LockTasks: len(lock.Tasks), InventorySHA: inventorySHA,
			ManifestMatch:     inventorySHA == loaded.Manifest.Dataset.ManifestSHA256,
			DatasetTreeSHA256: datasetTree.SHA256, EvaluatorTreeSHA256: evaluatorTree.SHA256,
			EvaluatorManifestSHA256: evaluatorManifestSHA,
		})
	}
	if command == "score" || command == "ledger" {
		return executeArtifactCommand(command, options, loaded, output)
	}
	backend, err := pierbackend.New(config)
	if err != nil {
		return err
	}
	report, err := buildPreflight(ctx, command, options, loaded, backend)
	if err != nil {
		return err
	}
	if err := encodeJSON(output, report); err != nil {
		return err
	}
	if command == "preflight" || !options.execute {
		return nil
	}
	statePath := filepath.Join(report.ArtifactRoot, loaded.Manifest.Artifacts.StateRelativePath)
	_, stateErr := os.Stat(statePath)
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return stateErr
	}
	if command == "run" && stateErr == nil {
		return i18nSemanticError(i18n.DetectOrLoadLanguage(), i18n.KeyBenchmarkCLIRunStateExists)
	}
	if command == "resume" && errors.Is(stateErr, os.ErrNotExist) {
		return i18nSemanticError(i18n.DetectOrLoadLanguage(), i18n.KeyBenchmarkCLIResumeMissing)
	}
	runner := harness.Runner{
		Loaded: loaded, Backend: backend, WorkDir: options.workDir,
		HostEnvironment: os.Environ(), StopAfterOracles: command == "oracle",
	}
	state, _, err := runner.Run(ctx)
	if err != nil {
		return err
	}
	result := commandResult{SchemaVersion: "agentic-bench/command-result-v1", Command: command, Status: state.Status, ArtifactRoot: report.ArtifactRoot}
	for _, oracle := range state.Oracle {
		if oracle.Validated {
			result.OraclePassed++
		}
	}
	if state.Status == harness.ExperimentComplete {
		score, scoreErr := harness.ScoreExperimentForManifest(loaded, state, report.Plan)
		if scoreErr != nil {
			return scoreErr
		}
		result.Scorecard = &score
	}
	return encodeJSON(output, result)
}

func buildPreflight(ctx context.Context, command string, options options, loaded harness.LoadedManifest, backend *pierbackend.Backend) (preflightReport, error) {
	artifactRoot := filepath.Join(options.workDir, filepath.FromSlash(loaded.Manifest.Artifacts.Root))
	if err := backend.BindInventoryLockArchive(ctx, filepath.Join(artifactRoot, harness.InventoryLockArchiveRelativePath)); err != nil {
		return preflightReport{}, err
	}
	snapshot, err := backend.Preflight(ctx, loaded.Manifest)
	if err != nil {
		return preflightReport{}, err
	}
	inventory, err := backend.Inventory(ctx, loaded.Manifest.Dataset)
	if err != nil {
		return preflightReport{}, err
	}
	selected, err := harness.SelectTasks(loaded.Manifest.Selection, inventory)
	if err != nil {
		return preflightReport{}, err
	}
	plan, err := harness.BuildPlan(loaded.SHA256, loaded.Manifest, selected)
	if err != nil {
		return preflightReport{}, err
	}
	planSHA, err := harness.HashCanonical(plan)
	if err != nil {
		return preflightReport{}, err
	}
	agents := make([]harness.AgentSnapshot, 0, len(loaded.Manifest.Agents))
	for _, agent := range loaded.Manifest.Agents {
		agentSnapshot, snapshotErr := harness.SnapshotAgent(ctx, agent, time.Now())
		if snapshotErr != nil {
			return preflightReport{}, snapshotErr
		}
		agents = append(agents, agentSnapshot)
	}
	selectedIDs := make([]string, 0, len(selected))
	for _, task := range selected {
		selectedIDs = append(selectedIDs, task.ID)
	}
	trials := int64(len(selected) + len(plan.Entries))
	return preflightReport{
		SchemaVersion: "agentic-bench/preflight-v1", Command: command,
		ExternalExecution: options.execute && command != "preflight", ManifestSHA256: loaded.SHA256,
		InventorySHA256: loaded.Manifest.Dataset.ManifestSHA256, SelectedTaskIDs: selectedIDs,
		Plan: plan, PlanSHA256: planSHA, Backend: snapshot, Agents: agents,
		MaxAgentSeconds:    int64(len(plan.Entries) * loaded.Manifest.Timeouts.AgentSeconds),
		MaxSetupSeconds:    trials * int64(loaded.Manifest.Timeouts.SetupSeconds),
		MaxVerifierSeconds: trials * int64(loaded.Manifest.Timeouts.VerifierSeconds),
		MaxTeardownSeconds: trials * int64(loaded.Manifest.Timeouts.TeardownSeconds),
		MaxParallelPairs:   loaded.Manifest.Scheduling.MaxParallelPairs,
		Pricing:            loaded.Manifest.Pricing, ArtifactRoot: artifactRoot,
	}, nil
}

func executeArtifactCommand(command string, options options, loaded harness.LoadedManifest, output io.Writer) error {
	root := filepath.Join(options.workDir, filepath.FromSlash(loaded.Manifest.Artifacts.Root))
	planRaw, err := os.ReadFile(filepath.Join(root, "plan.json"))
	if err != nil {
		return err
	}
	var plan harness.RunPlan
	if err := json.Unmarshal(planRaw, &plan); err != nil {
		return err
	}
	planSHA, err := harness.HashCanonical(plan)
	if err != nil {
		return err
	}
	state, err := harness.LoadState(filepath.Join(root, loaded.Manifest.Artifacts.StateRelativePath), loaded.SHA256, planSHA)
	if err != nil {
		return err
	}
	if err := validateArtifactInventoryLock(root, loaded, state); err != nil {
		return err
	}
	if command == "score" {
		scorecard, err := harness.ScoreExperimentForManifest(loaded, state, plan)
		if err != nil {
			return err
		}
		return encodeJSON(output, commandResult{SchemaVersion: "agentic-bench/command-result-v1", Command: command, Status: state.Status, ArtifactRoot: root, Scorecard: &scorecard})
	}
	ledger, err := harness.BuildArtifactLedger(root, loaded.Manifest.Artifacts.LedgerRelativePath, loaded.SHA256)
	if err != nil {
		return err
	}
	if options.execute {
		if err := harness.WriteJSONAtomic(filepath.Join(root, loaded.Manifest.Artifacts.LedgerRelativePath), ledger, 0o644); err != nil {
			return err
		}
	}
	return encodeJSON(output, commandResult{SchemaVersion: "agentic-bench/command-result-v1", Command: command, Status: state.Status, ArtifactRoot: root, Ledger: &ledger})
}

func validateArtifactInventoryLock(root string, loaded harness.LoadedManifest, state harness.ExperimentState) error {
	lock := state.Backend.InventoryLock
	inventoryLockPath := filepath.Join(root, filepath.FromSlash(lock.RelativePath))
	archivedInventory, err := harness.ValidateInventoryLockArchive(inventoryLockPath, lock)
	if err != nil {
		return err
	}
	inventorySHA, err := harness.HashTaskInventory(archivedInventory)
	if err != nil {
		return err
	}
	manifest := loaded.Manifest
	if inventorySHA != manifest.Dataset.ManifestSHA256 ||
		lock.TaskInventorySHA256 != manifest.Dataset.ManifestSHA256 ||
		lock.DatasetCommit != manifest.Dataset.Commit ||
		lock.Coverage != state.Backend.InventoryCoverage ||
		lock.TaskCount != state.Backend.InventoryTaskCount ||
		lock.UniverseTaskCount != state.Backend.UniverseTaskCount ||
		lock.UniverseTaskCount != manifest.Selection.ExpectedTaskCount {
		return errors.New("archived inventory lock differs from the manifest or backend snapshot")
	}
	switch manifest.Selection.Mode {
	case "full", "sample":
		if lock.Coverage != "full" || lock.TaskCount != lock.UniverseTaskCount {
			return errors.New("archived inventory lock does not preserve the full task universe")
		}
	case "tasks":
		if lock.Coverage != "tasks" || lock.TaskCount != len(manifest.Selection.TaskIDs) {
			return errors.New("archived inventory lock does not preserve the explicit task selection")
		}
	default:
		return errors.New("archived inventory lock has unsupported selection coverage")
	}
	return nil
}

func encodeJSON(writer io.Writer, value any) error {
	return json.NewEncoder(writer).Encode(value)
}

func i18nSemanticError(language i18n.Language, key i18n.Key) error {
	return errors.New(i18n.Text(language, key))
}
