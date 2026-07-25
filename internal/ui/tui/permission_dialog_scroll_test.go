package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/permissions"
	gtui "github.com/grindlemire/go-tui"
)

func permissionScrollTestRequest() *DecisionRequest {
	return &DecisionRequest{
		DecisionID:         "decision-scroll",
		ActorID:            "agent-" + strings.Repeat("0123456789", 18),
		ActorType:          "general-purpose",
		WorkUnitID:         "work-" + strings.Repeat("abcdefghij", 18),
		ExecutionSessionID: "session-" + strings.Repeat("9876543210", 18),
		Kind:               permissions.PromptKindPermission,
		Action:             "Write",
		Target:             "/workspace/" + strings.Repeat("very-long-directory/", 18) + "artifact.go",
		Impact:             strings.Repeat("preserve the complete external impact value ", 12),
		RiskLevel:          2,
		RiskReason:         strings.Repeat("review the complete external risk reason ", 12),
		ApprovalScope:      "once",
		RuleSource:         "workspace-policy",
		Choices:            []string{"allow_once", "reject", "always_allow"},
	}
}

func renderPermissionScrollTestRoot(t *testing.T, root *RootComponent, width, height int) *gtui.Element {
	t.Helper()
	element := root.renderAtSize(nil, width, height)
	element.Render(gtui.NewBuffer(width, height), width, height)
	if root.decisionRef.El() == nil {
		t.Fatal("permission details ref was not rendered")
	}
	return element
}

func firePermissionScrollBinding(t *testing.T, root *RootComponent, key gtui.Key, mod gtui.Modifier) {
	t.Helper()
	for _, binding := range root.KeyMap() {
		if !binding.Pattern.AnyKey && binding.Pattern.Key == key && binding.Pattern.Mod == mod {
			binding.Handler(gtui.KeyEvent{Key: key, Mod: mod})
			return
		}
	}
	t.Fatalf("permission binding not found for key=%v mod=%v", key, mod)
}

func TestPermissionMouseWheelRoutesByViewport(t *testing.T) {
	state := NewAppState()
	state.DecisionReq.Set(permissionScrollTestRequest())
	messages := make([]Message, 80)
	for i := range messages {
		messages[i] = Message{Kind: MsgAssistant, Text: "transcript message"}
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	root.stickToBottom.Set(false)
	renderPermissionScrollTestRoot(t, root, 80, 32)

	details := root.decisionRef.El()
	transcript := root.contentRef.El()
	_, decisionMax := details.MaxScroll()
	_, transcriptMax := transcript.MaxScroll()
	if decisionMax < 3 || transcriptMax < 3 {
		t.Fatalf("test requires scrollable viewports: decision=%d transcript=%d", decisionMax, transcriptMax)
	}
	detailsRect := details.ContentRect()
	transcriptRect := transcript.ContentRect()

	if !root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseWheelDown, X: detailsRect.X, Y: detailsRect.Y}) {
		t.Fatal("permission wheel was not consumed")
	}
	if got := root.decisionScroll.Get(); got != 3 {
		t.Fatalf("permission wheel offset = %d, want 3", got)
	}
	if got := root.decisionScrollTarget.Get(); got != decisionScrollDetails {
		t.Fatalf("permission wheel focus = %d, want details", got)
	}

	if !root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseWheelDown, X: transcriptRect.X, Y: transcriptRect.Y}) {
		t.Fatal("transcript wheel was not consumed while permission was open")
	}
	if got := root.scrollY.Get(); got != 3 {
		t.Fatalf("transcript wheel offset = %d, want 3", got)
	}
	if got := root.decisionScrollTarget.Get(); got != decisionScrollTranscript {
		t.Fatalf("transcript wheel focus = %d, want transcript", got)
	}
	if got := root.decisionScroll.Get(); got != 3 {
		t.Fatalf("transcript wheel changed permission offset to %d", got)
	}

	if !root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseWheelUp, X: detailsRect.X, Y: detailsRect.Y}) {
		t.Fatal("permission wheel up was not consumed")
	}
	if got := root.decisionScroll.Get(); got != 0 {
		t.Fatalf("permission wheel up offset = %d, want 0", got)
	}
	if got := root.scrollY.Get(); got != 3 {
		t.Fatalf("permission wheel changed transcript offset to %d", got)
	}
}

