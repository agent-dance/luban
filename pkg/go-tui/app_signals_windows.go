//go:build windows

package tui

import "os"

func appTerminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
