package pierbackend

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestFrozenCodexV7CanaryCoreMatchesRuntimeValidator(t *testing.T) {
	raw, err := os.ReadFile("../pier/codex-exec-v7-multi-agent-wire.receipt.json")
	if err != nil {
		t.Fatalf("read frozen Codex v7 canary: %v", err)
	}
	const frozenReceiptSHA256 = "1d420772136459b44666ea5744376e0dcb34b61bed84be6851dfef8458af9649"
	if got := sha256Hex(raw); got != frozenReceiptSHA256 {
		t.Fatalf("frozen Codex v7 canary SHA-256 = %s, want %s", got, frozenReceiptSHA256)
	}

	var receipt map[string]any
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatalf("decode frozen Codex v7 canary: %v", err)
	}
	effective := canaryFixtureObject(t, receipt, "effective_argv_receipt")
	projection := canaryFixtureObject(t, effective, "semantic_projection")

	invocation := harness.AgentInvocation{
		Agent: harness.AgentSpec{
			ID:           canaryFixtureString(t, receipt, "agent_kind"),
			BinarySHA256: canaryFixtureString(t, receipt, "binary_sha256"),
			Model: harness.ModelRequestSpec{
				Provider:                   canaryFixtureString(t, projection, "provider"),
				Model:                      canaryFixtureString(t, projection, "model"),
				ReasoningEffort:            canaryFixtureString(t, projection, "reasoning_effort"),
				ServiceTier:                canaryFixtureString(t, projection, "service_tier"),
				ServiceTierRequestEncoding: harness.ServiceTierEncodingClientCanonical,
			},
		},
		Task: harness.PublicTaskView{BaseCommit: canaryFixtureString(t, receipt, "base_commit")},
	}
	adapter := adapterBinding{SHA256: canaryFixtureString(t, receipt, "adapter_sha256")}
	bundle := codexBundleBinding{
		ManifestSHA256: canaryFixtureString(t, receipt, "bundle_manifest_sha256"),
		TreeSHA256:     canaryFixtureString(t, receipt, "source_bundle_tree_sha256"),
	}
	effectiveReceiptSHA := canaryFixtureString(t, receipt, "effective_argv_receipt_sha256")

	delete(receipt, "effective_argv_receipt")
	delete(receipt, "egress_proxy_image")
	delete(receipt, "overlay")
	core := marshalSandboxCanaryFixture(t, receipt)
	if err := validateSandboxCanaryReceipt(core, invocation, adapter, bundle, effectiveReceiptSHA); err != nil {
		t.Fatalf("validate frozen Codex v7 canary core: %v", err)
	}
}

