package report

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestRegisteredDeepSWEContractAcceptsExactIdentity(t *testing.T) {
	manifest, plan, state, meta := registeredDeepSWEContractFixture()
	if err := validatePublishedBenchmarkContract(ClassFormal, meta, manifest, plan, state); err != nil {
		t.Fatalf("validatePublishedBenchmarkContract: %v", err)
	}
}

func TestRegisteredDeepSWEPilotDevelopmentContractAcceptsExactIdentity(t *testing.T) {
	manifest, plan, state, meta := registeredDeepSWEPilotContractFixture()
	if err := validatePublishedBenchmarkContract(ClassPilot, meta, manifest, plan, state); err != nil {
		t.Fatalf("validatePublishedBenchmarkContract(pilot): %v", err)
	}
}

func TestRegisteredDeepSWEContractRejectsClassAndIDCrossContamination(t *testing.T) {
	fullManifest, fullPlan, fullState, fullMeta := registeredDeepSWEContractFixture()
	pilotManifest, pilotPlan, pilotState, pilotMeta := registeredDeepSWEPilotContractFixture()
	for _, test := range []struct {
		name     string
		class    ExperimentClass
		meta     ReportMeta
		manifest harness.Manifest
		plan     harness.RunPlan
		state    harness.ExperimentState
	}{
		{name: "full artifacts under development ID", class: ClassFormal, meta: pilotMeta, manifest: fullManifest, plan: fullPlan, state: fullState},
		{name: "pilot artifacts under formal ID", class: ClassPilot, meta: fullMeta, manifest: pilotManifest, plan: pilotPlan, state: pilotState},
		{name: "pilot artifacts labeled formal", class: ClassFormal, meta: pilotMeta, manifest: pilotManifest, plan: pilotPlan, state: pilotState},
		{name: "full artifacts labeled pilot", class: ClassPilot, meta: fullMeta, manifest: fullManifest, plan: fullPlan, state: fullState},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validatePublishedBenchmarkContract(test.class, test.meta, test.manifest, test.plan, test.state); err == nil {
				t.Fatal("cross-contaminated contract identity was accepted")
			}
		})
	}
}

func TestRegisteredDeepSWEPilotContractRejectsTaskOrInventoryMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*harness.Manifest)
	}{
		{name: "partial inventory", mutate: func(manifest *harness.Manifest) {
			manifest.Dataset.ManifestSHA256 = registeredDeepSWEV11FullInventorySHA256
		}},
		{name: "task order", mutate: func(manifest *harness.Manifest) {
			manifest.Selection.TaskIDs[0], manifest.Selection.TaskIDs[1] = manifest.Selection.TaskIDs[1], manifest.Selection.TaskIDs[0]
		}},
		{name: "task identity", mutate: func(manifest *harness.Manifest) { manifest.Selection.TaskIDs[0] = "different-task" }},
		{name: "repetitions", mutate: func(manifest *harness.Manifest) { manifest.Scheduling.Repetitions = 4 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest, plan, state, meta := registeredDeepSWEPilotContractFixture()
			test.mutate(&manifest)
			if err := validatePublishedBenchmarkContract(ClassPilot, meta, manifest, plan, state); err == nil {
				t.Fatal("mutated pilot contract was accepted")
			}
		})
	}
}

func TestRegisteredDeepSWEContractIDIsExact(t *testing.T) {
	manifest, plan, state, meta := registeredDeepSWEContractFixture()
	for _, contractID := range []string{
		"", "DEEPSWE-V1.1-FULL113", "deepswe-v1.1-full113\u200b", "d\u0435epswe-v1.1-full113", "deepswe-v1.1-full112",
	} {
		t.Run(contractID, func(t *testing.T) {
			meta := meta
			meta.BenchmarkContractID = contractID
			if err := validatePublishedBenchmarkContract(ClassFormal, meta, manifest, plan, state); err == nil {
				t.Fatalf("contract ID %q was accepted", contractID)
			}
		})
	}
}

func TestRegisteredDeepSWEContractIgnoresDisplayLabelsForIdentity(t *testing.T) {
	manifest, plan, state, meta := registeredDeepSWEContractFixture()
	meta.Benchmark = "D\u0435\u0435pSWE\u200b display only"
	meta.BenchmarkVersion = "not-an-identity"
	if err := validatePublishedBenchmarkContract(ClassFormal, meta, manifest, plan, state); err != nil {
		t.Fatalf("display text changed non-display contract identity: %v", err)
	}
}

