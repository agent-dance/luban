//go:build windows

package tui

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestWindowsReaderCancelCoversReadSubmissionGap(t *testing.T) {
	for _, test := range []struct {
		name   string
		cancel func(*stdinReader)
	}{
		{name: "pause", cancel: func(reader *stdinReader) { reader.Pause() }},
		{name: "close", cancel: func(reader *stdinReader) { _ = reader.Close() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			for iteration := 0; iteration < 10; iteration++ {
				readFile, writeFile, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				readerInterface, err := NewEventReader(readFile)
				if err != nil {
					t.Fatal(err)
				}
				reader := readerInterface.(*stdinReader)

				readEntered := make(chan struct{})
				releaseRead := make(chan struct{})
				cancelAttempted := make(chan struct{})
				var enterOnce sync.Once
				var cancelOnce sync.Once
				reader.readFn = func(buffer []byte) (int, error) {
					enterOnce.Do(func() { close(readEntered) })
					<-releaseRead
					return readFile.Read(buffer)
				}
				reader.cancelHook = func() {
					cancelOnce.Do(func() { close(cancelAttempted) })
				}

				pollDone := make(chan struct{})
				go func() {
					_, _ = reader.PollEvent(-1)
					close(pollDone)
				}()
				waitWindowsReaderTest(t, readEntered, "read worker did not reach the pre-submit gate")

				cancelDone := make(chan struct{})
				go func() {
					test.cancel(reader)
					close(cancelDone)
				}()
				waitWindowsReaderTest(t, cancelAttempted, "cancellation was not attempted before read submission")

				// The first cancellation has already found no submitted ReadFile.
				// Releasing the worker now verifies that cancellation keeps covering
				// the gap until the kernel operation actually exists.
				close(releaseRead)
				waitWindowsReaderTest(t, pollDone, "PollEvent remained blocked after cancellation")
				waitWindowsReaderTest(t, cancelDone, "reader cancellation remained blocked")

				_ = reader.Close()
				_ = readFile.Close()
				_ = writeFile.Close()
			}
		})
	}
}

func TestWindowsReaderPauseDropsCompletingPriorGeneration(t *testing.T) {
	readFile, writeFile, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readFile.Close()
	defer writeFile.Close()

	readerInterface, err := NewEventReader(readFile)
	if err != nil {
		t.Fatal(err)
	}
	reader := readerInterface.(*stdinReader)
	defer reader.Close()

	readEntered := make(chan struct{})
	releaseRead := make(chan struct{})
	reader.readFn = func(buffer []byte) (int, error) {
		close(readEntered)
		<-releaseRead
		buffer[0] = 'x'
		return 1, nil
	}

	type pollResult struct {
		event Event
		ok    bool
	}
	pollDone := make(chan pollResult, 1)
	go func() {
		event, ok := reader.PollEvent(-1)
		pollDone <- pollResult{event: event, ok: ok}
	}()
	waitWindowsReaderTest(t, readEntered, "read worker did not start")

	pauseDone := make(chan struct{})
	go func() {
		reader.Pause()
		close(pauseDone)
	}()
	waitWindowsReaderCondition(t, reader.paused.Load, "Pause did not establish the new generation")
	close(releaseRead)

	select {
	case result := <-pollDone:
		if result.ok || result.event != nil {
			t.Fatalf("prior-generation read escaped Pause: event=%#v ok=%v", result.event, result.ok)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PollEvent did not finish")
	}
	waitWindowsReaderTest(t, pauseDone, "Pause did not finish")

	reader.Resume()
	if event, ok := reader.PollEvent(0); ok || event != nil {
		t.Fatalf("prior-generation event survived Resume: event=%#v ok=%v", event, ok)
	}
}

func TestWindowsReaderPauseDropsPendingEvents(t *testing.T) {
	readFile, writeFile, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readFile.Close()
	defer writeFile.Close()

	readerInterface, err := NewEventReader(readFile)
	if err != nil {
		t.Fatal(err)
	}
	reader := readerInterface.(*stdinReader)
	defer reader.Close()

	if _, err := writeFile.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	assertWindowsReaderRune(t, reader, time.Second, 'a')
	reader.Pause()
	reader.Resume()
	if event, ok := reader.PollEvent(0); ok || event != nil {
		t.Fatalf("pending event from prior generation survived Pause: event=%#v ok=%v", event, ok)
	}

	// Interrupt has edge-triggered semantics: with no active read owner it
	// must not poison the first poll of the resumed generation.
	if err := reader.Interrupt(); err != nil {
		t.Fatal(err)
	}
	if _, err := writeFile.Write([]byte("c")); err != nil {
		t.Fatal(err)
	}
	assertWindowsReaderRune(t, reader, time.Second, 'c')
}

func TestWindowsReaderRepeatedTimeoutsLeaveNoReadOwner(t *testing.T) {
	readFile, writeFile, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readFile.Close()
	defer writeFile.Close()

	readerInterface, err := NewEventReader(readFile)
	if err != nil {
		t.Fatal(err)
	}
	reader := readerInterface.(*stdinReader)
	defer reader.Close()

	for iteration := 0; iteration < 25; iteration++ {
		if event, ok := reader.PollEvent(2 * time.Millisecond); ok || event != nil {
			t.Fatalf("timeout %d returned event=%#v ok=%v", iteration, event, ok)
		}
		reader.activeMu.Lock()
		active := reader.active
		reader.activeMu.Unlock()
		if active != nil {
			t.Fatalf("timeout %d left an active read owner", iteration)
		}
	}

	if _, err := writeFile.Write([]byte("z")); err != nil {
		t.Fatal(err)
	}
	assertWindowsReaderRune(t, reader, time.Second, 'z')
}

func assertWindowsReaderRune(t *testing.T, reader *stdinReader, timeout time.Duration, want rune) {
	t.Helper()
	event, ok := reader.PollEvent(timeout)
	key, keyOK := event.(KeyEvent)
	if !ok || !keyOK || key.Rune != want {
		t.Fatalf("PollEvent() = %#v, %v; want rune %q", event, ok, want)
	}
}

func waitWindowsReaderTest(t *testing.T, done <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal(message)
	}
}

func waitWindowsReaderCondition(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(time.Millisecond)
	}
}
