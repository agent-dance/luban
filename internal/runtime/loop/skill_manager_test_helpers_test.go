package loop

import (
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/skills"
)

// newLoopTestSkillManager creates a production-shaped manager with explicit
// persistent and session override layers. Tests must not rely on an
// unconfigured Manager silently accepting catalog operations.
func newLoopTestSkillManager(t testing.TB, dirs ...skills.DirSource) *skills.Manager {
	t.Helper()
	settingsDir := t.TempDir()
	store, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings:    filepath.Join(settingsDir, "user", "settings.json"),
		ProjectSettings: filepath.Join(settingsDir, "project", "settings.json"),
	}, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(dirs...)
	manager.SetOverrideStore(store)
	return manager
}
