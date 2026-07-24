package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestGrepPaginationCarriesTypedCompleteness(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "matches.txt"), []byte("needle one\nneedle two\nneedle three\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern": "needle", "path": dir, "output_mode": "content", "head_limit": float64(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != types.ToolOutcomePartial || result.Completeness.Source != types.ToolResultCompletenessComplete || result.Completeness.View != types.ToolResultCompletenessPagination {
		t.Fatalf("pagination result = %+v", result)
	}
	if result.Completeness.Pagination == nil || result.Completeness.Pagination.NextOffset != 2 {
		t.Fatalf("pagination continuation = %+v", result.Completeness.Pagination)
	}
	output, ok := result.Data.(GrepOutput)
	if !ok || output.Completeness.View != types.ToolResultCompletenessPagination {
		t.Fatalf("typed Grep output = %+v (%T)", result.Data, result.Data)
	}
}

func TestGrepOffsetAloneIsStillPagination(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "matches.txt"), []byte("needle one\nneedle two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern": "needle", "path": dir, "output_mode": "content", "offset": float64(1), "head_limit": float64(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != types.ToolOutcomePartial || result.Completeness.View != types.ToolResultCompletenessPagination || result.Completeness.Pagination == nil || result.Completeness.Pagination.Offset != 1 {
		t.Fatalf("offset result = %+v", result)
	}
}

func TestGrepCaptureDroppedCannotClaimFullEvidence(t *testing.T) {
	completeness := grepResultCompleteness(grepPartialStdoutCap, false, false, 0, 0, 25)
	if completeness.Source != types.ToolResultCompletenessCaptureDropped || completeness.CanRetainFullEvidence() {
		t.Fatalf("stdout-cap completeness = %+v", completeness)
	}
}

func TestReadOversizeCarriesCaptureDroppedProvenance(t *testing.T) {
	result := fileReadSizeLimitError("oversize.txt", 4096, 1024)
	if result.Outcome != types.ToolOutcomeFailed || result.Completeness.Source != types.ToolResultCompletenessCaptureDropped || !result.IsError {
		t.Fatalf("oversize result = %+v", result)
	}
	if strings.Contains(result.Content, "capture_dropped") {
		t.Fatalf("protocol provenance leaked into localized copy: %q", result.Content)
	}
}
