package pierbackend

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
	"github.com/agent-dance/luban/i18n"
)

var semverPattern = regexp.MustCompile(`(?:^|[^0-9])v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:[^0-9]|$)`)

const (
	formalDatasetCommit         = "8cae5984d5dd0ee37445beff0e928dc10c331116"
	formalDatasetTreeSHA256     = "ce6b3f3c7eff0b512d11060976c7f548267755afc26e377f50851b4523db98ea"
	formalEvaluatorCommit       = "e69a20e4e0ac073ec71fde0274bab3d9f40bac87"
	formalEvaluatorTreeSHA256   = "600c65f30f803d1a9219432f01dd8637e1bf1c636558b3606b0c957f156af197"
	formalTaskCount             = 113
	formalPierVersion           = "0.3.0"
	formalProviderCredentialEnv = "AGENTIC_SUB_API_KEY"
)

type Backend struct {
	config Config

	mu            sync.RWMutex
	manifest      harness.Manifest
	lock          InventoryLock
	lockSnapshot  harness.InventoryLockSnapshot
	lockBound     bool
	tasks         map[string]LockedTask
	bundle        codexBundleBinding
	adapter       adapterBinding
	codexCanary   formalCodexCanonicalCanaryBinding
	lubanCanary   formalExecutionCanaryBinding
	proxy         egressProxyImageSnapshot
	endpoint      harness.ProviderEndpointSnapshot
	datasetTree   harness.TreeInventory
	evaluatorTree harness.TreeInventory
	ready         bool
	development   bool
}

var _ harness.Backend = (*Backend)(nil)

func New(config Config) (*Backend, error) {
	return newBackend(config, true)
}

// NewDevelopment creates the same pinned Pier backend while leaving current-
// generation execution-canary authority out of the non-formal pilot. Dataset,
// evaluator, model, HTTP transport, provider origin, adapter, bundle, network,
// and tool evidence remain checked by DevelopmentPreflight and each run.
func NewDevelopment(config Config) (*Backend, error) {
	return newBackend(config, false)
}

func newBackend(config Config, requireExecutionCanaries bool) (*Backend, error) {
	if config.ProviderCredentialEnv != formalProviderCredentialEnv {
		return nil, errors.New("Pier backend provider credential environment is not the frozen benchmark secret channel")
	}
	if config.ProxyListenAddress == "" {
		config.ProxyListenAddress = "0.0.0.0:0"
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.RegistryGatePath == "" {
		config.RegistryGatePath = sharedRegistryGatePath(config)
	}
	for name, path := range map[string]string{
		"PierBinary": config.PierBinary, "DatasetRepositoryRoot": config.DatasetRepositoryRoot,
		"EvaluatorRepositoryRoot": config.EvaluatorRepositoryRoot, "EvaluatorManifestPath": config.EvaluatorManifestPath,
		"InventoryLockPath": config.InventoryLockPath, "PythonModuleRoot": config.PythonModuleRoot,
		"PrivateWorkRoot": config.PrivateWorkRoot, "RegistryGatePath": config.RegistryGatePath,
	} {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("Pier backend %s must be absolute", name)
		}
	}
	for name, path := range map[string]string{
		"CodexV8CanaryReceiptPath": config.CodexV8CanaryReceiptPath,
		"LubanV8CanaryReceiptPath": config.LubanV8CanaryReceiptPath,
	} {
		if path != "" && !filepath.IsAbs(path) {
			return nil, fmt.Errorf("Pier backend %s must be absolute", name)
		}
	}
	if requireExecutionCanaries {
		if err := validateConfiguredCanaryPinShape(config, true); err != nil {
			return nil, err
		}
	}
	if config.ProxyAdvertiseHost == "" || strings.ContainsAny(config.ProxyAdvertiseHost, "/: ") {
		return nil, errors.New("Pier backend proxy advertise host must be a bare DNS name")
	}
	if err := validateEgressProxyImageReference(config.EgressProxyImage); err != nil {
		return nil, err
	}
	upstream, err := url.Parse(config.ProviderUpstream)
	if err != nil || upstream.Scheme != "https" || upstream.Host == "" || upstream.User != nil || upstream.Path != "" || upstream.RawQuery != "" || upstream.Fragment != "" || config.ProviderUpstream != harness.FormalProviderOrigin {
		return nil, errors.New("Pier backend provider upstream must equal the preregistered canonical HTTPS origin")
	}
	return &Backend{config: config}, nil
}

func (backend *Backend) Preflight(ctx context.Context, manifest harness.Manifest) (harness.BackendSnapshot, error) {
	return backend.preflight(ctx, manifest, false)
}

// DevelopmentPreflight is a non-formal pilot entry. It skips only the external
// v8 execution-canary receipt authority; it never relaxes task, source,
// evaluator, provider-origin, HTTP transport, model, or pricing pins.
func (backend *Backend) DevelopmentPreflight(ctx context.Context, manifest harness.Manifest) (harness.BackendSnapshot, error) {
	return backend.preflight(ctx, manifest, true)
}

