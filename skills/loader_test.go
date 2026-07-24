package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager_LoadDirectorySkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "refactor")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
description: Refactor code
allowed-tools: Read, Write, Edit
model: sonnet
---
# Refactor

Refactor the code carefully.
`), 0644)

	m := NewManager(DirSource{Dir: dir, Source: SourceProject})
	skill := m.Get("refactor")
	if skill == nil {
		t.Fatal("expected 'refactor' skill")
	}
	if skill.Description != "Refactor code" {
		t.Errorf("Description: got %q", skill.Description)
	}
	if skill.HasGeneratedDescription {
		t.Error("frontmatter description was marked as generated")
	}
	if len(skill.AllowedTools) != 3 {
		t.Errorf("AllowedTools: got %v", skill.AllowedTools)
	}
	if skill.Model != "sonnet" {
		t.Errorf("Model: got %q", skill.Model)
	}
	if skill.Source != SourceProject {
		t.Errorf("Source: got %q", skill.Source)
	}
	if skill.Content == "" || skill.Content == skill.RawContent {
		t.Errorf("Content should be stripped of frontmatter")
	}
}

func TestManager_LoadFileSkill_DirectoryFormat(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
description: Greet the user
when_to_use: When user says hello
---
Say hello!
`), 0644)

	m := NewManager(DirSource{Dir: dir, Source: SourceUser})
	skill := m.Get("greet")
	if skill == nil {
		t.Fatal("expected 'greet' skill")
	}
	if skill.Description != "Greet the user" {
		t.Errorf("Description: got %q", skill.Description)
	}
	if skill.WhenToUse != "When user says hello" {
		t.Errorf("WhenToUse: got %q", skill.WhenToUse)
	}
	if skill.Source != SourceUser {
		t.Errorf("Source: got %q", skill.Source)
	}
}

func TestManager_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "simple")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Simple\nJust do it."), 0644)

	m := NewManager(DirSource{Dir: dir, Source: SourceProject})
	skill := m.Get("simple")
	if skill == nil {
		t.Fatal("expected 'simple' skill")
	}
	if skill.Description != "Skill: simple" {
		t.Errorf("Description: got %q (expected default)", skill.Description)
	}
	if !skill.HasGeneratedDescription {
		t.Fatal("default description was not marked as generated")
	}
	snapshot, err := m.Snapshot("")
	if err != nil || len(snapshot.Skills) != 1 {
		t.Fatalf("Snapshot() = %#v, %v", snapshot, err)
	}
	if !snapshot.Skills[0].SummaryGenerated {
		t.Fatal("catalog summary lost its generated-copy marker")
	}
}

func TestManager_PriorityFirstDirWins(t *testing.T) {
	highDir := t.TempDir()
	lowDir := t.TempDir()

	// Same skill name in both dirs (directory format)
	highSkillDir := filepath.Join(highDir, "deploy")
	os.MkdirAll(highSkillDir, 0755)
	os.WriteFile(filepath.Join(highSkillDir, "SKILL.md"), []byte(`---
description: From high-priority
---
high
`), 0644)

	lowSkillDir := filepath.Join(lowDir, "deploy")
	os.MkdirAll(lowSkillDir, 0755)
	os.WriteFile(filepath.Join(lowSkillDir, "SKILL.md"), []byte(`---
description: From low-priority
---
low
`), 0644)

	m := NewManager(
		DirSource{Dir: highDir, Source: SourceProject},
		DirSource{Dir: lowDir, Source: SourceUser},
	)
	skill := m.Get("deploy")
	if skill == nil {
		t.Fatal("expected 'deploy' skill")
	}
	if skill.Description != "From high-priority" {
		t.Errorf("expected high-priority, got: %q", skill.Description)
	}
}

func TestManager_All(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		skillDir := filepath.Join(dir, name)
		os.MkdirAll(skillDir, 0755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# "+name), 0644)
	}

	m := NewManager(DirSource{Dir: dir, Source: SourceProject})
	all := m.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(all))
	}
	// Should be sorted
	if all[0].Name != "alpha" || all[1].Name != "beta" {
		t.Errorf("unexpected order: %s, %s", all[0].Name, all[1].Name)
	}
}

func TestManager_Names(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z", "a"} {
		skillDir := filepath.Join(dir, name)
		os.MkdirAll(skillDir, 0755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(name), 0644)
	}

	m := NewManager(DirSource{Dir: dir, Source: SourceProject})
	names := m.Names()
	if len(names) != 2 || names[0] != "a" || names[1] != "z" {
		t.Errorf("expected sorted [a, z], got %v", names)
	}
}

func TestManager_SessionAvailabilityIsIsolated(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		skillDir := filepath.Join(dir, name)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	m := NewManager(DirSource{Dir: dir, Source: SourceProject})
	if changed, found := m.SetEnabled("session-a", "alpha", false); !found || !changed {
		t.Fatalf("disable alpha = changed %t found %t, want true/true", changed, found)
	}
	if m.IsEnabled("session-a", "alpha") {
		t.Fatal("alpha remained enabled in session-a")
	}
	if !m.IsEnabled("session-b", "alpha") {
		t.Fatal("session-a override leaked into session-b")
	}
	if got := m.EnabledNames("session-a"); len(got) != 1 || got[0] != "beta" {
		t.Fatalf("session-a enabled names = %v, want [beta]", got)
	}

	if changed, found := m.SetEnabled("session-a", "alpha", false); !found || changed {
		t.Fatalf("idempotent disable = changed %t found %t, want false/true", changed, found)
	}
	if changed, found := m.SetEnabled("session-a", "missing", false); found || changed {
		t.Fatalf("unknown disable = changed %t found %t, want false/false", changed, found)
	}
}

