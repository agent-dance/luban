package tui

import (
	"bytes"
	"os"
	"testing"
)

type clipboardControlSink struct {
	sequence []byte
}

func (s *clipboardControlSink) WriteTerminalControl(sequence []byte) error {
	s.sequence = append([]byte(nil), sequence...)
	return nil
}

func TestTryOSC52UsesTerminalControlSinkWithoutStderr(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("TERM", "xterm-256color")
	sink := &clipboardControlSink{}

	stderr := captureClipboardStderr(t, func() {
		if err := tryOSC52WithSink("copied text", sink); err != nil {
			t.Fatalf("tryOSC52WithSink: %v", err)
		}
	})
	if stderr != "" {
		t.Fatalf("OSC52 wrote ordinary stderr text: %q", stderr)
	}
	if !bytes.HasPrefix(sink.sequence, []byte("\x1b]52;")) || !bytes.HasSuffix(sink.sequence, []byte{'\a'}) {
		t.Fatalf("terminal-control sequence = %q, want OSC52 framing", sink.sequence)
	}
}

func captureClipboardStderr(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stderr
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = write
	defer func() { os.Stderr = original }()

	fn()
	if err := write.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	var captured bytes.Buffer
	if _, err := captured.ReadFrom(read); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if err := read.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return captured.String()
}
