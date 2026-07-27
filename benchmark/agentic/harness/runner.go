package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"
)

type Runner struct {
	Loaded          LoadedManifest
	Backend         Backend
	WorkDir         string
	HostEnvironment []string
	Now             func() time.Time
	// StopAfterOracles persists the immutable plan, snapshots, state, and all
	// selected gold validations without starting either model-backed agent.
	StopAfterOracles bool
}

type InvalidExperimentError struct{ Reason string }

func (e InvalidExperimentError) Error() string { return "benchmark experiment is invalid: " + e.Reason }

type InfrastructureError struct {
	Phase string
	Err   error
}

func (e InfrastructureError) Error() string {
	return "benchmark infrastructure failed during " + e.Phase + ": " + e.Err.Error()
}
func (e InfrastructureError) Unwrap() error { return e.Err }

// Run executes or resumes a plan. It is intentionally pair-serial: entries in
// a task pair remain adjacent and the manifest randomizes which agent runs
// first. A distributed backend can parallelize independent pairs externally.
func (runner Runner) Run(ctx context.Context) (ExperimentState, RunPlan, error) {
	if runner.Backend == nil {
		return ExperimentState{}, RunPlan{}, errors.New("benchmark backend is required")
	}
	if runner.Now == nil {
		runner.Now = time.Now
	}
	if err := ValidateManifest(runner.Loaded.Manifest); err != nil {
		return ExperimentState{}, RunPlan{}, err
	}
	workDir, err := filepath.Abs(runner.WorkDir)
	if err != nil {
		return ExperimentState{}, RunPlan{}, err
	}
	artifactRoot, err := artifactPath(workDir, runner.Loaded.Manifest.Artifacts.Root)
	if err != nil {
		return ExperimentState{}, RunPlan{}, err
	}
	if err := mkdirAllDurable(artifactRoot, 0o755); err != nil {
		return ExperimentState{}, RunPlan{}, err
	}
	for _, agent := range runner.Loaded.Manifest.Agents {
		if agent.SourceSnapshot == nil {
			continue
		}
		worktree, pathErr := filepath.Abs(agent.SourceSnapshot.Worktree)
		if pathErr != nil {
			return ExperimentState{}, RunPlan{}, pathErr
		}
		if artifactRoot == worktree || stringsHasPathPrefix(artifactRoot, worktree) {
			return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: "artifact root must be outside every agent source worktree"}
		}
	}
	hostStorageBackend, ok := runner.Backend.(HostStoragePreflightBackend)
	if !ok {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: "formal backend does not implement outer-host storage preflight"}
	}
	storagePreflight, err := hostStorageBackend.CheckHostStoragePreflight(ctx, StorageAdmissionRequest{
		ControllerRoot: workDir, ArtifactRoot: artifactRoot, Resources: runner.Loaded.Manifest.Resources,
	})
	if err != nil {
		return ExperimentState{}, RunPlan{}, InfrastructureError{Phase: "outer-host storage preflight", Err: err}
	}
	if err := ValidateStoragePreflightReceipt(storagePreflight, runner.Loaded.Manifest.Resources); err != nil {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: err.Error()}
	}
	archiver, ok := runner.Backend.(InventoryLockArchiver)
	if !ok {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: "benchmark backend cannot archive its exact inventory lock"}
	}
	inventoryArchivePath := filepath.Join(artifactRoot, InventoryLockArchiveRelativePath)
	_, inventoryArchiveStatErr := os.Lstat(inventoryArchivePath)
	inventoryArchiveExisted := inventoryArchiveStatErr == nil
	if inventoryArchiveStatErr != nil && !errors.Is(inventoryArchiveStatErr, os.ErrNotExist) {
		return ExperimentState{}, RunPlan{}, InfrastructureError{Phase: "inventory-lock archive inspection", Err: inventoryArchiveStatErr}
	}
	if err := archiver.BindInventoryLockArchive(ctx, inventoryArchivePath); err != nil {
		if inventoryArchiveExisted {
			return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: "existing inventory-lock archive is invalid: " + err.Error()}
		}
		return ExperimentState{}, RunPlan{}, InfrastructureError{Phase: "inventory-lock archival", Err: err}
	}
	backendSnapshot, err := runner.Backend.Preflight(ctx, runner.Loaded.Manifest)
	if err != nil {
		return ExperimentState{}, RunPlan{}, InfrastructureError{Phase: "backend preflight", Err: err}
	}
	if err := verifyBackendSnapshot(runner.Loaded.Manifest, backendSnapshot); err != nil {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: err.Error()}
	}
	if !reflect.DeepEqual(backendSnapshot.StoragePreflight, storagePreflight) {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: "backend snapshot does not bind the outer-host storage preflight receipt"}
	}
	archivedInventory, err := ValidateInventoryLockArchive(inventoryArchivePath, backendSnapshot.InventoryLock)
	if err != nil {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: err.Error()}
	}
	inventory, err := runner.Backend.Inventory(ctx, runner.Loaded.Manifest.Dataset)
	if err != nil {
		return ExperimentState{}, RunPlan{}, InfrastructureError{Phase: "dataset inventory", Err: err}
	}
	inventorySHA, err := HashTaskInventory(inventory)
	if err != nil {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: err.Error()}
	}
	if inventorySHA != runner.Loaded.Manifest.Dataset.ManifestSHA256 {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: "resolved task and image inventory differs from the frozen dataset manifest"}
	}
	archivedInventorySHA, err := HashTaskInventory(archivedInventory)
	if err != nil {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: err.Error()}
	}
	if archivedInventorySHA != inventorySHA {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: "backend inventory differs from the archived inventory-lock bytes"}
	}
	selected, err := SelectTasks(runner.Loaded.Manifest.Selection, inventory)
	if err != nil {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: err.Error()}
	}
	plan, err := BuildPlan(runner.Loaded.SHA256, runner.Loaded.Manifest, selected)
	if err != nil {
		return ExperimentState{}, RunPlan{}, err
	}
	planSHA, err := HashCanonical(plan)
	if err != nil {
		return ExperimentState{}, RunPlan{}, err
	}
	if err := runner.ensureImmutableSnapshots(artifactRoot, plan, planSHA); err != nil {
		return ExperimentState{}, RunPlan{}, InvalidExperimentError{Reason: err.Error()}
	}
	statePath, err := artifactPath(artifactRoot, runner.Loaded.Manifest.Artifacts.StateRelativePath)
	if err != nil {
		return ExperimentState{}, RunPlan{}, err
	}
	state, err := runner.loadOrCreateState(ctx, artifactRoot, statePath, plan, planSHA, backendSnapshot)
	if err != nil {
		return ExperimentState{}, RunPlan{}, err
	}
	if state.Status == ExperimentInvalid {
		return state, plan, InvalidExperimentError{Reason: state.InvalidReason}
	}
	if state.Status == ExperimentComplete {
		if err := runner.writeFinalArtifacts(artifactRoot, state, plan); err != nil {
			return state, plan, err
		}
		return state, plan, nil
	}
	state.Status = ExperimentRunning
	if err := runner.saveState(statePath, &state); err != nil {
		return state, plan, err
	}
	for _, task := range selected {
		if oracle, exists := state.Oracle[task.ID]; exists && oracle.Validated {
			continue
		}
		oracleDir := filepath.Join(artifactRoot, "oracle", task.ID)
		if err := mkdirAllDurable(oracleDir, 0o755); err != nil {
			return state, plan, err
		}
		oracleContext, cancel := context.WithTimeout(ctx, time.Duration(runner.Loaded.Manifest.Timeouts.VerifierSeconds)*time.Second)
		result, verifyErr := runner.Backend.VerifyOracle(oracleContext, OracleRequest{
			TaskID: task.ID, ArtifactDir: oracleDir,
			Timeout:   time.Duration(runner.Loaded.Manifest.Timeouts.VerifierSeconds) * time.Second,
			Resources: runner.Loaded.Manifest.Resources,
		})
		cancel()
		if verifyErr != nil {
			state.Oracle[task.ID] = OracleRecord{TaskID: task.ID, Failure: verifyErr.Error()}
			_ = runner.saveState(statePath, &state)
			return state, plan, InfrastructureError{Phase: "oracle verifier", Err: verifyErr}
		}
		if !validOracleResult(result) || validateVerifierArtifacts(oracleDir, result.ArtifactPaths) != nil {
			state.Oracle[task.ID] = OracleRecord{TaskID: task.ID, Verification: result, Failure: "oracle did not receive a valid full reward"}
			state.Status = ExperimentInvalid
			state.InvalidReason = "oracle validation failed for " + task.ID
			_ = runner.saveState(statePath, &state)
			return state, plan, InvalidExperimentError{Reason: state.InvalidReason}
		}
		state.Oracle[task.ID] = OracleRecord{TaskID: task.ID, Validated: true, Verification: result}
		if err := runner.saveState(statePath, &state); err != nil {
			return state, plan, err
		}
	}
	if runner.StopAfterOracles {
		return state, plan, nil
	}
	tasksByID := make(map[string]Task, len(selected))
	for _, task := range selected {
		tasksByID[task.ID] = task
	}
	for _, entry := range plan.Entries {
		record := state.Runs[RunKey(entry)]
		if record.Phase == RunComplete {
			continue
		}
		if record.Attempts == 0 {
			admissionBackend, ok := runner.Backend.(StorageAdmissionBackend)
			if !ok {
				return state, plan, InvalidExperimentError{Reason: "formal backend does not implement host storage admission"}
			}
			admission, admissionErr := admissionBackend.CheckStorageAdmission(ctx, StorageAdmissionRequest{
				ControllerRoot: workDir, ArtifactRoot: artifactRoot, Resources: runner.Loaded.Manifest.Resources,
			})
			if admissionErr != nil {
				return state, plan, InfrastructureError{Phase: "host storage admission", Err: admissionErr}
			}
			if err := ValidateStorageAdmissionReceipt(admission, runner.Loaded.Manifest.Resources); err != nil {
				return state, plan, InvalidExperimentError{Reason: err.Error()}
			}
			record.StorageAdmission = admission
		} else if err := ValidateStorageAdmissionReceipt(record.StorageAdmission, runner.Loaded.Manifest.Resources); err != nil {
			return state, plan, InvalidExperimentError{Reason: "reserved slot has invalid immutable storage admission: " + err.Error()}
		}
		invocation, recoverAttempt, err := runner.prepareAgentInvocation(ctx, artifactRoot, tasksByID[entry.TaskID], entry, &record)
		if err != nil {
			state.Runs[RunKey(entry)] = record
			runner.invalidateOnProtocolError(&state, err)
			_ = runner.saveState(statePath, &state)
			return state, plan, err
		}
		// Persist the raw slot before any provider request. A process crash can
		// only enter RecoverAgent on resume; it can never create a replacement.
		state.Runs[RunKey(entry)] = record
		if !recoverAttempt {
			if err := runner.saveState(statePath, &state); err != nil {
				return state, plan, err
			}
		}
		if err := runner.executeAgent(ctx, invocation, recoverAttempt, &record); err != nil {
			state.Runs[RunKey(entry)] = record
			runner.invalidateOnProtocolError(&state, err)
			_ = runner.saveState(statePath, &state)
			return state, plan, err
		}
		state.Runs[RunKey(entry)] = record
		if err := runner.saveState(statePath, &state); err != nil {
			return state, plan, err
		}
	}
	if err := ValidatePromptCacheKeyIsolation(state); err != nil {
		state.Status = ExperimentInvalid
		state.InvalidReason = err.Error()
		_ = runner.saveState(statePath, &state)
		return state, plan, InvalidExperimentError{Reason: err.Error()}
	}
	now := runner.Now().UTC()
	state.Status = ExperimentComplete
	state.CompletedAt = &now
	if err := runner.saveState(statePath, &state); err != nil {
		return state, plan, err
	}
	if err := runner.writeFinalArtifacts(artifactRoot, state, plan); err != nil {
		return state, plan, err
	}
	return state, plan, nil
}

