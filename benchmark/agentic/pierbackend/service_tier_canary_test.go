package pierbackend

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestValidateCodexWebSearchCanaryBindsServiceTierAndMultiAgentNamespace(t *testing.T) {
	model := harness.ModelRequestSpec{
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: harness.FormalServiceTier,
		ServiceTierRequestEncoding: harness.ServiceTierEncodingClientCanonical,
	}
	fixture := codexConfigurationCanaryFixture(t)
	if err := validateCodexWebSearchCanary(mustCanaryJSON(t, fixture), model); err != nil {
		t.Fatalf("valid Codex configuration canary: %v", err)
	}

	t.Run("default wire encoding", func(t *testing.T) {
		mutated := fixture
		mutated.ServiceTierDefaultWireEncoding = "explicit"
		if err := validateCodexWebSearchCanary(mustCanaryJSON(t, mutated), model); err == nil {
			t.Fatal("explicit Codex default wire encoding was accepted")
		}
	})

	t.Run("service tier counterfactual", func(t *testing.T) {
		mutated := fixture
		var control serviceTierNegativeControl
		if err := json.Unmarshal(mutated.ServiceTierNegativeControl, &control); err != nil {
			t.Fatal(err)
		}
		control.OnlyReplacedConfigValue = false
		mutated.ServiceTierNegativeControl = mustCanaryJSON(t, control)
		if err := validateCodexWebSearchCanary(mustCanaryJSON(t, mutated), model); err == nil {
			t.Fatal("multi-value service-tier counterfactual was accepted")
		}
	})

	t.Run("multi agent namespace", func(t *testing.T) {
		mutated := fixture
		var control webSearchNegativeControl
		if err := json.Unmarshal(mutated.AgentsNegativeControl, &control); err != nil {
			t.Fatal(err)
		}
		var request codexStandardCanaryRequest
		if err := json.Unmarshal(control.Request, &request); err != nil {
			t.Fatal(err)
		}
		request.MultiAgentNamespacePresent = false
		control.Request = mustCanaryJSON(t, request)
		mutated.AgentsNegativeControl = mustCanaryJSON(t, control)
		if err := validateCodexWebSearchCanary(mustCanaryJSON(t, mutated), model); err == nil {
			t.Fatal("unbound multi-agent namespace evidence was accepted")
		}
	})

	t.Run("priority wire request", func(t *testing.T) {
		mutated := fixture
		var control serviceTierNegativeControl
		if err := json.Unmarshal(mutated.ServiceTierNegativeControl, &control); err != nil {
			t.Fatal(err)
		}
		var request codexStandardCanaryRequest
		if err := json.Unmarshal(control.Request, &request); err != nil {
			t.Fatal(err)
		}
		request.RequestServiceTierPresent = false
		control.Request = mustCanaryJSON(t, request)
		mutated.ServiceTierNegativeControl = mustCanaryJSON(t, control)
		if err := validateCodexWebSearchCanary(mustCanaryJSON(t, mutated), model); err == nil {
			t.Fatal("priority request without explicit wire presence was accepted")
		}
	})
}

