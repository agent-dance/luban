package report

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/evidenceproxy"
	"github.com/agent-dance/luban/benchmark/agentic/harness"
	"github.com/agent-dance/luban/benchmark/agentic/pierbackend"
)

type publishedBenchmarkValidator func(ExperimentClass, ReportMeta, harness.Manifest, harness.RunPlan, harness.ExperimentState) error

// Compile always applies the registered benchmark contract. The validator is
// injectable only through an unexported function so tests can exercise a tiny
// synthetic artifact bundle without creating a production contract bypass.
func Compile(inputPath string) (Data, error) {
	return compileWithPublishedBenchmarkValidator(inputPath, validatePublishedBenchmarkContract)
}

func compileWithPublishedBenchmarkValidator(inputPath string, contractValidator publishedBenchmarkValidator) (Data, error) {
	input, optimizationLedger, err := LoadInput(inputPath)
	if err != nil {
		return Data{}, err
	}
	base := filepath.Dir(inputPath)
	publicReferences, err := loadPublicReferences(input.PublicReferences)
	if err != nil {
		return Data{}, err
	}
	data := Data{
		Meta: input.Report, Statistics: input.Statistics,
		PublicReferences: publicReferences, Limitations: slices.Clone(input.Limitations),
		DevelopmentContract: input.Report.BenchmarkContractID == BenchmarkContractDeepSWEV11Pilot5,
	}
	for _, source := range input.ArtifactSources {
		experiment, loadErr := loadArtifactExperiment(base, source, input.Report, input.Statistics, contractValidator)
		if loadErr != nil {
			return Data{}, fmt.Errorf("load artifact experiment %s: %w", source.ID, loadErr)
		}
		data.Experiments = append(data.Experiments, experiment)
	}
	for _, diagnostic := range input.DiagnosticExperiments {
		data.Experiments = append(data.Experiments, loadDiagnosticExperiment(diagnostic, input.Report, input.Statistics))
	}
	slices.SortFunc(data.Experiments, func(left, right ExperimentData) int {
		if rank := classRank(right.Class) - classRank(left.Class); rank != 0 {
			return rank
		}
		return strings.Compare(left.ID, right.ID)
	})
	if err := attachFailureAnnotations(&data, input.FailureAnnotations); err != nil {
		return Data{}, err
	}
	optimizations, err := buildOptimizations(data.Experiments, optimizationLedger, input.Statistics)
	if err != nil {
		return Data{}, err
	}
	data.Optimizations = optimizations
	for _, command := range input.Reproduction {
		data.Reproduction = append(data.Reproduction, RenderedCommand{Label: command.Label, Command: shellJoin(command.Argv)})
	}
	for _, experiment := range data.Experiments {
		switch experiment.Class {
		case ClassFormal:
			data.HasFormal = true
		case ClassPilot:
			data.HasPilot = true
		}
	}
	data.HasDiagnosticOnly = !data.HasFormal && !data.HasPilot
	if err := validatePublicReferencePolicy(data); err != nil {
		return Data{}, err
	}
	data.Verdict = buildVerdict(data.Experiments)
	// This guard is intentionally redundant with contract/source validation.
	// A development-set report must remain non-scoreable even if a future
	// refactor accidentally classifies one of its inputs as formal evidence.
	if data.DevelopmentContract {
		data.Verdict.Status = "insufficient"
		for index := range data.Verdict.Criteria {
			data.Verdict.Criteria[index].Passed = nil
		}
	}
	return data, nil
}

func validatePublicReferencePolicy(data Data) error {
	if !data.HasFormal {
		return nil
	}
	if len(data.PublicReferences) != 1 || data.PublicReferences[0].ComputedArtifact != computedDeepSWEGPT56SolXHighReference || data.PublicReferences[0].Computed == nil {
		return errors.New("formal report requires exactly one registered, locally recomputed public reference artifact")
	}
	return nil
}

func loadArtifactExperiment(base string, source ArtifactSource, meta ReportMeta, statistics StatisticsSpec, contractValidator publishedBenchmarkValidator) (ExperimentData, error) {
	root, err := resolveConfigPath(base, source.Root)
	if err != nil {
		return ExperimentData{}, err
	}
	if err := rejectNonRegularArtifactTree(root); err != nil {
		return ExperimentData{}, err
	}
	loadedManifest, err := harness.LoadManifest(filepath.Join(root, "manifest.json"))
	if err != nil {
		return ExperimentData{}, err
	}
	manifest := loadedManifest.Manifest
	ledgerPath, err := pathInside(root, manifest.Artifacts.LedgerRelativePath)
	if err != nil {
		return ExperimentData{}, err
	}
	ledgerFileSHA, err := hashFile(ledgerPath)
	if err != nil {
		return ExperimentData{}, err
	}
	if ledgerFileSHA != source.LedgerFileSHA256 {
		return ExperimentData{}, errors.New("artifact ledger file differs from the report input pin")
	}
	var ledger harness.ArtifactLedger
	if err := decodeStrictFile(ledgerPath, &ledger); err != nil {
		return ExperimentData{}, fmt.Errorf("decode artifact ledger: %w", err)
	}
	if ledger.SchemaVersion != "agentic-bench/artifact-ledger-v1" || ledger.ManifestSHA256 != loadedManifest.SHA256 || !hex64Pattern.MatchString(ledger.LedgerSHA256) {
		return ExperimentData{}, errors.New("artifact ledger identity is invalid")
	}
	recomputedLedger, err := harness.BuildArtifactLedger(root, manifest.Artifacts.LedgerRelativePath, loadedManifest.SHA256)
	if err != nil {
		return ExperimentData{}, fmt.Errorf("recompute artifact ledger: %w", err)
	}
	if !reflect.DeepEqual(ledger, recomputedLedger) {
		return ExperimentData{}, errors.New("artifact inventory, file hashes, or canonical ledger hash changed")
	}

	var plan harness.RunPlan
	if err := decodeStrictFile(filepath.Join(root, "plan.json"), &plan); err != nil {
		return ExperimentData{}, fmt.Errorf("decode run plan: %w", err)
	}
	if plan.SchemaVersion != "agentic-bench/plan-v1" || plan.ManifestSHA256 != loadedManifest.SHA256 {
		return ExperimentData{}, errors.New("run plan does not bind the archived manifest")
	}
	planSHA, err := harness.HashCanonical(plan)
	if err != nil {
		return ExperimentData{}, err
	}
	statePath, err := pathInside(root, manifest.Artifacts.StateRelativePath)
	if err != nil {
		return ExperimentData{}, err
	}
	var state harness.ExperimentState
	if err := decodeStrictFile(statePath, &state); err != nil {
		return ExperimentData{}, fmt.Errorf("decode experiment state: %w", err)
	}
	if state.SchemaVersion != "agentic-bench/state-v2" || state.ManifestSHA256 != loadedManifest.SHA256 || state.PlanSHA256 != planSHA || state.Status != harness.ExperimentComplete || state.CompletedAt == nil {
		return ExperimentData{}, errors.New("experiment state is not a complete run of the archived manifest and plan")
	}
	if state.CompletedAt.After(meta.AsOf) {
		return ExperimentData{}, errors.New("experiment completed after the report evidence cutoff")
	}
	if state.Backend.Dataset.Commit != manifest.Dataset.Commit || state.Backend.Dataset.TreeSHA256 != manifest.Dataset.TreeSHA256 || state.Backend.Dataset.ManifestSHA256 != manifest.Dataset.ManifestSHA256 ||
		state.Backend.Evaluator.Commit != manifest.Evaluator.Commit || state.Backend.Evaluator.TreeSHA256 != manifest.Evaluator.TreeSHA256 || state.Backend.Evaluator.ManifestSHA256 != manifest.Evaluator.ManifestSHA256 ||
		state.Backend.EvaluatorVersion == "" || state.Backend.EvaluatorBinarySHA256 != manifest.Evaluator.BinarySHA256 {
		return ExperimentData{}, errors.New("backend dataset or evaluator snapshot differs from the manifest")
	}
	inventoryLockPath, err := pathInside(root, state.Backend.InventoryLock.RelativePath)
	if err != nil {
		return ExperimentData{}, fmt.Errorf("resolve archived inventory lock: %w", err)
	}
	archivedInventory, err := harness.ValidateInventoryLockArchive(inventoryLockPath, state.Backend.InventoryLock)
	if err != nil {
		return ExperimentData{}, fmt.Errorf("validate archived inventory lock: %w", err)
	}
	archivedInventorySHA, err := harness.HashTaskInventory(archivedInventory)
	if err != nil {
		return ExperimentData{}, err
	}
	if archivedInventorySHA != manifest.Dataset.ManifestSHA256 ||
		state.Backend.InventoryLock.DatasetCommit != manifest.Dataset.Commit ||
		state.Backend.InventoryLock.Coverage != state.Backend.InventoryCoverage ||
		state.Backend.InventoryLock.TaskCount != state.Backend.InventoryTaskCount ||
		state.Backend.InventoryLock.UniverseTaskCount != state.Backend.UniverseTaskCount {
		return ExperimentData{}, errors.New("archived inventory lock differs from the manifest or backend snapshot")
	}
	if !state.Backend.AgentNetworkDeny || !state.Backend.VerifierNetworkDeny || strings.TrimSpace(state.Backend.NetworkAttestation) == "" {
		return ExperimentData{}, errors.New("backend did not preserve effective agent and verifier network-denial attestation")
	}
	_, proxyDigest, hasProxyDigest := strings.Cut(state.Backend.EgressProxyImage, "@")
	if !hasProxyDigest || !harness.IsImageDigest(proxyDigest) || !harness.IsImageDigest(state.Backend.EgressProxyImageID) {
		return ExperimentData{}, errors.New("backend did not preserve an immutable egress-proxy image digest and local image ID")
	}
	if state.Backend.UniverseTaskCount != manifest.Selection.ExpectedTaskCount {
		return ExperimentData{}, errors.New("backend task universe differs from selection.expected_task_count")
	}
	if source.Class == ClassFormal && (state.Backend.InventoryCoverage != "full" || state.Backend.InventoryTaskCount != manifest.Selection.ExpectedTaskCount) {
		return ExperimentData{}, errors.New("formal backend inventory does not cover the full declared task universe")
	}
	if manifest.Selection.Mode == "tasks" && (state.Backend.InventoryCoverage != "tasks" || state.Backend.InventoryTaskCount != len(manifest.Selection.TaskIDs)) {
		return ExperimentData{}, errors.New("explicit pilot backend inventory does not exactly cover the preregistered task IDs")
	}
	if manifest.Selection.Mode == "sample" && (state.Backend.InventoryCoverage != "full" || state.Backend.InventoryTaskCount != manifest.Selection.ExpectedTaskCount) {
		return ExperimentData{}, errors.New("sampled pilot backend inventory does not preserve the full sampling universe")
	}
	if err := validatePlanAndState(source.Class, manifest, plan, state, meta, contractValidator); err != nil {
		return ExperimentData{}, err
	}
	if err := validateOracleArtifactFiles(root, plan, state); err != nil {
		return ExperimentData{}, err
	}
	var scorecard harness.Scorecard
	if err := decodeStrictFile(filepath.Join(root, "scorecard.json"), &scorecard); err != nil {
		return ExperimentData{}, fmt.Errorf("decode scorecard: %w", err)
	}
	recomputedScorecard, err := harness.ScoreExperimentForManifest(loadedManifest, state, plan)
	if err != nil {
		return ExperimentData{}, err
	}
	if !reflect.DeepEqual(scorecard, recomputedScorecard) {
		return ExperimentData{}, errors.New("scorecard differs from state-derived scoring")
	}
	if scorecard.SchemaVersion != "agentic-bench/scorecard-v2" || scorecard.Profile != manifest.Scoring.Profile || scorecard.DeepSWEPublic == nil {
		return ExperimentData{}, errors.New("scorecard does not contain the frozen public scoring profile")
	}

	bundle := formalBundle{Root: root, Manifest: loadedManifest, Plan: plan, State: state, Scorecard: scorecard, Ledger: ledger}
	experiment, err := bundleExperiment(source, bundle, ledgerFileSHA, planSHA, meta, statistics)
	if err != nil {
		return ExperimentData{}, err
	}
	return experiment, nil
}