func (runner Runner) loadOrCreateState(ctx context.Context, artifactRoot, statePath string, plan RunPlan, planSHA string, backend BackendSnapshot) (ExperimentState, error) {
	snapshots := make([]AgentSnapshot, 0, len(runner.Loaded.Manifest.Agents))
	for _, agent := range runner.Loaded.Manifest.Agents {
		snapshot, snapshotErr := SnapshotAgentAt(ctx, agent, runner.Now(), filepath.Join(artifactRoot, "sources", agent.ID))
		if snapshotErr != nil {
			return ExperimentState{}, InvalidExperimentError{Reason: snapshotErr.Error()}
		}
		snapshots = append(snapshots, snapshot)
	}
	state, err := LoadState(statePath, runner.Loaded.SHA256, planSHA)
	if err == nil {
		currentBackendSHA, hashErr := HashCanonical(resumableBackendIdentity(backend))
		if hashErr != nil {
			return ExperimentState{}, hashErr
		}
		stateBackendSHA, hashErr := HashCanonical(resumableBackendIdentity(state.Backend))
		if hashErr != nil {
			return ExperimentState{}, hashErr
		}
		if currentBackendSHA != stateBackendSHA {
			return ExperimentState{}, InvalidExperimentError{Reason: "backend identity changed since the run started"}
		}
		if !sameAgentSnapshots(state.Agents, snapshots) {
			return ExperimentState{}, InvalidExperimentError{Reason: "agent binary or immutable source snapshot changed since the run started"}
		}
		return state, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return ExperimentState{}, err
	}
	state, err = NewExperimentState(runner.Loaded.SHA256, plan, backend, snapshots, runner.Now())
	if err != nil {
		return ExperimentState{}, err
	}
	if err := runner.saveState(statePath, &state); err != nil {
		return ExperimentState{}, err
	}
	return state, nil
}