func (backend *Backend) preflight(ctx context.Context, manifest harness.Manifest, development bool) (harness.BackendSnapshot, error) {
	if !development {
		configuredCanaries, err := validateConfiguredExecutionCanaries(backend.config, manifest)
		if err != nil {
			return harness.BackendSnapshot{}, err
		}
		if err := validateConfiguredExecutionCanaryHeaders(configuredCanaries); err != nil {
			return harness.BackendSnapshot{}, err
		}
	}
	if err := validateComparisonManifest(manifest, backend.config, !development); err != nil {
		return harness.BackendSnapshot{}, err
	}
	datasetCommit, err := gitCommit(ctx, backend.config.DatasetRepositoryRoot)
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	evaluatorCommit, err := gitCommit(ctx, backend.config.EvaluatorRepositoryRoot)
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	if err := requirePristineGitSourceRoot(ctx, backend.config.DatasetRepositoryRoot, manifest.Dataset.Root); err != nil {
		return harness.BackendSnapshot{}, err
	}
	if err := requirePristineGitSourceRoot(ctx, backend.config.EvaluatorRepositoryRoot, manifest.Evaluator.Root); err != nil {
		return harness.BackendSnapshot{}, err
	}
	datasetTree, err := harness.HashTree(filepath.Join(backend.config.DatasetRepositoryRoot, manifest.Dataset.Root))
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	evaluatorTree, err := harness.HashTree(filepath.Join(backend.config.EvaluatorRepositoryRoot, manifest.Evaluator.Root))
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	evaluatorManifestSHA, err := harness.HashFile(backend.config.EvaluatorManifestPath)
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	backend.mu.RLock()
	lock, lockSnapshot, lockBound := backend.lock, backend.lockSnapshot, backend.lockBound
	backend.mu.RUnlock()
	if !lockBound {
		return harness.BackendSnapshot{}, errors.New("inventory-lock archive was not bound before backend preflight")
	}
	if err := validateInventoryCoverage(lock, manifest.Selection); err != nil {
		return harness.BackendSnapshot{}, err
	}
	if lock.DatasetCommit != datasetCommit {
		return harness.BackendSnapshot{}, errors.New("Pier inventory lock is bound to a different dataset commit")
	}
	tasks := make(map[string]LockedTask, len(lock.Tasks))
	inventory := make([]harness.Task, 0, len(lock.Tasks))
	for _, task := range lock.Tasks {
		if _, duplicate := tasks[task.ID]; duplicate {
			return harness.BackendSnapshot{}, fmt.Errorf("Pier inventory task %s is duplicated", task.ID)
		}
		if err := validateLockedTask(backend.config.DatasetRepositoryRoot, manifest.Dataset.Root, task, manifest); err != nil {
			return harness.BackendSnapshot{}, err
		}
		tasks[task.ID] = task
		inventory = append(inventory, task.HarnessTask())
	}
	inventorySHA, err := harness.HashTaskInventory(inventory)
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	if lockSnapshot.TaskInventorySHA256 != inventorySHA || lockSnapshot.DatasetCommit != lock.DatasetCommit || lockSnapshot.TaskCount != len(lock.Tasks) {
		return harness.BackendSnapshot{}, errors.New("bound inventory-lock snapshot differs from its decoded tasks")
	}
	pierBinarySHA, err := harness.HashFile(backend.config.PierBinary)
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	if pierBinarySHA != manifest.Evaluator.BinarySHA256 {
		return harness.BackendSnapshot{}, errors.New("Pier executable differs from the frozen evaluator binary")
	}
	networkAttestation, err := attestPierNetworkPolicy(ctx, backend.config, manifest, lock.Tasks)
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	versionOutput, err := runOutput(ctx, backend.config.PierBinary, []string{"--version"}, sanitizedProcessEnvironment(nil, backend.config.ProviderCredentialEnv), "")
	if err != nil {
		return harness.BackendSnapshot{}, fmt.Errorf("resolve Pier version: %w", err)
	}
	version, err := parseSemver(string(versionOutput))
	if err != nil || semverLess(version, manifest.Evaluator.MinimumVersion) {
		return harness.BackendSnapshot{}, fmt.Errorf("Pier version %q does not satisfy %s", version, manifest.Evaluator.MinimumVersion)
	}
	if datasetCommit != manifest.Dataset.Commit || datasetTree.SHA256 != manifest.Dataset.TreeSHA256 || inventorySHA != manifest.Dataset.ManifestSHA256 {
		return harness.BackendSnapshot{}, errors.New("DeepSWE source, tree, or resolved inventory differs from the manifest")
	}
	if evaluatorCommit != manifest.Evaluator.Commit || evaluatorTree.SHA256 != manifest.Evaluator.TreeSHA256 || evaluatorManifestSHA != manifest.Evaluator.ManifestSHA256 {
		return harness.BackendSnapshot{}, errors.New("Pier source, tree, or runtime manifest differs from the manifest")
	}
	bundle, err := resolveCodexBundleBinding(manifest, backend.config)
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	adapter, err := resolvePinnedAdapterBinding(backend.config)
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	var canaries map[string]formalExecutionCanaryBinding
	if development {
		canaries = make(map[string]formalExecutionCanaryBinding, len(manifest.Agents))
		for _, agent := range manifest.Agents {
			canaries[agent.ID] = formalExecutionCanaryBinding{
				AgentID: agent.ID, Generation: formalCodexV8CanaryGeneration,
				TransportRequirement: harness.TransportRequirementHTTPInference,
				Path:                 adapter.Path, SHA256: agent.ExecutionCanary.ReceiptSHA256,
			}
		}
	} else {
		canaries, err = resolveFormalExecutionCanaryBindings(backend.config, manifest, adapter, bundle)
		if err != nil {
			return harness.BackendSnapshot{}, err
		}
	}
	endpoint, err := preflightProviderEndpoint(ctx, manifest.ProviderEndpoint, backend.config.ProviderUpstream)
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	proxyImage, err := ensureEgressProxyImage(ctx, backend.config)
	if err != nil {
		return harness.BackendSnapshot{}, err
	}
	if err := os.MkdirAll(backend.config.PrivateWorkRoot, 0o700); err != nil {
		return harness.BackendSnapshot{}, err
	}
	if err := requireSourceTreeUnchanged(ctx, backend.config.DatasetRepositoryRoot, manifest.Dataset.Root, datasetTree); err != nil {
		return harness.BackendSnapshot{}, err
	}
	if err := requireSourceTreeUnchanged(ctx, backend.config.EvaluatorRepositoryRoot, manifest.Evaluator.Root, evaluatorTree); err != nil {
		return harness.BackendSnapshot{}, err
	}
	backend.mu.Lock()
	backend.manifest, backend.lock, backend.tasks, backend.bundle, backend.adapter, backend.codexCanary, backend.lubanCanary, backend.proxy, backend.endpoint = manifest, lock, tasks, bundle, adapter, canaries["codex"], canaries["luban"], proxyImage, endpoint
	backend.datasetTree, backend.evaluatorTree, backend.ready, backend.development = datasetTree, evaluatorTree, true, development
	backend.mu.Unlock()
	canarySnapshots := executionCanarySnapshots(canaries)
	if development {
		canarySnapshots = nil
	}
	return harness.BackendSnapshot{
		Dataset:                harness.SourceSnapshot{Commit: datasetCommit, TreeSHA256: datasetTree.SHA256, RawTreeSHA256: datasetTree.RawSHA256, ManifestSHA256: inventorySHA},
		Evaluator:              harness.SourceSnapshot{Commit: evaluatorCommit, TreeSHA256: evaluatorTree.SHA256, RawTreeSHA256: evaluatorTree.RawSHA256, ManifestSHA256: evaluatorManifestSHA},
		InventoryLock:          lockSnapshot,
		EvaluatorVersion:       version,
		EvaluatorBinarySHA256:  pierBinarySHA,
		InventoryCoverage:      lock.Coverage,
		InventoryTaskCount:     len(lock.Tasks),
		UniverseTaskCount:      lock.UniverseTaskCount,
		AgentNetworkDeny:       networkAttestation.AgentNetworkDeny,
		VerifierNetworkDeny:    networkAttestation.VerifierNetworkDeny,
		NetworkAttestation:     networkAttestation.ParserModuleSHA256,
		EgressProxyImage:       proxyImage.Reference,
		EgressProxyImageID:     proxyImage.ImageID,
		AdapterImportPath:      AdapterImportPath,
		AdapterVersion:         PinnedAdapterVersion,
		AdapterSHA256:          adapter.SHA256,
		ProviderEndpoint:       endpoint,
		AgentExecutionCanaries: canarySnapshots,
	}, nil
}

