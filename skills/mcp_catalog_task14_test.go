package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
)

func TestMCPPromptCatalogIdentityDigestAndMetadataRevision(t *testing.T) {
	first, err := NewMCPPromptCatalogInput("GitHub", "review/pr", "Review a pull request", []string{"number"}, "Review $number.")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewMCPPromptCatalogInput("GitHub", "review/pr", "Review a pull request", []string{"number"}, "Review $number.")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Locator != second.Locator || first.Digest != second.Digest {
		t.Fatalf("same prompt was not stable:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("catalog input violates contract: %v", err)
	}
	if source, ok := first.ID.Source(); !ok || source != SourceMCP {
		t.Fatalf("ID source = %q, %v", source, ok)
	}

	changedBodyInput, err := NewMCPPromptCatalogInput("GitHub", "review/pr", "Review a pull request", []string{"number"}, "Review $number.\n")
	if err != nil {
		t.Fatal(err)
	}
	if changedBodyInput.ID != first.ID || changedBodyInput.Locator != first.Locator {
		t.Fatal("body update changed prompt identity")
	}
	if changedBodyInput.Digest == first.Digest {
		t.Fatal("body update did not change exact-content digest")
	}

	metadataInput, err := NewMCPPromptCatalogInput("GitHub", "review/pr", "Review pull requests carefully", []string{"number"}, "Review $number.")
	if err != nil {
		t.Fatal(err)
	}
	if metadataInput.ID != first.ID || metadataInput.Digest != first.Digest {
		t.Fatal("metadata-only update changed identity or body digest")
	}
	baseFingerprint, err := skillRevisionFingerprint(task14EffectiveSkill(first, 1))
	if err != nil {
		t.Fatal(err)
	}
	metadataFingerprint, err := skillRevisionFingerprint(task14EffectiveSkill(metadataInput, 1))
	if err != nil {
		t.Fatal(err)
	}
	if metadataFingerprint == baseFingerprint {
		t.Fatal("metadata update did not change effective revision input")
	}
}

func TestMCPResourceCatalogIdentityIncludesServerAndOriginalObject(t *testing.T) {
	const markdown = "---\ndescription: Review\n---\nbody\n"
	first := task14ResourceInput(t, "Server-A", catalog.Resource{
		URI: "SKILL://Review.EXAMPLE/catalog/./review/SKILL.md", Name: "shared",
	}, markdown)
	equivalent := task14ResourceInput(t, "Server-A", catalog.Resource{
		URI: "skill://review.example/catalog/review/SKILL.md", Name: "renamed",
	}, markdown)
	if first.ID != equivalent.ID || first.Locator != equivalent.Locator {
		t.Fatalf("canonical resource spellings differ:\nfirst=%#v\nequivalent=%#v", first, equivalent)
	}
	if first.Skill.Name == equivalent.Skill.Name {
		t.Fatal("test setup did not change display metadata")
	}

	otherServer := task14ResourceInput(t, "server-b", catalog.Resource{
		URI: "skill://review.example/catalog/review/SKILL.md", Name: "shared",
	}, markdown)
	if otherServer.ID == first.ID {
		t.Fatal("two servers publishing the same resource URI collided")
	}
	otherResource := task14ResourceInput(t, "Server-A", catalog.Resource{
		URI: "skill://review.example/catalog/other/SKILL.md", Name: "shared",
	}, markdown)
	if otherResource.ID == first.ID {
		t.Fatal("two resource URIs on one server collided")
	}

	changedMarkdown := task14ResourceInput(t, "Server-A", catalog.Resource{
		URI: "skill://review.example/catalog/review/SKILL.md", Name: "shared",
	}, markdown+"\n")
	if changedMarkdown.ID != first.ID {
		t.Fatal("markdown update changed resource identity")
	}
	if changedMarkdown.Digest == first.Digest {
		t.Fatal("markdown byte change did not change digest")
	}
	if first.Digest != ComputeSkillDigest(markdown) {
		t.Fatalf("digest = %q, want exact remote markdown digest", first.Digest)
	}
	if first.Skill.FilePath != "SKILL://Review.EXAMPLE/catalog/./review/SKILL.md" {
		t.Fatalf("resource FilePath changed: %q", first.Skill.FilePath)
	}
}

func TestMCPCatalogStoreRetainsSameDisplayNameAndIndependentNamespaces(t *testing.T) {
	manager := newCatalogManagerForTest()
	first := task14ResourceInput(t, "srv", catalog.Resource{URI: "skill://one/SKILL.md", Name: "shared"}, "one")
	second := task14ResourceInput(t, "srv", catalog.Resource{URI: "skill://two/SKILL.md", Name: "shared"}, "two")
	prompt, err := NewMCPPromptCatalogInput("srv", "shared", "", nil, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ReplaceMCPCatalogInputsAtGeneration(manager.ProjectGeneration(), []MCPCatalogInput{second, prompt, first}); err != nil {
		t.Fatal(err)
	}

	inputs := manager.MCPCatalogInputs()
	if len(inputs) != 3 {
		t.Fatalf("catalog inputs = %#v, want three stable entries", inputs)
	}
	for index := 1; index < len(inputs); index++ {
		if inputs[index-1].ID >= inputs[index].ID {
			t.Fatalf("inputs are not sorted by stable ID: %#v", inputs)
		}
	}
	inputs[0].Skill.Name = "mutated"
	inputsAgain := manager.MCPCatalogInputs()
	for _, input := range inputsAgain {
		if input.Skill.Name == "mutated" {
			t.Fatal("catalog snapshot exposed store-owned Skill pointer")
		}
	}

	if err := manager.ReplaceMCPCatalogInputsAtGeneration(manager.ProjectGeneration(), []MCPCatalogInput{prompt}); err != nil {
		t.Fatal(err)
	}
	remaining := manager.MCPCatalogInputs()
	if len(remaining) != 1 || remaining[0].ID != prompt.ID {
		t.Fatalf("clearing resources changed prompt entries: %#v", remaining)
	}
}

func TestMCPCatalogRefreshRetainsStateOnReadErrorAndRecovers(t *testing.T) {
	t.Setenv(FeatureFlagMCPSkills, "1")
	manager := newCatalogManagerForTest()
	goodRaw := &task14MCPRawCaller{
		resources: []catalog.Resource{{URI: "skill://review/SKILL.md", Name: "review"}},
		reads: map[string]catalog.ReadResourceResult{
			"skill://review/SKILL.md": {Contents: []catalog.ResourceContent{{Text: "body"}}},
		},
	}
	goodState := task14ConnectedState(t, "srv", goodRaw)
	if err := task14RefreshCurrentMCPCatalog(context.Background(), manager, []mcpmanager.MCPServerConnection{goodState}); err != nil {
		t.Fatal(err)
	}
	initial := manager.MCPCatalogInputs()
	if len(initial) != 1 {
		t.Fatalf("initial inputs = %#v", initial)
	}

	failingRaw := &task14MCPRawCaller{
		resources: goodRaw.resources,
		readErr:   errors.New("temporary read failure"),
	}
	failingState := task14ConnectedState(t, "srv", failingRaw)
	if err := task14RefreshCurrentMCPCatalog(context.Background(), manager, []mcpmanager.MCPServerConnection{failingState}); err == nil {
		t.Fatal("read failure was not reported")
	}
	afterFailure := manager.MCPCatalogInputs()
	if !reflect.DeepEqual(afterFailure, initial) {
		t.Fatalf("transient error replaced last authoritative state:\nbefore=%#v\nafter=%#v", initial, afterFailure)
	}

	if err := task14RefreshCurrentMCPCatalog(context.Background(), manager, []mcpmanager.MCPServerConnection{{
		Name: "srv", Type: mcpmanager.MCPStateFailed,
	}}); err != nil {
		t.Fatal(err)
	}
	if len(manager.MCPCatalogInputs()) != 0 {
		t.Fatal("successful disconnected snapshot did not clear resources")
	}
	if err := task14RefreshCurrentMCPCatalog(context.Background(), manager, []mcpmanager.MCPServerConnection{goodState}); err != nil {
		t.Fatal(err)
	}
	reconnected := manager.MCPCatalogInputs()
	if len(reconnected) != 1 || reconnected[0].ID != initial[0].ID {
		t.Fatalf("reconnect identity changed: %#v", reconnected)
	}

	t.Setenv(FeatureFlagMCPSkills, "0")
	if err := task14RefreshCurrentMCPCatalog(context.Background(), manager, []mcpmanager.MCPServerConnection{goodState}); err != nil {
		t.Fatal(err)
	}
	if len(manager.MCPCatalogInputs()) != 0 {
		t.Fatal("feature-gated empty snapshot did not clear resources")
	}
}

func TestMCPDigestTracksExactMultipartRemoteMarkdown(t *testing.T) {
	parts := []catalog.ResourceContent{{Text: "first\r\n"}, {Text: "second\n"}}
	markdown, ok := mcpSkillMarkdown(parts)
	if !ok {
		t.Fatal("multipart markdown was not produced")
	}
	input := task14ResourceInput(t, "srv", catalog.Resource{URI: "skill://multi/SKILL.md", Name: "multi"}, markdown)
	if input.Digest != ComputeSkillDigest("first\r\n\nsecond\n") {
		t.Fatalf("multipart digest = %q for markdown %q", input.Digest, markdown)
	}
	if input.Digest == ComputeSkillDigest("first\n\nsecond\n") {
		t.Fatal("CRLF was normalized before digesting")
	}
}

func TestMCPCatalogConcurrentReplaceAndSnapshotIsRaceSafe(t *testing.T) {
	manager := newCatalogManagerForTest()
	input := task14ResourceInput(t, "srv", catalog.Resource{URI: "skill://race/SKILL.md", Name: "race"}, "body")
	prompt, err := NewMCPPromptCatalogInput("srv", "prompt", "", nil, "body")
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for iteration := 0; iteration < 100; iteration++ {
				switch worker % 4 {
				case 0:
					_ = manager.ReplaceMCPCatalogInputsAtGeneration(manager.ProjectGeneration(), []MCPCatalogInput{prompt})
				case 1:
					_ = manager.ReplaceMCPCatalogInputsAtGeneration(manager.ProjectGeneration(), []MCPCatalogInput{input})
				case 2:
					_ = manager.MCPCatalogInputs()
				default:
					_ = manager.MCPCatalogInputs()
				}
			}
		}(worker)
	}
	workers.Wait()
	for _, catalogInput := range manager.MCPCatalogInputs() {
		if err := catalogInput.Validate(); err != nil {
			t.Fatalf("concurrent catalog input: %v", err)
		}
	}
}

