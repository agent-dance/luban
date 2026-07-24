package tui

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
	gtui "github.com/grindlemire/go-tui"
)

func TestLongTranscriptRenderWindowIsBounded(t *testing.T) {
	messages := make([]Message, 100_000)
	for i := range messages {
		messages[i] = Message{Kind: MsgAssistant, Text: fmt.Sprintf("message-%d", i)}
	}
	messages[7].ObservationID = "pinned-old"
	messages[7].Disclosure.UserPinned = true
	got := boundedTranscriptMessages(messages, 40)
	if len(got) > 40*4+33 {
		t.Fatalf("render projection contains %d rows for 100k observations", len(got))
	}
	if got[0].ObservationID != "pinned-old" {
		t.Fatalf("pinned expanded observation was not retained: %+v", got[0])
	}
}

func TestHundredThousandObservationRenderTreeTracksViewportAndUserPins(t *testing.T) {
	state := NewAppState()
	messages := make([]Message, 100_000)
	for i := range messages {
		messages[i] = Message{Kind: MsgInfo, Text: fmt.Sprintf("observation-%d", i), ObservationID: fmt.Sprintf("obs-%d", i)}
		if i < 5_000 {
			messages[i].Disclosure.UserPinned = true
		}
	}
	state.Messages.Set(messages)
	state.Observations.mu.Lock()
	for i := 0; i < 100_000; i++ {
		state.Observations.appendLocked(Observation{
			ID: fmt.Sprintf("obs-%d", i), ToolName: "Observation",
			Disclosure: DisclosureState{Level: DisclosureDetail, UserPinned: i < 5_000},
		})
	}
	got := len(state.Observations.observations)
	state.Observations.mu.Unlock()
	if got != 100_000 {
		t.Fatalf("fixture retained %d observations, want 100000", got)
	}
	root := NewRootComponent(state, nil, nil)
	tree := root.renderMessageArea(40)
	nodes := countElementTreeNodes(tree)
	if nodes < 5_000 || nodes > 16_000 {
		t.Fatalf("100k render tree nodes = %d, want viewport + all 5000 user pins", nodes)
	}
}

