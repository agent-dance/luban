package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// ─── Schema validation ────────────────────────────────────────────────────────

func TestValidateAskUserQuestions_RejectsEmpty(t *testing.T) {
	if err := ValidateAskUserQuestions(nil); err == nil {
		t.Fatal("expected error for empty questions")
	}
}

func TestValidateAskUserQuestions_RejectsTooMany(t *testing.T) {
	q := QuestionSpec{
		Question: "Q?", Header: "H",
		Options: []OptionSpec{
			{Label: "A", Description: "a"}, {Label: "B", Description: "b"},
		},
	}
	if err := ValidateAskUserQuestions([]QuestionSpec{q, q, q, q, q}); err == nil {
		t.Fatal("expected error for >4 questions")
	}
}

func TestValidateAskUserQuestions_RejectsMissingQuestionMark(t *testing.T) {
	q := QuestionSpec{
		Question: "No question mark", Header: "H",
		Options: []OptionSpec{
			{Label: "A", Description: "a"}, {Label: "B", Description: "b"},
		},
	}
	err := ValidateAskUserQuestions([]QuestionSpec{q})
	if err == nil || !strings.Contains(err.Error(), "?") {
		t.Fatalf("expected '?' error, got %v", err)
	}
}

func TestValidateAskUserQuestions_RejectsLongHeader(t *testing.T) {
	q := QuestionSpec{
		Question: "Pick?", Header: strings.Repeat("X", 13),
		Options: []OptionSpec{
			{Label: "A", Description: "a"}, {Label: "B", Description: "b"},
		},
	}
	err := ValidateAskUserQuestions([]QuestionSpec{q})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected length error, got %v", err)
	}
}

func TestValidateAskUserQuestions_RejectsTooFewOptions(t *testing.T) {
	q := QuestionSpec{
		Question: "Pick?", Header: "H",
		Options: []OptionSpec{{Label: "Only", Description: "alone"}},
	}
	if err := ValidateAskUserQuestions([]QuestionSpec{q}); err == nil {
		t.Fatal("expected error for <2 options")
	}
}

func TestValidateAskUserQuestions_RejectsTooManyOptions(t *testing.T) {
	opts := []OptionSpec{
		{Label: "A", Description: "a"},
		{Label: "B", Description: "b"},
		{Label: "C", Description: "c"},
		{Label: "D", Description: "d"},
		{Label: "E", Description: "e"},
	}
	q := QuestionSpec{Question: "Pick?", Header: "H", Options: opts}
	if err := ValidateAskUserQuestions([]QuestionSpec{q}); err == nil {
		t.Fatal("expected error for >4 options")
	}
}

func TestValidateAskUserQuestions_RejectsOtherLabel(t *testing.T) {
	q := QuestionSpec{
		Question: "Pick?", Header: "H",
		Options: []OptionSpec{
			{Label: "A", Description: "a"},
			{Label: "Other", Description: "free"},
		},
	}
	err := ValidateAskUserQuestions([]QuestionSpec{q})
	if err == nil || !strings.Contains(err.Error(), "Other") {
		t.Fatalf("expected 'Other' reserved error, got %v", err)
	}
}

func TestValidateAskUserQuestions_EnforcesConciseOptionLabels(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		wantErr bool
	}{
		{name: "five words accepted", label: "One two three four five"},
		{name: "six words rejected", label: "One two three four five six", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := QuestionSpec{
				Question: "Pick?", Header: "H",
				Options: []OptionSpec{
					{Label: tt.label, Description: "candidate"},
					{Label: "Fallback", Description: "fallback"},
				},
			}
			err := ValidateAskUserQuestions([]QuestionSpec{q})
			if tt.wantErr && (err == nil || !strings.Contains(err.Error(), "1-5 words")) {
				t.Fatalf("expected 1-5 word label error, got %v", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected label to be accepted, got %v", err)
			}
		})
	}
}

func TestValidateAskUserQuestions_RejectsDuplicateLabels(t *testing.T) {
	q := QuestionSpec{
		Question: "Pick?", Header: "H",
		Options: []OptionSpec{
			{Label: "A", Description: "a"},
			{Label: "a", Description: "alt"}, // case-insensitive duplicate
		},
	}
	if err := ValidateAskUserQuestions([]QuestionSpec{q}); err == nil {
		t.Fatal("expected duplicate label error")
	}
}

func TestValidateAskUserQuestions_RejectsDuplicateQuestions(t *testing.T) {
	opts := []OptionSpec{
		{Label: "A", Description: "a"}, {Label: "B", Description: "b"},
	}
	q := QuestionSpec{Question: "Same?", Header: "H", Options: opts}
	if err := ValidateAskUserQuestions([]QuestionSpec{q, q}); err == nil {
		t.Fatal("expected duplicate question error")
	}
}

