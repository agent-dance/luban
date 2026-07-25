package skills

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type task23TypedNilSessionLayer struct{}

func (*task23TypedNilSessionLayer) Snapshot(string) (map[SkillID]VisibilityOverride, error) {
	panic("typed nil session layer must be replaced")
}
func (*task23TypedNilSessionLayer) Set(string, VisibilityOverride) error {
	panic("typed nil session layer must be replaced")
}
func (*task23TypedNilSessionLayer) Reset(string, SkillID) error {
	panic("typed nil session layer must be replaced")
}

func TestFileOverrideStoreRejectsTypedNilSessionLayer(t *testing.T) {
	var pointer *task23TypedNilSessionLayer
	var injected SessionOverrideLayer = pointer
	root := t.TempDir()
	if _, err := NewFileOverrideStoreAt(OverrideStorePaths{
		UserSettings: filepath.Join(root, "user.json"), ProjectSettings: filepath.Join(root, "project.json"),
	}, nil, injected); err == nil {
		t.Fatal("typed nil session layer was accepted")
	}
}

func TestSessionOverrideReplaceSessionIsAtomicAndDefensive(t *testing.T) {
	layer := NewMemorySessionOverrideLayer()
	first := SkillID("skill:project:first")
	second := SkillID("skill:project:second")
	old := map[SkillID]VisibilityOverride{
		first: {SkillID: first, Scope: SkillScopeSession, Visibility: VisibilityOff},
	}
	if err := layer.ReplaceSession("session-a", old); err != nil {
		t.Fatalf("install old session overlay: %v", err)
	}
	old[first] = VisibilityOverride{SkillID: first, Scope: SkillScopeSession, Visibility: VisibilityAuto}

	invalid := map[SkillID]VisibilityOverride{
		second: {SkillID: second, Scope: SkillScopeProject, Visibility: VisibilityOff},
	}
	if err := layer.ReplaceSession("session-a", invalid); err == nil {
		t.Fatal("invalid replacement unexpectedly succeeded")
	}
	got, err := layer.Snapshot("session-a")
	if err != nil || got[first].Visibility != VisibilityOff || len(got) != 1 {
		t.Fatalf("failed replacement changed live state: got=%+v err=%v", got, err)
	}

	next := map[SkillID]VisibilityOverride{
		first:  {SkillID: first, Scope: SkillScopeSession, Visibility: VisibilityManualOnly},
		second: {SkillID: second, Scope: SkillScopeSession, Visibility: VisibilityNameOnly},
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if replaceErr := layer.ReplaceSession("session-a", next); replaceErr != nil {
			t.Errorf("replace session overlay: %v", replaceErr)
		}
	}()
	for i := 0; i < 100; i++ {
		snapshot, snapshotErr := layer.Snapshot("session-a")
		if snapshotErr != nil {
			t.Fatalf("snapshot session overlay: %v", snapshotErr)
		}
		oldShape := len(snapshot) == 1 && snapshot[first].Visibility == VisibilityOff
		newShape := len(snapshot) == 2 && snapshot[first].Visibility == VisibilityManualOnly && snapshot[second].Visibility == VisibilityNameOnly
		if !oldShape && !newShape {
			t.Fatalf("observed partial session replacement: %+v", snapshot)
		}
	}
	wg.Wait()

	if err := layer.ReplaceSession("session-a", nil); err != nil {
		t.Fatalf("clear session overlay: %v", err)
	}
	if cleared, clearErr := layer.Snapshot("session-a"); clearErr != nil || len(cleared) != 0 {
		t.Fatalf("session overlay not cleared: got=%+v err=%v", cleared, clearErr)
	}
}

