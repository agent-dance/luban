package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	gtui "github.com/grindlemire/go-tui"
)

func TestLLMRequestStatusRendersOnlyWorkingAndDimCallMetrics(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Language.Set(i18n.LangEN)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}

	renderer.LLMRequestStatusAtEpoch(1, loop.EventRequestStart, loop.RequestStatusEvent{
		RequestID: "request-1", RequestMilliseconds: 120, TotalMilliseconds: 120,
	})
	renderer.LLMRequestStatusAtEpoch(1, loop.EventRequestFirstToken, loop.RequestStatusEvent{
		RequestID: "request-1", FirstTokenMilliseconds: 800, TotalMilliseconds: 800,
	})
	status := state.LLMCall.Get()
	if status == nil {
		t.Fatal("LLM call status was not published")
	}
	element := NewRootComponent(state, nil, nil).renderLLMStatus(status)
	text := collectElementText(element)
	for _, want := range []string{"Working", "API request 120ms", "First token 800ms", "Total"} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLM request status omitted %q: %q", want, text)
		}
	}
	for _, forbidden := range []string{"activities", "agents", "Agent"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("LLM request status retained activity detail %q: %q", forbidden, text)
		}
	}

	buffer := gtui.NewBuffer(120, 1)
	element.Render(buffer, 120, 1)
	apiColumn := strings.Index(buffer.String(), "API request")
	if apiColumn < 0 || !buffer.Cell(apiColumn, 0).Style.HasAttr(gtui.AttrDim) {
		t.Fatalf("call metrics are not rendered dim: %q", buffer.String())
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
	text := collectElementText(NewRootComponent(state, nil, nil).renderLLMStatus(status))
	for _, want := range []string{"工作中", "API 请求 —", "首 token —", "总耗时"} {
		if !strings.Contains(text, want) {
			t.Fatalf("pre-provider working status omitted %q: %q", want, text)
		}
	}
}

func TestLLMRequestRetryStatusShowsProblemAttemptDelayAndCause(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Language.Set(i18n.LangZH)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	renderer.LLMRequestStatusAtEpoch(1, loop.EventRequestRetry, loop.RequestStatusEvent{
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
		if strings.Contains(line, "Working") && strings.Contains(line, "API request") {
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
	renderer.LLMRequestStatusAtEpoch(1, loop.EventRequestStart, loop.RequestStatusEvent{RequestID: "tool-request", TotalMilliseconds: 100})
	started := state.LLMCall.Get().WorkStartedAt
	renderer.LLMRequestStatusAtEpoch(1, loop.EventRequestEnd, loop.RequestStatusEvent{RequestID: "tool-request"})
	if current := state.LLMCall.Get(); current == nil || current.RequestID != "tool-request" {
		t.Fatal("tool-use stream completion cleared the working status")
	}
	renderer.LLMRequestStatusAtEpoch(1, loop.EventRequestStart, loop.RequestStatusEvent{RequestID: "follow-up", TotalMilliseconds: 50})
	if current := state.LLMCall.Get(); current == nil || current.RequestID != "follow-up" || !current.WorkStartedAt.Equal(started) {
		t.Fatalf("follow-up request did not preserve the execution timer: %+v", current)
	}
	renderer.LLMRequestStatusAtEpoch(1, loop.EventRequestEnd, loop.RequestStatusEvent{RequestID: "follow-up"})
	if state.LLMCall.Get() == nil {
		t.Fatal("final response cleared before query settlement")
	}
	state.ClearLLMCall()
	if state.LLMCall.Get() != nil {
		t.Fatal("query settlement did not clear the working status")
	}
}
