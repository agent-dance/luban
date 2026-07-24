package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
	gtui "github.com/grindlemire/go-tui"
)

func TestDecisionLayoutMatrixAndLinearSemantics(t *testing.T) {
	request := &DecisionRequest{
		DecisionID: "decision-cjk", ActorID: "后台-agent", ActorType: "executor", WorkUnitID: "验证单元",
		Kind: permissions.PromptKindPermission, Action: "写入配置", Target: "/非常/长的/路径/" + strings.Repeat("目录/", 30) + "配置.json",
		Impact: "替换现有内容", RiskLevel: 3, RiskReason: "数据可能被覆盖",
		RuleSource: "项目安全规则", ApprovalScope: "仅本次调用",
		Choices: []string{"allow_once", "reject", "always_allow"},
	}
	for _, size := range []struct{ width, height int }{{40, 12}, {80, 24}, {120, 40}} {
		state := NewAppState()
		root := NewRootComponent(state, nil, nil)
		root.termWidth, root.termHeight = size.width, size.height
		dialog := root.renderDecisionDialog(request)
		buffer := gtui.NewBuffer(size.width, size.height)
		dialog.Render(buffer, size.width, size.height)
		rect := dialog.Rect()
		if rect.Width > size.width || rect.Height > size.height {
			t.Fatalf("%dx%d decision rect overflows terminal: %+v", size.width, size.height, rect)
		}
		text := collectElementText(dialog)
		for _, semantic := range []string{"Actor:", "Action:", "Target:", "Impact:", "Risk:", "Scope:", "Rule:", "Allow once", "Reject", "Always allow"} {
			if !strings.Contains(text, semantic) {
				t.Fatalf("%dx%d decision relies on color/layout for %q: %s", size.width, size.height, semantic, text)
			}
		}
	}
}

func TestDecisionReceiptKeepsOutcomeVisibleAtFortyColumns(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	receipt := formatDecisionReceipt(permissions.PromptRequest{Action: "write " + strings.Repeat("very-long-target/", 20)}, permissions.PromptResponse{Outcome: permissions.PromptOutcomeTimedOut})
	element := root.renderDecisionReceipt(receipt)
	buffer := gtui.NewBuffer(40, 12)
	element.Render(buffer, 40, 12)
	if rendered := buffer.String(); !strings.Contains(rendered, i18n.RuntimeDecisionOutcomeLabel(i18n.LangEN, string(permissions.PromptOutcomeTimedOut))) {
		t.Fatalf("40-column decision receipt hid outcome: %q", rendered)
	}
}

func TestActivitySummaryDoesNotRelyOnAnimationOrColor(t *testing.T) {
	state := NewAppState()
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	for _, event := range []ActivityEvent{
		{ID: "tool:1", SessionID: "session", Epoch: 1, Kind: ActivityTool, Phase: ActivityPhaseVerifying, State: ActivityRunning},
		{ID: "agent:2", SessionID: "session", Epoch: 1, Kind: ActivityAgent, Phase: ActivityPhaseVerifying, State: ActivityNeedsInput},
	} {
		if err := state.Activities.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	text := NewRootComponent(state, nil, nil).activityStatusText()
	for _, want := range []string{"Verifying", "2 activities", "1 needs input"} {
		if !strings.Contains(text, want) {
			t.Fatalf("activity summary omitted semantic %q: %q", want, text)
		}
	}
}

func TestProductionToolActivityEmitsAndPrioritizesVerifyingPhase(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	ctx := ToolEventContext{SessionID: "session", TurnID: "session:turn-1", ActorID: "assistant", WorkUnitID: "work"}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "execute", Name: "Bash", Input: map[string]any{"command": "echo ready"}}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "verify", Name: "Bash", Input: map[string]any{"command": "go test ./..."}}); err != nil {
		t.Fatal(err)
	}
	first, _ := state.GetActivity("tool:execute")
	second, _ := state.GetActivity("tool:verify")
	if first.Phase != ActivityPhaseExecuting || second.Phase != ActivityPhaseVerifying {
		t.Fatalf("production phases = execute %q verify %q", first.Phase, second.Phase)
	}
	if status := NewRootComponent(state, nil, nil).activityStatusText(); !strings.HasPrefix(status, "Verifying") {
		t.Fatalf("verifying activity was hidden by executing sibling: %q", status)
	}
}

