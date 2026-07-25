package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func newManagerWithOverrideStore(store OverrideStore, dirs ...DirSource) *Manager {
	manager := NewManager(dirs...)
	manager.SetOverrideStore(store)
	return manager
}

type emptyTestOverrideStore struct{}

func (emptyTestOverrideStore) Snapshot(string) (OverrideSnapshot, error) {
	return OverrideSnapshot{
		User: map[SkillID]VisibilityOverride{}, Project: map[SkillID]VisibilityOverride{},
		Managed: map[SkillID]VisibilityOverride{}, Session: map[SkillID]VisibilityOverride{},
	}, nil
}

func (emptyTestOverrideStore) Set(string, VisibilityOverride) error        { return nil }
func (emptyTestOverrideStore) Reset(string, SkillScope, SkillID) error     { return nil }
func (emptyTestOverrideStore) RestoreProject(ProjectOverrideRestore) error { return nil }
func (emptyTestOverrideStore) CompareAndSetProject(OverrideStoreRevision, SkillID, *VisibilityOverride) (ProjectOverrideRestore, error) {
	return ProjectOverrideRestore{}, ErrOverrideRevisionConflict
}

func newCatalogManagerForTest(dirs ...DirSource) *Manager {
	return newManagerWithOverrideStore(emptyTestOverrideStore{}, dirs...)
}

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

	m := newCatalogManagerForTest(DirSource{Dir: dir, Source: SourceProject})
	skill := resolvedLoaderTestSkill(t, m, "refactor")
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

	m := newCatalogManagerForTest(DirSource{Dir: dir, Source: SourceUser})
	skill := resolvedLoaderTestSkill(t, m, "greet")
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

	m := newCatalogManagerForTest(DirSource{Dir: dir, Source: SourceProject})
	skill := resolvedLoaderTestSkill(t, m, "simple")
	if skill.Description != "Skill: simple" {
		t.Errorf("Description: got %q (expected default)", skill.Description)
	}
	if !skill.HasGeneratedDescription {
		t.Fatal("default description was not marked as generated")
	}
	snapshot, err := m.Snapshot("session")
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

	m := newCatalogManagerForTest(
		DirSource{Dir: highDir, Source: SourceProject},
		DirSource{Dir: lowDir, Source: SourceUser},
	)
	skill := resolvedLoaderTestSkill(t, m, "deploy")
	if skill.Description != "From high-priority" {
		t.Errorf("expected high-priority, got: %q", skill.Description)
	}
}

func TestManager_SnapshotDiscoversAll(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		skillDir := filepath.Join(dir, name)
		os.MkdirAll(skillDir, 0755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# "+name), 0644)
	}

	m := newCatalogManagerForTest(DirSource{Dir: dir, Source: SourceProject})
	snapshot, err := m.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Skills) != 2 || !loaderSnapshotHasName(snapshot, "alpha") || !loaderSnapshotHasName(snapshot, "beta") {
		t.Fatalf("unexpected snapshot: %#v", snapshot.Skills)
	}
}

func TestManager_RefreshSnapshot(t *testing.T) {
	dir := t.TempDir()
	firstDir := filepath.Join(dir, "first")
	os.MkdirAll(firstDir, 0755)
	os.WriteFile(filepath.Join(firstDir, "SKILL.md"), []byte("First"), 0644)

	m := newCatalogManagerForTest(DirSource{Dir: dir, Source: SourceProject})
	if _, err := m.Snapshot("session"); err != nil {
		t.Fatal(err)
	}

	// Add new skill
	secondDir := filepath.Join(dir, "second")
	os.MkdirAll(secondDir, 0755)
	os.WriteFile(filepath.Join(secondDir, "SKILL.md"), []byte("Second"), 0644)

	// Should not be visible yet (cached)
	// After refresh, should be visible
	if _, err := m.RefreshSnapshot("session"); err != nil {
		t.Fatal(err)
	}
	resolvedLoaderTestSkill(t, m, "second")
}

func TestManager_NonexistentDir(t *testing.T) {
	m := newCatalogManagerForTest(DirSource{Dir: "/nonexistent/dir/12345", Source: SourceProject})
	result, err := m.ResolveLatest(SkillResolveRequest{
		SessionID: "session", Selector: "anything", Origin: InvocationOriginUser,
		ExpectedProjectGeneration: m.ProjectGeneration(),
	}, nil)
	if err != nil || result.Outcome != SkillResolveNotFound {
		t.Errorf("expected no resolution from nonexistent dir, result=%#v err=%v", result, err)
	}
}

func TestManager_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	m := newCatalogManagerForTest(DirSource{Dir: dir, Source: SourceProject})
	snapshot, err := m.Snapshot("session")
	if err != nil || len(snapshot.Skills) != 0 {
		t.Errorf("expected no skills, got %#v, err=%v", snapshot.Skills, err)
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

	m := newCatalogManagerForTest(DirSource{Dir: dir, Source: SourceProject})
	snapshot, snapshotErr := m.Snapshot("session")
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	// Should load both names since they have different skill names,
	// but one of them will be deduplicated by realpath
	// Actually: the second one (alias) should be skipped because it points
	// to the same real file as "real"
	if len(snapshot.Skills) != 1 {
		t.Errorf("expected 1 skill (dedup via symlink), got %d: %v", len(snapshot.Skills), snapshot.Skills)
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

	m := newCatalogManagerForTest(DirSource{Dir: dir, Source: SourceProject})
	// Should be findable by the frontmatter name, not the directory name
	skill := resolvedLoaderTestSkill(t, m, "custom-name")
	if skill.Description != "Overridden" {
		t.Errorf("Description: got %q", skill.Description)
	}
}

func TestSkillEffectiveDescription(t *testing.T) {
	skill := &Skill{
		Description: "Deploy code",
		WhenToUse:   "When deploying to production",
	}
	expected := "Deploy code - When deploying to production"
	if got := skill.effectiveDescription(); got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}

	skill2 := &Skill{Description: "Simple skill"}
	if got := skill2.effectiveDescription(); got != "Simple skill" {
		t.Errorf("got %q", got)
	}
}

func resolvedLoaderTestSkill(t testing.TB, manager *Manager, selector string) *Skill {
	t.Helper()
	var resolved ResolvedSkill
	result, err := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "session", Selector: selector, Origin: InvocationOriginUser,
		ExpectedProjectGeneration: manager.ProjectGeneration(),
	}, func(current ResolvedSkill) error {
		resolved = current
		return nil
	})
	if err != nil || result.Outcome != SkillResolveResolved || resolved.Skill == nil {
		t.Fatalf("ResolveLatest(%q) = %#v, resolved=%#v, err=%v", selector, result, resolved, err)
	}
	return resolved.Skill
}

func loaderSnapshotHasName(snapshot CatalogSnapshot, name string) bool {
	for _, skill := range snapshot.Skills {
		if skill.Name == name {
			return true
		}
	}
	return false
}
