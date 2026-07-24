package tools

// PreviewMode describes how the UI should render previews for a question.
//
//	PreviewModeNone        — stack options vertically (no preview pane)
//	PreviewModeSideBySide  — show preview pane next to options
type PreviewMode int

const (
	PreviewModeNone PreviewMode = iota
	PreviewModeSideBySide
)

// ResolvePreviewMode returns the preview rendering mode for a question.
// Multi-select questions never use previews (UI cannot disambiguate).
func ResolvePreviewMode(q QuestionSpec) PreviewMode {
	if q.MultiSelect {
		return PreviewModeNone
	}
	if QuestionHasPreview(q) {
		return PreviewModeSideBySide
	}
	return PreviewModeNone
}

// String renders PreviewMode as the canonical UI tag used by the TS client.
func (m PreviewMode) String() string {
	switch m {
	case PreviewModeSideBySide:
		return "side-by-side"
	default:
		return "stacked"
	}
}