func TestHundredThousandObservationHistoryWindowCanReachOldMessages(t *testing.T) {
	state := NewAppState()
	messages := make([]Message, 100_000)
	for i := range messages {
		messages[i] = Message{Kind: MsgInfo, Text: fmt.Sprintf("history-%06d", i)}
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	root.termHeight = 24

	invokeRootHistoryBinding(t, root, gtui.KeyHome, gtui.ModCtrl)
	window := root.boundedTranscriptMessages(24)
	if len(window) == 0 || window[0].Text != "history-000000" {
		t.Fatalf("Home window did not reach oldest history: first=%+v", window[0])
	}
	root.pageTranscriptHistory(1) // PageDown key path.
	if got := root.historyStart.Get(); got <= 0 {
		t.Fatalf("PageDown did not advance logical history window: %d", got)
	}
	invokeRootHistoryBinding(t, root, gtui.KeyEnd, gtui.ModCtrl)
	window = root.boundedTranscriptMessages(24)
	if got := window[len(window)-1].Text; got != "history-099999" {
		t.Fatalf("End window tail = %q, want newest message", got)
	}
}

func TestInteractionRestoreResetsLogicalWindowAcrossLongSessionsAndHonorsAnchor(t *testing.T) {
	state := NewAppState()
	messages := make([]Message, 1_000)
	for i := range messages {
		messages[i] = Message{Kind: MsgInfo, Text: fmt.Sprintf("target-%04d", i), ObservationID: fmt.Sprintf("obs-%04d", i)}
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	root.termHeight = 24
	root.historyStart.Set(0) // stale Home position from the previous session

	root.restoreInteractionViewport(SessionInteraction{})
	window := root.boundedTranscriptMessages(24)
	if got := window[len(window)-1].Text; got != "target-0999" {
		t.Fatalf("bottom interaction retained stale logical window: tail=%q historyStart=%d", got, root.historyStart.Get())
	}

	root.restoreInteractionViewport(SessionInteraction{ScrollAnchorID: "obs-0400", ScrollOffset: 3})
	window = root.boundedTranscriptMessages(24)
	found := false
	for _, message := range window {
		found = found || message.ObservationID == "obs-0400"
	}
	if !found {
		t.Fatalf("restored anchor is outside logical window: historyStart=%d", root.historyStart.Get())
	}
}

func invokeRootHistoryBinding(t *testing.T, root *RootComponent, key gtui.Key, modifier gtui.Modifier) {
	t.Helper()
	for _, binding := range root.KeyMap() {
		if binding.Pattern.Key == key && binding.Pattern.Mod == modifier {
			if !binding.Preempt || !binding.Stop {
				t.Fatalf("history binding for %v/%v is not preemptive", key, modifier)
			}
			binding.Handler(gtui.KeyEvent{Key: key, Mod: modifier})
			return
		}
	}
	t.Fatalf("history binding for %v/%v not found", key, modifier)
}

func TestFirstTokenToRootBufferP95UnderAcceptanceBudget(t *testing.T) {
	durations := make([]time.Duration, 50)
	for i := range durations {
		state := NewAppState()
		state.SessionEpoch.Set(1)
		renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
		root := NewRootComponent(state, nil, nil)
		start := time.Now()
		renderer.TextAtEpoch(1, "visible-token")
		element := root.renderAtSize(nil, 80, 24)
		buffer := gtui.NewBuffer(80, 24)
		element.Render(buffer, 80, 24)
		durations[i] = time.Since(start)
		if !strings.Contains(buffer.String(), "visible-token") {
			t.Fatal("first token did not reach the terminal buffer")
		}
	}
	p95 := durationP95(durations)
	t.Logf("first token to Root buffer p95: %s", p95)
	if p95 >= 100*time.Millisecond {
		t.Fatalf("first token to Root buffer p95 = %s, want <100ms", p95)
	}
}

func countElementTreeNodes(element interface{ Children() []*gtui.Element }) int {
	count := 1
	for _, child := range element.Children() {
		count += countElementTreeNodes(child)
	}
	return count
}

func TestMemoryDetailReadP95UnderAcceptanceBudget(t *testing.T) {
	store := NewMemoryDetailStore()
	ref, err := store.Put("perf", []byte("complete evidence"))
	if err != nil {
		t.Fatal(err)
	}
	durations := make([]time.Duration, 200)
	for i := range durations {
		start := time.Now()
		if _, err := store.Get(ref); err != nil {
			t.Fatal(err)
		}
		durations[i] = time.Since(start)
	}
	p95 := durationP95(durations)
	t.Logf("memory detail read p95: %s", p95)
	if p95 >= 100*time.Millisecond {
		t.Fatalf("memory detail p95 = %s, want <100ms", p95)
	}
}

func TestFileDetailReadP95UnderAcceptanceBudget(t *testing.T) {
	store, err := NewFileDetailStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ref, err := store.Put("perf", []byte("complete evidence"))
	if err != nil {
		t.Fatal(err)
	}
	durations := make([]time.Duration, 100)
	for i := range durations {
		start := time.Now()
		if _, err := store.Get(ref); err != nil {
			t.Fatal(err)
		}
		durations[i] = time.Since(start)
	}
	p95 := durationP95(durations)
	t.Logf("file detail read p95: %s", p95)
	if p95 >= 250*time.Millisecond {
		t.Fatalf("file detail p95 = %s, want <250ms", p95)
	}
}

func TestDetailDisclosureToRootBufferP95UnderAcceptanceBudgets(t *testing.T) {
	if raceDetectorEnabled {
		t.Skip("performance budgets are measured without race instrumentation")
	}
	t.Run("memory", func(t *testing.T) {
		measureDetailDisclosureToRootBuffer(t, NewMemoryDetailStore(), 50, 100*time.Millisecond)
	})
	t.Run("file", func(t *testing.T) {
		store, err := NewFileDetailStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		measureDetailDisclosureToRootBuffer(t, store, 25, 250*time.Millisecond)
	})
}

func measureDetailDisclosureToRootBuffer(t *testing.T, details DetailStore, samples int, budget time.Duration) {
	t.Helper()
	state := NewAppState()
	state.Details = details
	state.Observations = NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "perf-session", TurnID: "perf-turn", Outcome: OutcomeSucceeded}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "perf-tool", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	evidence := strings.Repeat("complete retained detail line\n", 300) + "DETAIL_SENTINEL"
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: "perf-tool", Content: evidence}); err != nil {
		t.Fatal(err)
	}
	observationID := toolObservationID(ctx.SessionID, "perf-tool")
	root := NewRootComponent(state, nil, nil)
	durations := make([]time.Duration, samples)
	for i := range durations {
		if err := state.RevealObservation(observationID, DisclosureSummary); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if err := state.RevealObservation(observationID, DisclosureDetail); err != nil {
			t.Fatal(err)
		}
		element := root.renderAtSize(nil, 80, 24)
		buffer := gtui.NewBuffer(80, 24)
		element.Render(buffer, 80, 24)
		durations[i] = time.Since(start)
		if !strings.Contains(buffer.String(), "DETAIL_SENTINEL") {
			t.Fatal("expanded detail did not reach the visible Root buffer")
		}
	}
	p95 := durationP95(durations)
	t.Logf("detail disclosure to Root buffer p95: %s", p95)
	if p95 >= budget {
		t.Fatalf("detail disclosure to Root buffer p95 = %s, want <%s", p95, budget)
	}
}

func TestFirstTokenAndStateTransitionP95UnderAcceptanceBudget(t *testing.T) {
	firstToken := make([]time.Duration, 200)
	transition := make([]time.Duration, 200)
	for i := range firstToken {
		state := NewAppState()
		start := time.Now()
		state.AppendOrStreamTextForTurn("visible", i+1)
		firstToken[i] = time.Since(start)
		if messages := state.Messages.Get(); len(messages) != 1 || messages[0].Text != "visible" {
			t.Fatalf("first token was not immediately visible: %+v", messages)
		}

		start = time.Now()
		if err := state.ApplyToolCall(ToolEventContext{}, types.ToolUseBlock{ID: fmt.Sprintf("tool-%d", i), Name: "Read"}); err != nil {
			t.Fatal(err)
		}
		transition[i] = time.Since(start)
	}
	firstP95 := durationP95(firstToken)
	transitionP95 := durationP95(transition)
	t.Logf("first token p95: %s; state transition p95: %s", firstP95, transitionP95)
	if firstP95 >= 100*time.Millisecond {
		t.Fatalf("first token p95 = %s, want <100ms", firstP95)
	}
	if transitionP95 >= 100*time.Millisecond {
		t.Fatalf("state transition p95 = %s, want <100ms", transitionP95)
	}
}

func durationP95(values []time.Duration) time.Duration {
	copyValues := append([]time.Duration(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	return copyValues[(len(copyValues)*95-1)/100]
}
