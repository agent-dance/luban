//go:build unix

package tui

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestUnixReaderInterruptCoalescesWithoutPoll(t *testing.T) {
	reader, closeInput := newInterruptTestReader(t)
	defer closeInput()

	reader.Pause()
	done := make(chan error, 1)
	go func() {
		for range 250_000 {
			if err := reader.Interrupt(); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Interrupt() failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("high-frequency Interrupt blocked with no active PollEvent")
	}
	if !reader.interruptPending.Load() {
		t.Fatal("Interrupt did not retain a coalesced wakeup")
	}

	pauseDone := make(chan struct{})
	go func() {
		reader.Pause()
		close(pauseDone)
	}()
	select {
	case <-pauseDone:
	case <-time.After(time.Second):
		t.Fatal("Pause blocked with a pending interrupt")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- reader.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close blocked with a pending interrupt")
	}
	if err := reader.EnableInterrupt(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("EnableInterrupt() after Close = %v, want os.ErrClosed", err)
	}
}

func TestUnixReaderCoalescedInterruptCancelsPollEvent(t *testing.T) {
	reader, closeInput := newInterruptTestReader(t)
	defer closeInput()
	defer reader.Close()

	for range 10_000 {
		if err := reader.Interrupt(); err != nil {
			t.Fatal(err)
		}
	}
	if event, ok := reader.PollEvent(time.Second); ok || event != nil {
		t.Fatalf("interrupted PollEvent() = (%#v, %v), want (nil, false)", event, ok)
	}
	if reader.interruptPending.Load() {
		t.Fatal("PollEvent did not consume the coalesced interrupt")
	}

	pollDone := make(chan struct{})
	go func() {
		event, ok := reader.PollEvent(InputLatencyBlocking)
		if ok || event != nil {
			t.Errorf("interrupted blocking PollEvent() = (%#v, %v), want (nil, false)", event, ok)
		}
		close(pollDone)
	}()
	time.Sleep(10 * time.Millisecond)
	if err := reader.Interrupt(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pollDone:
	case <-time.After(time.Second):
		t.Fatal("Interrupt did not cancel blocking PollEvent")
	}
}

func TestUnixReaderPauseDropsPendingEventsFromPreviousGeneration(t *testing.T) {
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
	event, ok := reader.PollEvent(time.Second)
	key, keyOK := event.(KeyEvent)
	if !ok || !keyOK || key.Rune != 'a' {
		t.Fatalf("first input = %#v, ok=%v", event, ok)
	}

	reader.Pause()
	reader.Resume()
	if event, ok := reader.PollEvent(0); ok || event != nil {
		t.Fatalf("old-generation input survived pause: (%#v, %v)", event, ok)
	}
}

func newInterruptTestReader(t *testing.T) (*stdinReader, func()) {
	t.Helper()
	readFile, writeFile, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	readerInterface, err := NewEventReader(readFile)
	if err != nil {
		readFile.Close()
		writeFile.Close()
		t.Fatal(err)
	}
	reader := readerInterface.(*stdinReader)
	if err := reader.EnableInterrupt(); err != nil {
		readFile.Close()
		writeFile.Close()
		t.Fatal(err)
	}
	return reader, func() {
		readFile.Close()
		writeFile.Close()
	}
}
