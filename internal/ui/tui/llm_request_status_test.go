package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/contracts/stream"

	"github.com/agent-dance/luban/i18n"
	gtui "github.com/grindlemire/go-tui"
)

func TestLLMRequestStatusShimmersWorkingAndDimsInterruptHintAndCallMetrics(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Language.Set(i18n.LangEN)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}

	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestStart, stream.RequestStatusEvent{
		RequestID: "request-1", RequestMilliseconds: 70_000, TotalMilliseconds: 70_000,
	})
	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestFirstToken, stream.RequestStatusEvent{
		RequestID: "request-1", FirstTokenMilliseconds: 130_000, TotalMilliseconds: 130_000,
	})
	status := state.LLMCall.Get()
	if status == nil {
		t.Fatal("LLM call status was not published")
	}
	root := NewRootComponent(state, nil, nil)
	now := time.Unix(100, 0)
	status.WorkStartedAt = now.Add(-(time.Hour + time.Minute + time.Second))
	root.now = func() time.Time { return now }
	element := root.renderLLMStatus(status)
	text := collectElementText(element)
	for _, want := range []string{"• Working", "(1h1m1s • Ctrl+C to interrupt)", "Connection 1m10s", "First token 2m10s"} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLM request status omitted %q: %q", want, text)
		}
	}
	for _, forbidden := range []string{"activities", "agents", "Agent", "Total"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("LLM request status retained activity detail %q: %q", forbidden, text)
		}
	}

	buffer := gtui.NewBuffer(120, 1)
	element.Render(buffer, 120, 1)
	if rendered := buffer.String(); !strings.Contains(rendered, "• Working (1h1m1s • Ctrl+C to interrupt)  Connection 1m10s · First token 2m10s") {
		t.Fatalf("LLM request status layout = %q", rendered)
	}
	hintColumn := strings.Index(buffer.String(), "Ctrl+C to interrupt")
	if hintColumn < 0 || !buffer.Cell(hintColumn, 0).Style.HasAttr(gtui.AttrDim) {
		t.Fatalf("interrupt hint is not rendered dim: %q", buffer.String())
	}
	connectionColumn := strings.Index(buffer.String(), "Connection")
	if connectionColumn < 0 || !buffer.Cell(connectionColumn, 0).Style.HasAttr(gtui.AttrDim) {
		t.Fatalf("call metrics are not rendered dim: %q", buffer.String())
	}
}

func TestLLMWorkingShimmerMovesLeftToRight(t *testing.T) {
	runes := []rune("Working")
	left := llmWorkingShimmerSpansAtPosition(runes, llmWorkingShimmerPadding)
	right := llmWorkingShimmerSpansAtPosition(runes, llmWorkingShimmerPadding+len(runes)-1)

	if len(left) != len(runes) || len(right) != len(runes) {
		t.Fatalf("shimmer span counts = %d/%d, want %d", len(left), len(right), len(runes))
	}
	last := len(runes) - 1
	if !left[0].Style.HasAttr(gtui.AttrBold) || !left[last].Style.HasAttr(gtui.AttrDim) {
		t.Fatalf("left shimmer frame styles = first %v, last %v", left[0].Style, left[last].Style)
	}
	if !left[0].Style.Fg.IsDefault() || !left[last].Style.Fg.IsDefault() {
		t.Fatalf("fallback shimmer forced a foreground color: first=%v last=%v", left[0].Style.Fg, left[last].Style.Fg)
	}
	if !right[0].Style.HasAttr(gtui.AttrDim) || !right[last].Style.HasAttr(gtui.AttrBold) {
		t.Fatalf("right shimmer frame styles = first %v, last %v", right[0].Style, right[last].Style)
	}
	for index, character := range runes {
		if left[index].Text != string(character) || right[index].Text != string(character) {
			t.Fatalf("shimmer changed character %d: left=%q right=%q want=%q", index, left[index].Text, right[index].Text, string(character))
		}
	}
}

func TestLLMWorkingShimmerRefreshRateDoesNotExceedTenFPS(t *testing.T) {
	const maximumFramesPerSecond = 10
	minimumInterval := time.Second / maximumFramesPerSecond
	if llmWorkingShimmerFrameInterval < minimumInterval {
		t.Fatalf("shimmer interval %v exceeds %d frames per second", llmWorkingShimmerFrameInterval, maximumFramesPerSecond)
	}
}

