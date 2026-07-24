package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

// AskUserQuestion 100% alignment red-tests.
//
// Pin the ask-user contract against the TS reference
// (src/tools/AskUserQuestionTool/AskUserQuestionTool.ts and schema.ts). The
// current Go implementation:
//   - returns a JSON object with the answers map (no envelope)
//   - validates duplicate question text case-sensitively
//   - has no per-question `_meta.source` selector
//   - never threads a SendMessage envelope (request_id) through the result
//
// The TS-aligned target replaces the JSON-blob output with a text payload
// "User has answered your questions: \"Q\"=\"A\""; treats duplicate questions
// case-insensitively; accepts a `_meta` block per question; and routes through
// the same envelope used by SendMessage so the host can correlate replies.
// See alignment_audit.md P2-3 / P2-4 and tasks/askuser.json.

// helper — drive AskUserQuestion with stdin pre-filled and return the parsed
// JSON output (if any) plus the raw text.
func runAskUserAlignment(t *testing.T, input map[string]any, stdin string) (
	contentJSON map[string]any,
	contentText string,
	isError bool,
) {
	t.Helper()
	var out bytes.Buffer
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader(stdin),
		Writer: &out,
	}
	res, err := executeAskUserThroughPermission(context.Background(), tool, input)
	if err != nil {
		t.Fatalf("AskUserQuestion.Execute infra error: %v", err)
	}
	contentText = res.Content
	isError = res.IsError

	var decoded map[string]any
	if err := json.Unmarshal([]byte(res.Content), &decoded); err == nil {
		contentJSON = decoded
	}
	return
}

func askUserAlignmentText(decoded map[string]any, raw string) string {
	if decoded == nil {
		return raw
	}
	if msg, ok := decoded["message"].(string); ok && msg != "" {
		return msg
	}
	if text, ok := decoded["text"].(string); ok && text != "" {
		return text
	}
	return raw
}

// makeSingleQuestionInput builds a one-question single-select input shaped
// like the TS schema for reuse across red-tests.
func makeSingleQuestionInput(text, header string) map[string]any {
	return map[string]any{
		"questions": []any{
			map[string]any{
				"question": text,
				"header":   header,
				"options": []any{
					map[string]any{"label": "Yes", "description": "Agree"},
					map[string]any{"label": "No", "description": "Disagree"},
				},
				"multiSelect": false,
			},
		},
	}
}

// TSref: src/tools/AskUserQuestionTool/AskUserQuestionTool.ts:204 — the
// output text follows the canonical "User has answered your questions:
// \"Q\"=\"A\"" form, NOT a JSON map of answers.
func TestAskUserAlignment_OutputUsesUserHasAnsweredText(t *testing.T) {
	input := makeSingleQuestionInput("Do you like Go?", "Go Opinion")
	decoded, raw, isError := runAskUserAlignment(t, input, "1\n")
	if isError {
		t.Fatalf("AskUserQuestion returned IsError unexpectedly: %s", raw)
	}
	text := askUserAlignmentText(decoded, raw)
	if !strings.Contains(text, "User has answered your questions") {
		t.Fatalf("expected text output starting with \"User has answered your questions\", got: %q", text)
	}
}

// TSref: src/tools/AskUserQuestionTool/AskUserQuestionTool.ts:208 — answers
// are emitted in the "<question>"="<answer>" inline form so the model can
// parse the reply without a JSON decoder.
func TestAskUserAlignment_OutputContainsQuotedQuestionEqualsAnswer(t *testing.T) {
	input := makeSingleQuestionInput("Do you like Go?", "Go Opinion")
	decoded, raw, isError := runAskUserAlignment(t, input, "1\n")
	if isError {
		t.Fatalf("AskUserQuestion returned IsError unexpectedly: %s", raw)
	}
	text := askUserAlignmentText(decoded, raw)
	wantPair := `"Do you like Go?"="Yes"`
	if !strings.Contains(text, wantPair) {
		t.Fatalf("expected output to contain %q, got: %q", wantPair, text)
	}
}