// resumableBackendIdentity excludes dynamic TLS peer and free-space
// observations. A
// valid certificate rotation may change leaf/SPKI hashes between controller
// processes, while the approved origin and endpoint semantics remain frozen.
// Every actual transport round still carries its own verified peer receipt.
func resumableBackendIdentity(snapshot BackendSnapshot) BackendSnapshot {
	endpoint := snapshot.ProviderEndpoint
	snapshot.ProviderEndpoint = ProviderEndpointSnapshot{
		ApprovedOrigin:  endpoint.ApprovedOrigin,
		SemanticsSHA256: endpoint.SemanticsSHA256,
	}
	snapshot.StoragePreflight = StorageAdmissionReceipt{}
	return snapshot
}

func (runner Runner) prepareAgentInvocation(ctx context.Context, artifactRoot string, task Task, entry PlanEntry, record *RunRecord) (AgentInvocation, bool, error) {
	agent, ok := findAgent(runner.Loaded.Manifest.Agents, entry.AgentID)
	if !ok {
		return AgentInvocation{}, false, InvalidExperimentError{Reason: "plan references unknown agent " + entry.AgentID}
	}
	publicTask, err := runner.Backend.PublicTask(ctx, entry.TaskID)
	if err != nil {
		return AgentInvocation{}, false, InfrastructureError{Phase: "public task preparation", Err: err}
	}
	if publicTask.ID != entry.TaskID || publicTask.BaseCommit != task.BaseCommit || publicTask.InstructionSHA256 != task.InstructionSHA256 || publicTask.Image != task.Image || publicTask.ImageDigest != task.ImageDigest || publicTask.InstructionPath == "" || publicTask.WorkspacePath == "" {
		record.Phase, record.Failure = RunProtocolFailed, "public task identity is incomplete or mismatched"
		return AgentInvocation{}, false, InvalidExperimentError{Reason: record.Failure}
	}
	environment, err := FilterEnvironment(runner.HostEnvironment, runner.Loaded.Manifest.Environment.HostEnvAllowlist, agent.Command.RequiredEnv)
	if err != nil {
		record.Phase, record.Failure = RunProtocolFailed, err.Error()
		return AgentInvocation{}, false, InvalidExperimentError{Reason: err.Error()}
	}
	recoverAttempt := record.Attempts == 1
	if record.Attempts == 0 {
		record.Attempts = 1
		record.SlotReservedAt = runner.Now().UTC()
		record.ArtifactDir = filepath.Join(artifactRoot, "runs", entry.PairID, entry.AgentID, "attempt-001")
	} else if record.Attempts != 1 || record.ArtifactDir == "" || record.SlotReservedAt.IsZero() {
		record.Phase, record.Failure = RunProtocolFailed, "raw attempt ledger permits exactly one immutable slot"
		return AgentInvocation{}, false, InvalidExperimentError{Reason: record.Failure}
	}
	if err := mkdirAllDurable(record.ArtifactDir, 0o755); err != nil {
		return AgentInvocation{}, false, err
	}
	return AgentInvocation{
		PlanEntry: entry, Agent: agent, Task: publicTask, ArtifactDir: record.ArtifactDir,
		Environment: environment, Timeout: time.Duration(runner.Loaded.Manifest.Timeouts.AgentSeconds) * time.Second,
		Resources: runner.Loaded.Manifest.Resources, AllowedEgress: runner.Loaded.Manifest.Environment.AgentEgressHosts,
		StorageAdmission: record.StorageAdmission,
	}, recoverAttempt, nil
}

