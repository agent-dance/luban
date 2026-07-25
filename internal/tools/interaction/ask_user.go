package interaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/i18n"
	interactioncontract "github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

// AskUserQuestionTool presents multiple-choice questions to the user and
// collects their answers. Interactive runtimes inject exactly one structured
// surface owner. Execute is non-interactive and never touches process streams.
//
// Optional PlanState is consulted to flag approval-style questions when plan
// mode is active (TS rule: use ExitPlanMode for approval, AskUserQuestion for
// genuine multi-choice).
type AskUserQuestionTool struct {
	planState *PlanState

	interactionMu sync.RWMutex
	interaction   interactioncontract.AskUserInteractionRequester
}

type askUserQuestionInput struct {
	Questions []interactioncontract.QuestionSpec `json:"questions"`
}

func NewAskUserQuestionTool(planState *PlanState) *AskUserQuestionTool {
	return &AskUserQuestionTool{planState: planState}
}

// SetInteractionRequester atomically binds the runtime's sole interactive
// input owner. Passing nil unbinds it during surface teardown.
func (t *AskUserQuestionTool) SetInteractionRequester(requester interactioncontract.AskUserInteractionRequester) {
	if t == nil {
		return
	}
	t.interactionMu.Lock()
	t.interaction = requester
	t.interactionMu.Unlock()
}

func (t *AskUserQuestionTool) interactionRequester() interactioncontract.AskUserInteractionRequester {
	if t == nil {
		return nil
	}
	t.interactionMu.RLock()
	defer t.interactionMu.RUnlock()
	return t.interaction
}

func (t *AskUserQuestionTool) Name() string { return "AskUserQuestion" }

func (t *AskUserQuestionTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	return runtime.Interactive
}

func (t *AskUserQuestionTool) Description() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolInteractionAskUserDescription)
}

func (t *AskUserQuestionTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"questions":   askUserQuestionsSchema(),
		"answers":     map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
		"annotations": askUserAnnotationsSchema(),
	}, "questions")
}

func (t *AskUserQuestionTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true}
}

func (t *AskUserQuestionTool) CheckPermissions(ctx context.Context, input map[string]any, permission types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	in, toolErr := toolbase.ParseInputOrError[askUserQuestionInput](input)
	if toolErr != nil {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolErr.Content}, nil
	}
	if err := ValidateAskUserQuestions(in.Questions); err != nil {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeErrorPrefix, err)}, nil
	}
	if question, blocked := t.planApprovalQuestion(in.Questions); blocked {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserPlanApproval, question)}, nil
	}
	if err := ctx.Err(); err != nil {
		return types.ToolPermissionResult{}, err
	}
	requester := t.interactionRequester()
	if requester == nil {
		return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserInteractiveUnavailable), nil
	}

	execution, _ := executioncontract.ToolExecutionContextFromContext(ctx)
	requestID := "ask-user:" + permission.SessionID + ":" + permission.ToolUseID
	if permission.SessionID == "" || permission.ToolUseID == "" {
		requestID = newRequestID("askuser")
	}
	response, err := requester.AskUserQuestions(ctx, interactioncontract.AskUserInteractionRequest{
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
	case interactioncontract.AskUserInteractionCompleted:
		updated, ok := updatedAskUserInput(input, in.Questions, response.Answers, response.Annotations)
		if !ok {
			return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserNoValidSelection), nil
		}
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: updated}, nil
	case interactioncontract.AskUserInteractionCancelled:
		return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserInteractionCancelled), nil
	case interactioncontract.AskUserInteractionStale:
		return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserInteractionStale), nil
	default:
		return askUserPermissionDeny(i18n.KeyToolRuntimeAskUserInteractiveUnavailable), nil
	}
}

func askUserPermissionDeny(key i18n.Key) types.ToolPermissionResult {
	return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: i18n.Text(i18n.DetectOrLoadLanguage(), key), Required: true}
}

