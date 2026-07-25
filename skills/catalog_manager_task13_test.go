package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestCatalogManagerSameNameWinnerAndShadowResolve(t *testing.T) {
	high := t.TempDir()
	low := t.TempDir()
	task13WriteSkill(t, high, "high", "shared", "high priority", "high body")
	task13WriteSkill(t, low, "low", "shared", "low priority", "low body")

	settingsRoot := t.TempDir()
	store, err := NewFileOverrideStoreAt(OverrideStorePaths{
		UserSettings:    filepath.Join(settingsRoot, "user", "settings.json"),
		ProjectSettings: filepath.Join(settingsRoot, "project", "settings.json"),
	}, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager := newManagerWithOverrideStore(store,
		DirSource{Dir: high, Source: SourceProject},
		DirSource{Dir: low, Source: SourceUser},
	)
	snapshot, err := manager.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 1 || len(snapshot.Skills) != 2 {
		t.Fatalf("snapshot = %#v, want revision 1 with two stable entries", snapshot)
	}

	var winner, shadow EffectiveSkill
	for _, row := range snapshot.Skills {
		if row.ShadowedBy == "" {
			winner = row
		} else {
			shadow = row
		}
	}
	if winner.Source != SourceProject || shadow.Source != SourceUser || shadow.ShadowedBy != winner.ID || winner.ID == shadow.ID {
		t.Fatalf("winner/shadow = %#v / %#v", winner, shadow)
	}
	if shadow.ModelVisible || shadow.UserInvocable || shadow.Executable {
		t.Fatalf("shadowed entry remained invocable: %#v", shadow)
	}
	generation := manager.ProjectGeneration()

	resolvedResult, err := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "session", Selector: "shared", Origin: InvocationOriginUser,
		ExpectedProjectGeneration: generation,
	}, nil)
	if err != nil || resolvedResult.Outcome != SkillResolveResolved || resolvedResult.Resolved == nil || resolvedResult.Resolved.Effective.ID != winner.ID || resolvedResult.Resolved.Skill.Content != "high body" {
		t.Fatalf("ResolveLatest(name) = %#v, %v", resolvedResult, err)
	}
	shadowResult, err := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "session", Selector: string(shadow.ID), Origin: InvocationOriginUser,
		ExpectedProjectGeneration: generation,
	}, nil)
	if err != nil || shadowResult.Outcome != SkillResolveShadowed || shadowResult.Resolved == nil || shadowResult.Resolved.Effective.ShadowedBy != winner.ID || shadowResult.Resolved.Skill.Content != "low body" {
		t.Fatalf("ResolveLatest(shadow ID) = %#v, %v", shadowResult, err)
	}
	snapshot.Skills[0].Name = "mutated"
	resolvedResult.Resolved.Skill.Content = "mutated"
	again, err := manager.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	if again.Revision != 1 || again.Skills[0].Name == "mutated" {
		t.Fatalf("snapshot exposed manager storage: %#v", again)
	}
	againResolved, err := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "session", Selector: "shared", Origin: InvocationOriginUser,
		ExpectedProjectGeneration: generation,
	}, nil)
	if err != nil || againResolved.Resolved == nil || againResolved.Resolved.Skill.Content != "high body" {
		t.Fatalf("resolved content exposed manager storage: %#v, %v", againResolved, err)
	}

	latest, err := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "session", Selector: string(shadow.ID), Origin: InvocationOriginUser,
		ExpectedProjectGeneration: generation,
	}, nil)
	if err != nil || latest.Outcome != SkillResolveShadowed || latest.Resolved == nil || latest.Resolved.Effective.ID != shadow.ID {
		t.Fatalf("ResolveLatest(shadow) = %#v, %v", latest, err)
	}
	if err := latest.Validate(); err != nil {
		t.Fatalf("ResolveLatest(shadow) result invalid: %v", err)
	}
	toggled, err := manager.ToggleProjectVisibility("session", shadow.ID, again.Revision)
	if err != nil || toggled.Outcome != ProjectVisibilityToggleCommitted || toggled.Skill == nil || toggled.Skill.ID != shadow.ID || toggled.Skill.Visibility != VisibilityOff {
		t.Fatalf("stable-ID shadow toggle = %#v, %v", toggled, err)
	}
	currentWinner, found := toggled.Snapshot.Find(winner.ID)
	if !found || currentWinner.Visibility != winner.Visibility || currentWinner.Revision != winner.Revision {
		t.Fatalf("shadow toggle changed winner: before=%#v after=%#v", winner, currentWinner)
	}
}