// ─── Preview rules ───────────────────────────────────────────────────────────

func TestValidateAskUserQuestions_RejectsPreviewOnMultiSelect(t *testing.T) {
	q := QuestionSpec{
		Question: "Pick?", Header: "H", MultiSelect: true,
		Options: []OptionSpec{
			{Label: "A", Description: "a", Preview: "preview A"},
			{Label: "B", Description: "b"},
		},
	}
	err := ValidateAskUserQuestions([]QuestionSpec{q})
	if err == nil || !strings.Contains(err.Error(), "preview") {
		t.Fatalf("expected preview-on-multiSelect error, got %v", err)
	}
}

func TestValidateAskUserQuestions_RejectsOversizePreview(t *testing.T) {
	q := QuestionSpec{
		Question: "Pick?", Header: "H",
		Options: []OptionSpec{
			{Label: "A", Description: "a", Preview: strings.Repeat("X", 4001)},
			{Label: "B", Description: "b"},
		},
	}
	err := ValidateAskUserQuestions([]QuestionSpec{q})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversize preview error, got %v", err)
	}
}

func TestResolvePreviewMode_StackedByDefault(t *testing.T) {
	q := QuestionSpec{
		Options: []OptionSpec{{Label: "A"}, {Label: "B"}},
	}
	if got := ResolvePreviewMode(q); got != PreviewModeNone {
		t.Errorf("expected PreviewModeNone, got %v", got)
	}
	if got := ResolvePreviewMode(q).String(); got != "stacked" {
		t.Errorf("expected 'stacked', got %q", got)
	}
}

func TestResolvePreviewMode_SideBySideWithPreview(t *testing.T) {
	q := QuestionSpec{
		Options: []OptionSpec{
			{Label: "A", Preview: "x"},
			{Label: "B"},
		},
	}
	if got := ResolvePreviewMode(q); got != PreviewModeSideBySide {
		t.Errorf("expected side-by-side, got %v", got)
	}
	if got := ResolvePreviewMode(q).String(); got != "side-by-side" {
		t.Errorf("expected 'side-by-side', got %q", got)
	}
}

func TestResolvePreviewMode_NeverForMultiSelect(t *testing.T) {
	// Note: schema would reject this, but ResolvePreviewMode is defensive.
	q := QuestionSpec{
		MultiSelect: true,
		Options: []OptionSpec{
			{Label: "A", Preview: "x"},
			{Label: "B"},
		},
	}
	if got := ResolvePreviewMode(q); got != PreviewModeNone {
		t.Errorf("expected PreviewModeNone for multiSelect, got %v", got)
	}
}

// ─── Other-sentinel parsing ──────────────────────────────────────────────────

func TestParseOtherSentinel_Basic(t *testing.T) {
	other, ok := ParseOtherSentinel("Other:my custom answer")
	if !ok || other != "my custom answer" {
		t.Errorf("expected ok=true other=\"my custom answer\", got ok=%v other=%q", ok, other)
	}
}

func TestParseOtherSentinel_CaseInsensitive(t *testing.T) {
	other, ok := ParseOtherSentinel("OTHER:hello")
	if !ok || other != "hello" {
		t.Errorf("expected case-insensitive match, got ok=%v other=%q", ok, other)
	}
}

func TestParseOtherSentinel_RejectsNonSentinel(t *testing.T) {
	if _, ok := ParseOtherSentinel("Yes"); ok {
		t.Error("expected ok=false for non-sentinel input")
	}
}

func TestParseMultiSelectReply_Mixed(t *testing.T) {
	opts := []OptionSpec{
		{Label: "Apple", Description: "a"},
		{Label: "Banana", Description: "b"},
	}
	sel, unknown := ParseMultiSelectReply("Apple, Other:my pick, mango", opts)
	if len(sel.Selection) != 1 || sel.Selection[0] != "Apple" {
		t.Errorf("expected Selection=[Apple], got %v", sel.Selection)
	}
	if sel.OtherText != "my pick" {
		t.Errorf("expected OtherText='my pick', got %q", sel.OtherText)
	}
	if len(unknown) != 1 || unknown[0] != "mango" {
		t.Errorf("expected unknown=[mango], got %v", unknown)
	}
}

func TestParseMultiSelectReply_DeDup(t *testing.T) {
	opts := []OptionSpec{
		{Label: "Apple"},
		{Label: "Banana"},
	}
	sel, _ := ParseMultiSelectReply("Apple, apple, Banana", opts)
	if len(sel.Selection) != 2 {
		t.Errorf("expected 2 unique selections, got %v", sel.Selection)
	}
}

// ─── Approval-style detection ────────────────────────────────────────────────

