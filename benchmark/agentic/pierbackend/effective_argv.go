package pierbackend

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

const (
	effectiveArgvSchemaVersion  = "agentic-bench/effective-argv-v2"
	pinnedAdapterRelativePath   = "benchmark/agentic/pier/pinned_agent.py"
	privateProviderBaseURLToken = "{provider_base_url}"
	remoteLubanBinary           = "/opt/agentic-bench/agent"
	remoteVendorRoot            = "/opt/agentic-bench/vendor"
	lubanDisallowedTools        = "WebSearch,WebFetch,Agent,Skill,TeamCreate,SendMessage"
	codexHTTPProviderSelection  = `model_provider="agentic_http"`
	codexHTTPProviderConfig     = `model_providers.agentic_http={name="OpenAI",base_url="{provider_base_url}",wire_api="responses",requires_openai_auth=true,supports_websockets=false}`
)

type adapterBinding struct {
	Path   string
	SHA256 string
}

// Field order is alphabetical by JSON name. json.Marshal therefore produces
// the same compact, sorted-key representation emitted by pinned_agent.py.
type effectiveArgvSemanticProjection struct {
	AgentsEnabled                 bool   `json:"agents_enabled"`
	API                           string `json:"api"`
	ApprovalPolicy                string `json:"approval_policy"`
	InstructionTransport          string `json:"instruction_transport"`
	Model                         string `json:"model"`
	ModelFallback                 bool   `json:"model_fallback"`
	Provider                      string `json:"provider"`
	ProviderEndpoint              string `json:"provider_endpoint"`
	ProviderTransport             string `json:"provider_transport"`
	ReasoningEffort               string `json:"reasoning_effort"`
	ResponseStorage               bool   `json:"response_storage"`
	ResponsesAPIProfile           string `json:"responses_api_profile"`
	ResponsesLite                 bool   `json:"responses_lite"`
	ResponsesTransportRequirement string `json:"responses_transport_requirement"`
	SandboxPolicy                 string `json:"sandbox_policy"`
	ServiceTier                   string `json:"service_tier"`
	UserConfig                    string `json:"user_config"`
	WebSearch                     bool   `json:"web_search"`
}

// Field order is alphabetical by JSON name; see the projection above.
type effectiveArgvReceipt struct {
	AdapterSHA256             string                          `json:"adapter_sha256"`
	AdapterVersion            string                          `json:"adapter_version"`
	AgentKind                 string                          `json:"agent_kind"`
	BundleManifestSHA256      string                          `json:"bundle_manifest_sha256"`
	BundleTreeSHA256          string                          `json:"bundle_tree_sha256"`
	EffectiveArgv             []string                        `json:"effective_argv"`
	EffectiveArgvSHA256       string                          `json:"effective_argv_sha256"`
	ExecutionArgvSHA256       string                          `json:"execution_argv_sha256"`
	PrivateProxyBaseURLSHA256 string                          `json:"private_proxy_base_url_sha256"`
	SchemaVersion             string                          `json:"schema_version"`
	SemanticProjection        effectiveArgvSemanticProjection `json:"semantic_projection"`
	SemanticProjectionSHA256  string                          `json:"semantic_projection_sha256"`
	SourceCommandArgvSHA256   string                          `json:"source_command_argv_sha256"`
}

