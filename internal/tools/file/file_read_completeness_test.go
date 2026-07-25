package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestReadOversizeCarriesCaptureDroppedProvenance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oversize.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", defaultReadMaxSizeBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := (&FileReadTool{AllowedDirs: []string{dir}}).Execute(
		context.Background(), map[string]any{"file_path": path},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != types.ToolOutcomeFailed ||
		result.Completeness.Source != types.ToolResultCompletenessCaptureDropped ||
		!result.IsError {
		t.Fatalf("oversize result = %+v", result)
	}
	if strings.Contains(result.Content, "capture_dropped") {
		t.Fatalf("protocol provenance leaked into localized copy: %q", result.Content)
	}
}