func (backend *Backend) Inventory(context.Context, harness.SourcePin) ([]harness.Task, error) {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	if !backend.ready {
		return nil, errors.New("Pier backend inventory requested before preflight")
	}
	result := make([]harness.Task, 0, len(backend.lock.Tasks))
	for _, task := range backend.lock.Tasks {
		result = append(result, task.HarnessTask())
	}
	return result, nil
}

func (backend *Backend) PublicTask(_ context.Context, taskID string) (harness.PublicTaskView, error) {
	task, manifest, err := backend.resolvedTask(taskID)
	if err != nil {
		return harness.PublicTaskView{}, err
	}
	directory, err := pathWithin(filepath.Join(backend.config.DatasetRepositoryRoot, manifest.Dataset.Root), task.RelativePath)
	if err != nil {
		return harness.PublicTaskView{}, err
	}
	return harness.PublicTaskView{
		ID: task.ID, BaseCommit: task.BaseCommit, InstructionSHA256: task.InstructionSHA256,
		InstructionPath: filepath.Join(directory, "instruction.md"), WorkspacePath: "/app",
		Image: task.Image, ImageDigest: task.ImageDigest,
	}, nil
}

func (backend *Backend) VerifyOracle(ctx context.Context, request harness.OracleRequest) (harness.VerificationResult, error) {
	task, manifest, err := backend.resolvedTask(request.TaskID)
	if err != nil {
		return harness.VerificationResult{}, err
	}
	privateDir, cleanup, materializedSHA, err := backend.preparePrivateTask(task, manifest)
	if err != nil {
		return harness.VerificationResult{}, err
	}
	defer cleanup()
	jobsRoot := filepath.Join(filepath.Dir(privateDir), "jobs")
	jobName := "oracle-" + task.ID
	args := commonPierArgs(privateDir, jobsRoot, jobName, manifest)
	args = append(args, "--agent", "oracle")
	stdout, stderr, commandErr := backend.runPier(ctx, args, nil, filepath.Dir(privateDir))
	if isRuntimeSourceIntegrityError(commandErr) {
		return harness.VerificationResult{}, commandErr
	}
	trialDir, locateErr := findSingleTrial(filepath.Join(jobsRoot, jobName))
	if locateErr != nil {
		return harness.VerificationResult{}, joinCommandError(commandErr, locateErr, stdout, stderr)
	}
	parsed, parseErr := parseTrialResult(filepath.Join(trialDir, "result.json"))
	if parseErr != nil {
		return harness.VerificationResult{}, parseErr
	}
	if commandErr != nil && len(parsed.Rewards) == 0 {
		return harness.VerificationResult{}, joinCommandError(commandErr, nil, stdout, stderr)
	}
	result, err := exportVerification(trialDir, request.ArtifactDir, parsed, nil)
	if err != nil {
		return harness.VerificationResult{}, err
	}
	receipt, err := backend.makeRunReceipt(args, materializedSHA, "", nil, nil, nil)
	if err != nil {
		return harness.VerificationResult{}, err
	}
	path := filepath.Join(request.ArtifactDir, "pier-receipt.json")
	if err := harness.WriteJSONAtomic(path, receipt, 0o644); err != nil {
		return harness.VerificationResult{}, err
	}
	result.ArtifactPaths = append(result.ArtifactPaths, path)
	return result, nil
}

