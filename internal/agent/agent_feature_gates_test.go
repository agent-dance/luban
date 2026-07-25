package agent

import (
	"testing"
)

func TestFilterBuiltinAgentProfilesByFeatureGates_DefaultOn(t *testing.T) {
	profiles := []agentProfile{
		{Name: "general-purpose"},
		{Name: "Explore"},
		{Name: "Plan"},
		{Name: "verification"},
	}
	out := filterBuiltinAgentProfilesByFeatureGates(profiles)
	if len(out) != 4 {
		t.Fatalf("default should keep all 4 builtin profiles, got %d", len(out))
	}
}

func TestFilterBuiltinAgentProfilesByFeatureGates_ExploreDisabled(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_EXPLORE_PLAN_AGENTS", "1")
	profiles := []agentProfile{
		{Name: "general-purpose"},
		{Name: "Explore"},
		{Name: "Plan"},
	}
	out := filterBuiltinAgentProfilesByFeatureGates(profiles)
	if len(out) != 1 {
		t.Fatalf("expected only general-purpose to survive, got %v", names(out))
	}
	if out[0].Name != "general-purpose" {
		t.Fatalf("survivor should be general-purpose, got %q", out[0].Name)
	}
}

func TestFilterBuiltinAgentProfilesByFeatureGates_VerificationDisabled(t *testing.T) {
	t.Setenv("VERIFICATION_AGENT", "0")
	profiles := []agentProfile{
		{Name: "Verification"},
		{Name: "general-purpose"},
	}
	out := filterBuiltinAgentProfilesByFeatureGates(profiles)
	if len(out) != 1 || out[0].Name != "general-purpose" {
		t.Fatalf("expected verification dropped, got %v", names(out))
	}
}

func names(profiles []agentProfile) []string {
	out := make([]string, len(profiles))
	for i, p := range profiles {
		out[i] = p.Name
	}
	return out
}
