package report

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

const (
	// HashTaskInventory binds all 113 task manifests, instructions, base
	// commits, image names, and immutable image digests. The lock-file digest
	// is a separate identity and must never be substituted for it.
	registeredDeepSWEV11FullInventorySHA256  = "85f7f80eb0c48ea3480f95e145d13bacf5782c9aea1c576f79c65a14626d3a7a"
	registeredDeepSWEV11FullLockFileSHA256   = "e23cb7c40f696e191122647295d24ef6a4c2e7d2df2dca359acfaebc05e28263"
	registeredDeepSWEV11PilotInventorySHA256 = "0d76a2c978a96350d1dc8468746e56ce25f34526aeffe85094d720979bf6a96b"
	registeredDeepSWEV11PilotLockFileSHA256  = "82b7be87fe9a25118564319959afac9c4ab8d9033a8c3b01dfed96664887a94e"
	registeredCodex0145BinarySHA256          = "a2a05dafaa1acb002a45eaec0a462de5b13694fcfcd7bc43305f14781ce7be14"
	registeredEgressProxyImage               = "ubuntu/squid@sha256:93d2d581a961f475ca5b23fe47fc3c3afadbe5849a6925a5b5435068502d7051"
)

var registeredDeepSWEV11PilotTasks = []string{
	"abs-module-cache-flags",
	"adaptix-name-mapping-aliases",
	"cliffy-config-file-parsing",
	"wasmi-trap-coredumps",
	"yjs-map-conflict-detection",
}

