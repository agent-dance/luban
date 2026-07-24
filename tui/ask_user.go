package tui

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/tools"
	gotui "github.com/grindlemire/go-tui"
)

// AskUserPromptState is the draft owned by the active structured decision.
// It is process-local and excluded from session checkpoints.
type AskUserPromptState struct {
	DecisionID    string
	QuestionIndex int
	SelectedIndex int
	Selected      []bool
	CustomActive  bool
	CustomText    string
	NotesActive   bool
	NotesText     string
	Answers       map[string]permissions.AskUserAnswer
}

func cloneAskUserPrompt(prompt *AskUserPromptState) *AskUserPromptState {
	if prompt == nil {
		return nil
	}
	cloned := *prompt
	cloned.Selected = append([]bool(nil), prompt.Selected...)
	cloned.Answers = make(map[string]permissions.AskUserAnswer, len(prompt.Answers))
	for question, answer := range prompt.Answers {
		answer.Selection = append([]string(nil), answer.Selection...)
		cloned.Answers[question] = answer
	}
	return &cloned
}

func cloneAskUserQuestionnaire(questionnaire *permissions.AskUserQuestionnaire) *permissions.AskUserQuestionnaire {
	if questionnaire == nil {
		return nil
	}
	cloned := &permissions.AskUserQuestionnaire{Questions: make([]permissions.AskUserQuestion, len(questionnaire.Questions))}
	for index, question := range questionnaire.Questions {
		cloned.Questions[index] = question
		cloned.Questions[index].Options = append([]permissions.AskUserOption(nil), question.Options...)
	}
	return cloned
}

func cloneAskUserQuestionnaireResponse(response *permissions.AskUserQuestionnaireResponse) *permissions.AskUserQuestionnaireResponse {
	if response == nil {
		return nil
	}
	cloned := &permissions.AskUserQuestionnaireResponse{Answers: make(map[string]permissions.AskUserAnswer, len(response.Answers))}
	for question, answer := range response.Answers {
		answer.Selection = append([]string(nil), answer.Selection...)
		cloned.Answers[question] = answer
	}
	return cloned
}

func (r *TuiRenderer) AskUserQuestions(ctx context.Context, request tools.AskUserInteractionRequest) (tools.AskUserInteractionResponse, error) {
	result := tools.AskUserInteractionResponse{RequestID: request.RequestID, Outcome: tools.AskUserInteractionShutdown}
	if r == nil || r.state == nil || strings.TrimSpace(request.RequestID) == "" {
		return result, nil
	}
	if strings.TrimSpace(request.SessionID) == "" || r.state.SessionID.Get() != request.SessionID {
		result.Outcome = tools.AskUserInteractionStale
		return result, nil
	}
	requestEpoch := r.state.SessionEpoch.Get()
	questionnaire := &permissions.AskUserQuestionnaire{Questions: make([]permissions.AskUserQuestion, len(request.Questions))}
	for index, question := range request.Questions {
		converted := permissions.AskUserQuestion{
			Question: question.Question, Header: question.Header, MultiSelect: question.MultiSelect,
			Options: make([]permissions.AskUserOption, len(question.Options)),
		}
		for optionIndex, option := range question.Options {
			converted.Options[optionIndex] = permissions.AskUserOption{Label: option.Label, Description: option.Description, Preview: option.Preview}
		}
		questionnaire.Questions[index] = converted
	}
	response := r.DecisionRequest(ctx, permissions.PromptRequest{
		DecisionID: request.RequestID, SessionID: request.SessionID, TurnID: request.TurnID,
		ToolUseID: request.ToolUseID, ToolName: "AskUserQuestion", ActorID: request.ActorID,
		ActorType: request.ActorType, WorkUnitID: request.WorkUnitID, Kind: permissions.PromptKindAskUser,
		Action:        i18n.Text(r.state.Language.Get(), i18n.KeyToolActionAskUser),
		Questionnaire: questionnaire,
	})
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if r.state.SessionID.Get() != request.SessionID || r.state.SessionEpoch.Get() != requestEpoch {
		result.Outcome = tools.AskUserInteractionStale
		return result, nil
	}
	if response.Outcome != permissions.PromptOutcomeApproved || response.Questionnaire == nil {
		result.Outcome = tools.AskUserInteractionCancelled
		return result, nil
	}
	result.Answers = make(map[string]tools.AnswerSelection, len(response.Questionnaire.Answers))
	result.Annotations = make(map[string]tools.AnnotationEntry)
	for _, question := range request.Questions {
		answer, ok := response.Questionnaire.Answers[question.Question]
		if !ok {
			continue
		}
		result.Answers[question.Question] = tools.AnswerSelection{Selection: append([]string(nil), answer.Selection...), OtherText: answer.OtherText}
		annotation := tools.AnnotationEntry{Notes: answer.Notes}
		if !question.MultiSelect && len(answer.Selection) == 1 {
			for _, option := range question.Options {
				if option.Label == answer.Selection[0] {
					annotation.Preview = option.Preview
					break
				}
			}
		}
		if annotation.Notes != "" || annotation.Preview != "" {
			result.Annotations[question.Question] = annotation
		}
	}
	result.Outcome = tools.AskUserInteractionCompleted
	return result, nil
}

