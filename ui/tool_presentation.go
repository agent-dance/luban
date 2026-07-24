package ui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
)

const (
	ToolPresentationStateStarted   = "started"
	ToolPresentationStateRunning   = "running"
	ToolPresentationStateSucceeded = "succeeded"
	ToolPresentationStateFailed    = "failed"
	ToolPresentationStatePartial   = "partial"
	ToolPresentationStateDenied    = "denied"
	ToolPresentationStateCancelled = "cancelled"
	ToolPresentationStateTimedOut  = "timed_out"
	ToolPresentationStateUnknown   = "unknown"

	ToolPresentationLevelHidden     = "hidden_member"
	ToolPresentationLevelFolded     = "folded"
	ToolPresentationLevelStructured = "structured"
	ToolPresentationLevelEvidence   = "evidence"

	maxToolPresentationIdentityRunes = 160
	maxToolPresentationSummaryRunes  = 360
	maxToolPresentationDetailRunes   = 240
	maxToolPresentationDetails       = 3
	maxToolPresentationReasons       = 8
)

// ToolPresentation is the renderer-neutral, already-redacted projection of a
// semantic tool transition. State is an execution fact supplied by the caller;
// renderers must not infer it from Result or DetailLines.
//
// Raw tool input and output deliberately do not belong in this DTO. HasMore
// preserves the path to retained evidence without flooding append-only output.
type ToolPresentation struct {
	ToolName          string
	ToolUseID         string
	WorkUnitID        string
	Actor             string
	Action            string
	Object            string
	State             string
	Result            string
	NextAction        string
	DetailLines       []string
	PresentationLevel string
	ReasonCodes       []string
	HasMore           bool
	Redacted          bool
}

// SemanticToolRenderer is an optional capability. Existing Renderer methods
// remain the compatibility surface for callers that still provide raw events.
type SemanticToolRenderer interface {
	RenderToolPresentation(ToolPresentation)
}

type normalizedToolPresentation struct {
	ToolPresentation
	omittedDetails int
	omittedReasons int
}

type toolPresentationField struct {
	label string
	value string
}

func normalizeToolPresentation(value ToolPresentation) normalizedToolPresentation {
	value.ToolName = boundedToolPresentationText(value.ToolName, maxToolPresentationIdentityRunes)
	value.ToolUseID = boundedToolPresentationText(value.ToolUseID, maxToolPresentationIdentityRunes)
	value.WorkUnitID = boundedToolPresentationText(value.WorkUnitID, maxToolPresentationIdentityRunes)
	value.Actor = boundedToolPresentationText(value.Actor, maxToolPresentationIdentityRunes)
	value.Action = boundedToolPresentationText(value.Action, maxToolPresentationSummaryRunes)
	value.Object = boundedToolPresentationText(value.Object, maxToolPresentationSummaryRunes)
	value.State = boundedToolPresentationText(value.State, maxToolPresentationIdentityRunes)
	if value.State == "" {
		value.State = ToolPresentationStateUnknown
	}
	value.Result = boundedToolPresentationText(value.Result, maxToolPresentationSummaryRunes)
	value.NextAction = boundedToolPresentationText(value.NextAction, maxToolPresentationSummaryRunes)
	value.PresentationLevel = boundedToolPresentationText(value.PresentationLevel, maxToolPresentationIdentityRunes)
	if value.PresentationLevel == "" {
		value.PresentationLevel = ToolPresentationLevelFolded
	}
	if value.PresentationLevel == ToolPresentationLevelFolded {
		value.DetailLines = nil
		value.ReasonCodes = nil
	}

	normalized := normalizedToolPresentation{ToolPresentation: value}
	if len(value.DetailLines) > maxToolPresentationDetails {
		normalized.omittedDetails = len(value.DetailLines) - maxToolPresentationDetails
		value.DetailLines = value.DetailLines[:maxToolPresentationDetails]
	}
	normalized.DetailLines = make([]string, 0, len(value.DetailLines))
	for _, detail := range value.DetailLines {
		if detail = boundedToolPresentationText(detail, maxToolPresentationDetailRunes); detail != "" {
			normalized.DetailLines = append(normalized.DetailLines, detail)
		}
	}

	if len(value.ReasonCodes) > maxToolPresentationReasons {
		normalized.omittedReasons = len(value.ReasonCodes) - maxToolPresentationReasons
		value.ReasonCodes = value.ReasonCodes[:maxToolPresentationReasons]
	}
	normalized.ReasonCodes = make([]string, 0, len(value.ReasonCodes))
	for _, reason := range value.ReasonCodes {
		if reason = boundedToolPresentationText(reason, maxToolPresentationIdentityRunes); reason != "" {
			normalized.ReasonCodes = append(normalized.ReasonCodes, reason)
		}
	}
	return normalized
}

