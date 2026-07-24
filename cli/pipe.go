package cli

import (
	"os"
)

// IsStdinTerminal reports whether os.Stdin is connected to a TTY.
func IsStdinTerminal() bool {
	return isTerminal(os.Stdin)
}

// IsStdoutTerminal reports whether os.Stdout is connected to a TTY.
func IsStdoutTerminal() bool {
	return isTerminal(os.Stdout)
}

// IsInteractive reports whether both stdin and stdout are TTYs.
// When false, the process is likely receiving piped input or writing to a pipe.
func IsInteractive() bool {
	return IsStdinTerminal() && IsStdoutTerminal()
}

// isTerminal returns true if f is a character device (TTY).
// It uses os.File.Stat() and checks ModeCharDevice to avoid adding
// a dependency on golang.org/x/term (which is already an indirect dep
// via charmbracelet/x/term, but not a direct one).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