func TestRegisteredDeepSWEContractRejectsIdentityBitFlips(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*harness.Manifest, *harness.ExperimentState)
	}{
		{name: "inventory", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Dataset.ManifestSHA256 = strings.Repeat("f", 64)
		}},
		{name: "dataset tree-v2", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Dataset.TreeSHA256 = strings.Repeat("f", 64)
		}},
		{name: "evaluator tree-v2", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Evaluator.TreeSHA256 = strings.Repeat("f", 64)
		}},
		{name: "codex binary", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Agents[0].BinarySHA256 = strings.Repeat("f", 64)
		}},
		{name: "codex argv", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Agents[0].Command.Argv = append(manifest.Agents[0].Command.Argv, "--config", "disable_response_storage=true")
		}},
		{name: "codex service tier argv", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Agents[0].Command.Argv[14] = `service_tier="auto"`
		}},
		{name: "luban service tier argv", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Agents[1].Command.Argv[13] = "auto"
		}},
		{name: "model service tier", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Agents[1].Model.ServiceTier = "auto"
		}},
		{name: "codex service tier encoding", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Agents[0].Model.ServiceTierRequestEncoding = harness.ServiceTierEncodingExplicitDefault
		}},
		{name: "luban service tier encoding", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Agents[1].Model.ServiceTierRequestEncoding = harness.ServiceTierEncodingClientCanonical
		}},
		{name: "pricing effective date", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Pricing.EffectiveAt = manifest.Pricing.EffectiveAt.Add(24 * time.Hour)
		}},
		{name: "pricing observation date", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.Pricing.ObservedAt = manifest.Pricing.ObservedAt.Add(-24 * time.Hour)
		}},
		{name: "gateway semantics", mutate: func(manifest *harness.Manifest, _ *harness.ExperimentState) {
			manifest.ProviderEndpoint.Semantics.WebSocketAllowed = false
		}},
		{name: "gateway TLS receipt", mutate: func(_ *harness.Manifest, state *harness.ExperimentState) {
			state.Backend.ProviderEndpoint.TLSVerified = false
		}},
		{name: "proxy digest", mutate: func(_ *harness.Manifest, state *harness.ExperimentState) {
			state.Backend.EgressProxyImage = "ubuntu/squid@sha256:" + strings.Repeat("f", 64)
		}},
		{name: "inventory lock file", mutate: func(_ *harness.Manifest, state *harness.ExperimentState) {
			state.Backend.InventoryLock.FileSHA256 = strings.Repeat("f", 64)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest, plan, state, meta := registeredDeepSWEContractFixture()
			test.mutate(&manifest, &state)
			if err := validatePublishedBenchmarkContract(ClassFormal, meta, manifest, plan, state); err == nil {
				t.Fatal("identity mutation was accepted")
			}
		})
	}
}

func TestRegisteredDeepSWEContractRejectsUnbalancedCrossover(t *testing.T) {
	manifest, plan, state, meta := registeredDeepSWEContractFixture()
	plan.Entries[0].AgentID, plan.Entries[1].AgentID = plan.Entries[1].AgentID, plan.Entries[0].AgentID
	if err := validatePublishedBenchmarkContract(ClassFormal, meta, manifest, plan, state); err == nil || !strings.Contains(err.Error(), "crossover") {
		t.Fatalf("error = %v, want crossover rejection", err)
	}
}