// TSref: src/tools/AskUserQuestionTool/AskUserQuestionTool.ts:79 — the result
// rides on the same SendMessage envelope, so a top-level request_id is
// present for ApprovalSink correlation.
func TestAskUserAlignment_OutputContainsRequestID(t *testing.T) {
	input := makeSingleQuestionInput("Do you like Go?", "Go Opinion")
	decoded, raw, isError := runAskUserAlignment(t, input, "1\n")
	if isError {
		t.Fatalf("AskUserQuestion returned IsError unexpectedly: %s", raw)
	}
	if decoded != nil {
		rid, _ := decoded["request_id"].(string)
		if rid == "" {
			rid, _ = decoded["RequestID"].(string)
		}
		if strings.TrimSpace(rid) == "" {
			t.Fatalf("expected JSON envelope with non-empty request_id, got %#v", decoded)
		}
		return
	}
	// Fall through: text output should embed the envelope id, e.g. "request_id=…".
	if !strings.Contains(raw, "request_id") {
		t.Fatalf("expected request_id token somewhere in AskUserQuestion result, got: %q", raw)
	}
}

// TSref: src/tools/AskUserQuestionTool/schema.ts:38 — duplicate detection is
// case-insensitive. The current Go validator uses TrimSpace only and accepts
// "Do you like Go?" alongside "DO YOU LIKE GO?".
func TestAskUserAlignment_DuplicateQuestionsAreCaseInsensitive(t *testing.T) {
	input := map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Do you like Go?",
				"header":   "Go A",
				"options": []any{
					map[string]any{"label": "Yes", "description": "Agree"},
					map[string]any{"label": "No", "description": "Disagree"},
				},
				"multiSelect": false,
			},
			map[string]any{
				"question": "DO YOU LIKE GO?",
				"header":   "Go B",
				"options": []any{
					map[string]any{"label": "Yes", "description": "Agree"},
					map[string]any{"label": "No", "description": "Disagree"},
				},
				"multiSelect": false,
			},
		},
	}
	_, raw, isError := runAskUserAlignment(t, input, "1\n1\n")
	if !isError {
		t.Fatalf("expected duplicate (case-insensitive) questions to be rejected, got OK: %s", raw)
	}
	if !strings.Contains(strings.ToLower(raw), "duplicate") {
		t.Fatalf("expected error message to mention duplicate, got: %q", raw)
	}
}

// TSref: src/tools/AskUserQuestionTool/schema.ts:64 — questions accept an
// optional `_meta` object (e.g. `{ source: "tui" }`) that flows through to
// analytics. The current Go schema neither declares the field nor reflects it
// on the output.
func TestAskUserAlignment_MetaSourceIsEchoedOnAnswer(t *testing.T) {
	input := map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Do you like Go?",
				"header":   "Go Opinion",
				"options": []any{
					map[string]any{"label": "Yes", "description": "Agree"},
					map[string]any{"label": "No", "description": "Disagree"},
				},
				"multiSelect": false,
				"_meta": map[string]any{
					"source": "tui",
				},
			},
		},
	}
	decoded, raw, isError := runAskUserAlignment(t, input, "1\n")
	if isError {
		t.Fatalf("AskUserQuestion rejected the _meta payload: %s", raw)
	}
	if decoded == nil {
		// Text output: the source tag should at minimum appear in the
		// rendered transcript so analytics has it.
		if !strings.Contains(raw, "tui") {
			t.Fatalf("expected _meta.source=\"tui\" to appear in result, got: %q", raw)
		}
		return
	}
	src, _ := decoded["source"].(string)
	if src == "" {
		// Try the nested _meta carrier the TS reference uses.
		if metaRaw, ok := decoded["_meta"].(map[string]any); ok {
			src, _ = metaRaw["source"].(string)
		}
	}
	if src != "tui" {
		t.Fatalf("expected _meta.source=\"tui\" on AskUserQuestion result, got %#v", decoded)
	}
}