func validatePlanAndState(class ExperimentClass, manifest harness.Manifest, plan harness.RunPlan, state harness.ExperimentState, meta ReportMeta, contractValidator publishedBenchmarkValidator) error {
	if contractValidator == nil {
		return errors.New("published benchmark validator is required")
	}
	if err := contractValidator(class, meta, manifest, plan, state); err != nil {
		return err
	}
	if class == ClassFormal && manifest.Selection.Mode != "full" {
		return errors.New("formal report source must use the frozen full selection")
	}
	if class == ClassPilot && manifest.Selection.Mode == "full" {
		return errors.New("pilot report source cannot use the full selection")
	}
	if len(manifest.Agents) != 2 || !manifest.Scheduling.PairAgents {
		return errors.New("report comparison requires exactly two paired agents")
	}
	agents := map[string]harness.AgentSpec{}
	for _, agent := range manifest.Agents {
		agents[agent.ID] = agent
	}
	if _, ok := agents[meta.BaselineAgentID]; !ok {
		return fmt.Errorf("baseline agent %s is absent from manifest", meta.BaselineAgentID)
	}
	if _, ok := agents[meta.ContenderAgentID]; !ok {
		return fmt.Errorf("contender agent %s is absent from manifest", meta.ContenderAgentID)
	}
	baseline, contender := agents[meta.BaselineAgentID], agents[meta.ContenderAgentID]
	if manifest.Scoring.BaselineAgentID != meta.BaselineAgentID || manifest.Scoring.ChallengerAgentID != meta.ContenderAgentID {
		return errors.New("report baseline and contender differ from the frozen scoring direction")
	}
	if baseline.Model.Provider != contender.Model.Provider || baseline.Model.Model != contender.Model.Model ||
		baseline.Model.ReasoningEffort != contender.Model.ReasoningEffort || baseline.Model.ServiceTier != contender.Model.ServiceTier {
		return errors.New("paired agents do not share the exact provider/model/reasoning contract")
	}
	selectedTaskCount := selectionTaskCount(manifest.Selection)
	if len(plan.Entries) != selectedTaskCount*manifest.Scheduling.Repetitions*len(manifest.Agents) || len(state.Runs) != len(plan.Entries) {
		return errors.New("plan or state does not cover the declared task universe and repetitions")
	}
	seenRuns := map[string]struct{}{}
	pairs := map[string][]harness.PlanEntry{}
	for index, entry := range plan.Entries {
		if entry.Ordinal != index || entry.Repetition < 0 || entry.Repetition >= manifest.Scheduling.Repetitions {
			return errors.New("plan ordinals or repetitions are invalid")
		}
		if _, ok := agents[entry.AgentID]; !ok {
			return errors.New("plan references an unknown agent")
		}
		key := harness.RunKey(entry)
		if _, exists := seenRuns[key]; exists {
			return errors.New("plan contains a duplicate run key")
		}
		seenRuns[key] = struct{}{}
		pairs[entry.PairID] = append(pairs[entry.PairID], entry)
		record, exists := state.Runs[key]
		if !exists || record.Entry != entry || record.Phase != harness.RunComplete || record.Execution == nil || record.Attempts != 1 {
			return fmt.Errorf("run %s is incomplete", key)
		}
		execution := record.Execution
		controllerRecovery := isRecoveredControllerExclusion(record)
		if controllerRecovery {
			if err := validateRecoveredControllerExecution(record); err != nil {
				return fmt.Errorf("run %s: %w", key, err)
			}
		} else if err := validateNormalControllerExecution(record); err != nil {
			return fmt.Errorf("run %s: %w", key, err)
		}
		hasAgentTiming := !execution.StartedAt.IsZero() || !execution.FinishedAt.IsZero()
		switch record.Disposition {
		case harness.DeepSWEAttemptScored:
			if controllerRecovery || record.Verification == nil || record.Metrics == nil || !hasAgentTiming || execution.Lifecycle.ProviderAttemptState != "provider_attempt_sealed" {
				return fmt.Errorf("scored run %s lacks execution, verifier, or usage evidence", key)
			}
		case harness.DeepSWEAttemptExcluded:
			if record.Verification != nil || !validReportInfrastructureCategory(record.FailureCategory) || (record.Metrics != nil && !hasAgentTiming) ||
				(record.FailureCategory == harness.DeepSWEFailureControllerInfrastructure) != controllerRecovery {
				return fmt.Errorf("excluded run %s lacks a typed infrastructure disposition", key)
			}
		default:
			return fmt.Errorf("run %s has no typed scoring disposition", key)
		}
	}
	uniqueTasks := map[string]struct{}{}
	for pairID, entries := range pairs {
		if len(entries) != len(manifest.Agents) {
			return fmt.Errorf("pair %s is incomplete", pairID)
		}
		seenAgents := map[string]struct{}{}
		for _, entry := range entries {
			if entry.TaskID != entries[0].TaskID || entry.Repetition != entries[0].Repetition {
				return fmt.Errorf("pair %s mixes tasks or repetitions", pairID)
			}
			seenAgents[entry.AgentID] = struct{}{}
			uniqueTasks[entry.TaskID] = struct{}{}
		}
		if len(seenAgents) != len(manifest.Agents) {
			return fmt.Errorf("pair %s contains duplicate agents", pairID)
		}
	}
	if len(uniqueTasks) != selectedTaskCount {
		return errors.New("plan task count differs from the frozen selection")
	}
	if manifest.Selection.Mode == "tasks" {
		selected := slices.Clone(manifest.Selection.TaskIDs)
		slices.Sort(selected)
		if !slices.Equal(sortedMapKeys(uniqueTasks), selected) {
			return errors.New("plan task IDs differ from the explicit preregistered selection")
		}
	}
	selectedTasks := make([]harness.Task, 0, len(uniqueTasks))
	for _, taskID := range sortedMapKeys(uniqueTasks) {
		selectedTasks = append(selectedTasks, harness.Task{ID: taskID})
	}
	expectedPlan, err := harness.BuildPlan(plan.ManifestSHA256, manifest, selectedTasks)
	if err != nil {
		return fmt.Errorf("rebuild frozen paired schedule: %w", err)
	}
	if !reflect.DeepEqual(plan, expectedPlan) {
		return errors.New("run plan differs from the deterministic adjacent paired schedule")
	}
	for _, taskID := range sortedMapKeys(uniqueTasks) {
		oracle, exists := state.Oracle[taskID]
		if !exists || !oracle.Validated || !oracle.Verification.ProtocolValid || oracle.Verification.Reward != 1 {
			return fmt.Errorf("task %s has no passing oracle", taskID)
		}
	}
	if len(state.Oracle) != len(uniqueTasks) {
		return errors.New("state oracle records do not exactly match the frozen task selection")
	}
	snapshots := map[string]harness.AgentSnapshot{}
	for _, snapshot := range state.Agents {
		if _, exists := snapshots[snapshot.AgentID]; exists {
			return errors.New("state contains duplicate agent snapshots")
		}
		snapshots[snapshot.AgentID] = snapshot
	}
	if len(snapshots) != len(manifest.Agents) {
		return errors.New("state agent snapshots are incomplete")
	}
	for _, agent := range manifest.Agents {
		snapshot, exists := snapshots[agent.ID]
		if !exists || snapshot.BinarySHA256 != agent.BinarySHA256 {
			return fmt.Errorf("agent snapshot %s differs from manifest", agent.ID)
		}
		if agent.SourceSnapshot == nil {
			if snapshot.Source != nil {
				return fmt.Errorf("binary-only agent %s has an unexpected source snapshot", agent.ID)
			}
			continue
		}
		expected := harness.AgentSourceSnapshot{
			BaseCommit: agent.SourceSnapshot.BaseCommit, TreeOID: agent.SourceSnapshot.TreeOID,
			PatchSHA256: agent.SourceSnapshot.PatchSHA256, ArchiveSHA256: agent.SourceSnapshot.ArchiveSHA256,
			PathPolicy: agent.SourceSnapshot.PathPolicy, PathPolicySHA256: agent.SourceSnapshot.PathPolicySHA256,
			ExclusionReceiptSHA256: agent.SourceSnapshot.ExclusionReceiptSHA256,
			BuildReceiptSHA256:     agent.SourceSnapshot.BuildReceiptSHA256,
		}
		if snapshot.Source == nil || !reflect.DeepEqual(*snapshot.Source, expected) {
			return fmt.Errorf("source-built agent %s has an inconsistent immutable source snapshot", agent.ID)
		}
	}
	return nil
}

func isRecoveredControllerExclusion(record harness.RunRecord) bool {
	return record.Disposition == harness.DeepSWEAttemptExcluded &&
		record.FailureCategory == harness.DeepSWEFailureControllerInfrastructure &&
		record.Execution != nil && record.Execution.Lifecycle.Recovered
}

func validateRecoveredControllerExecution(record harness.RunRecord) error {
	execution := record.Execution
	if execution == nil {
		return errors.New("controller recovery lacks an execution lifecycle")
	}
	if err := harness.ValidateRecoveredControllerAttempt(*execution); err != nil {
		return err
	}
	if !record.AttemptStartedAt.Equal(execution.Lifecycle.ControllerStartedAt) || record.Verification != nil || record.Metrics != nil {
		return errors.New("controller recovery is not bound to its durable start or fabricates scored evidence")
	}
	return nil
}

func validateNormalControllerExecution(record harness.RunRecord) error {
	execution := record.Execution
	if execution == nil {
		return errors.New("execution lifecycle is absent")
	}
	lifecycle := execution.Lifecycle
	if lifecycle.SchemaVersion != "agentic-bench/attempt-lifecycle-v1" || lifecycle.RunIdentity != execution.EvidenceRunIdentity ||
		!hex64Pattern.MatchString(lifecycle.RunIdentity) || lifecycle.ControllerStartedAt.IsZero() ||
		lifecycle.ControllerFinishedAt.Before(lifecycle.ControllerStartedAt) || lifecycle.Recovered {
		return errors.New("execution lacks a sealed controller lifecycle")
	}
	if execution.TrialStartedAt.IsZero() || execution.TrialFinishedAt.Before(execution.TrialStartedAt) ||
		!record.AttemptStartedAt.Equal(execution.TrialStartedAt) || execution.TrialStartedAt.Before(lifecycle.ControllerStartedAt) ||
		lifecycle.ControllerFinishedAt.Before(execution.TrialFinishedAt) {
		return errors.New("execution is not bound to a Pier trial nested in its controller lifecycle")
	}
	hasAgentTiming := !execution.StartedAt.IsZero() || !execution.FinishedAt.IsZero()
	if hasAgentTiming && (execution.StartedAt.IsZero() || execution.FinishedAt.Before(execution.StartedAt) ||
		execution.StartedAt.Before(execution.TrialStartedAt) || execution.FinishedAt.After(execution.TrialFinishedAt)) {
		return errors.New("execution has invalid nested agent timing")
	}
	switch lifecycle.ProviderAttemptState {
	case "provider_attempt_sealed":
		seal := execution.ProviderEvidence
		if lifecycle.ProviderAttemptCount == 0 || lifecycle.ProviderAttemptCount != seal.StartedAttemptCount ||
			seal.StartedAttemptCount != seal.PersistedAttemptCount || seal.PersistedAttemptCount != seal.RecordCount ||
			!hex64Pattern.MatchString(seal.LastEvidenceHash) {
			return errors.New("controller lifecycle disagrees with the provider evidence seal")
		}
		if execution.EvidencePath == "" || record.Metrics == nil {
			return errors.New("sealed provider attempt lacks normalized usage evidence")
		}
	case "no_provider_attempt":
		if lifecycle.ProviderAttemptCount != 0 || execution.ProviderEvidence != (harness.ProviderEvidenceSeal{}) || execution.EvidencePath != "" || record.Metrics != nil ||
			record.Disposition != harness.DeepSWEAttemptExcluded || record.FailureCategory != harness.DeepSWEFailureNetworkInfrastructure {
			return errors.New("no-provider-attempt lifecycle contains provider evidence or a scoreable outcome")
		}
	default:
		return fmt.Errorf("execution has invalid provider attempt state %q", lifecycle.ProviderAttemptState)
	}
	return nil
}

func validReportInfrastructureCategory(category harness.DeepSWEFailureCategory) bool {
	return category == harness.DeepSWEFailureProviderInfrastructure ||
		category == harness.DeepSWEFailureVerifierInfrastructure ||
		category == harness.DeepSWEFailureNetworkInfrastructure ||
		category == harness.DeepSWEFailureControllerInfrastructure
}

