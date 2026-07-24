package session

import (
	"path/filepath"
	"testing"
)

func TestRepositoryArtifactsDirRejectsTraversalWithoutCurrentProject(t *testing.T) {
	repo := NewRepository(t.TempDir())
	got := repo.ArtifactsDir("../outside", "")
	want := filepath.Join(repo.LegacyRoot(), ".invalid-session-id")
	if got != want {
		t.Fatalf("ArtifactsDir traversal = %q, want safe sentinel %q", got, want)
	}
	if rel, err := filepath.Rel(repo.LegacyRoot(), got); err != nil || filepath.IsAbs(rel) || rel == ".." {
		t.Fatalf("ArtifactsDir traversal escaped legacy root: path=%q rel=%q err=%v", got, rel, err)
	}
}
