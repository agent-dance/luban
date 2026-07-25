package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestOutcomeCompletenessMatrix(t *testing.T) {
	complete := types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete}
	tests := []struct {
		name       string
		tool       string
		outcome    ObservationOutcome
		result     types.ToolResultBlock
		wantFull   bool
		wantSource types.ToolResultCompletenessKind
		wantView   types.ToolResultCompletenessKind
		warningKey i18n.Key
	}{
		{
			name: "Grep pagination", tool: "Grep", outcome: OutcomePartial,
			result: types.ToolResultBlock{Content: "a.go:1:match", Outcome: types.ToolOutcomePartial, Completeness: types.ToolResultCompleteness{
				Source: types.ToolResultCompletenessComplete, View: types.ToolResultCompletenessPagination,
				Pagination: &types.ToolResultPagination{Offset: 0, Limit: 1, NextOffset: 1, HasMore: true},
			}},
			wantSource: types.ToolResultCompletenessComplete, wantView: types.ToolResultCompletenessPagination,
			warningKey: i18n.KeyPresentationPaginationWarning,
		},
		{
			name: "Read oversize", tool: "Read", outcome: OutcomeFailed,
			result: types.ToolResultBlock{Content: "size limit", IsError: true, Outcome: types.ToolOutcomeFailed,
				Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessCaptureDropped}},
			wantSource: types.ToolResultCompletenessCaptureDropped, warningKey: i18n.KeyPresentationCaptureDroppedWarning,
		},
		{
			name: "Grep capture dropped", tool: "Grep", outcome: OutcomePartial,
			result: types.ToolResultBlock{Content: "captured prefix", Outcome: types.ToolOutcomePartial,
				Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessCaptureDropped}},
			wantSource: types.ToolResultCompletenessCaptureDropped, warningKey: i18n.KeyPresentationCaptureDroppedWarning,
		},
		{
			name: "Grep source truncated", tool: "Grep", outcome: OutcomeTimedOut,
			result: types.ToolResultBlock{Content: "partial timeout output", Outcome: types.ToolOutcomeTimedOut,
				Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessSourceTruncated}},
			wantSource: types.ToolResultCompletenessSourceTruncated, warningKey: i18n.KeyPresentationSourceTruncatedWarning,
		},
		{
			name: "Edit stale", tool: "Edit", outcome: OutcomeFailed,
			result:   types.ToolResultBlock{Content: "stale snapshot", IsError: true, Outcome: types.ToolOutcomeFailed, Completeness: complete},
			wantFull: true, wantSource: types.ToolResultCompletenessComplete,
		},
		{
			name: "Bash exit 1", tool: "Bash", outcome: OutcomeFailed,
			result: types.ToolResultBlock{Content: "test failed", IsError: true, Outcome: types.ToolOutcomeFailed, Completeness: complete,
				Metadata: map[string]string{"exit_code": "1"}},
			wantFull: true, wantSource: types.ToolResultCompletenessComplete,
		},
		{
			name: "permission denied", tool: "Write", outcome: OutcomeDenied,
			result:   types.ToolResultBlock{Content: "permission denied", IsError: true, Outcome: types.ToolOutcomeDenied, Completeness: complete},
			wantFull: true, wantSource: types.ToolResultCompletenessComplete,
		},
		{
			name: "producer display preview", tool: "Read", outcome: OutcomeSucceeded,
			result: types.ToolResultBlock{Content: "producer supplied preview", Outcome: types.ToolOutcomeSucceeded,
				Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete, View: types.ToolResultCompletenessDisplayPreview}},
			wantSource: types.ToolResultCompletenessComplete, wantView: types.ToolResultCompletenessDisplayPreview,
			warningKey: i18n.KeyPresentationDisplayPreviewWarning,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewObservationStore(NewMemoryDetailStore())
			ctx := ToolEventContext{SessionID: "session", TurnID: "turn", Outcome: test.outcome, Language: i18n.LangEN, LanguageSet: true}
			call := types.ToolUseBlock{ID: "tool", Name: test.tool, Input: map[string]any{"file_path": "/private/workspace/item"}}
			if err := store.ApplyToolCall(ctx, call); err != nil {
				t.Fatal(err)
			}
			test.result.ToolUseID = call.ID
			if err := store.ApplyToolResult(ctx, test.result); err != nil {
				t.Fatal(err)
			}
			observation, ok := store.Get(toolObservationID(ctx.SessionID, call.ID))
			if !ok {
				t.Fatal("observation missing")
			}
			_, hasFull := observation.FullEvidenceRef()
			if observation.Outcome != test.outcome || hasFull != test.wantFull || observation.Presentation.FullEvidenceAvailable != test.wantFull ||
				observation.Presentation.Completeness.Source != test.wantSource || observation.Presentation.Completeness.View != test.wantView {
				t.Fatalf("observation = %+v", observation)
			}
			details := strings.Join(observation.Presentation.DetailLines, "\n")
			if test.warningKey != "" && !strings.Contains(details, i18n.Text(i18n.LangEN, test.warningKey)) {
				t.Fatalf("details %q missing semantic warning %s", details, test.warningKey)
			}
			if !test.wantFull && strings.Contains(details, i18n.Text(i18n.LangEN, i18n.KeyPresentationDisplayPreviewEvidence)) {
				t.Fatalf("incomplete result claimed full evidence: %q", details)
			}
		})
	}
}

func TestDisplayPreviewClaimsFullEvidenceOnlyAfterRetention(t *testing.T) {
	content := strings.Repeat("complete retained output ", 100)
	result := types.ToolResultBlock{ToolUseID: "tool", Content: content, Outcome: types.ToolOutcomeSucceeded,
		Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete}}
	direct := FormatToolPresentationInLanguage(i18n.LangEN, "Read", map[string]any{"file_path": "/private/workspace/main.go"}, OutcomeSucceeded, &result)
	if direct.FullEvidenceAvailable || direct.Completeness.View != types.ToolResultCompletenessDisplayPreview {
		t.Fatalf("unretained preview = %+v", direct)
	}
	if strings.Contains(strings.Join(direct.DetailLines, "\n"), i18n.Text(i18n.LangEN, i18n.KeyPresentationDisplayPreviewEvidence)) {
		t.Fatal("unretained preview claimed complete evidence")
	}

	store := NewObservationStore(NewMemoryDetailStore())
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", Outcome: OutcomeSucceeded, Language: i18n.LangEN, LanguageSet: true}
	if err := store.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool", Name: "Read", Input: map[string]any{"file_path": "/private/workspace/main.go"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyToolResult(ctx, result); err != nil {
		t.Fatal(err)
	}
	observation, _ := store.Get(toolObservationID(ctx.SessionID, "tool"))
	if _, ok := observation.FullEvidenceRef(); !ok || !observation.Presentation.FullEvidenceAvailable {
		t.Fatalf("retained preview lacks full evidence ref: %+v", observation)
	}
	if !strings.Contains(strings.Join(observation.Presentation.DetailLines, "\n"), i18n.Text(i18n.LangEN, i18n.KeyPresentationDisplayPreviewEvidence)) {
		t.Fatalf("retained preview detail = %q", observation.Presentation.DetailLines)
	}
}

func TestDefaultSummaryDoesNotExposeAbsolutePath(t *testing.T) {
	formatted := FormatToolPresentationInLanguage(i18n.LangEN, "Read", map[string]any{"file_path": "/private/workspace/main.go"}, OutcomeRunning, nil)
	if strings.Contains(formatted.Summary, "/private/workspace/main.go") {
		t.Fatalf("default projection exposed absolute path: %+v", formatted)
	}
}