func validateOracleArtifactFiles(root string, plan harness.RunPlan, state harness.ExperimentState) error {
	if len(plan.Entries) == 0 {
		return errors.New("cannot rebase oracle artifacts without a run plan")
	}
	firstEntry := plan.Entries[0]
	firstRecord := state.Runs[harness.RunKey(firstEntry)]
	relativeRunDir := filepath.Join("runs", firstEntry.PairID, firstEntry.AgentID, fmt.Sprintf("attempt-%03d", firstRecord.Attempts))
	cleanArtifactDir := filepath.Clean(firstRecord.ArtifactDir)
	separatorSuffix := string(filepath.Separator) + filepath.Clean(relativeRunDir)
	if !strings.HasSuffix(cleanArtifactDir, separatorSuffix) {
		return errors.New("cannot derive the archived oracle root from state artifact paths")
	}
	originalRoot := strings.TrimSuffix(cleanArtifactDir, separatorSuffix)
	for taskID, oracle := range state.Oracle {
		if len(oracle.Verification.ArtifactPaths) == 0 {
			return fmt.Errorf("oracle task %s has no auditable artifacts", taskID)
		}
		originalOracleRoot := filepath.Join(originalRoot, "oracle", taskID)
		currentOracleRoot, err := pathInside(root, filepath.Join("oracle", taskID))
		if err != nil {
			return err
		}
		for _, original := range oracle.Verification.ArtifactPaths {
			relative, relErr := filepath.Rel(originalOracleRoot, original)
			if relErr != nil || validateRelativePath(relative) != nil {
				return fmt.Errorf("oracle task %s artifact cannot be safely rebased", taskID)
			}
			current, pathErr := pathInside(currentOracleRoot, relative)
			if pathErr != nil {
				return pathErr
			}
			if info, statErr := os.Stat(current); statErr != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("oracle task %s rebased artifact is absent or non-regular", taskID)
			}
		}
	}
	return nil
}

func selectionTaskCount(selection harness.SelectionSpec) int {
	switch selection.Mode {
	case "full":
		return selection.ExpectedTaskCount
	case "tasks":
		return len(selection.TaskIDs)
	case "sample":
		return selection.SampleCount
	default:
		return 0
	}
}

func loadArchivedAgentSource(root string, agent harness.AgentSpec, snapshot harness.AgentSnapshot) (harness.AgentBuildReceipt, error) {
	sourceDir, err := pathInside(root, filepath.Join("sources", agent.ID))
	if err != nil {
		return harness.AgentBuildReceipt{}, err
	}
	if agent.SourceSnapshot == nil {
		if snapshot.Source != nil {
			return harness.AgentBuildReceipt{}, fmt.Errorf("binary-only agent %s has an unexpected source snapshot", agent.ID)
		}
		if _, statErr := os.Lstat(sourceDir); statErr == nil {
			return harness.AgentBuildReceipt{}, fmt.Errorf("binary-only agent %s has unexpected archived source evidence", agent.ID)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return harness.AgentBuildReceipt{}, statErr
		}
		return harness.AgentBuildReceipt{}, nil
	}
	if snapshot.Source == nil {
		return harness.AgentBuildReceipt{}, fmt.Errorf("source-built agent %s has no archived source identity", agent.ID)
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return harness.AgentBuildReceipt{}, fmt.Errorf("read agent %s source archive: %w", agent.ID, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			return harness.AgentBuildReceipt{}, fmt.Errorf("agent %s source archive contains an unexpected directory", agent.ID)
		}
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"build-receipt.json", "source-exclusions.json", "source.patch", "source.tar"}) {
		return harness.AgentBuildReceipt{}, fmt.Errorf("agent %s source archive does not contain the exact frozen evidence set", agent.ID)
	}
	patchPath, err := pathInside(sourceDir, "source.patch")
	if err != nil {
		return harness.AgentBuildReceipt{}, err
	}
	archivePath, err := pathInside(sourceDir, "source.tar")
	if err != nil {
		return harness.AgentBuildReceipt{}, err
	}
	receiptPath, err := pathInside(sourceDir, "build-receipt.json")
	if err != nil {
		return harness.AgentBuildReceipt{}, err
	}
	exclusionPath, err := pathInside(sourceDir, "source-exclusions.json")
	if err != nil {
		return harness.AgentBuildReceipt{}, err
	}
	patchSHA, err := hashFile(patchPath)
	if err != nil {
		return harness.AgentBuildReceipt{}, err
	}
	archiveSHA, err := hashFile(archivePath)
	if err != nil {
		return harness.AgentBuildReceipt{}, err
	}
	receiptSHA, err := hashFile(receiptPath)
	if err != nil {
		return harness.AgentBuildReceipt{}, err
	}
	exclusionSHA, err := hashFile(exclusionPath)
	if err != nil {
		return harness.AgentBuildReceipt{}, err
	}
	if patchSHA != snapshot.Source.PatchSHA256 || archiveSHA != snapshot.Source.ArchiveSHA256 || receiptSHA != snapshot.Source.BuildReceiptSHA256 || exclusionSHA != snapshot.Source.ExclusionReceiptSHA256 {
		return harness.AgentBuildReceipt{}, fmt.Errorf("agent %s archived source evidence differs from the frozen hashes", agent.ID)
	}
	var exclusion harness.SourceExclusionReceipt
	if err := decodeStrictFile(exclusionPath, &exclusion); err != nil {
		return harness.AgentBuildReceipt{}, fmt.Errorf("decode agent %s source exclusion receipt: %w", agent.ID, err)
	}
	if exclusion.SchemaVersion != "agentic-bench/source-exclusion-receipt-v1" || !exclusion.Applied ||
		exclusion.Implementation != "git-negative-pathspec-before-content-scan-v1" ||
		exclusion.PathPolicySHA256 != snapshot.Source.PathPolicySHA256 || !reflect.DeepEqual(exclusion.PathPolicy, snapshot.Source.PathPolicy) {
		return harness.AgentBuildReceipt{}, fmt.Errorf("agent %s source exclusion receipt does not bind the content-blind path policy", agent.ID)
	}
	var receipt harness.AgentBuildReceipt
	if err := decodeStrictFile(receiptPath, &receipt); err != nil {
		return harness.AgentBuildReceipt{}, fmt.Errorf("decode agent %s build receipt: %w", agent.ID, err)
	}
	if receipt.SchemaVersion != "agentic-bench/agent-build-receipt-v2" || receipt.AgentID != agent.ID ||
		receipt.BaseCommit != snapshot.Source.BaseCommit || receipt.TreeOID != snapshot.Source.TreeOID ||
		receipt.PatchSHA256 != snapshot.Source.PatchSHA256 || receipt.ArchiveSHA256 != snapshot.Source.ArchiveSHA256 ||
		!reflect.DeepEqual(receipt.PathPolicy, snapshot.Source.PathPolicy) || receipt.PathPolicySHA256 != snapshot.Source.PathPolicySHA256 ||
		receipt.ExclusionReceiptSHA256 != snapshot.Source.ExclusionReceiptSHA256 ||
		receipt.BinarySHA256 != snapshot.BinarySHA256 || len(receipt.BuildArgv) == 0 || strings.TrimSpace(receipt.Toolchain) == "" || receipt.BuiltAt.IsZero() {
		return harness.AgentBuildReceipt{}, fmt.Errorf("agent %s build receipt does not bind the archived source and binary", agent.ID)
	}
	return receipt, nil
}

func indexArchivedSourceFiles(root, agentID string) (map[string]string, error) {
	archivePath, err := pathInside(root, filepath.Join("sources", agentID, "source.tar"))
	if err != nil {
		return nil, err
	}
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer archiveFile.Close()

	result := map[string]string{}
	reader := tar.NewReader(archiveFile)
	for {
		header, nextErr := reader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("read agent %s source tar: %w", agentID, nextErr)
		}
		name := header.Name
		if header.Typeflag == tar.TypeDir {
			canonicalDirectory := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSuffix(name, "/")))) + "/"
			if name == "/" || canonicalDirectory != name || validateRelativePath(strings.TrimSuffix(name, "/")) != nil {
				return nil, fmt.Errorf("agent %s source tar contains a non-canonical directory", agentID)
			}
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, fmt.Errorf("agent %s source tar contains unsupported entry type %d", agentID, header.Typeflag)
		}
		canonical := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
		if name != canonical || validateRelativePath(filepath.FromSlash(name)) != nil {
			return nil, fmt.Errorf("agent %s source tar contains a non-canonical file path", agentID)
		}
		if _, duplicate := result[name]; duplicate {
			return nil, fmt.Errorf("agent %s source tar contains duplicate file %s", agentID, name)
		}
		digest := sha256.New()
		if _, err := io.Copy(digest, reader); err != nil {
			return nil, fmt.Errorf("hash agent %s archived file %s: %w", agentID, name, err)
		}
		result[name] = hex.EncodeToString(digest.Sum(nil))
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("agent %s source tar contains no regular files", agentID)
	}
	return result, nil
}

func bundleExperiment(source ArtifactSource, bundle formalBundle, ledgerFileSHA, planSHA string, meta ReportMeta, statistics StatisticsSpec) (ExperimentData, error) {
	manifest := bundle.Manifest.Manifest
	if err := harness.ValidateStoragePreflightReceipt(bundle.State.Backend.StoragePreflight, manifest.Resources); err != nil {
		return ExperimentData{}, fmt.Errorf("validate host experiment storage preflight: %w", err)
	}
	normalizedInput, err := harness.DeepSWEPublicInputFromState(bundle.Manifest, bundle.State, bundle.Plan)
	if err != nil {
		return ExperimentData{}, fmt.Errorf("normalize public scoring attempts: %w", err)
	}
	normalizedAttempts := make(map[string]harness.DeepSWEPublicAttempt, len(normalizedInput.Attempts))
	for _, attempt := range normalizedInput.Attempts {
		if _, duplicate := normalizedAttempts[attempt.AttemptID]; duplicate {
			return ExperimentData{}, fmt.Errorf("normalized public attempt %s is duplicated", attempt.AttemptID)
		}
		normalizedAttempts[attempt.AttemptID] = attempt
	}
	experiment := ExperimentData{
		ID: source.ID, Label: source.Label, Class: source.Class, Description: source.Description,
		Manifest: &ManifestData{
			ExperimentID: manifest.Experiment.ID,
			DatasetName:  manifest.Dataset.Name, DatasetRepository: manifest.Dataset.Repository,
			DatasetCommit: manifest.Dataset.Commit, DatasetTreeSHA: manifest.Dataset.TreeSHA256,
			EvaluatorName: manifest.Evaluator.Name, EvaluatorCommit: manifest.Evaluator.Commit,
			EvaluatorVersion: bundle.State.Backend.EvaluatorVersion, EvaluatorProtocol: manifest.Evaluator.Protocol,
			ScoringProfile: manifest.Scoring.Profile,
			SelectionMode:  manifest.Selection.Mode, ExpectedTasks: manifest.Selection.ExpectedTaskCount,
			Repetitions: manifest.Scheduling.Repetitions, PairingSeed: manifest.Scheduling.Seed,
			MaxParallelPairs: manifest.Scheduling.MaxParallelPairs,
			TaskNetwork:      manifest.Environment.TaskNetworkMode, VerifierNetwork: manifest.Environment.VerifierNetworkMode,
			NetworkAttestation: bundle.State.Backend.NetworkAttestation,
			EgressProxyImage:   bundle.State.Backend.EgressProxyImage, EgressProxyImageID: bundle.State.Backend.EgressProxyImageID,
			HostEnvAllowlist: slices.Clone(manifest.Environment.HostEnvAllowlist),
			AgentEgressHosts: slices.Clone(manifest.Environment.AgentEgressHosts), CPUs: manifest.Resources.CPUs,
			MemoryMB: manifest.Resources.MemoryMB, StorageMB: manifest.Resources.StorageMB,
			HostStorageGuard: manifest.Resources.HostStorageGuard, GuestStorageGuard: manifest.Resources.GuestStorageGuard,
			StoragePreflight: bundle.State.Backend.StoragePreflight,
			AgentTimeout:     manifest.Timeouts.AgentSeconds, VerifierTimeout: manifest.Timeouts.VerifierSeconds,
			PricingCurrency: manifest.Pricing.Currency, PricingUnit: manifest.Pricing.UnitTokens,
			PricingAt: manifest.Pricing.EffectiveAt, PricingObservedAt: manifest.Pricing.ObservedAt, PricingSource: manifest.Pricing.SourceURL,
			PricingRates:                 slices.Clone(manifest.Pricing.Rates),
			ProviderOrigin:               manifest.ProviderEndpoint.ApprovedOrigin,
			ProviderSemanticsSHA:         manifest.ProviderEndpoint.SemanticsSHA256,
			ProviderObservationAuthority: manifest.ProviderEndpoint.Semantics.ObservationAuthority,
			ProviderTLSRequired:          manifest.ProviderEndpoint.Semantics.TLSRequired,
			ProviderWebSocketAllowed:     manifest.ProviderEndpoint.Semantics.WebSocketAllowed,
		},
		Hashes: []HashData{
			{Name: "manifest_sha256", Value: bundle.Manifest.SHA256, Source: "manifest.json"},
			{Name: "plan_sha256", Value: planSHA, Source: "plan.json"},
			{Name: "ledger_file_sha256", Value: ledgerFileSHA, Source: manifest.Artifacts.LedgerRelativePath},
			{Name: "ledger_canonical_sha256", Value: bundle.Ledger.LedgerSHA256, Source: manifest.Artifacts.LedgerRelativePath},
			{Name: "dataset_tree_sha256", Value: manifest.Dataset.TreeSHA256, Source: "manifest.json"},
			{Name: "evaluator_tree_sha256", Value: manifest.Evaluator.TreeSHA256, Source: "manifest.json"},
		},
	}
	snapshots := map[string]harness.AgentSnapshot{}
	for _, snapshot := range bundle.State.Agents {
		snapshots[snapshot.AgentID] = snapshot
	}
	for _, agent := range manifest.Agents {
		snapshot := snapshots[agent.ID]
		agentData := ManifestAgentData{
			ID: agent.ID, Provider: agent.Model.Provider, Model: agent.Model.Model,
			ReasoningEffort: agent.Model.ReasoningEffort, ServiceTier: agent.Model.ServiceTier, BinarySHA256: agent.BinarySHA256,
			ServiceTierRequestEncoding: agent.Model.ServiceTierRequestEncoding,
			Argv:                       slices.Clone(agent.Command.Argv),
		}
		receipt, sourceErr := loadArchivedAgentSource(bundle.Root, agent, snapshot)
		if sourceErr != nil {
			return ExperimentData{}, sourceErr
		}
		if snapshot.Source != nil {
			archivedFiles, archiveErr := indexArchivedSourceFiles(bundle.Root, agent.ID)
			if archiveErr != nil {
				return ExperimentData{}, archiveErr
			}
			agentData.SourceBaseCommit = snapshot.Source.BaseCommit
			agentData.SourceTreeOID = snapshot.Source.TreeOID
			agentData.SourcePatchSHA = snapshot.Source.PatchSHA256
			agentData.SourceArchiveSHA = snapshot.Source.ArchiveSHA256
			agentData.BuildReceiptSHA = snapshot.Source.BuildReceiptSHA256
			agentData.BuildArgv = slices.Clone(receipt.BuildArgv)
			agentData.BuildToolchain = receipt.Toolchain
			agentData.ArchivedSourceFiles = archivedFiles
		}
		experiment.Manifest.Agents = append(experiment.Manifest.Agents, agentData)
		experiment.Hashes = append(experiment.Hashes, HashData{Name: agent.ID + "_binary_sha256", Value: agent.BinarySHA256, Source: "manifest.json"})
		if snapshot.Source != nil {
			experiment.Hashes = append(experiment.Hashes,
				HashData{Name: agent.ID + "_source_archive_sha256", Value: snapshot.Source.ArchiveSHA256, Source: filepath.Join("sources", agent.ID, "source.tar")},
				HashData{Name: agent.ID + "_build_receipt_sha256", Value: snapshot.Source.BuildReceiptSHA256, Source: filepath.Join("sources", agent.ID, "build-receipt.json")},
			)
		}
	}
	for index, entry := range bundle.Plan.Entries {
		record := bundle.State.Runs[harness.RunKey(entry)]
		agent := findManifestAgent(manifest.Agents, entry.AgentID)
		position := "first"
		if index%2 == 1 {
			position = "second"
		}
		run, err := loadFormalRun(bundle.Root, source, agent, record, manifest.Resources, manifest.Pricing, manifest.ProviderEndpoint, position)
		if err != nil {
			return ExperimentData{}, fmt.Errorf("run %s: %w", harness.RunKey(entry), err)
		}
		normalized, exists := normalizedAttempts[run.AttemptID]
		if !exists || normalized.AgentID != run.AgentID || normalized.TaskID != run.TaskID || normalized.Slot != run.Repetition+1 || string(normalized.Disposition) != run.Disposition || string(normalized.FailureCategory) != run.FailureCategory {
			return ExperimentData{}, fmt.Errorf("run %s differs from its normalized public scoring attempt", run.AttemptID)
		}
		run.Passed = cloneBool(normalized.Passed)
		delete(normalizedAttempts, run.AttemptID)
		experiment.Runs = append(experiment.Runs, run)
		experiment.ProviderRounds = append(experiment.ProviderRounds, run.Rounds...)
	}
	if len(normalizedAttempts) != 0 {
		return ExperimentData{}, errors.New("normalized public scoring attempts contain entries outside the report runs")
	}
	applySymmetricComparableCostBasis(&experiment)
	finalizeExperiment(&experiment, meta, statistics)
	experiment.PublicScorecard = publicScorecardWithoutCost(bundle.Scorecard.DeepSWEPublic)
	experiment.Gates = artifactGates(experiment, bundle)
	return experiment, nil
}

