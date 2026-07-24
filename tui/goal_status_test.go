package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/loop"
)

const testGoalStatusMaxCells = 40

type goalStatusSetter interface {
	SetGoalStatus(status, objective string)
}

func setGoalStatusForTest(t *testing.T, state *AppState, status, objective string) {
	t.Helper()
	setter, ok := any(state).(goalStatusSetter)
	if !ok {
		t.Fatal("AppState must implement SetGoalStatus(status, objective)")
	}
	setter.SetGoalStatus(status, objective)
}

func goalStatusSegment(t *testing.T, root *RootComponent, width int) string {
	t.Helper()
	for _, line := range strings.Split(collectElementText(root.renderStatusBar(width)), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Goal") {
			return line
		}
	}
	return ""
}

func TestGoalStatusAbsentAddsNoStatusBarOutput(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	text := strings.ToLower(collectElementText(root.renderStatusBar(120)))
	if strings.Contains(text, "goal") {
		t.Fatalf("status bar exposed a goal segment without goal state: %q", text)
	}
}

func TestGoalStatusRendersActiveAndPausedCompactly(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		objective string
		want      string
	}{
		{name: "active", status: "active", objective: "ship the goal display", want: "Goal: ship the goal display"},
		{name: "paused", status: "paused", objective: "wait for user input", want: "Goal paused: wait for user input"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := NewAppState()
			setGoalStatusForTest(t, state, tt.status, tt.objective)
			root := NewRootComponent(state, nil, nil)
			if got := goalStatusSegment(t, root, 120); got != tt.want {
				t.Fatalf("goal status segment = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGoalStatusTruncatesCJKByTerminalCells(t *testing.T) {
	state := NewAppState()
	objective := strings.Repeat("目标", 18)
	setGoalStatusForTest(t, state, "active", objective)
	root := NewRootComponent(state, nil, nil)

	segment := goalStatusSegment(t, root, 120)
	if segment == "" {
		t.Fatal("wide status bar hid the active goal instead of truncating it")
	}
	if width := terminalCellWidth(segment); width > testGoalStatusMaxCells {
		t.Fatalf("CJK goal segment width = %d cells, want at most %d: %q", width, testGoalStatusMaxCells, segment)
	}
	if !strings.HasSuffix(segment, "…") {
		t.Fatalf("truncated CJK goal segment lacks ellipsis: %q", segment)
	}
	if !strings.HasPrefix(segment, "Goal: 目标") {
		t.Fatalf("truncated CJK goal segment lost its label or objective prefix: %q", segment)
	}
}

func TestGoalStatusSanitizesControlCharactersToOneLine(t *testing.T) {
	state := NewAppState()
	setGoalStatusForTest(t, state, "active", "ship\nrelease\t\x1b safely\rnow")
	root := NewRootComponent(state, nil, nil)

	if got := goalStatusSegment(t, root, 120); got != "Goal: ship release safely now" {
		t.Fatalf("goal status segment = %q, want a single sanitized line", got)
	}
}

func TestGoalStatusHidesWholeSegmentOnNarrowTerminal(t *testing.T) {
	state := NewAppState()
	setGoalStatusForTest(t, state, "active", "ship release")
	root := NewRootComponent(state, nil, nil)

	rendered := renderElementText(root.renderStatusBar(24), 24, 2)
	if !strings.Contains(rendered, "Auto mode") {
		t.Fatalf("narrow status bar lost its high-priority mode segment:\n%s", rendered)
	}
	for _, partial := range []string{"Goal", "ship", "release"} {
		if strings.Contains(rendered, partial) {
			t.Fatalf("narrow status bar partially rendered hidden goal segment %q:\n%s", partial, rendered)
		}
	}
}

func TestTuiRendererGoalStatusEventKeepsCriteriaThroughTerminalState(t *testing.T) {
	state := NewAppState()
	state.SessionEpoch.Set(9)
	renderer := &TuiRenderer{
		state: state,
		enqueue: func(update func()) bool {
			update()
			return true
		},
	}

	renderer.GoalStatusAtEpoch(9, loop.GoalStatusEvent{
		Status: "active", Objective: "finish the release", Revision: 2,
		Criteria: []loop.GoalCriterionStatusEvent{{ID: "AC-1", Text: "tests pass", Status: "pending"}},
	})
	if got := state.Goal.Get(); got == nil || got.Status != "active" || got.Objective != "finish the release" || len(got.Criteria) != 1 {
		t.Fatalf("active goal state = %+v", got)
	}

	renderer.GoalStatusAtEpoch(9, loop.GoalStatusEvent{
		Status: "achieved", Objective: "finish the release", Revision: 2,
		Criteria: []loop.GoalCriterionStatusEvent{{ID: "AC-1", Text: "tests pass", Status: "met", Reason: "verified"}},
	})
	if got := state.Goal.Get(); got == nil || got.Status != "achieved" || got.Criteria[0].Status != "met" {
		t.Fatalf("terminal goal lost acceptance state: %+v", got)
	}

	renderer.GoalStatusAtEpoch(9, loop.GoalStatusEvent{Status: "cleared", Objective: "finish the release"})
	if got := state.Goal.Get(); got != nil {
		t.Fatalf("cleared goal remained visible: %+v", got)
	}
}

func TestGoalAcceptancePanelShowsAgentCriteriaAndStatuses(t *testing.T) {
	state := NewAppState()
	state.SetGoalView(&GoalViewState{
		Status: "active", Objective: "ship the release", Revision: 3,
		Criteria: []GoalCriterionViewState{
			{ID: "AC-1", Text: "focused tests pass", Status: "met"},
			{ID: "AC-2", Text: "release notes updated", Status: "unmet", Reason: "missing evidence"},
			{ID: "AC-3", Text: "user-visible copy is localized", Status: "pending"},
		},
	})
	root := NewRootComponent(state, nil, nil)
	text := collectElementText(root.renderGoalView(state.Goal.Get(), 4))
	for _, want := range []string{
		"Goal acceptance", "revision 3", "1/3 met", "ship the release",
		"[met] AC-1: focused tests pass", "[not met] AC-2: release notes updated",
		"[pending] AC-3: user-visible copy is localized",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("goal acceptance panel omitted %q:\n%s", want, text)
		}
	}
	if got := goalStatusSegment(t, root, 120); !strings.Contains(got, "1/3 met") {
		t.Fatalf("goal status omitted acceptance progress: %q", got)
	}
	frame := renderElementText(root.renderAtSize(nil, 100, 24), 100, 24)
	for _, want := range []string{"Goal acceptance", "[met] AC-1", "[not met] AC-2", "[pending] AC-3"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("full TUI frame omitted %q:\n%s", want, frame)
		}
	}
}