func (value normalizedToolPresentation) identity() string {
	return value.identityInLanguage(i18n.DetectOrLoadLanguage())
}

func (value normalizedToolPresentation) identityInLanguage(lang i18n.Language) string {
	parts := make([]string, 0, 3)
	if value.ToolName != "" {
		parts = append(parts, i18n.Format(lang, i18n.KeyPresentationToolID, value.ToolName))
	}
	if value.ToolUseID != "" {
		parts = append(parts, i18n.Format(lang, i18n.KeyPresentationToolUseID, value.ToolUseID))
	}
	if value.WorkUnitID != "" {
		parts = append(parts, i18n.Format(lang, i18n.KeyPresentationWorkUnit, value.WorkUnitID))
	}
	if len(parts) == 0 {
		return i18n.Text(lang, i18n.KeyPresentationToolUpdate)
	}
	return i18n.Text(lang, i18n.KeyPresentationToolUpdate) + ". " + strings.Join(parts, ". ")
}

// semanticFields defines the field order shared by visual and assistive
// renderers. Do not reorder this list without updating the presentation
// contract and its parity tests.
func (value normalizedToolPresentation) semanticFields() []toolPresentationField {
	return value.semanticFieldsInLanguage(i18n.DetectOrLoadLanguage())
}

func (value normalizedToolPresentation) semanticFieldsInLanguage(lang i18n.Language) []toolPresentationField {
	fields := make([]toolPresentationField, 0, 8)
	appendField := func(label, fieldValue string) {
		if fieldValue != "" {
			fields = append(fields, toolPresentationField{label: label, value: fieldValue})
		}
	}
	appendField(i18n.Text(lang, i18n.KeyPresentationActor), value.Actor)
	appendField(i18n.Text(lang, i18n.KeyPresentationAction), value.Action)
	appendField(i18n.Text(lang, i18n.KeyPresentationObject), value.Object)
	appendField(i18n.Text(lang, i18n.KeyPresentationState), i18n.RuntimeActivityStateLabel(lang, value.State))
	appendField(i18n.Text(lang, i18n.KeyPresentationResult), value.Result)
	appendField(i18n.Text(lang, i18n.KeyPresentationNextAction), value.NextAction)
	for index, detail := range value.DetailLines {
		appendField(i18n.Format(lang, i18n.KeyPresentationDetail, index+1), detail)
	}
	if value.omittedDetails > 0 {
		appendField(i18n.Text(lang, i18n.KeyPresentationDetailsOmitted), i18n.Format(lang, i18n.KeyPresentationAdditionalLines, value.omittedDetails))
	}
	appendField(i18n.Text(lang, i18n.KeyPresentationLevel), i18n.RuntimePresentationLevelLabel(lang, value.PresentationLevel))
	if len(value.ReasonCodes) > 0 {
		reasons := strings.Join(value.ReasonCodes, ", ")
		if value.omittedReasons > 0 {
			reasons += i18n.Format(lang, i18n.KeyPresentationMoreReasons, value.omittedReasons)
		}
		appendField(i18n.Text(lang, i18n.KeyPresentationReasonCodes), reasons)
	}
	return fields
}

