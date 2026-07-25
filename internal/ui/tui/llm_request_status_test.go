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
	if rendered := buffer.String(); !strings.Contains(rendered, "• Working (1h1m1s • Ctrl+C to interrupt) Connection 1m10s · First token 2m10s") {
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
	root.now = func() time.Time { return now }
	element := root.renderLLMStatus(status)
	text := collectElementText(element)
	for _, want := range []string{"• 工作中", "(5.2s • Ctrl+C 中断)", "建立连接 —", "首 token —"} {
		if !strings.Contains(text, want) {
			t.Fatalf("pre-provider working status omitted %q: %q", want, text)
		}
	}
	if strings.Contains(text, "总耗时") {
		t.Fatalf("working status retained duplicate total-duration metric: %q", text)
	}
}

func TestLLMRequestRetryStatusShowsProblemAttemptDelayAndCause(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Language.Set(i18n.LangZH)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	renderer.LLMRequestStatusAtEpoch(1, stream.EventRequestRetry, stream.RequestStatusEvent{
		RequestID: "request-1", Attempt: 2, MaxRetries: 10, RetryDelayMilliseconds: 2000, Error: "connection reset",
	})

	status := state.LLMCall.Get()
	text := collectElementText(NewRootComponent(state, nil, nil).renderLLMStatus(status))
	for _, want := range []string{"LLM API 请求出错", "第 2/10 次重试", "2.0s 后继续", "connection reset"} {
		if !strings.Contains(text, want) {
			t.Fatalf("retry status omitted %q: %q", want, text)
		}
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
