package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/types"
)

// AskUserQuestionTool presents multiple-choice questions to the user and
// collects their answers. Interactive runtimes inject exactly one structured
// surface owner. Reader/Writer remain an explicit paired stream adapter for
// embedders and tests, but are consumed only by CheckPermissions; Execute is
// non-interactive. Nil never falls back to process stdin/stdout.
//
// Optional PlanState is consulted to flag approval-style questions when plan
// mode is active (TS rule: use ExitPlanMode for approval, AskUserQuestion for
// genuine multi-choice).
//
// Metadata is attached to the structured result under the "metadata" key —
// callers can use it for analytics or session correlation.
type AskUserQuestionTool struct {
	Reader    io.Reader
	Writer    io.Writer
	PlanState *PlanState
	Metadata  map[string]any

	interactionMu sync.RWMutex
	interaction   AskUserInteractionRequester
	streamMu      sync.Mutex
}

// SetInteractionRequester atomically binds the runtime's sole interactive
// input owner. Passing nil unbinds it during surface teardown.
func (t *AskUserQuestionTool) SetInteractionRequester(requester AskUserInteractionRequester) {
	if t == nil {
		return
	}
	t.interactionMu.Lock()
	t.interaction = requester
	t.interactionMu.Unlock()
}

func (t *AskUserQuestionTool) interactionRequester() AskUserInteractionRequester {
	if t == nil {
		return nil
	}
	t.interactionMu.RLock()
	defer t.interactionMu.RUnlock()
	return t.interaction
}

func (t *AskUserQuestionTool) Name() string { return "AskUserQuestion" }

func (t *AskUserQuestionTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	return runtime.Interactive && !runtime.ChannelsActive && !AskUserChannelsActive()
}

func (t *AskUserQuestionTool) Description() string {
	base := "Ask the user a question with multiple-choice options"
	// askuser-prompt-cache-stable-prefix: when the host has opted into a
	// preview format, append the format-specific guidance so the model
	// emits previews in the expected shape (HTML fragment, markdown, or
	// plain). Mirrors AskUserQuestionTool.tsx:117-124.
	if extra := AskUserPreviewFormatPromptSuffix(); extra != "" {
		return base + "\n\n" + extra
	}
	return base
}

func (t *AskUserQuestionTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"questions":   askUserQuestionsSchema(),
		"answers":     map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
		"annotations": askUserAnnotationsSchema(),
		"metadata": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{"type": "string"},
			},
		},
	}, "questions")
}

func (t *AskUserQuestionTool) ToolContract() types.ToolContract {
	return types.ToolContract{
		OutputSchema: &types.JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"questions":   askUserQuestionsSchema(),
				"answers":     map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				"annotations": askUserAnnotationsSchema(),
			},
			Required: []string{"questions", "answers"},
		},
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 100_000,
	}
}