// publicScorecardWithoutCost preserves the frozen public quality score while
// deliberately removing its inference-only cost projection. Formal cost is
// reported exclusively from the all-transport, symmetric comparable basis.
func publicScorecardWithoutCost(scorecard *harness.DeepSWEPublicScorecard) *harness.DeepSWEPublicScorecard {
	if scorecard == nil {
		return nil
	}
	result := *scorecard
	result.Agents = slices.Clone(scorecard.Agents)
	for index := range result.Agents {
		efficiency := &result.Agents[index].AllExecutedEfficiency
		efficiency.CostUSD = harness.DeepSWEFloatAggregate{Total: efficiency.Attempts}
	}
	return &result
}

// applySymmetricComparableCostBasis applies one same-gateway frozen rate card
// to visible all-transport input, cached-input, and output receipts. Cache-write
// premiums and provider invoices are intentionally excluded: they are useful
// audits but are not emitted consistently enough to be the pilot headline.
// Missing usage or served-model/tier identity remains unknown per run, so
// partial pilot evidence can render without fabricating a zero; a formal run's
// complete-spend gate still requires every run to have this comparable value.
func applySymmetricComparableCostBasis(experiment *ExperimentData) {
	if experiment == nil || len(experiment.Runs) == 0 {
		return
	}
	for index := range experiment.Runs {
		metrics := &experiment.Runs[index].Metrics
		metrics.ComparableCostBasis = comparableCostBasisUnknown
		metrics.ComparableCost = nil
		if metrics.TransportAttempts == nil || *metrics.TransportAttempts < 0 || metrics.PrewarmAttempts == nil || *metrics.PrewarmAttempts < 0 || *metrics.PrewarmAttempts > *metrics.TransportAttempts {
			continue
		}
		transportAttempts := *metrics.TransportAttempts
		usageComplete := metrics.AllExecutedUsageObserved == transportAttempts && metrics.AllExecutedUsageTotal == transportAttempts
		identityComplete := metrics.CostIdentityUnknownAttempts != nil && *metrics.CostIdentityUnknownAttempts == 0
		if !usageComplete || !identityComplete || metrics.KnownCatalogCostLowerBound == nil {
			continue
		}
		visibleTokenCost := *metrics.KnownCatalogCostLowerBound
		if metrics.KnownCacheWriteSurcharge != nil {
			visibleTokenCost -= *metrics.KnownCacheWriteSurcharge
		}
		if visibleTokenCost < 0 || math.IsNaN(visibleTokenCost) || math.IsInf(visibleTokenCost, 0) {
			continue
		}
		metrics.ComparableCostBasis = comparableCostBasisFrozen
		metrics.ComparableCost = pointerFloat(visibleTokenCost)
	}
}

