package tools

import "testing"

func TestGroupAgentDefinitionsBySource_OrdersByPriority(t *testing.T) {
	defs := []AgentDefinition{
		{Name: "builtin-A", Source: "builtin"},
		{Name: "user-Z", Source: "user"},
		{Name: "project-A", Source: "project"},
		{Name: "user-A", Source: "user"},
		{Name: "plugin-A", Source: "plugin"},
	}
	groups := GroupAgentDefinitionsBySource(defs)
	if len(groups) != 4 {
		t.Fatalf("expected 4 non-empty groups, got %d", len(groups))
	}
	want := []AgentSourceGroup{
		AgentSourceProject, AgentSourceUser, AgentSourcePlugin, AgentSourceBuiltin,
	}
	for i, g := range groups {
		if g.Source != want[i] {
			t.Fatalf("groups[%d]=%s want %s", i, g.Source, want[i])
		}
	}
	userGroup := groups[1]
	if userGroup.Source != AgentSourceUser {
		t.Fatalf("expected user group at idx 1")
	}
	if len(userGroup.Definitions) != 2 {
		t.Fatalf("expected 2 user definitions, got %d", len(userGroup.Definitions))
	}
}

func TestGroupAgentDefinitionsBySource_EmptySource_FallsBackToBuiltin(t *testing.T) {
	defs := []AgentDefinition{{Name: "x"}}
	groups := GroupAgentDefinitionsBySource(defs)
	if len(groups) != 1 || groups[0].Source != AgentSourceBuiltin {
		t.Fatalf("expected single builtin group, got %+v", groups)
	}
}