// validatePublishedBenchmarkContract prevents a self-consistent but unrelated
// bundle from presenting itself as a known published benchmark. The artifact
// ledger remains the run identity; these constants are the independently
// reviewed semantic identity for DeepSWE v1.1.
func validatePublishedBenchmarkContract(class ExperimentClass, meta ReportMeta, manifest harness.Manifest, plan harness.RunPlan, state harness.ExperimentState) error {
	// Display labels deliberately do not participate in identity. In
	// particular, EqualFold, Unicode confusables, and zero-width characters can
	// neither opt into nor escape a registered contract.
	if meta.BenchmarkContractID != BenchmarkContractDeepSWEV11Full113 && meta.BenchmarkContractID != BenchmarkContractDeepSWEV11Pilot5 {
		return errors.New("no published benchmark contract is registered for the exact benchmark_contract_id")
	}
	if (class == ClassFormal) != (meta.BenchmarkContractID == BenchmarkContractDeepSWEV11Full113) ||
		(class == ClassPilot) != (meta.BenchmarkContractID == BenchmarkContractDeepSWEV11Pilot5) {
		return errors.New("benchmark_contract_id and artifact evidence class do not match")
	}
	if manifest.Dataset.Name != "deep-swe-v1.1" ||
		manifest.Dataset.Repository != "https://github.com/datacurve-ai/deep-swe" ||
		manifest.Dataset.Commit != "8cae5984d5dd0ee37445beff0e928dc10c331116" ||
		manifest.Dataset.Root != "tasks" ||
		manifest.Dataset.TreeSHA256 != "ce6b3f3c7eff0b512d11060976c7f548267755afc26e377f50851b4523db98ea" ||
		manifest.Selection.ExpectedTaskCount != 113 {
		return errors.New("artifact dataset does not match the registered DeepSWE v1.1 universe")
	}
	registeredLockFileSHA256 := ""
	registeredInventorySHA256 := ""
	registeredInventoryCoverage := ""
	registeredInventoryTaskCount := 0
	switch meta.BenchmarkContractID {
	case BenchmarkContractDeepSWEV11Full113:
		if manifest.Dataset.ManifestSHA256 != registeredDeepSWEV11FullInventorySHA256 || manifest.Selection.Mode != "full" || len(manifest.Selection.TaskIDs) != 0 {
			return errors.New("artifact selection does not match the registered full113 inventory")
		}
		registeredLockFileSHA256 = registeredDeepSWEV11FullLockFileSHA256
		registeredInventorySHA256 = registeredDeepSWEV11FullInventorySHA256
		registeredInventoryCoverage = "full"
		registeredInventoryTaskCount = 113
	case BenchmarkContractDeepSWEV11Pilot5:
		if manifest.Dataset.ManifestSHA256 != registeredDeepSWEV11PilotInventorySHA256 || manifest.Selection.Mode != "tasks" || !slices.Equal(manifest.Selection.TaskIDs, registeredDeepSWEV11PilotTasks) {
			return errors.New("artifact selection does not match the registered pilot5 development inventory")
		}
		registeredLockFileSHA256 = registeredDeepSWEV11PilotLockFileSHA256
		registeredInventorySHA256 = registeredDeepSWEV11PilotInventorySHA256
		registeredInventoryCoverage = "tasks"
		registeredInventoryTaskCount = len(registeredDeepSWEV11PilotTasks)
	}
	lock := state.Backend.InventoryLock
	if lock.RelativePath != harness.InventoryLockArchiveRelativePath ||
		lock.FileSHA256 != registeredLockFileSHA256 ||
		lock.SchemaVersion != harness.PierInventoryLockSchemaVersion ||
		lock.HashAlgorithm != harness.TaskInventoryHashAlgorithm ||
		lock.DatasetCommit != manifest.Dataset.Commit ||
		lock.Coverage != registeredInventoryCoverage ||
		lock.TaskCount != registeredInventoryTaskCount ||
		lock.UniverseTaskCount != 113 ||
		lock.TaskInventorySHA256 != registeredInventorySHA256 ||
		state.Backend.InventoryCoverage != registeredInventoryCoverage ||
		state.Backend.InventoryTaskCount != registeredInventoryTaskCount ||
		state.Backend.UniverseTaskCount != 113 {
		return errors.New("archived inventory lock differs from the registered exact lock bytes or task universe")
	}
	if manifest.Evaluator.Name != "datacurve-pier" ||
		manifest.Evaluator.Repository != "https://github.com/datacurve-ai/pier" ||
		manifest.Evaluator.Commit != "e69a20e4e0ac073ec71fde0274bab3d9f40bac87" ||
		manifest.Evaluator.Root != "src" ||
		manifest.Evaluator.TreeSHA256 != "600c65f30f803d1a9219432f01dd8637e1bf1c636558b3606b0c957f156af197" ||
		manifest.Evaluator.ManifestSHA256 != "8afbae2c8c78ed6eaa3a49656bb4639d77c07cf6cd2b72266e4ad2283d8dc943" ||
		manifest.Evaluator.Protocol != "pier-harbor-separate-verifier" ||
		manifest.Evaluator.MinimumVersion != "0.3.0" {
		return errors.New("artifact evaluator does not match the registered DeepSWE v1.1 contract")
	}
	if manifest.Scoring.Profile != harness.ScoringProfileDeepSWEV11PublicCI {
		return errors.New("artifact scoring profile does not match the registered DeepSWE v1.1 public rules")
	}
	if manifest.ProviderEndpoint != harness.FormalProviderEndpoint() ||
		state.Backend.ProviderEndpoint.ApprovedOrigin != harness.FormalProviderOrigin ||
		state.Backend.ProviderEndpoint.SemanticsSHA256 != harness.FormalProviderEndpointSemanticsSHA256 ||
		state.Backend.ProviderEndpoint.TLSServerName != harness.FormalProviderTLSServerName ||
		!state.Backend.ProviderEndpoint.TLSVerified ||
		!hex64Pattern.MatchString(state.Backend.ProviderEndpoint.TLSPeerLeafCertSHA256) ||
		!hex64Pattern.MatchString(state.Backend.ProviderEndpoint.TLSPeerSPKISHA256) {
		return errors.New("configured gateway endpoint or observed TLS identity differs from the registered contract")
	}
	wantHostEnvironment := []string{"AGENTIC_SUB_API_KEY", "LANG", "LC_ALL", "PATH", "SSL_CERT_DIR", "SSL_CERT_FILE", "TMPDIR"}
	if !slices.Equal(manifest.Environment.HostEnvAllowlist, wantHostEnvironment) ||
		!slices.Equal(manifest.Environment.AgentEgressHosts, []string{"host.docker.internal"}) ||
		manifest.Environment.TaskNetworkMode != "no-network" || manifest.Environment.VerifierNetworkMode != "no-network" ||
		manifest.Timeouts.SetupSeconds != 1800 || manifest.Timeouts.AgentSeconds != 5400 ||
		manifest.Timeouts.VerifierSeconds != 1800 || manifest.Timeouts.TeardownSeconds != 300 ||
		manifest.Resources.CPUs != 2 || manifest.Resources.MemoryMB != 8192 || manifest.Resources.StorageMB != 20480 || manifest.Resources.GPUs != 0 ||
		manifest.Resources.HostStorageGuard != harness.FormalHostStorageGuard() || manifest.Resources.GuestStorageGuard != harness.FormalGuestStorageGuard() ||
		state.Backend.HostStorageGuard != harness.FormalHostStorageGuard() || state.Backend.GuestStorageGuard != harness.FormalGuestStorageGuard() ||
		manifest.Scheduling.Seed != 20260726 || manifest.Scheduling.MaxParallelPairs != 1 {
		return errors.New("artifact environment, resources, timeouts, or schedule differs from the registered DeepSWE v1.1 contract")
	}
	wantPricingEffectiveAt := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	wantPricingObservedAt := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	wantRate := harness.PricingRate{
		Provider: "openai", Model: "gpt-5.6-sol", Input: 5, CachedInput: 0.5, Output: 30,
		CacheWriteInputMultiplier: 1.25,
		RequestTiers: []harness.PricingTier{{
			Name: "long-context", ThresholdInputTokens: 272000,
			InputMultiplier: 2, CachedInputMultiplier: 2, OutputMultiplier: 1.5,
		}},
	}
	if manifest.Pricing.Currency != "USD" || manifest.Pricing.UnitTokens != 1_000_000 ||
		!manifest.Pricing.EffectiveAt.Equal(wantPricingEffectiveAt) || !manifest.Pricing.ObservedAt.Equal(wantPricingObservedAt) || manifest.Pricing.SourceURL != "https://developers.openai.com/api/docs/models/gpt-5.6-sol" ||
		len(manifest.Pricing.Rates) != 1 || !reflect.DeepEqual(manifest.Pricing.Rates[0], wantRate) {
		return errors.New("artifact pricing differs from the registered DeepSWE v1.1 catalog")
	}
	if meta.BaselineAgentID != "codex" || meta.ContenderAgentID != "luban" || len(manifest.Agents) != 2 {
		return errors.New("registered DeepSWE comparison requires baseline codex and contender luban")
	}
	agents := make(map[string]harness.AgentSpec, len(manifest.Agents))
	expectedTierEncoding := map[string]string{
		"codex": harness.ServiceTierEncodingClientCanonical,
		"luban": harness.ServiceTierEncodingExplicitDefault,
	}
	for _, agent := range manifest.Agents {
		if _, exists := agents[agent.ID]; exists {
			return errors.New("registered DeepSWE comparison contains a duplicate agent ID")
		}
		agents[agent.ID] = agent
		if agent.Model.Provider != "openai" || agent.Model.Model != "gpt-5.6-sol" || agent.Model.ReasoningEffort != "xhigh" || agent.Model.ServiceTier != harness.FormalServiceTier ||
			agent.Model.ServiceTierRequestEncoding != expectedTierEncoding[agent.ID] || agent.Model.TransportRequirement != harness.TransportRequirementHTTPInference {
			return errors.New("DeepSWE v1.1 comparison is not pinned to openai/gpt-5.6-sol/xhigh/default")
		}
		if agent.ExecutionCanary.Generation != harness.FormalExecutionCanaryGeneration || !hex64Pattern.MatchString(agent.ExecutionCanary.ReceiptSHA256) {
			return errors.New("registered DeepSWE comparison lacks a current v8 execution canary")
		}
		if !agent.RequestEvidence.Required || agent.RequestEvidence.RelativePath != "metrics/provider-requests.jsonl" {
			return errors.New("registered DeepSWE comparison requires normalized provider request evidence")
		}
	}
	codex, codexOK := agents["codex"]
	luban, lubanOK := agents["luban"]
	if !codexOK || !lubanOK || codex.SourceSnapshot != nil || luban.SourceSnapshot == nil || codex.BinarySHA256 != registeredCodex0145BinarySHA256 {
		return errors.New("agent source or binary identity differs from the registered Codex 0.145.0 versus Luban contract")
	}
	wantCodexArgv := []string{
		codex.Binary, "--ask-for-approval", "never", "--sandbox", "workspace-write",
		"exec", "--json", "--ephemeral", "--ignore-user-config", "--model", "gpt-5.6-sol",
		"--config", "model_reasoning_effort=xhigh", "--config", `service_tier="default"`, "--config", `web_search="disabled"`, "--config", "agents.enabled=false",
		"--config", `model_provider="agentic_http"`, "--config", `model_providers.agentic_http={name="OpenAI",base_url="{provider_base_url}",wire_api="responses",requires_openai_auth=true,supports_websockets=false}`,
		"{instruction_path}",
	}
	wantLubanArgv := []string{
		luban.Binary, "--print", "--output-format", "stream-json", "--provider", "openai", "--api", "responses",
		"--model", "gpt-5.6-sol", "--reasoning-effort", "xhigh", "--service-tier", "default", "--pinned-model", "--no-model-fallback",
		"--allow-all", "--force-sandbox-tools", "{instruction_path}",
	}
	wantRequiredEnv := []string{"AGENTIC_SUB_API_KEY", "PATH"}
	if !slices.Equal(codex.Command.Argv, wantCodexArgv) || !slices.Equal(luban.Command.Argv, wantLubanArgv) ||
		!slices.Equal(codex.Command.RequiredEnv, wantRequiredEnv) || !slices.Equal(luban.Command.RequiredEnv, wantRequiredEnv) {
		return errors.New("agent argv differs from the registered formal invocation contract")
	}
	if state.Backend.EgressProxyImage != registeredEgressProxyImage {
		return errors.New("egress proxy image differs from the registered Squid digest")
	}
	if class == ClassFormal {
		if manifest.Scheduling.Repetitions != 4 {
			return errors.New("formal DeepSWE v1.1 comparison must preserve four paired repetitions")
		}
		if err := validateRegisteredCrossoverCounts(plan, manifest.Agents, 226, 226); err != nil {
			return err
		}
	}
	if class == ClassPilot {
		if selectionTaskCount(manifest.Selection) != 5 || manifest.Scheduling.Repetitions != 1 {
			return errors.New("pilot5 development comparison must preserve five tasks and one paired repetition")
		}
		if err := validateRegisteredCrossoverCounts(plan, manifest.Agents, 3, 2); err != nil {
			return err
		}
	}
	return nil
}

func validateRegisteredCrossoverCounts(plan harness.RunPlan, agents []harness.AgentSpec, high, low int) error {
	counts := make(map[string][2]int, len(agents))
	if len(plan.Entries)%2 != 0 {
		return errors.New("registered crossover plan has an odd number of entries")
	}
	for index := 0; index < len(plan.Entries); index += 2 {
		first, second := plan.Entries[index], plan.Entries[index+1]
		if first.PairID != second.PairID || first.TaskID != second.TaskID || first.Repetition != second.Repetition || first.AgentID == second.AgentID {
			return errors.New("registered crossover plan is not composed of adjacent two-agent pairs")
		}
		firstCounts := counts[first.AgentID]
		firstCounts[0]++
		counts[first.AgentID] = firstCounts
		secondCounts := counts[second.AgentID]
		secondCounts[1]++
		counts[second.AgentID] = secondCounts
	}
	for _, agent := range agents {
		count, exists := counts[agent.ID]
		if !exists || !((count[0] == high && count[1] == low) || (count[0] == low && count[1] == high)) {
			return fmt.Errorf("agent %s crossover first/second counts are %d/%d; want %d/%d in either direction", agent.ID, count[0], count[1], high, low)
		}
	}
	return nil
}
