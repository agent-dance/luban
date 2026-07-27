// Package pilot provides a development-only, fail-closed five-task runner.
//
// It deliberately does not use harness.Runner or any formal scorer. The Pier
// backend remains the sole owner of task materialization, model execution, and
// the separate verifier. This package only schedules the frozen pilot pair and
// persists a non-formal, immutable-attempt development ledger.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

const (
	PreflightSchemaVersion       = "agentic-bench/development-pilot-preflight-v1"
	LedgerSchemaVersion          = "agentic-bench/development-pilot-ledger-v1"
	OracleReceiptSchemaVersion   = "agentic-bench/development-pilot-oracle-v1"
	AttemptReceiptSchemaVersion  = "agentic-bench/development-pilot-attempt-v1"
	LedgerRelativePath           = "development-pilot-ledger.json"
	PlanRelativePath             = "development-pilot-plan.json"
	ManifestRelativePath         = "development-pilot-manifest.json"
	BackendRelativePath          = "development-pilot-backend.json"
	AttemptReceiptName           = "development-attempt-receipt.json"
	OracleReceiptName            = "development-oracle-receipt.json"
	GuestAgentReceiptRelative    = "agent/pilot-guest-storage-receipt.json"
	GuestVerifierReceiptRelative = "verifier/pilot-guest-storage-receipt.json"
)

var ExactTaskIDs = []string{
	"abs-module-cache-flags",
	"adaptix-name-mapping-aliases",
	"cliffy-config-file-parsing",
	"wasmi-trap-coredumps",
	"yjs-map-conflict-detection",
}

type Runner struct {
	Loaded                    harness.LoadedManifest
	Backend                   harness.Backend
	WorkDir                   string
	HostEnvironment           []string
	HostStorageReceiptPath    string
	GuestStoragePreflightPath string
	PairLimit                 int
	Now                       func() time.Time
}

type developmentPreflightBackend interface {
	DevelopmentPreflight(context.Context, harness.Manifest) (harness.BackendSnapshot, error)
}

type StoragePaths struct {
	FormalCompatible                    bool     `json:"formal_compatible"`
	HostReceiptPath                     string   `json:"host_receipt_path"`
	GuestPreflightReceiptPath           string   `json:"guest_preflight_receipt_path"`
	GuestPreflightReceiptSHA256         string   `json:"guest_preflight_receipt_sha256,omitempty"`
	PerAttemptGuestReceiptRelativePaths []string `json:"per_attempt_guest_receipt_relative_paths"`
}

type Preflight struct {
	SchemaVersion     string                  `json:"schema_version"`
	FormalCompatible  bool                    `json:"formal_compatible"`
	ExternalExecution bool                    `json:"external_execution"`
	ManifestSHA256    string                  `json:"manifest_sha256"`
	InventorySHA256   string                  `json:"inventory_sha256"`
	SelectedTaskIDs   []string                `json:"selected_task_ids"`
	Plan              harness.RunPlan         `json:"plan"`
	PlanSHA256        string                  `json:"plan_sha256"`
	Backend           harness.BackendSnapshot `json:"backend"`
	ArtifactRoot      string                  `json:"artifact_root"`
	Storage           StoragePaths            `json:"development_storage"`
	Timeouts          harness.TimeoutSpec     `json:"timeouts"`
	Resources         harness.ResourceSpec    `json:"resources"`
}

type OracleReceipt struct {
	SchemaVersion    string                     `json:"schema_version"`
	FormalCompatible bool                       `json:"formal_compatible"`
	ManifestSHA256   string                     `json:"manifest_sha256"`
	TaskID           string                     `json:"task_id"`
	SealedAt         time.Time                  `json:"sealed_at"`
	Verification     harness.VerificationResult `json:"verification"`
}

type AttemptReceipt struct {
	SchemaVersion    string                     `json:"schema_version"`
	FormalCompatible bool                       `json:"formal_compatible"`
	ManifestSHA256   string                     `json:"manifest_sha256"`
	PlanSHA256       string                     `json:"plan_sha256"`
	RunKey           string                     `json:"run_key"`
	Entry            harness.PlanEntry          `json:"entry"`
	Model            harness.ModelRequestSpec   `json:"model"`
	SealedAt         time.Time                  `json:"sealed_at"`
	Execution        harness.AgentExecution     `json:"execution"`
	Verification     harness.VerificationResult `json:"verification"`
	Metrics          harness.UsageMetrics       `json:"metrics"`
}

