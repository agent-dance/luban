package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

func TestSetupRegistryApplyPatchCustomToolUsesGateOrProviderCapability(t *testing.T) {
	t.Setenv("LUBAN_CODE_EXPERIMENTAL_APPLY_PATCH_CUSTOM_TOOL", "")
	baseline := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	baselineSnapshot, err := baseline.Registry.SnapshotVisibleTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := baselineSnapshot.Names(); !reflect.DeepEqual(got, []string{"Inspect", "ApplyPatch", "Run"}) {
		t.Fatalf("baseline names = %v", got)
	}
	baselinePatch := definitionByName(t, baselineSnapshot.Definitions(), "ApplyPatch")
	if baselinePatch.IsCustom() || baselinePatch.Format != nil || !baselinePatch.Strict {
		t.Fatalf("default ApplyPatch must remain JSON function: %#v", baselinePatch)
	}

	t.Setenv("LUBAN_CODE_EXPERIMENTAL_APPLY_PATCH_CUSTOM_TOOL", "true")
	custom := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	customSnapshot, err := custom.Registry.SnapshotVisibleTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	customDefinitions := customSnapshot.Definitions()
	if got := customSnapshot.Names(); !reflect.DeepEqual(got, []string{"Inspect", "ApplyPatch", "Run"}) {
		t.Fatalf("custom names = %v", got)
	}
	customPatch := definitionByName(t, customDefinitions, "ApplyPatch")
	if !customPatch.IsCustom() || customPatch.Format == nil || customPatch.Format.Type != "grammar" || customPatch.Format.Syntax != "lark" {
		t.Fatalf("custom ApplyPatch definition = %#v", customPatch)
	}
	for _, name := range []string{"Inspect", "Run"} {
		definition := definitionByName(t, customDefinitions, name)
		if definition.IsCustom() || definition.Format != nil {
			t.Fatalf("%s changed from function tool: %#v", name, definition)
		}
	}
	if customSnapshot.Digest() == baselineSnapshot.Digest() {
		t.Fatal("custom grammar did not change the visible schema fingerprint")
	}
	customPatch.Format.Definition = "corrupted"
	if got := definitionByName(t, customSnapshot.Definitions(), "ApplyPatch").Format.Definition; got == "corrupted" {
		t.Fatal("snapshot grammar was mutated through a returned definition")
	}

	t.Setenv("LUBAN_CODE_EXPERIMENTAL_APPLY_PATCH_CUSTOM_TOOL", "")
	capableProvider := provider.NewResponses(provider.Config{
		ProviderName: "deepseek", Model: "deepseek-v4-flash", ResponsesSemantics: provider.ResponsesSemanticsDeepSeek,
	})
	capable := SetupRegistry(provider.NewProviderRef(capableProvider), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	capableSnapshot, err := capable.Registry.SnapshotVisibleTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	if definition := definitionByName(t, capableSnapshot.Definitions(), "ApplyPatch"); !definition.IsCustom() {
		t.Fatalf("custom-capable provider did not activate grammar ApplyPatch: %#v", definition)
	}
}

func TestDefaultCodingKernelPublicResponsesWireIsExactlyThreeFunctionTools(t *testing.T) {
	t.Setenv("LUBAN_CODE_EXPERIMENTAL_APPLY_PATCH_CUSTOM_TOOL", "")
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	snapshot, err := deps.Registry.SnapshotVisibleTools(nil)
	if err != nil {
		t.Fatal(err)
	}

	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-openai-internal-codex-responses-lite") != "" {
			t.Error("public Responses request used private Lite semantics")
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.created\ndata: {\"response\":{\"id\":\"resp_wire\",\"model\":\"gpt-5.6-sol\"}}\n\n"+
			"event: response.completed\ndata: {\"response\":{\"id\":\"resp_wire\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n")
	}))
	defer server.Close()

	responses := provider.NewResponses(provider.Config{
		ProviderName: "openai", BaseURL: server.URL, APIKey: "test", Model: "gpt-5.6-sol",
		ResponsesSemantics: provider.ResponsesSemanticsOpenAIPublic,
	})
	stream, err := responses.CreateStream(context.Background(), provider.Params{
		Model: "gpt-5.6-sol", Messages: []types.Message{types.UserMessage("inspect then patch")},
		Tools: snapshot.Definitions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for event := range stream {
		if event.Type == types.EventError {
			t.Fatalf("stream error: %v", event.Error)
		}
	}

	wireTools, ok := request["tools"].([]any)
	if !ok || len(wireTools) != 3 {
		t.Fatalf("wire tools = %#v", request["tools"])
	}
	wantNames := []string{"Inspect", "ApplyPatch", "Run"}
	seen := make(map[string]struct{}, 3)
	for index, raw := range wireTools {
		tool := raw.(map[string]any)
		name, _ := tool["name"].(string)
		if name != wantNames[index] || tool["type"] != "function" || tool["parameters"] == nil {
			t.Fatalf("wire tool[%d] = %#v, want function %s", index, tool, wantNames[index])
		}
		if _, duplicate := seen[name]; duplicate {
			t.Fatalf("duplicate wire tool %q", name)
		}
		seen[name] = struct{}{}
	}
	for _, raw := range request["input"].([]any) {
		if item, ok := raw.(map[string]any); ok && item["type"] == "additional_tools" {
			t.Fatalf("public input leaked additional_tools: %#v", request["input"])
		}
	}
}

func definitionByName(t *testing.T, definitions []types.ToolDefinition, name string) types.ToolDefinition {
	t.Helper()
	for _, definition := range definitions {
		if definition.Name == name {
			return definition
		}
	}
	t.Fatalf("missing definition %q", name)
	return types.ToolDefinition{}
}
