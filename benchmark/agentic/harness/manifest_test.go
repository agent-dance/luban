package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateManifestRequiresReproducibilityGates(t *testing.T) {
	manifest := fixtureManifest(t)
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("valid fixture rejected: %v", err)
	}

	manifest.Oracle.SeparateEnvironment = false
	manifest.Artifacts.CaptureUntracked = false
	manifest.Agents[0].RequestEvidence.Required = false
	err := ValidateManifest(manifest)
	if err == nil {
		t.Fatal("manifest missing reproducibility gates was accepted")
	}
	for _, expected := range []string{"separate environment", "untracked", "request_evidence.required"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("validation error does not mention %q: %v", expected, err)
		}
	}
}

func TestValidateManifestPinsPublicScoringProfileAndComparisonDirection(t *testing.T) {
	manifest := fixtureManifest(t)
	manifest.Scoring.Profile = "generic"
	manifest.Scoring.BaselineAgentID = "missing"
	manifest.Scoring.ChallengerAgentID = "missing"
	manifest.Scheduling.Repetitions = 2
	err := ValidateManifest(manifest)
	if err == nil {
		t.Fatal("unfrozen scoring contract was accepted")
	}
	for _, expected := range []string{"deepswe-v1.1-public-ci", "baseline_agent_id", "challenger_agent_id", "one pilot run or four public runs", "baseline and challenger must differ"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("validation error does not mention %q: %v", expected, err)
		}
	}
}

func TestValidateManifestPinsProviderEndpointServiceTierAndPricingObservation(t *testing.T) {
	for name, mutate := range map[string]func(*Manifest){
		"endpoint origin": func(value *Manifest) {
			value.ProviderEndpoint.ApprovedOrigin = "https://api.openai.com"
		},
		"endpoint authority": func(value *Manifest) {
			value.ProviderEndpoint.Semantics.ProviderIdentityAttested = true
		},
		"endpoint digest": func(value *Manifest) {
			value.ProviderEndpoint.SemanticsSHA256 = strings.Repeat("0", 64)
		},
		"omitted service tier": func(value *Manifest) {
			value.Agents[0].Model.ServiceTier = ""
		},
		"automatic service tier": func(value *Manifest) {
			value.Agents[0].Model.ServiceTier = "auto"
		},
		"wrong client tier encoding": func(value *Manifest) {
			value.Agents[0].Model.ServiceTierRequestEncoding = ServiceTierEncodingExplicitDefault
		},
		"pricing observation before effective date": func(value *Manifest) {
			value.Pricing.ObservedAt = value.Pricing.EffectiveAt.Add(-time.Second)
		},
	} {
		t.Run(name, func(t *testing.T) {
			manifest := fixtureManifest(t)
			mutate(&manifest)
			if err := ValidateManifest(manifest); err == nil {
				t.Fatal("unfrozen provider or pricing contract was accepted")
			}
		})
	}
}

