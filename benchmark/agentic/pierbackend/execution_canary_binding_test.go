package pierbackend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestConfiguredExecutionCanariesFailClosedPerAgent(t *testing.T) {
	manifest := formalManifestFixture()
	config := Config{
		CodexV8CanaryReceiptPath: filepath.Join(t.TempDir(), "codex.json"), CodexV8CanaryReceiptSHA256: strings.Repeat("1", 64),
		LubanV8CanaryReceiptPath: filepath.Join(t.TempDir(), "luban.json"), LubanV8CanaryReceiptSHA256: strings.Repeat("2", 64),
	}
	if _, err := validateConfiguredExecutionCanaries(config, manifest); err != nil {
		t.Fatalf("valid configured canary pair: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Config, *harness.Manifest)
	}{
		{"missing pair", func(config *Config, _ *harness.Manifest) { *config = Config{} }},
		{"partial Codex pin", func(config *Config, _ *harness.Manifest) { config.CodexV8CanaryReceiptSHA256 = "" }},
		{"reused path", func(config *Config, _ *harness.Manifest) {
			config.LubanV8CanaryReceiptPath = config.CodexV8CanaryReceiptPath
		}},
		{"reused hash", func(config *Config, _ *harness.Manifest) {
			config.LubanV8CanaryReceiptSHA256 = config.CodexV8CanaryReceiptSHA256
		}},
		{"swapped manifest hashes", func(_ *Config, manifest *harness.Manifest) {
			manifest.Agents[0].ExecutionCanary.ReceiptSHA256, manifest.Agents[1].ExecutionCanary.ReceiptSHA256 = manifest.Agents[1].ExecutionCanary.ReceiptSHA256, manifest.Agents[0].ExecutionCanary.ReceiptSHA256
		}},
		{"stale v7", func(_ *Config, manifest *harness.Manifest) { manifest.Agents[0].ExecutionCanary.Generation = "v7" }},
		{"WebSocket transport", func(_ *Config, manifest *harness.Manifest) {
			manifest.Agents[1].Model.TransportRequirement = harness.TransportRequirementWebSocket
		}},
		{"duplicate agent", func(_ *Config, manifest *harness.Manifest) { manifest.Agents[1].ID = manifest.Agents[0].ID }},
		{"missing agent", func(_ *Config, manifest *harness.Manifest) { manifest.Agents = manifest.Agents[:1] }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutatedConfig, mutatedManifest := config, manifest
			test.mutate(&mutatedConfig, &mutatedManifest)
			if _, err := validateConfiguredExecutionCanaries(mutatedConfig, mutatedManifest); err == nil {
				t.Fatal("counterfeit per-agent canary configuration was accepted")
			}
		})
	}
}