func registeredDeepSWEContractFixture() (harness.Manifest, harness.RunPlan, harness.ExperimentState, ReportMeta) {
	codexBinary := "/opt/codex/x86_64-unknown-linux-musl/bin/codex"
	lubanBinary := "/opt/luban/luban"
	model := harness.ModelRequestSpec{Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: harness.FormalServiceTier}
	codexModel, lubanModel := model, model
	codexModel.ServiceTierRequestEncoding = harness.ServiceTierEncodingClientCanonical
	lubanModel.ServiceTierRequestEncoding = harness.ServiceTierEncodingExplicitDefault
	codexModel.TransportRequirement = harness.TransportRequirementHTTPInference
	lubanModel.TransportRequirement = harness.TransportRequirementHTTPInference
	evidence := harness.RequestEvidenceSpec{RelativePath: "metrics/provider-requests.jsonl", Required: true}
	manifest := harness.Manifest{
		ProviderEndpoint: harness.FormalProviderEndpoint(),
		Dataset: harness.SourcePin{
			Name: "deep-swe-v1.1", Repository: "https://github.com/datacurve-ai/deep-swe",
			Commit: "8cae5984d5dd0ee37445beff0e928dc10c331116", Root: "tasks",
			TreeSHA256:     "ce6b3f3c7eff0b512d11060976c7f548267755afc26e377f50851b4523db98ea",
			ManifestSHA256: registeredDeepSWEV11FullInventorySHA256,
		},
		Evaluator: harness.EvaluatorSpec{
			SourcePin: harness.SourcePin{
				Name: "datacurve-pier", Repository: "https://github.com/datacurve-ai/pier",
				Commit: "e69a20e4e0ac073ec71fde0274bab3d9f40bac87", Root: "src",
				TreeSHA256:     "600c65f30f803d1a9219432f01dd8637e1bf1c636558b3606b0c957f156af197",
				ManifestSHA256: "8afbae2c8c78ed6eaa3a49656bb4639d77c07cf6cd2b72266e4ad2283d8dc943",
			},
			Protocol: "pier-harbor-separate-verifier", MinimumVersion: "0.3.0",
		},
		Agents: []harness.AgentSpec{
			{ID: "codex", Binary: codexBinary, BinarySHA256: registeredCodex0145BinarySHA256, Model: codexModel, ExecutionCanary: harness.ExecutionCanarySpec{Generation: harness.FormalExecutionCanaryGeneration, ReceiptSHA256: strings.Repeat("8", 64)}, RequestEvidence: evidence, Command: harness.CommandSpec{Argv: []string{codexBinary, "--ask-for-approval", "never", "--sandbox", "workspace-write", "exec", "--json", "--ephemeral", "--ignore-user-config", "--model", "gpt-5.6-sol", "--config", "model_reasoning_effort=xhigh", "--config", `service_tier="default"`, "--config", `web_search="disabled"`, "--config", "agents.enabled=false", "--config", `model_provider="agentic_http"`, "--config", `model_providers.agentic_http={name="OpenAI",base_url="{provider_base_url}",wire_api="responses",requires_openai_auth=true,supports_websockets=false}`, "{instruction_path}"}, RequiredEnv: []string{"AGENTIC_SUB_API_KEY", "PATH"}}},
			{ID: "luban", Binary: lubanBinary, BinarySHA256: strings.Repeat("a", 64), SourceSnapshot: &harness.AgentSourceSpec{}, Model: lubanModel, ExecutionCanary: harness.ExecutionCanarySpec{Generation: harness.FormalExecutionCanaryGeneration, ReceiptSHA256: strings.Repeat("9", 64)}, RequestEvidence: evidence, Command: harness.CommandSpec{Argv: []string{lubanBinary, "--print", "--output-format", "stream-json", "--provider", "openai", "--api", "responses", "--model", "gpt-5.6-sol", "--reasoning-effort", "xhigh", "--service-tier", "default", "--pinned-model", "--no-model-fallback", "--allow-all", "--force-sandbox-tools", "{instruction_path}"}, RequiredEnv: []string{"AGENTIC_SUB_API_KEY", "PATH"}}},
		},
		Selection:  harness.SelectionSpec{Mode: "full", ExpectedTaskCount: 113},
		Scheduling: harness.SchedulingSpec{PairAgents: true, Seed: 20260726, Repetitions: 4, MaxParallelPairs: 1},
		Scoring:    harness.ScoringSpec{Profile: harness.ScoringProfileDeepSWEV11PublicCI, BaselineAgentID: "codex", ChallengerAgentID: "luban"},
		Environment: harness.EnvironmentSpec{
			HostEnvAllowlist: []string{"AGENTIC_SUB_API_KEY", "LANG", "LC_ALL", "PATH", "SSL_CERT_DIR", "SSL_CERT_FILE", "TMPDIR"},
			AgentEgressHosts: []string{"host.docker.internal"}, TaskNetworkMode: "no-network", VerifierNetworkMode: "no-network",
		},
		Timeouts:  harness.TimeoutSpec{SetupSeconds: 1800, AgentSeconds: 5400, VerifierSeconds: 1800, TeardownSeconds: 300},
		Resources: harness.ResourceSpec{CPUs: 2, MemoryMB: 8192, StorageMB: 20480, HostStorageGuard: harness.FormalHostStorageGuard(), GuestStorageGuard: harness.FormalGuestStorageGuard()},
		Pricing: harness.PricingCatalog{
			Currency: "USD", UnitTokens: 1_000_000,
			EffectiveAt: time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), ObservedAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
			SourceURL: "https://developers.openai.com/api/docs/models/gpt-5.6-sol",
			Rates: []harness.PricingRate{{
				Provider: "openai", Model: "gpt-5.6-sol", Input: 5, CachedInput: 0.5, Output: 30, CacheWriteInputMultiplier: 1.25,
				RequestTiers: []harness.PricingTier{{Name: "long-context", ThresholdInputTokens: 272000, InputMultiplier: 2, CachedInputMultiplier: 2, OutputMultiplier: 1.5}},
			}},
		},
	}
	plan := harness.RunPlan{Entries: make([]harness.PlanEntry, 0, 904)}
	ordinal := 0
	for repetition := 0; repetition < 4; repetition++ {
		for task := 0; task < 113; task++ {
			first, second := "codex", "luban"
			if (repetition*113+task)%2 == 1 {
				first, second = second, first
			}
			pairID := fmtPairID(repetition, task)
			for _, agentID := range []string{first, second} {
				plan.Entries = append(plan.Entries, harness.PlanEntry{Ordinal: ordinal, PairID: pairID, TaskID: pairID, AgentID: agentID, Repetition: repetition})
				ordinal++
			}
		}
	}
	state := harness.ExperimentState{Backend: harness.BackendSnapshot{
		InventoryLock: harness.InventoryLockSnapshot{
			RelativePath: harness.InventoryLockArchiveRelativePath, FileSHA256: registeredDeepSWEV11FullLockFileSHA256,
			SchemaVersion: harness.PierInventoryLockSchemaVersion, HashAlgorithm: harness.TaskInventoryHashAlgorithm,
			DatasetCommit: manifest.Dataset.Commit, Coverage: "full", TaskCount: 113, UniverseTaskCount: 113,
			TaskInventorySHA256: registeredDeepSWEV11FullInventorySHA256,
		},
		InventoryCoverage: "full", InventoryTaskCount: 113, UniverseTaskCount: 113,
		EgressProxyImage: registeredEgressProxyImage,
		HostStorageGuard: harness.FormalHostStorageGuard(), GuestStorageGuard: harness.FormalGuestStorageGuard(),
		ProviderEndpoint: harness.ProviderEndpointSnapshot{
			ApprovedOrigin: harness.FormalProviderOrigin, SemanticsSHA256: harness.FormalProviderEndpointSemanticsSHA256,
			TLSServerName: harness.FormalProviderTLSServerName, TLSVerified: true,
			TLSPeerLeafCertSHA256: strings.Repeat("b", 64), TLSPeerSPKISHA256: strings.Repeat("c", 64),
		},
	}}
	meta := ReportMeta{BenchmarkContractID: BenchmarkContractDeepSWEV11Full113, Benchmark: "DeepSWE", BenchmarkVersion: "v1.1", BaselineAgentID: "codex", ContenderAgentID: "luban"}
	return manifest, plan, state, meta
}