func (c *RootComponent) activeAskUserRequest() *DecisionRequest {
	request := c.state.DecisionReq.Get()
	if request == nil || request.Kind != permissions.PromptKindAskUser || request.Questionnaire == nil || len(request.Questionnaire.Questions) == 0 {
		return nil
	}
	return request
}

func (c *RootComponent) ensureAskUserPrompt(request *DecisionRequest) *AskUserPromptState {
	if request == nil || request.Questionnaire == nil || len(request.Questionnaire.Questions) == 0 {
		return nil
	}
	if draft := c.state.AskUserDraft.Get(); draft != nil && draft.DecisionID == request.DecisionID && draft.QuestionIndex < len(request.Questionnaire.Questions) {
		return draft
	}
	draft := &AskUserPromptState{
		DecisionID: request.DecisionID, Answers: make(map[string]permissions.AskUserAnswer),
		Selected: make([]bool, len(request.Questionnaire.Questions[0].Options)),
	}
	c.state.AskUserDraft.Set(draft)
	return draft
}

func (c *RootComponent) askUserDialogHeight(request *DecisionRequest) int {
	draft := c.ensureAskUserPrompt(request)
	if draft == nil {
		return 0
	}
	question := request.Questionnaire.Questions[draft.QuestionIndex]
	rows := 8 + len(question.Options)
	for _, option := range question.Options {
		rows += strings.Count(option.Description, "\n")
		if option.Preview != "" {
			rows += strings.Count(option.Preview, "\n") + 1
		}
	}
	if draft.CustomActive || draft.NotesActive {
		rows += 2
	}
	if c.termHeight > 0 && rows > c.termHeight-4 {
		rows = c.termHeight - 4
	}
	if rows < 8 {
		rows = 8
	}
	return rows
}

