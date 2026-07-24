package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	svcmcp "github.com/agent-dance/luban/services/mcp"
)

func TestMCPPromptCatalogIdentityDigestAndMetadataRevision(t *testing.T) {
	prompt := MCPPrompt{
		Server: "GitHub", Name: "review/pr", Description: "Review a pull request",
		WhenToUse: "Use for pull requests", ArgNames: []string{"number"}, Body: "Review $number.",
	}
	first, err := prompt.CatalogInput()
	if err != nil {
		t.Fatal(err)
	}
	second, err := prompt.CatalogInput()
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

	bodyChanged := prompt
	bodyChanged.Body += "\n"
	changedBodyInput, err := bodyChanged.CatalogInput()
	if err != nil {
		t.Fatal(err)
	}
	if changedBodyInput.ID != first.ID || changedBodyInput.Locator != first.Locator {
		t.Fatal("body update changed prompt identity")
	}
	if changedBodyInput.Digest == first.Digest {
		t.Fatal("body update did not change exact-content digest")
	}

	metadataChanged := prompt
	metadataChanged.Description = "Review pull requests carefully"
	metadataInput, err := metadataChanged.CatalogInput()
	if err != nil {
		t.Fatal(err)
	}
	if metadataInput.ID != first.ID || metadataInput.Digest != first.Digest {
		t.Fatal("metadata-only update changed identity or body digest")
	}
	baseFingerprint, err := SkillRevisionFingerprint(task14EffectiveSkill(first, 1))
	if err != nil {
		t.Fatal(err)
	}
	metadataFingerprint, err := SkillRevisionFingerprint(task14EffectiveSkill(metadataInput, 1))
	if err != nil {
		t.Fatal(err)
	}
	if metadataFingerprint == baseFingerprint {
		t.Fatal("metadata update did not change effective revision input")
	}
}