func loadFormalRun(root string, source ArtifactSource, agent harness.AgentSpec, record harness.RunRecord, resources harness.ResourceSpec, pricing harness.PricingCatalog, endpoint harness.ProviderEndpointSpec, executionPosition string) (RunData, error) {
	entry := record.Entry
	execution := record.Execution
	relativeArtifactDir := filepath.Join("runs", entry.PairID, entry.AgentID, fmt.Sprintf("attempt-%03d", record.Attempts))
	artifactDir, err := pathInside(root, relativeArtifactDir)
	if err != nil {
		return RunData{}, err
	}
	if !pathEndsWith(record.ArtifactDir, relativeArtifactDir) {
		return RunData{}, errors.New("state artifact_dir cannot be safely rebased into the ledger root")
	}
	var trialWall *float64
	if !isRecoveredControllerExclusion(record) {
		trialWall, err = finiteDuration(execution.TrialStartedAt, execution.TrialFinishedAt)
		if err != nil {
			return RunData{}, err
		}
	}
	run := RunData{
		AttemptID: harness.RunKey(entry), ExperimentID: source.ID, Class: source.Class, PairID: entry.PairID, TaskID: entry.TaskID,
		AgentID: entry.AgentID, Variant: entry.AgentID, Provider: agent.Model.Provider, Model: agent.Model.Model,
		ReasoningEffort: agent.Model.ReasoningEffort, Repetition: entry.Repetition, Attempt: record.Attempts,
		ExecutionPosition: executionPosition,
		Disposition:       string(record.Disposition), FailureCategory: string(record.FailureCategory),
		AttemptStartedAt:    record.AttemptStartedAt,
		ControllerStartedAt: execution.Lifecycle.ControllerStartedAt, ControllerFinishedAt: execution.Lifecycle.ControllerFinishedAt,
		ControllerRecovered: execution.Lifecycle.Recovered, ProviderAttemptState: execution.Lifecycle.ProviderAttemptState,
		ProviderAttemptCount: execution.Lifecycle.ProviderAttemptCount,
		TrialStartedAt:       execution.TrialStartedAt, TrialFinishedAt: execution.TrialFinishedAt, TrialDurationSeconds: trialWall,
	}
	normalizedOutcome, err := harness.NormalizeDeepSWEPublicOutcome(record)
	if err != nil {
		return RunData{}, fmt.Errorf("normalize public scoring outcome: %w", err)
	}
	run.Passed = cloneBool(normalizedOutcome.Passed)
	run.Metrics.TrialDurationSeconds = cloneFloat(trialWall)
	if !isRecoveredControllerExclusion(record) {
		if err := harness.ValidateStorageAdmissionReceipt(record.StorageAdmission, resources); err != nil {
			return RunData{}, fmt.Errorf("validate host storage admission: %w", err)
		}
		run.StorageAdmission = record.StorageAdmission
		if execution.Lifecycle.ProviderAttemptState == "provider_attempt_sealed" {
			if err := harness.ValidateStorageResourceEvidence(artifactDir, execution.StorageEvidence, record.StorageAdmission, resources); err != nil {
				return RunData{}, fmt.Errorf("validate host storage runtime evidence: %w", err)
			}
			if err := harness.ValidateGuestStorageResourceEvidence(artifactDir, execution.GuestStorageEvidence, resources); err != nil {
				return RunData{}, fmt.Errorf("validate guest storage runtime evidence: %w", err)
			}
			run.HostStorageEvidence = execution.StorageEvidence
			run.GuestStorageEvidence = slices.Clone(execution.GuestStorageEvidence)
		} else if !reflect.DeepEqual(execution.StorageEvidence, harness.StorageResourceEvidence{}) || len(execution.GuestStorageEvidence) != 0 || execution.ServiceTierCanonicalization != (harness.ServiceTierCanonicalizationEvidence{}) {
			return RunData{}, errors.New("no-provider attempt contains post-WAL runtime or service-tier evidence")
		}
	}
	if isRecoveredControllerExclusion(record) {
		startedAttempts := int(execution.Lifecycle.ProviderAttemptCount)
		run.Metrics.TransportAttempts = pointerInt(startedAttempts)
		run.Metrics.UnknownCostAttempts = pointerInt(startedAttempts)
		run.Metrics.CostReceiptTotal = startedAttempts
		run.Metrics.AllExecutedUsageTotal = startedAttempts
		run.Metrics.AllExecutedCacheWriteTotal = startedAttempts
	} else if execution.Lifecycle.ProviderAttemptState == "no_provider_attempt" {
		run.Metrics.TransportAttempts = pointerInt(0)
		run.Metrics.PrewarmAttempts = pointerInt(0)
		run.Metrics.PrewarmErrors = pointerInt(0)
		run.Metrics.LLMCallsStarted = pointerInt(0)
		run.Metrics.CompletedLLMResponses = pointerInt(0)
		run.Metrics.HTTPInferenceRequests = pointerInt(0)
		run.Metrics.WebSocketInferenceRequests = pointerInt(0)
		run.Metrics.WebSocketConnections = pointerInt(0)
		run.Metrics.PrewarmUsageObservations = pointerInt(0)
		run.Metrics.PrewarmInputTokens = pointerInt64(0)
		run.Metrics.PrewarmCachedInputTokens = pointerInt64(0)
		run.Metrics.PrewarmOutputTokens = pointerInt64(0)
		run.Metrics.PrewarmUnknownCostAttempts = pointerInt(0)
		run.Metrics.ProviderRequests = pointerInt(0)
		run.Metrics.ProviderRounds = pointerInt(0)
		run.Metrics.ProviderErrors = pointerInt(0)
		run.Metrics.UnknownCostAttempts = pointerInt(0)
		run.Metrics.CostIdentityUnknownAttempts = pointerInt(0)
		run.Metrics.CatalogCost = pointerFloat(0)
		run.Metrics.KnownCatalogCostLowerBound = pointerFloat(0)
		run.Metrics.AllExecutedInputTokens = pointerInt64(0)
		run.Metrics.AllExecutedCachedTokens = pointerInt64(0)
		run.Metrics.AllExecutedUncachedTokens = pointerInt64(0)
		run.Metrics.AllExecutedNonCachedBaseTokens = pointerInt64(0)
		run.Metrics.AllExecutedCacheWriteInputTokens = pointerInt64(0)
		run.Metrics.AllExecutedOutputTokens = pointerInt64(0)
	}
	if !execution.StartedAt.IsZero() {
		wall, durationErr := finiteDuration(execution.StartedAt, execution.FinishedAt)
		if durationErr != nil {
			return RunData{}, durationErr
		}
		exitCode := execution.ExitCode
		run.ExitClass, run.ExitCode = execution.ExitClass, &exitCode
		run.StartedAt, run.FinishedAt = execution.StartedAt, execution.FinishedAt
		run.Metrics.WallTimeSeconds = wall
	}
	if record.Disposition == harness.DeepSWEAttemptScored {
		if execution.SubmissionPatch == "" || execution.AuditWorkspacePatch == "" {
			return RunData{}, errors.New("scored run lacks its official submission or audit workspace patch")
		}
		officialFile, hashErr := rebaseAndHashRunFile(artifactDir, record.ArtifactDir, execution.SubmissionPatch)
		if hashErr != nil {
			return RunData{}, fmt.Errorf("official submission patch: %w", hashErr)
		}
		auditFile, hashErr := rebaseAndHashRunFile(artifactDir, record.ArtifactDir, execution.AuditWorkspacePatch)
		if hashErr != nil {
			return RunData{}, fmt.Errorf("audit workspace patch: %w", hashErr)
		}
		if officialFile.Relative != "submission.patch" || auditFile.Relative != "audit-workspace.patch" || officialFile.Path == auditFile.Path {
			return RunData{}, errors.New("official submission and audit workspace patches do not use distinct frozen artifact paths")
		}
		capture := execution.Capture
		if capture.Method != "official-git-diff+temporary-index-audit-v2" || !hex40Pattern.MatchString(capture.BaseCommit) ||
			capture.PatchSHA256 != officialFile.SHA256 || capture.AuditPatchSHA256 != auditFile.SHA256 ||
			capture.UncommittedChangesPresent != (officialFile.SHA256 != auditFile.SHA256) || !capture.IncludesTracked || !capture.IncludesUntracked || !capture.IncludesBinary {
			return RunData{}, errors.New("official submission and audit capture receipt is incomplete or mismatched")
		}
		var archivedCapture struct {
			SchemaVersion string `json:"schema_version"`
			harness.SubmissionCaptureEvidence
		}
		archivedCapturePath, pathErr := pathInside(artifactDir, filepath.Join("pier", "agent-workspace-capture.json"))
		if pathErr != nil {
			return RunData{}, pathErr
		}
		if decodeErr := decodeStrictFile(archivedCapturePath, &archivedCapture); decodeErr != nil {
			return RunData{}, fmt.Errorf("decode archived workspace capture receipt: %w", decodeErr)
		}
		if archivedCapture.SchemaVersion != "agentic-bench/workspace-capture-v2" || !reflect.DeepEqual(archivedCapture.SubmissionCaptureEvidence, capture) {
			return RunData{}, errors.New("state capture evidence differs from the archived workspace-capture-v2 receipt")
		}
	}
	if record.Disposition == harness.DeepSWEAttemptScored {
		if len(record.Verification.ArtifactPaths) == 0 {
			return RunData{}, errors.New("verifier has no auditable artifacts")
		}
		verifierRoot := filepath.Join(artifactDir, "verifier")
		originalVerifierRoot := filepath.Join(record.ArtifactDir, "verifier")
		for _, original := range record.Verification.ArtifactPaths {
			relative, relErr := filepath.Rel(originalVerifierRoot, original)
			if relErr != nil || validateRelativePath(relative) != nil {
				return RunData{}, errors.New("verifier artifact cannot be safely rebased")
			}
			current, pathErr := pathInside(verifierRoot, relative)
			if pathErr != nil {
				return RunData{}, pathErr
			}
			if info, statErr := os.Stat(current); statErr != nil || !info.Mode().IsRegular() {
				return RunData{}, errors.New("rebased verifier artifact is absent or non-regular")
			}
		}
	}
	if record.Metrics == nil {
		if execution.EvidencePath != "" {
			return RunData{}, errors.New("run has provider evidence but no state usage metrics")
		}
		return run, nil
	}
	evidencePath, err := pathInside(artifactDir, agent.RequestEvidence.RelativePath)
	if err != nil {
		return RunData{}, err
	}
	relativeEvidence, relErr := filepath.Rel(record.ArtifactDir, execution.EvidencePath)
	if execution.EvidencePath == "" || relErr != nil || filepath.Clean(relativeEvidence) != filepath.FromSlash(agent.RequestEvidence.RelativePath) {
		return RunData{}, errors.New("execution evidence path differs from the manifest path")
	}
	rounds, err := harness.ReadJSONLines[harness.ProviderRoundEvidence](evidencePath)
	if err != nil {
		return RunData{}, err
	}
	if !hex64Pattern.MatchString(execution.EvidenceRunIdentity) {
		return RunData{}, errors.New("execution has an invalid provider-evidence run identity")
	}
	for _, round := range rounds {
		if round.RunIdentity != execution.EvidenceRunIdentity {
			return RunData{}, errors.New("provider evidence crosses the sealed execution run identity")
		}
		if round.ProviderAttemptKind == "inference" {
			run.ToolCatalogObserved++
		}
		for _, definition := range round.ToolDefinitions {
			if reportCollaborationToolName(definition.Name) {
				run.NestedToolDefinitions++
			}
		}
	}
	metrics, err := harness.ValidateAndAggregateEvidence(rounds, agent.Model, pricing)
	if err != nil {
		return RunData{}, err
	}
	rawProviderEvidence, err := validateArchivedProviderSeal(artifactDir, record.ArtifactDir, execution, rounds)
	if err != nil {
		return RunData{}, err
	}
	if err := harness.ValidateServiceTierCanonicalizationArchive(artifactDir, agent.ID, *execution, rounds); err != nil {
		return RunData{}, fmt.Errorf("validate service-tier canonicalization archive: %w", err)
	}
	if err := pierbackend.ValidateArchivedProviderProjection(
		rawProviderEvidence,
		execution.ProviderEvidence.RawEvidenceSHA256,
		rounds,
		agent,
		execution.EvidenceRunIdentity,
		execution.ServiceTierCanonicalization.BindingSHA256,
		endpoint,
	); err != nil {
		return RunData{}, fmt.Errorf("validate archived raw-v6 provider projection: %w", err)
	}
	run.ServiceTierEvidence = execution.ServiceTierCanonicalization
	run.CacheInitialState = "unknown"
	run.CacheEvidenceClass = "observational"
	if !reflect.DeepEqual(*record.Metrics, metrics) {
		return RunData{}, errors.New("state usage metrics differ from normalized provider evidence")
	}
	run.Metrics.TransportAttempts = pointerInt(metrics.TransportAttempts)
	run.Metrics.PrewarmAttempts = pointerInt(metrics.PrewarmAttempts)
	run.Metrics.PrewarmErrors = pointerInt(metrics.PrewarmErrors)
	// The frozen evidence validator admits an inference round only when the
	// transport attempt started with generate=true. This all-started count is
	// the headline efficiency unit; successful provider responses and committed
	// controller turns remain separate diagnostics.
	llmCallsStarted := metrics.TransportAttempts - metrics.PrewarmAttempts
	if llmCallsStarted != metrics.ProviderRequests {
		return RunData{}, errors.New("all-started LLM-call count differs from transport attempts minus prewarm attempts")
	}
	run.Metrics.LLMCallsStarted = pointerInt(llmCallsStarted)
	if metrics.LLMCallsStarted != llmCallsStarted {
		return RunData{}, errors.New("normalized LLM-call count differs from all-started inference attempts")
	}
	run.Metrics.CompletedLLMResponses = pointerInt(metrics.CompletedLLMResponses)
	run.Metrics.RetryAmplification = cloneFloat(metrics.RetryAmplification)
	run.Metrics.CachePolicyObservedRequests = pointerInt(metrics.CachePolicyObservedRequests)
	run.Metrics.CacheKeyPresentRequests = pointerInt(metrics.CacheKeyPresentRequests)
	run.Metrics.CacheUniqueKeyCount = pointerInt(metrics.CacheUniqueKeyCount)
	run.Metrics.CacheKeyTransitions = pointerInt(metrics.CacheKeyTransitions)
	run.Metrics.CacheLineageStable = cloneBool(&metrics.CacheLineageStable)
	run.Metrics.HTTPInferenceRequests = pointerInt(metrics.HTTPInferenceRequests)
	run.Metrics.WebSocketInferenceRequests = pointerInt(metrics.WebSocketInferenceRequests)
	run.Metrics.WebSocketConnections = pointerInt(metrics.WebSocketConnections)
	run.Metrics.PrewarmUsageObservations = pointerInt(metrics.PrewarmUsageObservations)
	run.Metrics.PrewarmUnknownCostAttempts = pointerInt(metrics.PrewarmUnknownCostAttempts)
	run.Metrics.UnknownCostAttempts = pointerInt(metrics.UnknownCostAttempts)
	run.Metrics.CostReceiptObserved, run.Metrics.CostReceiptTotal = metrics.CostReceiptObservations, metrics.CostReceiptTotal
	run.Metrics.AllExecutedUsageObserved = metrics.AllExecutedUsageObservations
	run.Metrics.AllExecutedUsageTotal = metrics.TransportAttempts
	run.Metrics.AllExecutedCacheWriteObserved = metrics.AllExecutedCacheWriteObservations
	run.Metrics.AllExecutedCacheWriteTotal = metrics.AllExecutedUsageObservations
	run.Metrics.AllExecutedUnreportedCacheWrite = metrics.AllExecutedUnreportedCacheWriteAttempts
	run.Metrics.KnownCatalogCostLowerBound = pointerFloat(metrics.KnownCatalogCostLowerBound)
	if metrics.PrewarmUsageObservations > 0 {
		run.Metrics.PrewarmInputTokens = pointerInt64(metrics.PrewarmInputTokens)
		run.Metrics.PrewarmCachedInputTokens = pointerInt64(metrics.PrewarmCachedInputTokens)
		run.Metrics.PrewarmOutputTokens = pointerInt64(metrics.PrewarmOutputTokens)
	}
	if err := applyAllExecutedTokenPartition(&run.Metrics, metrics); err != nil {
		return RunData{}, err
	}
	run.Metrics.AllExecutedCacheHit = cloneFloat(metrics.AllExecutedCacheHitRate)
	run.Metrics.ProviderRequests = pointerInt(metrics.ProviderRequests)
	run.Metrics.ProviderRounds = pointerInt(metrics.ProviderRounds)
	run.Metrics.ProviderErrors = pointerInt(metrics.ProviderErrors)
	run.Metrics.ToolBearingRounds = pointerInt(metrics.ToolBearingRounds)
	run.Metrics.ToolInvocations = pointerInt(metrics.ToolInvocations)
	run.Metrics.ToolTraceMatched = pointerInt(metrics.ToolTraceIDMatches + metrics.ToolTraceOrderedMatches)
	run.Metrics.ToolTraceUnmatched = pointerInt(metrics.ToolTraceUnmatched)
	if metrics.UsageReceiptObservations == metrics.UsageReceiptTotal && metrics.UsageReceiptTotal > 0 {
		run.Metrics.InputTokens = pointerInt64(metrics.InputTokens)
		run.Metrics.CachedInputTokens = pointerInt64(metrics.CachedInputTokens)
		run.Metrics.OutputTokens = pointerInt64(metrics.OutputTokens)
		miss := metrics.InputTokens - metrics.CachedInputTokens
		run.Metrics.CacheMissTokens = &miss
	}
	if metrics.ReasoningTokenObservations == metrics.UsageReceiptTotal && metrics.UsageReceiptTotal > 0 {
		run.Metrics.ReasoningOutputTokens = pointerInt64(metrics.ReasoningOutputTokens)
	}
	run.Metrics.TokenWeightedCacheHit = cloneFloat(metrics.CacheHitRate)
	run.Metrics.CatalogCost = cloneFloat(metrics.CatalogCost)
	run.Metrics.CatalogCostPartial = cloneFloat(metrics.CatalogCostPartial)
	run.Metrics.ProviderReportedCost = cloneFloat(metrics.ProviderReportedCost)
	run.Metrics.ProviderCostPartial = cloneFloat(metrics.ProviderReportedCostPartial)
	run.Metrics.CacheWriteTokenObserved, run.Metrics.CacheWriteTokenTotal = metrics.CacheWriteTokenObservations, metrics.UsageReceiptTotal
	run.Metrics.UnreportedCacheWriteRounds = metrics.UnreportedCacheWriteRounds
	if metrics.CacheWriteTokenObservations == metrics.UsageReceiptTotal && metrics.UsageReceiptTotal > 0 {
		run.Metrics.CacheWriteInputTokens = pointerInt64(metrics.CacheWriteInputTokens)
	}
	if metrics.UsageReceiptObservations > 0 {
		run.Metrics.KnownCacheWriteSurcharge = pointerFloat(metrics.KnownCacheWriteSurcharge)
	}
	requestHits, requestObserved := 0, 0
	costIdentityUnknown := 0
	physicalCoverage, criticalCoverage, totalLatencyCoverage, queueCoverage := 0, 0, 0, 0
	toolByName := map[string]*ToolData{}
	for _, round := range rounds {
		if round.StartedAt.Before(record.Execution.StartedAt) || round.FinishedAt.After(record.Execution.FinishedAt) {
			return RunData{}, fmt.Errorf("provider round %d falls outside agent-only timing", round.Round)
		}
		if err := harness.ValidateCacheRequestEvidence(round); err != nil {
			return RunData{}, fmt.Errorf("provider round %d cache evidence: %w", round.Round, err)
		}
		if cacheHit, eligible := eligibleRequestCacheObservation(round); eligible {
			requestObserved++
			if cacheHit {
				requestHits++
			}
		}
		if round.ResponseModel != agent.Model.Model || round.ResponseServiceTierRaw != harness.FormalServiceTier {
			costIdentityUnknown++
		}
		roundView := RoundData{
			ExperimentID: source.ID, AgentID: entry.AgentID, TaskID: entry.TaskID, Repetition: entry.Repetition,
			Round: round.Round, Outcome: round.Outcome, ErrorCode: round.ErrorCode,
			Transport: round.Transport, ProviderAttemptKind: round.ProviderAttemptKind, TransportDisposition: round.TransportDisposition,
			RequestIDHash: round.RequestID, ResponseIDHash: round.ResponseIDHash,
			RequestedReasoningMode: round.RequestedReasoningMode, ReasoningModeCanonical: round.RequestedReasoningModeCanonical,
			RequestedTextVerbosity:   round.RequestedTextVerbosity,
			MaxOutputTokensSpecified: round.MaxOutputTokensSpecified, MaxOutputTokens: cloneInt64(round.MaxOutputTokens),
			RequestedServiceTier: round.RequestedServiceTierRaw, ResponseServiceTier: round.ResponseServiceTierRaw,
			RequestedServiceTierPresent:        round.RequestedServiceTierPresent,
			RequestedServiceTierCanonical:      round.RequestedServiceTierCanonical,
			RequestedServiceTierRepresentation: round.RequestedServiceTierRepresentation,
			ClientCanonicalizationProofSHA256:  round.ClientCanonicalizationProofSHA256, ClientAgentID: round.ClientAgentID,
			OriginalRequestBodySHA256: round.OriginalRequestBodySHA256, ForwardedRequestBodySHA256: round.ForwardedRequestBodySHA256,
			OriginalRequestCanonicalSHA256: round.OriginalRequestCanonicalSHA256, ForwardedRequestCanonicalSHA256: round.ForwardedRequestCanonicalSHA256,
			OriginalWithoutTierSHA256: round.OriginalRequestWithoutServiceTierSHA256, ForwardedWithoutTierSHA256: round.ForwardedRequestWithoutServiceTierSHA256,
			OriginalServiceTierPresent: round.OriginalServiceTierPresent, OriginalServiceTier: round.OriginalServiceTier,
			ForwardedServiceTierPresent: round.ForwardedServiceTierPresent, ForwardedServiceTier: round.ForwardedServiceTier,
			ForwardedRequestBytes: round.ForwardedRequestBytes, ServiceTierTransformation: round.ServiceTierTransformation,
			ServiceTierTransformationExactDiff:   round.ServiceTierTransformationExactDiff,
			ServiceTierTransformationProofSHA256: round.ServiceTierTransformationProofSHA256,
			ResponseServiceTierCanonical:         round.ResponseServiceTierCanonical, ServiceTierComparable: round.ServiceTierComparable,
			ResponseCreatedModel: round.ResponseCreatedModel, ResponseModel: round.ResponseModel,
			StartedAt: round.StartedAt, FinishedAt: round.FinishedAt,
			ProviderMS:  round.FinishedAt.Sub(round.StartedAt).Seconds() * 1000,
			ToolCalls:   len(round.ToolCalls),
			InputTokens: cloneInt64(round.InputTokens), CachedInputTokens: cloneInt64(round.CachedInputTokens),
			CacheWriteTokens: cloneInt64(round.CacheWriteInputTokens), OutputTokens: cloneInt64(round.OutputTokens),
			PromptCacheKeyPresent: round.PromptCacheKeyPresent, PromptCacheKeyHash: round.PromptCacheKeyHash,
			CachePolicyObserved: round.CachePolicyObserved, PromptCacheOptionsPresent: round.PromptCacheOptionsPresent,
			PromptCacheOptionsMode: round.PromptCacheOptionsMode, PromptCacheTTLSeconds: cloneInt64(round.PromptCacheTTLSeconds),
			PromptCacheRetentionPresent: round.PromptCacheRetentionPresent, PromptCacheRetention: round.PromptCacheRetention,
			CacheBreakpointCount: round.CacheBreakpointCount, CacheBreakpointPositionHashes: slices.Clone(round.CacheBreakpointPositionHashes),
		}
		if round.Outcome == "success" || round.Outcome == "prewarm" {
			if round.Transport == "http_sse" {
				roundView.HeadersMS = pointerFloat(round.UpstreamHeadersAt.Sub(round.StartedAt).Seconds() * 1000)
				roundView.FirstByteMS = pointerFloat(round.FirstResponseByteAt.Sub(round.UpstreamHeadersAt).Seconds() * 1000)
			} else {
				roundView.FirstByteMS = pointerFloat(round.FirstResponseByteAt.Sub(round.StartedAt).Seconds() * 1000)
			}
			roundView.StreamMS = pointerFloat(round.FinishedAt.Sub(round.FirstResponseByteAt).Seconds() * 1000)
		}
		if round.UsagePresent && round.CachedInputTokens != nil {
			cacheHit := *round.CachedInputTokens > 0
			roundView.CacheHit = &cacheHit
		}
		roundView.PhysicalTools = cloneInt(round.PhysicalToolOperations)
		roundView.ToolCriticalMS = cloneInt64(round.ToolCriticalPathMS)
		roundView.ToolTotalMS = cloneInt64(round.ToolTotalLatencyMS)
		roundView.ToolQueueMS = cloneInt64(round.ToolQueueMS)
		// Operational tool measurements are semantically zero on a round with
		// no logical tool calls. On tool-bearing rounds, only an explicit
		// provider/adapter receipt counts as observed.
		if round.ProviderAttemptKind == "inference" {
			if len(round.ToolCalls) == 0 || round.PhysicalToolOperations != nil {
				physicalCoverage++
			}
			if len(round.ToolCalls) == 0 || round.ToolCriticalPathMS != nil {
				criticalCoverage++
			}
			if len(round.ToolCalls) == 0 || round.ToolTotalLatencyMS != nil {
				totalLatencyCoverage++
			}
			if len(round.ToolCalls) == 0 || round.ToolQueueMS != nil {
				queueCoverage++
			}
		}
		run.Rounds = append(run.Rounds, roundView)
		for _, call := range round.ToolCalls {
			tool := toolByName[call.Name]
			if tool == nil {
				zeroCalls, zeroErrors := 0, 0
				tool = &ToolData{ExperimentID: source.ID, AgentID: entry.AgentID, Name: call.Name, Calls: &zeroCalls, Errors: &zeroErrors}
				toolByName[call.Name] = tool
			}
			*tool.Calls++
			tool.ErrorTotal++
			tool.TimingTotal++
			if call.Error != nil {
				tool.ErrorKnown++
				if *call.Error {
					*tool.Errors++
				}
			}
			if call.DurationMS != nil {
				tool.TimingKnown++
				if tool.DurationMS == nil {
					tool.DurationMS = pointerInt64(0)
				}
				*tool.DurationMS += *call.DurationMS
			}
		}
	}
	run.Metrics.CostIdentityUnknownAttempts = pointerInt(costIdentityUnknown)
	if costIdentityUnknown > 0 {
		// Tokens from a response whose served model/tier is not pinned cannot be
		// priced as an exact cost under the requested-model catalog.
		run.Metrics.CatalogCost = nil
		if run.Metrics.CatalogCostPartial == nil && run.Metrics.KnownCatalogCostLowerBound != nil {
			run.Metrics.CatalogCostPartial = cloneFloat(run.Metrics.KnownCatalogCostLowerBound)
		}
	}
	if len(run.Rounds) > 0 {
		for index := range run.Rounds {
			var gap time.Duration
			if index+1 < len(run.Rounds) {
				gap = run.Rounds[index+1].StartedAt.Sub(run.Rounds[index].FinishedAt)
			} else {
				gap = record.Execution.FinishedAt.Sub(run.Rounds[index].FinishedAt)
			}
			gapMS := gap.Seconds() * 1000
			run.Rounds[index].PostRoundGapMS = &gapMS
		}
	}
	run.Metrics.RequestCacheHits, run.Metrics.RequestCacheObserved = pointerInt(requestHits), pointerInt(requestObserved)
	if requestObserved > 0 {
		rate := float64(requestHits) / float64(requestObserved)
		run.Metrics.RequestCacheHit = &rate
	}
	run.Metrics.ProviderCostObserved, run.Metrics.ProviderCostTotal = metrics.ProviderCostObservations, metrics.TransportAttempts
	if run.Metrics.ProviderCostObserved != run.Metrics.ProviderCostTotal && run.Metrics.ProviderReportedCost != nil {
		run.Metrics.ProviderCostPartial = cloneFloat(run.Metrics.ProviderReportedCost)
		run.Metrics.ProviderReportedCost = nil
	}
	run.Metrics.PhysicalToolObserved, run.Metrics.PhysicalToolTotal = physicalCoverage, metrics.ProviderRequests
	run.Metrics.ToolCriticalObserved, run.Metrics.ToolCriticalTotal = criticalCoverage, metrics.ProviderRequests
	run.Metrics.ToolTotalObserved, run.Metrics.ToolTotalTotal = totalLatencyCoverage, metrics.ProviderRequests
	run.Metrics.ToolQueueObserved, run.Metrics.ToolQueueTotal = queueCoverage, metrics.ProviderRequests
	run.Metrics.ToolTimingObserved = min(criticalCoverage, totalLatencyCoverage, queueCoverage)
	run.Metrics.ToolTimingTotal = metrics.ProviderRequests
	if physicalCoverage == metrics.ProviderRequests {
		run.Metrics.PhysicalToolOperations = pointerInt(metrics.PhysicalToolOperations)
	}
	run.Metrics.ToolErrorObserved, run.Metrics.ToolErrorTotal = metrics.ToolErrorObservations, metrics.ToolInvocations
	if metrics.ToolErrorObservations == metrics.ToolInvocations {
		run.Metrics.ToolErrors = pointerInt(metrics.ToolErrors)
	}
	run.Metrics.TokenUsageObserved, run.Metrics.TokenUsageTotal = metrics.UsageReceiptObservations, metrics.UsageReceiptTotal
	run.Metrics.ReasoningTokenObserved, run.Metrics.ReasoningTokenTotal = metrics.ReasoningTokenObservations, metrics.UsageReceiptTotal
	if criticalCoverage == metrics.ProviderRequests {
		run.Metrics.ToolCriticalPathMS = pointerInt64(metrics.ToolCriticalPathMS)
	}
	if totalLatencyCoverage == metrics.ProviderRequests {
		run.Metrics.ToolTotalLatencyMS = pointerInt64(metrics.ToolTotalLatencyMS)
	}
	if queueCoverage == metrics.ProviderRequests {
		run.Metrics.ToolQueueMS = pointerInt64(metrics.ToolQueueMS)
	}
	for _, name := range sortedMapKeys(toolByName) {
		tool := *toolByName[name]
		if tool.ErrorKnown != tool.ErrorTotal {
			tool.Errors = nil
		}
		if tool.TimingKnown != tool.TimingTotal {
			tool.DurationMS = nil
		}
		run.Tools = append(run.Tools, tool)
	}
	return run, nil
}