func validateFormalManifest(manifest harness.Manifest, config Config) error {
	return validateComparisonManifest(manifest, config, true)
}

func validateComparisonManifest(manifest harness.Manifest, config Config, requireExecutionCanaries bool) error {
	if config.ProviderCredentialEnv != formalProviderCredentialEnv {
		return errors.New("formal comparison requires the task-specific provider credential environment")
	}
	if err := validateEgressProxyImageReference(config.EgressProxyImage); err != nil {
		return err
	}
	if config.EgressProxyImage != FrozenEgressProxyImage {
		return errors.New("formal comparison requires the frozen Canonical Squid image")
	}
	if config.ProviderUpstream != manifest.ProviderEndpoint.ApprovedOrigin || manifest.ProviderEndpoint != harness.FormalProviderEndpoint() {
		return errors.New("formal comparison provider endpoint differs from its preregistered gateway semantics")
	}
	if len(manifest.Agents) != 2 {
		return errors.New("formal Pier comparison requires exactly two agents")
	}
	if manifest.Dataset.Commit != formalDatasetCommit || manifest.Dataset.TreeSHA256 != formalDatasetTreeSHA256 ||
		manifest.Evaluator.Commit != formalEvaluatorCommit || manifest.Evaluator.TreeSHA256 != formalEvaluatorTreeSHA256 ||
		manifest.Evaluator.MinimumVersion != formalPierVersion {
		return errors.New("formal comparison is not pinned to DeepSWE v1.1 and Pier 0.3.0 release commits")
	}
	if manifest.Evaluator.Protocol != "pier-harbor-separate-verifier" {
		return errors.New("formal comparison requires Pier's separate-verifier protocol")
	}
	if manifest.Selection.ExpectedTaskCount != formalTaskCount {
		return fmt.Errorf("formal comparison requires the %d-task DeepSWE universe", formalTaskCount)
	}
	if !manifest.Scheduling.PairAgents || manifest.Scheduling.MaxParallelPairs != 1 {
		return errors.New("formal comparison requires adjacent agent pairs and one active pair")
	}
	switch manifest.Selection.Mode {
	case "full":
		if manifest.Scheduling.Repetitions != 4 || len(manifest.Selection.TaskIDs) != 0 {
			return errors.New("full formal comparison requires 113 tasks and four repetitions")
		}
	case "tasks":
		if manifest.Scheduling.Repetitions != 1 || len(manifest.Selection.TaskIDs) == 0 {
			return errors.New("explicit pilot comparison requires one repetition")
		}
	default:
		return errors.New("formal comparison permits only full or explicit-task selection")
	}
	if !slices.Equal(manifest.Environment.AgentEgressHosts, []string{config.ProxyAdvertiseHost}) {
		return errors.New("formal agent egress must contain only the private evidence proxy host")
	}
	if !slices.Contains(manifest.Environment.HostEnvAllowlist, formalProviderCredentialEnv) ||
		slices.Contains(manifest.Environment.HostEnvAllowlist, "OPENAI_API_KEY") ||
		slices.Contains(manifest.Environment.HostEnvAllowlist, "CODEX_LB_API_KEY") {
		return errors.New("formal host environment does not use the isolated benchmark provider credential channel")
	}
	if manifest.Environment.TaskNetworkMode != "no-network" || manifest.Environment.VerifierNetworkMode != "no-network" {
		return errors.New("formal task and verifier environments must deny network access")
	}
	if !manifest.Oracle.Required || !manifest.Oracle.SeparateEnvironment {
		return errors.New("formal comparison requires the oracle in a separate environment")
	}
	if !hasFormalGPT56SolPricing(manifest.Pricing) {
		return errors.New("formal comparison requires exact GPT-5.6 Sol base, cache-write, and long-context pricing")
	}
	seen := map[string]struct{}{}
	seenCanarySHA := map[string]struct{}{}
	expectedTierEncoding := map[string]string{
		"codex": harness.ServiceTierEncodingClientCanonical,
		"luban": harness.ServiceTierEncodingExplicitDefault,
	}
	for _, agent := range manifest.Agents {
		if _, duplicate := seen[agent.ID]; duplicate {
			return fmt.Errorf("formal agent ID %s is duplicated", agent.ID)
		}
		seen[agent.ID] = struct{}{}
		if agent.Model.Provider != "openai" || agent.Model.Model != "gpt-5.6-sol" || agent.Model.ReasoningEffort != "xhigh" || agent.Model.ServiceTier != harness.FormalServiceTier ||
			agent.Model.ServiceTierRequestEncoding != expectedTierEncoding[agent.ID] || agent.Model.TransportRequirement != harness.TransportRequirementHTTPInference {
			return fmt.Errorf("agent %s is not pinned to openai/gpt-5.6-sol/xhigh/default", agent.ID)
		}
		if !slices.Equal(agent.Command.RequiredEnv, []string{formalProviderCredentialEnv, "PATH"}) {
			return fmt.Errorf("agent %s does not use the exact task-specific provider credential environment", agent.ID)
		}
		if requireExecutionCanaries {
			if agent.ExecutionCanary.Generation != harness.FormalExecutionCanaryGeneration || !lowerHexSHA256(agent.ExecutionCanary.ReceiptSHA256) {
				return fmt.Errorf("agent %s does not pin a formal v8 execution canary", agent.ID)
			}
			if _, duplicate := seenCanarySHA[agent.ExecutionCanary.ReceiptSHA256]; duplicate {
				return errors.New("formal agents reuse one execution canary receipt SHA-256")
			}
			seenCanarySHA[agent.ExecutionCanary.ReceiptSHA256] = struct{}{}
		}
		if len(agent.Command.Argv) == 0 || agent.Command.Argv[0] != agent.Binary || !slices.Contains(agent.Command.RequiredEnv, "PATH") {
			return fmt.Errorf("agent %s command is not bound to its binary and runtime PATH", agent.ID)
		}
		switch agent.ID {
		case "codex":
			if agent.SourceSnapshot != nil {
				return errors.New("formal Codex binary must not claim a local source snapshot")
			}
		case "luban":
			if agent.SourceSnapshot == nil {
				return errors.New("formal Luban agent requires an immutable source/build snapshot")
			}
		default:
			return fmt.Errorf("unsupported formal agent ID %s", agent.ID)
		}
		if err := validateFormalSourceCommand(agent); err != nil {
			return err
		}
	}
	if _, ok := seen["codex"]; !ok {
		return errors.New("formal comparison lacks Codex")
	}
	if _, ok := seen["luban"]; !ok {
		return errors.New("formal comparison lacks Luban")
	}
	return nil
}