func TestLLMWorkingShimmerTrueColorBlendsForegroundTowardBackground(t *testing.T) {
	runes := []rune("Working")
	palette := llmWorkingShimmerPalette{
		base:      [3]uint8{200, 210, 220},
		highlight: [3]uint8{0, 0, 0},
	}
	spans := llmWorkingShimmerSpansAtPositionWithPalette(
		runes, llmWorkingShimmerPadding, true, palette)
	last := len(spans) - 1

	centerR, centerG, centerB := spans[0].Style.Fg.RGB()
	outsideR, outsideG, outsideB := spans[last].Style.Fg.RGB()
	if centerR > 20 || centerG > 21 || centerB > 22 {
		t.Fatalf("shimmer center did not blend toward background: rgb(%d,%d,%d)", centerR, centerG, centerB)
	}
	if outsideR != 200 || outsideG != 210 || outsideB != 220 {
		t.Fatalf("shimmer edge changed base foreground: rgb(%d,%d,%d)", outsideR, outsideG, outsideB)
	}
	for index, span := range spans {
		if span.Style.Fg.Type() != gtui.ColorRGB || !span.Style.HasAttr(gtui.AttrBold) || span.Style.HasAttr(gtui.AttrDim) {
			t.Fatalf("true-color shimmer span %d style = %v", index, span.Style)
		}
	}
}

func TestLLMWorkingShimmerDoesNotBleedIntoInterruptParentheses(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("COLORFGBG", "15;0")

	state := NewAppState()
	state.Language.Set(i18n.LangZH)
	status := &LLMCallStatus{
		Phase:         LLMCallWorking,
		WorkStartedAt: time.Unix(95, 0),
	}
	root := NewRootComponent(state, nil, nil)
	root.now = func() time.Time { return time.Unix(100, 0) }
	element := root.renderLLMStatus(status)
	buffer := gtui.NewBuffer(120, 1)
	element.Render(buffer, 120, 1)

	parentheses := make([]gtui.Cell, 0, 2)
	for x := 0; x < buffer.Width(); x++ {
		cell := buffer.Cell(x, 0)
		if cell.Rune == '(' || cell.Rune == ')' {
			parentheses = append(parentheses, cell)
		}
	}
	if len(parentheses) != 2 {
		t.Fatalf("rendered interrupt status has %d parentheses, want 2: %q", len(parentheses), buffer.String())
	}
	left, right := parentheses[0].Style, parentheses[1].Style
	if !left.Equal(right) {
		t.Fatalf("interrupt parenthesis styles differ: left=%v right=%v", left, right)
	}
	if !left.Fg.IsDefault() || !left.HasAttr(gtui.AttrDim) || left.HasAttr(gtui.AttrBold) {
		t.Fatalf("interrupt parentheses inherited shimmer style: %v", left)
	}
}

func TestFormatLLMStatusDurationUsesCompactUnits(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{name: "milliseconds", duration: 800 * time.Millisecond, want: "800ms"},
		{name: "fractional seconds", duration: 5200 * time.Millisecond, want: "5.2s"},
		{name: "minutes", duration: 70 * time.Second, want: "1m10s"},
		{name: "hours", duration: time.Hour + 2*time.Minute + 3*time.Second, want: "1h2m3s"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := formatLLMStatusDuration(test.duration); got != test.want {
				t.Fatalf("formatLLMStatusDuration(%s) = %q, want %q", test.duration, got, test.want)
			}
		})
	}
}

func TestBeginLLMWorkIsVisibleBeforeProviderRequestStarts(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangZH)
	state.BeginLLMWork()
	status := state.LLMCall.Get()
	if status == nil || status.Phase != LLMCallWorking || status.WorkStartedAt.IsZero() || status.HasRequestDuration {
		t.Fatalf("initial submitted-message status = %+v", status)
	}
	root := NewRootComponent(state, nil, nil)
	now := time.Unix(100, 0)
	status.WorkStartedAt = now.Add(-5200 * time.Millisecond)
	status.StageStartedAt = status.WorkStartedAt
	root.now = func() time.Time { return now }
	element := root.renderLLMStatus(status)
	text := collectElementText(element)
	for _, want := range []string{"• 准备模型请求", "阶段 5.2s", "(5.2s • Ctrl+C 中断)", "建立连接 —", "首 token —"} {
		if !strings.Contains(text, want) {
			t.Fatalf("pre-provider working status omitted %q: %q", want, text)
		}
	}
	if strings.Contains(text, "总耗时") {
		t.Fatalf("working status retained duplicate total-duration metric: %q", text)
	}
}

