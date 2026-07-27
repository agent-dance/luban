package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeSessionDirIsExternalAndIsolated(t *testing.T) {
	configHome := t.TempDir()
	projectA := filepath.Join(t.TempDir(), "repo")
	projectB := filepath.Join(t.TempDir(), "repo")

	a1 := RuntimeSessionDirAt(configHome, projectA, "session/a")
	a1Again := RuntimeSessionDirAt(configHome, projectA, "session/a")
	a2 := RuntimeSessionDirAt(configHome, projectA, "session-a")
	b1 := RuntimeSessionDirAt(configHome, projectB, "session/a")

	if a1 != a1Again {
		t.Fatalf("same runtime identity changed: %q != %q", a1, a1Again)
	}
	if a1 == a2 {
		t.Fatalf("sanitizer-colliding sessions share %q", a1)
	}
	if a1 == b1 {
		t.Fatalf("different projects share %q", a1)
	}
	for _, got := range []string{a1, a2, b1} {
		configRuntimeRoot := filepath.Join(canonicalizeRuntimePath(configHome), "runtime")
		if !strings.HasPrefix(got, configRuntimeRoot+string(filepath.Separator)) {
			t.Fatalf("runtime path %q escaped config home %q", got, configHome)
		}
		if strings.HasPrefix(got, filepath.Clean(projectA)+string(filepath.Separator)) ||
			strings.HasPrefix(got, filepath.Clean(projectB)+string(filepath.Separator)) {
			t.Fatalf("runtime path %q is inside a user project", got)
		}
	}
}

func TestRuntimeProcessAndServiceNamespacesAreStableAndSeparate(t *testing.T) {
	configHome := t.TempDir()
	project := t.TempDir()
	processA := RuntimeSessionDirAt(configHome, project, "")
	processB := RuntimeSessionDirAt(configHome, project, "")
	schedule := RuntimeServiceDirAt(configHome, project, "schedule")
	memory := RuntimeServiceDirAt(configHome, project, "agent-memory")

	if processA != processB {
		t.Fatalf("process namespace changed: %q != %q", processA, processB)
	}
	if processA == schedule || schedule == memory {
		t.Fatalf("runtime namespaces are not isolated: process=%q schedule=%q memory=%q", processA, schedule, memory)
	}
}
