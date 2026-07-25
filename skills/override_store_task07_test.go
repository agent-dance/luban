package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

const (
	overrideStoreID      SkillID = "skill:project:/repo/.agents/skills/review"
	overrideStoreOtherID SkillID = "skill:user:/home/me/.agents/skills/explain"
)

func TestOverrideStorePersistenceAndLastNonOff(t *testing.T) {
	paths := overrideStoreTestPaths(t)
	if err := os.MkdirAll(filepath.Dir(paths.ProjectSettings), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ProjectSettings, []byte("{\n  \"theme\": \"dark\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := newOverrideStoreForTest(t, paths, nil, nil)

	if err := store.Set("", VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityManualOnly,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("", VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityOff,
	}); err != nil {
		t.Fatal(err)
	}

	fresh := newOverrideStoreForTest(t, paths, nil, nil)
	snapshot, err := fresh.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	stored := snapshot.Project[overrideStoreID]
	if stored.Visibility != VisibilityOff || stored.LastNonOff == nil || *stored.LastNonOff != VisibilityManualOnly {
		t.Fatalf("stored override = %#v, want off remembering manual-only", stored)
	}

	if err := fresh.Set("", VisibilityOverride{SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityManualOnly}); err != nil {
		t.Fatal(err)
	}
	reenabled := VisibilityOverride{SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityManualOnly}
	if reenabled.Visibility != VisibilityManualOnly || reenabled.LastNonOff != nil {
		t.Fatalf("re-enabled override = %#v", reenabled)
	}
	if err := fresh.Set("", VisibilityOverride{SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityOff}); err != nil {
		t.Fatal(err)
	}
	disabledSnapshot, err := fresh.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	disabled := disabledSnapshot.Project[overrideStoreID]
	if disabled.Visibility != VisibilityOff || disabled.RestoreVisibility() != VisibilityManualOnly {
		t.Fatalf("disabled override = %#v", disabled)
	}

	afterRestart := newOverrideStoreForTest(t, paths, nil, nil)
	restarted, err := afterRestart.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.Project[overrideStoreID].RestoreVisibility(); got != VisibilityManualOnly {
		t.Fatalf("RestoreVisibility() after restart = %q, want manual-only", got)
	}

	raw, err := os.ReadFile(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if string(document["theme"]) != `"dark"` {
		t.Fatalf("unrelated setting changed: %s", document["theme"])
	}
	var wire map[string]map[string]json.RawMessage
	if err := json.Unmarshal(document[skillOverridesSettingsKey], &wire); err != nil {
		t.Fatal(err)
	}
	record := wire[string(overrideStoreID)]
	if _, exists := record["skill_id"]; exists {
		t.Fatal("canonical map record redundantly persisted skill_id")
	}
	if _, exists := record["scope"]; exists {
		t.Fatal("canonical map record redundantly persisted scope")
	}
	if string(record["last_non_off"]) != `"manual-only"` {
		t.Fatalf("last_non_off wire value = %s", record["last_non_off"])
	}
	info, err := os.Stat(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("settings mode = %o, want 600", got)
	}
}

func TestOverridePrecedenceManagedSessionProjectUser(t *testing.T) {
	paths := overrideStoreTestPaths(t)
	base := newOverrideStoreForTest(t, paths, nil, nil)
	if err := base.Set("", VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeUser, Visibility: VisibilityAuto,
	}); err != nil {
		t.Fatal(err)
	}
	if err := base.Set("", VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityNameOnly,
	}); err != nil {
		t.Fatal(err)
	}
	if err := base.Set("session-a", VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeSession, Visibility: VisibilityManualOnly,
	}); err != nil {
		t.Fatal(err)
	}

	snapshotA, err := base.Snapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := snapshotA.Session[overrideStoreID]
	if !ok || resolved.Visibility != VisibilityManualOnly {
		t.Fatalf("session resolution = %#v, %v", resolved, ok)
	}
	snapshotB, err := base.Snapshot("session-b")
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok = snapshotB.Project[overrideStoreID]
	if !ok || resolved.Visibility != VisibilityNameOnly {
		t.Fatalf("session isolation resolution = %#v, %v", resolved, ok)
	}

	remembered := VisibilityNameOnly
	managed := map[SkillID]VisibilityOverride{
		overrideStoreID: {
			Visibility: VisibilityOff,
			LastNonOff: &remembered,
		},
	}
	managedStore := newOverrideStoreForTest(t, paths, managed, base.session)
	managedSnapshot, err := managedStore.Snapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok = managedSnapshot.Managed[overrideStoreID]
	if !ok || resolved.Visibility != VisibilityOff {
		t.Fatalf("managed resolution = %#v, %v", resolved, ok)
	}
	before, err := os.ReadFile(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	if err := managedStore.Set("", VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityAuto,
	}); !errors.Is(err, ErrManagedOverrideReadOnly) {
		t.Fatalf("managed Set error = %v", err)
	}
	if err := managedStore.Reset("session-a", SkillScopeSession, overrideStoreID); !errors.Is(err, ErrManagedOverrideReadOnly) {
		t.Fatalf("managed Reset error = %v", err)
	}
	after, err := os.ReadFile(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("managed mutation changed project settings")
	}

	managedSnapshot.Project[overrideStoreID] = VisibilityOverride{}
	*managedSnapshot.Managed[overrideStoreID].LastNonOff = VisibilityAuto
	again, err := managedStore.Snapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	if again.Project[overrideStoreID].Visibility != VisibilityNameOnly || again.Managed[overrideStoreID].RestoreVisibility() != VisibilityNameOnly {
		t.Fatal("snapshot exposed mutable store state")
	}
}