func (t *AskUserQuestionTool) CheckPermissions(ctx context.Context, input map[string]any, permission types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	if AskUserChannelsActive() {
		return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserRemoteUnavailable), nil
	}
	in, toolErr := parseInputOrError[AskUserQuestionInput](input)
	if toolErr != nil {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolErr.Content}, nil
	}
	if err := ValidateAskUserQuestions(in.Questions); err != nil {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolRuntimeFormat(i18n.KeyToolRuntimeErrorPrefix, err)}, nil
	}
	if question, blocked := t.planApprovalQuestion(in.Questions); blocked {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolRuntimeFormat(i18n.KeyToolRuntimeAskUserPlanApproval, question)}, nil
	}
	if err := ctx.Err(); err != nil {
		return types.ToolPermissionResult{}, err
	}
	requester := t.interactionRequester()
	if requester == nil {
		if t.Reader != nil && t.Writer != nil {
			answers, annotations, err := t.collectStreamAnswers(ctx, in.Questions)
			if err != nil {
				if isAskUserContextError(err) {
					return types.ToolPermissionResult{}, err
				}
				return types.ToolPermissionResult{
					Behavior: types.PermissionBehaviorDeny,
					Message:  toolRuntimeFormat(i18n.KeyToolRuntimeAskUserReadAnswerFailed, err),
					Required: true,
				}, nil
			}
			updated, ok := updatedAskUserInput(input, in.Questions, answers, annotations)
			if !ok {
				return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserNoValidSelection), nil
			}
			return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: updated}, nil
		}
		return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserInteractiveUnavailable), nil
	}

	execution, _ := loop.ToolExecutionContextFromContext(ctx)
	requestID := "ask-user:" + permission.SessionID + ":" + permission.ToolUseID
	if permission.SessionID == "" || permission.ToolUseID == "" {
		requestID = NewRequestID("askuser")
	}
	response, err := requester.AskUserQuestions(ctx, AskUserInteractionRequest{
		RequestID: requestID, SessionID: permission.SessionID, TurnID: permission.TurnID, ToolUseID: permission.ToolUseID,
		ActorID: execution.ActorID, ActorType: execution.ActorType, WorkUnitID: execution.WorkUnitID,
		Questions: cloneAskUserQuestions(in.Questions),
	})
	if err != nil {
		if isAskUserContextError(err) {
			return types.ToolPermissionResult{}, err
		}
		return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserInteractiveUnavailable), nil
	}
	if response.RequestID != requestID {
		return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserInteractionStale), nil
	}
	switch response.Outcome {
	case AskUserInteractionCompleted:
		updated, ok := updatedAskUserInput(input, in.Questions, response.Answers, response.Annotations)
		if !ok {
			return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserNoValidSelection), nil
		}
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: updated}, nil
	case AskUserInteractionCancelled:
		return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserInteractionCancelled), nil
	case AskUserInteractionStale:
		return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserInteractionStale), nil
	default:
		return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserInteractiveUnavailable), nil
	}
}

func askUserPermissionDeny(key i18n.Key) types.ToolPermissionResult {
	return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolRuntimeText(key), Required: true}
}

func (t *AskUserQuestionTool) planApprovalQuestion(questions []QuestionSpec) (string, bool) {
	if t == nil || t.PlanState == nil || !t.PlanState.IsActive() {
		return "", false
	}
	for _, question := range questions {
		if IsApprovalStyleQuestion(question) {
			return question.Question, true
		}
	}
	return "", false
}

func cloneAskUserInput(input map[string]any) map[string]any {
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if json.Unmarshal(encoded, &cloned) != nil {
		return nil
	}
	return cloned
}

func updatedAskUserInput(input map[string]any, questions []QuestionSpec, answers map[string]AnswerSelection, annotations map[string]AnnotationEntry) (map[string]any, bool) {
	updated := cloneAskUserInput(input)
	if updated == nil {
		return nil, false
	}
	formatted := make(map[string]any, len(questions))
	trustedAnnotations := make(map[string]any)
	for _, question := range questions {
		selection, ok := canonicalAskUserSelection(question, answers[question.Question])
		if !ok {
			return nil, false
		}
		parts := append([]string(nil), selection.Selection...)
		if selection.OtherText != "" {
			parts = append(parts, selection.OtherText)
		}
		formatted[question.Question] = strings.Join(parts, ", ")
		if annotation, exists := annotations[question.Question]; exists && (annotation.Notes != "" || annotation.Preview != "") {
			trustedAnnotations[question.Question] = annotation
		}
	}
	updated["answers"] = formatted
	if len(trustedAnnotations) > 0 {
		updated["annotations"] = trustedAnnotations
	} else {
		delete(updated, "annotations")
	}
	return updated, true
}

func canonicalAskUserSelection(question QuestionSpec, selection AnswerSelection) (AnswerSelection, bool) {
	valid := make(map[string]struct{}, len(question.Options))
	for _, option := range question.Options {
		valid[option.Label] = struct{}{}
	}
	canonical := AnswerSelection{OtherText: strings.TrimSpace(selection.OtherText)}
	seen := make(map[string]struct{}, len(selection.Selection))
	for _, label := range selection.Selection {
		if _, exists := valid[label]; !exists {
			return AnswerSelection{}, false
		}
		if _, duplicate := seen[label]; duplicate {
			continue
		}
		seen[label] = struct{}{}
		canonical.Selection = append(canonical.Selection, label)
	}
	count := len(canonical.Selection) + boolInt(canonical.OtherText != "")
	if count == 0 || (!question.MultiSelect && count != 1) {
		return AnswerSelection{}, false
	}
	return canonical, true
}

