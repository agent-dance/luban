package skills

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
)

type mcpSkillRawCaller struct {
	t         *testing.T
	resources []catalog.Resource
	reads     map[string]catalog.ReadResourceResult
}

func (f *mcpSkillRawCaller) CallRaw(_ context.Context, method string, params any, out any) error {
	var value any
	switch method {
	case "resources/list":
		value = catalog.ListResourcesResult{Resources: f.resources}
	case "resources/read":
		uri := params.(map[string]any)["uri"].(string)
		value = f.reads[uri]
	default:
		f.t.Fatalf("unexpected method %s", method)
	}
	data, err := json.Marshal(value)
	if err != nil {
		f.t.Fatal(err)
	}
	return json.Unmarshal(data, out)
}

func TestMCPSkillsFeatureGateDisablesDiscovery(t *testing.T) {
	t.Setenv(FeatureFlagMCPSkills, "")
	client := newMCPProtocolTestClient(t, &mcpSkillRawCaller{t: t})
	inputs, err := discoverMCPResourceCatalogInputsFromConnections(context.Background(), []mcpmanager.MCPServerConnection{{
		Name:         "srv",
		Type:         mcpmanager.MCPStateConnected,
		Client:       client,
		Capabilities: catalog.ServerCapabilities{"resources": map[string]any{}},
		Resources:    []catalog.Resource{{URI: "skill://review", Name: "review"}},
	}})
	if err != nil {
		t.Fatalf("discoverMCPResourceCatalogInputsFromConnections: %v", err)
	}
	if len(inputs) != 0 {
		t.Fatalf("expected no skills when gate disabled, got %#v", inputs)
	}
}

func TestMCPSkillsDiscoverSkillResourcesWithFrontmatter(t *testing.T) {
	t.Setenv(FeatureFlagMCPSkills, "1")
	markdown := `---
description: Review GitHub issues
allowed-tools: Read, Grep
arguments: owner, repo
argument-hint: <owner> <repo>
when_to_use: Use when triaging issues
version: 1.2.3
context: fork
agent: verifier
effort: high
---
Review $owner/$repo.`
	raw := &mcpSkillRawCaller{
		t: t,
		resources: []catalog.Resource{
			{URI: "skill://review/SKILL.md", Name: "review", Description: "fallback"},
			{URI: "file://ignored", Name: "ignored"},
		},
		reads: map[string]catalog.ReadResourceResult{
			"skill://review/SKILL.md": {Contents: []catalog.ResourceContent{{URI: "skill://review/SKILL.md", Text: markdown, MimeType: "text/markdown"}}},
		},
	}
	client := newMCPProtocolTestClient(t, raw)
	discovered, err := discoverMCPResourceCatalogInputsFromConnections(context.Background(), []mcpmanager.MCPServerConnection{{
		Name:         "github",
		Type:         mcpmanager.MCPStateConnected,
		Client:       client,
		Capabilities: catalog.ServerCapabilities{"resources": map[string]any{}},
		Resources:    raw.resources,
	}})
	if err != nil {
		t.Fatalf("discoverMCPResourceCatalogInputsFromConnections: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("skills = %#v", discovered)
	}
	skill := discovered[0].Skill
	if skill.Name != "github:review" {
		t.Fatalf("name = %q", skill.Name)
	}
	if skill.Source != SourceMCP {
		t.Fatalf("source = %s", skill.Source)
	}
	if skill.Description != "Review GitHub issues" || !skill.HasUserSpecifiedDescription {
		t.Fatalf("description = %q hasUser=%v", skill.Description, skill.HasUserSpecifiedDescription)
	}
	if reflect.DeepEqual(skill.AllowedTools, []string{"Read", "Grep"}) {
		// expected
	} else {
		t.Fatalf("allowed tools = %#v", skill.AllowedTools)
	}
	if !reflect.DeepEqual(skill.ArgNames, []string{"owner", "repo"}) {
		t.Fatalf("arg names = %#v", skill.ArgNames)
	}
	if skill.ArgumentHint != "<owner> <repo>" || skill.WhenToUse == "" || skill.Version != "1.2.3" {
		t.Fatalf("frontmatter fields not preserved: %#v", skill)
	}
	if skill.Context != ContextFork || skill.Agent != "verifier" || skill.Effort != "high" {
		t.Fatalf("execution fields not preserved: %#v", skill)
	}
	if skill.SkillDir != "" || skill.FilePath != "skill://review/SKILL.md" {
		t.Fatalf("remote skill location fields = dir %q file %q", skill.SkillDir, skill.FilePath)
	}
	if skill.Content != "Review $owner/$repo." {
		t.Fatalf("content = %q", skill.Content)
	}
}

func TestMCPSkillsReadBase64MarkdownAndRegisterUnifiedCatalog(t *testing.T) {
	t.Setenv(FeatureFlagMCPSkills, "1")
	body := "---\ndescription: Encoded skill\n---\nencoded body"
	raw := &mcpSkillRawCaller{
		t:         t,
		resources: []catalog.Resource{{URI: "skill://encoded", Name: "encoded"}},
		reads: map[string]catalog.ReadResourceResult{
			"skill://encoded": {Contents: []catalog.ResourceContent{{URI: "skill://encoded", Blob: base64.StdEncoding.EncodeToString([]byte(body)), MimeType: "text/markdown"}}},
		},
	}
	client := newMCPProtocolTestClient(t, raw)
	discovered, err := discoverMCPResourceCatalogInputsFromConnections(context.Background(), []mcpmanager.MCPServerConnection{{
		Name:         "srv",
		Type:         mcpmanager.MCPStateConnected,
		Client:       client,
		Capabilities: catalog.ServerCapabilities{"resources": map[string]any{}},
		Resources:    raw.resources,
	}})
	if err != nil {
		t.Fatalf("discoverMCPResourceCatalogInputsFromConnections: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("discovered = %#v", discovered)
	}
	resourceInput := discovered[0]
	promptInput, err := NewMCPPromptCatalogInput("srv", "prompt", "prompt desc", nil, "prompt body")
	if err != nil {
		t.Fatal(err)
	}
	manager := newCatalogManagerForTest()
	if err := manager.ReplaceMCPCatalogInputsAtGeneration(manager.ProjectGeneration(), []MCPCatalogInput{promptInput, resourceInput}); err != nil {
		t.Fatal(err)
	}

	resolvedLoaderTestSkill(t, manager, "srv:prompt")
	got := resolvedLoaderTestSkill(t, manager, "srv:encoded")
	if got.Content != "encoded body" {
		t.Fatalf("content = %q", got.Content)
	}
	if err := manager.ReplaceMCPCatalogInputsAtGeneration(manager.ProjectGeneration(), []MCPCatalogInput{promptInput}); err != nil {
		t.Fatal(err)
	}
	generation := manager.ProjectGeneration()
	encoded, encodedErr := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "session", Selector: "srv:encoded", Origin: InvocationOriginUser, ExpectedProjectGeneration: generation,
	}, nil)
	prompt, promptErr := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "session", Selector: "srv:prompt", Origin: InvocationOriginUser, ExpectedProjectGeneration: generation,
	}, nil)
	if encodedErr != nil || promptErr != nil || encoded.Outcome != SkillResolveNotFound || prompt.Outcome != SkillResolveResolved {
		t.Fatal("unified catalog replacement did not publish the exact projection")
	}
}

