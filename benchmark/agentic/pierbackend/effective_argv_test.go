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

const effectiveArgvTestProxy = "http://host.docker.internal:43123/unguessable-run/v1"

func TestEffectiveArgvReceiptBindsManifestAdapterBundleAndPrivateProxyWithoutLeakingIt(t *testing.T) {
	for _, kind := range []string{"codex", "luban"} {
		t.Run(kind, func(t *testing.T) {
			invocation := effectiveArgvInvocation(t, kind)
			adapter := adapterBinding{Path: "/frozen/pinned_agent.py", SHA256: strings.Repeat("a", 64)}
			bundle := codexBundleBinding{ManifestSHA256: strings.Repeat("b", 64), TreeSHA256: strings.Repeat("c", 64)}
			receipt := validEffectiveArgvReceipt(t, invocation.Agent, adapter, bundle, effectiveArgvTestProxy)
			path := writeEffectiveArgvReceipt(t, receipt)
			parsed, receiptSHA, err := readEffectiveArgvReceipt(path, invocation, adapter, bundle, effectiveArgvTestProxy)
			if err != nil {
				t.Fatal(err)
			}
			if !lowerHexSHA256(receiptSHA) || !slices.Equal(parsed.EffectiveArgv, receipt.EffectiveArgv) {
				t.Fatalf("parsed receipt = %#v sha=%q", parsed, receiptSHA)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), effectiveArgvTestProxy) {
				t.Fatal("content-safe argv leaked the unguessable proxy URL")
			}
		})
	}
}

func TestEffectiveArgvReceiptRejectsSemanticAndExecutionDrift(t *testing.T) {
	invocation := effectiveArgvInvocation(t, "codex")
	adapter := adapterBinding{SHA256: strings.Repeat("a", 64)}
	bundle := codexBundleBinding{ManifestSHA256: strings.Repeat("b", 64), TreeSHA256: strings.Repeat("c", 64)}
	tests := []struct {
		name   string
		mutate func(*effectiveArgvReceipt)
	}{
		{"search", func(value *effectiveArgvReceipt) {
			value.EffectiveArgv = slices.Insert(value.EffectiveArgv, 1, "--search")
			value.EffectiveArgvSHA256 = hashCanonicalForTest(t, value.EffectiveArgv)
		}},
		{"sandbox", func(value *effectiveArgvReceipt) {
			replaceArgForTest(t, value.EffectiveArgv, "workspace-write", "danger-full-access")
			value.EffectiveArgvSHA256 = hashCanonicalForTest(t, value.EffectiveArgv)
		}},
		{"approval", func(value *effectiveArgvReceipt) {
			replaceArgForTest(t, value.EffectiveArgv, "never", "on-request")
			value.EffectiveArgvSHA256 = hashCanonicalForTest(t, value.EffectiveArgv)
		}},
		{"model", func(value *effectiveArgvReceipt) {
			value.SemanticProjection.Model = "gpt-5.6"
			value.SemanticProjectionSHA256 = hashCanonicalForTest(t, value.SemanticProjection)
		}},
		{"effort", func(value *effectiveArgvReceipt) {
			value.SemanticProjection.ReasoningEffort = "high"
			value.SemanticProjectionSHA256 = hashCanonicalForTest(t, value.SemanticProjection)
		}},
		{"store", func(value *effectiveArgvReceipt) {
			value.SemanticProjection.ResponseStorage = true
			value.SemanticProjectionSHA256 = hashCanonicalForTest(t, value.SemanticProjection)
		}},
		{"service tier", func(value *effectiveArgvReceipt) {
			value.SemanticProjection.ServiceTier = "auto"
			value.SemanticProjectionSHA256 = hashCanonicalForTest(t, value.SemanticProjection)
		}},
		{"fallback", func(value *effectiveArgvReceipt) {
			value.SemanticProjection.ModelFallback = true
			value.SemanticProjectionSHA256 = hashCanonicalForTest(t, value.SemanticProjection)
		}},
		{"responses transport", func(value *effectiveArgvReceipt) {
			value.SemanticProjection.ResponsesTransportRequirement = "http_allowed"
			value.SemanticProjectionSHA256 = hashCanonicalForTest(t, value.SemanticProjection)
		}},
		{"responses profile", func(value *effectiveArgvReceipt) {
			value.SemanticProjection.ResponsesAPIProfile = "openai_public"
			value.SemanticProjectionSHA256 = hashCanonicalForTest(t, value.SemanticProjection)
		}},
		{"agents", func(value *effectiveArgvReceipt) {
			value.SemanticProjection.AgentsEnabled = true
			value.SemanticProjectionSHA256 = hashCanonicalForTest(t, value.SemanticProjection)
		}},
		{"execution argv", func(value *effectiveArgvReceipt) { value.ExecutionArgvSHA256 = strings.Repeat("d", 64) }},
		{"proxy binding", func(value *effectiveArgvReceipt) { value.PrivateProxyBaseURLSHA256 = strings.Repeat("e", 64) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipt := validEffectiveArgvReceipt(t, invocation.Agent, adapter, bundle, effectiveArgvTestProxy)
			test.mutate(&receipt)
			if _, _, err := readEffectiveArgvReceipt(writeEffectiveArgvReceipt(t, receipt), invocation, adapter, bundle, effectiveArgvTestProxy); err == nil {
				t.Fatal("drifted effective argv receipt was accepted")
			}
		})
	}
}