func TestOverrideStoreProjectCASAndExactRestore(t *testing.T) {
	paths := overrideStoreTestPaths(t)
	original := []byte(fmt.Sprintf("{\n    \"theme\": \"solarized\",\n    \"skillOverrides\": {%q: {\"visibility\": \"manual-only\"}}\n}\n", overrideStoreID))
	writeOverrideSettingsFixture(t, paths.ProjectSettings, string(original))
	store := newOverrideStoreForTest(t, paths, nil, nil)
	snapshot, err := store.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}

	next := VisibilityOverride{SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityOff}
	restore, err := store.CompareAndSetProject(snapshot.ProjectRevision, overrideStoreID, &next)
	if err != nil {
		t.Fatal(err)
	}
	if restore.beforeRevision != snapshot.ProjectRevision || !restore.afterRevision.Valid() {
		t.Fatalf("restore revisions = %q -> %q", restore.beforeRevision, restore.afterRevision)
	}
	committed, err := store.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	if got := committed.Project[overrideStoreID]; got.Visibility != VisibilityOff || got.RestoreVisibility() != VisibilityManualOnly {
		t.Fatalf("CAS record = %#v", got)
	}
	committedBytes, err := os.ReadFile(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndSetProject(snapshot.ProjectRevision, overrideStoreID, &VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityNameOnly,
	}); !errors.Is(err, ErrOverrideRevisionConflict) {
		t.Fatalf("stale CAS error = %v", err)
	}
	unchanged, err := os.ReadFile(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(unchanged, committedBytes) {
		t.Fatal("stale CAS changed settings")
	}

	if err := store.RestoreProject(restore); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, original) {
		t.Fatalf("restore bytes = %q, want exact %q", restored, original)
	}

	missingPaths := overrideStoreTestPaths(t)
	missingStore := newOverrideStoreForTest(t, missingPaths, nil, nil)
	missingSnapshot, err := missingStore.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	createdReceipt, err := missingStore.CompareAndSetProject(missingSnapshot.ProjectRevision, overrideStoreID, &VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityAuto,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := missingStore.RestoreProject(createdReceipt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missingPaths.ProjectSettings); !os.IsNotExist(err) {
		t.Fatalf("compensating restore did not remove newly created file: %v", err)
	}
}