func (runner Runner) executeAgent(ctx context.Context, invocation AgentInvocation, recoverAttempt bool, record *RunRecord) error {
	// A Pier Backend owns setup, the timed agent phase, separate verification,
	// and teardown as one physical trial. The invocation still carries the exact
	// agent timeout, while the outer infrastructure deadline covers every phase.
	totalSandboxSeconds := runner.Loaded.Manifest.Timeouts.SetupSeconds +
		runner.Loaded.Manifest.Timeouts.AgentSeconds +
		runner.Loaded.Manifest.Timeouts.VerifierSeconds +
		runner.Loaded.Manifest.Timeouts.TeardownSeconds
	agentContext, cancel := context.WithTimeout(ctx, time.Duration(totalSandboxSeconds)*time.Second)
	var execution AgentExecution
	var runErr error
	if recoverAttempt {
		execution, runErr = runner.Backend.RecoverAgent(agentContext, invocation)
		var safeRestart SafeRestartAttemptError
		if errors.As(runErr, &safeRestart) {
			execution, runErr = runner.Backend.RunAgent(agentContext, invocation)
		}
	} else {
		execution, runErr = runner.Backend.RunAgent(agentContext, invocation)
	}
	cancel()
	var attemptProtocol AttemptProtocolError
	if errors.As(runErr, &attemptProtocol) {
		record.Phase, record.Failure = RunProtocolFailed, attemptProtocol.Error()
		return InvalidExperimentError{Reason: record.Failure}
	}
	var attemptInfra AttemptInfrastructureError
	if runErr != nil && !errors.As(runErr, &attemptInfra) {
		record.Failure = runErr.Error()
		return InfrastructureError{Phase: "sealed raw attempt", Err: runErr}
	}
	if runErr != nil && !validInfrastructureCategory(attemptInfra.Category) {
		record.Phase, record.Failure = RunProtocolFailed, runErr.Error()
		return InvalidExperimentError{Reason: "backend returned an invalid raw-attempt failure category"}
	}
	controllerRecovery := runErr != nil && attemptInfra.Category == DeepSWEFailureControllerInfrastructure
	if controllerRecovery {
		if err := ValidateRecoveredControllerAttempt(execution); err != nil {
			record.Phase, record.Failure = RunProtocolFailed, err.Error()
			return InvalidExperimentError{Reason: err.Error()}
		}
		record.AttemptStartedAt = execution.Lifecycle.ControllerStartedAt
		record.Execution = &execution
	} else if !execution.TrialStartedAt.IsZero() || execution.EvidencePath != "" || execution.SubmissionPatch != "" || execution.AuditWorkspacePatch != "" {
		requireEvidence := runErr == nil || attemptInfra.Category != DeepSWEFailureNetworkInfrastructure
		if err := runner.validateAgentTelemetry(invocation, &execution, record, requireEvidence); err != nil {
			return err
		}
		if execution.SubmissionPatch != "" {
			if err := runner.validateAgentSubmission(invocation, &execution, record); err != nil {
				return err
			}
		}
	} else if runErr != nil {
		record.Failure = runErr.Error()
		return InfrastructureError{Phase: "unsealed raw attempt", Err: runErr}
	}
	if runErr == nil && execution.SubmissionPatch == "" {
		record.Phase, record.Failure = RunProtocolFailed, "agent adapter produced no submission patch"
		return InvalidExperimentError{Reason: record.Failure}
	}
	if runErr != nil {
		if execution.Verification != nil {
			record.Phase, record.Failure = RunProtocolFailed, "infrastructure exclusion contains a verifier result"
			return InvalidExperimentError{Reason: record.Failure}
		}
		record.Disposition = DeepSWEAttemptExcluded
		record.FailureCategory = attemptInfra.Category
		record.Phase, record.Failure = RunComplete, runErr.Error()
		return nil
	}
	if execution.Verification == nil {
		record.Phase, record.Failure = RunProtocolFailed, "Pier trial omitted its separate-verifier result"
		return InvalidExperimentError{Reason: record.Failure}
	}
	verification := *execution.Verification
	execution.Verification = nil
	if !verification.ProtocolValid || math.IsNaN(verification.Reward) || math.IsInf(verification.Reward, 0) || (verification.Reward != 0 && verification.Reward != 1) {
		record.Phase, record.Failure = RunProtocolFailed, "verifier returned an invalid binary reward"
		return InvalidExperimentError{Reason: record.Failure}
	}
	failureCategory := failureCategoryForExecution(execution)
	if failureCategory == DeepSWEFailureAgentTimeout || failureCategory == DeepSWEFailureContext {
		rawReward := verification.Reward
		verification.RawReward = &rawReward
		verification.Reward = 0
	}
	if err := validateVerifierArtifacts(filepath.Join(record.ArtifactDir, "verifier"), verification.ArtifactPaths); err != nil {
		record.Phase, record.Failure = RunProtocolFailed, err.Error()
		return InvalidExperimentError{Reason: err.Error()}
	}
	record.Execution = &execution
	record.Verification = &verification
	record.Disposition = DeepSWEAttemptScored
	record.FailureCategory = failureCategory
	record.Phase, record.Failure = RunComplete, ""
	return nil
}

