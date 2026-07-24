package tui

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

type serializedControlTerminal struct {
	*MockTerminal
	active    atomic.Int32
	maxActive atomic.Int32
	mu        sync.Mutex
	chunks    []string
}

func (t *serializedControlTerminal) WriteDirect(sequence []byte) (int, error) {
	active := t.active.Add(1)
	for {
		maximum := t.maxActive.Load()
		if active <= maximum || t.maxActive.CompareAndSwap(maximum, active) {
			break
		}
	}
	// Make an interleaving deterministic when the App forgets terminalMu.
	runtime.Gosched()
	t.mu.Lock()
	t.chunks = append(t.chunks, string(sequence))
	t.mu.Unlock()
	t.active.Add(-1)
	return len(sequence), nil
}

func TestAppWriteTerminalControlSerializesCompleteSequences(t *testing.T) {
	terminal := &serializedControlTerminal{MockTerminal: NewMockTerminal(80, 24)}
	app := &App{terminal: terminal}
	app.opened.Store(true)

	const writers = 64
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			if err := app.WriteTerminalControl([]byte("\x1b]52;c;payload\a")); err != nil {
				t.Errorf("WriteTerminalControl: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := terminal.maxActive.Load(); got != 1 {
		t.Fatalf("concurrent terminal writes = %d, want 1", got)
	}
	terminal.mu.Lock()
	defer terminal.mu.Unlock()
	if got := len(terminal.chunks); got != writers {
		t.Fatalf("complete control chunks = %d, want %d", got, writers)
	}
	for i, chunk := range terminal.chunks {
		if chunk != "\x1b]52;c;payload\a" {
			t.Fatalf("chunk %d interleaved or truncated: %q", i, chunk)
		}
	}
}

func TestAppWriteTerminalControlRespectsTerminalHandoff(t *testing.T) {
	terminal := &serializedControlTerminal{MockTerminal: NewMockTerminal(80, 24)}
	app := &App{terminal: terminal}
	app.opened.Store(true)
	app.externalActive.Store(true)

	err := app.WriteTerminalControl([]byte{'\a'})
	if !errors.Is(err, ErrTerminalControlUnavailable) {
		t.Fatalf("error = %v, want ErrTerminalControlUnavailable", err)
	}
	terminal.mu.Lock()
	defer terminal.mu.Unlock()
	if len(terminal.chunks) != 0 {
		t.Fatalf("terminal handoff received control writes: %q", terminal.chunks)
	}
}

func TestAppTerminalControlLeaseRoutesGlobalWrites(t *testing.T) {
	terminal := &serializedControlTerminal{MockTerminal: NewMockTerminal(80, 24)}
	app := &App{terminal: terminal}
	app.opened.Store(true)
	app.installTerminalControlSink()
	t.Cleanup(app.releaseTerminalControlSink)

	if err := WriteTerminalControl([]byte("\x1b]9;owned\a")); err != nil {
		t.Fatalf("WriteTerminalControl: %v", err)
	}
	terminal.mu.Lock()
	defer terminal.mu.Unlock()
	if len(terminal.chunks) != 1 || terminal.chunks[0] != "\x1b]9;owned\a" {
		t.Fatalf("terminal chunks = %q", terminal.chunks)
	}
}

type recordingControlSink struct {
	mu     sync.Mutex
	writes []string
}

func (s *recordingControlSink) WriteTerminalControl(sequence []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes = append(s.writes, string(sequence))
	return nil
}

func TestTerminalControlLeaseRestoresPreviousOwner(t *testing.T) {
	base := &recordingControlSink{}
	temporary := &recordingControlSink{}
	releaseBase := InstallTerminalControlSink(base)
	t.Cleanup(releaseBase)
	releaseTemporary := InstallTerminalControlSink(temporary)

	if err := WriteTerminalControl([]byte("temporary")); err != nil {
		t.Fatalf("temporary write: %v", err)
	}
	releaseTemporary()
	if err := WriteTerminalControl([]byte("base")); err != nil {
		t.Fatalf("base write: %v", err)
	}

	temporary.mu.Lock()
	defer temporary.mu.Unlock()
	if len(temporary.writes) != 1 || temporary.writes[0] != "temporary" {
		t.Fatalf("temporary writes = %q", temporary.writes)
	}
	base.mu.Lock()
	defer base.mu.Unlock()
	if len(base.writes) != 1 || base.writes[0] != "base" {
		t.Fatalf("base writes = %q", base.writes)
	}
}