func TestLoadManifestRejectsUnknownAndTrailingJSON(t *testing.T) {
	manifest := fixtureManifest(t)
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	unknown := append(encoded[:len(encoded)-1], []byte(`,"unknown":true}`)...)
	if err := os.WriteFile(path, unknown, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatal("unknown manifest field was accepted")
	}
	if err := os.WriteFile(path, append(encoded, []byte(` {}`)...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
}

func TestFilterEnvironmentIsAllowlistOnlyAndStable(t *testing.T) {
	host := []string{"SECRET=never-copy", "PATH=/bin", "OPENAI_API_KEY=test", "LANG=C"}
	filtered, err := FilterEnvironment(host, []string{"OPENAI_API_KEY", "PATH"}, []string{"OPENAI_API_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"OPENAI_API_KEY=test", "PATH=/bin"}
	if strings.Join(filtered, "\n") != strings.Join(want, "\n") {
		t.Fatalf("filtered environment = %q, want %q", filtered, want)
	}
	if _, err := FilterEnvironment(host, []string{"PATH"}, []string{"OPENAI_API_KEY"}); err == nil {
		t.Fatal("missing required environment was accepted")
	}
}

func fixtureManifest(t testing.TB) Manifest {
	t.Helper()
	binaryPath := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(binaryPath, []byte("fixture agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	binarySHA, err := HashFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	hashA := strings.Repeat("a", 64)
	hashB := strings.Repeat("b", 64)
	tasks := fixtureTasks(1)
	taskManifestSHA, err := HashTaskInventory(tasks)
	if err != nil {
		t.Fatal(err)
	}
	commitA := strings.Repeat("a", 40)
	commitB := strings.Repeat("b", 40)
	lock := map[string]any{
		"schema_version": PierInventoryLockSchemaVersion, "dataset_commit": commitA,
		"coverage": "full", "universe_task_count": 1,
		"tasks": []map[string]any{{
			"id": tasks[0].ID, "relative_path": tasks[0].ID, "base_commit": tasks[0].BaseCommit,
			"manifest_sha256": tasks[0].ManifestSHA256, "instruction_sha256": tasks[0].InstructionSHA256,
			"image": tasks[0].Image, "image_digest": tasks[0].ImageDigest,
		}},
	}
	lockPath := filepath.Join(t.TempDir(), "inventory-lock.json")
	if err := WriteJSONAtomic(lockPath, lock, 0o644); err != nil {
		t.Fatal(err)
	}
	lockFileSHA, err := HashFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	agents := []AgentSpec{
		{
			ID: "codex", Binary: binaryPath, BinarySHA256: binarySHA,
			Command: CommandSpec{Argv: []string{binaryPath, "{instruction_path}"}, RequiredEnv: []string{"OPENAI_API_KEY"}},
			Model: ModelRequestSpec{Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: FormalServiceTier,
				ServiceTierRequestEncoding: ServiceTierEncodingClientCanonical, TransportRequirement: TransportRequirementHTTPInference, ToolCatalog: fixtureToolCatalog("codex")},
			ExecutionCanary: ExecutionCanarySpec{Generation: FormalExecutionCanaryGeneration, ReceiptSHA256: strings.Repeat("8", 64)},
			RequestEvidence: RequestEvidenceSpec{RelativePath: "metrics/provider-requests.jsonl", Required: true},
		},
		{
			ID: "luban", Binary: binaryPath, BinarySHA256: binarySHA,
			Command: CommandSpec{Argv: []string{binaryPath, "{instruction_path}"}, RequiredEnv: []string{"OPENAI_API_KEY"}},
			Model: ModelRequestSpec{Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: FormalServiceTier,
				ServiceTierRequestEncoding: ServiceTierEncodingExplicitDefault, TransportRequirement: TransportRequirementHTTPInference, ToolCatalog: fixtureToolCatalog("luban")},
			ExecutionCanary: ExecutionCanarySpec{Generation: FormalExecutionCanaryGeneration, ReceiptSHA256: strings.Repeat("9", 64)},
			RequestEvidence: RequestEvidenceSpec{RelativePath: "metrics/provider-requests.jsonl", Required: true},
		},
	}
	return Manifest{
		SchemaVersion:    SchemaVersion,
		Experiment:       ExperimentSpec{ID: "fixture"},
		Dataset:          SourcePin{Name: "deep-swe-v1.1", Repository: "https://github.com/datacurve-ai/deep-swe", Commit: commitA, Root: "tasks", TreeSHA256: hashA, ManifestSHA256: taskManifestSHA, InventoryLockFileSHA256: lockFileSHA},
		Evaluator:        EvaluatorSpec{SourcePin: SourcePin{Name: "pier", Repository: "https://github.com/datacurve-ai/pier", Commit: commitB, Root: "src", TreeSHA256: hashB, ManifestSHA256: hashA}, Protocol: "pier-harbor-separate-verifier", MinimumVersion: "0.3.0", BinarySHA256: hashB},
		Agents:           agents,
		Selection:        SelectionSpec{Mode: "full", ExpectedTaskCount: 1},
		Scheduling:       SchedulingSpec{PairAgents: true, Seed: 42, Repetitions: 1, MaxParallelPairs: 1},
		Scoring:          ScoringSpec{Profile: ScoringProfileDeepSWEV11PublicCI, BaselineAgentID: "codex", ChallengerAgentID: "luban"},
		ProviderEndpoint: FormalProviderEndpoint(),
		Environment:      EnvironmentSpec{HostEnvAllowlist: []string{"OPENAI_API_KEY", "PATH"}, AgentEgressHosts: []string{"api.openai.com"}, TaskNetworkMode: "no-network", VerifierNetworkMode: "no-network"},
		Timeouts:         TimeoutSpec{SetupSeconds: 60, AgentSeconds: 5400, VerifierSeconds: 1800, TeardownSeconds: 60},
		Resources:        ResourceSpec{CPUs: 2, MemoryMB: 8192, StorageMB: 20480, HostStorageGuard: FormalHostStorageGuard(), GuestStorageGuard: FormalGuestStorageGuard()},
		Pricing: PricingCatalog{Currency: "USD", UnitTokens: 1_000_000, EffectiveAt: time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), ObservedAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC), SourceURL: "https://developers.openai.com/api/docs/models/gpt-5.6-sol", Rates: []PricingRate{{
			Provider: "openai", Model: "gpt-5.6-sol", Input: 5, CachedInput: .5, Output: 30, CacheWriteInputMultiplier: 1.25,
			RequestTiers: []PricingTier{{Name: "long-context", ThresholdInputTokens: 272000, InputMultiplier: 2, CachedInputMultiplier: 2, OutputMultiplier: 1.5}},
		}}},
		Artifacts: ArtifactSpec{Root: "artifacts", LedgerRelativePath: "ledger.json", StateRelativePath: "state.json", CaptureBinaryDiff: true, CaptureUntracked: true},
		Oracle:    OracleSpec{Required: true, SeparateEnvironment: true, SolutionRoot: "solution"},
	}
}

func fixtureToolCatalog(agentID string) ToolCatalogSpec {
	definitions := formalEvidenceTestDefinitions(agentID)
	tools := make([]ToolIdentitySpec, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, ToolIdentitySpec{Type: definition.Type, Name: definition.Name, DefinitionSHA256: definition.DefinitionSHA256})
	}
	return ToolCatalogSpec{SchemaVersion: FormalToolCatalogSchemaVersion, SemanticSHA256: StableToolCatalogSHA256(definitions), Tools: tools}
}

func fixtureLoaded(t testing.TB, manifest Manifest) LoadedManifest {
	t.Helper()
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}