func codexConfigurationCanaryFixture(t *testing.T) webSearchConfigurationCanary {
	t.Helper()
	store := false
	name := func(value string) *string { return &value }
	request := func(index int) codexStandardCanaryRequest {
		return codexStandardCanaryRequest{
			RequestIndex: index, Model: "gpt-5.6-sol", Store: &store, ReasoningEffort: "xhigh",
			IncludeEncryptedReasoning: true, Stream: true, AuthorizationHeaderPresent: true, Originator: "codex_exec",
			RequestServiceTierCanonical: "default", RequestServiceTierSource: "client_canonicalized_default",
			ResponseModel: "gpt-5.6-sol", ResponseServiceTier: "default", ResponseServiceTierCanonical: "default", ResponseRequestIDPresent: true,
			ResponseUsage: canaryUsage{InputTokens: 7, CachedInputTokens: 2, CacheWriteInputTokens: 1, OutputTokens: 3, ReasoningOutputTokens: 1},
		}
	}
	baseline := []canaryTool{
		{Type: "function", Name: name("exec_command")},
		{Type: "function", Name: name("write_stdin")},
		{Type: "function", Name: name("update_plan")},
		{Type: "function", Name: name("request_user_input")},
		{Type: "function", Name: name("view_image")},
	}
	positiveRequest := request(0)
	positiveRequest.OrderedToolCatalog = append([]canaryTool(nil), baseline...)
	webRequest := request(1)
	webRequest.OrderedToolCatalog = append(append([]canaryTool(nil), baseline...), canaryTool{Type: "web_search"})
	webRequest.WebSearchToolCount = 1
	webRequest.WebSearchExternalAccess = []bool{false}
	agentsRequest := request(2)
	agentsRequest.OrderedToolCatalog = append(append([]canaryTool(nil), baseline...), canaryTool{Type: "namespace", Name: name("multi_agent_v1")})
	agentsRequest.MultiAgentNamespacePresent = true
	agentsRequest.SubagentToolPresent = true
	serviceRequest := request(3)
	serviceRequest.OrderedToolCatalog = append([]canaryTool(nil), baseline...)
	serviceRequest.RequestServiceTierPresent = true
	serviceRequest.RequestServiceTier = name("priority")
	serviceRequest.RequestServiceTierCanonical = "priority"
	serviceRequest.RequestServiceTierSource = "wire_explicit"

	positiveRequest.ConfigurationAccepted = true
	return webSearchConfigurationCanary{
		SchemaVersion: "agentic-bench/fairness-configuration-canary-v2", ProviderTransport: "responses-http-sse-standard-diagnostic",
		Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", EffectiveConfig: `web_search="disabled"`, AgentsEffectiveConfig: "agents.enabled=false",
		ServiceTierEffectiveConfig: `service_tier="default"`, ServiceTierDefaultWireEncoding: "omitted", ServiceTierDefaultSource: "client_canonicalized_default",
		ModelCatalogSHA256: strings.Repeat("5", 64),
		Positive: mustCanaryJSON(t, webSearchPositiveControl{
			EffectiveArgvSHA256: strings.Repeat("1", 64), ExpectedCLIExitCode: 0, ActualCLIExitCode: 0, ValidReceiptEmitted: true, Request: mustCanaryJSON(t, positiveRequest),
		}),
		NegativeControl: mustCanaryJSON(t, webSearchNegativeControl{
			ConfigRemoved: `web_search="disabled"`, OnlyRemovedConfig: true, EffectiveArgvSHA256: strings.Repeat("2", 64), CounterfactualArgvSHA256: strings.Repeat("2", 64),
			ExpectedCLIExitCode: "nonzero", ActualCLIExitCode: 1, Request: mustCanaryJSON(t, webRequest),
		}),
		AgentsNegativeControl: mustCanaryJSON(t, webSearchNegativeControl{
			ConfigRemoved: "agents.enabled=false", OnlyRemovedConfig: true, EffectiveArgvSHA256: strings.Repeat("3", 64), CounterfactualArgvSHA256: strings.Repeat("3", 64),
			ExpectedCLIExitCode: "nonzero", ActualCLIExitCode: 1, Request: mustCanaryJSON(t, agentsRequest),
		}),
		ServiceTierNegativeControl: mustCanaryJSON(t, serviceTierNegativeControl{
			ConfigReplaced: `service_tier="default"`, ReplacementConfig: `service_tier="priority"`, OnlyReplacedConfigValue: true,
			EffectiveArgvSHA256: strings.Repeat("4", 64), CounterfactualArgvSHA256: strings.Repeat("4", 64), ExpectedCLIExitCode: "nonzero", ActualCLIExitCode: 1,
			Request: mustCanaryJSON(t, serviceRequest),
		}),
	}
}

func mustCanaryJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
