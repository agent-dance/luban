package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/permissions"
	gtui "github.com/grindlemire/go-tui"
)

func askUserTUITestRequest(id, sessionID string, questions ...interaction.QuestionSpec) interaction.AskUserInteractionRequest {
	return interaction.AskUserInteractionRequest{
		RequestID: id, SessionID: sessionID, TurnID: "turn", ToolUseID: "tool", ActorID: "assistant", WorkUnitID: "work",
		Questions: questions,
	}
}

func askUserTUITestQuestion(text string, multi bool) interaction.QuestionSpec {
	return interaction.QuestionSpec{
		Question: text, Header: "Choice", MultiSelect: multi,
		Options: []interaction.OptionSpec{{Label: "Alpha", Description: "First"}, {Label: "Beta", Description: "Second", Preview: "preview-beta"}},
	}
}

func waitForAskUserDecision(t *testing.T, state *AppState, decisionID string) *DecisionRequest {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		request := state.DecisionReq.Get()
		if request != nil && request.DecisionID == decisionID && request.Kind == permissions.PromptKindAskUser {
			return request
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("AskUser decision %q was not published: %+v", decisionID, state.DecisionReq.Get())
	return nil
}

func TestTUIAskUserDoesNotMutateComposerAndReturnsTypedMultiCustomAnswer(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	state.SetInteractionEditor("keep this draft", 4)
	root := NewRootComponent(state, nil, nil)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	returned := make(chan interaction.AskUserInteractionResponse, 1)
	errs := make(chan error, 1)
	go func() {
		response, err := renderer.AskUserQuestions(context.Background(), askUserTUITestRequest(
			"ask-one", "session", askUserTUITestQuestion("Choose several?", true), askUserTUITestQuestion("Choose one?", false),
		))
		returned <- response
		errs <- err
	}()
	waitForAskUserDecision(t, state, "ask-one")
	root.ensureAskUserPrompt(state.DecisionReq.Get())
	root.toggleAskUserSelection() // Alpha
	root.moveAskUserSelection(2)  // Other
	root.toggleAskUserSelection()
	for _, char := range "Zig" {
		root.appendAskUserCustom(char)
	}
	root.toggleAskUserSelection() // custom-space path
	for _, char := range "构建" {
		root.appendAskUserCustom(char)
	}
	root.appendAskUserCustom('删')
	root.backspaceAskUserCustom()
	root.confirmAskUserSelection()
	root.moveAskUserSelection(1) // Beta
	root.beginOrAppendAskUserNotes()
	for _, char := range "trusted note" {
		if char == ' ' {
			root.toggleAskUserSelection()
		} else if char == 'n' {
			root.beginOrAppendAskUserNotes()
		} else {
			root.appendAskUserCustom(char)
		}
	}
	root.confirmAskUserSelection()

	select {
	case err := <-errs:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("AskUser did not return")
	}
	response := <-returned
	if response.Outcome != interaction.AskUserInteractionCompleted {
		t.Fatalf("outcome = %q", response.Outcome)
	}
	multi := response.Answers["Choose several?"]
	if strings.Join(multi.Selection, ",") != "Alpha" || multi.OtherText != "Zig 构建" {
		t.Fatalf("multi answer = %+v", multi)
	}
	if single := response.Answers["Choose one?"]; strings.Join(single.Selection, ",") != "Beta" {
		t.Fatalf("single answer = %+v", single)
	}
	if preview := response.Annotations["Choose one?"].Preview; preview != "preview-beta" {
		t.Fatalf("selected preview = %q", preview)
	}
	if notes := response.Annotations["Choose one?"].Notes; notes != "trusted note" {
		t.Fatalf("notes = %q", notes)
	}
	interaction := state.ActiveSessionInteraction()
	if interaction.InputDraft != "keep this draft" || interaction.InputCursor != 4 {
		t.Fatalf("composer mutated by AskUser: %+v", interaction)
	}
}

func TestTUIAskUserKeymapKeepsCustomTypingOutOfComposer(t *testing.T) {
	state := newDecisionBrokerTestState()
	request := &DecisionRequest{
		DecisionID: "keys", SessionID: "session", Kind: permissions.PromptKindAskUser,
		Questionnaire: &permissions.AskUserQuestionnaire{Questions: []permissions.AskUserQuestion{{
			Question: "Choose?", Header: "Choice", Options: []permissions.AskUserOption{{Label: "Alpha"}, {Label: "Beta"}},
		}}},
	}
	state.DecisionReq.Set(request)
	root := NewRootComponent(state, nil, nil)
	root.input.SetText("COMPOSER_SENTINEL")
	root.input.SetCursorPosition(3)
	root.input.Focus()
	root.ensureAskUserPrompt(request)
	root.moveAskUserSelection(2)
	root.confirmAskUserSelection()
	for _, event := range []gtui.KeyEvent{{Key: gtui.KeyRune, Rune: 'Z'}, {Key: gtui.KeyRune, Rune: 'i'}, {Key: gtui.KeyRune, Rune: 'g'}, {Key: gtui.KeyRune, Rune: ' '}, {Key: gtui.KeyRune, Rune: 'n'}} {
		dispatchRootKeyForTest(t, root, event)
	}
	if draft := state.AskUserDraft.Get(); draft == nil || draft.CustomText != "Zig n" {
		t.Fatalf("custom key input = %+v", draft)
	}
	if root.input.Text() != "COMPOSER_SENTINEL" || root.input.CursorPosition() != 3 || !root.input.IsFocused() {
		t.Fatalf("AskUser keymap mutated composer text=%q cursor=%d focused=%v", root.input.Text(), root.input.CursorPosition(), root.input.IsFocused())
	}
}

func TestTUIAskUserAdmissionRejectsNonActiveSession(t *testing.T) {
	state := newDecisionBrokerTestState()
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	response, err := renderer.AskUserQuestions(context.Background(), askUserTUITestRequest("wrong", "another-session", askUserTUITestQuestion("Choose?", false)))
	if err != nil {
		t.Fatal(err)
	}
	if response.Outcome != interaction.AskUserInteractionStale || state.DecisionReq.Get() != nil {
		t.Fatalf("wrong-session response=%+v overlay=%+v", response, state.DecisionReq.Get())
	}
}

func TestTUIAskUserSessionEpochChangeFailsClosed(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	returned := make(chan interaction.AskUserInteractionResponse, 1)
	go func() {
		response, _ := renderer.AskUserQuestions(context.Background(), askUserTUITestRequest("stale", "session", askUserTUITestQuestion("Choose?", false)))
		returned <- response
	}()
	waitForAskUserDecision(t, state, "stale")
	state.SessionEpoch.Set(2)
	response := decisionResponse("stale", permissions.PromptOutcomeApproved, "submit")
	response.Questionnaire = &permissions.AskUserQuestionnaireResponse{Answers: map[string]permissions.AskUserAnswer{"Choose?": {Selection: []string{"Alpha"}}}}
	state.DecisionResp <- response
	select {
	case result := <-returned:
		if result.Outcome != interaction.AskUserInteractionStale || len(result.Answers) != 0 {
			t.Fatalf("stale result = %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("stale AskUser waiter leaked")
	}
	if state.DecisionReq.Get() != nil {
		t.Fatalf("stale overlay remained: %+v", state.DecisionReq.Get())
	}
}

func TestTUIDecisionAndAskUserShareOneSerializedInputOwner(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	permissionDone := make(chan permissions.PromptResponse, 1)
	go func() {
		permissionDone <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{DecisionID: "permission", SessionID: "session", Kind: permissions.PromptKindPermission})
	}()
	waitForDecisionID(t, state, "permission")
	askDone := make(chan interaction.AskUserInteractionResponse, 1)
	go func() {
		response, _ := renderer.AskUserQuestions(context.Background(), askUserTUITestRequest("ask-queued", "session", askUserTUITestQuestion("Choose?", false)))
		askDone <- response
	}()
	waitForDecisionAttentionCount(t, state, 2)
	if active := state.DecisionReq.Get(); active == nil || active.DecisionID != "permission" || active.Questionnaire != nil {
		t.Fatalf("AskUser displaced active permission: %+v", active)
	}
	state.DecisionResp <- decisionResponse("permission", permissions.PromptOutcomeRejected, "reject")
	<-permissionDone
	waitForAskUserDecision(t, state, "ask-queued")
	answer := decisionResponse("ask-queued", permissions.PromptOutcomeApproved, "submit")
	answer.Questionnaire = &permissions.AskUserQuestionnaireResponse{Answers: map[string]permissions.AskUserAnswer{"Choose?": {Selection: []string{"Alpha"}}}}
	state.DecisionResp <- answer
	select {
	case result := <-askDone:
		if result.Outcome != interaction.AskUserInteractionCompleted {
			t.Fatalf("AskUser result = %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("serialized AskUser did not complete")
	}
}

func TestTUIAskUserContextCancellationClearsOverlay(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan error, 1)
	go func() {
		_, err := renderer.AskUserQuestions(ctx, askUserTUITestRequest("cancel", "session", askUserTUITestQuestion("Choose?", false)))
		returned <- err
	}()
	waitForAskUserDecision(t, state, "cancel")
	cancel()
	select {
	case err := <-returned:
		if err != context.Canceled {
			t.Fatalf("cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled AskUser waiter leaked")
	}
	if state.DecisionReq.Get() != nil {
		t.Fatalf("cancelled overlay remained: %+v", state.DecisionReq.Get())
	}
}

func TestTUIAskUserUsesActiveLanguageAndVisualizesUntrustedControls(t *testing.T) {
	state := newDecisionBrokerTestState()
	state.Language.Set(i18n.LangZH)
	request := &DecisionRequest{
		DecisionID: "render", SessionID: "session", Kind: permissions.PromptKindAskUser,
		Questionnaire: &permissions.AskUserQuestionnaire{Questions: []permissions.AskUserQuestion{{
			Header: "原始\x1b]0;bad\a", Question: "选择\x1b[31m?", Options: []permissions.AskUserOption{{Label: "甲\x00", Description: "说明\u009b", Preview: "预览\r"}, {Label: "乙", Description: "第二"}},
		}}},
	}
	state.DecisionReq.Set(request)
	root := NewRootComponent(state, nil, nil)
	text := collectElementText(root.renderAskUserDialog(request))
	for _, want := range []string{"回答这些问题", "第 1 个问题，共 1 个", "原始", "选择", "甲", `\x1b`, `\x00`, `\u009b`, `\r`} {
		if !strings.Contains(text, want) {
			t.Fatalf("render omitted %q: %q", want, text)
		}
	}
	for _, forbidden := range []rune{'\x00', '\x07', '\x1b', '\x9b', '\r'} {
		if strings.ContainsRune(text, forbidden) {
			t.Fatalf("render retained executable control U+%04X: %q", forbidden, text)
		}
	}
}

func TestApplySessionSnapshotClearsAskUserTransientDraft(t *testing.T) {
	state := newDecisionBrokerTestState()
	state.DecisionReq.Set(&DecisionRequest{DecisionID: "old-ask", SessionID: "session", Kind: permissions.PromptKindAskUser})
	state.AskUserDraft.Set(&AskUserPromptState{DecisionID: "old-ask", CustomActive: true, CustomText: "must not cross sessions"})
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity:   SessionIdentity{Namespace: "project", SessionID: "next-session", Epoch: 2},
		Projection: SessionProjection{Details: NewMemoryDetailStore()},
	}); err != nil {
		t.Fatal(err)
	}
	if state.DecisionReq.Get() != nil || state.AskUserDraft.Get() != nil {
		t.Fatalf("AskUser transients crossed snapshot: request=%+v draft=%+v", state.DecisionReq.Get(), state.AskUserDraft.Get())
	}
}