func hasFormalGPT56SolPricing(catalog harness.PricingCatalog) bool {
	formalEffectiveAt := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	formalObservedAt := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	if catalog.Currency != "USD" || catalog.UnitTokens != 1_000_000 ||
		catalog.EffectiveAt != formalEffectiveAt || catalog.ObservedAt != formalObservedAt ||
		catalog.SourceURL != "https://developers.openai.com/api/docs/models/gpt-5.6-sol" || len(catalog.Rates) != 1 {
		return false
	}
	rate := catalog.Rates[0]
	if rate.Provider != "openai" || rate.Model != "gpt-5.6-sol" || rate.Input != 5 || rate.CachedInput != .5 || rate.Output != 30 || rate.CacheWriteInputMultiplier != 1.25 || len(rate.RequestTiers) != 1 {
		return false
	}
	tier := rate.RequestTiers[0]
	return tier.Name == "long-context" && tier.ThresholdInputTokens == 272000 && tier.InputMultiplier == 2 && tier.CachedInputMultiplier == 2 && tier.OutputMultiplier == 1.5
}

func (backend *Backend) resolvedTask(taskID string) (LockedTask, harness.Manifest, error) {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	if !backend.ready {
		return LockedTask{}, harness.Manifest{}, errors.New("Pier backend used before preflight")
	}
	task, ok := backend.tasks[taskID]
	if !ok {
		return LockedTask{}, harness.Manifest{}, fmt.Errorf("task %s is absent from the pinned inventory", taskID)
	}
	return task, backend.manifest, nil
}