func TestManagerRevisionChangesOnlyWithEffectiveStateAndAdvancesOnReadd(t *testing.T) {
	root := t.TempDir()
	skillDir := task13WriteSkill(t, root, "review", "review", "review files", "first")
	manager := newCatalogManagerForTest(DirSource{Dir: root, Source: SourceProject})

	first := task13Snapshot(t, manager, "session")
	unchanged, err := manager.RefreshSnapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Revision != first.Revision || unchanged.Skills[0].Revision != first.Skills[0].Revision {
		t.Fatalf("unchanged refresh advanced revisions: %#v -> %#v", first, unchanged)
	}

	task13WriteSkill(t, root, "review", "review", "review files", "second")
	changed, err := manager.RefreshSnapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision != first.Revision+1 || changed.Skills[0].Revision != first.Skills[0].Revision+1 || changed.Skills[0].Digest == first.Skills[0].Digest {
		t.Fatalf("content update revisions = %#v -> %#v", first, changed)
	}

	if err := os.RemoveAll(skillDir); err != nil {
		t.Fatal(err)
	}
	removed, err := manager.RefreshSnapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	if removed.Revision != changed.Revision+1 || len(removed.Skills) != 0 {
		t.Fatalf("removed snapshot = %#v", removed)
	}
	task13WriteSkill(t, root, "review", "review", "review files", "second")
	readded, err := manager.RefreshSnapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	if readded.Revision != removed.Revision+1 || readded.Skills[0].Revision != changed.Skills[0].Revision+1 {
		t.Fatalf("re-add did not advance lifecycle revisions: removed=%#v readded=%#v", removed, readded)
	}
}

