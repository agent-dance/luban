package skills

import (
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestProjectGenerationBindsSnapshotAndRejectsOldWorkspaceExecution(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeTask23Skill(t, filepath.Join(rootA, ".luban-code", "skills"), "only-a", "workspace A")
	writeTask23Skill(t, filepath.Join(rootB, ".luban-code", "skills"), "only-b", "workspace B")

	store, err := NewFileOverrideStore(rootA, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := ProjectDirs(rootA)
	if err != nil {
		t.Fatal(err)
	}
	manager := newManagerWithOverrideStore(store, dirs...)
	bindingA, err := manager.SnapshotBinding("generation-session")
	if err != nil {
		t.Fatal(err)
	}
	if err := bindingA.Validate(); err != nil {
		t.Fatalf("binding A invalid: %v", err)
	}
	if !task23HasSkill(bindingA.Snapshot, "only-a") || task23HasSkill(bindingA.Snapshot, "only-b") {
		t.Fatalf("binding A catalog = %+v", bindingA.Snapshot.Skills)
	}

	if _, err := manager.RefreshSnapshot("generation-session"); err != nil {
		t.Fatal(err)
	}
	if got := manager.ProjectGeneration(); got != bindingA.ProjectGeneration {
		t.Fatalf("ordinary refresh advanced project generation: %d -> %d", bindingA.ProjectGeneration, got)
	}
	if err := manager.ReplaceProjectSources(rootB); err != nil {
		t.Fatal(err)
	}
	bindingB, err := manager.SnapshotBinding("generation-session")
	if err != nil {
		t.Fatal(err)
	}
	if bindingB.ProjectGeneration == bindingA.ProjectGeneration {
		t.Fatalf("project retarget retained generation %d", bindingB.ProjectGeneration)
	}
	if task23HasSkill(bindingB.Snapshot, "only-a") || !task23HasSkill(bindingB.Snapshot, "only-b") {
		t.Fatalf("binding B catalog = %+v", bindingB.Snapshot.Skills)
	}

	var consumed atomic.Int32
	_, err = manager.ResolveLatest(SkillResolveRequest{
		SessionID: "generation-session", Selector: "only-a", Origin: InvocationOriginModel,
		ExpectedProjectGeneration: bindingA.ProjectGeneration,
	}, func(ResolvedSkill) error {
		consumed.Add(1)
		return nil
	})
	if !errors.Is(err, ErrSkillProjectGenerationChanged) {
		t.Fatalf("old generation resolve error = %v", err)
	}
	if consumed.Load() != 0 {
		t.Fatal("old generation crossed execution callback")
	}

	resolved, err := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "generation-session", Selector: "only-b", Origin: InvocationOriginModel,
		ExpectedProjectGeneration: bindingB.ProjectGeneration,
	}, func(current ResolvedSkill) error {
		consumed.Add(1)
		if current.Effective.Name != "only-b" {
			t.Fatalf("resolved wrong workspace skill: %+v", current.Effective)
		}
		return nil
	})
	if err != nil || resolved.Outcome != SkillResolveResolved || consumed.Load() != 1 {
		t.Fatalf("current generation resolve = %+v, consumed=%d, err=%v", resolved, consumed.Load(), err)
	}
}

func TestResolveRequiresPinnedProjectGeneration(t *testing.T) {
	manager := NewManager()
	for _, origin := range []InvocationOrigin{InvocationOriginModel, InvocationOriginUser} {
		_, err := manager.ResolveLatest(SkillResolveRequest{
			SessionID: "generation-session",
			Selector:  "missing",
			Origin:    origin,
		}, nil)
		if err == nil {
			t.Fatalf("%s-origin resolve accepted current-authority fallback", origin)
		}
	}
}