func TestLLMActivityStageShowsElapsedTimeAndHonestSlowHint(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangZH)
	started := time.Unix(100, 0)
	state.SetLLMCall(&LLMCallStatus{
		Phase: LLMCallWorking, Stage: LLMStageToolInput, StageDetail: "ApplyPatch",
		StageStartedAt: started, WorkStartedAt: started.Add(-5 * time.Second),
	})
	root := NewRootComponent(state, nil, nil)
	root.now = func() time.Time { return started.Add(45 * time.Second) }
	text := collectElementText(root.renderLLMStatus(state.LLMCall.Get()))
	for _, want := range []string{"正在生成 ApplyPatch 的工具输入", "阶段 45.0s", "此阶段耗时较长", "Ctrl+C 中断"} {
		if !strings.Contains(text, want) {
			t.Fatalf("long tool-input stage omitted %q: %q", want, text)
		}
	}
	if strings.Contains(text, "%") {
		t.Fatalf("indeterminate model activity invented a percentage: %q", text)
	}
}

func TestLLMToolInputActivityShowsOnlyObservedReceivedBytes(t *testing.T) {
	for _, test := range []struct {
		name  string
		bytes int
		want  string
	}{
		{name: "one byte", bytes: 1, want: "已接收工具输入 1 B"},
		{name: "ten kibibytes", bytes: 10 * 1024, want: "已接收工具输入 10.0 KiB"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := NewAppState()
			state.Language.Set(i18n.LangZH)
			started := time.Unix(100, 0)
			state.SetLLMCall(&LLMCallStatus{
				Phase: LLMCallWorking, Stage: LLMStageToolInput, StageDetail: "ApplyPatch",
				ToolInputBytes: test.bytes, StageStartedAt: started, WorkStartedAt: started,
			})
			text := collectElementText(NewRootComponent(state, nil, nil).renderLLMStatus(state.LLMCall.Get()))
			if !strings.Contains(text, test.want) {
				t.Fatalf("tool-input status omitted %q: %q", test.want, text)
			}
			if strings.Contains(text, "%") {
				t.Fatalf("tool-input status invented completion percentage: %q", text)
			}
		})
	}
}

func TestLLMToolInputByteUpdatesDoNotResetStageElapsedTime(t *testing.T) {
	state := NewAppState()
	started := time.Unix(100, 0)
	state.SetLLMCall(&LLMCallStatus{
		Phase: LLMCallWorking, Stage: LLMStageToolInput, StageDetail: "ApplyPatch",
		ToolInputBytes: 1, StageStartedAt: started,
	})
	if !state.setLLMActivityAt(LLMStageToolInput, "ApplyPatch", started.Add(10*time.Second), 10*1024) {
		t.Fatal("larger observed byte count did not update activity state")
	}
	got := state.LLMCall.Get()
	if got.ToolInputBytes != 10*1024 || !got.StageStartedAt.Equal(started) {
		t.Fatalf("byte update reset stage timer: %+v", got)
	}
}

func TestLLMActivityReducerDoesNotResetElapsedTimeForStreamingDeltas(t *testing.T) {
	state := NewAppState()
	started := time.Unix(100, 0)
	state.SetLLMCall(&LLMCallStatus{
		Phase: LLMCallWorking, Stage: LLMStageThinking, StageStartedAt: started,
	})
	if state.setLLMActivityAt(LLMStageThinking, "", started.Add(5*time.Second)) {
		t.Fatal("same-stage streaming delta changed activity state")
	}
	if got := state.LLMCall.Get().StageStartedAt; !got.Equal(started) {
		t.Fatalf("same-stage streaming delta reset timer to %s", got)
	}
	if !state.setLLMActivityAt(LLMStageResponse, "", started.Add(8*time.Second)) {
		t.Fatal("observed response transition did not advance activity state")
	}
	got := state.LLMCall.Get()
	if got.Stage != LLMStageResponse || !got.StageStartedAt.Equal(started.Add(8*time.Second)) {
		t.Fatalf("response activity state = %+v", got)
	}
}