// TSref: src/tools/AskUserQuestionTool/AskUserQuestionTool.ts:215 — when the
// user picks the "Other" sentinel, the structured answer carries the custom
// text directly in the inline form (no JSON unwrapping required).
func TestAskUserAlignment_OutputUsesQuotesAroundOtherText(t *testing.T) {
	input := makeSingleQuestionInput("Do you like Go?", "Go Opinion")
	decoded, raw, isError := runAskUserAlignment(t, input, "o\nMaybe later\n")
	if isError {
		t.Fatalf("AskUserQuestion returned IsError unexpectedly: %s", raw)
	}
	text := askUserAlignmentText(decoded, raw)
	wantPair := `"Do you like Go?"="Maybe later"`
	if !strings.Contains(text, wantPair) {
		t.Fatalf("expected output to contain %q for Other sentinel, got: %q", wantPair, text)
	}
}

// TSref: src/tools/AskUserQuestionTool/AskUserQuestionTool.ts:222 — multi
// select answers are joined with ", " between picks in the inline form.
func TestAskUserAlignment_MultiSelectInlineFormatJoinsWithComma(t *testing.T) {
	input := map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Pick languages?",
				"header":   "Langs",
				"options": []any{
					map[string]any{"label": "Go", "description": "Go"},
					map[string]any{"label": "Rust", "description": "Rust"},
					map[string]any{"label": "TS", "description": "TS"},
				},
				"multiSelect": true,
			},
		},
	}
	decoded, raw, isError := runAskUserAlignment(t, input, "1,2\n")
	if isError {
		t.Fatalf("AskUserQuestion returned IsError unexpectedly: %s", raw)
	}
	text := askUserAlignmentText(decoded, raw)
	wantPair := `"Pick languages?"="Go, Rust"`
	if !strings.Contains(text, wantPair) {
		t.Fatalf("expected joined multi-select answer %q in output, got: %q", wantPair, text)
	}
}

// TSref: src/tools/AskUserQuestionTool/AskUserQuestionTool.ts:79 — the result
// is wrapped in a SendMessage envelope, so it carries a `kind` discriminator
// matching `ask_user_question_response`.
func TestAskUserAlignment_OutputCarriesEnvelopeKind(t *testing.T) {
	input := makeSingleQuestionInput("Do you like Go?", "Go Opinion")
	decoded, raw, isError := runAskUserAlignment(t, input, "1\n")
	if isError {
		t.Fatalf("AskUserQuestion returned IsError unexpectedly: %s", raw)
	}
	if decoded != nil {
		kind, _ := decoded["kind"].(string)
		if kind == "" {
			t.Fatalf("expected envelope kind on AskUserQuestion result, got %#v", decoded)
		}
		return
	}
	if !strings.Contains(raw, "ask_user_question") {
		t.Fatalf("expected ask_user_question kind tag in result, got: %q", raw)
	}
}

// TSref: src/tools/AskUserQuestionTool/AskUserQuestionTool.tsx:61-83 and
// 190-234 — the tool declares strict input/output schemas and maps typed
// call() data into canonical tool-result text.
func TestAskUserAlignment_DeclaresStrictTypedContractAndMapper(t *testing.T) {
	tool := &AskUserQuestionTool{}
	def := types.ToDefinition(tool)
	if !def.Strict {
		t.Fatalf("expected strict AskUserQuestion contract")
	}
	if !def.InputSchema.RejectsUnknownFields() {
		t.Fatalf("expected strict root input schema: %#v", def.InputSchema)
	}
	for _, field := range []string{"questions", "answers", "annotations", "metadata"} {
		if _, ok := def.InputSchema.Properties[field]; !ok {
			t.Fatalf("input schema missing TS field %q: %#v", field, def.InputSchema.Properties)
		}
	}
	if def.OutputSchema == nil {
		t.Fatalf("expected typed output schema")
	}
	for _, field := range []string{"questions", "answers", "annotations"} {
		if _, ok := def.OutputSchema.Properties[field]; !ok {
			t.Fatalf("output schema missing TS field %q: %#v", field, def.OutputSchema.Properties)
		}
	}
	if !def.Metadata.ReadOnly || !def.Metadata.ConcurrencySafe || def.Metadata.MaxResultSizeChars != 100_000 {
		t.Fatalf("unexpected AskUserQuestion metadata: %#v", def.Metadata)
	}

	output := AskUserQuestionOutput{
		Questions: []QuestionSpec{{
			Question: "Pick one?",
			Header:   "Pick",
			Options: []OptionSpec{
				{Label: "A", Description: "a"},
				{Label: "B", Description: "b"},
			},
			MultiSelect: false,
		}},
		Answers: map[string]string{"Pick one?": "A"},
	}
	block := tool.MapToolResultToToolResultBlock(output, "toolu_123")
	if block.ToolUseID != "toolu_123" {
		t.Fatalf("tool use id not preserved: %#v", block)
	}
	if !strings.Contains(block.Content, `"Pick one?"="A"`) {
		t.Fatalf("mapper did not render canonical answer text: %q", block.Content)
	}
	if !strings.Contains(block.Content, "You can now continue") {
		t.Fatalf("mapper missing TS continuation sentence: %q", block.Content)
	}
}