func TestMCPSkillsDeriveNameFromSkillMDSiblingDirectory(t *testing.T) {
	t.Setenv(FeatureFlagMCPSkills, "1")
	raw := &mcpSkillRawCaller{
		t:         t,
		resources: []catalog.Resource{{URI: "skill://catalog/review/SKILL.md"}},
		reads: map[string]catalog.ReadResourceResult{
			"skill://catalog/review/SKILL.md": {Contents: []catalog.ResourceContent{{Text: "---\ndescription: Review\n---\nbody"}}},
		},
	}
	client := newMCPProtocolTestClient(t, raw)
	discovered, err := discoverMCPResourceCatalogInputsFromConnections(context.Background(), []mcpmanager.MCPServerConnection{{
		Name:         "srv",
		Type:         mcpmanager.MCPStateConnected,
		Client:       client,
		Capabilities: catalog.ServerCapabilities{"resources": map[string]any{}},
		Resources:    raw.resources,
	}})
	if err != nil {
		t.Fatalf("discoverMCPResourceCatalogInputsFromConnections: %v", err)
	}
	if len(discovered) != 1 || discovered[0].Skill.Name != "srv:review" {
		t.Fatalf("discovered = %#v", discovered)
	}
}

func TestMCPSkillsSkipDisconnectedAndNoResourceCapability(t *testing.T) {
	t.Setenv(FeatureFlagMCPSkills, "1")
	raw := &mcpSkillRawCaller{t: t}
	client := newMCPProtocolTestClient(t, raw)
	discovered, err := discoverMCPResourceCatalogInputsFromConnections(context.Background(), []mcpmanager.MCPServerConnection{
		{Name: "pending", Type: mcpmanager.MCPStatePending, Client: client, Capabilities: catalog.ServerCapabilities{"resources": map[string]any{}}},
		{Name: "tools-only", Type: mcpmanager.MCPStateConnected, Client: client, Capabilities: catalog.ServerCapabilities{"tools": map[string]any{}}},
	})
	if err != nil {
		t.Fatalf("discoverMCPResourceCatalogInputsFromConnections: %v", err)
	}
	if len(discovered) != 0 {
		t.Fatalf("expected no discovered skills, got %#v", discovered)
	}
}