func TestIsApprovalStyleQuestion_DetectsPhrase(t *testing.T) {
	cases := []string{
		"Should I proceed?",
		"Shall I continue?",
		"Do you approve this plan?",
		"Ready to proceed?",
		"Is it ok to proceed?",
	}
	for _, q := range cases {
		if !IsApprovalStyleQuestion(QuestionSpec{Question: q}) {
			t.Errorf("expected %q to be flagged", q)
		}
	}
}

func TestIsApprovalStyleQuestion_AllowsContent(t *testing.T) {
	cases := []string{
		"What colour do you prefer?",
		"Which framework should we use?",
		"How do you want this organised?",
	}
	for _, q := range cases {
		if IsApprovalStyleQuestion(QuestionSpec{Question: q}) {
			t.Errorf("did not expect %q to be flagged", q)
		}
	}
}

// ─── Plan-mode integration ───────────────────────────────────────────────────

func TestAskUserQuestion_PlanModeBlocksApprovalQuestion(t *testing.T) {
	planState := NewPlanState(t.TempDir())
	planState.Enter("")
	defer planState.Exit()

	tool := &AskUserQuestionTool{
		Reader:    strings.NewReader(""),
		Writer:    &bytes.Buffer{},
		PlanState: planState,
	}
	input := map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Should I proceed?",
				"header":   "Approve",
				"options": []any{
					map[string]any{"label": "Yes", "description": "y"},
					map[string]any{"label": "No", "description": "n"},
				},
				"multiSelect": false,
			},
		},
	}
	result, err := executeAskUserThroughPermission(context.Background(), tool, input)
	if err != nil {
		t.Fatalf("Execute returned err: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected IsError=true for approval-style in plan mode")
	}
	if !strings.Contains(result.Content, "ExitPlanMode") {
		t.Errorf("expected ExitPlanMode hint, got %q", result.Content)
	}
}

func TestAskUserQuestion_PlanModeAllowsContentQuestion(t *testing.T) {
	planState := NewPlanState(t.TempDir())
	planState.Enter("")
	defer planState.Exit()

	tool := &AskUserQuestionTool{
		Reader:    strings.NewReader("1\n"),
		Writer:    &bytes.Buffer{},
		PlanState: planState,
	}
	input := map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Which colour?",
				"header":   "Colour",
				"options": []any{
					map[string]any{"label": "Red", "description": "r"},
					map[string]any{"label": "Blue", "description": "b"},
				},
				"multiSelect": false,
			},
		},
	}
	result, err := executeAskUserThroughPermission(context.Background(), tool, input)
	if err != nil {
		t.Fatalf("Execute returned err: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got IsError=true: %s", result.Content)
	}
}

// ─── Structured result + metadata ────────────────────────────────────────────

func TestAskUserQuestion_StructuredResultEmbeddedInOutput(t *testing.T) {
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader("1\n"),
		Writer: &bytes.Buffer{},
		Metadata: map[string]any{
			"source": "test",
		},
	}
	result, err := executeAskUserThroughPermission(context.Background(), tool, makeAskInput(false))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if result.IsError {
		t.Fatalf("got IsError: %s", result.Content)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(result.Content), &raw); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if _, ok := raw["_structured"]; !ok {
		t.Error("expected _structured key in output")
	}
}

func TestAskUserQuestion_InteractiveMetadataRoundTripsInTypedResult(t *testing.T) {
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader("1\n"),
		Writer: &bytes.Buffer{},
	}
	input := makeAskInput(false)
	input["metadata"] = map[string]any{"source": "interview"}

	result, err := executeAskUserThroughPermission(context.Background(), tool, input)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	output, ok := result.Data.(AskUserQuestionOutput)
	if !ok {
		t.Fatalf("result.Data = %T, want AskUserQuestionOutput", result.Data)
	}
	if got := output.Metadata["source"]; got != "interview" {
		t.Fatalf("metadata source = %v, want interview; output=%#v", got, output)
	}
}

func TestAskUserQuestion_ContextCancellationReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader(""),
		Writer: &bytes.Buffer{},
	}

	_, err := executeAskUserThroughPermission(ctx, tool, makeAskInput(false))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context.Canceled", err)
	}
}

// ─── Other sentinel through the tool interface ──────────────────────────────

func TestAskUserQuestion_OtherSentinelViaPrompt(t *testing.T) {
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader("Other:my custom\n"),
		Writer: &bytes.Buffer{},
	}
	result, err := executeAskUserThroughPermission(context.Background(), tool, makeAskInput(false))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if result.IsError {
		t.Fatalf("IsError: %s", result.Content)
	}
	if !strings.Contains(result.Content, "my custom") {
		t.Errorf("expected custom answer, got %q", result.Content)
	}
}
