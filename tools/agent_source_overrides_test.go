package tools

import "testing"

func TestResolveAgentOverrides_ProjectBeatsAll(t *testing.T) {
	defs := []AgentDefinition{
		{Name: "explore", Source: string(AgentSourceBuiltin), Model: "builtin"},
		{Name: "explore", Source: string(AgentSourcePlugin), Model: "plugin"},
		{Name: "explore", Source: string(AgentSourceUser), Model: "user"},
		{Name: "explore", Source: string(AgentSourceProject), Model: "project"},
	}
	out := ResolveAgentOverrides(defs)
	if len(out) != 1 {
		t.Fatalf("expected 1 entry after dedup, got %d", len(out))
	}
	if out[0].Model != "project" {
		t.Fatalf("expected project to win, got %q", out[0].Model)
	}
}

func TestResolveAgentOverrides_OrderPreserved(t *testing.T) {
	defs := []AgentDefinition{
		{Name: "alpha", Source: string(AgentSourceBuiltin)},
		{Name: "beta", Source: string(AgentSourceUser)},
		{Name: "gamma", Source: string(AgentSourceProject)},
	}
	out := ResolveAgentOverrides(defs)
	if len(out) != 3 || out[0].Name != "alpha" || out[1].Name != "beta" || out[2].Name != "gamma" {
		t.Fatalf("expected stable insertion order, got %+v", out)
	}
}

func TestResolveAgentOverrides_UnknownSourceCannotShadow(t *testing.T) {
	defs := []AgentDefinition{
		{Name: "x", Source: string(AgentSourceBuiltin), Model: "builtin"},
		{Name: "x", Source: "weird-future-tier", Model: "weird"},
	}
	out := ResolveAgentOverrides(defs)
	if len(out) != 1 || out[0].Model != "builtin" {
		t.Fatalf("expected builtin to win over unknown source, got %+v", out)
	}
}

func TestAgentSourceRank_PriorityOrder(t *testing.T) {
	if agentSourceRank(string(AgentSourceProject)) >= agentSourceRank(string(AgentSourceUser)) {
		t.Fatalf("project should outrank user")
	}
	if agentSourceRank(string(AgentSourceUser)) >= agentSourceRank(string(AgentSourcePlugin)) {
		t.Fatalf("user should outrank plugin")
	}
	if agentSourceRank(string(AgentSourcePlugin)) >= agentSourceRank(string(AgentSourceBuiltin)) {
		t.Fatalf("plugin should outrank builtin")
	}
}

func TestGroupAgentDefinitionsBySource_PriorityOrderedAndStable(t *testing.T) {
	defs := []AgentDefinition{
		{Name: "verify", Source: string(AgentSourceBuiltin)},
		{Name: "explore", Source: string(AgentSourceProject)},
		{Name: "plan", Source: string(AgentSourceProject)},
		{Name: "guide", Source: string(AgentSourceUser)},
		{Name: "manage", Source: string(AgentSourceManaged)},
	}
	groups := GroupAgentDefinitionsBySource(defs)
	if len(groups) != 4 {
		t.Fatalf("expected 4 non-empty groups, got %d", len(groups))
	}
	if groups[0].Source != AgentSourceProject {
		t.Fatalf("first group should be project, got %s", groups[0].Source)
	}
	if len(groups[0].Definitions) != 2 || groups[0].Definitions[0].Name != "explore" || groups[0].Definitions[1].Name != "plan" {
		t.Fatalf("project group lost insertion order: %+v", groups[0].Definitions)
	}
	if groups[1].Source != AgentSourceUser || groups[1].Definitions[0].Name != "guide" {
		t.Fatalf("expected user group with guide, got %+v", groups[1])
	}
	if groups[2].Source != AgentSourceManaged {
		t.Fatalf("expected managed third, got %s", groups[2].Source)
	}
	if groups[3].Source != AgentSourceBuiltin || groups[3].Definitions[0].Name != "verify" {
		t.Fatalf("expected builtin last with verify, got %+v", groups[3])
	}
}

func TestGroupAgentDefinitionsBySource_EmptyInput(t *testing.T) {
	if got := GroupAgentDefinitionsBySource(nil); len(got) != 0 {
		t.Fatalf("expected no groups for nil input, got %v", got)
	}
}