func TestMCPResourceCatalogIdentityIncludesServerAndOriginalObject(t *testing.T) {
	const markdown = "---\ndescription: Review\n---\nbody\n"
	first := task14ResourceInput(t, "Server-A", svcmcp.Resource{
		URI: "SKILL://Review.EXAMPLE/catalog/./review/SKILL.md", Name: "shared",
	}, markdown)
	equivalent := task14ResourceInput(t, "Server-A", svcmcp.Resource{
		URI: "skill://review.example/catalog/review/SKILL.md", Name: "renamed",
	}, markdown)
	if first.ID != equivalent.ID || first.Locator != equivalent.Locator {
		t.Fatalf("canonical resource spellings differ:\nfirst=%#v\nequivalent=%#v", first, equivalent)
	}
	if first.Skill.Name == equivalent.Skill.Name {
		t.Fatal("test setup did not change display metadata")
	}

	otherServer := task14ResourceInput(t, "server-b", svcmcp.Resource{
		URI: "skill://review.example/catalog/review/SKILL.md", Name: "shared",
	}, markdown)
	if otherServer.ID == first.ID {
		t.Fatal("two servers publishing the same resource URI collided")
	}
	otherResource := task14ResourceInput(t, "Server-A", svcmcp.Resource{
		URI: "skill://review.example/catalog/other/SKILL.md", Name: "shared",
	}, markdown)
	if otherResource.ID == first.ID {
		t.Fatal("two resource URIs on one server collided")
	}

	changedMarkdown := task14ResourceInput(t, "Server-A", svcmcp.Resource{
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
		t.Fatalf("legacy resource FilePath changed: %q", first.Skill.FilePath)
	}
}

func TestMCPCatalogStoreRetainsSameDisplayNameAndIndependentNamespaces(t *testing.T) {
	manager := NewManager()
	first := task14ResourceInput(t, "srv", svcmcp.Resource{URI: "skill://one/SKILL.md", Name: "shared"}, "one")
	second := task14ResourceInput(t, "srv", svcmcp.Resource{URI: "skill://two/SKILL.md", Name: "shared"}, "two")
	manager.RegisterMCPSkillCatalogInputs([]MCPCatalogInput{second, first})
	manager.RegisterMCPPrompts([]MCPPrompt{{Name: "shared", Body: "prompt"}})

	inputs := manager.MCPCatalogInputs()
	if len(inputs) != 3 {
		t.Fatalf("catalog inputs = %#v, want three stable entries", inputs)
	}
	for index := 1; index < len(inputs); index++ {
		if inputs[index-1].ID >= inputs[index].ID {
			t.Fatalf("inputs are not sorted by stable ID: %#v", inputs)
		}
	}
	resourceInputs := manager.MCPSkillCatalogInputs()
	if len(resourceInputs) != 2 || resourceInputs[0].ID == resourceInputs[1].ID {
		t.Fatalf("same-name resources were collapsed: %#v", resourceInputs)
	}
	if got := len(manager.MCPSkills()); got != 2 {
		t.Fatalf("legacy MCPSkills length = %d, want 2", got)
	}

	inputs[0].Skill.Name = "mutated"
	inputsAgain := manager.MCPCatalogInputs()
	for _, input := range inputsAgain {
		if input.Skill.Name == "mutated" {
			t.Fatal("catalog snapshot exposed store-owned Skill pointer")
		}
	}

	manager.RegisterMCPPrompts(nil)
	if len(manager.MCPCatalogInputs()) != 2 || len(manager.MCPSkillCatalogInputs()) != 2 {
		t.Fatal("clearing prompts also cleared resource-backed skills")
	}
	manager.RegisterMCPSkillCatalogInputs(nil)
	if len(manager.MCPCatalogInputs()) != 0 {
		t.Fatal("clearing resources left stale MCP catalog inputs")
	}
}

func TestMCPRevisionDeletionAndReconnectProducesRevokeAndReenable(t *testing.T) {
	input := task14ResourceInput(t, "srv", svcmcp.Resource{URI: "skill://review/SKILL.md", Name: "review"}, "body")
	before, err := NewCatalogSnapshot(1, []EffectiveSkill{task14EffectiveSkill(input, 1)})
	if err != nil {
		t.Fatal(err)
	}
	disconnected, err := NewCatalogSnapshot(2, nil)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := DiffCatalog(before, disconnected)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Revokes) != 1 || removed.Revokes[0].ID != input.ID || removed.Revokes[0].Reason != CatalogRevokeDeleted {
		t.Fatalf("removal delta = %#v", removed)
	}

	reconnectedSkill := task14EffectiveSkill(input, 2)
	reconnected, err := NewCatalogSnapshot(3, []EffectiveSkill{reconnectedSkill})
	if err != nil {
		t.Fatal(err)
	}
	coalesced, err := CoalesceCatalogSnapshots(before, disconnected, reconnected)
	if err != nil {
		t.Fatal(err)
	}
	if len(coalesced.Upserts) != 1 || coalesced.Upserts[0].Skill.ID != input.ID || coalesced.Upserts[0].Reason != CatalogUpsertReenabled {
		t.Fatalf("reconnect delta = %#v", coalesced)
	}
}

func TestMCPCatalogRefreshRetainsStateOnReadErrorAndRecovers(t *testing.T) {
	t.Setenv(FeatureFlagMCPSkills, "1")
	manager := NewManager()
	goodRaw := &task14MCPRawCaller{
		resources: []svcmcp.Resource{{URI: "skill://review/SKILL.md", Name: "review"}},
		reads: map[string]svcmcp.ReadResourceResult{
			"skill://review/SKILL.md": {Contents: []svcmcp.ResourceContent{{Text: "body"}}},
		},
	}
	goodState := task14ConnectedState("srv", goodRaw)
	if err := RefreshMCPSkillCatalogFromConnections(context.Background(), manager, []svcmcp.MCPServerConnection{goodState}); err != nil {
		t.Fatal(err)
	}
	initial := manager.MCPSkillCatalogInputs()
	if len(initial) != 1 {
		t.Fatalf("initial inputs = %#v", initial)
	}

	failingRaw := &task14MCPRawCaller{
		resources: goodRaw.resources,
		readErr:   errors.New("temporary read failure"),
	}
	failingState := task14ConnectedState("srv", failingRaw)
	if err := RefreshMCPSkillCatalogFromConnections(context.Background(), manager, []svcmcp.MCPServerConnection{failingState}); err == nil {
		t.Fatal("read failure was not reported")
	}
	afterFailure := manager.MCPSkillCatalogInputs()
	if !reflect.DeepEqual(afterFailure, initial) {
		t.Fatalf("transient error replaced last authoritative state:\nbefore=%#v\nafter=%#v", initial, afterFailure)
	}

	if err := RefreshMCPSkillCatalogFromConnections(context.Background(), manager, []svcmcp.MCPServerConnection{{
		Name: "srv", Type: svcmcp.MCPStateFailed,
	}}); err != nil {
		t.Fatal(err)
	}
	if len(manager.MCPSkillCatalogInputs()) != 0 {
		t.Fatal("successful disconnected snapshot did not clear resources")
	}
	if err := RefreshMCPSkillCatalogFromConnections(context.Background(), manager, []svcmcp.MCPServerConnection{goodState}); err != nil {
		t.Fatal(err)
	}
	reconnected := manager.MCPSkillCatalogInputs()
	if len(reconnected) != 1 || reconnected[0].ID != initial[0].ID {
		t.Fatalf("reconnect identity changed: %#v", reconnected)
	}

	t.Setenv(FeatureFlagMCPSkills, "0")
	if err := RefreshMCPSkillCatalogFromConnections(context.Background(), manager, []svcmcp.MCPServerConnection{goodState}); err != nil {
		t.Fatal(err)
	}
	if len(manager.MCPSkillCatalogInputs()) != 0 {
		t.Fatal("feature-gated empty snapshot did not clear resources")
	}
}