func TestProjectGenerationLinearizesOldReaderBeforeRetarget(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeTask23Skill(t, filepath.Join(rootA, ".luban-code", "skills"), "only-a", "workspace A")
	writeTask23Skill(t, filepath.Join(rootB, ".luban-code", "skills"), "only-b", "workspace B")

	store, err := NewFileOverrideStore(rootA, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := ProjectDirs(rootA)
	if err != nil {
		t.Fatal(err)
	}
	manager := newManagerWithOverrideStore(store, dirs...)
	bindingA, err := manager.SnapshotBinding("linear-session")
	if err != nil {
		t.Fatal(err)
	}
	planB, err := manager.PrepareProjectSources(rootB)
	if err != nil {
		t.Fatal(err)
	}

	readerEntered := make(chan struct{})
	releaseReader := make(chan struct{})
	readerDone := make(chan error, 1)
	go func() {
		result, resolveErr := manager.ResolveLatest(SkillResolveRequest{
			SessionID: "linear-session", Selector: "only-a", Origin: InvocationOriginModel,
			ExpectedProjectGeneration: bindingA.ProjectGeneration,
		}, func(current ResolvedSkill) error {
			if current.Effective.Name != "only-a" {
				return errors.New("old reader resolved a different workspace")
			}
			close(readerEntered)
			<-releaseReader
			return nil
		})
		if resolveErr == nil && result.Outcome != SkillResolveResolved {
			resolveErr = errors.New("old reader was not resolved")
		}
		readerDone <- resolveErr
	}()
	<-readerEntered

	writerDone := make(chan struct{})
	go func() {
		manager.ApplyProjectSources(planB)
		close(writerDone)
	}()
	select {
	case <-writerDone:
		t.Fatal("project retarget crossed an active old-generation resolver")
	case <-time.After(75 * time.Millisecond):
	}
	close(releaseReader)
	if err := <-readerDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-writerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("project retarget did not resume after old reader exited")
	}

	if got := manager.ProjectGeneration(); got == bindingA.ProjectGeneration {
		t.Fatalf("retarget did not advance generation: %d", got)
	}
	if _, err := manager.ResolveLatest(SkillResolveRequest{
		SessionID: "linear-session", Selector: "only-a", Origin: InvocationOriginModel,
		ExpectedProjectGeneration: bindingA.ProjectGeneration,
	}, nil); !errors.Is(err, ErrSkillProjectGenerationChanged) {
		t.Fatalf("completed retarget accepted old generation: %v", err)
	}
}

func TestProjectGenerationAllowsSameAuthorityDynamicDiscovery(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()
	writeTask23Skill(t, projectDir, "same-run-project", "same authority")
	manager := newCatalogManagerForTest(
		DirSource{Dir: userDir, Source: SourceUser},
		DirSource{Dir: projectDir, Source: SourceProject},
	)
	initial := manager.ProjectGeneration()
	dynamic := t.TempDir()
	writeTask23Skill(t, dynamic, "nearby-project", "same authority nearby")
	if err := manager.AddDirectoriesAtGeneration(initial, []string{dynamic, dynamic}); err != nil {
		t.Fatal(err)
	}
	if got := manager.ProjectGeneration(); got != initial {
		t.Fatalf("same-authority AddDirectoriesAtGeneration advanced generation: %d -> %d", initial, got)
	}
	snapshot, err := manager.SnapshotAtGeneration("dynamic-session", initial)
	if err != nil || !task23HasSkill(snapshot, "same-run-project") || !task23HasSkill(snapshot, "nearby-project") {
		t.Fatalf("same-generation dynamic snapshot = %+v, err=%v", snapshot.Skills, err)
	}
	if err := manager.AddDirectoriesAtGeneration(initial, []string{dynamic}); err != nil {
		t.Fatal(err)
	}
	if got := manager.ProjectGeneration(); got != initial {
		t.Fatalf("duplicate project directory advanced generation: %d -> %d", initial, got)
	}
}