func TestAltOTogglesGlobalShowAllWithoutFocusedObservation(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	found := false
	for _, binding := range root.KeyMap() {
		if binding.Pattern.Rune == 'o' && binding.Pattern.Mod == gtui.ModAlt {
			binding.Handler(gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'o', Mod: gtui.ModAlt})
			found = true
			break
		}
	}
	if !found || !state.TranscriptShowAll.Get() {
		t.Fatalf("Alt+O show-all binding found=%v enabled=%v", found, state.TranscriptShowAll.Get())
	}
}

func TestActivitySummaryShowsOnlyActionableTerminalAttention(t *testing.T) {
	state := NewAppState()
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	for _, event := range []ActivityEvent{
		{ID: "denied", SessionID: "session", Epoch: 1, State: ActivityFailed, Outcome: OutcomeDenied},
		{ID: "partial", SessionID: "session", Epoch: 1, State: ActivityFailed, Outcome: OutcomePartial},
		{ID: "cancelled", SessionID: "session", Epoch: 1, State: ActivityCancelled, Outcome: OutcomeCancelled},
		{ID: "timeout", SessionID: "session", Epoch: 1, State: ActivityCancelled, Outcome: OutcomeTimedOut},
	} {
		if err := state.Activities.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	text := NewRootComponent(state, nil, nil).activityStatusText()
	for _, want := range []string{"1 activities", "1 timed out"} {
		if !strings.Contains(text, want) {
			t.Fatalf("activity summary swallowed %q: %q", want, text)
		}
	}
}

func TestActivitySummaryHidesNonActionableTerminalHistory(t *testing.T) {
	state := NewAppState()
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	if err := state.Activities.Apply(ActivityEvent{
		ID: "failed", SessionID: "session", Epoch: 1,
		State: ActivityFailed, Outcome: OutcomeFailed,
	}); err != nil {
		t.Fatal(err)
	}

	text := NewRootComponent(state, nil, nil).activityStatusText()
	if text != "" {
		t.Fatalf("non-actionable historical failure leaked into footer: %q", text)
	}
}

func TestActivitySummaryCountsCorrelatedAgentOnceAndNamesPendingResultOnce(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	for _, event := range []ActivityEvent{
		{
			ID: "tool:agent-call", SessionID: "session", Epoch: 1, Kind: ActivityAgent,
			State: ActivityCompleted, Outcome: OutcomeSucceeded,
		},
		{
			ID: "background:agent-1", SessionID: "session", Epoch: 1, Kind: ActivityAgent,
			State: ActivityReadyReview, Outcome: OutcomeSucceeded,
			Progress: ActivityProgress{AgentID: "agent-1", ParentToolUseID: "agent-call"},
		},
	} {
		if err := state.Activities.Apply(event); err != nil {
			t.Fatal(err)
		}
	}

	text := NewRootComponent(state, nil, nil).activityStatusText()
	for _, want := range []string{
		i18n.Text(i18n.LangEN, i18n.KeyActivityHistory),
		i18n.Format(i18n.LangEN, i18n.KeyActivitySummary, i18n.Text(i18n.LangEN, i18n.KeyActivityHistory), 1),
		i18n.Format(i18n.LangEN, i18n.KeyActivityAgents, 1),
		i18n.Format(i18n.LangEN, i18n.KeyActivityResultsPendingView, 1),
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("logical Agent summary omitted %q: %q", want, text)
		}
	}
	for _, forbidden := range []string{
		i18n.Text(i18n.LangEN, i18n.KeyActivityWorking),
		i18n.Format(i18n.LangEN, i18n.KeyActivityAgents, 2),
		i18n.Format(i18n.LangEN, i18n.KeyActivityReadyReview, 1),
		i18n.Format(i18n.LangEN, i18n.KeyActivityCompleted, 1),
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("logical Agent summary retained duplicate/misleading %q: %q", forbidden, text)
		}
	}
	pending := i18n.Format(i18n.LangEN, i18n.KeyActivityResultsPendingView, 1)
	if strings.Count(text, pending) != 1 {
		t.Fatalf("pending result appeared %d times: %q", strings.Count(text, pending), text)
	}
}

