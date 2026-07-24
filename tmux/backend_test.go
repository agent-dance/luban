package tmux

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

func requirePOSIXShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("tmux backend fake command tests require POSIX shell scripts")
	}
}

// capturedCmd records the last tmux invocation for inspection.
// We override exec.Command by running a fake "tmux" script.

// fakeBackend returns a Backend wired to use a fake tmux binary that simply
// prints its arguments to stdout.  We write the fake binary to a temp dir
// and prepend that dir to PATH so exec.LookPath and exec.Command find it.
func fakeBackend(t *testing.T, socket string) *Backend {
	t.Helper()

	// Create a temp dir for the fake tmux script.
	dir := t.TempDir()
	fake := dir + "/tmux"

	// The fake script echoes all arguments separated by newlines to stdout.
	script := "#!/bin/sh\necho \"$@\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	// Prepend dir to PATH so our fake is picked up.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	inside := os.Getenv("TMUX") != ""
	return &Backend{socket: socket, insideTmux: inside}
}

// runArgs executes the fake backend and returns the arguments seen by tmux.
func runArgs(t *testing.T, b *Backend, subcmd ...string) []string {
	t.Helper()
	out, err := b.run(context.Background(), subcmd...)
	if err != nil {
		t.Fatalf("run %v: %v", subcmd, err)
	}
	return strings.Fields(out)
}

// ---- New() ----

func TestNew_InsideTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	b := New()
	if !b.InsideTmux() {
		t.Error("expected insideTmux=true when TMUX is set")
	}
	if b.Socket() != "" {
		t.Errorf("expected empty socket inside tmux, got %q", b.Socket())
	}
}

func TestNew_OutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	b := New()
	if b.InsideTmux() {
		t.Error("expected insideTmux=false when TMUX is unset")
	}
	if b.Socket() == "" {
		t.Error("expected non-empty socket outside tmux")
	}
}

// ---- Available() ----

func TestAvailable_WithFakeTmux(t *testing.T) {
	requirePOSIXShell(t)
	b := fakeBackend(t, "")

	if !b.Available() {
		t.Error("expected Available()=true with fake tmux on PATH")
	}
}

func TestAvailable_NoTmux(t *testing.T) {
	// Hide tmux by using an empty PATH.
	t.Setenv("PATH", "")

	b := &Backend{}
	if b.Available() {
		t.Error("expected Available()=false with empty PATH")
	}
}

// ---- Socket flag prepending ----

func TestArgs_NoSocket(t *testing.T) {
	b := &Backend{socket: ""}
	got := b.args("new-session", "-d")
	want := []string{"new-session", "-d"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("args without socket: got %v, want %v", got, want)
	}
}

func TestArgs_WithSocket(t *testing.T) {
	b := &Backend{socket: "my-socket"}
	got := b.args("new-session", "-d")
	want := []string{"-L", "my-socket", "new-session", "-d"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("args with socket: got %v, want %v", got, want)
	}
}

// ---- Command construction tests via real public API ----

// fakeBackendCapture returns a Backend whose fake tmux script captures all
// arguments to a file so we can inspect them after the call, even when the
// output contains shell-quoted tokens that strings.Fields would mis-split.
func fakeBackendCapture(t *testing.T, socket string) (*Backend, func() string) {
	t.Helper()
	dir := t.TempDir()
	fake := dir + "/tmux"
	capFile := dir + "/captured"

	// Write each argument on its own line so we can split reliably.
	script := "#!/bin/sh\nfor arg in \"$@\"; do echo \"$arg\" >> " + capFile + "; done\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	readArgs := func() string {
		data, _ := os.ReadFile(capFile)
		return string(data)
	}

	inside := os.Getenv("TMUX") != ""
	b := &Backend{socket: socket, insideTmux: inside}
	return b, readArgs
}

// mustContainLine checks that needle appears as one of the newline-separated
// lines in captured output.
func mustContainLine(t *testing.T, captured, needle string) {
	t.Helper()
	for _, line := range strings.Split(captured, "\n") {
		if line == needle {
			return
		}
	}
	t.Errorf("captured args do not contain line %q\nfull output:\n%s", needle, captured)
}

func TestCreateSession_CommandArgs(t *testing.T) {
	requirePOSIXShell(t)
	b, readArgs := fakeBackendCapture(t, "")

	_, err := b.CreateSession(context.Background(), "myteam")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got := readArgs()
	mustContainLine(t, got, "new-session")
	mustContainLine(t, got, "-d")
	mustContainLine(t, got, "-s")
	mustContainLine(t, got, "myteam")
	mustContainLine(t, got, "-P")
	mustContainLine(t, got, "-F")
}