func TestStaleProjectGenerationCannotMutateNewWorkspaceDiscovery(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	lateA := t.TempDir()
	lateB := t.TempDir()
	writeTask23Skill(t, filepath.Join(rootA, ".luban-code", "skills"), "base-a", "workspace A")
	writeTask23Skill(t, filepath.Join(rootB, ".luban-code", "skills"), "base-b", "workspace B")
	writeTask23Skill(t, lateA, "late-a", "stale A discovery")
	writeTask23Skill(t, lateB, "late-b", "live B discovery")

	store, err := NewFileOverrideStore(rootA, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := ProjectDirs(rootA)
	if err != nil {
		t.Fatal(err)
	}
	manager := newManagerWithOverrideStore(store, dirs...)
	bindingA, err := manager.SnapshotBinding("stale-mutator")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ReplaceProjectSources(rootB); err != nil {
		t.Fatal(err)
	}
	bindingB, err := manager.SnapshotBinding("stale-mutator")
	if err != nil {
		t.Fatal(err)
	}

	if err := manager.AddDirectoriesAtGeneration(bindingA.ProjectGeneration, []string{lateA}); !errors.Is(err, ErrSkillProjectGenerationChanged) {
		t.Fatalf("stale AddDirectoriesAtGeneration error = %v", err)
	}
	if err := manager.ActivateConditionalForPathAtGeneration(bindingA.ProjectGeneration, filepath.Join(lateA, "late-a")); !errors.Is(err, ErrSkillProjectGenerationChanged) {
		t.Fatalf("stale path activation error = %v", err)
	}
	unchanged, err := manager.SnapshotAtGeneration("stale-mutator", bindingB.ProjectGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if task23HasSkill(unchanged, "late-a") {
		t.Fatalf("stale A mutator polluted B: skills=%+v", unchanged.Skills)
	}

	if err := manager.AddDirectoriesAtGeneration(bindingB.ProjectGeneration, []string{lateB}); err != nil {
		t.Fatal(err)
	}
	if err := manager.ActivateConditionalForPathAtGeneration(bindingB.ProjectGeneration, filepath.Join(lateB, "late-b")); err != nil {
		t.Fatal(err)
	}
	current, err := manager.SnapshotAtGeneration("stale-mutator", bindingB.ProjectGeneration)
	if err != nil || !task23HasSkill(current, "late-b") || task23HasSkill(current, "late-a") {
		t.Fatalf("live B mutation = %+v, err=%v", current.Skills, err)
	}
}

func TestSameProjectRetargetKeepsGenerationForLiveConsumers(t *testing.T) {
	root := t.TempDir()
	writeTask23Skill(t, filepath.Join(root, ".luban-code", "skills"), "same-root", "same workspace")
	store, err := NewFileOverrideStore(root, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := ProjectDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	manager := newManagerWithOverrideStore(store, dirs...)
	if err := manager.ReplaceProjectSources(root); err != nil {
		t.Fatal(err)
	}
	binding, err := manager.SnapshotBinding("same-root-session")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ReplaceProjectSources(root); err != nil {
		t.Fatal(err)
	}
	if got := manager.ProjectGeneration(); got != binding.ProjectGeneration {
		t.Fatalf("same-root retarget invalidated live generation: %d -> %d", binding.ProjectGeneration, got)
	}
	if _, err := manager.SnapshotAtGeneration("same-root-session", binding.ProjectGeneration); err != nil {
		t.Fatalf("same-root live consumer became stale: %v", err)
	}
}

func TestProjectSourceAfterPublishRemainsInsideWriterTransaction(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeTask23Skill(t, filepath.Join(rootA, ".luban-code", "skills"), "publish-a", "workspace A")
	writeTask23Skill(t, filepath.Join(rootB, ".luban-code", "skills"), "publish-b", "workspace B")
	store, err := NewFileOverrideStore(rootA, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := ProjectDirs(rootA)
	if err != nil {
		t.Fatal(err)
	}
	manager := newManagerWithOverrideStore(store, dirs...)
	planB, err := manager.PrepareProjectSources(rootB)
	if err != nil {
		t.Fatal(err)
	}

	var publishedRoot atomic.Value
	publishedRoot.Store(rootA)
	afterEntered := make(chan struct{})
	releaseAfter := make(chan struct{})
	commitDone := make(chan error, 1)
	go func() {
		commitDone <- manager.CommitProjectSourcesWithAfter(planB, nil, func() {
			publishedRoot.Store(rootB)
			close(afterEntered)
			<-releaseAfter
		})
	}()
	<-afterEntered
	if got := publishedRoot.Load().(string); got != rootB {
		t.Fatalf("afterPublish root = %q, want B", got)
	}

	observed := make(chan CatalogSnapshot, 1)
	observeErr := make(chan error, 1)
	go func() {
		snapshot, snapshotErr := manager.Snapshot("after-publish-observer")
		if snapshotErr != nil {
			observeErr <- snapshotErr
			return
		}
		observed <- snapshot
	}()
	select {
	case snapshot := <-observed:
		t.Fatalf("Manager reader crossed afterPublish writer transaction: %+v", snapshot.Skills)
	case err := <-observeErr:
		t.Fatalf("Manager reader returned during afterPublish: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseAfter)
	if err := <-commitDone; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-observeErr:
		t.Fatal(err)
	case snapshot := <-observed:
		if task23HasSkill(snapshot, "publish-a") || !task23HasSkill(snapshot, "publish-b") {
			t.Fatalf("post-publish observer saw mixed catalog: %+v", snapshot.Skills)
		}
	case <-time.After(time.Second):
		t.Fatal("Manager reader remained blocked after afterPublish returned")
	}
}