func TestBackendConfigRequiresIsolatedCredentialAndPreservesBothCanaryPins(t *testing.T) {
	root := t.TempDir()
	config := Config{
		PierBinary: filepath.Join(root, "pier"), DatasetRepositoryRoot: filepath.Join(root, "dataset"),
		EvaluatorRepositoryRoot: filepath.Join(root, "evaluator"), EvaluatorManifestPath: filepath.Join(root, "evaluator.json"),
		InventoryLockPath: filepath.Join(root, "inventory.json"), PythonModuleRoot: filepath.Join(root, "module"),
		PrivateWorkRoot: filepath.Join(root, "private"), EgressProxyImage: FrozenEgressProxyImage,
		ProxyListenAddress: "127.0.0.1:0", ProxyAdvertiseHost: "host.docker.internal",
		ProviderUpstream: harness.FormalProviderOrigin, ProviderCredentialEnv: formalProviderCredentialEnv,
		CodexV8CanaryReceiptPath: filepath.Join(root, "opaque-a"), CodexV8CanaryReceiptSHA256: strings.Repeat("1", 64),
		LubanV8CanaryReceiptPath: filepath.Join(root, "opaque-b"), LubanV8CanaryReceiptSHA256: strings.Repeat("2", 64),
	}
	if _, err := New(config); err != nil {
		t.Fatalf("valid isolated backend config: %v", err)
	}
	for _, credentialEnv := range []string{"", "OPENAI_API_KEY", "CODEX_LB_API_KEY"} {
		mutated := config
		mutated.ProviderCredentialEnv = credentialEnv
		if _, err := New(mutated); err == nil {
			t.Fatalf("controller credential environment %q was accepted", credentialEnv)
		}
	}

	fileConfig := FileConfig{
		PierBinary: config.PierBinary, DatasetRepositoryRoot: config.DatasetRepositoryRoot,
		EvaluatorRepositoryRoot: config.EvaluatorRepositoryRoot, EvaluatorManifestPath: config.EvaluatorManifestPath,
		InventoryLockPath: config.InventoryLockPath, PythonModuleRoot: config.PythonModuleRoot, PrivateWorkRoot: config.PrivateWorkRoot,
		CodexV8CanaryReceiptPath: config.CodexV8CanaryReceiptPath, CodexV8CanaryReceiptSHA256: config.CodexV8CanaryReceiptSHA256,
		LubanV8CanaryReceiptPath: config.LubanV8CanaryReceiptPath, LubanV8CanaryReceiptSHA256: config.LubanV8CanaryReceiptSHA256,
		EgressProxyImage: config.EgressProxyImage, ProxyListenAddress: config.ProxyListenAddress, ProxyAdvertiseHost: config.ProxyAdvertiseHost,
		ProviderUpstream: config.ProviderUpstream, ProviderCredentialEnv: config.ProviderCredentialEnv,
	}
	raw, err := json.Marshal(fileConfig)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "backend.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CodexV8CanaryReceiptPath != config.CodexV8CanaryReceiptPath || loaded.CodexV8CanaryReceiptSHA256 != config.CodexV8CanaryReceiptSHA256 ||
		loaded.LubanV8CanaryReceiptPath != config.LubanV8CanaryReceiptPath || loaded.LubanV8CanaryReceiptSHA256 != config.LubanV8CanaryReceiptSHA256 {
		t.Fatalf("loaded per-agent canary pins = %#v", loaded)
	}
}

func TestConfiguredExecutionCanaryHeadersRejectPendingSwappedAndStaleBeforePreflight(t *testing.T) {
	directory := t.TempDir()
	writeHeader := func(name, agentID, generation, authority string) configuredExecutionCanary {
		t.Helper()
		receipt := map[string]any{
			"schema_version": "agentic-bench/sandbox-canary-v4", "agent_kind": agentID,
			"provider_canary_transport": "responses-http-inference-required",
		}
		if agentID == "codex" {
			receipt["canonical_authority"] = map[string]any{
				"generation": generation, "authority_scope": authority,
				"responses_transport_requirement": harness.TransportRequirementHTTPInference,
			}
		}
		raw, err := json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		digest, err := harness.HashFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return configuredExecutionCanary{AgentID: agentID, ReceiptPath: path, ReceiptSHA256: digest}
	}
	valid := map[string]configuredExecutionCanary{
		"codex": writeHeader("codex.json", "codex", "v8", string(codexCanaryVerifiedFormal)),
		"luban": writeHeader("luban.json", "luban", "", ""),
	}
	if err := validateConfiguredExecutionCanaryHeaders(valid); err != nil {
		t.Fatalf("valid per-agent header pair: %v", err)
	}
	tests := []struct {
		name string
		pins map[string]configuredExecutionCanary
	}{
		{"pending Codex", map[string]configuredExecutionCanary{
			"codex": writeHeader("pending.json", "codex", "v8", "pending_repin"), "luban": valid["luban"],
		}},
		{"stale Codex", map[string]configuredExecutionCanary{
			"codex": writeHeader("v7.json", "codex", "v7", string(codexCanaryVerifiedFormal)), "luban": valid["luban"],
		}},
		{"swapped agents", map[string]configuredExecutionCanary{
			"codex": {AgentID: "codex", ReceiptPath: valid["luban"].ReceiptPath, ReceiptSHA256: valid["luban"].ReceiptSHA256},
			"luban": {AgentID: "luban", ReceiptPath: valid["codex"].ReceiptPath, ReceiptSHA256: valid["codex"].ReceiptSHA256},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateConfiguredExecutionCanaryHeaders(test.pins); err == nil {
				t.Fatal("invalid early execution canary header was accepted")
			}
		})
	}
}