func TestOverrideStoreRestoreRejectsInterveningWriter(t *testing.T) {
	paths := overrideStoreTestPaths(t)
	store := newOverrideStoreForTest(t, paths, nil, nil)
	snapshot, err := store.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	restore, err := store.CompareAndSetProject(snapshot.ProjectRevision, overrideStoreID, &VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityOff,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("", VisibilityOverride{
		SkillID: overrideStoreOtherID, Scope: SkillScopeProject, Visibility: VisibilityNameOnly,
	}); err != nil {
		t.Fatal(err)
	}
	beforeRestore, err := os.ReadFile(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RestoreProject(restore); !errors.Is(err, ErrOverrideRevisionConflict) {
		t.Fatalf("stale restore error = %v", err)
	}
	afterRestore, err := os.ReadFile(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(afterRestore, beforeRestore) {
		t.Fatal("stale restore overwrote intervening writer")
	}
}

func TestAtomicOverrideFailureAndCorruptionPreserveSettings(t *testing.T) {
	paths := overrideStoreTestPaths(t)
	store := newOverrideStoreForTest(t, paths, nil, nil)
	if err := store.Set("", VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityAuto,
	}); err != nil {
		t.Fatal(err)
	}
	valid, err := os.ReadFile(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	store.atomicWrite = func(string, []byte) error { return errors.New("injected write failure") }
	if err := store.Set("", VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityOff,
	}); err == nil {
		t.Fatal("injected write failure unexpectedly succeeded")
	}
	afterFailure, err := os.ReadFile(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(afterFailure, valid) {
		t.Fatal("failed atomic write changed the valid file")
	}

	corrupt := []byte(`{"skillOverrides":`)
	if err := os.WriteFile(paths.ProjectSettings, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	store.atomicWrite = atomicWriteOverrideSettings
	if _, err := store.Snapshot("session"); err == nil {
		t.Fatal("corrupt settings unexpectedly loaded")
	}
	if err := store.Set("", VisibilityOverride{
		SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: VisibilityAuto,
	}); err == nil {
		t.Fatal("write unexpectedly replaced corrupt settings")
	}
	stillCorrupt, err := os.ReadFile(paths.ProjectSettings)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stillCorrupt, corrupt) {
		t.Fatal("corrupt settings were overwritten")
	}

	writeOverrideSettingsFixture(t, paths.ProjectSettings, `{"skillOverrides":null}`)
	if _, err := store.Snapshot("session"); err == nil {
		t.Fatal("null skillOverrides unexpectedly loaded")
	}
}

func TestOverrideStoreCASSerializesStoreInstances(t *testing.T) {
	paths := overrideStoreTestPaths(t)
	first := newOverrideStoreForTest(t, paths, nil, nil)
	second := newOverrideStoreForTest(t, paths, nil, nil)
	snapshot, err := first.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errorsByWriter := make(chan error, 2)
	var wait sync.WaitGroup
	for index, store := range []*FileOverrideStore{first, second} {
		index, store := index, store
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			visibility := VisibilityNameOnly
			if index == 1 {
				visibility = VisibilityManualOnly
			}
			_, err := store.CompareAndSetProject(snapshot.ProjectRevision, overrideStoreID, &VisibilityOverride{
				SkillID: overrideStoreID, Scope: SkillScopeProject, Visibility: visibility,
			})
			errorsByWriter <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByWriter)
	successes, conflicts := 0, 0
	for err := range errorsByWriter {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrOverrideRevisionConflict):
			conflicts++
		default:
			t.Fatalf("CAS writer error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("CAS outcomes: successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestOverrideStoreSessionLayerIsRaceSafeAndIsolated(t *testing.T) {
	paths := overrideStoreTestPaths(t)
	store := newOverrideStoreForTest(t, paths, nil, nil)
	const sessions = 24
	var wait sync.WaitGroup
	for index := 0; index < sessions; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			sessionID := fmt.Sprintf("session-%d", index)
			if err := store.Set(sessionID, VisibilityOverride{
				SkillID: overrideStoreID, Scope: SkillScopeSession, Visibility: VisibilityNameOnly,
			}); err != nil {
				t.Errorf("Set(%s): %v", sessionID, err)
				return
			}
			if err := store.Set(sessionID, VisibilityOverride{SkillID: overrideStoreID, Scope: SkillScopeSession, Visibility: VisibilityOff}); err != nil {
				t.Errorf("Set off (%s): %v", sessionID, err)
			}
		}()
	}
	wait.Wait()
	for index := 0; index < sessions; index++ {
		sessionID := fmt.Sprintf("session-%d", index)
		snapshot, err := store.Snapshot(sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Session) != 1 || snapshot.Session[overrideStoreID].RestoreVisibility() != VisibilityNameOnly {
			t.Fatalf("%s snapshot = %#v", sessionID, snapshot.Session)
		}
	}
	if err := store.Reset("session-0", SkillScopeSession, overrideStoreID); err != nil {
		t.Fatal(err)
	}
	zero, err := store.Snapshot("session-0")
	if err != nil {
		t.Fatal(err)
	}
	one, err := store.Snapshot("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(zero.Session) != 0 || len(one.Session) != 1 {
		t.Fatalf("session reset leaked: zero=%#v one=%#v", zero.Session, one.Session)
	}
}

func overrideStoreTestPaths(t *testing.T) OverrideStorePaths {
	t.Helper()
	directory := t.TempDir()
	return OverrideStorePaths{
		UserSettings:    filepath.Join(directory, "user", ".luban-code", "settings.json"),
		ProjectSettings: filepath.Join(directory, "project", ".luban-code", "settings.json"),
	}
}

func newOverrideStoreForTest(t *testing.T, paths OverrideStorePaths, managed map[SkillID]VisibilityOverride, session SessionOverrideLayer) *FileOverrideStore {
	t.Helper()
	if session == nil {
		session = NewMemorySessionOverrideLayer()
	}
	store, err := NewFileOverrideStoreAt(paths, managed, session)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writeOverrideSettingsFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