func TestManagerReplaceProjectSourcesPreservesNonProjectAndRetargetsStore(t *testing.T) {
	initial := t.TempDir()
	target := t.TempDir()
	pluginRoot := t.TempDir()
	writeTask23Skill(t, filepath.Join(initial, ".luban-code", "skills"), "old-project", "old project")
	writeTask23Skill(t, filepath.Join(target, ".luban-code", "skills"), "new-project", "new project")
	writeTask23Skill(t, pluginRoot, "shared-plugin", "plugin")

	layer := NewMemorySessionOverrideLayer()
	store, err := NewFileOverrideStore(initial, nil, layer)
	if err != nil {
		t.Fatalf("create override store: %v", err)
	}
	initialDirs, err := ProjectDirs(initial)
	if err != nil {
		t.Fatalf("initial project dirs: %v", err)
	}
	manager := newManagerWithOverrideStore(store, append(initialDirs, DirSource{Dir: pluginRoot, Source: SourcePlugin})...)

	before, err := manager.Snapshot("session-a")
	if err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}
	if !task23HasSkill(before, "old-project") || !task23HasSkill(before, "shared-plugin") {
		t.Fatalf("initial discovery mismatch: %+v", before.Skills)
	}
	if err := manager.ReplaceProjectSources(target); err != nil {
		t.Fatalf("replace project sources: %v", err)
	}
	after, err := manager.Snapshot("session-a")
	if err != nil {
		t.Fatalf("target snapshot: %v", err)
	}
	if task23HasSkill(after, "old-project") || !task23HasSkill(after, "new-project") || !task23HasSkill(after, "shared-plugin") {
		t.Fatalf("workspace retarget leaked or dropped sources: %+v", after.Skills)
	}
	var targetRow EffectiveSkill
	for _, row := range after.Skills {
		if row.Name == "new-project" {
			targetRow = row
			break
		}
	}
	result, err := manager.ToggleProjectVisibility("session-a", targetRow.ID, after.Revision)
	if err != nil || result.Outcome != ProjectVisibilityToggleCommitted {
		t.Fatalf("toggle target project visibility: result=%+v err=%v", result, err)
	}
	targetSettings := filepath.Join(target, ".luban-code", "settings.json")
	if _, err := os.Stat(targetSettings); err != nil {
		t.Fatalf("target project override was not persisted at %q: %v", targetSettings, err)
	}
	if _, err := os.Stat(filepath.Join(initial, ".luban-code", "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old project settings changed after retarget: %v", err)
	}
}

func TestManagerReplaceProjectSourcesFailsClosedWithoutOverrideStore(t *testing.T) {
	initial := t.TempDir()
	target := t.TempDir()
	writeTask23Skill(t, filepath.Join(initial, ".luban-code", "skills"), "old-project", "old project")
	writeTask23Skill(t, filepath.Join(target, ".luban-code", "skills"), "new-project", "new project")
	dirs, err := ProjectDirs(initial)
	if err != nil {
		t.Fatalf("initial project dirs: %v", err)
	}
	manager := NewManager(dirs...)
	if err := manager.ReplaceProjectSources(target); !errors.Is(err, ErrSkillOverrideStoreMissing) {
		t.Fatalf("replace without store error = %v, want %v", err, ErrSkillOverrideStoreMissing)
	}
	snapshot, err := manager.Snapshot("session-a")
	if !errors.Is(err, ErrSkillOverrideStoreMissing) || len(snapshot.Skills) != 0 {
		t.Fatalf("snapshot without store = %+v, %v", snapshot.Skills, err)
	}
}

func TestProjectSourcePlanRejectsExternalSettingsChangeBeforePublicationAndRetries(t *testing.T) {
	initial := t.TempDir()
	target := t.TempDir()
	writeTask23Skill(t, filepath.Join(initial, ".luban-code", "skills"), "old-project", "old project")
	writeTask23Skill(t, filepath.Join(target, ".luban-code", "skills"), "new-project", "new project")
	store, err := NewFileOverrideStore(initial, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatalf("create override store: %v", err)
	}
	dirs, err := ProjectDirs(initial)
	if err != nil {
		t.Fatalf("initial project dirs: %v", err)
	}
	manager := newManagerWithOverrideStore(store, dirs...)
	if _, err := manager.Snapshot("session-a"); err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}
	plan, err := manager.PrepareProjectSources(target)
	if err != nil {
		t.Fatalf("prepare target project: %v", err)
	}
	settings := filepath.Join(target, ".luban-code", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatalf("create target settings dir: %v", err)
	}
	if err := os.WriteFile(settings, []byte("{\"unrelated\":true}\n"), 0o600); err != nil {
		t.Fatalf("change target settings after prepare: %v", err)
	}
	if err := manager.ApplyProjectSources(plan); !errors.Is(err, ErrOverrideRevisionConflict) {
		t.Fatalf("prepare/apply race error = %v, want revision conflict", err)
	}
	unchanged, err := manager.Snapshot("session-a")
	if err != nil || !task23HasSkill(unchanged, "old-project") || task23HasSkill(unchanged, "new-project") {
		t.Fatalf("rejected target changed catalog = %+v, err=%v", unchanged.Skills, err)
	}

	// A fresh staged plan observes the new revision and explicitly recovers.
	reprepared, err := manager.PrepareProjectSources(target)
	if err != nil {
		t.Fatalf("reprepare target project: %v", err)
	}
	if err := manager.ApplyProjectSources(reprepared); err != nil {
		t.Fatalf("apply reparsed target: %v", err)
	}
	snapshot, err := manager.Snapshot("session-a")
	if err != nil || !task23HasSkill(snapshot, "new-project") || task23HasSkill(snapshot, "old-project") {
		t.Fatalf("reprepared target snapshot = %+v, err=%v", snapshot.Skills, err)
	}
}

func writeTask23Skill(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\nbody"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func task23HasSkill(snapshot CatalogSnapshot, name string) bool {
	for _, row := range snapshot.Skills {
		if row.Name == name {
			return true
		}
	}
	return false
}