// TSref: the dedicated permission component collects answers and calls
// onAllow(updatedInput); no generic approval prompt is shown first.
func TestAskUserAlignment_CheckPermissionsCollectsAnswersAndAllowsUpdatedInput(t *testing.T) {
	tool := &AskUserQuestionTool{}
	tool.SetInteractionRequester(AskUserInteractionRequesterFunc(func(_ context.Context, request AskUserInteractionRequest) (AskUserInteractionResponse, error) {
		return AskUserInteractionResponse{
			RequestID: request.RequestID, Outcome: AskUserInteractionCompleted,
			Answers: map[string]AnswerSelection{"Do you like Go?": {Selection: []string{"Yes"}}},
		}, nil
	}))
	input := makeSingleQuestionInput("Do you like Go?", "Go Opinion")
	decision, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{SessionID: "session", ToolUseID: "tool"})
	if err != nil {
		t.Fatalf("CheckPermissions error: %v", err)
	}
	if decision.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("behavior = %q, want allow", decision.Behavior)
	}
	if decision.UpdatedInput == nil || decision.UpdatedInput["questions"] == nil {
		t.Fatalf("updatedInput did not preserve questions: %#v", decision.UpdatedInput)
	}
	answers, _ := decision.UpdatedInput["answers"].(map[string]any)
	if answers["Do you like Go?"] != "Yes" {
		t.Fatalf("updatedInput answers = %#v", decision.UpdatedInput["answers"])
	}
}

// TSref: call({ questions, answers, annotations }) returns the already
// collected typed answers. If updatedInput has answers, Execute must not block
// waiting for stdin.
func TestAskUserAlignment_ExecuteUsesUpdatedInputAnswers(t *testing.T) {
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader(""),
		Writer: &bytes.Buffer{},
	}
	input := makeSingleQuestionInput("Do you like Go?", "Go Opinion")
	input["answers"] = map[string]any{"Do you like Go?": "Yes"}
	input["annotations"] = map[string]any{
		"Do you like Go?": map[string]any{"notes": "ship it"},
	}
	input["metadata"] = map[string]any{"source": "permission-ui"}

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error: %s", result.Content)
	}
	output, ok := result.Data.(AskUserQuestionOutput)
	if !ok {
		t.Fatalf("result.Data = %T, want AskUserQuestionOutput", result.Data)
	}
	if output.Answers["Do you like Go?"] != "Yes" {
		t.Fatalf("typed answer not preserved: %#v", output.Answers)
	}
	if output.Annotations["Do you like Go?"].Notes != "ship it" {
		t.Fatalf("annotation not preserved: %#v", output.Annotations)
	}
	text := RenderAskUserQuestionOutputText(output)
	if !strings.Contains(text, `"Do you like Go?"="Yes"`) || !strings.Contains(text, "user notes: ship it") {
		t.Fatalf("updatedInput output text mismatch: %q", text)
	}
}