func TestMCPDigestTracksExactMultipartRemoteMarkdown(t *testing.T) {
	parts := []svcmcp.ResourceContent{{Text: "first\r\n"}, {Text: "second\n"}}
	markdown, ok := mcpSkillMarkdown(parts)
	if !ok {
		t.Fatal("multipart markdown was not produced")
	}
	input := task14ResourceInput(t, "srv", svcmcp.Resource{URI: "skill://multi/SKILL.md", Name: "multi"}, markdown)
	if input.Digest != ComputeSkillDigest("first\r\n\nsecond\n") {
		t.Fatalf("multipart digest = %q for markdown %q", input.Digest, markdown)
	}
	if input.Digest == ComputeSkillDigest("first\n\nsecond\n") {
		t.Fatal("CRLF was normalized before digesting")
	}
}

func TestMCPCatalogSameDisplayNameLocalIdentityRemainsDistinct(t *testing.T) {
	prompt, err := (MCPPrompt{Name: "shared", Body: "remote"}).CatalogInput()
	if err != nil {
		t.Fatal(err)
	}
	localLocator, err := CanonicalFilesystemSkillLocator(filepath.Join(t.TempDir(), "shared", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	localID, err := ComputeSkillID(SourceProject, localLocator)
	if err != nil {
		t.Fatal(err)
	}
	if prompt.Skill.Name != "shared" || prompt.ID == localID {
		t.Fatalf("same display name identities: MCP=%q local=%q", prompt.ID, localID)
	}
}

func TestMCPCatalogConcurrentReplaceAndSnapshotIsRaceSafe(t *testing.T) {
	manager := NewManager()
	input := task14ResourceInput(t, "srv", svcmcp.Resource{URI: "skill://race/SKILL.md", Name: "race"}, "body")
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for iteration := 0; iteration < 100; iteration++ {
				switch worker % 4 {
				case 0:
					manager.RegisterMCPPrompts([]MCPPrompt{{Server: "srv", Name: "prompt", Body: "body"}})
				case 1:
					manager.RegisterMCPSkillCatalogInputs([]MCPCatalogInput{input})
				case 2:
					_ = manager.MCPCatalogInputs()
				default:
					_ = manager.MCPPrompts()
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
	resources []svcmcp.Resource
	reads     map[string]svcmcp.ReadResourceResult
	readErr   error
}

func (caller *task14MCPRawCaller) CallRaw(_ context.Context, method string, params any, out any) error {
	var value any
	switch method {
	case "resources/list":
		value = svcmcp.ListResourcesResult{Resources: caller.resources}
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

func task14ConnectedState(serverName string, raw *task14MCPRawCaller) svcmcp.MCPServerConnection {
	return svcmcp.MCPServerConnection{
		Name:         serverName,
		Type:         svcmcp.MCPStateConnected,
		Client:       svcmcp.NewClient(raw, nil),
		Capabilities: svcmcp.ServerCapabilities{"resources": map[string]any{}},
		Resources:    append([]svcmcp.Resource(nil), raw.resources...),
	}
}

func task14ResourceInput(t *testing.T, serverName string, resource svcmcp.Resource, markdown string) MCPCatalogInput {
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
		Summary:            input.Skill.EffectiveDescription(),
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
