//go:build !darwin && !linux

package main

import (
	"os"
	"testing"
)

func task28SurfacePTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	t.Skip("task28 real-TUI PTY acceptance requires darwin or linux")
	return nil, nil
}
