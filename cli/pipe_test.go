package cli_test

import (
	"os"
	"testing"

	"github.com/agent-dance/luban/cli"
)

// TestIsStdinTerminal_pipe checks that a pipe is NOT detected as a terminal.
// We replace os.Stdin with the read end of an os.Pipe(), which is never a TTY.
func TestIsStdinTerminal_pipe(t *testing.T) {
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	os.Stdin = r
	if cli.IsStdinTerminal() {
		t.Error("IsStdinTerminal() returned true for a pipe; expected false")
	}
}

// TestIsStdoutTerminal_pipe checks that a pipe is NOT detected as a terminal.
func TestIsStdoutTerminal_pipe(t *testing.T) {
	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	os.Stdout = w
	if cli.IsStdoutTerminal() {
		t.Error("IsStdoutTerminal() returned true for a pipe; expected false")
	}
}

// TestIsInteractive_bothPipes checks that IsInteractive returns false when
// both stdin and stdout are pipes.
func TestIsInteractive_bothPipes(t *testing.T) {
	origStdin := os.Stdin
	origStdout := os.Stdout
	defer func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
	}()

	r1, w1, _ := os.Pipe()
	r2, w2, _ := os.Pipe()
	defer r1.Close()
	defer w1.Close()
	defer r2.Close()
	defer w2.Close()

	os.Stdin = r1
	os.Stdout = w2

	if cli.IsInteractive() {
		t.Error("IsInteractive() returned true when both stdin/stdout are pipes; expected false")
	}
}
