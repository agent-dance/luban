package types

import "testing"

func TestToolResultCompletenessSeparatesSourceAndView(t *testing.T) {
	complete := ToolResultCompleteness{Source: ToolResultCompletenessComplete}
	if complete.IsIncomplete() || !complete.CanRetainFullEvidence() {
		t.Fatalf("complete provenance = %+v", complete)
	}

	pagination := complete
	pagination.View = ToolResultCompletenessPagination
	pagination.Pagination = &ToolResultPagination{Offset: 20, Limit: 10, NextOffset: 30, HasMore: true}
	if !pagination.IsIncomplete() || pagination.CanRetainFullEvidence() {
		t.Fatalf("pagination provenance = %+v", pagination)
	}
	cloned := pagination.Clone()
	cloned.Pagination.NextOffset = 99
	if pagination.Pagination.NextOffset != 30 {
		t.Fatalf("Clone shared pagination state: original=%+v clone=%+v", pagination, cloned)
	}

	preview := complete.WithDisplayPreview()
	if !preview.IsIncomplete() || preview.RetainedResultIncomplete() || preview.CanRetainFullEvidence() {
		t.Fatalf("display preview provenance = %+v", preview)
	}

	for _, source := range []ToolResultCompletenessKind{ToolResultCompletenessSourceTruncated, ToolResultCompletenessCaptureDropped} {
		value := ToolResultCompleteness{Source: source}
		if !value.RetainedResultIncomplete() || value.CanRetainFullEvidence() {
			t.Fatalf("source provenance %q = %+v", source, value)
		}
	}
}