func askUserQuestionsSchema() map[string]any {
	return map[string]any{
		"type":     "array",
		"minItems": 1,
		"maxItems": 4,
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "The question to ask",
				},
				"header": map[string]any{
					"type":        "string",
					"description": "Short label (max 12 chars)",
				},
				"options": map[string]any{
					"type":     "array",
					"minItems": 2,
					"maxItems": 4,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"label":       map[string]any{"type": "string"},
							"description": map[string]any{"type": "string"},
							"preview":     map[string]any{"type": "string"},
						},
						"required": []string{"label", "description"},
					},
				},
				"multiSelect": map[string]any{
					"type":    "boolean",
					"default": false,
				},
			},
			"required": []string{"question", "header", "options", "multiSelect"},
		},
	}
}

func askUserAnnotationsSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"additionalProperties": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"preview": map[string]any{"type": "string"},
				"notes":   map[string]any{"type": "string"},
			},
		},
	}
}

func (t *AskUserQuestionTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(AskUserQuestionOutput)
	if !ok {
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
		}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   RenderAskUserQuestionOutputText(output),
	}
}

func RenderAskUserQuestionOutputText(output AskUserQuestionOutput) string {
	pairs := make([]string, 0, len(output.Questions))
	for _, q := range output.Questions {
		answer := output.Answers[q.Question]
		parts := []string{fmt.Sprintf("%q=%q", q.Question, answer)}
		if annotation, ok := output.Annotations[q.Question]; ok {
			if annotation.Preview != "" {
				parts = append(parts, toolRuntimeFormat(i18n.KeyToolRuntimeAskUserSelectedPreview, annotation.Preview))
			}
			if annotation.Notes != "" {
				parts = append(parts, toolRuntimeFormat(i18n.KeyToolRuntimeAskUserNotes, annotation.Notes))
			}
		}
		pairs = append(pairs, strings.Join(parts, " "))
	}
	return toolRuntimeFormat(i18n.KeyToolRuntimeAskUserAnsweredContinue, strings.Join(pairs, ", "))
}

func (t *AskUserQuestionTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	// askuser-channels-isenabled: when the user has been routed onto a
	// remote channel (Telegram/Discord/Slack relay) the multiple-choice TUI
	// dialog has no UI to render. Mirrors the TS isEnabled() short-circuit
	// at src/tools/AskUserQuestionTool/AskUserQuestionTool.tsx:135-145 so
	// the model picks a different code path instead of hanging on stdin.
	if AskUserChannelsActive() {
		return types.ToolResult{
			Content: toolRuntimeText(i18n.KeyToolRuntimeAskUserRemoteUnavailable),
			IsError: true, Outcome: types.ToolOutcomeFailed,
		}, nil
	}

	in, toolErr := parseInputOrError[AskUserQuestionInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}

	// Strict schema validation matching the TS reference. Fast-fail on count
	// boundaries so the existing "too many questions" test path still hits.
	if err := ValidateAskUserQuestions(in.Questions); err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeErrorPrefix, err), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}

	// Pre-check context — return early if already cancelled.
	if err := ctx.Err(); err != nil {
		return types.ToolResult{}, err
	}
	if question, blocked := t.planApprovalQuestion(in.Questions); blocked {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeAskUserPlanApproval, question), IsError: true, Outcome: types.ToolOutcomeDenied}, nil
	}

	if answers, ok := completeAskUserAnswerStrings(in.Questions, input); ok {
		output := AskUserQuestionOutput{
			Questions:   in.Questions,
			Answers:     answers,
			Annotations: extractAskUserAnnotations(input),
			Metadata:    mergeAskUserMetadata(t.Metadata, extractAskUserMetadata(input)),
		}
		return askUserToolResultFromOutput(input, output)
	}

	// Execute is deliberately non-interactive. CheckPermissions is the sole
	// owner of the user interaction and must enrich the input with one complete
	// answer per question before execution. This prevents direct calls and
	// retries from opening a second dialog or touching any terminal stream.
	return types.ToolResult{
		Content: toolRuntimeText(i18n.KeyToolRuntimeAskUserAnswersRequired),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}, nil
}