func (t *AskUserQuestionTool) planApprovalQuestion(questions []interactioncontract.QuestionSpec) (string, bool) {
	if t == nil || t.planState == nil || !t.planState.IsActive() {
		return "", false
	}
	for _, question := range questions {
		if isApprovalStyleQuestion(question) {
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

func updatedAskUserInput(input map[string]any, questions []interactioncontract.QuestionSpec, answers map[string]interactioncontract.AnswerSelection, annotations map[string]interactioncontract.AnnotationEntry) (map[string]any, bool) {
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

func canonicalAskUserSelection(question interactioncontract.QuestionSpec, selection interactioncontract.AnswerSelection) (interactioncontract.AnswerSelection, bool) {
	valid := make(map[string]struct{}, len(question.Options))
	for _, option := range question.Options {
		valid[option.Label] = struct{}{}
	}
	canonical := interactioncontract.AnswerSelection{OtherText: strings.TrimSpace(selection.OtherText)}
	seen := make(map[string]struct{}, len(selection.Selection))
	for _, label := range selection.Selection {
		if _, exists := valid[label]; !exists {
			return interactioncontract.AnswerSelection{}, false
		}
		if _, duplicate := seen[label]; duplicate {
			continue
		}
		seen[label] = struct{}{}
		canonical.Selection = append(canonical.Selection, label)
	}
	count := len(canonical.Selection) + boolInt(canonical.OtherText != "")
	if count == 0 || (!question.MultiSelect && count != 1) {
		return interactioncontract.AnswerSelection{}, false
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
					"description": i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolInteractionAskUserQuestion),
				},
				"header": map[string]any{
					"type":        "string",
					"description": i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolInteractionAskUserHeader),
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
	output, ok := data.(askUserQuestionOutput)
	if !ok {
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
		}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   renderAskUserQuestionOutputText(output),
	}
}

func renderAskUserQuestionOutputText(output askUserQuestionOutput) string {
	pairs := make([]string, 0, len(output.Questions))
	for _, q := range output.Questions {
		answer := output.Answers[q.Question]
		parts := []string{fmt.Sprintf("%q=%q", q.Question, answer)}
		if annotation, ok := output.Annotations[q.Question]; ok {
			if annotation.Preview != "" {
				parts = append(parts, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserSelectedPreview, annotation.Preview))
			}
			if annotation.Notes != "" {
				parts = append(parts, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserNotes, annotation.Notes))
			}
		}
		pairs = append(pairs, strings.Join(parts, " "))
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserAnsweredContinue, strings.Join(pairs, ", "))
}

func (t *AskUserQuestionTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := toolbase.ParseInputOrError[askUserQuestionInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}

	// Strict schema validation matching the TS reference. Fast-fail on count
	// boundaries so the existing "too many questions" test path still hits.
	if err := ValidateAskUserQuestions(in.Questions); err != nil {
		return types.ToolResult{Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeErrorPrefix, err), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}

	// Pre-check context — return early if already cancelled.
	if err := ctx.Err(); err != nil {
		return types.ToolResult{}, err
	}
	if question, blocked := t.planApprovalQuestion(in.Questions); blocked {
		return types.ToolResult{Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserPlanApproval, question), IsError: true, Outcome: types.ToolOutcomeDenied}, nil
	}

	if answers, ok := completeAskUserAnswerStrings(in.Questions, input); ok {
		output := askUserQuestionOutput{
			Questions:   in.Questions,
			Answers:     answers,
			Annotations: extractAskUserAnnotations(input),
		}
		return askUserToolResultFromOutput(output)
	}

	// Execute is deliberately non-interactive. CheckPermissions is the sole
	// owner of the user interaction and must enrich the input with one complete
	// answer per question before execution. This prevents direct calls and
	// retries from opening a second dialog or touching any terminal stream.
	return types.ToolResult{
		Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserAnswersRequired),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}, nil
}

func completeAskUserAnswerStrings(questions []interactioncontract.QuestionSpec, input map[string]any) (map[string]string, bool) {
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

func cloneAskUserQuestions(questions []interactioncontract.QuestionSpec) []interactioncontract.QuestionSpec {
	cloned := make([]interactioncontract.QuestionSpec, len(questions))
	for index := range questions {
		cloned[index] = questions[index]
		cloned[index].Options = append([]interactioncontract.OptionSpec(nil), questions[index].Options...)
	}
	return cloned
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func askUserToolResultFromOutput(output askUserQuestionOutput) (types.ToolResult, error) {
	text := renderAskUserQuestionOutputText(output)
	return types.ToolResult{Content: text, Data: output, Outcome: types.ToolOutcomeSucceeded}, nil
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

func extractAskUserAnnotations(input map[string]any) map[string]interactioncontract.AnnotationEntry {
	raw, ok := input["annotations"]
	if !ok {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var annotations map[string]interactioncontract.AnnotationEntry
	if err := json.Unmarshal(data, &annotations); err != nil {
		return nil
	}
	return annotations
}

func isAskUserContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