func TestCatalogManagerScopedVisibilityIsSessionIsolated(t *testing.T) {
	manager, _, row := task13SingleManager(t, nil)
	off := VisibilityOverride{SkillID: row.ID, Scope: SkillScopeSession, Visibility: VisibilityOff}
	sessionA, err := manager.SetVisibility("session-a", off)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := sessionA.Find(row.ID); got.Visibility != VisibilityOff || got.VisibilitySource != SkillScopeSession {
		t.Fatalf("session-a row = %#v", got)
	}
	sessionB := task13Snapshot(t, manager, "session-b")
	if got, _ := sessionB.Find(row.ID); got.Visibility == VisibilityOff || got.VisibilitySource == SkillScopeSession {
		t.Fatalf("session override leaked to session-b: %#v", got)
	}
	reset, err := manager.ResetVisibility("session-a", SkillScopeSession, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reset.Find(row.ID); got.Visibility != VisibilityAuto || got.VisibilitySource != SkillScopeDefault {
		t.Fatalf("reset row = %#v", got)
	}
}

func TestCatalogManagerProjectToggleCASAndLastNonOff(t *testing.T) {
	manager, store, row := task13SingleManager(t, nil)
	if err := store.Set("", VisibilityOverride{
		SkillID: row.ID, Scope: SkillScopeProject, Visibility: VisibilityManualOnly,
	}); err != nil {
		t.Fatal(err)
	}
	manual := task13Snapshot(t, manager, "session")
	manualRow, _ := manual.Find(row.ID)
	if manualRow.Visibility != VisibilityManualOnly || manualRow.VisibilitySource != SkillScopeProject {
		t.Fatalf("manual row = %#v", manualRow)
	}

	disabled, err := manager.ToggleProjectVisibility("session", row.ID, manual.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err := disabled.Validate(); err != nil {
		t.Fatalf("disabled result invalid: %v (%#v)", err, disabled)
	}
	if disabled.Outcome != ProjectVisibilityToggleCommitted || disabled.Skill.Visibility != VisibilityOff {
		t.Fatalf("disable result = %#v", disabled)
	}
	persisted, err := store.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	stored := persisted.Project[row.ID]
	if stored.Visibility != VisibilityOff || stored.LastNonOff == nil || *stored.LastNonOff != VisibilityManualOnly {
		t.Fatalf("stored project override = %#v", stored)
	}

	enabled, err := manager.ToggleProjectVisibility("session", row.ID, disabled.CurrentRevision)
	if err != nil {
		t.Fatal(err)
	}
	if enabled.Outcome != ProjectVisibilityToggleCommitted || enabled.Skill.Visibility != VisibilityManualOnly {
		t.Fatalf("re-enable result = %#v", enabled)
	}
	stale, err := manager.ToggleProjectVisibility("session", row.ID, manual.Revision)
	if err != nil || stale.Outcome != ProjectVisibilityToggleRejected || stale.Reason != ProjectVisibilityToggleReasonStaleRevision {
		t.Fatalf("stale result = %#v, %v", stale, err)
	}
	if err := stale.Validate(); err != nil {
		t.Fatalf("stale result invalid: %v", err)
	}
}

func TestCatalogManagerProjectToggleRejectsSessionManagedAndUnknown(t *testing.T) {
	t.Run("session override", func(t *testing.T) {
		manager, store, row := task13SingleManager(t, nil)
		if err := store.Set("session", VisibilityOverride{
			SkillID: row.ID, Scope: SkillScopeSession, Visibility: VisibilityNameOnly,
		}); err != nil {
			t.Fatal(err)
		}
		current := task13Snapshot(t, manager, "session")
		result, err := manager.ToggleProjectVisibility("session", row.ID, current.Revision)
		if err != nil || result.Outcome != ProjectVisibilityToggleRejected || result.Reason != ProjectVisibilityToggleReasonSessionOverride {
			t.Fatalf("result = %#v, %v", result, err)
		}
		after, err := store.Snapshot("session")
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := after.Project[row.ID]; exists {
			t.Fatal("session rejection wrote a project override")
		}
	})

	t.Run("managed read only", func(t *testing.T) {
		baseManager, baseStore, row := task13SingleManager(t, nil)
		_ = baseManager
		managed := map[SkillID]VisibilityOverride{row.ID: {
			SkillID: row.ID, Scope: SkillScopeManaged, Visibility: VisibilityAuto,
		}}
		managedStore, err := NewFileOverrideStoreAt(baseStore.paths, managed, NewMemorySessionOverrideLayer())
		if err != nil {
			t.Fatal(err)
		}
		manager := newManagerWithOverrideStore(managedStore, baseManager.dirs...)
		current := task13Snapshot(t, manager, "session")
		managedRow, _ := current.Find(row.ID)
		if managedRow.Mutable || managedRow.ReadOnlyReason == "" {
			t.Fatalf("managed row = %#v", managedRow)
		}
		result, toggleErr := manager.ToggleProjectVisibility("session", row.ID, current.Revision)
		if toggleErr != nil || result.Reason != ProjectVisibilityToggleReasonReadOnly {
			t.Fatalf("managed toggle = %#v, %v", result, toggleErr)
		}
	})

	t.Run("unknown stable ID", func(t *testing.T) {
		manager, _, _ := task13SingleManager(t, nil)
		current := task13Snapshot(t, manager, "session")
		unknown := SkillID("skill:project:unknown")
		result, err := manager.ToggleProjectVisibility("session", unknown, current.Revision)
		if err != nil || result.Reason != ProjectVisibilityToggleReasonUnknownSkill || result.Skill != nil {
			t.Fatalf("unknown toggle = %#v, %v", result, err)
		}
		if err := result.Validate(); err != nil {
			t.Fatalf("unknown result invalid: %v", err)
		}
	})
}

func TestCatalogManagerProjectToggleFailureTruth(t *testing.T) {
	t.Run("persistence failure leaves live state unchanged", func(t *testing.T) {
		fault := &task13FaultStore{compareErr: errors.New("write failed")}
		manager, _, row := task13SingleManager(t, fault)
		before := task13Snapshot(t, manager, "session")
		result, err := manager.ToggleProjectVisibility("session", row.ID, before.Revision)
		if err == nil || result.Outcome != ProjectVisibilityToggleRejected || result.Reason != ProjectVisibilityToggleReasonPersistenceFailed {
			t.Fatalf("result = %#v, %v", result, err)
		}
		after := task13Snapshot(t, manager, "session")
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("persistence failure changed live state: %#v -> %#v", before, after)
		}
	})

	t.Run("live apply failure rolls persistence back", func(t *testing.T) {
		fault := &task13FaultStore{failSnapshotsAfterCAS: 1}
		manager, base, row := task13SingleManager(t, fault)
		before := task13Snapshot(t, manager, "session")
		result, err := manager.ToggleProjectVisibility("session", row.ID, before.Revision)
		if err == nil || result.Outcome != ProjectVisibilityToggleRejected || result.Reason != ProjectVisibilityToggleReasonLiveApplyRolledBack {
			t.Fatalf("result = %#v, %v", result, err)
		}
		stored, snapshotErr := base.Snapshot("session")
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		if _, exists := stored.Project[row.ID]; exists {
			t.Fatalf("rolled-back override remains: %#v", stored.Project[row.ID])
		}
	})

	t.Run("rollback failure returns refreshed degraded truth", func(t *testing.T) {
		fault := &task13FaultStore{failSnapshotsAfterCAS: 1, restoreErr: errors.New("rollback failed")}
		manager, base, row := task13SingleManager(t, fault)
		before := task13Snapshot(t, manager, "session")
		result, err := manager.ToggleProjectVisibility("session", row.ID, before.Revision)
		if err == nil || result.Outcome != ProjectVisibilityToggleDegraded || result.Reason != ProjectVisibilityToggleReasonRollbackFailed {
			t.Fatalf("result = %#v, %v", result, err)
		}
		if result.Skill == nil || result.Skill.Visibility != VisibilityOff || result.CurrentRevision == before.Revision {
			t.Fatalf("degraded result was not authoritatively refreshed: %#v", result)
		}
		stored, snapshotErr := base.Snapshot("session")
		if snapshotErr != nil || stored.Project[row.ID].Visibility != VisibilityOff {
			t.Fatalf("persisted state = %#v, %v", stored.Project[row.ID], snapshotErr)
		}
	})

	t.Run("rollback and refresh failure retain prior evidence", func(t *testing.T) {
		fault := &task13FaultStore{failSnapshotsAfterCAS: 4, restoreErr: errors.New("rollback failed")}
		manager, _, row := task13SingleManager(t, fault)
		before := task13Snapshot(t, manager, "session")
		result, err := manager.ToggleProjectVisibility("session", row.ID, before.Revision)
		if err == nil || result.Outcome != ProjectVisibilityToggleDegraded || result.Reason != ProjectVisibilityToggleReasonAuthoritativeRefresh {
			t.Fatalf("result = %#v, %v", result, err)
		}
		if !reflect.DeepEqual(result.Snapshot, before) {
			t.Fatalf("refresh failure did not retain last authoritative evidence: %#v", result)
		}
	})
}

func TestCatalogManagerResolveLatestLinearizesExecutionAndTypedDenials(t *testing.T) {
	manager, _, row := task13SingleManager(t, nil)
	generation := manager.ProjectGeneration()
	before := task13Snapshot(t, manager, "session")
	entered := make(chan struct{})
	release := make(chan struct{})
	resolveDone := make(chan SkillResolveResult, 1)
	resolveErr := make(chan error, 1)
	go func() {
		result, err := manager.ResolveLatest(SkillResolveRequest{
			SessionID: "session", Selector: string(row.ID), ExpectedRevision: row.Revision, Origin: InvocationOriginUser,
			ExpectedProjectGeneration: generation,
		}, func(resolved ResolvedSkill) error {
			if resolved.Effective.ID != row.ID || resolved.Skill == nil {
				return errors.New("callback received mismatched resolution")
			}
			close(entered)
			<-release
			return nil
		})
		resolveDone <- result
		resolveErr <- err
	}()
	<-entered

	toggleDone := make(chan ProjectVisibilityToggleResult, 1)
	toggleErr := make(chan error, 1)
	go func() {
		result, err := manager.ToggleProjectVisibility("session", row.ID, before.Revision)
		toggleDone <- result
		toggleErr <- err
	}()
	select {
	case result := <-toggleDone:
		t.Fatalf("toggle crossed execution linearization boundary: %#v", result)
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	if result := <-resolveDone; result.Outcome != SkillResolveResolved {
		t.Fatalf("resolve result = %#v", result)
	}
	if err := <-resolveErr; err != nil {
		t.Fatal(err)
	}
	disabled := <-toggleDone
	if err := <-toggleErr; err != nil || disabled.Outcome != ProjectVisibilityToggleCommitted {
		t.Fatalf("toggle result = %#v, %v", disabled, err)
	}

	denied, err := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "session", Selector: string(row.ID), Origin: InvocationOriginUser,
		ExpectedProjectGeneration: generation,
	}, nil)
	if err != nil || denied.Outcome != SkillResolvePolicyDenied {
		t.Fatalf("policy denial = %#v, %v", denied, err)
	}
	if err := denied.Validate(); err != nil {
		t.Fatalf("policy denial result invalid: %v", err)
	}
	stale, err := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "session", Selector: string(row.ID), ExpectedRevision: row.Revision, Origin: InvocationOriginUser,
		ExpectedProjectGeneration: generation,
	}, nil)
	if err != nil || stale.Outcome != SkillResolveStale {
		t.Fatalf("stale denial = %#v, %v", stale, err)
	}
	notFound, err := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "session", Selector: "missing", Origin: InvocationOriginModel,
		ExpectedProjectGeneration: manager.ProjectGeneration(),
	}, nil)
	if err != nil || notFound.Outcome != SkillResolveNotFound {
		t.Fatalf("not-found result = %#v, %v", notFound, err)
	}
}