func completeAskUserAnswerStrings(questions []QuestionSpec, input map[string]any) (map[string]string, bool) {
	answers := extractAnswerStrings(input)
	if len(answers) != len(questions) {
		return nil, false
	}
	for _, question := range questions {
		answer, ok := answers[question.Question]
		if !ok || strings.TrimSpace(answer) == "" {
			return nil, false
		}
	}
	return answers, true
}

// collectStreamAnswers adapts an explicitly paired Reader/Writer surface to
// the permission-stage interaction contract. Execute never calls this method.
func (t *AskUserQuestionTool) collectStreamAnswers(ctx context.Context, questions []QuestionSpec) (map[string]AnswerSelection, map[string]AnnotationEntry, error) {
	t.streamMu.Lock()
	defer t.streamMu.Unlock()

	r, w := t.Reader, t.Writer
	if r == nil || w == nil {
		return nil, nil, errors.New(toolRuntimeText(i18n.KeyToolRuntimeAskUserInteractiveUnavailable))
	}
	lang := i18n.DetectOrLoadLanguage()
	answers := make(map[string]AnswerSelection, len(questions))
	annotations := make(map[string]AnnotationEntry, len(questions))
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for _, question := range questions {
		fmt.Fprintf(w, "\n[%s] %s\n", question.Header, question.Question)
		if ResolvePreviewMode(question) == PreviewModeSideBySide {
			fmt.Fprint(w, i18n.Text(lang, i18n.KeyAskUserPreviewSideBySide))
		}
		for index, option := range question.Options {
			if option.Preview != "" {
				fmt.Fprintf(w, "  %d) %s — %s  (%s)\n", index+1, option.Label, option.Description, option.Preview)
			} else {
				fmt.Fprintf(w, "  %d) %s — %s\n", index+1, option.Label, option.Description)
			}
		}

		if question.MultiSelect {
			fmt.Fprint(w, i18n.Text(lang, i18n.KeyAskUserMultiPrompt))
			line, err := scanLineCtx(ctx, scanner)
			if err != nil {
				return nil, nil, err
			}
			line, notes := splitTrailingNotes(line)
			selection, _, err := readMultiSelectFromLine(line, question.Options)
			if err != nil {
				return nil, nil, err
			}
			answers[question.Question] = selection
			if notes != "" {
				annotations[question.Question] = AnnotationEntry{Notes: notes}
			}
			continue
		}

		fmt.Fprint(w, i18n.Format(lang, i18n.KeyAskUserSinglePrompt, len(question.Options)))
		line, err := scanLineCtx(ctx, scanner)
		if err != nil {
			return nil, nil, err
		}
		line, notes := splitTrailingNotes(line)
		label, selection, err := parseSingleSelectLine(ctx, scanner, w, line, question.Options)
		if err != nil {
			return nil, nil, err
		}
		answers[question.Question] = selection
		annotation := AnnotationEntry{Notes: notes}
		for _, option := range question.Options {
			if strings.EqualFold(strings.TrimSpace(option.Label), strings.TrimSpace(label)) {
				annotation.Preview = option.Preview
				break
			}
		}
		if annotation.Notes != "" || annotation.Preview != "" {
			annotations[question.Question] = annotation
		}
	}
	return answers, annotations, nil
}

func cloneAskUserQuestions(questions []QuestionSpec) []QuestionSpec {
	cloned := make([]QuestionSpec, len(questions))
	for index := range questions {
		cloned[index] = questions[index]
		cloned[index].Options = append([]OptionSpec(nil), questions[index].Options...)
	}
	return cloned
}

