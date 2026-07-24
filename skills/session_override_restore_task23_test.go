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

func TestFileOverrideStoreReplacesGenericTypedNilSessionLayer(t *testing.T) {
	newStore := func() *FileOverrideStore {
		var pointer *task23TypedNilSessionLayer
		var injected SessionOverrideLayer = pointer
		root := t.TempDir()
		store, err := NewFileOverrideStoreAt(OverrideStorePaths{
			UserSettings: filepath.Join(root, "user.json"), ProjectSettings: filepath.Join(root, "project.json"),
		}, nil, injected)
		if err != nil {
			t.Fatalf("construct store with typed nil session layer: %v", err)
		}
		return store
	}
	first := newStore()
	second := newStore()
	id := SkillID("skill:project:typed-nil")
	if _, err := first.Snapshot("session-a"); err != nil {
		t.Fatalf("snapshot typed-nil replacement: %v", err)
	}
	if err := first.Set("session-a", VisibilityOverride{
		SkillID: id, Scope: SkillScopeSession, Visibility: VisibilityManualOnly,
	}); err != nil {
		t.Fatalf("set typed-nil replacement: %v", err)
	}
	firstSnapshot, err := first.Snapshot("session-a")
	if err != nil || firstSnapshot.Session[id].Visibility != VisibilityManualOnly {
		t.Fatalf("first replacement snapshot = %+v, err=%v", firstSnapshot.Session, err)
	}
	secondSnapshot, err := second.Snapshot("session-a")
	if err != nil || len(secondSnapshot.Session) != 0 {
		t.Fatalf("typed-nil replacements share session state: %+v, err=%v", secondSnapshot.Session, err)
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
	manager := NewManagerWithOverrideStore(store, append(initialDirs, DirSource{Dir: pluginRoot, Source: SourcePlugin})...)

	before, err := manager.Snapshot("session-a")
	if err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}
	if !task23HasSkill(before, "old-project") || !task23HasSkill(before, "shared-plugin") {
		t.Fatalf("initial discovery mismatch: %+v", before.Skills)
	}
	manager.ActivateConditionalSkill("old-project")
	if len(manager.ActivatedConditionalSkillNames()) != 1 {
		t.Fatal("failed to establish old-workspace conditional activation fixture")
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
	if activated := manager.ActivatedConditionalSkillNames(); len(activated) != 0 {
		t.Fatalf("old workspace conditional activation leaked after retarget: %v", activated)
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
	if err != nil {
		t.Fatalf("snapshot after rejected switch: %v", err)
	}
	if !task23HasSkill(snapshot, "old-project") || task23HasSkill(snapshot, "new-project") {
		t.Fatalf("rejected switch changed project discovery: %+v", snapshot.Skills)
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
	manager := NewManagerWithOverrideStore(store, dirs...)
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

func TestProjectSourcePlanClearsOnlyProjectOwnedLegacyDisables(t *testing.T) {
	initial := t.TempDir()
	target := t.TempDir()
	userRoot := t.TempDir()
	writeTask23Skill(t, filepath.Join(initial, ".luban-code", "skills"), "shared-name", "old project")
	writeTask23Skill(t, filepath.Join(target, ".luban-code", "skills"), "shared-name", "new project")
	writeTask23Skill(t, userRoot, "user-skill", "user")
	store, err := NewFileOverrideStore(initial, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatalf("create override store: %v", err)
	}
	dirs, err := ProjectDirs(initial)
	if err != nil {
		t.Fatalf("initial project dirs: %v", err)
	}
	dirs = append(dirs, DirSource{Dir: userRoot, Source: SourceUser})
	manager := NewManagerWithOverrideStore(store, dirs...)
	if snapshot, err := manager.Snapshot("session-a"); err != nil ||
		!task23HasSkill(snapshot, "shared-name") || !task23HasSkill(snapshot, "user-skill") {
		t.Fatalf("initial mixed snapshot = %+v, err=%v", snapshot.Skills, err)
	}
	if changed := manager.SetAllEnabled("session-a", false); changed != 2 {
		t.Fatalf("SetAllEnabled changed %d skills, want 2", changed)
	}
	if changed, found := manager.SetEnabled("session-b", "shared-name", false); !changed || !found {
		t.Fatalf("disable project skill in second session: changed=%v found=%v", changed, found)
	}
	if manager.IsEnabled("session-a", "shared-name") || manager.IsEnabled("session-a", "user-skill") ||
		manager.IsEnabled("session-b", "shared-name") {
		t.Fatal("failed to establish mixed legacy disable fixtures")
	}

	if err := manager.ReplaceProjectSources(target); err != nil {
		t.Fatalf("replace project sources: %v", err)
	}
	if !manager.IsEnabled("session-a", "shared-name") || !manager.IsEnabled("session-b", "shared-name") {
		t.Fatal("project-owned legacy disable leaked into target workspace")
	}
	if manager.IsEnabled("session-a", "user-skill") {
		t.Fatal("user-owned legacy disable was cleared by project switch")
	}
}

func TestProjectSourcePlanApplyReconcilesLateProjectLegacyDisable(t *testing.T) {
	initial := t.TempDir()
	target := t.TempDir()
	lateProjectRoot := t.TempDir()
	userRoot := t.TempDir()
	writeTask23Skill(t, filepath.Join(initial, ".luban-code", "skills"), "initial-only", "initial")
	writeTask23Skill(t, filepath.Join(target, ".luban-code", "skills"), "late-name", "target")
	writeTask23Skill(t, lateProjectRoot, "late-name", "late old project")
	writeTask23Skill(t, userRoot, "user-skill", "user")
	store, err := NewFileOverrideStore(initial, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatalf("create override store: %v", err)
	}
	dirs, err := ProjectDirs(initial)
	if err != nil {
		t.Fatalf("initial project dirs: %v", err)
	}
	dirs = append(dirs, DirSource{Dir: userRoot, Source: SourceUser})
	manager := NewManagerWithOverrideStore(store, dirs...)
	if _, err := manager.Snapshot("session-a"); err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}
	plan, err := manager.PrepareProjectSources(target)
	if err != nil {
		t.Fatalf("prepare target project: %v", err)
	}

	// Materialize a new project winner and its legacy disable after Prepare.
	// Apply must reconcile this in-memory state without rescanning the filesystem.
	manager.AddDir(lateProjectRoot, SourceProject)
	if changed, found := manager.SetEnabled("session-a", "late-name", false); !changed || !found {
		t.Fatalf("disable late project winner: changed=%v found=%v", changed, found)
	}
	if changed, found := manager.SetEnabled("session-a", "user-skill", false); !changed || !found {
		t.Fatalf("disable user skill: changed=%v found=%v", changed, found)
	}

	manager.ApplyProjectSources(plan)
	if !manager.IsEnabled("session-a", "late-name") {
		t.Fatal("late project legacy disable leaked into same-name target skill")
	}
	if manager.IsEnabled("session-a", "user-skill") {
		t.Fatal("late reconciliation cleared nonproject legacy disable")
	}
}

func TestProjectSourcePlanPreservesDisableReboundToNonProjectWinner(t *testing.T) {
	initial := t.TempDir()
	target := t.TempDir()
	userRoot := t.TempDir()
	initialSkills := filepath.Join(initial, ".luban-code", "skills")
	writeTask23Skill(t, initialSkills, "shared-name", "old project")
	writeTask23Skill(t, filepath.Join(target, ".luban-code", "skills"), "shared-name", "target project")
	writeTask23Skill(t, userRoot, "shared-name", "user fallback")
	store, err := NewFileOverrideStore(initial, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatalf("create override store: %v", err)
	}
	dirs, err := ProjectDirs(initial)
	if err != nil {
		t.Fatalf("initial project dirs: %v", err)
	}
	dirs = append(dirs, DirSource{Dir: userRoot, Source: SourceUser})
	manager := NewManagerWithOverrideStore(store, dirs...)
	if winner := manager.Get("shared-name"); winner == nil || winner.Source != SourceProject {
		t.Fatalf("initial winner = %+v, want project", winner)
	}
	plan, err := manager.PrepareProjectSources(target)
	if err != nil {
		t.Fatalf("prepare target project: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(initialSkills, "shared-name")); err != nil {
		t.Fatalf("remove old project winner: %v", err)
	}
	manager.Refresh()
	if changed, found := manager.SetEnabled("session-a", "shared-name", false); !changed || !found {
		t.Fatalf("disable rebound user winner: changed=%v found=%v", changed, found)
	}
	if winner := manager.Get("shared-name"); winner == nil || winner.Source != SourceUser {
		t.Fatalf("rebound winner = %+v, want user", winner)
	}

	manager.ApplyProjectSources(plan)
	if manager.IsEnabled("session-a", "shared-name") {
		t.Fatal("apply cleared a legacy disable rebound to a nonproject winner")
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