// RenderToolPresentation emits one semantic transition to an append-only,
// screen-reader-safe stream. Presentation ticks are represented by D0 and are
// intentionally silent; no spinner or cursor-control sequence is emitted.
func (r *ScreenReaderRenderer) RenderToolPresentation(value ToolPresentation) {
	presentation := normalizeToolPresentation(value)
	if presentation.PresentationLevel == ToolPresentationLevelHidden {
		return
	}
	lang := screenReaderLanguage()
	r.write("%s.\n", presentation.identityInLanguage(lang))
	for _, field := range presentation.semanticFieldsInLanguage(lang) {
		r.write("%s: %s.\n", field.label, field.value)
	}
	if presentation.Redacted {
		r.write("%s.\n", i18n.Text(lang, i18n.KeyPresentationRedacted))
	}
	if presentation.HasMore {
		r.write("%s.\n", i18n.Text(lang, i18n.KeyPresentationDetailsAvailable))
	}
}

// RenderToolPresentation emits the same ordered semantic fields in classic
// terminal styling. It remains bounded even when the retained evidence is
// large; callers expose that evidence through the existing detail path.
func (r *TermRenderer) RenderToolPresentation(value ToolPresentation) {
	presentation := normalizeToolPresentation(value)
	if presentation.PresentationLevel == ToolPresentationLevelHidden {
		return
	}
	lang := i18n.DetectOrLoadLanguage()
	fmt.Fprintln(r.w, r.yellowStyle.Render(presentation.identityInLanguage(lang)))
	for _, field := range presentation.semanticFieldsInLanguage(lang) {
		line := fmt.Sprintf("  %s: %s", field.label, field.value)
		switch field.label {
		case i18n.Text(lang, i18n.KeyPresentationState):
			line = renderToolPresentationState(r, line, presentation.State)
		case i18n.Text(lang, i18n.KeyPresentationResult), i18n.Format(lang, i18n.KeyPresentationDetail, 1), i18n.Format(lang, i18n.KeyPresentationDetail, 2), i18n.Format(lang, i18n.KeyPresentationDetail, 3):
			line = r.dimStyle.Render(line)
		}
		fmt.Fprintln(r.w, line)
	}
	if presentation.Redacted {
		fmt.Fprintln(r.w, r.yellowStyle.Render("  "+i18n.Text(lang, i18n.KeyPresentationRedacted)))
	}
	if presentation.HasMore {
		fmt.Fprintln(r.w, r.dimStyle.Render("  "+i18n.Text(lang, i18n.KeyPresentationDetailsAvailable)))
	}
}

func renderToolPresentationState(r *TermRenderer, line, state string) string {
	switch state {
	case ToolPresentationStateSucceeded:
		return r.greenStyle.Render(line)
	case ToolPresentationStateFailed, ToolPresentationStateDenied, ToolPresentationStateTimedOut:
		return r.boldRedStyle.Render(line)
	case ToolPresentationStatePartial, ToolPresentationStateCancelled:
		return r.yellowStyle.Render(line)
	default:
		return line
	}
}

func boundedToolPresentationText(value string, limit int) string {
	var cleaned strings.Builder
	cleaned.Grow(len(value))
	spacePending := false
	for _, current := range strings.TrimSpace(value) {
		if unicode.IsSpace(current) || unicode.IsControl(current) {
			spacePending = cleaned.Len() > 0
			continue
		}
		if spacePending {
			cleaned.WriteByte(' ')
			spacePending = false
		}
		cleaned.WriteRune(current)
	}
	result := strings.TrimSpace(cleaned.String())
	if limit <= 0 || utf8.RuneCountInString(result) <= limit {
		return result
	}
	runes := []rune(result)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

var (
	_ SemanticToolRenderer = (*ScreenReaderRenderer)(nil)
	_ SemanticToolRenderer = (*TermRenderer)(nil)
)