func buildAskUserInteractionResult(
	questions []QuestionSpec,
	answers map[string]AnswerSelection,
	annotations map[string]AnnotationEntry,
	metadata map[string]any,
	metaSources []string,
	requestID string,
) (types.ToolResult, error) {
	structured := AskUserQuestionResult{
		Answers: make(map[string]AnswerSelection, len(questions)), Annotations: make(map[string]AnnotationEntry), Metadata: metadata,
	}
	output := AskUserQuestionOutput{
		Questions: cloneAskUserQuestions(questions), Answers: make(map[string]string, len(questions)), Annotations: structured.Annotations, Metadata: metadata,
	}
	type qa struct{ question, answer string }
	ordered := make([]qa, 0, len(questions))
	for _, question := range questions {
		selection, ok := answers[question.Question]
		if !ok || (len(selection.Selection) == 0 && strings.TrimSpace(selection.OtherText) == "") {
			return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolRuntimeAskUserNoValidSelection), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
		}
		valid := make(map[string]struct{}, len(question.Options))
		for _, option := range question.Options {
			valid[option.Label] = struct{}{}
		}
		seen := make(map[string]struct{}, len(selection.Selection))
		canonical := AnswerSelection{OtherText: strings.TrimSpace(selection.OtherText)}
		for _, label := range selection.Selection {
			if _, exists := valid[label]; !exists {
				return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolRuntimeAskUserNoValidSelection), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
			}
			if _, duplicate := seen[label]; duplicate {
				continue
			}
			seen[label] = struct{}{}
			canonical.Selection = append(canonical.Selection, label)
		}
		if !question.MultiSelect && len(canonical.Selection)+boolInt(canonical.OtherText != "") != 1 {
			return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolRuntimeAskUserNoValidSelection), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
		}
		structured.Answers[question.Question] = canonical
		parts := append([]string(nil), canonical.Selection...)
		if canonical.OtherText != "" {
			parts = append(parts, canonical.OtherText)
		}
		formatted := strings.Join(parts, ", ")
		output.Answers[question.Question] = formatted
		ordered = append(ordered, qa{question: question.Question, answer: formatted})
		if annotation, exists := annotations[question.Question]; exists && (annotation.Notes != "" || annotation.Preview != "") {
			structured.Annotations[question.Question] = annotation
		}
	}

	pairs := make([]string, 0, len(ordered))
	for _, answer := range ordered {
		pairs = append(pairs, fmt.Sprintf("%q=%q", answer.question, answer.answer))
	}
	text := toolRuntimeFormat(i18n.KeyToolRuntimeAskUserAnswered, strings.Join(pairs, ", "))
	for _, answer := range ordered {
		annotation, ok := structured.Annotations[answer.question]
		if !ok {
			continue
		}
		if annotation.Preview != "" {
			text += "\n" + toolRuntimeFormat(i18n.KeyToolRuntimeAskUserSelectedPreview, annotation.Preview)
		}
		if annotation.Notes != "" {
			text += "\n" + toolRuntimeFormat(i18n.KeyToolRuntimeAskUserNotes, annotation.Notes)
		}
	}
	kind := "ask_user_question_response"
	text += fmt.Sprintf("\nrequest_id=%s kind=%s", requestID, kind)
	if len(metaSources) > 0 {
		text += "\nsource=" + strings.Join(metaSources, ",")
	}
	payload := map[string]any{"_structured": structured, "kind": kind, "message": text, "request_id": requestID, "text": text}
	if len(metadata) > 0 {
		payload["metadata"] = metadata
	}
	if len(metaSources) > 0 {
		source := strings.Join(metaSources, ",")
		payload["source"] = source
		payload["_meta"] = map[string]any{"source": source}
	}
	for _, question := range questions {
		selection := structured.Answers[question.Question]
		var value any = output.Answers[question.Question]
		switch {
		case selection.OtherText != "" && len(selection.Selection) > 0:
			items := append([]string(nil), selection.Selection...)
			value = append(items, selection.OtherText)
		case selection.OtherText != "":
			value = selection.OtherText
		case len(selection.Selection) == 1:
			value = selection.Selection[0]
		case len(selection.Selection) > 1:
			value = append([]string(nil), selection.Selection...)
		}
		payload[question.Question] = value
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeResponseMarshalFailed, err), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	return types.ToolResult{Content: string(encoded), Data: output, Outcome: types.ToolOutcomeSucceeded}, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func askUserToolResultFromOutput(input map[string]any, output AskUserQuestionOutput) (types.ToolResult, error) {
	text := RenderAskUserQuestionOutputText(output)
	requestID := NewRequestID("askuser")
	payload := map[string]any{
		"_structured": output,
		"kind":        "ask_user_question_response",
		"message":     text,
		"request_id":  requestID,
		"text":        text,
		"questions":   output.Questions,
		"answers":     output.Answers,
		"annotations": output.Annotations,
	}
	if len(output.Metadata) > 0 {
		payload["metadata"] = output.Metadata
	}
	if sources := extractMetaSources(input); len(sources) > 0 {
		source := strings.Join(sources, ",")
		payload["source"] = source
		payload["_meta"] = map[string]any{"source": source}
	}
	for _, question := range output.Questions {
		payload[question.Question] = output.Answers[question.Question]
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeResponseMarshalFailed, err), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	return types.ToolResult{Content: string(data), Data: output, Outcome: types.ToolOutcomeSucceeded}, nil
}