// ValidateRecoveredControllerAttempt is the shared authority for a crash that
// happened after a durable provider-attempt start but before its evidence seal.
// A sealed attempt is never reclassified as a controller exclusion, and a
// recovery receipt must not fabricate any unavailable trial/provider output.
func ValidateRecoveredControllerAttempt(execution AgentExecution) error {
	lifecycle := execution.Lifecycle
	if lifecycle.SchemaVersion != "agentic-bench/attempt-lifecycle-v1" || !hex64Pattern.MatchString(lifecycle.RunIdentity) || lifecycle.ControllerStartedAt.IsZero() || !lifecycle.ControllerFinishedAt.IsZero() || !lifecycle.Recovered || lifecycle.ProviderAttemptCount == 0 ||
		lifecycle.ProviderAttemptState != "provider_attempt_started_unsealed" {
		return errors.New("controller recovery lacks a durable provider-attempt lifecycle")
	}
	if execution.ExitClass != "" || execution.ExitCode != 0 || execution.TrialStartedAt != (time.Time{}) || execution.TrialFinishedAt != (time.Time{}) || execution.StartedAt != (time.Time{}) || execution.FinishedAt != (time.Time{}) || execution.SubmissionPatch != "" || execution.AuditWorkspacePatch != "" || execution.Capture != (SubmissionCaptureEvidence{}) || execution.EvidencePath != "" || execution.EvidenceRunIdentity != "" || execution.ProviderEvidence != (ProviderEvidenceSeal{}) || execution.ServiceTierCanonicalization != (ServiceTierCanonicalizationEvidence{}) || !reflect.DeepEqual(execution.StorageEvidence, StorageResourceEvidence{}) || len(execution.GuestStorageEvidence) != 0 || execution.Verification != nil || execution.TerminalEvidence != (AgentTerminalEvidence{}) {
		return errors.New("controller recovery fabricates unavailable trial output")
	}
	return nil
}

func validateRecoveredControllerAttempt(execution AgentExecution) error {
	return ValidateRecoveredControllerAttempt(execution)
}

func (runner Runner) validateAgentTelemetry(invocation AgentInvocation, execution *AgentExecution, record *RunRecord, required bool) error {
	lifecycle := execution.Lifecycle
	validLifecycleState := lifecycle.ProviderAttemptState == "provider_attempt_sealed" && lifecycle.ProviderAttemptCount > 0
	if !required {
		validLifecycleState = validLifecycleState || lifecycle.ProviderAttemptState == "no_provider_attempt" && lifecycle.ProviderAttemptCount == 0
	}
	if lifecycle.SchemaVersion != "agentic-bench/attempt-lifecycle-v1" || lifecycle.RunIdentity != execution.EvidenceRunIdentity || !hex64Pattern.MatchString(lifecycle.RunIdentity) || lifecycle.ControllerStartedAt.IsZero() || lifecycle.ControllerFinishedAt.Before(lifecycle.ControllerStartedAt) || lifecycle.Recovered || !validLifecycleState {
		record.Phase, record.Failure = RunProtocolFailed, "agent execution lacks a sealed controller lifecycle"
		return InvalidExperimentError{Reason: record.Failure}
	}
	if execution.TrialStartedAt.IsZero() || execution.TrialFinishedAt.Before(execution.TrialStartedAt) ||
		execution.StartedAt.IsZero() != execution.FinishedAt.IsZero() ||
		(!execution.StartedAt.IsZero() && (execution.FinishedAt.Before(execution.StartedAt) || execution.StartedAt.Before(execution.TrialStartedAt) || execution.FinishedAt.After(execution.TrialFinishedAt))) {
		record.Phase, record.Failure = RunProtocolFailed, "agent execution lacks nested trial and agent timing"
		return InvalidExperimentError{Reason: record.Failure}
	}
	if execution.TrialStartedAt.Before(lifecycle.ControllerStartedAt) || lifecycle.ControllerFinishedAt.Before(execution.TrialFinishedAt) {
		record.Phase, record.Failure = RunProtocolFailed, "Pier trial timing falls outside the controller lifecycle"
		return InvalidExperimentError{Reason: record.Failure}
	}
	if lifecycle.ProviderAttemptCount > 0 {
		if err := ValidateStorageResourceEvidence(record.ArtifactDir, execution.StorageEvidence, invocation.StorageAdmission, invocation.Resources); err != nil {
			record.Phase, record.Failure = RunProtocolFailed, err.Error()
			return InvalidExperimentError{Reason: err.Error()}
		}
		if err := ValidateGuestStorageResourceEvidence(record.ArtifactDir, execution.GuestStorageEvidence, invocation.Resources); err != nil {
			record.Phase, record.Failure = RunProtocolFailed, err.Error()
			return InvalidExperimentError{Reason: err.Error()}
		}
	} else if !reflect.DeepEqual(execution.StorageEvidence, StorageResourceEvidence{}) || len(execution.GuestStorageEvidence) != 0 {
		record.Phase, record.Failure = RunProtocolFailed, "no-provider attempt contains post-WAL storage evidence"
		return InvalidExperimentError{Reason: record.Failure}
	}
	if execution.StartedAt.IsZero() && !required {
		if execution.TerminalEvidence != (AgentTerminalEvidence{}) {
			record.Phase, record.Failure = RunProtocolFailed, "pre-agent infrastructure failure contains terminal evidence"
			return InvalidExperimentError{Reason: record.Failure}
		}
	} else if err := validateAgentTerminalEvidence(*execution); err != nil {
		record.Phase, record.Failure = RunProtocolFailed, err.Error()
		return InvalidExperimentError{Reason: err.Error()}
	}
	record.AttemptStartedAt = execution.TrialStartedAt
	record.Execution = execution
	if execution.EvidencePath == "" {
		if required {
			record.Phase, record.Failure = RunProtocolFailed, "raw attempt lacks required provider evidence"
			return InvalidExperimentError{Reason: record.Failure}
		}
		return nil
	}
	if !hex64Pattern.MatchString(execution.EvidenceRunIdentity) {
		record.Phase, record.Failure = RunProtocolFailed, "agent execution lacks a valid evidence run identity"
		return InvalidExperimentError{Reason: record.Failure}
	}
	seal := execution.ProviderEvidence
	for _, path := range []string{seal.RawEvidencePath, seal.AttemptJournalPath, seal.SealPath} {
		if err := requirePathWithin(record.ArtifactDir, path); err != nil {
			record.Phase, record.Failure = RunProtocolFailed, err.Error()
			return InvalidExperimentError{Reason: err.Error()}
		}
	}
	for path, claimed := range map[string]string{
		seal.RawEvidencePath:    seal.RawEvidenceSHA256,
		seal.AttemptJournalPath: seal.AttemptJournalSHA256,
		seal.SealPath:           seal.SealSHA256,
	} {
		if !hex64Pattern.MatchString(claimed) {
			record.Phase, record.Failure = RunProtocolFailed, "provider evidence artifact lacks a valid digest"
			return InvalidExperimentError{Reason: record.Failure}
		}
		actual, hashErr := HashFile(path)
		if hashErr != nil || actual != claimed {
			record.Phase, record.Failure = RunProtocolFailed, "provider evidence artifact digest mismatch"
			return InvalidExperimentError{Reason: record.Failure}
		}
	}
	if seal.StartedAttemptCount == 0 || seal.StartedAttemptCount != seal.PersistedAttemptCount || seal.PersistedAttemptCount != seal.RecordCount || !hex64Pattern.MatchString(seal.LastEvidenceHash) {
		record.Phase, record.Failure = RunProtocolFailed, "provider evidence seal counts are incomplete"
		return InvalidExperimentError{Reason: record.Failure}
	}
	if lifecycle.ProviderAttemptCount != seal.StartedAttemptCount {
		record.Phase, record.Failure = RunProtocolFailed, "controller lifecycle disagrees with the provider attempt seal"
		return InvalidExperimentError{Reason: record.Failure}
	}
	evidencePath, err := artifactPath(record.ArtifactDir, invocation.Agent.RequestEvidence.RelativePath)
	if err != nil {
		return err
	}
	if execution.EvidencePath != evidencePath {
		record.Phase, record.Failure = RunProtocolFailed, "agent evidence path does not match the manifest"
		return InvalidExperimentError{Reason: record.Failure}
	}
	rounds, err := ReadJSONLines[ProviderRoundEvidence](evidencePath)
	if err != nil {
		record.Phase, record.Failure = RunProtocolFailed, err.Error()
		return InvalidExperimentError{Reason: err.Error()}
	}
	for _, round := range rounds {
		if round.RunIdentity != execution.EvidenceRunIdentity {
			record.Phase, record.Failure = RunProtocolFailed, "provider evidence belongs to another execution"
			return InvalidExperimentError{Reason: record.Failure}
		}
	}
	if err := ValidateServiceTierCanonicalizationArchive(record.ArtifactDir, invocation.Agent.ID, *execution, rounds); err != nil {
		record.Phase, record.Failure = RunProtocolFailed, err.Error()
		return InvalidExperimentError{Reason: err.Error()}
	}
	if len(rounds) == 0 || uint64(len(rounds)) != seal.RecordCount || rounds[len(rounds)-1].EvidenceHash != seal.LastEvidenceHash {
		record.Phase, record.Failure = RunProtocolFailed, "normalized provider evidence is not bound to the raw seal"
		return InvalidExperimentError{Reason: record.Failure}
	}
	metrics, err := ValidateAndAggregateEvidence(rounds, invocation.Agent.Model, runner.Loaded.Manifest.Pricing)
	if err != nil {
		record.Phase, record.Failure = RunProtocolFailed, err.Error()
		return InvalidExperimentError{Reason: err.Error()}
	}
	record.Metrics = &metrics
	return nil
}

