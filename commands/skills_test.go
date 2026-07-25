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
	manager := newCommandSkillsManager(t, skills.DirSource{Dir: dir, Source: skills.SourceProject})
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
	manager := newCommandSkillsManager(t, skills.DirSource{Dir: dir, Source: skills.SourceUser})
	command := commands.NewSkillsCommand()

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

func TestSkillsCommand_RequiresLiveBackend(t *testing.T) {
	command := commands.NewSkillsCommand()
	err := command.Execute(&commands.Context{}, "list")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing backend error = %v", err)
	}
}

func executeSkillsCommand(t *testing.T, command commands.Command, manager *skills.Manager, sessionID, args string) string {
	t.Helper()
	var output strings.Builder
	ctx := &commands.Context{
		SessionID:             sessionID,
		SkillManager:          manager,
		OnEvent:               func(value string) { output.WriteString(value) },
		OnCommandPresentation: captureCompletedCommand(&output),
	}
	if err := command.Execute(ctx, args); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func newCommandSkillsManager(t *testing.T, source skills.DirSource) *skills.Manager {
	t.Helper()
	root := t.TempDir()
	store, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings: filepath.Join(root, "user-settings.json"), ProjectSettings: filepath.Join(root, "project-settings.json"),
	}, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(source)
	manager.SetOverrideStore(store)
	return manager
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