func eligibleRequestCacheObservation(round harness.ProviderRoundEvidence) (hit, eligible bool) {
	const minimumInputTokens = int64(1024)
	if !round.UsagePresent || round.InputTokens == nil || round.CachedInputTokens == nil || *round.InputTokens < minimumInputTokens {
		return false, false
	}
	return *round.CachedInputTokens > 0, true
}

func applyAllExecutedTokenPartition(destination *MetricData, metrics harness.UsageMetrics) error {
	if destination == nil || metrics.AllExecutedUsageObservations == 0 {
		return nil
	}
	destination.AllExecutedInputTokens = pointerInt64(metrics.AllExecutedInputTokens)
	destination.AllExecutedCachedTokens = pointerInt64(metrics.AllExecutedCachedInputTokens)
	destination.AllExecutedOutputTokens = pointerInt64(metrics.AllExecutedOutputTokens)
	destination.AllExecutedCacheWriteInputTokens = pointerInt64(metrics.AllExecutedCacheWriteInputTokens)
	nonCachedBaseTokens := metrics.AllExecutedInputTokens - metrics.AllExecutedCachedInputTokens
	if nonCachedBaseTokens < 0 {
		return errors.New("all-executed cached input exceeds input tokens")
	}
	destination.AllExecutedNonCachedBaseTokens = pointerInt64(nonCachedBaseTokens)
	if metrics.AllExecutedCacheWriteObservations != metrics.AllExecutedUsageObservations {
		return nil
	}
	ordinaryUncachedTokens := nonCachedBaseTokens - metrics.AllExecutedCacheWriteInputTokens
	if ordinaryUncachedTokens < 0 || metrics.AllExecutedInputTokens != metrics.AllExecutedCachedInputTokens+metrics.AllExecutedCacheWriteInputTokens+ordinaryUncachedTokens {
		return errors.New("all-executed input token partition is inconsistent")
	}
	destination.AllExecutedUncachedTokens = pointerInt64(ordinaryUncachedTokens)
	return nil
}