func (backend *Backend) preparePrivateTask(task LockedTask, manifest harness.Manifest) (string, func(), string, error) {
	root, err := os.MkdirTemp(backend.config.PrivateWorkRoot, "pier-trial-*")
	if err != nil {
		return "", nil, "", err
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	source, err := pathWithin(filepath.Join(backend.config.DatasetRepositoryRoot, manifest.Dataset.Root), task.RelativePath)
	if err != nil {
		cleanup()
		return "", nil, "", err
	}
	destination := filepath.Join(root, "task")
	inventory, err := materializeTask(source, destination, task.Image, task.ImageDigest)
	if err != nil {
		cleanup()
		return "", nil, "", err
	}
	return destination, cleanup, inventory.SHA256, nil
}

func commonPierArgs(taskDir, jobsRoot, jobName string, manifest harness.Manifest) []string {
	setupMultiplier := float64(manifest.Timeouts.SetupSeconds) / 360.0
	return []string{
		"run", "--path", taskDir, "--jobs-dir", jobsRoot, "--job-name", jobName,
		"--n-attempts", "1", "--n-concurrent", "1", "--max-retries", "0",
		"--timeout-multiplier", "1", "--agent-setup-timeout-multiplier", strconv.FormatFloat(setupMultiplier, 'f', -1, 64),
		"--cpus", "limit", "--memory", "limit", "--quiet", "--yes",
	}
}

func (backend *Backend) runPier(ctx context.Context, args, environment []string, directory string) ([]byte, []byte, error) {
	if err := backend.requireRuntimeEvaluatorSource(ctx); err != nil {
		return nil, nil, err
	}
	registryLease, err := acquireRegistryGate(ctx, sharedRegistryGatePath(backend.config))
	if err != nil {
		return nil, nil, fmt.Errorf("acquire shared registry coordination: %w", err)
	}
	if err := backend.requireRuntimeEvaluatorSource(ctx); err != nil {
		return nil, nil, errors.Join(err, registryLease.finish(false, false, ""))
	}
	command := exec.CommandContext(ctx, backend.config.PierBinary, args...)
	command.Dir = directory
	command.Env = sanitizedProcessEnvironment(environment, backend.config.ProviderCredentialEnv)
	pythonPath := backend.config.PythonModuleRoot
	for _, item := range command.Env {
		if strings.HasPrefix(item, "PYTHONPATH=") {
			pythonPath += string(os.PathListSeparator) + strings.TrimPrefix(item, "PYTHONPATH=")
		}
	}
	command.Env = replaceEnvironment(command.Env, "PYTHONPATH", pythonPath)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	commandErr := command.Run()
	sourceContext, cancelSourceCheck := context.WithTimeout(context.Background(), 30*time.Second)
	sourceErr := backend.requireRuntimeEvaluatorSource(sourceContext)
	cancelSourceCheck()
	err = errors.Join(commandErr, sourceErr)
	stdoutBytes, stderrBytes := stdout.Bytes(), stderr.Bytes()
	throttled := registryThrottleEvidence(stdoutBytes, stderrBytes, []byte(fmt.Sprint(err)))
	gateErr := registryLease.finish(err == nil, throttled, "")
	return stdoutBytes, stderrBytes, errors.Join(err, gateErr)
}

func (backend *Backend) requireRuntimeEvaluatorSource(ctx context.Context) error {
	backend.mu.RLock()
	manifest, tree, ready := backend.manifest, backend.evaluatorTree, backend.ready
	backend.mu.RUnlock()
	if !ready {
		return runtimeSourceIntegrityError{cause: i18n.NewError(i18n.KeyBenchmarkSourceInspectFailed)}
	}
	err := requireSourceTreeUnchangedWithKey(
		ctx, backend.config.EvaluatorRepositoryRoot, manifest.Evaluator.Root, tree,
		i18n.KeyBenchmarkSourceMutatedDuringExecution,
	)
	if err != nil {
		return runtimeSourceIntegrityError{cause: err}
	}
	return nil
}

func sanitizedProcessEnvironment(additional []string, credentialName string) []string {
	allowed := map[string]bool{"PATH": true, "LANG": true, "LC_ALL": true, "SSL_CERT_DIR": true, "SSL_CERT_FILE": true, "TMPDIR": true, "DOCKER_HOST": true, "HOME": true}
	values := map[string]string{}
	for _, item := range append(os.Environ(), additional...) {
		name, value, ok := strings.Cut(item, "=")
		if ok && allowed[name] && name != credentialName {
			values[name] = value
		}
	}
	// Pier is installed from the pinned evaluator source. Never let Python
	// materialize path-, timestamp-, or interpreter-specific bytecode inside
	// that immutable provenance root.
	values["PYTHONDONTWRITEBYTECODE"] = "1"
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name+"="+values[name])
	}
	return result
}