func TestValidateSandboxCanaryReceiptRejectsCounterfeitEvidence(t *testing.T) {
	fixture, invocation, adapter, bundle, effectiveReceiptSHA := validCodexSandboxCanaryFixture()

	t.Run("valid_v3_receipt", func(t *testing.T) {
		raw := marshalSandboxCanaryFixture(t, fixture)
		if err := validateSandboxCanaryReceipt(raw, invocation, adapter, bundle, effectiveReceiptSHA); err != nil {
			t.Fatalf("validate known-good sandbox canary: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(t *testing.T, receipt map[string]any)
	}{
		{
			name: "old_top_level_schema",
			mutate: func(_ *testing.T, receipt map[string]any) {
				receipt["schema_version"] = "agentic-bench/sandbox-canary-v2"
			},
		},
		{
			name: "old_fairness_schema",
			mutate: func(t *testing.T, receipt map[string]any) {
				canaryFixtureObject(t, receipt, "web_search_configuration_canary")["schema_version"] = "agentic-bench/fairness-configuration-canary-v1"
			},
		},
		{
			name: "old_three_control_fairness_shape",
			mutate: func(t *testing.T, receipt map[string]any) {
				fairness := canaryFixtureObject(t, receipt, "web_search_configuration_canary")
				delete(fairness, "service_tier_effective_config")
				delete(fairness, "service_tier_default_wire_encoding")
				delete(fairness, "service_tier_default_source")
				delete(fairness, "service_tier_negative_control")
			},
		},
		{
			name: "missing_request_service_tier_canonical",
			mutate: func(t *testing.T, receipt map[string]any) {
				delete(canaryFixtureObject(t, receipt, "web_search_configuration_canary", "positive", "request"), "request_service_tier_canonical")
			},
		},
		{
			name: "missing_request_service_tier_source",
			mutate: func(t *testing.T, receipt map[string]any) {
				delete(canaryFixtureObject(t, receipt, "web_search_configuration_canary", "positive", "request"), "request_service_tier_source")
			},
		},
		{
			name: "omitted_default_has_raw_auto_tier",
			mutate: func(t *testing.T, receipt map[string]any) {
				request := canaryFixtureObject(t, receipt, "web_search_configuration_canary", "positive", "request")
				request["request_service_tier"] = "auto"
			},
		},
		{
			name: "omitted_default_has_empty_string_tier",
			mutate: func(t *testing.T, receipt map[string]any) {
				request := canaryFixtureObject(t, receipt, "web_search_configuration_canary", "positive", "request")
				request["request_service_tier"] = ""
			},
		},
		{
			name: "explicit_priority_claims_omitted_wire",
			mutate: func(t *testing.T, receipt map[string]any) {
				request := canaryFixtureObject(t, receipt, "web_search_configuration_canary", "service_tier_negative_control", "request")
				request["request_service_tier_present"] = false
			},
		},
		{
			name: "response_service_tier_drift",
			mutate: func(t *testing.T, receipt map[string]any) {
				request := canaryFixtureObject(t, receipt, "web_search_configuration_canary", "positive", "request")
				request["response_service_tier"] = "priority"
			},
		},
		{
			name: "response_service_tier_canonical_drift",
			mutate: func(t *testing.T, receipt map[string]any) {
				request := canaryFixtureObject(t, receipt, "web_search_configuration_canary", "positive", "request")
				request["response_service_tier_canonical"] = "priority"
			},
		},
		{
			name: "positive_control_nonzero_exit",
			mutate: func(t *testing.T, receipt map[string]any) {
				canaryFixtureObject(t, receipt, "web_search_configuration_canary", "positive")["actual_cli_exit_code"] = 1
			},
		},
		{
			name: "negative_control_zero_exit",
			mutate: func(t *testing.T, receipt map[string]any) {
				canaryFixtureObject(t, receipt, "web_search_configuration_canary", "negative_control")["actual_cli_exit_code"] = 0
			},
		},
		{
			name: "service_tier_priority_control_falsely_succeeds",
			mutate: func(t *testing.T, receipt map[string]any) {
				control := canaryFixtureObject(t, receipt, "web_search_configuration_canary", "service_tier_negative_control")
				control["actual_cli_exit_code"] = 0
				control["valid_receipt_emitted"] = true
			},
		},
		{
			name: "service_tier_priority_control_replaces_more_than_value",
			mutate: func(t *testing.T, receipt map[string]any) {
				canaryFixtureObject(t, receipt, "web_search_configuration_canary", "service_tier_negative_control")["only_replaced_config_value"] = false
			},
		},
		{
			name: "exec_wait_tool_types_swapped",
			mutate: func(t *testing.T, receipt map[string]any) {
				request := canaryFixtureArray(t, receipt, "provider_canary_requests")[0].(map[string]any)
				tools := canaryFixtureArray(t, request, "tool_catalog")
				tools[0].(map[string]any)["type"] = "function"
				tools[1].(map[string]any)["type"] = "custom"
			},
		},
		{
			name: "tool_catalog_item_hides_extra_field",
			mutate: func(t *testing.T, receipt map[string]any) {
				request := canaryFixtureArray(t, receipt, "provider_canary_requests")[0].(map[string]any)
				tools := canaryFixtureArray(t, request, "tool_catalog")
				tools[0].(map[string]any)["unsealed"] = true
			},
		},
		{
			name: "response_usage_missing_field",
			mutate: func(t *testing.T, receipt map[string]any) {
				request := canaryFixtureObject(t, receipt, "web_search_configuration_canary", "positive", "request")
				delete(canaryFixtureObject(t, request, "response_usage"), "cached_input_tokens")
			},
		},
		{
			name: "response_usage_hides_extra_field",
			mutate: func(t *testing.T, receipt map[string]any) {
				request := canaryFixtureObject(t, receipt, "web_search_configuration_canary", "positive", "request")
				canaryFixtureObject(t, request, "response_usage")["unsealed_tokens"] = 1
			},
		},
		{
			name: "multi_agent_namespace_claim_without_catalog_content",
			mutate: func(t *testing.T, receipt map[string]any) {
				request := canaryFixtureObject(t, receipt, "web_search_configuration_canary", "agents_negative_control", "request")
				request["ordered_tool_catalog"] = []any{}
			},
		},
		{
			name: "multi_agent_namespace_content_without_claim",
			mutate: func(t *testing.T, receipt map[string]any) {
				request := canaryFixtureObject(t, receipt, "web_search_configuration_canary", "agents_negative_control", "request")
				request["multi_agent_namespace_present"] = false
			},
		},
		{
			name: "positive_sandbox_tool_exit_drift",
			mutate: func(t *testing.T, receipt map[string]any) {
				requests := canaryFixtureArray(t, receipt, "provider_canary_requests")
				requests[1].(map[string]any)["tool_output_exit_code"] = 91
			},
		},
		{
			name: "negative_sandbox_expected_tool_exit_drift",
			mutate: func(t *testing.T, receipt map[string]any) {
				canaryFixtureObject(t, receipt, "sandbox_negative_control")["expected_tool_exit_code"] = 90
			},
		},
		{
			name: "negative_sandbox_observed_tool_exit_drift",
			mutate: func(t *testing.T, receipt map[string]any) {
				negative := canaryFixtureObject(t, receipt, "sandbox_negative_control")
				requests := canaryFixtureArray(t, negative, "provider_canary_requests")
				requests[1].(map[string]any)["tool_output_exit_code"] = 0
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			counterfeit := cloneSandboxCanaryFixture(t, fixture)
			test.mutate(t, counterfeit)
			raw := marshalSandboxCanaryFixture(t, counterfeit)
			if err := validateSandboxCanaryReceipt(raw, invocation, adapter, bundle, effectiveReceiptSHA); err == nil {
				t.Fatal("counterfeit sandbox canary was accepted")
			}
		})
	}
}

func validCodexSandboxCanaryFixture() (map[string]any, harness.AgentInvocation, adapterBinding, codexBundleBinding, string) {
	baseCommit := strings.Repeat("1", 40)
	binarySHA := strings.Repeat("2", 64)
	adapterSHA := strings.Repeat("3", 64)
	bundleManifestSHA := strings.Repeat("4", 64)
	effectiveReceiptSHA := strings.Repeat("5", 64)
	modelCatalogSHA := strings.Repeat("6", 64)
	positiveArgvSHA := strings.Repeat("7", 64)
	webNegativeArgvSHA := strings.Repeat("8", 64)
	agentsNegativeArgvSHA := strings.Repeat("9", 64)
	serviceTierNegativeArgvSHA := strings.Repeat("a", 64)
	indexEntriesSHA := strings.Repeat("b", 64)

	model := harness.ModelRequestSpec{
		Provider:                   "openai",
		Model:                      "gpt-5.6-sol",
		ReasoningEffort:            "xhigh",
		ServiceTier:                "default",
		ServiceTierRequestEncoding: harness.ServiceTierEncodingClientCanonical,
	}
	invocation := harness.AgentInvocation{
		Agent: harness.AgentSpec{
			ID:           "codex",
			BinarySHA256: binarySHA,
			Model:        model,
		},
		Task: harness.PublicTaskView{BaseCommit: baseCommit},
	}
	adapter := adapterBinding{SHA256: adapterSHA}
	bundle := codexBundleBinding{ManifestSHA256: bundleManifestSHA, TreeSHA256: CodexBundleTreeSHA256}

	positiveLiteRequests := codexLiteCanaryFixture(0)
	negativeLiteRequests := codexLiteCanaryFixture(91)
	positiveStandard := codexStandardCanaryFixture(0, codexStandardBaselineTools(), []any{}, false, false, false, nil, "default", "client_canonicalized_default", true)
	webNegativeStandard := codexStandardCanaryFixture(
		1,
		append(codexStandardBaselineTools(), map[string]any{"type": "web_search", "name": nil}),
		[]any{false},
		false,
		false,
		false,
		nil,
		"default",
		"client_canonicalized_default",
		false,
	)
	agentsNegativeStandard := codexStandardCanaryFixture(
		2,
		append(codexStandardBaselineTools(), map[string]any{"type": "namespace", "name": "multi_agent_v1"}),
		[]any{},
		false,
		false,
		true,
		nil,
		"default",
		"client_canonicalized_default",
		false,
	)
	serviceTierNegativeStandard := codexStandardCanaryFixture(
		3,
		codexStandardBaselineTools(),
		[]any{},
		true,
		false,
		false,
		"priority",
		"priority",
		"wire_explicit",
		false,
	)

	fixture := map[string]any{
		"schema_version":                "agentic-bench/sandbox-canary-v3",
		"agent_kind":                    "codex",
		"binary_sha256":                 binarySHA,
		"base_commit":                   baseCommit,
		"adapter_sha256":                adapterSHA,
		"bundle_manifest_sha256":        bundleManifestSHA,
		"effective_argv_receipt_sha256": effectiveReceiptSHA,
		"controller_proxy_reachable":    true,
		"tool_proxy_reachable":          false,
		"credential_in_agent":           false,
		"source_bundle_tree_sha256":     CodexBundleTreeSHA256,
		"runtime_payload_tree_sha256":   CodexBundleTreeSHA256,
		"provider_canary_requests":      positiveLiteRequests,
		"provider_canary_transport":     "responses-lite-websocket-426-http-sse-diagnostic",
		"websocket_fallback": map[string]any{
			"websocket_upgrade_request_count":   1,
			"websocket_upgrade_response_status": 426,
			"websocket_generation_payload_sent": false,
			"http_generation_request_count":     2,
			"expected_logical_generation_count": 2,
			"duplicate_generation_detected":     false,
			"fallback_transport":                "http-sse",
		},
		"sandbox_negative_control": map[string]any{
			"schema_version":                "agentic-bench/sandbox-negative-control-v1",
			"sandbox_policy":                "danger-full-access",
			"expected_tool_exit_code":       91,
			"marker_written":                false,
			"valid_sandbox_receipt_emitted": false,
			"provider_canary_requests":      negativeLiteRequests,
		},
		"web_search_configuration_canary": map[string]any{
			"schema_version":                     "agentic-bench/fairness-configuration-canary-v2",
			"provider_transport":                 "responses-http-sse-standard-diagnostic",
			"model":                              model.Model,
			"reasoning_effort":                   model.ReasoningEffort,
			"effective_config":                   `web_search="disabled"`,
			"agents_effective_config":            "agents.enabled=false",
			"service_tier_effective_config":      `service_tier="default"`,
			"service_tier_default_wire_encoding": "omitted",
			"service_tier_default_source":        "client_canonicalized_default",
			"model_catalog_sha256":               modelCatalogSHA,
			"positive": map[string]any{
				"effective_argv_sha256":  positiveArgvSHA,
				"expected_cli_exit_code": 0,
				"actual_cli_exit_code":   0,
				"valid_receipt_emitted":  true,
				"request":                positiveStandard,
			},
			"negative_control": map[string]any{
				"config_removed":             `web_search="disabled"`,
				"only_removed_config":        true,
				"effective_argv_sha256":      webNegativeArgvSHA,
				"counterfactual_argv_sha256": webNegativeArgvSHA,
				"expected_cli_exit_code":     "nonzero",
				"actual_cli_exit_code":       1,
				"valid_receipt_emitted":      false,
				"request":                    webNegativeStandard,
			},
			"agents_negative_control": map[string]any{
				"config_removed":             "agents.enabled=false",
				"only_removed_config":        true,
				"effective_argv_sha256":      agentsNegativeArgvSHA,
				"counterfactual_argv_sha256": agentsNegativeArgvSHA,
				"expected_cli_exit_code":     "nonzero",
				"actual_cli_exit_code":       1,
				"valid_receipt_emitted":      false,
				"request":                    agentsNegativeStandard,
			},
			"service_tier_negative_control": map[string]any{
				"config_replaced":            `service_tier="default"`,
				"replacement_config":         `service_tier="priority"`,
				"only_replaced_config_value": true,
				"effective_argv_sha256":      serviceTierNegativeArgvSHA,
				"counterfactual_argv_sha256": serviceTierNegativeArgvSHA,
				"expected_cli_exit_code":     "nonzero",
				"actual_cli_exit_code":       1,
				"valid_receipt_emitted":      false,
				"request":                    serviceTierNegativeStandard,
			},
		},
		"workspace_state": map[string]any{
			"schema_version":                 "agentic-bench/sandbox-workspace-state-v1",
			"head":                           baseCommit,
			"expected_base_commit":           baseCommit,
			"head_matches_base_commit":       true,
			"index_entries_sha256":           indexEntriesSHA,
			"index_matches_head":             true,
			"tracked_worktree_matches_index": true,
			"status_porcelain_v1_z_sha256":   sha256Hex(nil),
			"status_entry_count":             0,
			"positive_marker_absent":         true,
			"negative_marker_absent":         true,
		},
	}
	return fixture, invocation, adapter, bundle, effectiveReceiptSHA
}

func codexStandardBaselineTools() []any {
	return []any{
		map[string]any{"type": "function", "name": "exec_command"},
		map[string]any{"type": "function", "name": "write_stdin"},
		map[string]any{"type": "function", "name": "update_plan"},
		map[string]any{"type": "function", "name": "request_user_input"},
		map[string]any{"type": "function", "name": "view_image"},
	}
}

func codexLiteCanaryFixture(expectedToolExit int) []any {
	requests := make([]any, 0, 2)
	for index := 0; index < 2; index++ {
		var toolExit any
		customToolOutputCount := 0
		if index == 1 {
			toolExit = expectedToolExit
			customToolOutputCount = 1
		}
		requests = append(requests, map[string]any{
			"request_index":                  index,
			"model":                          "gpt-5.6-sol",
			"store":                          false,
			"reasoning_effort":               "xhigh",
			"reasoning_context":              "all_turns",
			"include_encrypted_reasoning":    true,
			"stream":                         true,
			"request_service_tier_present":   false,
			"request_service_tier":           nil,
			"request_service_tier_canonical": "default",
			"request_service_tier_source":    "client_canonicalized_default",
			"top_level_tool_count":           0,
			"tool_catalog": []any{
				map[string]any{"type": "custom", "name": "exec"},
				map[string]any{"type": "function", "name": "wait"},
				map[string]any{"type": "function", "name": "request_user_input"},
			},
			"web_search_tool_present":                false,
			"web_search_tool_count":                  0,
			"collaboration_namespace_present":        false,
			"subagent_tool_present":                  false,
			"exec_cell_wait_present":                 true,
			"websocket_upgrade_count_before_request": 1,
			"websocket_upgrade_header_present":       true,
			"websocket_key_header_present":           true,
			"responses_lite_header_present":          true,
			"authorization_header_present":           true,
			"originator":                             "codex_exec",
			"user_agent_present":                     true,
			"previous_response_id_present":           false,
			"custom_tool_output_count":               customToolOutputCount,
			"tool_output_exit_code":                  toolExit,
			"response_model":                         "gpt-5.6-sol",
			"response_service_tier":                  "default",
			"response_service_tier_canonical":        "default",
			"response_request_id_present":            true,
			"response_usage":                         canaryUsageFixture(11, 3, 2, 5, 1),
		})
	}
	return requests
}

func codexStandardCanaryFixture(
	index int,
	tools []any,
	webSearchExternalAccess []any,
	requestServiceTierPresent bool,
	collaborationNamespacePresent bool,
	multiAgentNamespacePresent bool,
	requestServiceTier any,
	requestServiceTierCanonical string,
	requestServiceTierSource string,
	configurationAccepted bool,
) map[string]any {
	webSearchCount := 0
	for _, rawTool := range tools {
		tool := rawTool.(map[string]any)
		if strings.Contains(tool["type"].(string), "web_search") {
			webSearchCount++
		}
	}
	return map[string]any{
		"request_index":                   index,
		"model":                           "gpt-5.6-sol",
		"store":                           false,
		"reasoning_effort":                "xhigh",
		"include_encrypted_reasoning":     true,
		"stream":                          true,
		"responses_lite_header_present":   false,
		"authorization_header_present":    true,
		"originator":                      "codex_exec",
		"request_service_tier_present":    requestServiceTierPresent,
		"request_service_tier":            requestServiceTier,
		"request_service_tier_canonical":  requestServiceTierCanonical,
		"request_service_tier_source":     requestServiceTierSource,
		"ordered_tool_catalog":            tools,
		"web_search_tool_count":           webSearchCount,
		"web_search_external_access":      webSearchExternalAccess,
		"collaboration_namespace_present": collaborationNamespacePresent,
		"multi_agent_namespace_present":   multiAgentNamespacePresent,
		"subagent_tool_present":           multiAgentNamespacePresent || collaborationNamespacePresent,
		"configuration_accepted":          configurationAccepted,
		"response_model":                  "gpt-5.6-sol",
		"response_service_tier":           "default",
		"response_service_tier_canonical": "default",
		"response_request_id_present":     true,
		"response_usage":                  canaryUsageFixture(7, 2, 1, 3, 1),
	}
}

func canaryUsageFixture(input, cached, cacheWrite, output, reasoning int) map[string]any {
	return map[string]any{
		"input_tokens":             input,
		"cached_input_tokens":      cached,
		"cache_write_input_tokens": cacheWrite,
		"output_tokens":            output,
		"reasoning_output_tokens":  reasoning,
	}
}

func cloneSandboxCanaryFixture(t *testing.T, fixture map[string]any) map[string]any {
	t.Helper()
	raw := marshalSandboxCanaryFixture(t, fixture)
	var clone map[string]any
	if err := json.Unmarshal(raw, &clone); err != nil {
		t.Fatalf("clone sandbox canary fixture: %v", err)
	}
	return clone
}

func marshalSandboxCanaryFixture(t *testing.T, fixture map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal sandbox canary fixture: %v", err)
	}
	return raw
}

func canaryFixtureObject(t *testing.T, root map[string]any, path ...string) map[string]any {
	t.Helper()
	current := root
	for _, field := range path {
		next, ok := current[field].(map[string]any)
		if !ok {
			t.Fatalf("sandbox canary fixture field %q is not an object", strings.Join(path, "."))
		}
		current = next
	}
	return current
}

func canaryFixtureArray(t *testing.T, root map[string]any, field string) []any {
	t.Helper()
	value, ok := root[field].([]any)
	if !ok {
		t.Fatalf("sandbox canary fixture field %q is not an array", field)
	}
	return value
}

func canaryFixtureString(t *testing.T, root map[string]any, field string) string {
	t.Helper()
	value, ok := root[field].(string)
	if !ok || value == "" {
		t.Fatalf("sandbox canary fixture field %q is not a non-empty string", field)
	}
	return value
}
