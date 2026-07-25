package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScheduleImplementationHasSingleInternalOwner(t *testing.T) {
	root := architectureModuleRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal", "tools", "schedule"))
	if err != nil {
		t.Fatalf("read internal schedule owner: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && filepath.Ext(name) == ".go" && !strings.HasSuffix(name, "_test.go") {
			return
		}
	}
	if len(entries) == 0 {
		t.Fatal("internal/tools/schedule has no implementation")
	}
	t.Fatal("internal/tools/schedule has no production Go implementation")
}