func validateAgentTerminalEvidence(execution AgentExecution) error {
	evidence := execution.TerminalEvidence
	if evidence.SchemaVersion != "agentic-bench/terminal-evidence-v1" || !hex64Pattern.MatchString(evidence.EvidenceSHA256) {
		return errors.New("agent execution lacks structured terminal evidence")
	}
	expectedSource, expectedCode := "", ""
	switch execution.ExitClass {
	case "completed":
		expectedSource, expectedCode = "process_exit", "completed"
		if execution.ExitCode != 0 {
			return errors.New("completed agent execution has a nonzero exit code")
		}
	case "nonzero":
		expectedSource, expectedCode = "process_exit", "nonzero_exit"
		if execution.ExitCode == 0 {
			return errors.New("nonzero agent execution has a zero exit code")
		}
	case "timeout":
		expectedSource, expectedCode = "pier_trial", "agent_timeout"
	case "context_failure":
		expectedSource, expectedCode = "provider_event", "context_length_exceeded"
	default:
		return fmt.Errorf("agent execution has unsupported exit class %q", execution.ExitClass)
	}
	if evidence.Source != expectedSource || evidence.Code != expectedCode {
		return fmt.Errorf("agent exit class %s disagrees with structured terminal evidence", execution.ExitClass)
	}
	return nil
}

func (runner Runner) validateAgentSubmission(invocation AgentInvocation, execution *AgentExecution, record *RunRecord) error {
	if err := requirePathWithin(record.ArtifactDir, execution.SubmissionPatch); err != nil {
		record.Phase, record.Failure = RunProtocolFailed, err.Error()
		return InvalidExperimentError{Reason: err.Error()}
	}
	patchSHA, err := HashFile(execution.SubmissionPatch)
	if err != nil {
		record.Phase, record.Failure = RunProtocolFailed, err.Error()
		return InvalidExperimentError{Reason: err.Error()}
	}
	if err := requirePathWithin(record.ArtifactDir, execution.AuditWorkspacePatch); err != nil {
		record.Phase, record.Failure = RunProtocolFailed, err.Error()
		return InvalidExperimentError{Reason: err.Error()}
	}
	auditSHA, err := HashFile(execution.AuditWorkspacePatch)
	if err != nil {
		record.Phase, record.Failure = RunProtocolFailed, err.Error()
		return InvalidExperimentError{Reason: err.Error()}
	}
	capture := execution.Capture
	if capture.Method != "official-git-diff+temporary-index-audit-v2" || capture.BaseCommit != invocation.Task.BaseCommit ||
		capture.PatchSHA256 != patchSHA || capture.AuditPatchSHA256 != auditSHA ||
		capture.UncommittedChangesPresent != (patchSHA != auditSHA) ||
		!capture.IncludesTracked || !capture.IncludesUntracked || !capture.IncludesBinary {
		record.Phase, record.Failure = RunProtocolFailed, "submission capture evidence is incomplete or mismatched"
		return InvalidExperimentError{Reason: record.Failure}
	}
	return nil
}