func replaceEnvironment(environment []string, name, value string) []string {
	filtered := environment[:0]
	for _, item := range environment {
		if !strings.HasPrefix(item, name+"=") {
			filtered = append(filtered, item)
		}
	}
	return append(filtered, name+"="+value)
}

func gitCommit(ctx context.Context, root string) (string, error) {
	output, err := runOutput(ctx, "git", []string{"-C", root, "rev-parse", "HEAD"}, sanitizedProcessEnvironment(nil, ""), "")
	return strings.TrimSpace(string(output)), err
}

func runOutput(ctx context.Context, binary string, args, environment []string, directory string) ([]byte, error) {
	command := exec.CommandContext(ctx, binary, args...)
	command.Env, command.Dir = environment, directory
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", filepath.Base(binary), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func parseSemver(value string) (string, error) {
	match := semverPattern.FindStringSubmatch(value)
	if len(match) != 4 {
		return "", fmt.Errorf("no semantic version in %q", strings.TrimSpace(value))
	}
	return strings.Join(match[1:], "."), nil
}

func semverLess(actual, minimum string) bool {
	parse := func(value string) [3]int {
		match := semverPattern.FindStringSubmatch(value)
		var result [3]int
		if len(match) == 4 {
			for index := range result {
				result[index], _ = strconv.Atoi(match[index+1])
			}
		}
		return result
	}
	a, b := parse(actual), parse(minimum)
	for index := range a {
		if a[index] != b[index] {
			return a[index] < b[index]
		}
	}
	return false
}