func TestManager_SetAllEnabledAndRefreshPreserveSessionPolicy(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		skillDir := filepath.Join(dir, name)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	m := NewManager(DirSource{Dir: dir, Source: SourceProject})
	if changed := m.SetAllEnabled("session-a", false); changed != 2 {
		t.Fatalf("disabled count = %d, want 2", changed)
	}
	m.Refresh()
	if got := m.EnabledNames("session-a"); len(got) != 0 {
		t.Fatalf("refresh silently re-enabled skills: %v", got)
	}
	if changed := m.SetAllEnabled("session-a", true); changed != 2 {
		t.Fatalf("enabled count = %d, want 2", changed)
	}
	if got := m.EnabledNames("session-a"); len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("enabled names = %v, want [alpha beta]", got)
	}
}

func TestManager_Refresh(t *testing.T) {
	dir := t.TempDir()
	firstDir := filepath.Join(dir, "first")
	os.MkdirAll(firstDir, 0755)
	os.WriteFile(filepath.Join(firstDir, "SKILL.md"), []byte("First"), 0644)

	m := NewManager(DirSource{Dir: dir, Source: SourceProject})
	m.Get("first") // populate

	// Add new skill
	secondDir := filepath.Join(dir, "second")
	os.MkdirAll(secondDir, 0755)
	os.WriteFile(filepath.Join(secondDir, "SKILL.md"), []byte("Second"), 0644)

	// Should not be visible yet (cached)
	// After refresh, should be visible
	m.Refresh()
	if m.Get("second") == nil {
		t.Error("expected 'second' to be visible after Refresh()")
	}
}

func TestManager_NonexistentDir(t *testing.T) {
	m := NewManager(DirSource{Dir: "/nonexistent/dir/12345", Source: SourceProject})
	if skill := m.Get("anything"); skill != nil {
		t.Errorf("expected nil from nonexistent dir, got %+v", skill)
	}
}

func TestManager_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(DirSource{Dir: dir, Source: SourceProject})
	if names := m.Names(); len(names) != 0 {
		t.Errorf("expected no skills, got %v", names)
	}
}

func TestManager_SymlinkDedup(t *testing.T) {
	dir := t.TempDir()
	// Create a real skill directory
	realDir := filepath.Join(dir, "real")
	os.MkdirAll(realDir, 0755)
	os.WriteFile(filepath.Join(realDir, "SKILL.md"), []byte(`---
description: Real skill
---
Content
`), 0644)

	// Create a symlink to the same directory
	linkPath := filepath.Join(dir, "alias")
	err := os.Symlink(realDir, linkPath)
	if err != nil {
		t.Skip("symlink not supported:", err)
	}

	m := NewManager(DirSource{Dir: dir, Source: SourceProject})
	all := m.All()
	// Should load both names since they have different skill names,
	// but one of them will be deduplicated by realpath
	// Actually: the second one (alias) should be skipped because it points
	// to the same real file as "real"
	if len(all) != 1 {
		t.Errorf("expected 1 skill (dedup via symlink), got %d: %v", len(all), m.Names())
	}
}

func TestManager_FrontmatterNameOverride(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "filename")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: custom-name
description: Overridden
---
content
`), 0644)

	m := NewManager(DirSource{Dir: dir, Source: SourceProject})
	// Should be findable by the frontmatter name, not the directory name
	skill := m.Get("custom-name")
	if skill == nil {
		t.Fatal("expected skill with frontmatter name 'custom-name'")
	}
	if skill.Description != "Overridden" {
		t.Errorf("Description: got %q", skill.Description)
	}
}

func TestManager_AddDir(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	aDir := filepath.Join(dir1, "a")
	os.MkdirAll(aDir, 0755)
	os.WriteFile(filepath.Join(aDir, "SKILL.md"), []byte("A"), 0644)

	bDir := filepath.Join(dir2, "b")
	os.MkdirAll(bDir, 0755)
	os.WriteFile(filepath.Join(bDir, "SKILL.md"), []byte("B"), 0644)

	m := NewManager(DirSource{Dir: dir1, Source: SourceProject})
	if m.Get("a") == nil {
		t.Fatal("expected 'a'")
	}

	m.AddDir(dir2, SourceUser)
	if m.Get("b") == nil {
		t.Fatal("expected 'b' after AddDir")
	}
}

func TestManager_EffectiveDescription(t *testing.T) {
	skill := &Skill{
		Description: "Deploy code",
		WhenToUse:   "When deploying to production",
	}
	expected := "Deploy code - When deploying to production"
	if got := skill.EffectiveDescription(); got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}

	skill2 := &Skill{Description: "Simple skill"}
	if got := skill2.EffectiveDescription(); got != "Simple skill" {
		t.Errorf("got %q", got)
	}
}