func (c *RootComponent) renderAskUserDialog(request *DecisionRequest) *gotui.Element {
	draft := c.ensureAskUserPrompt(request)
	lang := c.state.Language.Get()
	question := request.Questionnaire.Questions[draft.QuestionIndex]
	container := gotui.New(
		gotui.WithDirection(gotui.Column), gotui.WithBorder(gotui.BorderDouble),
		gotui.WithBorderStyle(gotui.NewStyle().Foreground(gotui.Cyan)),
		gotui.WithPaddingTRBL(0, 1, 0, 1), gotui.WithHeight(c.askUserDialogHeight(request)), gotui.WithWidthPercent(100),
	)
	container.AddChild(gotui.New(gotui.WithText(i18n.Text(lang, i18n.KeyAskUserPermission)), gotui.WithTextStyle(gotui.NewStyle().Foreground(gotui.Cyan).Bold())))
	container.AddChild(gotui.New(gotui.WithText(i18n.Format(lang, i18n.KeyAskUserProgress, draft.QuestionIndex+1, len(request.Questionnaire.Questions))), gotui.WithTextStyle(gotui.NewStyle().Dim())))
	container.AddChild(gotui.New(gotui.WithText("["+safeAskUserText(question.Header)+"] "+safeAskUserText(question.Question)), gotui.WithTextStyle(gotui.NewStyle().Bold())))
	for index, option := range question.Options {
		marker := "  "
		if index == draft.SelectedIndex {
			marker = "> "
		}
		checked := ""
		if question.MultiSelect {
			checked = "[ ] "
			if index < len(draft.Selected) && draft.Selected[index] {
				checked = "[x] "
			}
		}
		container.AddChild(gotui.New(gotui.WithText(marker+checked+safeAskUserText(option.Label)+" — "+safeAskUserText(option.Description)), gotui.WithWidthPercent(100)))
		if option.Preview != "" {
			container.AddChild(gotui.New(gotui.WithText("    "+safeAskUserText(option.Preview)), gotui.WithTextStyle(gotui.NewStyle().Dim()), gotui.WithWidthPercent(100)))
		}
	}
	otherIndex := len(question.Options)
	marker := "  "
	if draft.SelectedIndex == otherIndex {
		marker = "> "
	}
	container.AddChild(gotui.New(gotui.WithText(marker + i18n.Text(lang, i18n.KeyAskUserOtherOption))))
	if draft.CustomActive {
		container.AddChild(gotui.New(gotui.WithText(i18n.Text(lang, i18n.KeyAskUserCustomPrompt)+safeAskUserText(draft.CustomText)+"|"), gotui.WithWidthPercent(100)))
		container.AddChild(gotui.New(gotui.WithText(i18n.Text(lang, i18n.KeyAskUserTUICustomHint)), gotui.WithTextStyle(gotui.NewStyle().Dim())))
	} else if draft.NotesActive {
		container.AddChild(gotui.New(gotui.WithText(i18n.Text(lang, i18n.KeyAskUserTUINotesPrompt)+safeAskUserText(draft.NotesText)+"|"), gotui.WithWidthPercent(100)))
		container.AddChild(gotui.New(gotui.WithText(i18n.Text(lang, i18n.KeyAskUserTUINotesHint)), gotui.WithTextStyle(gotui.NewStyle().Dim())))
	} else {
		hint := i18n.KeyAskUserTUISingleHint
		if question.MultiSelect {
			hint = i18n.KeyAskUserTUIMultiHint
		}
		container.AddChild(gotui.New(gotui.WithText(i18n.Text(lang, hint)), gotui.WithTextStyle(gotui.NewStyle().Dim())))
		container.AddChild(gotui.New(gotui.WithText(i18n.Text(lang, i18n.KeyAskUserTUINotesAvailable)), gotui.WithTextStyle(gotui.NewStyle().Dim())))
	}
	return container
}

func safeAskUserText(value string) string {
	var safe strings.Builder
	for _, char := range value {
		switch {
		case char == '\n':
			safe.WriteByte('\n')
		case char == '\r':
			safe.WriteString(`\r`)
		case char == '\t':
			safe.WriteString(`\t`)
		case char == 0x1b:
			safe.WriteString(`\x1b`)
		case char < 0x20 || char == 0x7f:
			fmt.Fprintf(&safe, `\x%02x`, char)
		case char >= 0x80 && char <= 0x9f:
			fmt.Fprintf(&safe, `\u%04x`, char)
		default:
			safe.WriteRune(char)
		}
	}
	return safe.String()
}

func (c *RootComponent) updateAskUserPrompt(update func(*AskUserPromptState)) {
	draft := cloneAskUserPrompt(c.state.AskUserDraft.Get())
	if draft == nil {
		return
	}
	update(draft)
	c.state.AskUserDraft.Set(draft)
}

func (c *RootComponent) currentAskUserQuestion(request *DecisionRequest, draft *AskUserPromptState) *permissions.AskUserQuestion {
	if request == nil || request.Questionnaire == nil || draft == nil || draft.QuestionIndex < 0 || draft.QuestionIndex >= len(request.Questionnaire.Questions) {
		return nil
	}
	return &request.Questionnaire.Questions[draft.QuestionIndex]
}

func (c *RootComponent) moveAskUserSelection(delta int) {
	request := c.activeAskUserRequest()
	c.updateAskUserPrompt(func(draft *AskUserPromptState) {
		question := c.currentAskUserQuestion(request, draft)
		if question == nil || draft.CustomActive || draft.NotesActive {
			return
		}
		count := len(question.Options) + 1
		draft.SelectedIndex = (draft.SelectedIndex + delta) % count
		if draft.SelectedIndex < 0 {
			draft.SelectedIndex += count
		}
	})
}

func (c *RootComponent) toggleAskUserSelection() {
	request := c.activeAskUserRequest()
	c.updateAskUserPrompt(func(draft *AskUserPromptState) {
		question := c.currentAskUserQuestion(request, draft)
		if question == nil {
			return
		}
		if draft.CustomActive {
			draft.CustomText += " "
			return
		}
		if draft.NotesActive {
			draft.NotesText += " "
			return
		}
		if !question.MultiSelect {
			return
		}
		if draft.SelectedIndex == len(question.Options) {
			draft.CustomActive = true
			return
		}
		draft.Selected[draft.SelectedIndex] = !draft.Selected[draft.SelectedIndex]
	})
}

