package architecture_test

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestRemovedRootPackagesHaveNoFilesystemFacade(t *testing.T) {
	root := architectureModuleRoot(t)
	var violations []string
	for _, name := range []string{
		"compact", "config", "coordinator", "engine", "goal", "input", "loop",
		"mcp", "render", filepath.Join("services", "mcp"), "session",
		"terminaltheme", "tmux", "tools", "tui", "ui",
	} {
		path := filepath.Join(root, name)
		_, err := os.Lstat(path)
		switch {
		case err == nil:
			violations = append(violations, name+" still exists at the repository root")
		case errors.Is(err, os.ErrNotExist):
			continue
		default:
			t.Fatalf("inspect removed root package %s: %v", name, err)
		}
	}
	rootEntries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read repository root: %v", err)
	}
	for _, entry := range rootEntries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".go") {
			violations = append(violations, entry.Name()+" restores a module-root Go package; entry points belong under cmd")
		}
	}
	if len(violations) != 0 {
		sort.Strings(violations)
		t.Fatalf("removed root package facades returned:\n%s", strings.Join(violations, "\n"))
	}
}