func TestTranscriptLinearTextIncludesPublicOutcomesWithoutInternalIdentity(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.AppendMessage(Message{Kind: MsgUser, Text: "inspect configuration"})
	state.AppendMessage(Message{Kind: MsgAssistant, Text: "Checking the requested file."})
	ctx := ToolEventContext{SessionID: "session", TurnID: "session:turn-1", WorkUnitID: "work-1", ActorID: "agent-1"}
	state.ApplyToolCall(ctx, types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tool-1", Name: "Read", Input: map[string]any{"file_path": "/restricted/config"}})
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "tool-1", Content: "policy denied access", IsError: true, Outcome: types.ToolOutcomeDenied}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyRuntimeError(ctx, "tool-1", "runtime transport failed", nil, map[string]any{"attempt": 1}); err != nil {
		t.Fatal(err)
	}

	linear := collectElementText(NewRootComponent(state, nil, nil).renderMessageArea(24))
	for _, semantic := range []string{"You: inspect configuration", "Checking the requested file.", "Read", "denied", "policy denied access", "Error: " + i18n.Text(state.Language.Get(), i18n.KeyRuntimeErrorPublicSummary)} {
		if !strings.Contains(linear, semantic) {
			t.Fatalf("linear transcript omitted %q: %s", semantic, linear)
		}
	}
	for _, internal := range []string{"agent-1", "work-1", "runtime transport failed"} {
		if strings.Contains(linear, internal) {
			t.Fatalf("linear transcript exposed internal runtime detail %q: %s", internal, linear)
		}
	}
}

