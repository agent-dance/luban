package config

import "testing"

func TestUnsupportedOriginalPromptSettingsDocumented(t *testing.T) {
	got := UnsupportedOriginalPromptSettings()
	if len(got) == 0 {
		t.Fatal("unsupported original prompt settings should be documented")
	}
	seen := map[string]bool{}
	for _, id := range got {
		if id == "" {
			t.Fatal("unsupported setting id must not be empty")
		}
		if seen[id] {
			t.Fatalf("duplicate unsupported setting id %q", id)
		}
		seen[id] = true
	}
}
