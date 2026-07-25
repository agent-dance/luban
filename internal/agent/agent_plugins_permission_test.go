package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePluginAgentProfileRejectsUnknownFrontmatterField(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "reviewer.md")
	content := "---\nname: reviewer\nunknownField: value\n---\nReview the code.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write plugin agent: %v", err)
	}

	_, ok, err := parsePluginAgentProfileFile(path, pluginAgentRoot{Name: "demo"}, nil)
	if ok {
		t.Fatal("unknown field should not produce a usable plugin agent profile")
	}
	if err == nil || !strings.Contains(err.Error(), "unknownField") {
		t.Fatalf("expected strict frontmatter error, got %v", err)
	}
}

func TestParsePluginAgentProfileCanonicalFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "reviewer.md")
	content := "---\nname: reviewer\ndescription: Reviews code\nmodel: sonnet\n---\nReview the code.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write plugin agent: %v", err)
	}

	profile, ok, err := parsePluginAgentProfileFile(path, pluginAgentRoot{Name: "demo"}, nil)
	if err != nil || !ok {
		t.Fatalf("parse ordinary plugin agent ok=%v err=%v", ok, err)
	}
	if profile.Name != "demo:reviewer" {
		t.Fatalf("profile name = %q, want %q", profile.Name, "demo:reviewer")
	}
	if profile.SystemPrefix != "Review the code." {
		t.Fatalf("profile system prefix = %q", profile.SystemPrefix)
	}
}

func TestLoadPluginAgentProfilePropagatesUnknownFieldError(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	agentsPath := filepath.Join(rootPath, "agents")
	if err := os.MkdirAll(agentsPath, 0o700); err != nil {
		t.Fatalf("create plugin agents directory: %v", err)
	}
	path := filepath.Join(agentsPath, "reviewer.md")
	content := "---\nname: reviewer\nunknownField: value\n---\nReview the code.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write plugin agent: %v", err)
	}

	_, ok, err := loadPluginAgentProfileFromRoot(pluginAgentRoot{Name: "demo", Path: rootPath}, "demo:reviewer")
	if ok {
		t.Fatal("unknown field should not produce a usable plugin agent profile")
	}
	if err == nil || !strings.Contains(err.Error(), "unknownField") {
		t.Fatalf("expected strict frontmatter error, got %v", err)
	}
}
