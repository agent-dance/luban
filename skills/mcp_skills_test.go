package skills

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"

	svcmcp "github.com/agent-dance/luban/services/mcp"
)

type mcpSkillRawCaller struct {
	t         *testing.T
	resources []svcmcp.Resource
	reads     map[string]svcmcp.ReadResourceResult
}

func (f *mcpSkillRawCaller) CallRaw(_ context.Context, method string, params any, out any) error {
	var value any
	switch method {
	case "resources/list":
		value = svcmcp.ListResourcesResult{Resources: f.resources}
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
	client := svcmcp.NewClient(&mcpSkillRawCaller{t: t}, nil)
	skills, err := DiscoverMCPSkillsFromConnections(context.Background(), []svcmcp.MCPServerConnection{{
		Name:         "srv",
		Type:         svcmcp.MCPStateConnected,
		Client:       client,
		Capabilities: svcmcp.ServerCapabilities{"resources": map[string]any{}},
		Resources:    []svcmcp.Resource{{URI: "skill://review", Name: "review"}},
	}})
	if err != nil {
		t.Fatalf("DiscoverMCPSkillsFromConnections: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected no skills when gate disabled, got %#v", skills)
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
		resources: []svcmcp.Resource{
			{URI: "skill://review/SKILL.md", Name: "review", Description: "fallback"},
			{URI: "file://ignored", Name: "ignored"},
		},
		reads: map[string]svcmcp.ReadResourceResult{
			"skill://review/SKILL.md": {Contents: []svcmcp.ResourceContent{{URI: "skill://review/SKILL.md", Text: markdown, MimeType: "text/markdown"}}},
		},
	}
	client := svcmcp.NewClient(raw, nil)
	discovered, err := DiscoverMCPSkillsFromConnections(context.Background(), []svcmcp.MCPServerConnection{{
		Name:         "github",
		Type:         svcmcp.MCPStateConnected,
		Client:       client,
		Capabilities: svcmcp.ServerCapabilities{"resources": map[string]any{}},
		Resources:    raw.resources,
	}})
	if err != nil {
		t.Fatalf("DiscoverMCPSkillsFromConnections: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("skills = %#v", discovered)
	}
	skill := discovered[0]
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

func TestMCPSkillsReadBase64MarkdownAndRegisterIndependentlyFromPrompts(t *testing.T) {
	t.Setenv(FeatureFlagMCPSkills, "1")
	body := "---\ndescription: Encoded skill\n---\nencoded body"
	raw := &mcpSkillRawCaller{
		t:         t,
		resources: []svcmcp.Resource{{URI: "skill://encoded", Name: "encoded"}},
		reads: map[string]svcmcp.ReadResourceResult{
			"skill://encoded": {Contents: []svcmcp.ResourceContent{{URI: "skill://encoded", Blob: base64.StdEncoding.EncodeToString([]byte(body)), MimeType: "text/markdown"}}},
		},
	}
	client := svcmcp.NewClient(raw, nil)
	discovered, err := DiscoverMCPSkillsFromConnections(context.Background(), []svcmcp.MCPServerConnection{{
		Name:         "srv",
		Type:         svcmcp.MCPStateConnected,
		Client:       client,
		Capabilities: svcmcp.ServerCapabilities{"resources": map[string]any{}},
		Resources:    raw.resources,
	}})
	if err != nil {
		t.Fatalf("DiscoverMCPSkillsFromConnections: %v", err)
	}
	manager := NewManager()
	manager.RegisterMCPPrompts([]MCPPrompt{{Server: "srv", Name: "prompt", Description: "prompt desc", Body: "prompt body"}})
	manager.RegisterMCPSkills(discovered)

	if manager.Get("srv:prompt") == nil {
		t.Fatal("prompt-backed MCP skill should remain registered")
	}
	got := manager.Get("srv:encoded")
	if got == nil {
		t.Fatal("resource-backed MCP skill should be registered")
	}
	if got.Content != "encoded body" {
		t.Fatalf("content = %q", got.Content)
	}
	if len(manager.MCPSkills()) != 1 {
		t.Fatalf("MCPSkills snapshot = %#v", manager.MCPSkills())
	}

	InvalidateMCPSkills(manager)
	if manager.Get("srv:encoded") != nil {
		t.Fatal("expected InvalidateMCPSkills to clear resource-backed MCP skills")
	}
	if manager.Get("srv:prompt") == nil {
		t.Fatal("prompt-backed MCP skill should not be cleared by resource skill invalidation")
	}
}

func TestMCPSkillsDeriveNameFromSkillMDSiblingDirectory(t *testing.T) {
	t.Setenv(FeatureFlagMCPSkills, "1")
	raw := &mcpSkillRawCaller{
		t:         t,
		resources: []svcmcp.Resource{{URI: "skill://catalog/review/SKILL.md"}},
		reads: map[string]svcmcp.ReadResourceResult{
			"skill://catalog/review/SKILL.md": {Contents: []svcmcp.ResourceContent{{Text: "---\ndescription: Review\n---\nbody"}}},
		},
	}
	client := svcmcp.NewClient(raw, nil)
	discovered, err := DiscoverMCPSkillsFromConnections(context.Background(), []svcmcp.MCPServerConnection{{
		Name:         "srv",
		Type:         svcmcp.MCPStateConnected,
		Client:       client,
		Capabilities: svcmcp.ServerCapabilities{"resources": map[string]any{}},
		Resources:    raw.resources,
	}})
	if err != nil {
		t.Fatalf("DiscoverMCPSkillsFromConnections: %v", err)
	}
	if len(discovered) != 1 || discovered[0].Name != "srv:review" {
		t.Fatalf("discovered = %#v", discovered)
	}
}

func TestMCPSkillsSkipDisconnectedAndNoResourceCapability(t *testing.T) {
	t.Setenv(FeatureFlagMCPSkills, "1")
	raw := &mcpSkillRawCaller{t: t}
	client := svcmcp.NewClient(raw, nil)
	discovered, err := DiscoverMCPSkillsFromConnections(context.Background(), []svcmcp.MCPServerConnection{
		{Name: "pending", Type: svcmcp.MCPStatePending, Client: client, Capabilities: svcmcp.ServerCapabilities{"resources": map[string]any{}}},
		{Name: "tools-only", Type: svcmcp.MCPStateConnected, Client: client, Capabilities: svcmcp.ServerCapabilities{"tools": map[string]any{}}},
	})
	if err != nil {
		t.Fatalf("DiscoverMCPSkillsFromConnections: %v", err)
	}
	if len(discovered) != 0 {
		t.Fatalf("expected no discovered skills, got %#v", discovered)
	}
}