func TestLubanEffectiveArgvReceiptRejectsFallbackSandboxAndAgentCatalogDrift(t *testing.T) {
	invocation := effectiveArgvInvocation(t, "luban")
	adapter := adapterBinding{SHA256: strings.Repeat("a", 64)}
	bundle := codexBundleBinding{ManifestSHA256: strings.Repeat("b", 64), TreeSHA256: strings.Repeat("c", 64)}
	for _, value := range []string{"--pinned-model", "--no-model-fallback", "--force-sandbox-tools"} {
		t.Run(value, func(t *testing.T) {
			receipt := validEffectiveArgvReceipt(t, invocation.Agent, adapter, bundle, effectiveArgvTestProxy)
			index := slices.Index(receipt.EffectiveArgv, value)
			if index < 0 {
				t.Fatalf("effective argv lacks %s", value)
			}
			receipt.EffectiveArgv = slices.Delete(receipt.EffectiveArgv, index, index+1)
			receipt.EffectiveArgvSHA256 = hashCanonicalForTest(t, receipt.EffectiveArgv)
			if _, _, err := readEffectiveArgvReceipt(writeEffectiveArgvReceipt(t, receipt), invocation, adapter, bundle, effectiveArgvTestProxy); err == nil {
				t.Fatal("drifted Luban effective argv receipt was accepted")
			}
		})
	}
	t.Run("agent catalog", func(t *testing.T) {
		receipt := validEffectiveArgvReceipt(t, invocation.Agent, adapter, bundle, effectiveArgvTestProxy)
		replaceArgForTest(t, receipt.EffectiveArgv, lubanDisallowedTools, "WebSearch,WebFetch")
		receipt.EffectiveArgvSHA256 = hashCanonicalForTest(t, receipt.EffectiveArgv)
		if _, _, err := readEffectiveArgvReceipt(writeEffectiveArgvReceipt(t, receipt), invocation, adapter, bundle, effectiveArgvTestProxy); err == nil {
			t.Fatal("Luban agent-tool catalog drift was accepted")
		}
	})
	t.Run("service tier", func(t *testing.T) {
		receipt := validEffectiveArgvReceipt(t, invocation.Agent, adapter, bundle, effectiveArgvTestProxy)
		replaceArgForTest(t, receipt.EffectiveArgv, harness.FormalServiceTier, "auto")
		receipt.EffectiveArgvSHA256 = hashCanonicalForTest(t, receipt.EffectiveArgv)
		if _, _, err := readEffectiveArgvReceipt(writeEffectiveArgvReceipt(t, receipt), invocation, adapter, bundle, effectiveArgvTestProxy); err == nil {
			t.Fatal("drifted Luban service tier was accepted")
		}
	})
}

func TestProjectLubanEffectiveArgvRequiresDefaultServiceTier(t *testing.T) {
	agent := effectiveArgvInvocation(t, "luban").Agent
	argv, err := expectedEffectiveArgv(agent, effectiveArgvTestProxy, false)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := projectEffectiveArgv("luban", argv)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ServiceTier != harness.FormalServiceTier || projection.ProviderTransport != "responses-http" ||
		projection.ResponsesAPIProfile != "openai_public" || projection.ResponsesLite ||
		projection.ResponsesTransportRequirement != harness.TransportRequirementHTTPInference {
		t.Fatalf("Luban projection = %#v", projection)
	}
	replaceArgForTest(t, argv, harness.FormalServiceTier, "auto")
	if _, err := projectEffectiveArgv("luban", argv); err == nil {
		t.Fatal("non-default service tier was accepted")
	}
}

func effectiveArgvInvocation(t *testing.T, kind string) harness.AgentInvocation {
	t.Helper()
	manifest := formalManifestFixture()
	for _, agent := range manifest.Agents {
		if agent.ID == kind {
			return harness.AgentInvocation{Agent: agent}
		}
	}
	t.Fatalf("fixture lacks %s", kind)
	return harness.AgentInvocation{}
}

func validEffectiveArgvReceipt(t *testing.T, agent harness.AgentSpec, adapter adapterBinding, bundle codexBundleBinding, proxy string) effectiveArgvReceipt {
	t.Helper()
	safe, err := expectedEffectiveArgv(agent, proxy, true)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := expectedEffectiveArgv(agent, proxy, false)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := projectEffectiveArgv(agent.ID, execution)
	if err != nil {
		t.Fatal(err)
	}
	return effectiveArgvReceipt{
		AdapterSHA256: adapter.SHA256, AdapterVersion: PinnedAdapterVersion, AgentKind: agent.ID,
		BundleManifestSHA256: bundle.ManifestSHA256, BundleTreeSHA256: bundle.TreeSHA256,
		EffectiveArgv: safe, EffectiveArgvSHA256: hashCanonicalForTest(t, safe),
		ExecutionArgvSHA256:       hashCanonicalForTest(t, execution),
		PrivateProxyBaseURLSHA256: hashStringForTest(proxy), SchemaVersion: effectiveArgvSchemaVersion,
		SemanticProjection: projection, SemanticProjectionSHA256: hashCanonicalForTest(t, projection),
		SourceCommandArgvSHA256: hashCanonicalForTest(t, agent.Command.Argv),
	}
}

func writeEffectiveArgvReceipt(t *testing.T, receipt effectiveArgvReceipt) string {
	t.Helper()
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "effective-argv.json")
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func hashCanonicalForTest(t *testing.T, value any) string {
	t.Helper()
	digest, err := harness.HashCanonical(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func hashStringForTest(value string) string {
	return sha256Hex([]byte(value))
}

func replaceArgForTest(t *testing.T, argv []string, oldValue, newValue string) {
	t.Helper()
	index := slices.Index(argv, oldValue)
	if index < 0 {
		t.Fatalf("argv lacks %q", oldValue)
	}
	argv[index] = newValue
}
