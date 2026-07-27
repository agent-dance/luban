package cli

import (
	"io"
	"os"
)

// CommandIO owns the process writers for standalone, non-interactive command
// entry points. Commands pass these writers into testable run functions rather
// than reaching around the terminal lifecycle from implementation packages.
type CommandIO struct {
	Stdout io.Writer
	Stderr io.Writer
}

// ProcessCommandIO binds a standalone command to the process output streams.
func ProcessCommandIO() CommandIO {
	return CommandIO{Stdout: os.Stdout, Stderr: os.Stderr}
}