func (c *RootComponent) appendAskUserCustom(char rune) {
	c.updateAskUserPrompt(func(draft *AskUserPromptState) {
		if draft.CustomActive {
			draft.CustomText += string(char)
		} else if draft.NotesActive {
			draft.NotesText += string(char)
		}
	})
}

func (c *RootComponent) backspaceAskUserCustom() {
	c.updateAskUserPrompt(func(draft *AskUserPromptState) {
		if draft.CustomActive && draft.CustomText != "" {
			_, size := utf8.DecodeLastRuneInString(draft.CustomText)
			draft.CustomText = draft.CustomText[:len(draft.CustomText)-size]
		} else if draft.NotesActive && draft.NotesText != "" {
			_, size := utf8.DecodeLastRuneInString(draft.NotesText)
			draft.NotesText = draft.NotesText[:len(draft.NotesText)-size]
		}
	})
}

func (c *RootComponent) beginOrAppendAskUserNotes() {
	c.updateAskUserPrompt(func(draft *AskUserPromptState) {
		if draft.CustomActive {
			draft.CustomText += "n"
			return
		}
		if draft.NotesActive {
			draft.NotesText += "n"
			return
		}
		draft.NotesActive = true
	})
}

func (c *RootComponent) confirmAskUserSelection() {
	request := c.activeAskUserRequest()
	draft := cloneAskUserPrompt(c.state.AskUserDraft.Get())
	question := c.currentAskUserQuestion(request, draft)
	if question == nil {
		return
	}
	answer := askUserSelection(question, draft)
	if draft.NotesActive {
		draft.NotesActive = false
	}
	if draft.CustomActive {
		answer.OtherText = strings.TrimSpace(draft.CustomText)
		if answer.OtherText == "" {
			return
		}
	} else if draft.SelectedIndex == len(question.Options) {
		draft.CustomActive = true
		c.state.AskUserDraft.Set(draft)
		return
	} else if question.MultiSelect {
		if len(answer.Selection) == 0 {
			draft.Selected[draft.SelectedIndex] = true
			c.state.AskUserDraft.Set(draft)
			return
		}
	} else {
		answer.Selection = []string{question.Options[draft.SelectedIndex].Label}
	}
	answer.Notes = strings.TrimSpace(draft.NotesText)
	draft.Answers[question.Question] = answer
	if draft.QuestionIndex+1 < len(request.Questionnaire.Questions) {
		draft.QuestionIndex++
		draft.SelectedIndex = 0
		draft.Selected = make([]bool, len(request.Questionnaire.Questions[draft.QuestionIndex].Options))
		draft.CustomActive = false
		draft.CustomText = ""
		draft.NotesActive = false
		draft.NotesText = ""
		c.state.AskUserDraft.Set(draft)
		return
	}
	c.sendAskUserDecision(request.DecisionID, permissions.PromptOutcomeApproved, "submit", &permissions.AskUserQuestionnaireResponse{Answers: draft.Answers})
}

func askUserSelection(question *permissions.AskUserQuestion, draft *AskUserPromptState) permissions.AskUserAnswer {
	answer := permissions.AskUserAnswer{}
	for index, selected := range draft.Selected {
		if selected && index < len(question.Options) {
			answer.Selection = append(answer.Selection, question.Options[index].Label)
		}
	}
	return answer
}

func (c *RootComponent) escapeAskUser() {
	request := c.activeAskUserRequest()
	draft := cloneAskUserPrompt(c.state.AskUserDraft.Get())
	if request == nil || draft == nil {
		return
	}
	if draft.CustomActive {
		draft.CustomActive = false
		draft.CustomText = ""
		c.state.AskUserDraft.Set(draft)
		return
	}
	if draft.NotesActive {
		draft.NotesActive = false
		draft.NotesText = ""
		c.state.AskUserDraft.Set(draft)
		return
	}
	c.sendAskUserDecision(request.DecisionID, permissions.PromptOutcomeEscaped, "", nil)
}

func (c *RootComponent) sendAskUserDecision(decisionID string, outcome permissions.PromptOutcome, choice string, questionnaire *permissions.AskUserQuestionnaireResponse) {
	response := decisionResponse(decisionID, outcome, choice)
	response.Questionnaire = cloneAskUserQuestionnaireResponse(questionnaire)
	select {
	case c.state.DecisionResp <- response:
	default:
	}
}