func extractAnswerStrings(input map[string]any) map[string]string {
	raw, ok := input["answers"]
	if !ok {
		return nil
	}
	switch answers := raw.(type) {
	case map[string]string:
		out := make(map[string]string, len(answers))
		for key, value := range answers {
			out[key] = value
		}
		return out
	case map[string]any:
		out := make(map[string]string, len(answers))
		for key, value := range answers {
			if s, ok := value.(string); ok {
				out[key] = s
			}
		}
		return out
	default:
		return nil
	}
}

func extractAskUserAnnotations(input map[string]any) map[string]AnnotationEntry {
	raw, ok := input["annotations"]
	if !ok {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var annotations map[string]AnnotationEntry
	if err := json.Unmarshal(data, &annotations); err != nil {
		return nil
	}
	return annotations
}

func extractAskUserMetadata(input map[string]any) map[string]any {
	raw, ok := input["metadata"].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]any, len(raw))
	for key, value := range raw {
		out[key] = value
	}
	return out
}

func mergeAskUserMetadata(base, override map[string]any) map[string]any {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(override))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range override {
		out[key] = value
	}
	return out
}

func isAskUserContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// splitTrailingNotes peels an optional " n:<notes>" suffix off the input
// line, returning (remaining, notes). The 'n:' marker must be at a token
// boundary (preceded by whitespace) so labels containing 'n:' aren't
// accidentally clipped.
func splitTrailingNotes(line string) (string, string) {
	trimmed := strings.TrimSpace(line)
	// Look for the LAST " n:" or "\tn:" so the user can put notes at end.
	for i := len(trimmed) - 1; i >= 2; i-- {
		if (trimmed[i-2] == ' ' || trimmed[i-2] == '\t') &&
			(trimmed[i-1] == 'n' || trimmed[i-1] == 'N') &&
			trimmed[i] == ':' {
			head := strings.TrimSpace(trimmed[:i-2])
			notes := strings.TrimSpace(trimmed[i+1:])
			return head, notes
		}
	}
	return trimmed, ""
}

// parseSingleSelectLine factors out the body of readSingleSelectCtx so we
// can pre-strip notes before interpreting the selection.
func parseSingleSelectLine(ctx context.Context, scanner *bufio.Scanner, w io.Writer, line string, options []OptionSpec) (string, AnswerSelection, error) {
	line = strings.TrimSpace(line)
	if other, ok := ParseOtherSentinel(line); ok {
		return other, AnswerSelection{OtherText: other}, nil
	}
	if strings.EqualFold(line, "o") {
		fmt.Fprint(w, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAskUserCustomPrompt))
		extra, err := scanLineCtx(ctx, scanner)
		if err != nil {
			return "", AnswerSelection{}, err
		}
		extra = strings.TrimSpace(extra)
		return extra, AnswerSelection{OtherText: extra}, nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(options) {
		return "", AnswerSelection{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserInvalidSingleSelection, line, len(options)))
	}
	label := options[n-1].Label
	return label, AnswerSelection{Selection: []string{label}}, nil
}