func TestCatalogManagerResolveLatestReadersRunConcurrentlyWhileWriterWaits(t *testing.T) {
	manager, _, row := task13SingleManager(t, nil)
	generation := manager.ProjectGeneration()
	before := task13Snapshot(t, manager, "session")
	entered := make(chan int, 2)
	release := make(chan struct{})
	results := make(chan SkillResolveResult, 2)
	errorsSeen := make(chan error, 2)
	for worker := 0; worker < 2; worker++ {
		go func(worker int) {
			result, err := manager.ResolveLatest(SkillResolveRequest{
				SessionID: "session", Selector: string(row.ID), Origin: InvocationOriginUser,
				ExpectedProjectGeneration: generation,
			}, func(ResolvedSkill) error {
				entered <- worker
				<-release
				return nil
			})
			results <- result
			errorsSeen <- err
		}(worker)
	}
	for count := 0; count < 2; count++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("ResolveLatest readers were serialized")
		}
	}

	writerDone := make(chan ProjectVisibilityToggleResult, 1)
	writerErr := make(chan error, 1)
	go func() {
		result, err := manager.ToggleProjectVisibility("session", row.ID, before.Revision)
		writerDone <- result
		writerErr <- err
	}()
	select {
	case result := <-writerDone:
		t.Fatalf("writer crossed active ResolveLatest readers: %#v", result)
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	for count := 0; count < 2; count++ {
		if result := <-results; result.Outcome != SkillResolveResolved {
			t.Fatalf("reader result = %#v", result)
		}
		if err := <-errorsSeen; err != nil {
			t.Fatal(err)
		}
	}
	if result := <-writerDone; result.Outcome != ProjectVisibilityToggleCommitted {
		t.Fatalf("writer result = %#v", result)
	}
	if err := <-writerErr; err != nil {
		t.Fatal(err)
	}
}

