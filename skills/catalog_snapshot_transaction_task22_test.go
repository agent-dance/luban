package skills

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConsumeSnapshotAtGenerationBlocksRefreshAndReturnsDefensiveSnapshot(t *testing.T) {
	root := t.TempDir()
	task22WriteTransactionalSkill(t, root, "transactional", "initial")
	manager := newCatalogManagerForTest(DirSource{Dir: root, Source: SourceProject})
	binding, err := manager.SnapshotBinding("task22-session")
	if err != nil {
		t.Fatal(err)
	}

	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	consumeDone := make(chan error, 1)
	go func() {
		consumeDone <- manager.ConsumeSnapshotAtGeneration("task22-session", binding.ProjectGeneration, func(snapshot CatalogSnapshot) error {
			if len(snapshot.Skills) != 1 || snapshot.Skills[0].Summary != "initial" {
				return errors.New("unexpected transactional snapshot")
			}
			// Mutating the callback value must not change Manager state.
			snapshot.Skills[0].Summary = "callback mutation"
			close(callbackStarted)
			<-releaseCallback
			return nil
		})
	}()
	<-callbackStarted

	refreshDone := make(chan struct{})
	go func() {
		_, _ = manager.RefreshSnapshot("task22-session")
		close(refreshDone)
	}()
	select {
	case <-refreshDone:
		t.Fatal("RefreshSnapshot crossed an active ConsumeSnapshotAtGeneration read transaction")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCallback)
	if err := <-consumeDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-refreshDone:
	case <-time.After(time.Second):
		t.Fatal("RefreshSnapshot remained blocked after ConsumeSnapshotAtGeneration returned")
	}

	snapshot, err := manager.Snapshot("task22-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Skills) != 1 || snapshot.Skills[0].Summary != "initial" {
		t.Fatalf("callback mutated Manager snapshot: %#v", snapshot)
	}
}

func task22WriteTransactionalSkill(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ndescription: " + description + "\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
