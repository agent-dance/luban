package swarm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agent-dance/luban/brand"
)

// teamMailbox overrides the home-dir resolution for tests by pointing
// teamConfigPath at a temp directory via a helper that patches the function.
// Since teamConfigPath uses os.UserHomeDir, we redirect HOME instead.

// withTempHome temporarily sets HOME (and USERPROFILE on windows-compatible
// paths) to dir, runs f, then restores the original value.
func withTempHome(t *testing.T, f func(dir string)) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	f(dir)
}

// ---- SaveTeamConfig / LoadTeamConfig ----

func testTeamDir(home, name string) string {
	return filepath.Join(home, brand.ConfigDirName, "teams", name)
}

func testTeamConfigPath(home, name string) string {
	return filepath.Join(testTeamDir(home, name), "team.json")
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	withTempHome(t, func(home string) {
		cfg := &TeamConfig{
			Name:        "alpha",
			Description: "test team",
			CreatedAt:   time.Now().Unix(),
			LeadAgentID: "leader-1",
			Members: []TeamMember{
				{
					AgentID:    "worker-1",
					Name:       "worker-1",
					Color:      "blue",
					TmuxPaneID: "%5",
					CWD:        "/tmp/work",
					IsActive:   true,
				},
			},
		}

		if err := SaveTeamConfig(cfg); err != nil {
			t.Fatalf("SaveTeamConfig: %v", err)
		}

		// Verify file exists.
		path := testTeamConfigPath(home, "alpha")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected config file at %s: %v", path, err)
		}

		loaded, err := LoadTeamConfig("alpha")
		if err != nil {
			t.Fatalf("LoadTeamConfig: %v", err)
		}

		if loaded.Name != cfg.Name {
			t.Errorf("Name: got %q, want %q", loaded.Name, cfg.Name)
		}
		if loaded.Description != cfg.Description {
			t.Errorf("Description: got %q, want %q", loaded.Description, cfg.Description)
		}
		if loaded.LeadAgentID != cfg.LeadAgentID {
			t.Errorf("LeadAgentID: got %q, want %q", loaded.LeadAgentID, cfg.LeadAgentID)
		}
		if len(loaded.Members) != 1 {
			t.Fatalf("Members: got %d, want 1", len(loaded.Members))
		}
		m := loaded.Members[0]
		if m.AgentID != "worker-1" || m.TmuxPaneID != "%5" || !m.IsActive {
			t.Errorf("member mismatch: %+v", m)
		}
	})
}

func TestSave_CreatesDirectory(t *testing.T) {
	withTempHome(t, func(home string) {
		cfg := &TeamConfig{
			Name:        "beta",
			LeadAgentID: "lead",
		}
		if err := SaveTeamConfig(cfg); err != nil {
			t.Fatalf("SaveTeamConfig: %v", err)
		}

		dir := testTeamDir(home, "beta")
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected directory %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("expected directory, got file")
		}
	})
}

func TestLoad_NotFound(t *testing.T) {
	withTempHome(t, func(_ string) {
		_, err := LoadTeamConfig("nonexistent-team")
		if err == nil {
			t.Error("expected error loading nonexistent team")
		}
	})
}

func TestSave_NilConfig(t *testing.T) {
	if err := SaveTeamConfig(nil); err == nil {
		t.Error("expected error saving nil config")
	}
}

// ---- DeleteTeamConfig ----

func TestDelete_RemovesFile(t *testing.T) {
	withTempHome(t, func(home string) {
		cfg := &TeamConfig{Name: "gamma", LeadAgentID: "lead"}
		_ = SaveTeamConfig(cfg)

		path := testTeamConfigPath(home, "gamma")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("config not saved: %v", err)
		}

		if err := DeleteTeamConfig("gamma"); err != nil {
			t.Fatalf("DeleteTeamConfig: %v", err)
		}

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("expected config file to be removed")
		}
	})
}

func TestDelete_RetainsStableLockDirectory(t *testing.T) {
	withTempHome(t, func(home string) {
		cfg := &TeamConfig{Name: "delta", LeadAgentID: "lead"}
		_ = SaveTeamConfig(cfg)

		_ = DeleteTeamConfig("delta")

		dir := testTeamDir(home, "delta")
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("expected team lock directory to remain: %v", err)
		}
		if _, err := os.Stat(testTeamConfigPath(home, "delta")); !os.IsNotExist(err) {
			t.Errorf("expected config to be removed, got %v", err)
		}
	})
}

func TestDelete_NonexistentIsNoop(t *testing.T) {
	withTempHome(t, func(_ string) {
		if err := DeleteTeamConfig("does-not-exist"); err != nil {
			t.Errorf("DeleteTeamConfig on nonexistent: %v", err)
		}
	})
}

func TestDelete_PreservesNonEmptyDir(t *testing.T) {
	withTempHome(t, func(home string) {
		cfg := &TeamConfig{Name: "epsilon", LeadAgentID: "lead"}
		_ = SaveTeamConfig(cfg)

		// Add a sibling file so the directory is not empty after deletion.
		dir := testTeamDir(home, "epsilon")
		extra := filepath.Join(dir, "extra.txt")
		_ = os.WriteFile(extra, []byte("keep"), 0o644)

		_ = DeleteTeamConfig("epsilon")

		// Directory should still exist because it's not empty.
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("expected directory to remain when non-empty: %v", err)
		}
	})
}

// ---- TeamMember fields ----

func TestTeamMember_Fields(t *testing.T) {
	withTempHome(t, func(_ string) {
		cfg := &TeamConfig{
			Name:        "zeta",
			LeadAgentID: "lead",
			Members: []TeamMember{
				{
					AgentID:    "a1",
					Name:       "Alice",
					Color:      "red",
					TmuxPaneID: "%10",
					CWD:        "/projects/foo",
					IsActive:   false,
				},
			},
		}
		_ = SaveTeamConfig(cfg)

		loaded, _ := LoadTeamConfig("zeta")
		if len(loaded.Members) != 1 {
			t.Fatal("expected 1 member")
		}
		got := loaded.Members[0]
		if got.AgentID != "a1" {
			t.Errorf("AgentID: %q", got.AgentID)
		}
		if got.Name != "Alice" {
			t.Errorf("Name: %q", got.Name)
		}
		if got.Color != "red" {
			t.Errorf("Color: %q", got.Color)
		}
		if got.TmuxPaneID != "%10" {
			t.Errorf("TmuxPaneID: %q", got.TmuxPaneID)
		}
		if got.CWD != "/projects/foo" {
			t.Errorf("CWD: %q", got.CWD)
		}
		if got.IsActive {
			t.Error("IsActive should be false")
		}
	})
}
