package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/agent-dance/luban/i18n"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
	gotui "github.com/grindlemire/go-tui"
)

func writeToClipboardInLanguage(lang i18n.Language, text string) error {
	// Try platform-native clipboard first — works reliably in all terminal modes.
	platformErr := tryPlatformClipboardInLanguage(lang, text)
	if platformErr == nil {
		return nil
	}

	// Fallback to OSC52 for environments without native commands (e.g. SSH).
	if err := tryOSC52(text); err == nil {
		return nil
	}
	// Preserve the localized platform error rather than exposing an internal
	// terminal-owner lifecycle error to the UI.
	return platformErr
}

type terminalControlSinkFunc func([]byte) error

func (f terminalControlSinkFunc) WriteTerminalControl(sequence []byte) error {
	return f(sequence)
}

// tryOSC52 writes an OSC52 escape sequence through the active terminal owner.
// Returns nil when the sequence was accepted; the terminal clipboard cannot be
// queried portably to verify that it processed the sequence.
func tryOSC52(text string) error {
	return tryOSC52WithSink(text, terminalControlSinkFunc(gotui.WriteTerminalControl))
}

func tryOSC52WithSink(text string, sink gotui.TerminalControlSink) error {
	seq := osc52.New(text)

	// Detect tmux and wrap the sequence accordingly.
	if isTmux() {
		seq = seq.Tmux()
	} else if isScreen() {
		seq = seq.Screen()
	}

	var encoded bytes.Buffer
	if _, err := seq.WriteTo(&encoded); err != nil {
		return err
	}
	return sink.WriteTerminalControl(encoded.Bytes())
}

// isTmux returns true if running inside tmux.
func isTmux() bool {
	return os.Getenv("TMUX") != ""
}

// isScreen returns true if running inside GNU screen.
func isScreen() bool {
	term := os.Getenv("TERM")
	return strings.HasPrefix(term, "screen")
}

func tryPlatformClipboardInLanguage(lang i18n.Language, text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try wl-copy (Wayland) first, then xclip (X11), then xsel (X11)
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("%s", i18n.Text(lang, i18n.KeyRuntimeClipboardCommandMissing))
		}
	case "windows":
		cmd = exec.Command("clip.exe")
	default:
		return fmt.Errorf("%s", i18n.Format(lang, i18n.KeyRuntimeClipboardUnsupportedOS, runtime.GOOS))
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