func randomHex(bytesCount int) (string, error) {
	buffer := make([]byte, bytesCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func (backend *Backend) makeRunReceipt(args []string, taskSHA, agentSHA string, effective *effectiveArgvReceipt, endpoint *providerEndpointRunBinding, canonicalization *serviceTierCanonicalizationRunBinding) (runReceipt, error) {
	backend.mu.RLock()
	proxyImage, ready := backend.proxy, backend.ready
	backend.mu.RUnlock()
	if !ready || proxyImage.Reference == "" || proxyImage.ImageID == "" {
		return runReceipt{}, errors.New("Pier run receipt requested before proxy image preflight")
	}
	pierSHA, err := harness.HashFile(backend.config.PierBinary)
	if err != nil {
		return runReceipt{}, err
	}
	argvSHA, err := harness.HashCanonical(args)
	if err != nil {
		return runReceipt{}, err
	}
	receipt := runReceipt{
		SchemaVersion: "agentic-bench/pier-run-receipt-v5", PierBinarySHA256: pierSHA,
		PierArgvSHA256: argvSHA, MaterializedTaskSHA256: taskSHA, AgentBinarySHA256: agentSHA,
		ProviderMeter: "content-free-reverse-proxy-v2", VerifierEnvironment: "separate-pristine",
		EgressProxyImage: proxyImage.Reference, EgressProxyImageID: proxyImage.ImageID,
	}
	if agentSHA != "" {
		if effective == nil {
			return runReceipt{}, errors.New("agent Pier receipt lacks effective argv provenance")
		}
		if endpoint == nil {
			return runReceipt{}, errors.New("agent Pier receipt lacks provider endpoint TLS provenance")
		}
		preflightEndpoint, err := backend.providerEndpointSnapshot()
		if err != nil {
			return runReceipt{}, err
		}
		if endpoint.ApprovedOrigin != preflightEndpoint.ApprovedOrigin ||
			endpoint.SemanticsSHA256 != preflightEndpoint.SemanticsSHA256 ||
			endpoint.TLSServerName != preflightEndpoint.TLSServerName || !endpoint.TLSObservationComplete ||
			endpoint.TLSBackedRoundCount < 0 || endpoint.TLSAbsentTransportFailureCount < 0 ||
			endpoint.TLSBackedRoundCount+endpoint.TLSAbsentTransportFailureCount <= 0 ||
			!validProviderTLSPeerObservations(endpoint.PeerObservations, endpoint.TLSBackedRoundCount) {
			return runReceipt{}, errors.New("agent Pier receipt provider endpoint differs from verified preflight")
		}
		bundle, err := backend.codexBundleSnapshot()
		if err != nil {
			return runReceipt{}, err
		}
		receipt.AgentBundleManifestSHA256 = bundle.ManifestSHA256
		receipt.AgentSourceBundleTreeSHA256 = bundle.TreeSHA256
		receipt.AdapterImportPath = AdapterImportPath
		receipt.AdapterVersion = effective.AdapterVersion
		receipt.AdapterSHA256 = effective.AdapterSHA256
		receipt.SourceCommandArgvSHA256 = effective.SourceCommandArgvSHA256
		receipt.EffectiveArgvSHA256 = effective.EffectiveArgvSHA256
		receipt.ExecutionArgvSHA256 = effective.ExecutionArgvSHA256
		receipt.EffectiveArgv = slices.Clone(effective.EffectiveArgv)
		projection := effective.SemanticProjection
		receipt.EffectiveArgvSemantics = &projection
		receipt.ProviderApprovedOrigin = endpoint.ApprovedOrigin
		receipt.ProviderEndpointSemanticsSHA256 = endpoint.SemanticsSHA256
		receipt.ProviderObservationAuthority = providerEndpointObservationAuthority
		receipt.ProviderIdentityAttested = false
		receipt.ProviderTLSServerName = endpoint.TLSServerName
		receipt.ProviderTLSObservationComplete = endpoint.TLSObservationComplete
		receipt.ProviderPreflightTLSPeerLeafCertSHA256 = preflightEndpoint.TLSPeerLeafCertSHA256
		receipt.ProviderPreflightTLSPeerSPKISHA256 = preflightEndpoint.TLSPeerSPKISHA256
		receipt.ProviderTLSBackedRoundCount = endpoint.TLSBackedRoundCount
		receipt.ProviderTLSAbsentTransportFailureCount = endpoint.TLSAbsentTransportFailureCount
		receipt.ProviderTLSPeerObservations = slices.Clone(endpoint.PeerObservations)
		if effective.AgentKind == "codex" {
			canary, err := backend.codexCanonicalCanarySnapshot()
			if err != nil {
				return runReceipt{}, err
			}
			receipt.CodexCanonicalCanaryGeneration = canary.Generation
			relativeCanaryPath, err := filepath.Rel(backend.config.PythonModuleRoot, canary.Path)
			if err != nil || relativeCanaryPath == "." || filepath.IsAbs(relativeCanaryPath) || relativeCanaryPath == ".." || strings.HasPrefix(relativeCanaryPath, ".."+string(os.PathSeparator)) {
				return runReceipt{}, errors.New("Codex canonical canary receipt is outside the frozen module root")
			}
			receipt.CodexCanonicalCanaryReceiptRelativePath = filepath.ToSlash(relativeCanaryPath)
			receipt.CodexCanonicalCanaryReceiptSHA256 = canary.SHA256
			if canonicalization == nil || !lowerHexSHA256(canonicalization.ReceiptSHA256) ||
				canonicalization.Receipt.CanonicalCanaryGeneration != canary.Generation ||
				canonicalization.Receipt.Representation != serviceTierCanonicalizationRepresentation ||
				canonicalization.Receipt.FrozenCanonicalCanaryReceiptSHA256 != canary.SHA256 ||
				canonicalization.Receipt.EffectiveArgvSHA256 != effective.EffectiveArgvSHA256 ||
				!lowerHexSHA256(canonicalization.Receipt.BindingSHA256) ||
				!lowerHexSHA256(canonicalization.Receipt.StaticProofSHA256) ||
				!lowerHexSHA256(canonicalization.Receipt.TransformationEvidenceSHA256) ||
				canonicalization.Receipt.TransformedProviderRoundCount <= 0 {
				return runReceipt{}, errors.New("Codex Pier receipt lacks its two-stage service-tier canonicalization proof")
			}
			receipt.ServiceTierCanonicalizationRepresentation = canonicalization.Receipt.Representation
			receipt.ServiceTierCanonicalizationReceiptRelativePath = filepath.ToSlash(filepath.Join("pier", serviceTierCanonicalizationReceiptName))
			receipt.ServiceTierCanonicalizationReceiptSHA256 = canonicalization.ReceiptSHA256
			receipt.ServiceTierCanonicalizationBindingSHA256 = canonicalization.Receipt.BindingSHA256
			receipt.ServiceTierCanonicalizationStaticProofSHA256 = canonicalization.Receipt.StaticProofSHA256
			receipt.ServiceTierTransformationEvidenceSHA256 = canonicalization.Receipt.TransformationEvidenceSHA256
			receipt.ServiceTierTransformedProviderRoundCount = canonicalization.Receipt.TransformedProviderRoundCount
		} else if canonicalization != nil || receipt.CodexCanonicalCanaryGeneration != "" ||
			receipt.CodexCanonicalCanaryReceiptRelativePath != "" || receipt.CodexCanonicalCanaryReceiptSHA256 != "" {
			return runReceipt{}, errors.New("Luban Pier receipt unexpectedly contains Codex canonicalization authority")
		}
	}
	return receipt, nil
}

func joinCommandError(commandErr, secondary error, stdout, stderr []byte) error {
	parts := []string{}
	if commandErr != nil {
		parts = append(parts, commandErr.Error())
	}
	if secondary != nil {
		parts = append(parts, secondary.Error())
	}
	if len(stdout) > 0 {
		parts = append(parts, "stdout: "+truncate(string(stdout), 4000))
	}
	if len(stderr) > 0 {
		parts = append(parts, "stderr: "+truncate(string(stderr), 4000))
	}
	return errors.New(strings.Join(parts, "; "))
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "[truncated]"
}
