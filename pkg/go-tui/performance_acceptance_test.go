package tui

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	firstTokenFrameBudget   = 100 * time.Millisecond
	firstTokenFrameWatchdog = time.Second
)

type frameLatencyComponent struct {
	text string
}

func (c *frameLatencyComponent) Render(_ *App) *Element {
	return New(WithText(c.text))
}

// TestFirstTokenEventToTerminalFrameP95UnderBudget is an acceptance
// measurement, not a regression for a newly introduced implementation. It
// deliberately exercises the existing asynchronous update fan-in and complete
// render pipeline instead of manufacturing a failing implementation change.
func TestFirstTokenEventToTerminalFrameP95UnderBudget(t *testing.T) {
	const samples = 100
	terminal := NewMockTerminal(80, 24)
	app := &App{
		terminal:      terminal,
		buffer:        NewBuffer(80, 24),
		focus:         newFocusManager(),
		inputEvents:   make(chan Event, samples),
		updates:       make(chan Event, samples),
		merged:        make(chan Event, samples),
		watcherQueue:  make(chan func(), samples),
		stopCh:        make(chan struct{}),
		mounts:        newMountState(),
		batch:         newBatchContext(),
		frameDuration: 16 * time.Millisecond,
	}
	app.startEventMerge()
	defer app.Stop()

	component := &frameLatencyComponent{text: "warming-up"}
	app.SetRootComponent(component)
	app.Render()

	durations := make([]time.Duration, samples)
	for i := range durations {
		token := fmt.Sprintf("visible-token-%03d", i)
		start := time.Now()
		if !app.QueueUpdateLossless(func() {
			component.text = token
			app.MarkDirty()
		}) {
			t.Fatal("app stopped before the first-token update was queued")
		}

		select {
		case event := <-app.Events():
			app.Dispatch(event)
			app.Render()
		case <-time.After(firstTokenFrameWatchdog):
			t.Fatalf("first-token event was not dispatched within watchdog %s", firstTokenFrameWatchdog)
		}

		durations[i] = time.Since(start)
		if visible := terminal.String(); !strings.Contains(visible, token) {
			t.Fatalf("sample %d did not reach the terminal buffer: %q", i, visible)
		}
	}

	p95 := frameDurationP95(durations)
	t.Logf("QueueUpdateLossless -> fan-in -> Dispatch -> Render -> terminal buffer p95: %s (%d samples)", p95, samples)
	if p95 >= firstTokenFrameBudget {
		t.Fatalf("first-token event to terminal frame p95 = %s, want <%s", p95, firstTokenFrameBudget)
	}
}

func frameDurationP95(durations []time.Duration) time.Duration {
	ordered := append([]time.Duration(nil), durations...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	index := (95*len(ordered)+99)/100 - 1
	return ordered[index]
}
