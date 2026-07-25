package runtimescope

import "testing"

func TestEmptyAllowedToolsClearsWhitelist(t *testing.T) {
	scope := NewRuntimeScope(t.TempDir(), true)
	scope.SetAllowedTools([]string{"Read"})
	if allowed := scope.ToolRuntimeContext().AllowedTools; !allowed["Read"] {
		t.Fatalf("explicit whitelist = %v, want Read", allowed)
	}

	scope.SetAllowedTools([]string{"   "})
	if allowed := scope.ToolRuntimeContext().AllowedTools; allowed != nil {
		t.Fatalf("empty normalized whitelist = %v, want nil (unrestricted)", allowed)
	}
}