func registeredDeepSWEPilotContractFixture() (harness.Manifest, harness.RunPlan, harness.ExperimentState, ReportMeta) {
	manifest, _, state, meta := registeredDeepSWEContractFixture()
	manifest.Dataset.ManifestSHA256 = registeredDeepSWEV11PilotInventorySHA256
	manifest.Selection = harness.SelectionSpec{
		Mode: "tasks", TaskIDs: slices.Clone(registeredDeepSWEV11PilotTasks), ExpectedTaskCount: 113,
	}
	manifest.Scheduling.Repetitions = 1
	state.Backend.InventoryLock.FileSHA256 = registeredDeepSWEV11PilotLockFileSHA256
	state.Backend.InventoryLock.Coverage = "tasks"
	state.Backend.InventoryLock.TaskCount = len(registeredDeepSWEV11PilotTasks)
	state.Backend.InventoryLock.TaskInventorySHA256 = registeredDeepSWEV11PilotInventorySHA256
	state.Backend.InventoryCoverage = "tasks"
	state.Backend.InventoryTaskCount = len(registeredDeepSWEV11PilotTasks)
	plan := harness.RunPlan{Entries: make([]harness.PlanEntry, 0, 10)}
	ordinal := 0
	for taskIndex, taskID := range registeredDeepSWEV11PilotTasks {
		first, second := "codex", "luban"
		if taskIndex%2 == 1 {
			first, second = second, first
		}
		pairID := "r000-" + taskID
		for _, agentID := range []string{first, second} {
			plan.Entries = append(plan.Entries, harness.PlanEntry{
				Ordinal: ordinal, PairID: pairID, TaskID: taskID, AgentID: agentID, Repetition: 0,
			})
			ordinal++
		}
	}
	meta.BenchmarkContractID = BenchmarkContractDeepSWEV11Pilot5
	return manifest, plan, state, meta
}

func fmtPairID(repetition, task int) string {
	return fmt.Sprintf("r%03d-task-%03d", repetition, task)
}
