package tui

import (
	"strings"
	"testing"

	gtui "github.com/grindlemire/go-tui"
)

func TestTaskCreateTask26TaskViewCollapsesToCenteredSummary(t *testing.T) {
	state := NewAppState()
	items := []TaskViewItem{
		{ID: "1", Subject: "Pending work", Status: "pending"},
		{ID: "2", Subject: "Active work", Status: "in_progress", Owner: "worker-a", BlockedBy: []string{"1"}},
		{ID: "3", Subject: "Finished work", Status: "completed"},
	}
	state.RefreshTasksView(items)
	root := NewRootComponent(state, nil, nil)
	element := root.renderTaskView(state.TaskViewItems.Get(), 1)
	buffer := gtui.NewBuffer(80, 1)
	element.Render(buffer, 80, 1)
	text := buffer.String()
	if !strings.Contains(text, "Tasks (3) ▸") {
		t.Fatalf("collapsed task view missing summary: %q", text)
	}
	if strings.Contains(text, "Pending work") {
		t.Fatalf("collapsed task view leaked task details: %q", text)
	}
	if column := strings.Index(text, "Tasks (3) ▸"); column < 33 || column > 35 {
		t.Fatalf("collapsed task summary column = %d, want centered in 80 cells: %q", column, text)
	}
}

func TestTaskCreateTask26TaskViewHasMatchingVerticalSpacing(t *testing.T) {
	state := NewAppState()
	state.RefreshTasksView([]TaskViewItem{{ID: "1", Subject: "Pending work", Status: "pending"}})
	root := NewRootComponent(state, nil, nil)
	buffer := gtui.NewBuffer(80, 24)
	root.renderAtSize(nil, 80, 24).Render(buffer, 80, 24)

	summary := root.taskViewRef.El()
	if summary == nil || summary.Rect().IsEmpty() {
		t.Fatalf("task summary ref was not laid out: %#v", summary)
	}
	y := summary.Rect().Y
	if y <= 0 || y+1 >= buffer.Height() {
		t.Fatalf("task summary row %d has no room for symmetric spacing", y)
	}
	above := extractBufferLine(buffer, y-1, buffer.Width())
	below := extractBufferLine(buffer, y+1, buffer.Width())
	if strings.TrimSpace(above) != "" || strings.TrimSpace(below) != "" {
		t.Fatalf("task spacing differs: above=%q below=%q\n%s", above, below, buffer.String())
	}
}

func TestTaskCreateTask26TaskSummaryClickExpandsBoundedSnapshot(t *testing.T) {
	state := NewAppState()
	items := []TaskViewItem{
		{ID: "1", Subject: "Pending work", Status: "pending"},
		{ID: "2", Subject: "Active work", Status: "in_progress", Owner: "worker-a", BlockedBy: []string{"1"}},
		{ID: "3", Subject: "Finished work", Status: "completed"},
	}
	state.RefreshTasksView(items)
	root := NewRootComponent(state, nil, nil)
	frame := root.renderAtSize(nil, 80, 24)
	frame.Render(gtui.NewBuffer(80, 24), 80, 24)
	summary := root.taskViewRef.El()
	if summary == nil || summary.Rect().IsEmpty() {
		t.Fatalf("task summary ref was not laid out: %#v", summary)
	}
	rect := summary.Rect()
	if !root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseLeft, Action: gtui.MousePress, X: rect.X, Y: rect.Y}) {
		t.Fatal("task summary click was not handled")
	}
	if state.ExpandedView.Get() != "tasks" {
		t.Fatalf("task summary click did not expand view: %q", state.ExpandedView.Get())
	}

	expanded := root.renderTaskView(state.TaskViewItems.Get(), 4)
	expanded.Render(gtui.NewBuffer(80, 4), 80, 4)
	text := collectElementText(expanded)
	for _, want := range []string{"Tasks (3)", "[ ] #1 Pending work", "[~] #2 Active work @worker-a (blocked by #1)", "[x] #3 Finished work"} {
		if !strings.Contains(text, want) {
			t.Errorf("task view missing %q: %q", want, text)
		}
	}
	summary = root.taskViewRef.El()
	rect = summary.Rect()
	if !root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseLeft, Action: gtui.MousePress, X: rect.X, Y: rect.Y}) {
		t.Fatal("expanded task summary click was not handled")
	}
	if state.ExpandedView.Get() != "" {
		t.Fatalf("expanded task summary click did not collapse view: %q", state.ExpandedView.Get())
	}
	state.SetExpandedView("tasks")

	many := make([]TaskViewItem, 8)
	for i := range many {
		many[i] = TaskViewItem{ID: string(rune('1' + i)), Subject: "bounded", Status: "pending"}
	}
	bounded := collectElementText(root.renderTaskView(many, 6))
	if !strings.Contains(bounded, "... 4 more") {
		t.Fatalf("bounded task view did not summarize overflow: %q", bounded)
	}
}