func resolvePinnedAdapterBinding(config Config) (adapterBinding, error) {
	path := filepath.Join(config.PythonModuleRoot, filepath.FromSlash(pinnedAdapterRelativePath))
	info, err := os.Lstat(path)
	if err != nil {
		return adapterBinding{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return adapterBinding{}, errors.New("Pier pinned adapter must be a regular non-symlink file")
	}
	digest, err := harness.HashFile(path)
	if err != nil {
		return adapterBinding{}, err
	}
	return adapterBinding{Path: path, SHA256: digest}, nil
}

func (backend *Backend) pinnedAdapterSnapshot() (adapterBinding, error) {
	backend.mu.RLock()
	expected, ready := backend.adapter, backend.ready
	backend.mu.RUnlock()
	if !ready || expected.Path == "" || !lowerHexSHA256(expected.SHA256) {
		return adapterBinding{}, errors.New("Pier adapter requested before verified preflight")
	}
	actual, err := resolvePinnedAdapterBinding(backend.config)
	if err != nil {
		return adapterBinding{}, err
	}
	if actual != expected {
		return adapterBinding{}, errors.New("pinned_agent.py changed after benchmark preflight")
	}
	return actual, nil
}

func formalSourceArgvTail(agentKind string) ([]string, error) {
	switch agentKind {
	case "codex":
		return []string{
			"--ask-for-approval", "never", "--sandbox", "workspace-write",
			"exec", "--json", "--ephemeral", "--ignore-user-config",
			"--model", "gpt-5.6-sol", "--config", "model_reasoning_effort=xhigh",
			"--config", `service_tier="default"`,
			"--config", `web_search="disabled"`,
			"--config", "agents.enabled=false",
			"--config", codexHTTPProviderSelection,
			"--config", codexHTTPProviderConfig,
			"{instruction_path}",
		}, nil
	case "luban":
		return []string{
			"--print", "--output-format", "stream-json", "--provider", "openai",
			"--api", "responses", "--model", "gpt-5.6-sol", "--reasoning-effort", "xhigh",
			"--service-tier", "default",
			"--pinned-model", "--no-model-fallback", "--allow-all", "--force-sandbox-tools",
			"{instruction_path}",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported formal agent ID %s", agentKind)
	}
}

func validateFormalSourceCommand(agent harness.AgentSpec) error {
	tail, err := formalSourceArgvTail(agent.ID)
	if err != nil {
		return err
	}
	if len(agent.Command.Argv) != len(tail)+1 || agent.Command.Argv[0] != agent.Binary || !slices.Equal(agent.Command.Argv[1:], tail) {
		return fmt.Errorf("agent %s command differs from the exact frozen source argv", agent.ID)
	}
	return validateContentSafeArgv(agent.Command.Argv)
}

func expectedEffectiveArgv(agent harness.AgentSpec, proxyBaseURL string, contentSafe bool) ([]string, error) {
	if err := validateFormalSourceCommand(agent); err != nil {
		return nil, err
	}
	argv := append([]string(nil), agent.Command.Argv...)
	switch agent.ID {
	case "codex":
		argv[0] = remoteVendorRoot + "/" + CodexBinaryRelativePath
		argv[len(argv)-1] = "-"
		providerConfigIndex := slices.Index(argv, codexHTTPProviderConfig)
		if providerConfigIndex < 0 || strings.Count(argv[providerConfigIndex], privateProviderBaseURLToken) != 1 {
			return nil, errors.New("frozen Codex command lacks one private provider base URL token")
		}
		if !contentSafe {
			argv[providerConfigIndex] = strings.Replace(argv[providerConfigIndex], privateProviderBaseURLToken, proxyBaseURL, 1)
		}
		execIndex := slices.Index(argv, "exec")
		if execIndex < 0 {
			return nil, errors.New("frozen Codex command lacks exec")
		}
		argv = slices.Insert(argv, execIndex+1, "--cd", "/app")
	case "luban":
		argv[0] = remoteLubanBinary
		argv = argv[:len(argv)-1]
		argv = slices.Insert(argv, 1, "--disallowed-tools", lubanDisallowedTools)
	default:
		return nil, fmt.Errorf("unsupported formal agent ID %s", agent.ID)
	}
	if err := validateContentSafeArgv(argv); err != nil {
		return nil, err
	}
	return argv, nil
}

func projectEffectiveArgv(agentKind string, argv []string) (effectiveArgvSemanticProjection, error) {
	oneValue := func(flag string) (string, error) {
		index := -1
		for candidate, value := range argv {
			if value != flag {
				continue
			}
			if index >= 0 || candidate+1 >= len(argv) {
				return "", fmt.Errorf("effective argv must contain exactly one %s", flag)
			}
			index = candidate
		}
		if index < 0 {
			return "", fmt.Errorf("effective argv lacks %s", flag)
		}
		return argv[index+1], nil
	}
	if slices.Contains(argv, "--search") {
		return effectiveArgvSemanticProjection{}, errors.New("effective argv enabled web search")
	}
	projection := effectiveArgvSemanticProjection{
		API: "responses", InstructionTransport: "stdin", Provider: "openai", ResponseStorage: false,
	}
	switch agentKind {
	case "codex":
		approval, err := oneValue("--ask-for-approval")
		if err != nil {
			return effectiveArgvSemanticProjection{}, err
		}
		model, err := oneValue("--model")
		if err != nil {
			return effectiveArgvSemanticProjection{}, err
		}
		sandbox, err := oneValue("--sandbox")
		if err != nil {
			return effectiveArgvSemanticProjection{}, err
		}
		configs := make([]string, 0, 7)
		for index, value := range argv[:len(argv)-1] {
			if value == "--config" {
				configs = append(configs, argv[index+1])
			}
		}
		if countString(configs, "model_reasoning_effort=xhigh") != 1 || countString(configs, `service_tier="default"`) != 1 ||
			countString(configs, `web_search="disabled"`) != 1 || countString(configs, "agents.enabled=false") != 1 ||
			countString(configs, codexHTTPProviderSelection) != 1 ||
			countCodexHTTPProviderConfigs(configs) != 1 {
			return effectiveArgvSemanticProjection{}, errors.New("Codex effective config drifted in reasoning, service tier, search, agents, or HTTP provider")
		}
		projection.AgentsEnabled = false
		projection.ApprovalPolicy = approval
		projection.Model = model
		projection.ModelFallback = false
		projection.ProviderEndpoint = "private-proxy"
		projection.ProviderTransport = "responses-http"
		projection.ReasoningEffort = "xhigh"
		projection.ResponsesAPIProfile = "codex_lite"
		projection.ResponsesLite = true
		projection.ResponsesTransportRequirement = harness.TransportRequirementHTTPInference
		projection.SandboxPolicy = sandbox
		projection.ServiceTier = "default"
		projection.UserConfig = "ignored"
		projection.WebSearch = false
	case "luban":
		model, err := oneValue("--model")
		if err != nil {
			return effectiveArgvSemanticProjection{}, err
		}
		effort, err := oneValue("--reasoning-effort")
		if err != nil {
			return effectiveArgvSemanticProjection{}, err
		}
		serviceTier, err := oneValue("--service-tier")
		if err != nil {
			return effectiveArgvSemanticProjection{}, err
		}
		if serviceTier != harness.FormalServiceTier {
			return effectiveArgvSemanticProjection{}, errors.New("Luban effective argv service tier is not default")
		}
		disallowed, err := oneValue("--disallowed-tools")
		if err != nil {
			return effectiveArgvSemanticProjection{}, err
		}
		disabled := strings.Split(disallowed, ",")
		for _, required := range []string{"WebSearch", "WebFetch", "Agent", "TeamCreate", "SendMessage"} {
			if !slices.Contains(disabled, required) {
				return effectiveArgvSemanticProjection{}, errors.New("Luban effective argv does not disable web or agent tools")
			}
		}
		projection.AgentsEnabled = false
		projection.ApprovalPolicy = "prompt"
		if slices.Contains(argv, "--allow-all") {
			projection.ApprovalPolicy = "never"
		}
		projection.Model = model
		projection.ModelFallback = !(slices.Contains(argv, "--pinned-model") && slices.Contains(argv, "--no-model-fallback"))
		projection.ProviderEndpoint = "private-proxy-env"
		projection.ProviderTransport = "responses-http"
		projection.ReasoningEffort = effort
		projection.ResponsesAPIProfile = "openai_public"
		projection.ResponsesLite = false
		projection.ResponsesTransportRequirement = harness.TransportRequirementHTTPInference
		projection.SandboxPolicy = "unforced"
		if slices.Contains(argv, "--force-sandbox-tools") {
			projection.SandboxPolicy = "forced-tools"
		}
		projection.ServiceTier = serviceTier
		projection.UserConfig = "empty-home"
		projection.WebSearch = false
	default:
		return effectiveArgvSemanticProjection{}, fmt.Errorf("unsupported formal agent ID %s", agentKind)
	}
	return projection, nil
}

func countString(values []string, expected string) int {
	count := 0
	for _, value := range values {
		if value == expected {
			count++
		}
	}
	return count
}

func countCodexHTTPProviderConfigs(configs []string) int {
	count := 0
	prefix, suffix, found := strings.Cut(codexHTTPProviderConfig, privateProviderBaseURLToken)
	if !found {
		return 0
	}
	for _, config := range configs {
		if strings.HasPrefix(config, prefix) && strings.HasSuffix(config, suffix) && len(config) > len(prefix)+len(suffix) {
			count++
		}
	}
	return count
}

func readEffectiveArgvReceipt(path string, invocation harness.AgentInvocation, adapter adapterBinding, bundle codexBundleBinding, proxyBaseURL string) (effectiveArgvReceipt, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return effectiveArgvReceipt{}, "", err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Contains(trimmed, []byte(proxyBaseURL)) {
		return effectiveArgvReceipt{}, "", errors.New("effective argv receipt is empty or leaks private proxy authority")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var receipt effectiveArgvReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return effectiveArgvReceipt{}, "", fmt.Errorf("decode effective argv receipt: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return effectiveArgvReceipt{}, "", errors.New("effective argv receipt contains trailing JSON")
	}
	canonical, err := json.Marshal(receipt)
	if err != nil {
		return effectiveArgvReceipt{}, "", err
	}
	if !bytes.Equal(trimmed, canonical) {
		return effectiveArgvReceipt{}, "", errors.New("effective argv receipt is not canonical compact JSON")
	}
	receiptSum := sha256.Sum256(canonical)
	receiptSHA := hex.EncodeToString(receiptSum[:])

	commandSHA, err := harness.HashCanonical(invocation.Agent.Command.Argv)
	if err != nil {
		return effectiveArgvReceipt{}, "", err
	}
	safeArgv, err := expectedEffectiveArgv(invocation.Agent, proxyBaseURL, true)
	if err != nil {
		return effectiveArgvReceipt{}, "", err
	}
	executionArgv, err := expectedEffectiveArgv(invocation.Agent, proxyBaseURL, false)
	if err != nil {
		return effectiveArgvReceipt{}, "", err
	}
	projection, err := projectEffectiveArgv(invocation.Agent.ID, executionArgv)
	if err != nil {
		return effectiveArgvReceipt{}, "", err
	}
	projectionSHA, err := harness.HashCanonical(projection)
	if err != nil {
		return effectiveArgvReceipt{}, "", err
	}
	safeSHA, err := harness.HashCanonical(safeArgv)
	if err != nil {
		return effectiveArgvReceipt{}, "", err
	}
	executionSHA, err := harness.HashCanonical(executionArgv)
	if err != nil {
		return effectiveArgvReceipt{}, "", err
	}
	proxySum := sha256.Sum256([]byte(proxyBaseURL))
	proxySHA := hex.EncodeToString(proxySum[:])
	if receipt.SchemaVersion != effectiveArgvSchemaVersion ||
		receipt.AgentKind != invocation.Agent.ID || receipt.AdapterVersion != PinnedAdapterVersion ||
		receipt.AdapterSHA256 != adapter.SHA256 || receipt.SourceCommandArgvSHA256 != commandSHA ||
		receipt.BundleManifestSHA256 != bundle.ManifestSHA256 || receipt.BundleTreeSHA256 != bundle.TreeSHA256 ||
		receipt.PrivateProxyBaseURLSHA256 != proxySHA || !slices.Equal(receipt.EffectiveArgv, safeArgv) ||
		receipt.EffectiveArgvSHA256 != safeSHA || receipt.ExecutionArgvSHA256 != executionSHA ||
		receipt.SemanticProjection != projection || receipt.SemanticProjectionSHA256 != projectionSHA {
		return effectiveArgvReceipt{}, "", errors.New("effective argv receipt differs from the frozen adapter, manifest, proxy binding, or semantic policy")
	}
	return receipt, receiptSHA, nil
}

func validateContentSafeArgv(argv []string) error {
	if len(argv) == 0 || len(argv) > 64 {
		return errors.New("effective argv must be a non-empty bounded JSON array")
	}
	for _, value := range argv {
		if value == "" || len(value) > 4096 {
			return errors.New("effective argv contains an empty or oversized argument")
		}
		for _, character := range []byte(value) {
			if character < 0x20 || character > 0x7e {
				return errors.New("effective argv must contain printable ASCII only")
			}
		}
	}
	return nil
}