func TestCatalogManagerExecutionReceiptMetadataContract(t *testing.T) {
	if err := InvocationOriginModel.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := InvocationOrigin("unknown").Validate(); err == nil {
		t.Fatal("unknown invocation origin was accepted")
	}
	receipt := SkillExecutionReceipt{
		ContextEpoch:            17,
		SkillID:                 SkillID("skill:project:receipt"),
		ContentDigest:           ComputeSkillDigest("content"),
		InvocationPayloadDigest: DigestInvocationPayload("rendered"),
		InvocationEnvelopeKind:  InvocationEnvelopeFull,
	}
	metadata, err := EncodeSkillExecutionReceiptMetadata(receipt)
	if err != nil {
		t.Fatal(err)
	}
	decoded, found, err := DecodeSkillExecutionReceiptMetadata(metadata)
	if err != nil || !found || decoded != receipt {
		t.Fatalf("receipt round trip = %#v, %t, %v", decoded, found, err)
	}
	if _, found, err := DecodeSkillExecutionReceiptMetadata(map[string]string{"other": "value"}); err != nil || found {
		t.Fatalf("absent receipt = found %t, err %v", found, err)
	}
	encoded := metadata[SkillExecutionReceiptMetadataKey]
	if _, err := unmarshalSkillExecutionReceipt(encoded[:len(encoded)-1] + `,"extra":true}`); err == nil {
		t.Fatal("receipt with unknown field was accepted")
	}
	receipt.ContextEpoch = 0
	if _, err := EncodeSkillExecutionReceiptMetadata(receipt); err == nil {
		t.Fatal("zero-epoch receipt was accepted")
	}
}

