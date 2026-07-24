package commands_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/skills"
)

func TestSkillsCommand_RegisteredAndListsCatalogMetadata(t *testing.T) {
	dir := t.TempDir()
	path := writeCommandSkill(t, dir, "review", `---
description: Review a change carefully
allowed-tools: Read, Grep
---
# Review
`)
	manager := skills.NewManager(skills.DirSource{Dir: dir, Source: skills.SourceProject})
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	command := registry.Find("skills")
	if command == nil {
		t.Fatal("/skills is not registered")
	}

	output := executeSkillsCommand(t, command, manager, "session-a", "")
	for _, want := range []string{
		"1 discovered, 1 enabled, 0 disabled",
		"[enabled] review",
		"Summary: Review a change carefully",
		"Source: project",
		path,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("/skills list omitted %q:\n%s", want, output)
		}
	}
}

func TestSkillsCommand_ShowIncludesDetailedMetadata(t *testing.T) {
	dir := t.TempDir()
	writeCommandSkill(t, dir, "review", `---
description: Review a change carefully
allowed-tools: Read, Grep
model: sonnet
version: 2.1
disable-model-invocation: true
---
# Review
`)
	manager := skills.NewManager(skills.DirSource{Dir: dir, Source: skills.SourceUser})
	command := commands.NewSkillsCommand(manager)

	output := executeSkillsCommand(t, command, manager, "session-a", "show review")
	for _, want := range []string{
		"Skill: review",
		"Status: enabled",
		"Source: user",
		"Model invocation: disabled by frontmatter",
		"Model override: sonnet",
		"Version: 2.1",
		"Allowed tools: Read, Grep",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("/skills show omitted %q:\n%s", want, output)
		}
	}
}

func TestSkillsCommand_ToggleIsImmediateIdempotentAndSessionScoped(t *testing.T) {
	dir := t.TempDir()
	writeCommandSkill(t, dir, "review", "# Review\n")
	manager := skills.NewManager(skills.DirSource{Dir: dir, Source: skills.SourceProject})
	command := commands.NewSkillsCommand(manager)

	output := executeSkillsCommand(t, command, manager, "session-a", "disable review")
	if !strings.Contains(output, `Disabled skill "review"`) || manager.IsEnabled("session-a", "review") {
		t.Fatalf("disable did not apply immediately: output=%q enabled=%t", output, manager.IsEnabled("session-a", "review"))
	}
	if !manager.IsEnabled("session-b", "review") {
		t.Fatal("session-a disable leaked into session-b")
	}

	output = executeSkillsCommand(t, command, manager, "session-a", "disable review")
	if !strings.Contains(output, "already disabled") {
		t.Fatalf("idempotent disable output = %q", output)
	}

	output = executeSkillsCommand(t, command, manager, "session-a", "enable review")
	if !strings.Contains(output, `Enabled skill "review"`) || !manager.IsEnabled("session-a", "review") {
		t.Fatalf("enable did not apply: output=%q enabled=%t", output, manager.IsEnabled("session-a", "review"))
	}
}

func TestSkillsCommand_ToggleAllAndUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	writeCommandSkill(t, dir, "alpha", "# Alpha\n")
	writeCommandSkill(t, dir, "beta", "# Beta\n")
	manager := skills.NewManager(skills.DirSource{Dir: dir, Source: skills.SourceProject})
	command := commands.NewSkillsCommand(manager)

	output := executeSkillsCommand(t, command, manager, "session-a", "disable all")
	if !strings.Contains(output, "Disabled 2 skill(s)") || manager.IsEnabled("session-a", "alpha") || manager.IsEnabled("session-a", "beta") {
		t.Fatalf("disable all failed: %q", output)
	}
	output = executeSkillsCommand(t, command, manager, "session-a", "enable all")
	if !strings.Contains(output, "Enabled 2 skill(s)") || !manager.IsEnabled("session-a", "alpha") || !manager.IsEnabled("session-a", "beta") {
		t.Fatalf("enable all failed: %q", output)
	}

	output = executeSkillsCommand(t, command, manager, "session-a", "disable missing")
	if !strings.Contains(output, `Skill "missing" not found`) {
		t.Fatalf("unknown target output = %q", output)
	}
}

func TestSkillsCommand_RequiresLiveBackend(t *testing.T) {
	command := commands.NewSkillsCommand(nil)
	err := command.Execute(&commands.Context{}, "list")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing backend error = %v", err)
	}
}

func executeSkillsCommand(t *testing.T, command commands.Command, manager *skills.Manager, sessionID, args string) string {
	t.Helper()
	var output strings.Builder
	ctx := &commands.Context{
		SessionID:    sessionID,
		SkillManager: manager,
		OnEvent:      func(value string) { output.WriteString(value) },
	}
	if err := command.Execute(ctx, args); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func writeCommandSkill(t *testing.T, root, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
