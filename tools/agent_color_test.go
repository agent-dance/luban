package tools

import "testing"

func TestAgentColor_GeneralPurposeNoOverride(t *testing.T) {
	ResetAgentColorMap()
	if got := GetAgentColor("general-purpose"); got != "" {
		t.Fatalf("general-purpose should return no color, got %q", got)
	}
	if got := AssignAgentColor("general-purpose"); got != "" {
		t.Fatalf("AssignAgentColor general-purpose should return no color, got %q", got)
	}
	if snap := AgentColorMapSnapshot(); len(snap) != 0 {
		t.Fatalf("general-purpose should not be persisted: %v", snap)
	}
}

func TestAgentColor_PersistAndRoundRobin(t *testing.T) {
	ResetAgentColorMap()
	c1 := AssignAgentColor("explorer")
	if c1 == "" {
		t.Fatalf("AssignAgentColor returned empty for explorer")
	}
	c2 := AssignAgentColor("planner")
	if c2 == c1 {
		t.Fatalf("expected distinct colors for first two agents, got both %q", c1)
	}
	if again := GetAgentColor("explorer"); again != c1 {
		t.Fatalf("GetAgentColor explorer should be stable %q, got %q", c1, again)
	}
}

func TestAgentColor_SetAndDelete(t *testing.T) {
	ResetAgentColorMap()
	SetAgentColor("alpha", "purple")
	if got := GetAgentColor("alpha"); got != "purple" {
		t.Fatalf("expected purple, got %q", got)
	}
	// invalid color -> ignored
	SetAgentColor("beta", "magenta")
	if got := GetAgentColor("beta"); got != "" {
		t.Fatalf("invalid color should be ignored, got %q", got)
	}
	// empty color -> delete
	SetAgentColor("alpha", "")
	if got := GetAgentColor("alpha"); got != "" {
		t.Fatalf("alpha should be cleared, got %q", got)
	}
}

func TestAgentColor_PaletteSizeIsEight(t *testing.T) {
	if len(AgentPaletteColors) != 8 {
		t.Fatalf("expected 8 palette colors, got %d", len(AgentPaletteColors))
	}
}