func TestCatalogManagerConcurrentSnapshotResolveRefreshAndToggle(t *testing.T) {
	manager, _, row := task13SingleManager(t, nil)
	generation := manager.ProjectGeneration()
	var wait sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for iteration := 0; iteration < 20; iteration++ {
				sessionID := fmt.Sprintf("session-%d", worker%2)
				if _, err := manager.Snapshot(sessionID); err != nil {
					t.Errorf("Snapshot: %v", err)
					return
				}
				if _, err := manager.ResolveLatest(SkillResolveRequest{
					SessionID: sessionID, Selector: string(row.ID), Origin: InvocationOriginUser,
					ExpectedProjectGeneration: generation,
				}, nil); err != nil {
					t.Errorf("ResolveLatest: %v", err)
					return
				}
				if iteration%5 == 0 {
					if _, err := manager.RefreshSnapshot(sessionID); err != nil {
						t.Errorf("RefreshSnapshot: %v", err)
						return
					}
				}
			}
		}(worker)
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		for iteration := 0; iteration < 20; iteration++ {
			snapshot, err := manager.Snapshot("toggle-session")
			if err != nil {
				t.Errorf("toggle Snapshot: %v", err)
				return
			}
			result, err := manager.ToggleProjectVisibility("toggle-session", row.ID, snapshot.Revision)
			if err != nil || result.Outcome != ProjectVisibilityToggleCommitted {
				t.Errorf("ToggleProjectVisibility: %#v, %v", result, err)
				return
			}
		}
	}()
	wait.Wait()
}

func task13WriteSkill(t *testing.T, root, directory, name, description, body string) string {
	t.Helper()
	dir := filepath.Join(root, directory)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n%s", name, description, body)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func task13Snapshot(t *testing.T, manager *Manager, sessionID string) CatalogSnapshot {
	t.Helper()
	snapshot, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("invalid manager snapshot: %v", err)
	}
	return snapshot
}

func task13SingleManager(t *testing.T, fault *task13FaultStore) (*Manager, *FileOverrideStore, EffectiveSkill) {
	t.Helper()
	root := t.TempDir()
	task13WriteSkill(t, root, "review", "review", "review files", "review body")
	settingsRoot := t.TempDir()
	base, err := NewFileOverrideStoreAt(OverrideStorePaths{
		UserSettings:    filepath.Join(settingsRoot, "user", "settings.json"),
		ProjectSettings: filepath.Join(settingsRoot, "project", "settings.json"),
	}, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	var store OverrideStore = base
	if fault != nil {
		fault.base = base
		store = fault
	}
	manager := newManagerWithOverrideStore(store, DirSource{Dir: root, Source: SourceProject})
	snapshot := task13Snapshot(t, manager, "session")
	if len(snapshot.Skills) != 1 {
		t.Fatalf("single manager snapshot = %#v", snapshot)
	}
	return manager, base, snapshot.Skills[0]
}

type task13FaultStore struct {
	base                  OverrideStore
	mu                    sync.Mutex
	compareErr            error
	restoreErr            error
	failSnapshotsAfterCAS int
	snapshotFailures      int
}

func (store *task13FaultStore) Snapshot(sessionID string) (OverrideSnapshot, error) {
	store.mu.Lock()
	if store.snapshotFailures > 0 {
		store.snapshotFailures--
		store.mu.Unlock()
		return OverrideSnapshot{}, errors.New("live snapshot failed")
	}
	store.mu.Unlock()
	return store.base.Snapshot(sessionID)
}

func (store *task13FaultStore) Set(sessionID string, override VisibilityOverride) error {
	return store.base.Set(sessionID, override)
}

func (store *task13FaultStore) Reset(sessionID string, scope SkillScope, id SkillID) error {
	return store.base.Reset(sessionID, scope, id)
}

func (store *task13FaultStore) CompareAndSetProject(expected OverrideStoreRevision, id SkillID, next *VisibilityOverride) (ProjectOverrideRestore, error) {
	store.mu.Lock()
	compareErr := store.compareErr
	store.mu.Unlock()
	if compareErr != nil {
		return ProjectOverrideRestore{}, compareErr
	}
	restore, err := store.base.CompareAndSetProject(expected, id, next)
	if err == nil {
		store.mu.Lock()
		store.snapshotFailures += store.failSnapshotsAfterCAS
		store.mu.Unlock()
	}
	return restore, err
}

func (store *task13FaultStore) RestoreProject(restore ProjectOverrideRestore) error {
	store.mu.Lock()
	restoreErr := store.restoreErr
	store.mu.Unlock()
	if restoreErr != nil {
		return restoreErr
	}
	return store.base.RestoreProject(restore)
}

var _ OverrideStore = (*task13FaultStore)(nil)
