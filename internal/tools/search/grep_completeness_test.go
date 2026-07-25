package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestGrepPaginationCarriesTypedCompleteness(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "matches.txt"), []byte("needle one\nneedle two\nneedle three\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrep(nil).Execute(context.Background(), map[string]any{
		"pattern": "needle", "path": dir, "output_mode": "content", "head_limit": float64(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != types.ToolOutcomePartial ||
		result.Completeness.Source != types.ToolResultCompletenessComplete ||
		result.Completeness.View != types.ToolResultCompletenessPagination {
		t.Fatalf("pagination result = %+v", result)
	}
	if result.Completeness.Pagination == nil || result.Completeness.Pagination.NextOffset != 2 {
		t.Fatalf("pagination continuation = %+v", result.Completeness.Pagination)
	}
}

func TestGrepOffsetAloneIsStillPagination(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "matches.txt"), []byte("needle one\nneedle two\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrep(nil).Execute(context.Background(), map[string]any{
		"pattern": "needle", "path": dir, "output_mode": "content", "offset": float64(1), "head_limit": float64(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != types.ToolOutcomePartial ||
		result.Completeness.View != types.ToolResultCompletenessPagination ||
		result.Completeness.Pagination == nil ||
		result.Completeness.Pagination.Offset != 1 {
		t.Fatalf("offset result = %+v", result)
	}
}
