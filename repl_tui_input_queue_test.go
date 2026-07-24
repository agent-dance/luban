package main

import (
	"reflect"
	"testing"

	"github.com/agent-dance/luban/tui"
)

func TestTUIInputSchedulerQueuesAndDrainsFIFO(t *testing.T) {
	var launched []func()
	var ran []tuiInputSubmission
	var counts []int
	scheduler := newTUIInputScheduler(
		func(fn func()) bool { launched = append(launched, fn); return true },
		nil,
		func(input tuiInputSubmission) { ran = append(ran, input) },
		nil,
		func(count int) { counts = append(counts, count) },
	)

	if queued := scheduler.Submit("first", nil); queued {
		t.Fatal("idle submission was queued")
	}
	images := []tui.ImageAttachment{{ID: 7, MediaType: "image/png"}}
	if queued := scheduler.Submit("second", func() []tui.ImageAttachment { return images }); !queued {
		t.Fatal("active submission did not queue")
	}
	if queued := scheduler.Submit("third", nil); !queued {
		t.Fatal("second active submission did not queue")
	}
	if len(launched) != 1 {
		t.Fatalf("launches before completion = %d, want 1", len(launched))
	}

	launched[0]()
	if len(launched) != 2 {
		t.Fatalf("launches after first completion = %d, want 2", len(launched))
	}
	launched[1]()
	launched[2]()
	texts := make([]string, 0, len(ran))
	for _, submission := range ran {
		texts = append(texts, submission.text)
	}
	if !reflect.DeepEqual(texts, []string{"first", "second", "third"}) {
		t.Fatalf("run order = %#v", texts)
	}
	if !ran[1].imagesCaptured || len(ran[1].images) != 1 || ran[1].images[0].ID != 7 {
		t.Fatalf("queued image capture = %+v", ran[1])
	}
	if !reflect.DeepEqual(counts, []int{1, 2, 1, 0, 0}) {
		t.Fatalf("queue counts = %#v", counts)
	}
}

func TestTUIInputSchedulerPromotesOldestQueuedMessageToSteering(t *testing.T) {
	var launched []func()
	cancelled := 0
	var ran []tuiInputSubmission
	scheduler := newTUIInputScheduler(
		func(fn func()) bool { launched = append(launched, fn); return true },
		nil,
		func(input tuiInputSubmission) { ran = append(ran, input) },
		func() bool { cancelled++; return true },
		nil,
	)
	scheduler.Submit("current", nil)
	scheduler.Submit("guide", nil)
	scheduler.Submit("later", nil)
	if !scheduler.PromoteQueuedToSteering() {
		t.Fatal("queued message was not promoted")
	}
	if scheduler.PromoteQueuedToSteering() {
		t.Fatal("same queued message was promoted twice")
	}
	if cancelled != 1 {
		t.Fatalf("cancel calls = %d, want 1", cancelled)
	}
	launched[0]()
	launched[1]()
	if len(ran) < 2 || ran[1].text != "guide" || !ran[1].steering {
		t.Fatalf("promoted submission = %+v", ran)
	}
}

func TestTUIInputSchedulerPreparesAdmissionBeforeSubmitReturns(t *testing.T) {
	prepared := false
	var launched func()
	scheduler := newTUIInputScheduler(
		func(fn func()) bool { launched = fn; return true },
		func(*tuiInputSubmission) bool { prepared = true; return true },
		func(tuiInputSubmission) {},
		nil,
		nil,
	)
	scheduler.Submit("current", nil)
	if !prepared {
		t.Fatal("foreground admission was not prepared synchronously")
	}
	if launched == nil {
		t.Fatal("prepared submission was not launched")
	}
	launched()
}

func TestTUIInputSchedulerRejectsBusySubmissionWhenQueueDisabled(t *testing.T) {
	var launched []func()
	scheduler := newTUIInputScheduler(
		func(fn func()) bool { launched = append(launched, fn); return true },
		nil,
		func(tuiInputSubmission) {},
		nil,
		nil,
	)
	if accepted, queued := scheduler.TrySubmit("first", nil, false); !accepted || queued {
		t.Fatalf("first admission accepted=%v queued=%v", accepted, queued)
	}
	if accepted, queued := scheduler.TrySubmit("keep this draft", nil, false); accepted || queued {
		t.Fatalf("busy admission accepted=%v queued=%v", accepted, queued)
	}
	if len(launched) != 1 {
		t.Fatalf("busy submission launched %d workers", len(launched))
	}
	launched[0]()
}

func TestDefaultTUIComposerAdmissionDoesNotImplicitlyQueue(t *testing.T) {
	var launched []func()
	var captured int
	scheduler := newTUIInputScheduler(
		func(fn func()) bool { launched = append(launched, fn); return true },
		nil,
		func(tuiInputSubmission) {},
		nil,
		nil,
	)
	if accepted, queued := scheduler.TrySubmit("active", nil, false); !accepted || queued {
		t.Fatalf("initial admission accepted=%v queued=%v", accepted, queued)
	}
	if accepted, queued := scheduler.TrySubmit("draft", func() []tui.ImageAttachment {
		captured++
		return []tui.ImageAttachment{{ID: 1}}
	}, false); accepted || queued {
		t.Fatalf("busy composer admission accepted=%v queued=%v", accepted, queued)
	}
	if captured != 0 {
		t.Fatalf("rejected busy admission consumed composer attachments %d times", captured)
	}
	if len(launched) != 1 {
		t.Fatalf("busy composer admission launched %d workers", len(launched))
	}
	launched[0]()
}