type task14MCPRawCaller struct {
	resources []catalog.Resource
	reads     map[string]catalog.ReadResourceResult
	readErr   error
}

func (caller *task14MCPRawCaller) CallRaw(_ context.Context, method string, params any, out any) error {
	var value any
	switch method {
	case "resources/list":
		value = catalog.ListResourcesResult{Resources: caller.resources}
	case "resources/read":
		if caller.readErr != nil {
			return caller.readErr
		}
		uri := params.(map[string]any)["uri"].(string)
		value = caller.reads[uri]
	default:
		return fmt.Errorf("unexpected MCP method %q", method)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func task14ConnectedState(t testing.TB, serverName string, raw *task14MCPRawCaller) mcpmanager.MCPServerConnection {
	t.Helper()
	return mcpmanager.MCPServerConnection{
		Name:         serverName,
		Type:         mcpmanager.MCPStateConnected,
		Client:       newMCPProtocolTestClient(t, raw),
		Capabilities: catalog.ServerCapabilities{"resources": map[string]any{}},
		Resources:    append([]catalog.Resource(nil), raw.resources...),
	}
}

func task14RefreshCurrentMCPCatalog(ctx context.Context, manager *Manager, states []mcpmanager.MCPServerConnection) error {
	inputs, err := DiscoverMCPCatalogInputsFromConnections(ctx, states)
	if err != nil {
		return err
	}
	return manager.ReplaceMCPCatalogInputsAtGeneration(manager.ProjectGeneration(), inputs)
}

func task14ResourceInput(t *testing.T, serverName string, resource catalog.Resource, markdown string) MCPCatalogInput {
	t.Helper()
	skill := skillFromMCPResource(serverName, resource, markdown)
	if skill == nil {
		t.Fatalf("skillFromMCPResource(%q, %#v) returned nil", serverName, resource)
	}
	input, err := newMCPResourceCatalogInput(serverName, skill)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func task14EffectiveSkill(input MCPCatalogInput, revision SkillRevision) EffectiveSkill {
	return EffectiveSkill{
		ID:                 input.ID,
		Name:               input.Skill.Name,
		Summary:            input.Skill.effectiveDescription(),
		Source:             SourceMCP,
		Locator:            input.Locator,
		Digest:             input.Digest,
		Revision:           revision,
		Visibility:         VisibilityAuto,
		VisibilitySource:   SkillScopeDefault,
		ModelVisible:       true,
		DescriptionVisible: true,
		UserInvocable:      true,
		Executable:         true,
		Mutable:            true,
	}
}
