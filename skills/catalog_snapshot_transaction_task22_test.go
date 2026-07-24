package skills

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConsumeSnapshotBlocksRefreshAndReturnsDefensiveSnapshot(t *testing.T) {
	root := t.TempDir()
	task22WriteTransactionalSkill(t, root, "transactional", "initial")
	manager := NewManager(DirSource{Dir: root, Source: SourceProject})

	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	consumeDone := make(chan error, 1)
	go func() {
		consumeDone <- manager.ConsumeSnapshot("task22-session", func(snapshot CatalogSnapshot) error {
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
		manager.Refresh()
		close(refreshDone)
	}()
	select {
	case <-refreshDone:
		t.Fatal("Refresh crossed an active ConsumeSnapshot read transaction")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCallback)
	if err := <-consumeDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-refreshDone:
	case <-time.After(time.Second):
		t.Fatal("Refresh remained blocked after ConsumeSnapshot returned")
	}

	snapshot, err := manager.Snapshot("task22-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Skills) != 1 || snapshot.Skills[0].Summary != "initial" {
		t.Fatalf("callback mutated Manager snapshot: %#v", snapshot)
	}
}

func TestConsumeSnapshotBlocksVisibilityWriterAndPropagatesCallbackError(t *testing.T) {
	root := t.TempDir()
	task22WriteTransactionalSkill(t, root, "visibility", "visible")
	manager := NewManager(DirSource{Dir: root, Source: SourceProject})
	snapshot, err := manager.Snapshot("task22-visibility")
	if err != nil || len(snapshot.Skills) != 1 {
		t.Fatalf("snapshot = %#v err=%v", snapshot, err)
	}
	name := snapshot.Skills[0].Name

	wantErr := errors.New("task22 callback stopped")
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	consumeDone := make(chan error, 1)
	go func() {
		consumeDone <- manager.ConsumeSnapshot("task22-visibility", func(CatalogSnapshot) error {
			close(callbackStarted)
			<-releaseCallback
			return wantErr
		})
	}()
	<-callbackStarted

	writerDone := make(chan bool, 1)
	go func() {
		changed, found := manager.SetEnabled("task22-visibility", name, false)
		writerDone <- changed && found
	}()
	select {
	case result := <-writerDone:
		t.Fatalf("visibility writer crossed transaction: result=%t", result)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCallback)
	if err := <-consumeDone; !errors.Is(err, wantErr) {
		t.Fatalf("ConsumeSnapshot error = %v, want %v", err, wantErr)
	}
	select {
	case ok := <-writerDone:
		if !ok {
			t.Fatal("visibility writer did not commit after transaction")
		}
	case <-time.After(time.Second):
		t.Fatal("visibility writer remained blocked after callback error")
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