type OracleRecord struct {
	FormalCompatible bool                       `json:"formal_compatible"`
	TaskID           string                     `json:"task_id"`
	ReceiptPath      string                     `json:"receipt_path"`
	ReceiptSHA256    string                     `json:"receipt_sha256"`
	Verification     harness.VerificationResult `json:"verification"`
}

type RunRecord struct {
	FormalCompatible       bool                        `json:"formal_compatible"`
	Entry                  harness.PlanEntry           `json:"entry"`
	State                  string                      `json:"state"`
	AttemptNumber          int                         `json:"attempt_number"`
	ReservedAt             time.Time                   `json:"reserved_at,omitempty"`
	SealedAt               time.Time                   `json:"sealed_at,omitempty"`
	ArtifactDir            string                      `json:"artifact_dir"`
	ReceiptPath            string                      `json:"receipt_path,omitempty"`
	ReceiptSHA256          string                      `json:"receipt_sha256,omitempty"`
	Model                  harness.ModelRequestSpec    `json:"model"`
	Execution              *harness.AgentExecution     `json:"execution,omitempty"`
	Verification           *harness.VerificationResult `json:"verification,omitempty"`
	NormalizedEvidencePath string                      `json:"normalized_evidence_path,omitempty"`
	Metrics                *harness.UsageMetrics       `json:"metrics,omitempty"`
}

type CompletionMarker struct {
	SchemaVersion    string `json:"schema_version"`
	FormalCompatible bool   `json:"formal_compatible"`
	Complete         bool   `json:"complete"`
	SealedRunCount   int    `json:"sealed_run_count"`
	ExpectedRunCount int    `json:"expected_run_count"`
}

