//go:build !unix && !windows

package ui

import (
	"io"
	"sync/atomic"
)

func scanScreenReaderInput(input io.Reader, lines chan<- screenReaderLine, stop <-chan struct{}, done chan<- struct{}, sequence *atomic.Uint64) {
	scanScreenReaderInputGeneric(input, lines, stop, done, sequence)
}