func TestCreateSession_WithSocket(t *testing.T) {
	requirePOSIXShell(t)
	b, readArgs := fakeBackendCapture(t, "testsocket")

	_, err := b.CreateSession(context.Background(), "x")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got := readArgs()
	mustContainLine(t, got, "-L")
	mustContainLine(t, got, "testsocket")
	mustContainLine(t, got, "new-session")
}

func TestSplitPane_HorizontalArgs(t *testing.T) {
	requirePOSIXShell(t)
	b, readArgs := fakeBackendCapture(t, "")

	_, err := b.SplitPane(context.Background(), "%1", true, 50)
	if err != nil {
		t.Fatalf("SplitPane: %v", err)
	}
	got := readArgs()
	mustContainLine(t, got, "split-window")
	mustContainLine(t, got, "-h")
	mustContainLine(t, got, "-t")
	mustContainLine(t, got, "%1")
	mustContainLine(t, got, "50")
}

func TestSplitPane_VerticalArgs(t *testing.T) {
	requirePOSIXShell(t)
	b, readArgs := fakeBackendCapture(t, "")

	_, err := b.SplitPane(context.Background(), "%2", false, 30)
	if err != nil {
		t.Fatalf("SplitPane: %v", err)
	}
	got := readArgs()
	mustContainLine(t, got, "-v")
}

func TestSendKeys_CommandArgs(t *testing.T) {
	requirePOSIXShell(t)
	b, readArgs := fakeBackendCapture(t, "")

	if err := b.SendKeys(context.Background(), "%5", "echo hello"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	got := readArgs()
	mustContainLine(t, got, "send-keys")
	mustContainLine(t, got, "-t")
	mustContainLine(t, got, "%5")
	mustContainLine(t, got, "Enter")
}

func TestKillPane_CommandArgs(t *testing.T) {
	requirePOSIXShell(t)
	b, readArgs := fakeBackendCapture(t, "")

	if err := b.KillPane(context.Background(), "%3"); err != nil {
		t.Fatalf("KillPane: %v", err)
	}
	got := readArgs()
	mustContainLine(t, got, "kill-pane")
	mustContainLine(t, got, "%3")
}

func TestSelectLayout_CommandArgs(t *testing.T) {
	requirePOSIXShell(t)
	b, readArgs := fakeBackendCapture(t, "")

	if err := b.SelectLayout(context.Background(), "%1", "tiled"); err != nil {
		t.Fatalf("SelectLayout: %v", err)
	}
	got := readArgs()
	mustContainLine(t, got, "select-layout")
	mustContainLine(t, got, "tiled")
}

func TestResizePane_CommandArgs(t *testing.T) {
	requirePOSIXShell(t)
	b, readArgs := fakeBackendCapture(t, "")

	if err := b.ResizePane(context.Background(), "%1", 50); err != nil {
		t.Fatalf("ResizePane: %v", err)
	}
	got := readArgs()
	mustContainLine(t, got, "resize-pane")
	mustContainLine(t, got, "-x")
}

func TestGetLeaderPaneID_NotInTmux(t *testing.T) {
	b := &Backend{insideTmux: false}
	_, err := b.GetLeaderPaneID(context.Background())
	if err == nil {
		t.Error("expected error when not inside tmux")
	}
}

func TestGetLeaderPaneID_InTmux(t *testing.T) {
	requirePOSIXShell(t)
	b := fakeBackend(t, "")
	b.insideTmux = true

	// The fake tmux just echoes args; we test it runs without error.
	args := runArgs(t, b, "display-message", "-p", "#{pane_id}")
	mustContain(t, args, "display-message")
}

// ---- Run error handling ----

func TestRun_ErrorPropagates(t *testing.T) {
	requirePOSIXShell(t)
	// Use a binary that always exits 1.
	dir := t.TempDir()
	fake := dir + "/tmux"
	script := "#!/bin/sh\necho 'bad pane' >&2\nexit 1\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	b := &Backend{}
	_, err := b.run(context.Background(), "kill-pane", "-t", "%999")
	if err == nil {
		t.Error("expected error from failing tmux command")
	}
	if !strings.Contains(err.Error(), "bad pane") {
		t.Errorf("expected stderr in error message, got: %v", err)
	}
}

// ---- Helpers ----

func mustContain(t *testing.T, haystack []string, needle string) {
	t.Helper()
	for _, s := range haystack {
		if s == needle {
			return
		}
	}
	t.Errorf("args %v does not contain %q", haystack, needle)
}