func TestLLMRequestAndContentEventsAdvanceObservedStages(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.BeginLLMWork()
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}

	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestStart, stream.RequestStatusEvent{RequestID: "request-1"})
	if got := state.LLMCall.Get().Stage; got != LLMStageWaitingFirstToken {
		t.Fatalf("request-start stage = %q", got)
	}
	renderer.LLMActivityAtEpoch(1, LLMStageToolInput, "Inspect")
	if got := state.LLMCall.Get(); got.Stage != LLMStageToolInput || got.StageDetail != "Inspect" {
		t.Fatalf("tool-input stage = %+v", got)
	}
	renderer.LLMActivityAtEpoch(1, LLMStageToolExecution, "Inspect")
	renderer.LLMActivityAtEpoch(1, LLMStageWaitingAfterTools, "")
	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestStart, stream.RequestStatusEvent{RequestID: "request-2"})
	if got := state.LLMCall.Get().Stage; got != LLMStageWaitingAfterTools {
		t.Fatalf("post-tool request stage = %q", got)
	}
	renderer.ThinkingAtEpoch(1, "reasoning")
	if got := state.LLMCall.Get().Stage; got != LLMStageThinking {
		t.Fatalf("thinking stage = %q", got)
	}
	renderer.TextAtEpoch(1, "answer")
	if got := state.LLMCall.Get().Stage; got != LLMStageResponse {
		t.Fatalf("response stage = %q", got)
	}
}

func TestLLMRequestRetryStatusShowsProblemAttemptDelayAndCause(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Language.Set(i18n.LangZH)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestRetry, stream.RequestStatusEvent{
		RequestID: "request-1", Attempt: 2, MaxRetries: 10, RetryDelayMilliseconds: 2000, RetryKind: "stream", Error: "connection reset",
	})

	status := state.LLMCall.Get()
	text := collectElementText(NewRootComponent(state, nil, nil).renderLLMStatus(status))
	for _, want := range []string{"正在重连 2/10", "2.0s 后继续", "问题：connection reset"} {
		if !strings.Contains(text, want) {
			t.Fatalf("retry status omitted %q: %q", want, text)
		}
	}
	if strings.Contains(text, "LLM API 请求出错") {
		t.Fatalf("retry status used a generic header instead of the actionable reconnect state: %q", text)
	}
}

func TestLLMRequestRetryProblemPersistsUntilReplacementRequestProducesOutput(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Language.Set(i18n.LangEN)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}

	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestRetry, stream.RequestStatusEvent{
		RequestID: "failed-request", Attempt: 1, MaxRetries: 5, RetryDelayMilliseconds: 200,
		RetryKind: "stream", Error: "idle timeout waiting for SSE",
	})
	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestStart, stream.RequestStatusEvent{
		RequestID: "replacement-request", Attempt: 2, MaxRetries: 5, RequestMilliseconds: 30,
	})

	status := state.LLMCall.Get()
	if status == nil || status.RequestID != "replacement-request" || status.Phase != LLMCallRetrying || status.Error != "idle timeout waiting for SSE" {
		t.Fatalf("replacement request hid retry problem before output: %+v", status)
	}
	text := collectElementText(NewRootComponent(state, nil, nil).renderLLMStatus(status))
	for _, want := range []string{"Reconnecting 1/5", "Problem: idle timeout waiting for SSE"} {
		if !strings.Contains(text, want) {
			t.Fatalf("persistent retry status omitted %q: %q", want, text)
		}
	}

	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestFirstToken, stream.RequestStatusEvent{
		RequestID: "replacement-request", FirstTokenMilliseconds: 450,
	})
	if recovered := state.LLMCall.Get(); recovered == nil || recovered.Phase != LLMCallWorking || !recovered.HasFirstToken {
		t.Fatalf("first output did not clear retry problem state: %+v", recovered)
	}
}

