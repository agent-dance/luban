package skills_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/tools"
)

func TestCatalogRaceAcceptanceRealSkillToolRefreshToggleSnapshotAndInvoke(t *testing.T) {
	manager, _, row := task28RaceManager(t, nil)
	tool := &tools.SkillTool{
		Manager:           manager,
		FallbackSessionID: "invoke-session",
		LoadedLedgerResolver: func(context.Context, string, skills.SkillID) tools.SkillLoadedLedgerState {
			return tools.SkillLoadedLedgerState{ContextEpoch: 23}
		},
	}
	first, err := tool.Execute(context.Background(), map[string]any{"skill": string(row.ID), "revision": uint64(row.Revision)})
	if err != nil || first.IsError {
		t.Fatalf("initial real SkillTool invocation=%#v err=%v", first, err)
	}
	receipt, found, err := skills.DecodeSkillExecutionReceiptMetadata(first.Metadata)
	if err != nil || !found || receipt.ContextEpoch != 23 || receipt.SkillID != row.ID || receipt.ContentDigest != row.Digest {
		t.Fatalf("initial SkillTool receipt=%#v found=%t err=%v", receipt, found, err)
	}

	const iterations = 35
	errCh := make(chan error, 8)
	var successes atomic.Int64
	successes.Store(1)
	var wait sync.WaitGroup
	run := func(fn func(int) error) {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for i := 0; i < iterations; i++ {
				if runErr := fn(i); runErr != nil {
					errCh <- runErr
					return
				}
			}
		}()
	}

	run(func(int) error {
		snapshot, snapshotErr := manager.Snapshot("snapshot-session")
		if snapshotErr == nil {
			snapshotErr = snapshot.Validate()
		}
		return snapshotErr
	})
	run(func(int) error {
		_, refreshErr := manager.RefreshSnapshot("refresh-session")
		return refreshErr
	})
	run(func(int) error {
		current, snapshotErr := manager.Snapshot("toggle-session")
		if snapshotErr != nil {
			return snapshotErr
		}
		result, toggleErr := manager.ToggleProjectVisibility("toggle-session", row.ID, current.Revision)
		if toggleErr != nil {
			return toggleErr
		}
		if result.Outcome != skills.ProjectVisibilityToggleCommitted &&
			!(result.Outcome == skills.ProjectVisibilityToggleRejected && result.Reason == skills.ProjectVisibilityToggleReasonStaleRevision) {
			return fmt.Errorf("unexpected toggle result: %#v", result)
		}
		return result.Validate()
	})
	run(func(int) error {
		result, invokeErr := tool.Execute(context.Background(), map[string]any{"skill": string(row.ID)})
		if invokeErr != nil {
			return invokeErr
		}
		if result.IsError {
			if result.Metadata["registryOutcome"] != string(skills.SkillResolvePolicyDenied) {
				return fmt.Errorf("unexpected SkillTool rejection: %#v", result)
			}
			return nil
		}
		receipt, receiptFound, receiptErr := skills.DecodeSkillExecutionReceiptMetadata(result.Metadata)
		if receiptErr != nil || !receiptFound || receipt.SkillID != row.ID || receipt.ContextEpoch != 23 {
			return fmt.Errorf("invalid concurrent SkillTool receipt: %#v found=%t err=%v", receipt, receiptFound, receiptErr)
		}
		successes.Add(1)
		return nil
	})
	run(func(i int) error {
		visibility := skills.VisibilityNameOnly
		if i%2 == 0 {
			visibility = skills.VisibilityManualOnly
		}
		_, setErr := manager.SetVisibility("overlay-session", skills.VisibilityOverride{
			SkillID: row.ID, Scope: skills.SkillScopeSession, Visibility: visibility,
		})
		return setErr
	})
	run(func(int) error {
		_, resetErr := manager.ResetVisibility("overlay-session", skills.SkillScopeSession, row.ID)
		return resetErr
	})
	run(func(int) error {
		_, _, resolveErr := manager.Resolve("resolve-session", string(row.ID))
		return resolveErr
	})

	wait.Wait()
	close(errCh)
	for runErr := range errCh {
		t.Error(runErr)
	}
	if successes.Load() == 0 {
		t.Fatal("race run produced no validated real SkillTool receipt")
	}
	final, finalErr := manager.Snapshot("final-session")
	if finalErr != nil || final.Validate() != nil {
		t.Fatalf("final snapshot=%#v err=%v", final, finalErr)
	}
}