type rebasedRunFile struct {
	Relative string
	Path     string
	SHA256   string
}

func rebaseAndHashRunFile(artifactDir, originalArtifactDir, originalPath string) (rebasedRunFile, error) {
	relative, err := filepath.Rel(originalArtifactDir, originalPath)
	if err != nil || validateRelativePath(relative) != nil {
		return rebasedRunFile{}, errors.New("path cannot be safely rebased")
	}
	relative = filepath.Clean(relative)
	current, err := pathInside(artifactDir, relative)
	if err != nil {
		return rebasedRunFile{}, err
	}
	digest, err := hashFile(current)
	if err != nil {
		return rebasedRunFile{}, err
	}
	return rebasedRunFile{Relative: relative, Path: current, SHA256: digest}, nil
}

func validateArchivedProviderSeal(artifactDir, originalArtifactDir string, execution *harness.AgentExecution, rounds []harness.ProviderRoundEvidence) ([]byte, error) {
	if execution == nil || execution.Lifecycle.ProviderAttemptState != "provider_attempt_sealed" {
		return nil, errors.New("normalized provider evidence lacks a sealed controller lifecycle")
	}
	seal := execution.ProviderEvidence
	if seal.StartedAttemptCount == 0 || seal.StartedAttemptCount != seal.PersistedAttemptCount ||
		seal.PersistedAttemptCount != seal.RecordCount || uint64(len(rounds)) != seal.RecordCount ||
		execution.Lifecycle.ProviderAttemptCount != seal.StartedAttemptCount || len(rounds) == 0 ||
		rounds[len(rounds)-1].EvidenceHash != seal.LastEvidenceHash || !hex64Pattern.MatchString(seal.LastEvidenceHash) {
		return nil, errors.New("normalized provider evidence disagrees with its lifecycle or raw seal")
	}
	files := []struct {
		label  string
		path   string
		digest string
	}{
		{label: "raw provider evidence", path: seal.RawEvidencePath, digest: seal.RawEvidenceSHA256},
		{label: "provider attempt journal", path: seal.AttemptJournalPath, digest: seal.AttemptJournalSHA256},
		{label: "provider evidence seal", path: seal.SealPath, digest: seal.SealSHA256},
	}
	seen := map[string]struct{}{}
	contents := map[string][]byte{}
	for _, file := range files {
		if file.path == "" || !hex64Pattern.MatchString(file.digest) {
			return nil, fmt.Errorf("%s lacks a path or SHA-256", file.label)
		}
		rebased, raw, err := rebaseAndReadRunFile(artifactDir, originalArtifactDir, file.path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.label, err)
		}
		if _, duplicate := seen[rebased.Path]; duplicate {
			return nil, errors.New("provider seal artifacts reuse the same archived path")
		}
		seen[rebased.Path] = struct{}{}
		if rebased.SHA256 != file.digest {
			return nil, fmt.Errorf("%s digest differs from the archived artifact", file.label)
		}
		contents[file.label] = raw
	}
	validated, err := validateEvidenceSealSnapshot(
		contents["raw provider evidence"],
		contents["provider attempt journal"],
		contents["provider evidence seal"],
		execution.Lifecycle.RunIdentity,
	)
	if err != nil {
		return nil, fmt.Errorf("validate archived provider evidence chain: %w", err)
	}
	if validated.StartedAttemptCount != seal.StartedAttemptCount || validated.PersistedAttemptCount != seal.PersistedAttemptCount ||
		validated.RecordCount != seal.RecordCount || validated.LastEvidenceHash != seal.LastEvidenceHash {
		return nil, errors.New("archived raw provider seal differs from the normalized lifecycle seal")
	}
	return contents["raw provider evidence"], nil
}

// rebaseAndReadRunFile freezes one immutable in-memory view for digest and
// semantic validation. In particular, the raw ledger bytes returned here are
// the exact bytes later supplied to the strict raw-v6 projection validator.
func rebaseAndReadRunFile(artifactDir, originalArtifactDir, originalPath string) (rebasedRunFile, []byte, error) {
	relative, err := filepath.Rel(originalArtifactDir, originalPath)
	if err != nil || validateRelativePath(relative) != nil {
		return rebasedRunFile{}, nil, errors.New("path cannot be safely rebased")
	}
	relative = filepath.Clean(relative)
	current, err := pathInside(artifactDir, relative)
	if err != nil {
		return rebasedRunFile{}, nil, err
	}
	raw, err := os.ReadFile(current)
	if err != nil {
		return rebasedRunFile{}, nil, err
	}
	digest := sha256.Sum256(raw)
	return rebasedRunFile{
		Relative: relative,
		Path:     current,
		SHA256:   hex.EncodeToString(digest[:]),
	}, raw, nil
}

// validateEvidenceSealSnapshot is the byte-snapshot counterpart of the live
// proxy validator. It preserves the same chain, journal, and seal invariants
// while avoiding a second read of any archived file during report loading.
func validateEvidenceSealSnapshot(rawRecords, rawJournal, rawSeal []byte, runIdentity string) (evidenceproxy.EvidenceSeal, error) {
	var seal evidenceproxy.EvidenceSeal
	if err := json.Unmarshal(rawSeal, &seal); err != nil {
		return seal, err
	}
	if seal.SchemaVersion != "agentic-bench/provider-evidence-seal-v1" || seal.RunIdentity != runIdentity || seal.Fatal {
		return seal, errors.New("provider evidence seal is invalid or fatal")
	}
	records, err := decodeEvidenceJSONLinesSnapshot[evidenceproxy.Record](rawRecords)
	if err != nil {
		return seal, err
	}
	previous := ""
	persistedAttempts := uint64(0)
	for index, record := range records {
		if record.RunIdentity != runIdentity || record.EvidenceSequence != uint64(index) || record.PreviousEvidenceHash != previous || record.EvidenceHash == "" {
			return seal, errors.New("provider evidence hash chain identity or sequence mismatch")
		}
		claimed := record.EvidenceHash
		record.EvidenceHash = ""
		canonical, err := json.Marshal(record)
		if err != nil {
			return seal, err
		}
		digest := sha256.Sum256(canonical)
		if claimed != hex.EncodeToString(digest[:]) {
			return seal, errors.New("provider evidence hash chain digest mismatch")
		}
		previous = claimed
		if record.ProviderAttemptStarted {
			persistedAttempts++
		}
	}
	journal, err := decodeEvidenceJSONLinesSnapshot[evidenceproxy.AttemptStartJournalEntry](rawJournal)
	if err != nil {
		return seal, err
	}
	for _, entry := range journal {
		if entry.SchemaVersion != "agentic-bench/provider-attempt-start-v1" || entry.RunIdentity != runIdentity {
			return seal, errors.New("provider attempt journal identity mismatch")
		}
	}
	startedAttempts := uint64(len(journal))
	if seal.RecordCount != uint64(len(records)) || seal.StartedAttemptCount != startedAttempts ||
		seal.PersistedAttemptCount != persistedAttempts || startedAttempts != persistedAttempts || seal.LastEvidenceHash != previous {
		return seal, errors.New("provider evidence seal counts or terminal hash mismatch")
	}
	return seal, nil
}

