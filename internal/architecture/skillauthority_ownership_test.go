package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentUsesCanonicalSkillAuthorityOwner(t *testing.T) {
	root := architectureModuleRoot(t)
	agentRoot := filepath.Join(root, "internal", "agent")
	forbidden := []string{
		"type toolSkillAuthority",
		"validateToolSkillAuthority",
		"sameToolRuntimePath",
		"withGenerationLease",
	}
	canonicalImport := modulePath + "/internal/runtime/skillauthority"
	importedCanonical := false

	err := filepath.WalkDir(agentRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(data)
		if strings.Contains(source, canonicalImport) {
			importedCanonical = true
		}
		for _, symbol := range forbidden {
			if strings.Contains(source, symbol) {
				t.Errorf("%s restores duplicate skill authority symbol %q", path, symbol)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !importedCanonical {
		t.Fatal("internal/agent does not consume the canonical runtime skill authority")
	}
}