func TestExpandedActivityViewListsIndependentWorkAndActions(t *testing.T) {
	state := NewAppState()
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	for _, event := range []ActivityEvent{
		{ID: "tool:one", SessionID: "session", Epoch: 1, WorkUnitID: "work-1", Actor: ActivityActor{ID: "agent-1", Type: "agent"}, Kind: ActivityTool, Name: "Read", State: ActivityRunning, Control: ActivityControl{Cancelable: true, JumpTarget: "obs-1"}},
		{ID: "tool:two", SessionID: "session", Epoch: 1, WorkUnitID: "work-2", Actor: ActivityActor{ID: "agent-2", Type: "agent"}, Kind: ActivityMCP, Name: "Search", State: ActivityFailed, Control: ActivityControl{JumpTarget: "obs-2", DetailRefs: []DetailRef{{Source: "memory", Key: "e", Size: 1, Digest: strings.Repeat("0", 64)}}}},
	} {
		if err := state.Activities.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	text := collectElementText(NewRootComponent(state, nil, nil).renderActivityView(state.Activities.Snapshot(), 5))
	for _, want := range []string{"tool:one", "work-1", "agent-1", "cancel", "jump", "tool:two", "work-2", "agent-2", "details"} {
		if !strings.Contains(text, want) {
			t.Fatalf("activity view omitted %q: %s", want, text)
		}
	}
}

func TestExpandedActivityViewRendersProgressAtWideAndFortyColumns(t *testing.T) {
	state := NewAppState()
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	if err := state.Activities.Apply(ActivityEvent{
		ID: "background:acceptance-tests", SessionID: "session", Epoch: 1,
		WorkUnitID: "acceptance", Actor: ActivityActor{ID: "verifier"}, Kind: ActivityBackground,
		Name: "Tests", State: ActivityRunning, Outcome: OutcomeRunning,
		Progress: ActivityProgress{Current: 47, Total: 100, Message: "running tests"},
	}); err != nil {
		t.Fatal(err)
	}

	wideRoot := NewRootComponent(state, nil, nil)
	wideRoot.termWidth = 120
	wide := collectElementText(wideRoot.renderActivityView(state.Activities.Snapshot(), 5))
	for _, want := range []string{"progress=47/100", "running tests"} {
		if !strings.Contains(wide, want) {
			t.Fatalf("wide activity view omitted %q: %s", want, wide)
		}
	}

	narrowRoot := NewRootComponent(state, nil, nil)
	narrowRoot.termWidth = 40
	narrow := renderElementText(narrowRoot.renderActivityView(state.Activities.Snapshot(), 5), 40, 5)
	for _, want := range []string{"running", "47/100", "running tests"} {
		if !strings.Contains(narrow, want) {
			t.Fatalf("40-column activity view omitted %q:\n%s", want, narrow)
		}
	}
}

func TestExpandedActivityViewGroupsByWorkUnitThenActor(t *testing.T) {
	state := NewAppState()
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	for _, event := range []ActivityEvent{
		{ID: "work-a-agent-a-second", SessionID: "session", Epoch: 1, WorkUnitID: "work-a", Actor: ActivityActor{ID: "agent-a"}, Kind: ActivityTool, Name: "second", State: ActivityRunning},
		{ID: "work-b-agent-c", SessionID: "session", Epoch: 1, WorkUnitID: "work-b", Actor: ActivityActor{ID: "agent-c"}, Kind: ActivityTool, Name: "third", State: ActivityRunning},
		{ID: "work-a-agent-b", SessionID: "session", Epoch: 1, WorkUnitID: "work-a", Actor: ActivityActor{ID: "agent-b"}, Kind: ActivityTool, Name: "fourth", State: ActivityRunning},
		{ID: "work-a-agent-a-first", SessionID: "session", Epoch: 1, WorkUnitID: "work-a", Actor: ActivityActor{ID: "agent-a"}, Kind: ActivityTool, Name: "first", State: ActivityRunning},
	} {
		if err := state.Activities.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	text := collectElementText(NewRootComponent(state, nil, nil).renderActivityView(state.Activities.Snapshot(), 8))
	if strings.Count(text, "work=work-a") != 1 || strings.Count(text, "actor=agent-a") != 1 {
		t.Fatalf("group headings should be emitted once per contiguous group:\n%s", text)
	}
	positions := []int{
		strings.Index(text, "work-a-agent-a-first"),
		strings.Index(text, "work-a-agent-a-second"),
		strings.Index(text, "work-a-agent-b"),
		strings.Index(text, "work-b-agent-c"),
	}
	for index, position := range positions {
		if position < 0 || index > 0 && position <= positions[index-1] {
			t.Fatalf("activity groups are not work-unit/actor ordered: positions=%v\n%s", positions, text)
		}
	}
}

func TestActivityOverlayKeyboardActionsBypassFocusedComposer(t *testing.T) {
	state := NewAppState()
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	ref := DetailRef{Source: "memory", Key: "detail", Size: 1, Digest: strings.Repeat("d", 64)}
	for _, event := range []ActivityEvent{
		{ID: "first", SessionID: "session", Epoch: 1, State: ActivityRunning, Control: ActivityControl{Cancelable: true, JumpTarget: "obs-first"}},
		{ID: "second", SessionID: "session", Epoch: 1, State: ActivityFailed, Outcome: OutcomeFailed, Control: ActivityControl{JumpTarget: "obs-second", DetailRefs: []DetailRef{ref}}},
	} {
		if err := state.Activities.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	state.ExpandedView.Set("activities")
	state.ActivityFocus.Set("second")
	root := NewRootComponent(state, nil, nil)
	var gotID string
	var gotAction ActivityAction
	root.onActivityAction = func(id string, action ActivityAction) { gotID, gotAction = id, action }
	if root.input.KeyMap() != nil {
		t.Fatal("focused composer retained stop handlers while activity overlay was open")
	}
	fireRootBinding(t, root, gtui.KeyEnter, 0)
	if gotID != "second" || gotAction != ActivityDetails {
		t.Fatalf("Enter action = %s/%s, want second/details", gotID, gotAction)
	}
	fireRootBinding(t, root, 0, 'g')
	if gotID != "second" || gotAction != ActivityJump {
		t.Fatalf("g action = %s/%s, want second/jump", gotID, gotAction)
	}
	fireRootBinding(t, root, gtui.KeyDown, 0)
	if focused := state.ActivityFocus.Get(); focused != "first" {
		t.Fatalf("Down focused %q, want first", focused)
	}
	fireRootBinding(t, root, 0, 'c')
	if gotID != "first" || gotAction != ActivityCancel {
		t.Fatalf("c action = %s/%s, want first/cancel", gotID, gotAction)
	}
}

func TestActivityActionRunesRemainComposerInputOutsideOverlay(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	for _, binding := range root.KeyMap() {
		if binding.Pattern.Mod == 0 && (binding.Pattern.Rune == 'c' || binding.Pattern.Rune == 'd' || binding.Pattern.Rune == 'g') && binding.Preempt && binding.Stop {
			t.Fatalf("plain %q was stolen from the focused composer outside activity view", binding.Pattern.Rune)
		}
	}
}

func TestActivityViewHeightIncludesBorderAndLastVisibleRow(t *testing.T) {
	state := NewAppState()
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	for index := range 7 {
		if err := state.Activities.Apply(ActivityEvent{ID: fmt.Sprintf("activity-%d", index), SessionID: "session", Epoch: 1, State: ActivityRunning}); err != nil {
			t.Fatal(err)
		}
	}
	view := NewRootComponent(state, nil, nil).renderActivityView(state.Activities.Snapshot(), 10)
	buffer := gtui.NewBuffer(80, 10)
	view.Render(buffer, 80, 10)
	if rendered := buffer.String(); !strings.Contains(rendered, "activity-6") {
		t.Fatalf("last visible activity was clipped by border accounting:\n%s", rendered)
	}
}

func fireRootBinding(t *testing.T, root *RootComponent, key gtui.Key, r rune) {
	t.Helper()
	for _, binding := range root.KeyMap() {
		if r != 0 && binding.Pattern.Rune == r && binding.Pattern.Mod == 0 {
			binding.Handler(gtui.KeyEvent{Key: gtui.KeyRune, Rune: r})
			return
		}
		if r == 0 && binding.Pattern.Key == key && binding.Pattern.Mod == 0 {
			binding.Handler(gtui.KeyEvent{Key: key})
			return
		}
	}
	t.Fatalf("missing root binding key=%v rune=%q", key, r)
}

func TestFullRootViewportMatrixKeepsModeDecisionAndInputVisible(t *testing.T) {
	for _, size := range []struct{ width, height int }{{40, 12}, {80, 24}, {120, 40}} {
		state := NewAppState()
		state.Provider.Set("anthropic")
		state.Model.Set("模型-" + strings.Repeat("很长", 20))
		state.SessionID.Set("会话-" + strings.Repeat("路径", 20))
		state.Mode.Set(ModePlanEdit)
		state.Messages.Set([]Message{{Kind: MsgUser, Text: "用户消息"}, {Kind: MsgAssistant, Text: "助手回答"}})
		request := &DecisionRequest{DecisionID: "matrix", ActorID: "agent", Kind: permissions.PromptKindPermission,
			Action: "写入", Target: "/长路径/" + strings.Repeat("目录/", 30), Impact: "修改文件", RiskReason: "覆盖风险", ApprovalScope: "本次", RuleSource: "规则", Choices: []string{"allow_once", "reject"}}
		state.DecisionReq.Set(request)
		root := NewRootComponent(state, nil, nil)
		root.termWidth, root.termHeight = size.width, size.height
		root.inputText.Set("待发送草稿")
		element := root.renderAtSize(nil, size.width, size.height)
		buffer := gtui.NewBuffer(size.width, size.height)
		element.Render(buffer, size.width, size.height)
		assertElementTreeWithinViewport(t, element, size.width, size.height)
		if rect := element.Rect(); rect.Width > size.width || rect.Height > size.height {
			t.Fatalf("%dx%d root overflows viewport: %+v", size.width, size.height, rect)
		}
		linear := collectElementText(element)
		for _, want := range []string{"plan>", "Actor:", "Action:", "待发送草稿"} {
			if !strings.Contains(linear, want) {
				t.Fatalf("%dx%d full root omitted %q: %s", size.width, size.height, want, linear)
			}
		}
		rendered := buffer.String()
		for _, want := range []string{"Allow once", "plan>", "待发送草稿"} {
			if !strings.Contains(rendered, want) {
				t.Fatalf("%dx%d final buffer omitted %q:\n%s", size.width, size.height, want, rendered)
			}
		}
	}
}

func TestDynamicResizePreservesEmojiCombiningAndAmbiguousWidthSemantics(t *testing.T) {
	state := NewAppState()
	state.Provider.Set("provider-Ω")
	state.Model.Set("model-👩🏽‍💻-e\u0301")
	state.SessionID.Set("session-中-·")
	state.Mode.Set(ModePlanEdit)
	state.Messages.Set([]Message{{Kind: MsgAssistant, Text: "status 👨‍👩‍👧‍👦 cafe\u0301 Ω 中"}})
	state.DecisionReq.Set(&DecisionRequest{
		DecisionID: "unicode-resize", ActorID: "agent-👩🏽‍💻-e\u0301", Kind: permissions.PromptKindPermission,
		Action: "write Ω 中", Target: "/workspace/👨‍👩‍👧‍👦/cafe\u0301/" + strings.Repeat("目录/", 24),
		Impact: "replace · existing content", RiskReason: "覆盖风险 ⚠️", ApprovalScope: "once", RuleSource: "policy",
		Choices: []string{"allow_once", "reject"},
	})

	root := NewRootComponent(state, nil, nil)
	root.inputText.Set("draft 👩🏽‍💻 e\u0301 Ω")
	for _, size := range []struct{ width, height int }{{120, 40}, {40, 12}, {80, 24}, {40, 12}} {
		element := root.renderAtSize(nil, size.width, size.height)
		buffer := gtui.NewBuffer(size.width, size.height)
		element.Render(buffer, size.width, size.height)
		assertElementTreeWithinViewport(t, element, size.width, size.height)
		if rect := element.Rect(); rect.Width > size.width || rect.Height > size.height {
			t.Fatalf("resize to %dx%d overflowed viewport: %+v", size.width, size.height, rect)
		}
		linear := collectElementText(element)
		for _, want := range []string{"Actor:", "Action:", "plan>", "draft 👩🏽‍💻 e\u0301 Ω"} {
			if !strings.Contains(linear, want) {
				t.Fatalf("resize to %dx%d lost %q: %s", size.width, size.height, want, linear)
			}
		}
	}
}

func assertElementTreeWithinViewport(t *testing.T, element *gtui.Element, width, height int) {
	t.Helper()
	rect := element.Rect()
	if rect.X < 0 || rect.Y < 0 || rect.X+rect.Width > width || rect.Y+rect.Height > height {
		t.Fatalf("element rect escapes %dx%d viewport: %+v", width, height, rect)
	}
	for _, child := range element.Children() {
		assertElementTreeWithinViewport(t, child, width, height)
	}
}

func TestActivityAgentSignalUsesLatestRunAndAttentionPriority(t *testing.T) {
	activities := []Activity{
		{ActivityEvent: ActivityEvent{ID: "agent-a", RunID: "run-1", Attempt: 1, Kind: ActivityAgent, Lifecycle: ActivityLifecycleCompleted}, FirstSequence: 1},
		{ActivityEvent: ActivityEvent{ID: "agent-a", RunID: "run-2", Attempt: 2, Kind: ActivityAgent, Lifecycle: ActivityLifecycleRunning}, FirstSequence: 2},
		{ActivityEvent: ActivityEvent{ID: "agent-b", RunID: "run-b", Attempt: 1, Kind: ActivityAgent, Lifecycle: ActivityLifecycleBlocked,
			Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Unread: true}}, FirstSequence: 3},
		{ActivityEvent: ActivityEvent{ID: "agent-c", RunID: "run-c", Attempt: 1, Kind: ActivityAgent, Lifecycle: ActivityLifecycleFailed}, FirstSequence: 4},
	}
	got := activityAgentSignal(i18n.LangEN, activities)
	for _, want := range []string{"3 agents", "1 needs input", "1 failed", "1 running"} {
		if !strings.Contains(got, want) {
			t.Fatalf("signal %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "completed") {
		t.Fatalf("older completed run leaked into latest-agent signal: %q", got)
	}
}

func TestActivityAgentDescendantSummaryUsesPathAndWorstState(t *testing.T) {
	parent := Activity{ActivityEvent: ActivityEvent{ID: "parent", Kind: ActivityAgent, AgentPath: "lead", State: ActivityRunning, Lifecycle: ActivityLifecycleRunning}}
	activities := []Activity{
		parent,
		{ActivityEvent: ActivityEvent{ID: "child-a", Kind: ActivityAgent, AgentPath: "lead/a", State: ActivityCompleted, Lifecycle: ActivityLifecycleCompleted}},
		{ActivityEvent: ActivityEvent{ID: "child-b", Kind: ActivityAgent, AgentPath: "lead/b", State: ActivityNeedsInput, Lifecycle: ActivityLifecycleBlocked, Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Unread: true}}},
		{ActivityEvent: ActivityEvent{ID: "unrelated", Kind: ActivityAgent, AgentPath: "other/c", State: ActivityFailed, Lifecycle: ActivityLifecycleFailed}},
	}
	got := activityAgentDescendantSummary(i18n.LangEN, parent, activities)
	if !strings.Contains(got, "descendants=2") || !strings.Contains(got, "worst=needs input") {
		t.Fatalf("descendant summary=%q", got)
	}
}