func TestPermissionKeyboardSeparatesSelectionScrollingAndConfirmation(t *testing.T) {
	state := NewAppState()
	state.DecisionReq.Set(permissionScrollTestRequest())
	messages := make([]Message, 80)
	for i := range messages {
		messages[i] = Message{Kind: MsgAssistant, Text: "transcript message"}
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	root.stickToBottom.Set(false)
	renderPermissionScrollTestRoot(t, root, 80, 32)

	firePermissionScrollBinding(t, root, gtui.KeyDown, gtui.ModNone)
	if got := root.decisionScroll.Get(); got != 1 {
		t.Fatalf("Down permission offset = %d, want 1", got)
	}
	if got := state.DecisionSelected.Get(); got != 0 {
		t.Fatalf("Down changed permission choice to %d", got)
	}
	firePermissionScrollBinding(t, root, gtui.KeyRight, gtui.ModNone)
	if got := state.DecisionSelected.Get(); got != 1 {
		t.Fatalf("Right selected choice = %d, want 1", got)
	}
	if got := root.decisionScroll.Get(); got != 1 {
		t.Fatalf("Right changed permission scroll offset to %d", got)
	}

	firePermissionScrollBinding(t, root, gtui.KeyTab, gtui.ModNone)
	if got := root.decisionScrollTarget.Get(); got != decisionScrollTranscript {
		t.Fatalf("Tab focus = %d, want transcript", got)
	}
	firePermissionScrollBinding(t, root, gtui.KeyPageDown, gtui.ModNone)
	if got := root.scrollY.Get(); got == 0 {
		t.Fatal("PageDown did not scroll transcript while permission was open")
	}
	if got := root.decisionScroll.Get(); got != 1 {
		t.Fatalf("transcript PageDown changed permission offset to %d", got)
	}
	firePermissionScrollBinding(t, root, gtui.KeyEnter, gtui.ModNone)
	select {
	case response := <-state.DecisionResp:
		t.Fatalf("Enter confirmed permission while transcript had focus: %+v", response)
	default:
	}

	firePermissionScrollBinding(t, root, gtui.KeyTab, gtui.ModNone)
	firePermissionScrollBinding(t, root, gtui.KeyEnter, gtui.ModNone)
	select {
	case response := <-state.DecisionResp:
		if response.Choice != "reject" || response.Outcome != permissions.PromptOutcomeRejected {
			t.Fatalf("focused permission confirmation = %+v, want reject", response)
		}
	default:
		t.Fatal("Enter did not confirm while permission details had focus")
	}
}

func TestPermissionKeyboardClampsAndCanScrollTranscriptDirectly(t *testing.T) {
	state := NewAppState()
	state.DecisionReq.Set(permissionScrollTestRequest())
	messages := make([]Message, 80)
	for i := range messages {
		messages[i] = Message{Kind: MsgAssistant, Text: "transcript message"}
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	root.stickToBottom.Set(false)
	renderPermissionScrollTestRoot(t, root, 80, 32)

	_, decisionMax := root.decisionRef.El().MaxScroll()
	firePermissionScrollBinding(t, root, gtui.KeyEnd, gtui.ModNone)
	if got := root.decisionScroll.Get(); got != decisionMax {
		t.Fatalf("permission End offset = %d, want %d", got, decisionMax)
	}
	firePermissionScrollBinding(t, root, gtui.KeyPageDown, gtui.ModNone)
	if got := root.decisionScroll.Get(); got != decisionMax {
		t.Fatalf("permission PageDown escaped max: got %d want %d", got, decisionMax)
	}
	firePermissionScrollBinding(t, root, gtui.KeyHome, gtui.ModNone)
	if got := root.decisionScroll.Get(); got != 0 {
		t.Fatalf("permission Home offset = %d, want 0", got)
	}

	firePermissionScrollBinding(t, root, gtui.KeyPageDown, gtui.ModCtrl)
	if got := root.scrollY.Get(); got == 0 {
		t.Fatal("Ctrl+PageDown did not scroll transcript directly")
	}
	if got := root.decisionScrollTarget.Get(); got != decisionScrollDetails {
		t.Fatalf("direct transcript scroll moved focus to %d", got)
	}
}

func TestPermissionScrollFocusHasDistinctOutlines(t *testing.T) {
	state := NewAppState()
	request := permissionScrollTestRequest()
	state.DecisionReq.Set(request)
	root := NewRootComponent(state, nil, nil)
	root.termWidth, root.termHeight = 80, 32

	detailsFocusedDialog := root.renderDecisionDialog(request)
	if detailsFocusedDialog.BorderStyle().Fg.Equal(gtui.BrightBlack) {
		t.Fatal("focused permission dialog used the inactive outline")
	}
	root.setDecisionScrollTarget(decisionScrollTranscript)
	transcript := root.renderMessageArea(8)
	dialog := root.renderDecisionDialog(request)
	if transcript.Border() != gtui.BorderSingle || !transcript.BorderStyle().Fg.Equal(gtui.Cyan) {
		t.Fatalf("focused transcript outline = border:%v style:%+v", transcript.Border(), transcript.BorderStyle())
	}
	if !dialog.BorderStyle().Fg.Equal(gtui.BrightBlack) {
		t.Fatalf("inactive permission outline = %+v, want bright black", dialog.BorderStyle())
	}
}

func TestPermissionNarrowViewportWrapsCompleteIdentifiersAndPath(t *testing.T) {
	state := NewAppState()
	request := permissionScrollTestRequest()
	state.DecisionReq.Set(request)
	root := NewRootComponent(state, nil, nil)
	element := renderPermissionScrollTestRoot(t, root, 40, 12)

	linear := collectElementText(element)
	for label, complete := range map[string]string{
		"actor":             request.ActorID,
		"work unit":         request.WorkUnitID,
		"execution session": request.ExecutionSessionID,
		"target path":       request.Target,
	} {
		if !strings.Contains(linear, complete) {
			t.Fatalf("narrow permission dialog lost complete %s value", label)
		}
	}
	details := root.decisionRef.El()
	_, maxY := details.MaxScroll()
	if maxY == 0 {
		t.Fatal("narrow permission details did not expose wrapped overflow through scrolling")
	}
	if details.Rect().Right() > 40 {
		t.Fatalf("permission details escaped narrow viewport: %+v", details.Rect())
	}
	for _, child := range details.Children() {
		if child.Rect().Width > details.ContentRect().Width {
			t.Fatalf("permission detail did not wrap to viewport: child=%+v viewport=%+v", child.Rect(), details.ContentRect())
		}
	}
}
