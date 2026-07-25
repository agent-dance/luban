package app

import (
	"bytes"
	"log"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// A permission denial already returns through the attributed tool-result
// pipeline. Wiring a second callback to stderr corrupts the alternate-screen
// renderer because stderr bypasses its front/back buffer.
func TestTUIDangerousCommandDenialDoesNotWriteStderr(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, forbidden := range []string{"checker.OnDeny =", "checker.SetOnDeny("} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("permission denial still installs an out-of-band terminal sink: %s", forbidden)
		}
	}
}

func TestApplicationRuntimeDoesNotTerminateTheProcess(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "os.Exit(") {
		t.Fatal("internal application runtime must return an exit code to cmd/luban-code")
	}
}

func TestInteractiveDiagnosticsUsePrivateBoundedSink(t *testing.T) {
	sink, restore := installInteractiveDiagnosticLogger()
	defer restore()

	log.Print("private standard diagnostic")
	slog.Error("private structured diagnostic", "cause", "private cause")
	snapshot := string(sink.Snapshot())
	for _, expected := range []string{"private standard diagnostic", "private structured diagnostic"} {
		if !strings.Contains(snapshot, expected) {
			t.Fatalf("private diagnostic sink omitted %q: %q", expected, snapshot)
		}
	}

	large := strings.Repeat("x", interactiveDiagnosticCapacity+1024)
	_, _ = sink.Write([]byte(large))
	if got := len(sink.Snapshot()); got != interactiveDiagnosticCapacity {
		t.Fatalf("private diagnostic retention = %d bytes, want %d", got, interactiveDiagnosticCapacity)
	}
}

func TestInteractiveDiagnosticsRedirectBothLoggersAndRestore(t *testing.T) {
	previousSlog := slog.Default()
	previousLogWriter, previousLogFlags, previousLogPrefix := log.Writer(), log.Flags(), log.Prefix()
	defer func() {
		slog.SetDefault(previousSlog)
		log.SetOutput(previousLogWriter)
		log.SetFlags(previousLogFlags)
		log.SetPrefix(previousLogPrefix)
	}()

	var processStderr bytes.Buffer
	log.SetOutput(&processStderr)
	slog.SetDefault(slog.New(slog.NewTextHandler(&processStderr, nil)))
	sink, restore := installInteractiveDiagnosticLogger()

	log.Print("interactive-standard-private")
	slog.Error("interactive-structured-private")
	if processStderr.Len() != 0 {
		t.Fatalf("interactive diagnostic reached process stderr: %q", processStderr.String())
	}
	private := string(sink.Snapshot())
	if !strings.Contains(private, "interactive-standard-private") || !strings.Contains(private, "interactive-structured-private") {
		t.Fatalf("interactive sink lost diagnostics: %q", private)
	}

	restore()
	log.Print("restored-standard")
	slog.Error("restored-structured")
	restored := processStderr.String()
	if !strings.Contains(restored, "restored-standard") || !strings.Contains(restored, "restored-structured") {
		t.Fatalf("diagnostic logger lifecycle was not restored: %q", restored)
	}
}