func TestLLMRequestRetryRendersProblemOnDedicatedNarrowTerminalRow(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	status := &LLMCallStatus{
		Phase: LLMCallRetrying, Attempt: 2, MaxRetries: 5, RetryKind: "stream",
		RetryDelay: 400 * time.Millisecond, Error: "connection reset by provider",
	}
	rendered := renderElementText(NewRootComponent(state, nil, nil).renderLLMStatus(status), 36, 2)
	lines := strings.Split(rendered, "\n")
	if len(lines) < 2 || !strings.Contains(lines[0], "Reconnecting 2/5") || !strings.Contains(lines[1], "Problem: connection reset") {
		t.Fatalf("narrow retry status did not reserve a problem row:\n%s", rendered)
	}
}

func TestLLMWorkingStatusRetainsCurrentStreamAttempt(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Language.Set(i18n.LangEN)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestStart, stream.RequestStatusEvent{
		RequestID: "request-3", Attempt: 3, MaxRetries: 5, RequestMilliseconds: 25,
	})
	status := state.LLMCall.Get()
	text := collectElementText(NewRootComponent(state, nil, nil).renderLLMStatus(status))
	if !strings.Contains(text, "Attempt 3/6") {
		t.Fatalf("working status hid current stream attempt: %q", text)
	}
}

func TestLLMRequestRetryKindDistinguishesRequestRetry(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	status := &LLMCallStatus{
		Phase: LLMCallRetrying, Attempt: 1, MaxRetries: 4, RetryKind: "request",
		RetryDelay: 200 * time.Millisecond, Error: "temporary failure",
	}
	text := collectElementText(NewRootComponent(state, nil, nil).renderLLMStatus(status))
	if !strings.Contains(text, "Request retry 1/4") {
		t.Fatalf("request retry kind was not rendered: %q", text)
	}
}

func TestLLMRequestStatusHasEqualBlankRowsAboveAndBelow(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.SetLLMCall(&LLMCallStatus{RequestID: "request-1", Phase: LLMCallWorking})
	root := NewRootComponent(state, nil, nil)
	rendered := renderElementText(root.renderAtSize(nil, 100, 24), 100, 24)
	lines := strings.Split(rendered, "\n")
	statusRow := -1
	for index, line := range lines {
		if strings.Contains(line, "Working") && strings.Contains(line, "Connection") {
			statusRow = index
			break
		}
	}
	if statusRow <= 0 || statusRow+1 >= len(lines) {
		t.Fatalf("LLM status row was not placed inside the viewport:\n%s", rendered)
	}
	if strings.TrimSpace(lines[statusRow-1]) != "" || strings.TrimSpace(lines[statusRow+1]) != "" {
		t.Fatalf("LLM status is not vertically centered between blank rows: above=%q status=%q below=%q", lines[statusRow-1], lines[statusRow], lines[statusRow+1])
	}
}

func TestLLMRequestEndStaysWorkingAcrossToolUseUntilQuerySettlement(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestStart, stream.RequestStatusEvent{RequestID: "tool-request", TotalMilliseconds: 100})
	started := state.LLMCall.Get().WorkStartedAt
	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestEnd, stream.RequestStatusEvent{RequestID: "tool-request"})
	if current := state.LLMCall.Get(); current == nil || current.RequestID != "tool-request" {
		t.Fatal("tool-use stream completion cleared the working status")
	}
	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestStart, stream.RequestStatusEvent{RequestID: "follow-up", TotalMilliseconds: 50})
	if current := state.LLMCall.Get(); current == nil || current.RequestID != "follow-up" || !current.WorkStartedAt.Equal(started) {
		t.Fatalf("follow-up request did not preserve the execution timer: %+v", current)
	}
	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestEnd, stream.RequestStatusEvent{RequestID: "follow-up"})
	if state.LLMCall.Get() == nil {
		t.Fatal("final response cleared before query settlement")
	}
	state.ClearLLMCall()
	if state.LLMCall.Get() != nil {
		t.Fatal("query settlement did not clear the working status")
	}
}

