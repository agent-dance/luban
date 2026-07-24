package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/terminaltheme"
)

func TestConfigureTerminalThemeUsesProjectTheme(t *testing.T) {
	project := t.TempDir()
	settingsDir := filepath.Join(project, brand.ConfigDirName)
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(`{"theme":"burgundy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { terminaltheme.Configure("system") })

	if err := configureTerminalTheme(project); err != nil {
		t.Fatalf("configureTerminalTheme: %v", err)
	}
	if got := terminaltheme.Current().Name; got != "burgundy" {
		t.Fatalf("theme = %q, want burgundy", got)
	}
}

func TestConfigureTerminalThemeDefaultsToSystemColors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Cleanup(func() { terminaltheme.Configure("system") })

	if err := configureTerminalTheme(t.TempDir()); err != nil {
		t.Fatalf("configureTerminalTheme: %v", err)
	}
	palette := terminaltheme.Current()
	if palette.Name != "system" || palette.Background != "" || palette.Foreground != "" {
		t.Fatalf("theme = %+v, want terminal-owned foreground and background", palette)
	}
}