func TestCatalogRaceAcceptanceTransactionalFailureTruth(t *testing.T) {
	t.Run("persistence failure", func(t *testing.T) {
		fault := &task28RaceFaultStore{compareErr: errors.New("injected write failure")}
		manager, _, row := task28RaceManager(t, fault)
		before := task28RaceSnapshot(t, manager, "session")
		result, err := manager.ToggleProjectVisibility("session", row.ID, before.Revision)
		if err == nil || result.Outcome != skills.ProjectVisibilityToggleRejected || result.Reason != skills.ProjectVisibilityToggleReasonPersistenceFailed {
			t.Fatalf("persistence result=%#v err=%v", result, err)
		}
		if after := task28RaceSnapshot(t, manager, "session"); !reflect.DeepEqual(after, before) || !reflect.DeepEqual(result.Snapshot, before) {
			t.Fatalf("persistence failure changed authority: before=%#v result=%#v after=%#v", before, result, after)
		}
	})

	t.Run("live apply failure rolls persistence back", func(t *testing.T) {
		fault := &task28RaceFaultStore{failSnapshotsAfterCAS: 1}
		manager, store, row := task28RaceManager(t, fault)
		before := task28RaceSnapshot(t, manager, "session")
		result, err := manager.ToggleProjectVisibility("session", row.ID, before.Revision)
		if err == nil || result.Outcome != skills.ProjectVisibilityToggleRejected || result.Reason != skills.ProjectVisibilityToggleReasonLiveApplyRolledBack {
			t.Fatalf("rollback result=%#v err=%v", result, err)
		}
		persisted, snapshotErr := store.Snapshot("session")
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		if _, exists := persisted.Project[row.ID]; exists || !reflect.DeepEqual(result.Snapshot, before) {
			t.Fatalf("successful compensation truth persisted=%#v result=%#v", persisted, result)
		}
	})

	t.Run("rollback failure returns fresh degraded authority", func(t *testing.T) {
		fault := &task28RaceFaultStore{failSnapshotsAfterCAS: 1, restoreErr: errors.New("injected rollback failure")}
		manager, store, row := task28RaceManager(t, fault)
		before := task28RaceSnapshot(t, manager, "session")
		result, err := manager.ToggleProjectVisibility("session", row.ID, before.Revision)
		if err == nil || result.Outcome != skills.ProjectVisibilityToggleDegraded || result.Reason != skills.ProjectVisibilityToggleReasonRollbackFailed || !result.RefreshRequired() {
			t.Fatalf("degraded result=%#v err=%v", result, err)
		}
		if result.Skill == nil || result.Skill.Visibility != skills.VisibilityOff || result.CurrentRevision == before.Revision {
			t.Fatalf("degraded receipt is not freshly authoritative=%#v", result)
		}
		persisted, snapshotErr := store.Snapshot("session")
		if snapshotErr != nil || persisted.Project[row.ID].Visibility != skills.VisibilityOff {
			t.Fatalf("degraded persisted authority=%#v err=%v", persisted.Project[row.ID], snapshotErr)
		}
	})
}

func task28RaceManager(t *testing.T, fault *task28RaceFaultStore) (*skills.Manager, *skills.FileOverrideStore, skills.EffectiveSkill) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "skills", "review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: review\ndescription: Review\n---\nreal SkillTool body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings:    filepath.Join(root, "settings", "user.json"),
		ProjectSettings: filepath.Join(root, "settings", "project.json"),
	}, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	var store skills.OverrideStore = base
	if fault != nil {
		fault.base = base
		store = fault
	}
	manager := skills.NewManagerWithOverrideStore(store, skills.DirSource{Dir: filepath.Join(root, "skills"), Source: skills.SourceProject})
	snapshot := task28RaceSnapshot(t, manager, "session")
	if len(snapshot.Skills) != 1 {
		t.Fatalf("single skill snapshot=%#v", snapshot)
	}
	return manager, base, snapshot.Skills[0]
}

func task28RaceSnapshot(t *testing.T, manager *skills.Manager, sessionID string) skills.CatalogSnapshot {
	t.Helper()
	snapshot, err := manager.Snapshot(sessionID)
	if err != nil || snapshot.Validate() != nil {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	return snapshot
}

type task28RaceFaultStore struct {
	base                  skills.OverrideStore
	mu                    sync.Mutex
	compareErr            error
	restoreErr            error
	failSnapshotsAfterCAS int
	snapshotFailures      int
}

func (store *task28RaceFaultStore) Snapshot(sessionID string) (skills.OverrideSnapshot, error) {
	store.mu.Lock()
	if store.snapshotFailures > 0 {
		store.snapshotFailures--
		store.mu.Unlock()
		return skills.OverrideSnapshot{}, errors.New("injected live snapshot failure")
	}
	store.mu.Unlock()
	return store.base.Snapshot(sessionID)
}

func (store *task28RaceFaultStore) Set(sessionID string, override skills.VisibilityOverride) error {
	return store.base.Set(sessionID, override)
}

func (store *task28RaceFaultStore) Toggle(sessionID string, scope skills.SkillScope, id skills.SkillID) (skills.VisibilityOverride, error) {
	return store.base.Toggle(sessionID, scope, id)
}

func (store *task28RaceFaultStore) Reset(sessionID string, scope skills.SkillScope, id skills.SkillID) error {
	return store.base.Reset(sessionID, scope, id)
}

func (store *task28RaceFaultStore) CompareAndSetProject(expected skills.OverrideStoreRevision, id skills.SkillID, next *skills.VisibilityOverride) (skills.ProjectOverrideRestore, error) {
	store.mu.Lock()
	compareErr := store.compareErr
	store.mu.Unlock()
	if compareErr != nil {
		return skills.ProjectOverrideRestore{}, compareErr
	}
	restore, err := store.base.CompareAndSetProject(expected, id, next)
	if err == nil {
		store.mu.Lock()
		store.snapshotFailures += store.failSnapshotsAfterCAS
		store.mu.Unlock()
	}
	return restore, err
}

func (store *task28RaceFaultStore) RestoreProject(restore skills.ProjectOverrideRestore) error {
	store.mu.Lock()
	restoreErr := store.restoreErr
	store.mu.Unlock()
	if restoreErr != nil {
		return restoreErr
	}
	return store.base.RestoreProject(restore)
}

var _ skills.OverrideStore = (*task28RaceFaultStore)(nil)
