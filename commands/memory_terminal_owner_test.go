package commands

import (
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

func TestMemoryCommandUsesTerminalOwnedEditorCallback(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDITOR", "editor-from-test")
	var opened string
	command := &memoryCmd{}
	err := command.Execute(&Context{
		Language: i18n.LangEN,
		CWD:      root,
		OnEvent:  func(string) {},
		OpenFileEditor: func(path string) error {
			opened = path
			return nil
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, brand.InstructionsFile)
	if opened != want {
		t.Fatalf("terminal-owned editor opened %q, want %q", opened, want)
	}
}
