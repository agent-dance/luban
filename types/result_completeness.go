package types

// ToolResultCompletenessKind is a stable, language-independent provenance
// value. Source and View use the same closed vocabulary so persisted tool
// results can be inspected without parsing localized output or metadata prose.
type ToolResultCompletenessKind string

const (
	ToolResultCompletenessUnknown         ToolResultCompletenessKind = ""
	ToolResultCompletenessComplete        ToolResultCompletenessKind = "complete"
	ToolResultCompletenessPagination      ToolResultCompletenessKind = "pagination"
	ToolResultCompletenessSourceTruncated ToolResultCompletenessKind = "source_truncated"
	ToolResultCompletenessCaptureDropped  ToolResultCompletenessKind = "capture_dropped"
	ToolResultCompletenessDisplayPreview  ToolResultCompletenessKind = "display_preview"
)

// ToolResultPagination identifies the exact page represented by a result and
// the next offset the caller can request. It never claims that omitted entries
// were retained elsewhere.
type ToolResultPagination struct {
	Offset     int  `json:"offset"`
	Limit      int  `json:"limit"`
	NextOffset int  `json:"next_offset"`
	HasMore    bool `json:"has_more"`
}

// ToolResultCompleteness separates acquisition loss from presentation loss.
// Source is complete/source_truncated/capture_dropped. View is empty,
// pagination, or display_preview. A paginated result may also have an
// incomplete source, so the two dimensions cannot safely be collapsed into a
// single truncated boolean.
type ToolResultCompleteness struct {
	Source     ToolResultCompletenessKind `json:"source,omitempty"`
	View       ToolResultCompletenessKind `json:"view,omitempty"`
	Pagination *ToolResultPagination      `json:"pagination,omitempty"`
}

// IsZero reports whether an older producer supplied no provenance.
func (c ToolResultCompleteness) IsZero() bool {
	return c.Source == ToolResultCompletenessUnknown && c.View == ToolResultCompletenessUnknown && c.Pagination == nil
}

// IsIncomplete reports whether the produced result or its current projection
// omits information.
func (c ToolResultCompleteness) IsIncomplete() bool {
	return c.Source == ToolResultCompletenessSourceTruncated ||
		c.Source == ToolResultCompletenessCaptureDropped ||
		c.View == ToolResultCompletenessPagination ||
		c.View == ToolResultCompletenessDisplayPreview
}

// RetainedResultIncomplete reports loss in the tool-produced result itself.
// A display preview is excluded because the immutable retained result remains
// complete and presentation policy should still be free to keep it folded.
func (c ToolResultCompleteness) RetainedResultIncomplete() bool {
	return c.Source == ToolResultCompletenessSourceTruncated ||
		c.Source == ToolResultCompletenessCaptureDropped ||
		c.View == ToolResultCompletenessPagination
}

// CanRetainFullEvidence reports whether storing the exact ToolResult content
// can produce a full evidence reference. Any producer-declared view loss fails
// closed. A UI may add display_preview only after it has retained the exact
// source result and established the reference independently.
func (c ToolResultCompleteness) CanRetainFullEvidence() bool {
	return c.Source == ToolResultCompletenessComplete && c.View == ToolResultCompletenessUnknown
}

// WithDisplayPreview records a presentation-only preview without overwriting
// stronger source or pagination provenance.
func (c ToolResultCompleteness) WithDisplayPreview() ToolResultCompleteness {
	if c.View == ToolResultCompletenessUnknown {
		c.View = ToolResultCompletenessDisplayPreview
	}
	return c
}

// Clone returns an independent value suitable for persisted observations and
// concurrent presentation snapshots.
func (c ToolResultCompleteness) Clone() ToolResultCompleteness {
	if c.Pagination != nil {
		pagination := *c.Pagination
		c.Pagination = &pagination
	}
	return c
}