func TestResolveFormalExecutionCanariesReturnsTwoExactUniqueSnapshots(t *testing.T) {
	manifest := formalManifestFixture()
	manifest.Agents[0].BinarySHA256 = Codex0145BinarySHA256
	manifest.Agents[1].BinarySHA256 = strings.Repeat("9", 64)
	adapter := adapterBinding{SHA256: strings.Repeat("a", 64)}
	bundle := codexBundleBinding{ManifestSHA256: strings.Repeat("b", 64), TreeSHA256: CodexBundleTreeSHA256}
	directory := t.TempDir()
	codexPath := filepath.Join(directory, "authority-a.json")
	lubanPath := filepath.Join(directory, "authority-b.json")
	writeFormalCanaryFixture(t, codexPath, manifest.Agents[0], adapter, bundle)
	writeFormalCanaryFixture(t, lubanPath, manifest.Agents[1], adapter, bundle)
	codexSHA, err := harness.HashFile(codexPath)
	if err != nil {
		t.Fatal(err)
	}
	lubanSHA, err := harness.HashFile(lubanPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Agents[0].ExecutionCanary.ReceiptSHA256 = codexSHA
	manifest.Agents[1].ExecutionCanary.ReceiptSHA256 = lubanSHA
	config := Config{
		CodexV8CanaryReceiptPath: codexPath, CodexV8CanaryReceiptSHA256: codexSHA,
		LubanV8CanaryReceiptPath: lubanPath, LubanV8CanaryReceiptSHA256: lubanSHA,
	}
	bindings, err := resolveFormalExecutionCanaryBindings(config, manifest, adapter, bundle)
	if err != nil {
		t.Fatal(err)
	}
	snapshots := executionCanarySnapshots(bindings)
	want := []harness.ExecutionCanarySnapshot{
		{AgentID: "codex", Generation: "v8", TransportRequirement: harness.TransportRequirementHTTPInference, ReceiptSHA256: codexSHA},
		{AgentID: "luban", Generation: "v8", TransportRequirement: harness.TransportRequirementHTTPInference, ReceiptSHA256: lubanSHA},
	}
	if !slices.Equal(snapshots, want) {
		t.Fatalf("execution canary snapshots = %#v, want %#v", snapshots, want)
	}

	t.Run("swapped receipt content", func(t *testing.T) {
		swapped := config
		swapped.CodexV8CanaryReceiptPath, swapped.LubanV8CanaryReceiptPath = swapped.LubanV8CanaryReceiptPath, swapped.CodexV8CanaryReceiptPath
		swapped.CodexV8CanaryReceiptSHA256, swapped.LubanV8CanaryReceiptSHA256 = swapped.LubanV8CanaryReceiptSHA256, swapped.CodexV8CanaryReceiptSHA256
		swappedManifest := manifest
		swappedManifest.Agents[0].ExecutionCanary.ReceiptSHA256 = swapped.CodexV8CanaryReceiptSHA256
		swappedManifest.Agents[1].ExecutionCanary.ReceiptSHA256 = swapped.LubanV8CanaryReceiptSHA256
		if _, err := resolveFormalExecutionCanaryBindings(swapped, swappedManifest, adapter, bundle); err == nil {
			t.Fatal("agent-swapped receipt contents were accepted")
		}
	})

	t.Run("stale v7 receipt", func(t *testing.T) {
		stalePath := filepath.Join(directory, "stale-v7.json")
		if err := os.WriteFile(stalePath, []byte(`{"schema_version":"agentic-bench/sandbox-canary-v3","agent_kind":"luban"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		staleSHA, err := harness.HashFile(stalePath)
		if err != nil {
			t.Fatal(err)
		}
		staleConfig, staleManifest := config, manifest
		staleConfig.LubanV8CanaryReceiptPath, staleConfig.LubanV8CanaryReceiptSHA256 = stalePath, staleSHA
		staleManifest.Agents[1].ExecutionCanary.ReceiptSHA256 = staleSHA
		if _, err := resolveFormalExecutionCanaryBindings(staleConfig, staleManifest, adapter, bundle); err == nil {
			t.Fatal("historical v7-shaped receipt was accepted as v8 authority")
		}
	})
}

func TestFormalToolDefinitionEvidenceRejectsWeakOrReorderedCatalogs(t *testing.T) {
	manifest := formalManifestFixture()
	catalog := manifest.Agents[0].Model.ToolCatalog
	definitions, total := formalToolDefinitionsFixture(catalog)
	if err := validateFormalToolDefinitionEvidence(definitions, catalog.SemanticSHA256, total, catalog); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*[]formalToolDefinition, *string, *int64)
	}{
		{"missing definitions", func(definitions *[]formalToolDefinition, _ *string, _ *int64) { *definitions = nil }},
		{"wrong definition hash", func(definitions *[]formalToolDefinition, _ *string, _ *int64) {
			(*definitions)[0].DefinitionSHA256 = strings.Repeat("f", 64)
		}},
		{"wrong semantic hash", func(_ *[]formalToolDefinition, semantic *string, _ *int64) { *semantic = strings.Repeat("f", 64) }},
		{"swapped order", func(definitions *[]formalToolDefinition, _ *string, _ *int64) {
			(*definitions)[0], (*definitions)[1] = (*definitions)[1], (*definitions)[0]
		}},
		{"duplicate identity", func(definitions *[]formalToolDefinition, _ *string, _ *int64) { (*definitions)[1] = (*definitions)[0] }},
		{"wrong byte total", func(_ *[]formalToolDefinition, _ *string, total *int64) { *total++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, semantic, bytes := slices.Clone(definitions), catalog.SemanticSHA256, total
			test.mutate(&got, &semantic, &bytes)
			if err := validateFormalToolDefinitionEvidence(got, semantic, bytes, catalog); err == nil {
				t.Fatal("weak or counterfeit tool definition evidence was accepted")
			}
		})
	}
}

func TestFormalToolDefinitionDecoderRejectsUnknownRawAndDuplicateFields(t *testing.T) {
	fixtures := []string{
		`{"type":"function","name":"Run","definition_sha256":"` + strings.Repeat("a", 64) + `","definition_bytes":1,"raw_schema":{"type":"object"}}`,
		`{"type":"function","name":"Run","name":"Inspect","definition_sha256":"` + strings.Repeat("a", 64) + `","definition_bytes":1}`,
		`{"type":"function","name":"Run","definition_sha256":"` + strings.Repeat("a", 64) + `"}`,
	}
	for index, raw := range fixtures {
		var definition formalToolDefinition
		if err := json.Unmarshal([]byte(raw), &definition); err == nil {
			t.Fatalf("counterfeit tool definition fixture %d was accepted", index)
		}
	}
}

func writeFormalCanaryFixture(t *testing.T, path string, agent harness.AgentSpec, adapter adapterBinding, bundle codexBundleBinding) {
	t.Helper()
	effective := formalEffectiveArgvFixture(t, agent, adapter, bundle)
	effectiveRaw, err := json.Marshal(effective)
	if err != nil {
		t.Fatal(err)
	}
	policy := formalCachePolicy{
		Observed: true, ShapeValid: true, PromptCacheKeyPresent: true, PromptCacheKeySHA256: strings.Repeat("c", 64),
		PromptCacheOptionsPresent: true, PromptCacheOptionsMode: "automatic", PromptCacheOptionsTTLPresent: true,
		PromptCacheOptionsTTL: "1h", PromptCacheOptionsTTLSeconds: intPointer(3600),
		PromptCacheBreakpointCount: 1, PromptCacheBreakpointPositionHashes: []string{strings.Repeat("d", 64)},
	}
	definitions, definitionBytes := formalToolDefinitionsFixture(agent.Model.ToolCatalog)
	requests := make([]any, 0, 2)
	if agent.ID == "codex" {
		catalog := make([]canaryTool, 0, len(agent.Model.ToolCatalog.Tools))
		for _, tool := range agent.Model.ToolCatalog.Tools {
			name := tool.Name
			catalog = append(catalog, canaryTool{Type: tool.Type, Name: &name})
		}
		for index := range 2 {
			var exit *int
			if index == 1 {
				exit = intPointer(0)
			}
			requests = append(requests, formalCodexHTTPRequest{
				codexLiteCanaryRequest: codexLiteCanaryRequest{
					RequestIndex: index, Model: agent.Model.Model, Store: boolPointer(false), ReasoningEffort: agent.Model.ReasoningEffort,
					ReasoningContext: "all_turns", IncludeEncryptedReasoning: true, Stream: true,
					RequestServiceTierCanonical: harness.FormalServiceTier, RequestServiceTierSource: serviceTierCanonicalizationRepresentation,
					ToolCatalog: catalog, ExecCellWaitPresent: true, ResponsesLiteHeaderPresent: true, AuthorizationHeaderPresent: true,
					Originator: "codex_exec", UserAgentPresent: true, CustomToolOutputCount: index, ToolOutputExitCode: exit,
					ResponseModel: agent.Model.Model, ResponseServiceTier: harness.FormalServiceTier, ResponseServiceTierCanonical: harness.FormalServiceTier,
					ResponseRequestIDPresent: true, ResponseUsage: canaryUsage{InputTokens: 11, CachedInputTokens: 3, CacheWriteInputTokens: 2, OutputTokens: 5, ReasoningOutputTokens: 1},
				},
				Transport: "http_sse", CachePolicy: policy, ToolDefinitions: definitions,
				ToolCatalogSemanticSHA256: agent.Model.ToolCatalog.SemanticSHA256, ToolCatalogCanonicalBytes: definitionBytes,
			})
		}
	} else {
		names := make([]string, 0, len(agent.Model.ToolCatalog.Tools))
		for _, tool := range agent.Model.ToolCatalog.Tools {
			names = append(names, tool.Name)
		}
		for index := range 2 {
			requests = append(requests, formalLubanHTTPRequest{
				lubanCanaryRequest: lubanCanaryRequest{
					RequestIndex: index, Model: agent.Model.Model, Store: boolPointer(false), ReasoningEffort: agent.Model.ReasoningEffort,
					RequestServiceTierPresent: true, RequestServiceTier: stringPointer(harness.FormalServiceTier),
					RequestServiceTierCanonical: harness.FormalServiceTier, RequestServiceTierSource: "wire_explicit_default", ToolNames: names,
					ResponsesLiteHeader: json.RawMessage("null"), ResponseModel: agent.Model.Model, ResponseServiceTier: harness.FormalServiceTier,
					ResponseServiceTierCanonical: harness.FormalServiceTier, ResponseRequestIDPresent: true,
				},
				Transport: "http_sse", CachePolicy: policy, ToolDefinitions: definitions,
				ToolCatalogSemanticSHA256: agent.Model.ToolCatalog.SemanticSHA256, ToolCatalogCanonicalBytes: definitionBytes,
			})
		}
	}
	receipt := map[string]any{
		"schema_version": "agentic-bench/sandbox-canary-v4", "agent_kind": agent.ID, "binary_sha256": agent.BinarySHA256,
		"base_commit": strings.Repeat("e", 40), "controller_proxy_reachable": true, "tool_proxy_reachable": false, "credential_in_agent": false,
		"adapter_sha256": adapter.SHA256, "bundle_manifest_sha256": bundle.ManifestSHA256, "effective_argv_receipt_sha256": sha256Hex(effectiveRaw),
		"source_bundle_tree_sha256": bundle.TreeSHA256, "runtime_payload_tree_sha256": bundle.TreeSHA256,
		"provider_canary_requests": requests, "provider_canary_transport": "responses-http-inference-required",
		"http_transport": formalHTTPTransportReceipt{SchemaVersion: "agentic-bench/http-inference-transport-v1", Requirement: harness.TransportRequirementHTTPInference, HTTPInferenceRequestCount: 2},
		"cache_wire":     formalCacheWireFixture(policy), "effective_argv_receipt": effective, "overlay": map[string]any{"services": map[string]any{}},
		"egress_proxy_image": FrozenEgressProxyImage,
	}
	if agent.ID == "codex" {
		receipt["canonical_authority"] = codexCanonicalAuthorityReceipt{Generation: "v8", AuthorityScope: string(codexCanaryVerifiedFormal), ResponsesTransportRequirement: harness.TransportRequirementHTTPInference}
		receipt["sandbox_negative_control"] = map[string]any{}
		receipt["web_search_configuration_canary"] = map[string]any{}
		receipt["workspace_state"] = map[string]any{}
	} else {
		receipt["runtime_payload_tree_sha256"] = LubanRuntimeTreeSHA256
		receipt["luban_runtime_versions"] = []string{"bwrap 1", "rg 1"}
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func formalEffectiveArgvFixture(t *testing.T, agent harness.AgentSpec, adapter adapterBinding, bundle codexBundleBinding) effectiveArgvReceipt {
	t.Helper()
	argv, err := expectedEffectiveArgv(agent, "", true)
	if err != nil {
		t.Fatal(err)
	}
	argvSHA, err := harness.HashCanonical(argv)
	if err != nil {
		t.Fatal(err)
	}
	commandSHA, err := harness.HashCanonical(agent.Command.Argv)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := projectEffectiveArgv(agent.ID, argv)
	if err != nil {
		t.Fatal(err)
	}
	projectionSHA, err := harness.HashCanonical(projection)
	if err != nil {
		t.Fatal(err)
	}
	return effectiveArgvReceipt{
		AdapterSHA256: adapter.SHA256, AdapterVersion: PinnedAdapterVersion, AgentKind: agent.ID,
		BundleManifestSHA256: bundle.ManifestSHA256, BundleTreeSHA256: bundle.TreeSHA256, EffectiveArgv: argv,
		EffectiveArgvSHA256: argvSHA, ExecutionArgvSHA256: strings.Repeat("8", 64), PrivateProxyBaseURLSHA256: strings.Repeat("7", 64),
		SchemaVersion: effectiveArgvSchemaVersion, SemanticProjection: projection, SemanticProjectionSHA256: projectionSHA, SourceCommandArgvSHA256: commandSHA,
	}
}

func formalToolDefinitionsFixture(catalog harness.ToolCatalogSpec) ([]formalToolDefinition, int64) {
	definitions := make([]formalToolDefinition, 0, len(catalog.Tools))
	var total int64
	for index, tool := range catalog.Tools {
		bytes := int64(index + 1)
		definitions = append(definitions, formalToolDefinition{Type: tool.Type, Name: tool.Name, DefinitionSHA256: tool.DefinitionSHA256, DefinitionBytes: bytes})
		total += bytes
	}
	return definitions, total
}

func formalCacheWireFixture(policy formalCachePolicy) formalCacheWireReceipt {
	return formalCacheWireReceipt{
		SchemaVersion: "agentic-bench/content-free-cache-wire-v1", ObservedRequests: 2, ShapeValidRequests: 2,
		KeyPresentRequests: 2, UniqueKeyCount: 1, FirstKeySHA256: policy.PromptCacheKeySHA256, Stable: true,
		PromptCacheOptionsModes:      []string{policy.PromptCacheOptionsMode, policy.PromptCacheOptionsMode},
		PromptCacheOptionsTTLs:       []string{policy.PromptCacheOptionsTTL, policy.PromptCacheOptionsTTL},
		PromptCacheOptionsTTLSeconds: []*int{policy.PromptCacheOptionsTTLSeconds, policy.PromptCacheOptionsTTLSeconds},
		PromptCacheRetentions:        []string{"", ""}, BreakpointCounts: []int{1, 1},
		BreakpointPositionHashes: [][]string{slices.Clone(policy.PromptCacheBreakpointPositionHashes), slices.Clone(policy.PromptCacheBreakpointPositionHashes)},
	}
}

func stringPointer(value string) *string { return &value }