func TestLLMWorkingStatusAlignsWithModeStatusBar(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	root := NewRootComponent(state, nil, nil)
	status := &LLMCallStatus{Phase: LLMCallWorking, WorkStartedAt: time.Unix(99, 0)}
	root.now = func() time.Time { return time.Unix(100, 0) }

	firstVisibleColumn := func(element *gtui.Element) int {
		buffer := gtui.NewBuffer(80, 1)
		element.Render(buffer, 80, 1)
		for column := 0; column < 80; column++ {
			if r := buffer.Cell(column, 0).Rune; r != 0 && r != ' ' {
				return column
			}
		}
		return -1
	}
	if working, mode := firstVisibleColumn(root.renderLLMStatus(status)), firstVisibleColumn(root.renderStatusBar(80)); working != 0 || mode != 0 {
		t.Fatalf("working/mode first visible columns = %d/%d, want 0/0", working, mode)
	}
}

func TestAssistantReplyUsesBulletHangingIndentAndDuration(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	root := NewRootComponent(state, nil, nil)
	element := root.renderMessage(Message{
		Kind: MsgAssistant, Text: "first paragraph wraps onto another line because it is deliberately long\n\nsecond paragraph",
		WorkDuration: 8*time.Minute + 2*time.Second,
	})
	buffer := gtui.NewBuffer(32, 8)
	element.Render(buffer, 32, 8)
	rendered := buffer.String()
	if buffer.Cell(0, 0).Rune != '●' || buffer.Cell(1, 0).Rune != ' ' {
		t.Fatalf("assistant bullet row does not start at column zero: %q", rendered)
	}
	for row := 1; row < 3; row++ {
		if r := buffer.Cell(0, row).Rune; r != 0 && r != ' ' {
			t.Fatalf("assistant continuation row %d escaped hanging indent: %q", row, rendered)
		}
		if r := buffer.Cell(1, row).Rune; r != 0 && r != ' ' {
			t.Fatalf("assistant continuation row %d escaped bullet width: %q", row, rendered)
		}
	}
	if !strings.Contains(rendered, "Worked for 8m 02s") {
		t.Fatalf("assistant completion duration missing: %q", rendered)
	}
	durationRow := strings.Index(rendered, "Worked for 8m 02s") / (buffer.Width() + 1)
	if durationRow < 0 || buffer.Cell(buffer.Width()-1, durationRow).Rune != '─' {
		t.Fatalf("assistant completion rule did not extend to the right edge: %q", rendered)
	}
}

func TestClearLLMCallAttachesDurationOnlyToLatestReplyFromCurrentWork(t *testing.T) {
	state := NewAppState()
	started := time.Unix(100, 0)
	state.Messages.Set([]Message{
		{Kind: MsgAssistant, Text: "old", Timestamp: started.Add(-time.Minute)},
		{Kind: MsgAssistant, Text: "current", Timestamp: started.Add(time.Second)},
	})
	state.SetLLMCall(&LLMCallStatus{Phase: LLMCallWorking, WorkStartedAt: started})
	state.clearLLMCallAt(started.Add(8*time.Minute+2*time.Second), "")

	messages := state.Messages.Get()
	if messages[0].WorkDuration != 0 || messages[1].WorkDuration != 8*time.Minute+2*time.Second {
		t.Fatalf("assistant work durations = %s/%s", messages[0].WorkDuration, messages[1].WorkDuration)
	}
	if state.LLMCall.Get() != nil {
		t.Fatal("completed work status was not cleared")
	}
}

func TestClearLLMCallWithoutCurrentReplyDoesNotAnnotateHistory(t *testing.T) {
	state := NewAppState()
	started := time.Unix(100, 0)
	state.Messages.Set([]Message{{Kind: MsgAssistant, Text: "old", Timestamp: started.Add(-time.Second)}})
	state.SetLLMCall(&LLMCallStatus{Phase: LLMCallWorking, WorkStartedAt: started})
	state.clearLLMCallAt(started.Add(time.Minute))
	if got := state.Messages.Get()[0].WorkDuration; got != 0 {
		t.Fatalf("old assistant reply received current work duration %s", got)
	}
}

func TestFormatAssistantWorkDurationUsesClockLikeUnits(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{duration: 8 * time.Second, want: "8s"},
		{duration: 8*time.Minute + 2*time.Second, want: "8m 02s"},
		{duration: time.Hour + 2*time.Minute + 3*time.Second, want: "1h 02m 03s"},
	}
	for _, test := range tests {
		if got := formatAssistantWorkDuration(test.duration); got != test.want {
			t.Fatalf("formatAssistantWorkDuration(%s) = %q, want %q", test.duration, got, test.want)
		}
	}
}