func decodeEvidenceJSONLinesSnapshot[T any](raw []byte) ([]T, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	lines := bytes.Split(trimmed, []byte{'\n'})
	result := make([]T, 0, len(lines))
	for _, line := range lines {
		var value T
		if err := json.Unmarshal(line, &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func loadDiagnosticExperiment(source DiagnosticExperiment, meta ReportMeta, statistics StatisticsSpec) ExperimentData {
	experiment := ExperimentData{
		ID: source.ID, Label: source.Label, Class: source.Class, Description: source.Description,
		SourceNote: source.SourceNote,
		Gates: []GateData{
			{Name: "classification", Status: GateUnknown, Detail: string(ClassDiagnosticCanary)},
			{Name: "formal_score", Status: GateFail, Detail: "diagnostic_only"},
			{Name: "artifact_integrity", Status: GateUnknown, Detail: "no_formal_harness_bundle"},
		},
	}
	for _, diagnostic := range source.Runs {
		run := RunData{
			AttemptID:    fmt.Sprintf("diagnostic/%s/%s/%d", diagnostic.TaskID, diagnostic.AgentID, diagnostic.Repetition),
			ExperimentID: source.ID, Class: source.Class, PairID: fmt.Sprintf("diagnostic/%s/%d", diagnostic.TaskID, diagnostic.Repetition),
			TaskID: diagnostic.TaskID, AgentID: diagnostic.AgentID, Variant: diagnostic.Variant,
			Provider: diagnostic.Provider, Model: diagnostic.Model, ReasoningEffort: diagnostic.ReasoningEffort,
			Repetition: diagnostic.Repetition, Attempt: 1, Passed: cloneBool(diagnostic.Passed),
			Metrics: optionalMetricData(diagnostic.Metrics),
		}
		for _, tool := range diagnostic.Tools {
			view := ToolData{ExperimentID: source.ID, AgentID: diagnostic.AgentID, Name: tool.Name, Calls: cloneInt(tool.Calls), Errors: cloneInt(tool.Errors), DurationMS: cloneInt64(tool.DurationMS)}
			if tool.Calls != nil {
				view.ErrorTotal, view.TimingTotal = *tool.Calls, *tool.Calls
				if tool.Errors != nil {
					view.ErrorKnown = *tool.Calls
				}
				if tool.DurationMS != nil {
					view.TimingKnown = *tool.Calls
				}
			}
			run.Tools = append(run.Tools, view)
		}
		experiment.Runs = append(experiment.Runs, run)
	}
	finalizeExperiment(&experiment, meta, statistics)
	return experiment
}

func optionalMetricData(metrics OptionalMetrics) MetricData {
	result := MetricData{
		WallTimeSeconds: cloneFloat(metrics.WallTimeSeconds), TrialDurationSeconds: cloneFloat(metrics.TrialDurationSeconds),
		LLMCallsStarted: cloneInt(metrics.LLMCallsStarted),
		ProviderRounds:  cloneInt(metrics.ProviderRounds), ProviderErrors: cloneInt(metrics.ProviderErrors),
		ToolBearingRounds: cloneInt(metrics.ToolBearingRounds), ToolInvocations: cloneInt(metrics.ToolInvocations),
		PhysicalToolOperations: cloneInt(metrics.PhysicalToolOperations), NativeEvents: cloneInt(metrics.NativeEvents),
		ToolErrors: cloneInt(metrics.ToolErrors), ToolCriticalPathMS: cloneInt64(metrics.ToolCriticalPathMS),
		ToolTotalLatencyMS: cloneInt64(metrics.ToolTotalLatencyMS), ToolQueueMS: cloneInt64(metrics.ToolQueueMS),
		InputTokens: cloneInt64(metrics.InputTokens), CachedInputTokens: cloneInt64(metrics.CachedInputTokens),
		CacheWriteInputTokens: cloneInt64(metrics.CacheWriteInputTokens), OutputTokens: cloneInt64(metrics.OutputTokens),
		ReasoningOutputTokens: cloneInt64(metrics.ReasoningOutputTokens),
		ComparableCost:        cloneFloat(metrics.ComparableCost), ComparableCostBasis: metrics.ComparableCostBasis,
		ProviderReportedCost: cloneFloat(metrics.ProviderReportedCost),
	}
	if result.InputTokens != nil && result.CachedInputTokens != nil {
		miss := *result.InputTokens - *result.CachedInputTokens
		result.CacheMissTokens = &miss
		result.AllExecutedInputTokens = cloneInt64(result.InputTokens)
		result.AllExecutedCachedTokens = cloneInt64(result.CachedInputTokens)
		result.AllExecutedNonCachedBaseTokens = pointerInt64(miss)
		result.AllExecutedOutputTokens = cloneInt64(result.OutputTokens)
		result.AllExecutedUsageObserved, result.AllExecutedUsageTotal = 1, 1
		result.AllExecutedCacheWriteTotal = 1
		if result.CacheWriteInputTokens != nil {
			ordinaryUncached := miss - *result.CacheWriteInputTokens
			result.AllExecutedCacheWriteInputTokens = cloneInt64(result.CacheWriteInputTokens)
			result.AllExecutedUncachedTokens = pointerInt64(ordinaryUncached)
			result.AllExecutedCacheWriteObserved = 1
		}
		if *result.InputTokens > 0 {
			rate := float64(*result.CachedInputTokens) / float64(*result.InputTokens)
			result.TokenWeightedCacheHit = &rate
			result.AllExecutedCacheHit = cloneFloat(result.TokenWeightedCacheHit)
		}
	}
	if metrics.RequestCache != nil {
		hits, observed := metrics.RequestCache.Hits, metrics.RequestCache.Observed
		result.RequestCacheHits, result.RequestCacheObserved = &hits, &observed
		rate := float64(hits) / float64(observed)
		result.RequestCacheHit = &rate
	}
	return result
}

func artifactGates(experiment ExperimentData, bundle formalBundle) []GateData {
	manifest := bundle.Manifest.Manifest
	spendComplete := true
	singleAgentCatalogComplete := true
	toolExecutionKnown := true
	toolExecutionComplete := true
	sealedProjectionRuns := 0
	noProviderProjectionRuns := 0
	recoveredProjectionExclusions := 0
	for _, run := range experiment.Runs {
		switch {
		case run.ProviderAttemptState == "provider_attempt_sealed":
			// A sealed run can only reach this gate after loadFormalRun has
			// validated its one-read raw snapshot against the frozen SHA, raw
			// seal/journal, endpoint TLS contract, and normalized projection.
			sealedProjectionRuns++
		case run.ProviderAttemptState == "no_provider_attempt":
			noProviderProjectionRuns++
		case run.ControllerRecovered:
			recoveredProjectionExclusions++
		}
		spendComplete = spendComplete && run.Attempt == 1 && run.Metrics.ComparableCost != nil &&
			run.Metrics.ComparableCostBasis == comparableCostBasisFrozen
		if run.Metrics.ToolInvocations == nil || run.Metrics.ToolTraceMatched == nil || run.Metrics.ToolTraceUnmatched == nil {
			toolExecutionKnown = false
		} else if *run.Metrics.ToolTraceMatched != *run.Metrics.ToolInvocations || *run.Metrics.ToolTraceUnmatched != 0 {
			toolExecutionComplete = false
		}
		if run.Metrics.ProviderRequests == nil || run.ToolCatalogObserved != *run.Metrics.ProviderRequests || run.NestedToolDefinitions != 0 {
			singleAgentCatalogComplete = false
		}
	}
	attemptStatus := GatePass
	if !spendComplete {
		attemptStatus = GateFail
	}
	toolExecutionStatus := GatePass
	if !toolExecutionComplete {
		toolExecutionStatus = GateFail
	} else if !toolExecutionKnown {
		toolExecutionStatus = GateUnknown
	}
	singleAgentStatus := GatePass
	codexSingleAgentConfig := false
	for _, agent := range manifest.Agents {
		if agent.ID == "codex" && slices.Contains(agent.Command.Argv, "agents.enabled=false") {
			codexSingleAgentConfig = true
		}
	}
	if !codexSingleAgentConfig || !singleAgentCatalogComplete {
		singleAgentStatus = GateFail
	}
	formalStatus := GateFail
	if experiment.Class == ClassFormal {
		formalStatus = GatePass
	}
	exclusionStatus, exclusionDetail := GateUnknown, "formal_only"
	if experiment.Class == ClassFormal {
		exclusionStatus, exclusionDetail = formalExclusionSymmetryGate(experiment.PublicScorecard)
	}
	return []GateData{
		{Name: "classification", Status: GatePass, Detail: string(experiment.Class)},
		{Name: "formal_score", Status: formalStatus, Detail: string(experiment.Class)},
		{Name: "artifact_integrity", Status: GatePass, Detail: bundle.Ledger.LedgerSHA256},
		{Name: "projection_integrity", Status: GatePass, Detail: fmt.Sprintf("raw_snapshot_sha256+seal+journal+strict_v6_to_normalized+tls+service_tier=validated;sealed_runs=%d;no_provider_attempts=%d;recovered_exclusions=%d", sealedProjectionRuns, noProviderProjectionRuns, recoveredProjectionExclusions)},
		{Name: "storage_evidence", Status: GatePass, Detail: "host_preflight+host_slot_admission+host_runtime+guest_agent+guest_verifier=validated;host_deltas=shared_host_diagnostic_only"},
		{Name: "scorecard_recomputed", Status: GatePass, Detail: bundle.Scorecard.SchemaVersion},
		{Name: "paired_schedule", Status: GatePass, Detail: fmt.Sprintf("tasks=%d;repetitions=%d;seed=%d", manifest.Selection.ExpectedTaskCount, manifest.Scheduling.Repetitions, manifest.Scheduling.Seed)},
		{Name: "model_contract", Status: GatePass, Detail: fmt.Sprintf("%s/%s/%s;service_tier=%s;gateway_origin=%s;gateway_semantics_sha256=%s", manifest.Agents[0].Model.Provider, manifest.Agents[0].Model.Model, manifest.Agents[0].Model.ReasoningEffort, manifest.Agents[0].Model.ServiceTier, manifest.ProviderEndpoint.ApprovedOrigin, manifest.ProviderEndpoint.SemanticsSHA256)},
		{Name: "single_agent_fairness", Status: singleAgentStatus, Detail: fmt.Sprintf("fixed_agents=2;codex_agents_enabled=false;catalog_nested_tools=0;catalog_complete=%t", singleAgentCatalogComplete)},
		{Name: "network_isolation", Status: GatePass, Detail: manifest.Environment.TaskNetworkMode + "/" + manifest.Environment.VerifierNetworkMode},
		{Name: "oracle", Status: GatePass, Detail: fmt.Sprintf("tasks=%d", len(bundle.State.Oracle))},
		{Name: "complete_spend", Status: attemptStatus, Detail: fmt.Sprintf("symmetric_all_transport_comparable_cost_complete=%t;basis=%s", spendComplete, experimentComparableCostBasis(experiment))},
		{Name: "tool_execution_coverage", Status: toolExecutionStatus, Detail: fmt.Sprintf("logical_calls_execution_matched=%t;coverage_known=%t", toolExecutionComplete, toolExecutionKnown)},
		{Name: "exclusion_symmetry", Status: exclusionStatus, Detail: exclusionDetail},
		{Name: "controller_duration", Status: GateUnknown, Detail: "unobserved; trial_duration_is_not_controller_end_to_end"},
	}
}

func experimentComparableCostBasis(experiment ExperimentData) string {
	if len(experiment.Runs) == 0 {
		return "unknown"
	}
	basis := experiment.Runs[0].Metrics.ComparableCostBasis
	for _, run := range experiment.Runs[1:] {
		if run.Metrics.ComparableCostBasis != basis {
			return "mixed_invalid"
		}
	}
	if basis == "" {
		return "unknown"
	}
	return basis
}

func reportCollaborationToolName(name string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(name, "-", "_"))
	switch normalized {
	case "agent", "team", "collaboration", "spawn_agent", "send_message", "wait_agent", "followup_task", "interrupt_agent", "list_agents":
		return true
	default:
		return strings.HasPrefix(normalized, "collaboration_") || strings.HasPrefix(normalized, "agent_")
	}
}

func formalExclusionSymmetryGate(scorecard *harness.DeepSWEPublicScorecard) (GateStatus, string) {
	if scorecard == nil || len(scorecard.Agents) != 2 || len(scorecard.ExclusionAnalysis.Agents) != 2 || scorecard.ExclusionAnalysis.PairedImbalance == nil {
		return GateFail, "public_scorecard_missing"
	}
	expectedRaw := scorecard.TaskCount * scorecard.Repetitions
	maxExcluded := expectedRaw / 100
	maxDifference := expectedRaw / 200
	summaries := make(map[string]harness.DeepSWEAgentExclusionSummary, len(scorecard.ExclusionAnalysis.Agents))
	for _, summary := range scorecard.ExclusionAnalysis.Agents {
		if _, duplicate := summaries[summary.AgentID]; duplicate {
			return GateFail, "duplicate_exclusion_summary"
		}
		summaries[summary.AgentID] = summary
	}
	for _, agent := range scorecard.Agents {
		summary, exists := summaries[agent.AgentID]
		if !exists || summary.Raw != agent.Counts.Raw || summary.Scored != agent.Counts.Scored || summary.Excluded != agent.Counts.Excluded {
			return GateFail, "exclusion_summary_disagrees_with_recomputed_agent_score"
		}
	}
	imbalance := scorecard.ExclusionAnalysis.PairedImbalance
	challenger, challengerOK := summaries[imbalance.ChallengerAgentID]
	baseline, baselineOK := summaries[imbalance.BaselineAgentID]
	detail := fmt.Sprintf("excluded=%s:%d,%s:%d;raw_pairs=%d;common_scored=%d;any_excluded=%d;both_excluded=%d;discordant=%d;absolute_difference=%d;max_per_agent=%d;max_difference=%d",
		imbalance.ChallengerAgentID, imbalance.ChallengerExcluded, imbalance.BaselineAgentID, imbalance.BaselineExcluded,
		imbalance.RawPairs, imbalance.CommonScored, imbalance.AnyExcluded, imbalance.BothExcluded,
		imbalance.DiscordantExclusionSlots, imbalance.AbsoluteCountDifference, maxExcluded, maxDifference)
	coverageValid := expectedRaw > 0 && challengerOK && baselineOK && imbalance.RawPairs == expectedRaw &&
		imbalance.CommonScored+imbalance.AnyExcluded == imbalance.RawPairs &&
		imbalance.BothExcluded+imbalance.ChallengerOnlyExcluded+imbalance.BaselineOnlyExcluded == imbalance.AnyExcluded &&
		imbalance.ChallengerExcluded == imbalance.BothExcluded+imbalance.ChallengerOnlyExcluded &&
		imbalance.BaselineExcluded == imbalance.BothExcluded+imbalance.BaselineOnlyExcluded &&
		imbalance.ChallengerExcluded == challenger.Excluded && imbalance.BaselineExcluded == baseline.Excluded
	if !coverageValid || challenger.Raw != expectedRaw || baseline.Raw != expectedRaw ||
		challenger.Excluded > maxExcluded || baseline.Excluded > maxExcluded || imbalance.AbsoluteCountDifference > maxDifference {
		return GateFail, detail
	}
	return GatePass, detail
}

func rejectNonRegularArtifactTree(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("artifact root must be a real directory")
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact tree contains a symlink or special file: %s", path)
		}
		return nil
	})
}

func pathInside(root, relative string) (string, error) {
	if err := validateRelativePath(filepath.FromSlash(relative)); err != nil {
		return "", err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if path == root || !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return "", errors.New("artifact path escapes its root")
	}
	return path, nil
}

func pathEndsWith(path, relative string) bool {
	clean := filepath.Clean(path)
	suffix := filepath.Clean(relative)
	return clean == suffix || strings.HasSuffix(clean, string(filepath.Separator)+suffix)
}

func findManifestAgent(agents []harness.AgentSpec, id string) harness.AgentSpec {
	for _, agent := range agents {
		if agent.ID == id {
			return agent
		}
	}
	return harness.AgentSpec{}
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func classRank(class ExperimentClass) int {
	switch class {
	case ClassFormal:
		return 3
	case ClassPilot:
		return 2
	case ClassDiagnosticCanary:
		return 1
	default:
		return 0
	}
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func pointerInt(value int) *int { return &value }

func pointerInt64(value int64) *int64 { return &value }

func pointerFloat(value float64) *float64 { return &value }

func positiveInt64(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return pointerInt64(value)
}

func nonNegativeIfInput(value, input int64) *int64 {
	if input <= 0 {
		return nil
	}
	return pointerInt64(value)
}

func finiteDuration(start, finish time.Time) (*float64, error) {
	if start.IsZero() || finish.IsZero() || finish.Before(start) {
		return nil, errors.New("invalid timing interval")
	}
	seconds := finish.Sub(start).Seconds()
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return nil, errors.New("non-finite timing interval")
	}
	return &seconds, nil
}