func validInfrastructureCategory(category DeepSWEFailureCategory) bool {
	return category == DeepSWEFailureProviderInfrastructure || category == DeepSWEFailureVerifierInfrastructure || category == DeepSWEFailureNetworkInfrastructure || category == DeepSWEFailureControllerInfrastructure
}

func failureCategoryForExecution(execution AgentExecution) DeepSWEFailureCategory {
	switch execution.ExitClass {
	case "timeout":
		return DeepSWEFailureAgentTimeout
	case "context_failure":
		return DeepSWEFailureContext
	default:
		return DeepSWEFailureNone
	}
}

func (runner Runner) saveState(path string, state *ExperimentState) error {
	state.UpdatedAt = runner.Now().UTC()
	return WriteJSONAtomic(path, state, 0o644)
}

func (runner Runner) ensureImmutableSnapshots(root string, plan RunPlan, planSHA string) error {
	manifestPath := filepath.Join(root, "manifest.json")
	if digest, err := HashFile(manifestPath); err == nil {
		if digest != runner.Loaded.SHA256 {
			return errors.New("archived manifest differs from the requested manifest")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := WriteBytesAtomic(manifestPath, runner.Loaded.Raw, 0o644); err != nil {
			return err
		}
	} else {
		return err
	}
	planPath := filepath.Join(root, "plan.json")
	if raw, err := os.ReadFile(planPath); err == nil {
		var archived RunPlan
		if err := json.Unmarshal(raw, &archived); err != nil {
			return err
		}
		digest, err := HashCanonical(archived)
		if err != nil {
			return err
		}
		if digest != planSHA {
			return errors.New("archived plan differs from the deterministic plan")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := WriteJSONAtomic(planPath, plan, 0o644); err != nil {
			return err
		}
	} else {
		return err
	}
	return nil
}

func (runner Runner) writeFinalArtifacts(root string, state ExperimentState, plan RunPlan) error {
	scorecard, err := ScoreExperimentForManifest(runner.Loaded, state, plan)
	if err != nil {
		return err
	}
	if err := WriteJSONAtomic(filepath.Join(root, "scorecard.json"), scorecard, 0o644); err != nil {
		return err
	}
	ledger, err := BuildArtifactLedger(root, runner.Loaded.Manifest.Artifacts.LedgerRelativePath, runner.Loaded.SHA256)
	if err != nil {
		return err
	}
	ledgerPath, err := artifactPath(root, runner.Loaded.Manifest.Artifacts.LedgerRelativePath)
	if err != nil {
		return err
	}
	return WriteJSONAtomic(ledgerPath, ledger, 0o644)
}

func sameAgentSnapshots(expected, actual []AgentSnapshot) bool {
	if len(expected) != len(actual) {
		return false
	}
	byID := make(map[string]AgentSnapshot, len(expected))
	for _, snapshot := range expected {
		byID[snapshot.AgentID] = snapshot
	}
	for _, snapshot := range actual {
		prior, exists := byID[snapshot.AgentID]
		if !exists || prior.BinarySHA256 != snapshot.BinarySHA256 || !sameSourceSnapshot(prior.Source, snapshot.Source) {
			return false
		}
	}
	return true
}

func sameSourceSnapshot(left, right *AgentSourceSnapshot) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.BaseCommit == right.BaseCommit && left.TreeOID == right.TreeOID && left.PatchSHA256 == right.PatchSHA256 &&
		left.ArchiveSHA256 == right.ArchiveSHA256 && left.PathPolicy.SchemaVersion == right.PathPolicy.SchemaVersion &&
		slices.Equal(left.PathPolicy.ExcludedPrefixes, right.PathPolicy.ExcludedPrefixes) && left.PathPolicySHA256 == right.PathPolicySHA256 &&
		left.ExclusionReceiptSHA256 == right.ExclusionReceiptSHA256 && left.BuildReceiptSHA256 == right.BuildReceiptSHA256
}

func (runner Runner) invalidateOnProtocolError(state *ExperimentState, err error) {
	var invalid InvalidExperimentError
	if errors.As(err, &invalid) {
		state.Status = ExperimentInvalid
		state.InvalidReason = invalid.Reason
	}
}

func verifyBackendSnapshot(manifest Manifest, actual BackendSnapshot) error {
	if err := verifyExecutionCanarySnapshots(manifest.Agents, actual.AgentExecutionCanaries); err != nil {
		return err
	}
	if actual.StorageEnforcement != FormalStorageEnforcement || actual.HostStorageGuard != manifest.Resources.HostStorageGuard || actual.GuestStorageGuard != manifest.Resources.GuestStorageGuard {
		return errors.New("storage enforcement or host guard differs from the frozen manifest")
	}
	if err := ValidateStoragePreflightReceipt(actual.StoragePreflight, manifest.Resources); err != nil {
		return fmt.Errorf("backend outer-host storage preflight: %w", err)
	}
	endpoint := actual.ProviderEndpoint
	if endpoint.ApprovedOrigin != manifest.ProviderEndpoint.ApprovedOrigin ||
		endpoint.SemanticsSHA256 != manifest.ProviderEndpoint.SemanticsSHA256 ||
		endpoint.TLSServerName != FormalProviderTLSServerName ||
		!endpoint.TLSVerified ||
		!hex64Pattern.MatchString(endpoint.TLSPeerLeafCertSHA256) ||
		!hex64Pattern.MatchString(endpoint.TLSPeerSPKISHA256) {
		return errors.New("provider endpoint origin, semantics, or verified TLS peer evidence differs from the frozen manifest")
	}
	if actual.Dataset.Commit != manifest.Dataset.Commit || actual.Dataset.TreeSHA256 != manifest.Dataset.TreeSHA256 || actual.Dataset.ManifestSHA256 != manifest.Dataset.ManifestSHA256 {
		return errors.New("dataset commit, tree hash, or manifest hash differs from the frozen manifest")
	}
	if !hex64Pattern.MatchString(actual.Dataset.RawTreeSHA256) || !hex64Pattern.MatchString(actual.Evaluator.RawTreeSHA256) {
		return errors.New("raw checkout tree evidence is missing")
	}
	if actual.Evaluator.Commit != manifest.Evaluator.Commit || actual.Evaluator.TreeSHA256 != manifest.Evaluator.TreeSHA256 || actual.Evaluator.ManifestSHA256 != manifest.Evaluator.ManifestSHA256 {
		return errors.New("evaluator commit, tree hash, or manifest hash differs from the frozen manifest")
	}
	if actual.EvaluatorVersion == "" {
		return errors.New("evaluator version evidence is missing")
	}
	if actual.EvaluatorBinarySHA256 != manifest.Evaluator.BinarySHA256 {
		return errors.New("evaluator binary differs from the frozen manifest")
	}
	lock := actual.InventoryLock
	if lock.RelativePath != InventoryLockArchiveRelativePath ||
		lock.SchemaVersion != PierInventoryLockSchemaVersion ||
		lock.HashAlgorithm != TaskInventoryHashAlgorithm ||
		lock.FileSHA256 != manifest.Dataset.InventoryLockFileSHA256 ||
		lock.DatasetCommit != manifest.Dataset.Commit ||
		lock.TaskInventorySHA256 != manifest.Dataset.ManifestSHA256 ||
		lock.Coverage != actual.InventoryCoverage ||
		lock.TaskCount != actual.InventoryTaskCount ||
		lock.UniverseTaskCount != actual.UniverseTaskCount {
		return errors.New("archived inventory-lock identity is incomplete or inconsistent")
	}
	if actual.UniverseTaskCount != manifest.Selection.ExpectedTaskCount {
		return errors.New("archived inventory-lock universe differs from the frozen selection")
	}
	switch manifest.Selection.Mode {
	case "full", "sample":
		if lock.Coverage != "full" || lock.TaskCount != lock.UniverseTaskCount {
			return errors.New("full or sampled run lacks a full archived inventory universe")
		}
	case "tasks":
		if lock.Coverage != "tasks" || lock.TaskCount != len(manifest.Selection.TaskIDs) {
			return errors.New("explicit run lacks its exact archived partial inventory")
		}
	default:
		return errors.New("archived inventory-lock has unsupported selection coverage")
	}
	if !actual.AgentNetworkDeny || !actual.VerifierNetworkDeny || actual.NetworkAttestation == "" {
		return errors.New("effective agent and verifier network denial was not attested")
	}
	if actual.AdapterImportPath == "" || actual.AdapterVersion == "" || !hex64Pattern.MatchString(actual.AdapterSHA256) {
		return errors.New("agent adapter import, version, or content hash evidence is missing")
	}
	_, proxyDigest, hasProxyDigest := strings.Cut(actual.EgressProxyImage, "@")
	if !hasProxyDigest || !IsImageDigest(proxyDigest) || !IsImageDigest(actual.EgressProxyImageID) {
		return errors.New("egress proxy image digest or local image identity is missing")
	}
	return nil
}

func verifyExecutionCanarySnapshots(agents []AgentSpec, snapshots []ExecutionCanarySnapshot) error {
	if len(snapshots) != len(agents) {
		return errors.New("backend execution canary set differs from the frozen manifest")
	}
	byID := make(map[string]ExecutionCanarySnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		if _, duplicate := byID[snapshot.AgentID]; duplicate {
			return errors.New("backend execution canary set contains a duplicate agent")
		}
		byID[snapshot.AgentID] = snapshot
	}
	for _, agent := range agents {
		snapshot, exists := byID[agent.ID]
		if !exists || snapshot.Generation != agent.ExecutionCanary.Generation || snapshot.Generation != FormalExecutionCanaryGeneration ||
			snapshot.TransportRequirement != agent.Model.TransportRequirement || snapshot.ReceiptSHA256 != agent.ExecutionCanary.ReceiptSHA256 {
			return errors.New("backend execution canary differs from the frozen v8 agent and transport receipt")
		}
	}
	return nil
}

func validOracleResult(result VerificationResult) bool {
	return result.ProtocolValid && result.Reward == 1 && !math.IsNaN(result.Reward) && !math.IsInf(result.Reward, 0)
}

func validateVerifierArtifacts(root string, paths []string) error {
	if len(paths) == 0 {
		return errors.New("verifier produced no auditable artifacts")
	}
	for _, path := range paths {
		if err := requirePathWithin(root, path); err != nil {
			return fmt.Errorf("invalid verifier artifact: %w", err)
		}
	}
	return nil
}

func findAgent(agents []AgentSpec, id string) (AgentSpec, bool) {
	for _, agent := range agents {
		if agent.ID == id {
			return agent, true
		}
	}
	return AgentSpec{}, false
}

func requirePathWithin(root, path string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if pathAbs != rootAbs && !stringsHasPathPrefix(pathAbs, rootAbs) {
		return errors.New("adapter artifact path escapes the run directory")
	}
	info, err := os.Stat(pathAbs)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("adapter artifact is not a regular file")
	}
	return nil
}

func stringsHasPathPrefix(path, root string) bool {
	return len(path) > len(root) && path[:len(root)] == root && path[len(root)] == filepath.Separator
}