type Ledger struct {
	SchemaVersion    string                  `json:"schema_version"`
	FormalCompatible bool                    `json:"formal_compatible"`
	Status           string                  `json:"status"`
	ManifestSHA256   string                  `json:"manifest_sha256"`
	PlanSHA256       string                  `json:"plan_sha256"`
	StartedAt        time.Time               `json:"started_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
	CompletedAt      *time.Time              `json:"completed_at,omitempty"`
	Backend          harness.BackendSnapshot `json:"backend"`
	Storage          StoragePaths            `json:"development_storage"`
	Oracle           map[string]OracleRecord `json:"oracle"`
	Runs             map[string]RunRecord    `json:"runs"`
	CompletionMarker *CompletionMarker       `json:"completion_marker,omitempty"`
}

func (runner Runner) Preflight(ctx context.Context) (Preflight, error) {
	return runner.prepare(ctx, false)
}

func (runner Runner) Run(ctx context.Context) (Ledger, Preflight, error) {
	prepared, err := runner.prepare(ctx, true)
	if err != nil {
		return Ledger{}, Preflight{}, err
	}
	ledger, err := runner.loadOrCreateLedger(prepared)
	if err != nil {
		return Ledger{}, prepared, err
	}
	if ledger.Status == "complete" {
		return ledger, prepared, nil
	}
	selectedByID := make(map[string]harness.Task, len(ExactTaskIDs))
	inventory, err := runner.Backend.Inventory(ctx, runner.Loaded.Manifest.Dataset)
	if err != nil {
		return ledger, prepared, err
	}
	selected, err := harness.SelectTasks(runner.Loaded.Manifest.Selection, inventory)
	if err != nil {
		return ledger, prepared, err
	}
	for _, task := range selected {
		selectedByID[task.ID] = task
	}
	if err := runner.resumeSealedReceipts(&ledger, prepared); err != nil {
		return ledger, prepared, err
	}
	if err := runner.runOracles(ctx, &ledger, prepared); err != nil {
		return ledger, prepared, err
	}
	allowedPairs := map[string]struct{}{}
	if runner.PairLimit > 0 {
		for _, entry := range prepared.Plan.Entries {
			if len(allowedPairs) == runner.PairLimit {
				break
			}
			allowedPairs[entry.PairID] = struct{}{}
		}
	}
	for _, entry := range prepared.Plan.Entries {
		if runner.PairLimit > 0 {
			if _, allowed := allowedPairs[entry.PairID]; !allowed {
				continue
			}
		}
		key := harness.RunKey(entry)
		record := ledger.Runs[key]
		if record.State == "sealed" {
			continue
		}
		if record.State != "unreserved" {
			return ledger, prepared, contractError("pilot_unsealed_attempt_requires_manual_adjudication")
		}
		task, ok := selectedByID[entry.TaskID]
		if !ok {
			return ledger, prepared, contractError("pilot_plan_task_missing")
		}
		if err := runner.runEntry(ctx, &ledger, prepared, task, entry); err != nil {
			return ledger, prepared, err
		}
	}
	if runner.PairLimit > 0 {
		ledger.Status = "paused_after_pair_limit"
		if err := runner.saveLedger(prepared.ArtifactRoot, &ledger); err != nil {
			return ledger, prepared, err
		}
		return ledger, prepared, nil
	}
	now := runner.now().UTC()
	ledger.Status, ledger.CompletedAt = "complete", &now
	ledger.CompletionMarker = &CompletionMarker{
		SchemaVersion:    "agentic-bench/development-pilot-completion-v1",
		FormalCompatible: false, Complete: true,
		SealedRunCount: len(prepared.Plan.Entries), ExpectedRunCount: len(prepared.Plan.Entries),
	}
	if err := runner.saveLedger(prepared.ArtifactRoot, &ledger); err != nil {
		return ledger, prepared, err
	}
	return ledger, prepared, nil
}

func (runner Runner) prepare(ctx context.Context, external bool) (Preflight, error) {
	if runner.Backend == nil {
		return Preflight{}, contractError("pilot_backend_required")
	}
	if runner.PairLimit < 0 || runner.PairLimit > len(ExactTaskIDs) {
		return Preflight{}, contractError("pilot_pair_limit_invalid")
	}
	if err := validatePilotManifest(runner.Loaded); err != nil {
		return Preflight{}, err
	}
	workDir, err := filepath.Abs(runner.WorkDir)
	if err != nil {
		return Preflight{}, err
	}
	artifactRoot, err := safeJoin(workDir, runner.Loaded.Manifest.Artifacts.Root)
	if err != nil {
		return Preflight{}, err
	}
	if err := os.MkdirAll(artifactRoot, 0o700); err != nil {
		return Preflight{}, err
	}
	storage, err := runner.storagePaths(external)
	if err != nil {
		return Preflight{}, err
	}
	archiver, ok := runner.Backend.(harness.InventoryLockArchiver)
	if !ok {
		return Preflight{}, contractError("pilot_inventory_archiver_required")
	}
	if err := archiver.BindInventoryLockArchive(ctx, filepath.Join(artifactRoot, harness.InventoryLockArchiveRelativePath)); err != nil {
		return Preflight{}, err
	}
	developmentBackend, ok := runner.Backend.(developmentPreflightBackend)
	if !ok {
		return Preflight{}, contractError("pilot_development_preflight_required")
	}
	backend, err := developmentBackend.DevelopmentPreflight(ctx, runner.Loaded.Manifest)
	if err != nil {
		return Preflight{}, err
	}
	inventory, err := runner.Backend.Inventory(ctx, runner.Loaded.Manifest.Dataset)
	if err != nil {
		return Preflight{}, err
	}
	selected, err := harness.SelectTasks(runner.Loaded.Manifest.Selection, inventory)
	if err != nil {
		return Preflight{}, err
	}
	selectedIDs := make([]string, 0, len(selected))
	for _, task := range selected {
		selectedIDs = append(selectedIDs, task.ID)
	}
	if !slices.Equal(selectedIDs, ExactTaskIDs) {
		return Preflight{}, contractError("pilot_selected_tasks_changed")
	}
	plan, err := harness.BuildPlan(runner.Loaded.SHA256, runner.Loaded.Manifest, selected)
	if err != nil {
		return Preflight{}, err
	}
	if err := validatePairedPlan(plan); err != nil {
		return Preflight{}, err
	}
	planSHA, err := harness.HashCanonical(plan)
	if err != nil {
		return Preflight{}, err
	}
	inventorySHA, err := harness.HashTaskInventory(inventory)
	if err != nil {
		return Preflight{}, err
	}
	return Preflight{
		SchemaVersion: PreflightSchemaVersion, FormalCompatible: false,
		ExternalExecution: external, ManifestSHA256: runner.Loaded.SHA256,
		InventorySHA256: inventorySHA, SelectedTaskIDs: slices.Clone(selectedIDs),
		Plan: plan, PlanSHA256: planSHA, Backend: backend, ArtifactRoot: artifactRoot,
		Storage: storage, Timeouts: runner.Loaded.Manifest.Timeouts,
		Resources: runner.Loaded.Manifest.Resources,
	}, nil
}

func validatePilotManifest(loaded harness.LoadedManifest) error {
	if err := harness.ValidateManifest(loaded.Manifest); err != nil {
		return err
	}
	manifest := loaded.Manifest
	if manifest.Selection.Mode != "tasks" || !slices.Equal(manifest.Selection.TaskIDs, ExactTaskIDs) || manifest.Selection.ExpectedTaskCount != 113 {
		return contractError("pilot_manifest_requires_exact_five_tasks")
	}
	if !manifest.Scheduling.PairAgents || manifest.Scheduling.Repetitions != 1 || manifest.Scheduling.MaxParallelPairs != 1 || len(manifest.Agents) != 2 {
		return contractError("pilot_manifest_requires_serial_pairs")
	}
	ids := []string{manifest.Agents[0].ID, manifest.Agents[1].ID}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"codex", "luban"}) {
		return contractError("pilot_manifest_requires_codex_luban")
	}
	for _, agent := range manifest.Agents {
		if agent.Model.TransportRequirement != harness.TransportRequirementHTTPInference {
			return contractError("pilot_manifest_requires_http_inference")
		}
	}
	return nil
}

func validatePairedPlan(plan harness.RunPlan) error {
	if len(plan.Entries) != len(ExactTaskIDs)*2 {
		return contractError("pilot_plan_matrix_invalid")
	}
	seen := make(map[string]int, len(ExactTaskIDs))
	for index := 0; index < len(plan.Entries); index += 2 {
		left, right := plan.Entries[index], plan.Entries[index+1]
		if left.Ordinal != index || right.Ordinal != index+1 || left.PairID != right.PairID || left.TaskID != right.TaskID || left.AgentID == right.AgentID || left.Repetition != 0 || right.Repetition != 0 {
			return contractError("pilot_plan_pair_invalid")
		}
		seen[left.TaskID]++
	}
	for _, taskID := range ExactTaskIDs {
		if seen[taskID] != 1 {
			return contractError("pilot_plan_task_coverage_invalid")
		}
	}
	return nil
}

func (runner Runner) storagePaths(external bool) (StoragePaths, error) {
	host, err := filepath.Abs(runner.HostStorageReceiptPath)
	if err != nil || runner.HostStorageReceiptPath == "" {
		return StoragePaths{}, contractError("pilot_host_storage_receipt_required")
	}
	guest, err := filepath.Abs(runner.GuestStoragePreflightPath)
	if err != nil || runner.GuestStoragePreflightPath == "" {
		return StoragePaths{}, contractError("pilot_guest_storage_preflight_required")
	}
	guestSHA := ""
	if _, statErr := os.Stat(guest); statErr == nil {
		guestSHA, err = harness.HashFile(guest)
		if err != nil {
			return StoragePaths{}, err
		}
		if external {
			var receipt map[string]json.RawMessage
			if err := readStrictJSON(guest, &receipt); err != nil {
				return StoragePaths{}, err
			}
			var schema, status string
			var formal bool
			if json.Unmarshal(receipt["schema_version"], &schema) != nil || json.Unmarshal(receipt["status"], &status) != nil || json.Unmarshal(receipt["formal_compatible"], &formal) != nil || schema != "agentic-bench/pilot-guest-storage-preflight-v1" || formal || status != "passed" {
				return StoragePaths{}, contractError("pilot_guest_storage_preflight_not_passed")
			}
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return StoragePaths{}, statErr
	} else if external {
		return StoragePaths{}, contractError("pilot_guest_storage_preflight_missing")
	}
	return StoragePaths{
		FormalCompatible: false, HostReceiptPath: host,
		GuestPreflightReceiptPath: guest, GuestPreflightReceiptSHA256: guestSHA,
		PerAttemptGuestReceiptRelativePaths: []string{GuestAgentReceiptRelative, GuestVerifierReceiptRelative},
	}, nil
}

func (runner Runner) loadOrCreateLedger(preflight Preflight) (Ledger, error) {
	if err := ensureImmutableBytes(filepath.Join(preflight.ArtifactRoot, ManifestRelativePath), runner.Loaded.Raw, runner.Loaded.SHA256); err != nil {
		return Ledger{}, err
	}
	if err := ensureImmutableJSON(filepath.Join(preflight.ArtifactRoot, PlanRelativePath), preflight.Plan, preflight.PlanSHA256); err != nil {
		return Ledger{}, err
	}
	backendSHA, err := harness.HashCanonical(preflight.Backend)
	if err != nil {
		return Ledger{}, err
	}
	if err := ensureImmutableJSON(filepath.Join(preflight.ArtifactRoot, BackendRelativePath), preflight.Backend, backendSHA); err != nil {
		return Ledger{}, err
	}
	path := filepath.Join(preflight.ArtifactRoot, LedgerRelativePath)
	var ledger Ledger
	if err := readStrictJSON(path, &ledger); err == nil {
		if err := validateLedgerIdentity(ledger, preflight); err != nil {
			return Ledger{}, err
		}
		return ledger, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Ledger{}, err
	}
	now := runner.now().UTC()
	ledger = Ledger{
		SchemaVersion: LedgerSchemaVersion, FormalCompatible: false, Status: "running",
		ManifestSHA256: preflight.ManifestSHA256, PlanSHA256: preflight.PlanSHA256,
		StartedAt: now, UpdatedAt: now, Backend: preflight.Backend, Storage: preflight.Storage,
		Oracle: map[string]OracleRecord{}, Runs: make(map[string]RunRecord, len(preflight.Plan.Entries)),
	}
	for _, entry := range preflight.Plan.Entries {
		agent, ok := findAgent(runner.Loaded.Manifest.Agents, entry.AgentID)
		if !ok {
			return Ledger{}, contractError("pilot_plan_agent_missing")
		}
		ledger.Runs[harness.RunKey(entry)] = RunRecord{
			FormalCompatible: false, Entry: entry, State: "unreserved", AttemptNumber: 1,
			ArtifactDir: filepath.Join(preflight.ArtifactRoot, "runs", entry.PairID, entry.AgentID, "attempt-001"),
			Model:       agent.Model,
		}
	}
	if err := runner.saveLedger(preflight.ArtifactRoot, &ledger); err != nil {
		return Ledger{}, err
	}
	return ledger, nil
}

func validateLedgerIdentity(ledger Ledger, preflight Preflight) error {
	if ledger.SchemaVersion != LedgerSchemaVersion || ledger.FormalCompatible || ledger.ManifestSHA256 != preflight.ManifestSHA256 || ledger.PlanSHA256 != preflight.PlanSHA256 || ledger.StartedAt.IsZero() || len(ledger.Runs) != len(preflight.Plan.Entries) {
		return contractError("pilot_resume_ledger_identity_invalid")
	}
	for _, entry := range preflight.Plan.Entries {
		record, ok := ledger.Runs[harness.RunKey(entry)]
		if !ok || record.Entry != entry || record.AttemptNumber != 1 {
			return contractError("pilot_resume_attempt_matrix_invalid")
		}
		switch record.State {
		case "unreserved", "reserved", "sealed":
		default:
			return contractError("pilot_resume_attempt_state_invalid")
		}
	}
	return nil
}

func (runner Runner) runOracles(ctx context.Context, ledger *Ledger, preflight Preflight) error {
	for _, taskID := range ExactTaskIDs {
		if record, ok := ledger.Oracle[taskID]; ok {
			if err := validateOracleRecord(record, preflight.ManifestSHA256, taskID); err != nil {
				return err
			}
			continue
		}
		directory := filepath.Join(preflight.ArtifactRoot, "oracle", taskID)
		if _, err := os.Lstat(directory); err == nil {
			return contractError("pilot_oracle_directory_without_ledger")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(directory), 0o700); err != nil {
			return err
		}
		if err := os.Mkdir(directory, 0o700); err != nil {
			return err
		}
		timeout := time.Duration(runner.Loaded.Manifest.Timeouts.VerifierSeconds) * time.Second
		oracleContext, cancel := context.WithTimeout(ctx, timeout)
		verification, verifyErr := runner.Backend.VerifyOracle(oracleContext, harness.OracleRequest{
			TaskID: taskID, ArtifactDir: directory, Timeout: timeout,
			Resources: runner.Loaded.Manifest.Resources,
		})
		cancel()
		if verifyErr != nil {
			return verifyErr
		}
		if !verification.ProtocolValid || verification.Reward != 1 {
			return contractError("pilot_oracle_did_not_pass")
		}
		receipt := OracleReceipt{
			SchemaVersion: OracleReceiptSchemaVersion, FormalCompatible: false,
			ManifestSHA256: preflight.ManifestSHA256, TaskID: taskID,
			SealedAt: runner.now().UTC(), Verification: verification,
		}
		receiptPath := filepath.Join(directory, OracleReceiptName)
		if err := writeJSONNoClobber(receiptPath, receipt, 0o600); err != nil {
			return err
		}
		digest, err := harness.HashFile(receiptPath)
		if err != nil {
			return err
		}
		ledger.Oracle[taskID] = OracleRecord{FormalCompatible: false, TaskID: taskID, ReceiptPath: receiptPath, ReceiptSHA256: digest, Verification: verification}
		if err := runner.saveLedger(preflight.ArtifactRoot, ledger); err != nil {
			return err
		}
	}
	return nil
}

func validateOracleRecord(record OracleRecord, manifestSHA, taskID string) error {
	if record.TaskID != taskID || record.ReceiptPath == "" || record.ReceiptSHA256 == "" || !record.Verification.ProtocolValid || record.Verification.Reward != 1 {
		return contractError("pilot_oracle_record_invalid")
	}
	digest, err := harness.HashFile(record.ReceiptPath)
	if err != nil || digest != record.ReceiptSHA256 {
		return contractError("pilot_oracle_receipt_digest_invalid")
	}
	var receipt OracleReceipt
	if err := readStrictJSON(record.ReceiptPath, &receipt); err != nil {
		return err
	}
	if receipt.SchemaVersion != OracleReceiptSchemaVersion || receipt.FormalCompatible || receipt.ManifestSHA256 != manifestSHA || receipt.TaskID != taskID || receipt.SealedAt.IsZero() || !receipt.Verification.ProtocolValid || receipt.Verification.Reward != 1 {
		return contractError("pilot_oracle_receipt_invalid")
	}
	return nil
}

func (runner Runner) runEntry(ctx context.Context, ledger *Ledger, preflight Preflight, task harness.Task, entry harness.PlanEntry) error {
	key := harness.RunKey(entry)
	record := ledger.Runs[key]
	if _, err := os.Lstat(record.ArtifactDir); err == nil {
		return contractError("pilot_attempt_directory_already_exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(record.ArtifactDir), 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(record.ArtifactDir, 0o700); err != nil {
		return err
	}
	agent, ok := findAgent(runner.Loaded.Manifest.Agents, entry.AgentID)
	if !ok {
		return contractError("pilot_plan_agent_missing")
	}
	publicTask, err := runner.Backend.PublicTask(ctx, task.ID)
	if err != nil {
		return err
	}
	if publicTask.ID != task.ID || publicTask.BaseCommit != task.BaseCommit || publicTask.InstructionSHA256 != task.InstructionSHA256 || publicTask.ImageDigest != task.ImageDigest {
		return contractError("pilot_public_task_identity_invalid")
	}
	environment, err := harness.FilterEnvironment(runner.HostEnvironment, runner.Loaded.Manifest.Environment.HostEnvAllowlist, agent.Command.RequiredEnv)
	if err != nil {
		return err
	}
	record.State, record.ReservedAt = "reserved", runner.now().UTC()
	ledger.Runs[key] = record
	if err := runner.saveLedger(preflight.ArtifactRoot, ledger); err != nil {
		return err
	}
	totalSeconds := runner.Loaded.Manifest.Timeouts.SetupSeconds + runner.Loaded.Manifest.Timeouts.AgentSeconds + runner.Loaded.Manifest.Timeouts.VerifierSeconds + runner.Loaded.Manifest.Timeouts.TeardownSeconds
	runContext, cancel := context.WithTimeout(ctx, time.Duration(totalSeconds)*time.Second)
	execution, runErr := runner.Backend.RunAgent(runContext, harness.AgentInvocation{
		PlanEntry: entry, Agent: agent, Task: publicTask, ArtifactDir: record.ArtifactDir,
		Environment: environment, Timeout: time.Duration(runner.Loaded.Manifest.Timeouts.AgentSeconds) * time.Second,
		Resources:     runner.Loaded.Manifest.Resources,
		AllowedEgress: slices.Clone(runner.Loaded.Manifest.Environment.AgentEgressHosts),
	})
	cancel()
	if execution.Lifecycle.ProviderAttemptState != "provider_attempt_sealed" || execution.Lifecycle.ProviderAttemptCount == 0 || execution.Verification == nil {
		if runErr != nil {
			return runErr
		}
		return contractError("pilot_attempt_not_sealed")
	}
	metrics, err := aggregateExecutionMetrics(execution, agent, runner.Loaded.Manifest.Pricing)
	if err != nil {
		return err
	}
	receipt := AttemptReceipt{
		SchemaVersion: AttemptReceiptSchemaVersion, FormalCompatible: false,
		ManifestSHA256: preflight.ManifestSHA256, PlanSHA256: preflight.PlanSHA256,
		RunKey: key, Entry: entry, Model: agent.Model, SealedAt: runner.now().UTC(),
		Execution: execution, Verification: *execution.Verification, Metrics: metrics,
	}
	receiptPath := filepath.Join(record.ArtifactDir, AttemptReceiptName)
	if err := writeJSONNoClobber(receiptPath, receipt, 0o600); err != nil {
		return err
	}
	digest, err := harness.HashFile(receiptPath)
	if err != nil {
		return err
	}
	record.State, record.SealedAt = "sealed", receipt.SealedAt
	record.ReceiptPath, record.ReceiptSHA256 = receiptPath, digest
	record.Execution, record.Verification, record.Metrics = &receipt.Execution, &receipt.Verification, &receipt.Metrics
	record.NormalizedEvidencePath = receipt.Execution.EvidencePath
	ledger.Runs[key] = record
	if err := runner.saveLedger(preflight.ArtifactRoot, ledger); err != nil {
		return err
	}
	if runErr != nil {
		return runErr
	}
	return nil
}

func (runner Runner) resumeSealedReceipts(ledger *Ledger, preflight Preflight) error {
	changed := false
	for _, entry := range preflight.Plan.Entries {
		key := harness.RunKey(entry)
		record := ledger.Runs[key]
		receiptPath := filepath.Join(record.ArtifactDir, AttemptReceiptName)
		switch record.State {
		case "unreserved":
			if _, err := os.Lstat(record.ArtifactDir); err == nil {
				return contractError("pilot_unledgered_attempt_directory")
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		case "reserved":
			if _, err := os.Lstat(receiptPath); errors.Is(err, os.ErrNotExist) {
				return contractError("pilot_unsealed_attempt_requires_manual_adjudication")
			} else if err != nil {
				return err
			}
		case "sealed":
			if receiptPath != record.ReceiptPath {
				return contractError("pilot_attempt_receipt_path_changed")
			}
		}
		var receipt AttemptReceipt
		if err := readStrictJSON(receiptPath, &receipt); err != nil {
			return err
		}
		agent, ok := findAgent(runner.Loaded.Manifest.Agents, entry.AgentID)
		if !ok {
			return contractError("pilot_plan_agent_missing")
		}
		if err := validateAttemptReceipt(receipt, preflight, key, entry, agent, runner.Loaded.Manifest.Pricing); err != nil {
			return err
		}
		digest, err := harness.HashFile(receiptPath)
		if err != nil {
			return err
		}
		if record.State == "sealed" && digest != record.ReceiptSHA256 {
			return contractError("pilot_attempt_receipt_digest_changed")
		}
		record.State, record.SealedAt = "sealed", receipt.SealedAt
		record.ReceiptPath, record.ReceiptSHA256 = receiptPath, digest
		record.Execution, record.Verification, record.Metrics = &receipt.Execution, &receipt.Verification, &receipt.Metrics
		record.NormalizedEvidencePath = receipt.Execution.EvidencePath
		ledger.Runs[key], changed = record, true
	}
	if changed {
		return runner.saveLedger(preflight.ArtifactRoot, ledger)
	}
	return nil
}

func validateAttemptReceipt(receipt AttemptReceipt, preflight Preflight, key string, entry harness.PlanEntry, agent harness.AgentSpec, pricing harness.PricingCatalog) error {
	if receipt.SchemaVersion != AttemptReceiptSchemaVersion || receipt.FormalCompatible || receipt.ManifestSHA256 != preflight.ManifestSHA256 || receipt.PlanSHA256 != preflight.PlanSHA256 || receipt.RunKey != key || receipt.Entry != entry || !reflect.DeepEqual(receipt.Model, agent.Model) || receipt.SealedAt.IsZero() {
		return contractError("pilot_attempt_receipt_identity_invalid")
	}
	execution := receipt.Execution
	if execution.Lifecycle.ProviderAttemptState != "provider_attempt_sealed" || execution.Lifecycle.ProviderAttemptCount == 0 || execution.Lifecycle.RunIdentity != execution.EvidenceRunIdentity || execution.Verification == nil || !reflect.DeepEqual(*execution.Verification, receipt.Verification) {
		return contractError("pilot_attempt_receipt_not_sealed")
	}
	metrics, err := aggregateExecutionMetrics(execution, agent, pricing)
	if err != nil {
		return err
	}
	expected, err := harness.HashCanonical(metrics)
	if err != nil {
		return err
	}
	actual, err := harness.HashCanonical(receipt.Metrics)
	if err != nil {
		return err
	}
	if actual != expected {
		return contractError("pilot_attempt_metrics_changed")
	}
	return nil
}

func aggregateExecutionMetrics(execution harness.AgentExecution, agent harness.AgentSpec, pricing harness.PricingCatalog) (harness.UsageMetrics, error) {
	if execution.EvidencePath == "" {
		return harness.UsageMetrics{}, contractError("pilot_provider_evidence_missing")
	}
	rounds, err := harness.ReadJSONLines[harness.ProviderRoundEvidence](execution.EvidencePath)
	if err != nil {
		return harness.UsageMetrics{}, err
	}
	return harness.ValidateAndAggregateEvidence(rounds, agent.Model, pricing)
}

func (runner Runner) saveLedger(root string, ledger *Ledger) error {
	ledger.UpdatedAt = runner.now().UTC()
	return harness.WriteJSONAtomic(filepath.Join(root, LedgerRelativePath), ledger, 0o600)
}

func (runner Runner) now() time.Time {
	if runner.Now != nil {
		return runner.Now()
	}
	return time.Now()
}

func findAgent(agents []harness.AgentSpec, id string) (harness.AgentSpec, bool) {
	for _, agent := range agents {
		if agent.ID == id {
			return agent, true
		}
	}
	return harness.AgentSpec{}, false
}

func ensureImmutableBytes(path string, expected []byte, expectedSHA string) error {
	if digest, err := harness.HashFile(path); err == nil {
		if digest != expectedSHA {
			return contractError("pilot_immutable_snapshot_changed")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeBytesNoClobber(path, expected, 0o600)
}

func ensureImmutableJSON(path string, value any, expectedSHA string) error {
	if _, err := os.Lstat(path); err == nil {
		var raw any
		if err := readStrictJSON(path, &raw); err != nil {
			return err
		}
		digest, err := harness.HashCanonical(raw)
		if err != nil {
			return err
		}
		if digest != expectedSHA {
			return contractError("pilot_immutable_snapshot_changed")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeJSONNoClobber(path, value, 0o600)
}

func writeJSONNoClobber(path string, value any, mode os.FileMode) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeBytesNoClobber(path, raw, mode)
}

func writeBytesNoClobber(path string, raw []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pilot-write-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func readStrictJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return contractError("pilot_json_contains_trailing_value")
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
					return contractError("pilot_json_object_key_invalid")
				}
				if _, duplicate := seen[key]; duplicate {
					return contractError("pilot_json_duplicate_object_key")
				}
				seen[key] = struct{}{}
				if err := scan(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return contractError("pilot_json_object_invalid")
			}
		case '[':
			for decoder.More() {
				if err := scan(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return contractError("pilot_json_array_invalid")
			}
		default:
			return contractError("pilot_json_delimiter_invalid")
		}
		return nil
	}
	return scan()
}

func safeJoin(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", contractError("pilot_artifact_root_must_be_relative")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	joined, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	if joined != root && !strings.HasPrefix(joined, root+string(filepath.Separator)) {
		return "", contractError("pilot_artifact_root_escapes_workdir")
	}
	return joined, nil
}

type contractError string

func (err contractError) Error() string { return string(err) }

var _ error = contractError("")