// extractMetaSources walks the raw questions input and pulls each
// _meta.source value. Using the raw map avoids a parse.go change to add a
// Meta field on QuestionSpec.
func extractMetaSources(input map[string]any) []string {
	if input == nil {
		return nil
	}
	rawQs, ok := input["questions"].([]any)
	if !ok {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	for _, raw := range rawQs {
		qm, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		meta, ok := qm["_meta"].(map[string]any)
		if !ok {
			continue
		}
		src, ok := meta["source"].(string)
		if !ok {
			continue
		}
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}
		if _, dup := seen[src]; dup {
			continue
		}
		seen[src] = struct{}{}
		out = append(out, src)
	}
	return out
}

// scanLineCtx wraps scanner.Scan() with ctx.Done() so a cancelled context
// returns immediately even if the underlying reader blocks.
func scanLineCtx(ctx context.Context, scanner *bufio.Scanner) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	type result struct {
		line string
		ok   bool
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		ok := scanner.Scan()
		ch <- result{line: scanner.Text(), ok: ok, err: scanner.Err()}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		if !res.ok {
			if res.err != nil {
				return "", res.err
			}
			return "", fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeAskUserUnexpectedEnd))
		}
		return res.line, nil
	}
}

// readSingleSelectCtx reads one line and returns (label, AnswerSelection).
// On 'o' input it asks for a follow-up custom answer.
func readSingleSelectCtx(ctx context.Context, scanner *bufio.Scanner, options []OptionSpec, w io.Writer) (string, AnswerSelection, error) {
	line, err := scanLineCtx(ctx, scanner)
	if err != nil {
		return "", AnswerSelection{}, err
	}
	line = strings.TrimSpace(line)

	if other, ok := ParseOtherSentinel(line); ok {
		return other, AnswerSelection{OtherText: other}, nil
	}
	if strings.EqualFold(line, "o") {
		fmt.Fprint(w, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAskUserCustomPrompt))
		extra, err := scanLineCtx(ctx, scanner)
		if err != nil {
			return "", AnswerSelection{}, err
		}
		extra = strings.TrimSpace(extra)
		return extra, AnswerSelection{OtherText: extra}, nil
	}

	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(options) {
		return "", AnswerSelection{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserInvalidSingleSelection, line, len(options)))
	}
	label := options[n-1].Label
	return label, AnswerSelection{Selection: []string{label}}, nil
}

// readMultiSelectFromLine parses a comma-separated list of indices (or the
// "Other:<text>" sentinel) and returns both the structured AnswerSelection
// and a legacy []string view of selected labels for backward-compat.
func readMultiSelectFromLine(line string, options []OptionSpec) (AnswerSelection, []string, error) {
	line = strings.TrimSpace(line)
	parts := strings.Split(line, ",")
	seen := make(map[string]struct{}, len(parts))
	var sel AnswerSelection
	var labels []string

	for _, raw := range parts {
		piece := strings.TrimSpace(raw)
		if piece == "" {
			continue
		}
		// Other sentinel: "Other:<text>" or "o:<text>".
		if other, ok := ParseOtherSentinel(piece); ok {
			if other != "" {
				sel.OtherText = other
			}
			continue
		}
		if strings.HasPrefix(strings.ToLower(piece), "o:") {
			text := strings.TrimSpace(piece[2:])
			if text != "" {
				sel.OtherText = text
			}
			continue
		}

		// Numeric index.
		n, err := strconv.Atoi(piece)
		if err == nil && n >= 1 && n <= len(options) {
			label := options[n-1].Label
			if _, dup := seen[label]; !dup {
				sel.Selection = append(sel.Selection, label)
				labels = append(labels, label)
				seen[label] = struct{}{}
			}
			continue
		}

		// Label match.
		matched := false
		for _, o := range options {
			if strings.EqualFold(strings.TrimSpace(o.Label), piece) {
				if _, dup := seen[o.Label]; !dup {
					sel.Selection = append(sel.Selection, o.Label)
					labels = append(labels, o.Label)
					seen[o.Label] = struct{}{}
				}
				matched = true
				break
			}
		}
		if !matched {
			return AnswerSelection{}, nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserInvalidMultiSelection, piece, len(options)))
		}
	}

	if len(sel.Selection) == 0 && sel.OtherText == "" {
		return AnswerSelection{}, nil, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeAskUserNoValidSelection))
	}
	return sel, labels, nil
}
