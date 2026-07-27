package pierbackend

import (
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestValidateFormalManifestPinsReleaseCorpusScheduleAndCommands(t *testing.T) {
	config := Config{
		ProxyAdvertiseHost: "host.docker.internal", ProviderCredentialEnv: formalProviderCredentialEnv,
		EgressProxyImage: FrozenEgressProxyImage, ProviderUpstream: harness.FormalProviderOrigin,
	}
	full := formalManifestFixture()
	if err := validateFormalManifest(full, config); err != nil {
		t.Fatalf("valid full manifest: %v", err)
	}
	pilot := formalManifestFixture()
	pilot.Selection.Mode = "tasks"
	pilot.Selection.TaskIDs = []string{"task-one", "task-two"}
	pilot.Scheduling.Repetitions = 1
	if err := validateFormalManifest(pilot, config); err != nil {
		t.Fatalf("valid explicit pilot manifest: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*harness.Manifest)
	}{
		{"dataset commit", func(value *harness.Manifest) { value.Dataset.Commit = "0000000000000000000000000000000000000000" }},
		{"dataset tree", func(value *harness.Manifest) { value.Dataset.TreeSHA256 = strings.Repeat("0", 64) }},
		{"evaluator commit", func(value *harness.Manifest) { value.Evaluator.Commit = "0000000000000000000000000000000000000000" }},
		{"evaluator tree", func(value *harness.Manifest) { value.Evaluator.TreeSHA256 = strings.Repeat("0", 64) }},
		{"Pier version", func(value *harness.Manifest) { value.Evaluator.MinimumVersion = "0.2.0" }},
		{"sample selection", func(value *harness.Manifest) { value.Selection.Mode = "sample" }},
		{"full repetitions", func(value *harness.Manifest) { value.Scheduling.Repetitions = 1 }},
		{"parallel pairs", func(value *harness.Manifest) { value.Scheduling.MaxParallelPairs = 2 }},
		{"task network", func(value *harness.Manifest) { value.Environment.TaskNetworkMode = "internet" }},
		{"Codex search", func(value *harness.Manifest) {
			value.Agents[0].Command.Argv = append(value.Agents[0].Command.Argv[:1], append([]string{"--search"}, value.Agents[0].Command.Argv[1:]...)...)
		}},
		{"Codex approval", func(value *harness.Manifest) { value.Agents[0].Command.Argv[2] = "on-request" }},
		{"Codex sandbox", func(value *harness.Manifest) { value.Agents[0].Command.Argv[4] = "danger-full-access" }},
		{"Codex model", func(value *harness.Manifest) { value.Agents[0].Command.Argv[10] = "gpt-5.6" }},
		{"Codex effort", func(value *harness.Manifest) { value.Agents[0].Command.Argv[12] = "model_reasoning_effort=high" }},
		{"Codex service tier", func(value *harness.Manifest) { value.Agents[0].Command.Argv[14] = `service_tier="auto"` }},
		{"Codex service tier encoding", func(value *harness.Manifest) {
			value.Agents[0].Model.ServiceTierRequestEncoding = harness.ServiceTierEncodingExplicitDefault
		}},
		{"Codex web search", func(value *harness.Manifest) { value.Agents[0].Command.Argv[16] = `web_search="enabled"` }},
		{"Codex agents", func(value *harness.Manifest) { value.Agents[0].Command.Argv[18] = "agents.enabled=true" }},
		{"Codex store override", func(value *harness.Manifest) {
			value.Agents[0].Command.Argv = append(value.Agents[0].Command.Argv, "--config", "disable_response_storage=false")
		}},
		{"Luban service tier", func(value *harness.Manifest) { value.Agents[1].Command.Argv[13] = "auto" }},
		{"Luban service tier encoding", func(value *harness.Manifest) {
			value.Agents[1].Model.ServiceTierRequestEncoding = harness.ServiceTierEncodingClientCanonical
		}},
		{"Luban fallback", func(value *harness.Manifest) { value.Agents[1].Command.Argv[15] = "--model-fallback" }},
		{"Luban sandbox", func(value *harness.Manifest) { value.Agents[1].Command.Argv[17] = "--no-force-sandbox-tools" }},
		{"Codex WebSocket transport", func(value *harness.Manifest) {
			value.Agents[0].Model.TransportRequirement = harness.TransportRequirementWebSocket
		}},
		{"Luban stale canary", func(value *harness.Manifest) { value.Agents[1].ExecutionCanary.Generation = "v7" }},
		{"reused canary", func(value *harness.Manifest) {
			value.Agents[1].ExecutionCanary.ReceiptSHA256 = value.Agents[0].ExecutionCanary.ReceiptSHA256
		}},
		{"default provider credential", func(value *harness.Manifest) {
			value.Environment.HostEnvAllowlist = append(value.Environment.HostEnvAllowlist, "OPENAI_API_KEY")
		}},
		{"legacy provider credential", func(value *harness.Manifest) {
			value.Agents[0].Command.RequiredEnv = []string{"CODEX_LB_API_KEY", "PATH"}
		}},
		{"pricing currency", func(value *harness.Manifest) { value.Pricing.Currency = "EUR" }},
		{"pricing unit", func(value *harness.Manifest) { value.Pricing.UnitTokens = 1_000_000_000_000 }},
		{"pricing source", func(value *harness.Manifest) { value.Pricing.SourceURL = "https://example.invalid" }},
		{"pricing effective", func(value *harness.Manifest) {
			value.Pricing.EffectiveAt = time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
		}},
		{"pricing effective timezone", func(value *harness.Manifest) {
			value.Pricing.EffectiveAt = time.Date(2026, 7, 9, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
		}},
		{"pricing observed", func(value *harness.Manifest) {
			value.Pricing.ObservedAt = time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
		}},
		{"pricing observed timezone", func(value *harness.Manifest) {
			value.Pricing.ObservedAt = time.Date(2026, 7, 26, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
		}},
		{"pricing duplicate", func(value *harness.Manifest) {
			value.Pricing.Rates = append(value.Pricing.Rates, value.Pricing.Rates[0])
		}},
		{"pricing rate", func(value *harness.Manifest) { value.Pricing.Rates[0].Input = 4.99 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := formalManifestFixture()
			test.mutate(&manifest)
			if err := validateFormalManifest(manifest, config); err == nil {
				t.Fatal("invalid formal manifest was accepted")
			}
		})
	}
}

func TestHasFormalGPT56SolPricingPinsAuthorityDatesAndUTCRepresentation(t *testing.T) {
	catalog := formalManifestFixture().Pricing
	if !hasFormalGPT56SolPricing(catalog) {
		t.Fatal("exact formal pricing catalog was rejected")
	}

	tests := map[string]func(*harness.PricingCatalog){
		"effective date": func(value *harness.PricingCatalog) {
			value.EffectiveAt = time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
		},
		"observed date": func(value *harness.PricingCatalog) {
			value.ObservedAt = time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
		},
		"effective equivalent offset": func(value *harness.PricingCatalog) {
			value.EffectiveAt = time.Date(2026, 7, 9, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
		},
		"observed equivalent offset": func(value *harness.PricingCatalog) {
			value.ObservedAt = time.Date(2026, 7, 26, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := catalog
			mutate(&candidate)
			if hasFormalGPT56SolPricing(candidate) {
				t.Fatal("pricing authority date drift was accepted")
			}
		})
	}
}

func formalManifestFixture() harness.Manifest {
	codexBinary := "/opt/codex/vendor/x86_64-unknown-linux-musl/bin/codex"
	lubanBinary := "/opt/luban/luban"
	model := harness.ModelRequestSpec{
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: "default",
		TransportRequirement: harness.TransportRequirementHTTPInference,
	}
	codexModel, lubanModel := model, model
	codexModel.ServiceTierRequestEncoding = harness.ServiceTierEncodingClientCanonical
	lubanModel.ServiceTierRequestEncoding = harness.ServiceTierEncodingExplicitDefault
	codexModel.ToolCatalog = formalTestToolCatalog("codex")
	lubanModel.ToolCatalog = formalTestToolCatalog("luban")
	return harness.Manifest{
		ProviderEndpoint: harness.FormalProviderEndpoint(),
		Dataset:          harness.SourcePin{Commit: formalDatasetCommit, TreeSHA256: formalDatasetTreeSHA256},
		Evaluator: harness.EvaluatorSpec{
			SourcePin: harness.SourcePin{Commit: formalEvaluatorCommit, TreeSHA256: formalEvaluatorTreeSHA256},
			Protocol:  "pier-harbor-separate-verifier", MinimumVersion: formalPierVersion,
		},
		Agents: []harness.AgentSpec{
			{
				ID: codexBinaryID, Binary: codexBinary, Model: codexModel,
				ExecutionCanary: harness.ExecutionCanarySpec{Generation: harness.FormalExecutionCanaryGeneration, ReceiptSHA256: strings.Repeat("1", 64)},
				Command: harness.CommandSpec{
					Argv: []string{
						codexBinary, "--ask-for-approval", "never", "--sandbox", "workspace-write",
						"exec", "--json", "--ephemeral", "--ignore-user-config", "--model", "gpt-5.6-sol",
						"--config", "model_reasoning_effort=xhigh", "--config", `service_tier="default"`, "--config", `web_search="disabled"`, "--config", "agents.enabled=false",
						"--config", codexHTTPProviderSelection, "--config", codexHTTPProviderConfig, "{instruction_path}",
					},
					RequiredEnv: []string{formalProviderCredentialEnv, "PATH"},
				},
			},
			{
				ID: "luban", Binary: lubanBinary, Model: lubanModel,
				ExecutionCanary: harness.ExecutionCanarySpec{Generation: harness.FormalExecutionCanaryGeneration, ReceiptSHA256: strings.Repeat("2", 64)},
				SourceSnapshot: &harness.AgentSourceSpec{
					Worktree: "/src/luban", BaseCommit: strings.Repeat("a", 40), TreeOID: strings.Repeat("b", 40),
					PatchSHA256: strings.Repeat("c", 64), ArchiveSHA256: strings.Repeat("d", 64),
					BuildReceipt: "/receipts/luban.json", BuildReceiptSHA256: strings.Repeat("e", 64),
				},
				Command: harness.CommandSpec{
					Argv: []string{
						lubanBinary, "--print", "--output-format", "stream-json", "--provider", "openai", "--api", "responses",
						"--model", "gpt-5.6-sol", "--reasoning-effort", "xhigh", "--service-tier", "default", "--pinned-model", "--no-model-fallback",
						"--allow-all", "--force-sandbox-tools", "{instruction_path}",
					},
					RequiredEnv: []string{formalProviderCredentialEnv, "PATH"},
				},
			},
		},
		Selection:  harness.SelectionSpec{Mode: "full", ExpectedTaskCount: formalTaskCount},
		Scheduling: harness.SchedulingSpec{PairAgents: true, Repetitions: 4, MaxParallelPairs: 1},
		Pricing: harness.PricingCatalog{
			Currency: "USD", UnitTokens: 1_000_000, EffectiveAt: time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC),
			ObservedAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
			SourceURL:  "https://developers.openai.com/api/docs/models/gpt-5.6-sol",
			Rates: []harness.PricingRate{{
				Provider: "openai", Model: "gpt-5.6-sol", Input: 5, CachedInput: .5, Output: 30, CacheWriteInputMultiplier: 1.25,
				RequestTiers: []harness.PricingTier{{Name: "long-context", ThresholdInputTokens: 272000, InputMultiplier: 2, CachedInputMultiplier: 2, OutputMultiplier: 1.5}},
			}}},
		Environment: harness.EnvironmentSpec{
			HostEnvAllowlist: []string{formalProviderCredentialEnv, "PATH"}, AgentEgressHosts: []string{"host.docker.internal"}, TaskNetworkMode: "no-network", VerifierNetworkMode: "no-network",
		},
		Oracle: harness.OracleSpec{Required: true, SeparateEnvironment: true},
	}
}

func formalTestToolCatalog(agentID string) harness.ToolCatalogSpec {
	definitions, semanticSHA, _ := fixtureProviderToolCatalog(agentID)
	tools := make([]harness.ToolIdentitySpec, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, harness.ToolIdentitySpec{Type: definition.Type, Name: definition.Name, DefinitionSHA256: definition.DefinitionSHA256})
	}
	return harness.ToolCatalogSpec{SchemaVersion: harness.FormalToolCatalogSchemaVersion, SemanticSHA256: semanticSHA, Tools: tools}
}

const codexBinaryID = "codex"
